# ratelimit: a token-bucket Limiter (x/time/rate shape) and a bounded per-key limiter

Status: SHIPPED (2026-11-13; reviewer blocker fixed, see "Bug fixed in review"; wave 6: moved onto calcbench
v0.3.0, see "Wave 6: calcbench v0.3.0 switch")
Package: `ratelimit` · SHORT: `RATE` · Layer: core · Tier: T3
Work folder: `doc/wave5_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #26
Builds on: `clock` (Clock, Timer; tests with Fake), calcbench v0.3.0 `algo/ratelimit` (ALG-RATELM-001,
ALG-RATELM-005, imported as `meter`), calcbench `algo/list` (ALG-LIST-015, ALG-LIST-018), calcbench `num/integer`
(NUM-INTEG-013). Not used: `panics` (no caller code ever runs inside the
limiter: no callback, no goroutine), `guard` (one plain `sync.Mutex` per limiter, named beside its fields, is the
whole guard; `guard.Guarded` would add an indirection for nothing) · Decisions/assumptions: "For the lead"

## Research

### a. The reference (code-heim; a reading list, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `go_77_rate_limit/main.go` | HTTP middleware: one `rate.Limiter` per client IP in a plain `map[string]*rate.Limiter` (`:27`). | **Data race**: the map is read and written from every request goroutine with no lock (`:34-38`). **No eviction**: the map grows with every IP ever seen. Test `TestKeyed_go77_concurrentKeysNoDataRace` (16 keys × 8 goroutines at one instant under `-race`; each key gets exactly its burst). |
| `go_82_rate_limiting_revised/server/main.go` | Same with a `sync.Mutex` around the map (`:27-36`). | Does not compile: `sync` is not imported. **No eviction** (unbounded map). `log.Printf` per request. |
| `go_82_rate_limiting_revised/server_with_sync_map/main.go` | Global `sync.Map`, `LoadOrStore(ip, rate.NewLimiter(...))`. | **Allocation per request**: the new Limiter is built before LoadOrStore whether or not the key exists; **global state**; no eviction. Test `TestKeyed_go82_boundedAndNoAllocationPerRequest` (1000 distinct keys never hold more than `WithMaxKeys(8)`; `testing.AllocsPerRun` of Allow on a known key is 0). |

### b. The authority

- golang.org/x/time/rate. Opened: https://pkg.go.dev/golang.org/x/time/rate (v0.16.0, 2026-08-19), 2026-11-13,
  and its source https://raw.githubusercontent.com/golang/time/master/rate/rate.go, 2026-11-13. Relied on:
  "A Limiter controls how frequently events are allowed to happen. It implements a 'token bucket' of size b,
  initially full and refilled at rate r tokens per second"; the zero Limiter rejects all events; `WaitN`: "It
  returns an error if n exceeds the Limiter's burst size, the Context is canceled, or the expected wait time
  exceeds the Context's Deadline"; `CancelAt`: "reverses the effects of this Reservation on the rate limit".
  From the source: `reserveN` takes the tokens even when they are not there (the balance goes negative: debt)
  and sets `timeToAct = t + wait`; `AllowN` is `reserveN` with `maxFutureReserve = 0`; `CancelAt` returns at
  once when `timeToAct` is before t, else restores `tokens − tokensFromDuration(lastEvent − timeToAct)` (n
  minus what later reservations took) capped at burst; `wait` checks `n > burst` before ctx.
- Heinanen and Guérin, "A Single Rate Three Color Marker", RFC 2697, IETF, 1999, §3 Metering (through calcbench
  ALG-RATELM-001's citation: the bucket starts full at CBS, refills at CIR up to CBS, a request of B conforms
  when Tc − B ≥ 0).
- Microsoft, Azure Architecture Center, "Rate Limiting pattern" (ms.date 2026-06-08). Opened:
  https://learn.microsoft.com/azure/architecture/patterns/rate-limiting-pattern, 2026-11-13. Relied on:
  "propagate back-pressure signals (for example, HTTP 429 with Retry-After)" → `Keyed.Allow` returns the
  retry-after.

### c. Today's Go and why the export is still needed

Go 1.27 has no rate limiter in the standard library; x/time/rate is the familiar one. What it does not give, and
every consumer hand-rolls (LADDER #26: 6 sites in advo/red): a per-key table that is bounded and evicts idle keys
(the go_77/go_82 defects), a retry-after for a 429, an injectable clock (x/time/rate takes explicit `time.Time`
arguments instead, so Wait cannot be driven by a fake clock), and exact integer arithmetic (x/time/rate's
float64 tokens round; `Every(3*time.Nanosecond)` with burst 7 drifts, `TestLimiter_noDrift`).

### d. calcbench (confirmed in `/Users/devinanderson/Desktop/calcbench/INDEX.md`)

- **ALG-RATELM-001 TokenBucket: USED** for all bucket arithmetic (Limiter and every Keyed bucket), under the
  limiter's mutex (calcbench meters are not safe for concurrent use, `algo/ratelimit/doc.go:18`). x/time/rate's
  shape needs two things the meter has no method for, and both are expressed with it, not re-implemented:
  1. *Debt* (`ReserveN` takes tokens that are not there yet; the meter never goes below zero). Expressed by
     time: a reservation that must wait d is taken from the meter at `now+d` (`meter.Allow(now+d, n)`, which
     conforms by construction since d is the meter's own wait). The meter treats any earlier `now` as its
     latest one, so no event conforms before `now+d`; from `now+d` it refills from the same level x/time/rate's
     negative balance reaches at that instant. A call at `now` < the meter's latest instant is decided on a
     **copy** of the meter (`probe := l.st.b`) so a refusal changes nothing (`limiter.go:184`). Debt is recorded explicitly
     (`state.debt`), never inferred from the meter's latest instant (see "Bug fixed in review").
  2. *Cancel* (wave 6): **Math: ALG-RATELM-005** TokenBucket.Restore. The count is x/time/rate CancelAt's:
     n minus the tokens taken after this reservation, measured from its time to act to the latest one
     (`state.lastEvent`), never below 0; nothing once the clock is after the time to act. Not in debt, the
     count goes back with `Restore(k, now)` (refill to now, add, cap at burst: CancelAt's `advance` then `+=`
     then cap). In debt the meter sits empty at the latest time to act, which is in the future, and x/time/rate's
     balance after `+k` is zero k token-times earlier; Restore adds at the meter's latest instant and cannot
     express that, so the meter is rebuilt empty at `debt − timeFor(k)` (ALG-RATELM-001: a fresh copy that takes
     its burst there) and `debt` moves with it. Token counts and token-times are measured on a copy of the unused
     meter (`Limiter.tokensBetween`, `Limiter.timeFor`), no float and no formula here. The wave 5 snapshot and
     generation counter are gone (`_delete/ratelimit/limiter.go.removed.go.txt`,
     `_delete/ratelimit/reservation.go.removed.go.txt`).
  Proof that debt-by-time equals x/time/rate (rate r, reservation at t needing n with level L < n): x/time/rate
  balance at u ≥ t is `L − n + (u−t)·r`; ours is the meter at `t + (n−L)/r` holding `0`, then `(u − t − (n−L)/r)·r`
  = the same; before `t + (n−L)/r` x/time/rate's balance is negative (refuses any m ≥ 1) and ours refuses (debt
  branch). The compare tests replay this on x/time/rate.
- **ALG-RATELM-002/-003/-004**: not used (LADDER says "by option"; no option was asked for in B3: out of scope).
- **NUM-INTEG-013 CeilDiv: USED** for the default idle TTL ⌈burst × Per / Events⌉ (`keyed.go:67`).
- **ALG-LIST-015 LRUCache, ALG-LIST-018 Oldest: USED** (wave 6) for Keyed's table, capacity WithMaxKeys, under
  Keyed's mutex (calcbench types are not safe for concurrent use). A hit is `Get` (marks the key most recently
  seen); idle eviction peeks `Oldest` and `Delete`s it while it is idle; a full table with a non-idle oldest key
  refuses with ErrKeys and the retry-after from `Oldest`'s `seen`, so `Put` never evicts. The wave 5
  `map` + `container/list` is gone (`_delete/ratelimit/keyed.go.removed.go.txt`). A key not equal to itself
  (NaN) is refused by `Put` with `list.ErrKeyNotComparable`; Keyed returns ErrKeys wrapping it (before, each NaN
  call added an entry nobody could find until the table filled; `TestKeyed_NaNKeyRefused`).

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-RATE-001` | `Limit` | `type Limit struct{ Events int64; Per time.Duration }` |
| `TB-RATE-002` | `Every` | `func Every(interval time.Duration) Limit` |
| `TB-RATE-003` | `Limiter` | `func NewLimiter(r Limit, burst int, opts ...Option) (*Limiter, error)`; `Allow() bool`, `AllowN(n int) bool`, `Reserve() *Reservation`, `ReserveN(n int) *Reservation`, `Wait(ctx) error`, `WaitN(ctx, n int) error`, `Limit() Limit`, `Burst() int` |
| `TB-RATE-004` | `Reservation` | `OK() bool`, `Delay() time.Duration`, `DelayFrom(t time.Time) time.Duration`, `Cancel()` |
| `TB-RATE-005` | `Keyed` | `func NewKeyed[K comparable](r Limit, burst int, opts ...Option) (*Keyed[K], error)`; `Allow(key K) (ok bool, retryAfter time.Duration)`, `AllowN(key K, n int) (ok bool, retryAfter time.Duration, err error)`, `Len() int`, `Limit() Limit`, `Burst() int` |
| `TB-RATE-006` | `Option` | `type Option func(*config)` |
| `TB-RATE-007` | `WithClock` | `func WithClock(c clock.Clock) Option` |
| `TB-RATE-008` | `WithMaxKeys` | `func WithMaxKeys(n int) Option` |
| `TB-RATE-009` | `WithIdleTTL` | `func WithIdleTTL(d time.Duration) Option` |
| `TB-RATE-010` | `ErrLimit` | `var ErrLimit error` |
| `TB-RATE-011` | `ErrBurst` | `var ErrBurst error` |
| `TB-RATE-012` | `ErrKeys` | `var ErrKeys error` |
| `TB-RATE-013` | `DefaultMaxKeys` | `const DefaultMaxKeys = 10_000` |
| `TB-RATE-014` | `InfDuration` | `const InfDuration = time.Duration(math.MaxInt64)` |
| `TB-RATE-015` | `NewLimiter` | `func NewLimiter(r Limit, burst int, opts ...Option) (*Limiter, error)` |
| `TB-RATE-016` | `NewKeyed` | `func NewKeyed[K comparable](r Limit, burst int, opts ...Option) (*Keyed[K], error)` |

