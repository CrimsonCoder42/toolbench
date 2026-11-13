package schedule

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/CrimsonCoder42/calcbench/num/prob"
	"github.com/CrimsonCoder42/toolbench/clock"
)

// Option configures Run and Heartbeat. The zero value (a nil Option) is ignored.
//
// ID:          TB-SCHED-006
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration
type Option func(*config)

type config struct {
	clock   clock.Clock
	jitter  float64 // in [0, 1]
	onError func(error)
	rand    *rand.Rand // nil: the top-level math/rand/v2 source
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

// offset returns the jitter for a wait of the given interval: a uniform value in [-jitter·interval,
// +jitter·interval) from calcbench NUM-PROB-059, or 0 when there is no jitter.
func (c *config) offset(interval time.Duration) time.Duration {
	if c.jitter <= 0 || interval <= 0 {
		return 0
	}
	var u float64
	if c.rand != nil {
		u = c.rand.Float64()
	} else {
		u = rand.Float64()
	}
	span := c.jitter * float64(interval)
	x, err := prob.UniformQuantile(u, -span, span)
	if err != nil { // unreachable: u is in [0, 1) and span > 0 is finite
		return 0
	}
	return time.Duration(math.Round(x))
}

// WithClock sets the clock Run and Heartbeat wait on. The default, and a nil c, is clock.Real(). Tests pass a
// *clock.Fake and move time with AdvanceContext.
//
// ID:          TB-SCHED-007
// Pattern:     injected clock (test double seam)
// Authority:   pkg.go.dev time, "func NewTimer" and "func (*Timer) Reset"
// Tags:        options, clock, time, testing
func WithClock(c clock.Clock) Option {
	return func(cfg *config) { cfg.clock = c }
}

// WithJitter shifts each wait of Run by a uniform random offset in [-frac·I, +frac·I), where I is the interval
// from the time given to Next to the run time it returned, so replicas on the same schedule do not all run at
// once. The run times themselves are not moved: jitter does not accumulate. The default is 0 (no jitter); a
// frac below 0 or NaN is 0, above 1 is 1. A wait that the offset would make negative is 0. Heartbeat ignores
// it.
//
// ID:          TB-SCHED-008
// Pattern:     jitter (randomised delay)
// Authority:   pkg.go.dev time, "func NewTicker"
// Math:        NUM-PROB-059
// Tags:        options, jitter, random, schedule
func WithJitter(frac float64) Option {
	switch {
	case !(frac > 0): // also NaN
		frac = 0
	case frac > 1:
		frac = 1
	}
	return func(cfg *config) { cfg.jitter = frac }
}

// WithOnError passes every task error (and every recovered panic, as a *panics.PanicError) to f, on the Run
// goroutine, and keeps the schedule going. Without it, or with a nil f, Run stops at the first task error and
// returns it. A panic in f stops Run and is returned as panics.ErrPanic. Heartbeat ignores it.
//
// ID:          TB-SCHED-009
// Pattern:     error callback (hook)
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, errors, hook, schedule
func WithOnError(f func(error)) Option {
	return func(cfg *config) { cfg.onError = f }
}

// WithRand sets the source of the uniform value WithJitter draws (math/rand/v2). The default, and a nil r, is
// the top-level math/rand/v2 source. r is used only from the Run goroutine: do not share one r between Runs
// that run at the same time. Tests pass rand.New(rand.NewPCG(seed1, seed2)) for repeatable jitter. Heartbeat
// ignores it.
//
// ID:          TB-SCHED-010
// Pattern:     dependency injection (injected random source)
// Authority:   pkg.go.dev math/rand/v2, "func New" and "func NewPCG"
// Tags:        options, random, jitter, testing
func WithRand(r *rand.Rand) Option {
	return func(cfg *config) { cfg.rand = r }
}
