package group_test

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/group"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var (
	errA = errors.New("a failed")
	errB = errors.New("b failed")
)

// bounded returns a context that ends the test if a wait hangs.
func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// recv waits for one value on ch or fails the test when ctx ends.
func recv[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-ctx.Done():
		t.Fatalf("timed out waiting: %v", context.Cause(ctx))
		var zero T
		return zero
	}
}

// inFlight tracks how many functions run at once and the most seen.
type inFlight struct {
	now, max atomic.Int64
}

func (f *inFlight) enter() {
	n := f.now.Add(1)
	for {
		m := f.max.Load()
		if n <= m || f.max.CompareAndSwap(m, n) {
			return
		}
	}
}

func (f *inFlight) leave() { f.now.Add(-1) }

func TestGroup_zeroValue(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	var n atomic.Int64
	for range 10 {
		g.Go(func() error { n.Add(1); return nil })
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
	if n.Load() != 10 {
		t.Fatalf("ran %d, want 10", n.Load())
	}
}

func TestGroup_waitWithoutGo(t *testing.T) {
	var g group.Group
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
	g2, ctx := group.WithContext(t.Context())
	if err := g2.Wait(); err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatalf("cause after Wait = %v, want context.Canceled", context.Cause(ctx))
	}
}

func TestGroup_firstErrorWinsAndCancels(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	g, ctx := group.WithContext(tctx)
	g.Go(func() error { return errA })
	g.Go(func() error {
		select {
		case <-ctx.Done():
			return errB // after the first error: dropped
		case <-tctx.Done():
			return tctx.Err()
		}
	})
	if err := g.Wait(); !errors.Is(err, errA) {
		t.Fatalf("Wait = %v, want %v", err, errA)
	}
	if cause := context.Cause(ctx); !errors.Is(cause, errA) {
		t.Fatalf("context.Cause = %v, want %v", cause, errA)
	}
}

func TestGroup_waitWaitsForAll(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	g, ctx := group.WithContext(tctx)
	const n = 20
	var finished atomic.Int64
	for range n {
		g.Go(func() error {
			select {
			case <-ctx.Done(): // only after the failing function below
			case <-tctx.Done():
			}
			finished.Add(1)
			return nil
		})
	}
	g.Go(func() error { return errA })
	if err := g.Wait(); !errors.Is(err, errA) {
		t.Fatalf("Wait = %v, want %v", err, errA)
	}
	if got := finished.Load(); got != n {
		t.Fatalf("finished before Wait returned = %d, want %d", got, n)
	}
}

func TestGroup_SetLimitBoundsConcurrency(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const limit, tasks = 3, 12
	var g group.Group
	g.SetLimit(limit)
	var f inFlight
	started := make(chan struct{}, tasks)
	release := make(chan struct{})
	submitted := make(chan struct{})
	go func() {
		defer close(submitted)
		for range tasks {
			g.Go(func() error {
				f.enter()
				defer f.leave()
				started <- struct{}{}
				<-release
				return nil
			})
		}
	}()
	for range limit {
		recv(t, ctx, started)
	}
	if g.TryGo(func() error { return nil }) {
		t.Fatal("TryGo at the limit = true, want false")
	}
	close(release)
	recv(t, ctx, submitted)
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	if got := f.max.Load(); got != limit {
		t.Fatalf("max in flight = %d, want %d", got, limit)
	}
	if len(started) != tasks-limit {
		t.Fatalf("started after release = %d, want %d", len(started), tasks-limit)
	}
}

func TestGroup_SetLimitNegativeIsNoLimit(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const tasks = 50
	var g group.Group
	g.SetLimit(2)
	g.SetLimit(-1)
	started := make(chan struct{}, tasks)
	release := make(chan struct{})
	for range tasks {
		g.Go(func() error { started <- struct{}{}; <-release; return nil })
	}
	for range tasks { // all 50 run at once: no limit
		recv(t, ctx, started)
	}
	close(release)
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
}

