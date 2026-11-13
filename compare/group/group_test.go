// Package group_test compares toolbench/group.Group with golang.org/x/sync/errgroup v0.25.0: the same functions
// passed to both give the same Wait result, the same context cause, the same bound on goroutines in flight and
// the same TryGo answers.
//
// Known differences, by design (doc/specs/group.md, deviations 2-3 and the §9 item 14 answer):
//   - A panic in a function: errgroup v0.25.0 does not recover it (no recover in the source since v0.16.0), so it
//     crashes the process; it cannot run here. Ours returns *panics.PanicError (panics.ErrPanic) through Wait.
//     TestCompare_panicWrappedWithTry shows that errgroup with panics.Try around f gives ours' result.
//   - SetLimit while goroutines are active: errgroup panics on the calling goroutine (recovered here); ours
//     applies the new limit to later calls.
//   - Go with a limit of zero: errgroup blocks forever (not run here); ours records ErrZeroLimit. TryGo is
//     compared (both false).
//   - runtime.Goexit in a function: the same (both Wait return nil).
package group_test

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/CrimsonCoder42/toolbench/group"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// runner is the shape both groups share.
type runner interface {
	Go(func() error)
	TryGo(func() error) bool
	SetLimit(int)
	Wait() error
}

type impl struct {
	name        string
	withContext func(context.Context) (runner, context.Context)
	zero        func() runner
}

var impls = []impl{
	{
		name:        "errgroup",
		withContext: func(ctx context.Context) (runner, context.Context) { return errgroup.WithContext(ctx) },
		zero:        func() runner { return new(errgroup.Group) },
	},
	{
		name:        "toolbench",
		withContext: func(ctx context.Context) (runner, context.Context) { return group.WithContext(ctx) },
		zero:        func() runner { return new(group.Group) },
	},
}

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

func recv[T any](t *testing.T, ctx context.Context, ch <-chan T) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatalf("timed out: %v", context.Cause(ctx))
	}
}

func TestCompare_firstErrorWinsAndIsTheCause(t *testing.T) {
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			tctx := bounded(t)
			g, ctx := im.withContext(tctx)
			g.Go(func() error { return errA })
			g.Go(func() error {
				select {
				case <-ctx.Done():
					return errB
				case <-tctx.Done():
					return tctx.Err()
				}
			})
			if err := g.Wait(); !errors.Is(err, errA) {
				t.Fatalf("Wait = %v, want %v", err, errA)
			}
			if c := context.Cause(ctx); !errors.Is(c, errA) {
				t.Fatalf("Cause = %v, want %v", c, errA)
			}
		})
	}
}

func TestCompare_waitWaitsForAll(t *testing.T) {
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			tctx := bounded(t)
			g, ctx := im.withContext(tctx)
			var finished atomic.Int64
			for range 10 {
				g.Go(func() error {
					select {
					case <-ctx.Done():
					case <-tctx.Done():
					}
					finished.Add(1)
					return nil
				})
			}
			g.Go(func() error { return errA })
			_ = g.Wait()
			if finished.Load() != 10 {
				t.Fatalf("finished = %d, want 10", finished.Load())
			}
		})
	}
}

func TestCompare_SetLimitMaxInFlight(t *testing.T) {
	const limit, tasks = 3, 9
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			ctx := bounded(t)
			g := im.zero()
			g.SetLimit(limit)
			var now, peak atomic.Int64
			started := make(chan struct{}, tasks)
			release := make(chan struct{})
			submitted := make(chan struct{})
			go func() {
				defer close(submitted)
				for range tasks {
					g.Go(func() error {
						n := now.Add(1)
						for m := peak.Load(); n > m && !peak.CompareAndSwap(m, n); m = peak.Load() {
						}
						defer now.Add(-1)
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
				t.Fatal("TryGo at the limit = true")
			}
			close(release)
			recv(t, ctx, submitted)
			if err := g.Wait(); err != nil {
				t.Fatalf("Wait = %v", err)
			}
			if peak.Load() != limit {
				t.Fatalf("peak = %d, want %d", peak.Load(), limit)
			}
		})
	}
}

