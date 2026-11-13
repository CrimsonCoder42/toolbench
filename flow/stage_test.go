package flow_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func ExampleStage() {
	ctx := context.Background()
	nums, waitGen := flow.FromSeq(ctx, slices.Values([]int{1, 2, 3}))
	squares, wait1 := flow.Stage(ctx, nums, square)
	words, wait2 := flow.Stage(ctx, squares, func(_ context.Context, v int) (string, error) {
		return "n=" + strconv.Itoa(v), nil
	})
	for w := range words {
		fmt.Println(w)
	}
	fmt.Println(waitGen(), wait1(), wait2())
	// Output:
	// n=1
	// n=4
	// n=9
	// <nil> <nil> <nil>
}

func TestStage_keepsInputOrder(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	out, wait := flow.Stage(ctx, source(ctx, upTo(200)), square)
	got := collect(t, ctx, out)
	for i, v := range got {
		if v != i*i {
			t.Fatalf("got[%d] = %d, want %d", i, v, i*i)
		}
	}
	if len(got) != 200 || wait() != nil {
		t.Fatalf("got %d values", len(got))
	}
}

func TestStage_nilAndClosed(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	for _, in := range []<-chan int{nil, filled[int]()} {
		out, wait := flow.Stage(ctx, in, square)
		if got := collect(t, ctx, out); len(got) != 0 || wait() != nil {
			t.Fatalf("got %v", got)
		}
	}
}

// go_21: a consumer that stops early left every stage blocked on its send. Here cancel stops the whole
// pipeline; leak.Verify fails the test if any stage goroutine is left.
func TestStage_earlyConsumerExitNoLeak(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	s1, w1 := flow.Stage(ctx, source(ctx, count()), square)
	s2, w2 := flow.Stage(ctx, s1, square)
	s3, w3 := flow.Stage(ctx, s2, square)
	for range 3 {
		<-s3
	}
	cancel()
	collect(t, bound, s3)
	for _, w := range []func() error{w1, w2, w3} {
		if err := w(); !errors.Is(err, context.Canceled) {
			t.Fatalf("wait = %v, want context.Canceled", err)
		}
	}
}

func TestStage_panicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	out, wait := flow.Stage(ctx, filled(1, 2, 3), func(context.Context, int) (int, error) { panic("stage") })
	collect(t, ctx, out)
	if err := wait(); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("wait = %v, want ErrPanic", err)
	}
}

func TestStage_cancelBeforeAndAfter(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	out, wait := flow.Stage(ctx, filled(1, 2), square)
	if got := collect(t, bound, out); len(got) != 0 || !errors.Is(wait(), context.Canceled) {
		t.Fatalf("before: got %v, %v", got, wait())
	}
	ctx2, cancel2 := context.WithCancel(bound)
	out2, wait2 := flow.Stage(ctx2, filled(1, 2), square)
	got := collect(t, bound, out2)
	cancel2()
	if !slices.Equal(got, []int{1, 4}) || wait2() != nil {
		t.Fatalf("after: got %v, %v", got, wait2())
	}
}
