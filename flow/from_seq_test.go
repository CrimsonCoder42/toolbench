package flow_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func ExampleFromSeq() {
	ctx := context.Background()
	out, wait := flow.FromSeq(ctx, slices.Values([]string{"a", "b", "c"}))
	for v := range out {
		fmt.Println(v)
	}
	fmt.Println(wait())
	// Output:
	// a
	// b
	// c
	// <nil>
}

func TestFromSeq_cases(t *testing.T) {
	tests := []struct {
		name string
		in   []int
	}{{"zero", nil}, {"one", []int{7}}, {"many", []int{1, 2, 3, 4, 5, 6, 7, 8, 9}}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := testCtx(t)
			defer cancel()
			out, wait := flow.FromSeq(ctx, slices.Values(tc.in))
			got := collect(t, ctx, out)
			if !slices.Equal(got, tc.in) || wait() != nil {
				t.Fatalf("got %v, %v, want %v, nil", got, wait(), tc.in)
			}
		})
	}
}

func TestFromSeq_nilSeq(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	out, wait := flow.FromSeq[int](ctx, nil)
	if got := collect(t, ctx, out); len(got) != 0 || wait() != nil {
		t.Fatalf("got %v, %v, want nothing, nil", got, wait())
	}
}

// go_56: the generator must stop, and run its cleanup, when the consumer stops (here by cancelling ctx).
func TestFromSeq_earlyBreakStopsGenerator(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	cleaned := make(chan struct{})
	seq := func(yield func(int) bool) {
		defer close(cleaned)
		for i := 0; ; i++ {
			if !yield(i) {
				return
			}
		}
	}
	out, wait := flow.FromSeq(ctx, seq)
	for range 3 {
		<-out
	}
	cancel()
	collect(t, bound, out) // closed after cancel
	if err := wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait = %v, want context.Canceled", err)
	}
	select {
	case <-cleaned: // wait returned after seq returned, so this is already closed
	default:
		t.Fatal("seq's deferred cleanup did not run after cancel")
	}
}

func TestFromSeq_cancelBefore(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	out, wait := flow.FromSeq(ctx, count())
	if got := collect(t, bound, out); len(got) != 0 {
		t.Fatalf("got %d values after cancel, want none", len(got))
	}
	if err := wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait = %v, want context.Canceled", err)
	}
}

func TestFromSeq_cancelAfter(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	out, wait := flow.FromSeq(ctx, upTo(5))
	got := collect(t, bound, out)
	cancel()
	if len(got) != 5 || wait() != nil {
		t.Fatalf("got %v, %v, want 5 values, nil", got, wait())
	}
}

func TestFromSeq_panicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	seq := func(yield func(int) bool) {
		yield(1)
		panic("boom")
	}
	out, wait := flow.FromSeq(ctx, seq)
	if got := collect(t, ctx, out); !slices.Equal(got, []int{1}) {
		t.Fatalf("got %v, want [1]", got)
	}
	err := wait()
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("wait = %v, want ErrPanic with the value", err)
	}
	if wait() != err {
		t.Fatal("second wait returned a different error")
	}
}

func TestFromSeq_withBuffer(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	out, wait := flow.FromSeq(ctx, upTo(10), flow.WithBuffer(4))
	if c := cap(out); c != 4 {
		t.Fatalf("cap = %d, want 4", c)
	}
	if got := collect(t, ctx, out); len(got) != 10 || wait() != nil {
		t.Fatalf("got %d values, want 10", len(got))
	}
}
