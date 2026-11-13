package pool_test

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/pool"
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

// mustNew creates a pool or fails the test.
func mustNew[T any](t *testing.T, ctx context.Context, workers int, fn func(context.Context, T) error, opts ...pool.Option) *pool.Pool[T] {
	t.Helper()
	p, err := pool.New(ctx, workers, fn, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// mustSubmit submits v or fails the test.
func mustSubmit[T any](t *testing.T, ctx context.Context, p *pool.Pool[T], v T) {
	t.Helper()
	if err := p.Submit(ctx, v); err != nil {
		t.Fatalf("Submit(%v): %v", v, err)
	}
}

// inFlight tracks how many calls run at once and the most seen.
type inFlight struct{ now, max atomic.Int64 }

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

func TestPool_happyPath(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	for _, tc := range []struct {
		name           string
		workers, queue int
		values         int
	}{
		{"zero values", 3, 0, 0},
		{"one value", 3, 0, 1},
		{"more values than workers", 3, 0, 50},
		{"workers = 1", 1, 0, 20},
		{"with queue", 4, 8, 100},
		{"queue larger than values", 2, 64, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sum, ran atomic.Int64
			p := mustNew(t, ctx, tc.workers, func(_ context.Context, v int) error {
				sum.Add(int64(v))
				ran.Add(1)
				return nil
			}, pool.WithQueue(tc.queue))
			for v := range tc.values {
				mustSubmit(t, ctx, p, v)
			}
			if err := p.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			want := int64(tc.values * (tc.values - 1) / 2)
			if ran.Load() != int64(tc.values) || sum.Load() != want {
				t.Fatalf("ran %d sum %d, want %d and %d", ran.Load(), sum.Load(), tc.values, want)
			}
		})
	}
}

func TestPool_exactlyOnce(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const n = 1000
	var counts [n]atomic.Int64
	p := mustNew(t, ctx, 4, func(_ context.Context, v int) error {
		counts[v].Add(1)
		return nil
	}, pool.WithQueue(8))
	for v := range n {
		mustSubmit(t, ctx, p, v)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for v := range n {
		if c := counts[v].Load(); c != 1 {
			t.Fatalf("value %d ran %d times, want 1", v, c)
		}
	}
}

// With one worker the queue is FIFO, so values run in submission order.
func TestPool_oneWorkerSubmissionOrder(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var got []int // written only by the one worker; read after Close, which waits for it
	p := mustNew(t, ctx, 1, func(_ context.Context, v int) error {
		got = append(got, v)
		return nil
	}, pool.WithQueue(4))
	for v := range 30 {
		mustSubmit(t, ctx, p, v)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("got[%d] = %d, want %d (order %v)", i, v, i, got)
		}
	}
	if len(got) != 30 {
		t.Fatalf("ran %d values, want 30", len(got))
	}
}

// The number of fn calls in flight never exceeds the worker count, and reaches it.
func TestPool_maxWorkersInFlight(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const workers, values = 3, 12
	var f inFlight
	var ran atomic.Int64
	started := make(chan struct{}, values)
	release := make(chan struct{})
	p := mustNew(t, ctx, workers, func(_ context.Context, _ int) error {
		f.enter()
		defer f.leave()
		ran.Add(1)
		started <- struct{}{}
		<-release
		return nil
	}, pool.WithQueue(2))
	submitted := make(chan error, 1)
	go func() {
		var err error
		for v := range values {
			err = errors.Join(err, p.Submit(ctx, v))
		}
		submitted <- err
	}()
	for range workers { // every worker is inside fn at once
		recv(t, ctx, started)
	}
	close(release)
	if err := recv(t, ctx, submitted); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if m := f.max.Load(); m != workers {
		t.Fatalf("max in flight %d, want %d", m, workers)
	}
	if ran.Load() != values {
		t.Fatalf("ran %d, want %d", ran.Load(), values)
	}
}

// Submit blocks while every worker is busy and the queue is full, and returns once a slot frees. synctest.Wait
// returns only when every goroutine in the bubble is durably blocked, so "still blocked" is a fact, not a timing.
func TestPool_submitBlocksWhenFull(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		release := make(chan struct{})
		var ran atomic.Int64
		p := mustNew(t, ctx, 1, func(context.Context, int) error {
			<-release
			ran.Add(1)
			return nil
		}, pool.WithQueue(1))
		mustSubmit(t, ctx, p, 1) // the worker takes it
		mustSubmit(t, ctx, p, 2) // the queue holds it
		var returned atomic.Bool
		done := make(chan error, 1)
		go func() {
			err := p.Submit(ctx, 3)
			returned.Store(true)
			done <- err
		}()
		synctest.Wait()
		if returned.Load() {
			t.Fatal("Submit returned while the worker was busy and the queue full")
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatalf("Submit(3): %v", err)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if ran.Load() != 3 {
			t.Fatalf("ran %d, want 3", ran.Load())
		}
	})
}

