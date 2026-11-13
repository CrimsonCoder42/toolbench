# retry: call again with capped exponential backoff and jitter, bounded attempts, per-attempt timeout

Status: SHIPPED (2026-11-13)
Package: `retry` · SHORT: `RETRY` · Layer: core · Tier: T3
Work folder: `doc/wave5_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #24, with #29 (timeout) folded
in as `WithAttemptTimeout` (LADDER §9 item 11)
Builds on: `clock` (Clock, Timer, AfterFunc; tests with Fake), `panics` (Try, TryValue, ErrPanic), calcbench v0.2.0
`algo/backoff` (ALG-BACKOF-001..004) · Decisions: see "For the lead" · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; a reading list, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `go_37_background_work/simple_jobs/process_jobs/worker.go:32` (also `middleware_and_context/…:67,69`, `checkin/…:67,69`) | gocraft/work `JobOptions{Priority: 10, MaxFails: 1}`. | The retry bound is a hidden struct field and `MaxFails: 1` means a failed job is never retried; no backoff is visible; gocraft/work unmaintained since 2019 (LADDER §7). Test `TestDo_go37MaxFailsOneCallsOnce` (WithAttempts(1) is exactly one call, ErrExhausted says so). |
| `go_68_asynq/producer/main.go:32,42`, `tasks/email.go:36` (also `image_processor.go:28`, `report.go:29`) | asynq `MaxRetry(10)`, `Timeout(3*time.Minute)`; handlers return `fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)`. | The non-retryable marker is wrapped with `%w` but the real cause with `%v`: `errors.Is/As` lose the cause. Test `TestDo_go68SkipRetryWrappedKeepsCause` (Permanent wrapped inside another error still stops at once and `errors.As` finds the cause). MaxRetry + Timeout map to WithAttempts + WithAttemptTimeout. |
| `go_78_resty/client/main.go:24-30` and the other calls | resty client; only the transport `err` is checked, the response (status) ignored; no retry configured, `log.Fatalf` on error. | A 4xx/5xx is never classified: nothing decides what is retryable. Test `TestDo_go78NonRetryableStopsAtOnce` (WithRetryIf rejects a "status 400" error: one call, error unchanged; a retryable one is retried). |

### b. The authority

- Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015-03-04. Opened:
  https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/, 2026-11-13. Sections "Adding Backoff",
  "Adding Jitter". Relied on: the three jittered schedules (Full: random in [0, min(cap, base·2^attempt));
  Equal: half kept plus random half; Decorrelated: from the previous sleep, up to 3×); the finding that Full Jitter
  "uses less work, but slightly more time" than Decorrelated and Equal Jitter is worst → our default is FullJitter.
- Azure Architecture Center, "Retry pattern", Microsoft Learn (ms.date 2024-07-18). Opened:
  https://learn.microsoft.com/azure/architecture/patterns/retry, 2026-11-13. Relied on: "Retry strategies"
  (**Cancel** a non-transient fault → `Permanent`/`WithRetryIf`; retry immediately; retry after delay "until some
  maximum number of requests have been attempted" → bounded `WithAttempts`); "Solution" ("unsuccessful after a
  predefined number of attempts … treat the fault as an exception" → `ErrExhausted`); "Impact on performance"
  (interactive callers: few retries, short delays → small default of 3); "Idempotency" (stated in the package doc:
  only retry idempotent work); "Exception type"; "General guidance" (do not stack retry layers; "Log all
  connectivity failures that cause a retry" → `WithOnRetry`, no logging inside the package).
- gRPC, "GRPC Connection Backoff Protocol", grpc/grpc `doc/connection-backoff.md` (master). Opened:
  https://github.com/grpc/grpc/blob/master/doc/connection-backoff.md, 2026-11-13. Section "Proposed Backoff
  Algorithm": `INITIAL_BACKOFF` 1 s, `MULTIPLIER` 1.6, `JITTER` 0.2, `MAX_BACKOFF` 120 s, `MIN_CONNECT_TIMEOUT` 20 s;
  `current_backoff = Min(current_backoff * MULTIPLIER, MAX_BACKOFF)`. Relied on: a capped exponential with a
  per-connect-attempt deadline (our WithAttemptTimeout). We keep calcbench's multiplier 2 (Brooker); the gRPC
  multiplier 1.6 and ±20 % jitter are not offered (calcbench has no such function; not requested, see "For the lead").
- Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns" ("Timeouts", and the warning
  that retries amplify load). Book, not online: cited, NOT opened.
- pkg.go.dev `context`, "func WithTimeoutCause", "func WithCancelCause", "func Cause" (go1.21+): the attempt ctx.

### c. Today's Go

Go 1.27 (go.mod). Neither the standard library nor `golang.org/x` has a retry helper; `x/time/rate` limits rate,
not failure. `context.WithTimeoutCause` (go1.21) gives the per-attempt timeout its named cause; `math/rand/v2`
gives a seedable `*rand.Rand` (PCG) for repeatable jitter; go1.23 timer semantics come through `clock`. The export
is still needed: every project hand-rolls a loop (LADDER §2: advo medlineplus/worker.go:259, red reader/link.go:75).

### d. calcbench (confirmed in calcbench INDEX.md and the reply `calcbench/doc/requests/toolbench_reply_2026-11-13.md`; v0.2.0 pinned)

- ALG-BACKOF-001 `backoff.Exponential(attempt, base, max)` → `Exponential` Backoff.
- ALG-BACKOF-002 `backoff.FullJitter(attempt, base, max, u)` → `FullJitter` (default).
- ALG-BACKOF-003 `backoff.EqualJitter(attempt, base, max, u)` → `EqualJitter`.
- ALG-BACKOF-004 `backoff.DecorrelatedJitter(prev, base, max, u)` → `DecorrelatedJitter` (prev = the previous wait
  of the same Do call, base before the first retry).
- Errors ErrAttempt/ErrBase/ErrMax/ErrDraw/ErrPrev cannot occur: the Backoff is normalised (base > 0, max ≥ base),
  attempt ≥ 0, u from `Float64` ∈ [0, 1), prev ≥ base (DecorrelatedJitter returns ≥ base). If one ever occurs, Do
  returns it unchanged (`retry/retry.go:97-100`), never a panic. Nothing is re-implemented here; the draw u comes
  from `WithRand` (calcbench functions draw nothing).

## Exports

| ID | Name | Signature | file:line |
|---|---|---|---|
| `TB-RETRY-001` | `Do` | `func Do(ctx context.Context, fn func(context.Context) error, opts ...Option) error` | retry/retry.go:37 |
| `TB-RETRY-002` | `DoValue` | `func DoValue[T any](ctx context.Context, fn func(context.Context) (T, error), opts ...Option) (T, error)` | retry/retry.go:59 |
| `TB-RETRY-003` | `Permanent` | `func Permanent(err error) error` | retry/permanent.go:14 |
| `TB-RETRY-004` | `ErrExhausted` | `var ErrExhausted error` | retry/retry.go:19 |
| `TB-RETRY-005` | `ErrAttemptTimeout` | `var ErrAttemptTimeout error` | retry/attempt_timeout.go:18 |
| `TB-RETRY-006` | `Backoff` | `type Backoff struct{ /* kind, base, max */ }` | retry/backoff.go:34 |
| `TB-RETRY-007` | `Exponential` | `func Exponential(base, max time.Duration) Backoff` | retry/backoff.go:64 |
| `TB-RETRY-008` | `FullJitter` | `func FullJitter(base, max time.Duration) Backoff` | retry/backoff.go:74 |
| `TB-RETRY-009` | `EqualJitter` | `func EqualJitter(base, max time.Duration) Backoff` | retry/backoff.go:84 |
| `TB-RETRY-010` | `DecorrelatedJitter` | `func DecorrelatedJitter(base, max time.Duration) Backoff` | retry/backoff.go:94 |
| `TB-RETRY-011` | `Option` | `type Option func(*config)` | retry/option.go:22 |
| `TB-RETRY-012` | `WithAttempts` | `func WithAttempts(n int) Option` | retry/option.go:83 |
| `TB-RETRY-013` | `WithBackoff` | `func WithBackoff(b Backoff) Option` | retry/option.go:93 |
| `TB-RETRY-014` | `WithAttemptTimeout` | `func WithAttemptTimeout(d time.Duration) Option` | retry/option.go:110 |
| `TB-RETRY-015` | `WithRetryIf` | `func WithRetryIf(f func(error) bool) Option` | retry/option.go:124 |
| `TB-RETRY-016` | `WithOnRetry` | `func WithOnRetry(f func(attempt int, err error, wait time.Duration)) Option` | retry/option.go:136 |
| `TB-RETRY-017` | `WithClock` | `func WithClock(c clock.Clock) Option` | retry/option.go:147 |
| `TB-RETRY-018` | `WithRand` | `func WithRand(r *rand.Rand) Option` | retry/option.go:159 |

Options:

| Option | Default | Bound it sets |
|---|---|---|
| `WithAttempts(n)` | 3 (n ≤ 0 → 3) | total calls of fn, first included; no unbounded setting |
| `WithBackoff(b)` | `FullJitter(100ms, 10s)` (also the zero Backoff) | every wait ≤ max |
| `WithAttemptTimeout(d)` | none (d ≤ 0 → none) | each attempt's ctx ends after d with cause ErrAttemptTimeout |
| `WithRetryIf(f)` | all errors except ErrPanic, context.Canceled, context.DeadlineExceeded; an attempt timeout IS retried | which errors are retried |
| `WithOnRetry(f)` | none | — (observability hook, caller's goroutine) |
| `WithClock(c)` | `clock.Real()` | — |
| `WithRand(r)` | top-level math/rand/v2 | — |

Backoff normalisation (no panic, no error on input): base ≤ 0 → 100 ms; max ≤ 0 → max(10 s, base); 0 < max < base →
base (so `Exponential(d, d)` is a constant d). Waits ≤ 0 (FullJitter can draw 0) arm no timer.

## Per export

### `TB-RETRY-001` `Do` / `TB-RETRY-002` `DoValue`
- **Pattern**: retry with capped exponential backoff and jitter (Azure "Retry pattern", Brooker 2015).
- **Familiar shape**: cenkalti/backoff/v4 `Retry(op, b)` and avast/retry-go `Do(fn, opts...)`; ctx first and
  functional options as toolbench standard. `DoValue` mirrors backoff/v5 `Retry[T]` (generic value).
- **Goroutines**: none; fn runs on the caller's goroutine. With WithAttemptTimeout and a clock other than
  clock.Real, the clock's AfterFunc runs the cancel (clock.Fake: on its own goroutine during Advance, waited for by
  Advance); the timer is stopped when the attempt ends.
- **Order of decisions after an attempt** (`retry/retry.go:72-110`): nil → return the value; outer ctx done → stop;
  Permanent anywhere in the chain → return fn's error unchanged; WithRetryIf false → return unchanged;
  attempts used up → `ErrExhausted` wrapping the last error (`"retry: attempts exhausted after N attempts: <err>"`);
  else compute the wait (calcbench), call WithOnRetry(attempt, err, wait), wait.
- **Cancellation**: ctx done before the first attempt → no call, `context.Cause(ctx)`. During a wait → returns at
  once (timer stopped). During fn → fn sees it; Do returns the cause and makes no further attempt. In every case the
  cause is wrapped with the last attempt's error when that error is not ctx's own (`stopped`, `retry.go:145`), so
  errors.Is matches both and nothing is lost silently (rule 7).
- **Attempt timeout vs outer ctx**: the attempt ctx is a child of ctx. An error after the attempt ctx timed out (and
  ctx is not done) is wrapped `"<ErrAttemptTimeout after d>: <fn's error>"`; it matches ErrAttemptTimeout and fn's
  error (e.g. context.Canceled), and the default WithRetryIf retries it (checked before the context-error rule,
  `option.go:64`). If the outer ctx ends, the outer cause wins (`TestWithAttemptTimeout_outerCtxWins`).
- **Errors**: panics in fn → `*panics.PanicError` (ErrPanic), not retried by default (WithRetryIf may retry it).
  A panic in the WithRetryIf or WithOnRetry function → ErrPanic, Do stops. A nil fn is a panic in fn → ErrPanic.
- **Blocking**: while fn runs and during each wait (one clock timer per wait). fn must honour ctx: Do never
  abandons a running fn (that would leak it, the go_51 defect).
- **Zero value**: functions; no state between calls. A nil ctx is context.Background().
- **Clock**: WithClock for waits and attempt timeouts.
- **calcbench IDs**: ALG-BACKOF-001..004 (`Math:` line).
- **Compared against**: github.com/cenkalti/backoff/v4, `compare/retry/retry_test.go` (attempt counts, Permanent,
  cancel inside attempt k, cancel before start; never timings).
- **Worked example**: `ExampleDo` (3 calls, Exponential 100 ms → fake elapsed 300ms), `ExampleDoValue`.

### `TB-RETRY-003` `Permanent`
Transparent marker: `Error()` is err's message, `Unwrap` is err; detected with `errors.As` anywhere in fn's error
chain (so `fmt.Errorf("…: %w", Permanent(e))` works, the go_68 lesson). Do returns fn's error unchanged (cenkalti
returns the inner error; same message, errors.Is/As identical; recorded as a difference). `Permanent(nil)` is nil.
WithRetryIf is not consulted for it.

### `TB-RETRY-004` `ErrExhausted` · `TB-RETRY-005` `ErrAttemptTimeout`
Sentinels for errors.Is. The attempt-timeout cause is `fmt.Errorf("%w after %v", ErrAttemptTimeout, d)`, one per Do
call; the code compares the attempt ctx's cause to that exact value, so a fn that returns ErrAttemptTimeout itself is
not mistaken for a timeout of this Do.

### `TB-RETRY-006..010` `Backoff`, `Exponential`, `FullJitter`, `EqualJitter`, `DecorrelatedJitter`
A closed value type: which calcbench function, base, max (shape: builder's call per the brief). Value, safe to
copy/share. Zero value = FullJitter(100ms, 10s). Retry k (0-based: k = 0 is the wait after attempt 1) is calcbench's
`attempt`. Constant delay = `Exponential(d, d)`; no separate Constant (it would be a local formula).

### `TB-RETRY-011..018` options
As the table. `WithAttemptTimeout` implementation (deviation from the brief's "context.WithTimeoutCause per attempt",
`attempt_timeout.go:23`): with `clock.Real()` it IS `context.WithTimeoutCause` (the ctx carries a deadline, ctx.Err()
= DeadlineExceeded, gRPC/HTTP clients can propagate it); with any other clock it is `context.WithCancelCause` +
`clock.AfterFunc(d, cancel(cause))`, so the timeout fires on clock.Fake (no deadline; ctx.Err() = Canceled; the cause
is the same). `context.WithTimeoutCause` alone would read the wall clock and could not be tested on the fake. The
`c == clock.Real()` test is an interface comparison: different dynamic types compare unequal without a run-time
panic, so a non-comparable Clock implementation is safe. `WithRand`'s `*rand.Rand` is used only on the calling
goroutine: one per concurrent Do.

## Deviations from LADDER §4

1. §4 had `type Policy struct{ MaxAttempts int; Base, Max time.Duration; Jitter Jitter }` passed positionally to
   `Do(ctx, p, fn, opts...)`. The wave 5 request replaces it with options only: `WithAttempts`, `WithBackoff(Backoff)`.
   Why: one configuration style (functional options, as every other toolbench package), defaults without a zero
   Policy footgun (MaxAttempts 0), and the Backoff value names the calcbench function instead of a Jitter enum.
   Lost alternative: Policy struct (shareable as a value; but a `[]retry.Option` is equally shareable).
   Consequence for LADDER §6 `httpx.RetryTransport(next, p retry.Policy)`: becomes `RetryTransport(next, opts ...retry.Option)`.
2. §4 had `Hedge` in the package line (LADDER §3 "hedged requests", not ranked): not built (out of scope below).
3. Added `ErrAttemptTimeout` (TB-RETRY-005), not in §4: callers need to tell a per-attempt timeout from the outer
   ctx (the go_68 Timeout case).
4. `Backoff` constructors `Exponential/FullJitter/EqualJitter/DecorrelatedJitter` keep calcbench's names (the
   CLAUDE.md "same name as calcbench" rule).

## Tests (written; not run by the builder)

`retry/main_test.go` (leak.Main; `instant`: a clock.Fake whose NewTimer advances by d at once, so waits are exact
fake time with no goroutine), `retry/retry_test.go`, `retry/option_test.go`. Every Example has `// Output:`.
- Waits = calcbench for a fixed seed: `TestDo_waitsEqualCalcbench` (all four Backoffs, PCG(1,2), twin source, fake
  elapsed = sum). Defaults: `TestDo_defaultBackoff`, `TestDo_defaultAttemptsIsThree`, `TestBackoff_maxDefault`,
  `TestBackoff_constantAndMaxBelowBase`.
