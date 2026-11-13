package flow

import (
	"context"
	"sync"
)

// Merge sends every value from every input on one channel (fan-in) and closes it when every input is closed or
// ctx is done. Values from one input keep their order; how inputs interleave is not specified. Nil inputs are
// skipped; with no non-nil input the channel is closed at once by the closer goroutine.
//
// ID:          TB-FLOW-005
// Pattern:     fan-in
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Fan-out, fan-in"
// Goroutines:  one per non-nil input + 1 closer; all have exited when the channel is closed
// Cancel:      ctx done: every forwarder stops at its next receive or send (a value in hand is dropped); the channel is closed
// Blocking:    sends wait for the consumer, so a slow consumer holds back every input; Merge itself returns at once
// Tags:        concurrency, pipeline, fan-in, merge
func Merge[T any](ctx context.Context, ins ...<-chan T) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup
	for _, in := range ins {
		if in == nil {
			continue
		}
		wg.Go(func() { forward(ctx, in, out) })
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// forward sends every value from in on out until in is closed or ctx is done. It reports whether it stopped
// because ctx was done.
func forward[T any](ctx context.Context, in <-chan T, out chan<- T) (cancelled bool) {
	for {
		if ctx.Err() != nil {
			return true
		}
		select {
		case v, ok := <-in:
			if !ok {
				return false
			}
			select {
			case out <- v:
			case <-ctx.Done():
				return true
			}
		case <-ctx.Done():
			return true
		}
	}
}
