package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/bloom"
	"github.com/CrimsonCoder42/calcbench/algo/list"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/singleflight"
)

// Memory is an in-memory cache, safe for concurrent use: a fixed number of entries in least-recently-used
// order (calcbench ALG-LIST-015), each with an optional expiry read from a clock.Clock. It implements Cache.
// Expired entries are removed lazily, when a call meets them, plus a bounded sweep (at most 16 entries per
// write) from the least recently used end (ALG-LIST-018 Oldest): no goroutine per entry, no background
// sweeper. GetOrLoad loads a missing key once however many callers ask (singleflight). A value is stored as
// given and returned as is: a V that holds a pointer is shared memory; do not mutate it.
//
// Create with New. The zero value and a nil *Memory are not usable: every method returns ErrConfig (Len
// returns 0) instead of panicking. A Memory must not be copied after first use.
//
// ID:          TB-CACHE-002
// Pattern:     in-memory cache (LRU with TTL, read-through)
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Local caching", "Evicting data", "Lifetime of cached data"
// Goroutines:  none of its own; GetOrLoad runs a load on one singleflight goroutine, exited before GetOrLoad returns
// Cancel:      Get, Set, Delete never block; GetOrLoad returns the cause when ctx ends, the load's ctx ends when every waiter has left
// Errors:      a miss is ok == false, never an error; a panic in a load, hash or hook is panics.ErrPanic
// Blocking:    Get, Set and Delete hold one mutex for O(1) work plus at most 16 expired removals
// Math:        ALG-LIST-015, ALG-LIST-018, ALG-BLOOM-001
// Tags:        cache, lru, ttl, concurrency, generic
type Memory[K comparable, V any] struct {
	mu     sync.Mutex                  // guards lru and filter (calcbench types are not safe for concurrent use)
	lru    *list.LRUCache[K, entry[V]] // the entries, front = most recently used
	filter *bloom.BloomFilter          // nil without WithBloom

	clock   clock.Clock
	ttl     time.Duration
	onEvict func(K, V, Reason)
	hash    func(K) []byte

	flight singleflight.Group[K, V] // one load per key in flight

	// loads marks the key of each load in flight (guarded by mu; at most one entry per key, so bounded by the
	// loads in flight). Set and Delete of that key mark it stale, and a stale load is returned to its callers
	// but not stored: a write during a load is never overwritten by the older loaded value.
	loads map[K]*loadMark
}

// loadMark is one load in flight; stale is set (under Memory.mu) by a Set or Delete of its key.
type loadMark struct{ stale bool }

// errNotNew is returned by every method of a Memory not created with New (the zero value, a nil pointer).
var errNotNew = fmt.Errorf("%w: Memory not created with New", ErrConfig)

// usable reports whether m was created with New.
func (m *Memory[K, V]) usable() bool { return m != nil && m.lru != nil }

// invalidate marks a load of k in flight stale. m.mu must be held.
func (m *Memory[K, V]) invalidate(k K) {
	if lm, ok := m.loads[k]; ok {
		lm.stale = true
	}
}

// entry is a stored value and its expiry (zero: never).
type entry[V any] struct {
	v   V
	exp time.Time
}

func (e entry[V]) expired(now time.Time) bool { return !e.exp.IsZero() && !now.Before(e.exp) }

// removal is one entry that left the cache, reported to the hook after the lock is released.
type removal[K comparable, V any] struct {
	k K
	v V
	r Reason
}

// New returns an empty Memory cache configured by opts: at most DefaultCapacity entries, no expiry, the real
// clock, no hook and no bloom guard unless options say otherwise. Every invalid option is reported, joined,
// as ErrConfig.
//
// ID:          TB-CACHE-003
// Pattern:     in-memory cache (constructor)
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Local caching"
// Errors:      ErrConfig (errors.Join of every invalid option)
// Math:        ALG-LIST-015, ALG-BLOOM-001, ALG-BLOOM-003
// Tags:        cache, constructor, lru, ttl
func New[K comparable, V any](opts ...Option[K, V]) (*Memory[K, V], error) {
	c := newConfig(opts)
	if len(c.errs) > 0 {
		return nil, errors.Join(c.errs...)
	}
	lru, err := list.NewLRUCache[K, entry[V]](c.capacity)
	if err != nil { // unreachable: capacity >= 1 here
		return nil, fmt.Errorf("%w: %w", ErrConfig, err)
	}
	m := &Memory[K, V]{lru: lru, clock: c.clock, ttl: c.ttl, onEvict: c.onEvict, hash: c.hash, loads: make(map[K]*loadMark)}
	if c.bloomM > 0 {
		f, err := bloom.NewBloomFilter(c.bloomM, c.bloomK)
		if err != nil { // unreachable: OptimalParams' result is always valid
			return nil, fmt.Errorf("%w: %w", ErrConfig, err)
		}
		m.filter = f
	}
	return m, nil
}