- Counts: `TestDo_attemptsCount`, `TestDo_successAfterRetries`, `TestDo_exhaustedWrapsLastError`.
- Permanent: `TestDo_permanentStopsAtOnce`, `TestPermanent`, `TestWithRetryIf_notCalledForPermanent`.
- Default predicate: `TestDo_defaultSkipsContextErrors`, `TestDo_panicIsErrPanicNotRetried`,
  `TestDo_panicRetriedWhenRetryIfSaysSo`, `TestDo_nilFnIsErrPanic`.
- Timeout: `TestWithAttemptTimeout_firesOnFakeClockAndRetries` (BlockUntil + AdvanceContext; fn's cause is
  ErrAttemptTimeout; OnRetry sees it; second attempt succeeds; fake now = start+6s), `…_realClockSetsDeadline` (no
  wait), `…_outerCtxWins`, `…_nonPositiveIsNone`.
- Cancel: `TestDo_cancelBeforeStart`, `TestDo_cancelDuringWait` (1 h fake wait, cancel: returns at once, fake time
  unmoved, cause and last error both match, leak.Verify), `TestDo_cancelDuringAttempt`, `TestDo_cancelAfterCompletion`.
- Hooks: `TestWithOnRetry_seesAttemptErrWait`, `TestWithOnRetry_panicIsErrPanic`, `TestWithRetryIf_panicIsErrPanic`.
- Repo defects: `TestDo_go37MaxFailsOneCallsOnce`, `TestDo_go68SkipRetryWrappedKeepsCause`, `TestDo_go78NonRetryableStopsAtOnce`.
- `BenchmarkDo_success` (ReportAllocs). No time.Sleep, no wall time (the real-clock test never waits); deterministic
  under -count=200 (fixed seeds; Examples with the top-level source print only range checks).