func TestGroup_SetLimitZero(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetLimit(0)
	if g.TryGo(func() error { return nil }) {
		t.Fatal("TryGo with limit 0 = true, want false")
	}
	ran := false
	g.Go(func() error { ran = true; return nil }) // returns at once, does not block forever
	if err := g.Wait(); !errors.Is(err, group.ErrZeroLimit) {
		t.Fatalf("Wait = %v, want ErrZeroLimit", err)
	}
	if ran {
		t.Fatal("fn ran with limit 0")
	}
}

func TestGroup_SetLimitZeroReleasesWaitingGo(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var g group.Group
	g.SetLimit(1)
	release := make(chan struct{})
	running := make(chan struct{})
	g.Go(func() error { close(running); <-release; return nil })
	recv(t, ctx, running)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		g.Go(func() error { return nil }) // waits for a slot
	}()
	g.SetLimit(0) // the waiting Go re-checks and records ErrZeroLimit
	recv(t, ctx, returned)
	close(release)
	if err := g.Wait(); !errors.Is(err, group.ErrZeroLimit) {
		t.Fatalf("Wait = %v, want ErrZeroLimit", err)
	}
}

func TestGroup_SetLimitWhileActiveDoesNotPanic(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var g group.Group
	g.SetLimit(2)
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	for range 2 {
		g.Go(func() error { started <- struct{}{}; <-release; return nil })
	}
	recv(t, ctx, started)
	recv(t, ctx, started)
	g.SetLimit(1) // errgroup panics here; ours applies the limit to later calls
	if g.TryGo(func() error { return nil }) {
		t.Fatal("TryGo with 2 active and limit 1 = true, want false")
	}
	g.SetLimit(3)
	if !g.TryGo(func() error { return nil }) {
		t.Fatal("TryGo with 2 active and limit 3 = false, want true")
	}
	close(release)
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
}

func TestGroup_TryGo(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var g group.Group
	g.SetLimit(1)
	release := make(chan struct{})
	running := make(chan struct{})
	if !g.TryGo(func() error { close(running); <-release; return nil }) {
		t.Fatal("TryGo below the limit = false, want true")
	}
	recv(t, ctx, running)
	if g.TryGo(func() error { return nil }) {
		t.Fatal("TryGo at the limit = true, want false")
	}
	close(release)
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	if !g.TryGo(func() error { return errA }) {
		t.Fatal("TryGo after the slot freed = false, want true")
	}
	if err := g.Wait(); !errors.Is(err, errA) {
		t.Fatalf("Wait = %v, want %v", err, errA)
	}
}

func TestGroup_panicReturnsErrPanic(t *testing.T) {
	leak.Verify(t)
	g, ctx := group.WithContext(t.Context())
	g.Go(func() error { panic("boom") })
	err := g.Wait()
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Wait = %v, want ErrPanic", err)
	}
	var pe *panics.PanicError
	if !errors.As(err, &pe) || pe.Value != "boom" || len(pe.Stack) == 0 {
		t.Fatalf("Wait = %#v, want *PanicError with value boom and a stack", err)
	}
	if !errors.Is(context.Cause(ctx), panics.ErrPanic) {
		t.Fatalf("cause = %v, want ErrPanic", context.Cause(ctx))
	}
}

func TestGroup_panicNilFunc(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.Go(nil)
	if err := g.Wait(); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Wait = %v, want ErrPanic", err)
	}
}

func TestGroup_panicDoesNotLoseSlot(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetLimit(1)
	g.Go(func() error { panic(errA) })
	g.Go(func() error { return nil }) // would block forever if the panic leaked the slot
	err := g.Wait()
	if !errors.Is(err, panics.ErrPanic) || !errors.Is(err, errA) {
		t.Fatalf("Wait = %v, want ErrPanic wrapping %v", err, errA)
	}
}

func TestGroup_goexitDoesNotHang(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetLimit(1)
	g.Go(func() error { runtime.Goexit(); return errA })
	g.Go(func() error { return nil }) // the Goexit released its slot
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v, want nil (Goexit is not an error, as errgroup)", err)
	}
}

