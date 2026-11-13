// Package cache is the cache-aside pattern: a small Cache interface (Get, Set, Delete) that in-memory and
// network caches implement, Aside to read through any Cache, and Memory, an in-memory cache bounded by entry
// count in least-recently-used order with per-entry TTL, an optional bloom guard for negative lookups, and
// GetOrLoad, which loads a missing key once however many callers miss it at the same time.
//
// A miss is never an error and an error is never a miss: Get returns (v, ok, err), and a load error is
// returned, not cached. Expired entries are removed when a call meets them, plus a bounded sweep (at most 16
// per write) from the least recently used end; nothing runs in the background and no goroutine is started
// per entry. Time comes from a clock.Clock (WithClock), so tests use clock.Fake. The LRU order is calcbench
// ALG-LIST-015 LRUCache and ALG-LIST-018 Oldest, the bloom guard ALG-BLOOM-001 BloomFilter sized by
// ALG-BLOOM-003 OptimalParams, all under the Memory's one mutex. The eviction hook runs outside that mutex.
//
// ID short form: CACHE (IDs are TB-CACHE-NNN).
//
// Exports:
//
//	TB-CACHE-001  Cache            the cache-aside interface: Get (miss is ok == false), Set with a TTL, Delete
//	TB-CACHE-002  Memory           an in-memory LRU cache with TTL, safe for concurrent use: Get, Set, Delete, GetOrLoad, AddKnown, Len
//	TB-CACHE-003  New              an empty Memory configured by options; ErrConfig when an option is invalid
//	TB-CACHE-004  Option           configures New
//	TB-CACHE-005  WithCapacity     bounds the entries (default DefaultCapacity); least recently used is evicted
//	TB-CACHE-006  WithTTL          sets the default lifetime of an entry (default 0: no expiry)
//	TB-CACHE-007  WithClock        sets the clock expiry reads (default clock.Real)
//	TB-CACHE-008  WithOnEvict      sets a hook told of every entry that leaves, with its Reason, outside the lock
//	TB-CACHE-009  WithBloom        adds a bloom guard: GetOrLoad on a certainly-absent key returns ErrNotFound without loading
//	TB-CACHE-010  Reason           why an entry left: Evicted, Expired, Deleted, Replaced
//	TB-CACHE-011  Aside            the cache-aside read over any Cache: get, on a miss load and set
//	TB-CACHE-012  ErrConfig        the sentinel for an invalid option given to New
//	TB-CACHE-013  ErrTTL           the sentinel for a negative ttl given to Set
//	TB-CACHE-014  ErrNotFound      the sentinel GetOrLoad returns when the bloom guard rules a key out
//	TB-CACHE-015  ErrCache         the sentinel Aside returns with a valid value when the cache failed
//	TB-CACHE-016  DefaultCapacity  the entry bound when WithCapacity is not given: 10 000
package cache
