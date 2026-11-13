package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// Group is a run group with ordered start and reverse-order stop. Add the components, then Run starts them in
// Add order, runs each component's Run on its own goroutine, waits until ctx (or the WithNotify context) is done
// or the first Run returns, and stops the started ones in reverse order within the grace period, then waits for
// the remaining Runs within the same period. Live and Ready aggregate the components' health checks; Ready is
// false until every Start has returned and from the instant shutdown begins. A Group runs once. The zero value
// is ready to use; a Group must not be copied after first use. Add, Run, Live and Ready are safe for concurrent
// use.
//
// ID:          TB-LIFE-004
// Pattern:     graceful shutdown (run group, ordered start, first failure ends all, reverse-order stop, drain deadline)
// Authority:   pkg.go.dev net/http, "func (*Server) Shutdown"
// Authority:   pkg.go.dev os/signal, "func NotifyContext"
// Authority:   pkg.go.dev golang.org/x/sync/errgroup, "func WithContext" ("canceled the first time a function passed to Go returns a non-nil error")
// Authority:   The Twelve-Factor App, "IX. Disposability" (https://12factor.net/disposability)
// Goroutines:  Run starts one per non-nil Component.Run (together, after the last Start) and one per Stop (one at a time); each has exited when Run returns, except a Stop or Run still running when the grace period ends (named in the error; it ends when that call returns; D-027)
// Cancel:      ctx done before every Start was called: the rest are not started, the started ones are stopped, Run returns the cause; after: shutdown, Run returns the stop errors; the first Component.Run to return has the same effect as ctx done
// Errors:      a Start error (or panic), the Run errors (or panics) and every Stop error (or panic), errors.Join; ErrStopTimeout; ErrClosed on a second Run; panics are panics.ErrPanic; Ready: ErrNotReady
// Blocking:    Run blocks until shutdown has finished or the grace period has ended
// Tags:        lifecycle, shutdown, graceful, signal, run group, health
type Group struct {
	mu    sync.Mutex  // guards comps and ran
	comps []Component // in Add order
	ran   bool        // Run has been called

	state atomic.Int32 // the readiness state (stateStarting, stateReady, stateStopping); written by Run only
}

// Add appends c to the components Run starts, after those already added. It returns ErrClosed, and c is not
// added, once Run has been called.
func (g *Group) Add(c Component) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ran {
		return ErrClosed
	}
	g.comps = append(g.comps, c)
	return nil
}

// Run starts the components, runs them and stops them; it returns when every started component has stopped or
// the grace period has ended.
//
// Start: with WithNotify, notify(ctx) is called first and its context replaces ctx. Then each Start is called
// on Run's goroutine with ctx, in Add order; component k+1 starts only after Start k returned nil. If Start k
// returns an error or panics, components k+1.. are not started and k-1..1 are stopped in reverse; k is not
// stopped (a failed Start cleans up after itself, as a constructor that returns an error). If ctx is done
// before Start k is called, the same happens to 1..k-1 and Run returns context.Cause(ctx).
//
// Run: once every Start has returned nil, Ready becomes true and each non-nil Component.Run is called on its own
// goroutine with the run ctx, a child of ctx. Run then waits for ctx to be done or for the first Component.Run
// to return, nil or error: either begins the shutdown. The run ctx is cancelled when shutdown begins, before
// the first Stop, with a cause naming the component that ended the group (or ctx's own cause).
//
// Stop: Ready becomes false, then the Stop of every started component is called in reverse order, one at a
// time, each with a ctx that keeps ctx's values, is not cancelled by ctx, and is cancelled with cause
// ErrStopTimeout when the grace period (WithGrace, measured on WithClock) ends. The stop ctx has no Deadline,
// since the clock may be fake. After the stops, every Component.Run not yet returned is waited for, one at a
// time in Add order, as a step of the same grace period named "run <name>". When the grace period ends first,
// Run returns at once with an ErrStopTimeout error naming the step still running (a Stop or a Run) and those
// never waited for. A clean shutdown returns nil; otherwise the start error or the first Run's error (unless it
// is the run ctx's own cancellation error), the later Run errors and every stop error, errors.Join, each naming
// its component. A nil ctx is context.Background(). A second Run returns ErrClosed. A Start that calls
// runtime.Goexit ends Run's goroutine, as it would any caller.
func (g *Group) Run(ctx context.Context, opts ...Option) error {
	if ctx == nil {
		ctx = context.Background()
	}
	g.mu.Lock()
	if g.ran {
		g.mu.Unlock()
		return ErrClosed
	}
	g.ran = true
	comps := slices.Clone(g.comps)
	g.mu.Unlock()

	cfg := newConfig(opts)
	if cfg.notify != nil {
		nctx, stop, err := callNotify(cfg.notify, ctx)
		if err != nil {
			return fmt.Errorf("lifecycle: notify: %w", err)
		}
		if stop != nil {
			defer stop()
		}
		if nctx != nil {
			ctx = nctx
		}
	}

	started := 0
	var firstErr error // the start error, or the first Run's error
	for i, c := range comps {
		if ctx.Err() != nil {
			firstErr = context.Cause(ctx)
			break
		}
		if c.Start != nil {
			if err := panics.Try(func() error { return c.Start(ctx) }); err != nil {
				firstErr = fmt.Errorf("lifecycle: start %s: %w", componentName(c, i), err)
				break
			}
		}
		started = i + 1
	}
	var runs *runners
	if firstErr == nil {
		g.state.Store(stateReady)
		runs = startRunners(ctx, comps)
		select {
		case <-ctx.Done():
		case j := <-runs.ended: // the first Run to return ends the group, as ctx done does
			firstErr = runs.first(j)
		}
	}
	g.state.Store(stateStopping) // readiness is false from here: before the run ctx is cancelled, before any Stop
	if runs != nil {
		runs.cancel(nil) // a no-op when ctx or first already did; keeps the cause already set
	}

	var steps []step
	for i := started - 1; i >= 0; i-- {
		if c := comps[i]; c.Stop != nil {
			steps = append(steps, step{name: "stop " + componentName(c, i), fn: c.Stop})
		}
	}
	if runs != nil {
		steps = append(steps, runs.steps()...)
	}
	stopErr := shutdown(ctx, cfg.grace, cfg.clock, steps)
	if firstErr == nil {
		return stopErr
	}
	if stopErr == nil {
		return firstErr
	}
	return errors.Join(firstErr, stopErr)
}

// componentName is c.Name, or "component <i+1>" when it is empty.
func componentName(c Component, i int) string {
	if c.Name != "" {
		return c.Name
	}
	return "component " + strconv.Itoa(i+1)
}

// callNotify calls notify with panics recovered.
func callNotify(notify func(context.Context) (context.Context, context.CancelFunc), ctx context.Context) (context.Context, context.CancelFunc, error) {
	type pair struct {
		ctx  context.Context
		stop context.CancelFunc
	}
	p, err := panics.TryValue(func() (pair, error) {
		c, s := notify(ctx)
		return pair{c, s}, nil
	})
	return p.ctx, p.stop, err
}
