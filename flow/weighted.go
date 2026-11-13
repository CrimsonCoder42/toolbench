package flow

import (
	"context"
	"fmt"
	"slices"

	"github.com/CrimsonCoder42/calcbench/algo/sample"
)

// Weighted merges its inputs by weight: each turn it picks, among the inputs whose value flow has received, input i
// with probability weights[i] / (sum of the ready inputs' weights), and sends that value. A ready input is never
// skipped to wait for an empty one, and every input with a value gets turns in proportion to its weight, so none
// starves. Values from one input keep their order. Nil inputs are skipped; the channel is closed when every
// input is closed or ctx is done. The random source is WithRand (default: a new PCG source per call).
//
// ID:          TB-FLOW-010
// Pattern:     weighted priority queue (weighted fair dequeue)
// Authority:   Microsoft, Azure Architecture Center, "Priority Queue pattern", 2026, "Multiple consumer pools"
// Authority:   Microsoft, Azure Architecture Center, "Priority Queue pattern", 2026, "Consider low-priority processing"
// Goroutines:  1 merger + at most len(ins) one-shot receivers while it waits, all stopped and waited for before it picks; all have exited when the channel is closed
// Cancel:      ctx done: the values held (and a value a receiver had just taken) are lost, every goroutine exits, the channel is closed
// Errors:      ErrInvalid (nil channel, nothing started) when len(ins) != len(weights), there are no inputs, a weight is below 1, or the total overflows int
// Blocking:    sends wait for the consumer (WithBuffer adds slack); Weighted itself returns at once
// Math:        ALG-SAMPLE-003 NewWeightedPicker, ALG-SAMPLE-004 WeightedPicker.Pick
// Tags:        concurrency, priority, weighted, merge, fan-in, random
func Weighted[T any](ctx context.Context, ins []<-chan T, weights []int, opts ...Option) (<-chan T, error) {
	if len(ins) == 0 || len(ins) != len(weights) {
		return nil, fmt.Errorf("%w: Weighted got %d inputs and %d weights, want the same non-zero count", ErrInvalid, len(ins), len(weights))
	}
	for i, w := range weights {
		if w < 1 {
			return nil, fmt.Errorf("%w: Weighted weights[%d] = %d, want >= 1", ErrInvalid, i, w)
		}
	}
	// The full set is checked once, so every subset picked from below has a total that fits.
	if _, err := sample.NewWeightedPicker(weights); err != nil {
		return nil, fmt.Errorf("%w: Weighted weights: %w", ErrInvalid, err)
	}
	weights = slices.Clone(weights) // the caller may reuse its slice
	c := newConfig(opts)
	rng := c.random()
	out := make(chan T, c.buffer)
	go func() {
		defer close(out)
		ws := make([]int, 0, len(ins))
		chooseMerge(ctx, ins, out, func(ready []int) int {
			if len(ready) == 1 {
				return ready[0]
			}
			ws = ws[:0]
			for _, i := range ready {
				ws = append(ws, weights[i])
			}
			p, err := sample.NewWeightedPicker(ws)
			if err != nil {
				return ready[0] // unreachable: every weight is >= 1 and the full total fits
			}
			k, err := p.Pick(rng)
			if err != nil {
				return ready[0] // unreachable: p and rng are non-nil
			}
			return ready[k]
		})
	}()
	return out, nil
}
