# breaker: a circuit breaker (Closed, Open, HalfOpen) around a call

Status: SHIPPED (2026-11-13)
Package: `breaker` · SHORT: `BRK` · Layer: core · Tier: T3
Work folder: `doc/wave5_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #27
Builds on: `clock` (Clock.Now; tests with Fake), `panics` (Try, ErrPanic) · calcbench: none (see d) ·
Decisions: see "For the lead" · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; a reading list, never cited)

No code-heim repo shows this pattern (BRIEF B4). There are therefore no repo defects to turn into tests; the
defects tested are the ones the authorities name (calls on an open breaker, a flood of trial calls on a
recovering service, a breaker that cannot be observed).

### b. The authority

- Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns", "Circuit Breaker".
  A BOOK, NOT OPENED: cited as the origin of the pattern (closed/open/half-open, fail fast while open, a trial
  call after a timeout).
- Fowler, "CircuitBreaker", martinfowler.com (bliki), 2014. Opened: https://martinfowler.com/bliki/CircuitBreaker.html,
  2026-11-13. Relied on: "Once the failures reach a certain threshold, the circuit breaker trips, and all further
  calls to the circuit breaker return with an error, without the protected call being made at all" (ErrOpen,
  fn not called); the example's `@failure_threshold = 5` (our default threshold); "half open - meaning the
  circuit is ready to make a real call as trial to see if the problem is fixed"; "Not all errors should trip the
  circuit, some should reflect normal failures" (context errors of the caller do not count); "Any change in
  breaker state should be logged and breakers should reveal details of their state for deeper monitoring"
  (WithOnStateChange, State).
- Microsoft, "Circuit Breaker pattern", Azure Architecture Center (ms.date 2025-02-05). Opened:
  https://learn.microsoft.com/azure/architecture/patterns/circuit-breaker, 2026-11-13. Relied on, section
  "Solution": Closed "maintains a count of the number of recent failures ... If the number of recent failures
  exceeds a specified threshold within a given time period, the proxy is placed into the Open state and starts
  a time-out timer. When the timer expires, the proxy is placed into the Half-Open state"; Open "fails
  immediately"; Half-Open "A limited number of requests from the application are allowed to pass through ...
  If these requests are successful ... switches to the Closed state. The failure counter is reset. If any
  request fails ... it reverts to the Open state. It restarts the time-out timer"; "The circuit breaker reverts
  to the Closed state after a specified number of successful, consecutive operation invocations". Section
  "Problems and considerations": "Concurrency: ... The implementation shouldn't block concurrent requests or
  add excessive overhead" (no lock held while fn or the hook runs); "Types of exceptions" (the failure
  definition is a decision); "Manual override" (OUT OF SCOPE, below).

### c. Today's Go

Go 1.27 has no circuit breaker in the standard library or golang.org/x. `x/time/rate` limits a rate, not
failures; `context` cancels but does not fail fast on a known-bad dependency. Hence the package. The
best-known Go library is github.com/sony/gobreaker (v2 generic `CircuitBreaker[T]`, `Execute`); it has no
ctx, no clock seam (time.Now), re-panics, and calls OnStateChange while holding its mutex (a hook that calls
the breaker deadlocks). Ours takes ctx, a clock, recovers panics, and calls the hook outside the lock.

### d. calcbench

**None called.** LADDER §5 lists ALG-STREAM-002 HitCounter (confirmed in calcbench INDEX.md, `algo/stream`)
"only if the failure window is time-based". The threshold model decided here is consecutive failures
(below), which is a single counter, not mathematics: no calcbench call, no `Math:` line. A windowed failure
ratio (Azure's "within a given time period") is a later additive option `WithWindow` that WOULD call
ALG-STREAM-002 for calls and failures in the window; out of scope for this wave.

## Exports

| ID | Name | Signature | file:line |
|---|---|---|---|
| `TB-BRK-001` | `Breaker` | `type Breaker struct{ … }` with `Do(ctx, fn func(context.Context) error) error`, `State() State` | `breaker/breaker.go:25` (Do :105, State :151) |
| `TB-BRK-002` | `New` | `func New(opts ...Option) *Breaker` | `breaker/breaker.go:59` |
| `TB-BRK-003` | `DoValue` | `func DoValue[T any](ctx context.Context, b *Breaker, fn func(context.Context) (T, error)) (T, error)` | `breaker/breaker.go:139` |
| `TB-BRK-004` | `State` | `type State int` (`Closed`, `Open`, `HalfOpen`; `String`) | `breaker/state.go:12` |
| `TB-BRK-005` | `ErrOpen` | `var ErrOpen = errors.New("breaker: circuit open")` | `breaker/errors.go:13` |
| `TB-BRK-006` | `Option` | `type Option func(*Breaker)` | `breaker/option.go:23` |
| `TB-BRK-007` | `WithFailureThreshold` | `func WithFailureThreshold(n int) Option` | `breaker/option.go:32` |
| `TB-BRK-008` | `WithOpenFor` | `func WithOpenFor(d time.Duration) Option` | `breaker/option.go:43` |
| `TB-BRK-009` | `WithHalfOpenProbes` | `func WithHalfOpenProbes(n int) Option` | `breaker/option.go:54` |
| `TB-BRK-010` | `WithClock` | `func WithClock(c clock.Clock) Option` | `breaker/option.go:65` |
| `TB-BRK-011` | `WithOnStateChange` | `func WithOnStateChange(f func(from, to State)) Option` | `breaker/option.go:81` |

| Option | Default | Bound it sets |
|---|---|---|
| `WithFailureThreshold(n)` | 5 (also n <= 0) | consecutive failures in Closed that open the breaker |
| `WithOpenFor(d)` | 60 s (also d <= 0) | time Open before the next call becomes a probe |
| `WithHalfOpenProbes(n)` | 1 (also n <= 0) | probes running at once in HalfOpen = successes that close it |
| `WithClock(c)` | `clock.Real()` (also nil) | the time source of OpenFor (Now only; no timers) |
| `WithOnStateChange(f)` | nil: no hook | — |

Sentinel errors: `ErrOpen` (open, or half-open with all probe slots taken) · `panics.ErrPanic` (fn or the hook
panicked).

Deviations from LADDER.md §4: §4 listed `WithFailureRatio, WithWindow` (a windowed ratio on HitCounter); BRIEF
B4 (the agreed API for this wave) replaces them with `WithFailureThreshold(n)` (consecutive) and adds
`WithOnStateChange`, `DoValue` and `ErrOpen`. Built as BRIEF B4. `Option` is `func(*Breaker)` (no separate
config struct: the zero Breaker must work, so the settings live on it and "zero means default").

## Decisions (BRIEF B4 asks each to be stated)

1. **What counts as a failure**: any non-nil error from fn, and a panic in fn (returned as `*panics.PanicError`,
   `errors.Is(err, panics.ErrPanic)`; a nil fn panics when called, so it is a failure too). NOT counted, neither
   success nor failure: an error from fn that matches `ctx.Err()` or `context.Cause(ctx)` once the caller's ctx
   is done (the caller gave up; the dependency did not fail). A `context.DeadlineExceeded` that fn produced
   itself while the caller's ctx is alive IS a failure: a slow dependency is what a breaker exists for. A ctx
   already done before Do returns `context.Cause(ctx)` without calling fn or touching the breaker.
   Alternative that lost: gobreaker's "every error, IsSuccessful to override" (a cancelled request would trip
   the breaker during a shutdown). An `IsFailure` option is additive later if needed.
2. **Threshold model: consecutive failures** in Closed; a success resets the count; an uncounted (ctx) result
   leaves it. Alternatives that lost: a failure ratio over a time window (Azure's "within a given time period",
   LADDER §4) needs a minimum-calls rule and a window and calls ALG-STREAM-002; it is the later additive
   `WithWindow`. Consecutive is Fowler's model and gobreaker's default `ReadyToTrip`, deterministic to test.
3. **Half-open probes**: n probes (WithHalfOpenProbes) may be admitted; a slot is taken at admission and freed
   only by an uncounted (ctx) result, so at most n run at once and at most n succeed per half-open period
   (gobreaker's MaxRequests semantics). All n succeed → Closed (Azure: "a specified number of successful,
   consecutive operation invocations"). Any probe failure → Open, OpenFor restarts from the reopen. A call while
   every slot is taken → `ErrOpen` without calling fn.
4. **Open → HalfOpen is lazy**: no timer and no goroutine; the first Do at or after openedAt + OpenFor
   performs the transition (gobreaker does the same). `State()` reports HalfOpen once OpenFor has passed but
   does not transition or call the hook (State runs no caller code). Alternative that lost: a timer per open
   period (a goroutine or AfterFunc to stop on a Close the type does not have).
5. **Clock reads**: `Now` is read only under the breaker's lock (`openForPassed` and `setState`), so the
   opened-at time and every comparison against it are ordered by the lock: a call cannot judge OpenFor with a
   reading taken before another call reopened the breaker (the ratelimit blocker pattern does not occur).
6. **Stale results**: every transition bumps a generation; a call admitted in an older generation (a Closed
   call that finishes after the breaker opened, a probe from an earlier half-open period) is not counted.
7. **Hook placement**: transitions are queued under the lock and delivered by `notify` outside it, in order,
   one hook call at a time, on the goroutine of a Do call (usually the one that caused the transition; under
   contention the goroutine already delivering delivers it, possibly after the causing Do returned). The hook
   may call Do or State (no deadlock: notify finds delivery in progress and returns). The transition into
   HalfOpen is delivered before fn runs. A hook panic is recovered and joined (`errors.Join`) to the result of
   the Do that ran it; delivery continues. **The hook must not block** (documented on WithOnStateChange): the
   Do delivering it does not return while it runs, and the queue is NOT bounded by transitions per Do (each Do
   adds at most 2, but other goroutines keep calling). So the queue is capped at 64 undelivered transitions
   (`maxPending`, breaker/breaker.go); later ones are dropped and the hook sees the gap as a `from` that
   differs from the previous `to` (`TestWithOnStateChange_pendingBounded`). Alternative that lost: calling the hook under the lock (gobreaker; deadlocks a hook that
   calls the breaker, and a slow hook blocks every call, against Azure's "Concurrency").
8. **Zero value**: ready to use with the defaults (`var b breaker.Breaker`); every setting reads "zero means
   default". A nil `*Breaker` is always closed (Do runs fn with panic recovery and counts nothing; State is
   Closed): no panic on input (D-011). Must not be copied after first use (it holds a mutex; vet copylocks).
9. **Defaults**: threshold 5 (Fowler's example), OpenFor 60 s and 1 probe (gobreaker's Timeout and
   MaxRequests defaults). Invalid values are the default, never a panic.

## Per export

### `TB-BRK-001` `Breaker` (Do, State)
- **Pattern**: circuit breaker. **Authority**: Nygard ch.5 (not opened); Fowler 2014; Azure "Solution".
- **Familiar shape**: gobreaker `CircuitBreaker.Execute`/`State`, with ctx first (`Do(ctx, fn)`, the
  `errgroup.Go`/`singleflight.Do` naming) and `func(context.Context) error`.
- **Goroutines**: none. fn and the hook run on the caller's goroutine.
- **Cancellation**: before: `context.Cause(ctx)`, fn not called. During: fn sees ctx; its ctx error is not
  counted. After: nothing.
- **Errors**: `ErrOpen` (fn not called); fn's error unchanged; `ErrPanic`; hook panic joined.
- **Blocking**: only while fn and the hook run and on the short internal lock; never waits for the breaker.
- **Ordering and counts**: at most n probes in flight; each call counted at most once, only in its generation.
- **Zero value**: ready to use (above). **Clock**: WithClock (Now only).
- **Worked example**: `ExampleBreaker` (`open`, `breaker: circuit open`), `ExampleBreaker_Do`, `ExampleBreaker_State`.

### `TB-BRK-002` `New` — options applied in order, nil ignored, starts Closed. `ExampleNew`.
### `TB-BRK-003` `DoValue` — `Do` for `fn` returning `(T, error)`: fn's value and error as returned; T's zero and
`ErrOpen` when rejected; T's zero and ErrPanic on a panic. `ExampleDoValue` (`9.99 <nil>`).
### `TB-BRK-004` `State` — `Closed` (0, zero value), `Open`, `HalfOpen`; `String` "closed", "open",
"half-open" (gobreaker's strings), "State(n)" otherwise. `ExampleState`.
### `TB-BRK-005` `ErrOpen` — `ExampleErrOpen`.
### `TB-BRK-006..011` Options — functional options (Pike 2014); examples per option.

## Tests (`breaker/*_test.go`, all on `clock.Fake`; `leak.Main` in `main_test.go`, `leak.Verify` in every goroutine test)

closed → open at the threshold, success resets (`TestDo_closedToOpenAtThreshold`) · open rejects with ErrOpen
without calling fn · open → half-open exactly at OpenFor (1 ns before: still open) · probes all succeed →
closed · one fails → open, OpenFor restarts, earlier successes do not carry over · probes in flight beyond the
limit → ErrOpen, fn not called (`TestDo_probesInFlightBeyondLimit`) · context errors not counted (done before,
cancelled during, Cause, wrapped; fn's own DeadlineExceeded counts; a cancelled probe frees its slot) · ctx
result does not reset the failure count · panic counts and is ErrPanic (also nil fn) · nil ctx · stale result
ignored (`TestDo_staleResultIgnored`) · hook sees every transition in order, State() does not trigger it ·
hook panic joined · hook queue capped at 64 · hook re-entrant (calls State and Do) · DoValue (value, error, ErrOpen, nil b + panic) ·
zero value · nil *Breaker · invalid options are defaults · State.String · concurrency: 50 racing calls in
half-open with 3 probes → exactly 3 run (atomic) · 8×200 mixed calls with a moving clock → the hook chain is
consistent and never overlaps · benchmark `BenchmarkDo_closed` · Examples with `// Output:` for every export.
No sleeps, no polling; every test is independent and deterministic under `-count=200`.

## Compared against

github.com/sony/gobreaker/v2 (v2.4.0, pinned in compare/go.mod), `compare/breaker/breaker_test.go`: a scripted run of
results gives the same rejection and state after every call and the same transition sequence; with all probes
in flight both reject (ours ErrOpen, theirs ErrTooManyRequests). gobreaker has no clock seam: the compare test
waits on one real timer of OpenFor (1 s) + 1 ms per open period. Leaving Open is deterministic (expiry set
before the timer; gobreaker needs expiry strictly before now); staying Open assumes < 1 s of wall time between
a (re)open and the next checks. Accepted in compare/ only; stated in the file header.

## Out of scope

Windowed failure ratio (`WithWindow`, would call calcbench ALG-STREAM-002) · manual override (`Trip`/`Reset`,
Azure "Manual override") · increasing open time (Azure "Solution"; would call calcbench backoff) · a
failure-classifier option (`IsFailure`) · counts/metrics export · HTTP and gRPC middleware (`httpx`, `grpcx`)
· retry through a breaker (the caller composes `retry` with `Do`; retry should stop on ErrOpen).

## For the lead

- INDEX.md: 11 rows TB-BRK-001..011 (doc.go list). taxonomy: `breaker` status planned → built; its "Uses"
  column is `clock, panics` (no calcbench call this wave; LADDER §5's HitCounter applies only to the later
  WithWindow).
- compare/go.mod has `github.com/sony/gobreaker/v2 v2.4.0`; the compare test passes against it.
- DECISIONS candidates: failure definition (decision 1), consecutive threshold (2), lazy half-open and
  State not transitioning (4), hook outside the lock with ordered delivery and panics joined (6), zero value
  and nil *Breaker (7), defaults (8). doc/sources: Fowler bliki and Azure Circuit Breaker page used by TB-BRK-*;
  Nygard cited, not opened.

Verified 2026-11-13: `go test -race -timeout 5m -count=50 ./breaker/` ok; in compare/,
`go test -race -timeout 10m -count=3 ./breaker/` ok. REMAINING: none.
