# singleflight: duplicate call suppression, generic and ctx-aware (panics returned as errors)

Status: SHIPPED (2026-11-13)
Package: `singleflight` · SHORT: `FLIGHT` · Layer: core · Tier: T3
Work folder: `doc/wave5_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #25
Builds on: `panics` (TB-PANIC-001 ErrPanic, TB-PANIC-004 TryValue) · Decisions: D-017 (model) · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; reading list only, never cited)

No code-heim repo shows the pattern. go_45 (cache) implies it: a miss on a hot key lets every concurrent request
load the same value (a cache stampede). Not opened in this wave; the stampede is the motivating case only
(`ExampleGroup`, `TestGroup_Do_suppressesDuplicates`).

### b. The authority (opened 2026-11-13)

- pkg.go.dev `golang.org/x/sync/singleflight`, latest **v0.25.0 (Oct 15, 2026)**. Opened:
  https://pkg.go.dev/golang.org/x/sync/singleflight, 2026-11-13. Relied on:
  - `Group`: "represents a class of work and forms a namespace in which units of work can be executed with
    duplicate suppression."
  - `Do`: "making sure that only one execution is in-flight for a given key at a time. If a duplicate comes in,
    the duplicate caller waits for the original to complete and receives the same results. The return value
    shared indicates whether v was given to multiple callers."
  - `DoChan`: "like Do but returns a channel that will receive the results when they are ready. The returned
    channel will not be closed."
  - `Forget`: "Future calls to Do for this key will call the function rather than waiting for an earlier call to
    complete."
- The source, v0.25.0. Opened: https://raw.githubusercontent.com/golang/sync/v0.25.0/singleflight/singleflight.go,
  2026-11-13; line numbers below are from the same file pinned in the module cache
  (`golang.org/x/sync@v0.25.0/singleflight/singleflight.go`, the version compare/go.mod requires):
  - `:20` `errGoexit` (unexported): the marker for a Goexit in fn.
  - `:96-106` `Do` for a duplicate: `c.dups++`, `c.wg.Wait()`, then **`panic(e)` if the call panicked (`:101-102`)
    and `runtime.Goexit()` if fn called Goexit (`:103-104`)**: x/sync re-raises both in every caller.
  - `:113-114` the first caller runs fn **inline on its own goroutine** (`g.doCall`); `shared` is `c.dups > 0`.
  - `:144-205` `doCall`: the double defer that tells a panic from a Goexit ("use double-defer to distinguish
    panic from runtime.Goexit, more details see https://golang.org/cl/134395", `:148-149`); `:192-194` the
    recover makes `newPanicError` (value + stack, first stack line trimmed, `:43-53`).
  - `:163-171` a panic with DoChan waiters: **`go panic(e)` then `select {}` (`:166-168`)**, "to prevent the
    waiting channels from being blocked forever, needs to ensure that this panic cannot be recovered": the process
    crashes. Without DoChan waiters: `panic(e)` (`:170`) in the first caller.
  - `:159-161` the finished call is deleted only if `g.m[key] == c` (so Forget + a new call is safe); `:210-214`
    `Forget` is a plain delete under the mutex.
- pkg.go.dev `context`: `WithoutCancel` (go1.21: "returns a derived context that points to the parent context and
  is not canceled when parent is canceled ... no Deadline or Err"), `WithCancelCause`/`Cause` (go1.20).
- pkg.go.dev `runtime`, `Goexit`.

### c. Today's Go and why the export is still needed

The standard library has no singleflight (an internal copy lives in `internal/singleflight` for net's resolver,
not importable). x/sync's is `string` keys and `any` values (a type assertion on every result), has no ctx (a
waiter cannot give up; the shared fn cannot be cancelled when nobody wants the result any more), re-panics in
every caller and kills the process with a DoChan waiter (rule 6: a panic in caller code on our goroutine becomes
an error), and calls Goexit in every caller. Ours: type parameters, ctx on Do/DoChan, ErrPanic, ErrGoexit.

### d. calcbench

None. Nothing mathematical; the map is a Go map under the Group's mutex.

### e. Exports

| ID | Name | Signature |
|---|---|---|
| `TB-FLIGHT-001` | `Group` | `type Group[K comparable, V any] struct{...}`; methods `Do(ctx context.Context, key K, fn func(context.Context) (V, error)) (v V, shared bool, err error)`, `DoChan(ctx context.Context, key K, fn func(context.Context) (V, error)) <-chan Result[V]`, `Forget(key K)` |
| `TB-FLIGHT-002` | `Result` | `type Result[V any] struct{ Val V; Err error; Shared bool }` |
| `TB-FLIGHT-003` | `ErrGoexit` | `var ErrGoexit = errors.New("singleflight: runtime.Goexit was called in the shared function")` |

Options: none (no bound to set: one goroutine per in-flight key, and the keys in flight are bounded by the callers
waiting on them). Sentinel errors: `ErrGoexit` · `panics.ErrPanic` (from `panics`, not re-declared).

**Deviations from LADDER.md §4 / the BRIEF B2 shape, and from x/sync** (each argued):
1. **`ErrGoexit` is added** (exported, ID'd): callers need to match it; x/sync's `errGoexit` is unexported because
   it never reaches a caller (it calls Goexit instead).
2. **Do's results are `(v, shared, err)`** (BRIEF order; x/sync is `(v, err, shared)`): error last, rules
   `go-functions.md` "Signatures".
3. **DoChan's channel is closed** after its one Result (rule 3, the creator closes; x/sync never closes it). Buffer
   1: a caller that never reads blocks nothing.
4. **A panic in fn is `*panics.PanicError` to every caller** (x/sync re-panics; with DoChan waiters it crashes the
   process from an unrecoverable goroutine). The same `*PanicError` value is shared by all callers. As D-017 for
   errgroup: the stack is kept in `PanicError.Stack`.
5. **Goexit in fn is `ErrGoexit` to every caller** (x/sync calls Goexit in each caller). fn runs on the Group's
   goroutine, not the caller's, so ending the callers' goroutines would be wrong.
6. **fn always runs on its own goroutine** (x/sync: the first Do caller runs it inline). Required so that the
   first caller can leave on ctx while the call continues for the others.

## Decisions (BRIEF B2 "decide and state")

- **Which ctx the shared fn runs with**: `context.WithCancelCause(context.WithoutCancel(firstCallerCtx))`: the first
  caller's **values** (trace ids, auth) are kept, its **cancellation and deadline are not** (one caller's timeout
  must not fail the others). It is cancelled when **every caller has left** (refcount of waiters reaches 0), with
  the **last leaver's cause**, and released (`cancel(nil)`) when fn returns. Alternative that lost: the first
  caller's ctx as is (x/sync-with-ctx libraries often do this): a short-deadline first caller fails every
  joiner with its own deadline. A per-call timeout is the caller's (wrap fn with `context.WithTimeoutCause`).
- **A waiter whose ctx ends** returns `context.Cause(ctx)` at once, zero V, shared false; the call continues for
  the others. If fn had already returned when the waiter looked (decided under the mutex), the waiter gets the
  result instead (no lost result).
- **The last waiter to leave** removes the call from the map (a later caller starts a new call, never joins a
  cancelled one), cancels fn's ctx with its cause, and **waits for fn to return** before it returns. This keeps
  rule 2 without a new exception: no goroutine outlives every caller of its call. The cost: the last leaver is
  as prompt as fn is in honouring its ctx (the same contract as `group.Wait`). Alternative that lost: return at
  once and abandon the goroutine (a D-027-like exception, which the BRIEF forbids without a lead decision).
- **Ctx done before Do/DoChan**: fn is neither started nor joined; the cause is returned (DoChan: a closed channel
  holding it; no goroutine).
- **shared** = more than one caller ever joined the call (x/sync's `dups > 0`), including a caller that left.
- **A shared result is shared memory**: every caller gets the same V (a pointer, slice or map aliases); documented
  on `Group` and `Result`: do not mutate.
- **Forget during a call**: the call in flight is not cancelled and still delivers to its callers; the next caller
  starts a new call; the old call, when it finishes, does not delete the new one (`g.m[key] == c` check, as x/sync).
- **nil ctx** = `context.Background()`; **nil fn** panics when called inside `panics.TryValue` → `ErrPanic`.
- **Zero value** ready (map made lazily); not copied after first use (vet copylocks).

## Per export

### `TB-FLIGHT-001` `Group` (`singleflight/group.go:23`; Do :56, Forget :70; DoChan `do_chan.go:24`)
- **Pattern**: duplicate call suppression (singleflight).
- **Authority**: pkg.go.dev golang.org/x/sync/singleflight v0.25.0, "Group", "Group.Do", "Group.DoChan", "Group.Forget".
- **Familiar shape**: x/sync `Group` method for method; deviations above.
- **Goroutines**: one per call (not per caller) running fn: it ends when fn returns, panics (recovered) or calls
  Goexit; it has exited when the last caller of that call returns. DoChan adds one per DoChan caller that waits
  and sends the Result; it exits after the send (channel closed).
- **Cancellation**: per caller (above); fn's ctx cancelled only when every caller has left.
- **Errors**: fn's error unchanged to every caller; `ErrPanic`; `ErrGoexit`; a leaving caller's own cause.
- **Blocking**: Do until fn returns or ctx ends (last leaver: until fn returns). DoChan never blocks.
- **Ordering and counts**: fn runs at most once at a time per key (per Forget generation); exactly one Result per
  DoChan.
- **Zero value**: ready.
- **Clock**: none (never waits on time).
- **calcbench IDs called**: none.
- **Compared against**: golang.org/x/sync/singleflight v0.25.0, `compare/singleflight/singleflight_test.go`.
- **Worked example**: `ExampleGroup` (`42 true 42 true 1`), `ExampleGroup_Do`, `ExampleGroup_DoChan`, `ExampleGroup_Forget`.

### `TB-FLIGHT-002` `Result` (`singleflight/do_chan.go:13`)
x/sync's `Result` with `Val V`. Zero value: an empty result. Example `ExampleResult`.

### `TB-FLIGHT-003` `ErrGoexit` (`singleflight/errors.go:14`)
Returned to every caller when fn called `runtime.Goexit`. Example `ExampleErrGoexit`.

## 1g. Comparison (`compare/singleflight/singleflight_test.go`, written, not built or run)

Same inputs to x/sync and ours: N concurrent Do on one key (one run, all shared), one caller (not shared), error
passthrough, DoChan sharing + Forget during a call (new run, old callers still served). Differences shown on each
side: panic in Do (x/sync re-panics in the caller, recovered by the test; ours ErrPanic), Goexit in Do (x/sync ends
the caller's goroutine; ours ErrGoexit). The x/sync DoChan panic case is not run (it crashes the test binary).

## Tests

`group_test.go`, `do_chan_test.go`, `example_test.go`: Example per export with `// Output:`; N concurrent Do on one
key run fn once (atomic counter, released by a channel, joins proven with `synctest.Wait`); shared flag; sequential
calls run again; different keys in flight together; error shared; ctx done before Do / DoChan; waiter cancel
(starter leaves, joiner leaves) returns its cause while the other gets the result and fn's ctx is not cancelled;
all leave → fn's ctx cancelled with the last cause, the last caller returns after fn, the next Do runs anew; a new
caller during the last leaver's wait starts a new call; fn ctx keeps values, drops deadline; panic → the same
ErrPanic to every waiter (Do and DoChan); nil ctx, nil fn; Goexit → ErrGoexit and the key is freed; DoChan joins
before returning, shares with Do, channel closed, cancel mid-call; Forget during a call; zero value and many keys
race-free; `leak.Main` + `leak.Verify`. No `time.Sleep`; joins and blocking proven by `testing/synctest`.
`BenchmarkGroup_Do` with `b.ReportAllocs()`.

