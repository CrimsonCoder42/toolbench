package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// Stopper stops one component. It should return once the component has stopped (connections drained, work
// finished, goroutines ended), or when ctx is done, with ctx's error: the shape of http.Server.Shutdown, so
// srv.Shutdown is a Stopper. ctx is cancelled with cause ErrStopTimeout when the grace period ends; a Stopper
// that ignores it is abandoned and named in Run's error. A nil Stopper is skipped: Run and Group.Run never call
// it, and it counts as stopped.
//
// ID:          TB-LIFE-001
// Pattern:     graceful shutdown (drain with deadline)
// Authority:   pkg.go.dev net/http, "func (*Server) Shutdown"
// Errors:      its error is returned by Run wrapped with the component's name, joined with the others
// Blocking:    until the component has stopped or ctx is done
// Tags:        lifecycle, shutdown, stop, drain
type Stopper func(ctx context.Context) error

// Run calls start(ctx) on its own goroutine and waits until start returns or ctx is done. Then it calls the
// stops in reverse order (stops[len-1] first), one at a time, and waits for start to return, all within grace
// (a grace <= 0 is DefaultGrace), measured on the real clock. Each stop gets a ctx that keeps ctx's values, is
// not cancelled by ctx and is cancelled with cause ErrStopTimeout when grace ends; when grace ends first, Run
// returns at once with an ErrStopTimeout error naming the stop (or "start") still running and the stops never
// called. A typical start is an http.Server's ListenAndServe and a typical stop its Shutdown.
//
// start's error is returned, wrapped as "lifecycle: start: ...", except, after ctx is done, an error that
// matches ctx.Err() or context.Cause(ctx) (start returning because it was cancelled is a clean stop);
// http.ErrServerClosed is returned like any other error, so filter it in start. A start that returns before ctx
// is done (nil or an error) begins the shutdown at once. A nil start waits for ctx; a nil stop is skipped; a nil
// ctx is context.Background(). If ctx is done before the call, start and the stops are not called and Run
// returns context.Cause(ctx). For several components started in order, use Group.
//
// ID:          TB-LIFE-002
// Pattern:     graceful shutdown (drain with deadline, reverse-order stop)
// Authority:   pkg.go.dev net/http, "func (*Server) Shutdown"
// Authority:   The Twelve-Factor App, "IX. Disposability" (https://12factor.net/disposability)
// Goroutines:  one for start, one per stop (one at a time); all have exited when Run returns, except those still running when grace ends (named in the error; D-027): a stuck stop leaves one, a stuck start at most two (start's own and the one waiting for it), each ending when that call returns
// Cancel:      ctx done before the call: nothing runs, the cause is returned; while start runs: the shutdown begins
// Errors:      start's error and every stop error, errors.Join, each named; ErrStopTimeout; a panic is panics.ErrPanic
// Blocking:    until start has returned and every stop has returned, or grace has ended
// Tags:        lifecycle, shutdown, graceful, drain, server
func Run(ctx context.Context, grace time.Duration, start func(context.Context) error, stops ...Stopper) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if grace <= 0 {
		grace = DefaultGrace
	}
	if start == nil {
		start = func(ctx context.Context) error { <-ctx.Done(); return nil }
	}
	startDone := make(chan error, 1) // buffered: start's goroutine never blocks on its send
	go func() {
		err := errGoexit
		defer func() { startDone <- err }()
		err = panics.Try(func() error { return start(ctx) })
	}()

	var startErr error
	finished := false
	select {
	case err := <-startDone:
		finished = true
		if err != nil && !cancelled(ctx, err) {
			startErr = fmt.Errorf("lifecycle: start: %w", err)
		}
	case <-ctx.Done():
	}

	var steps []step
	for i := len(stops) - 1; i >= 0; i-- {
		if stops[i] != nil {
			steps = append(steps, step{name: "stop " + strconv.Itoa(i+1), fn: stops[i]})
		}
	}
	if !finished {
		steps = append(steps, step{name: "start", fn: func(context.Context) error {
			if err := <-startDone; err != nil && !cancelled(ctx, err) {
				return err
			}
			return nil
		}})
	}
	stopErr := shutdown(ctx, grace, clock.Real(), steps)
	if startErr == nil {
		return stopErr
	}
	if stopErr == nil {
		return startErr
	}
	return errors.Join(startErr, stopErr)
}

// cancelled reports whether err is ctx's own error after ctx is done: a start that returned because it was
// cancelled.
func cancelled(ctx context.Context, err error) bool {
	if ctx.Err() == nil {
		return false
	}
	return errors.Is(err, ctx.Err()) || errors.Is(err, context.Cause(ctx))
}