// A Submit blocked on a full pool returns its ctx's cause when that ctx is cancelled; the value is not run.
func TestPool_submitCtxCancelledWhileBlocked(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		release := make(chan struct{})
		var ran atomic.Int64
		p := mustNew(t, ctx, 1, func(context.Context, int) error {
			<-release
			ran.Add(1)
			return nil
		})
		mustSubmit(t, ctx, p, 1)
		sctx, cancel := context.WithCancelCause(ctx)
		done := make(chan error, 1)
		go func() { done <- p.Submit(sctx, 2) }()
		synctest.Wait()
		cancel(errB)
		if err := <-done; !errors.Is(err, errB) {
			t.Fatalf("Submit = %v, want the cause %v", err, errB)
		}
		close(release)
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if ran.Load() != 1 {
			t.Fatalf("ran %d, want 1", ran.Load())
		}
	})
}

func TestPool_submitCtxDoneBefore(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	p := mustNew(t, ctx, 1, func(context.Context, int) error { return nil }, pool.WithQueue(4))
	sctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := p.Submit(sctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("Submit = %v, want context.Canceled", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// Close while a Submit is blocked: the Submit returns ErrClosed, the accepted values still run.
func TestPool_closeUnblocksSubmit(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		release := make(chan struct{})
		var ran atomic.Int64
		p := mustNew(t, ctx, 1, func(context.Context, int) error {
			<-release
			ran.Add(1)
			return nil
		}, pool.WithQueue(1))
		mustSubmit(t, ctx, p, 1)
		mustSubmit(t, ctx, p, 2)
		done := make(chan error, 1)
		go func() { done <- p.Submit(ctx, 3) }()
		synctest.Wait()
		closed := make(chan error, 1)
		go func() { closed <- p.Close() }()
		if err := <-done; !errors.Is(err, pool.ErrClosed) {
			t.Fatalf("Submit = %v, want ErrClosed", err)
		}
		close(release)
		if err := <-closed; err != nil {
			t.Fatalf("Close: %v", err)
		}
		if ran.Load() != 2 {
			t.Fatalf("ran %d, want 2 (the accepted values)", ran.Load())
		}
	})
}

// Close does not return until every accepted value has run.
func TestPool_closeDrainsQueue(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	release := make(chan struct{})
	var ran atomic.Int64
	p := mustNew(t, ctx, 1, func(context.Context, int) error {
		<-release
		ran.Add(1)
		return nil
	}, pool.WithQueue(3))
	for v := range 4 { // one in the worker (once it takes it), three in the queue
		mustSubmit(t, ctx, p, v)
	}
	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()
	close(release)
	if err := recv(t, ctx, closed); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if ran.Load() != 4 {
		t.Fatalf("ran %d, want 4", ran.Load())
	}
}

func TestPool_submitAfterCloseIsErrClosed(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	p := mustNew(t, ctx, 2, func(context.Context, int) error { return nil })
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := p.Submit(ctx, 1); !errors.Is(err, pool.ErrClosed) {
		t.Fatalf("Submit after Close = %v, want ErrClosed", err)
	}
}

func TestPool_closeTwiceSameResult(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	p := mustNew(t, ctx, 1, func(context.Context, int) error { return errA })
	mustSubmit(t, ctx, p, 1)
	first := p.Close()
	second := p.Close()
	if !errors.Is(first, errA) || first != second {
		t.Fatalf("Close = %v then %v, want the same error matching errA", first, second)
	}
}

func TestPool_zeroValueAndNil(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var z pool.Pool[int]
	if err := z.Submit(ctx, 1); !errors.Is(err, pool.ErrClosed) {
		t.Fatalf("zero Submit = %v, want ErrClosed", err)
	}
	if err := z.Close(); err != nil {
		t.Fatalf("zero Close = %v, want nil", err)
	}
	var np *pool.Pool[int]
	if err := np.Submit(ctx, 1); !errors.Is(err, pool.ErrClosed) {
		t.Fatalf("nil Submit = %v, want ErrClosed", err)
	}
	if err := np.Close(); err != nil {
		t.Fatalf("nil Close = %v, want nil", err)
	}
}

func TestNew_workersNotPositive(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	for _, w := range []int{0, -1, -1 << 30} {
		p, err := pool.New(ctx, w, func(context.Context, int) error { return nil })
		if !errors.Is(err, pool.ErrWorkers) || p != nil {
			t.Fatalf("New(workers=%d) = %v, %v; want nil, ErrWorkers", w, p, err)
		}
	}
}

func TestNew_nilCtxNilOptionNegativeQueue(t *testing.T) {
	leak.Verify(t)
	var nilCtx context.Context
	var ran atomic.Int64
	p, err := pool.New(nilCtx, 2, func(context.Context, int) error { ran.Add(1); return nil }, nil, pool.WithQueue(-5))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for v := range 5 {
		if err := p.Submit(nilCtx, v); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	if err := p.Close(); err != nil || ran.Load() != 5 {
		t.Fatalf("Close = %v, ran %d; want nil, 5", err, ran.Load())
	}
}

func TestNew_nilFnIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	p := mustNew[int](t, ctx, 1, nil)
	mustSubmit(t, ctx, p, 1)
	if err := p.Close(); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Close = %v, want ErrPanic", err)
	}
}

