# group: an error group and a result group (cancel on first error, bounded, panics returned as errors)

Status: SHIPPED (2026-11-13)
Package: `group` · SHORT: `GROUP` · Layer: core · Tier: T1
Work folder: `doc/wave2_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #7
Builds on: `panics` (TB-PANIC-001 ErrPanic, TB-PANIC-003 Try) · Decisions: D-006 · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; reading list only, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `_reference/code-heim/go_61_err_group/simple/main.go` | `errgroup.Group` + `SetLimit(2)` fetches four URLs (`:46-63`); results go into a `map` behind a separate mutex passed by pointer to a map (`:19,36-38`). | `*map[string]string` with a hand-held mutex is what a result group is for (`Results[T]`, submission order, no shared map). `fmt.Printf` inside the task (Quiet rule). Network in the example. |
| `_reference/code-heim/go_61_err_group/with_context/main.go` | `errgroup.WithContext` + `SetLimit(2)`; each task checks `ctx.Done()` once before starting (`:21-25`). | **Defect: returning nil on cancel hides the cause** (`:22-25` returns `nil` when the ctx is done). When the parent ctx is cancelled (timeout, shutdown) and no task failed, every task returns nil and `Wait` returns nil: the caller is told "all URLs fetched" while none were. The task must return `context.Cause(ctx)`. Tests: `TestGroup_returningNilOnCancelHidesTheCause` (the defect pattern gives nil; returning the cause gives the cause) and, in `Results`, the library itself refuses to start a task after cancel and records the cause (`TestResults_cancelBeforeRecordsCause`). Also: the check is a `select` with `default` before a blocking `http.Get` that ignores ctx (no `NewRequestWithContext`), so cancel does not stop work in flight. |
| `_reference/code-heim/go_67_conc/result_pool/main.go` | `conc` `pool.NewWithResults[int]().WithMaxGoroutines(5)` (`:28`); results printed after `Wait` (`:40-43`). | conc's result pool returns results in **completion** order (its docs say order is not guaranteed); ours keeps submission order (`TestResults_submissionOrder`). `apiID := apiID` copy (`:32`, dead since go1.22); `time.Sleep` as work and a time-seeded `rand` (`:15-20`). |
| `_reference/code-heim/go_67_conc/pool_with_errors/main.go`, `pool_with_context/main.go` | conc `WithErrors().WithMaxGoroutines(2)` (`:31`), `WithContext(ctx)` (`:36`). | conc joins all errors by default (`WithFirstError` to change); errgroup and ours keep the first. `time.After` in a loop (`:45`). |
| `_reference/code-heim/go_67_conc/group/main.go` | conc `WaitGroup.Go` re-raises a task panic on `Wait`. | Re-raising crashes the caller; ours returns `panics.ErrPanic` (already a wave 1 test in `panics`; here `TestGroup_panicReturnsErrPanic`). |

### b. The authority (opened 2026-11-13)

- pkg.go.dev `golang.org/x/sync/errgroup`, latest **v0.25.0 (Oct 15, 2026)**: `Group`, `WithContext`, `Go`, `TryGo`,
  `SetLimit`, `Wait`. Opened: https://pkg.go.dev/golang.org/x/sync/errgroup, 2026-11-13, and the versions tab
  https://pkg.go.dev/golang.org/x/sync/errgroup?tab=versions. Relied on:
  - `WithContext`: "The derived Context is canceled the first time a function passed to Go returns a non-nil
    error or the first time Wait returns, whichever occurs first."
  - `Go`: "The first call to Go must happen before a Wait. It blocks until the new goroutine can be added without
    the number of goroutines in the group exceeding the configured limit."
  - `TryGo`: "only if the number of active goroutines in the group is currently below the configured limit".
  - `SetLimit`: "A negative value indicates no limit. A limit of zero will prevent any new goroutines from being
    added ... The limit must not be modified while any goroutines in the group are active."
  - `Wait`: "blocks until all function calls from the Go method have returned, then returns the first non-nil
    error (if any) from them."
  - `Group`: "A zero Group is valid, has no limit on the number of active goroutines, and does not cancel on error."
- The source, v0.25.0. Opened: https://raw.githubusercontent.com/golang/sync/v0.25.0/errgroup/errgroup.go and
  https://raw.githubusercontent.com/golang/sync/master/errgroup/errgroup.go, 2026-11-13; history
  https://github.com/golang/sync/commits/master/errgroup/errgroup.go; revert commit
  https://github.com/golang/sync/commit/7fad2c9213e0821bd78435a9c106806f2fc383f1; old behaviour
  https://raw.githubusercontent.com/golang/sync/v0.15.0/errgroup/errgroup.go and v0.16.0 likewise. Relied on:
  `WithContext` uses `context.WithCancelCause` and cancels with the first error (so `context.Cause(ctx)` is that
  error); `Wait` (`:51`) is `wg.Wait()`, `cancel(g.err)`, `return g.err`; `SetLimit` (`:118`) panics with
  "errgroup: modify limit while %v goroutines in the group are still active".
- Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4 "Error Handling" (errors are values sent back to the
  goroutine that has the full picture; the goroutine that starts work decides). The book is not online: cited by
  section, **not opened** in this session. The errgroup docs above carry every rule this package relies on.

### LADDER.md §9 item 14, settled: does x/sync errgroup re-panic from Wait?

**No, not in the current release.** In **v0.25.0** (and every release since **v0.16.0**, Jun 20, 2025) the source
has no `recover` at all: a panic in a function passed to `Go`/`TryGo` is unrecovered on errgroup's own goroutine
and **crashes the process**; `Wait` never sees it. `Group.Go` carries the "tsunami stone" comment at v0.25.0
lines 76-88: "It is tempting to propagate panics from f() up to the goroutine that calls Wait, but it creates more
problems than it solves: it delays panics arbitrarily ...; it turns f's panic stack into a mere value, hiding it
from crash-monitoring tools; it risks deadlocks that hide the panic entirely ... See #53757, #74275, #74304,
#74306." **Goexit**: a function that calls `runtime.Goexit` ends its goroutine, the deferred `done` runs, and `Wait`
returns normally (nil unless another function failed).
History: commit 506c70f "errgroup: propagate panic and Goexit through Wait" (Apr 7, 2025, released in **v0.14.0
and v0.15.0**) made `Wait` re-panic with `PanicError`/`PanicValue` and call `runtime.Goexit`; commit 7fad2c9
"errgroup: revert propagation of panics" (Jun 20, 2025, **v0.16.0**) removed it ("This caused more problems than it
solved"). The R3 note that errgroup re-panics was true only of v0.14.0-v0.15.0.

**Ours**: a panic in a function passed to `Go`/`TryGo`/`Results.Go` is recovered on our goroutine with
`panics.Try` and returned as the group's error through `Wait` (`errors.Is(err, panics.ErrPanic)`, first error wins,
the derived ctx is cancelled with it). Go's own objection (stack hidden) is met because `*panics.PanicError`
carries the stack of the panicking goroutine. **Goexit**: as errgroup and `panics.Try`: not an error; the goroutine
ends, its slot and `WaitGroup` count are released by a deferred call, `Wait` does not hang.
**Compare test**: the panic case cannot run against errgroup (it would kill the test binary), so it is run on
ours only; `TestCompare_panicWrappedWithTry` shows that errgroup with `panics.Try` around `f` gives the same
`Wait` result as ours. Goexit is compared directly (both return nil). `SetLimit` while active is compared by
recovering errgroup's panic on the test goroutine (it panics in the caller, so it is recoverable) against ours,
which does not panic.

### c. Today's Go

- `golang.org/x/sync/errgroup` (v0.25.0) is the shape; it is not the standard library, and its panic rule (crash)
  conflicts with CLAUDE.md "a panic inside a worker is recovered and returned as an error". Its `SetLimit` panics
  on input, `Go` with limit 0 blocks forever, and it has no result group.
- `sync.WaitGroup.Go` (go1.25) covers the no-error, no-limit case; ours uses it underneath (wrap, not
  re-implement) and adds the first error, cancel, limit and panic-to-error.
- `context.WithCancelCause` / `context.Cause` (go1.20): the derived ctx's cause is the first error, as errgroup.
- Why toolbench still needs `group`: panics as values (the one shared `panics.ErrPanic`), no panic or deadlock on
  input (`SetLimit` while active, limit 0, nil fn, nil ctx), and `Results[T]`: typed results in submission order,
  which go_61 builds by hand with a mutex and a map.

### d. calcbench

None. The limit is a counter under the group's mutex, with a broadcast channel for waiters (not
ALG-QUEUE-*: it must block the caller, and it is concurrent). Nothing in calcbench is re-implemented.

### e. Exports

| ID | Name | Signature |
|---|---|---|
| `TB-GROUP-001` | `Group` | `type Group struct{...}`; methods `Go(fn func() error)`, `TryGo(fn func() error) bool`, `SetLimit(n int)`, `Wait() error` |
| `TB-GROUP-002` | `WithContext` | `func WithContext(ctx context.Context) (*Group, context.Context)` |
| `TB-GROUP-003` | `Results` | `type Results[T any] struct{...}`; methods `Go(fn func(ctx context.Context) (T, error))`, `Wait() ([]T, error)` |
| `TB-GROUP-004` | `NewResults` | `func NewResults[T any](ctx context.Context, opts ...Option) *Results[T]` |
| `TB-GROUP-005` | `Option` | `type Option func(*config)` |
| `TB-GROUP-006` | `WithLimit` | `func WithLimit(n int) Option` |
| `TB-GROUP-007` | `ErrZeroLimit` | `var ErrZeroLimit = errors.New("group: limit is zero: no goroutine can be started")` |

Options:

| Option | Default | Bound it sets |
|---|---|---|
| `WithLimit(n int)` | `-1`: no limit (errgroup's default); n < 0 = no limit; 0 = `ErrZeroLimit` | active goroutines in a `Results` |

Sentinel errors: `ErrZeroLimit` (Go called while the limit is 0) · `panics.ErrPanic` (a recovered panic, from
package panics, not re-declared).

**Deviations from LADDER.md §4** (each argued):
1. **`Option`, `WithLimit` and `ErrZeroLimit` are added.** §4 gives `NewResults(ctx, opts ...Option)` without
   naming the options; `WithLimit` is `SetLimit` for a `Results` (the only bound it needs). `ErrZeroLimit` is how
   limit 0 is "documented" without blocking forever (below).
2. **`SetLimit` while goroutines are active does not panic** (errgroup panics): the new limit applies from the
   next `Go`/`TryGo`; goroutines already running are not stopped; a `Go` that is waiting for a slot re-checks
   against the new limit (so lowering the limit below the active count makes new `Go` calls wait until enough
   goroutines finish). Rationale: a library never panics on input; the counter design makes the change safe.
3. **Limit 0**: errgroup "prevents any new goroutines": `TryGo` returns false (same here), and `Go` blocks forever
   (a silent deadlock). Ours: `Go` does not start fn, records `ErrZeroLimit` as the group's error (first error
   wins, cancels the derived ctx) and returns at once, so `Wait` reports it. A `Go` already waiting for a slot
   when the limit becomes 0 does the same.
4. **nil ctx** to `WithContext`/`NewResults` is `context.Background()` (`context.WithCancelCause(nil)` panics).
   **nil fn** is a panic of the call and returns `ErrPanic` through `Wait`.
5. **`Results.Wait` on error returns `nil, err`** (no partial slice), so a caller cannot read a zero value as a
   result. Results of a `Results` that are all successes are in submission order.
6. **`Results.Go` does not start fn after the ctx is done**: it records `context.Cause(ctx)` instead (the go_61
   defect, fixed in the library). `Group.Go` keeps errgroup's rule (it always starts fn; fn decides).

## Per export

### `TB-GROUP-001` `Group`
- **Pattern**: error group (structured concurrency: fork, join, first error cancels).
- **Authority**: pkg.go.dev golang.org/x/sync/errgroup v0.25.0, `Group`; Cox-Buday, Concurrency in Go, 2017,
  ch.4 "Error Handling".
- **Familiar shape**: `errgroup.Group` method for method (`Go`, `TryGo`, `SetLimit`, `Wait`). Differences:
  panics are returned (not crashes), `SetLimit` while active and limit 0 do not panic or deadlock (above).
- **Goroutines**: one per started `Go`/`TryGo`; each ends when fn returns, panics (recovered) or calls Goexit;
  all have exited when `Wait` returns.
- **Cancellation**: a `Group` from `WithContext` cancels its ctx (cause = the first error) on the first error,
  or when `Wait` returns (cause `context.Canceled`). fn must watch the ctx and should return `context.Cause(ctx)`
  (not nil) when it stops early. Parent cancelled before `Go`: fn still starts (errgroup rule) and sees a done ctx.
- **Errors**: first non-nil error wins (fn's error, a `*panics.PanicError`, or `ErrZeroLimit`); later errors are
  dropped (errgroup's rule; `Results` the same). `Wait` returns it.
- **Blocking**: `Go` blocks while the active count is at the limit (no ctx: errgroup's shape; slots free as fns
  return). `TryGo` never blocks. `Wait` blocks until every started fn has returned.
- **Ordering and counts**: each fn runs exactly once, on its own goroutine, in no particular order.
- **Zero value**: ready: no limit, no cancel on error (errgroup's rule). Not copied after first use (vet copylocks).
- **Reuse**: `Go` concurrent with `Wait` while no fn is running is a misuse (as `sync.WaitGroup`); a fn may call
  `Go` on its own group. Group is used for one task.
- **Clock**: none (never waits on time).
- **calcbench IDs called**: none.
- **Compared against**: `golang.org/x/sync/errgroup` v0.25.0, `compare/group/group_test.go`.
- **Worked example**: `ExampleGroup` (three squares summed, `// Output: 14`), `ExampleGroup_SetLimit`,
  `ExampleGroup_TryGo`.

