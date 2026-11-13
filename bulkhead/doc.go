// Package bulkhead isolates a dependency behind a fixed number of concurrent calls, so that when it fails or
// slows down it can tie up only its own share of the caller's resources (Nygard, Release It!, "Bulkheads";
// Azure, "Bulkhead pattern"). Do runs fn when one of maxConcurrent slots is free; when all are busy the call
// fails fast with ErrFull, or, with WithQueue(n), waits first in first out among at most n others, each for at
// most WithQueueTimeout (ErrQueueTimeout, which also matches ErrFull). Admission is built on semaphore.Weighted
// (TB-SEMA-001); time comes from WithClock; a panic in fn is returned as panics.ErrPanic and its slot is given
// back. No goroutine is started. WithOnEvent observes every admission, rejection, wait, timeout, cancellation
// and release, on the caller's goroutine, outside any lock.
//
// ID short form: BULK (IDs are TB-BULK-NNN).
//
// Exports:
//
//	TB-BULK-001  Bulkhead          bounds concurrent calls: Do runs fn in a free slot, queues or fails fast with ErrFull
//	TB-BULK-002  New               a Bulkhead with maxConcurrent slots; maxConcurrent <= 0 is ErrMaxConcurrent
//	TB-BULK-003  DoValue           Bulkhead.Do for a function that returns a value
//	TB-BULK-004  ErrFull           returned without calling fn when every slot and queue place is taken
//	TB-BULK-005  ErrQueueTimeout   returned when a queued call waited WithQueueTimeout without a slot; matches ErrFull
//	TB-BULK-006  ErrMaxConcurrent  returned by New when maxConcurrent <= 0
//	TB-BULK-007  Option            configures New
//	TB-BULK-008  WithQueue         how many calls may wait, FIFO, for a slot (default 0: fail fast)
//	TB-BULK-009  WithQueueTimeout  how long a queued call waits on the clock (default: until ctx ends)
//	TB-BULK-010  WithClock         the clock the queue timeout and waits are measured on (default clock.Real)
//	TB-BULK-011  WithOnEvent       a metrics hook called with every Event, on the caller's goroutine, outside any lock
//	TB-BULK-012  Event             what happened to one call (Kind) and how long it queued (Waited)
//	TB-BULK-013  EventKind         admitted, rejected, queued, timed-out, cancelled, released
package bulkhead
