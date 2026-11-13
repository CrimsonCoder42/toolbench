package group

import (
	"context"
	"errors"
	"sync"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// ErrZeroLimit is recorded as the group's error when Go is called, or is waiting for a slot, while the limit
// is zero. errgroup's Go blocks forever in that case; here fn is not started, Go returns at once and Wait
// reports ErrZeroLimit (first error wins, so an earlier error is kept).
//
// ID:          TB-GROUP-007
// Pattern:     error group (sentinel error)
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "func (*Group) SetLimit"
// Tags:        concurrency, errgroup, limit, sentinel
var ErrZeroLimit = errors.New("group: limit is zero: no goroutine can be started")

// Group runs functions on their own goroutines and waits for them; the first non-nil error is kept and, for a
// Group made by WithContext, cancels its context. It mirrors golang.org/x/sync/errgroup.Group, except that a
// panic in a function is recovered and returned by Wait as *panics.PanicError (errors.Is(err,
// panics.ErrPanic)), SetLimit never panics, and Go with a limit of zero records ErrZeroLimit instead of
// blocking forever. runtime.Goexit in a function is not an error: its goroutine ends and Wait does not hang.
// The zero value is ready to use: no limit, no cancel on error. A Group must not be copied after first use and
// is meant for one task.
//
// ID:          TB-GROUP-001
// Pattern:     error group (fork-join, first error cancels)
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "type Group"
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4 "Error Handling"
// Goroutines:  one per started Go or TryGo; each ends when its function returns; all have exited when Wait returns
// Cancel:      WithContext's ctx is cancelled with the first error as its cause, or when Wait returns
// Errors:      the first of: a function's error, a recovered panic (panics.ErrPanic), ErrZeroLimit; every one joined with SetCollect(true)
// Blocking:    Go blocks while the active count is at the limit; TryGo never blocks; Wait blocks until all return
// Tags:        concurrency, errgroup, cancel, limit, panic
type Group struct {
	wg sync.WaitGroup // one count per started goroutine

	mu      sync.Mutex              // guards every field below
	cancel  context.CancelCauseFunc // nil for a zero Group
	err     error                   // the first error; set once
	collect bool                    // SetCollect: keep every error in errs
	errs    []error                 // every error in completion order while collect is true; one per failed Go
	kept    bool                    // the first error is errs[0] (recorded while collecting); never compared, errors may be uncomparable
	limited bool                    // false: no limit (zero value, or SetLimit with n < 0)
	limit   int                     // the most active goroutines when limited
	active  int                     // goroutines started and not yet returned
	freed   chan struct{}           // closed when a slot frees or the limit changes; nil when nobody waits
}

// WithContext returns a new Group and a context derived from ctx. The derived context is cancelled the first
// time a function passed to Go returns a non-nil error (or panics), with that error as its cause
// (context.Cause), or the first time Wait returns, whichever comes first. A nil ctx is context.Background().
//
// ID:          TB-GROUP-002
// Pattern:     error group (fork-join, first error cancels)
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "func WithContext"
// Goroutines:  none
// Cancel:      the returned ctx is done when ctx is, on the group's first error, or when Wait returns
// Tags:        concurrency, errgroup, context, cancel
func WithContext(ctx context.Context) (*Group, context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancelCause(ctx)
	return &Group{cancel: cancel}, ctx
}

// Go calls fn on a new goroutine. It blocks until the goroutine can start without the number of active
// goroutines exceeding the limit. If the limit is zero, fn is not started and ErrZeroLimit is recorded. The
// first fn to return a non-nil error or to panic sets the error Wait returns and cancels the Group's context.
// As with errgroup, the first call to Go must happen before Wait; fn may call Go on its own Group. A nil fn is a
// panic of the call and is returned as panics.ErrPanic.
func (g *Group) Go(fn func() error) {
	ok, err := g.reserve(true)
	if err != nil {
		g.fail(err)
		return
	}
	if ok {
		g.start(fn)
	}
}

// TryGo calls fn on a new goroutine only if the number of active goroutines is below the limit, and reports
// whether it started. With a limit of zero it returns false and records nothing (errgroup's rule).
func (g *Group) TryGo(fn func() error) bool {
	ok, _ := g.reserve(false)
	if ok {
		g.start(fn)
	}
	return ok
}

// SetLimit limits the number of active goroutines to n. A negative n means no limit; zero means no goroutine
// can start (Go records ErrZeroLimit, TryGo returns false). Unlike errgroup it may be called while goroutines
// are active: running goroutines are not stopped, and the new limit applies to every later Go and TryGo,
// including a Go that is already waiting for a slot.
func (g *Group) SetLimit(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.limited = n >= 0
	g.limit = max(n, 0)
	g.wake()
}

// Wait blocks until every function started by Go and TryGo has returned, then cancels the Group's context and
// returns the first error, or nil. After SetCollect(true) it returns every error joined with errors.Join, in
// completion order, the first error first.
func (g *Group) Wait() error {
	g.wg.Wait()
	g.mu.Lock()
	err, cancel := g.err, g.cancel
	if g.collect && len(g.errs) > 0 {
		errs := g.errs
		if !g.kept { // the first error was recorded before collecting began: it is still first
			errs = append([]error{err}, errs...)
		}
		err = errors.Join(errs...)
	}
	g.mu.Unlock()
	if cancel != nil {
		cancel(err)
	}
	return err
}

// reserve takes one slot under the limit and adds one to wg. With wait it blocks until a slot is free; without
// it, it reports false when none is. It returns ErrZeroLimit when wait is true and the limit is zero.
func (g *Group) reserve(wait bool) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for g.limited && g.active >= g.limit {
		if !wait {
			return false, nil
		}
		if g.limit == 0 {
			return false, ErrZeroLimit
		}
		if g.freed == nil {
			g.freed = make(chan struct{})
		}
		freed := g.freed
		g.mu.Unlock()
		<-freed // closed by release or SetLimit; a running fn always returns, so a slot always frees
		g.mu.Lock()
	}
	g.active++
	g.wg.Add(1)
	return true, nil
}

// start runs fn on a new goroutine whose slot was reserved. The deferred calls run on return, on a recovered
// panic and on runtime.Goexit alike, so the slot and the wg count are always released.
func (g *Group) start(fn func() error) {
	go func() {
		defer g.wg.Done()
		defer g.release()
		if err := panics.Try(fn); err != nil {
			g.fail(err)
		}
	}()
}

// release frees one slot and wakes every Go waiting for one.
func (g *Group) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active--
	g.wake()
}

// wake closes freed so every waiting Go re-checks the limit. g.mu must be held.
func (g *Group) wake() {
	if g.freed != nil {
		close(g.freed)
		g.freed = nil
	}
}

// fail records err: the first error is kept and cancels the context with it; while collecting, every error is
// appended to errs in the order fail takes g.mu (completion order), bounded by one entry per failed Go.
func (g *Group) fail(err error) {
	g.mu.Lock()
	if g.collect {
		g.errs = append(g.errs, err)
	}
	if g.err != nil {
		g.mu.Unlock()
		return
	}
	g.err, g.kept = err, g.collect
	cancel := g.cancel
	g.mu.Unlock()
	if cancel != nil {
		cancel(err)
	}
}