### `TB-GROUP-002` `WithContext`
- As errgroup: returns a new Group and a ctx derived with `context.WithCancelCause`; cancelled with the first
  error or when `Wait` returns. nil ctx = `context.Background()`. Example `ExampleWithContext`.

### `TB-GROUP-003` `Results`
- **Pattern**: result group (fork-join with typed results; errgroup + ordered collection).
- **Authority**: errgroup v0.25.0 (`Group`, `WithContext`); Cox-Buday ch.4 "Error Handling".
- **Familiar shape**: conc `pool.ResultContextPool[T]` (`Go(func(ctx) (T, error))`, `Wait() ([]T, error)`), with
  errgroup's first-error rule and **submission order** (conc: completion order).
- **Goroutines**: one per `Go` (bounded by `WithLimit`); all exited when `Wait` returns.
- **Cancellation**: fn receives the derived ctx, cancelled on the first error or when `Wait` returns. A fn whose
  goroutine starts after the ctx is done is not called; `context.Cause(ctx)` is recorded (so parent cancellation
  is never reported as success).
- **Errors**: first error wins: fn's error, `ErrPanic`, `ErrZeroLimit`, or the ctx cause. `Wait` returns nil, err.
- **Blocking**: `Go` blocks at the limit; `Wait` blocks until all have returned.
- **Ordering**: `Wait` returns one value per `Go`, in the order `Go` was called.
- **Zero value**: ready: `context.Background()`, no limit. `NewResults` to pass a ctx or options.
- **Compared against**: none directly (errgroup has no result group); the order rule is tested here.
- **Worked example**: `ExampleResults` (`// Output: [1 4 9]`).

