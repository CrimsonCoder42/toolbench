# batch: size-or-time batching over an iter.Seq2 or a channel

Status: SHIPPED (2026-11-13)
Package: `batch` · SHORT: `BATCH` · Layer: core · Tier: T2
Work folder: `doc/wave3_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #23
Builds on: `clock` (Clock, Timer; tests with Fake), `panics` (Try, ErrPanic), calcbench `algo/seq` (ALG-SEQ-004) ·
Decisions: see "For the lead" · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; a reading list, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `go_38_csv/controllers/report.go` | Upload handler: `csv.NewReader(file)` then `reader.ReadAll()` (`:90`), the whole file in memory, then `models.AddRows(&rows)` (`:97`). | **Unbounded ReadAll**: memory grows with the upload. Test `TestRun_go38_boundedMemoryPerBatch` (10 000 values, size 100: never more than 100 read and not yet flushed). |
| `go_38_csv/models/user_stats.go` | `AddRows` (`:56-80`): one transaction, one prepared statement, `stmt.Exec` per row. | **Per-row insert**: one round trip per row. Test `TestRun_go38_oneFlushPerBatchNotPerRow` (1001 values, size 100: 11 flushes). **strconv errors ignored**: `loginCount, _ := strconv.Atoi(record[2])` (`:72`), `active, _ := strconv.ParseBool(record[4])` (`:73`). Test `TestRun_go38_parseErrorNotIgnored` (the parse error is yielded by `in`, returned by Run, matched with `errors.As(*strconv.NumError)`). `log.Fatal` in a library path (`:21`, `:36`). |

### b. The authority (opened 2026-11-13)

- Hohpe, Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Aggregator".
  Opened: https://www.enterpriseintegrationpatterns.com/patterns/messaging/Aggregator.html, 2026-11-13.
  Relied on: "Use a stateful filter, an Aggregator, to collect and store individual messages until a complete
  set of related messages has been received"; the completeness conditions "Wait for All" ("Wait until all
  responses are received") and "Time Out" ("Wait for a specified length of time for responses and then make a
  decision by evaluating those responses received within that time limit"); once complete, the Aggregator
  "publishes a single, aggregated message to the output channel". Our mapping: correlation is "consecutive in
  input order"; completeness is count (`size`, Wait for All of a fixed-size set) or timeout (`maxWait` from the
  batch's first value, Time Out); the aggregation algorithm is "append into a slice"; publishing is `flush`
  (Run) or the send on the output channel (Channel).
- Go standard library, package `iter`, "Seq2" (https://pkg.go.dev/iter, opened 2026-11-13 by flow's spec;
  same page): an iterator stops when yield returns false. Run stops `in` that way on every error.
- Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Explicit cancellation"
  (https://go.dev/blog/pipelines, opened for flow 2026-11-13): every send selects on done; the stage closes
  its output. Channel follows it.
- pkg.go.dev time, "NewTimer", "func (*Timer) Stop" (go1.23 rules via clock): no stale value after Stop, so
  a stopped batch timer never closes the next batch early (`TestChannel_timerRestartsPerBatch`).

### c. Today's Go
go1.23 `iter.Seq2` is the natural shape for a lazily read import (a CSV reader, a rows cursor) that can fail
per record: Run consumes it without ReadAll. The standard library has no batcher; `slices.Chunk` (go1.23)
chunks a slice already in memory (the go_38 defect) and calcbench `seq.Chunk` chunks an `iter.Seq` without
errors or time. Neither closes a batch by time nor stops on an error from the source, so the export is still
needed.

### d. calcbench (confirmed in calcbench INDEX.md, v0.1.0 pinned)
- **ALG-SEQ-004 Chunk**: CALLED by Run (`batch/run.go`). Run adapts `in` (Seq2[T, error]) to a Seq[T] that
  records the first error and stops, and ranges `seq.Chunk(values, size)`. Chunk reuses one buffer between
  groups, so Run passes `slices.Clone(b)`: a fresh slice per batch the caller may keep.
- **ALG-SEQ-007 Batch**: confirmed; NOT called. With a nil `endsBatch` it groups exactly as Chunk, and the
  agreed §4 Run has no predicate parameter. A predicate-closed Run (records up to a commit marker) would be a
  new additive export calling ALG-SEQ-007; out of scope this wave.
- Channel re-implements the grouping on purpose: it must select over the input channel, a timer and ctx at
  once on one goroutine; Chunk is a synchronous pull over an `iter.Seq` and cannot close a group on a timer.
- `flow` is NOT imported: no flow export selects on an input, a timer and ctx together; wrapping `in` in
  `flow.OrDone` would add a goroutine for nothing (Channel's own select already has the ctx case).

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-BATCH-001` | `Run` | `func Run[T any](ctx context.Context, in iter.Seq2[T, error], size int, flush func(context.Context, []T) error) error` |
| `TB-BATCH-002` | `Channel` | `func Channel[T any](ctx context.Context, in <-chan T, size int, maxWait time.Duration, opts ...Option) <-chan []T` |
| `TB-BATCH-003` | `Option` | `type Option func(*config)` |
| `TB-BATCH-004` | `WithClock` | `func WithClock(c clock.Clock) Option` |
| `TB-BATCH-005` | `ErrSize` | `var ErrSize = errors.New("batch: size must be at least 1")` |

