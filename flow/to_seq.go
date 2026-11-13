package flow

import (
	"context"
	"iter"
)

// ToSeq returns an iter.Seq that yields the values received from in until in is closed, ctx is done or the
// loop body stops (yield returns false). A nil in yields nothing. A channel is read once: ranging over the
// Seq a second time continues with the values not yet received.
//
// ID:          TB-FLOW-002
// Pattern:     channel-to-iterator adapter
// Authority:   pkg.go.dev iter, "Seq"
// Goroutines:  none; the loop body runs on the caller's goroutine
// Cancel:      ctx done: the range ends at the next receive; values still in the channel are left there
// Blocking:    each step waits for a value from in or for ctx
// Tags:        concurrency, iterator, channel
func ToSeq[T any](ctx context.Context, in <-chan T) iter.Seq[T] {
	return func(yield func(T) bool) {
		if in == nil {
			return
		}
		for {
			// A cancelled ctx wins over a ready value, so no value is yielded after cancellation is seen.
			if ctx.Err() != nil {
				return
			}
			select {
			case v, ok := <-in:
				if !ok || !yield(v) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
}
