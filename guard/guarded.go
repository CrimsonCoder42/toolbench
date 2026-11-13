package guard

import "sync"

// Guarded is a value of type T behind its own mutex: the only ways to reach the value are Do, which runs a
// function on it under the lock, and Load, which returns a copy taken under the lock. Every write made in one
// Do is visible to every later Do and Load (the Go memory model's rule for Unlock before Lock).
//
// The zero value is ready to use and holds T's zero value. A Guarded holds a sync.Mutex: it must not be copied
// after first use (go vet copylocks reports a copy); pass *Guarded[T] or keep it in a struct used by pointer.
//
// ID:          TB-GUARD-001
// Pattern:     mutual exclusion (a mutex beside the value it guards)
// Authority:   The Go Memory Model, go.dev, "Synchronization", "Locks"
// Authority:   Effective Go, go.dev, "Concurrency", "Share by communicating"
// Goroutines:  none; Do's fn runs on the caller's goroutine
// Blocking:    Do and Load wait for the mutex (not cancellable, like sync.Mutex.Lock); keep fn short
// Tags:        concurrency, mutex, lock, shared-state, generic
type Guarded[T any] struct {
	mu sync.Mutex // guards v
	v  T
}

// Do locks g, calls fn with a pointer to the guarded value and unlocks g when fn returns. A nil fn is a no-op.
//
// The pointer is valid only until fn returns: keeping it, sending it elsewhere or handing it to a goroutine
// and using it after fn returns is a data race this type cannot prevent; the same holds for maps, slices and
// pointers inside T. fn must not call Do or Load on the same g: sync.Mutex is not re-entrant and that call
// deadlocks. A panic in fn is not recovered: g is unlocked first (deferred), then the panic continues up the
// caller's stack, so the next Do does not deadlock; the value keeps whatever fn wrote before panicking.
// Wrap the call in panics.Try to turn that panic into an error.
func (g *Guarded[T]) Do(fn func(*T)) {
	if fn == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	fn(&g.v)
}

// Load returns a copy of the guarded value, taken under the lock. The copy is shallow: a map, slice, pointer
// or channel inside T still refers to the guarded data, and reading or writing through it outside Do is a data
// race. For such a T, copy deeply inside Do instead.
func (g *Guarded[T]) Load() T {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.v
}