// Default mode: every error is kept, every value still runs, Close joins them.
func TestPool_collectsAllErrors(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var ran atomic.Int64
	p := mustNew(t, ctx, 2, func(_ context.Context, v int) error {
		ran.Add(1)
		switch v {
		case 2:
			return errA
		case 4:
			return errB
		}
		return nil
	})
	for v := range 6 {
		mustSubmit(t, ctx, p, v)
	}
	err := p.Close()
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("Close = %v, want both errA and errB", err)
	}
	if ran.Load() != 6 {
		t.Fatalf("ran %d, want 6", ran.Load())
	}
}

func TestPool_panicCollectedAsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var ran atomic.Int64
	p := mustNew(t, ctx, 2, func(_ context.Context, v int) error {
		if v == 2 {
			panic("boom")
		}
		ran.Add(1)
		return nil
	})
	for v := range 5 {
		mustSubmit(t, ctx, p, v)
	}
	err := p.Close()
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("Close = %v, want a *PanicError with value boom", err)
	}
	if ran.Load() != 4 {
		t.Fatalf("ran %d, want 4 (the worker survives the panic)", ran.Load())
	}
}

func TestPool_failFastPanic(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	p := mustNew(t, ctx, 1, func(context.Context, int) error { panic(errA) }, pool.WithFailFast())
	mustSubmit(t, ctx, p, 1)
	err := p.Close()
	var pe *panics.PanicError
	if !errors.As(err, &pe) || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Close = %v, want *PanicError", err)
	}
}

// WithFailFast: the first error cancels the pool's ctx; Submit then returns it as the cause; Close returns it.
func TestPool_failFastFirstErrorCancels(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	seen := make(chan context.Context, 1)
	p := mustNew(t, ctx, 1, func(fctx context.Context, _ int) error {
		seen <- fctx
		return errA
	}, pool.WithFailFast())
	mustSubmit(t, ctx, p, 1)
	fctx := recv(t, ctx, seen)
	select {
	case <-fctx.Done():
	case <-ctx.Done():
		t.Fatal("the pool's ctx was not cancelled by the first error")
	}
	if err := p.Submit(ctx, 2); !errors.Is(err, errA) {
		t.Fatalf("Submit after the first error = %v, want errA", err)
	}
	if err := p.Close(); err != errA {
		t.Fatalf("Close = %v, want errA itself", err)
	}
}

// WithFailFast: a value blocked on the pool's ctx is released by another worker's error; Close returns the first.
func TestPool_failFastStopsOtherWorkers(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	p := mustNew(t, ctx, 2, func(fctx context.Context, v int) error {
		if v == 1 {
			<-fctx.Done() // waits for v == 2 to fail
			return context.Cause(fctx)
		}
		return errA
	}, pool.WithFailFast())
	mustSubmit(t, ctx, p, 1)
	mustSubmit(t, ctx, p, 2)
	if err := p.Close(); err != errA {
		t.Fatalf("Close = %v, want errA", err)
	}
}

// WithFailFast and values left in the queue: Close reports the first error and the unrun count, both matched.
func TestPool_failFastReportsUnrunValues(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var ran atomic.Int64
	p := mustNew(t, ctx, 1, func(context.Context, int) error {
		ran.Add(1)
		started <- struct{}{}
		<-release
		return errA
	}, pool.WithFailFast(), pool.WithQueue(2))
	mustSubmit(t, ctx, p, 1)
	recv(t, ctx, started) // the worker holds value 1
	mustSubmit(t, ctx, p, 2)
	mustSubmit(t, ctx, p, 3) // both in the queue
	close(release)           // value 1 fails; the only worker stops
	err := p.Close()
	if !errors.Is(err, errA) || !strings.Contains(err.Error(), "2 submitted values not run") {
		t.Fatalf("Close = %v, want errA joined with 2 values not run", err)
	}
	if ran.Load() != 1 {
		t.Fatalf("ran %d, want 1", ran.Load())
	}
}

