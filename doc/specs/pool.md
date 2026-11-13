# pool: a long-lived worker pool (competing consumers, bounded parallelism, backpressure)

Status: SHIPPED (2026-11-13)
Package: `pool` · SHORT: `POOL` · Layer: core · Tier: T2
Work folder: `doc/wave3_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #18
Builds on: `group` (TB-GROUP-001 Group, TB-GROUP-002 WithContext), `panics` (TB-PANIC-001 ErrPanic, TB-PANIC-003
Try) · `semaphore`: not used (below) · Decisions: none new · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; reading list only, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `_reference/code-heim/go_18_worker_pool/worker_pool.go` | `WorkerPool{Tasks []Task; concurrency int}`; `Run` makes `chan Task` of `len(Tasks)` (`:58`), starts `concurrency` workers (`:61-63`), `wg.Add(len(Tasks))` and `wg.Done()` per task (`:51-53,66`). | **Queue sized to the input** (`:58`): memory grows with the input, no backpressure → `TestPool_defectGo18_queueBoundedNotSizedToInput`. **`Task.Process()` has no ctx and no error** (`:10-12`): failures are lost, nothing can be cancelled → `TestPool_defectGo18_errorAndCtxReachTheCaller`. No panic recovery (a panicking task kills the process) → `TestPool_panicCollectedAsErrPanic`. WaitGroup per task instead of per worker; `for i := 0; i < n; i++` (go1.22 `range n`); `fmt.Printf` in tasks; one-shot (`Run` over a fixed slice, not values arriving over time). |
| `_reference/code-heim/go_66_tunny/simple/main.go` | `tunny.NewFunc(numCPUs, func(payload interface{}) interface{})` (`:22`), 100 goroutines each calling `pool.Process` (`:48-60`). | `interface{}` payload + chains of type assertions (`:23-41`): generics (`Pool[T]`, compile-time typed; no test needed). **Polls `QueueLength()` with `time.Sleep`** (`:63-71`) and sleeps 3 s more (`:73`) to guess completion → `TestPool_defectGo66_closeWaitsWithoutPolling`. A goroutine per value to call a blocking `Process` (unbounded goroutines in the caller). |
| `_reference/code-heim/go_66_tunny/with_states/main.go` | `tunny.New(numCPUs, func() tunny.Worker { return myWorker{} })`. | **Stateful worker with value receivers** (`Process`, `BlockUntilReady`, `Interrupt`, `Terminate` on `myWorker`, not `*myWorker`): every state write is on a copy and lost → `TestPool_defectGo66_valueReceiverLosesState`. Sleep + `QueueLength` polling again. |
| `_reference/code-heim/go_67_conc/pool/main.go`, `pool_with_errors/main.go`, `pool_with_context/main.go` | conc `pool.New().WithMaxGoroutines(10)` (`:19`), `.WithErrors()`, `.WithContext(ctx)`; `Go`/`Wait`. | conc **re-raises a task panic from `Wait`** on the caller's goroutine → `TestPool_defectGo67_panicReturnedNotReraised`. `time.After` in a select per task (`pool_with_context/main.go:45`). conc is a fork-join group (Go/Wait over a known set), not a long-lived pool; in toolbench that shape is `group`. |

### b. The authority (opened 2026-11-13)

- Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, section **"Bounded
  parallelism"**. Opened: https://go.dev/blog/pipelines, 2026-11-13. Relied on: "Start a fixed number of
  goroutines to read and digest files" (fixed worker count reading one shared channel; workers end when the
  channel is closed, `for path := range paths`); the results side is closed after `wg.Wait()`. From
  "Explicit cancellation" (the guidelines): "stages close their outbound channels when all the send operations
  are done" and "stages keep receiving values from inbound channels until those channels are closed or the
  senders are unblocked". Ours: New starts the fixed set; Close (the only sender's owner) closes the queue once,
  after the last accepted send; workers range until it is closed and empty; a cancelled pool unblocks every
  sender (Submit selects on the pool's ctx and on Close's quit channel).
- Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, **"Competing Consumers"**. Opened:
  https://www.enterpriseintegrationpatterns.com/patterns/messaging/CompetingConsumers.html, 2026-11-13. Relied
  on: problem "[an application] cannot process messages as fast as they're being added to the channel";
  solution "Create multiple Competing Consumers on a single channel so that the consumers can process multiple
  messages concurrently"; each message is received by exactly one of the consumers. Ours: one Go channel, N
  workers receiving from it; a value is received by exactly one worker (`TestPool_exactlyOnce`).
- pkg.go.dev `golang.org/x/sync/errgroup` v0.25.0 "func WithContext" (first error cancels the derived ctx): the
  WithFailFast rule, inherited from `group` (opened for doc/specs/group.md, same day).
- pkg.go.dev `net/http` "ErrServerClosed": the standard library's after-close sentinel, the model for ErrClosed.

### c. Today's Go (GO_RELEASES.md)

- No standard-library worker pool exists in go1.27. `sync.WaitGroup.Go` (1.25) and `errgroup` cover fork-join
  over a known set, not values arriving over time with a bounded queue, backpressure and Close-drains.
- `testing/synctest` (1.25 GA) makes "Submit is blocked" a fact (synctest.Wait returns only when every bubble
  goroutine is durably blocked): used for the backpressure tests instead of timing.
- go1.22 per-iteration loop variables and `range n`; `testing.B.Loop` (1.24) in the benchmark.
- Why toolbench still needs `pool`: the repos show three defective hand-rolls (input-sized queue, no ctx/error,
  no panic recovery, polling) and the advo sites in LADDER.md row 18 (cleanup_worker.go:95,
  respiratory_reports.go:772, metabolic_reports.go:575) hand-roll the same thing.

### d. calcbench

None. The queue is a Go channel, not ALG-QUEUE-002: it must block the producer (backpressure) and be safe
across goroutines (LADDER.md §5 "Re-implemented on purpose"). Nothing in calcbench is re-implemented.

### e. Built on

- `group.WithContext` runs the workers: one `Go` per worker; with WithFailFast a worker returns the first error,
  the group records it and cancels the pool's ctx; `Close` returns `g.Wait()`. A worker that calls
  `runtime.Goexit` (through fn) is replaced by a deferred `g.Go` (allowed from a group goroutine).
- `panics.Try` wraps every fn call, so a panic is a `*panics.PanicError` and the worker survives it (default) or
  returns it (WithFailFast). The group's own `Try` is the backstop.
- `semaphore` is **not used**: the worker count already bounds concurrency and the channel's capacity bounds
  the queue; a semaphore would add a second bound that no export asks for. If a weighted bound per value is
  ever wanted it is a new option built on `semaphore`.

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-POOL-001` | `Pool` | `type Pool[T any] struct{...}`; methods `Submit(ctx context.Context, v T) error`, `Close() error` |
| `TB-POOL-002` | `New` | `func New[T any](ctx context.Context, workers int, fn func(context.Context, T) error, opts ...Option) (*Pool[T], error)` |
| `TB-POOL-003` | `Option` | `type Option func(*config)` |
| `TB-POOL-004` | `WithQueue` | `func WithQueue(n int) Option` |
| `TB-POOL-005` | `WithFailFast` | `func WithFailFast() Option` |
| `TB-POOL-006` | `ErrClosed` | `var ErrClosed = errors.New("pool: closed")` |
| `TB-POOL-007` | `ErrWorkers` | `var ErrWorkers = errors.New("pool: worker count must be positive")` |

