package flow_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func square(_ context.Context, v int) (int, error) { return v * v, nil }

func ExampleFanOut() {
	ctx := context.Background()
	nums, waitGen := flow.FromSeq(ctx, slices.Values([]int{1, 2, 3, 4}))
	out, wait := flow.FanOut(ctx, nums, 3, square)
	got := slices.Sorted(flow.ToSeq(ctx, out)) // completion order: sort before printing
	fmt.Println(got, wait(), waitGen())
	// Output:
	// [1 4 9 16] <nil> <nil>
}

func TestFanOut_cases(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		workers int
	}{
		{"zero items", 0, 4},
		{"one item", 1, 4},
		{"many items", 100, 4},
		{"workers=1", 50, 1},
		{"more workers than items", 3, 10},
		{"workers<1 is 1", 10, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := testCtx(t)
			defer cancel()
			out, wait := flow.FanOut(ctx, source(ctx, upTo(tc.n)), tc.workers, square)
			got := collect(t, ctx, out)
			if err := wait(); err != nil {
				t.Fatalf("wait = %v", err)
			}
			var want []int
			for i := range tc.n {
				want = append(want, i*i)
			}
			if tc.workers <= 1 {
				if !slices.Equal(got, want) { // one worker keeps input order
					t.Fatalf("got %v, want %v in order", got, want)
				}
				return
			}
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Fatalf("got %v, want %v (each exactly once)", got, want)
			}
		})
	}
}

func TestFanOut_nilAndClosedInput(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	for _, in := range []<-chan int{nil, filled[int]()} {
		out, wait := flow.FanOut(ctx, in, 3, square)
		if got := collect(t, ctx, out); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
		if err := wait(); err != nil {
			t.Fatalf("wait = %v", err)
		}
	}
}

// go_25: one goroutine per item. Here at most workers calls of fn run at once.
func TestFanOut_boundedWorkers_go25(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	const workers, items = 3, 30
	var active, peak atomic.Int32
	started := make(chan struct{}, items)
	gate := make(chan struct{})
	fn := func(ctx context.Context, v int) (int, error) {
		n := active.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		started <- struct{}{}
		<-gate
		active.Add(-1)
		return v, nil
	}
	out, wait := flow.FanOut(ctx, source(ctx, upTo(items)), workers, fn, flow.WithBuffer(items))
	for range workers {
		<-started // every worker is inside fn
	}
	close(gate)
	if got := collect(t, ctx, out); len(got) != items {
		t.Fatalf("got %d values, want %d", len(got), items)
	}
	if err := wait(); err != nil {
		t.Fatal(err)
	}
	if p := peak.Load(); p != workers {
		t.Fatalf("peak concurrent fn calls = %d, want %d", p, workers)
	}
}

// go_25: the collector had to know the item count; here the channel is closed instead.
func TestFanOut_outputClosedNoCountNeeded(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	out, wait := flow.FanOut(ctx, source(ctx, upTo(17)), 4, square)
	n := 0
	for range flow.ToSeq(ctx, out) {
		n++
	}
	if n != 17 || wait() != nil {
		t.Fatalf("got %d values", n)
	}
}

// A slow consumer holds the producer back: at every read, produced - consumed stays within the in-flight bound
// (FromSeq holds one value, the worker holds one, the seq has counted the next). Checked at each step, no sleep.
func TestFanOut_slowConsumerBackpressure(t *testing.T) {
	for _, buf := range []int{0, 5} {
		t.Run(fmt.Sprintf("buffer=%d", buf), func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := testCtx(t)
			defer cancel()
			var produced atomic.Int64
			seq := func(yield func(int) bool) {
				for i := range 1000 {
					produced.Add(1)
					if !yield(i) {
						return
					}
				}
			}
			in := source(ctx, seq, flow.WithBuffer(buf))
			out, wait := flow.FanOut(ctx, in, 1, square, flow.WithBuffer(buf))
			bound := int64(3 + 2*buf)
			consumed := int64(0)
			for range flow.ToSeq(ctx, out) {
				consumed++
				if p := produced.Load(); p-consumed > bound {
					t.Fatalf("produced %d with %d consumed: more than %d ahead, no backpressure", p, consumed, bound)
				}
			}
			if consumed != 1000 || wait() != nil {
				t.Fatalf("consumed %d", consumed)
			}
		})
	}
}

func TestFanOut_firstErrorWins(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	errBoom := errors.New("boom")
	fn := func(ctx context.Context, v int) (int, error) {
		if v == 5 {
			return 0, errBoom
		}
		return v, nil
	}
	out, wait := flow.FanOut(ctx, source(ctx, count()), 3, fn)
	collect(t, ctx, out)
	if err := wait(); !errors.Is(err, errBoom) {
		t.Fatalf("wait = %v, want errBoom", err)
	}
}

func TestFanOut_panicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	fn := func(ctx context.Context, v int) (int, error) {
		if v == 3 {
			panic("worker exploded")
		}
		return v, nil
	}
	out, wait := flow.FanOut(ctx, source(ctx, count()), 2, fn)
	collect(t, ctx, out)
	err := wait()
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "worker exploded" {
		t.Fatalf("wait = %v, want ErrPanic with the value", err)
	}
	if err2 := wait(); err2 != err {
		t.Fatalf("second wait = %v, want the same error", err2)
	}
}

func TestFanOut_nilFnIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	out, wait := flow.FanOut[int, int](ctx, filled(1), 1, nil)
	collect(t, ctx, out)
	if err := wait(); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("wait = %v, want ErrPanic", err)
	}
}

func TestFanOut_cancelBefore(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	var calls atomic.Int32
	fn := func(ctx context.Context, v int) (int, error) { calls.Add(1); return v, nil }
	out, wait := flow.FanOut(ctx, filled(1, 2, 3), 2, fn)
	if got := collect(t, bound, out); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if err := wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait = %v, want context.Canceled", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("fn called %d times after cancel", calls.Load())
	}
}

func TestFanOut_cancelMid(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	out, wait := flow.FanOut(ctx, source(ctx, count()), 4, square)
	<-out
	<-out
	cancel()
	collect(t, bound, out)
	if err := wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait = %v, want context.Canceled", err)
	}
}

func TestFanOut_cancelAfter(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	out, wait := flow.FanOut(ctx, source(ctx, upTo(5)), 2, square)
	collect(t, bound, out)
	if err := wait(); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := wait(); err != nil {
		t.Fatalf("wait after cancel = %v, want nil", err)
	}
}

func BenchmarkFanOut(b *testing.B) {
	b.ReportAllocs()
	ctx := b.Context()
	for b.Loop() {
		out, wait := flow.FanOut(ctx, source(ctx, upTo(1000)), 4, square)
		for range out {
		}
		if err := wait(); err != nil {
			b.Fatal(err)
		}
	}
}
