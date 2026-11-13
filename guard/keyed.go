package guard

import (
	"context"
	"fmt"
	"sync"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// Keyed is a set of locks, one per key (lock striping): Do runs fn while no other Do or TryDo with the same
// key runs, and callers on distinct keys never wait on each other. Entries are reference-counted (holders plus
// waiters) and removed when the last one leaves, so the table never grows past the keys in use and nothing
// sweeps it; WithMaxKeys bounds the distinct keys held or waited on at once (ErrKeys when full). Every write
// made in one fn is visible to the next fn on the same key (the Go memory model's rule for a buffered channel:
// the holder's receive is synchronized before the next holder's send completes).
//
// The zero value is ready to use (the bound is DefaultMaxKeys); NewKeyed only applies options. A Keyed holds a
// sync.Mutex: it must not be copied after first use (go vet copylocks reports a copy); pass *Keyed[K].
//
// Unlike Guarded.Do, a panic in fn is recovered here (Do has an error result) and returned as a
// *panics.PanicError with the key released. The locks are not re-entrant: fn calling Do on the same key
// deadlocks (as sync.Mutex); on different keys the caller orders the keys to avoid an A/B-B/A deadlock.
//
// ID:          TB-GUARD-003
// Pattern:     lock striping (one lock per key; mutual exclusion per key)
// Authority:   The Go Memory Model, go.dev, "Synchronization", "Locks" and "Channel communication"
// Authority:   pkg.go.dev sync, "type Mutex", "func (*Mutex) TryLock"
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.3, "The sync Package", "Mutex and RWMutex"
// Goroutines:  none; fn runs on the caller's goroutine; no sweeper
// Cancel:      Do: ctx done while waiting for the key returns context.Cause(ctx) and leaves no entry; fn, once started, is not interrupted
// Errors:      Do and TryDo: fn's error unchanged; ErrKeys (wrapped) when the bound is full, the key is not equal to itself or is not comparable; a panic in fn as *panics.PanicError
// Blocking:    Do waits for the key or ctx; TryDo never blocks; the table mutex is held for O(1) per call
// Tags:        concurrency, mutex, lock, striping, per-key, shared-state, bounded, generic
type Keyed[K comparable] struct {
	mu      sync.Mutex        // guards table, maxKeys and every entry's refs
	table   map[K]*keyedEntry // key → its lock; an entry exists only while refs > 0
	maxKeys int               // bound on len(table); 0 means DefaultMaxKeys
}

// keyedEntry is one key's lock. lock has capacity 1: a send takes the key, a receive gives it back, so a waiter
// can select between the send and ctx.Done(). refs counts holders and waiters, under Keyed.mu.
type keyedEntry struct {
	lock chan struct{}
	refs int
}

// NewKeyed returns an empty Keyed with opts applied (nil options are ignored). It exists only to apply options:
// the zero value of Keyed is ready to use with the defaults.
//
// ID:          TB-GUARD-004
// Pattern:     lock striping (constructor)
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        concurrency, lock, striping, per-key, constructor, generic
func NewKeyed[K comparable](opts ...KeyedOption) *Keyed[K] {
	c := newKeyedConfig(opts)
	return &Keyed[K]{maxKeys: c.maxKeys}
}

// Do waits until no other Do or TryDo holds key (or ctx is done), runs fn, releases key and returns fn's
// error. A ctx already done returns context.Cause(ctx) before touching the table; ctx done while waiting
// returns context.Cause(ctx) and the waiter's entry is dropped (the holder is unaffected). A new key when the
// table holds WithMaxKeys keys, a key not equal to itself (a NaN) or an interface key holding an uncomparable
// value (a slice) is refused with ErrKeys, never a panic. A panic in fn
// is returned as a *panics.PanicError (errors.Is(err, panics.ErrPanic)) with the key released; this differs
// from Guarded.Do, which has no error result and lets the panic propagate. A nil fn is a no-op that returns
// nil without taking the key; a nil ctx is context.Background().
func (g *Keyed[K]) Do(ctx context.Context, key K, fn func() error) error {
	if fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	e, err := g.acquire(key)
	if err != nil {
		return err
	}
	select {
	case e.lock <- struct{}{}:
	case <-ctx.Done():
		g.release(key, e)
		return context.Cause(ctx)
	}
	defer g.unlock(key, e) // also on runtime.Goexit in fn, so the key is never held by a dead goroutine
	return panics.Try(fn)
}

// TryDo runs fn under key if the key is free right now and reports whether it ran; it never waits. ran is
// false, with a nil error, when another Do or TryDo holds key; this is sync.Mutex.TryLock's shape, and as its
// doc says, needing it is often a sign of a deeper problem: prefer Do with a ctx. ErrKeys, panics and a nil fn
// are as for Do (a nil fn reports (true, nil) without taking the key).
func (g *Keyed[K]) TryDo(key K, fn func() error) (ran bool, err error) {
	if fn == nil {
		return true, nil
	}
	e, err := g.tryAcquire(key)
	if err != nil {
		return false, err
	}
	if e == nil {
		return false, nil
	}
	defer g.unlock(key, e)
	return true, panics.Try(fn)
}

// Len reports how many distinct keys are held or waited on right now; 0 when g is idle. It is the size of the
// table, which WithMaxKeys bounds.
func (g *Keyed[K]) Len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.table)
}

