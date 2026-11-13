# Brief for the wave 5 builders (T3 resilience + schedule.Cron, 2026-11-13)

Request: `doc/requests/wave5_2026-11-13.md`. The standard, the mechanics and the hard limits are those of
`doc/wave3_2026-11-13/BRIEF.md` (read it first: standard verbatim, owned paths, doc.go and doc-comment shape, test
rules, builders never `go test` and never `go mod`, hand-back format), plus the lessons in
`doc/wave4_2026-11-13/BRIEF.md` ("Built on" and "Lessons"). Rules 2 and 5 now carry the D-027 and D-024 exceptions
(`.claude/rules/go-concurrency.md`); no new exception without a lead decision. Folder: `doc/wave5_2026-11-13/`.

## STEP 0 is done

`go.mod` requires `github.com/CrimsonCoder42/calcbench v0.2.0` (main go.mod sha256 a12ebe1b…, go.sum e6b44175…;
these must not change again). The contract for what v0.2.0 adds is
`/Users/devinanderson/Desktop/calcbench/doc/requests/toolbench_reply_2026-11-13.md` (read it). Pinned source:
`$(go env GOMODCACHE)/github.com/!crimson!coder42/calcbench@v0.2.0/`. Confirmed in calcbench INDEX.md:
ALG-BACKOF-001 Exponential, -002 FullJitter, -003 EqualJitter, -004 DecorrelatedJitter (`algo/backoff`; errors
ErrAttempt, ErrBase, ErrMax, ErrDraw, ErrPrev); ALG-CRON-001 ParseCron, ALG-CRON-002 CronSpec.Next (`algo/cron`;
ErrSyntax, ErrRange, ErrNeverFires; POSIX only, no `*/N`, names or `@` macros; dom/dow ORed only when both are not
`*` (A-068); DST gap → the transition instant once, overlap → the first occurrence only); NUM-INTEG-013 CeilDiv;
ALG-RATELM-001 TokenBucket (exact integer refill), -002 LeakyBucket (GCRA), -003 FixedWindow, -004 SlidingWindowLog.

## Built on (never duplicated)

`clock` (every wait and every measured duration: `WithClock(clock.Clock)`, tests on `clock.Fake` with
`AdvanceContext`/`BlockUntil`, never wall time, never `time.Sleep`), `panics` (every recover), `group`, `flow`,
`guard`, `semaphore` where they fit. The reviewer checks every timing claim on the fake clock.

## compare/

Write `compare/<package>/*_test.go`; never run `go mod tidy` (the lead tidies once after the wave; only the named
libraries enter compare/go.mod; main go.mod/go.sum stay as STEP 0 left them). Compare behaviour (counts, order,
errors, state transitions on a fake or manual clock), never wall-clock timings.

## The packages and the agreed LADDER.md §4 APIs (the request refines them; deviations are written down)

**B0 `schedule.ParseCron`** (finish the existing package; owner of `schedule/` and `doc/specs/schedule.md` only).
The spec already describes it as PENDING calcbench R-3: the PENDING mark becomes the ID (next free:
TB-SCHED-012). Call ALG-CRON-001/002 (`Math:` line). Shape from LADDER.md §4 / the spec:
`func ParseCron(spec string, loc *time.Location) (Schedule, error)` (nil loc = time.Local? or UTC? decide and
state; the calcbench Next works in `after.Location()`, so the returned Schedule converts `after` to loc first).
Tests from the reply file: `*/5` → calcbench ErrSyntax wrapped (errors.Is holds); names and `@daily` → ErrSyntax;
dow 7 → ErrRange; `0 0 31 2 *` → ErrNeverFires at ParseCron (construction), never at Run; America/New_York spring
gap and fall overlap days (gap fires once at the transition instant; overlap fires at the first occurrence only);
dom/dow OR rule; Run(ParseCron(...)) driven by clock.Fake fires at the right instants. No Sleep. Update doc.go,
the spec (Status SHIPPED), and the `ParseCron` Example.

