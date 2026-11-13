package pool

import "errors"

// ErrClosed is returned by Submit when Close has been called: the value was not accepted and fn will not see
// it. Match it with errors.Is.
//
// ID:          TB-POOL-006
// Pattern:     sentinel error
// Authority:   pkg.go.dev net/http, "ErrServerClosed" (the standard library's after-close sentinel)
// Tags:        pool, error, closed
var ErrClosed = errors.New("pool: closed")

// ErrWorkers is returned, wrapped with the count, by New when the worker count is zero or negative: a pool
// with no workers could never run a value. Match it with errors.Is.
//
// ID:          TB-POOL-007
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "Is"
// Tags:        pool, error, workers
var ErrWorkers = errors.New("pool: worker count must be positive")

// ErrQueueFull is returned, wrapped with the queue size, by Submit when it is called from inside a worker of
// the same pool (with the ctx fn received) and by TrySubmit, when neither the queue nor a waiting worker can
// take the value now: the value was not accepted and fn will not see it, so the caller handles the overflow
// itself; nothing is dropped silently. Match it with errors.Is.
//
// ID:          TB-POOL-008
// Pattern:     sentinel error
// Authority:   The Go Programming Language Specification, "Select statements" (a select with a default case never blocks), go1.27
// Tags:        pool, error, queue, full, non-blocking
var ErrQueueFull = errors.New("pool: queue full")
