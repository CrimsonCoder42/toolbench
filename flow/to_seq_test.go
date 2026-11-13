package flow_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
)

func ExampleToSeq() {
	ctx := context.Background()
	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)
	fmt.Println(slices.Collect(flow.ToSeq(ctx, in)))
	// Output:
	// [1 2 3]
}

func TestToSeq_nilAndClosed(t *testing.T) {
	ctx := t.Context()
	if got := slices.Collect(flow.ToSeq[int](ctx, nil)); len(got) != 0 {
		t.Fatalf("nil: got %v", got)
	}
	if got := slices.Collect(flow.ToSeq(ctx, filled[int]())); len(got) != 0 {
		t.Fatalf("closed: got %v", got)
	}
	if got := slices.Collect(flow.ToSeq(ctx, filled(5))); !slices.Equal(got, []int{5}) {
		t.Fatalf("one: got %v", got)
	}
}

func TestToSeq_breakLeavesRest(t *testing.T) {
	in := filled(1, 2, 3, 4)
	seq := flow.ToSeq(t.Context(), in)
	for v := range seq {
		if v == 2 {
			break
		}
	}
	if got := slices.Collect(seq); !slices.Equal(got, []int{3, 4}) {
		t.Fatalf("second range got %v, want [3 4]", got)
	}
}

func TestToSeq_cancelBefore(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got := slices.Collect(flow.ToSeq(ctx, filled(1, 2, 3))); len(got) != 0 {
		t.Fatalf("got %v, want nothing after cancel", got)
	}
}

func TestToSeq_cancelMid(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	in := filledOpen(1, 2, 3) // open: without cancel the range would block
	var got []int
	for v := range flow.ToSeq(ctx, in) {
		got = append(got, v)
		if v == 2 {
			cancel()
		}
	}
	if !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("got %v, want [1 2]", got)
	}
}

func TestToSeq_cancelAfter(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	got := slices.Collect(flow.ToSeq(ctx, filled(1, 2)))
	cancel()
	if !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("got %v", got)
	}
}
