package bulkhead

import (
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// config holds New's settings; zero means the documented default.
type config struct {
	queue   int
	timeout time.Duration
	clk     clock.Clock
	onEvent func(Event)
}

// Option configures New. A nil Option is ignored.
//
// ID:          TB-BULK-007
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration, bulkhead
type Option func(*config)

// WithQueue lets up to n calls wait, first in first out, for a slot when all are busy; a call beyond them gets
// ErrFull at once. The default, and any n <= 0, is 0: no queue, a call that finds every slot busy fails fast.
//
// ID:          TB-BULK-008
// Pattern:     bounded queue (queue-based load leveling)
// Authority:   Microsoft, "Queue-Based Load Leveling pattern", Azure Architecture Center, 2026, "Problems and considerations" ("Monitor queue depth ... or shed work at the producer")
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Bulkheads" and "Shed Load" (book, not opened)
// Tags:        options, bulkhead, queue, bounded, fifo
func WithQueue(n int) Option {
	return func(c *config) { c.queue = max(n, 0) }
}

// WithQueueTimeout bounds how long a queued call waits for a slot, measured on the WithClock clock; when d
// passes the call leaves the queue and Do returns ErrQueueTimeout (which also matches ErrFull). The default, and
// any d <= 0, is no timeout: a queued call waits until a slot is free or its ctx ends. It has no effect without
// WithQueue, and it never limits fn itself (fn runs with the caller's ctx).
//
// ID:          TB-BULK-009
// Pattern:     timeout (bounded wait)
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Timeouts" (book, not opened)
// Tags:        options, bulkhead, timeout, queue, time
func WithQueueTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = max(d, 0) }
}

// WithClock sets the clock the queue timeout and Event.Waited are measured on. The default, and a nil c, is
// clock.Real(). Tests pass a *clock.Fake: the timeout is one AfterFunc timer per queued call, armed before the
// call joins the queue, so BlockUntil sees it.
//
// ID:          TB-BULK-010
// Pattern:     injected clock
// Authority:   pkg.go.dev time, "func AfterFunc"; toolbench doc/specs/clock.md
// Tags:        options, bulkhead, clock, time, testing
func WithClock(c clock.Clock) Option {
	return func(cfg *config) { cfg.clk = c }
}

// WithOnEvent sets a hook called with every Event (see Event for the sequences a call produces). It runs on
// the goroutine of the Do call the event belongs to, holding no lock, possibly from many goroutines at once, so
// it must be safe for concurrent use and should not block: the call waits for it. A panic in the hook is
// recovered and joined (errors.Join) to that Do's result; the call itself goes on as if the hook had
// returned. The default, and a nil f, is no hook.
//
// ID:          TB-BULK-011
// Pattern:     observer (metrics hook)
// Authority:   Microsoft, "Bulkhead pattern", Azure Architecture Center, 2026, "Problems and considerations" ("Monitor each partition's performance")
// Tags:        options, bulkhead, metrics, observability, hook
func WithOnEvent(f func(Event)) Option {
	return func(c *config) { c.onEvent = f }
}