// Get returns the value under k and marks it most recently used. ok is false on a miss, including an entry
// whose TTL has passed (it is removed: Reason Expired). err is nil unless the eviction hook panicked
// (panics.ErrPanic), or ErrConfig when m was not created with New. ctx is not used: Get never blocks; a nil
// ctx is allowed.
func (m *Memory[K, V]) Get(ctx context.Context, k K) (v V, ok bool, err error) {
	if !m.usable() {
		return v, false, errNotNew
	}
	now := m.clock.Now()
	var gone []removal[K, V]
	m.mu.Lock()
	if e, hit := m.lru.Peek(k); hit {
		if e.expired(now) {
			m.lru.Delete(k)
			gone = append(gone, removal[K, V]{k, e.v, Expired})
		} else {
			m.lru.Get(k) // a use: most recently used
			v, ok = e.v, true
		}
	}
	m.mu.Unlock()
	return v, ok, m.notify(gone)
}

// Set stores v under k, marks it most recently used and adds k to the bloom guard. ttl > 0 is the entry's
// lifetime, ttl == 0 the cache's default (WithTTL; none by default); ttl < 0 is ErrTTL and nothing changes.
// Before storing a new key, up to 16 expired entries are swept from the least recently used end; if the cache
// is still full, the least recently used entry is evicted. The hook then sees each removal (Evicted, Expired,
// Replaced). A key not equal to itself (NaN) is refused with calcbench list.ErrKeyNotComparable wrapped.
// A Set while GetOrLoad loads k wins: the older loaded value is not stored over it. ErrConfig when m was not
// created with New. ctx is not used: Set never blocks.
func (m *Memory[K, V]) Set(ctx context.Context, k K, v V, ttl time.Duration) error {
	if !m.usable() {
		return errNotNew
	}
	if ttl < 0 {
		return fmt.Errorf("%w: ttl = %v", ErrTTL, ttl)
	}
	_, err := m.set(k, v, ttl, nil)
	return err
}

// set stores v under k for ttl (0: the default). With mark != nil (a GetOrLoad store) it stores only if the
// load is still the current, non-stale load of k and k is absent, and reports whether it stored; it always
// ends the load's registration.
func (m *Memory[K, V]) set(k K, v V, ttl time.Duration, mark *loadMark) (stored bool, err error) {
	if ttl == 0 {
		ttl = m.ttl
	}
	var kb []byte
	if m.filter != nil {
		b, err := m.keyBytes(k)
		if err != nil {
			if mark != nil {
				m.mu.Lock()
				m.endLoad(k, mark)
				m.mu.Unlock()
			}
			return false, err
		}
		kb = b
	}
	now := m.clock.Now()
	var exp time.Time
	if ttl > 0 {
		exp = now.Add(ttl)
	}
	var gone []removal[K, V]
	m.mu.Lock()
	old, existed := m.lru.Peek(k)
	if mark != nil {
		current := m.loads[k] == mark
		m.endLoad(k, mark)
		if !current || mark.stale || existed { // a Set or Delete of k came after the load began
			m.mu.Unlock()
			return false, nil
		}
	} else {
		m.invalidate(k)
	}
	if !existed {
		gone = m.sweep(now, gone)
	}
	evicted, didEvict, err := m.lru.Put(k, entry[V]{v: v, exp: exp})
	if err != nil {
		m.mu.Unlock()
		return false, errors.Join(fmt.Errorf("cache: %w", err), m.notify(gone))
	}
	switch {
	case existed:
		gone = append(gone, removal[K, V]{k, old.v, reasonFor(old, now, Replaced)})
	case didEvict:
		gone = append(gone, removal[K, V]{evicted.Key, evicted.Value.v, reasonFor(evicted.Value, now, Evicted)})
	}
	if m.filter != nil {
		m.filter.Add(kb)
	}
	m.mu.Unlock()
	return true, m.notify(gone)
}

// endLoad removes mark's registration if it is still the current one for k. m.mu must be held.
func (m *Memory[K, V]) endLoad(k K, mark *loadMark) {
	if m.loads[k] == mark {
		delete(m.loads, k)
	}
}

// Delete removes k (Reason Deleted, or Expired if its TTL had passed). An absent key is not an error. ctx is
// not used: Delete never blocks. The bloom guard keeps k (a Bloom filter cannot forget). A Delete while
// GetOrLoad loads k wins: the loaded value is returned to its callers but not stored (Azure's "update the
// store, then invalidate"). ErrConfig when m was not created with New.
func (m *Memory[K, V]) Delete(ctx context.Context, k K) error {
	if !m.usable() {
		return errNotNew
	}
	now := m.clock.Now()
	var gone []removal[K, V]
	m.mu.Lock()
	m.invalidate(k)
	if e, ok := m.lru.Peek(k); ok {
		m.lru.Delete(k)
		gone = append(gone, removal[K, V]{k, e.v, reasonFor(e, now, Deleted)})
	}
	m.mu.Unlock()
	return m.notify(gone)
}

