package bulkhead

import (
	"strconv"
	"time"
)

// Event is what the WithOnEvent hook receives: what happened to one call and, for a call that waited in the
// queue, how long it waited on the bulkhead's clock. Every Do call produces exactly one of these sequences:
// Rejected · Cancelled · Admitted, Released · Queued, Admitted, Released · Queued, TimedOut · Queued, Cancelled
// (and TimedOut or Cancelled without Queued when the wait ended before the call reached the queue). The zero
// value is an Event of kind 0, which no call produces.
//
// ID:          TB-BULK-012
// Pattern:     observer (metrics hook)
// Authority:   Microsoft, "Bulkhead pattern", Azure Architecture Center, 2026, "Problems and considerations" ("Monitor each partition's performance")
// Tags:        bulkhead, event, metrics, observability, hook
type Event struct {
	Kind EventKind
	// Waited is the time the call spent in the queue, measured on the bulkhead's clock: set for Admitted,
	// TimedOut and Cancelled after a wait; zero for a call that never waited.
	Waited time.Duration
}

// EventKind says what happened to a call. String gives "admitted", "rejected", "queued", "timed-out",
// "cancelled", "released", and "EventKind(n)" for any other value. The zero value is not a kind any call
// produces.
//
// ID:          TB-BULK-013
// Pattern:     enumeration (event kind)
// Authority:   Microsoft, "Bulkhead pattern", Azure Architecture Center, 2026, "Problems and considerations" ("Monitor each partition's performance")
// Tags:        bulkhead, event, metrics, enum
type EventKind int

// The event kinds, in the order a call can meet them.
const (
	EventAdmitted EventKind = iota + 1 // the call got a slot; fn runs next
	EventRejected                      // every slot and queue place was taken: Do returns ErrFull
	// EventQueued: the call is waiting in the FIFO queue for a slot. It is emitted when the slot semaphore
	// first calls Done on the waiter's context, which semaphore.Weighted.Acquire does only after linking the
	// waiter into its queue (D-036). TestDo_queuedEventMeansEnqueued pins this in one direction only: it
	// reliably catches Done never being called, but catches Done being called before the push only by chance.
	EventQueued
	EventTimedOut  // the queue wait reached WithQueueTimeout: Do returns ErrQueueTimeout
	EventCancelled // ctx ended before a slot was granted: Do returns context.Cause(ctx)
	EventReleased  // fn returned (or panicked) and its slot was given back
)

// String returns the kind's name.
func (k EventKind) String() string {
	switch k {
	case EventAdmitted:
		return "admitted"
	case EventRejected:
		return "rejected"
	case EventQueued:
		return "queued"
	case EventTimedOut:
		return "timed-out"
	case EventCancelled:
		return "cancelled"
	case EventReleased:
		return "released"
	}
	return "EventKind(" + strconv.Itoa(int(k)) + ")"
}
