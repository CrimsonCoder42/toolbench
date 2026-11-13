# Requests for calcbench (from toolbench wave 5, 2026-11-13)

Same shape as `doc/wave1_2026-11-13/calcbench_requests.md`. Nothing is built for them in toolbench; `ratelimit`
works around both today (doc/specs/ratelimit.md, D-031).

## R-4 TokenBucket restore (or a debt-taking reserve)

| ID | Name | Signature |
|---|---|---|
| ALG-RATELM-? (`algo/ratelimit`) | `TokenBucket.Restore` | `func (b *TokenBucket) Restore(n int64, now time.Time)` — give back n tokens taken earlier, capped at the burst |

- **Source**: RFC 2697 §3 (the token bucket); golang.org/x/time/rate `Reservation.Cancel` (restores tokens not
  consumed by later reservations).
- **Why**: toolbench `ratelimit.Reservation.Cancel` today restores a saved copy of the whole bucket, so it can only
  restore when no later reservation has taken tokens; a calcbench `Restore` (or `Reserve` that may go into debt)
  would let it match x/time/rate exactly.
- **Worked example**: burst 5, take 3 at t0 (2 left), Restore(3, t0) → 5; Restore(10, t0) → 5 (capped).
- **Needed by**: toolbench `ratelimit` (TB-RATE-004 Reservation.Cancel).

## R-5 LRUCache oldest entry without eviction

| ID | Name | Signature |
|---|---|---|
| ALG-LIST-? (`algo/list`) | `LRUCache.Oldest` | `func (c *LRUCache[K, V]) Oldest() (K, V, bool)` — the least-recently-used entry, not removed, recency unchanged |

- **Source**: the LRU policy as ALG-LIST-015 already cites it.
- **Why**: toolbench `ratelimit.Keyed` must check whether the oldest key is idle before evicting it; ALG-LIST-015
  can only evict, so `Keyed` keeps its own map + container/list today.
- **Worked example**: Put a, b, c; Get a → Oldest() = b; Oldest() again = b (unchanged).
- **Needed by**: toolbench `ratelimit` (TB-RATE-005 Keyed), later `cache` (wave 6).
