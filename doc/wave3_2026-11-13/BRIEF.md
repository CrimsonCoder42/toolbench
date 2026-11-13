# Brief for the wave 3 builders (T1-T2, 2026-11-13)

Request: `doc/requests/waves3_4_2026-11-13.md`. Plan: `doc/ladder_2026-11-13/LADDER.md`. Rules: `.claude/CLAUDE.md`,
`.claude/rules/go-concurrency.md`, `go-functions.md`, `go-testing.md`, `sources.md`. Model specs (finished,
reviewed): `doc/specs/group.md`, `doc/specs/flow.md`, `doc/specs/semaphore.md`. Read this file, `doc/specs/TEMPLATE.md`,
the four rules files, your LADDER.md rows and the wave 1-2 packages you build on; not the whole repo.

## THE STANDARD (verbatim from the request)

THE STANDARD, per package, in this order, nothing skipped (same as waves 1-2):
1. RESEARCH as doc/specs/<package>.md from doc/specs/TEMPLATE.md: the code-heim repos that show the pattern
   (LADDER.md "repos"), what is outdated or defective in them (§7; defects become tests); the authority OPENED and
   cited to the section; today's Go (GO_RELEASES.md) and why the export is still needed; calcbench IDs called
   (§5), confirmed in calcbench INDEX.md, with the reason for anything re-implemented; every export (ID,
   signature per §4 with deviations written down, goroutines, cancellation, errors, blocking, zero value); the
   compare/ library; out of scope. Waves 1-2 packages are BUILT ON, never duplicated: pool uses group and
   semaphore, parallel uses flow, schedule and lifecycle use clock, every recover goes through panics.
2. BUILD from the spec only: doc.go, one file per concept, exact doc-comment shape, tests per
   .claude/rules/go-testing.md (Example, edge cases, cancellation, panic, leak.Verify/leak.Main, the repo
   defects, fake clock for anything timed; no time.Sleep), compare/ tests where LADDER.md names a library
   (compare/ may `go mod tidy` for those libraries only; the main go.mod and go.sum stay byte-identical).
   gofmt -l, go build ./..., go vet on the package. Builders never go test.

## Mechanics (unchanged from waves 1-2; `doc/wave1_2026-11-13/BRIEF.md` "Mechanics" has the detail)

- **Owned paths**: `<package>/`, `doc/specs/<package>.md`, `compare/<package>/`. Everything else is the lead's
  (INDEX.md, taxonomy, DECISIONS, ASSUMPTIONS, CHANGELOG, doc/sources, go.mod, compare/go.mod): put what they
  need under "For the lead" at the end of your spec.
- **Outside this repo** only `/Users/devinanderson/Desktop/calcbench` is read (read-only). Never open
  advo_production, red_lite_cache, `.env*`, `app.env`, `accounts.json`, `vendor/`, `node_modules/`.
