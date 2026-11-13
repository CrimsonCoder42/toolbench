# Brief for the wave 2 builders (T1, 2026-11-13)

The standard, the mechanics and the hard limits are those of `doc/wave1_2026-11-13/BRIEF.md`, including Devin's
amendment (steps 1f and 3 deleted; outside this repo only `/Users/devinanderson/Desktop/calcbench` is read;
advo_production and red_lite_cache are never opened). Read that file first, then this one. Folder for this wave:
`doc/wave2_2026-11-13/`. Owned paths per builder: `<package>/`, `doc/specs/<package>.md`, `compare/<package>/`.

## What wave 1 gives you (import these; never re-implement them)

- `github.com/CrimsonCoder42/toolbench/panics`: `ErrPanic` (TB-PANIC-001), `*PanicError` (TB-PANIC-002, Value and
  Stack; `Unwrap() []error` so `errors.Is(err, panics.ErrPanic)` holds), `Try(fn func() error) error`
  (TB-PANIC-003), `TryValue` (TB-PANIC-004). Every recovered panic in caller code on our goroutine is returned via
  these. Read `panics/doc.go` and `doc/specs/panics.md` (panic(nil) and Goexit handling).
- `github.com/CrimsonCoder42/toolbench/clock`: `Clock` (Now, NewTimer, AfterFunc, NewTicker), `Real()`, `Fake`
  (`NewFake`, `Advance`, `BlockUntil(ctx, n)`). Wave 2 packages do not wait on time, so they should not need it.
- `internal/leak` for tests (`leak.Main` in TestMain, `leak.Verify(t)` per goroutine-starting test).

## compare/

`compare/go.mod` exists (module `github.com/CrimsonCoder42/toolbench/compare`, `replace ../`). Its dependencies
could not be resolved in this session (`go mod tidy` is not permitted), so write your `compare/<package>/*_test.go`
files (package `<package>_test`, same inputs to ours and theirs, every difference named in the file header) but do
not build or vet them. Name in your hand-back the module path and version you compared against.

## Wave 2 packages and the agreed LADDER.md §4 APIs

