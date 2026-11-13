# cache: cache-aside interface, in-memory LRU+TTL cache, GetOrLoad, bloom guard

Status: SHIPPED (2026-11-13)
Package: `cache` · SHORT: `CACHE` · Layer: core · Tier: T4
Work folder: `doc/wave6_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #30
Builds on: `singleflight` (TB-FLIGHT-001 Group.Do), `clock` (TB-CLOCK-001/004/005), `panics` (TB-PANIC-001/003/004)
· Decisions: see "For the lead" · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; reading list only, never cited)

- `go_11_cache_sync_map/main.go`: a global `sync.Map`; on a hit the handler writes the JSON and **falls through**
  (no `return`), computes again and writes a second response; every Store starts `go func(){ time.Sleep(5*time.Minute);
  cache.Delete(n) }()` (one goroutine per entry, never cancellable, and a re-Store is deleted by the older sleeper);
  nothing bounds the map. Tests: `TestDefect_go11_hitWritesTwice`, `TestDefect_go11_goroutinePerEntry`,
  `TestDefect_go11_unbounded`.
- `go_45_memcache/models/memcache.go`: `if err != nil { // Cache miss or any other error.` (every error is a miss,
  so a cache outage becomes a load per request); the loader is `interface{}` called through `reflect` and
  `panic("function must return a slice of bytes")`; `panic(err)` when Set fails; no stampede guard (the 2 s load
  runs once per concurrent request); global client. Tests: `TestDefect_go45_errorIsNotAMiss`,
  `TestDefect_go45_noStampedeGuard`, `TestDefect_go45_reflection`, `TestAside_setErrorReportedWithValue`,
  `TestAside_getErrorFallsBackToLoad`, `TestGetOrLoad_errorNotCachedAndDistinctFromMiss`.
- `go_69_bloom_filter/models/*.go`: a global `*bloom.BloomFilter` built in `init`, filled by a seeder, tested by
  handlers with no lock (a race once anything Adds while serving); a definite miss skips the database (the useful
  idea kept). Test: `TestDefect_go69_globalFilterNoLock`.

### b. The authority (opened 2026-11-13)

- Microsoft, "Cache-Aside pattern", Azure Architecture Center (ms.date 2025-09-11). Opened:
  https://learn.microsoft.com/azure/architecture/patterns/cache-aside, 2026-11-13. Relied on:
  - "Solution", steps 1-3: read the cache; on a cache miss retrieve from the data store; "adds the item to the cache
    and then returns it" → `Aside`, `GetOrLoad`. "When an application updates information, it writes the change to
    the data store and then invalidates the corresponding item in the cache" → `Delete` (and the order note at the
    end: update the store before removing the item).
  - "Lifetime of cached data" (expiration policy) → `WithTTL`, per-entry ttl on `Set`.
  - "Evicting data": "Most caches have a limited size ... Most caches adopt a least-recently-used policy" →
    `WithCapacity`, LRU.
  - "Configuration": "configure cache behavior globally or per cached item" → default TTL plus per-Set ttl.
  - "Priming the cache" → `AddKnown` for the bloom guard (and plain `Set`).
  - "Consistency" and "Staleness after writes": no guarantee between store and cache → documented, not solved.
  - "Local caching": a private in-memory cache → `Memory`.
  - "Workload design", Reliability: "If the cache malfunctions, the workload can fall back to the origin data
    store" → `Aside` loads on a cache Get error and returns `ErrCache` with the value.
  - Example: "Avoid caching a null value" → a load error is never stored.
- Bloom, "Space/time trade-offs in hash coding with allowable errors", CACM 13(7):422-426, 1970,
  doi:10.1145/362686.362692: cited through calcbench ALG-BLOOM-001's `Source:` line (not opened here; calcbench
  opened it). A Bloom filter has no false negatives, so "certainly absent" is safe to act on.
- pkg.go.dev `time` (Now), `context` (Cause), `errors` (Is, Join).

### c. Today's Go and why the export is still needed

The standard library has no cache: `sync.Map` (go_11) has no bound, no TTL, no LRU and no load-once. go1.24
`weak`/`unique` are not caches with a bound or TTL. golang-lru/v2/expirable reads `time.Now` and its TTL cleanup
goroutine never exits (expirable_lru.go:79-81). Ours: generic, bounded, clock-injected, no goroutine, load once,
errors distinct from misses, ErrPanic.

