package flow

import "context"

// Priority merges its inputs in strict priority order: ins[0] is the highest. Each turn it looks at every input
// without blocking (holding at most one value per input) and sends the value of the highest-priority input among
// the values flow has received when it picks; when it holds none it waits for the first value on any input, and
// keeps every value received during that wait. A lower input is served only when no higher input has a value
// ready, so it can starve. Values from one input keep their order. Nil inputs are skipped; the channel is closed
// when every input is closed or ctx is done.
//
// ID:          TB-FLOW-009
// Pattern:     priority queue (multiple queues, single consumer, strict priority)
// Authority:   Microsoft, Azure Architecture Center, "Priority Queue pattern", 2026, "Single consumer pool"
// Goroutines:  1 merger + at most len(ins) one-shot receivers while it waits, all stopped and waited for before it picks; all have exited when the channel is closed
// Cancel:      ctx done: the values held (and a value a receiver had just taken) are lost, every goroutine exits, the channel is closed
// Blocking:    sends wait for the consumer; Priority itself returns at once
// Tags:        concurrency, priority, merge, fan-in
func Priority[T any](ctx context.Context, ins ...<-chan T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		chooseMerge(ctx, ins, out, func(ready []int) int { return ready[0] })
	}()
	return out
}