func TestPool_cancelBeforeNew(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancelCause(bounded(t))
	cancel(errB)
	var ran atomic.Int64
	p := mustNew(t, ctx, 3, func(context.Context, int) error { ran.Add(1); return nil }, pool.WithQueue(4))
	if err := p.Submit(bounded(t), 1); !errors.Is(err, errB) {
		t.Fatalf("Submit = %v, want the cause errB", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close = %v, want nil (nothing was accepted)", err)
	}
	if ran.Load() != 0 {
		t.Fatalf("ran %d, want 0", ran.Load())
	}
}

// Cancel mid-stream: the running value finishes, queued values are not run and Close reports how many.
func TestPool_cancelMidStream(t *testing.T) {
	leak.Verify(t)
	parent := bounded(t)
	ctx, cancel := context.WithCancelCause(parent)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var ran atomic.Int64
	p := mustNew(t, ctx, 1, func(context.Context, int) error {
		ran.Add(1)
		started <- struct{}{}
		<-release
		return nil
	}, pool.WithQueue(2))
	mustSubmit(t, parent, p, 1)
	recv(t, parent, started) // the worker holds value 1
	mustSubmit(t, parent, p, 2)
	mustSubmit(t, parent, p, 3) // both in the queue
	cancel(errB)
	close(release)
	err := p.Close()
	if !errors.Is(err, errB) || !strings.Contains(err.Error(), "2 submitted values not run") {
		t.Fatalf("Close = %v, want 2 values not run, cause errB", err)
	}
	if ran.Load() != 1 {
		t.Fatalf("ran %d, want 1", ran.Load())
	}
}

func TestPool_cancelAfterCompletion(t *testing.T) {
	leak.Verify(t)
	parent := bounded(t)
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{}, 3)
	p := mustNew(t, ctx, 2, func(context.Context, int) error { done <- struct{}{}; return nil })
	for v := range 3 {
		mustSubmit(t, parent, p, v)
	}
	for range 3 {
		recv(t, parent, done)
	}
	cancel() // after every value ran, before Close
	if err := p.Close(); err != nil {
		t.Fatalf("Close = %v, want nil (nothing was left unrun)", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
}

// runtime.Goexit in fn (t.FailNow is one) ends that worker; it is replaced, so later values still run.
func TestPool_goexitReplacesWorker(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var ran atomic.Int64
	p := mustNew(t, ctx, 1, func(_ context.Context, v int) error {
		if v == 1 {
			runtime.Goexit()
		}
		ran.Add(1)
		return nil
	})
	for v := range 4 {
		mustSubmit(t, ctx, p, v)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if ran.Load() != 3 {
		t.Fatalf("ran %d, want 3", ran.Load())
	}
}

// Many Submits racing one Close: each returns nil or ErrClosed, and exactly the accepted values run.
func TestPool_concurrentSubmitAndClose(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var ran, accepted atomic.Int64
	p := mustNew(t, ctx, 3, func(context.Context, int) error { ran.Add(1); return nil }, pool.WithQueue(4))
	start := make(chan struct{})
	var wg sync.WaitGroup
	var (
		badMu sync.Mutex
		bad   error // guarded by badMu
	)
	for range 8 {
		wg.Go(func() {
			<-start
			for v := range 100 {
				switch err := p.Submit(ctx, v); {
				case err == nil:
					accepted.Add(1)
				case !errors.Is(err, pool.ErrClosed):
					badMu.Lock()
					bad = err
					badMu.Unlock()
				}
			}
		})
	}
	close(start)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	wg.Wait()
	if bad != nil { // every submitter has returned (wg.Wait)
		t.Fatalf("Submit returned %v, want nil or ErrClosed", bad)
	}
	if ran.Load() != accepted.Load() {
		t.Fatalf("ran %d, accepted %d: every accepted value must run", ran.Load(), accepted.Load())
	}
}

// go_18 sizes its task channel to the whole input (make(chan Task, len(Tasks))): memory grows with the input.
// Ours holds at most workers + queue values, whatever the producer offers.
func TestPool_defectGo18_queueBoundedNotSizedToInput(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		release := make(chan struct{})
		p := mustNew(t, ctx, 1, func(context.Context, int) error { <-release; return nil }, pool.WithQueue(2))
		var accepted atomic.Int64
		done := make(chan struct{})
		go func() {
			defer close(done)
			for v := range 100 {
				if p.Submit(ctx, v) == nil {
					accepted.Add(1)
				}
			}
		}()
		synctest.Wait()
		if a := accepted.Load(); a != 3 {
			t.Fatalf("accepted %d while the worker was busy, want 3 (1 running + queue 2)", a)
		}
		close(release)
		<-done
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if accepted.Load() != 100 {
			t.Fatalf("accepted %d, want 100", accepted.Load())
		}
	})
}

// go_18's Task.Process() has no ctx and no error: a failure is lost. Ours passes the pool's ctx to fn and returns
// the error through Close.
func TestPool_defectGo18_errorAndCtxReachTheCaller(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	gotCtx := make(chan bool, 1)
	p := mustNew(t, ctx, 1, func(fctx context.Context, _ int) error {
		gotCtx <- fctx != nil
		return errA
	})
	mustSubmit(t, ctx, p, 1)
	if !recv(t, ctx, gotCtx) {
		t.Fatal("fn got a nil ctx")
	}
	if err := p.Close(); !errors.Is(err, errA) {
		t.Fatalf("Close = %v, want errA", err)
	}
}

type valueWorker struct{ n int }

func (w valueWorker) process(context.Context, int) error { w.n++; return nil }

type pointerWorker struct{ n int }

func (w *pointerWorker) process(context.Context, int) error { w.n++; return nil }

// go_66's stateful tunny worker has value receivers, so every state change is made on a copy and lost. With
// ours the state lives in fn's receiver: a pointer receiver keeps it (one worker, so no race), a value one does not.
func TestPool_defectGo66_valueReceiverLosesState(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var vw valueWorker
	pw := &pointerWorker{}
	pv := mustNew(t, ctx, 1, vw.process)
	pp := mustNew(t, ctx, 1, pw.process)
	for v := range 5 {
		mustSubmit(t, ctx, pv, v)
		mustSubmit(t, ctx, pp, v)
	}
	if err := errors.Join(pv.Close(), pp.Close()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if vw.n != 0 || pw.n != 5 {
		t.Fatalf("value receiver n=%d (want 0, the defect), pointer receiver n=%d (want 5)", vw.n, pw.n)
	}
}

// go_66 polls QueueLength() with time.Sleep to know when work is done. Ours: Close returns when it is done.
func TestPool_defectGo66_closeWaitsWithoutPolling(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var ran atomic.Int64
	p := mustNew(t, ctx, 2, func(context.Context, int) error { ran.Add(1); return nil }, pool.WithQueue(4))
	for v := range 20 {
		mustSubmit(t, ctx, p, v)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if ran.Load() != 20 {
		t.Fatalf("ran %d right after Close, want 20", ran.Load())
	}
}

// conc's pool re-raises a task's panic from Wait, crashing the caller. Ours returns it from Close as ErrPanic.
func TestPool_defectGo67_panicReturnedNotReraised(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	p := mustNew(t, ctx, 2, func(context.Context, int) error { panic("task panic") })
	mustSubmit(t, ctx, p, 1)
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Close panicked: %v", r)
			}
		}()
		err = p.Close()
	}()
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Close = %v, want ErrPanic", err)
	}
}

