package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/lifecycle"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// TestGroup_readinessFalseOnceShutdownBegins: no reference flips readiness before draining (they go from 200 to
// connection refused). Here Ready is nil while the group runs and is ErrNotReady from inside the first Stop,
// without the Ready checks being consulted, and stays so after Run returned. The fake clock never moves.
func TestGroup_readinessFalseOnceShutdownBegins(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	fc := clock.NewFake(time.Unix(0, 0))
	var checks atomic.Int32
	running, stopping := make(chan error, 1), make(chan error, 1)
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "db", Ready: func(context.Context) error { checks.Add(1); return nil }})
	_ = g.Add(lifecycle.Component{
		Name: "server",
		Run:  func(ctx context.Context) error { running <- g.Ready(ctx); return nil }, // returning ends the group
		Stop: func(ctx context.Context) error { stopping <- g.Ready(ctx); return nil },
	})
	if err := await(t, tctx, runAsync(tctx, &g, lifecycle.WithClock(fc), lifecycle.WithGrace(time.Hour))); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if err := <-running; err != nil { // sent before the Run returned, so it is there
		t.Fatalf("Ready while running = %v, want nil", err)
	}
	err := <-stopping
	if !errors.Is(err, lifecycle.ErrNotReady) || !strings.Contains(err.Error(), "shutting down") {
		t.Fatalf("Ready inside Stop = %v, want ErrNotReady: shutting down", err)
	}
	if err := g.Ready(tctx); !errors.Is(err, lifecycle.ErrNotReady) {
		t.Fatalf("Ready after Run = %v, want ErrNotReady", err)
	}
	if n := checks.Load(); n != 1 {
		t.Fatalf("db's Ready check ran %d times, want 1 (only while running)", n)
	}
}

func TestGroup_readinessFalseBeforeStartsComplete(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	var checks atomic.Int32
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "db", Ready: func(context.Context) error { checks.Add(1); return nil }})
	if err := g.Ready(tctx); !errors.Is(err, lifecycle.ErrNotReady) || !strings.Contains(err.Error(), "not every component has started") {
		t.Fatalf("Ready before Run = %v, want ErrNotReady: not every component has started", err)
	}
	duringStart := make(chan error, 1)
	_ = g.Add(lifecycle.Component{
		Name:  "server",
		Start: func(ctx context.Context) error { duringStart <- g.Ready(ctx); return nil },
		Run:   func(context.Context) error { return nil },
	})
	if err := await(t, tctx, runAsync(tctx, &g)); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if err := <-duringStart; !errors.Is(err, lifecycle.ErrNotReady) {
		t.Fatalf("Ready during a Start = %v, want ErrNotReady (db is up, server is not)", err)
	}
	if n := checks.Load(); n != 0 {
		t.Fatalf("db's Ready check ran %d times, want 0 (never while starting or stopping)", n)
	}

	// A failed Start: the group never becomes ready and is stopping once Run returned.
	var failed lifecycle.Group
	_ = failed.Add(lifecycle.Component{Name: "server", Start: func(context.Context) error { return errBoom }})
	if err := failed.Run(tctx); !errors.Is(err, errBoom) {
		t.Fatalf("Run = %v, want errBoom", err)
	}
	if err := failed.Ready(tctx); !errors.Is(err, lifecycle.ErrNotReady) {
		t.Fatalf("Ready after a failed start = %v, want ErrNotReady", err)
	}
}

