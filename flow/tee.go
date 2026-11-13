package flow

import (
	"context"
	"fmt"
)

// Tee copies every value from in to n returned channels: each value is sent exactly once on each output, in
// input order. The next value is read from in only after every output has taken the current one; WithBuffer
// gives each output that much slack. One goroutine may take each value from the outputs in turn, in any order;
// a consumer that drains one output before reading the others needs WithBuffer or a goroutine per output. All
// outputs are closed when in is closed or ctx is done. A nil in is an input with no values.
//
// ID:          TB-FLOW-007
// Pattern:     tee channel
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4, "The tee-channel"
// Goroutines:  n forwarders + 1 distributor; all have exited when every output is closed
// Cancel:      ctx done: the current value is not sent to the outputs that have not taken it; every output is closed
// Errors:      n < 1 returns nil and ErrInvalid; nothing is started
// Blocking:    the slowest consumer sets the pace for all (lock-step); Tee itself returns at once
// Tags:        concurrency, channel, tee, broadcast
func Tee[T any](ctx context.Context, in <-chan T, n int, opts ...Option) ([]<-chan T, error) {
	if n < 1 {
		return nil, fmt.Errorf("%w: Tee n = %d, want n >= 1", ErrInvalid, n)
	}
	c := newConfig(opts)
	outs := make([]<-chan T, n)
	handoffs := make([]chan T, n)
	acks := make(chan struct{}, n)
	for i := range n {
		out := make(chan T, c.buffer)
		outs[i] = out
		// One slot: the distributor hands a value over only after the previous one was acknowledged, so the
		// slot is always empty and the handoff never blocks.
		handoff := make(chan T, 1)
		handoffs[i] = handoff
		go func() {
			defer close(out)
			for v := range handoff {
				// A cancelled ctx wins over a ready consumer, so nothing is sent after cancellation is seen.
				if ctx.Err() == nil {
					select {
					case out <- v:
					case <-ctx.Done():
					}
				}
				acks <- struct{}{} // capacity n, at most n outstanding: never blocks
			}
		}()
	}
	go func() {
		defer func() {
			for _, h := range handoffs {
				close(h)
			}
		}()
		if in == nil {
			return
		}
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case v, ok := <-in:
				if !ok {
					return
				}
				for _, h := range handoffs {
					h <- v
				}
				for range n {
					<-acks
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return outs, nil
}
