package flow

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// FanOut runs fn on every value from in using a fixed number of worker goroutines and sends the results on
// the returned channel, in completion order. The first error (or panic) from fn cancels the ctx passed to the
// other calls and stops the workers; the returned wait func reports it. workers < 1 is 1. A nil in is an
// input with no values.
//
// ID:          TB-FLOW-004
// Pattern:     fan-out (competing consumers), bounded parallelism
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Fan-out, fan-in"
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Bounded parallelism"
// Goroutines:  workers + 1 closer; all have exited when the channel is closed and when wait returns
// Cancel:      ctx done: workers stop at their next receive or send (fn sees a cancelled ctx); the channel is closed
// Errors:      wait returns the first fn error, or a *panics.PanicError (errors.Is ErrPanic) for a panic, else ctx's error if work was cut short by ctx, else nil
// Blocking:    sends wait for the consumer (WithBuffer adds slack); wait blocks until the channel is closed, so drain it or cancel ctx first
// Tags:        concurrency, pipeline, workers, fan-out
func FanOut[In, Out any](ctx context.Context, in <-chan In, workers int, fn func(context.Context, In) (Out, error), opts ...Option) (<-chan Out, func() error) {
	c := newConfig(opts)
	workers = max(workers, 1)
	out := make(chan Out, c.buffer)
	done := make(chan struct{})
	wctx, cancel := context.WithCancel(ctx)

	var (
		once     sync.Once
		firstErr error
		cut      atomic.Bool // a worker stopped while ctx was done (work cut short)
		wg       sync.WaitGroup
		result   error
	)
	fail := func(err error) {
		once.Do(func() { firstErr = err })
		cancel()
	}
	for range workers {
		wg.Go(func() {
			if in == nil {
				return
			}
			for {
				// A cancelled ctx wins over a ready value, so fn is not started after cancellation is seen.
				if wctx.Err() != nil {
					cut.Store(true)
					return
				}
				select {
				case <-wctx.Done():
					cut.Store(true)
					return
				case v, ok := <-in:
					if !ok {
						// An upstream stage on the same ctx closes its output when ctx is done.
						if ctx.Err() != nil {
							cut.Store(true)
						}
						return
					}
					r, err := panics.TryValue(func() (Out, error) { return fn(wctx, v) })
					if err != nil {
						fail(err)
						return
					}
					select {
					case out <- r:
					case <-wctx.Done():
						cut.Store(true)
						return
					}
				}
			}
		})
	}
	go func() {
		wg.Wait()
		cancel()
		close(out)
		switch {
		case firstErr != nil:
			result = firstErr
		case cut.Load() && ctx.Err() != nil:
			result = ctx.Err()
		}
		close(done)
	}()
	return out, func() error {
		<-done
		return result
	}
}
