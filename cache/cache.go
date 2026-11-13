package cache

import (
	"context"
	"time"
)

// Cache is the cache-aside interface: the small set of calls Aside needs, implemented by Memory here and by
// the adapters (Redis, memcache) over a network. Get reports a miss as ok == false with a nil error; an error
// means the cache itself failed and is never a miss (the go_45 defect). Set stores v under k for ttl; ttl == 0
// is the implementation's default lifetime and ttl < 0 is ErrTTL. Delete removes k; deleting an absent key is
// not an error.
//
// ID:          TB-CACHE-001
// Pattern:     cache-aside (cache interface)
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Solution" (read, miss, load, add; invalidate on update)
// Errors:      a cache failure is an error, a miss is ok == false; ErrTTL for ttl < 0
// Tags:        cache, cache-aside, interface, generic
type Cache[K comparable, V any] interface {
	Get(ctx context.Context, k K) (V, bool, error)
	Set(ctx context.Context, k K, v V, ttl time.Duration) error
	Delete(ctx context.Context, k K) error
}
