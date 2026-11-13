# recycle: a typed object pool (sync.Pool with a type parameter, reset and keep)

Status: SHIPPED (2026-11-13)
Package: `recycle` · SHORT: `RECYC` · Layer: core · Tier: T1
Work folder: `doc/wave3_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #17
Builds on: nothing (standard library `sync` only; tests use `internal/leak`) · Decisions: none new · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim go_54_sync_pool, read, closed)
`pool1.go` and `pool2.go` build a `sync.Pool{New: func() interface{} { return make([]byte, 1024) }}` and call
`pool.Get().([]byte)`. Defects, each a named test:
1. `interface{}` payload and a type assertion at every Get (LADDER.md §7): a wrong type is a runtime panic.
   Test `TestPool_go54_typedNoAssertion` (Get returns `[]byte`, checked by the compiler).
2. `Put` of a non-pointer `[]byte`: the slice header is boxed into an interface, one allocation per Put, which
   is what the pool was meant to save (pkg.go.dev sync "should generally only return pointer types"; vet/
   staticcheck SA6002). recycle cannot remove this for a value T (its Put also boxes), so the doc says to pool a
   pointer. Tests `TestPool_go54_sliceValuePutAllocates` (>= 1 alloc, deterministic with and without -race:
   boxing precedes sync.Pool) and `TestPool_go54_pointerGetPutNoAlloc` (0 allocs for `*[]byte`, without -race
   only, see Tests).
3. `pool2.go` increments a plain `int` inside `New`, which sync.Pool calls from many goroutines: a data race.
   recycle documents that newFn, reset and keep must be safe for concurrent use; `TestPool_concurrentGetPut`
   counts with atomics under -race.
4. `pool2.go` coordinates with `time.Sleep` (§7): no sleep in any test here.

### b. The authority (opened 2026-11-13)
- pkg.go.dev `sync`, "type Pool". Opened: https://pkg.go.dev/sync#Pool, 2026-11-13. "A Pool is a set of temporary
  objects that may be individually saved and retrieved. Any item stored in the Pool may be removed automatically
  at any time without notification." "A Pool must not be copied after first use." "Pool's purpose is to cache
  allocated but unused items for later reuse, relieving pressure on the garbage collector. That is, it makes it
  easy to build efficient, thread-safe free lists." New field: "optionally specifies a function to generate a
  value when Get would otherwise return nil." "func (*Pool) Get": "selects an arbitrary item from the Pool,
  removes it from the Pool, and returns it ... If Get would otherwise return nil and p.New is non-nil, Get
  returns the result of calling p.New." "func (*Pool) Put": "adds x to the pool." Example note: "The Pool's New
  function should generally only return pointer types, since a pointer can be put into the return interface
  value without an allocation."
- golang/go#23199, Joe Tsai (dsnet), "sync: Pool example suggests incorrect usage", 2017-12-20, open. Opened:
  https://github.com/golang/go/issues/23199, 2026-11-13. "sync.Pool assumes memory cost of each element is
  approximately the same to be efficient"; "Dynamically growing unbounded buffers can cause large amounts of
  memory to be pinned and never freed in live-lock situations." Mitigation proposed in the thread: size-gating
  (`if b.Cap() <= 1<<12 { pool.Put(b) }`), or bucketing by size. `WithKeep` is the size gate; bucketing is out
  of scope.

### c. Today's Go (GO_RELEASES.md)
GO_RELEASES.md lists no typed pool through go1.27: `sync.Pool` stores `any` (generic methods arrived in go1.27; `sync.Pool` was
not given a type parameter). Every caller still writes `p.Get().(*T)`, re-implements reset in each Put, and
forgets the size gate of #23199. The export remains needed. `weak`/`runtime.AddCleanup` (go1.24) are not pools.

### d. calcbench
None (LADDER.md §5: "none"). Nothing mathematical; no calcbench request.

### e. Exports

| ID | Name | Signature |
|---|---|---|
| `TB-RECYC-001` | `Pool` | `type Pool[T any] struct{...}`; methods `Get() T`, `Put(v T)` |
| `TB-RECYC-002` | `New` | `func New[T any](newFn func() T, opts ...Option[T]) *Pool[T]` |
| `TB-RECYC-003` | `Option` | `type Option[T any] func(*config[T])` |
| `TB-RECYC-004` | `WithReset` | `func WithReset[T any](reset func(T) T) Option[T]` |
| `TB-RECYC-005` | `WithKeep` | `func WithKeep[T any](keep func(T) bool) Option[T]` |

`New` carries its own ID (a plain `New` returning `*Pool[T]`, the wave 2 lesson). Signatures exactly as LADDER.md
§4; no deviations.

| Option | Default | Effect |
|---|---|---|
| `WithReset(reset)` | none (values pooled unchanged) | applied on Put to each kept value; its result is pooled |
| `WithKeep(keep)` | keep all | on Put, `keep(v) == false` drops v (not pooled, reset not called) |

No sentinel errors (nothing fails). Decisions on input:

| Input | sync.Pool | recycle |
|---|---|---|
| nil newFn, empty pool | Get returns nil (`any`) | Get returns the zero T |
| newFn returns a nil interface (T an interface type) | Get returns nil | Get returns the zero T (comma-ok assertion, no panic) |
| zero value `Pool[T]{}` | ready, no New | ready: Get returns the zero T, Put pools unchanged |
| nil Option, nil reset, nil keep | n/a | ignored |
| Put(nil interface) | ignored | ignored (sync.Pool's rule) |
| Put of a typed nil pointer | pooled | pooled; Get may return it. Callers that Put nil reject it with WithKeep (detecting a typed nil generically needs reflection, CLAUDE.md: no reflection) |
| panic in newFn, keep, reset | n/a (New panics reach Get) | not recovered, reaches the caller of Get/Put; a panicking Put pools nothing |

Why panics are not recovered: these callbacks run synchronously on the caller's goroutine (no worker), and
Get/Put have no error result (§4, and sync.Pool's shape); recovering would have to swallow the panic or return a
zero value silently. Propagating is what sync.Pool and sort.Slice do. `panics` is therefore not used.
Why keep runs before reset: keep judges the value as the caller left it (the capacity it grew to; reset usually
keeps capacity anyway), and a dropped value is not worth resetting.

#### `TB-RECYC-001` `Pool` (`recycle/pool.go:20`), `TB-RECYC-002` `New` (`recycle/pool.go:36`)
- **Pattern**: object pool (free list).
- **Authority**: pkg.go.dev sync "type Pool", "func (*Pool) Get", "func (*Pool) Put"; golang/go#23199.
- **Familiar shape**: `sync.Pool` (`Get`, `Put`, `New` as the constructor argument instead of a field), typed.
- **Goroutines**: none. **Cancellation**: none (never blocks). **Errors**: none; callback panics propagate.
- **Blocking**: never; Get may run newFn, Put may run keep and reset, on the caller's goroutine.
- **Ordering and counts**: Get returns an arbitrary pooled value; any pooled value may vanish at a GC, so
  reuse is never guaranteed (under -race sync.Pool also drops a random quarter of Puts).
- **Zero value**: ready to use (see table). Must not be copied after first use (vet copylocks via sync.Pool).
- **Clock**: none. **calcbench IDs called**: none.
- **Compared against**: github.com/valyala/bytebufferpool and `sync.Pool`, `compare/recycle/recycle_test.go`.
- **Allocation**: a pointer T allocates nothing on Get/Put in steady state; a value T (slice, struct) allocates
  on every Put (boxing). Documented in doc.go and tested.
- **Worked example**: `ExamplePool` (`hello`, `0`), `ExampleNew` (zero Pool and nil newFn: `true`, `0`).

#### `TB-RECYC-003` `Option`, `TB-RECYC-004` `WithReset`, `TB-RECYC-005` `WithKeep` (`recycle/option.go:9,24,37`)
- Functional options (Pike 2014, the citation flow and group use). `ExampleOption` (`0`), `ExampleWithReset`
  (`0 8`: length cut, capacity kept), `ExampleWithKeep` (a 1 MiB buffer dropped: `true`).

### g. Comparison
`compare/recycle/recycle_test.go` (written against bytebufferpool v1.0.0; not built or vetted by the builder):
write/Put/Get yields an empty buffer in bytebufferpool and in recycle with WithReset; WithKeep drops an oversized
buffer at once (bytebufferpool calibrates its max size from a percentile of usage, not compared: needs ~42000
Puts and is not deterministic); an empty constructor-less pool gives nil from sync.Pool and the zero T from
recycle. Benchmarks for both with ReportAllocs.

### h. Out of scope
Size-bucketed pools (an array of pools by capacity class, the other #23199 mitigation; additive later as a new
export if asked), a byte-buffer type of our own (bytebufferpool's `ByteBuffer`: callers use `*bytes.Buffer`),
pool statistics (hits/misses: an observability hook later), bounded pools that block when empty (that is a
semaphore or a resource pool such as `database/sql`'s, not a free list), connection pools (adapters).

## Tests
Files: `recycle/doc.go`, `recycle/pool.go`, `recycle/option.go`, `recycle/example_test.go`, `recycle/pool_test.go`,
`recycle/main_test.go` (TestMain with leak.Main), `recycle/race_on_test.go` / `race_off_test.go` (`raceEnabled`).
Examples for all five exports · newFn on empty pool · nil newFn for int, pointer, slice, interface · zero value ·
newFn returning a nil interface and Put(nil) · reset applied on every kept Put (counted) and a reused value is
reset (asserted only when identity shows reuse) · reset's result is what is pooled · keep drops oversized (reset not
called, Get calls newFn: deterministic because the pool holds nothing else) · keep accepts small · nil options ·
callback panics propagate and pool nothing · concurrent Get/Put (16 x 500) under -race with leak.Verify, a 10 s
bound, no value held twice (atomic CAS), every Put reset · go_54 defects 1-3 · benchmarks GetPut, GetPutParallel,
SyncPool baseline with ReportAllocs.
Determinism under -count=200: no test asserts reuse. `TestPool_go54_pointerGetPutNoAlloc` skips under -race (random
Put drops); without -race it relies on no GC during 100 allocation-free runs inside AllocsPerRun (GOMAXPROCS 1),
which only the 2-minute forced GC could break. If the verifier ever sees it flake, relax to `allocs < 1`.
Checks run: `gofmt -l recycle`, `go build ./...`, `go vet ./recycle/` all clean.

## For the lead
- INDEX.md rows: TB-RECYC-001 Pool, 002 New, 003 Option, 004 WithReset, 005 WithKeep.
- compare/go.mod needs `github.com/valyala/bytebufferpool v1.0.0`.
- doc/sources: pkg.go.dev sync "type Pool" (TB-RECYC-001..004); golang/go#23199 (TB-RECYC-001, 005); Pike 2014 (003).
- ASSUMPTIONS: nil newFn gives the zero T; callback panics propagate (not ErrPanic: no error result, synchronous);
  keep runs before reset; a typed nil pointer is pooled unless WithKeep rejects it.

REMAINING: none for the builder; `go test -race` and the compare build are the verify phase.
