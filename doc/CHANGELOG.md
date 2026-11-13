# Changelog

## 2026-11-13
- Project created: .claude rules, docs skeleton. No code yet.
- Foundation (doc/foundation_2026-11-13/): go.mod (calcbench v0.1.0), go.sum, root doc.go, .gitignore entries,
  internal/calccheck (NUM-STATS-001 link test), internal/leak (goroutine-leak helper), INDEX.md (header only,
  generated), testing/index_test.go, doc/foundation_2026-11-13/gen_index.go, doc/specs/TEMPLATE.md,
  doc/sources/code-heim.md, doc/sources/calcbench.md. No IDs.
- Pattern ladder (doc/ladder_2026-11-13/): 81 code-heim repos read, 51 ladder rows, package map, API list, waves,
  3 calcbench requests, 14 decisions. Proposal only; doc/taxonomy.md has no rows yet.
- Waves 1 and 2 (doc/wave1_2026-11-13/, doc/wave2_2026-11-13/), specs in doc/specs/:
  - `panics` TB-PANIC-001..004 (ErrPanic, PanicError, Try, TryValue): panics/panic_error.go, panics/try.go.
  - `clock` TB-CLOCK-001..005 (Clock, Timer, Ticker, Real, Fake incl. AdvanceContext): clock/clock.go, real.go, fake.go.
  - `ctxkey` TB-CTXKEY-001..002 (Key, New): ctxkey/key.go.
  - `group` TB-GROUP-001..007 (Group, WithContext, Results, NewResults, Option, WithLimit, ErrZeroLimit): group/group.go, results.go.
  - `flow` TB-FLOW-001..013, 015 (FromSeq, ToSeq, Stage, FanOut, Merge, OrDone, Tee, Bridge, Priority, Weighted, Option,
    WithBuffer, WithRand, ErrInvalid); TB-FLOW-014 WithPanicHandler REMOVED before release (D-018).
  - `semaphore` TB-SEMA-001..004 (Weighted, ErrTooLarge, ErrNegativeWeight, New): semaphore/weighted.go, errors.go.
  - `guard` TB-GUARD-001..002 (Guarded, Confine): guard/guarded.go, confine.go.
  - Decisions D-006..D-020; calcbench requests R-1..R-3 (doc/wave1_2026-11-13/calcbench_requests.md); compare/ module
    and tests written (dependencies not resolved: `go mod tidy` not permitted in the session).
- compare/ resolved (Devin authorised `go -C compare mod tidy` once): golang.org/x/sync v0.25.0,
  github.com/sourcegraph/conc v0.3.0, github.com/benbjohnson/clock v1.3.5; main go.mod/go.sum unchanged (sha256).
  Two comparison-script bugs fixed (compare/clock/clock_test.go:89 want string; compare/semaphore/semaphore_test.go:55
  script step 1); no toolbench code changed. All five compare packages pass under -race.
- Waves 3 and 4 (doc/wave3_2026-11-13/, doc/wave4_2026-11-13/), specs in doc/specs/:
  - `recycle` TB-RECYC-001..005 (Pool, New, Option, WithReset, WithKeep): recycle/pool.go, option.go.
  - `pool` TB-POOL-001..007 (Pool, New, Option, WithQueue, WithFailFast, ErrClosed, ErrWorkers): pool/pool.go, options.go, errors.go.
  - `parallel` TB-PAR-001..003 (Map, Filter, ForEach): parallel/map.go, filter.go, for_each.go.
  - `batch` TB-BATCH-001..005 (Run, Channel, Option, WithClock, ErrSize): batch/run.go, channel.go, option.go, errors.go.
  - `schedule` TB-SCHED-001..011 (Schedule, Every, At, Run, Heartbeat, Option, WithClock, WithJitter, WithOnError,
    WithRand, ErrSchedule); ParseCron specified, not built (PENDING calcbench R-3).
  - `lifecycle` TB-LIFE-001..011 (Stopper, Run, Component, Group, Option, WithGrace, WithClock, WithNotify, ErrClosed,
    ErrStopTimeout, DefaultGrace).
  - `pubsub` TB-PUBSUB-001..010 (Publisher, Subscriber, Bus, NewBus, Option, WithBuffer, WithDropOldest,
    WithOnSubscribers, ErrClosed, ErrInvalid).
  - Decisions D-021..D-027; compare/ adds bytebufferpool v1.0.0 and oklog/run v1.2.0 (main go.mod/go.sum unchanged).