func BenchmarkPool_Submit(b *testing.B) {
	ctx := context.Background()
	p, err := pool.New(ctx, runtime.GOMAXPROCS(0), func(context.Context, int) error { return nil }, pool.WithQueue(128))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := p.Submit(ctx, 1); err != nil {
			b.Fatal(err)
		}
	}
	if err := p.Close(); err != nil {
		b.Fatal(err)
	}
}

// Git-Secrets-Scanner internal/progress/task.go:118-157: Wait polls with time.Sleep and returns on quit while
// workers still run. Ours: Close returns only after every worker has exited. Two workers are held inside fn;
// synctest.Wait makes "Close has not returned" a fact (every goroutine in the bubble is durably blocked).
func TestPool_closeReturnsOnlyAfterEveryWorkerExited(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		release := make(chan struct{})
		var inside, exited atomic.Int64
		p := mustNew(t, ctx, 2, func(context.Context, int) error {
			inside.Add(1)
			<-release
			exited.Add(1)
			return nil
		})
		mustSubmit(t, ctx, p, 1) // direct hand-off: a worker holds it
		mustSubmit(t, ctx, p, 2) // the other worker holds it
		closed := make(chan error, 1)
		go func() { closed <- p.Close() }()
		synctest.Wait()
		if inside.Load() != 2 {
			t.Fatalf("%d workers inside fn, want 2", inside.Load())
		}
		select {
		case err := <-closed:
			t.Fatalf("Close returned (%v) while both workers were still inside fn", err)
		default:
		}
		close(release)
		if err := <-closed; err != nil {
			t.Fatalf("Close: %v", err)
		}
		if exited.Load() != 2 {
			t.Fatalf("Close returned with %d of 2 workers past fn", exited.Load())
		}
	})
}

// Crawler frontier (any-website-sitemap-crawler main.go:62-92, guardian-scraper main.go:119): a worker feeds
// its own pool. With the worker's ctx Submit never blocks; an overflow (ErrQueueFull) is handled inline by the
// caller, so every value of a binary tree of 255 runs exactly once and nothing deadlocks.
func TestPool_submitFromWorkerWithQueueDoesNotDeadlock(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const total = 255 // value v feeds 2v and 2v+1 while they are <= total
	var counts [total + 1]atomic.Int64
	var ran, full atomic.Int64
	allRan := make(chan struct{})
	var once sync.Once
	var p *pool.Pool[int]
	var run func(ctx context.Context, v int) error
	run = func(ctx context.Context, v int) error {
		counts[v].Add(1)
		if ran.Add(1) == total {
			once.Do(func() { close(allRan) })
		}
		for _, child := range []int{2 * v, 2*v + 1} {
			if child > total {
				continue
			}
			switch err := p.Submit(ctx, child); {
			case err == nil:
			case errors.Is(err, pool.ErrQueueFull):
				full.Add(1)
				if err := run(ctx, child); err != nil { // the caller handles the overflow: here, inline
					return err
				}
			default:
				return err
			}
		}
		return nil
	}
	p = mustNew(t, ctx, 4, run, pool.WithQueue(4))
	mustSubmit(t, ctx, p, 1)
	recv(t, ctx, allRan)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for v := 1; v <= total; v++ {
		if c := counts[v].Load(); c != 1 {
			t.Fatalf("value %d ran %d times, want 1", v, c)
		}
	}
	t.Logf("%d of %d in-worker submits overflowed and were run inline", full.Load(), total-1)
}

