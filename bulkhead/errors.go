package bulkhead

import (
	"errors"
	"fmt"
)

// ErrFull is returned by Do and DoValue, without calling fn, when every slot is busy and the wait queue is
// full (or there is none: the default fails fast). It is also matched by ErrQueueTimeout, so a caller that only
// asks "did the bulkhead reject this call?" checks errors.Is(err, ErrFull). The caller usually sheds the load
// (a fallback, HTTP 503, a retry later) instead of waiting.
//
// ID:          TB-BULK-004
// Pattern:     sentinel error (fail fast, load shedding)
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Bulkheads" (book, not opened)
// Authority:   Microsoft, "Bulkhead pattern", Azure Architecture Center, 2026, "Context and problem" (resource exhaustion)
// Tags:        bulkhead, error, full, load-shedding, resilience
var ErrFull = errors.New("bulkhead: full")

// ErrQueueTimeout is returned by Do and DoValue, without calling fn, when the call waited in the queue for the
// WithQueueTimeout duration without getting a slot. errors.Is(err, ErrQueueTimeout) and errors.Is(err, ErrFull)
// are both true: a timed-out wait is a rejection that took longer.
//
// ID:          TB-BULK-005
// Pattern:     sentinel error (bounded wait)
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Timeouts" and "Bulkheads" (book, not opened)
// Tags:        bulkhead, error, timeout, queue, resilience
var ErrQueueTimeout = fmt.Errorf("bulkhead: queue timeout: %w", ErrFull)

// ErrMaxConcurrent is returned, wrapped with the value given, by New when maxConcurrent <= 0: a long-lived
// bulkhead sized by accident is a bug worth surfacing (toolbench decision D-023: a long-lived constructor
// returns an error for a count <= 0). Match it with errors.Is.
//
// ID:          TB-BULK-006
// Pattern:     sentinel error (invalid argument)
// Authority:   pkg.go.dev errors, "func Is"
// Tags:        bulkhead, error, constructor, validation
var ErrMaxConcurrent = errors.New("bulkhead: maxConcurrent must be > 0")