| Option | Default | Bound it sets |
|---|---|---|
| `WithClock(c clock.Clock)` | `clock.Real()` (also for nil) | the time source maxWait is measured with |

Sentinel errors: `ErrSize` (Run, size < 1, wrapped with the size) · `panics.ErrPanic` (a panic in `in` or `flush`).

## Per export

### `TB-BATCH-001` `Run`
- **Pattern**: aggregator, completeness by count.
- **Authority**: Hohpe and Woolf, EIP, 2003, "Aggregator" (Wait for All); pkg.go.dev iter "Seq2".
- **Familiar shape**: `for v, err := range rows` + `slices.Chunk`; the flush callback is the shape of
  `filepath.WalkDir`'s fn (an error stops the walk). Matches §4 exactly.
- **Goroutines**: none; `in` and `flush` run on the caller's goroutine.
- **Cancellation**: before the start: `in` is not read, `context.Cause(ctx)` returned. Between values: `in`
  is stopped (its yield returns false), the batch being collected is DROPPED, `context.Cause(ctx)` returned.
  ctx cancelled inside the last flush with nothing left to read: nil (every value was flushed). After: nothing.
- **Errors (decided)**: `size < 1` → `ErrSize`, `in` not read, flush not called. `flush` error → returned at
  once, `in` stopped, no further flush (that batch counts as delivered to flush). Error yielded by `in` → `in`
  is stopped, the values received before it are FLUSHED as a final batch, then the error is returned
  unchanged (joined with that flush's error via `errors.Join` if it failed). Why flush first: every value
  yielded before the error has then been passed to flush, so the caller's resume point is "the record after
  the last good one", independent of batch boundaries; the alternative (drop the partial batch) makes the
  resume point depend on size. A panic in `in` or `flush` → `*panics.PanicError` (`errors.Is ErrPanic`);
  flush runs inside its own `panics.Try`, so a flush panic never unwinds through the caller's `in`. A nil
  `flush` panics when first called → ErrPanic (empty `in`: nil). A nil `in` is empty; a nil ctx is Background.
- **Blocking**: for as long as `in` and `flush` run; a slow flush slows the reading of `in` (backpressure).
- **Ordering and counts**: input order; every value is passed to flush at most once (at-most-once); memory
  O(size) whatever the length of `in`.
- **Fresh slice**: yes, each batch is `slices.Clone` of Chunk's buffer; flush may keep it.
- **Zero value**: n/a (function).
- **Clock**: none (Run never waits on time).
- **calcbench IDs called**: ALG-SEQ-004 (`Math:`).
- **Compared against**: none (LADDER.md).
- **Worked example**: `ExampleRun` (`[1 2 3] [4 5 6] [7] <nil>`), `ExampleErrSize`.

### `TB-BATCH-002` `Channel`
- **Pattern**: aggregator, completeness by count or timeout.
- **Authority**: Hohpe and Woolf, EIP, "Aggregator" (Wait for All, Time Out); Ajmani 2014 "Explicit cancellation".
- **Familiar shape**: a flow-style stage (`<-chan T` in, `<-chan []T` out, closed by the creator). Matches §4.
- **size <= 0 (decided)**: treated as 1 (each value its own batch). Channel has no error result, and a
  time-only batcher would be unbounded (go-concurrency.md rule 4). Alternative (return a closed channel) lost:
  it silently drops the input.
- **maxWait <= 0 (decided)**: size-only; no timer is ever created (asserted by `TestChannel_maxWaitZeroIsSizeOnly`).
- **maxWait clock**: started when a batch receives its FIRST value (EIP Time Out per aggregate), stopped when
  the batch is sent by size; one timer per Channel, Reset between batches.
- **Goroutines**: 1 (none for a nil `in`); exits when `in` is closed and the last batch is received, or ctx is
  done; the output is closed as it exits; the timer is stopped on exit.
- **Cancellation**: before: nothing sent, channel closed. Mid-batch: the partial batch is DROPPED (at-most-once),
  channel closed. While a batch waits for the receiver: that send is abandoned (the receiver may or may not get
  it, never both it and a later one). A ready ctx wins over a ready input at the top of each loop.
- **Errors**: none; no caller callback runs, so a panic from caller code is impossible. A custom
  `clock.Clock` passed with WithClock is trusted like package time (not recovered).
- **Blocking**: each send waits for the receiver; while a batch waits, `in` is not read (backpressure, no drops).
- **Ordering and counts**: input order; each value in exactly one batch unless dropped by cancellation.
- **Fresh slice**: yes, a new slice after every send; the receiver may keep and append to it.
- **Nil / closed input**: nil `in` → output closed at once (flow's rule: no values); closed `in` → the
  partial final batch, then close.
- **Zero value**: n/a. **Clock**: `WithClock`, tests use `clock.Fake` with `BlockUntil` + `AdvanceContext`.
- **calcbench IDs called**: none (re-implemented on purpose, see §d).
- **Worked example**: `ExampleChannel` (`[1 2] [3 4] [5]`), `ExampleWithClock` (`[a b]` after maxWait).

### `TB-BATCH-003..005` `Option`, `WithClock`, `ErrSize`
Option: functional options (Pike 2014), a nil Option ignored; `ExampleOption`. WithClock: nil = `clock.Real()`.
ErrSize: callers match with `errors.Is`; it gets an ID (the brief's rule for matched sentinels).

## Tests (written; not run by the builder)
`batch/main_test.go` (`leak.Main`, helpers), `batch/run_test.go`, `batch/channel_test.go`, `batch/option_test.go`.
Examples with `// Output:` for every export · size boundaries (0, 1, size-1, size, size+1, 2·size+1, size 1,
huge size) for both · partial final batch · maxWait flush with `clock.Fake` (not due at maxWait-1ms, due at
maxWait) · stale-timer test after a size flush · maxWait <= 0 creates no timer · fresh slice per batch (mutate
and append to a kept batch) · in error (mid-batch, at a boundary, joined with flush error) · flush error stops
in (seq cleanup ran, 3 values read) · flush panic, in panic, nil flush → ErrPanic · cancel before / mid / inside
flush / after for Run (with `WithCancelCause`) and before / mid-batch / while sending / after for Channel · nil
and closed input channel, nil ctx · backpressure (in not read while a batch waits) · order and count with a
producer goroutine · go_38 defects (three named tests) · `leak.Verify` in every Channel test · benchmarks with
`ReportAllocs`. No `time.Sleep`, no polling; every wait bounded by `testCtx` (10 s). Deterministic under
`-count=200` by construction: every assertion of "not yet" is a select between an unbuffered send the
batcher can only accept in one state and the output.

## Out of scope
Predicate-closed batches (ALG-SEQ-007, a later additive export); concurrent flushes (compose with `pool` or
`parallel`); retrying a failed flush (`retry`); at-least-once delivery / persistence of a dropped partial batch
(`outbox`, `jobs`); byte-size limits per batch (a later option); CSV parsing itself (the caller's `in`).

## For the lead
- Decisions to log (DECISIONS or ASSUMPTIONS): (1) Run flushes the values before an error from `in`, then
  returns the error (alt: drop the partial batch); (2) ctx cancel drops the partial batch in Run and Channel,
  at-most-once (alt: flush it with a done ctx, pointless); (3) Channel size < 1 is 1 (alt: closed channel);
  (4) maxWait counts from a batch's first value (alt: a fixed ticker, which closes batches at arbitrary
  ages); (5) `ErrSize` TB-BATCH-005 added beyond §4 for Run's size < 1; (6) `flow` not imported (§d).
- `doc/sources/`: add `eip_aggregator.md` (Hohpe & Woolf 2003, "Aggregator", opened 2026-11-13) →
  TB-BATCH-001, 002; go_blog_pipelines → TB-BATCH-002; pkg.go.dev iter → 001; pike_2014_options → 003;
  pkg.go.dev time → 004; pkg.go.dev errors → 005; calcbench.md: ALG-SEQ-004 used by TB-BATCH-001.
- code-heim.md: go_38_csv → batch (defects above).
- INDEX.md rows TB-BATCH-001..005 (regenerate); taxonomy row says "flow, clock": builds on clock and panics,
  not flow.
- calcbench requests: none.

REMAINING: none (tests not run, per the builder rule).
