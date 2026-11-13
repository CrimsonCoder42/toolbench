package retry

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// defaultAttempts is the total number of attempts when WithAttempts is not passed (one call and two retries).
const defaultAttempts = 3

// Option configures Do and DoValue. The zero value (a nil Option) is ignored.
//
// ID:          TB-RETRY-011
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration
type Option func(*config)

type config struct {
	attempts       int
	backoff        Backoff
	attemptTimeout time.Duration // <= 0: none
	maxElapsed     time.Duration // <= 0: no cap
	retryIf        func(error) bool
	onRetry        func(attempt int, err error, wait time.Duration)
	clock          clock.Clock
	rand           *rand.Rand // nil: the top-level math/rand/v2 source
}

func newConfig(opts []Option) config {
	c := config{attempts: defaultAttempts}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.attempts <= 0 {
		c.attempts = defaultAttempts
	}
	c.backoff = c.backoff.normalized()
	if c.retryIf == nil {
		c.retryIf = retryable
	}
	if c.clock == nil {
		c.clock = clock.Real()
	}
	return c
}

// draw returns a uniform value in [0, 1) from the configured source.
func (c *config) draw() float64 {
	if c.rand != nil {
		return c.rand.Float64()
	}
	return rand.Float64()
}

// retryable is the default WithRetryIf: every error except a panic, and except a context error that is not an
// attempt timeout (a context error from fn means fn's own ctx ended, and retrying will not help).
func retryable(err error) bool {
	switch {
	case errors.Is(err, panics.ErrPanic):
		return false
	case errors.Is(err, ErrAttemptTimeout):
		return true
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return false
	}
	return true
}

// WithAttempts sets the total number of attempts, the first call included: WithAttempts(1) calls fn once and
// never retries. The default, and an n <= 0, is 3. There is no unbounded setting.
//
// ID:          TB-RETRY-012
// Pattern:     bounded retry count
// Authority:   Azure Architecture Center, "Retry pattern", Microsoft Learn, 2024, "Retry strategies" ("until some maximum number of requests have been attempted")
// Tags:        options, retry, bound
func WithAttempts(n int) Option {
	return func(c *config) { c.attempts = n }
}

// WithBackoff sets the delay between attempts. The default, and the zero Backoff, is FullJitter(100ms, 10s).
//
// ID:          TB-RETRY-013
// Pattern:     capped exponential backoff with jitter
// Authority:   Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015, "Adding Jitter"
// Tags:        options, retry, backoff, jitter
func WithBackoff(b Backoff) Option {
	return func(c *config) { c.backoff = b }
}

// WithAttemptTimeout bounds each attempt to d: fn gets a ctx that ends d after the attempt starts, with
// context.Cause equal to an error matching ErrAttemptTimeout. With clock.Real the ctx is made by
// context.WithTimeoutCause, so it carries a deadline and ctx.Err() is context.DeadlineExceeded; with any other
// clock (clock.Fake in tests) it is cancelled by the clock's AfterFunc, has no deadline, and ctx.Err() is
// context.Canceled. An attempt that fails after its timeout returns an error matching both ErrAttemptTimeout
// and fn's error, and the default WithRetryIf retries it. fn must honour its ctx: Do waits for fn to return.
// The default, and a d <= 0, is no per-attempt timeout.
//
// ID:          TB-RETRY-014
// Pattern:     timeout (per attempt, with cause)
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns", "Timeouts" (book, not opened)
// Authority:   pkg.go.dev context, "func WithTimeoutCause" and "func WithCancelCause"
// Tags:        options, retry, timeout, context, cause
func WithAttemptTimeout(d time.Duration) Option {
	return func(c *config) { c.attemptTimeout = d }
}

