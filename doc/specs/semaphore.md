# semaphore: a weighted counting semaphore with FIFO waiters and context cancellation

Status: SHIPPED (2026-11-13)
Package: `semaphore` · SHORT: `SEMA` · Layer: core · Tier: T1
Work folder: `doc/wave2_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #10
Builds on: nothing (standard library only; tests use `internal/leak`) · Decisions: D-006 · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim)
No code-heim repo shows this pattern (LADDER.md row 10: "standard"). Nothing read, no repo defects. The tests
guard instead against the failure modes the authority names: a large waiter starved by small ones (x/sync
`notifyWaiters` comment), a waiter cancelled at the moment it is granted leaking its permits (x/sync
`Acquire` "Acquired the semaphore after we were canceled"), and the panics x/sync raises on input.

### b. The authority (opened 2026-11-13)
- golang.org/x/sync/semaphore v0.25.0, pkg.go.dev. Opened: https://pkg.go.dev/golang.org/x/sync/semaphore,
  2026-11-13. `Weighted`: "provides a way to bound concurrent access to a resource. The callers can request
  access with a given non-negative weight." `NewWeighted`: "panics if n is negative." `Acquire`: "blocking
  until resources are available or ctx is done. On success, returns nil. On failure, returns ctx.Err() and
  leaves the semaphore unchanged." `TryAcquire`: "without blocking ... On failure, returns false and leaves the
  semaphore unchanged." `Release`: "releases the semaphore with a non-negative weight of n."
- Its source, `semaphore/semaphore.go` (master). Opened:
  https://raw.githubusercontent.com/golang/sync/master/semaphore/semaphore.go, 2026-11-13. Lines relied on:
  - L22-27 `NewWeighted` panics on n < 0. L42-44, L119-121, L133-135: `Acquire`, `TryAcquire`, `Release`
    panic on n < 0. L137-141: `Release` panics "released more than held".
  - L48-57: a ctx already done when `Acquire` takes the mutex fails even if the permits are free ("We prefer
    to fail even if we could acquire").
  - L58 and L123: the fast path takes permits only when `s.waiters.Len() == 0`, so a newcomer never jumps
    the queue (FIFO).
  - L68-73: n > size does not queue ("Don't make other Acquire calls block on one that's doomed to fail") but
    blocks on `<-done` until ctx ends: with `context.Background()` that is forever.
  - L80-98: cancelled while queued: if `ready` was already closed (granted), the permits are put back and
    other waiters notified; otherwise the waiter is removed, and if it was at the front, the waiters behind it
    are notified. L100-111: granted, but ctx is done: released, `ctx.Err()` returned.
  - L146-172 `notifyWaiters`: grant from the front while the front fits; stop at the first that does not
    ("under load that could cause starvation for large requests ... the writer will starve").
- Dijkstra, "Cooperating sequential processes", EWD123, 1965, section 3.2 "The Synchronizing Primitives".
  Opened: https://www.cs.utexas.edu/~EWD/transcriptions/EWD01xx/EWD123.html, 2026-11-13. V "increase[s] the
  value of its argument semaphore by 1 ... an indivisible operation"; P "decrease[s] the value of its argument
  semaphore by 1 as soon as the resulting value would be non-negative". Which waiting process proceeds after a
  V "is left unspecified". `Acquire(n)` is P applied n units at once (indivisibly), `Release(n)` is V by n; the
  FIFO choice of which waiter proceeds is ours (from x/sync), not Dijkstra's.

### c. Today's Go
The standard library has no counting semaphore (a buffered channel of struct{} is the idiom, unweighted and
without FIFO across weights). golang.org/x/sync/semaphore is the familiar API; toolbench does not depend on it
in core (CLAUDE.md: dependency-free core) and differs on purpose: no panic on any input (x/sync panics in five
places, L24, L43, L120, L134, L140), and `Acquire(n > size)` returns at once instead of blocking until ctx
ends. `pool` and `bulkhead` (T2, T3) build on it.

### d. calcbench
None (LADDER.md §5). The waiter queue is a small intrusive linked list under the mutex (O(1) removal of a
cancelled waiter); calcbench `algo/queue` has no remove-from-middle, and `container/list` would store `any`
(CLAUDE.md: generic, no type assertions).

### e. Exports

| ID | Name | Signature |
|---|---|---|
| `TB-SEMA-001` | `Weighted` | `type Weighted struct{...}`; `New(n int64) *Weighted`; methods `Acquire(ctx, n int64) error`, `TryAcquire(n int64) bool`, `Release(n int64)` |
| `TB-SEMA-002` | `ErrTooLarge` | `var ErrTooLarge = errors.New("semaphore: weight exceeds size")` |
| `TB-SEMA-003` | `ErrNegativeWeight` | `var ErrNegativeWeight = errors.New("semaphore: negative weight")` |
| `TB-SEMA-004` | `New` | `func New(n int64) *Weighted` |

No options. `New` carries its own ID, TB-SEMA-004: testing/index_test.go covers only New<T> where T has an ID under D-071, so a bare `New` needs one (verify finding, 2026-11-13).

**`New`, not `NewWeighted`** (LADDER.md §4 kept). `semaphore.New(10)` reads as the package's constructor
(`list.New`, `errors.New`); the type keeps x/sync's name `Weighted`, so `*semaphore.Weighted` fields and method
calls port unchanged and only the constructor line changes. Alternative that lost: `NewWeighted` for a
drop-in rename; it stutters only slightly, but there is one type in the package and a second constructor name
would be two ways to do one thing. Adding `NewWeighted` later is additive if Devin wants drop-in.

Decisions on input (each a deviation from x/sync, which panics or blocks):

| Input | x/sync | toolbench |
|---|---|---|
| `New(n < 0)` | panic | a semaphore of size 0 (only weight 0 is ever granted) |
| `Acquire(ctx, n < 0)` | panic | returns an error wrapping `ErrNegativeWeight`, at once, semaphore unchanged |
| `Acquire(ctx, n > size)` | blocks until ctx is done, returns ctx.Err() | returns an error wrapping `ErrTooLarge`, at once, semaphore unchanged, never queued |
| `Acquire(ctx, 0)` | granted when no one waits, else queued FIFO | same |
| ctx done before `Acquire` | ctx.Err() even if permits are free | same (input errors are checked first: a bad n is reported whatever the ctx) |
| nil ctx | panic (nil interface) | `context.Background()` |
| `TryAcquire(n < 0)`, `TryAcquire(n > size)` | panic / false | false, unchanged |
| `Release(n < 0)` | panic | no-op |
| `Release(n > held)` | panic "released more than held" | clamps: releases what is held (in use becomes 0), wakes waiters |
| `Release(0)` | wakes waiters | same |

Why clamp on over-release: `Release` has no error result (its signature is fixed by §4 and x/sync), a library
function must not panic on input (CLAUDE.md), and ignoring the call would leave permits held forever. Clamping
keeps the invariant 0 <= inUse <= size. The cost: the caller's double release is not reported. Alternative that
lost: count over-releases and expose them (an extra export; observability is a later hook if asked).

Why `ErrTooLarge` at once: the request can never be granted; x/sync's block-until-ctx turns a caller bug into a
hang under `context.Background()`. Returning at once is still "leaves the semaphore unchanged".

#### `TB-SEMA-001` `Weighted`
- **Pattern**: counting semaphore (weighted P/V), Dijkstra 1965; FIFO waiter queue as x/sync.
- **Authority**: Dijkstra EWD123 §3.2; pkg.go.dev golang.org/x/sync/semaphore `Weighted` and its source.
- **Familiar shape**: x/sync `Weighted` (`Acquire`, `TryAcquire`, `Release`), constructor `New`.
- **Goroutines**: none. `Acquire` blocks the caller's goroutine only.
- **Cancellation**: ctx done before the call: `ctx.Err()`, unchanged. Done while queued: the waiter leaves the
  queue, `ctx.Err()`; if it was at the front, the waiters behind it are re-checked (a large front waiter that
  gives up must not keep smaller ones blocked). Done at the moment of grant (permits already counted for it):
  they are given back and the queue re-checked, `ctx.Err()`; no permit leaks. Granted and ctx done when the
  waiter wakes: x/sync's rule, release and return `ctx.Err()`.
- **Errors**: `ErrNegativeWeight`, `ErrTooLarge` (wrapped with n and size via `%w`), `ctx.Err()`. No panics.
- **Blocking**: `Acquire` blocks while the permits are not free or anyone is queued ahead (FIFO). A large
  waiter at the front blocks smaller later ones even when they would fit (no starvation of large requests).
  `TryAcquire` never blocks and never jumps the queue (false while anyone waits). `Release` never blocks.
- **Ordering**: waiters are granted in arrival order (arrival = taking the mutex).
- **Zero value**: a semaphore of size 0 (only weight 0 is granted); create with `New`. Must not be copied
  after first use (holds a `sync.Mutex`; vet copylocks).
- **Clock**: none (does not wait on time).
- **calcbench IDs called**: none.
- **Compared against**: golang.org/x/sync/semaphore, `compare/semaphore/semaphore_test.go`.
- **Implementation**: `semaphore/weighted.go:26`. The queue is an intrusive doubly linked list of waiters (O(1)
  removal of a cancelled waiter, no `any` assertions, one allocation per queued Acquire, none on the fast path);
  not `container/list`, which stores `any`. An unexported field `queued func()`, called under the mutex when a
  waiter joins the queue, is the test seam that lets tests know a goroutine is queued without sleeping; it is
  never set outside the package's tests.
- **Worked example**: `ExampleWeighted` (`// Output:` <nil> false true true), `ExampleNew` (size -1 is size 0),
  `ExampleErrTooLarge`, `ExampleErrNegativeWeight` in `semaphore/example_test.go`.

