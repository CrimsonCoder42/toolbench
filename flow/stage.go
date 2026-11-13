package flow

import "context"

// Stage is one pipeline stage: it runs fn on every value from in, one at a time, and sends the results on the
// returned channel in input order. The first error (or panic) from fn stops the stage; the returned wait func
// reports it. It is FanOut with one worker. A nil in is an input with no values.
//
// ID:          TB-FLOW-003
// Pattern:     pipeline stage
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Squaring numbers"
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Explicit cancellation"
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4, "Pipelines"
// Goroutines:  2 (worker, closer); both have exited when the channel is closed and when wait returns
// Cancel:      ctx done: the stage stops at its next receive or send (fn sees a cancelled ctx); the channel is closed
// Errors:      wait returns the first fn error, or a *panics.PanicError (errors.Is ErrPanic) for a panic, else ctx's error if work was cut short by ctx, else nil
// Blocking:    sends wait for the consumer (WithBuffer adds slack); wait blocks until the channel is closed, so drain it or cancel ctx first
// Tags:        concurrency, pipeline, stage
func Stage[In, Out any](ctx context.Context, in <-chan In, fn func(context.Context, In) (Out, error), opts ...Option) (<-chan Out, func() error) {
	return FanOut(ctx, in, 1, fn, opts...)
}
