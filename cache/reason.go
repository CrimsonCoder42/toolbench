package cache

import "strconv"

// Reason says why an entry left a Memory cache, as WithOnEvict reports it. The zero value is Evicted.
//
// ID:          TB-CACHE-010
// Pattern:     eviction notification (removal cause)
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Problems and considerations" ("Lifetime of cached data", "Evicting data")
// Tags:        cache, eviction, ttl, hook
type Reason int

// The reasons an entry leaves the cache.
const (
	Evicted  Reason = iota // a new key arrived at a full cache and this was the least recently used entry
	Expired                // its TTL had passed (found on access, by the bounded sweep, or when removed or replaced)
	Deleted                // Delete removed it before it expired
	Replaced               // Set stored a new value under its key before it expired
)

// String returns "evicted", "expired", "deleted" or "replaced", and "Reason(n)" for any other value.
func (r Reason) String() string {
	switch r {
	case Evicted:
		return "evicted"
	case Expired:
		return "expired"
	case Deleted:
		return "deleted"
	case Replaced:
		return "replaced"
	}
	return "Reason(" + strconv.Itoa(int(r)) + ")"
}
