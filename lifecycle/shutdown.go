package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// step is one call of a shutdown: a Stop, or Run waiting for its start to return. name is used in errors.
type step struct {
	name string
	fn   func(context.Context) error
}

// shutdown calls the steps one at a time, in order, each on its own goroutine, all under one grace deadline
// measured on clk. Each step gets a ctx that keeps parent's values, is not cancelled by parent, and is
// cancelled with cause ErrStopTimeout when the deadline passes. Every step error is kept (errors.Join, in step
// order, each wrapped with the step's name). When the deadline passes first, shutdown cancels the step ctx and
// returns at once with an ErrStopTimeout error naming the running step and the steps never called: the running
// step's goroutine is not waited for (it ends when the step returns; its result channel is buffered).
func shutdown(parent context.Context, grace time.Duration, clk clock.Clock, steps []step) error {
	if len(steps) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(parent))
	defer cancel(nil)
	timer := clk.NewTimer(grace)
	defer timer.Stop()
	var errs []error
	for i, s := range steps {
		done := make(chan error, 1) // buffered: an abandoned step still ends
		go func() {
			err := errGoexit
			defer func() { done <- err }()
			err = panics.Try(func() error { return s.fn(ctx) })
		}()
		select {
		case err := <-done:
			if err != nil {
				errs = append(errs, fmt.Errorf("lifecycle: %s: %w", s.name, err))
			}
		case <-timer.C():
			cancel(ErrStopTimeout)
			var notCalled []string
			for _, r := range steps[i+1:] {
				notCalled = append(notCalled, r.name)
			}
			errs = append(errs, timeoutError(s.name, grace, notCalled))
			return errors.Join(errs...)
		}
	}
	return errors.Join(errs...)
}
