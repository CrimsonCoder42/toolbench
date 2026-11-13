# parallel: bounded parallel Map, Filter and ForEach over an iter.Seq, input order kept

Status: SHIPPED (2026-11-13)
Package: `parallel` · SHORT: `PAR` · Layer: core · Tier: T2
Work folder: `doc/wave3_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #21
Builds on: `flow` (TB-FLOW-001 FromSeq, TB-FLOW-004 FanOut), `panics` (TB-PANIC-001 ErrPanic, TB-PANIC-004 TryValue) ·
Decisions: D-016 (nil fn is ErrPanic, `nil, err` on error) followed · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; reading list only, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `go_25_fan_out_fan_in/after/main.go` | `resize` starts one goroutine per job into a channel buffered to `len(jobs)` (`:29-38`); `collectJobs` receives exactly `imageCnt` values (`:40-47`). | **Unbounded: one goroutine per job** (`TestMap_go25UnboundedGoroutinePerJob`: 1000 jobs, at most 4 in flight). **Completion order** reaches `saveImages` (`TestMap_orderKeptWhenCompletionIsReversed`). No ctx, no error path. `ReadImage` **panics** on a bad file (`image_processing.go:15,23`) and kills the process (`TestMap_go25ReadImagePanics`). `go func(job Job){}(job)` copy (dead since go1.22). |
| `go_67_conc/result_pool/main.go`, `group/main.go` | conc `pool.NewWithResults().WithMaxGoroutines(5)` (`:28`), `conc.WaitGroup` (`:16`). | Result pool order is completion order; `apiID := apiID` (`:32`); `time.Sleep` as work and a time-seeded rand (`:15-20`); conc re-raises panics on `Wait`. |
| `go_60_iter/prime/main.go` | `primeGenerator` returns an `iter.Seq[int]` (`:36-47`). | **Not restartable**: `num` lives outside the seq func (`:37`), so a second range continues where the first stopped (LADDER.md §7). `TestMap_go60IteratorNotRestartable`: Map ranges `in` exactly once per call; a restartable seq gives the same result twice; a single-use seq is read from its first value. |

### b. The authority (opened 2026-11-13)

- Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Bounded parallelism".
  Opened: https://go.dev/blog/pipelines, 2026-11-13. Relied on: the MD5All rewrite replaces a goroutine per file
  ("this may allocate more memory than is available on the machine") by a fixed `numDigesters` workers fed by
  one walker; a closed `done` channel stops every stage; "stages close their outbound channels when all the send
  operations are done"; "stages keep receiving values from inbound channels until those channels are closed or
  the senders are unblocked". `run` (ordered.go:28) is that shape: one generator (flow.FromSeq), N workers
  (flow.FanOut), a collector that drains until close, a cancel in place of `done`.
- pkg.go.dev `iter`, "Seq" ("seq calls yield(v) for each value v in the sequence, stopping early if yield returns
  false") and "Single-Use Iterators" ("Most iterators provide the ability to walk an entire sequence ... single-use
  iterators ... walk a sequence only once"). Opened: https://pkg.go.dev/iter, 2026-11-13. Hence: `in` is ranged
  once per call, and after the first error yield returns false so the seq's own cleanup runs.
- calcbench algo/seq (read-only, v0.1.0): ALG-SEQ-001 Map, ALG-SEQ-002 Filter, ALG-SEQ-004 Chunk, confirmed in
  calcbench INDEX.md rows 230-233. The sequential contracts: input order, stop seq at once when the consumer stops.

### c. Today's Go and why the export is still needed

go1.27 has `iter`, `slices.Collect`, `sync.WaitGroup.Go` and errgroup `SetLimit`, but no bounded, ordered,
cancelling parallel map: the hand-rolled form is an errgroup plus an index-addressed slice (LADDER.md counts 4
call sites in advo/red). It also cannot read an `iter.Seq` of unknown length, recover a panic, or stop the seq.

### d. calcbench IDs called: none (Math: none)

The twin is not a call, and why: ALG-SEQ-001/002 are lazy `iter.Seq` transforms with a `func(T) U` that cannot
fail or see a ctx; the concurrent twin must be eager (results finish out of order and are placed back by index)
and must stop on an error. A workers = 1 branch through `seq.Map` would be a second code path with different
cancellation and panic behaviour; instead workers = 1 runs the same flow path and the TESTS assert equality with
calcbench (`TestMap_equalsCalcbenchSeq`, `TestFilter_equalsCalcbenchSeq`, workers 1, 2, 7, 100, 200, 0).
ALG-SEQ-004 Chunk is not needed: no `Chunked` export in the agreed §4 API (see "For the lead").
Nothing calcbench has is re-implemented (index placement is `slices.Grow`, map.go:38).

## Exports

| ID | Name | Signature (file:line) |
|---|---|---|
| `TB-PAR-001` | `Map` | `func Map[In, Out any](ctx context.Context, in iter.Seq[In], workers int, fn func(context.Context, In) (Out, error)) ([]Out, error)` map.go:25 |
| `TB-PAR-002` | `Filter` | `func Filter[T any](ctx context.Context, in iter.Seq[T], workers int, keep func(context.Context, T) (bool, error)) ([]T, error)` filter.go:29 |
| `TB-PAR-003` | `ForEach` | `func ForEach[T any](ctx context.Context, in iter.Seq[T], workers int, fn func(context.Context, T) error) error` for_each.go:23 |

Exactly LADDER.md §4; no deviation. No options, no new sentinels (errors are fn's, `panics.ErrPanic`, or
`context.Cause(ctx)`).

## Per export (the three share `run`, ordered.go:28; the points below hold for all three)

- **Pattern**: bounded parallelism (parallel map / filter / for-each).
- **Familiar shape**: calcbench `seq.Map`/`seq.Filter` names and argument order after ctx; conc
  `iter.Mapper.MapErr` and errgroup `SetLimit` for the bound. Differences: takes an `iter.Seq` (not a slice),
  fn gets a ctx, first error wins (conc joins all).
- **workers <= 0**: GOMAXPROCS, as conc/iter (the compared library); D-023: one-shot calls default to GOMAXPROCS, long-lived constructors return an error. workers > items is
  fine: idle workers exit when the input closes.
- **Goroutines**: workers + 2 (the FromSeq generator, the FanOut workers, FanOut's closer); all have exited when
  the call returns (it waits on both wait funcs).
- **Cancellation**: before the call: nothing starts, `in` is not ranged, `context.Cause(ctx)` is returned. During:
  fn sees a cancelled ctx, no fn call starts after cancellation is seen (ordered.go wrapper check), `in`'s yield
  returns false, the cause is returned (also when fn ignores the cancel and returns nil). After: no effect.
- **Errors**: first error wins. It is recorded (sync.Once) BEFORE the shared ctx is cancelled, so a `ctx.Err()`
  returned by another call that saw the cancel never replaces it. A panic in fn or in `in` is a
  `*panics.PanicError` (`errors.Is(err, panics.ErrPanic)`). If fn's first error is ctx's own error, the cause is
  returned. Map and Filter return `nil` results on any error (as group.Results, D-016).
- **How much of `in` is consumed after an error**: the value the generator holds when the error happens, plus at
  most one value per worker that was waiting to receive at that instant; then yield returns false and `in`'s
  deferred cleanup runs before the call returns. Tests assert `pulled <= errIndex + 1 + workers + 1` on an
  infinite seq.
- **Blocking**: until every value is processed or the first error. Map/Filter hold every result (O(len(in))).
- **Ordering and counts**: Map and Filter return input order regardless of completion order; each value is
  passed to fn exactly once (ForEach asserts it per value); ForEach calls run in any order.
- **Nil inputs**: nil ctx is Background; nil `in` is `nil, nil`; nil fn/keep is ErrPanic on the first value
  (D-016's rule; calcbench's seq gives the empty sequence for a nil fn: deliberately not followed, a silent empty
  result would hide a bug).
- **Zero value / Clock**: functions only; nothing waits on time, no clock.
- **Compared against**: `compare/parallel/parallel_test.go`, conc `iter` v0.3.0 (Map, MapErr, ForEachIdx;
  Filter as Map + compaction) and errgroup v0.25.0 (SetLimit + indexed slice).
- **Worked examples**: `ExampleMap` `[2 4 8] <nil>`, `ExampleFilter` `[2 4 6] <nil>`, `ExampleForEach` `10 <nil>`.

## Tests (parallel/*_test.go; written, not run)

main_test.go `leak.Main`; every test `leak.Verify` and a 10 s `testCtx`; no sleep, no polling.
Map: table (0, 1, 500 items; workers 1, > items, 0, < 0) vs calcbench; equals calcbench seq for 5 worker counts;
order with completion forced 2,1,0 by gates; max in flight == workers (3 calls held at a barrier, atomics); go_25
unbounded (1000 jobs <= 4); first error cancels and stops an infinite seq with a bound on values pulled; panic in fn
and in seq; go_25 ReadImage panic as `*PanicError`; cancel before (nothing pulled, fn not called, cause), mid
(cause, seq stopped), after; nil ctx/in/fn; go_60 restartability; `BenchmarkMap` with ReportAllocs.
Filter: equals calcbench seq (5 counts), edges (empty, none kept, nil in, nil keep), reversed completion, first
error, panics, cancel before. ForEach: each value exactly once (workers=1 max in flight 1), first error with
bound, panics and nil fn, cancel mid with fn ignoring the cancel.

## Out of scope

Lazy/streaming results (a channel out: `flow.FanOut`/`flow.Stage`); a long-lived pool with Submit (`pool`);
collecting all errors (caller wraps fn); chunked work (`Chunked`, see below); unordered fast path.

## For the lead

- INDEX.md rows TB-PAR-001..003 (doc.go lists them); taxonomy `parallel` status planned → built; Builds on in
  taxonomy says "flow, group, panics, calcbench algo/seq": actual is flow + panics only (group not needed:
  FanOut already gives first-error-wins; calcbench only in tests).
- ASSUMPTIONS: workers <= 0 is GOMAXPROCS; nil fn is ErrPanic (D-016) where calcbench seq gives empty; Map/Filter
  return nil results on error; ctx errors are reported as `context.Cause(ctx)` (group's rule, not flow's `ctx.Err()`).
- LADDER.md §3 lists a `Chunked` export not in §4: not built; ALG-SEQ-004 not called. Devin decides if it is wanted.
- doc/sources: go.dev/blog/pipelines gains TB-PAR-001..003; pkg.go.dev iter gains TB-PAR-001..003.
- compare/: needs conc and x/sync (already in compare/go.mod); `go mod tidy` not run.

REMAINING: none (tests written, not run, per BRIEF).
