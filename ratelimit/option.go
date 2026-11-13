package ratelimit

import (
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// Option configures NewLimiter and NewKeyed. The zero value (a nil Option) is ignored. WithMaxKeys and
// WithIdleTTL apply to Keyed only; a Limiter ignores them.
//
// ID:          TB-RATE-006
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration
type Option func(*config)

// DefaultMaxKeys is the key bound of a Keyed limiter when WithMaxKeys is not passed (or n <= 0).
//
// ID:          TB-RATE-013
// Pattern:     bounded table (default bound)
// Authority:   pkg.go.dev golang.org/x/time/rate, "type Limiter" (one Limiter per client)
// Tags:        rate-limiting, bound, keys, default
const DefaultMaxKeys = 10_000

type config struct {
	clock   clock.Clock
	maxKeys int           // > 0 after newConfig
	idleTTL time.Duration // <= 0: the bucket's full-refill time (set by NewKeyed)
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
	if c.maxKeys <= 0 {
		c.maxKeys = DefaultMaxKeys
	}
	return c
}

// WithClock sets the clock the limiter reads and waits on. The default, and a nil c, is clock.Real(). Tests
// pass a *clock.Fake.
//
// ID:          TB-RATE-007
// Pattern:     functional options; injected clock
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, clock, time, testing
func WithClock(c clock.Clock) Option {
	return func(cfg *config) { cfg.clock = c }
}

// WithMaxKeys bounds how many keys a Keyed limiter holds at once. The default, and n <= 0, is
// DefaultMaxKeys (10 000). A new key that arrives when the bound is full and no key is idle is refused
// with ErrKeys.
//
// ID:          TB-RATE-008
// Pattern:     functional options; bounded table
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, rate-limiting, bound, keys
func WithMaxKeys(n int) Option {
	return func(cfg *config) { cfg.maxKeys = n }
}

// WithIdleTTL sets how long a key of a Keyed limiter may go unused before it is evicted. The default, and
// d <= 0, is the time the bucket takes to refill from empty to burst, rounded up to the nanosecond: a key
// idle that long has a full bucket, so evicting it changes no decision. A shorter d bounds memory more
// tightly but lets an evicted key start again with a full burst early.
//
// ID:          TB-RATE-009
// Pattern:     functional options; idle eviction
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, rate-limiting, eviction, ttl
func WithIdleTTL(d time.Duration) Option {
	return func(cfg *config) { cfg.idleTTL = d }
}
