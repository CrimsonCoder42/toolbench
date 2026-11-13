package schedule

import (
	"context"
	"errors"
	"fmt"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// ErrSchedule is returned (wrapped, with both times) by Run when a Schedule's Next returns a non-zero time that
// is not after the time it was given. Without it a broken Schedule would make Run loop without waiting.
//
// ID:          TB-SCHED-011
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "func Is"
// Tags:        errors, sentinel, schedule
var ErrSchedule = errors.New("schedule: Next did not move forward")

// Run calls task at each run time of s, on the calling goroutine, until ctx is done, s has no further runs
// (Run returns nil), or a task error stops it. Runs never overlap. The first run is at s.Next(now). After a
// run scheduled at T ends at E, the next is s.Next(T) when that is after E (fixed rate, no drift); otherwise
// the run times the slow run missed are skipped and the next is s.Next(E): no burst of catch-up runs. A nil s
// has no runs. A nil ctx is context.Background().
//
// ID:          TB-SCHED-004
// Pattern:     fixed-rate scheduler, one run at a time; an overrun re-phases from its end
// Authority:   pkg.go.dev time, "func NewTicker" and "func (*Timer) Reset"
// Authority:   Go 1.23 release notes, "Timer changes" (no stale value after Reset or Stop)
// Goroutines:  none; task runs on the caller's goroutine, one run at a time
// Cancel:      ctx done before or while waiting: the timer is stopped and context.Cause(ctx) is returned without another run; ctx done during a run: the task sees it and context.Cause(ctx) is returned (a task error that is ctx's own error is the cause, not passed to WithOnError; any other task error is handled first)
// Errors:      the first task error is returned unchanged unless WithOnError is passed (then each goes to its function and Run continues); a panic in task is a *panics.PanicError (ErrPanic) handled the same way; ErrSchedule for a Next that does not move forward
// Blocking:    until ctx is done, s ends, or an error stops it; each wait is on one clock timer (WithClock), shifted by WithJitter
// Math:        NUM-PROB-059
// Tags:        concurrency, schedule, timer, ticker, jitter, cancellation
func Run(ctx context.Context, s Schedule, task func(context.Context) error, opts ...Option) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if s == nil {
		return nil
	}
	c := newConfig(opts)
	var timer clock.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	base := c.clock.Now() // the time the next run time was computed from
	next := s.Next(base)
	for {
		if next.IsZero() {
			return nil
		}
		if !next.After(base) {
			return fmt.Errorf("%w: Next(%v) = %v", ErrSchedule, base, next)
		}
		wait := max(next.Sub(c.clock.Now())+c.offset(next.Sub(base)), 0)
		if timer == nil {
			timer = c.clock.NewTimer(wait)
		} else {
			timer.Reset(wait) // go1.23 rules: no stale fire is received after Reset
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-timer.C():
		}
		if ctx.Err() != nil { // a cancelled ctx wins over a timer that fired at the same time
			return context.Cause(ctx)
		}

		if err := panics.Try(func() error { return task(ctx) }); err != nil {
			if cancelled(ctx, err) {
				return context.Cause(ctx)
			}
			if c.onError == nil {
				return err
			}
			if perr := panics.Try(func() error { c.onError(err); return nil }); perr != nil {
				return perr
			}
		}
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}

		end := c.clock.Now()
		base, next = next, s.Next(next)
		if !next.IsZero() && !next.After(end) { // the run overran: skip the run times it missed
			base, next = end, s.Next(end)
		}
	}
}

// cancelled reports whether err is ctx's own error after ctx is done: a task that returned because it was
// cancelled.
func cancelled(ctx context.Context, err error) bool {
	if ctx.Err() == nil {
		return false
	}
	return errors.Is(err, ctx.Err()) || errors.Is(err, context.Cause(ctx))
}