### `TB-GROUP-004` `NewResults`
Returns a `Results[T]` bound to a ctx derived from ctx (nil = Background) and the options. Example `ExampleNewResults`.

### `TB-GROUP-005` `Option`, `TB-GROUP-006` `WithLimit`
Functional option for `NewResults`; `WithLimit(n)` is `SetLimit(n)` before the first `Go`. Examples `ExampleOption`,
`ExampleWithLimit`.

### `TB-GROUP-007` `ErrZeroLimit`
Recorded when `Go` is called (or is waiting) while the limit is 0. Example `ExampleErrZeroLimit`.

## 1g. Comparison

`compare/group/group_test.go` (package `group_test`) against `golang.org/x/sync/errgroup` **v0.25.0**: same inputs,
same observable result for: first error wins and is the ctx cause; Wait waits for every fn; SetLimit bounds the
max in flight (atomics); TryGo at the limit; negative limit = no limit; zero Group; Goexit (both nil). Named
differences: panic (errgroup crashes; ours `ErrPanic`; compared via `panics.Try` wrapping), SetLimit while active
(errgroup panics, recovered on the test goroutine; ours applies the new limit), limit 0 with `Go` (errgroup blocks
forever, not run; ours `ErrZeroLimit`). Written, not built or vetted (compare deps unresolvable this session).

