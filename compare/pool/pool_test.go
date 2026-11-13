// Package pool_test compares toolbench/pool.Pool with github.com/sourcegraph/conc/pool v0.3.0 (ContextPool:
// New().WithErrors().WithContext(ctx).WithMaxGoroutines(n)): the same values given to both run the same number of
// times, never with more than n calls in flight, and give the same errors.
//
// Known differences, by design (doc/specs/pool.md):
//   - conc starts a goroutine per task up to the limit (no long-lived workers, no queue); ours starts n workers in
//     New and Submit hands values to them through a bounded queue (WithQueue, default 0 = direct hand-off). Both
//     block the producer when n calls are in flight.
//   - A panic in a task: conc re-raises it from Wait on the caller's goroutine; ours returns panics.ErrPanic from
//     Close. TestCompare_panic recovers conc's re-panic to show both carry the value.
//   - Errors: conc joins all by default, WithFirstError keeps the first; ours the same (default joined,
//     WithFailFast the first). conc's WithCancelOnError is part of our WithFailFast.
//   - Submit after Close is ErrClosed here; conc's Go after Wait is undefined (a pool is not reusable).
//
// Also named in LADDER.md and not compared (different shape): github.com/Jeffail/tunny (Process(payload any) any,
// a synchronous request/response pool with interface{} payloads) and github.com/alitto/pond/v2 (task-returning
// Submit with futures and dynamic resize).
package pool_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	conc "github.com/sourcegraph/conc/pool"

	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/pool"
)

var (
	errA = errors.New("a failed")
	errB = errors.New("b failed")
)

func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

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

// runConc runs fn on every value with conc's ContextPool limited to workers.
func runConc(ctx context.Context, workers int, values []int, firstError bool, fn func(context.Context, int) error) error {
	p := conc.New().WithErrors().WithContext(ctx).WithMaxGoroutines(workers)
	if firstError {
		p = p.WithCancelOnError().WithFirstError()
	}
	for _, v := range values {
		p.Go(func(ctx context.Context) error { return fn(ctx, v) })
	}
	return p.Wait()
}

// runOurs runs fn on every value with a toolbench pool of workers.
func runOurs(ctx context.Context, workers int, values []int, failFast bool, fn func(context.Context, int) error) error {
	var opts []pool.Option
	if failFast {
		opts = append(opts, pool.WithFailFast())
	}
	p, err := pool.New(ctx, workers, fn, opts...)
	if err != nil {
		return err
	}
	for _, v := range values {
		if err := p.Submit(ctx, v); err != nil {
			break
		}
	}
	return p.Close()
}

func values(n int) []int {
	vs := make([]int, n)
	for i := range vs {
		vs[i] = i
	}
	return vs
}

func TestCompare_sameWorkSameBound(t *testing.T) {
	ctx := bounded(t)
	const workers = 4
	for name, run := range map[string]func(context.Context, int, []int, bool, func(context.Context, int) error) error{
		"conc": runConc, "toolbench": runOurs,
	} {
		t.Run(name, func(t *testing.T) {
			var f inFlight
			var sum atomic.Int64
			err := run(ctx, workers, values(200), false, func(_ context.Context, v int) error {
				f.enter()
				defer f.leave()
				sum.Add(int64(v))
				return nil
			})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if sum.Load() != 199*200/2 {
				t.Fatalf("%s: sum %d, want %d", name, sum.Load(), 199*200/2)
			}
			if f.max.Load() > workers {
				t.Fatalf("%s: %d in flight, want at most %d", name, f.max.Load(), workers)
			}
		})
	}
}

func TestCompare_errorsJoined(t *testing.T) {
	ctx := bounded(t)
	fn := func(_ context.Context, v int) error {
		switch v {
		case 3:
			return errA
		case 7:
			return errB
		}
		return nil
	}
	for name, err := range map[string]error{
		"conc":      runConc(ctx, 2, values(10), false, fn),
		"toolbench": runOurs(ctx, 2, values(10), false, fn),
	} {
		if !errors.Is(err, errA) || !errors.Is(err, errB) {
			t.Fatalf("%s: %v, want both errA and errB", name, err)
		}
	}
}

func TestCompare_firstError(t *testing.T) {
	ctx := bounded(t)
	fn := func(_ context.Context, v int) error {
		if v == 0 {
			return errA
		}
		return nil
	}
	// One worker and value 0 first: errA is the first error for both.
	for name, err := range map[string]error{
		"conc":      runConc(ctx, 1, values(5), true, fn),
		"toolbench": runOurs(ctx, 1, values(5), true, fn),
	} {
		if err != errA {
			t.Fatalf("%s: %v, want errA", name, err)
		}
	}
}

func TestCompare_panic(t *testing.T) {
	ctx := bounded(t)
	fn := func(context.Context, int) error { panic("boom") }

	var concPanic any
	func() {
		defer func() { concPanic = recover() }()
		_ = runConc(ctx, 2, values(1), false, fn)
	}()
	if concPanic == nil {
		t.Fatal("conc: Wait did not re-raise the panic")
	}

	err := runOurs(ctx, 2, values(1), false, fn)
	var pe *panics.PanicError
	if !errors.As(err, &pe) || !errors.Is(err, panics.ErrPanic) || pe.Value != "boom" {
		t.Fatalf("toolbench: %v, want *PanicError with value boom", err)
	}
}