#### `TB-SEMA-002` `ErrTooLarge`, `TB-SEMA-003` `ErrNegativeWeight`
Sentinels matched with `errors.Is`; the returned error adds n and size.

### g. Comparison
`compare/semaphore/semaphore_test.go` against golang.org/x/sync/semaphore (the version compare/go.mod resolves;
written against v0.25.0): the same script of TryAcquire/Release gives the same booleans; FIFO (a queued
large waiter blocks a later small TryAcquire in both); cancel while queued returns ctx.Err() and leaves the
permits free in both. Differences asserted by name: n > size (x/sync blocks until ctx, ours returns
ErrTooLarge at once), over-release and negative weights (x/sync panics, ours clamps / errors; the x/sync side
is run under `recover`).

### h. Out of scope
Reader-writer locks (`sync.RWMutex`), a bulkhead with a queue limit and fail-fast (`bulkhead`, T3), a worker
pool (`pool`, T2), distributed semaphores (an adapter with `lock`), resizing a semaphore, `AcquireTimeout`
(callers use `context.WithTimeout`), introspection (`InUse`, `Waiting`): not in §4; additive later if asked.

## Tests
ExampleWeighted · New(n<0) is size 0 · zero value · TryAcquire happy/invalid/queue-respecting · Acquire
invalid inputs (negative, too large, nil ctx, done ctx) · FIFO order asserted by channels (waiters queued one by
one, confirmed queued by TryAcquire(0) returning false, granted in order) · large waiter blocks later small one,
and a cancelled large front waiter unblocks the small one · cancel while waiting returns ctx.Err and leaks no
permit · cancel at the moment of grant (race loop) leaks no permit · Release over-held clamps, negative no-op ·
concurrent stress under -race counting max in flight with atomics · leak.Main + leak.Verify · BenchmarkWeighted
(uncontended Acquire/Release, parallel) with ReportAllocs.

