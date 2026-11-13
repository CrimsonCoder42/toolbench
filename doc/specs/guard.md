# guard: a value behind its own mutex, and a goroutine that owns its state (confinement)

Status: SHIPPED (2026-11-13)
Package: `guard` · SHORT: `GUARD` · Layer: core · Tier: T1
Work folder: `doc/wave2_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #16
Builds on: `panics` (TB-PANIC-002, TB-PANIC-003); tests use `internal/leak` · Decisions: D-006 · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim, read only, never run)
`_reference/code-heim/go_20_mutexes_and_confinement`, three programs selling 500 tickets to many buyers:
- `before/main.go`: 2000 goroutines decrement a shared `*int` with no guard. A data race (Go memory model,
  "Introduction": a write concurrent with another access); the count sold is wrong and `-race` reports it.
  Test case: `TestGuarded_go20UnguardedCounter` runs the same shape through `Guarded.Do` and asserts the exact total.
- `with_mutex/main.go`: a package-level `var mutex sync.Mutex` beside a pointer passed in. Correct, but the
  mutex is a global (CLAUDE.md "no globals") and nothing ties it to the value it guards. Ours: the mutex sits
  beside the value inside `Guarded[T]`, unexported, so the value cannot be reached without it.
- `with_confinement/main.go`: `manageTicket` owns `tickets` and serves requests from a channel. **Defect**: its
  `for { select {...} }` has no return; the `doneChan` case prints and loops again, so the owner goroutine never
  exits (it lives until `main` returns) and a second `doneChan` send would block forever. It also prints from the
  owner (no logging in a library) and has no way to give the final state back. Test case:
  `TestConfine_go20OwnerNeverExits` asserts that `Confine` returns on ctx cancel and on `reqs` closed, returns
  the final state, and that the goroutine count is back to the baseline (`leak.Verify`).
Outdated: `for i := 0; i < n; i++` (now `range n`), `chan bool` as a done signal (now `context.Context`).

### b. The authority (opened 2026-11-13)
- The Go Memory Model, go.dev, "Synchronization" / "Locks". Opened: https://go.dev/ref/mem, 2026-11-13. Relied
  on: "For any sync.Mutex or sync.RWMutex variable l and n < m, call n of l.Unlock() is synchronized before call
  m of l.Lock() returns." This is why every write made inside one `Do` is visible to the next `Do` and `Load`,
  and why nothing outside `Do` may touch the value (no happens-before edge, a data race). Also "Introduction":
  the definition of a data race, and "Don't be clever."
- Effective Go, go.dev, "Concurrency" / "Share by communicating". Opened:
  https://go.dev/doc/effective_go#sharing, 2026-11-13. Relied on: "Only one goroutine has access to the value at
  any given time. Data races cannot occur, by design." and "Do not communicate by sharing memory; instead, share
  memory by communicating." (that is `Confine`), and "This approach can be taken too far. Reference counts may be
  best done by putting a mutex around an integer variable" (that is `Guarded`).
- Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4 "Confinement" (ad hoc vs lexical confinement) and ch.3
  "The sync Package" / "Mutex and RWMutex". **Not opened** (the book is not online); cited for the pattern name
  "confinement" only. Every behaviour above rests on the two pages that were opened.

### c. Today's Go
The standard library has `sync.Mutex` and channels but no type that keeps a mutex beside the one value it
guards, and no owner loop with a defined stop. `sync.Mutex` is wrapped, never re-implemented (rule 12).
`sync/atomic.Pointer[T]`/`atomic.Value` cover a value replaced whole; `Guarded` is for read-modify-write of a
compound value. Why toolbench still needs it: ratelimit (Keyed map) and cache (wave 4/5) hold a map plus a
calcbench structure under one lock; the repos hand-roll it 4 times (LADDER.md row 16). Nothing in go1.27 or
golang.org/x replaces it.

### d. calcbench
None. No mathematics.

### e. Exports

| ID | Name | Signature |
|---|---|---|
| `TB-GUARD-001` | `Guarded` | `type Guarded[T any] struct{ mu sync.Mutex; v T }`; `func (g *Guarded[T]) Do(fn func(*T))`; `func (g *Guarded[T]) Load() T` |
| `TB-GUARD-002` | `Confine` | `func Confine[S any](ctx context.Context, state S, reqs <-chan func(*S)) (S, error)` |

No options. No sentinel of our own: `Confine` returns `context.Cause(ctx)` or a `*panics.PanicError`.

#### `TB-GUARD-001` `Guarded`
- **Pattern**: mutual exclusion (monitor: a mutex beside the value it guards).
- **Authority**: Go Memory Model, "Locks"; Effective Go, "Share by communicating" (the mutex-around-a-counter
  sentence).
- **Familiar shape**: `sync.Mutex` + the field it guards, made one type. `Do(fn)` mirrors `sync.Once.Do` in
  shape (a callback run under the type's guard).
- **Goroutines**: none; `fn` runs on the caller's goroutine.
- **Zero value**: ready to use; `v` is T's zero value. Contains a `sync.Mutex`, so `go vet` copylocks flags a
  copy of a `Guarded` (pass `*Guarded[T]`, embed it in a struct used by pointer). Must not be copied after
  first use, like `sync.Mutex`.
- **Do**: locks, runs `fn(&v)`, unlocks with `defer`. Decisions:
  - *Re-entry*: `sync.Mutex` is not re-entrant. `fn` calling `g.Do` or `g.Load` on the same `g` deadlocks
    (Go's deliberate choice: a re-entrant lock hides broken invariants). Documented; not detectable without a
    goroutine id, so not checked.
  - *Panic in fn*: the deferred unlock runs, then the panic continues up the caller's stack unchanged (not
    recovered). Why: `fn` runs on the caller's goroutine, so rule 6 ("panic ... on OUR goroutine") does not
    apply, and `Do` has no error result; this is what `sync.Once.Do` and a hand-written `defer mu.Unlock()` do.
    The mutex is unlocked afterwards, so the next `Do` does not deadlock; the value may be half-updated (the
    caller's invariant). Alternative that lost: recover and swallow (a silent failure) or a new `TryDo` with an
    error result (not in §4; a caller can wrap with `panics.Try(func() error { g.Do(fn); return nil })`).
  - *Nil fn*: a no-op (CLAUDE.md "never panics on input"); the mutex is not even taken.
  - *Escape*: the `*T` is valid only until `fn` returns. Keeping it (storing it, sending it on a channel,
    starting a goroutine with it) and using it later is a data race the type cannot prevent; documented on `Do`.
    Pointers, maps and slices inside T that `fn` hands out escape the same way.
- **Load**: locks, copies `v`, unlocks. The copy is **shallow**: a map, slice, pointer or channel in T is shared
  with the guarded value, and reading through it outside `Do` is a race. For such T use `Do` and copy deep
  inside `fn`.
- **Errors / Cancel**: none. **Blocking**: until the mutex is free (no ctx: a mutex wait is not cancellable,
  as `sync.Mutex.Lock`; `fn` should be short).

#### `TB-GUARD-002` `Confine`
- **Pattern**: confinement (one goroutine owns the state; others send it requests).
- **Authority**: Effective Go, "Share by communicating"; Cox-Buday, ch.4 "Confinement" (not opened).
- **Deviation from LADDER.md §4: the result is `(S, error)`, not `S`.** A request that panics must not kill the
  process and must not be swallowed. `Confine` recovers it with `panics.Try`, stops serving, and returns the
  state as it was when the panic happened together with the `*panics.PanicError`. An error result is also how
  the caller tells "reqs closed, every request ran" (`nil`) from "stopped by ctx" (`context.Cause(ctx)`). The
  alternative, `S` only and re-panic, lost: Confine's loop is the owner of everything it runs, and rule 6 plus
  CLAUDE.md "a panic inside a worker is recovered and returned as an error".
- **Goroutines**: none started. The loop runs on the caller's goroutine, which becomes the owner (the caller
  writes `go func() { s, err = guard.Confine(...) }()` when it wants a separate owner). This is the fix for the
  go_20 defect: the loop has exactly two exits, so that goroutine ends.
- **Cancellation**: ctx done before the call: returns `state, context.Cause(ctx)` without running a request.
  ctx done while waiting: returns at once. ctx done while a request runs: that request finishes (it is not
  interrupted), then `Confine` returns. A request already received runs to completion even if ctx is done by
  then (its sender's send has completed and may wait on a reply; dropping it lost values, reviewer BLOCKER); ctx
  is checked before every receive, so no request is received after the loop sees ctx done. A nil ctx is `context.Background()`.
- **reqs**: closed: every request already sent runs, then `state, nil`. A nil `reqs` is handled as closed
  (returns `state, nil` at once), not a hang (rule 3). A nil request func is skipped.
- **Errors**: `nil` (reqs closed or nil), `context.Cause(ctx)`, or `*panics.PanicError` (`errors.Is(err,
  panics.ErrPanic)`). `runtime.Goexit` in a request is not a panic: it ends the caller's goroutine and Confine
  does not return (as `panics.Try`).
- **Blocking**: until reqs is closed, ctx is done or a request panics. Senders block until the owner receives
  (backpressure; the caller chooses the buffer of `reqs`). Replies: a request closes over its own reply channel.
- **State**: `state` is copied in; requests get `*S` to the owner's copy, valid only during the call (same
  escape rule as `Do`). The returned S is a shallow copy of the final state.

### g. Comparison
None (LADDER.md row 16: no library replaced). No `compare/guard/`.

### h. Out of scope
`RWMutex`-based read sharing (`RLoad`): add as a new export when a consumer shows contention. A context-aware
lock (`semaphore.Weighted` of 1 covers it). Deep copy in `Load` (needs reflection or a Clone method). A
supervisor that restarts a failed owner (lifecycle, wave 3). Re-entrant locking (never).

## Tests
`guard/guarded_test.go`, `guard/confine_test.go`: `ExampleGuarded`, `ExampleConfine` with `// Output:`; concurrent
`Do` increments (1000 goroutines) give the exact total under -race; `TestGuarded_go20UnguardedCounter` (500
tickets, 2000 buyers, exactly 500 sold); panic in `Do` propagates and leaves the mutex unlocked (`guard/guarded_internal_test.go`,
`TryLock` on the inner mutex, so a regression fails instead of hanging); nil fn; zero value; `Load` is shallow (slice
shares backing array); Confine returns on ctx cancelled before, while waiting, after reqs closed, with nil
reqs, nil ctx, nil request, a request queued behind one that cancels is not run,
`TestConfine_receivedRequestRuns` (sender cancels right after its unbuffered send; the request still runs and
replies, 200 rounds), `runtime.Goexit` in a request, panic in a request → `ErrPanic` with the state at
the panic; `TestConfine_go20OwnerNeverExits` (owner in its own goroutine, stopped by cancel and by close,
`leak.Verify`); `leak.Main` in `TestMain`; `BenchmarkGuarded_Do`, `BenchmarkConfine` with `ReportAllocs`.

