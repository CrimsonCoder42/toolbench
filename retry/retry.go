package retry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// ErrExhausted is returned when every attempt failed with a retryable error. It wraps the last attempt's
// error: errors.Is matches both ErrExhausted and that error.
//
// ID:          TB-RETRY-004
// Pattern:     sentinel error
// Authority:   Azure Architecture Center, "Retry pattern", Microsoft Learn, 2024, "Solution" ("unsuccessful after a predefined number of attempts")
// Tags:        errors, sentinel, retry
var ErrExhausted = errors.New("retry: attempts exhausted")

// Do calls fn, on the calling goroutine, until it returns nil, returns an error that is not retried, ctx is
// done, or WithAttempts attempts (default 3) have failed. Between attempts it waits the Backoff's delay
// (default FullJitter(100ms, 10s)) on the WithClock clock. Every call is bounded: by WithAttempts (default 3;
// WithAttempts(0) is that default, never "forever"), by WithMaxElapsed, or by ctx; there is no retry-forever
// setting, at process start or anywhere else (ExampleDo_startupDependency). A nil ctx is context.Background();
// a nil fn is a panic in fn (panics.ErrPanic).
//
// ID:          TB-RETRY-001
// Pattern:     retry with capped exponential backoff and jitter
// Authority:   Azure Architecture Center, "Retry pattern", Microsoft Learn, 2024, "Retry strategies"
// Authority:   Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015, "Adding Jitter"
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns" (book, not opened)
// Goroutines:  none; fn runs on the caller's goroutine (with WithAttemptTimeout and a clock other than clock.Real, the clock runs the timeout's cancel, which has returned or been stopped when Do returns)
// Cancel:      ctx done before an attempt or during a wait: Do returns context.Cause(ctx) at once without another attempt; ctx done during fn: fn sees it and Do returns context.Cause(ctx); either way wrapped with the last attempt's error when there is one that is not ctx's own error
// Errors:      nil on success; an error marked Permanent, or rejected by WithRetryIf, is returned unchanged at once; a panic in fn is panics.ErrPanic (not retried by default); after the last attempt, or when the next wait would pass WithMaxElapsed, ErrExhausted wrapping the last error; a panic in a WithRetryIf or WithOnRetry function is panics.ErrPanic
// Blocking:    while fn runs and during each wait (one clock timer, stopped on return)
// Math:        ALG-BACKOF-001, ALG-BACKOF-002, ALG-BACKOF-003, ALG-BACKOF-004
// Tags:        retry, backoff, jitter, resilience, timeout, cancellation
func Do(ctx context.Context, fn func(context.Context) error, opts ...Option) error {
	var call func(context.Context) (struct{}, error)
	if fn != nil {
		call = func(ctx context.Context) (struct{}, error) { return struct{}{}, fn(ctx) }
	}
	_, err := DoValue(ctx, call, opts...)
	return err
}

// DoValue is Do for a function that returns a value: it returns the value of the attempt that succeeded, or
// T's zero value with the error.
//
// ID:          TB-RETRY-002
// Pattern:     retry with capped exponential backoff and jitter
// Authority:   Azure Architecture Center, "Retry pattern", Microsoft Learn, 2024, "Retry strategies"
// Authority:   Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015, "Adding Jitter"
// Goroutines:  none; as Do
// Cancel:      as Do
// Errors:      as Do; the value is T's zero value whenever the error is not nil
// Blocking:    as Do
// Math:        ALG-BACKOF-001, ALG-BACKOF-002, ALG-BACKOF-003, ALG-BACKOF-004
// Tags:        retry, backoff, jitter, resilience, generic
func DoValue[T any](ctx context.Context, fn func(context.Context) (T, error), opts ...Option) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	c := newConfig(opts)
	var timeoutCause error
	if c.attemptTimeout > 0 {
		timeoutCause = fmt.Errorf("%w after %v", ErrAttemptTimeout, c.attemptTimeout)
	}

	var began time.Time // the WithMaxElapsed origin; read only when there is a cap
	if c.maxElapsed > 0 {
		began = c.clock.Now()
	}

	var last error
	prev := c.backoff.base // DecorrelatedJitter's previous wait before the first retry
	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return zero, stopped(ctx, last)
		}
		v, err := runAttempt(ctx, &c, fn, timeoutCause)
		if err == nil {
			return v, nil
		}
		last = err
		if ctx.Err() != nil {
			return zero, stopped(ctx, last)
		}
		if isPermanent(err) {
			return zero, err
		}
		retry, perr := panics.TryValue(func() (bool, error) { return c.retryIf(err), nil })
		if perr != nil {
			return zero, perr
		}
		if !retry {
			return zero, err
		}
		if attempt >= c.attempts {
			return zero, fmt.Errorf("%w after %d attempts: %w", ErrExhausted, attempt, err)
		}
		wait, berr := c.backoff.next(attempt-1, prev, c.draw)
		if berr != nil { // unreachable: the Backoff is normalized
			return zero, berr
		}
		prev = wait
		if c.maxElapsed > 0 {
			// Written as wait > cap-elapsed so the sum cannot overflow; an attempt in flight past the cap is
			// never cut, it lands here with elapsed > cap and is not followed by another.
			if elapsed := c.clock.Now().Sub(began); wait > c.maxElapsed-elapsed {
				return zero, fmt.Errorf("%w: %v elapsed of the %v cap after %d attempts: %w", ErrExhausted, elapsed, c.maxElapsed, attempt, err)
			}
		}
		if c.onRetry != nil {
			if perr := panics.Try(func() error { c.onRetry(attempt, err, wait); return nil }); perr != nil {
				return zero, perr
			}
		}
		if err := c.sleep(ctx, wait); err != nil {
			return zero, stopped(ctx, last)
		}
	}
}

// runAttempt calls fn once with panic recovery and, when timeoutCause is set, a per-attempt ctx. An error after
// the attempt's ctx timed out (while ctx itself is not done) is wrapped with the timeout cause.
func runAttempt[T any](ctx context.Context, c *config, fn func(context.Context) (T, error), timeoutCause error) (T, error) {
	if timeoutCause == nil {
		return panics.TryValue(func() (T, error) { return fn(ctx) })
	}
	actx, release := attemptContext(ctx, c.clock, c.attemptTimeout, timeoutCause)
	defer release()
	v, err := panics.TryValue(func() (T, error) { return fn(actx) })
	if err != nil && ctx.Err() == nil && context.Cause(actx) == timeoutCause && !errors.Is(err, ErrAttemptTimeout) {
		err = fmt.Errorf("%w: %w", timeoutCause, err)
	}
	return v, err
}

// sleep waits d on the clock, or returns ctx's error as soon as ctx is done. A d <= 0 does not wait.
func (c *config) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := c.clock.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C():
		return nil
	}
}

// stopped is the error Do returns when ctx is done: its cause, wrapped with the last attempt's error when there
// is one that is not ctx's own error.
func stopped(ctx context.Context, last error) error {
	cause := context.Cause(ctx)
	if last == nil || errors.Is(last, cause) || errors.Is(last, ctx.Err()) {
		return cause
	}
	return fmt.Errorf("%w; last attempt: %w", cause, last)
}