**B1 `retry`, SHORT RETRY** (LADDER rank 24, T3; rank 29 timeout folds in as an option, D-006 §9.11). Repos
go_37_background_work (MaxFails), go_68_asynq (MaxRetry, SkipRetry), go_78_resty (implied retry; status ignored).
Authority: Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015 (cited through calcbench's
algo/backoff doc; open the post); Nygard, Release It!, 2nd ed., 2018, ch.5 "Stability Patterns" (book, not
online: cite, not opened); Azure Architecture Center, "Retry pattern"
(https://learn.microsoft.com/azure/architecture/patterns/retry); gRPC "Connection Backoff Protocol"
(https://github.com/grpc/grpc/blob/master/doc/connection-backoff.md). calcbench: ALG-BACKOF-001..004 by ID, never a
local formula; the draw u from the retry's own `WithRand` (math/rand/v2 source; fixed seed in tests). Builds on:
`clock` (the waits), `panics`. Compare: github.com/cenkalti/backoff/v4 (LADDER rank 24): attempt counts, Permanent,
ctx cancel; not timings.
```go
func Do(ctx context.Context, fn func(context.Context) error, opts ...Option) error
func DoValue[T any](ctx context.Context, fn func(context.Context) (T, error), opts ...Option) (T, error)
func Permanent(err error) error                         // marks err as not retryable; errors.Is/As still see err
type Option func(*config)
func WithAttempts(n int) Option                         // total attempts; default decided and documented (bounded!)
func WithBackoff(b Backoff) Option                      // which calcbench function, base, max (shape: builder's call, e.g. FullJitter(base, max))
func WithAttemptTimeout(d time.Duration) Option         // context.WithTimeoutCause per attempt
func WithRetryIf(f func(error) bool) Option             // default: any error except context errors and panics.ErrPanic
func WithOnRetry(f func(attempt int, err error, wait time.Duration)) Option
func WithClock(c clock.Clock) Option; func WithRand(r *rand.Rand) Option
var ErrExhausted                                        // wraps the last error (errors.Is both)
```
LADDER §4 had `Policy`; the request's option shape replaces it: record the deviation. State: a panic in fn
(ErrPanic, not retried by default), ctx cancelled during a wait (returns at once with the cause), attempt timeout
vs outer ctx, the go_78 defect (status ignored → a test that a non-retryable error stops at once).

**B2 `singleflight`, SHORT FLIGHT** (rank 25). No repo (go_45 stampede implied). Authority: pkg.go.dev
golang.org/x/sync/singleflight and its source (Do, DoChan, Forget; the panic and Goexit handling in its source).
Builds on: `panics`. Compare: golang.org/x/sync/singleflight.
```go
type Group[K comparable, V any] struct{ /* ... */ }      // zero value ready
func (g *Group[K, V]) Do(ctx context.Context, key K, fn func(context.Context) (V, error)) (v V, shared bool, err error)
func (g *Group[K, V]) DoChan(ctx context.Context, key K, fn func(context.Context) (V, error)) <-chan Result[V]
func (g *Group[K, V]) Forget(key K)
type Result[V any] struct{ Val V; Err error; Shared bool }
```
Decide and state: what ctx the shared fn runs with (the first caller's? a detached ctx cancelled when every waiter
has left? — x/sync has no ctx; the request puts ctx on Do), a waiter whose ctx ends returns its cause while the
call continues for others, a panic in fn → ErrPanic to every waiter (x/sync re-panics), a shared result is shared
memory (doc: do not mutate V), Forget during a call.

**B3 `ratelimit`, SHORT RATE** (rank 26). Repos go_77_rate_limit (data race on the per-IP map), go_82_rate_limiting_revised
(no eviction; `sync` not imported; LoadOrStore allocates per request). Authority: pkg.go.dev golang.org/x/time/rate
(Limiter, Allow, AllowN, Reserve, Wait, Limit, Burst, Every; and its source); RFC 2697 §3 (via calcbench
ALG-RATELM-001). calcbench: SEARCH INDEX.md by meaning first: ALG-RATELM-001 TokenBucket has exact integer refill;
use it for the bucket arithmetic (under the limiter's mutex: calcbench meters are not concurrent-safe,
algo/ratelimit/doc.go) unless the x/time/rate shape (float Limit, Reserve with a delay, Wait) cannot be expressed
with it — then say exactly why in the spec; ALG-LIST-015 LRUCache for the keyed bound if it fits. Builds on:
`clock`, `guard` or a plain mutex, `panics`. Compare: golang.org/x/time/rate. (RFC 6585 429 is httpx, later.)
```go
type Limit float64                                       // events per second, as x/time/rate (or keep §4's integer per-interval: decide, record)
func NewLimiter(r Limit, burst int, opts ...Option) *Limiter   // x/time/rate shape; §4 had New(perInterval, interval, burst)
func (l *Limiter) Allow() bool; AllowN(n int) bool; Reserve() *Reservation; Wait(ctx context.Context) error
func (l *Limiter) Limit() Limit; Burst() int
func NewKeyed[K comparable](r Limit, burst int, opts ...Option) *Keyed[K]   // WithMaxKeys(n), WithIdleTTL(d)
func (k *Keyed[K]) Allow(key K) (ok bool, retryAfter time.Duration)
var ErrKeys                                              // the key bound is full and nothing is idle to evict
func WithClock(c clock.Clock) Option
```
State: Wait with n > burst (x/time/rate returns an error: match it), Reservation.Cancel, idle eviction driven by
the clock (no background goroutine unless stated and stopped), the go_77/go_82 defects as named tests.

**B4 `breaker`, SHORT BRK** (rank 27). No repo. Authority: Nygard, Release It!, 2nd ed., ch.5 "Circuit Breaker"
(book, not opened); Fowler, "CircuitBreaker", martinfowler.com, 2014 (https://martinfowler.com/bliki/CircuitBreaker.html);
Azure "Circuit Breaker pattern" (https://learn.microsoft.com/azure/architecture/patterns/circuit-breaker).
calcbench: ALG-STREAM-002 HitCounter only if the failure window is time-based (LADDER §5); a consecutive-failure
threshold needs none — decide and state. Builds on: `clock`, `panics`. Compare: github.com/sony/gobreaker (v2 if it
fits: state transitions under the same scripted results on a manual clock).
```go
func New(opts ...Option) *Breaker                         // WithFailureThreshold(n), WithOpenFor(d), WithHalfOpenProbes(n), WithClock, WithOnStateChange
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error
func DoValue[T any](ctx context.Context, b *Breaker, fn func(context.Context) (T, error)) (T, error)
func (b *Breaker) State() State
type State int                                            // Closed, Open, HalfOpen (String method)
var ErrOpen                                               // also returned when half-open probes are all in flight
```
State: what counts as a failure (default any error except context errors; a panic counts and is ErrPanic), probes
in half-open (n concurrent; success of all closes, any failure re-opens), the hook runs where (caller's goroutine,
outside the lock), zero value usable or not.