**B4 `group`, SHORT GROUP**: rank 7. Repos go_61_err_group, go_67_conc. Authority: pkg.go.dev
golang.org/x/sync/errgroup (`Group`, `WithContext`, `Go`, `TryGo`, `SetLimit`, `Wait`) AND its current source
(https://cs.opensource.google/go/x/sync/+/master:errgroup/errgroup.go or the GitHub mirror); Cox-Buday, Concurrency
in Go, 2017, ch.4 "Error Handling". **Settle LADDER.md §9 item 14 in the spec**: open the current x/sync errgroup
source and state, with the version and the lines, whether `Wait` re-panics a panic from a `Go` function (and
Goexit). Ours returns `panics.ErrPanic` through `Wait` either way; say how the compare test treats the difference.
Compare: golang.org/x/sync/errgroup. Rules: first error wins and cancels the derived ctx; `Wait` returns only after
every started goroutine has returned; `SetLimit` with a negative n = no limit (errgroup's rule), 0 = documented;
calling `SetLimit` while goroutines are active is errgroup's panic — ours must not panic on input: say what it does.
```go
type Group struct{ /* ... */ }                          // zero value ready, like errgroup
func WithContext(ctx context.Context) (*Group, context.Context)
func (g *Group) Go(fn func() error)
func (g *Group) TryGo(fn func() error) bool
func (g *Group) SetLimit(n int)
func (g *Group) Wait() error
func NewResults[T any](ctx context.Context, opts ...Option) *Results[T]
func (r *Results[T]) Go(fn func(ctx context.Context) (T, error))
func (r *Results[T]) Wait() ([]T, error)               // results in submission order
```

**B5 `flow`, SHORT FLOW**: ranks 8, 9, 11-15. Repos go_21_goroutines_pipeline, go_24_unbuffered_buffered_channels,
go_25_fan_out_fan_in, go_42_faktory, go_56_generator, go_62_ordone_tee_channel. Authority: Ajmani, "Go Concurrency
Patterns: Pipelines and cancellation", The Go Blog, 2014 (https://go.dev/blog/pipelines: "Fan-out, fan-in",
"Explicit cancellation", "Bounded parallelism"); Cox-Buday, Concurrency in Go, 2017, ch.4 ("The or-done-channel",
"The tee-channel", "The bridge-channel", "Fan-Out, Fan-In", "Pipelines", "Generators" — the book is not online:
cite the sections and say in the spec that the text was not opened, citing the blog where the two overlap);
Azure "Priority Queue pattern" (https://learn.microsoft.com/azure/architecture/patterns/priority-queue);
pkg.go.dev `iter`. calcbench: ALG-SAMPLE-003 NewWeightedPicker, ALG-SAMPLE-004 WeightedPicker.Pick for Weighted
(confirm in /Users/devinanderson/Desktop/calcbench/INDEX.md; import `github.com/CrimsonCoder42/calcbench/algo/sample`;
the random source must be injectable for tests: an option, fixed seed in tests). Compare:
github.com/sourcegraph/conc/stream only where the shapes meet (ordered fan-out); otherwise none.
ONLY these exports (the concurrent Map/Filter/Chunk/Window twins of calcbench algo/seq are `parallel`, wave 3):
```go
func FromSeq[T any](ctx context.Context, seq iter.Seq[T], opts ...Option) <-chan T
func ToSeq[T any](ctx context.Context, in <-chan T) iter.Seq[T]
func Stage[In, Out any](ctx context.Context, in <-chan In, fn func(context.Context, In) (Out, error), opts ...Option) (<-chan Out, func() error)
func FanOut[In, Out any](ctx context.Context, in <-chan In, workers int, fn func(context.Context, In) (Out, error), opts ...Option) (<-chan Out, func() error)
func Merge[T any](ctx context.Context, ins ...<-chan T) <-chan T
func OrDone[T any](ctx context.Context, in <-chan T) <-chan T
func Tee[T any](ctx context.Context, in <-chan T, n int) ([]<-chan T, error)
func Bridge[T any](ctx context.Context, chans <-chan (<-chan T)) <-chan T
func Priority[T any](ctx context.Context, ins ...<-chan T) <-chan T
func Weighted[T any](ctx context.Context, ins []<-chan T, weights []int) (<-chan T, error)
type Option func(*config); func WithBuffer(n int) Option   // default 0 (unbuffered); n < 0 documented
```
Every channel returned is closed by flow exactly once; nil and closed inputs handled; ctx cancel closes outputs
and every goroutine exits; panics in fn become `panics.ErrPanic` through the returned wait func. The go_21, go_56,
go_62 (tee sends one value twice to one side), go_25 defects in LADDER.md §7 are named test cases.

**B6 `semaphore`, SHORT SEMA**: rank 10. No repo. Authority: pkg.go.dev golang.org/x/sync/semaphore (`Weighted`,
`NewWeighted`, `Acquire`, `TryAcquire`, `Release`) and its source (FIFO fairness: a large waiter blocks smaller
later ones); Dijkstra, "Cooperating sequential processes", EWD123, 1965 (https://www.cs.utexas.edu/~EWD/transcriptions/EWD01xx/EWD123.html), the P/V operations section.
Compare: golang.org/x/sync/semaphore. LADDER.md §4 notes `New` instead of x/sync's `NewWeighted` as a deliberate
difference: keep it or argue it in the spec. Releasing more than held is x/sync's panic: ours must not panic on
input — say what it does (error? clamp?). Acquire(n > size) must not block forever silently: say what it does.
```go
func New(n int64) *Weighted
func (s *Weighted) Acquire(ctx context.Context, n int64) error
func (s *Weighted) TryAcquire(n int64) bool
func (s *Weighted) Release(n int64)
```

**B7 `guard`, SHORT GUARD**: rank 16. Repo go_20_mutexes_and_confinement. Authority: The Go Memory Model
(https://go.dev/ref/mem, "Locks"); Effective Go "Share by communicating" (https://go.dev/doc/effective_go#sharing);
Cox-Buday ch.4 "Confinement" (not online: cite and say so). Compare: none. The go_20 defect (the confining owner
goroutine never exits) is a named test case: `Confine` returns when ctx is done or reqs is closed, and its goroutine
count returns to the baseline. `Guarded[T]` must be vet-clean for copylocks, zero value ready; `Do` must not let the
pointer escape usefully (document it); `Load` returns a copy (say shallow).
```go
type Guarded[T any] struct{ /* mu sync.Mutex beside v */ }
func (g *Guarded[T]) Do(fn func(*T))
func (g *Guarded[T]) Load() T
func Confine[S any](ctx context.Context, state S, reqs <-chan func(*S)) S
```