| Option | Default | Bound it sets |
|---|---|---|
| `WithClock(c)` | `clock.Real()` (also for nil) | every reading of time and every wait |
| `WithMaxKeys(n)` | `DefaultMaxKeys` 10 000 (also n <= 0) | keys a Keyed holds |
| `WithIdleTTL(d)` | ⌈burst × Per / Events⌉, the full-refill time (also d <= 0): eviction never changes a decision | how long an unused key is kept |

Sentinels: `ErrLimit` (Events < 1, Per <= 0, burst × Per overflows int64), `ErrBurst` (burst < 1 at
construction; n > burst in WaitN and Keyed.AllowN), `ErrKeys` (new key, table full, nothing idle). No `ErrPanic`:
no caller code runs.

### Deviations from LADDER §4 and the B3 brief, with why

1. **Limit is integer `{Events, Per}`, not `float64`** (brief offered both; §4 had `New(perInterval, interval,
   burst)`). Why: ALG-RATELM-001 is integer per interval and exact; a float rate cannot be converted to it exactly
   (`rate.Limit(0.3)` has no exact integer ratio over ns). `Every(d)` is exact for every d. x/time/rate's
   `rate.Limit(r)` for a whole r is `Limit{r, time.Second}`. Lost: `Inf` and a zero rate (both rejected with
   ErrLimit; "no limit" is not having a limiter).
