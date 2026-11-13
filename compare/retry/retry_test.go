// Package retry_test compares toolbench/retry with github.com/cenkalti/backoff/v4 on behaviour only: the number
// of attempts, Permanent, and cancellation. Never timings: both run with zero or fake-clock waits
// (cenkalti ZeroBackOff, toolbench a clock.Fake whose timers fire when armed).
//
// Known differences, by design (doc/specs/retry.md):
//   - cenkalti counts retries (WithMaxRetries(b, n) makes n+1 attempts) and is unbounded without it; toolbench
//     counts attempts (WithAttempts(n), default 3).
//   - cenkalti returns the last error bare after the retries run out; toolbench returns ErrExhausted wrapping
//     it. errors.Is(err, last) holds for both.
//   - cenkalti strips the Permanent marker and returns the inner error; toolbench returns fn's error unchanged
//     (the marker is transparent: same message, errors.Is/As see the inner error).
//   - cenkalti calls the operation once even when ctx is already done; toolbench makes no attempt.
package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/retry"
)

var errFlaky = errors.New("flaky")

// instant is a clock.Fake whose timers fire as soon as they are armed (see retry/main_test.go).
type instant struct{ *clock.Fake }

func (i instant) NewTimer(d time.Duration) clock.Timer {
	t := i.Fake.NewTimer(d)
	i.Fake.Advance(d)
	return t
}

func newInstant() instant {
	return instant{clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))}
}

// failing returns an operation that fails the first fails calls and its call counter.
func failing(fails int) (func() error, *int) {
	n := 0
	return func() error {
		n++
		if n <= fails {
			return errFlaky
		}
		return nil
	}, &n
}

func TestCompare_attemptCounts(t *testing.T) {
	for _, tc := range []struct{ attempts, fails int }{{1, 0}, {1, 5}, {3, 1}, {3, 2}, {3, 3}, {5, 10}, {10, 4}} {
		op, theirs := failing(tc.fails)
		theirErr := backoff.Retry(op, backoff.WithMaxRetries(&backoff.ZeroBackOff{}, uint64(tc.attempts-1)))

		op2, ours := failing(tc.fails)
		ourErr := retry.Do(t.Context(), func(context.Context) error { return op2() },
			retry.WithAttempts(tc.attempts), retry.WithClock(newInstant()))

		if *theirs != *ours {
			t.Errorf("%+v: calls cenkalti %d, toolbench %d", tc, *theirs, *ours)
		}
		if (theirErr == nil) != (ourErr == nil) || (theirErr != nil && (!errors.Is(theirErr, errFlaky) || !errors.Is(ourErr, errFlaky))) {
			t.Errorf("%+v: err cenkalti %v, toolbench %v", tc, theirErr, ourErr)
		}
		if ourErr != nil && !errors.Is(ourErr, retry.ErrExhausted) {
			t.Errorf("%+v: toolbench err %v is not ErrExhausted", tc, ourErr)
		}
	}
}

func TestCompare_permanentStopsAtOnce(t *testing.T) {
	cause := errors.New("bad input")
	theirs := 0
	theirErr := backoff.Retry(func() error {
		theirs++
		if theirs == 2 {
			return backoff.Permanent(cause)
		}
		return errFlaky
	}, backoff.WithMaxRetries(&backoff.ZeroBackOff{}, 9))

	ours := 0
	ourErr := retry.Do(t.Context(), func(context.Context) error {
		ours++
		if ours == 2 {
			return retry.Permanent(cause)
		}
		return errFlaky
	}, retry.WithAttempts(10), retry.WithClock(newInstant()))

	if theirs != 2 || ours != 2 {
		t.Fatalf("calls cenkalti %d, toolbench %d; want 2", theirs, ours)
	}
	if !errors.Is(theirErr, cause) || !errors.Is(ourErr, cause) || theirErr.Error() != ourErr.Error() {
		t.Fatalf("err cenkalti %v, toolbench %v", theirErr, ourErr)
	}
}

// ctx cancelled inside attempt k: both stop after k calls and return an error matching context.Canceled.
func TestCompare_cancelStops(t *testing.T) {
	for _, k := range []int{1, 2, 4} {
		ctx, cancel := context.WithCancel(t.Context())
		theirs := 0
		theirErr := backoff.Retry(func() error {
			theirs++
			if theirs == k {
				cancel()
			}
			return errFlaky
		}, backoff.WithContext(backoff.WithMaxRetries(&backoff.ZeroBackOff{}, 9), ctx))
		cancel()

		ctx2, cancel2 := context.WithCancel(t.Context())
		ours := 0
		ourErr := retry.Do(ctx2, func(context.Context) error {
			ours++
			if ours == k {
				cancel2()
			}
			return errFlaky
		}, retry.WithAttempts(10), retry.WithClock(newInstant()))
		cancel2()

		if theirs != k || ours != k {
			t.Errorf("k %d: calls cenkalti %d, toolbench %d", k, theirs, ours)
		}
		if !errors.Is(theirErr, context.Canceled) || !errors.Is(ourErr, context.Canceled) {
			t.Errorf("k %d: err cenkalti %v, toolbench %v", k, theirErr, ourErr)
		}
	}
}

// The documented difference: with ctx done before the call, cenkalti still calls once; toolbench never calls.
func TestCompare_cancelBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	op, theirs := failing(100)
	_ = backoff.Retry(op, backoff.WithContext(backoff.WithMaxRetries(&backoff.ZeroBackOff{}, 9), ctx))
	op2, ours := failing(100)
	err := retry.Do(ctx, func(context.Context) error { return op2() }, retry.WithClock(newInstant()))
	if *theirs != 1 || *ours != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("calls cenkalti %d (want 1), toolbench %d (want 0), err %v", *theirs, *ours, err)
	}
}