func TestGroup_liveAggregatesNamedErrors(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	errA, errB := errors.New("a down"), errors.New("b down")
	var r recorder
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "ok"}) // no Live: always alive
	_ = g.Add(lifecycle.Component{Name: "a", Live: func(context.Context) error { r.add("a"); return errA }})
	_ = g.Add(lifecycle.Component{Live: func(context.Context) error { r.add("3"); return errB }}) // unnamed
	_ = g.Add(lifecycle.Component{Name: "fine", Live: func(context.Context) error { r.add("fine"); return nil }})
	err := g.Live(nil) // before Run, nil ctx: Live is independent of the state
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("Live = %v, want errA and errB joined", err)
	}
	for _, s := range []string{"live a: a down", "live component 3: b down"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("Live = %q, want it to contain %q", err, s)
		}
	}
	if strings.Contains(err.Error(), "fine") {
		t.Fatalf("Live = %q: a passing check is not named", err)
	}
	if got, want := r.get(), []string{"a", "3", "fine"}; !slices.Equal(got, want) {
		t.Fatalf("check order = %v, want Add order %v", got, want)
	}
	// Live answers after Run returned too, and a Group without checks is alive.
	_ = g.Add(lifecycle.Component{Name: "end", Run: func(context.Context) error { return nil }}) // ends the group at once
	if err := await(t, tctx, runAsync(tctx, &g)); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if err := g.Live(tctx); !errors.Is(err, errA) {
		t.Fatalf("Live after Run = %v, want errA", err)
	}
	var empty lifecycle.Group
	if err := empty.Live(tctx); err != nil {
		t.Fatalf("Live on an empty Group = %v, want nil", err)
	}
}

func TestGroup_checkPanicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{
		Name:  "x",
		Live:  func(context.Context) error { panic("live x") },
		Ready: func(context.Context) error { panic("ready x") },
	})
	if err := g.Live(tctx); !errors.Is(err, panics.ErrPanic) || !strings.Contains(err.Error(), "live x") {
		t.Fatalf("Live = %v, want ErrPanic naming live x", err)
	}
	ready := make(chan error, 1)
	_ = g.Add(lifecycle.Component{Name: "probe", Run: func(ctx context.Context) error { ready <- g.Ready(ctx); return nil }})
	if err := await(t, tctx, runAsync(tctx, &g)); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	err := <-ready
	if !errors.Is(err, panics.ErrPanic) || !errors.Is(err, lifecycle.ErrNotReady) || !strings.Contains(err.Error(), "ready x") {
		t.Fatalf("Ready = %v, want ErrPanic under ErrNotReady naming ready x", err)
	}
}

func TestGroup_checksStopWhenCtxDone(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	errProbe := errors.New("probe deadline")
	var calls atomic.Int32
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "a", Live: func(context.Context) error { calls.Add(1); return nil }})
	_ = g.Add(lifecycle.Component{Name: "b", Live: func(context.Context) error { calls.Add(1); return nil }})
	// Done before the call: no check runs, the cause is returned.
	done, cancel := context.WithCancelCause(tctx)
	cancel(errProbe)
	if err := g.Live(done); !errors.Is(err, errProbe) {
		t.Fatalf("Live with a done ctx = %v, want errProbe", err)
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("%d checks ran with a done ctx, want 0", n)
	}
	// Done during the first check: the second is not called, the first's result is kept.
	var h lifecycle.Group
	mid, cancelMid := context.WithCancelCause(tctx)
	defer cancelMid(nil)
	_ = h.Add(lifecycle.Component{Name: "first", Live: func(context.Context) error { cancelMid(errProbe); return errBoom }})
	_ = h.Add(lifecycle.Component{Name: "second", Live: func(context.Context) error { calls.Add(1); return nil }})
	err := h.Live(mid)
	if !errors.Is(err, errBoom) || !errors.Is(err, errProbe) || !strings.Contains(err.Error(), "live first") {
		t.Fatalf("Live = %v, want errBoom (named) and errProbe", err)
	}
	if n := calls.Load(); n != 0 {
		t.Fatal("the second check ran after ctx was done")
	}
	// Ready with a done ctx while running: ErrNotReady wrapping the cause, no check called.
	var ready atomic.Int32
	res := make(chan error, 1)
	var k lifecycle.Group
	_ = k.Add(lifecycle.Component{Name: "db", Ready: func(context.Context) error { ready.Add(1); return nil }})
	_ = k.Add(lifecycle.Component{Name: "probe", Run: func(context.Context) error { res <- k.Ready(done); return nil }})
	if err := await(t, tctx, runAsync(tctx, &k)); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if err := <-res; !errors.Is(err, lifecycle.ErrNotReady) || !errors.Is(err, errProbe) {
		t.Fatalf("Ready with a done ctx = %v, want ErrNotReady wrapping errProbe", err)
	}
	if n := ready.Load(); n != 0 {
		t.Fatalf("%d Ready checks ran with a done ctx, want 0", n)
	}
}
