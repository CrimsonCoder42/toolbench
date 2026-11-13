# lifecycle: run group with ordered start, reverse-order stop and a drain deadline

Status: SHIPPED (2026-11-13)
Package: `lifecycle` · SHORT: `LIFE` · Layer: core · Tier: T2
Work folder: `doc/wave4_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #19
Builds on: `clock` (TB-CLOCK-001 Clock, TB-CLOCK-002 Timer; tests TB-CLOCK-005 Fake), `panics` (TB-PANIC-001
ErrPanic, TB-PANIC-003 Try, TB-PANIC-004 TryValue) · `group`: not used (below) · Decisions: none new ·
Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; reading list only, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `_reference/code-heim/go_42_faktory/worker/main.go` | `mgr.ShutdownTimeout = 25 * time.Second` (`:49`), `mgr.Run()` "does not return" (`:58-59`). | The deadline abandons in-flight jobs without saying which; the signal handling is hidden inside the manager → `TestGroup_defectGo42_shutdownTimeoutNamesTheAbandoned` (the error names who did not stop and who was never stopped). `time.Sleep` in jobs (`:31`). |
| `_reference/code-heim/go_37_background_work/*/process_jobs/worker.go` | `signalChan := make(chan os.Signal, 1)`; `signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)`; `<-signalChan`; `pool.Stop()` (simple_jobs `:37-42`, middleware_and_context and checkin `:74-79`). | Hand-rolled signal channel (§7: `signal.NotifyContext` since go1.16, cause names the signal since go1.26). **`pool.Stop()` has no deadline**: a stuck job hangs the process until SIGKILL → `TestGroup_defectGo37_signalThenBoundedStop` (signal through a test hook, Stop bounded by the grace period). One component, no order. gocraft/work unmaintained (§7). |
| `_reference/code-heim/go_80_tcp_udp_sockets/tcp/server/main.go` | `for { conn, err := ln.Accept(); ...; go handleConnectionNonBlocking(conn) }` (`:68-77`). | **No shutdown path at all**: the accept loop never ends and every connection goroutine outlives main; on an Accept error `conn` is nil and is still handed to a goroutine (`:71-76`, the §7 "handler spawned with a nil conn") → `TestGroup_defectGo80_connectionGoroutinesEndedByStop` (Stop ends the loop and waits for every handler; leak.Verify). `log.Fatalln`, read loop on 1 s deadlines. The TCP loop itself is out of scope (LADDER §9.9). |

### b. The authority (opened 2026-11-13)

- pkg.go.dev `net/http`, **"func (*Server) Shutdown"**. Opened: https://pkg.go.dev/net/http#Server.Shutdown,
  2026-11-13. Relied on: "Shutdown gracefully shuts down the server without interrupting any active
  connections ... waiting indefinitely for connections to return to idle"; "If the provided context expires
  before the shutdown is complete, Shutdown returns the context's error"; "Once Shutdown has been called on a
  server, it may not be reused". Ours: a Stopper has Shutdown's shape (`srv.Shutdown` is a Stopper); the deadline
  is in the ctx each Stop gets; the run ends with an error when it expires (ErrStopTimeout); a Group runs once
  (ErrClosed). "func (*Server) RegisterOnShutdown" (same page): extra functions called on Shutdown, the model
  for several stops per run.
- pkg.go.dev `os/signal`, **"func NotifyContext"**. Opened: https://pkg.go.dev/os/signal#NotifyContext,
  2026-11-13. Relied on: "returns a copy of the parent context that is marked done ... when one of the listed
  signals arrives, when the returned stop function is called, or when the parent context's Done channel is
  closed"; "If a signal causes the returned context to be canceled, calling context.Cause on it will return an
  error describing the signal"; call stop "as soon as the operations running in this Context complete". Ours:
  WithNotify takes a function of exactly NotifyContext's shape (minus the signal list); Run calls the returned
  stop when it returns; core never imports os/signal.
- The Twelve-Factor App, **"IX. Disposability"**. Opened: https://12factor.net/disposability, 2026-11-13. Relied
  on: "Processes shut down gracefully when they receive a SIGTERM signal from the process manager"; "graceful
  shutdown is achieved by ceasing to listen on the service port ..., allowing any current requests to finish,
  and then exiting"; "Processes should also be robust against sudden death". Ours: reverse order (the server
  stops listening and drains before what it depends on closes); a stuck Stop is abandoned at the deadline
  rather than holding the process (the manager would SIGKILL it anyway).

### c. Today's Go (GO_RELEASES.md)

- go1.26: `signal.NotifyContext`'s context cause names the signal: the WithNotify hook passes it through; the
  stop ctx cause is ErrStopTimeout (`context.WithCancelCause`, go1.20). `context.WithoutCancel` (go1.21) gives
  the stop ctx the run ctx's values without its cancellation. `sync.WaitGroup.Go` (go1.25) in the go_80 test.
- The standard library has the per-server drain (`Server.Shutdown`) but no run group: nothing starts several
  components in order and stops them in reverse within one deadline. Why toolbench needs it: LADDER row 19,
  three hand-rolled sites (red cmd/redlite/main.go:39, wire_replica.go:235, app.go:305) and the three defective
  repos above.

### d. calcbench

None: no mathematics (one deadline on a clock timer).

### e. Built on

- `clock`: the grace deadline is one `clock.Timer` from WithClock (default `clock.Real()`); tests use
  `clock.Fake` with `BlockUntil` (shutdown has armed the timer) and `AdvanceContext` (it fires).
- `panics`: `Try` around every Start, Stop and Run's start; `TryValue` around the notify hook.
- `group` is **not used** (deviation from LADDER row 19 "error group"). A group's `Wait` cannot be bounded: a
  stuck Stop would make Run wait past the grace period, the go_37 defect. Each Stop runs on its own goroutine
  with a buffered result channel and Run selects on it and on the grace timer, so it can return at the deadline.
  The starts are sequential on Run's goroutine (ordered start), so there is nothing to fork-join either.

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-LIFE-001` | `Stopper` | `type Stopper func(ctx context.Context) error` |
| `TB-LIFE-002` | `Run` | `func Run(ctx context.Context, grace time.Duration, start func(context.Context) error, stops ...Stopper) error` |
| `TB-LIFE-003` | `Component` | `type Component struct{ Name string; Start func(ctx context.Context) error; Stop Stopper }` |
| `TB-LIFE-004` | `Group` | `type Group struct{...}`; methods `Add(c Component) error`, `Run(ctx context.Context, opts ...Option) error` |
| `TB-LIFE-005` | `Option` | `type Option func(*config)` |
| `TB-LIFE-006` | `WithGrace` | `func WithGrace(d time.Duration) Option` |
| `TB-LIFE-007` | `WithClock` | `func WithClock(c clock.Clock) Option` |
| `TB-LIFE-008` | `WithNotify` | `func WithNotify(notify func(context.Context) (context.Context, context.CancelFunc)) Option` |
| `TB-LIFE-009` | `ErrClosed` | `var ErrClosed = errors.New("lifecycle: group already run")` |
| `TB-LIFE-010` | `ErrStopTimeout` | `var ErrStopTimeout = errors.New("lifecycle: grace period ended before every component stopped")` |
| `TB-LIFE-011` | `DefaultGrace` | `const DefaultGrace = 30 * time.Second` |

Options (Group.Run only):

| Option | Default | Bound it sets |
|---|---|---|
| `WithGrace(d)` | `DefaultGrace` (30s); d <= 0 is the default | total time every Stop together has |
| `WithClock(c)` | `clock.Real()`; nil is the default | the clock the grace period is measured on |
| `WithNotify(f)` | none | when shutdown begins (f's context is done) |

Sentinel errors: `ErrClosed` (second Run, Add after Run) · `ErrStopTimeout` (grace period ended) ·
`panics.ErrPanic` (a panic in Start, Stop, start or the notify hook).

**Deviations from LADDER.md §4**: `Stopper` and `Run` exactly as §4. Added, as the brief asked: `Component`,
`Group` (`Add`, `Run`), `Option`, `WithGrace`, `WithClock`, `WithNotify`, `ErrClosed`, `ErrStopTimeout`,
`DefaultGrace`. Choices within the brief's sketch: `Add` returns an error (`ErrClosed` after Run; oklog's Add
returns nothing, but a component added after Run would otherwise be silently never started nor stopped); §4's
`Run` has no options, so it measures grace on the real clock (its timed tests use a Stop that never returns until
released, so the result does not depend on time); `group` is not used (above).

## Per export

### `TB-LIFE-004` `Group` (with `Add`, `Run`) and `TB-LIFE-003` `Component`
- **Pattern**: graceful shutdown (run group, ordered start, reverse-order stop, drain deadline).
- **Authority**: net/http Server.Shutdown; os/signal NotifyContext; Twelve-Factor IX.
- **Familiar shape**: oklog/run `Group.Add`/`Run`; uber-go/fx lifecycle hooks (OnStart returns once started,
  OnStop in reverse); a Stop is `http.Server.Shutdown`. Difference from oklog: Start is a start hook (returns
  when up), not a blocking actor, so start can be ordered; run-time work lives on goroutines Stop ends.
- **Start**: Start k is called on Run's goroutine with ctx (the notify context when WithNotify is set) after
  Start k-1 returned nil. Start k returns an error or panics → k+1.. not started, k-1..1 stopped in reverse, k not
  stopped (it failed: it cleans up itself, as a constructor that returns an error); Run returns
  `lifecycle: start <name>: <err>` joined with the stop errors. A nil Start is a started component.
- **Goroutines**: one per Stop, one at a time; each has exited when Run returns, except a Stop still running at
  the grace deadline: it is abandoned (named in the error) and its goroutine ends when that Stop returns (its
  result channel is buffered). This is the one place a goroutine may outlive the call, by design: a stuck Stop
  must not hold a process that is shutting down (Twelve-Factor IX). The notify hook may start its own (NotifyContext
  does); its CancelFunc, called when Run returns, ends them.
- **Cancellation**: *before* (ctx done before Start 1): nothing starts, Run returns `context.Cause(ctx)`. *Mid-start*
  (done before Start k is called): k.. not started, 1..k-1 stopped in reverse, Run returns the cause (joined with
  stop errors); a Start that honours ctx and returns its error is a start failure (k not stopped). *After every
  Start*: ctx done is the normal shutdown request: stops in reverse; a clean shutdown returns nil (the signal is
  not an error). *After Run returned*: nothing; a second Run is `ErrClosed`.
- **Stop ctx**: keeps ctx's values (`context.WithoutCancel`), is not cancelled by ctx, and is cancelled with cause
  `ErrStopTimeout` when the grace period ends. No `Deadline()` (the clock may be fake).
- **Errors**: start error and every stop error, `errors.Join`, each wrapped `lifecycle: stop <name>: ...`; a
  failing or panicking Stop does not stop the others from being called. Grace deadline: `ErrStopTimeout` wrapped
  as `<still stopping> still stopping after <grace>; not stopped: <names>`, and Run returns at once. Name "" is
  `component <n>` (1-based Add order). runtime.Goexit in a Stop is reported as an error; in a Start it ends Run's
  goroutine, as for any caller.
- **Blocking**: Run blocks for the life of the process; Add never blocks.
- **Ordering and counts**: Start once each in Add order; Stop at most once each, in reverse, only for started
  components.
- **Zero value**: Group ready to use; Component's zero value does nothing. Group not copied (mutex). `mu` guards
  `comps` and `ran`.
- **Compared against**: github.com/oklog/run v1.2.0, `compare/lifecycle/lifecycle_test.go` (stop order, start
  failure; differences listed in its package comment).
- **Worked example**: `ExampleGroup` (start db, cache, server; stop server, cache, db; `<nil>`),
  `ExampleGroup_Add`, `ExampleGroup_Run`, `ExampleComponent`.

### `TB-LIFE-002` `Run` and `TB-LIFE-001` `Stopper`
- start runs on its own goroutine (it may block: ListenAndServe). Run waits for start to return or ctx to be done,
  then calls stops in reverse (`stops[len-1]` first, named `stop <position>`; a nil stop is skipped), then waits
  for start (named `start`), all under grace (<= 0: DefaultGrace) on the real clock. start returning early (nil or error) begins
  shutdown at once. start's error after ctx is done is dropped only when it matches `ctx.Err()` or
  `context.Cause(ctx)` (a cancelled start is a clean stop); `http.ErrServerClosed` is returned like any error
  (filter it in start: the package does not import net/http). nil start waits for ctx; nil stop skipped; ctx done
  before the call: nothing is called, the cause is returned. Goroutines (D-027): a stop still running at the
  deadline leaves one goroutine; a start still running leaves at most two (start's own and the step waiting on
  its result), each ending when that call returns. Examples: `ExampleRun`, `ExampleStopper`.

### `TB-LIFE-005..008` options, `TB-LIFE-009/010` sentinels, `TB-LIFE-011` `DefaultGrace`
- Table above. WithNotify: a nil returned context is ctx, a nil CancelFunc is not called, a panic is ErrPanic
  (`lifecycle: notify: ...`) and nothing starts. DefaultGrace 30s: Kubernetes' default
  terminationGracePeriodSeconds (not opened; stated as the reason for the number, not as an authority); faktory's
  ShutdownTimeout is 25s. Examples: `ExampleOption`, `ExampleWithGrace`, `ExampleWithClock`, `ExampleWithNotify`,
  `ExampleErrClosed`, `ExampleErrStopTimeout`, `ExampleDefaultGrace`.

## Tests (`lifecycle/group_test.go`, `run_test.go`, `example_test.go`, `main_test.go`)

`TestMain` with `leak.Main`; `leak.Verify(t)` in every test; every wait selects on `context.WithTimeout(t.Context(),
10s)`; no `time.Sleep`, no polling; the Group grace deadline with `clock.Fake` (`BlockUntil` then `AdvanceContext`);
never os/signal (WithNotify gets a test hook). Each test is ordered by channels only, so it is deterministic under
`-count=200`.
- Order: `TestGroup_startOrderReverseStop` (0, 1, 2, 5 components), `TestGroup_startFailureStopsEarlierInReverse`
  (k = 1..4: k-1..1 stopped in reverse, k+1.. not started), `TestRun_stopsInReverseAfterCtxDone`.
- Errors and panics: `TestGroup_startPanicIsErrPanic`, `TestGroup_stopErrorsJoinedAndPanicIsErrPanic`,
  `TestRun_panicsAndStopErrorsJoined`, `TestRun_startReturnsEarly` (nil / error), `TestRun_startErrorAfterCtxDoneIsReported`.
- Grace: `TestGroup_graceDeadlineNamesWhoDidNotStop` (fake clock; names the running and never-stopped; stop ctx
  cause ErrStopTimeout), `TestGroup_graceStopIgnoringCtxIsAbandoned`, `TestGroup_graceNotReachedWhenStopsReturn`,
  `TestGroup_stopCtxKeepsValuesAndIsNotCancelledByRunCtx`, `TestRun_graceNamesStuckStopAndStart`,
  `TestRun_graceStartIgnoresCtx` (real clock, grace 1ns, Stop/start blocked until released).
- Notify: `TestGroup_withNotify` (Start gets the hook's context; CancelFunc called before Run returns),
  `TestGroup_withNotifyNilResultsAndPanic`.
- Cancel: `TestGroup_cancelBefore`, `TestGroup_cancelMidStart` (Start ignores / honours ctx),
  `TestGroup_runTwiceAndAddAfterRun` (also cancel after completion), `TestGroup_noComponentsWaitsForCtx`,
  `TestGroup_nilCtxNilStartNilStopDefaultNames`, `TestRun_cancelBeforeNilStartNilCtx`.
- Repo defects: `TestGroup_defectGo80_connectionGoroutinesEndedByStop`, `TestGroup_defectGo37_signalThenBoundedStop`,
  `TestGroup_defectGo42_shutdownTimeoutNamesTheAbandoned`.
- No benchmark: not on a hot path (once per process).

## Out of scope

- A component failing at run time (after its Start returned) does not by itself begin the shutdown: its Stop
  returns the failure. To end the process on a server's Serve error, run it as §4 `Run`'s start, or cancel the
  ctx given to Group.Run. A later export (e.g. `Component.Wait`) could add it; not built without a request.
- Supervision and restart (Erlang-style supervisor, suture): not built.
- The TCP accept loop (go_80): LADDER §9.9 (`netx`, not core). An httpx helper that wraps `http.Server` as a
  Component belongs in `httpx/`.
- Signal handling itself: the caller's `signal.NotifyContext` through WithNotify.

## For the lead

- INDEX rows TB-LIFE-001..011 (doc.go lists them). doc/sources: net/http Server.Shutdown, os/signal NotifyContext,
  12factor disposability (all opened 2026-11-13); oklog/run v1.2.0 (pkg.go.dev opened for compare).
- compare/go.mod needs `github.com/oklog/run v1.2.0` (tidy after the wave).
- ASSUMPTIONS: DefaultGrace = 30s; a clean shutdown (ctx done after every Start) returns nil, not the signal cause;
  a stop still running at the deadline is abandoned (one goroutine may outlive Run until that Stop returns);
  `group` not used (DECISIONS candidate: LADDER row 19 lists "error group").
- `_delete/lifecycle/sync_test.go`: a scratch test helper of this builder, moved there (it went first via /tmp by
  mistake); please add the `_delete/LEDGER.md` line.

REMAINING: none (verified 2026-11-13; abandoned-goroutine rule recorded as D-027).

## Align (2026-11-13)

Work folder: `doc/align_2026-11-13/` · Matrix rows 21, 23, 25; list (b) ranks 2-3; decision 5 · Task D2 (Devin) ·
Released signatures unchanged; every change additive.

### Matrix rows

| Row | Pattern | REPEAT | Verdict | What this section does |
|---|---|---|---|---|
| 21 | run group (ordered start, first failure ends all, Stop once) | 3/4/7 | PARTIAL: run-time failure seam (sub-REPEAT 4) | `Component.Run` (rank 2) + the Stop-once test |
| 23 | health endpoint (liveness, readiness) | 0/7/7 (3 real) | PLANNED | the lifecycle half: `Component.Live`/`Ready`, `Group.Live`/`Ready`, `ErrNotReady` (D2) |
| 25 | graceful stop with a drain deadline | 2/4/6 | SAME | tests only: fresh deadline ctx, reverse order, start error unblocks |

### References read (reading list, never cited)

GO-CQRS `internal/pkg/http/echo/server/echo-server.go:42` (Shutdown with the already-cancelled ctx), `workers_runner.go:16,30,33`
(unbuffered errChan blocks the second failure forever; Stop N times per component, forward order); GO-Kafka-MongoDB
`server.go:156-162` (any goroutine failing cancels the rest: the one complete run-time seam), `:195` (Shutdown with
Background, no deadline); monolith-microservice `cmd/monolith/main.go:46-50` (closes a channel handlers still publish to:
wrong stop order); golang-todo `main.go:209-213` (waits for a signal even after a start failure); BulletProof-DynamoDB-GO
`handlers/health/health.go:22-29` (a dependency call on every probe, no deadline); GO-Kafka-MongoDB `http.go:27`
(constant 200). The readers' rows: `doc/align_2026-11-13/M2.md` "signal hook", "graceful stop", "run group", "health /
readiness seam".

### Authority (opened 2026-11-13; `.claude/rules/sources.md`)

- pkg.go.dev `golang.org/x/sync/errgroup`, **"func WithContext"**: "The derived Context is canceled the first time a function
  passed to Go returns a non-nil error or the first time Wait returns, whichever occurs first"; "func (*Group) Go": "The
  first goroutine in the group that returns a non-nil error will cancel the associated Context ... The error will be
  returned by Wait." Ours: the run ctx is cancelled when the first Run returns; its error is in Run's result.
- pkg.go.dev `github.com/oklog/run` v1.2.0, **"func (*Group) Run"**: "When the first actor returns, all others are
  interrupted. Run only returns when all actors have exited. Run returns the error returned by the first exiting actor."
  Ours: the first Run to return, nil or error, begins shutdown (familiar shape only; the library is compared, not copied).
  Difference kept: ours returns every error (errors.Join) and waits for the actors within the grace period, not without bound.
- Microsoft Azure Architecture Center, **"Health Endpoint Monitoring pattern"** (learn.microsoft.com, page dated 2026-09-24),
  "Solution" and "Example > Endpoint behavior": "Deep checks verify the application and the dependencies required to serve
  requests. Use them for readiness signals"; "Readiness (/healthz/ready). Check initialization and both required
  dependencies. Return 200 (OK) when the application can serve requests, or 503 (Service Unavailable) when a required check
  fails. Bound each dependency check with a timeout"; "health signals that load balancers and traffic routers can use to
  direct requests to healthy instances"; "Allow for initialization before treating startup delays as failures". Ours:
  liveness and readiness are two checks; each check runs under the caller's ctx (httpx sets the timeout and the 503);
  readiness is false until initialization (every Start) is complete. The page says nothing about shutdown: readiness
  flipping to false the instant shutdown begins follows from "direct requests to healthy instances" plus Twelve-Factor IX
  (stop listening, then drain): a draining instance must not receive new traffic. The Kubernetes convention (readiness
  false on SIGTERM) is named as the convention, not opened, not cited as authority.
- pkg.go.dev `net/http`, "func (*Server) Shutdown"; The Twelve-Factor App, "IX. Disposability": as in the 2026-11-13 wave
  section above (reopened, unchanged).

### Today's Go

go1.27 adds nothing new here: `context.WithCancelCause` (go1.20) gives the run ctx a cause naming who ended the group;
`sync/atomic.Int32` (go1.19) is the readiness state; `errors.Join` and multiple `%w` (go1.20) join the check errors under
`ErrNotReady`. The standard library still has no run group and no health aggregator.

### calcbench

None (no mathematics).

### Changes

**`TB-LIFE-003` `Component`** gains three fields (no new ID; additive for keyed literals, decision 5):

| Field | Called by | Contract |
|---|---|---|
| `Run func(ctx context.Context) error` | `Group.Run`, on its own goroutine, once every Start returned nil | the component's blocking work (Serve, a consumer loop). The FIRST Run to return, nil or error, begins shutdown exactly as ctx being done does; ctx is a child of the run ctx, cancelled when shutdown begins (before the first Stop) with a cause naming who ended the group (the parent's cause when ctx ended it). A Run that must drain in order ignores ctx and lets its Stop end it (`srv.Serve`/`srv.Shutdown`). |
| `Live func(ctx context.Context) error` | `Group.Live`, on the caller's goroutine | liveness: nil = alive. Independent of the start/stop state. nil field = always alive. |
| `Ready func(ctx context.Context) error` | `Group.Ready`, on the caller's goroutine, only while the group is running | readiness: nil = ready. nil field = always ready. |

Unkeyed-literal caveat: `lifecycle.Component{"db", start, stop}` compiled before and does not now (`go vet` composites
has always warned on it); keyed literals, the documented form, are unchanged. For the lead: ASSUMPTIONS.

**`Group.Run` (TB-LIFE-004)**, after every Start returned nil: sets the state to ready; starts one goroutine per non-nil
Run with the run ctx; waits for ctx to be done OR the first Run to return (`runner.go`). Then sets the state to stopping,
cancels the run ctx, stops the started components in reverse (unchanged), and then waits for every Run goroutine not yet
returned, as further steps of the same grace period named `run <name>` (one at a time, in Add order). The first Run's
error, unless it is its own ctx's cancellation error (`cancelled`, as `Run`'s start), is in the result wrapped
`lifecycle: run <name>: ...` where the start error would be; every later Run's error is joined with the stop errors
under the same wrapping; a panic in a Run is `panics.ErrPanic`; a Run still running at the deadline is named in the
ErrStopTimeout error (`run <name> still running after <grace>`; a Stop stays `still stopping`) and abandoned (D-027: its goroutine ends when that Run
returns; its result channel is buffered). A Run returning while another already ended the group is not a second
shutdown: the state flips once and the steps list is built once, on Run's goroutine. Goroutines: one per Stop (one at
a time) plus one per non-nil Run (together); each has exited when Run returns except a Stop or Run still running at the
deadline (D-027). A Run goroutine's result channel has capacity 1 and the "first returned" channel has capacity equal
to the Run count, so no Run goroutine ever blocks on reporting (GO-CQRS workers_runner.go:16).

**New exports**

| ID | Name | Signature | Contract |
|---|---|---|---|
| `TB-LIFE-012` | `Live` | `func (g *Group) Live(ctx context.Context) error` | calls every non-nil `Component.Live`, one at a time on the caller's goroutine, each with ctx, in Add order, every added component (before, during and after Run); returns their errors joined, each wrapped `lifecycle: live <name>: ...`; a panic is ErrPanic; once ctx is done the remaining checks are not called and `context.Cause(ctx)` is joined; no goroutines; blocks for as long as the checks take (bound it with ctx); nil ctx is Background; nil on a Group with no checks. Safe for concurrent use with Add, Run and Ready (`mu` for the components, nothing else shared). |
| `TB-LIFE-013` | `Ready` | `func (g *Group) Ready(ctx context.Context) error` | the lifecycle half of readiness: returns an error wrapping `ErrNotReady` before every Start has returned nil (`not every component has started`, also before Run is called and after a Start failed) and from the instant shutdown begins (`shutting down`: set before the run ctx is cancelled and before the first Stop; stays so after Run returned), regardless of the checks; otherwise calls every non-nil `Component.Ready` as Live does and, if any fails, returns `ErrNotReady` wrapping the joined errors (so `errors.Is(err, ErrNotReady)` is true whenever Ready fails; httpx maps it to 503). The state is read once, before the checks (an atomic; no lock held while a check runs). |
| `TB-LIFE-014` | `ErrNotReady` | `var ErrNotReady = errors.New("lifecycle: not ready")` | sentinel matched by every error Ready returns. |

Sequential, not parallel: lifecycle must not import `group` (D-025 reason, and the 2026-11-13 "group not used" note
above); a check that must be parallel is the caller's to fan out. No caching here: the caller (httpx, wave 9) caches
and sets the deadline (Azure: "Bound each dependency check with a timeout"; "Configure timeouts and failure thresholds").

### Scale note

Bounded memory: yes (one channel of capacity 1 per Run, one of capacity N, the clone of the component slice per
Live/Ready call). Contention point, from the code (no benchmark: not a hot path): `g.mu` for the clone in Live/Ready
(held for the copy only, never while a check runs) and the one atomic load for the state. Per-call allocations: Live/Ready allocate the clone
(`len(comps)` words) and the joined error on failure; Run allocates the channels once per process. At 10x probe
callers: 10x sequential check calls into the dependencies (the BulletProof defect) unless the caller caches: this
package runs the checks it is asked to run, under the ctx it is given, and says so; the cache lives in httpx where the
HTTP semantics (503, Cache-Control) live. At 100x: the same, plus 100x acquisitions of `g.mu` for the clone, each
held for a slice copy; from the code the dependencies break first, which the cache fixes. Run goroutines: at most one per component with a
Run, fixed at Run time; no growth.

### Tests added (`lifecycle/group_test.go`, `runner_test.go`, `health_test.go`, `example_test.go`)

D2 (code unchanged, tests named for the defect): `TestGroup_stopGetsFreshDeadlineNotTheCancelledCtx` (GO-CQRS
echo-server.go:42; GO-Kafka-MongoDB server.go:195), `TestGroup_stopOncePerComponentUnderConcurrentFailures` (GO-CQRS
workers_runner.go:33), `TestGroup_stopOrderIsReverseOfStart` (monolith main.go:46-50),
`TestGroup_startErrorUnblocksBeforeAnySignal` (golang-todo main.go:209-213).
Run seam: `TestGroup_runFailureBeginsShutdown` (GO-Kafka-MongoDB server.go:156-162),
`TestGroup_concurrentRunFailuresJoinedNoLeak` (GO-CQRS workers_runner.go:16,30), `TestGroup_runReturningNilEndsGroup`,
`TestGroup_runPanicIsErrPanic`, `TestGroup_runIgnoringCtxIsAbandonedAndNamed` (fake clock), `TestGroup_runCtxCancelledOnShutdownWithCause`,
`TestGroup_runCancelledBeforeRunsStart`, `ExampleGroup_run`.
Health: `TestGroup_readinessFalseOnceShutdownBegins` (fake clock; checked from inside a Stop),
`TestGroup_readinessFalseBeforeStartsComplete`, `TestGroup_liveAggregatesNamedErrors`, `TestGroup_checkPanicIsErrPanic`,
`TestGroup_checksStopWhenCtxDone`, `ExampleGroup_ready`, `ExampleGroup_Live`, `ExampleErrNotReady`. All with
`leak.Verify(t)`, bounded by `bounded(t)` (10 s), fake clock wherever a deadline is waited on, no `time.Sleep`. No
benchmark: once per process; a probe is bounded by the caller's ctx, not a hot path in core.

### For the lead

- INDEX rows: TB-LIFE-012 Live, TB-LIFE-013 Ready, TB-LIFE-014 ErrNotReady (doc.go lists them); TB-LIFE-003 and -004
  rows change their Does/Goroutines text (regenerate).
- DECISIONS: decision 5 of MATRIX.md (Component.Run as a field; Group.Go lost) is now built; the sequential checks
  (no `group` import) follow the existing "group not used" reason, no new D needed unless Devin wants parallel checks.
- ASSUMPTIONS: unkeyed `Component{...}` literals break (vet has always warned); the first Run returning NIL ends the
  group (oklog/run, errgroup's Wait: a server that returns from Serve has stopped serving); Ready wraps ErrNotReady on a
  failing check too; Live runs on every added component regardless of state; Kubernetes readiness-on-SIGTERM named as
  convention only.
- doc/sources to add: `azure_health_endpoint_monitoring.md` (opened 2026-11-13; TB-LIFE-012, -013, -014, -003),
  `oklog_run.md` (pkg.go.dev opened 2026-11-13; TB-LIFE-003, -004; compared in compare/lifecycle); `x_sync_errgroup.md`
  gains TB-LIFE-003, -004 ("func WithContext"); `go_standard_library.md` and `twelve_factor_disposability.md` gain
  TB-LIFE-012..014.
- "Out of scope" above (a run-time failure does not begin shutdown): superseded by this section.

### Measured (VERIFY, 2026-11-13, go1.27.3 darwin/arm64, -race on; raw in doc/align_2026-11-13/out/bench_new.txt)
no benchmark (once per process); the race suite passed with every new test (lifecycle 1.7 s).
