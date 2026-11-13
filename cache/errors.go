package cache

import "errors"

// Errors returned by this package. They are wrapped with detail; test with errors.Is. A panic in caller code
// (a load, a hash, an eviction hook) is panics.ErrPanic, never one of these.
var (
	// ErrConfig is returned by New for an option that is not valid: a capacity < 1, a TTL < 0, a bloom guard
	// with a nil hash or parameters calcbench ALG-BLOOM-003 rejects.
	//
	// ID:          TB-CACHE-012
	// Pattern:     sentinel error
	// Authority:   pkg.go.dev errors, "errors.Is"
	// Tags:        cache, error, configuration
	ErrConfig = errors.New("cache: invalid configuration")

	// ErrTTL is returned by Set (and Aside through it) for a ttl < 0.
	//
	// ID:          TB-CACHE-013
	// Pattern:     sentinel error
	// Authority:   pkg.go.dev errors, "errors.Is"
	// Tags:        cache, error, ttl
	ErrTTL = errors.New("cache: ttl must be >= 0")

	// ErrNotFound is returned by GetOrLoad when the bloom guard (WithBloom) says the key is certainly not
	// in the backing store: the load is skipped. It is never returned without a bloom guard.
	//
	// ID:          TB-CACHE-014
	// Pattern:     sentinel error (negative lookup)
	// Authority:   Bloom, "Space/time trade-offs in hash coding with allowable errors", CACM 13(7), 1970 (through calcbench ALG-BLOOM-001)
	// Tags:        cache, error, bloom-filter, miss
	ErrNotFound = errors.New("cache: key certainly not in the backing store (bloom guard)")

	// ErrCache is returned by Aside, together with a valid value, when the load succeeded but the cache
	// failed (its Get or its Set returned an error): the store answered, the cache did not. errors.Is also
	// matches the cache's own error.
	//
	// ID:          TB-CACHE-015
	// Pattern:     sentinel error (cache fallback)
	// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Workload design" (Reliability: "If the cache malfunctions, the workload can fall back to the origin data store")
	// Tags:        cache, error, cache-aside, fallback
	ErrCache = errors.New("cache: the cache failed; the value came from the load")
)