// Len returns the number of entries held, including expired ones not yet removed (0 when m was not created
// with New).
func (m *Memory[K, V]) Len() int {
	if !m.usable() {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lru.Len()
}

// AddKnown adds keys to the bloom guard: they exist in the backing store, so GetOrLoad may load them
// (priming, Azure "Priming the cache"). Without WithBloom it does nothing and returns nil. A panic in the
// hash is panics.ErrPanic and the keys before it stay added. ErrConfig when m was not created with New.
func (m *Memory[K, V]) AddKnown(keys ...K) error {
	if !m.usable() {
		return errNotNew
	}
	if m.filter == nil {
		return nil
	}
	for _, k := range keys {
		b, err := m.keyBytes(k)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.filter.Add(b)
		m.mu.Unlock()
	}
	return nil
}

// GetOrLoad returns the value under k; on a miss it calls load once, however many callers miss k at the same
// time (singleflight TB-FLIGHT-001), stores the result with the default TTL and returns it to every caller.
//
// With a bloom guard (WithBloom), a key the filter has certainly never seen returns ErrNotFound and load is
// not called. A load error is returned to every waiting caller and nothing is stored: the next GetOrLoad
// loads again (an error is never cached and never a miss: the go_45 defect). A panic in load is
// panics.ErrPanic. load runs on its own goroutine with a ctx that keeps the first caller's values; a caller
// whose ctx ends returns context.Cause(ctx) at once, and load's ctx is cancelled when every caller has left
// (the last one waits for load to return). A nil ctx is context.Background(); a nil load is panics.ErrPanic.
// The eviction hook for the store runs on the load's goroutine. A Set or Delete of k while the load runs
// wins: the loaded value is returned to the callers but not stored. ErrConfig when m was not created with
// New.
func (m *Memory[K, V]) GetOrLoad(ctx context.Context, k K, load func(context.Context) (V, error)) (V, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var zero V
	if !m.usable() {
		return zero, errNotNew
	}
	if v, ok, err := m.Get(ctx, k); ok || err != nil {
		return v, err
	}
	if m.filter != nil {
		b, err := m.keyBytes(k)
		if err != nil {
			return zero, err
		}
		m.mu.Lock()
		maybe := m.filter.MayContain(b)
		m.mu.Unlock()
		if !maybe {
			return zero, ErrNotFound // the key is not in the message: keys can be sensitive
		}
	}
	v, _, err := m.flight.Do(ctx, k, func(fctx context.Context) (V, error) {
		// Register the load before the re-check, so a Set or Delete from here on marks it stale.
		mark := &loadMark{}
		m.mu.Lock()
		m.loads[k] = mark
		m.mu.Unlock()
		defer func() { // every exit, also a panic or Goexit in load: the registration never outlives the load
			m.mu.Lock()
			m.endLoad(k, mark)
			m.mu.Unlock()
		}()
		// Another load may have stored k between the miss above and this call joining no flight.
		if v, ok, err := m.Get(fctx, k); ok || err != nil {
			return v, err
		}
		v, err := load(fctx)
		if err != nil {
			return zero, err
		}
		_, err = m.set(k, v, 0, mark)
		return v, err
	})
	return v, err
}

// sweep removes up to sweepLimit expired entries from the least recently used end and appends them to gone.
// It stops at the first oldest entry that has not expired. m.mu must be held.
func (m *Memory[K, V]) sweep(now time.Time, gone []removal[K, V]) []removal[K, V] {
	for range sweepLimit {
		k, e, ok := m.lru.Oldest()
		if !ok || !e.expired(now) {
			break
		}
		m.lru.Delete(k)
		gone = append(gone, removal[K, V]{k, e.v, Expired})
	}
	return gone
}

// notify calls the hook for each removal, outside the lock; panics are joined as panics.ErrPanic.
func (m *Memory[K, V]) notify(gone []removal[K, V]) error {
	if m.onEvict == nil || len(gone) == 0 {
		return nil
	}
	var errs []error
	for _, g := range gone {
		if err := panics.Try(func() error { m.onEvict(g.k, g.v, g.r); return nil }); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// keyBytes is the caller's hash of k, with a panic in it returned as panics.ErrPanic.
func (m *Memory[K, V]) keyBytes(k K) ([]byte, error) {
	return panics.TryValue(func() ([]byte, error) { return m.hash(k), nil })
}

// reasonFor is Expired for an entry whose TTL has passed, otherwise r.
func reasonFor[V any](e entry[V], now time.Time, r Reason) Reason {
	if e.expired(now) {
		return Expired
	}
	return r
}