2. **`NewLimiter` / `NewKeyed` return `(…, error)`**, as §4's `New` did; x/time/rate's `NewLimiter` returns none.
   Why: wave lesson "invalid counts are an error or a documented default, never a panic"; there is no sensible
   default rate.
3. **Name `NewLimiter`** (x/time/rate's) instead of §4's `New`; `AllowN(n) bool` (x/time/rate's) instead of §4's
   `(ok, retryAfter)`: the retry-after lives on `Keyed.Allow` (as §4 says, for the 429) and on
   `Reservation.Delay` for Limiter.
4. **Cancel matches CancelAt** (wave 6; until wave 6 it gave back nothing after a later take). Remaining
   differences: whole tokens (when the later takes span a fraction of a token-time, the fraction CancelAt would
   subtract is not subtracted: at most one token more), never more than n (a later take with an earlier time
   to act, possible only with out-of-order times, counts as 0), and a second Cancel after one that gave tokens
   back does nothing (CancelAt can give back twice). `AllowN(0)` does not move `lastEvent` (x/time/rate's
   does). Tested: `TestReservation_Cancel`, `TestCompare_CancelAfterLaterReservation`,
   `TestCompare_CancelAtTimeToAct`, `TestCompare_CancelEarlierInDebt`.
5. **No `…At(t time.Time)` variants** (`AllowN(t, n)`, `ReserveN(t, n)`, `CancelAt`): time comes from
   `WithClock`, the toolbench rule. Tests drive a `clock.Fake`.
6. **WaitN does not pre-check the ctx deadline**: x/time/rate fails at once when the wait exceeds the deadline;
   ours waits until ctx is done. Why: the deadline is wall time and the wait is on the limiter's clock (a Fake in
   tests); comparing them is wrong under a fake clock. The cancelled Wait gives its tokens back (Cancel).
7. **`Keyed.AllowN` returns `(ok, retryAfter, err)`** in addition to the brief's `Allow(key) (ok, retryAfter)`:
   ErrKeys and ErrBurst need a channel; `Allow` folds ErrKeys into `(false, time until a slot frees)`, so an HTTP
   429 still gets a correct Retry-After. §4's `Keyed.Run(ctx)` idle sweep is dropped: eviction happens on each
   call from the clock reading (no goroutine), as the brief asks.
8. `Keyed.Len()` added (observability for tests and metrics; evicts idle keys first).

## Per export

### `TB-RATE-003` `Limiter` (with TB-RATE-004 `Reservation`)
- **Pattern**: token bucket rate limiter. **Authority**: x/time/rate `Limiter`, `AllowN`, `ReserveN`, `WaitN`;
  RFC 2697 §3.
- **Familiar shape**: x/time/rate method set (deviations 1–6).
- **Goroutines**: none. WaitN blocks on one clock timer on the caller's goroutine; the timer is stopped on return.
- **Cancellation**: WaitN: ctx done before the call → `context.Cause(ctx)`, nothing taken; during the wait → the
  timer is stopped, the reservation is cancelled, `context.Cause(ctx)`; n > burst is checked first (as
  x/time/rate). Nil ctx is Background.
- **Errors**: ErrLimit, ErrBurst (wrapped). Never panics on input: n <= 0 allowed and takes nothing; zero value
  refuses n >= 1 (burst 0) and WaitN returns ErrBurst.
- **Blocking**: only WaitN. **Ordering**: decisions in mutex order; reservations queue in debt in call order.
- **Zero value**: refuses every event; create with NewLimiter. Not copied after first use.
- **Clock**: `WithClock`. **calcbench**: ALG-RATELM-001, ALG-RATELM-005 (Cancel). **Compared**: x/time/rate, `compare/ratelimit/`.
- **Worked example**: `ExampleLimiter` → `true true true false` / `true`; `ExampleLimiter_Reserve` → delays
  `0s 1s 2s`; `ExampleReservation_Cancel` → `1s` / `1s`.

### `TB-RATE-005` `Keyed`
- **Pattern**: per-client token bucket in a bounded table with idle eviction. **Authority**: x/time/rate `Limiter`
  (one per client); Azure "Rate Limiting pattern" (429 + Retry-After); RFC 2697 §3.
- **Goroutines**: none; eviction runs inside each call (amortised O(1): each key is removed once).
- **Errors**: ErrBurst (n > burst, retryAfter InfDuration), ErrKeys (retryAfter = until the least recently seen
  key idles). n <= 0: `(true, 0, nil)` and no key is added.
- **Blocking**: never; one mutex for O(1) work. **Table**: ALG-LIST-015 LRUCache + ALG-LIST-018 Oldest. A refused call still counts as "seen" (an active key is never
  evicted). With the default TTL eviction is lossless (`TestKeyed_defaultEvictionIsLossless`).
- **Zero value**: refuses (ErrBurst); create with NewKeyed. **calcbench**: ALG-RATELM-001, ALG-LIST-015,
  ALG-LIST-018, NUM-INTEG-013.
- **Compared**: none beyond the Limiter (x/time/rate has no keyed limiter).
- **Worked example**: `ExampleKeyed` → `true 0s` / `false 1s` / `true 0s`.

### `TB-RATE-001` `Limit`, `TB-RATE-002` `Every`, options, sentinels, constants
Pure values; examples `ExampleLimit`, `ExampleEvery`, `ExampleWithClock`, `ExampleWithMaxKeys`,
`ExampleWithIdleTTL`, `ExampleOption`, `ExampleErrLimit`, `ExampleErrBurst`, `ExampleErrKeys`,
`ExampleInfDuration`, `ExampleDefaultMaxKeys`, `ExampleNewLimiter`.

## Tests (run in review: -race, -count=50, green)

`ratelimit/limiter_test.go`, `keyed_test.go`, `example_test.go`, `main_test.go` (`leak.Main`; `leak.Verify` in
each goroutine-starting test; 10 s bound; clock.Fake only, no Sleep, no polling): constructor errors (both
types); Allow/AllowN refill exactly per the clock (RFC 2697 script); burst then rate; n <= 0 and n > burst;
no drift (7 per 3 ns, 10 000 steps); Reserve delays and debt; DelayFrom; not-OK reservation; Cancel (restores;
immediate; after act; after a later take; twice); Wait blocks until the fake clock advances (BlockUntil, 499 ms
no return, 1 ms more returns); Wait ctx cancel returns the cause and gives the tokens back; done ctx; WaitN n >
burst before ctx; zero values; 64 goroutines at one instant → exactly burst (atomic); no allocation in Allow;
Keyed per-key buckets, AllowN, idle eviction on the clock (default and WithIdleTTL), lossless default eviction
(Keyed vs Limiter step by step), WithMaxKeys → ErrKeys with retry-after then admission after idling; go_77 and
go_82 defects (above); benchmarks with `b.ReportAllocs()`. Every test is a fixed script: deterministic under
`-count=200`.

`compare/ratelimit/ratelimit_test.go` vs golang.org/x/time/rate: the same scripted AllowN / ReserveN /
DelayFrom / Cancel at the same instants (x/time/rate driven with explicit times, ours with clock.Fake), rates 1,
2, 4 per second so x/time/rate's floats are exact; Cancel of an earlier reservation in debt; WaitN n > burst;
Wait with a done ctx.

## Out of scope

SetLimit/SetBurst (and their `At` forms), `Tokens()`, `Inf`; LeakyBucket/FixedWindow/SlidingWindow options
(ALG-RATELM-002..004; a later additive `With…` if asked); HTTP 429 middleware and client-IP keying (`httpx`
RateLimit, LADDER #47); gRPC interceptor (`grpcx`); distributed limits (an adapter over Redis).

## For the lead

- INDEX.md: 16 rows TB-RATE-001..016 (doc.go lists them; 015 NewLimiter and 016 NewKeyed added in review).
- compare/go.mod: add `golang.org/x/time` (written against v0.16.0); `go mod tidy` in compare/.
- doc/DECISIONS.md: deviations 1 (integer Limit), 2 (constructors return error), 4 (Cancel after a later take),
  6 (no deadline pre-check), 7 (Keyed.AllowN with err; no Keyed.Run).
- doc/sources: x/time/rate (pkg.go.dev + rate.go source) → TB-RATE-001..005, 011, 014; RFC 2697 §3 → 001, 003,
  005; Azure "Rate Limiting pattern" → 005.
- calcbench requests: (R-a) `TokenBucket.Restore(now, n)` or a debt-capable `Reserve(now, n) (timeToAct)` so
  Cancel can give back tokens after a later take as x/time/rate does; (R-b) `LRUCache.Oldest() (K, V, bool)`
  (peek the least recently used entry without evicting) so Keyed can use ALG-LIST-015 for its bound.

## Bug fixed in review (reviewer BLOCKER, 2026-11-13)

- **Bug**: `reserve` read "in debt" as `st.last.After(now)`. `now` is read before the mutex, so two goroutines'
  readings can reach the lock out of order (G1 reads t+1ns, G2 reads t, G1 locks first); G2 was then refused
  (AllowN false, Reserve delayed 1 ns, Wait blocked) with 9 of 10 tokens left. x/time/rate allows it: spurious
  429s under load.
- **Reproduced**: `TestLimiter_outOfOrderClockReadings` (`ratelimit/clock_order_test.go`, a test Clock that
  returns scripted readings, no Sleep): AllowN, ReserveN, WaitN subtests failed before the fix.
- **Fix** (`limiter.go:166`): debt is a recorded instant `state.debt`, set only when a reservation's time to
  act is after its reading; only `st.debt.After(now)` refuses. A reading earlier than `st.last` without debt
  is decided at `st.last` (no time passed, the meter's own rule). Cancel restores `debt` with the snapshot.
  Keyed (`keyed.go`) never inferred debt (the meter decides an earlier reading as no time passed:
  `TestKeyed_outOfOrderClockReadings` passed before and after); it now also reads Now under its mutex and never
  moves a key's `seen` back.
- **Verified**: `go test -race -timeout 5m -count=50 ./ratelimit/` → `ok github.com/CrimsonCoder42/toolbench/ratelimit 2.824s`.

REMAINING: compare/ratelimit not run here (needs `golang.org/x/time` in compare/go.mod, the lead's).

## Wave 6: calcbench v0.3.0 switch (2026-11-13, builder B0)

- Released API unchanged: TB-RATE-001..016 keep name, parameters and results; no export added.
- `Reservation.Cancel` (`ratelimit/reservation.go`) now follows x/time/rate CancelAt (source read in the module
  cache, golang.org/x/time@v0.16.0 rate/rate.go:169-204): restore `n − tokens later`, nothing after the time to
  act, cap at burst, `lastEvent` moves back when the latest take is cancelled. Math: ALG-RATELM-005 (not in
  debt), ALG-RATELM-001 (in debt: rebuilt empty at the earlier instant; see section d). The out-of-order fix of
  wave 5 is kept: `state.debt` is still the only thing that refuses before its time, and Cancel moves it with
  the meter (`TestLimiter_outOfOrderClockReadings` unchanged).
- Reviewer NIT fixed: Cancel rebuilds the meter only when `st.debt` is after the reading AND `st.last` is not
  after `st.debt` (the meter is empty at the debt); a stale reading after a later call moved `st.last` past the
  debt now takes Restore instead of throwing tokens away (`TestReservation_Cancel_outOfOrderClockReading`,
  `ratelimit/clock_order_test.go`; pre-edit copy `_delete/ratelimit/clock_order_test.go.removed.go.txt`).
- `Keyed` (`ratelimit/keyed.go`): ALG-LIST-015 + ALG-LIST-018 replace the map + container/list.
- Tests added: `TestReservation_Cancel` "after a later smaller reservation gives back n minus its tokens", "at
  its time to act gives back by Restore"; `TestKeyed_NaNKeyRefused`; compare/ratelimit
  `TestCompare_CancelAfterLaterReservation`, `TestCompare_CancelAtTimeToAct`. Not run by the builder (the
  reviewer runs them once).
- Saved before editing (whole pre-wave-6 files, for the ledger): `_delete/ratelimit/limiter.go.removed.go.txt`,
  `reservation.go.removed.go.txt`, `keyed.go.removed.go.txt`, `limiter_test.go.removed.go.txt`,
  `keyed_test.go.removed.go.txt`; `_delete/compare/ratelimit/ratelimit_test.go.removed.go.txt`. No file moved.

### For the lead (wave 6)

- doc/DECISIONS.md: a follow-up to D-031 (and D-034): Cancel now matches CancelAt (D-031's "Cancel restores
  nothing once a later reservation has taken tokens" ends; remaining differences are deviation 4); the key
  table is ALG-LIST-015/018, not map + container/list; a NaN key is ErrKeys. Alternative that lost: Restore
  alone in debt too (adds at the future instant; exact only when 1 token goes back, so a later Allow would be
  refused up to k−1 token-times too long).
- INDEX.md: TB-RATE-003, -004 Math gain ALG-RATELM-005; TB-RATE-005 and -016 gain ALG-LIST-015 (005 also
  ALG-LIST-018). doc.go export lines unchanged.
- calcbench requests R-4 and R-5 (wave 5 "R-a", "R-b") are answered and used. Optional new request: a meter
  that can be moved back in debt (or a debt-capable Reserve) would let Cancel in debt use one call instead of
  rebuilding; not needed for exactness.