// acquire finds or makes key's entry and counts the caller in, under the table mutex.
func (g *Keyed[K]) acquire(key K) (*keyedEntry, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.acquireLocked(key)
}

// tryAcquire is acquire plus a non-blocking take of the key, under the table mutex (released by defer on every
// path). A nil entry with a nil error means the key is held by someone else.
func (g *Keyed[K]) tryAcquire(key K) (*keyedEntry, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, err := g.acquireLocked(key)
	if err != nil {
		return nil, err
	}
	select {
	case e.lock <- struct{}{}:
		return e, nil
	default:
		g.releaseLocked(key, e)
		return nil, nil
	}
}

// keyError reports why key cannot be a table key, or nil. An interface K holding an uncomparable value (a
// slice, a map, a func) panics on comparison and on map lookup, so it is checked here first, with the panic
// recovered; a key not equal to itself (a NaN, or a struct holding one) could never be deleted from the map.
// Both are ErrKeys, so the table mutex is never left locked and the table never gains an entry it cannot drop.
func keyError[K comparable](key K) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("%w: key is not comparable", ErrKeys)
		}
	}()
	if key != key {
		return fmt.Errorf("%w: key is not equal to itself", ErrKeys)
	}
	return nil
}

// acquireLocked is acquire with g.mu held. The bound applies to new keys only: a key already in the table is
// joined, never refused.
func (g *Keyed[K]) acquireLocked(key K) (*keyedEntry, error) {
	if err := keyError(key); err != nil {
		return nil, err
	}
	if g.table == nil {
		g.table = make(map[K]*keyedEntry)
	}
	e, ok := g.table[key]
	if !ok {
		limit := g.maxKeys
		if limit <= 0 {
			limit = DefaultMaxKeys
		}
		if n := len(g.table); n >= limit {
			return nil, fmt.Errorf("%w: %d keys held or waited on", ErrKeys, n)
		}
		e = &keyedEntry{lock: make(chan struct{}, 1)}
		g.table[key] = e
	}
	e.refs++
	return e, nil
}

// unlock gives key back (the receive that the next waiter's send is waiting on) and then counts the caller out.
// The order matters: the next holder may already be counted, so the entry survives until the last one leaves.
func (g *Keyed[K]) unlock(key K, e *keyedEntry) {
	<-e.lock
	g.release(key, e)
}

// release counts the caller out and deletes the entry when it was the last holder or waiter.
func (g *Keyed[K]) release(key K, e *keyedEntry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releaseLocked(key, e)
}

// releaseLocked is release with g.mu held.
func (g *Keyed[K]) releaseLocked(key K, e *keyedEntry) {
	e.refs--
	if e.refs == 0 {
		delete(g.table, key)
	}
}