## Tests

`group_test.go`, `results_test.go`, `example_test.go`: Example per export with `// Output:`; first error wins and
cancels with cause; Wait waits for all; SetLimit bounds concurrency (atomic max in flight == limit); TryGo at and
below the limit; negative limit = no limit; limit 0 (`ErrZeroLimit`, TryGo false, waiting Go released);
SetLimit while active (no panic, new limit applies); panic and nil fn → `ErrPanic`; Goexit does not hang;
zero-value Group and Results; nil ctx; Results submission order under reverse completion; Results cancel before /
during / after; Results panic; `TestGroup_returningNilOnCancelHidesTheCause`; `leak.Main` + `leak.Verify`.
Benchmark `BenchmarkGroup_Go` with `b.ReportAllocs()`.

## 1h. Out of scope

Collecting all errors (`errors.Join`): `parallel`/`pool` options, wave 3. Long-lived worker pools with a queue:
`pool`. Weighted limits: `semaphore`. A run group with ordered shutdown: `lifecycle`. Results streamed as they
complete: `flow.FanOut`.

## For the lead

- INDEX.md rows TB-GROUP-001..007; taxonomy row `group` → status built.
- doc/sources: pkg.go.dev x/sync/errgroup v0.25.0 + source URLs above; Cox-Buday ch.4 (not opened).
- LADDER.md §9 item 14 answer above (no re-panic since v0.16.0); R3 note to correct.
- DECISIONS candidates: deviations 2-6 above; Goexit in a group fn is not an error (as errgroup and `panics.Try`).
- compare/go.mod needs `golang.org/x/sync v0.25.0`. `compare/group/group_test.go` written, not built or vetted.
- Files: `group/doc.go`, `group/group.go` (Group :38, WithContext :60, ErrZeroLimit :19), `group/results.go`
  (Option :15, WithLimit :28, Results :49, NewResults :66), tests `group/group_test.go`, `group/results_test.go`,
  `group/example_test.go`. Checks run: `gofmt -l group` clean, `go build ./...` ok, `go vet ./group/` clean.