### d. calcbench (confirmed in calcbench INDEX.md and the pinned v0.3.0 source)

- ALG-LIST-015 `LRUCache` (algo/list/lru_cache.go:42): the entries and their recency; Get/Peek/Put/Delete.
- ALG-LIST-018 `LRUCache.Oldest` (algo/list/lru_cache_oldest.go:14): the bounded sweep peeks the least recently
  used entry without making it a use, removes it if expired, repeats up to 16 times.
- ALG-BLOOM-001 `BloomFilter` (NewBloomFilter, Add, MayContain) and ALG-BLOOM-003 `OptimalParams` (m, k from
  expected keys and fpr) for `WithBloom`. ALG-BLOOM-002 not called (the guard needs no rate report).
- All under `Memory.mu` (calcbench types are not safe for concurrent use).
- Not called: ALG-LIST-016 LFUCache (LADDER's `WithLFU` is not in the request; out of scope), ALG-HEAP-002/004/005
  (LADDER §7 suggested "one timer plus a heap" for expiry; lazy expiry plus the Oldest sweep needs neither a timer
  nor a heap, and the request asks for "no background sweeper unless stated and stopped").

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-CACHE-001` | `Cache` | `type Cache[K comparable, V any] interface { Get(ctx, k) (V, bool, error); Set(ctx, k, v, ttl) error; Delete(ctx, k) error }` |
| `TB-CACHE-002` | `Memory` | `type Memory[K comparable, V any] struct{…}`: Get, Set, Delete, Len, AddKnown, GetOrLoad |
| `TB-CACHE-003` | `New` | `func New[K comparable, V any](opts ...Option[K, V]) (*Memory[K, V], error)` |
| `TB-CACHE-004` | `Option` | `type Option[K comparable, V any] func(*config[K, V])` |
| `TB-CACHE-005` | `WithCapacity` | `func WithCapacity[K comparable, V any](n int) Option[K, V]` |
| `TB-CACHE-006` | `WithTTL` | `func WithTTL[K comparable, V any](d time.Duration) Option[K, V]` |
| `TB-CACHE-007` | `WithClock` | `func WithClock[K comparable, V any](c clock.Clock) Option[K, V]` |
| `TB-CACHE-008` | `WithOnEvict` | `func WithOnEvict[K comparable, V any](f func(K, V, Reason)) Option[K, V]` |
| `TB-CACHE-009` | `WithBloom` | `func WithBloom[K comparable, V any](expected uint64, fpr float64, hash func(K) []byte) Option[K, V]` |
| `TB-CACHE-010` | `Reason` | `type Reason int` (Evicted, Expired, Deleted, Replaced; String) |
| `TB-CACHE-011` | `Aside` | `func Aside[K comparable, V any](ctx, c Cache[K, V], k K, ttl time.Duration, load func(context.Context) (V, error)) (V, error)` |
| `TB-CACHE-012` | `ErrConfig` | invalid option given to New |
| `TB-CACHE-013` | `ErrTTL` | Set with ttl < 0 |
| `TB-CACHE-014` | `ErrNotFound` | GetOrLoad: the bloom guard rules the key out |
| `TB-CACHE-015` | `ErrCache` | Aside: the cache failed, the value came from the load |
| `TB-CACHE-016` | `DefaultCapacity` | `const DefaultCapacity = 10_000` |

| Option | Default | Bound it sets |
|---|---|---|
| `WithCapacity(n)` | `DefaultCapacity` (10 000) | entries held; n < 1 → ErrConfig |
| `WithTTL(d)` | 0 (no expiry) | lifetime for Set(ttl 0) and GetOrLoad stores; d < 0 → ErrConfig |
| `WithClock(c)` | `clock.Real()` | — |
| `WithOnEvict(f)` | none | — |
| `WithBloom(n, p, hash)` | none | filter bits from ALG-BLOOM-003 (≤ calcbench MaxBits, 128 MiB) |
| (internal) sweep | 16 | expired entries removed per write |

## Decisions taken (BRIEF B2 points)

1. **Size and count bounds**: count only (`WithCapacity`, ALG-LIST-015 counts entries). No byte budget/cost func:
   it would need a cost-weighted LRU calcbench does not have (a calcbench request if Devin wants it) and V's size is
   not knowable generically. Alternative that lost: a `WithMaxCost(budget, cost func(V) int64)` evicting until under
   budget over the LRU: needs a second bound in the same structure; deferred.
2. **Miss vs error** (go_45): `Get` returns `(v, ok, err)`; a miss is `ok == false, err == nil`. Memory's err is only
   a hook panic. A load error is returned to every waiter and never stored.
3. **Bloom guard on a definite miss**: only `GetOrLoad` consults it: `ErrNotFound`, load not called. Get/Set/Delete
   ignore it; `Aside` works over the interface and holds no filter. The filter holds keys the STORE has: filled by
   `AddKnown` (priming) and by every Set/successful load. Alternative that lost: fill it only from Sets (then every
   never-cached key would be refused: useless) or treat a definite miss as a plain miss and still load (no saving).
   A Bloom filter cannot forget: Delete leaves the key "maybe" (the load runs: safe direction).
4. **TTL 0**: on `Set`, 0 = the cache default (`WithTTL`), which is itself 0 = no expiry. ttl < 0 = `ErrTTL`
   (hashicorp treats ≤ 0 as no expiry; we refuse a negative rather than guess). An entry expires AT `set + ttl`
   (`!now.Before(exp)`).
5. **Expiry**: lazy (Get, Delete, Set of that key see it, Reason Expired) plus a bounded sweep (16) from the LRU end
   via ALG-LIST-018 Oldest before every Set of a new key, stopping at the first unexpired oldest; it runs before the
   capacity check so an expired entry is removed before a live one is evicted. No goroutine, no timer, no heap.
   Expired entries deeper than the first live one wait for access or eviction (the bound still holds).
6. **Eviction hook**: collected under the lock, called after unlock on the goroutine of the call that removed the
   entry (for a GetOrLoad store: the singleflight goroutine). It may call the cache. A hook panic is returned by
   that call as ErrPanic (Get/Set/Delete; GetOrLoad via singleflight); the cache state is already consistent.
7. **Aside on cache failure**: Get error → still load (Azure Reliability); success → (v, ErrCache wrapping the cache
   error). Set error → (v, ErrCache). Alternative that lost: return the Get error without loading (a cache outage
   becomes an application outage, go_45's inverse) or swallow the error (not honest).
8. **ctx on Memory Get/Set/Delete** is accepted for the interface and not used (nothing blocks); nil allowed.
9. **A write during a load wins** (reviewer blocker, fixed): each GetOrLoad load registers a mark per key under
   `mu` (`Memory.loads`, at most one per key in flight); Set and Delete of that key mark it stale; the load's
   value is returned to its callers but stored only if the mark is current, not stale and the key is absent
   (Azure "update the store, then invalidate"). Tests `TestGetOrLoad_deleteDuringLoadNotOverwritten`,
   `TestGetOrLoad_setDuringLoadNotOverwritten`, `TestGetOrLoad_writeToOtherKeyStillStores`. Alternative that
   lost: one global write counter (any write to any key would stop every load from caching).
10. **Not created with New** (zero value, nil pointer): every method returns `ErrConfig` (Len 0), never panics;
   Aside over a typed-nil `*Memory` therefore loads and returns the value with `ErrCache` (wrapping ErrConfig).
11. **NaN key** on Set: refused with `list.ErrKeyNotComparable`, joined with the hook results of entries the sweep
   already removed (they are still reported). `ErrNotFound` never carries the key (keys can be sensitive).

## Per export (summary; the doc comments are the full contract)

- **Goroutines**: none, except GetOrLoad's one singleflight goroutine per in-flight key, exited before the last
  waiting GetOrLoad returns (TB-FLIGHT-001's guarantee).
- **Cancellation**: GetOrLoad: ctx done before → cause, no load; during → cause at once, load's ctx cancelled when
  every waiter left; after → the result. Aside: ctx done before (or after a cache error) → cause, no load.
- **Errors**: ErrConfig (joined, every bad option), ErrTTL, ErrNotFound, ErrCache, calcbench
  `list.ErrKeyNotComparable` wrapped (NaN key), load's error unchanged, `panics.ErrPanic` for load/hash/hook.
- **Blocking**: Get/Set/Delete: one mutex, O(1) + ≤ 16 removals; GetOrLoad: until the load or ctx.
- **Ordering and counts**: LRU (Get and Set are uses; Delete, Oldest and the bloom are not). One load per key in
  flight. Hook once per removed entry, in removal order.
- **Zero value**: not usable; create with New. Not copied after use.
- **Clock**: WithClock; tests use clock.Fake (Advance only: the cache arms no timer).
- **Compared against**: github.com/hashicorp/golang-lru/v2/expirable v2.0.7, `compare/cache/cache_test.go`: same
  Add/Get/Remove scripts (fixed seeds, sizes 1, 2, 5, 16) give the same Get results, Len and removal order; TTL
  not compared (wall time; its cleanup goroutine never exits). Differences named: size 0 unbounded there vs
  ErrConfig here; their callback under their lock, ours after with a Reason; ours reports Replaced.
  jellydator/ttlcache/v3 and bits-and-blooms/bloom/v3 (also in LADDER) not compared: ttlcache's loader and
  expiry run on wall time; bloom's hashing differs from calcbench's so bit-level agreement is meaningless.
- **Worked examples**: `ExampleNew` (LRU evicts a), `ExampleMemory` (TTL on the fake clock), `ExampleMemory_GetOrLoad`
  (loads: 1), `ExampleAside`, `ExampleWithOnEvict`, `ExampleWithBloom`, one per export in `cache/example_test.go`.

## Tests (cache/*_test.go)

main_test.go (leak.Main, helpers) · memory_test.go (Get/Set/Delete, nil ctx, LRU order at capacity, TTL at the
exact instant, no TTL, lazy expiry, bounded sweep via Oldest (16 then the rest), sweep before capacity eviction,
every Reason, hook panic → ErrPanic, hook may call the cache, ErrTTL, NaN key, every invalid option, concurrent use
under -race) · get_or_load_test.go (load once then hits, 64 concurrent misses → 1 load (atomics, channels), error
not cached / distinct from miss, error returns zero V, panic and nil load → ErrPanic, hit skips load, cancel
before/during/after, bloom definite miss skips load, Set adds to bloom, Get ignores bloom, hash panic) ·
aside_test.go · writes_during_load_test.go (write during load, NaN sweep report, not-from-New, ErrNotFound without the key) · defects_test.go (go_11 ×3, go_45 ×3, go_69) · example_test.go. No time.Sleep; no wall clock.

## Out of scope

LFU (`WithLFU`, ALG-LIST-016) and a byte budget (decision 1); write-through/write-behind; a cross-instance cache
(adapters/redis, adapters/memcache implement `Cache`); stale-while-revalidate; negative caching of load errors.

## For the lead

- taxonomy: `cache` status planned → built. INDEX: TB-CACHE-001..016 (regenerate).
- DECISIONS: the eight points above (count-only bound; bloom only in GetOrLoad with AddKnown; ttl < 0 = ErrTTL;
  lazy + 16-entry Oldest sweep instead of LADDER §7's timer+heap; hook outside the lock with ErrPanic; Aside falls
  back on cache failure with ErrCache; ctx unused by Memory ops).
- Deviations from LADDER §4: `Store` interface → `Cache` (BRIEF), `New` returns `(*Memory, error)` not `*Cache`;
  `Get(k) (V, bool)` → `Get(ctx, k) (V, bool, error)`; `NewBloom` standalone type → `WithBloom` option + `AddKnown`
  (AddKnown is an addition to the BRIEF's shape: the guard needs a way to be primed); `WithLFU` dropped; `Len` added.
- compare/go.mod: add `github.com/hashicorp/golang-lru/v2 v2.0.7` (already in the module cache); compare/README row.
- doc/sources: Azure Cache-Aside (TB-CACHE-001..016), Bloom 1970 via calcbench (TB-CACHE-009, -014).
- calcbench requests: none required. Optional: a cost-weighted LRU if a byte budget is wanted.
- Checks: `gofmt -l cache` clean, `go build ./...` clean, `go vet ./cache/` clean (tests included).

Verified: `go test -race -timeout 5m -count=50 ./cache/` ok (after the reviewer's blocker fix).