## For the lead
- DECISIONS (D-needed): `Confine` returns `(S, error)` instead of `S` (LADDER.md §4 deviation, argued in §e).
- DECISIONS (D-needed): a panic in `Guarded.Do`'s fn is not recovered: unlock, then it propagates (alternative:
  recover into a new `TryDo`; lost, not in §4 and `panics.Try` composes).
- ASSUMPTIONS: nil `reqs` returns at once like a closed channel; nil fn / nil request are no-ops; nil ctx is
  `context.Background()` (as clock).
- `doc/sources/`: Go Memory Model "Locks" → TB-GUARD-001; Effective Go "Share by communicating" →
  TB-GUARD-001, -002; Cox-Buday ch.4 "Confinement" (not opened) → TB-GUARD-002.
- INDEX.md rows TB-GUARD-001, TB-GUARD-002; taxonomy row `guard` now builds on `panics`.

REMAINING: none.

## Align (2026-11-13)

Work folder: `doc/align_2026-11-13/` · Task D1 (decided by Devin) · Builder: guard · Additive only.

### Matrix row
`MATRIX.md` row 9: mutual exclusion, mutex beside the value · TB-GUARD-001 · REPEAT 4/11/15 · verdict PARTIAL
(sub-REPEAT 3: keyed mutex). List (b) rank 1: keyed mutex (lock striping per key, idle entries removed,
WithMaxKeys) → `guard`, builds D1. Scale table: "one Guarded serialises every key (go-ratelimit's defect)" → D1 Keyed.
`M1.md` row "mutual exclusion": uncovered → tests (1) Keyed: distinct keys do not serialise, same key does, bounded
keys, ref-counted removal at zero; (3) a `Guarded[map[K]V]` Example with N writers under -race.

