package batch_test

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/batch"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func ExampleChannel() {
	in := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		in <- i
	}
	close(in)
	for b := range batch.Channel(context.Background(), in, 2, 0) {
		fmt.Println(b)
	}
	// Output:
	// [1 2]
	// [3 4]
	// [5]
}

var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

func TestChannel_sizeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		n, size int
		want    [][]int
	}{
		{0, 3, nil},
		{1, 3, [][]int{{1}}},
		{3, 3, [][]int{{1, 2, 3}}},
		{4, 3, [][]int{{1, 2, 3}, {4}}},
		{7, 3, [][]int{{1, 2, 3}, {4, 5, 6}, {7}}},
		{3, 1, [][]int{{1}, {2}, {3}}},
		{3, 0, [][]int{{1}, {2}, {3}}}, // size below 1 is 1
		{2, -5, [][]int{{1}, {2}}},     // size below 1 is 1
		{2, 1 << 40, [][]int{{1, 2}}},  // a huge size costs nothing until it fills
	} {
		t.Run(fmt.Sprintf("n=%d,size=%d", tc.n, tc.size), func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := testCtx(t)
			defer cancel()
			got := collect(t, ctx, batch.Channel(ctx, filled(seqOf(tc.n)...), tc.size, 0))
			if !slices.EqualFunc(got, tc.want, slices.Equal) {
				t.Errorf("batches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestChannel_orderAndCountWithProducer(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	const n = 1000
	in := make(chan int)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(in)
		for i := 1; i <= n; i++ {
			select {
			case in <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	var flat []int
	for _, b := range collect(t, ctx, batch.Channel(ctx, in, 7, 0)) {
		if len(b) == 0 || len(b) > 7 {
			t.Fatalf("batch of %d values, want 1..7", len(b))
		}
		flat = append(flat, b...)
	}
	<-done
	if !slices.Equal(flat, seqOf(n)) {
		t.Errorf("values out of order, lost or repeated: got %d values", len(flat))
	}
}

func TestChannel_freshSlicePerBatch(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	got := collect(t, ctx, batch.Channel(ctx, filled(1, 2, 3, 4, 5), 2, 0))
	got[0][0] = 100
	got[0] = append(got[0], 99) // appending to a kept batch must not write into the next one
	if want := [][]int{{100, 2, 99}, {3, 4}, {5}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("batches = %v, want %v (batches share memory)", got, want)
	}
}

func TestChannel_nilIn(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if got := collect(t, ctx, batch.Channel[int](ctx, nil, 3, time.Second)); len(got) != 0 {
		t.Errorf("nil in: batches = %v, want none", got)
	}
}

func TestChannel_closedEmptyIn(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if got := collect(t, ctx, batch.Channel(ctx, filled[int](), 3, time.Second)); len(got) != 0 {
		t.Errorf("closed empty in: batches = %v, want none", got)
	}
}

func TestChannel_nilCtx(t *testing.T) {
	leak.Verify(t)
	bound, cancel := testCtx(t)
	defer cancel()
	var nilCtx context.Context // documented as context.Background()
	got := collect(t, bound, batch.Channel(nilCtx, filled(1, 2, 3), 2, 0))
	if want := [][]int{{1, 2}, {3}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("batches = %v, want %v", got, want)
	}
}

func TestChannel_maxWaitFlushesPartial(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	fake := clock.NewFake(t0)
	in := make(chan int)
	out := batch.Channel(ctx, in, 10, time.Second, batch.WithClock(fake))

	send(t, ctx, in, 1) // arms the batch timer: due at t0 + 1s
	send(t, ctx, in, 2)
	if err := fake.BlockUntil(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := fake.AdvanceContext(ctx, time.Second-time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select { // not due yet: the goroutine is still reading in, not sending
	case in <- 3:
	case b := <-out:
		t.Fatalf("batch %v sent before maxWait", b)
	case <-ctx.Done():
		t.Fatal("bound")
	}
	if err := fake.AdvanceContext(ctx, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if b := recv(t, ctx, out); !slices.Equal(b, []int{1, 2, 3}) {
		t.Errorf("batch = %v, want [1 2 3]", b)
	}
	close(in)
	if rest := collect(t, ctx, out); len(rest) != 0 {
		t.Errorf("after close: %v, want none", rest)
	}
}

// After a size flush the timer of that batch is stopped; the next batch's maxWait counts from its own first
// value, not from the first batch's.
func TestChannel_timerRestartsPerBatch(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	fake := clock.NewFake(t0)
	in := make(chan int)
	out := batch.Channel(ctx, in, 2, time.Second, batch.WithClock(fake))

	send(t, ctx, in, 1) // timer due at t0 + 1s
	if err := fake.BlockUntil(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := fake.AdvanceContext(ctx, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	send(t, ctx, in, 2)
	if b := recv(t, ctx, out); !slices.Equal(b, []int{1, 2}) {
		t.Fatalf("batch = %v, want [1 2] (by size)", b)
	}
	send(t, ctx, in, 3) // timer due at t0 + 1.5s
	if err := fake.BlockUntil(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := fake.AdvanceContext(ctx, 600*time.Millisecond); err != nil { // t0 + 1.1s: past the stale deadline
		t.Fatal(err)
	}
	select {
	case in <- 4:
	case b := <-out:
		t.Fatalf("batch %v sent by the first batch's stale timer", b)
	case <-ctx.Done():
		t.Fatal("bound")
	}
	if b := recv(t, ctx, out); !slices.Equal(b, []int{3, 4}) {
		t.Errorf("batch = %v, want [3 4]", b)
	}
	close(in)
	collect(t, ctx, out)
}

// countingClock counts the timers Channel creates.
type countingClock struct {
	*clock.Fake
	timers atomic.Int32
}

func (c *countingClock) NewTimer(d time.Duration) clock.Timer {
	c.timers.Add(1)
	return c.Fake.NewTimer(d)
}

func TestChannel_maxWaitZeroIsSizeOnly(t *testing.T) {
	for _, maxWait := range []time.Duration{0, -time.Second} {
		t.Run(maxWait.String(), func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := testCtx(t)
			defer cancel()
			cc := &countingClock{Fake: clock.NewFake(t0)}
			in := make(chan int)
			out := batch.Channel(ctx, in, 3, maxWait, batch.WithClock(cc))
			send(t, ctx, in, 1)
			send(t, ctx, in, 2)
			close(in)
			got := collect(t, ctx, out)
			if want := [][]int{{1, 2}}; !slices.EqualFunc(got, want, slices.Equal) {
				t.Errorf("batches = %v, want %v", got, want)
			}
			if n := cc.timers.Load(); n != 0 {
				t.Errorf("%d timers created, want 0", n)
			}
		})
	}
}

func TestChannel_fullBatchNeedsNoTimer(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	cc := &countingClock{Fake: clock.NewFake(t0)}
	in := make(chan int)
	out := batch.Channel(ctx, in, 1, time.Second, batch.WithClock(cc))
	for i := 1; i <= 5; i++ {
		send(t, ctx, in, i)
		recv(t, ctx, out)
	}
	close(in)
	collect(t, ctx, out)
	if n := cc.timers.Load(); n != 0 {
		t.Errorf("%d timers created for size-1 batches, want 0 (a full batch needs no timer)", n)
	}
}

func TestChannel_slowConsumerBackpressure(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	in := make(chan int)
	out := batch.Channel(ctx, in, 1, 0)
	send(t, ctx, in, 1) // batch [1] waits for the consumer; in is not read meanwhile
	select {
	case in <- 2:
		t.Fatal("in was read while a batch waited for the consumer")
	default:
	}
	if b := recv(t, ctx, out); !slices.Equal(b, []int{1}) {
		t.Errorf("batch = %v, want [1]", b)
	}
	close(in)
	collect(t, ctx, out)
}

func TestChannel_cancelBefore(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	if got := collect(t, bound, batch.Channel(ctx, filled(1, 2, 3), 1, 0)); len(got) != 0 {
		t.Errorf("batches = %v, want none", got)
	}
}

func TestChannel_cancelMidBatchDropsPartial(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	fake := clock.NewFake(t0)
	in := make(chan int) // never closed: only ctx ends the batcher
	out := batch.Channel(ctx, in, 10, time.Second, batch.WithClock(fake))
	send(t, bound, in, 1)
	send(t, bound, in, 2)
	cancel()
	if got := collect(t, bound, out); len(got) != 0 {
		t.Errorf("batches = %v, want none (the partial batch is dropped)", got)
	}
}

func TestChannel_cancelWhileSending(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	in := make(chan int)
	out := batch.Channel(ctx, in, 1, 0)
	send(t, bound, in, 1) // [1] is now being sent to a consumer that is not reading
	cancel()
	if got := collect(t, bound, out); len(got) > 1 {
		t.Errorf("batches = %v, want at most the one in flight", got)
	}
}

func TestChannel_cancelAfter(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	got := collect(t, bound, batch.Channel(ctx, filled(1, 2, 3), 2, 0))
	cancel()
	if want := [][]int{{1, 2}, {3}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("batches = %v, want %v", got, want)
	}
}

func BenchmarkChannel(b *testing.B) {
	b.ReportAllocs()
	ctx := context.Background()
	for b.Loop() {
		in := make(chan int, 1000)
		for i := range 1000 {
			in <- i
		}
		close(in)
		for range batch.Channel(ctx, in, 100, 0) {
		}
	}
}
