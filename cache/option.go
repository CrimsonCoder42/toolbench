package cache

import (
	"fmt"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/bloom"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// Option configures New. The zero value (a nil Option) is ignored. An option given an invalid value makes New
// return ErrConfig; it never panics.
//
// ID:          TB-CACHE-004
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        cache, options, configuration
type Option[K comparable, V any] func(*config[K, V])

// DefaultCapacity is the entry bound of a Memory cache when WithCapacity is not passed.
//
// ID:          TB-CACHE-016
// Pattern:     bounded cache (default bound)
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Evicting data" ("Most caches have a limited size")
// Tags:        cache, bound, capacity, default
const DefaultCapacity = 10_000

// sweepLimit bounds the expired entries one write removes from the least recently used end (lazy expiry
// plus a bounded sweep: no goroutine, no unbounded pause).
const sweepLimit = 16

type config[K comparable, V any] struct {
	capacity int
	ttl      time.Duration
	clock    clock.Clock
	onEvict  func(K, V, Reason)
	bloomM   uint64 // > 0 when WithBloom was given and valid
	bloomK   int
	hash     func(K) []byte
	errs     []error
}

func newConfig[K comparable, V any](opts []Option[K, V]) config[K, V] {
	c := config[K, V]{capacity: DefaultCapacity}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.clock == nil {
		c.clock = clock.Real()
	}
	return c
}

// WithCapacity bounds the number of entries (default DefaultCapacity). A new key at a full cache evicts the
// least recently used entry (Reason Evicted). n < 1 makes New return ErrConfig. The bound is a count of
// entries; there is no byte budget (doc/specs/cache.md, "Size bound").
//
// ID:          TB-CACHE-005
// Pattern:     bounded cache, least recently used eviction
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Evicting data" ("Most caches adopt a least-recently-used policy")
// Math:        ALG-LIST-015
// Tags:        cache, capacity, lru, option
func WithCapacity[K comparable, V any](n int) Option[K, V] {
	return func(c *config[K, V]) {
		if n < 1 {
			c.errs = append(c.errs, fmt.Errorf("%w: capacity = %d, want >= 1", ErrConfig, n))
			return
		}
		c.capacity = n
	}
}

// WithTTL sets the default lifetime of an entry: what Set with ttl == 0 and every GetOrLoad store use
// (default 0: no expiry). d < 0 makes New return ErrConfig.
//
// ID:          TB-CACHE-006
// Pattern:     time-to-live expiry
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Lifetime of cached data"
// Tags:        cache, ttl, expiry, option
func WithTTL[K comparable, V any](d time.Duration) Option[K, V] {
	return func(c *config[K, V]) {
		if d < 0 {
			c.errs = append(c.errs, fmt.Errorf("%w: ttl = %v, want >= 0", ErrConfig, d))
			return
		}
		c.ttl = d
	}
}

// WithClock sets the clock that expiry reads (default clock.Real()). Tests pass a *clock.Fake. A nil clock
// is the default.
//
// ID:          TB-CACHE-007
// Pattern:     injectable clock
// Authority:   pkg.go.dev time, "Now"
// Tags:        cache, clock, testing, option
func WithClock[K comparable, V any](cl clock.Clock) Option[K, V] {
	return func(c *config[K, V]) { c.clock = cl }
}

// WithOnEvict sets a hook called once for every entry that leaves the cache, with its key, value and Reason.
// It runs on the goroutine of the call that removed the entry (for a GetOrLoad store, the load's goroutine),
// after the cache's lock is released, so it may call the cache. A panic in it is returned by that call as
// panics.ErrPanic; the cache stays consistent. Default: none.
//
// ID:          TB-CACHE-008
// Pattern:     eviction listener (hook)
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Evicting data"
// Tags:        cache, eviction, hook, observability, option
func WithOnEvict[K comparable, V any](f func(K, V, Reason)) Option[K, V] {
	return func(c *config[K, V]) { c.onEvict = f }
}

// WithBloom adds a bloom guard for negative lookups: a Bloom filter sized for expected keys at false-positive
// rate fpr (calcbench ALG-BLOOM-003), holding the keys the backing store is known to have. The caller fills it
// with Memory.AddKnown (priming); Set and every successful GetOrLoad also add their key. GetOrLoad on a key
// the filter has certainly never seen returns ErrNotFound without calling load (the store is not asked about
// keys it cannot have). Get, Set, Delete and Aside ignore the guard. hash turns a key into the bytes the filter
// hashes; it must give equal bytes for equal keys. A nil hash, expected < 1 or fpr outside (0, 1) makes New
// return ErrConfig. A Bloom filter cannot forget: a deleted key keeps testing "maybe" (the load then runs).
//
// ID:          TB-CACHE-009
// Pattern:     bloom filter guard (negative lookup)
// Authority:   Bloom, "Space/time trade-offs in hash coding with allowable errors", CACM 13(7):422-426, 1970 (through calcbench ALG-BLOOM-001)
// Math:        ALG-BLOOM-001, ALG-BLOOM-003
// Tags:        cache, bloom-filter, negative-lookup, option
func WithBloom[K comparable, V any](expected uint64, fpr float64, hash func(K) []byte) Option[K, V] {
	return func(c *config[K, V]) {
		if hash == nil {
			c.errs = append(c.errs, fmt.Errorf("%w: WithBloom hash is nil", ErrConfig))
			return
		}
		m, k, err := bloom.OptimalParams(expected, fpr)
		if err != nil {
			c.errs = append(c.errs, fmt.Errorf("%w: WithBloom: %w", ErrConfig, err))
			return
		}
		c.bloomM, c.bloomK, c.hash = m, k, hash
	}
}
