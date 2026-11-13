package breaker

import (
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// The defaults: the threshold is Fowler's example (5); OpenFor and the probe count are the
// defaults of github.com/sony/gobreaker (Timeout 60 s, MaxRequests 1), the best-known Go breaker.
const (
	defaultThreshold = 5
	defaultOpenFor   = 60 * time.Second
	defaultProbes    = 1
)

// Option configures New. The zero value (a nil Option) is ignored.
//
// ID:          TB-BRK-006
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration, breaker
type Option func(*Breaker)

// WithFailureThreshold sets how many consecutive failures in Closed open the breaker. A success resets the
// count. The default, and any n <= 0, is 5.
//
// ID:          TB-BRK-007
// Pattern:     circuit breaker failure threshold
// Authority:   Fowler, "CircuitBreaker", martinfowler.com, 2014 ("Once the failures reach a certain threshold, the circuit breaker trips")
// Tags:        options, breaker, threshold
func WithFailureThreshold(n int) Option {
	return func(b *Breaker) { b.threshold = n }
}

// WithOpenFor sets how long the breaker stays Open before the next call moves it to HalfOpen (the reset
// timeout). The default, and any d <= 0, is 60 s.
//
// ID:          TB-BRK-008
// Pattern:     circuit breaker reset timeout
// Authority:   Microsoft, "Circuit Breaker pattern", Azure Architecture Center, "Solution" (the time-out timer of the Open state)
// Tags:        options, breaker, timeout, time
func WithOpenFor(d time.Duration) Option {
	return func(b *Breaker) { b.openFor = d }
}

// WithHalfOpenProbes sets how many probe calls may run at the same time in HalfOpen, which is also how many
// must succeed to close the breaker; any probe failure opens it again. The default, and any n <= 0, is 1.
//
// ID:          TB-BRK-009
// Pattern:     circuit breaker half-open trial requests
// Authority:   Microsoft, "Circuit Breaker pattern", Azure Architecture Center, "Solution" ("A limited number of requests from the application are allowed to pass through")
// Tags:        options, breaker, half-open, probes
func WithHalfOpenProbes(n int) Option {
	return func(b *Breaker) { b.probes = n }
}

// WithClock sets the clock OpenFor is measured on. The default, and a nil c, is clock.Real(). Tests pass a
// *clock.Fake and move time with Advance; the breaker arms no timers, it only reads Now.
//
// ID:          TB-BRK-010
// Pattern:     injected clock (test double seam)
// Authority:   pkg.go.dev time, "func Now" and "func (Time) Sub"
// Tags:        options, clock, time, testing
func WithClock(c clock.Clock) Option {
	return func(b *Breaker) { b.clock = c }
}

// WithOnStateChange sets a hook called with every transition (from, to), in the order they happened, one call
// at a time, outside the breaker's lock, on the goroutine of a Do call: usually the one that caused the
// transition; when calls race, possibly another caller's, which may be after the causing Do has returned.
// The hook may call the breaker (Do, State) without deadlock. The hook must not block: while it runs, the Do
// delivering it does not return, and at most 64 transitions wait for it; later ones are dropped, which the hook
// sees as a from that differs from the previous to. A panic in the hook is recovered and returned,
// joined with its own result, by the Do call that ran it. A nil f is no hook (the default).
//
// ID:          TB-BRK-011
// Pattern:     state-change hook (observer)
// Authority:   Fowler, "CircuitBreaker", martinfowler.com, 2014 ("Any change in breaker state should be logged")
// Tags:        options, breaker, hook, observability
func WithOnStateChange(f func(from, to State)) Option {
	return func(b *Breaker) { b.onChange = f }
}