## Out of scope

Hedged requests (`Hedge`, Dean & Barroso 2013): a later rank. Retry budgets / token-bucket retry throttling (gRPC
retry throttling, AWS SDK retry quota): `ratelimit` + caller. Server pushback (`Retry-After`, gRPC
`grpc-retry-pushback-ms`): httpx/grpcx. A max elapsed time: the caller's ctx deadline does it. gRPC's 1.6 multiplier
and ±20 % jitter: needs a calcbench function. Circuit breaking after repeated failure: `breaker`. Logging: WithOnRetry.

## For the lead

- INDEX/taxonomy: 18 rows TB-RETRY-001..018 (doc.go `retry/doc.go`); taxonomy row `retry` → status shipped/built;
  its patterns line drops `Policy` and `Hedge` (Hedge stays "expected", unranked).
- Decisions to log: (1) options replace §4 Policy (alt: Policy struct) — deviation 1; (2) default attempts 3 total
  (alt: cenkalti's unbounded with 15 min MaxElapsedTime; retry-go 10; asynq 25) — Azure "Impact on performance"
  favours few retries for interactive callers and §rule 4 forbids unbounded; (3) default Backoff FullJitter(100ms,
  10s) (alt: gRPC 1 s/1.6/120 s; Decorrelated) — Brooker's least-work finding, calcbench's worked-example numbers;
  (4) default WithRetryIf skips context errors and ErrPanic but retries an attempt timeout (alt: retry everything as
  cenkalti does); (5) WithAttemptTimeout: WithTimeoutCause on clock.Real, AfterFunc+WithCancelCause on other clocks
  (alt: AfterFunc always — loses the ctx deadline; WithTimeoutCause always — untestable on the fake); (6) Permanent
  returns fn's error unchanged (alt: strip the marker as cenkalti); (7) ctx done → cause wrapped with the last error
  (alt: cause only, losing the last error); (8) ctx already done → zero attempts (alt: cenkalti calls once).
- Assumptions: nil ctx = Background; attempts ≤ 0 → 3; base ≤ 0 → 100 ms; max ≤ 0 → max(10 s, base); max < base → base.
- compare/go.mod: add `github.com/cenkalti/backoff/v4` (lead's tidy). compare/retry written against the v4 API
  (`Retry`, `WithMaxRetries`, `WithContext`, `ZeroBackOff`, `Permanent`) without building it (v4 not in the module
  cache); if v4.3's Permanent handling differs, `TestCompare_permanentStopsAtOnce` is the one to check.
- doc/sources: brooker_2015_backoff.md, azure_retry_pattern.md, grpc_connection_backoff.md (new, opened
  2026-11-13); nygard_release_it.md (not opened); calcbench.md gains ALG-BACKOF-001..004 → TB-RETRY-001/002/007..010.
- LADDER §6 httpx `RetryTransport(next, p retry.Policy)` → `RetryTransport(next, opts ...retry.Option)`.
- calcbench requests: none required; optional: a gRPC-style backoff (multiplier 1.6, ±jitter fraction) if a
  project wants gRPC-identical connect backoff.

REMAINING: none in code; the reviewer runs `go test -race` (retry and compare/retry after the lead's tidy).

## Align (2026-11-13)

Work folder: `doc/align_2026-11-13/` · MATRIX list (b) rank 7, decisions 3 and 4 · M3 rows "retry: attempt loop",
"retry: total-elapsed cap", "retry: startup retry", "retry: permanent (non-retryable) errors and the retry predicate"
and the breaker row's composition note · Task D4 (decided by Devin).

### The matrix rows

| Pattern | REPEAT (code-heim / list b / total) | Verdict | What D4 adds |
|---|---|---|---|
| retry: total-elapsed cap | 0 / 1 / 1 | PARTIAL (narrow) | `WithMaxElapsed` (TB-RETRY-019), one additive option |
| retry: startup retry | 0 / 3 / 3 | PARTIAL (docs + the no-retry-forever decision) | the policy in the package doc and `Do`; `ExampleDo_startupDependency` |
| retry x breaker (breaker row, composition) | — | decision 3 | `ExampleWithRetryIf_breakerOpen` + a doc line on `WithRetryIf`; no import in non-test code |
| retry: attempt loop / backoff / predicate | SAME | SAME | reference-defect tests only (below) |

### References read (a reading list, never cited)

- GO-CQRS-Microservices `rabbitmq.go:31-47` (also `db.go:53-69`): cenkalti `MaxElapsedTime = 10s` paired with
  `maxRetries = 5`; bounded both ways. The shape `WithMaxElapsed` mirrors (familiarity only).
- go-grpc-graphql-microservices `account/cmd/account/main.go:24-30`: `retry.ForeverSleep(2*time.Second, ...)`, no ctx,
  never exits; monolith-microservice `pkg/common/cmd/wait.go:9-22`: polls a TCP dial forever every 500 ms, no ctx. A
  process that never comes up never exits non-zero, so the orchestrator cannot restart it.
- kubernetes-assistant-go `cmd/cli/completion.go:117-131`: classifies by `errors.As` on a wrapped typed request error
  and retries only HTTP 429 (allow-list predicate).
- go-openai-discord-bot `pkg/commands/gpt/message_handler.go:68-82`: retries with no delay, a hot loop on a failing API.

### The authority (opened 2026-11-13; `.claude/rules/sources.md` order)

- Azure Architecture Center, "Retry pattern", Microsoft Learn (ms.date 2024-07-18, updated 2025-12-09). Opened
  2026-11-13: https://learn.microsoft.com/azure/architecture/patterns/retry. Relied on: "Retry strategies" ("until
  some maximum number of requests have been attempted"); "Solution" ("unsuccessful after a predefined number of
  attempts, the application should treat the fault as an exception"); "Issues and considerations" > "Impact on
  performance" ("could also affect the responsiveness of the application if it's continually trying to perform a
  failing operation"; "it's better to fail after a smaller number of retries"); "When to use this pattern" (not
  useful "when a fault is likely to be long lasting"). NOTE for the lead: the page bounds retries by a NUMBER of
  attempts only; it has no sentence naming a maximum elapsed TIME (the task's paraphrase "attempts or time" is not
  on the page) and nothing about startup. The time cap is therefore cited to "Impact on performance" (stop
  continually retrying a failing operation) and to Nygard's Timeouts, not to a quote that does not exist.
- Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns", "Timeouts" (bound how long
  you are willing to wait) and the warning that retries amplify load. Book: cited, NOT opened.
- The Twelve-Factor App, IX "Disposability" (fast startup; crash-only: a process that cannot start exits so the
  platform restarts it). Already in `doc/sources/twelve_factor_disposability.md`; relied on by the startup Example.
- pkg.go.dev `errors`, "func AsType" (go1.26): the allow-list predicate test.
- cenkalti/backoff `MaxElapsedTime`: API familiarity only, not authority, not copied.

### Today's Go

go1.27 (go.mod `go 1.27`, toolchain go1.27.3). Nothing in the standard library bounds a retry loop by elapsed time;
`context.WithTimeout` bounds the whole call but cuts fn in flight and returns `DeadlineExceeded`, which is exactly
what `WithMaxElapsed` is NOT (it stops starting waits/attempts and keeps the last real error). `errors.AsType[T]`
(go1.26) replaces the `var target T; errors.As(err, &target)` two-liner in predicates. `time.Duration.String()`
prints `2.5s` for 2500 ms (the Example output).

### calcbench

ALG-BACKOF-001..004 unchanged; no new mathematics (the cap is a subtraction and a comparison on the clock). No
request for calcbench.

### New export

| ID | Name | Signature | file:line |
|---|---|---|---|
| `TB-RETRY-019` | `WithMaxElapsed` | `func WithMaxElapsed(d time.Duration) Option` | retry/option.go (after WithAttemptTimeout) |

Contract:
- **Measured** on the WithClock clock: one `Now()` just before the first attempt, one per failed attempt that
  would be retried (after WithAttempts and the predicate have let it through, before WithOnRetry).
- **Rule**: after a failed, retryable attempt, when `elapsed + wait > d` Do does not start the wait and returns
  `ErrExhausted` wrapping the last error: message `"retry: attempts exhausted: <elapsed> elapsed of the <d> cap after
  <n> attempts: <err>"`. So `errors.Is(err, ErrExhausted)` holds in both the attempts-exhausted and the time-exhausted
  case; the two are told apart by the message. No separate `ErrElapsed`: callers branch on "give up" (one sentinel),
  not on which bound fired; a second sentinel would be a second thing to `errors.Is` for the same decision.
- **An attempt in flight is never cut**: fn runs to its end and its result is kept (a success past the cap is a
  success; a failure past the cap is `ErrExhausted` with that failure, no further attempt). That is the difference
  from a ctx deadline, which cuts fn. The cap bounds the waits between attempts, not fn: pair it with a ctx deadline
  or WithAttemptTimeout to bound fn.
- **Goroutines**: none. **Cancel**: ctx done still wins at every point it did before (returns `context.Cause(ctx)`,
  wrapped with the last error). **Errors**: as above. **Blocking**: unchanged. **Zero value**: `d <= 0` is no cap
  (the default). **Interaction with WithAttempts**: whichever bound is reached first stops the call; the attempts
  message is unchanged. **WithOnRetry** is not called for a wait that is not started. The comparison is written
  `wait > d-elapsed` so it cannot overflow.

### Policy (package doc and Do)

Every call is bounded: by WithAttempts (default 3; `WithAttempts(0)` stays the default, never "forever"), by
WithMaxElapsed, or by ctx. There is no retry-forever setting, including at process start: a dependency that does not
come up inside the bounds makes the process exit non-zero (Twelve-Factor IX), which is what lets the orchestrator
restart it. `ExampleDo_startupDependency` shows the shape: a stub that fails twice, `WithAttempts(5)`,
`WithMaxElapsed(10*time.Second)`, a ctx deadline, and the exit path in a comment (a fake clock would not make the
Example's Output deterministic, so a deterministic stub stands in for the dial).

### Composition with breaker (MATRIX decision 3)

`retry`'s default predicate retries `breaker.ErrOpen` (it is not a context error or a panic), so `retry.Do` around
`breaker.Do` waits out three attempts against a breaker that is going to say ErrOpen each time. The fix is
documentation: `WithRetryIf(func(err error) bool { return !errors.Is(err, breaker.ErrOpen) })`, shown in
`ExampleWithRetryIf_breakerOpen` (`retry/breaker_example_test.go`, package `retry_test`, which imports `breaker`;
no import between the two packages in non-test code) and named in WithRetryIf's doc comment. Alternative (breaker
wraps ErrOpen in `retry.Permanent`): lost, a cross-package import and a changed error chain.

### Tests added (written, never run by the builder; `retry/option_test.go`, `retry/retry_test.go`, `retry/breaker_example_test.go`)

- `ExampleWithMaxElapsed`; `TestWithMaxElapsed_stopsBeforeAWaitThatWouldPass` (constant 1 s waits, cap 2.5 s: three
  attempts, 2 s elapsed, OnRetry twice, message names 2s and 2.5s); `TestWithMaxElapsed_attemptInFlightNotCut` (fn
  moves the fake clock 10 s past a 5 s cap: its failure is kept and wrapped with no further attempt, and its success
  is returned as such); `TestWithMaxElapsed_zeroIsNoCap` (0 and -1: five attempts, four 1 h waits);
  `TestWithMaxElapsed_attemptsBoundWinsWhenEarlier`; `TestWithMaxElapsed_cancelDuringWaitWins` (leak.Verify, fake
  clock: the ctx cause, not ErrExhausted).
- `ExampleDo_startupDependency`; `ExampleWithRetryIf_breakerOpen`.
- Reference-defect tests: `TestWithRetryIf_allowListTypedError` (kubernetes-assistant-go completion.go:117-131:
  `errors.AsType` on a `%w`-wrapped typed error, only code 429 retried), `TestDo_zeroWaitBackoffStillBounded`
  (go-openai-discord-bot message_handler.go:68-82: `Exponential(0, 0)` waits 100 ms, 200 ms, 400 ms and the zero
  Backoff is FullJitter(100 ms, 10 s) under the default 3 attempts), `TestBackoff_capHoldsAfter60Doublings`
  (kubernetes-assistant-go completion.go:101 uncapped doublings: 70 attempts of Exponential(1 s, 10 s) sum to
  665 s and the 69th wait is exactly 10 s; FullJitter never exceeds 10 s).
- Benchmarks: `BenchmarkDo_success` exists already (`retry/retry_test.go`); `BenchmarkDo_withMaxElapsed` added
  (ReportAllocs; the success path with the cap set: one extra `Now()`).

### Scale note

Bounded memory: yes, unchanged (nothing is held across calls). Contention point: none (no shared state; the clock
read is `time.Now` on clock.Real). Per-call allocations: unchanged on the success path (the config copy and
closures Do already makes; `time.Time` is a value on the stack). Measured: `BenchmarkDo_withMaxElapsed` 528 ns/op,
1 alloc/op on the success path (the lead's run, 2026-11-13). Read from the code, not measured: the time-exhausted
path builds its error with one `fmt.Errorf` (`retry/retry.go:113`), the same shape as the attempts-exhausted error.
Per retry: one clock read, no goroutine, no timer beyond the one wait timer Do already arms (from the code, not
measured). What breaks first at 10x and 100x callers is still the dependency (3x offered load under the default
3 attempts, no fleet-wide retry budget); the cap makes the worst-case time per call bounded even with
FullJitter(100 ms, 10 s) and a large WithAttempts (previously ~60 s of waits the caller could bound only by ctx).

### For the lead

- INDEX: one row, TB-RETRY-019 `WithMaxElapsed` (regenerate from the doc comments; `Do`'s Errors line and
  `WithRetryIf`'s doc line changed, so their rows re-generate too). taxonomy: `retry` patterns line gains "max elapsed".
- Decisions to log: (D4-a) no separate `ErrElapsed`: `ErrExhausted` wraps the last error in both cases, the message
  tells them apart (alt: an `ErrElapsed` that also matches `ErrExhausted` via an `Is` method: a second sentinel for
  the same "give up" decision); (D4-b) the cap stops a wait that would END after it (`elapsed + wait > d`); an
  attempt may start at exactly d (alt: `>=`; cenkalti uses `>`); (D4-c) WithOnRetry is not called for a wait that is
  not started (alt: call it with the refused wait: it would report a wait that never happens).
- Assumptions: `d <= 0` is no cap; the first `Now()` is read just before the loop (a ctx already done returns
  without an attempt and the read is unused).
- Sources: `doc/sources/azure_retry.md` gains "Issues and considerations" > "Impact on performance" and TB-RETRY-019,
  and the note that the page has no maximum-elapsed-time sentence; `nygard_2018_release_it.md` gains TB-RETRY-019
  (Timeouts); `twelve_factor_disposability.md` gains `ExampleDo_startupDependency`; `go_standard_library.md` gains
  `errors.AsType` (go1.26) for the predicate test.
- Not done (outside D4): `TestWithAttemptTimeout_fnIgnoringCtxDelaysNextAttempt` (M3 row "per-attempt timeout")
  remains a suggestion for the hardening run.

### Measured (VERIFY, 2026-11-13, go1.27.3 darwin/arm64, -race on; raw in doc/align_2026-11-13/out/bench_new.txt)
BenchmarkDo_withMaxElapsed 528 ns/op, 96 B/op, 1 alloc/op (the cap adds one clock read per retry; the alloc is the existing config/closure cost).