## 1h. Out of scope

Caching the result after the call (that is `cache.GetOrLoad`, T4, built on this). A per-call timeout (the caller
wraps fn). Result expiry / negative caching (`cache`). Distributed (cross-process) suppression (`lock`).

## For the lead

- INDEX.md rows TB-FLIGHT-001..003; taxonomy row `singleflight` → built.
- doc/sources: pkg.go.dev x/sync/singleflight v0.25.0 and the raw source URL above (lines cited); pkg.go.dev
  context (`WithoutCancel`), runtime (`Goexit`).
- DECISIONS candidates: the fn-ctx rule (WithoutCancel + cancel when all leave, last leaver waits for fn);
  deviations 3-5 (DoChan closed, ErrPanic not re-panic, ErrGoexit not Goexit), D-017-style finding: x/sync
  singleflight v0.25.0 DOES re-panic (unlike errgroup since v0.16.0) and crashes the process with DoChan waiters.
- compare/go.mod already requires `golang.org/x/sync v0.25.0`; `compare/singleflight/` written, not built or vetted.
- Checks run: `gofmt -l singleflight` clean, `go build ./...` ok, `go vet ./singleflight/` clean.

REMAINING: none (verify phase: `go test -race -timeout 5m -count=200 ./singleflight/`, compare once tidied).