- Wave 5 (doc/wave5_2026-11-13/):
  - calcbench bumped to v0.2.0 (D-033): go.mod `require github.com/CrimsonCoder42/calcbench v0.2.0`, go.sum updated,
    compare tidied; build, vet, go test -race green before wave 5.
  - `schedule` TB-SCHED-012 ParseCron (ALG-CRON-001/002): schedule/cron.go.
  - `retry` TB-RETRY-001..018 (Do, DoValue, Permanent, ErrExhausted, ErrAttemptTimeout, Backoff, Exponential, FullJitter,
    EqualJitter, DecorrelatedJitter, Option, WithAttempts, WithBackoff, WithAttemptTimeout, WithRetryIf, WithOnRetry,
    WithClock, WithRand): retry/.
  - `singleflight` TB-FLIGHT-001..003 (Group, Result, ErrGoexit): singleflight/.
  - `ratelimit` TB-RATE-001..016 (Limit, Every, Limiter, Reservation, Keyed, Option, WithClock, WithMaxKeys,
    WithIdleTTL, ErrLimit, ErrBurst, ErrKeys, DefaultMaxKeys, InfDuration, NewLimiter, NewKeyed): ratelimit/.
  - `breaker` TB-BRK-001..011 (Breaker, New, DoValue, State, ErrOpen, Option, WithFailureThreshold, WithOpenFor,
    WithHalfOpenProbes, WithClock, WithOnStateChange): breaker/.
  - Decisions D-028..D-033; calcbench requests R-4, R-5 (doc/wave5_2026-11-13/calcbench_requests.md); compare/ adds
    golang.org/x/time v0.16.0, cenkalti/backoff/v4 v4.3.0, sony/gobreaker/v2 v2.4.0.
- Wave 6 (doc/wave6_2026-11-13/):
  - calcbench bumped to v0.3.0 (D-034): go.mod `require github.com/CrimsonCoder42/calcbench v0.3.0`, go.sum updated,
    compare tidied; build, vet, go test -race green before wave 6.
  - `ratelimit` onto calcbench (D-035): Reservation.Cancel follows x/time/rate CancelAt via ALG-RATELM-005 Restore;
    Keyed uses ALG-LIST-015 LRUCache + ALG-LIST-018 Oldest; signatures unchanged; 7 pre-edit files saved in _delete/.
  - `bulkhead` TB-BULK-001..013: bulkhead/.
  - `cache` TB-CACHE-001..016: cache/ (review blocker fixed: a write during an in-flight load is no longer overwritten).
  - `config` TB-CONFIG-001..021: config/.
  - `logging` TB-LOG-001..010: logging/.
  - Decisions D-034..D-039; one default secret-word list shared by config and logging (ASSUMPTIONS); compare/ adds
    hashicorp/golang-lru/v2 v2.0.7.
- Logging house format (doc/logging_house_2026-11-13/; D-040): `logging` gains Text, TextOption, WithLevel, WithColor,
  WithSource, WithTimeFormat, WithClock, WithJSON, Trace, ParseLevel, New (TB-LOG-011..021: logging/text.go, trace.go,
  level.go, new.go); the project-wide standard doc/standards/logging.md (indexed in doc/README.md). No released
  signature changed.
- Align run (doc/align_2026-11-13/): both reading lists cross-referenced against the code as built (MATRIX.md: 93
  rows, REPEAT measured per distinct repo; 29 repeated patterns delivered SAME/NEAR, 8 PARTIAL, 3 MISSING); the general
  gaps closed, all additive (0 changed or removed signatures, api_before/after.txt):
  - `guard` TB-GUARD-003..008 (Keyed, NewKeyed, KeyedOption, WithMaxKeys, DefaultMaxKeys, ErrKeys): guard/keyed.go, keyed_option.go, errors.go; Examples for Guarded over a map and a slice.
  - `lifecycle` TB-LIFE-012..014 (Group.Live, Group.Ready, ErrNotReady) and Component fields Run, Live, Ready: lifecycle/runner.go, health.go, component.go; tests for the fresh stop deadline, Stop-once, stop order, start error before any signal.
  - `pool` TB-POOL-008..011 (ErrQueueFull, Pool.TrySubmit, WithOnDone, WithMaxErrors) and the in-worker non-blocking Submit: pool/pool.go, options.go, errors.go.
  - `retry` TB-RETRY-019 (WithMaxElapsed) and the no-retry-forever policy; ExampleDo_startupDependency, ExampleWithRetryIf_breakerOpen: retry/option.go, retry.go, breaker_example_test.go.
  - `pubsub` TB-PUBSUB-011..017 (Message, HeaderRequestID, HeaderCorrelationID, ErrSlowSubscriber, WithDropSubscriber, WithOnDelivered, WithClock): pubsub/message.go, options.go, errors.go, subscription.go.
  - `ctxkey` TB-CTXKEY-003..010 (RequestID, MaxRequestIDLen, ErrRequestID, NewRequestID, ValidRequestID, WithRequestID, RequestIDFrom, EnsureRequestID): ctxkey/request_id.go.
  - `group` TB-GROUP-008..009 (Group.SetCollect, WithCollect): group/collect.go; compare/group TestCompare_collectVersusErrgroup.
  - Decisions D-041..D-052; assumptions appended; INDEX.md regenerated (211 -> 242 rows); doc/taxonomy.md rows updated
    (pubsub now depends on clock); doc/sources/AkhilSharma90.md written; LADDER.md note; calcbench requests R-6..R-9;
    DEFERRED.md. First recorded benchmark numbers in doc/specs (### Measured) and MATRIX.md §8. Two go1.27.3 internal
    compiler errors worked around in pool (ASSUMPTIONS).
