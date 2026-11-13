package flow_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func ExampleMerge() {
	ctx := context.Background()
	out := flow.Merge(ctx, filled(1, 2), filled(3), nil, filled(4, 5))
	fmt.Println(slices.Sorted(flow.ToSeq(ctx, out))) // interleaving is unspecified: sort before printing
	// Output:
	// [1 2 3 4 5]
}

func TestMerge_noInputsClosed(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if got := collect(t, ctx, flow.Merge[int](ctx)); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if got := collect(t, ctx, flow.Merge[int](ctx, nil, nil)); len(got) != 0 {
		t.Fatalf("nil inputs: got %v", got)
	}
}

func TestMerge_everyValueOnceAndPerInputOrder(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	type tagged struct{ in, i int }
	const inputs, each = 5, 200
	var ins []<-chan tagged
	for k := range inputs {
		vs := make([]tagged, each)
		for i := range each {
			vs[i] = tagged{k, i}
		}
		ins = append(ins, source(ctx, slices.Values(vs)))
	}
	next := make([]int, inputs)
	for _, v := range collect(t, ctx, flow.Merge(ctx, ins...)) {
		if v.i != next[v.in] {
			t.Fatalf("input %d: got %d, want %d (order within an input)", v.in, v.i, next[v.in])
		}
		next[v.in]++
	}
	for k, n := range next {
		if n != each {
			t.Fatalf("input %d delivered %d, want %d", k, n, each)
		}
	}
}

func TestMerge_cancelMid(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	never := make(chan int) // never sends, never closes
	out := flow.Merge(ctx, source(ctx, count()), never)
	<-out
	cancel()
	collect(t, bound, out)
}

func TestMerge_cancelBefore(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	if got := collect(t, bound, flow.Merge(ctx, filled(1, 2, 3))); len(got) != 0 {
		t.Fatalf("got %v after cancel", got)
	}
}
