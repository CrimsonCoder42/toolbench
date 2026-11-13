# clock: an injectable time source, a real clock and a fake clock for tests

Status: SHIPPED (2026-11-13)
Package: `clock` · SHORT: `CLOCK` · Layer: core · Tier: T0
Work folder: `doc/wave1_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #2
Builds on: nothing (standard library only; tests use `internal/leak`) · Decisions: D-006 · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim)
No code-heim repo shows this pattern (LADDER.md row 2: "standard"). Nothing read, nothing to cite, no repo
defects to turn into tests. The defects tests guard against come from the authority instead: stale timer
values after `Stop`/`Reset` (pre-go1.23 behaviour) and `time.Sleep` used to wait for timers.

### b. The authority (opened 2026-11-13)
- Go standard library, package `time`, `type Timer`, `NewTimer`, `AfterFunc`, `Timer.Stop`, `Timer.Reset`,
  `type Ticker`, `NewTicker`, `Ticker.Stop`, `Ticker.Reset`, `Since`.
  Opened: https://pkg.go.dev/time#Timer, 2026-11-13 (summary only; the page body was truncated by the fetcher),
  and the source doc comments, Opened: https://go.dev/src/time/sleep.go, 2026-11-13. Relied on:
  - `Timer.Stop`: "returns true if the call stops the timer, false if the timer has already expired or been
    stopped"; "any receive from t.C after Stop has returned is guaranteed to block rather than receive a stale
    time value from before the Stop"; for an AfterFunc timer "Stop does not wait for f to complete".
  - `Timer.Reset`: "returns true if the timer had been active"; "any receive from t.C after Reset has returned
    is guaranteed not to receive a time value corresponding to the previous timer settings".
  - `AfterFunc`: "calls f in its own goroutine"; the Timer it returns has a nil C.
  - `NewTicker`: "d must be greater than zero; if not, NewTicker will panic"; a ticker drops ticks for a slow
    receiver.
- Go 1.23 release notes, "Timer changes". Opened: https://go.dev/doc/go1.23#timer-changes, 2026-11-13. "the timer
  channel associated with a Timer or Ticker is now unbuffered, with capacity 0 ... no stale values prepared
  before that call will be sent or received after the call"; "len and cap of timer channels now returns 0".
  go1.27 removed `asynctimerchan` (GO_RELEASES.md), so synchronous channels are the only behaviour.
- Go standard library, package `testing/synctest`, "Time", "Blocking" (bubble, `Test`, `Wait`, `Sleep`).
  Opened: https://pkg.go.dev/testing/synctest, 2026-11-13.
- Meszaros, xUnit Test Patterns, 2007, "Test Double" / "Fake Object". xunitpatterns.com could NOT be fetched
  (connection refused); the definition was read in Fowler, "TestDouble", martinfowler.com, 2006 (opened
  https://martinfowler.com/bliki/TestDouble.html, 2026-11-13), which credits Meszaros: a Fake has "working
  implementations, but usually take some shortcut which makes them not suitable for production". The
  `Pattern:` line says "test double (fake object)" and the `Authority:` line cites the Fowler page.

How go1.23 behaviour is reproduced by the fake: the runtime keeps a one-slot buffer internally but reports
capacity 0, and `Stop`/`Reset` drain it and return true when they drained an unreceived value (the timer
counts as still active until its value is received). The fake does the same with a 1-slot channel that
`Stop`/`Reset` drain under the fake's mutex, so no stale value is ever received after they return and the
boolean results match the real timer. The one observable difference: `len`/`cap` of a fake timer channel are
up to 1, not 0 (the release note says such code should use a non-blocking receive anyway).

### c. Today's Go
`testing/synctest` (go1.25 GA, `Sleep` go1.27) gives tests a fake clock for code that calls `time` directly,
inside a bubble. Why toolbench still needs `clock`: (1) synctest only advances time when every goroutine in
the bubble is durably blocked, so code that talks to the network, takes a mutex for long or runs outside the
bubble (a server goroutine started before the test, an adapter, a gRPC stream) cannot use it; (2) toolbench
patterns (retry, ratelimit, breaker, cache, schedule) take `WithClock` per CLAUDE.md so a caller can pin time
in production too (replay, simulation, a monotonic test of TTLs); (3) `Advance` gives explicit, step-by-step
control ("after 3 s exactly one retry happened") that `synctest.Wait` does not. Both are documented: inside a
bubble, `Real()` works with synctest because it only calls `time`.

### d. calcbench
None. The fake keeps its armed timers in a slice and picks the earliest (deadline, then arming order) with a
linear scan under its mutex; a test has a handful of timers, so a heap (ALG-HEAP-*) buys nothing.

### e. Exports

| ID | Name | Signature |
|---|---|---|
| `TB-CLOCK-001` | `Clock` | `type Clock interface{ Now() time.Time; NewTimer(d time.Duration) Timer; AfterFunc(d time.Duration, f func()) Timer; NewTicker(d time.Duration) Ticker }` |
| `TB-CLOCK-002` | `Timer` | `type Timer interface{ C() <-chan time.Time; Stop() bool; Reset(d time.Duration) bool }` |
| `TB-CLOCK-003` | `Ticker` | `type Ticker interface{ C() <-chan time.Time; Stop(); Reset(d time.Duration) }` |
| `TB-CLOCK-004` | `Real` | `func Real() Clock` |
| `TB-CLOCK-005` | `Fake` | `type Fake struct{...}`; `NewFake(start time.Time) *Fake`; methods `Now`, `NewTimer`, `AfterFunc`, `NewTicker`, `Advance(d)`, `AdvanceContext(ctx, d) error`, `BlockUntil(ctx, n) error` |

No options, no sentinel errors (`BlockUntil` returns `context.Cause(ctx)`).

**Deviation from LADDER.md §4: `NewTicker` is in the interface now.** schedule (`Every`, `Heartbeat`), the
red/advo janitors and ratelimit sweeps all need a ticker. Adding a method to an exported interface later
breaks every implementation outside toolbench, and the public API is additive only, so it goes in now.
A ticker built from `Timer.Reset` in a loop drifts and loses the stdlib's drop-on-slow-receiver behaviour.
**`Since` / `Until` are NOT added**: they are `c.Now().Sub(t)` / `t.Sub(c.Now())`, and every method on the
interface is one more thing every fake must implement (the interface stays small, 4 methods).

**Addition to LADDER.md §4: `Fake.AdvanceContext(ctx, d) error`** (reviewer finding 1). `Advance(d)` takes no ctx
and waits for each AfterFunc function, which breaks go-concurrency rule 1 (context first on anything that
blocks) and deadlocks in two cases: a function that blocks on something the caller provides only after
Advance returns (`f.AfterFunc(1s, func(){ ch <- 1 }); f.Advance(1s); <-ch`), and a function that waits on a
fake timer due later in the same window. `Advance` stays (the familiar benbjohnson `Add` / clockwork `Advance`
shape) as `AdvanceContext(context.Background(), d)`, with both deadlocks stated in its doc comment and in
`Fake`'s. Alternative that lost: not waiting for callbacks (benbjohnson's `go fn()`), which makes callback order
and "Advance returned, so the callback ran" non-deterministic.

**Deviation: non-positive ticker period.** `time.NewTicker(d<=0)` panics; CLAUDE.md forbids a library panic on
input. Here `NewTicker(d<=0)` returns a stopped ticker (no ticks until `Reset` with d > 0) and
`Ticker.Reset(d<=0)` stops it. Decision for Devin (see "For the lead").

#### `TB-CLOCK-001` `Clock`
- **Pattern**: test double seam (dependency injection of the time source).
- **Familiar shape**: `time.Now`, `time.NewTimer`, `time.AfterFunc`, `time.NewTicker`; `C` is a method (an
  interface cannot hold a field), the same choice as github.com/jonboulle/clockwork (`Chan()`).
- **Zero value**: an interface; nil is not a clock. Patterns default a nil clock to `Real()`.

#### `TB-CLOCK-002` `Timer`, `TB-CLOCK-003` `Ticker`
- Semantics exactly as `*time.Timer` / `*time.Ticker` in go1.23+, with the non-positive-ticker deviation.
- `C()` of an AfterFunc timer is nil (as `time.AfterFunc(...).C`).

#### `TB-CLOCK-004` `Real`
- **Goroutines**: none of its own. `AfterFunc` callbacks run on the runtime's goroutine, as `time.AfterFunc`.
- **Errors**: none; a panic in an `AfterFunc` callback is not recovered (same as `time.AfterFunc`, the process
  dies); patterns that run caller code on a timer recover it themselves.
- **Blocking**: none. Returns the same zero-size value every call; safe for concurrent use.

#### `TB-CLOCK-005` `Fake`
- **Pattern**: test double (fake object), Meszaros 2007.
- **Time**: never moves on its own. Nothing fires outside `Advance`: a timer armed with d <= 0 fires on the next
  `Advance` (including `Advance(0)`).
- **Advance(d)**: moves time forward by d (d < 0 is treated as 0). Every armed timer and ticker whose deadline is
  <= the new time fires in deadline order (ties: arming order); `Now()` reads that deadline while it fires.
  A channel timer's value is its deadline. A ticker fires once per elapsed period; while its channel slot is
  full the extra ticks are dropped (as `time.Ticker`), skipping ahead in O(1). A timer armed by a callback
  inside the window fires in the same `Advance`.
- **Goroutines**: `Advance` starts one goroutine per AfterFunc callback it fires and waits for it to return
  before firing the next timer, so every goroutine has exited when `Advance` returns. A panic in a callback is
  recovered on that goroutine with `debug.Stack()` and re-panicked on the goroutine that called `Advance` with
  an error whose message holds the value and the callback's stack (`errors.Unwrap` gives the value when it is
  an error); not a crash from a stray goroutine. After a panic `Now()` is the callback's deadline, later due
  timers have not fired, and the fake is usable. A callback must not call `Advance` (it would wait for
  itself); it may call any other method. (A local type, not `panics.PanicError`: wave 1 packages import no
  other toolbench package.)
- **AdvanceContext(ctx, d)**: as Advance, returning `context.Cause(ctx)` when ctx is done before it starts or
  while another Advance runs (time does not move), or while an AfterFunc function still runs: then `Now()` is
  that function's deadline, the rest of the window is not advanced, the function keeps running, and the next
  AdvanceContext waits for it first (re-panicking its panic). A nil ctx is `context.Background()`.
- **BlockUntil(ctx, n)**: returns nil as soon as at least n timers and tickers are armed (counted, not
  stopped, not fired), or `context.Cause(ctx)` when ctx is done first (a nil ctx is `context.Background()`). Waits on a channel the fake closes when
  the count changes; never polls, never sleeps. Lets a test wait for "the code under test has armed its
  timer" before calling `Advance`.
- **Concurrency**: one mutex guards `now`, the armed list and the waiters; a one-slot channel semaphore
  serialises `AdvanceContext` calls (so a waiting caller can give up on ctx).
  Safe for concurrent use; channel sends and drains happen under the mutex with non-blocking operations, so no
  method blocks except `Advance` (on callbacks) and `BlockUntil`.
- **Zero value**: ready to use, at the zero `time.Time`. `NewFake(start)` sets the start. Not copied after use.

### g. Comparison
`compare/clock/clock_test.go` against github.com/benbjohnson/clock: the same channel timers (durations 5, 1, 3,
3, 10 s) and a ticker (2 s) on `Mock.Add(6s)` vs `Fake.Advance(6s)`; both fire the same timers with the same
timestamps (the deadline) in the same order (timestamp, ties in arming order), and the 10 s timer fires in
neither. `TestFake_StopNoStaleValue` asserts the known difference: benbjohnson keeps a stale value after
`Stop` (pre-go1.23), toolbench does not. AfterFunc order is not compared: benbjohnson starts each callback
with `go` and does not wait, so its order is not deterministic.

### h. Out of scope
`Sleep`, `After`, `Tick`, `Since`, `Until`, `WithTimeout`/`WithDeadline` on the clock (callers write
`c.NewTimer` + select, `c.Now().Sub`; a context deadline from a fake clock belongs to a later package if ever
needed). Monotonic-clock readings, time zones, `Set` (jumping backwards).

## Tests
`clock/real_test.go`, `clock/fake_test.go`: Examples with `// Output:` (Real, Fake, BlockUntil), deadline order
on Advance, ties in arming order, Stop/Reset results and no stale value (the go1.23 rules), Reset of a fired
but unreceived timer, AfterFunc waited for and on its own goroutine, callback re-arming inside the window,
callback panic re-panicked with its stack and the documented state, AdvanceContext returning on ctx for both
deadlock cases and on a ctx cancelled before, nil ctx, ticker drops and skips, non-positive ticker, BlockUntil satisfied/cancelled/
already satisfied, concurrent use under -race, zero-value Fake, `TestMain` with `leak.Main`.

## For the lead
- D-needed: non-positive ticker period returns a stopped ticker instead of panicking (alternative: mirror
  `time.NewTicker` and panic; lost because CLAUDE.md "a library function never panics on input").
- D-needed: a panic in a Fake AfterFunc function is re-panicked in Advance (alternative: AdvanceContext returns
  it as an error; lost because Advance has no error result and a test failure with the stack is what a test
  wants).
- DECISIONS: `Fake.AdvanceContext` added beside `Advance` (reviewer finding 1, argued in §e).
- D-needed (or ASSUMPTIONS): `NewTicker` added to `Clock` now; `Since`/`Until` left out (argued above).
- `doc/sources/`: go-time (pkg.go.dev time, go.dev/src/time/sleep.go), go1.23 release notes, testing/synctest,
  Fowler TestDouble (Meszaros) → TB-CLOCK-001..005.
- compare/go.mod needs `github.com/benbjohnson/clock` (latest v1.3.5).

REMAINING: none.
