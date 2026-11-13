package batch

import (
	"context"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// Channel receives values from in and sends them in batches, in input order, on the returned channel. A batch
// is sent when it holds size values, when maxWait has passed since its first value was received, or when in is
// closed (the partial final batch); then the returned channel is closed. A size below 1 is 1; a maxWait <= 0
// closes batches by size and the close of in only. Every batch is a fresh slice the receiver may keep. A nil in
// is a channel with no values: the returned channel is closed at once. A nil ctx is context.Background().
// Channel runs no caller callback, so it has no error path: no panic can come from caller code (a custom
// clock.Clock passed with WithClock is trusted like package time).
//
// ID:          TB-BATCH-002
// Pattern:     aggregator (completeness by count or timeout)
// Authority:   Hohpe and Woolf, Enterprise Integration Patterns, 2003, "Aggregator" (Wait for All, Time Out)
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Explicit cancellation"
// Goroutines:  1 (none for a nil in); it exits when in is closed and the last batch is received, or when ctx is done; the channel is closed as it exits
// Cancel:      ctx done: the batch being collected or sent is dropped (at-most-once), the timer is stopped and the channel is closed
// Blocking:    each send waits for the receiver (backpressure): while a batch waits, in is not read; maxWait is measured with WithClock's clock
// Tags:        concurrency, batch, aggregator, channel, timeout, size-limit
func Channel[T any](ctx context.Context, in <-chan T, size int, maxWait time.Duration, opts ...Option) <-chan []T {
	if ctx == nil {
		ctx = context.Background()
	}
	c := newConfig(opts)
	size = max(size, 1)
	out := make(chan []T)
	if in == nil {
		close(out)
		return out
	}
	go func() {
		defer close(out)
		var (
			buf    []T              // the batch being collected; a new slice after every send
			timer  clock.Timer      // made on the first batch that needs it, then Reset
			timerC <-chan time.Time // nil while no timer is armed
		)
		stop := func() {
			if timerC != nil {
				timer.Stop() // go1.23 rules: no stale value is received after Stop returns
				timerC = nil
			}
		}
		defer stop()
		send := func() bool {
			stop()
			b := buf
			buf = nil
			select {
			case out <- b:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for {
			// A cancelled ctx wins over a ready input or timer, so nothing is sent after cancellation is seen.
			if ctx.Err() != nil {
				return
			}
			select {
			case v, ok := <-in:
				if !ok {
					if len(buf) > 0 {
						send()
					}
					return
				}
				buf = append(buf, v)
				if len(buf) >= size {
					if !send() {
						return
					}
					continue
				}
				if len(buf) == 1 && maxWait > 0 {
					if timer == nil {
						timer = c.clock.NewTimer(maxWait)
					} else {
						timer.Reset(maxWait)
					}
					timerC = timer.C()
				}
			case <-timerC:
				timerC = nil // fired: nothing to stop
				if !send() {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
