// Package lifecycle_test compares toolbench/lifecycle with github.com/oklog/run v1.2.0 (Group.Add(execute,
// interrupt), Group.Run): the same components given to both are all started and all stopped, and the error
// that ended the run is returned by both.
//
// Known differences, by design (doc/specs/lifecycle.md):
//   - Order: oklog runs every actor concurrently and, when the first returns, calls every interrupt in Add
//     order; lifecycle.Group starts in Add order, one at a time, and stops in reverse order (the last started is
//     the first stopped), so a server drains before the database it uses is closed.
//   - Deadline: oklog's Run waits for every actor without bound; lifecycle stops within a grace period and
//     names the component still stopping (ErrStopTimeout).
//   - Errors: oklog returns only the first actor's error; lifecycle returns the start error and every stop
//     error joined (errors.Join), each naming its component.
//   - Panics: a panic in an oklog actor runs on oklog's goroutine and crashes the process (not testable here);
//     lifecycle returns panics.ErrPanic.
//   - Signals: oklog has SignalHandler (imports os/signal); lifecycle takes the hook through WithNotify.
package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/oklog/run"

	"github.com/CrimsonCoder42/toolbench/lifecycle"
)

var errFail = errors.New("start failed")

type recorder struct {
	mu  sync.Mutex
	log []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, s)
}

func (r *recorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.log)
}

func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

var names = []string{"db", "cache", "server"}

// TestCompare_stopOrder: both stop every component; oklog interrupts in Add order, lifecycle stops in reverse.
func TestCompare_stopOrder(t *testing.T) {
	// oklog: each actor blocks until interrupted; a final actor returns at once and ends the run.
	var ok recorder
	var g run.Group
	for _, n := range names {
		quit := make(chan struct{})
		g.Add(func() error { <-quit; return nil }, func(error) { ok.add("stop " + n); close(quit) })
	}
	g.Add(func() error { return nil }, func(error) {})
	if err := g.Run(); err != nil {
		t.Fatalf("oklog Run = %v", err)
	}

	var ours recorder
	ctx, cancel := context.WithCancel(bounded(t))
	defer cancel()
	var lg lifecycle.Group
	for _, n := range names {
		_ = lg.Add(lifecycle.Component{Name: n, Stop: func(context.Context) error { ours.add("stop " + n); return nil }})
	}
	_ = lg.Add(lifecycle.Component{Start: func(context.Context) error { cancel(); return nil }})
	if err := lg.Run(ctx); err != nil {
		t.Fatalf("lifecycle Run = %v", err)
	}

	if got, want := ok.get(), []string{"stop db", "stop cache", "stop server"}; !slices.Equal(got, want) {
		t.Fatalf("oklog order = %v, want %v", got, want)
	}
	if got, want := ours.get(), []string{"stop server", "stop cache", "stop db"}; !slices.Equal(got, want) {
		t.Fatalf("lifecycle order = %v, want %v", got, want)
	}
}

// TestCompare_startFailure: the failing component's error ends the run in both; every other running component
// is stopped. lifecycle does not start the components after the failing one.
func TestCompare_startFailure(t *testing.T) {
	var ok recorder
	var g run.Group
	for _, n := range names[:2] {
		quit := make(chan struct{})
		g.Add(func() error { <-quit; return nil }, func(error) { ok.add("stop " + n); close(quit) })
	}
	g.Add(func() error { return errFail }, func(error) {})
	if err := g.Run(); !errors.Is(err, errFail) {
		t.Fatalf("oklog Run = %v, want errFail", err)
	}

	var ours recorder
	var lg lifecycle.Group
	for _, n := range names[:2] {
		_ = lg.Add(lifecycle.Component{Name: n, Stop: func(context.Context) error { ours.add("stop " + n); return nil }})
	}
	_ = lg.Add(lifecycle.Component{Name: "server", Start: func(context.Context) error { return errFail }})
	if err := lg.Run(bounded(t)); !errors.Is(err, errFail) {
		t.Fatalf("lifecycle Run = %v, want errFail", err)
	}

	if got := ok.get(); len(got) != 2 {
		t.Fatalf("oklog stops = %v, want both", got)
	}
	if got, want := ours.get(), []string{"stop cache", "stop db"}; !slices.Equal(got, want) {
		t.Fatalf("lifecycle stops = %v, want %v", got, want)
	}
}