// WithMaxElapsed caps the total time of a Do call at d, measured on the WithClock clock from just before the
// first attempt. After a failed attempt that would be retried, when the elapsed time plus the next wait would
// pass d, Do does not start the wait and returns ErrExhausted wrapping the last error, with the elapsed time in
// the message; so no attempt starts after d has passed. Unlike a ctx deadline it never cuts an attempt in flight:
// fn runs to its end and its result is kept (a success past the cap is a success). It bounds the waits between
// attempts, not fn: pair it with a ctx deadline or WithAttemptTimeout to bound fn. Whichever of WithAttempts and
// WithMaxElapsed is reached first stops the call; WithOnRetry is not called for a wait that is not started. The
// default, and a d <= 0, is no cap.
//
// ID:          TB-RETRY-019
// Pattern:     bounded retry duration (maximum elapsed time)
// Authority:   Azure Architecture Center, "Retry pattern", Microsoft Learn, 2024, "Issues and considerations", "Impact on performance" (stop "continually trying to perform a failing operation")
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns", "Timeouts" (book, not opened)
// Tags:        options, retry, bound, timeout, elapsed
func WithMaxElapsed(d time.Duration) Option {
	return func(c *config) { c.maxElapsed = d }
}

// WithRetryIf sets the function that decides whether an attempt's error is retried; false returns that error
// unchanged at once. It is not called for an error marked Permanent (never retried) or after ctx is done. The
// default, and a nil f, retries every error except one matching panics.ErrPanic, context.Canceled or
// context.DeadlineExceeded, and retries an attempt that ended by WithAttemptTimeout. f runs on the caller's
// goroutine; a panic in f is returned as panics.ErrPanic. The default retries every other error, breaker.ErrOpen
// included: around a breaker.Do, pass WithRetryIf(func(err error) bool { return !errors.Is(err, breaker.ErrOpen) })
// so an open circuit is not waited out (ExampleWithRetryIf_breakerOpen).
//
// ID:          TB-RETRY-015
// Pattern:     retry predicate (transient-fault detection)
// Authority:   Azure Architecture Center, "Retry pattern", Microsoft Learn, 2024, "Retry strategies" and "Exception type"
// Tags:        options, retry, errors, predicate
func WithRetryIf(f func(error) bool) Option {
	return func(c *config) { c.retryIf = f }
}

// WithOnRetry calls f before each wait, on the caller's goroutine, with the number of the attempt that failed
// (1 for the first call), its error and the wait that follows. It is not called after the last attempt or for
// an error that is not retried. A panic in f stops Do and is returned as panics.ErrPanic. Default: none.
//
// ID:          TB-RETRY-016
// Pattern:     observer hook
// Authority:   Azure Architecture Center, "Retry pattern", Microsoft Learn, 2024, "General guidance" ("Log all connectivity failures that cause a retry")
// Tags:        options, retry, hook, observability
func WithOnRetry(f func(attempt int, err error, wait time.Duration)) Option {
	return func(c *config) { c.onRetry = f }
}

// WithClock sets the clock the waits and the WithAttemptTimeout timers use. The default, and a nil c, is
// clock.Real(). Tests pass a *clock.Fake and move time with AdvanceContext.
//
// ID:          TB-RETRY-017
// Pattern:     injected clock (test double seam)
// Authority:   pkg.go.dev time, "func NewTimer" and "func AfterFunc"
// Tags:        options, clock, time, testing
func WithClock(c clock.Clock) Option {
	return func(cfg *config) { cfg.clock = c }
}

// WithRand sets the source of the uniform draws the jittered Backoffs use (math/rand/v2). The default, and a
// nil r, is the top-level math/rand/v2 source. r is used only from the calling goroutine: do not share one r
// between Do calls that run at the same time. Tests pass rand.New(rand.NewPCG(seed1, seed2)).
//
// ID:          TB-RETRY-018
// Pattern:     dependency injection (injected random source)
// Authority:   pkg.go.dev math/rand/v2, "func New" and "func NewPCG"
// Tags:        options, random, jitter, testing
func WithRand(r *rand.Rand) Option {
	return func(cfg *config) { cfg.rand = r }
}
