package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// The readiness state of a Group (Group.state, an atomic): it moves forward only, on Run's goroutine.
const (
	stateStarting int32 = iota // Run not called, or a Start has not yet returned
	stateReady                 // every Start returned nil and shutdown has not begun
	stateStopping              // shutdown has begun, or a Start failed; the final state
)

// Live reports whether the process is alive: it calls the Live check of every added component, one at a time
// on the caller's goroutine, each with ctx, in Add order, and returns nil or their errors joined, each wrapped
// "lifecycle: live <name>: ...". It is independent of the start/stop state: it answers before Run, while the
// components start, while they run, while they stop and after Run returned, so a check must be able to answer
// before its Start. Checks run one at a time (this package does not fan out); the caller sets the deadline with
// ctx and caches the result where probes are frequent: nothing is cached here. A nil ctx is context.Background().
// Safe for concurrent use with Add, Run and Ready.
//
// ID:          TB-LIFE-012
// Pattern:     health endpoint monitoring (liveness check)
// Authority:   Microsoft Azure Architecture Center, "Health Endpoint Monitoring pattern", "Solution" ("Bound each dependency check with a timeout"; "health signals that load balancers and traffic routers can use to direct requests to healthy instances")
// Goroutines:  none; the checks run on the caller's goroutine
// Cancel:      ctx done: the remaining checks are not called and context.Cause(ctx) is joined with the errors so far
// Errors:      every failing check, errors.Join, each wrapped with the component's name; a panic in a check is panics.ErrPanic
// Blocking:    for as long as the checks take, one after another; bound it with ctx
// Tags:        lifecycle, health, liveness, check
func (g *Group) Live(ctx context.Context) error {
	return g.check(ctx, "live", func(c Component) func(context.Context) error { return c.Live })
}

// Ready reports whether the process can serve. It returns an error matching ErrNotReady, without calling any
// check, before every Start has returned nil (also before Run is called, and after a Start failed) and from the
// instant shutdown begins (ctx done, or the first Run returned): the state flips before the run ctx is cancelled
// and before the first Stop is called, so a load balancer that probes stops routing while the components drain;
// it stays not ready after Run returned. While the group is running it calls the Ready check of every added
// component as Live does, and returns nil or ErrNotReady wrapping their errors joined, each
// "lifecycle: ready <name>: ...". The state is read once, before the checks. A nil ctx is context.Background().
// Safe for concurrent use with Add, Run and Live.
//
// ID:          TB-LIFE-013
// Pattern:     health endpoint monitoring (readiness check; false while starting and from the start of shutdown)
// Authority:   Microsoft Azure Architecture Center, "Health Endpoint Monitoring pattern", "Example > Endpoint behavior" ("Readiness ... Check initialization and both required dependencies ... 503 (Service Unavailable) when a required check fails. Bound each dependency check with a timeout")
// Authority:   The Twelve-Factor App, "IX. Disposability" ("ceasing to listen on the service port ..., allowing any current requests to finish, and then exiting")
// Goroutines:  none; the checks run on the caller's goroutine
// Cancel:      ctx done: the remaining checks are not called and context.Cause(ctx) is joined under ErrNotReady
// Errors:      ErrNotReady, alone while starting or stopping, wrapping every failing check (each named) while running; a panic in a check is panics.ErrPanic
// Blocking:    for as long as the checks take, one after another; bound it with ctx; never while starting or stopping
// Tags:        lifecycle, health, readiness, check, shutdown
func (g *Group) Ready(ctx context.Context) error {
	switch g.state.Load() {
	case stateReady:
	case stateStopping:
		return fmt.Errorf("%w: shutting down", ErrNotReady)
	default:
		return fmt.Errorf("%w: not every component has started", ErrNotReady)
	}
	if err := g.check(ctx, "ready", func(c Component) func(context.Context) error { return c.Ready }); err != nil {
		return fmt.Errorf("%w: %w", ErrNotReady, err)
	}
	return nil
}

// check calls pick(c) for every added component whose pick is non-nil, one at a time with ctx, stopping once ctx
// is done, and joins the errors, each wrapped "lifecycle: <kind> <name>: ...". The components are copied under
// mu; no lock is held while a check runs.
func (g *Group) check(ctx context.Context, kind string, pick func(Component) func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	g.mu.Lock()
	comps := slices.Clone(g.comps)
	g.mu.Unlock()
	var errs []error
	for i, c := range comps {
		fn := pick(c)
		if fn == nil {
			continue
		}
		if ctx.Err() != nil {
			errs = append(errs, context.Cause(ctx))
			break
		}
		if err := panics.Try(func() error { return fn(ctx) }); err != nil {
			errs = append(errs, fmt.Errorf("lifecycle: %s %s: %w", kind, componentName(c, i), err))
		}
	}
	return errors.Join(errs...)
}
