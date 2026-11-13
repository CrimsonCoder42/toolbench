package parallel_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/CrimsonCoder42/calcbench/algo/seq"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/parallel"
)

func even(_ context.Context, v int) (bool, error) { return v%2 == 0, nil }

func ExampleFilter() {
	got, err := parallel.Filter(context.Background(), slices.Values([]int{1, 2, 3, 4, 5, 6}), 3, even)
	fmt.Println(got, err)
	// Output:
	// [2 4 6] <nil>
}

func TestFilter_equalsCalcbenchSeq(t *testing.T) {
	for _, w := range []int{1, 2, 5, 300, 0} {
		t.Run(fmt.Sprint("workers=", w), func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := testCtx(t)
			defer cancel()
			want := slices.Collect(seq.Filter(upTo(200), func(v int) bool { return v%3 == 0 }))
			got, err := parallel.Filter(ctx, upTo(200), w, func(_ context.Context, v int) (bool, error) { return v%3 == 0, nil })
			if err != nil || !slices.Equal(got, want) {
				t.Fatalf("got %v, %v; want %v", got, err, want)
			}
		})
	}
}

func TestFilter_edges(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if got, err := parallel.Filter(ctx, upTo(0), 2, even); got != nil || err != nil {
		t.Fatalf("zero items: got %v, %v", got, err)
	}
	if got, err := parallel.Filter(ctx, slices.Values([]int{1, 3}), 2, even); got != nil || err != nil {
		t.Fatalf("none kept: got %v, %v", got, err)
	}
	if got, err := parallel.Filter[int](ctx, nil, 2, even); got != nil || err != nil {
		t.Fatalf("nil in: got %v, %v", got, err)
	}
	if _, err := parallel.Filter[int](ctx, upTo(2), 2, nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("nil keep: err = %v, want ErrPanic", err)
	}
}

func TestFilter_orderKeptWhenCompletionIsReversed(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	gates := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})}
	started := make(chan int)
	finished := make(chan int)
	done := make(chan result[[]int], 1)
	go func() {
		v, err := parallel.Filter(ctx, upTo(4), 4, func(_ context.Context, v int) (bool, error) {
			started <- v
			<-gates[v]
			finished <- v
			return v != 1, nil
		})
		done <- result[[]int]{v, err}
	}()
	for range 4 {
		await(t, ctx, started)
	}
	for _, i := range []int{3, 2, 1, 0} {
		close(gates[i])
		if got := await(t, ctx, finished); got != i {
			t.Fatalf("finished %d, want %d", got, i)
		}
	}
	r := await(t, ctx, done)
	if r.err != nil || !slices.Equal(r.v, []int{0, 2, 3}) {
		t.Fatalf("got %v, %v; want [0 2 3]", r.v, r.err)
	}
}

func TestFilter_firstErrorCancelsAndStopsIn(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	var pulled atomic.Int64
	var stopped atomic.Bool
	got, err := parallel.Filter(ctx, counted(&pulled, &stopped), 2, func(c context.Context, v int) (bool, error) {
		switch {
		case v == 2:
			return false, errBoom
		case v > 2:
			<-c.Done()
			return false, c.Err()
		}
		return true, nil
	})
	if !errors.Is(err, errBoom) || got != nil || !stopped.Load() {
		t.Fatalf("got %v, %v, stopped %v; want nil, errBoom, true", got, err, stopped.Load())
	}
}

func TestFilter_panics(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	_, err := parallel.Filter(ctx, upTo(5), 2, func(_ context.Context, v int) (bool, error) {
		if v == 4 {
			panic("keep broke")
		}
		return true, nil
	})
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("panic in keep: err = %v", err)
	}
	if _, err := parallel.Filter(ctx, panicky(3), 2, even); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("panic in seq: err = %v", err)
	}
}

func TestFilter_cancelBefore(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := parallel.Filter(ctx, upTo(5), 2, even); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, %v; want nil, context.Canceled", got, err)
	}
}
