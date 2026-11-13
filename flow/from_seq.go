package flow

import (
	"context"
	"iter"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// FromSeq sends every value of seq, in order, on the returned channel from one goroutine, and closes the
// channel when seq ends, ctx is done or seq panics. When ctx is done, seq's yield returns false, so seq stops
// and its own deferred cleanup runs. A nil seq yields nothing. The returned wait func reports how it ended, in
// the same shape as Stage and FanOut.
//
// ID:          TB-FLOW-001
// Pattern:     generator
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "What is a pipeline?"
// Authority:   pkg.go.dev iter, "Seq"
// Goroutines:  1; it exits when seq ends, ctx is done or seq panics; the channel is closed and wait returns after it leaves seq
// Cancel:      ctx done: the pending send is abandoned, yield returns false, the channel is closed
// Errors:      wait returns a *panics.PanicError (errors.Is ErrPanic) if seq panicked, else ctx's error if ctx stopped seq early, else nil
// Blocking:    each send waits for the consumer (WithBuffer adds slack); wait blocks until the channel is closed, so drain it or cancel ctx first
// Tags:        concurrency, pipeline, generator, iterator
func FromSeq[T any](ctx context.Context, seq iter.Seq[T], opts ...Option) (<-chan T, func() error) {
	c := newConfig(opts)
	out := make(chan T, c.buffer)
	done := make(chan struct{})
	var result error
	go func() {
		defer close(done)
		defer close(out)
		if seq == nil {
			return
		}
		result = panics.Try(func() error {
			for v := range seq {
				// A cancelled ctx wins over a ready consumer, so nothing is sent after cancellation is seen.
				if ctx.Err() != nil {
					return ctx.Err()
				}
				select {
				case out <- v:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}()
	return out, func() error {
		<-done
		return result
	}
}
