package lifecycle

import (
	"context"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// DefaultGrace is the grace period when none, or a non-positive one, is given: the time every Stop together has
// to return once shutdown begins. 30s is the default termination grace period of common process managers
// (Kubernetes terminationGracePeriodSeconds) and faktory's ShutdownTimeout is 25s; choose one shorter than the
// manager's, which sends SIGKILL when it runs out.
//
// ID:          TB-LIFE-011
// Pattern:     graceful shutdown (drain deadline)
// Authority:   The Twelve-Factor App, "IX. Disposability" (https://12factor.net/disposability)
// Tags:        lifecycle, shutdown, deadline, default
const DefaultGrace = 30 * time.Second

// Option configures Group.Run. A nil Option is skipped.
//
// ID:          TB-LIFE-005
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        lifecycle, options
type Option func(*config)

type config struct {
	grace  time.Duration
	clock  clock.Clock
	notify func(context.Context) (context.Context, context.CancelFunc)
}

func newConfig(opts []Option) config {
	c := config{grace: DefaultGrace, clock: clock.Real()}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.grace <= 0 {
		c.grace = DefaultGrace
	}
	if c.clock == nil {
		c.clock = clock.Real()
	}
	return c
}

// WithGrace sets the grace period: once shutdown begins, every Stop together has d to return, measured on the
// clock from WithClock. A d <= 0 is DefaultGrace.
//
// ID:          TB-LIFE-006
// Pattern:     graceful shutdown (drain deadline)
// Authority:   pkg.go.dev net/http, "func (*Server) Shutdown"
// Tags:        lifecycle, shutdown, deadline, option
func WithGrace(d time.Duration) Option { return func(c *config) { c.grace = d } }

// WithClock sets the clock that measures the grace period (default clock.Real(); nil is clock.Real()). Tests
// pass a *clock.Fake.
//
// ID:          TB-LIFE-007
// Pattern:     injected clock (test double seam)
// Authority:   pkg.go.dev time, "func NewTimer"
// Tags:        lifecycle, clock, option, testing
func WithClock(c clock.Clock) Option { return func(cfg *config) { cfg.clock = c } }

// WithNotify sets the shutdown hook. Run calls notify(ctx) once, before the first Start, and uses the context
// it returns in place of ctx: when that context is done, shutdown begins. The CancelFunc is called when Run
// returns. Its shape is os/signal.NotifyContext's, so a main passes
//
//	lifecycle.WithNotify(func(ctx context.Context) (context.Context, context.CancelFunc) {
//		return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
//	})
//
// and this package never imports os/signal. A nil notify is no hook (the default); a nil context returned is
// ctx, a nil CancelFunc is not called; a panic in notify is returned by Run as panics.ErrPanic.
//
// ID:          TB-LIFE-008
// Pattern:     graceful shutdown (termination signal hook)
// Authority:   pkg.go.dev os/signal, "func NotifyContext"
// Authority:   The Twelve-Factor App, "IX. Disposability" ("Processes shut down gracefully when they receive a SIGTERM signal")
// Tags:        lifecycle, signal, shutdown, option
func WithNotify(notify func(context.Context) (context.Context, context.CancelFunc)) Option {
	return func(c *config) { c.notify = notify }
}