Options:

| Option | Default | Bound it sets |
|---|---|---|
| `WithQueue(n int)` | `0`: direct hand-off; n < 0 is 0 | values waiting for a worker (channel capacity) |
| `WithFailFast()` | off: collect every error, keep going | first error cancels the pool |

Workers: the `workers` parameter, no default; `workers <= 0` is `ErrWorkers` (wrapped with the count), no
goroutine started.

**Deviations from LADDER.md §4**: none in the signatures. Added: `ErrClosed` and `ErrWorkers` (the brief asked to
decide Submit-after-Close and workers <= 0; both are sentinels so callers match them with errors.Is). The
taxonomy row says "Submit/Close, Process": no `Process` (tunny's synchronous request/response) is built; §4 has
none, and a request/response call is `group.Results` or `parallel.Map`.

## Per export

### `TB-POOL-001` `Pool` (with `Submit`, `Close`)
- **Pattern**: worker pool (competing consumers, bounded parallelism).
- **Authority**: Ajmani 2014 "Bounded parallelism"; Hohpe & Woolf 2003 "Competing Consumers".
- **Familiar shape**: `Submit`/`Close` as pond's `Submit`/`StopAndWait` and an `http.Server`'s accept-until-Close
  (`ErrClosed` as `ErrServerClosed`); fn is `func(ctx, T) error` as errgroup/conc's context pool. Difference from
  conc: long-lived workers and a bounded queue instead of a goroutine per task.
- **Goroutines**: exactly `workers`, started by New, run in a `group.Group`; each ends when the queue is closed
  and empty, or the pool's ctx is done, or (WithFailFast) after its fn returns an error. A worker that Goexits
  is replaced. All have exited when Close returns.
- **Cancellation**: the pool's ctx is New's ctx (with WithFailFast, cancelled also on the first error).
  *Before New*: workers exit at once; Submit returns `context.Cause(ctx)`; Close returns nil (nothing accepted).
  *Mid-stream*: each worker finishes its current fn call (fn sees the done ctx), then exits; at most one queued
  value per worker may still start (a worker can pass its ctx check just before the cancel and then receive a
  ready value; that fn runs with a done ctx; no re-check after receive, which would drop it uncounted); the
  rest of the queue is not run; Close returns `pool: N submitted values not run: <cause>` (wraps `context.Cause`).
  *After completion*: nothing to drop; Close returns nil. Submit's own ctx: done before the call or while
  blocked → `context.Cause(ctx)`, value not accepted.
- **Errors**: default: every fn error and recovered panic is kept (completion order) and Close returns them
  with `errors.Join` (plus the not-run error); WithFailFast: Close returns the first error itself (fn's error or
  `*panics.PanicError`), joined (errors.Join) with the not-run count when values were left in the queue, so
  `errors.Is` matches both and dropped work is never silent (D-022); with nothing left unrun it is the first
  error itself. Submit: `ErrClosed` after Close
  (also for the zero value and a nil *Pool), the ctx cause, or the pool ctx's cause (first error with
  WithFailFast). Close twice (or concurrently): every call returns the first call's result (sync.Once).
