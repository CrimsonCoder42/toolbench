package batch

import "github.com/CrimsonCoder42/toolbench/clock"

// Option configures Channel. The zero value (a nil Option) is ignored.
//
// ID:          TB-BATCH-003
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration
type Option func(*config)

type config struct {
	clock clock.Clock
}

func newConfig(opts []Option) config {
	var c config
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.clock == nil {
		c.clock = clock.Real()
	}
	return c
}

// WithClock sets the clock Channel measures maxWait with. The default, and a nil c, is clock.Real(). Tests pass
// a *clock.Fake and move time with AdvanceContext after BlockUntil has seen the batch timer armed.
//
// ID:          TB-BATCH-004
// Pattern:     injected clock (test double seam)
// Authority:   pkg.go.dev time, "NewTimer" and "func (*Timer) Stop"
// Tags:        options, clock, time, testing
func WithClock(c clock.Clock) Option {
	return func(cfg *config) { cfg.clock = c }
}