REMAINING: none (verify phase: `go test -race -timeout 5m ./group/`, compare build once deps resolve).

## Align (2026-11-13)

**Matrix row**: MATRIX.md row 20 "error group (fork-join, first error cancels)", TB-GROUP-001..007, REPEAT 2/5/7,
verdict PARTIAL: collect-all errors (sub-pattern REPEAT 3: go_67 conc `pool.WithErrors`, Bootcamp
`waitgroups/error.go`, LLM-Projects `thinktank.go`). List (b) rank 4. Status: BUILT (2026-11-13), additive.

**References read** (reading list, never authority): `_reference/code-heim/go_67_conc/pool_with_errors/main.go`
(conc joins every error by default, `WithFirstError` is the opt-in); `_reference/AkhilSharma90/LLM-Projects/
medium-agent-expert-gemini/thinktank.go:33-58` (one `Err` per expert, results appended in completion order under a
mutex, so result i is not expert i); `_reference/AkhilSharma90/ULTIMATE-GO-Bootcamp-Course/small_examples/
waitgroups/error.go:8-31` (`sync.WaitGroup` and the `[]error` passed BY VALUE: does not compile as written; with
pointers, the errors would be kept one per task); `_reference/AkhilSharma90/AI-Code-Bundler/internal/files/
reading.go:107-110` (`errChan` sized `len(input)`, then `return nil, <-errChan`: every error but the first dropped
and every result dropped with it).

**Authority opened and cited**: pkg.go.dev `errors`, "func Join" (go1.27): "Join returns an error that wraps the
given errors. Any nil error values are discarded. Join returns nil if every value in errs is nil. The error formats
as the concatenation of the strings obtained by calling the Error method of each element of errs, with a newline
between each string. A non-nil error returned by Join implements the Unwrap() []error method." pkg.go.dev
`golang.org/x/sync/errgroup` v0.25.0 "func (*Group) SetLimit" (the method shape mirrored by `SetCollect`) and
"func (*Group) Wait" ("returns the first non-nil error": the default, unchanged). Cox-Buday, Concurrency in Go,
2017, ch.4 "Error Handling": not opened (named only).