// With no queue and the only worker inside fn, nobody can take the value: Submit from the worker returns
// ErrQueueFull (wrapped with the queue size) at once instead of waiting forever; the value is never run.
func TestPool_submitFromWorkerWithoutQueueReturnsErrQueueFull(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var ran atomic.Int64
	got := make(chan error, 1)
	var p *pool.Pool[int]
	p = mustNew(t, ctx, 1, func(ctx context.Context, v int) error {
		ran.Add(1)
		if v == 1 {
			got <- p.Submit(ctx, 2)
		}
		return nil
	})
	mustSubmit(t, ctx, p, 1)
	err := recv(t, ctx, got)
	if !errors.Is(err, pool.ErrQueueFull) || !strings.Contains(err.Error(), "queue size 0") {
		t.Fatalf("Submit from worker = %v, want ErrQueueFull with the queue size", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if ran.Load() != 1 {
		t.Fatalf("ran %d, want 1 (the refused value never runs)", ran.Load())
	}
}

// With a queue of 2 and the only worker inside fn, two in-worker Submits fill it and the third returns
// ErrQueueFull; the two queued values run after fn returns.
func TestPool_submitFromWorkerFullQueueReturnsErrQueueFull(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var ran atomic.Int64
	got := make(chan [3]error, 1)
	var p *pool.Pool[int]
	p = mustNew(t, ctx, 1, func(ctx context.Context, v int) error {
		ran.Add(1)
		if v == 1 {
			got <- [3]error{p.Submit(ctx, 2), p.Submit(ctx, 3), p.Submit(ctx, 4)}
		}
		return nil
	}, pool.WithQueue(2))
	mustSubmit(t, ctx, p, 1)
	errs := recv(t, ctx, got)
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("first two in-worker Submits = %v, %v, want nil (queue has room)", errs[0], errs[1])
	}
	if !errors.Is(errs[2], pool.ErrQueueFull) || !strings.Contains(errs[2].Error(), "queue size 2") {
		t.Fatalf("third in-worker Submit = %v, want ErrQueueFull with the queue size", errs[2])
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if ran.Load() != 3 {
		t.Fatalf("ran %d, want 3 (1, 2 and 3; 4 was refused)", ran.Load())
	}
}

// A worker that passes context.Background() to Submit gets the blocking form (the ctx is how the two are told
// apart): with no queue it waits, and Close releases it with ErrClosed.
func TestPool_submitFromWorkerBackgroundCtxBlocksUntilClose(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		got := make(chan error, 1)
		var p *pool.Pool[int]
		p = mustNew(t, ctx, 1, func(_ context.Context, v int) error {
			if v == 1 {
				got <- p.Submit(context.Background(), 2)
			}
			return nil
		})
		mustSubmit(t, ctx, p, 1)
		synctest.Wait() // the worker is durably blocked inside its Submit
		if len(got) != 0 {
			t.Fatalf("in-worker Submit with Background returned %v, want it blocked", <-got)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := <-got; !errors.Is(err, pool.ErrClosed) {
			t.Fatalf("blocked in-worker Submit released with %v, want ErrClosed", err)
		}
	})
}

// A ctx from ANOTHER pool's worker is "outside" for this pool: Submit blocks (backpressure), as from any caller.
func TestPool_submitFromOtherPoolWorkerBlocks(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		release := make(chan struct{})
		var ran atomic.Int64
		inner := mustNew(t, ctx, 1, func(context.Context, int) error {
			<-release
			ran.Add(1)
			return nil
		})
		mustSubmit(t, ctx, inner, 1) // inner's only worker is busy; no queue
		got := make(chan error, 1)
		outer := mustNew(t, ctx, 1, func(ctx context.Context, _ int) error {
			got <- inner.Submit(ctx, 2) // outer's worker ctx: not inner's, so this blocks
			return nil
		})
		mustSubmit(t, ctx, outer, 1)
		synctest.Wait()
		if len(got) != 0 {
			t.Fatalf("Submit with another pool's worker ctx returned %v, want it blocked", <-got)
		}
		close(release)
		if err := <-got; err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if err := outer.Close(); err != nil {
			t.Fatalf("outer Close: %v", err)
		}
		if err := inner.Close(); err != nil {
			t.Fatalf("inner Close: %v", err)
		}
		if ran.Load() != 2 {
			t.Fatalf("ran %d, want 2", ran.Load())
		}
	})
}

