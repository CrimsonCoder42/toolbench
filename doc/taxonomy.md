# Package map

Proposed by the pattern ladder (`doc/ladder_2026-11-13/LADDER.md`, section 3) and approved by Devin before any
package is created. Until a row is here, the package does not exist. One package per row; Package is the folder
path (`flow`, `adapters/redis`); Status is built (the folder holds code) or planned (name and SHORT reserved).
`testing/index_test.go` checks this table against the tree (the shape of calcbench D-072).

| Package | SHORT | Layer (core / httpx / grpcx / adapter) | Tier | Patterns | Depends on | Status (planned / built) |
|---|---|---|---|---|---|---|
| `batch` | BATCH | core | T2 | batching by size or time (aggregator) | clock, panics, calcbench algo/seq | built |
| `breaker` | BRK | core | T3 | circuit breaker (closed, open, half-open) | clock, panics | built |
| `bulkhead` | BULK | core | T3 | bulkhead: bounded concurrency, fail fast, bounded wait queue | semaphore, clock, panics | built |
| `cache` | CACHE | core | T4 | in-memory TTL+LRU cache, GetOrLoad, bloom guard, Cache interface (cache-aside) | singleflight, clock, panics, calcbench algo/list, algo/bloom | built |
| `clock` | CLOCK | core | T0 | clock (injectable time source) | stdlib | built |
| `config` | CONFIG | core | T4 | layered config (defaults < file < env < flags), typed getters, redaction | stdlib | built |
| `ctxkey` | CTXKEY | core | T0 | typed context key; the standard request-id key and helpers | stdlib | built |
| `flow` | FLOW | core | T1 | generator, stage, fan-out, merge, or-done, tee, bridge, priority and weighted merge | panics, calcbench algo/sample | built |
| `group` | GROUP | core | T1 | error group, result group, collect-all errors opt-in | panics | built |
| `guard` | GUARD | core | T1 | mutual exclusion (Guarded), keyed mutual exclusion (Keyed), confinement | panics | built |
| `lifecycle` | LIFE | core | T2 | run group: ordered start, run-time failure ends the group, reverse stop, drain deadline, signal hook, liveness and readiness seam | clock, panics | built |
| `logging` | LOG | core | T4 | slog handlers: redact, context attributes, multi, recorder | ctxkey, panics | built |
| `panics` | PANIC | core | T0 | panic-to-error | stdlib | built |
| `parallel` | PAR | core | T2 | bounded parallel Map, Filter, ForEach (twins of calcbench algo/seq) | flow, panics | built |
| `pool` | POOL | core | T2 | worker pool (competing consumers), in-worker non-blocking submit, progress hook, bounded error collection | group, panics | built |
| `pubsub` | PUBSUB | core | T2 | in-process publish-subscribe bus; Publisher and Subscriber interfaces; Message envelope with headers; slow-subscriber policies (block, drop-oldest, drop-subscriber); delivery hook | clock, panics, calcbench algo/queue | built |
| `ratelimit` | RATE | core | T3 | token-bucket limiter (x/time/rate shape), keyed limiter | clock, calcbench algo/ratelimit, algo/list, num/integer | built |
| `recycle` | RECYC | core | T1 | typed object pool over sync.Pool | stdlib | built |
| `retry` | RETRY | core | T3 | retry with backoff and jitter (Do, DoValue, Permanent), per-attempt timeout, total-elapsed cap | clock, panics, calcbench algo/backoff | built |
| `schedule` | SCHED | core | T2 | Every, At, Run, Heartbeat, ParseCron | clock, panics, calcbench num/prob, algo/cron | built |
| `semaphore` | SEMA | core | T1 | weighted semaphore | stdlib | built |
| `singleflight` | FLIGHT | core | T3 | duplicate call suppression | panics | built |