**Today's Go (go1.27)**: `errors.Join` (go1.20) and `Unwrap() []error` on the result; `errors.Is`/`errors.As` walk
the tree, so one joined error matches each member. errgroup v0.25.0 still has no collect mode. conc
`pool.WithErrors` joins by default. Nothing in the standard library collects for an errgroup; it stays a method here.

**calcbench**: none (no mathematics; a slice append under the existing mutex).

### New exports

| ID | Name | Signature | File |
|---|---|---|---|
| `TB-GROUP-008` | `Group.SetCollect` | `func (g *Group) SetCollect(collect bool)` | `group/collect.go` |
| `TB-GROUP-009` | `WithCollect` | `func WithCollect() Option` | `group/collect.go` |

**`TB-GROUP-008` `Group.SetCollect`**: mirrors `SetLimit`'s shape (a setter on the Group, not a constructor
option, because `WithContext` has no options and a second constructor would be a new shape). With `true`, every
function error, every recovered panic (`panics.ErrPanic`) and `ErrZeroLimit` are kept in completion order (the
order `fail` takes `g.mu`), bounded by one entry per failed `Go`/`TryGo`, and `Wait` returns them joined with
`errors.Join` (nil when none; a single failure is still wrapped by Join, so match with `errors.Is`/`errors.As`,
never `==`). **Collecting changes what `Wait` returns, not when the group stops**: the first error still cancels a
`WithContext` ctx with itself as the cause; a caller who wants the other functions to run to completion uses a
Group without a ctx (the zero Group: no cancel on error, errgroup's rule). With `false` (the default) nothing
changes: `Wait` returns the first error exactly as before; every existing test is untouched. Call it before the
first `Go` (as errgroup says of `SetLimit`); a later call applies to errors recorded afterwards, and an earlier
first error is never lost: `Wait` puts it first in the join if it was recorded before collecting began.
Goroutines: none. Blocking: none. Zero value: collecting off.
- Alternative that lost: a `WithContext`-style option (`WithContext(ctx, WithCollect())`): `WithContext` has no
  options today and errgroup's has none; a second constructor is a new shape. `SetCollect` is `SetLimit`'s shape.
- Decision for the ledger (D-needed): conc's `pool.WithErrors` joins by default and `WithFirstError` is the opt-in;
  ours keeps errgroup's first-error default because errgroup is the shape callers already know, and collecting is
  the opt-in (`SetCollect(true)` / `WithCollect()`).
- No `SetMaxErrors`: `pool` got `WithMaxErrors` (TB-POOL-011) because a pool is long-lived and its submissions are
  unbounded; a Group is one task whose `Go` calls the caller already counts (each failed Go cost a goroutine; the
  kept slice is strictly smaller). Added with its own ID and pool's "N further errors not kept" message shape if a
  consumer ever submits an unbounded number of Go calls to one Group.

**`TB-GROUP-009` `WithCollect`**: an `Option` for `NewResults` that sets `SetCollect(true)` on the Results' Group.
`Wait` then returns one value per `Go` in submission order, with the **zero value** in every slot whose function
returned an error, panicked, or was not called because the ctx was already done (its slot holds the zero value
and the ctx cause is in the join, once per such slot), AND the joined error of every failure; the first error
still cancels the ctx, so a function that has not started when it fires is not called (the value of a function
that is already running and then succeeds is kept). Without the option `Wait` returns `nil, firstErr` exactly as
before. `Each() iter.Seq2[T, error]` (the thinktank shape, one `(value, error)` per slot): **not added**; it needs a
per-slot `[]error` and a "call after Wait" rule for a new iterator export; the joined error plus the zero-value
slot already shows partial results. Left in Out of scope for a consumer request.

### Scale note

Bounded memory: yes. `errs` holds one entry per failed Go (at most the number of Go calls, each of which the
caller already paid a goroutine for); `Results.vals` is unchanged (one slot per Go). Contention point: unchanged,
`g.mu` in `fail` (now an append under it, one allocation per failure amortised by slice growth) and the `freed`
close-broadcast (`group.go:175-180`, MATRIX §6). Per-call allocations: unchanged for a Go that succeeds (not measured separately; the only number is 9 allocs/op per Go in total, MATRIX §8);
a Go that fails adds its error to the slice; `Wait` with collecting allocates the join once. At 10x callers
nothing changes; at 100x the `freed` broadcast (every blocked Go wakes per release, O(waiters)) breaks first, not
the collection. **`BenchmarkGroup_Go_blockedCallers`** (SetLimit(1), 1 000 Go calls blocked behind one slow
function, ReportAllocs) is added so VERIFY records the number; the wakeup mechanism is not changed in this run,
and the number decides the hardening item (a buffered-semaphore wait as errgroup's would wake one caller per
release).

### Tests (written, not run)

`group/collect_test.go`: `ExampleGroup_SetCollect` (two failures under `SetLimit(1)`, Output is Join's two-line
message), `TestGroup_collectJoinsEveryError` (three failures, `errors.Is` each, cause is the first),
`TestGroup_collectIncludesPanics`, `TestGroup_collectIncludesErrZeroLimit`, `TestGroup_collectNoErrorIsNil`,
`TestGroup_collectDefaultUnchanged` (zero and `SetCollect(false)`: the first error itself, `==`),
`TestGroup_collectOrderIsCompletionOrder` (serialised by `SetLimit(1)`: a failing function's error is recorded
before its slot frees, so the order is forced; with no limit the order is whichever function returned first and
is not observable deterministically from outside), `TestGroup_collectSetLateKeepsEarlierFirstError`,
`TestGroup_bootcampByValueWaitGroupLosesErrors` (Bootcamp `waitgroups/error.go:8-31`), `ExampleWithCollect`,
`TestResults_withCollectKeepsPartialValues`, `TestResults_withCollectZeroValueInFailedSlot` (the failed slot and
the not-started slot), `TestResults_withCollectPanic`, `TestResults_defaultUnchanged`,
`TestResults_thinktankOrderKept` (thinktank.go:33-58), `BenchmarkGroup_Go_blockedCallers`. `leak.Verify(t)` in
every test; every wait bounded by `bounded(t)` (10 s). `compare/group/group_test.go`:
`TestCompare_collectVersusErrgroup` (errgroup returns the first error only; ours with `SetCollect(true)` the join).

### For the lead

- INDEX rows: TB-GROUP-008 `Group.SetCollect` (`group/collect.go`, doc.go lists it as `Group.SetCollect`, the pool `Pool.TrySubmit` precedent), TB-GROUP-009 `WithCollect` (`group/collect.go`).
- D-needed: first-error default kept against conc's join-by-default (above). `SetCollect` as a method (not a
  `WithContext` option). No `SetMaxErrors` for Group (bounded by the number of Go calls).
