# MATRIX: both reading lists against toolbench (align, 2026-11-13)

Merged by the lead from M1-M4 (93 rows). REPEAT is the number of DISTINCT repos per list (code-heim 81 /
AkhilSharma90 92 / total 173) measured by the readers' own greps (fingerprints recorded at the top of each M file),
then confirmed by reading. Verdicts are against toolbench's own bar (CLAUDE.md, go-concurrency rules). No benchmark
number existed under doc/ before this run (every reader checked); scale notes come from the code, and VERIFY records
the first numbers (section 6).

Verdict counts over 93 rows: SAME 47 · NEAR 7 · PARTIAL 8 · MISSING 3 · PLANNED 27 · NOT-GENERAL 1.
(M1 14/1/2/0/0/0 · M2 11/2/3/2/1/0 · M3 10/0/2/0/1/0 · M4 12/3/1/1/25/1; M2's cron row is PARTIAL by calcbench, not
by toolbench code.)

## 1. Every pattern once, by total REPEAT

| # | pattern | toolbench | REPEAT ch/ak/total | VERDICT | reader |
|---|---|---|---|---|---|
| 1 | environment config layer | TB-CONFIG-001/006/020 | 6/37/43 | SAME | M4 |
| 2 | middleware chain (decorator) | planned httpx.Chain | 13/15/28 | PLANNED | M4 |
| 3 | serve: `log.Fatal(ListenAndServe)` with no shutdown or timeouts | planned httpx.Serve | 13/11/24 (timeouts set 0/4/4) | PLANNED | M4 |
| 4 | required config, fail fast at startup | TB-CONFIG-012/017 | 3/19/22 | SAME | M4 |
| 5 | access log | planned httpx.AccessLog | 16/5/21 | PLANNED | M4 |
| 6 | signal hook (termination signal to context) | TB-LIFE-008 | 6/14/20 | SAME | M2 |
| 7 | `.env` KEY=VALUE file layer | none | 8/12/20 | MISSING (decision, no authority) | M4 |
| 8 | flags config layer | TB-CONFIG-007/021 | 2/16/18 | SAME | M4 |
| 9 | mutual exclusion, mutex beside the value | TB-GUARD-001 | 4/11/15 | PARTIAL: keyed mutex (sub-REPEAT 3) | M1 |
| 10 | job queue (brokers; dead letter / max retry 1/6/7) | planned jobs | 6/9/15 | PLANNED | M4 |
| 11 | offset pagination (float-ceil page count 8/2/10) | planned page | 8/5/13 | PLANNED | M4 |
| 12 | sleep-loop scheduler or poll (`for { time.Sleep }`) | TB-SCHED-002/004 replace it | 3/10/13 | SAME (the defect ours removes) | M2 |
| 13 | repository (ErrNotFound sentinels 7/6/13) | planned store | 0/10/10 | PLANNED | M4 |
| 14 | DoJSON (client decode, status check, bounded body) | planned httpx.DoJSON | 1/9/10 | PLANNED | M4 |
| 15 | struct binding by tags (envconfig, viper.Unmarshal) | none | 1/8/9 | NOT-GENERAL (reflection) | M4 |
| 16 | fan-out (bounded workers) | TB-FLOW-004 | 3/5/8 | SAME | M1 |
| 17 | bounded parallel map / filter / for-each | TB-PAR-001..003 | 3/5/8 | SAME | M2 |
| 18 | retry attempt loop | TB-RETRY-001/002/004/012 | 1/7/8 | SAME | M3 |
| 19 | config file layer (YAML in every reference, JSON in ours) | TB-CONFIG-004/005 | 1/7/8 | PARTIAL: adapters/yaml (dependency decision) | M4 |
| 20 | error group (fork-join, first error cancels) | TB-GROUP-001..007 | 2/5/7 | PARTIAL: collect-all errors (sub-REPEAT 3) | M1 |
| 21 | run group (ordered start, first failure ends all, Stop once) | TB-LIFE-003/004 | 3/4/7 | PARTIAL: run-time failure seam (sub-REPEAT 4) | M2 |
| 22 | fixed-rate scheduler (Every + Run, ticker loops) | TB-SCHED-001/002/004 | 2/5/7 | SAME | M2 |
| 23 | health endpoint (liveness, readiness) | planned httpx.Health + a lifecycle seam | 0/7/7 (3 real) | PLANNED | M2, M4 |
| 24 | recover middleware | planned httpx.Recover | 1/6/7 | PLANNED | M4 |
| 25 | graceful stop with a drain deadline | TB-LIFE-001/002/006/010/011 | 2/4/6 | SAME | M2 |
| 26 | in-process publish-subscribe bus | TB-PUBSUB-001..006/009/010 | 0/6/6 | SAME (policy and envelope: rows 36, 37) | M2 |
| 27 | remote ip behind trusted proxies | planned httpx.RemoteIP | 2/4/6 | PLANNED | M4 |
| 28 | worker pool (Submit / Close) | TB-POOL-001/002/006/007 | 3/2/5 | SAME | M2 |
| 29 | generator (seq to channel) | TB-FLOW-001/002 | 3/2/5 | SAME | M1 |
| 30 | weighted semaphore (token channel) | TB-SEMA-001..004 | 0/5/5 | SAME | M1 |
| 31 | bulkhead (bounded concurrency, fail fast) | TB-BULK-001..004/006 | 0/5/5 | SAME | M3 |
| 32 | transaction closure | planned txn | 3/2/5 | PLANNED | M4 |
| 33 | rate-limit middleware (429) | planned httpx.RateLimit | 2/3/5 | PLANNED | M4 |
| 34 | gRPC status mapping | planned grpcx | 1/4/5 | PLANNED | M4 |
| 35 | fan-in / merge | TB-FLOW-005 | 1/3/4 | SAME | M1 |
| 36 | pubsub slow-subscriber policy (evict) | TB-PUBSUB-007 (drop-oldest only) | 0/2/2 | PARTIAL: enters by the scale clause (sequential fan-out behind a full chan, bus.go:124-130) | M2 |
| 37 | message envelope (headers, correlation id) | none | 0/2/2 | MISSING: enters by authority (EIP "Message", "Message Header", "Correlation Identifier") | M2 |
| 38 | typed context key | TB-CTXKEY-001/002 | 1/3/4 | SAME | M1 |
| 39 | token bucket (Allow / Reserve / Wait) | TB-RATE-001..004/010/011/014/015 | 2/2/4 | SAME | M3 |
| 40 | keyed (per-client) limiter, bounded table | TB-RATE-005/008/009/012/013/016 | 2/2/4 | SAME | M3 |
| 41 | 429 shape (Retry-After, body, key) | planned httpx.RateLimit | 2/2/4 | PLANNED | M3 |
| 42 | cache-aside / read-through (GetOrLoad, singleflight) | TB-CACHE-001/011/015 | 2/2/4 | SAME | M4 |
| 43 | request id / correlation id (HTTP) | planned httpx.RequestID + ctxkey | 0/4/4 | PLANNED (D6 builds the key) | M4 |
| 44 | retry transport (client) | planned httpx.RetryTransport | 1/3/4 | PLANNED | M4 |
| 45 | panic-to-error | TB-PANIC-001..004 | 1/2/3 | SAME | M1 |
| 46 | pipeline stage | TB-FLOW-003 | 2/1/3 | SAME | M1 |
| 47 | confinement (owner goroutine) | TB-GUARD-002 | 1/2/3 | SAME | M1 |
| 48 | pool bounded queue; pool fail-fast | TB-POOL-004; TB-POOL-005 | 1/2/3 each | SAME | M2 |
| 49 | cron schedule | TB-SCHED-012 | 2/1/3 | PARTIAL by calcbench: Vixie steps `*/N` (go_49's real spec) rejected | M2 |
| 50 | exponential backoff (capped) | TB-RETRY-006/007/013 | 0/3/3 | SAME | M3 |
| 51 | startup retry (wait for a dependency) | TB-RETRY-001/012 + ctx | 0/3/3 | PARTIAL: docs + the no-retry-forever decision | M3 |
| 52 | in-memory TTL cache | TB-CACHE-002/006/007/010 | 2/1/3 | SAME | M4 |
| 53 | house logger / handler | TB-LOG-011..021 | 3/5/8 (slog 3, custom 3, zap/zerolog 5) | SAME | M4 |
| 54 | blob store, presign, atomic put | planned blob | 1/2/3 | PLANNED | M4 |
| 55 | gRPC streams | planned grpcx | 0/3/3 | PLANNED | M4 |
| 56 | retry: permanent errors and predicate | TB-RETRY-003/015 | 1/1/2 | SAME | M3 |
| 57 | retry: per-attempt timeout | TB-RETRY-005/014 | 1/1/2 | SAME | M3 |
| 58 | heartbeat | TB-SCHED-005 | 1/1/2 | NEAR: "beat while fn runs" is a composition; one Example | M2 |
| 59 | object pool (typed sync.Pool) | TB-RECYC-001..005 | 1/1/2 | SAME | M1 |
| 60 | batch by size (Run); by size or time (Channel) | TB-BATCH-001/005; -002/003/004 | 2/0/2; 1/1/2 | SAME | M2 |
| 61 | one-shot schedule (At) | TB-SCHED-003 | 2/0/2 | SAME | M2 |
| 62 | bounded LRU | TB-CACHE-005/016 | 0/2/2 | NEAR: no Purge/Keys (nobody calls them) | M4 |
| 63 | multi / fan-out log handler | TB-LOG-009 | 1/1/2 | SAME | M4 |
| 64 | request body limit (413) | planned httpx.MaxBytes | 1/1/2 | PLANNED | M4 |
| 65 | gRPC interceptors; server lifecycle (keepalive, GracefulStop) | planned grpcx | 0/2/2 each | PLANNED | M4 |
| 66 | retry total-elapsed cap | none | 0/1/1 | PARTIAL (narrow; D4 builds it) | M3 |
| 67 | retry jitter; schedule jitter | TB-RETRY-008..010/018; TB-SCHED-008/010 | 0/1/1 each | SAME | M3, M2 |
| 68 | pool progress hook | none | 0/1/1 | MISSING (D3 builds it) | M2 |
| 69 | pubsub delivery hook | TB-PUBSUB-008 (subscriber count only) | 0/1/1 | NEAR (D5 builds OnDelivered) | M2 |
| 70 | log redaction | TB-LOG-001..005 | 1/0/1 | NEAR: no shipped default patterns | M4 |
| 71 | typed config getters | TB-CONFIG-008..011/018 | 0/1/1 | NEAR: no slice getter | M4 |
| 72 | or-done; tee | TB-FLOW-006; -007 | 1/0/1 each | SAME | M1 |
| 73 | bloom guard | TB-CACHE-009/014 | 1/0/1 | SAME | M4 |
| 74 | lock with lease; idempotency key; mail sender; problem details | planned | 1/0/1; 0/1/1; 1/0/1; 0/1/1 | PLANNED | M4 |
| 75 | clock | TB-CLOCK-001..005 | 0/0/0 | NEAR: no Sleep/After conveniences (synctest answers) | M1 |
| 76 | bridge; priority merge; weighted merge | TB-FLOW-008; -009; -010 | 0/0/0 | SAME to the authority | M1 |
| 77 | singleflight; circuit breaker; bulkhead queue and events | TB-FLIGHT-*; TB-BRK-*; TB-BULK-005/008..013 | 0/0/0 | SAME to the library / authority | M3 |
| 78 | eviction hook; recorder; context attrs handler; config redaction | TB-CACHE-008/010; TB-LOG-010; TB-LOG-006..008; TB-CONFIG-013..016 | 0/0/0 | SAME (no reference has them) | M4 |
| 79 | keyset pagination; outbox | planned page.Cursor; outbox | 0/0/0 | PLANNED (authority-only) | M4 |

## 2. (a) REPEATED AND DELIVERED (SAME or NEAR, REPEAT >= 3): the proof the base is right

env layer 43 · required 22 · signal hook 20 · flags 18 · mutex-beside-value 15 (the base; keyed is the gap) ·
sleep-loop scheduler 13 · fan-out 8 · parallel map 8 · retry loop 8 · logger 8 · fixed-rate scheduler 7 · graceful
stop 6 · pubsub bus 6 · worker pool 5 · generator 5 · semaphore 5 · bulkhead 5 · merge 4 · ctxkey 4 · token bucket
4 · keyed limiter 4 · cache-aside 4 · panics 3 · stage 3 · confinement 3 · pool queue 3 · pool fail-fast 3 · backoff 3
· TTL cache 3. In each, ours delivers everything the best reference does and removes the defect the broken ones
share (the readers name both per row). Twenty-nine patterns, every one with named tests for the reference defects.

## 3. (b) REPEATED AND PARTIAL: the build list (ranked by REPEAT, then scale impact)

Rule: enters if REPEAT >= 3 across the lists, OR an authority names it as standard, OR it fixes a scale limit in what
we ship. "D" marks Devin's decided items (built regardless).

| rank | item | package | why it enters | builds |
|---|---|---|---|---|
| 1 | keyed mutex (lock striping per key, idle entries removed, WithMaxKeys) | guard | REPEAT 15 base / 3 keyed; scale: one lock serialises all keys (go-ratelimit) | D1 |
| 2 | run group: a component failing AFTER Start ends the group (run-time failure seam) | lifecycle | REPEAT 7 / 4; oklog/run and errgroup name it | matrix (with D2) |
| 3 | lifecycle: fresh deadline ctx on Stop (test), Stop exactly once (test), stop-order test, readiness/liveness seam per component | lifecycle | D2; readiness flips false the instant shutdown begins (Azure Health Endpoint Monitoring) | D2 |
| 4 | error group collects every error (errors.Join) as an opt-in; Results keeps per-slot results | group | REPEAT 7 / 3; pkg.go.dev errors.Join | matrix |
| 5 | pool: Close returns only after every worker exited (test), Submit from inside a worker cannot deadlock (documented + test), WithOnDone progress hook; bounded error collection (scale: collect-mode `errs` has no cap, pool.go:199-201) | pool | D3; scale clause | D3 |
| 6 | pubsub: Message envelope with Headers; slow-subscriber policy block / drop-oldest / drop-subscriber, each reported; OnDelivered hook | pubsub | D5; envelope by authority (EIP); drop-subscriber by the scale clause | D5 |
| 7 | retry: WithMaxElapsed; policy "every retry is bounded by attempts, elapsed time or ctx; no retry-forever, including at startup"; startup Example; retry x breaker Example (the default predicate retries ErrOpen) | retry | D4; REPEAT 3 startup | D4 |
| 8 | ctxkey: standard request-id key, NewRequestID (crypto/rand), helpers | ctxkey | D6; REPEAT 4 sources; EIP Correlation Identifier | D6 |
| 9 | calcbench: ALG-CRON-001 Vixie step values `*/N`, `a-b/N` | calcbench request R-9 | REPEAT 3 (go_49's real spec); not toolbench code | lead (calcbench_requests.md) |
| 10 | logging.Context derives a handler per record carrying ctx attrs (context.go:74-82) | logging | scale clause; needs a number first: VERIFY runs BenchmarkContext and records it; the fix (cache the derived handler or Record.AddAttrs when no group) changes attr order and goes to the hardening run with the number | deferred (DEFERRED.md) |
| 11 | group `freed` close-broadcast wakes every blocked Go per release (group.go:165-170) | group | scale, no reference; VERIFY benchmarks 1k blocked Go on limit 1 and records it; fix only if the number shows the herd | deferred unless measured |
| 12 | flow Priority/Weighted goroutine churn per idle turn (choose_merge.go:86-106) | flow | scale, no reference, REPEAT 0; benchmark at 1/10/100 inputs in VERIFY, record | deferred unless measured |

## 4. (c) MISSING general patterns

| item | REPEAT | rule | outcome |
|---|---|---|---|
| message envelope (Headers, Body) | 2 | authority (EIP) | built, D5 |
| keyed mutex | 3 | REPEAT | built, D1 (also LADDER2 §2.1) |
| `.env` KEY=VALUE config layer | 20 | REPEAT, but no authority names `.env` (Twelve-Factor says the environment) | DECISION for Devin (section 7); not built this run; the caller runs godotenv in main then `config.FromEnv(os.LookupEnv)` |
| pool progress hook | 1 | below the bar | built anyway, D3 |
| atomic file replace; saboteur helper | 1 | LADDER2 §2 | deferred by D7 (DEFERRED.md) |

## 5. (d) One-off patterns (REPEAT 1-2 or 0) NOT built, with the reason

- bridge, priority merge, weighted merge (0): authority-only, already shipped; nothing to decide.
- clock Sleep/After/Since (0): `testing/synctest` is the answer; record in the clock spec (hardening).
- cache Purge/Keys/Peek (2, hashicorp API): no reference calls them; a decision when a consumer asks.
- config slice getter, Float (0): nobody uses them.
- logging DefaultPatterns (1): a shipped pattern set (Slack `xoxb-`/`xapp-`, Bearer, `sk-`, `AKIA`) is one option; hardening run, with the three secret-leaking repos as its test list.
- heartbeat "beat while fn runs" (2): five-line composition; one Example in the hardening run.
- pubsub OnDelivered (1): built only because D5 names it.
- recycle `Put((*T)(nil))` pools a typed nil (0 references; found by M1): test + decide in hardening.
- bloom overfilled-filter test (0): hardening.
- struct binding by tags (9): NOT-GENERAL by CLAUDE.md (reflection); recorded as a decision so it is not re-asked (section 7).
- YAML config file (8): `adapters/yaml`, a dependency decision at its wave (LADDER §3).
- startup "second signal forces exit" (0 once read): no reference implements it; the caller's WithNotify wrapper if wanted.
- breaker failure-ratio window and growing open time (0): Azure variant, additive options reserved (ALG-STREAM-002, ALG-BACKOF-001) for a consumer request.
- `httpx.RateLimit` mapping of `ErrKeys` (429 vs 503): a decision at the httpx wave.

## 6. Scale findings (what breaks first at 10x / 100x, from the code; numbers recorded by VERIFY in `out/`)

| package | finding | action |
|---|---|---|
| pubsub | one `Bus.mu` for every topic on every Publish (bus.go:98-101, :118-120); fan-out sequential behind a full subscriber chan (:124-130): one slow subscriber holds the topic | D5 drop-subscriber policy; `BenchmarkBus_Publish_manyTopicsManyPublishers` added by the builder, measured in VERIFY; sharding only if the number says so |
| group | `freed` close-broadcast: O(waiters) wakeups per release | benchmark in VERIFY; deferred fix |
| pool | collect-mode `errs` unbounded when most values fail (pool.go:199-201); Close waits for every in-flight Submit under the RWMutex | D3 bounded errors option |
| guard | one Guarded serialises every key (go-ratelimit's defect) | D1 Keyed |
| parallel | `workers` defaults to GOMAXPROCS per CALL: 100x callers = 100x GOMAXPROCS goroutines (ordered.go:38-40) | doc note (process-wide bound is the caller's semaphore); no code |
| flow | Priority/Weighted: n receiver goroutines per idle turn, O(n) poll per value; Weighted allocates a picker per pick | benchmark in VERIFY; deferred |
| panics | `Try` wraps a second closure around `TryValue` (try.go:20): 1-2 allocs per call on every hot path | record `BenchmarkTry` allocs/op in VERIFY; inline in hardening if > 0 |
| logging | Context derives a handler per record with ctx attrs; Redact runs every pattern over every string | record BenchmarkContext/BenchmarkRedact in VERIFY; hardening |
| cache | one mutex, count bound only; no Benchmark exists | hardening adds the benchmark |
| ratelimit.Keyed | fails CLOSED (ErrKeys) for new clients at 10 000 active keys inside one refill time; every reference fails open and grows | documented; httpx decides 429 vs 503 |
| clock.Fake | O(n) scan per fire, O(n^2) Advance over n timers (fake.go:255-291): test-only | note in the clock spec (hardening) |
| batch.Channel | `buf` rebuilt from nil per batch: log2(size) allocs per batch | `make([]T, 0, size)`: hardening |

## 7. Decisions surfaced by the matrix (recorded by the lead in DECISIONS/ASSUMPTIONS; Devin confirms)

1. `.env` loader: NOT built (no authority; Twelve-Factor says the environment; godotenv in main is three lines).
   Alternative: `config.FromDotEnv(io.Reader)` (REPEAT 20). Devin's call; recorded in ASSUMPTIONS until then.
2. Struct binding by tags: NOT-GENERAL for toolbench (reflection; typed getters + Required are the house answer).
3. retry x breaker: `retry`'s default predicate retries `breaker.ErrOpen`; fixed by an Example and doc (no import
   between the two T3 packages; `breaker` keeps returning exactly `ErrOpen`). Alternative: breaker wraps ErrOpen in
   `retry.Permanent` (a cross-package import and a changed error chain): lost.
4. No unbounded retry: `WithAttempts(0)` stays "default 3"; startup retry = bounded attempts + `WithMaxElapsed` +
   ctx deadline, then exit non-zero (Twelve-Factor IX).
5. `Component.Run` (run-time failure seam) is a new struct field: additive for keyed literals; an unkeyed
   `Component{...}` literal (vet composites warns) would break. Alternative `Group.Go(name, fn)`: lost (a
   component with both a Start and a Run would need two registrations).

Corrections to LADDER2.md found by the readers: `ratelimit.Keyed.Run(ctx)` does not exist (eviction is inline, D-031;
LADDER2 §3 ratelimit line); monolith signals.go does not force exit on a second signal, it swallows it (LADDER2 §3
lifecycle line). Both lines are annotated in LADDER2.md with a dated note.

## 8. Measured in VERIFY (go1.27.3, darwin/arm64, -race on, so absolute ns/op are inflated; allocs/op are exact; raw in `out/`)

| benchmark | ns/op | B/op | allocs/op | what it settles |
|---|---|---|---|---|
| panics BenchmarkTry | 172 | 0 | 0 | the "1-2 closure allocs per Try" claim in M1/M2/M3 scale notes is WRONG: the compiler keeps the closures on the stack; no hardening item for Try |
| panics BenchmarkTry_panic | 11 229 | 3 120 | 3 | the cost of a recovered panic (stack capture); fine |
| flow BenchmarkFanOut | 4.3 ms per run | 2 046 | 32 | per-run setup (workers + channels), not per item; per-item allocations need a per-item benchmark (hardening) |
| group BenchmarkGroup_Go | 26 906 | 278 | 9 | 9 allocs per Go is the number to cut if group is ever on a hot path; the `freed` broadcast herd was NOT measured (no blocked-caller benchmark exists; hardening) |
| logging BenchmarkContext | 2 294 | 672 | 10 | the per-record derived handler: 10 allocs per record with ctx attrs; the hardening fix has its baseline |
| logging BenchmarkRedact | 11 033 | 9 843 | 21 | pattern redaction cost per record; baseline for DefaultPatterns |
| logging BenchmarkText | 4 231 | 134 | 0 | the house handler allocates nothing per record |
| guard BenchmarkKeyed_Do / _DoDistinctKeys (final run) | 1 648 / 2 347 | 128 / 117 | 2 / 1 | every Do on an idle key allocates the entry and its channel (removed at refcount 0): a hardening option, not a defect; distinct keys do not serialise |
| pool BenchmarkPool_Submit / _SubmitFromWorker | 3 060 / 2 614 | 0 / 0 | 0 / 0 | the in-worker non-blocking path is free of allocation and never blocks (0.02 full/op) |
| pubsub BenchmarkBus_Publish / _manyTopicsSerial / _manyTopicsManyPublishers / _dropSubscriber (final run) | 2 298 / 2 664 / 3 553 (RunParallel: total throughput) / 24 946 (one evict-and-resubscribe cycle per op) | 0 / 0 / 0 / 931 | 0 / 0 / 0 / 15 | serially, 64 topics cost ~0.37 us over one topic, so Bus.mu is not the dominant cost at this scale; the RunParallel figure does not isolate the lock, sharding stays an open hardening question; the policy check on a draining subscriber allocates nothing, a full eviction cycle costs 15 allocs |
| group BenchmarkGroup_Go_blockedCallers | 514 ms per 1 000 blocked Go | 623 532 | 5 310 | ~5 allocs per blocked Go including the broadcast herd: the hardening baseline (MATRIX §3 rank 11) |
| retry BenchmarkDo_withMaxElapsed | 528 | 96 | 1 | the cap is one clock read per retry |
| ctxkey BenchmarkNewRequestID / EnsureRequestID hit / miss / ValidRequestID | 976 / 111 / 1 149 / 27 | 32 / 0 / 96 / 0 | 1 / 0 / 3 / 0 | a request id is one crypto/rand read; a hit allocates nothing |
