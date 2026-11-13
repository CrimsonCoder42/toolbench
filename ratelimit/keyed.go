package ratelimit

import (
	"fmt"
	"sync"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/list"
	meter "github.com/CrimsonCoder42/calcbench/algo/ratelimit"
	"github.com/CrimsonCoder42/calcbench/num/integer"
	"github.com/CrimsonCoder42/toolbench/clock"
)

// Keyed is a set of token buckets, one per key (a client IP, a user ID), each with the same Limit and
// burst, held in a bounded table: at most WithMaxKeys keys, and a key unused for WithIdleTTL is evicted on
// a later call (no background goroutine; eviction is driven by the clock reading of each call). A new key
// that finds the table full with nothing idle is refused with ErrKeys. Each bucket is calcbench
// ALG-RATELM-001; the table is calcbench ALG-LIST-015 LRUCache, whose least recently used entry
// (ALG-LIST-018 Oldest) is evicted while it is idle; the table and the buckets are guarded by one mutex. Safe for concurrent use. Create with
// NewKeyed; the zero value is not usable (its methods refuse every event), and a Keyed must not be copied
// after first use.
//
// ID:          TB-RATE-005
// Pattern:     keyed token bucket (per-client rate limiting)
// Authority:   pkg.go.dev golang.org/x/time/rate, "type Limiter" (one Limiter per client)
// Authority:   Microsoft, Azure Architecture Center, "Rate Limiting pattern", 2026
// Authority:   Heinanen and Guérin, "A Single Rate Three Color Marker", RFC 2697, IETF, 1999, §3 Metering
// Goroutines:  none
// Errors:      NewKeyed: ErrLimit, ErrBurst; AllowN: ErrBurst when n > burst, ErrKeys when the table is full (or the key is not equal to itself)
// Blocking:    never; one mutex held for O(1) amortised work per call
// Math:        ALG-RATELM-001, ALG-LIST-015, ALG-LIST-018, NUM-INTEG-013
// Tags:        concurrency, rate-limiting, token-bucket, per-key, eviction, bounded
type Keyed[K comparable] struct {
	clock   clock.Clock
	limit   Limit
	burst   int
	maxKeys int
	idleTTL time.Duration

	mu    sync.Mutex                // guards table and every entry (calcbench types are not safe for concurrent use)
	table *list.LRUCache[K, *entry] // key → bucket; least recently used = least recently seen; capacity maxKeys
}

// entry is one key's bucket and the latest clock reading that used it. Guarded by Keyed.mu.
type entry struct {
	b    *meter.TokenBucket
	seen time.Time
}

// NewKeyed returns an empty Keyed limiter: each key gets a full bucket of burst tokens refilled at rate r
// when it is first seen. Options: WithClock, WithMaxKeys (default DefaultMaxKeys), WithIdleTTL (default the
// full-refill time, ⌈burst × Per / Events⌉, so eviction never changes a decision).
//
// ID:          TB-RATE-016
// Pattern:     keyed token bucket (constructor)
// Authority:   pkg.go.dev golang.org/x/time/rate, "func NewLimiter" (one Limiter per client)
// Errors:      ErrLimit when r is not valid (Events < 1, Per <= 0, or burst × Per overflows int64); ErrBurst when burst < 1
// Math:        ALG-RATELM-001, ALG-LIST-015, NUM-INTEG-013
// Tags:        rate-limiting, token-bucket, per-key, constructor
func NewKeyed[K comparable](r Limit, burst int, opts ...Option) (*Keyed[K], error) {
	if _, err := newBucket(r, burst); err != nil {
		return nil, err
	}
	c := newConfig(opts)
	if c.idleTTL <= 0 {
		// burst × Per fits int64: newBucket checked it.
		fill, err := integer.CeilDiv(int64(burst)*int64(r.Per), r.Events)
		if err != nil { // unreachable: Events >= 1
			return nil, fmt.Errorf("%w: %w", ErrLimit, err)
		}
		c.idleTTL = time.Duration(max(fill, 1))
	}
	table, err := list.NewLRUCache[K, *entry](c.maxKeys)
	if err != nil { // unreachable: newConfig makes maxKeys >= 1
		return nil, fmt.Errorf("ratelimit: key table: %w", err)
	}
	return &Keyed[K]{
		clock: c.clock, limit: r, burst: burst, maxKeys: c.maxKeys, idleTTL: c.idleTTL, table: table,
	}, nil
}

