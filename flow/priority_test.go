package flow_test

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func ExamplePriority() {
	ctx := context.Background()
	high, low := filled("h1", "h2"), filled("l1", "l2")
	fmt.Println(slices.Collect(flow.ToSeq(ctx, flow.Priority(ctx, high, low))))
	// Output:
	// [h1 h2 l1 l2]
}

// Strictness: with every input pre-filled, all of a higher input comes before any of a lower one.
func TestPriority_strictWithPrefilledInputs(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	got := collect(t, ctx, flow.Priority(ctx, filled(1, 2, 3), nil, filled(10, 20), filled(100)))
	if want := []int{1, 2, 3, 10, 20, 100}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Strictness while inputs stay open: the high input is served first, then the low one once high is empty.
func TestPriority_strictWithOpenInputs(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	high, low := filledOpen(1, 2, 3), filledOpen(10, 20, 30)
	out := flow.Priority(ctx, high, low)
	var got []int
	for range 4 {
		got = append(got, <-out)
	}
	if want := []int{1, 2, 3, 10}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	cancel()
	collect(t, bound, out)
}

func TestPriority_noInputsAndNil(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if got := collect(t, ctx, flow.Priority[int](ctx)); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if got := collect(t, ctx, flow.Priority[int](ctx, nil, filled[int]())); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

// The waiting path: unbuffered inputs fed by goroutines; every value arrives once, in order per input.
func TestPriority_everyValueFromSlowProducers(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	a := source(ctx, upTo(300))
	b := source(ctx, func(yield func(int) bool) {
		for i := range 300 {
			if !yield(1000 + i) {
				return
			}
		}
	})
	var fromA, fromB []int
	for _, v := range collect(t, ctx, flow.Priority(ctx, a, b)) {
		if v >= 1000 {
			fromB = append(fromB, v-1000)
		} else {
			fromA = append(fromA, v)
		}
	}
	want := slices.Collect(upTo(300))
	if !slices.Equal(fromA, want) || !slices.Equal(fromB, want) {
		t.Fatalf("got %d from a, %d from b, want 300 each in order", len(fromA), len(fromB))
	}
}

func TestPriority_cancelBeforeAndMid(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	if got := collect(t, bound, flow.Priority(ctx, filled(1, 2))); len(got) != 0 {
		t.Fatalf("before: got %v", got)
	}
	ctx2, cancel2 := context.WithCancel(bound)
	silent := make(chan int) // the merger waits on it with a receiver goroutine
	out := flow.Priority(ctx2, silent, filledOpen(5))
	if v := <-out; v != 5 {
		t.Fatalf("got %d", v)
	}
	cancel2()
	collect(t, bound, out)
}

// Regression (reviewer BLOCKER 2): a value a one-shot receiver has already taken from the high input must not be
// passed over for values of the low input. Deterministic set-up: inside a synctest bubble, synctest.Wait returns
// when the merger and its two receivers are blocked; with GOMAXPROCS(1) the test goroutine keeps running after
// the first send to low readies the low receiver, so `high <- 100` is handed straight to the high receiver while
// both receivers hold a value. The merger must then stop and wait for both receivers and send 100 first.
// (GOMAXPROCS is set and restored inside this test; it is not run with t.Parallel.)
func TestPriority_heldHighValueBeatsReadyLow(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		high, low := make(chan int), make(chan int, 10)
		out := flow.Priority(ctx, high, low)
		synctest.Wait() // the merger waits on its receivers
		for i := range 10 {
			low <- i
		}
		high <- 100
		close(high)
		close(low)
		var got []int
		for v := range out {
			got = append(got, v)
		}
		if want := []int{100, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}
