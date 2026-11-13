package flow_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func ExampleBridge() {
	ctx := context.Background()
	chans := filled(filled(1, 2), nil, filled(3), filled(4, 5))
	fmt.Println(slices.Collect(flow.ToSeq(ctx, flow.Bridge(ctx, chans))))
	// Output:
	// [1 2 3 4 5]
}

func TestBridge_nilAndEmpty(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if got := collect(t, ctx, flow.Bridge[int](ctx, nil)); len(got) != 0 {
		t.Fatalf("nil: %v", got)
	}
	if got := collect(t, ctx, flow.Bridge(ctx, filled[<-chan int]())); len(got) != 0 {
		t.Fatalf("closed: %v", got)
	}
	if got := collect(t, ctx, flow.Bridge(ctx, filled(filled[int](), filled[int]()))); len(got) != 0 {
		t.Fatalf("empty inners: %v", got)
	}
}

func TestBridge_ordered(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	chans := make(chan (<-chan int))
	go func() {
		defer close(chans)
		for k := range 10 {
			select {
			case chans <- source(ctx, func(yield func(int) bool) {
				for i := range 10 {
					if !yield(k*10 + i) {
						return
					}
				}
			}):
			case <-ctx.Done():
				return
			}
		}
	}()
	got := collect(t, ctx, flow.Bridge(ctx, chans))
	want := slices.Collect(upTo(100))
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want 0..99 in order", got)
	}
}

func TestBridge_cancelMidOpenInner(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	inner := filledOpen(1) // never closed
	chans := filledOpen[<-chan int](inner)
	out := flow.Bridge(ctx, chans)
	if v := <-out; v != 1 {
		t.Fatalf("got %d", v)
	}
	cancel()
	collect(t, bound, out)
}