// Limit returns the rate every key's bucket has.
func (k *Keyed[K]) Limit() Limit { return k.limit }

// Burst returns the burst every key's bucket has.
func (k *Keyed[K]) Burst() int { return k.burst }

// Len returns the number of keys held now (after evicting the idle ones).
func (k *Keyed[K]) Len() int {
	if k.table == nil {
		return 0
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.evictIdle(k.clock.Now())
	return k.table.Len()
}

// Allow is AllowN(key, 1) without the error: ok reports whether one event for key may happen now (and if so
// takes its token); when refused, retryAfter is the shortest wait after which it would be allowed (for an
// HTTP 429 Retry-After), including the wait for a free slot when the table is full.
func (k *Keyed[K]) Allow(key K) (ok bool, retryAfter time.Duration) {
	ok, retryAfter, _ = k.AllowN(key, 1)
	return ok, retryAfter
}

// AllowN reports whether n events for key may happen now, and if so takes n tokens from key's bucket. A
// refused call takes nothing and returns the shortest wait after which the same call would be allowed.
// n <= 0 returns (true, 0, nil) and adds no key. Errors: ErrBurst (wrapped) when n > burst, with
// retryAfter InfDuration; ErrKeys when key is new, the table holds WithMaxKeys keys and none is idle, with
// retryAfter the time until the least recently seen key becomes idle; ErrKeys (wrapping calcbench's
// list.ErrKeyNotComparable) with retryAfter InfDuration when key is not equal to itself (a NaN), which no
// table can find again.
func (k *Keyed[K]) AllowN(key K, n int) (ok bool, retryAfter time.Duration, err error) {
	if n <= 0 {
		return true, 0, nil
	}
	if n > k.burst {
		return false, InfDuration, fmt.Errorf("%w: AllowN(n=%d) exceeds the burst %d", ErrBurst, n, k.burst)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	// Read under the lock, so callers' readings of a monotonic clock reach the table in order; a reading
	// earlier than a key's last one is still decided as no time passed (the meter's rule) and never moves
	// seen back.
	now := k.clock.Now()
	k.evictIdle(now)

	e, found := k.table.Get(key) // a hit marks key most recently seen
	if !found {
		if k.table.Len() >= k.maxKeys {
			_, oldest, _ := k.table.Oldest() // the table is full, so there is an oldest
			return false, oldest.seen.Add(k.idleTTL).Sub(now), ErrKeys
		}
		b, err := meter.NewTokenBucket(k.limit.Events, k.limit.Per, int64(k.burst))
		if err != nil { // unreachable: NewKeyed validated the same parameters
			return false, InfDuration, fmt.Errorf("%w: %w", ErrLimit, err)
		}
		e = &entry{b: b}
		if _, _, err := k.table.Put(key, e); err != nil { // never evicts: the table is not full
			return false, InfDuration, fmt.Errorf("%w: %w", ErrKeys, err)
		}
	}
	if now.After(e.seen) {
		e.seen = now
	}
	ok, retryAfter = e.b.Allow(now, int64(n))
	return ok, retryAfter, nil
}

// evictIdle removes, from the least recently seen end (ALG-LIST-018 Oldest), every key unseen for at least
// idleTTL. Amortised O(1): each key is removed once. Called with k.mu held.
func (k *Keyed[K]) evictIdle(now time.Time) {
	for {
		key, e, ok := k.table.Oldest()
		if !ok || now.Sub(e.seen) < k.idleTTL {
			return
		}
		k.table.Delete(key)
	}
}