func TestGroup_nilContext(t *testing.T) {
	leak.Verify(t)
	g, ctx := group.WithContext(nil) //nolint:staticcheck // a nil ctx is the input under test
	g.Go(func() error { return errA })
	if err := g.Wait(); !errors.Is(err, errA) {
		t.Fatalf("Wait = %v", err)
	}
	if !errors.Is(context.Cause(ctx), errA) {
		t.Fatalf("cause = %v", context.Cause(ctx))
	}
}

func TestGroup_cancelBefore(t *testing.T) {
	leak.Verify(t)
	parent, cancel := context.WithCancelCause(t.Context())
	cancel(errB)
	g, ctx := group.WithContext(parent)
	g.Go(func() error {
		<-ctx.Done() // already done: the function sees it at once
		return context.Cause(ctx)
	})
	if err := g.Wait(); !errors.Is(err, errB) {
		t.Fatalf("Wait = %v, want %v", err, errB)
	}
}

func TestGroup_cancelDuring(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	parent, cancel := context.WithCancelCause(tctx)
	g, ctx := group.WithContext(parent)
	running := make(chan struct{})
	g.Go(func() error {
		close(running)
		<-ctx.Done()
		return context.Cause(ctx)
	})
	recv(t, tctx, running)
	cancel(errB)
	if err := g.Wait(); !errors.Is(err, errB) {
		t.Fatalf("Wait = %v, want %v", err, errB)
	}
}

func TestGroup_cancelAfter(t *testing.T) {
	leak.Verify(t)
	parent, cancel := context.WithCancel(t.Context())
	g, _ := group.WithContext(parent)
	g.Go(func() error { return nil })
	err := g.Wait()
	cancel()
	if err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
}

// The go_61 with_context defect: a task that returns nil when its ctx is done turns a cancelled run into a
// reported success. The fix is to return context.Cause(ctx).
func TestGroup_returningNilOnCancelHidesTheCause(t *testing.T) {
	leak.Verify(t)
	errShutdown := errors.New("shutdown")
	run := func(onCancel func(ctx context.Context) error) error {
		parent, cancel := context.WithCancelCause(t.Context())
		cancel(errShutdown) // cancelled before any task did its work
		g, ctx := group.WithContext(parent)
		var did atomic.Int64
		for range 4 {
			g.Go(func() error {
				select {
				case <-ctx.Done():
					return onCancel(ctx)
				default:
					did.Add(1)
					return nil
				}
			})
		}
		err := g.Wait()
		if did.Load() != 0 {
			t.Fatalf("did %d tasks, want 0", did.Load())
		}
		return err
	}
	t.Run("defect: nil on cancel", func(t *testing.T) {
		if err := run(func(context.Context) error { return nil }); err != nil {
			t.Fatalf("Wait = %v; the defect pattern reports success with nil", err)
		}
	})
	t.Run("fixed: return the cause", func(t *testing.T) {
		if err := run(func(ctx context.Context) error { return context.Cause(ctx) }); !errors.Is(err, errShutdown) {
			t.Fatalf("Wait = %v, want %v", err, errShutdown)
		}
	})
}

func TestGroup_goFromInsideAFunction(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	var n atomic.Int64
	g.Go(func() error {
		n.Add(1)
		g.Go(func() error { n.Add(1); return nil })
		return nil
	})
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	if n.Load() != 2 {
		t.Fatalf("ran %d, want 2", n.Load())
	}
}

func TestGroup_concurrentGoAndSetLimit(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetLimit(4)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			if i%2 == 0 {
				g.SetLimit(2 + i%3)
			}
			g.Go(func() error { return nil })
		})
	}
	wg.Wait() // every Go has returned before Wait is called
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
}

func BenchmarkGroup_Go(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var g group.Group
		for range 8 {
			g.Go(func() error { return nil })
		}
		_ = g.Wait()
	}
}