- Assumption: a single collected failure is returned wrapped by `errors.Join` (uniform type; `errors.Is` matches).
- Reviewer fix (2026-11-13): `Wait` no longer compares error values (`errs[0] != err` would panic on an uncomparable
  error type such as `type errList []error`); `fail` records whether the first error was kept (`kept` bool).
  `TestGroup_collectUncomparableErrorDoesNotPanic` pins it with and without `SetCollect`.
- doc/sources: pkg.go.dev `errors` "func Join" (go1.27), used by TB-GROUP-008/009.
- Out of scope now also: `Results.Each() iter.Seq2[T, error]` (per-slot form), `Group.SetMaxErrors`.
- Checks run: `gofmt -l group compare/group`, `go build ./group/...`, `go vet ./group/...`. `go test` not run.

### Measured (VERIFY, 2026-11-13, go1.27.3 darwin/arm64, -race on; raw in doc/align_2026-11-13/out/bench_new.txt)
BenchmarkGroup_Go_blockedCallers (SetLimit(1), 1 000 Go calls blocked behind one slow fn) 514 ms/op, 5 310 allocs/op, about 5 allocs per blocked Go; the `freed` close-broadcast herd is in that number (every release wakes every waiter): the hardening item has its baseline. BenchmarkGroup_Go 26 906 ns/op, 9 allocs/op.