### References read (a reading list, never cited as authority; `_reference/`, read only)
- A-Database-Built-With-Golang `main.go:173-185` `getOrCreateMutex`: a `map[string]*sync.Mutex` that only grows
  (no removal, no bound), readers take no lock. Ours: entries reference-counted, deleted at zero, `WithMaxKeys`.
- go-ratelimit `per-client-rate-limit/main.go:19-70` (`:51-52`): one mutex serialises every client. Ours: the
  table mutex is held O(1); the wait is per key.
- Videocall-Chat-GO-Project `internal/server/server.go:62-68` (ranges the global `Rooms` map without `RoomsLock`),
  `pkg/webrtc/peers.go:17,49,61-68`; GO-P2P-Blockchain `main.go:185-187` (reads outside the mutex).
- Thirteen repos write a package-level map or slice from handler goroutines: A-Blockchain-With-Golang `main.go:74`,
  go-chi-CRUD `controllers/article.go:46`, Complete_GO_Blockchain `core/server.go:314`, CSRF-Golang-Killer-Project
  `db/db.go:21`, sitemap-crawler `main.go:126`, langchain-GO `chains/vacation.go:69`, Videocall-Chat-GO-Project
  `internal/server/server.go:62`; code-heim go_20 `before/main.go`. → `ExampleGuarded_map`, `ExampleGuarded_slice`,
  `TestGuarded_mapManyWriters`.
