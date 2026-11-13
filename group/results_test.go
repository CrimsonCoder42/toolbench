package group_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/CrimsonCoder42/toolbench/group"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestResults_zeroValue(t *testing.T) {
	leak.Verify(t)
	var r group.Results[int]
	for i := range 5 {
		r.Go(func(context.Context) (int, error) { return i * 10, nil })
	}
	got, err := r.Wait()
	if err != nil || !slices.Equal(got, []int{0, 10, 20, 30, 40}) {
		t.Fatalf("Wait = %v, %v", got, err)
	}
}

func TestResults_waitWithoutGo(t *testing.T) {
	r := group.NewResults[string](t.Context())
	got, err := r.Wait()
	if err != nil || got != nil {
		t.Fatalf("Wait = %v, %v; want nil, nil", got, err)
	}
}

// Each function finishes only after the one submitted after it: completion order is the reverse of
// submission order, and Wait still returns submission order.
func TestResults_submissionOrder(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const n = 16
	r := group.NewResults[int](ctx)
	done := make([]chan struct{}, n+1)
	for i := range done {
		done[i] = make(chan struct{})
	}
	close(done[n])
	var completed []int
	var order atomic.Int64
	completedAt := make([]int64, n)
	for i := range n {
		r.Go(func(ctx context.Context) (int, error) {
			select {
			case <-done[i+1]:
			case <-ctx.Done():
				return 0, context.Cause(ctx)
			}
			completedAt[i] = order.Add(1)
			close(done[i])
			return i, nil
		})
	}
	got, err := r.Wait()
	if err != nil {
		t.Fatalf("Wait error = %v", err)
	}
	for i := range n {
		completed = append(completed, i)
		if completedAt[i] != int64(n-i) {
			t.Fatalf("function %d completed %dth, want %dth", i, completedAt[i], n-i)
		}
	}
	if !slices.Equal(got, completed) {
		t.Fatalf("Wait = %v, want %v", got, completed)
	}
}

func TestResults_firstErrorReturnsNil(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	r := group.NewResults[int](tctx)
	r.Go(func(context.Context) (int, error) { return 1, nil })
	r.Go(func(context.Context) (int, error) { return 0, errA })
	r.Go(func(ctx context.Context) (int, error) {
		select {
		case <-ctx.Done(): // cancelled by errA
			return 0, errB
		case <-tctx.Done():
			return 0, tctx.Err()
		}
	})
	got, err := r.Wait()
	if !errors.Is(err, errA) || got != nil {
		t.Fatalf("Wait = %v, %v; want nil, %v", got, err, errA)
	}
}

func TestResults_panicReturnsErrPanic(t *testing.T) {
	leak.Verify(t)
	r := group.NewResults[int](t.Context())
	r.Go(func(context.Context) (int, error) { return 1, nil })
	r.Go(func(context.Context) (int, error) { panic("boom") })
	got, err := r.Wait()
	if !errors.Is(err, panics.ErrPanic) || got != nil {
		t.Fatalf("Wait = %v, %v; want nil, ErrPanic", got, err)
	}
}

func TestResults_panicNilFunc(t *testing.T) {
	leak.Verify(t)
	r := group.NewResults[int](t.Context())
	r.Go(nil)
	if _, err := r.Wait(); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Wait error = %v, want ErrPanic", err)
	}
}

// The go_61 defect fixed in the library: a function whose goroutine starts after the ctx is done is not
// called, and the cause is reported, never a silent success.
func TestResults_cancelBeforeRecordsCause(t *testing.T) {
	leak.Verify(t)
	errShutdown := errors.New("shutdown")
	parent, cancel := context.WithCancelCause(t.Context())
	cancel(errShutdown)
	r := group.NewResults[int](parent)
	var called atomic.Int64
	for range 3 {
		r.Go(func(context.Context) (int, error) { called.Add(1); return 1, nil })
	}
	got, err := r.Wait()
	if !errors.Is(err, errShutdown) || got != nil {
		t.Fatalf("Wait = %v, %v; want nil, %v", got, err, errShutdown)
	}
	if called.Load() != 0 {
		t.Fatalf("called %d functions after cancel, want 0", called.Load())
	}
}

func TestResults_cancelDuring(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	parent, cancel := context.WithCancelCause(tctx)
	r := group.NewResults[int](parent)
	running := make(chan struct{})
	r.Go(func(ctx context.Context) (int, error) {
		close(running)
		<-ctx.Done()
		return 0, context.Cause(ctx)
	})
	recv(t, tctx, running)
	cancel(errB)
	if _, err := r.Wait(); !errors.Is(err, errB) {
		t.Fatalf("Wait error = %v, want %v", err, errB)
	}
}

func TestResults_cancelAfter(t *testing.T) {
	leak.Verify(t)
	parent, cancel := context.WithCancel(t.Context())
	r := group.NewResults[int](parent)
	r.Go(func(context.Context) (int, error) { return 7, nil })
	got, err := r.Wait()
	cancel()
	if err != nil || !slices.Equal(got, []int{7}) {
		t.Fatalf("Wait = %v, %v", got, err)
	}
}

func TestResults_WithLimitBoundsConcurrency(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const limit, tasks = 2, 10
	r := group.NewResults[int](ctx, group.WithLimit(limit))
	var f inFlight
	started := make(chan struct{}, tasks)
	release := make(chan struct{})
	submitted := make(chan struct{})
	go func() {
		defer close(submitted)
		for i := range tasks {
			r.Go(func(context.Context) (int, error) {
				f.enter()
				defer f.leave()
				started <- struct{}{}
				<-release
				return i, nil
			})
		}
	}()
	for range limit {
		recv(t, ctx, started)
	}
	close(release)
	recv(t, ctx, submitted)
	got, err := r.Wait()
	if err != nil || len(got) != tasks {
		t.Fatalf("Wait = %v, %v", got, err)
	}
	if f.max.Load() != limit {
		t.Fatalf("max in flight = %d, want %d", f.max.Load(), limit)
	}
}

func TestResults_WithLimitZero(t *testing.T) {
	leak.Verify(t)
	r := group.NewResults[int](t.Context(), group.WithLimit(0))
	r.Go(func(context.Context) (int, error) { return 1, nil })
	if _, err := r.Wait(); !errors.Is(err, group.ErrZeroLimit) {
		t.Fatalf("Wait error = %v, want ErrZeroLimit", err)
	}
}

func TestResults_nilContextAndOption(t *testing.T) {
	leak.Verify(t)
	r := group.NewResults[int](nil, nil, group.WithLimit(1)) //nolint:staticcheck // nil ctx is the input under test
	r.Go(func(ctx context.Context) (int, error) { return 3, ctx.Err() })
	got, err := r.Wait()
	if err != nil || !slices.Equal(got, []int{3}) {
		t.Fatalf("Wait = %v, %v", got, err)
	}
}

func TestResults_waitReturnsACopy(t *testing.T) {
	leak.Verify(t)
	var r group.Results[int]
	r.Go(func(context.Context) (int, error) { return 1, nil })
	a, _ := r.Wait()
	a[0] = 99
	b, _ := r.Wait()
	if b[0] != 1 {
		t.Fatalf("second Wait = %v, want [1]", b)
	}
}