func TestCompare_negativeLimitAndZeroGroup(t *testing.T) {
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			ctx := bounded(t)
			g := im.zero()
			g.SetLimit(-1)
			started := make(chan struct{}, 20)
			release := make(chan struct{})
			for range 20 {
				g.Go(func() error { started <- struct{}{}; <-release; return nil })
			}
			for range 20 {
				recv(t, ctx, started)
			}
			close(release)
			if err := g.Wait(); err != nil {
				t.Fatalf("Wait = %v", err)
			}
		})
	}
}

func TestCompare_TryGoLimitZero(t *testing.T) {
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			g := im.zero()
			g.SetLimit(0)
			if g.TryGo(func() error { return nil }) {
				t.Fatal("TryGo with limit 0 = true")
			}
			if err := g.Wait(); err != nil {
				t.Fatalf("Wait = %v", err)
			}
		})
	}
}

func TestCompare_goexit(t *testing.T) {
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			g := im.zero()
			g.Go(func() error { runtime.Goexit(); return errA })
			if err := g.Wait(); err != nil {
				t.Fatalf("Wait = %v, want nil", err)
			}
		})
	}
}

// Differs: errgroup panics in SetLimit while goroutines are active; ours does not.
func TestCompare_SetLimitWhileActive(t *testing.T) {
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			ctx := bounded(t)
			g := im.zero()
			g.SetLimit(2)
			running := make(chan struct{})
			release := make(chan struct{})
			g.Go(func() error { close(running); <-release; return nil })
			recv(t, ctx, running)
			panicked := func() (p bool) {
				defer func() { p = recover() != nil }()
				g.SetLimit(1)
				return false
			}()
			close(release)
			_ = g.Wait()
			want := im.name == "errgroup"
			if panicked != want {
				t.Fatalf("SetLimit while active panicked = %v, want %v", panicked, want)
			}
		})
	}
}

// Differs: errgroup crashes on a panic; with panics.Try around f it returns what ours returns.
func TestCompare_panicWrappedWithTry(t *testing.T) {
	boom := func() error { panic(errA) }

	var eg errgroup.Group
	eg.Go(func() error { return panics.Try(boom) })
	theirs := eg.Wait()

	var g group.Group
	g.Go(boom)
	ours := g.Wait()

	for name, err := range map[string]error{"errgroup+Try": theirs, "toolbench": ours} {
		if !errors.Is(err, panics.ErrPanic) || !errors.Is(err, errA) {
			t.Fatalf("%s Wait = %v, want ErrPanic wrapping %v", name, err, errA)
		}
	}
}

// Differs by opt-in: errgroup's Wait returns the first error only; ours with SetCollect(true) returns every
// error joined with errors.Join (doc/specs/group.md "Align (2026-11-13)", TB-GROUP-008). A limit of one forces
// the completion order on both, so the comparison is deterministic.
func TestCompare_collectVersusErrgroup(t *testing.T) {
	errC := errors.New("c failed")
	fail := func(g runner) {
		g.SetLimit(1)
		g.Go(func() error { return errA })
		g.Go(func() error { return errB })
		g.Go(func() error { return errC })
	}

	var eg errgroup.Group
	fail(&eg)
	theirs := eg.Wait()
	if theirs != errA || errors.Is(theirs, errB) || errors.Is(theirs, errC) { //nolint:errorlint // the first error itself
		t.Fatalf("errgroup Wait = %v, want exactly %v (first error only)", theirs, errA)
	}

	var g group.Group
	g.SetCollect(true)
	fail(&g)
	ours := g.Wait()
	for _, want := range []error{errA, errB, errC} {
		if !errors.Is(ours, want) {
			t.Fatalf("toolbench Wait = %v, want it to match %v", ours, want)
		}
	}
	u, ok := ours.(interface{ Unwrap() []error })
	if !ok || len(u.Unwrap()) != 3 || u.Unwrap()[0] != errA {
		t.Fatalf("toolbench Wait = %v, want an errors.Join of three with %v first", ours, errA)
	}

	var g2 group.Group // the default is errgroup's: the first error only
	fail(&g2)
	if def := g2.Wait(); def != errA { //nolint:errorlint // the first error itself
		t.Fatalf("toolbench default Wait = %v, want exactly %v", def, errA)
	}
}