Files: `semaphore/doc.go`, `semaphore/weighted.go`, `semaphore/errors.go`, `semaphore/example_test.go`,
`semaphore/weighted_test.go` (internal: TestMain with leak.Main, the queue seam), `compare/semaphore/semaphore_test.go`
(written, not built or vetted). Checks run: gofmt -l, go build ./..., go vet ./semaphore/ all clean.

## For the lead
- INDEX.md rows: TB-SEMA-001 Weighted, TB-SEMA-002 ErrTooLarge, TB-SEMA-003 ErrNegativeWeight, TB-SEMA-004 New.
- Verify fix: TestWeighted_Acquire_largeWaiterBlocksSmall released 3 permits at once, freeing 4, so large and small were
  granted in the same Release and raced on the order channel (the FIFO code was right). It now releases one
  permit at a time, so small can only be granted after large releases.
- doc/sources: x/sync/semaphore (pkg.go.dev + source) and Dijkstra EWD123 §3.2, used by TB-SEMA-001.
- compare/go.mod needs `golang.org/x/sync` (the same requirement B4 group has).
- Decisions for Devin (D-ids or ASSUMPTIONS): New(n<0) = size 0; Release(n > held) clamps; Acquire(n > size)
  returns ErrTooLarge at once; input errors are checked before ctx.

REMAINING: none for the builder; `go test -race` and the compare build are the verify phase.
