package retry

import (
	"context"
	"errors"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// ErrAttemptTimeout is the cause (context.Cause) of an attempt's ctx when WithAttemptTimeout expires, and it is
// wrapped into the error of an attempt that failed after its timeout. Match it with errors.Is.
//
// ID:          TB-RETRY-005
// Pattern:     sentinel error
// Authority:   pkg.go.dev context, "func WithTimeoutCause"
// Tags:        errors, sentinel, retry, timeout
var ErrAttemptTimeout = errors.New("retry: attempt timed out")

// attemptContext returns the ctx for one attempt bounded by d with the given cause, and its release function.
// With clock.Real it is context.WithTimeoutCause (a real deadline); with any other clock the clock's AfterFunc
// cancels it, so a fake clock drives it.
func attemptContext(ctx context.Context, c clock.Clock, d time.Duration, cause error) (context.Context, func()) {
	// An interface comparison compares dynamic types first, so a Clock of any other type (comparable or not)
	// is simply unequal: no run-time panic.
	if c == clock.Real() {
		actx, cancel := context.WithTimeoutCause(ctx, d, cause)
		return actx, cancel
	}
	actx, cancel := context.WithCancelCause(ctx)
	t := c.AfterFunc(d, func() { cancel(cause) })
	return actx, func() {
		t.Stop()
		cancel(context.Canceled)
	}
}
