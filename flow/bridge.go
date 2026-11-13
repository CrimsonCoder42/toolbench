package flow

import "context"

// Bridge flattens a channel of channels: it forwards every value of the first inner channel until it is closed,
// then the next, and closes the returned channel when chans is closed or ctx is done. Values keep their order
// within an inner channel and inner channels are read in the order they arrive. Nil inner channels are
// skipped; a nil chans is an input with no values.
//
// ID:          TB-FLOW-008
// Pattern:     bridge channel
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4, "The bridge-channel"
// Goroutines:  1; it exits when chans is closed or ctx is done, then the channel is closed
// Cancel:      ctx done: the goroutine stops at its next receive or send (a value in hand is dropped); the channel is closed
// Blocking:    sends wait for the consumer; an open inner channel with no values holds back the ones after it
// Tags:        concurrency, channel, pipeline
func Bridge[T any](ctx context.Context, chans <-chan (<-chan T)) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		if chans == nil {
			return
		}
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case in, ok := <-chans:
				if !ok {
					return
				}
				if in != nil && forward(ctx, in, out) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