- **compare/**: write `compare/<package>/*_test.go`; do NOT run `go mod tidy` yourself (four builders editing
  compare/go.mod at once would collide): the lead runs it once after the wave. Do not build or vet compare/.
- **doc.go**: `ID short form: <SHORT> (IDs are TB-<SHORT>-NNN).` and the `Exports:` list (TAB, ID, exact name,
  Does line). Every exported function and type gets an ID; a method needs none when its receiver type has one;
  `New<T>` needs none when T has one, but a plain `New` returning `*T` DOES need its own ID (wave 2 lesson).
  Exported `Err...` variables get an ID when callers match on them.
- **Doc comment**: exact `.claude/rules/go-functions.md` shape; `Math:` just before `Tags:` when calcbench is
  called; unknown labels fail the index test; never cite code-heim; types state their zero value.
- **Tests**: `TestMain` with `leak.Main`, `leak.Verify(t)` in every goroutine-starting test, bounded by
  `context.WithTimeout(t.Context(), 10*time.Second)`, no `time.Sleep`, no polling loops; anything timed uses
  `clock.Fake` (with `AdvanceContext`/`BlockUntil`) or `testing/synctest`. Example per export with
  `// Output:`. Repo defects from LADDER.md §7 as named tests. Concurrency bounds asserted with atomics, never
  timing. Wave 2 lesson: write each test so it is deterministic under `-count=200`.
- **Panics**: every recover goes through `panics.Try`/`panics.TryValue` and returns `panics.ErrPanic`.
- **No panic on input** (waves 1-2 pattern, D-011, D-015, D-016): nil ctx is `context.Background()`, invalid
  counts are an error or a documented default, never a panic or an unbounded block.
- **Checks**, one command per call: `gofmt -l <pkg>`, `go build ./...`, `go vet ./<pkg>/`. Never `go test`,
  `go run`, `go get`, `go mod`.
- **Authority**: open it (WebFetch) and record `Opened: <URL>, 2026-11-13`; a book that is not online is cited and
  marked not opened.
- **30-minute cap**: stop with code that builds; list `REMAINING:` at the end of the spec.
- **Hand back** (≤ 15 lines): spec path; exports with ID and file:line; authority opened (URLs); calcbench IDs;
  wave 1-2 packages built on; deviations from §4 with why; For the lead; REMAINING.

## Wave 3 packages and the agreed LADDER.md §4 APIs

**B1 `recycle`, SHORT RECYC** (rank 17, T1). Repo go_54_sync_pool (defects: `interface{}` + assertion; Put of a
non-pointer slice allocates). Authority: pkg.go.dev `sync.Pool` (type doc: "A Pool must not be copied", "Any item
stored in the Pool may be removed automatically at any time"); golang/go#23199 (oversized-buffer retention,
https://github.com/golang/go/issues/23199). Builds on: nothing (stdlib). Compare: github.com/valyala/bytebufferpool
where the shapes meet (buffer reuse and reset), else `sync.Pool` itself.
```go
type Pool[T any] struct{ /* sync.Pool inside */ }      // create with New
func New[T any](newFn func() T, opts ...Option[T]) *Pool[T]
func (p *Pool[T]) Get() T
func (p *Pool[T]) Put(v T)
type Option[T any] func(*config[T])
func WithReset[T any](reset func(T) T) Option[T]     // applied on Put
func WithKeep[T any](keep func(T) bool) Option[T]    // false: dropped, not pooled (e.g. oversized buffers)
```
A nil newFn must not panic on Get (decide: zero T, documented). Tests cannot assert reuse deterministically
(the GC may clear a Pool): assert Reset/Keep effects and allocation counts with `testing.AllocsPerRun` where valid.

**B2 `pool`, SHORT POOL** (rank 18, T2). Repos go_18_worker_pool, go_66_tunny, go_67_conc (defects: no ctx, no
error, no panic recovery, WaitGroup per task; tunny's stateful worker with a value receiver; polling
`QueueLength()` with Sleep). Authority: Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog,
2014, "Bounded parallelism" (https://go.dev/blog/pipelines); Hohpe & Woolf, Enterprise Integration Patterns, 2003,
"Competing Consumers" (https://www.enterpriseintegrationpatterns.com/patterns/messaging/CompetingConsumers.html).
Builds on: `group` (its workers run in a group; first error / ErrPanic through Close), `semaphore` if a bound beyond
the worker count is needed, `panics`. Compare: github.com/sourcegraph/conc/pool (and note tunny, pond/v2 by name).
```go
func New[T any](ctx context.Context, workers int, fn func(context.Context, T) error, opts ...Option) (*Pool[T], error)
func (p *Pool[T]) Submit(ctx context.Context, v T) error   // blocks while the queue is full (backpressure)
func (p *Pool[T]) Close() error                             // stop accepting, drain, wait; errors.Join or first error
type Option func(*config)
func WithQueue(n int) Option        // default 0 (Submit hands off directly to a free worker)
func WithFailFast() Option          // first error cancels the pool's ctx and Close returns it; default collects (errors.Join)
```
Decide and state: Submit after Close (an `ErrClosed`), Submit when ctx is done, Close twice, workers <= 0.

**B3 `parallel`, SHORT PAR** (rank 21, T2). The concurrent twins of calcbench `algo/seq`: SAME NAMES, input order
kept by default. Repos go_25_fan_out_fan_in, go_67_conc, go_60_iter (defects: unbounded goroutine per job; no ctx;
`ReadImage` panics; non-restartable iterator). Authority: Ajmani 2014, "Bounded parallelism"; pkg.go.dev `iter`;
calcbench algo/seq (read `/Users/devinanderson/Desktop/calcbench/algo/seq` for the sequential contracts:
ALG-SEQ-001 Map, ALG-SEQ-002 Filter, ALG-SEQ-004 Chunk). Builds on: `flow` (FanOut or Stage for the workers) and/or
`group`, `panics`. Request: "parallel uses flow". Compare: github.com/sourcegraph/conc/iter and errgroup.
```go
func Map[In, Out any](ctx context.Context, in iter.Seq[In], workers int, fn func(context.Context, In) (Out, error)) ([]Out, error)
func Filter[T any](ctx context.Context, in iter.Seq[T], workers int, keep func(context.Context, T) (bool, error)) ([]T, error)
func ForEach[T any](ctx context.Context, in iter.Seq[T], workers int, fn func(context.Context, T) error) error
```
State: workers <= 0 (a documented default, e.g. GOMAXPROCS, as LADDER.md §4's pool options; or an error), workers
= 1 equals the calcbench sequential result, first error cancels the rest, a panic in `in` itself (the seq) and in
fn both become ErrPanic, how much of `in` is consumed after an error. The calcbench ID is cited on `Math:` only if
called; if not called, say in the spec why the twin is not a call.

**B4 `batch`, SHORT BATCH** (rank 23, T2). Repo go_38_csv (defects: `ReadAll` of an upload is unbounded; per-row
insert; strconv errors ignored). Authority: Hohpe & Woolf, EIP, "Aggregator"
(https://www.enterpriseintegrationpatterns.com/patterns/messaging/Aggregator.html), completeness by count or
timeout. calcbench: ALG-SEQ-004 Chunk, ALG-SEQ-007 Batch (confirm and call for `Run`). Builds on: `clock`
(maxWait via `clock.Clock`, `WithClock`, tests with `clock.Fake`), `flow` where it fits, `panics`. Compare: none.
```go
func Run[T any](ctx context.Context, in iter.Seq2[T, error], size int, flush func(context.Context, []T) error) error
func Channel[T any](ctx context.Context, in <-chan T, size int, maxWait time.Duration, opts ...Option) <-chan []T
type Option func(*config)
func WithClock(c clock.Clock) Option
```
State: size <= 0, maxWait <= 0 (size-only), whether a flushed slice may be retained by the caller (fresh slice per
batch), partial final batch, an error from `in` (Run stops and returns it, after flushing what? decide), ctx cancel
mid-batch (the partial batch: flushed or dropped? decide; at-most-once), a panic in flush. `Channel` has no error
path: a panic is impossible (no caller code runs); say so.