- **Blocking**: Submit blocks while every worker is busy and the queue (default 0) is full; a blocked Submit
  returns on its ctx, the pool's ctx, or Close. Close blocks until the workers have run every accepted value and
  exited; it must not be called from fn (it would wait for itself); fn must not Submit to its own full pool.
- **Ordering and counts**: each accepted value (Submit returned nil) is run exactly once, unless the pool's ctx
  ends first (then it is counted in Close's error: never a silent drop). With one worker, values run in
  submission order (FIFO channel); with more, in no particular order.
- **Zero value**: not usable: Submit returns ErrClosed, Close returns nil. Create with New. Not copied (mutexes).
- **Concurrency guard**: `mu` (RWMutex): Submit holds it for reading while it may send; Close closes `quit`
  first (waking blocked Submits), then takes it for writing to close the queue, so no send on a closed channel.
  `errMu` guards `errs`. Workers are the only receivers.
- **Compared against**: github.com/sourcegraph/conc/pool v0.3.0, `compare/pool/pool_test.go` (same work and bound,
  joined errors, first error, panic re-raise vs ErrPanic). tunny and pond/v2 named, not compared (other shapes).
- **Worked example**: `ExamplePool` (`<nil> 55`), `ExamplePool_Submit`, `ExamplePool_Close` (`true 2`).

### `TB-POOL-002` `New`
- Starts the workers; `workers <= 0` → `ErrWorkers` wrapped with the count, nil pool, no goroutine. nil ctx →
  `context.Background()`. nil fn → each value panics in fn and returns `ErrPanic` through Close (as `group`'s
  nil fn). A nil Option is skipped. Never blocks. Example: `ExampleNew`.

### `TB-POOL-003` `Option`, `TB-POOL-004` `WithQueue`, `TB-POOL-005` `WithFailFast`
- Functional options; table above. Examples: `ExampleOption`, `ExampleWithQueue` (`<nil> 16`),
  `ExampleWithFailFast` (`bad value 2`: one worker stops at the error, the next two Submits are rejected).

### `TB-POOL-006` `ErrClosed`, `TB-POOL-007` `ErrWorkers`
- Sentinels; Examples `ExampleErrClosed`, `ExampleErrWorkers`.

## Tests (`pool/pool_test.go`, `pool/example_test.go`)

`TestMain` with `leak.Main`; `leak.Verify(t)` in every test; bounded by `context.WithTimeout(t.Context(), 10s)` or
a synctest bubble; no `time.Sleep`, no polling. Happy path table (0, 1, many values, workers = 1, queue larger
than input) · exactly once (1000 values) · one-worker submission order · max in flight == workers (atomics) ·
Submit blocks when full, Submit ctx cancelled while blocked, Close unblocks a blocked Submit (synctest) · ctx done
before Submit · Close drains the queue · Submit after Close · Close twice same result · zero value and nil *Pool ·
workers 0/-1/-2^30 · nil ctx, nil Option, negative queue · nil fn → ErrPanic · collect errors · panic collected
(worker survives) · fail-fast panic · fail-fast first error cancels (Submit returns its cause) · fail-fast reports unrun values · fail-fast stops
other workers · cancel before New / mid-stream (count + cause) / after completion · Goexit replaces the worker ·
concurrent Submit and Close (ran == accepted) · defects go_18 ×2, go_66 ×2, go_67 · `BenchmarkPool_Submit` with
ReportAllocs. Each is deterministic under `-count=200`: every "is blocked" is a synctest fact or a channel
handshake, never a timing.

## Out of scope

Dynamic resizing (pond), per-worker state with lifecycle hooks (tunny's `Worker`: put state in fn's receiver),
a synchronous request/response `Process` (use `group.Results` or `parallel.Map`), result channels (use `flow`),
priorities, retries, delays and dead letters (`jobs`, T5), metrics hooks (a later additive option).

## For the lead

- INDEX.md rows: TB-POOL-001..007 (doc.go lists them). `doc/sources`: go.dev/blog/pipelines ("Bounded
  parallelism") and EIP "Competing Consumers" now used by TB-POOL-001/002/004; Pike 2014 "Self-referential functions and the design of options" by
  TB-POOL-003; pkg.go.dev errors "Is" by TB-POOL-007; errgroup "func WithContext"
  by TB-POOL-005; net/http "ErrServerClosed" by TB-POOL-006.
- compare/go.mod already requires github.com/sourcegraph/conc v0.3.0; no new module needed for compare/pool.
- ASSUMPTIONS: (1) workers <= 0 is an error (`ErrWorkers`), not a GOMAXPROCS default, because New already
  returns an error and a silent default hides a bug; (2) collect mode keeps every error (as conc and
  errors.Join): memory is bounded by the number of values the caller submits; a cap would be a later option;
  (3) on mid-stream cancel queued values are dropped and counted in Close's error rather than run with a done
  ctx; (4) with WithFailFast, Close returns the first error joined with the not-run count when there is one (reviewer finding 2, D-022).
- Taxonomy row 18 mentions "Process": not built (see Deviations).

REMAINING: none in code; tests and compare/ not run (by rule); compare/ not built or vetted (by rule).

## Align (2026-11-13)

Work folder: `doc/align_2026-11-13/` · MATRIX.md list (b) rank 5, section 6 (pool row) · task D3 (Devin) plus the
scale clause. Additive only: no released signature changed.

### Matrix rows
| Pattern | IDs | REPEAT | Verdict |
|---|---|---|---|
| worker pool: Submit / Close | TB-POOL-001/002/006/007 | 3/2/5 | SAME; item 1 is a test (Close already `g.Wait`s, pool.go Close) |
| bounded queue; fail-fast | TB-POOL-004; TB-POOL-005 | 1/2/3 each | SAME; nothing new |
| progress / completion hook | none -> TB-POOL-010 | 0/1/1 | MISSING, built (D3) |
| scale: collect-mode `errs` unbounded | -> TB-POOL-011 | - | bounded by option, default unchanged |

### References read (reading list, never cited)
- Git-Secrets-Scanner `internal/progress/task.go:118-157`: `Wait` polls with `time.Sleep(100ms)` and returns on
  `quit` before the workers exit -> `TestPool_closeReturnsOnlyAfterEveryWorkerExited`; `:129-136` silently drops a
  value when the queue is full -> `ErrQueueFull` is returned, never a drop; `progressbar.go:60` reads
  `TasksDoneCount` without the lock -> the hook receives the counts as arguments.
- any-website-sitemap-crawler `main.go:62-92`, guardian-scraper `main.go:119`: a worker re-feeds its own worklist
  (crawler frontier) -> Submit from inside a worker is non-blocking.

### Authority (opened)
- The Go Programming Language Specification, "Select statements" (opened locally: `$(go env GOROOT)/doc/go_spec.html`,
  go1.27.3, 2026-11-13): "if there is a default case, that case is chosen" when no communication can proceed. Relied
  on for the non-blocking send behind `ErrQueueFull` and `TrySubmit`.
- Ajmani, "Go Concurrency Patterns: Context", The Go Blog, 2014, "Example: Google Web Search" and pkg.go.dev
  `context` "func WithValue" (an unexported key type, the context documentation's rule): the worker marker is a request-scoped value under
  a key private to this package.
- Gamma et al., Design Patterns, 1994, "Observer" (not opened; named in MATRIX for the hook; the shape mirrors
  bulkhead `WithOnEvent` TB-BULK-011).
- pkg.go.dev `errors`, "func Join" (go1.27): the kept errors and the summary are joined so `errors.Is` sees both.

### Today's Go
go1.27.3: no standard-library pool; `context.WithValue` chains, `sync/atomic` typed counters, `testing/synctest`
(1.25 GA) makes "Close has not returned" and "the worker is blocked in Submit" facts, not timings.

### calcbench
None (counters and a bounded slice; no formula).

### New exports (IDs TB-POOL-008..011; the highest before was 007)
| ID | Name | Contract |
|---|---|---|
| TB-POOL-008 | `ErrQueueFull` | `var ErrQueueFull = errors.New("pool: queue full")`; returned (wrapped with the queue size) by Submit called from inside a worker of the same pool and by TrySubmit when the queue has no room (queue 0: no worker is waiting). The value was not accepted and is never run: the caller handles the overflow. No goroutine, never blocks. |
| TB-POOL-009 | `Pool.TrySubmit(v T) (ok bool, err error)` | non-blocking Submit from anywhere: (true, nil) when a worker or the queue took v; (false, ErrQueueFull wrapped) when full; (false, ErrClosed) after Close, for the zero value and a nil *Pool; (false, context.Cause of the pool's ctx) when the pool stopped. Never blocks, no goroutine. |
| TB-POOL-010 | `WithOnDone(f func(done, total int)) Option` | f runs after each value's fn returns (success, error or panic) on that worker's goroutine, outside every lock, with the pool's own atomic counters: done = values finished so far (this one included), total = values accepted so far (a Submit in flight is counted a moment before it returns, so done <= total always holds). A panic in f is recovered and recorded as that value's error (panics.ErrPanic, joined with fn's error when both). Not called for a value Submit refused or for a value left unrun by a cancelled pool. f must not call Close (it would wait for itself) and must be safe for concurrent use. Default and nil f: no hook. |
| TB-POOL-011 | `WithMaxErrors(n int) Option` | collect mode keeps the first n fn errors (completion order), counts the rest and remembers the last dropped one; Close joins the kept ones with one error `pool: N further errors not kept: <last>` that wraps the last dropped error (errors.Is still matches it) when any were dropped. Default, and n <= 0: keep all (today's behaviour, unchanged). No effect with WithFailFast (only the first error is ever kept). |

Behaviour change in Submit (documented, additive, no pinned test changed): fn receives a ctx that carries a private
marker naming its pool (`workerKey{}`, an unexported key type holding a `*poolID`; `poolID` has a byte so two pools never share an
address). `Submit(ctx, v)` with a ctx descended from that worker ctx of the SAME pool never blocks: it enqueues when
there is room and otherwise returns `ErrQueueFull`. From outside (or with context.Background() from inside a
worker) Submit blocks as before and the old caveat holds (a blocked in-worker Submit is released by Close with
ErrClosed: `TestPool_submitFromWorkerBackgroundCtxBlocksUntilClose`). A ctx from another pool's worker is "outside".

### Scale note
- Bounded memory: yes. The queue is `cap(tasks)` (unchanged); collect-mode errors are now `WithMaxErrors(n)`
  entries + one int + one error (default unchanged: one entry per failure, bounded by the submits). The counters
  are two `atomic.Int64`. The worker ctx is one `context.WithValue` made once in New (one allocation per pool,
  shared by every worker, immutable).
- Contention point: unchanged (`mu.RLock` per Submit, the channel lock, `errMu` per kept or dropped error). The
  hook runs outside every lock; a caller that prints inside it serialises its workers on stdout (its choice).
- Per-call allocations, read from the code (measured numbers are under "Measured" below): Submit from outside
  allocates nothing (unchanged; the marker check `ctx.Value(workerKey{})` at pool.go:151 is one interface call,
  the key `workerKey{}` is zero-size, pool.go:15-21); Submit from a worker allocates nothing (the wrapped
  ErrQueueFull is made once in New, `errFull`); TrySubmit allocates nothing. With WithOnDone set, the loop wraps
  each hook call in `panics.Try(func() error {...})` (pool.go, loop): by construction that is one closure per
  value, a statement from the code, not a measurement (no hook benchmark exists); without a hook, nothing.
- At 10x callers: as before, Submit serialises on the channel (backpressure by design). At 100x: the first thing
  to break was the unbounded `errs` slice when most values fail: now capped by WithMaxErrors; next is the RWMutex
  writer wait in Close while readers hold RLock. From the code: Close holds the write lock only to set `closed`
  and close the queue (pool.go, Close), and every reader returns promptly once `quit` is closed; how long that
  wait is under 100x readers is not measured.
- `BenchmarkPool_Submit` (kept) and `BenchmarkPool_SubmitFromWorker` (worker ctx, non-blocking path), ReportAllocs;
  the numbers the lead measured are under "Measured" (below): 0 allocs/op on both, 0.02 full/op.

### Tests added (`pool/pool_test.go`, `pool/example_test.go`)
`TestPool_closeReturnsOnlyAfterEveryWorkerExited` (Git-Secrets-Scanner task.go:118-157), `TestPool_submitAfterCloseIsErrClosed`
(renamed from `TestPool_submitAfterClose`), `TestPool_submitFromWorkerWithQueueDoesNotDeadlock` (queue 4, binary
re-feed to 255 values, overflow handled inline), `TestPool_submitFromWorkerWithoutQueueReturnsErrQueueFull`,
`TestPool_submitFromWorkerFullQueueReturnsErrQueueFull`, `TestPool_submitFromWorkerBackgroundCtxBlocksUntilClose`,
`TestPool_submitFromOtherPoolWorkerBlocks`, `TestPool_trySubmit`, `TestPool_onDoneCountsExact`,
`TestPool_onDonePanicIsErrPanic`, `TestPool_onDoneNotCalledForRefusedValue`, `TestPool_maxErrorsKeepsNAndCounts`,
`TestPool_maxErrorsDefaultKeepsAll`; examples `ExampleErrQueueFull`, `ExamplePool_TrySubmit`, `ExampleWithOnDone`,
`ExampleWithMaxErrors`. All with `leak.Verify(t)`, bounded (10 s ctx or synctest), no Sleep.

### For the lead
- INDEX rows: TB-POOL-008 ErrQueueFull, TB-POOL-009 Pool.TrySubmit (a METHOD with its own ID: the generator names
  it `Pool.TrySubmit`; doc.go lists it that way; no precedent in the repo, decide if a method may carry an ID),
  TB-POOL-010 WithOnDone, TB-POOL-011 WithMaxErrors.
- ASSUMPTIONS: (5) `total` in the hook counts a Submit in flight before it returns (so done <= total always);
  (6) WithMaxErrors n <= 0 means keep all; (7) the hook's panic is joined to fn's error for the same value.
- DECISION (D-needed): the "same pool" test is by ctx marker, so a worker that passes context.Background()
  to Submit keeps the blocking form; the alternative (a goroutine-id check) is not possible in Go and a
  per-pool flag could not tell the worker's goroutine from another; it lost.
- doc/sources: go_spec "Select statements" (new file or a row in go_standard_library.md) used by TB-POOL-008/009;
  go_blog_context.md + go_standard_library.md (context.WithValue with an unexported key) for the marker; gamma_1994_design_patterns.md "Observer" by TB-POOL-010;
  go_standard_library.md errors "func Join" by TB-POOL-011.
- pool does NOT import `ctxkey`: the first build did (`ctxkey.Key[*poolID]`), and go1.27.3 crashed with an internal compiler error compiling pool's test binary (`ctxkey/key.go: internal compiler error: ... nil pointer dereference`, both with and without -race, while `go build`/`go vet` passed); the lead replaced it with a plain unexported key type during VERIFY (align run 2026-11-13). LEFT FOR DEVIN: a minimal repro for a Go issue.

### Measured (VERIFY, 2026-11-13, go1.27.3 darwin/arm64, -race on; raw in doc/align_2026-11-13/out/bench_new.txt)
BenchmarkPool_Submit 3 060 ns/op, 0 allocs/op; BenchmarkPool_SubmitFromWorker 2 614 ns/op, 0 allocs/op, 0.02 full/op (the in-worker path allocates nothing and never blocks). Note: the benchmark loop is `for range b.N`, not `b.Loop`, because go1.27.3 crashes with an internal compiler error (escape analysis, iter.go:223) on that b.Loop body; LEFT FOR DEVIN.
