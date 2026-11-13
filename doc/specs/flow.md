# flow: channel pipelines, fan-out and fan-in, or-done, tee, bridge, priority and weighted merge

Status: SHIPPED (2026-11-13)
Package: `flow` · SHORT: `FLOW` · Layer: core · Tier: T1
Work folder: `doc/wave2_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #8, #9, #11-#15
Builds on: `panics` (ErrPanic), calcbench `algo/sample` · Decisions: D-006 · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; a reading list, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `go_21_goroutines_pipeline/main.go` | four stages (`loadImage`, `resize`, `convertToGrayscale`, `saveImage`), each a goroutine ranging over its input and closing its output (`:16-69`). | No done channel or ctx: a consumer that stops early leaves every stage blocked on its send forever (LADDER.md §7 "stages leak on early consumer exit"). Test: `TestStage_earlyConsumerExitNoLeak`. |
| `go_56_generator/main.go` | `primeGenerator(done, op)` computes the next value, then selects send vs done (`:35-50`); main breaks after 10 and sends `done <- true` (`:63`). | Channel generator for a synchronous producer (`iter.Seq` since go1.23); `done` is a send, not a close, so it is a one-shot signal to exactly one receiver; the next value is computed before cancellation is seen. Test: `TestFromSeq_earlyBreakStopsGenerator` (a consumer that stops via ctx makes the seq's `yield` return false; the seq's deferred cleanup runs; no goroutine left). |
| `go_62_ordone_tee_channel/main.go` | `orDone` (`:10-31`) and a two-way `tee` (`:34-55`). | **Tee defect**: `o1, o2 := out1, out2` is declared INSIDE the inner `for i := 0; i < 2` loop (`:44`), so the nil-ing of the side already served is undone on the second iteration and one side can receive the same value twice while the other gets none. The tee also has no done case (blocks forever on a gone consumer). `time.Sleep` coordination (`:68`, `:117`). Tests: `TestTee_eachValueExactlyOncePerOutput_go62` and `TestTee_cancelWithGoneConsumer`. |
| `go_25_fan_out_fan_in/after/main.go` | `resize` starts one goroutine PER job into a channel buffered `len(jobs)` (`:28-37`); `collectJobs` receives exactly `imageCnt` values (`:39-46`). | Unbounded fan-out (one goroutine per item), no close (the collector must know the count), no error or panic path, no cancellation. Tests: `TestFanOut_boundedWorkers_go25` (never more than `workers` fn calls at once), `TestFanOut_outputClosedNoCountNeeded`. |
| `go_24_unbuffered_buffered_channels/buffered.go` | buffered channel of 3 between a producer and a slow barista (`time.Sleep`). | Shows backpressure only by sleeping. Our default is unbuffered (`WithBuffer` default 0); test `TestFanOut_slowConsumerBackpressure` proves the producer is held back with channels, no sleep. |
| `go_42_faktory/worker/main.go` | `ProcessStrictPriorityQueues("critical","default","low_priority")` (`:52`); weighted variant commented out (`:55-56`). | Library usage only (Faktory server). Gives the two shapes: strict priority and weighted priority. Tests: `TestPriority_strictWithPrefilledInputs`, `TestWeighted_fixedSeedDistribution`. |

### b. The authority (opened 2026-11-13)

- Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014.
  Opened: https://go.dev/blog/pipelines, 2026-11-13. Sections (in order): Introduction, What is a pipeline?,
  Squaring numbers, Fan-out, fan-in, Stopping short, Explicit cancellation, Digesting a tree, Parallel
  digestion, Bounded parallelism, Conclusion. Relied on:
  - "Fan-out, fan-in": "Multiple functions can read from the same channel until that channel is closed; this is
    called fan-out." Fan-in: read "from multiple inputs and proceed until all are closed by multiplexing the
    input channels onto a single channel". `merge` starts one goroutine per input and "a goroutine to close
    out once all the output goroutines are done" (sync.WaitGroup). Our `Merge` and `FanOut` closer.
  - "Stopping short" / "Explicit cancellation": "stages close their outbound channels when all the send
    operations are done"; "stages keep receiving values from inbound channels until those channels are
    closed"; "Goroutines are not garbage collected; they must exit on their own"; closing done is "a broadcast
    signal to the senders". Ours: ctx.Done() is the done channel; every send selects on it.
  - "Bounded parallelism": "a fixed number of goroutines for reading files". Our `FanOut(workers)`.
- Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4 "Concurrency Patterns in Go": "Pipelines", "Generators"
  (in "Pipelines"), "Fan-Out, Fan-In", "The or-done-channel", "The tee-channel", "The bridge-channel".
  NOT OPENED: the book is not online. Cited by section for the names and shapes of OrDone, Tee and Bridge;
  where the blog covers the same ground (pipelines, generators, fan-out/fan-in, done channels) the code
  comment cites the blog.
- Microsoft, Azure Architecture Center, "Priority Queue pattern" (ms.date 2026-08-11).
  Opened: https://learn.microsoft.com/azure/architecture/patterns/priority-queue, 2026-11-13. Relied on:
  "Multiple queues" / "Single consumer pool": "Consumers process messages from the highest priority queue first
  and process messages from lower-priority queues only when there are no more high-priority messages ... This
  setup can lead to lower-priority messages being continually delayed and potentially never processed." (our
  `Priority`, its starvation documented); "Multiple consumer pools": "Higher-priority queues use more consumers
  or higher performance tiers"; "Consider low-priority processing": "Decide whether all high-priority items
  must be processed before any lower-priority items" (our `Weighted`: higher weight gets proportionally more
  turns, no input starves).
- Go standard library, package `iter`. Opened: https://pkg.go.dev/iter, 2026-11-13. "type Seq[V any] func(yield
  func(V) bool)"; the iterator "stops either when the sequence is finished or when yield returns false";
  "Yield panics if called after it returns false". Our `FromSeq` stops ranging when ctx is cancelled (yield
  returns false); `ToSeq` returns as soon as yield returns false and never calls it again.

### c. Today's Go
`iter.Seq` (go1.23) is the synchronous generator: a pipeline over in-memory data with no concurrency is
`iter`/calcbench `algo/seq` (ALG-SEQ-001..008), not channels. Channels stay for crossing goroutines. The
standard library and golang.org/x have no Merge, FanOut, OrDone, Tee, Bridge or priority merge;
`sync.WaitGroup.Go` (go1.25) is used inside. `FromSeq`/`ToSeq` are the bridge between the two worlds.
`reflect.Select` could select over N channels but CLAUDE.md forbids reflection: Priority/Weighted use one
short-lived receiver goroutine per idle input instead (see §e).

### d. calcbench
Confirmed in `/Users/devinanderson/Desktop/calcbench/INDEX.md:210-211` and the pinned v0.1.0 source
(`algo/sample/weighted_pick.go:49,76`): ALG-SAMPLE-003 `NewWeightedPicker[W integer](weights []W)
(*WeightedPicker[W], error)` and ALG-SAMPLE-004 `(*WeightedPicker[W]).Pick(rng *rand.Rand) (int, error)`
(`math/rand/v2`). `Weighted` calls both on every pick (over the weights of the inputs that have a value ready).
Nothing re-implemented. Buffers are channels, not ALG-QUEUE-002 (LADDER.md §5: they must block the producer).

### e. Exports

| ID | Name | Signature |
|---|---|---|
| `TB-FLOW-001` | `FromSeq` | `func FromSeq[T any](ctx context.Context, seq iter.Seq[T], opts ...Option) (<-chan T, func() error)` |
| `TB-FLOW-002` | `ToSeq` | `func ToSeq[T any](ctx context.Context, in <-chan T) iter.Seq[T]` |
| `TB-FLOW-003` | `Stage` | `func Stage[In, Out any](ctx context.Context, in <-chan In, fn func(context.Context, In) (Out, error), opts ...Option) (<-chan Out, func() error)` |
| `TB-FLOW-004` | `FanOut` | `func FanOut[In, Out any](ctx context.Context, in <-chan In, workers int, fn func(context.Context, In) (Out, error), opts ...Option) (<-chan Out, func() error)` |
| `TB-FLOW-005` | `Merge` | `func Merge[T any](ctx context.Context, ins ...<-chan T) <-chan T` |
| `TB-FLOW-006` | `OrDone` | `func OrDone[T any](ctx context.Context, in <-chan T) <-chan T` |
| `TB-FLOW-007` | `Tee` | `func Tee[T any](ctx context.Context, in <-chan T, n int, opts ...Option) ([]<-chan T, error)` |
| `TB-FLOW-008` | `Bridge` | `func Bridge[T any](ctx context.Context, chans <-chan (<-chan T)) <-chan T` |
| `TB-FLOW-009` | `Priority` | `func Priority[T any](ctx context.Context, ins ...<-chan T) <-chan T` |
| `TB-FLOW-010` | `Weighted` | `func Weighted[T any](ctx context.Context, ins []<-chan T, weights []int, opts ...Option) (<-chan T, error)` |
| `TB-FLOW-011` | `Option` | `type Option func(*config)` |
| `TB-FLOW-012` | `WithBuffer` | `func WithBuffer(n int) Option` |
| `TB-FLOW-013` | `WithRand` | `func WithRand(r *rand.Rand) Option` (`math/rand/v2`) |
| `TB-FLOW-014` | RETIRED | `WithPanicHandler` was removed before release (lead's decision on review); the ID is never reused |
| `TB-FLOW-015` | `ErrInvalid` | `var ErrInvalid = errors.New("flow: invalid argument")` |

Options:

| Option | Default | Bound it sets / used by |
|---|---|---|
| `WithBuffer(n)` | 0 (unbuffered: backpressure); n < 0 is 0 | capacity of each output channel: FromSeq, Stage, FanOut, Tee (each output), Weighted |
| `WithRand(r)` | a PCG source seeded from `math/rand/v2` per call | the random source for Weighted's picks; nil = default. Used by one goroutine only |

Deviations from LADDER.md §4 (written down, each a "For the lead" item):
1. `Weighted` gains `opts ...Option`: the brief requires an injectable random source as an option.
2. `Tee` gains `opts ...Option` (WithBuffer per output): decoupling consumers by a few values is the only
   tuning a tee has. Additive and optional.
3. `FromSeq` returns `(<-chan T, func() error)`, the same wait-func shape as Stage and FanOut (lead's decision
   on review, recorded as a D). Why: seq is caller code on our goroutine, so a panic must be returned, not
   swallowed (CLAUDE.md "Honest about errors"). The wait func returns the `*panics.PanicError`, else ctx's
   error if ctx stopped seq early, else nil. The alternative that lost: §4's `<-chan T` plus a
   `WithPanicHandler` hook (built first, then removed): the error was lost whenever no hook was passed.
4. `ErrInvalid` sentinel added (callers match bad `Tee`/`Weighted` arguments with errors.Is).

Shared rules (every export):
- **The creator closes.** Every channel flow returns is created and closed by flow, exactly once, after its last
  send, and only after every goroutine that could send on it has exited.
- **Nil input channel = an input with no values** (as if closed), never a hang: Merge, Priority and Weighted
  skip it; OrDone, Bridge, Tee, Stage, FanOut, ToSeq end at once. A nil channel inside Bridge's chans is
  skipped. A closed input ends normally.
- **Cancel** (ctx done, before, during or after): every goroutine stops at its next send/receive select, every
  output is closed. A value already received from an input but not yet sent is dropped on cancel (it was in
  flight; the caller cancelled). After normal completion, cancel has no effect.
- **Backpressure**: no value is ever dropped while ctx is live; a slow consumer blocks the sender.

#### `TB-FLOW-001` `FromSeq`
Pattern: generator. Authority: Ajmani 2014 "What is a pipeline?" (`gen`), "Explicit cancellation"; pkg.go.dev
iter "Seq". Goroutines: 1, ranges over seq; exits when seq ends, ctx is done (yield returns false, so seq's own
defers run), or seq panics. Closes out on exit, then releases wait. Order: seq order. Errors: wait returns a
*panics.PanicError for a panic, else ctx's error if ctx stopped seq early, else nil.

#### `TB-FLOW-002` `ToSeq`
Pattern: channel-to-iterator adapter. Authority: pkg.go.dev iter "Seq". Goroutines: none; the range body runs on
the caller's goroutine. Stops when in is closed, ctx is done, or yield returns false (never calls yield again).
Ranging the Seq twice continues reading the same channel (a channel is single-use): documented.

#### `TB-FLOW-003` `Stage`
Pattern: pipeline stage. Authority: Ajmani 2014 "Squaring numbers", "Explicit cancellation"; Cox-Buday ch.4
"Pipelines". It is `FanOut` with workers = 1, so output order = input order. Goroutines: 2 (worker, closer).

#### `TB-FLOW-004` `FanOut`
Pattern: fan-out (competing consumers), bounded parallelism. Authority: Ajmani 2014 "Fan-out, fan-in",
"Bounded parallelism". Familiar shape: errgroup first-error-wins. Goroutines: workers + 1 (closer); workers < 1
is 1. Each worker: receive (select ctx), `panics.TryValue(fn)`, send (select ctx). The closer waits for every
worker (WaitGroup), closes out, then releases the wait func. Errors: first fn error or ErrPanic wins and cancels
the derived ctx passed to fn; the others stop at their next select. The wait func blocks until the output is
closed and every goroutine has exited; it returns the first fn error, else ctx's error if any worker stopped
while ctx was done (including an input closed by an upstream stage on the same ctx), else nil. Callable many times, same result. Order:
completion order (workers = 1: input order). Exactly one output per input that completed without error.
A consumer must drain out or cancel ctx before (or while) calling wait; otherwise wait blocks behind the
consumer — documented (Blocking line).

#### `TB-FLOW-005` `Merge`
Pattern: fan-in. Authority: Ajmani 2014 "Fan-out, fan-in" (`merge`). Goroutines: one per non-nil input + 1
closer (none extra when there are no inputs: the channel is returned closed). Order: per-input order kept,
interleaving unspecified. Every value exactly once.

#### `TB-FLOW-006` `OrDone`
Pattern: or-done channel. Authority: Cox-Buday ch.4 "The or-done-channel" (not opened); Ajmani 2014 "Explicit
cancellation". Goroutines: 1; exits when in closes or ctx is done. Lets a consumer `range` with cancellation.

#### `TB-FLOW-007` `Tee`
Pattern: tee channel. Authority: Cox-Buday ch.4 "The tee-channel" (not opened). n outputs, every value sent
exactly once to each, in input order. Goroutines: n forwarders + 1 distributor. The distributor reads one value,
hands it to each forwarder (a 1-slot handoff channel per forwarder that is always empty at that moment) and waits
for n acknowledgements before reading the next value (lock-step: the next value is not read until every output
has taken the current one, unless WithBuffer gives slack). Each forwarder sends on its own output with a ctx
select, so a consumer may read the outputs in any order, even from one goroutine. Errors: n < 1 returns
(nil, ErrInvalid) and starts nothing. Cancel: forwarders acknowledge without sending; the distributor stops;
it closes the handoffs; each forwarder closes its output.

#### `TB-FLOW-008` `Bridge`
Pattern: bridge channel. Authority: Cox-Buday ch.4 "The bridge-channel" (not opened). Goroutines: 1; reads one
inner channel at a time, forwards it to the end, then the next. Order: inner channels in arrival order, values
in order within each. Exits when chans closes or ctx is done.

#### `TB-FLOW-009` `Priority`
Pattern: priority queue (multiple queues, single consumer: strict priority). Authority: Azure "Priority Queue
pattern", "Single consumer pool". ins[0] is highest. Each turn the merger polls every idle input without blocking
(at most one held value per input), then sends the held value of the lowest index. When nothing is held it
starts one receiver goroutine per open input, with a stop channel and WaitGroup for that wait only (each
receives at most one value into a buffered arrivals channel of capacity len(ins), so it never blocks after
receiving), and waits for the first arrival or ctx; then it closes that stop channel, waits for every receiver
and drains arrivals into the held slots before choosing. Fix after review (BLOCKER 2): the first version kept
receivers alive across turns and ignored an input while its receiver was active, so a high value a receiver had
already taken could be passed over for many low values (reproduced at GOMAXPROCS=1); regression test
`TestPriority_heldHighValueBeatsReadyLow` (synctest bubble + GOMAXPROCS(1), deterministic, no sleep).
Strictness: among the values flow has received when it picks, a higher-priority value is always sent first; a
lower input can starve (Azure: "potentially never processed"). On cancel, held values and a value a receiver had
just taken are lost. Goroutines: 1 merger + at most len(ins) receivers, none alive while it sends or after it
returns. Order within one input kept.

#### `TB-FLOW-010` `Weighted`
Pattern: weighted priority queue (weighted fair dequeue). Authority: Azure "Priority Queue pattern", "Multiple
consumer pools", "Consider low-priority processing". Math: ALG-SAMPLE-003, ALG-SAMPLE-004. Same merger as
Priority; each turn picks among inputs that have a value ready with probability weights[i] / Σ ready weights
(calcbench picker over the ready subset). Work-conserving: a ready input is never skipped to wait for an empty
heavier one. Errors (nothing started, nil channel): len(ins) != len(weights), len 0, a weight < 1 (a zero weight
would never be drained and its producer would block forever), or a total that overflows (calcbench ErrOverflow
wrapped): all `ErrInvalid`. Random source: WithRand; tests pass `rand.New(rand.NewPCG(1, 2))`.

#### `TB-FLOW-011..013` Option, WithBuffer, WithRand; `TB-FLOW-014` retired; `TB-FLOW-015` ErrInvalid — see table.
`Tee` forwarders check ctx before each send (review fix 6), so after cancel nothing more is sent.

### g. Comparison
`compare/flow/flow_test.go` (written, not built: compare/go.mod lacks the dependency) against
github.com/sourcegraph/conc v0.3.0, package `stream` (`stream.New().WithMaxGoroutines(n)`, `Go(func()
stream.Callback)`, `Wait`), the one place shapes meet: ordered fan-out. Same inputs (1..50, fn = square).
`Stage` (one worker) and conc stream both deliver in submission order: compared value for value. `FanOut`
(n workers) delivers in completion order, conc stream in submission order: compared as sorted multisets (named
difference). conc re-panics a task panic in `Wait`; ours returns ErrPanic from the wait func (named difference,
asserted on ours only).

### h. Out of scope
Concurrent Map/Filter/Chunk/Window (package `parallel`, wave 3); worker pools with Submit (`pool`); batching by
size or time (`batch`); dropping on overflow (`WithDropOldest` is not offered); persistent or distributed
priority queues and aging of low-priority values (`jobs`); N-way select by reflection.

## Tests
One `_test.go` per export file, `TestMain` with `leak.Main`, `leak.Verify(t)` in every goroutine test, each
waiting test under `context.WithTimeout(t.Context(), 10*time.Second)`, no `time.Sleep`: Examples with `// Output:`
(sorted where order is completion order); nil and closed inputs; zero/one/many; workers = 1 and workers > items;
slow consumer backpressure (a gate channel held shut, producer progress counted); cancel before / mid / after with
outputs closed; fn panic -> ErrPanic via wait; seq panic -> ErrPanic via FromSeq's wait; Tee exactly once per output (go_62);
Priority strictness with pre-filled buffered inputs; Weighted with `rand.New(rand.NewPCG(1, 2))` (proportions
within bounds, every value delivered, errors); the go_21, go_56, go_25 defects as named cases.