- Check-then-act races that an in-process keyed lock is the first fix for (LADDER2 §2.1):
  Golang-JWT-Gin-Authentication-project `controllers/userController.go:62-80`, Golang-Serverless-Project
  `pkg/user/user.go:83-99`.

### Authority (`.claude/rules/sources.md`)
- The Go Memory Model, go.dev, "Synchronization", "Locks" (opened for TB-GUARD-001, 2026-11-13; relied on again):
  Unlock n is synchronized before Lock m returns. For the per-key channel: "Channel communication" — a send on a
  channel is synchronized before the completion of the corresponding receive, and for a buffered channel the
  k-th receive is synchronized before the (k+C)-th send completes (C = 1 here). So the holder's writes in `fn`
  happen before the next holder's `fn` on the same key: the holder's `<-e.lock` (receive) is synchronized before
  the waiter's `e.lock <- struct{}{}` (send) completes.
- pkg.go.dev `sync`, "type Mutex", "func (*Mutex) TryLock" (go1.18): the `TryDo` shape and its warning ("use of
  TryLock is often a sign of a deeper problem"): documented on `TryDo`.
- Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.3 "The sync Package", "Mutex and RWMutex". **Not opened**
  (the book is not online); named for the pattern only.
- Compare-against (shape only, not opened as authority): github.com/puzpuzpuz/xsync (`MapOf`-based striping),
  golang.org/x/sync/singleflight (`Group`: a mutex beside `map[string]*call`, entries deleted when the last
  caller leaves: the same table shape; ours adds a bound, a cancellable wait and no result sharing).

### Today's Go (go1.27)
`sync.Mutex` (`TryLock` since 1.18) and buffered channels. No per-key lock in the standard library or
golang.org/x; `singleflight` shares a result, it does not serialise. `sync.Mutex.Lock` cannot be cancelled, so the
per-key lock is a `chan struct{}` of capacity 1 (Cox-Buday's "channel as a lock"; the Go Memory Model's buffered
channel rule above), which a `select` can abandon on `ctx.Done()`. `sync.WaitGroup.Go` (1.25) in tests.

### calcbench
None. A reference-counted `map[K]*entry` under one mutex: entries leave at refcount zero, so no eviction
structure (LRU, TTL) is needed, and no formula.

### Exports

| ID | Name | Signature |
|---|---|---|
| `TB-GUARD-003` | `Keyed` | `type Keyed[K comparable] struct{ mu sync.Mutex; table map[K]*keyedEntry; maxKeys int }`; `Do(ctx, key, fn func() error) error`; `TryDo(key, fn func() error) (ran bool, err error)`; `Len() int` |
| `TB-GUARD-004` | `NewKeyed` | `func NewKeyed[K comparable](opts ...KeyedOption) *Keyed[K]` |
| `TB-GUARD-005` | `KeyedOption` | `type KeyedOption func(*keyedConfig)` |
| `TB-GUARD-006` | `WithMaxKeys` | `func WithMaxKeys(n int) KeyedOption` |
| `TB-GUARD-007` | `DefaultMaxKeys` | `const DefaultMaxKeys = 10_000` |
| `TB-GUARD-008` | `ErrKeys` | `var ErrKeys = errors.New("guard: key bound full")` |

#### `TB-GUARD-003` `Keyed` (`guard/keyed.go`)
- **Pattern**: lock striping (one lock per key; mutual exclusion per key).
- **Shape**: `Do(ctx, key, fn)` as `Guarded.Do` with a key and an error result; `TryDo` as `sync.Mutex.TryLock`.
  Named `Keyed` as `ratelimit.Keyed` (the per-key version of the package's base type).
- **Goroutines**: none. `fn` runs on the caller's goroutine; no sweeper (entries are removed by the last caller
  to leave, so nothing idles in the table).
- **Zero value**: ready to use (`table` made on first use, `maxKeys` 0 means `DefaultMaxKeys`). Holds a
  `sync.Mutex`: must not be copied after first use (vet copylocks). `NewKeyed` only applies options.
- **Do**: (1) ctx already done → `context.Cause(ctx)` without touching the table. (2) Under `mu`: key not
  present and `len(table) >= maxKeys` → `ErrKeys` wrapped with the count (`"%w: %d keys held or waited on"`);
  otherwise get-or-create the entry and `refs++` (holders + waiters). (3) `select` on `e.lock <- struct{}{}` and
  `ctx.Done()`; cancelled → `refs--` (delete at 0), return `context.Cause(ctx)`. (4) `panics.Try(fn)`; deferred:
  `<-e.lock`, then `refs--` under `mu`, delete at 0. The deferred release also runs on `runtime.Goexit` in `fn`.
  Deletion only at refcount 0 means a pointer anyone holds is always counted, so there is no ABA: a late caller
  that finds the entry while the holder is between `<-e.lock` and `refs--` simply becomes the next holder.
- **Panic in fn**: recovered via `panics.Try` and returned as `*panics.PanicError` (`errors.Is(err,
  panics.ErrPanic)`) with the key released. **Differs from `Guarded.Do`**, which lets the panic propagate: `Do`
  here has an error result (rule 6 shape), `Guarded.Do` has none. Stated on both doc comments.
- **Nil fn**: a no-op; `Do` returns nil and `TryDo` returns `(true, nil)`; the table is not touched (as
  `Guarded.Do`). **Nil ctx**: `context.Background()` (as `Confine`).
- **A key not equal to itself** (a NaN float, or a struct holding one): `delete` on a map never removes it, so
  the entry would leak; refused with `ErrKeys` (as `ratelimit.Keyed`), the table untouched. **An interface key
  holding an uncomparable value** (`Keyed[any]` given a `[]int`): comparing or hashing it panics in Go; `keyError`
  compares it first with the panic recovered and returns `ErrKeys` ("key is not comparable"), before any map
  access, so the table mutex is never left locked (reviewer MUST, 2026-11-13). Every path that takes `g.mu`
  releases it by `defer` (`acquire`, `tryAcquire`, `release`, `Len`).
- **Re-entry**: `fn` calling `Do`/`TryDo` on the same key deadlocks/fails (not re-entrant, as `sync.Mutex`);
  a different key is fine (no lock ordering is imposed; the caller orders keys to avoid A/B-B/A deadlock).
- **Errors**: `context.Cause(ctx)`, `ErrKeys` (wrapped: the count, "key is not equal to itself" or "key is not
  comparable"), `*panics.PanicError`, or `fn`'s error unchanged.
- **Blocking**: `Do` waits for the key or ctx; `TryDo` never blocks (the non-blocking send under `mu`).
- **Len**: distinct keys held or waited on now (0 when idle). A method, no ID of its own (as `Do`, `Load`).

#### `TB-GUARD-004..008`
`NewKeyed` applies options to a new `Keyed` (nil options ignored). `KeyedOption` is named with the type's
name, not `Option`, so a future option for `Guarded` or `Confine` cannot be confused with it (D-needed below).
`WithMaxKeys(n)`: bound on distinct keys held or waited on at once; `n <= 0` means `DefaultMaxKeys` (as
`ratelimit.WithMaxKeys`). `DefaultMaxKeys = 10_000` (as `ratelimit.DefaultMaxKeys`). `ErrKeys` is the sentinel,
always wrapped with the count.

### Scale note
- **Bounded memory**: yes. The table holds at most `maxKeys` entries, each `(chan struct{}, int)`; an entry
  exists only while someone holds or waits on its key. No sweeper, no goroutine, no idle entries (unlike
  `ratelimit.Keyed`, which must keep idle buckets for their tokens).
- **Contention point**: the table mutex, held for O(1) work per `Do` (map lookup, counter) twice per call. Per-key
  waiting is on the entry channel: a hot key queues its own callers and no other key's (from the code; the
  benchmarks measure throughput, not fairness).
- **Per-call allocations**: an entry (struct + `chan`) is allocated by every `Do`/`TryDo` that finds its key
  idle, because `releaseLocked` deletes the entry at refcount 0: an uncontended call allocates 2 objects
  (measured by the reviewer: 2 allocs, 128 B/op in `BenchmarkKeyed_Do`); plus the caller's closure and the
  `ErrKeys` message when refused. The allocation is amortised only under contention (the second and later
  callers on a busy key join the existing entry). This is the price of "no table growth, no sweeper" taken
  literally. Hardening option, not done now (behaviour unchanged): keep idle entries up to the bound and drop
  them only when a new key needs the slot, or recycle entries through a free list; either makes a warm key
  allocation-free at the cost of a table that holds idle keys. Decide on a number from a consumer.
- **At 10x callers**: from the code, the table mutex is held for a map lookup and a counter per call, so it is
  not expected to show; not measured. **At 100x**: from the code, the table mutex is the one serial point every
  key shares (the same shape as `ratelimit.Keyed` and `singleflight.Group`); if a benchmark ever shows it, the
  fix is sharding the table by hash of the key (N tables, N mutexes) as a new option, not a change to `Do`. A
  hot key at 100x is the caller's own serialisation, which striping cannot fix (shorten `fn`). How the runtime
  orders waiters on the entry channel is not relied on or claimed.
- **How this change addresses the row**: a map of per-key work behind one `Guarded` serialises every key
  (go-ratelimit `:51-52`); `Keyed` serialises only same-key callers, and bounds and empties the table that
  A-Database (`:173-185`) grows forever.

### Tests (`guard/keyed_test.go`, `guard/guarded_test.go`; written, not run by the builder)
`ExampleKeyed` (`// Output:`); `TestKeyed_goRatelimitOneLockAllKeys` (distinct keys inside `fn` at the same time,
proved with two channels and a bounded select; go-ratelimit `:51-52`); `TestKeyed_sameKeySerialises` (1000
goroutines, one key, exact count under -race); `TestKeyed_aDatabaseGetOrCreateMutexGrows` (10 000 distinct keys,
`Len() == 0` after; A-Database `:173-185`); `TestKeyed_cancel` (before: cause, no entry; while a holder holds:
cause, `Len() == 1`, holder unlocks cleanly, key free after; after: no effect); `TestKeyed_TryDo` (false while
held, true when free, fn not run when false); `TestKeyed_ErrKeys` (bound 2, third key refused with the count in
the message, an existing key not counted, admission again after release); `TestKeyed_panicInFn` (Do and TryDo:
`ErrPanic`, key free, `Len() == 0`); `TestKeyed_nilFn`; `TestKeyed_zeroValue` (and `NewKeyed(nil)`);
`TestKeyed_nilCtx`; `TestKeyed_keyNotEqualToItself` (NaN → `ErrKeys`, `Len() == 0`);
`TestKeyed_uncomparableKeyIsErrKeys` (`Keyed[any]` with a `[]int` key: Do and TryDo → `ErrKeys`, `Len` still
answers from another goroutine, a later Do on a good key works); `TestKeyed_fnError` (fn's error unchanged);
`TestKeyed_WithMaxKeysNonPositive`; `ExampleNewKeyed`, `ExampleWithMaxKeys` (covers `KeyedOption` and
`DefaultMaxKeys` in its text), `ExampleErrKeys`, each with `// Output:`; `leak.Verify` in each;
`BenchmarkKeyed_Do`, `BenchmarkKeyed_DoDistinctKeys` (`ReportAllocs`). Guarded: `ExampleGuarded_map`, `ExampleGuarded_slice`, `TestGuarded_mapManyWriters` (the
thirteen repos above, named in its comment).

### For the lead
- DECISIONS (D-needed): option type named `KeyedOption` (not `Option`) so a later `Guarded`/`Confine` option
  cannot be confused with it; alternative `Option` shared by the package (as `ratelimit.Option`) lost because
  its options would not all apply to every constructor.
- DECISIONS (D-needed): a panic in `Keyed.Do`'s fn IS recovered (error result), unlike `Guarded.Do`; both doc
  comments state the difference.
- ASSUMPTIONS: nil fn is a no-op (`Do` nil, `TryDo` `(true, nil)`), table untouched; nil ctx is Background; a
  key not equal to itself, or an interface key holding an uncomparable value, is refused with `ErrKeys` (never a
  panic); `WithMaxKeys(n <= 0)` is the default; `Len` is a method without its own ID.
- TODO (hardening, needs a consumer's number): keep or recycle idle entries so an uncontended `Do` on a warm key
  does not allocate its entry and channel each time (scale note above).
- `doc/sources/`: Go Memory Model "Locks", "Channel communication" → TB-GUARD-003; pkg.go.dev sync "Mutex",
  "TryLock" → TB-GUARD-003; Cox-Buday ch.3 "Mutex and RWMutex" (not opened) → TB-GUARD-003; Pike "Self-referential
  functions" → TB-GUARD-005, -006; pkg.go.dev errors "func Is" → TB-GUARD-008; singleflight (shape note) →
  TB-GUARD-003, -007.
- INDEX.md rows TB-GUARD-003..008 (regenerate from `doc.go` and the doc comments).

### Measured (VERIFY, re-run after the reviewer fixes, 2026-11-13, go1.27.3 darwin/arm64, -race on; raw in doc/align_2026-11-13/out/bench_final.txt)
BenchmarkKeyed_Do 1 648 ns/op, 128 B/op, 2 allocs/op (one idle key: every Do allocates the entry and its channel, since the entry
is removed at refcount 0; amortised only under contention on that key: the hardening option above). BenchmarkKeyed_DoDistinctKeys
2 347 ns/op, 117 B/op, 1 alloc/op. ns/op under -race.
