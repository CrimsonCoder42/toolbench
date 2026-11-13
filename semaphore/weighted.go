package semaphore

import (
	"context"
	"fmt"
	"sync"
)

// Weighted bounds concurrent access to a resource: callers take n permits with Acquire or TryAcquire and give
// them back with Release, and at most size permits are held at once. Waiters are served first in, first out:
// a large waiter at the front blocks later, smaller ones even when they would fit, so large requests are
// never starved. No input panics: a negative weight or one larger than the size is an error from Acquire and
// false from TryAcquire; Release of a negative weight is a no-op and Release of more than is held releases
// what is held. Safe for concurrent use. The zero value is a semaphore of size 0 (only weight 0 is granted);
// create one with New. A Weighted must not be copied after first use.
//
// ID:          TB-SEMA-001
// Pattern:     counting semaphore (weighted P and V, FIFO waiters)
// Authority:   Dijkstra, "Cooperating sequential processes", EWD123, 1965, 3.2 "The Synchronizing Primitives"
// Authority:   pkg.go.dev golang.org/x/sync/semaphore v0.25.0, "type Weighted", "func (*Weighted) Acquire"
// Goroutines:  none; Acquire blocks the calling goroutine only
// Cancel:      Acquire returns ctx.Err() when ctx is done before or while it waits; permits granted to a cancelled waiter are given back
// Errors:      Acquire: ErrNegativeWeight, ErrTooLarge (both wrapped, returned at once), ctx.Err()
// Blocking:    Acquire waits while the permits are not free or anyone is queued ahead; TryAcquire and Release never block
// Tags:        concurrency, semaphore, limit, fifo, context
type Weighted struct {
	mu   sync.Mutex // guards every field below and the links of every queued waiter
	size int64      // the permits there are; fixed by New
	cur  int64      // the permits held now; 0 <= cur <= size
	head *waiter    // the FIFO queue of blocked Acquire calls, oldest first
	tail *waiter

	// queued, when non-nil, is called under mu each time a waiter joins the queue. It is a seam for this
	// package's tests (to know a waiter is queued without sleeping) and is never set outside them.
	queued func()
}

// waiter is one Acquire call blocked in the queue. Its links are guarded by Weighted.mu.
type waiter struct {
	n          int64
	ready      chan struct{} // closed by grant once n permits are counted for this waiter
	prev, next *waiter
}

// New returns a semaphore with n permits. A negative n gives a semaphore of size 0, on which only weight 0
// is granted (x/sync's NewWeighted panics instead).
//
// ID:          TB-SEMA-004
// Pattern:     counting semaphore (constructor)
// Authority:   pkg.go.dev golang.org/x/sync/semaphore v0.25.0, "func NewWeighted"
// Tags:        concurrency, semaphore, constructor
func New(n int64) *Weighted { return &Weighted{size: max(n, 0)} }

// Acquire takes n permits, blocking until they are free and every earlier waiter has been served, or until
// ctx is done. On success it returns nil. On failure the semaphore is left unchanged and the error is one of:
// an error wrapping ErrNegativeWeight (n < 0) or ErrTooLarge (n > size), returned at once whatever the state
// of ctx; or ctx.Err() when ctx is done before the call (even if the permits are free) or while it waits.
// When ctx is done at the moment the permits are granted, they are given back and ctx.Err() is returned, so
// a cancelled call never holds permits. A weight of 0 is granted when no one is waiting, else in its turn.
// A nil ctx is context.Background().
func (s *Weighted) Acquire(ctx context.Context, n int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if n < 0 {
		return fmt.Errorf("%w: n=%d", ErrNegativeWeight, n)
	}
	s.mu.Lock()
	if n > s.size {
		size := s.size
		s.mu.Unlock()
		return fmt.Errorf("%w: n=%d size=%d", ErrTooLarge, n, size)
	}
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.head == nil && s.size-s.cur >= n {
		s.cur += n
		s.mu.Unlock()
		return nil
	}
	w := &waiter{n: n, ready: make(chan struct{})}
	s.push(w)
	if s.queued != nil {
		s.queued()
	}
	s.mu.Unlock()

	select {
	case <-w.ready:
		if err := ctx.Err(); err != nil {
			// Granted, but ctx is done too: the caller sees a failure, so it must not hold the permits.
			s.Release(n)
			return err
		}
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		select {
		case <-w.ready:
			// Granted while ctx was being cancelled: give the permits back to the queue.
			s.cur -= n
			s.grant()
		default:
			front := s.head == w
			s.remove(w)
			if front {
				// A large waiter at the front may have been holding back smaller ones that now fit.
				s.grant()
			}
		}
		s.mu.Unlock()
		return ctx.Err()
	}
}

// TryAcquire takes n permits only if they are free and no one is waiting, without blocking. It returns true
// on success; otherwise false, with the semaphore unchanged. A negative n, or n larger than the size, is false.
func (s *Weighted) TryAcquire(n int64) bool {
	if n < 0 {
		return false
	}
	s.mu.Lock()
	ok := s.head == nil && s.size-s.cur >= n
	if ok {
		s.cur += n
	}
	s.mu.Unlock()
	return ok
}

// Release gives back n permits and grants the waiters at the front of the queue that now fit, in order.
// A negative n is a no-op. Releasing more than is held releases what is held, so the count never goes below
// zero (x/sync panics with "released more than held" instead); the caller's double release is not reported.
func (s *Weighted) Release(n int64) {
	if n < 0 {
		return
	}
	s.mu.Lock()
	s.cur -= min(n, s.cur)
	s.grant()
	s.mu.Unlock()
}

// grant hands permits to waiters from the front of the queue while the front one fits, and stops at the
// first that does not (FIFO: a smaller waiter behind it never jumps ahead). Called with s.mu held.
func (s *Weighted) grant() {
	for w := s.head; w != nil && s.size-s.cur >= w.n; w = s.head {
		s.cur += w.n
		s.remove(w)
		close(w.ready)
	}
}

// push appends w to the queue. Called with s.mu held.
func (s *Weighted) push(w *waiter) {
	w.prev = s.tail
	if s.tail != nil {
		s.tail.next = w
	} else {
		s.head = w
	}
	s.tail = w
}

// remove unlinks w from the queue in O(1). Called with s.mu held, only for a w that is queued.
func (s *Weighted) remove(w *waiter) {
	if w.prev != nil {
		w.prev.next = w.next
	} else {
		s.head = w.next
	}
	if w.next != nil {
		w.next.prev = w.prev
	} else {
		s.tail = w.prev
	}
	w.prev, w.next = nil, nil
}