## For the lead
- Remaining §4 deviations, each for DECISIONS or ASSUMPTIONS: (1) `Weighted` takes `opts ...Option`
  (`WithRand`, TB-FLOW-013); (2) `Tee` takes `opts ...Option` (`WithBuffer` per output); (3) `FromSeq` returns
  `(<-chan T, func() error)` (lead's D); (4) `ErrInvalid` (TB-FLOW-015) added; plus the rule "a nil input
  channel is an input with no values".
- INDEX.md: TB-FLOW-014 `WithPanicHandler` row REMOVED (no replacement; the panic path is FromSeq's wait func).
- `doc/sources/`: go-blog-pipelines (Ajmani 2014) -> TB-FLOW-001,003,004,005,006; cox-buday-2017 ch.4 (not
  opened) -> TB-FLOW-003,006,007,008; azure-priority-queue -> TB-FLOW-009,010; pkg.go.dev iter -> 001,002.
- compare/go.mod needs `github.com/sourcegraph/conc v0.3.0` (already needed by compare/panics).
- `Option` (TB-FLOW-011) cites Pike, "Self-referential functions and the design of options", command center
  blog, 24 Jan 2014 (opened https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html,
  2026-11-13); `WithBuffer` cites the Go spec "Channel types" (opened https://go.dev/ref/spec#Channel_types);
  `WithRand` cites pkg.go.dev math/rand/v2 (opened; "should be used by a single goroutine at a time");
  `ErrInvalid` cites pkg.go.dev errors "Is" (opened). Add to doc/sources/.

REMAINING: none. `go test -race -timeout 5m -count=50 ./flow/`: ok (4.1s).
