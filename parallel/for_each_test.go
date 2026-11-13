package parallel_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/parallel"
)

func ExampleForEach() {
	var sum atomic.Int64
	err := parallel.ForEach(context.Background(), slices.Values([]int64{1, 2, 3, 4}), 2, func(_ context.Context, v int64) error {
		sum.Add(v)
		return nil
	})
	fmt.Println(sum.Load(), err)
	// Output:
	// 10 <nil>
}

func TestForEach_everyValueOnce(t *testing.T) {
	for _, w := range []int{1, 3, 50, 0} {
		t.Run(fmt.Sprint("workers=", w), func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := testCtx(t)
			defer cancel()
			seen := make([]atomic.Int64, 20)
			var m maxTracker
			err := parallel.ForEach(ctx, upTo(20), w, func(_ context.Context, v int) error {
				m.enter()
				defer m.leave()
				seen[v].Add(1)
				return nil
			})
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			for i := range seen {
				if n := seen[i].Load(); n != 1 {
					t.Fatalf("value %d seen %d times, want 1", i, n)
				}
			}
			if w == 1 && m.max.Load() != 1 {
				t.Fatalf("workers=1: max in flight %d", m.max.Load())
			}
		})
	}
}

func TestForEach_firstErrorCancelsAndStopsIn(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	var pulled atomic.Int64
	var stopped atomic.Bool
	err := parallel.ForEach(ctx, counted(&pulled, &stopped), 4, func(c context.Context, v int) error {
		switch {
		case v == 0:
			return errBoom
		default:
			<-c.Done()
			return c.Err()
		}
	})
	if !errors.Is(err, errBoom) || !stopped.Load() {
		t.Fatalf("err = %v, stopped %v; want errBoom, true", err, stopped.Load())
	}
	if p := pulled.Load(); p > 1+4+1 {
		t.Fatalf("in advanced %d values; bound %d", p, 1+4+1)
	}
}

func TestForEach_panics(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if err := parallel.ForEach(ctx, upTo(3), 2, func(context.Context, int) error { panic("fn broke") }); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("panic in fn: err = %v", err)
	}
	if err := parallel.ForEach(ctx, panicky(2), 2, func(context.Context, int) error { return nil }); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("panic in seq: err = %v", err)
	}
	if err := parallel.ForEach[int](ctx, upTo(1), 2, nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("nil fn: err = %v", err)
	}
	if err := parallel.ForEach[int](ctx, nil, 2, nil); err != nil {
		t.Fatalf("nil in: err = %v", err)
	}
}

func TestForEach_cancelMid(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	var pulled atomic.Int64
	var stopped atomic.Bool
	started := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- parallel.ForEach(ctx, counted(&pulled, &stopped), 3, func(c context.Context, v int) error {
			select {
			case started <- struct{}{}:
			default:
			}
			<-c.Done()
			return nil // ignores the cancel: the ctx's error is still returned
		})
	}()
	await(t, bound, started)
	cancel()
	if err := await(t, bound, done); !errors.Is(err, context.Canceled) || !stopped.Load() {
		t.Fatalf("err = %v, stopped %v; want context.Canceled, true", err, stopped.Load())
	}
}