// TrySubmit: true while a worker or the queue takes the value, false with ErrQueueFull when full, with
// ErrClosed after Close, with the cause when the pool stopped (fail-fast), and for the zero value and nil.
func TestPool_trySubmit(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		release := make(chan struct{})
		var ran atomic.Int64
		p := mustNew(t, ctx, 1, func(context.Context, int) error {
			<-release
			ran.Add(1)
			return nil
		}, pool.WithQueue(1))
		if ok, err := p.TrySubmit(1); !ok || err != nil {
			t.Fatalf("TrySubmit(1) = %v, %v; want true, nil", ok, err)
		}
		synctest.Wait() // the worker has taken 1 and is blocked in fn
		if ok, err := p.TrySubmit(2); !ok || err != nil {
			t.Fatalf("TrySubmit(2) = %v, %v; want true, nil (the queue holds it)", ok, err)
		}
		if ok, err := p.TrySubmit(3); ok || !errors.Is(err, pool.ErrQueueFull) || !strings.Contains(err.Error(), "queue size 1") {
			t.Fatalf("TrySubmit(3) = %v, %v; want false, ErrQueueFull with the queue size", ok, err)
		}
		close(release)
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if ran.Load() != 2 {
			t.Fatalf("ran %d, want 2 (3 was refused)", ran.Load())
		}
		if ok, err := p.TrySubmit(4); ok || !errors.Is(err, pool.ErrClosed) {
			t.Fatalf("TrySubmit after Close = %v, %v; want false, ErrClosed", ok, err)
		}

		// A stopped pool (fail-fast, first error) returns its cause.
		ff := mustNew(t, ctx, 1, func(context.Context, int) error { return errA }, pool.WithFailFast())
		mustSubmit(t, ctx, ff, 1)
		synctest.Wait() // the worker returned errA and the group cancelled the pool's ctx
		if ok, err := ff.TrySubmit(2); ok || !errors.Is(err, errA) {
			t.Fatalf("TrySubmit on a stopped pool = %v, %v; want false, errA", ok, err)
		}
		if err := ff.Close(); !errors.Is(err, errA) {
			t.Fatalf("Close = %v, want errA", err)
		}

		var zero pool.Pool[int]
		if ok, err := zero.TrySubmit(1); ok || !errors.Is(err, pool.ErrClosed) {
			t.Fatalf("zero TrySubmit = %v, %v; want false, ErrClosed", ok, err)
		}
		var nilPool *pool.Pool[int]
		if ok, err := nilPool.TrySubmit(1); ok || !errors.Is(err, pool.ErrClosed) {
			t.Fatalf("nil TrySubmit = %v, %v; want false, ErrClosed", ok, err)
		}
	})
}

// Git-Secrets-Scanner progressbar.go:60 reads TasksDoneCount without the lock. Ours hands the hook the pool's
// own counters: after Close the last call saw done == total == submitted, done never exceeded total, and the
// hook ran once per value, outside the fn error path too.
func TestPool_onDoneCountsExact(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const n = 200
	var calls, maxDone, violations atomic.Int64
	var lastMu sync.Mutex
	var lastDone, lastTotal int
	p := mustNew(t, ctx, 4, func(_ context.Context, v int) error {
		if v%7 == 0 {
			return errA // the hook is called for failures too
		}
		return nil
	}, pool.WithQueue(8), pool.WithOnDone(func(done, total int) {
		calls.Add(1)
		if done > total {
			violations.Add(1)
		}
		for {
			m := maxDone.Load()
			if int64(done) <= m || maxDone.CompareAndSwap(m, int64(done)) {
				break
			}
		}
		lastMu.Lock()
		if done > lastDone {
			lastDone, lastTotal = done, total
		}
		lastMu.Unlock()
	}))
	for v := range n {
		mustSubmit(t, ctx, p, v)
	}
	if err := p.Close(); !errors.Is(err, errA) {
		t.Fatalf("Close = %v, want errA joined", err)
	}
	if calls.Load() != n || maxDone.Load() != n {
		t.Fatalf("hook calls %d, max done %d; want %d and %d", calls.Load(), maxDone.Load(), n, n)
	}
	if violations.Load() != 0 {
		t.Fatalf("done exceeded total in %d calls", violations.Load())
	}
	if lastDone != n || lastTotal != n {
		t.Fatalf("last hook call (done, total) = (%d, %d), want (%d, %d)", lastDone, lastTotal, n, n)
	}
}

// A panic in the hook is recovered on the worker and recorded as that value's error (ErrPanic), joined with
// fn's own error when there was one; the worker survives it.
func TestPool_onDonePanicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var ran atomic.Int64
	p := mustNew(t, ctx, 1, func(_ context.Context, v int) error {
		ran.Add(1)
		if v == 2 {
			return errB
		}
		return nil
	}, pool.WithOnDone(func(done, _ int) {
		if done == 2 {
			panic("hook failed")
		}
	}))
	for v := 1; v <= 3; v++ {
		mustSubmit(t, ctx, p, v)
	}
	err := p.Close()
	if !errors.Is(err, panics.ErrPanic) || !errors.Is(err, errB) {
		t.Fatalf("Close = %v, want ErrPanic and errB", err)
	}
	var pe *panics.PanicError
	if !errors.As(err, &pe) || pe.Value != "hook failed" {
		t.Fatalf("Close = %v, want the hook's panic value", err)
	}
	if ran.Load() != 3 {
		t.Fatalf("ran %d, want 3 (the worker survived the hook's panic)", ran.Load())
	}
}

