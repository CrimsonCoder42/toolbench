// Package pool is a long-lived worker pool (competing consumers): New starts a fixed number of workers that
// take values from one queue and call fn on each; Submit hands a value to the pool and blocks while the queue
// is full (backpressure, never a drop); Close stops accepting, lets the workers finish every accepted value and
// waits for them. The workers run in a group.Group, so a panic in fn is recovered and returned as
// panics.ErrPanic, and with WithFailFast the first error cancels the pool. By default every error is kept and
// Close returns them joined (errors.Join). Nothing here panics on input: a nil ctx is context.Background(), a
// non-positive worker count is ErrWorkers, Submit after Close is ErrClosed. A worker may feed its own pool: Submit
// with the ctx fn received never blocks (ErrQueueFull when there is no room), and TrySubmit is that form from
// anywhere; WithOnDone reports progress; WithMaxErrors bounds the errors kept.
//
// For a fixed batch of tasks use group (fork-join) or parallel (Map, Filter, ForEach); pool is for values that
// arrive over time and are processed by the same bounded set of workers.
//
// ID short form: POOL (IDs are TB-POOL-NNN).
//
// Exports:
//
//	TB-POOL-001  Pool          a worker pool: workers started by New, Submit with backpressure, Close drains and waits
//	TB-POOL-002  New           starts a Pool of workers calling fn on each submitted value
//	TB-POOL-003  Option        a functional option for New
//	TB-POOL-004  WithQueue     how many submitted values may wait for a worker (default 0: direct hand-off)
//	TB-POOL-005  WithFailFast  the first error cancels the pool and is what Close returns (default: collect all)
//	TB-POOL-006  ErrClosed     returned by Submit after Close has been called
//	TB-POOL-007  ErrWorkers    returned by New when the worker count is not positive
//	TB-POOL-008  ErrQueueFull    returned by Submit from inside a worker and by TrySubmit when the queue has no room
//	TB-POOL-009  Pool.TrySubmit  non-blocking Submit from anywhere: false with ErrQueueFull, ErrClosed or the pool's cause
//	TB-POOL-010  WithOnDone      a progress hook called after each value with (done, total) from the pool's counters
//	TB-POOL-011  WithMaxErrors   keep the first n fn errors, count the rest (default: keep all)
package pool
