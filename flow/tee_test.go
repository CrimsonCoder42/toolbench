package flow_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func ExampleTee() {
	ctx := context.Background()
	outs, err := flow.Tee(ctx, filled("x", "y"), 2)
	if err != nil {
		fmt.Println(err)
		return
	}
	// One goroutine may take each value from the outputs in turn.
	for range 2 {
		b, a := <-outs[1], <-outs[0]
		fmt.Println(a, b)
	}
	// Output:
	// x x
	// y y
}

// go_62: the tee reset its "already sent" markers inside the inner loop, so one side could get a value twice
// and the other none. Here every output receives every value exactly once, in order.
func TestTee_eachValueExactlyOncePerOutput_go62(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	const n, items = 3, 1000
	outs, err := flow.Tee(ctx, source(ctx, upTo(items)), n)
	if err != nil {
		t.Fatal(err)
	}
	got := make([][]int, n)
	var wg sync.WaitGroup
	for k, out := range outs {
		wg.Go(func() {
			for v := range flow.ToSeq(ctx, out) {
				got[k] = append(got[k], v)
			}
		})
	}
	wg.Wait()
	want := slices.Collect(upTo(items))
	for k := range n {
		if !slices.Equal(got[k], want) {
			t.Fatalf("output %d got %d values (want each of 0..%d exactly once, in order)", k, len(got[k]), items-1)
		}
	}
}

func TestTee_oneGoroutineAlternating(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	outs, err := flow.Tee(ctx, filled(1, 2, 3), 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{1, 2, 3} {
		if b, a := <-outs[1], <-outs[0]; a != want || b != want {
			t.Fatalf("got %d, %d, want %d twice", a, b, want)
		}
	}
	collect(t, ctx, outs[0])
	collect(t, ctx, outs[1])
}

func TestTee_invalidN(t *testing.T) {
	for _, n := range []int{0, -1} {
		outs, err := flow.Tee(t.Context(), filled(1), n)
		if !errors.Is(err, flow.ErrInvalid) || outs != nil {
			t.Fatalf("n=%d: got %v, %v, want nil, ErrInvalid", n, outs, err)
		}
	}
}

func TestTee_nilClosedAndOne(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	for _, in := range []<-chan int{nil, filled[int]()} {
		outs, err := flow.Tee(ctx, in, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range outs {
			if got := collect(t, ctx, o); len(got) != 0 {
				t.Fatalf("got %v", got)
			}
		}
	}
	outs, err := flow.Tee(ctx, filled(4, 5), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := collect(t, ctx, outs[0]); !slices.Equal(got, []int{4, 5}) {
		t.Fatalf("n=1: got %v", got)
	}
}

// go_62 again: the tee had no done case, so a consumer that went away blocked it forever.
func TestTee_cancelWithGoneConsumer(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	outs, err := flow.Tee(ctx, source(ctx, count()), 2)
	if err != nil {
		t.Fatal(err)
	}
	<-outs[0] // outs[1] is never read
	cancel()
	collect(t, bound, outs[0])
	collect(t, bound, outs[1])
}

func TestTee_withBufferDecouples(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	outs, err := flow.Tee(ctx, filled(1, 2, 3), 2, flow.WithBuffer(3))
	if err != nil {
		t.Fatal(err)
	}
	// With a buffer of 3, one output can be drained before the other is read.
	if got := collect(t, ctx, outs[0]); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("got %v", got)
	}
	if got := collect(t, ctx, outs[1]); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("got %v", got)
	}
}