// A value Submit refuses (after Close, ctx done, or ErrQueueFull from TrySubmit) never reaches fn, so the hook is
// not called for it and total does not count it.
func TestPool_onDoneNotCalledForRefusedValue(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var calls atomic.Int64
	lastTotal := make(chan int, 1)
	p := mustNew(t, ctx, 1, func(context.Context, int) error { return nil }, pool.WithOnDone(func(_, total int) {
		calls.Add(1)
		select {
		case lastTotal <- total:
		default:
		}
	}))
	done, cancel := context.WithCancel(ctx)
	cancel()
	if err := p.Submit(done, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("Submit with a done ctx = %v, want Canceled", err)
	}
	mustSubmit(t, ctx, p, 2)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := p.Submit(ctx, 3); !errors.Is(err, pool.ErrClosed) {
		t.Fatalf("Submit after Close = %v, want ErrClosed", err)
	}
	if ok, err := p.TrySubmit(4); ok || !errors.Is(err, pool.ErrClosed) {
		t.Fatalf("TrySubmit after Close = %v, %v; want false, ErrClosed", ok, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("hook called %d times, want 1 (only the accepted value)", calls.Load())
	}
	if total := recv(t, ctx, lastTotal); total != 1 {
		t.Fatalf("hook saw total %d, want 1 (refused values are not counted)", total)
	}
}

// Scale clause: WithMaxErrors(2) with 10 failures keeps the first 2, and Close adds one error reporting the 8
// not kept that wraps the last dropped one. One worker makes the completion order the submission order.
func TestPool_maxErrorsKeepsNAndCounts(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	errs := make([]error, 10)
	for i := range errs {
		errs[i] = errors.New("value " + strings.Repeat("x", i+1))
	}
	p := mustNew(t, ctx, 1, func(_ context.Context, v int) error { return errs[v] }, pool.WithMaxErrors(2))
	for v := range 10 {
		mustSubmit(t, ctx, p, v)
	}
	err := p.Close()
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("Close = %T %v, want a joined error", err, err)
	}
	if got := joined.Unwrap(); len(got) != 3 || !errors.Is(got[0], errs[0]) || !errors.Is(got[1], errs[1]) {
		t.Fatalf("Close joined %d errors %v, want the first 2 kept plus the summary", len(got), got)
	}
	if !strings.Contains(err.Error(), "pool: 8 further errors not kept") {
		t.Fatalf("Close = %q, want the count of errors not kept", err)
	}
	if !errors.Is(err, errs[9]) || errors.Is(err, errs[5]) {
		t.Fatalf("Close = %v, want it to wrap the LAST dropped error only", err)
	}
}

// The default keeps every error, exactly as before the option.
func TestPool_maxErrorsDefaultKeepsAll(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	for _, opts := range [][]pool.Option{nil, {pool.WithMaxErrors(0)}, {pool.WithMaxErrors(-1)}} {
		p := mustNew(t, ctx, 2, func(_ context.Context, v int) error { return errA }, opts...)
		for v := range 10 {
			mustSubmit(t, ctx, p, v)
		}
		err := p.Close()
		if got := err.(interface{ Unwrap() []error }).Unwrap(); len(got) != 10 {
			t.Fatalf("Close joined %d errors, want all 10", len(got))
		}
		if strings.Contains(err.Error(), "not kept") {
			t.Fatalf("Close = %q, want no summary when nothing was dropped", err)
		}
	}
}

// Submit with a worker's ctx takes the non-blocking path (marker check + select with default). Submits that
// find the queue full are counted, not failed: the workers may lag the benchmark goroutine.
func BenchmarkPool_SubmitFromWorker(b *testing.B) {
	ctx := context.Background()
	gotCtx := make(chan context.Context, 1)
	p, err := pool.New(ctx, runtime.GOMAXPROCS(0), func(ctx context.Context, _ int) error {
		select {
		case gotCtx <- ctx:
		default:
		}
		return nil
	}, pool.WithQueue(128))
	if err != nil {
		b.Fatal(err)
	}
	if err := p.Submit(ctx, 0); err != nil {
		b.Fatal(err)
	}
	wctx := <-gotCtx
	var full int
	b.ReportAllocs()
	// for range b.N, not b.Loop: go1.27.3's escape analysis crashes (internal compiler error, iter.go:223) on
	// a b.Loop body here (align run 2026-11-13; minimal repro left for Devin in doc/align_2026-11-13/README.md).
	for range b.N {
		err := p.Submit(wctx, 1)
		switch {
		case err == nil:
		case errors.Is(err, pool.ErrQueueFull):
			full++
		default:
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(full)/float64(b.N), "full/op")
	if err := p.Close(); err != nil {
		b.Fatal(err)
	}
}
