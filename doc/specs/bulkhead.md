# bulkhead: bounded concurrent calls to one dependency, fail fast or a bounded FIFO wait

Status: SHIPPED (2026-11-13)
Package: `bulkhead` · SHORT: `BULK` · Layer: core · Tier: T3
Work folder: `doc/wave6_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #28 (and §2 "load shedding / queue-based load leveling" → `WithQueue`)
Builds on: `semaphore` (TB-SEMA-001/004: all admission), `clock` (queue timeout, Event.Waited), `panics` (Try, ErrPanic)
· calcbench: none (see d) · Decisions: D-023 (count <= 0 in a long-lived constructor is an error) · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; a reading list, never cited)

No code-heim repo shows this pattern (BRIEF B1). No repo defects; the defects tested are the ones the
authorities name: a slow dependency exhausting the caller's resources (max concurrency asserted with atomics),
an unbounded wait (queue bound + queue timeout), a wait that cannot be observed (event hook).

### b. The authority

- Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns", "Bulkheads" (also
  "Timeouts", "Shed Load"). A BOOK, NOT OPENED: cited as the origin of the pattern (partition resources so a
  failure in one part cannot sink the whole).
- Microsoft, "Bulkhead pattern", Azure Architecture Center (ms.date 2026-03-19). Opened:
  https://learn.microsoft.com/azure/architecture/patterns/bulkhead, 2026-11-13. Relied on: "Context and
  problem": "the resources that the client's request uses might remain unavailable for an extended period. As
  requests to the service continue, those resources might be exhausted ... Eventually, the consumer can't send
  requests to any other services" (one Bulkhead per dependency; fail fast when full). "Solution": "a consumer
  that calls multiple services might be assigned a connection pool for each service. If a service begins to
  fail, it only affects the connection pool assigned for that service." "Problems and considerations": "When
  you partition consumers into bulkheads, consider using processes, thread pools, and semaphores. Projects like
  resilience4j and Polly offer a framework" (semaphore-based admission; resilience4j's SemaphoreBulkhead
  maxConcurrentCalls / maxWaitDuration is the familiar shape); "combining bulkheads with retry, circuit breaker,
  and throttling" (composition is the caller's: out of scope); "Monitor each partition's performance"
  (WithOnEvent).
- Microsoft, "Queue-Based Load Leveling pattern", Azure Architecture Center (ms.date 2026-06-09). Opened:
  https://learn.microsoft.com/azure/architecture/patterns/queue-based-load-leveling, 2026-11-13. Relied on,
  "Problems and considerations": "If your average producer rate exceeds the consumer rate, the queue continues
  to grow and latency increases. Monitor queue depth ... or shed work at the producer" (the queue is bounded;
  beyond it ErrFull sheds; a queued call is bounded in time by WithQueueTimeout).

### c. Today's Go

Go 1.27 has no bulkhead. `golang.org/x/sync/semaphore` and our `semaphore` bound concurrency but either always
wait (Acquire) or never wait (TryAcquire): neither has "wait, but only among n others and only for d", nor
ErrFull, nor panic recovery, nor metrics. A buffered channel as a semaphore (the usual hand-rolled bulkhead) has
no FIFO guarantee and no queue bound. Hence the package: two `semaphore.Weighted` composed, not a new primitive.

### d. calcbench

**None called.** Counting permits is a semaphore (toolbench), not mathematics; the timeout is one clock timer.
No calcbench request.

## Exports

| ID | Name | Signature | file:line |
|---|---|---|---|
| `TB-BULK-001` | `Bulkhead` | `type Bulkhead struct{ … }` with `Do(ctx context.Context, fn func(context.Context) error) error` | `bulkhead/bulkhead.go:34` (Do :87) |
| `TB-BULK-002` | `New` | `func New(maxConcurrent int, opts ...Option) (*Bulkhead, error)` | `bulkhead/bulkhead.go:53` |
| `TB-BULK-003` | `DoValue` | `func DoValue[T any](ctx context.Context, b *Bulkhead, fn func(context.Context) (T, error)) (T, error)` | `bulkhead/bulkhead.go:194` |
| `TB-BULK-004` | `ErrFull` | `var ErrFull = errors.New("bulkhead: full")` | `bulkhead/errors.go:18` |
| `TB-BULK-005` | `ErrQueueTimeout` | `var ErrQueueTimeout = fmt.Errorf("bulkhead: queue timeout: %w", ErrFull)` | `bulkhead/errors.go:28` |
| `TB-BULK-006` | `ErrMaxConcurrent` | `var ErrMaxConcurrent = errors.New("bulkhead: maxConcurrent must be > 0")` | `bulkhead/errors.go:37` |
| `TB-BULK-007` | `Option` | `type Option func(*config)` | `bulkhead/option.go:23` |
| `TB-BULK-008` | `WithQueue` | `func WithQueue(n int) Option` | `bulkhead/option.go:33` |
| `TB-BULK-009` | `WithQueueTimeout` | `func WithQueueTimeout(d time.Duration) Option` | `bulkhead/option.go:46` |
| `TB-BULK-010` | `WithClock` | `func WithClock(c clock.Clock) Option` | `bulkhead/option.go:58` |
| `TB-BULK-011` | `WithOnEvent` | `func WithOnEvent(f func(Event)) Option` | `bulkhead/option.go:72` |
| `TB-BULK-012` | `Event` | `type Event struct{ Kind EventKind; Waited time.Duration }` | `bulkhead/event.go:18` |
| `TB-BULK-013` | `EventKind` | `type EventKind int` (`EventAdmitted`, `EventRejected`, `EventQueued`, `EventTimedOut`, `EventCancelled`, `EventReleased`; `String`) | `bulkhead/event.go:33` |

| Option | Default | Bound it sets |
|---|---|---|
| `WithQueue(n)` | 0 (also n < 0): fail fast | calls waiting for a slot (FIFO) |
| `WithQueueTimeout(d)` | none (also d <= 0): wait until ctx ends | time a queued call waits, on the clock |
| `WithClock(c)` | `clock.Real()` (also nil) | — |
| `WithOnEvent(f)` | nil: no hook | — |

Sentinel errors: `ErrFull` (no slot, no queue place) · `ErrQueueTimeout` (matches ErrFull too) ·
`ErrMaxConcurrent` (New) · `panics.ErrPanic` (fn or the hook panicked).

Deviations from LADDER.md §4 (`New`, `Do`, `ErrFull`; options WithQueue, WithQueueTimeout): BRIEF B1 adds
`DoValue`, `WithClock`, `WithOnEvent`; this spec adds `ErrQueueTimeout`, `ErrMaxConcurrent`, `Event`,
`EventKind`. No released name changes (new package).

## Decisions (BRIEF B1 asks each to be stated)

1. **ErrFull vs a separate queue-timeout error: both.** `ErrQueueTimeout` wraps `ErrFull`, so
   `errors.Is(err, ErrFull)` is true for every rejection (the common "shed this call" check) and
   `errors.Is(err, ErrQueueTimeout)` tells a timed-out wait apart (a different metric and a different tuning
   knob: queue length vs timeout). Alternatives that lost: ErrFull only (the caller cannot tell "full now" from
   "waited 2 s and gave up"); an unrelated ErrQueueTimeout (every caller must check two errors to shed).
   resilience4j's single BulkheadFullException for both is the loser here.
2. **Admission = two semaphores, never re-implemented.** `room` (size maxConcurrent+queue) is taken with
   TryAcquire (fail → ErrFull at once); `slots` (size maxConcurrent) is taken with TryAcquire, else Acquire with
   a ctx bounded by the queue timeout. FIFO among queued calls is the semaphore's FIFO (TB-SEMA-001). A slot is
   released before its room place, so every holder of a room place can count on a slot (with queue 0, a call
   holding room always finds a free slot: fail fast is exact). `room` int64 sum is capped so
   maxConcurrent+queue never overflows.
3. **Event hook shape and where it runs.** `WithOnEvent(func(Event))`, `Event{Kind, Waited}`, kinds Admitted,
   Rejected, Queued, TimedOut, Cancelled, Released. Every Do produces exactly one sequence: Rejected ·
   Cancelled · Admitted→Released · Queued→Admitted→Released · Queued→TimedOut · Queued→Cancelled (TimedOut or
   Cancelled without Queued if the wait ended before the call reached the queue). The hook runs on the Do
   caller's goroutine, holding no lock (semaphore locks are never held while it runs), possibly concurrently
   from many callers; it must be concurrency-safe and should not block. A hook panic is recovered and joined
   (errors.Join) to that Do's result; the call proceeds (breaker D, decision 7). Alternatives that lost: a
   `func(kind, waited)` pair (not extensible additively); per-kind hooks (six options); calling the hook under a
   lock (a hook that calls Do deadlocks).
4. **When Queued fires.** It fires when the call is in the slot semaphore's FIFO queue, not before: the
   waiting call hands Acquire a `queuedCtx` whose first `Done()` call emits it, and Acquire reads Done only in
   its final select, after linking the waiter and releasing its lock (semaphore/weighted.go Acquire), never on
   its fast path. So an observer that waits for Queued and then starts another call is guaranteed FIFO order
   (deterministic tests). Lead decision D-036: keep this Done-wrapper detection, no new semaphore export this
   wave. The coupling to semaphore's internals is pinned by `TestDo_queuedEventMeansEnqueued` (after Queued,
   `slots.TryAcquire(0)` is false: someone waits) in ONE direction only: it reliably catches "Done is never
   called", but catches "Done is called before the push" only by chance (the hook runs on the waiter's
   goroutine, racing the test's TryAcquire(0)). Recorded on EventQueued and queuedCtx too. Alternative that
   lost: emit before Acquire (an observer's next call may reach the queue first; FIFO tests flake).
5. **FIFO of the queue**: first come, first served, from semaphore. A cancelled or timed-out waiter leaves
   the queue in O(1) and the next one moves up (semaphore's remove/grant).
6. **Queue timeout**: one `clock.AfterFunc(d, cancel(ErrQueueTimeout))` per queued call, armed before the
   call joins the queue (so `Fake.BlockUntil(1)` after Queued always sees it) and stopped when the wait ends.
   It bounds only the wait: fn always runs with the caller's ctx. A timeout and a caller cancel racing: the
   caller's cause wins (ctx checked first).
7. **ctx**: nil → Background. Done before Do: `context.Cause(ctx)`, fn not called, Cancelled event, nothing
   taken (even with a free slot; semaphore Acquire's rule). Done while queued: the call leaves the queue,
   releases only its room place (the semaphore guarantees a cancelled waiter holds no slot, including a grant
   that raced with the cancel), returns `context.Cause(ctx)`.
8. **Panic in fn → ErrPanic, slot released**: fn runs under `panics.Try`; the releases are deferred, so a
   panic, a nil fn and runtime.Goexit (t.FailNow) all give the slot and room place back.
9. **maxConcurrent <= 0**: `New` returns nil and `ErrMaxConcurrent` (D-023: a long-lived constructor errors).
   Zero value and nil *Bulkhead: Do returns ErrFull (no panic on input, D-011; a bulkhead with no slots).
10. **No goroutine** of ours. With the real clock, a timeout fires on the runtime's AfterFunc goroutine, which
    only cancels a ctx and exits; Stop prevents it when the wait ends first.

## Per export

### `TB-BULK-001` `Bulkhead` (Do)
- **Pattern**: bulkhead. **Authority**: Nygard ch.5 (not opened); Azure "Bulkhead pattern".
- **Familiar shape**: `breaker.Breaker.Do`/`errgroup.Go`-style `Do(ctx, fn)`; resilience4j SemaphoreBulkhead
  (maxConcurrentCalls, maxWaitDuration) for the knobs.
- **Goroutines**: none (decision 10). **Cancellation**: decision 7; during fn, fn sees ctx.
- **Errors**: ErrFull, ErrQueueTimeout, ctx cause, fn's error unchanged, *panics.PanicError; hook panic joined.
- **Blocking**: only while queued (ctx, timeout) and while fn runs. No silent drop: a rejection is an error.
- **Ordering and counts**: at most maxConcurrent fn running, at most queue waiting; queued calls admitted FIFO.
- **Zero value**: rejects every call with ErrFull; create with New. Not copied after first use.
- **Clock**: WithClock. **calcbench**: none. **Compared against**: none (LADDER names none; resilience4j and
  Polly are not Go). **Worked example**: `ExampleBulkhead` (`bulkhead: full true`, `<nil>`), `ExampleBulkhead_Do`.

### `TB-BULK-002` `New` — `ExampleNew`. ### `TB-BULK-003` `DoValue` — T's zero on rejection or panic; `ExampleDoValue` (`9.99 <nil>`).
### `TB-BULK-004..006` errors — `ExampleErrFull`, `ExampleErrQueueTimeout` (fake clock), `ExampleErrMaxConcurrent`.
### `TB-BULK-007..011` options — `ExampleOption`, `ExampleWithQueue` (third waiter rejected; FIFO `[0 1]`),
`ExampleWithQueueTimeout` (`… 500ms`), `ExampleWithClock`, `ExampleWithOnEvent` (admitted, rejected, released).
### `TB-BULK-012..013` `Event`, `EventKind` — `ExampleEvent`, `ExampleEventKind`.

## Tests (`bulkhead/*_test.go`; `leak.Main` in main_test.go, `leak.Verify` in every goroutine test; clock.Fake only; no Sleep, no polling)

`TestDo_maxConcurrentAsserted` (3 slots + 10 queued, peak in-flight by atomics = 3, 14th → ErrFull) ·
`TestDo_failFastWithoutQueue` · `TestDo_queueFull` · `TestDo_queueAdmitsInOrder` (5 queued, run 0..4) ·
`TestDo_queueTimeoutOnFakeClock` (d−1ns still waiting, d → ErrQueueTimeout+ErrFull, Waited = d, place freed) ·
`TestDo_queuedThenAdmittedStopsTimer` (Waited = 2 s; stopped timer fires nothing) ·
`TestDo_cancelWhileQueuedHoldsNothing` (cause returned; slot still the holder's: next queues, then ErrFull) ·
`TestDo_ctxDoneBefore` · `TestDo_panicReleasesSlot` (also nil fn) · `TestDo_goexitReleasesSlot` ·
`TestDo_fnErrorUnchangedAndCallerCtx` · `TestDo_nilCtx` · `TestDo_nilAndZeroBulkhead` ·
`TestDo_queuedEventMeansEnqueued` (decision 4) · `TestWithOnEvent_seesEverySequence` ·
`TestWithOnEvent_hookPanicJoined` · `TestDoValue` · `TestNew_maxConcurrentInvalid` · `TestNew_optionDefaults`
(incl. the int64 cap) · `TestEventKind_String` · `BenchmarkDo_uncontended` · an Example per export. Every
wait is on a channel or the fake clock; written to be deterministic under `-count=200`.

## Out of scope

Thread-pool bulkhead (resilience4j ThreadPoolBulkhead: that is `pool`) · per-key bulkheads (a map of Bulkhead
by the caller, or a later additive `Keyed`) · priority in the queue · dynamic resizing · retry/breaker
composition (the caller wraps `retry`/`breaker` around `Do`) · HTTP 503 / gRPC RESOURCE_EXHAUSTED mapping
(`httpx`, `grpcx`) · metrics export (the hook is the seam).

## For the lead

- INDEX.md: 13 rows TB-BULK-001..013 (doc.go list). taxonomy: `bulkhead` planned → built; uses `semaphore,
  clock, panics`; no calcbench.
- compare/: none (LADDER rank 28 names no library; resilience4j/Polly are not Go). No compare/bulkhead written.
- DECISIONS candidates: ErrQueueTimeout wraps ErrFull (1); two-semaphore admission (2); hook shape, placement,
  panic joined (3); Queued emitted via the queuedCtx Done seam, coupled to semaphore Acquire's internals and
  pinned by a test (4) — the lead may prefer an unexported/test-only seam in `semaphore` instead; zero value
  and nil reject with ErrFull (9). D-036 taken by the lead for decision 4.
- TB-BULK-006 ErrMaxConcurrent cites pkg.go.dev errors, "func Is" (D-023 stays in its prose).
- doc/sources: Azure "Bulkhead pattern" and "Queue-Based Load Leveling pattern" (opened 2026-11-13) used by
  TB-BULK-*; Nygard ch.5 cited, not opened.
- Checks: `gofmt -l bulkhead`, `go build ./...`, `go vet ./bulkhead/` all clean (a first `go build ./...`
  failed in B0's in-progress `ratelimit/reservation.go:70`; clean on re-run).

LEFT FOR DEVIN: the cleaner fix for decision 4 is an additive semaphore export, e.g.
`func (s *Weighted) AcquireNotify(ctx context.Context, n int64, onQueued func()) error` (a new TB-SEMA ID,
onQueued called after the waiter is linked, outside the lock); bulkhead would then drop queuedCtx.

REMAINING: none.
