package flow

import "context"

// OrDone forwards every value from in to the returned channel until in is closed or ctx is done, so a consumer
// can write `for v := range flow.OrDone(ctx, in)` and still stop on cancellation. A nil in is an input with no
// values: the channel is closed at once.
//
// ID:          TB-FLOW-006
// Pattern:     or-done channel
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4, "The or-done-channel"
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Explicit cancellation"
// Goroutines:  1; it exits when in is closed or ctx is done, then the channel is closed
// Cancel:      ctx done: the goroutine stops at its next receive or send (a value in hand is dropped); the channel is closed
// Blocking:    sends wait for the consumer; OrDone itself returns at once
// Tags:        concurrency, channel, cancellation
func OrDone[T any](ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		if in != nil {
			forward(ctx, in, out)
		}
	}()
	return out
}
