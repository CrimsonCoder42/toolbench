package flow_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func ExampleOrDone() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3 // in is never closed: OrDone ends the range when ctx is cancelled
	for v := range flow.OrDone(ctx, in) {
		fmt.Println(v)
		if v == 3 {
			cancel()
		}
	}
	// Output:
	// 1
	// 2
	// 3
}

func TestOrDone_nilClosedOneMany(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if got := collect(t, ctx, flow.OrDone[int](ctx, nil)); len(got) != 0 {
		t.Fatalf("nil: %v", got)
	}
	if got := collect(t, ctx, flow.OrDone(ctx, filled[int]())); len(got) != 0 {
		t.Fatalf("closed: %v", got)
	}
	if got := collect(t, ctx, flow.OrDone(ctx, filled(9))); !slices.Equal(got, []int{9}) {
		t.Fatalf("one: %v", got)
	}
	if got := collect(t, ctx, flow.OrDone(ctx, filled(1, 2, 3, 4))); !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Fatalf("many: %v", got)
	}
}

func TestOrDone_cancelWithSilentInput(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	out := flow.OrDone(ctx, make(chan int)) // never sends, never closes
	cancel()
	collect(t, bound, out)
}

func TestOrDone_cancelBefore(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	if got := collect(t, bound, flow.OrDone(ctx, filled(1, 2))); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
