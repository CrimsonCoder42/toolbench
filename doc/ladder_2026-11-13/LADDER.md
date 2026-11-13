# LADDER: the build plan for toolbench (2026-11-13)

Merged by the lead from R1-R4 (97 reader rows over 81 repos: 68 pattern rows, 22 SKIP, 7 FRAMEWORK)
plus the standard patterns the authorities expect. Today's Go is go1.27 (`GO_RELEASES.md`); every TODAY'S GO cell
in R1-R4 was checked against that list. code-heim is the reading list only; nothing here cites it. This file is a
PROPOSAL: nothing in it is written into `doc/taxonomy.md` or `.claude/` until Devin approves (section 9).

USED TODAY counts `file:line` hits found by the readers (read-only greps, at most three per project per pattern) in
backendAdvo (advo) and red_lite_cache (red); "-" means not searched (patterns not in code-heim).

## 1. The ladder

Sorted by tier, then by how many other ladder patterns build on it, then by USED TODAY.

| rank | pattern | package | tier | builds on | repos | used today | calcbench IDs | authority | compare against |
|---|---|---|---|---|---|---|---|---|---|
| 1 | panic-to-error (recover in caller code) | `panics` | T0 | - | go_27, go_67 | 5 (advo metabolic_reports.go:577, respiratory_reports.go:779, cardiovascular_reports.go:693; red reader/conn.go:540, internal/rpc/conn.go:231) | none | Gerrand, "Defer, Panic, and Recover", The Go Blog, 2010, "Recover"; Effective Go, "Recover" | github.com/sourcegraph/conc/panics |
| 2 | clock (injectable time source) | `clock` | T0 | - | standard | - | none | pkg.go.dev `time` (`Timer`, `AfterFunc`, go1.23 timer semantics); pkg.go.dev `testing/synctest`; Meszaros, xUnit Test Patterns, 2007, "Test Double" | github.com/benbjohnson/clock, github.com/jonboulle/clockwork |
| 3 | functional options | convention (per package) | T0 | - | go_14, go_41, go_85 | 3 (advo providers/provider.go:137, medlineplus/worker.go:50; red harness_leaks.go:42) | none | Pike, "Self-referential functions and the design of options", 2014; Cheney, "Functional options for friendly APIs", 2014 | none |
| 4 | typed context key | `ctxkey` | T0 | - | go_51 | 4 (advo cache/store.go:202, feed_profile.go:252; red internal/rpc/grpc.go:230, admin_auth.go:134) | none | pkg.go.dev `context.WithValue` (key must not be a built-in type); Ajmani, "Go Concurrency Patterns: Context", The Go Blog, 2014 | none |
| 5 | error joining | stdlib `errors.Join` (no export) | T0 | - | go_67, go_71 | - | none | pkg.go.dev `errors.Join` (go1.20) | none |
| 6 | must (panic at program start) | none proposed (decision 9.10) | T0 | - | go_70, go_71 | 0 | none | pkg.go.dev `regexp.MustCompile`, `text/template.Must` | none |
| 7 | error group (cancel on first error, bounded) | `group` | T1 | panic-to-error | go_61, go_67 | 2 (advo feed_sources.go:1407; red internal/app/app.go:265) | none | pkg.go.dev golang.org/x/sync/errgroup; Cox-Buday, Concurrency in Go, 2017, ch.4 "Error Handling" | golang.org/x/sync/errgroup, github.com/sourcegraph/conc |
| 8 | fan-out / fan-in (merge) | `flow` | T1 | panic-to-error | go_25 | 4 (advo metabolic_reports.go:569, feed_sources.go:1407; red replica/node.go:255, :399) | none (channels: see 5) | Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Fan-out, fan-in"; Cox-Buday ch.4 "Fan-Out, Fan-In" | github.com/sourcegraph/conc/stream |
| 9 | generator and pipeline stage | `flow` | T1 | panic-to-error | go_21, go_56 | 0 | ALG-SEQ-001..008 are the synchronous form (not duplicated) | Ajmani 2014, "What is a pipeline?", "Explicit cancellation"; Cox-Buday ch.4 "Generators", "Pipelines" | none |
| 10 | semaphore (weighted) | `semaphore` | T1 | - | standard | - | none | pkg.go.dev golang.org/x/sync/semaphore `Weighted`; Dijkstra, "Cooperating sequential processes", EWD123, 1965 | golang.org/x/sync/semaphore |
| 11 | or-done channel | `flow` | T1 | generator | go_62 | 0 | none | Cox-Buday ch.4 "The or-done-channel" | none |
| 12 | tee channel | `flow` | T1 | or-done | go_62 | 0 | none | Cox-Buday ch.4 "The tee-channel" | none |
| 13 | bridge channel | `flow` | T1 | or-done | standard | - | none | Cox-Buday ch.4 "The bridge-channel" | none |
| 14 | priority and weighted merge | `flow` | T1 | fan-in | go_42 | 0 | ALG-SAMPLE-003 NewWeightedPicker, ALG-SAMPLE-004 WeightedPicker.Pick | Azure Architecture Center, "Priority Queue pattern"; Hohpe & Woolf, EIP, 2003, "Message Dispatcher" | github.com/contribsys/faktory_worker_go |
| 15 | buffered vs unbuffered (backpressure default) | `flow` option `WithBuffer` | T1 | - | go_24 | 1 (red reader/reader.go:191) | none | Go spec, "Channel types"; The Go Memory Model, "Channel communication" | none |
| 16 | mutual exclusion and confinement | `guard` | T1 | - | go_20 | 4 (advo cache/store.go:433, formula.go:25; red reader/reader.go:151, tenant.go:48) | none | Cox-Buday ch.4 "Confinement", ch.3 "Mutex and RWMutex"; The Go Memory Model, "Locks" | none |
| 17 | object pool (typed sync.Pool) | `recycle` | T1 | - | go_54 | 0 | none | pkg.go.dev `sync.Pool`; golang/go#23199 (oversized-buffer retention) | github.com/valyala/bytebufferpool |
| 18 | worker pool (competing consumers) | `pool` | T2 | error group, semaphore, panic-to-error | go_18, go_66, go_67 | 3 (advo cleanup_worker.go:95, respiratory_reports.go:772, metabolic_reports.go:575) | none (queue is a channel: backpressure) | Ajmani 2014, "Bounded parallelism"; Hohpe & Woolf, EIP, "Competing Consumers" | github.com/sourcegraph/conc/pool, github.com/Jeffail/tunny, github.com/alitto/pond/v2 |
| 19 | graceful shutdown (run group, drain with deadline) | `lifecycle` | T2 | error group, clock | go_42, go_37, go_80 | 3 (red cmd/redlite/main.go:39, wire_replica.go:235, app.go:305) | none | pkg.go.dev net/http `Server.Shutdown`; pkg.go.dev os/signal `NotifyContext`; The Twelve-Factor App, "IX. Disposability" | github.com/oklog/run |
| 20 | task scheduling (every, at, cron, heartbeat) | `schedule` | T2 | clock, panic-to-error | go_49, go_50, go_37 | 7 (advo cmd/api/main.go:104, medlineplus/worker.go:88, pubmed/pubmed.go:91, api/middleware/rate_limit.go:100; red reader/reader.go:221, internal/api/touch.go:63, internal/lock/lock.go:161) | ALG-HEAP-002/004/005 (next-run order), NUM-PROB-059 UniformQuantile (jitter); MISSING cron next-fire | pkg.go.dev `time.Timer`, `time.Ticker`; IEEE Std 1003.1-2024, "crontab"; Cox-Buday ch.5 "Heartbeats" | github.com/go-co-op/gocron/v2, github.com/robfig/cron/v3 |
| 21 | bounded parallel map / filter / for-each | `parallel` | T2 | error group, panic-to-error | go_25, go_67, go_60 | 4 (advo feed_sources.go:1407, metabolic_reports.go:569; red node.go:255, :399) | ALG-SEQ-001 Map, ALG-SEQ-002 Filter (same names, sequential twins), ALG-SEQ-004 Chunk | Ajmani 2014, "Bounded parallelism"; Cox-Buday ch.4 "Fan-Out, Fan-In" | github.com/sourcegraph/conc/iter, golang.org/x/sync/errgroup |
| 22 | in-process publish-subscribe + Publisher/Subscriber interfaces | `pubsub` | T2 | fan-out, panic-to-error | socket_io_emitter, go_44, go_58 | 1 (red internal/sheet/sheet.go:54) | ALG-QUEUE-002 CircularBuffer (per-subscriber ring for `WithDropOldest`, under a mutex) | Hohpe & Woolf, EIP, "Publish-Subscribe Channel", "Event-Driven Consumer", "Point-to-Point Channel" | github.com/ThreeDotsLabs/watermill |
| 23 | batching (size or time) | `batch` | T2 | generator, clock | go_38 | 1 (advo fdc_import.go:892) | ALG-SEQ-004 Chunk, ALG-SEQ-007 Batch | Hohpe & Woolf, EIP, "Aggregator" (completeness by count or timeout) | none |
| 24 | retry with backoff and jitter | `retry` | T3 | clock, panic-to-error | go_37, go_68, go_78 (implied) | 2 (advo medlineplus/worker.go:259; red reader/link.go:75) | MISSING exponential backoff with jitter | Nygard, Release It!, 2nd ed., 2018, ch.5 "Stability Patterns"; Azure, "Retry pattern"; Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015 | github.com/cenkalti/backoff/v4, github.com/avast/retry-go |
| 25 | singleflight (duplicate suppression) | `singleflight` | T3 | panic-to-error | go_45 (stampede, implied) | - | none | pkg.go.dev golang.org/x/sync/singleflight | golang.org/x/sync/singleflight |
| 26 | rate limit (token bucket) and keyed limiter | `ratelimit` | T3 | clock, guard | go_77, go_82 | 6 (advo api/middleware/rate_limit.go:53, :82, :273; red adminauth/limiter.go:56, :62, :131) | ALG-RATELM-001 TokenBucket (and -002, -004 by option), ALG-LIST-015 LRUCache (`WithMaxKeys`), all under one mutex | pkg.go.dev golang.org/x/time/rate `Limiter`; RFC 2697 §3; Azure, "Rate Limiting pattern" | golang.org/x/time/rate |
| 27 | circuit breaker | `breaker` | T3 | clock | standard | - | ALG-STREAM-002 HitCounter (failures and calls in the last window) | Nygard, Release It!, 2nd ed., ch.5 "Circuit Breaker"; Fowler, "CircuitBreaker", martinfowler.com, 2014; Azure, "Circuit Breaker pattern" | github.com/sony/gobreaker/v2 |
| 28 | bulkhead (bounded concurrency, fail fast) | `bulkhead` | T3 | semaphore | standard | - | none | Nygard, Release It!, 2nd ed., ch.5 "Bulkheads"; Azure, "Bulkhead pattern" | none |
| 29 | timeout (per call, with cause) | `retry` option + stdlib | T3 | - | go_51 | 2 (red wire_replica.go:235, replica/node.go:402) | none | Nygard, Release It!, 2nd ed., ch.5 "Timeouts"; pkg.go.dev `context.WithTimeoutCause` | none |
| 30 | cache (TTL, LRU, cache-aside, bloom guard) | `cache` | T4 | singleflight, clock, guard | go_11, go_45, go_69 | 5 (advo cache/shared.go:109, :148, fitness_routines.go:89; red reader/local.go:78, internal/cache/store.go:18) | ALG-LIST-015 LRUCache, ALG-LIST-016 LFUCache, ALG-HEAP-002/004/005, ALG-BLOOM-001/002/003 (all under a mutex) | Azure, "Cache-Aside pattern"; Bloom, CACM 13(7), 1970 | github.com/hashicorp/golang-lru/v2/expirable, github.com/jellydator/ttlcache/v3, github.com/bits-and-blooms/bloom/v3 |
| 31 | config layering (defaults, file, environment) | `config` | T4 | error joining | go_17, go_31, go_72 | 6 (advo util/config.go:50, :166, cmd/api/main.go:25; red internal/config/config.go:126, :130, :237) | none | Wiggins, The Twelve-Factor App, "III. Config"; pkg.go.dev `os.LookupEnv` | github.com/spf13/viper, github.com/caarlos0/env/v11, github.com/kelseyhightower/envconfig |
| 32 | lock with lease (distributed lock, fencing token) | `lock` | T4 | retry, clock | go_8 | 4 (advo fdc_import.go:257, :267; red internal/persist/election.go:92, :128) | none (backoff via retry) | Redis docs, "Distributed Locks with Redis", "Correct implementation with a single instance"; Kleppmann, "How to do distributed locking", 2016 | github.com/bsm/redislock, github.com/go-redsync/redsync/v4 |
| 33 | structured logging handlers (redact, context attributes) | `logging` | T4 | - | go_19, go_26, go_79 | 3 (red internal/logger/logger.go:13, :28, :39) | none | pkg.go.dev log/slog `Handler`, `HandlerOptions.ReplaceAttr`, `LogValuer`; Amsterdam, "Structured Logging with slog", The Go Blog, 2023 | go.uber.org/zap, github.com/rs/zerolog |
| 34 | decorator (middleware chain) | `httpx.Chain`; gRPC has `grpc.ChainUnaryInterceptor` | T4 | - | go_7, go_43, go_79 | 3 (advo api/middleware/auth.go:21, rate_limit.go:203, access.go:1412) | none | Gamma et al., Design Patterns, 1994, "Decorator"; pkg.go.dev net/http `Handler` | github.com/justinas/alice |
| 35 | builder | functional options (no package) | T4 | functional options | go_14 | see rank 3 | none | Gamma et al., Design Patterns, 1994, "Builder" | none |
| 36 | dependency injection | constructor injection (no package) | T4 | small interfaces | go_41 | 0 | none | Fowler, "Inversion of Control Containers and the Dependency Injection pattern", 2004, "Constructor Injection" | go.uber.org/fx, github.com/google/wire |
| 37 | transaction (run in a transaction) | `txn` | T5 | panic-to-error, error joining | go_38, go_71, go_74 | 3 (advo db/sqlc/store.go:103, chatdb/sqlc/store.go:31; red db/store.go:57) | none | Fowler, PoEAA, 2002, ch.11 "Unit of Work"; pkg.go.dev database/sql `DB.BeginTx` | github.com/jackc/pgx/v5 `BeginFunc` |
| 38 | pagination (offset and keyset) | `page` | T5 | - | go_9, go_30, go_33 | 6 (advo cache/keys.go:1365, fitness_catalog.go:43, peak_expiratory_airflow_rate.go:425; red internal/api/sessions.go:70, :175, admin.sql.go:439) | MISSING integer ceiling division | Winand, Use The Index, Luke!, "Paging Through Results"; Azure, "Web API design", "Filter and paginate data"; RFC 8288 §3 | none |
| 39 | job queue with retry, delay, priority, dead letter | `jobs` | T5 | worker pool, retry, pubsub, priority merge | go_37, go_42, go_68 | 0 | ALG-SAMPLE-003/004 (weighted queues), ALG-HEAP-002/004/005 (delayed jobs) | Hohpe & Woolf, EIP, "Competing Consumers", "Dead Letter Channel", "Invalid Message Channel"; Azure, "Queue-Based Load Leveling pattern" | github.com/hibiken/asynq, github.com/riverqueue/river, github.com/gocraft/work |
| 40 | repository | `store` | T5 | pagination | go_65 | 1 (red internal/api/store.go:21, :49) | none | Fowler, PoEAA, 2002, ch.10 "Repository" | none |
| 41 | transactional outbox | `outbox` | T5 | transaction, pubsub, retry | standard (go_44 dual write shows the need) | - | none | Richardson, Microservices Patterns, 2018, ch.3 "Transactional outbox"; Hohpe & Woolf, EIP, "Guaranteed Delivery" | github.com/ThreeDotsLabs/watermill (forwarder) |
| 42 | idempotency key | `idempotency` | T5 | cache, lock | standard | - | none | IETF draft-ietf-httpapi-idempotency-key-header, "The Idempotency-Key HTTP Header Field"; Hohpe & Woolf, EIP, "Idempotent Receiver" | none |
| 43 | blob store (+ presigned URL) | `blob` | T5 | repository | go_76 | 0 | none | Fowler, PoEAA, ch.10 "Repository"; Azure, "Valet Key pattern" | gocloud.dev/blob |
| 44 | messaging gateway (mail sender) | `mail` | T5 | jobs, retry | go_47 | 2 (advo api/services/email/sender.go:15, :116) | none | Fowler, PoEAA, ch.18 "Gateway"; RFC 5322 §3.6 | github.com/wneessen/go-mail |
| 45 | HTTP middleware: chain, recover, access log | `httpx` | T6 | decorator, panic-to-error, clock, logging | go_43, go_79 | 3 (see rank 34) | none | pkg.go.dev net/http `Handler`, `ResponseController` | github.com/go-chi/chi/v5/middleware |
| 46 | HTTP error response (problem details) | `httpx` | T6 | - | go_32 | 4 (advo auth.go:76, admin/analytics.go:314; red internal/api/api.go:368, protocol/admin.go:56) | none | RFC 9457 §3; RFC 9110 §15 | none |
| 47 | HTTP rate-limit middleware (429, Retry-After, client IP) | `httpx` | T6 | rate limit | go_82 | 2 (advo rate_limit.go:203; red internal/api/api.go:50) | via ratelimit | RFC 6585 §4; RFC 9110 §10.2.3 | github.com/ulule/limiter/v3 |
| 48 | request body limit (413) | `httpx` | T6 | - | go_64 | 3 (red internal/api/api.go:359, tenancy.go:125, admin_reads.go:59) | none | pkg.go.dev `http.MaxBytesReader`, `MaxBytesError`; RFC 9110 §15.5.14 | none |
| 49 | HTTP client gateway (retry transport, JSON decode) | `httpx` | T6 | retry, timeout | go_78 | 1 (advo medlineplus/worker.go:259) | via retry | Fowler, PoEAA, ch.18 "Gateway"; RFC 9110 §9.2.2 | resty.dev/v3, github.com/hashicorp/go-retryablehttp |
| 50 | gRPC status mapping and interceptors | `grpcx` | T6 | panic-to-error, rate limit, retry | go_55 | 3 (red internal/rpc/grpc.go:198, :259, :265) | none | grpc.io, "Status codes and their use in gRPC"; pkg.go.dev google.golang.org/grpc/status | none |
| 51 | driver adapters | `adapters/<driver>` | T6 | the core interface each implements | go_8, go_44, go_45, go_58, go_65, go_68, go_74, go_75, go_76, go_47, go_73, go_31 | see section 3 | none | each driver's pkg.go.dev page | the driver itself |

Patterns per tier (ranks): T0 6 · T1 11 · T2 6 · T3 6 · T4 7 · T5 8 · T6 7 rows (rank 51 groups 11 adapters from 12 repos) = 51.

## 2. Standard patterns not in code-heim

Placed in the ladder above (marked "standard"): clock (2), semaphore (10), bridge channel (13), singleflight (25),
circuit breaker (27), bulkhead (28), transactional outbox (41), idempotency key (42). Retry with backoff and
jitter (24) is only implied by code-heim (MaxFails, MaxRetry). Also expected, placed but not yet ranked for a wave:

| pattern | tier | package | authority |
|---|---|---|---|
| hedged requests (tail-latency) | T3 | `retry` (`Hedge`) | Dean & Barroso, "The Tail at Scale", CACM 56(2), 2013, "Hedged requests" |
| load shedding / queue-based load leveling | T3 | `bulkhead` (`WithQueue`) | Azure, "Queue-Based Load Leveling pattern"; Nygard, Release It!, ch.5 "Shed Load" |
| health endpoint | T6 | `httpx` (`Health`) | Azure, "Health Endpoint Monitoring pattern" |
| HTTP server deadlines and graceful serve | T6 | `httpx` (`Serve`) | pkg.go.dev net/http `Server` (ReadHeaderTimeout, Shutdown) |
| deadline propagation across calls | T6 | `grpcx` | grpc.io, "Deadlines" guide |

## 3. Package map (PROPOSAL; not in doc/taxonomy.md)

| package | SHORT | layer | tier | patterns | depends on |
|---|---|---|---|---|---|
| `panics` | PANIC | core | T0 | ErrPanic, PanicError, Try | stdlib |
| `clock` | CLOCK | core | T0 | Clock interface, real clock, fake clock for tests | stdlib |
| `ctxkey` | CTXKEY | core | T0 | typed context key | stdlib |
| `group` | GROUP | core | T1 | error group, result group | panics |
| `flow` | FLOW | core | T1 | generator, stage, fan-out, merge, or-done, tee, bridge, priority/weighted merge | panics, calcbench algo/sample |
| `semaphore` | SEMA | core | T1 | weighted semaphore | stdlib |
| `guard` | GUARD | core | T1 | Guarded[T] (mutex beside the value), confinement owner | stdlib |
| `recycle` | RECYC | core | T1 | typed sync.Pool with reset and keep | stdlib |
| `pool` | POOL | core | T2 | worker pool (Submit/Close, Process) | group, semaphore, panics |
| `parallel` | PAR | core | T2 | bounded Map, Filter, ForEach, Chunked | group, panics, calcbench algo/seq |
| `batch` | BATCH | core | T2 | size-or-time batcher, Run over iter.Seq2 | flow, clock, calcbench algo/seq |
| `schedule` | SCHED | core | T2 | Every, At, Cron, Heartbeat | clock, panics, calcbench algo/heap, num/prob |
| `lifecycle` | LIFE | core | T2 | run group, ordered graceful stop | group, clock |
| `pubsub` | PUBSUB | core | T2 | in-process Bus, Publisher, Subscriber interfaces | flow, panics, calcbench algo/queue |
| `retry` | RETRY | core | T3 | Do, Policy, Permanent, per-attempt timeout, Hedge | clock, panics, calcbench (backoff, requested) |
| `singleflight` | FLIGHT | core | T3 | generic, ctx-aware duplicate suppression | panics |
| `ratelimit` | RATE | core | T3 | Limiter (Allow, AllowN, Wait), Keyed | clock, guard, calcbench algo/ratelimit, algo/list |
| `breaker` | BRK | core | T3 | circuit breaker | clock, calcbench algo/stream |
| `bulkhead` | BULK | core | T3 | bulkhead with optional queue | semaphore |
| `cache` | CACHE | core | T4 | TTL+LRU cache, GetOrLoad, Store interface, Bloom guard | singleflight, clock, guard, calcbench algo/list, algo/heap, algo/bloom |
| `config` | CONFIG | core | T4 | Lookup layering, typed getters | stdlib |
| `lock` | LOCK | core | T4 | Locker, Lease, Acquire | retry, clock |
| `logging` | LOG | core | T4 | slog handler wrappers: Redact, Context | stdlib |
| `txn` | TXN | core | T5 | Run in transaction | panics |
| `page` | PAGE | core | T5 | offset Request, keyset Cursor, Page[T] | calcbench (CeilDiv, requested) |
| `store` | STORE | core | T5 | Repository[K, V], ErrNotFound, in-memory | page |
| `jobs` | JOBS | core | T5 | Queue, Worker, retry, delay, priority, dead letter | pool, retry, pubsub, flow |
| `outbox` | OUTBOX | core | T5 | Outbox store interface, Relay | txn, pubsub, retry |
| `idempotency` | IDEM | core | T5 | key store interface, Guard | cache, lock |
| `blob` | BLOB | core | T5 | Store, Signer | store |
| `mail` | MAIL | core | T5 | Message, Sender | jobs, retry |
| `httpx` | HTTPX | httpx | T6 | Chain, Recover, AccessLog, RateLimit, RemoteIP, MaxBytes, WriteProblem, StatusOf, RetryTransport, DoJSON, ParsePage, Health | core |
| `grpcx` | GRPCX | grpcx (own go.mod) | T6 | status mapping, recover, rate limit, retry interceptors | core + google.golang.org/grpc |
| `adapters/redis` | REDIS | adapter | T6 | lock, cache Store, jobs Queue, pubsub | core + github.com/redis/go-redis/v9 |
| `adapters/pgx` | PGX | adapter | T6 | txn Beginner, advisory lock, outbox store | core + github.com/jackc/pgx/v5 |
| `adapters/kafka` | KAFKA | adapter | T6 | pubsub | core + a Kafka client (decision at its wave) |
| `adapters/rabbitmq` | RABBIT | adapter | T6 | pubsub with confirms | core + github.com/rabbitmq/amqp091-go |
| `adapters/s3` | S3 | adapter | T6 | blob Store, Signer | core + github.com/aws/aws-sdk-go-v2/service/s3 |
| `adapters/memcache` | MEMC | adapter | T6 | cache Store | core + github.com/bradfitz/gomemcache |
| `adapters/mongo` | MONGO | adapter | T6 | store Repository | core + go.mongodb.org/mongo-driver/v2 |
| `adapters/smtp` | SMTP | adapter | T6 | mail Sender | core + net/smtp or github.com/wneessen/go-mail |
| `adapters/websocket` | WS | adapter | T6 | connection read/write loop | core + github.com/coder/websocket |
| `adapters/lambda` | LAMBDA | adapter | T6 | Function URL event to http.Handler | core + github.com/aws/aws-lambda-go |
| `adapters/yaml` | YAML | adapter | T6 | config Lookup from a YAML file | core + a YAML module (decision at its wave) |

No package name shadows one a caller imports beside it (`.claude/rules/go-functions.md`). Toolbench's `ratelimit`
imports calcbench's `algo/ratelimit` (and `cache` imports `algo/bloom`) inside toolbench only, under an alias.

## 4. API list (one style)

Rules applied to every package: `ctx context.Context` first on anything that blocks or starts a goroutine;
directional channels; `error` last; per-package functional options `type Option func(*config)` named `With<Thing>`
(assumption, `doc/ASSUMPTIONS.md`); per-package sentinel errors `Err<Thing>` with the message prefix `"<pkg>: "`
(calcbench's style); a recovered panic is always `panics.ErrPanic` (one type, lowest package); time from
`WithClock(clock.Clock)`; first error wins and cancels unless the doc says it collects with `errors.Join`.

```go
// panics (T0)
var ErrPanic = errors.New("panics: recovered panic")
type PanicError struct{ Value any; Stack []byte }      // Unwrap() error returns ErrPanic
func Try(fn func() error) error
func TryValue[T any](fn func() (T, error)) (T, error)

// clock (T0)
type Clock interface{ Now() time.Time; NewTimer(d time.Duration) Timer; AfterFunc(d time.Duration, f func()) Timer }
type Timer interface{ C() <-chan time.Time; Stop() bool; Reset(d time.Duration) bool }
func Real() Clock
func NewFake(start time.Time) *Fake                     // Advance(d), BlockUntil(n)

// ctxkey (T0)
func New[T any](name string) *Key[T]
func (k *Key[T]) With(ctx context.Context, v T) context.Context
func (k *Key[T]) Value(ctx context.Context) (T, bool)

// group (T1): errgroup shape, panics become ErrPanic
func WithContext(ctx context.Context) (*Group, context.Context)
func (g *Group) Go(fn func() error); func (g *Group) TryGo(fn func() error) bool
func (g *Group) SetLimit(n int); func (g *Group) Wait() error
func NewResults[T any](ctx context.Context, opts ...Option) *Results[T] // Go(func(ctx) (T, error)); Wait() ([]T, error) in submission order

// flow (T1)
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
// options: WithBuffer(n int)

// semaphore (T1): x/sync/semaphore shape
func New(n int64) *Weighted
func (s *Weighted) Acquire(ctx context.Context, n int64) error; TryAcquire(n int64) bool; Release(n int64)

// guard (T1)
type Guarded[T any] struct{ /* mu beside v */ }        // zero value ready
func (g *Guarded[T]) Do(fn func(*T)); func (g *Guarded[T]) Load() T
func Confine[S any](ctx context.Context, state S, reqs <-chan func(*S)) S

// recycle (T1)
func New[T any](newFn func() T, opts ...Option[T]) *Pool[T]  // Get() T; Put(v T); WithReset, WithKeep

// pool (T2)
func New[T any](ctx context.Context, workers int, fn func(context.Context, T) error, opts ...Option) (*Pool[T], error)
func (p *Pool[T]) Submit(ctx context.Context, v T) error; func (p *Pool[T]) Close() error   // WithQueue, WithFailFast

// parallel (T2): concurrent twins of calcbench algo/seq, input order kept
func Map[In, Out any](ctx context.Context, in iter.Seq[In], workers int, fn func(context.Context, In) (Out, error)) ([]Out, error)
func Filter[T any](ctx context.Context, in iter.Seq[T], workers int, keep func(context.Context, T) (bool, error)) ([]T, error)
func ForEach[T any](ctx context.Context, in iter.Seq[T], workers int, fn func(context.Context, T) error) error

// batch (T2)
func Run[T any](ctx context.Context, in iter.Seq2[T, error], size int, flush func(context.Context, []T) error) error
func Channel[T any](ctx context.Context, in <-chan T, size int, maxWait time.Duration, opts ...Option) <-chan []T

// schedule (T2)
type Schedule interface{ Next(after time.Time) time.Time }
func Every(d time.Duration) Schedule; func ParseCron(spec string, loc *time.Location) (Schedule, error)
func Run(ctx context.Context, s Schedule, task func(context.Context) error, opts ...Option) error // no overlap; WithJitter, WithOnError, WithClock
func Heartbeat(ctx context.Context, interval time.Duration, opts ...Option) (beat func(), pulses <-chan time.Time)

// lifecycle (T2)
type Stopper func(ctx context.Context) error
func Run(ctx context.Context, grace time.Duration, start func(context.Context) error, stops ...Stopper) error // stops in reverse, errors.Join

// pubsub (T2)
type Publisher[M any] interface{ Publish(ctx context.Context, topic string, msgs ...M) error }
type Subscriber[M any] interface{ Subscribe(ctx context.Context, topic string, h func(context.Context, M) error) error }
func NewBus[M any](opts ...Option) *Bus[M]               // implements both; WithBuffer, WithDropOldest(report func(M))

// retry (T3)
type Policy struct{ MaxAttempts int; Base, Max time.Duration; Jitter Jitter }
func Do(ctx context.Context, p Policy, fn func(context.Context) error, opts ...Option) error
func DoValue[T any](ctx context.Context, p Policy, fn func(context.Context) (T, error), opts ...Option) (T, error)
func Permanent(err error) error                        // stop retrying; WithAttemptTimeout, WithRetryIf, WithClock
var ErrExhausted

// singleflight (T3)
func (g *Group[K, V]) Do(ctx context.Context, key K, fn func(context.Context) (V, error)) (v V, shared bool, err error)
func (g *Group[K, V]) Forget(key K)

// ratelimit (T3): x/time/rate shape
func New(perInterval int64, interval time.Duration, burst int64, opts ...Option) (*Limiter, error)
func (l *Limiter) Allow() bool; AllowN(n int64) (ok bool, retryAfter time.Duration); Wait(ctx context.Context) error
func NewKeyed[K comparable](perInterval int64, interval time.Duration, burst int64, opts ...Option) (*Keyed[K], error)
func (k *Keyed[K]) Allow(key K) (ok bool, retryAfter time.Duration); func (k *Keyed[K]) Run(ctx context.Context) error // idle sweep

// breaker (T3)
func New(opts ...Option) *Breaker                       // WithFailureRatio, WithWindow, WithOpenFor, WithHalfOpenProbes, WithClock
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error; State() State
var ErrOpen

// bulkhead (T3)
func New(maxConcurrent int, opts ...Option) (*Bulkhead, error) // WithQueue(n), WithQueueTimeout(d)
func (b *Bulkhead) Do(ctx context.Context, fn func(context.Context) error) error
var ErrFull

// cache (T4)
func New[K comparable, V any](opts ...Option) *Cache[K, V] // WithCapacity, WithTTL, WithLFU, WithClock
func (c *Cache[K, V]) Get(k K) (V, bool); Set(k K, v V); GetOrLoad(ctx context.Context, k K, load func(context.Context) (V, error)) (V, error)
type Store[K comparable, V any] interface{ Get(ctx context.Context, k K) (V, bool, error); Set(ctx context.Context, k K, v V, ttl time.Duration) error }
func Aside[K comparable, V any](ctx context.Context, s Store[K, V], k K, ttl time.Duration, load func(context.Context) (V, error)) (V, error)
func NewBloom(n uint64, fpr float64) (*Bloom, error)  // Add, MayContain; safe for concurrent use

// config (T4): no reflection, no environment reads (caller passes os.LookupEnv)
type Lookup func(key string) (string, bool)
func Layer(ls ...Lookup) Lookup; func FromMap(m map[string]string) Lookup
func String(l Lookup, key, def string) string
func Int(l Lookup, key string, def int) (int, error); Duration(...); Bool(...)
func Required(l Lookup, keys ...string) error          // all missing keys, errors.Join

// lock (T4)
type Locker interface{ TryLock(ctx context.Context, key string, ttl time.Duration) (Lease, error) }
type Lease interface{ Token() string; Refresh(ctx context.Context, ttl time.Duration) error; Unlock(ctx context.Context) error }
func Acquire(ctx context.Context, l Locker, key string, ttl time.Duration, opts ...Option) (Lease, error)
var ErrNotHeld, ErrLost

// logging (T4)
func Redact(h slog.Handler, keys ...string) slog.Handler
func Context(h slog.Handler, extract func(context.Context) []slog.Attr) slog.Handler

// txn (T5)
type Tx interface{ Commit(ctx context.Context) error; Rollback(ctx context.Context) error }
type Beginner[T Tx] interface{ Begin(ctx context.Context) (T, error) }
func Run[T Tx](ctx context.Context, b Beginner[T], fn func(context.Context, T) error) error // rollback on error or panic

// page (T5)
type Request struct{ Number, Size int }
func Normalize(r Request, opts ...Option) (offset, limit int); func Count(total int64, size int) (int64, error)
type Cursor[K any] struct{ After K; Limit int }; type Page[T any] struct{ Items []T; Next string; Total int64 }

// store, jobs, outbox, idempotency, blob, mail (T5): small interfaces + one in-memory implementation each
type Repository[K comparable, V any] interface{ Get(ctx, K) (V, error); Put(ctx, K, V) error; Delete(ctx, K) error; List(ctx, page.Request) (page.Page[V], error) }
type jobs.Queue[T any] interface{ Enqueue(ctx, Job[T], ...EnqueueOption) error; Dequeue(ctx) (Job[T], error); Ack(ctx, id string) error; Nack(ctx, id string, err error) error }
func jobs.Work[T any](ctx context.Context, q Queue[T], workers int, h func(context.Context, Job[T]) error, opts ...Option) error // WithMaxAttempts, WithDeadLetter
func outbox.Relay[M any](ctx context.Context, s Store[M], p pubsub.Publisher[M], opts ...Option) error
func idempotency.Guard[V any](ctx context.Context, s Store[V], key string, ttl time.Duration, fn func(context.Context) (V, error)) (V, error)
type blob.Store interface{ Put(ctx, key string, r io.Reader) error; Get(ctx, key string) (io.ReadCloser, error); List(ctx, prefix string) iter.Seq2[string, error] }
type mail.Sender interface{ Send(ctx context.Context, m Message) error }

// httpx (T6)
type Middleware func(http.Handler) http.Handler
func Chain(h http.Handler, mws ...Middleware) http.Handler
func Recover(onPanic func(*http.Request, error)) Middleware
func AccessLog(l *slog.Logger, opts ...Option) Middleware   // status, bytes, duration; wrapper has Unwrap()
func RateLimit(l *ratelimit.Keyed[string], key func(*http.Request) string) Middleware // 429 + Retry-After
func RemoteIP(r *http.Request, trusted ...netip.Prefix) netip.Addr
func MaxBytes(n int64) Middleware; func WriteProblem(w http.ResponseWriter, p Problem) error; func StatusOf(err error) int
func RetryTransport(next http.RoundTripper, p retry.Policy) http.RoundTripper
func DoJSON[Out any](ctx context.Context, c *http.Client, req *http.Request) (Out, error)

// grpcx (T6)
type Mapper func(error) (codes.Code, bool)
func UnaryServerStatus(m Mapper) grpc.UnaryServerInterceptor; StreamServerStatus(m Mapper) grpc.StreamServerInterceptor
func UnaryServerRecover() grpc.UnaryServerInterceptor; UnaryServerRateLimit(l *ratelimit.Keyed[string], key func(ctx context.Context) string) grpc.UnaryServerInterceptor
```

Deliberate differences from the familiar shapes (each spec states them): `semaphore.New` (x/sync names it
`NewWeighted`; the type stays `Weighted`); `ratelimit.New(perInterval, interval, burst)` and `AllowN` returning
`(ok, retryAfter)` follow calcbench ALG-RATELM-001's integer meter, not x/time/rate's `NewLimiter(r, b)` float
rate; `Allow() bool` keeps x/time/rate's form, while `Keyed.Allow` returns the retry-after an HTTP 429 needs.

Names settled in the merge (readers proposed several): `panics` (not `safe`), `group` for errgroup + results
(conc's `pool.WithResults` shape becomes `group.NewResults`), `pool` for the long-lived worker pool, `parallel` for
the bounded Map/Filter (so `flow` stays channels only), `txn` (not `tx`), `ratelimit` (not `limit`), `jobs` (not
`taskq`), `store` (not `broker`/`repository`); the go_21/go_56 `Generate(ctx, next)` is dropped for `FromSeq`
(an `iter.Seq` already is a generator). Contested names are in section 9.

## 5. calcbench use

22 IDs called, all from calcbench v0.1.0:

| calcbench package | IDs | called by |
|---|---|---|
| algo/seq | ALG-SEQ-001 Map, ALG-SEQ-002 Filter, ALG-SEQ-004 Chunk, ALG-SEQ-007 Batch | parallel (same names; workers = 1 is the sequential twin), batch |
| algo/queue | ALG-QUEUE-002 CircularBuffer | pubsub `WithDropOldest` (under a mutex) |
| algo/heap | ALG-HEAP-002 NewHeap, ALG-HEAP-004 Heap.Push, ALG-HEAP-005 Heap.Pop | schedule (next run), cache (expiry), jobs (delayed jobs) |
| algo/list | ALG-LIST-015 LRUCache, ALG-LIST-016 LFUCache | cache, ratelimit `WithMaxKeys` |
| algo/ratelimit | ALG-RATELM-001 TokenBucket, -002 LeakyBucket, -003 FixedWindow, -004 SlidingWindowLog | ratelimit (one mutex per limiter; the meters are not safe for concurrent use, algo/ratelimit/doc.go:18) |
| algo/bloom | ALG-BLOOM-001 BloomFilter, -002 FalsePositiveRate, -003 OptimalParams | cache `Bloom` |
| algo/sample | ALG-SAMPLE-003 NewWeightedPicker, ALG-SAMPLE-004 WeightedPicker.Pick | flow `Weighted`, jobs weighted queues |
| algo/stream | ALG-STREAM-002 HitCounter | breaker (calls and failures in the last window) |
| num/prob | NUM-PROB-059 UniformQuantile | schedule `WithJitter` (u from math/rand/v2) |
| num/stats | NUM-STATS-001 ArithmeticMean | internal/calccheck (the link test; no ID) |

Re-implemented on purpose (the row says why): flow, pool and group buffers are channels, not ALG-QUEUE-002, because
they must block the producer (backpressure) and be safe across goroutines.

**MISSING FROM CALCBENCH** (requests; nothing is built here):
1. Exponential backoff with jitter: full, equal and decorrelated (Brooker, "Exponential Backoff And Jitter", AWS
   Architecture Blog, 2015; gRPC "Connection Backoff Protocol", grpc/grpc doc/connection-backoff.md). Needed by
   retry, lock.Acquire, jobs, httpx.RetryTransport. Blocks wave 5's `retry`.
2. Overflow-safe integer ceiling division `CeilDiv(a, b int64) (int64, error)` (Knuth, Concrete Mathematics, §3.2).
   Needed by page.Count (the repos' `math.Ceil(float64)` is wrong above 2^53).
3. Cron next-fire time: the next instant matching a five-field POSIX crontab spec in a `time.Location`, with DST
   gaps and overlaps (IEEE Std 1003.1-2024, "crontab"). Needed by schedule.ParseCron. See decision 9.12.

Not missing (readers asked): bounded random jitter of a duration is NUM-PROB-059 with u from math/rand/v2; latency
percentiles are NUM-STATS-015; a sliding failure window is ALG-STREAM-002.

## 6. Build waves (at most 4 builders, one package each; no wave depends on a later one)

| wave | packages (builders) | tier | blocked on |
|---|---|---|---|
| **1** | `panics`, `clock`, `ctxkey` | T0 | taxonomy rows (Devin) |
| **2** | `group`, `flow`, `semaphore`, `guard` | T1 | wave 1 |
| 3 | `recycle`, `pool`, `parallel`, `batch` | T1-T2 | wave 2 |
| 4 | `schedule`, `lifecycle`, `pubsub` | T2 | wave 3; cron part on calcbench request 3 |
| 5 | `retry`, `singleflight`, `ratelimit`, `breaker` | T3 | wave 4; `retry` on calcbench request 1 |
| 6 | `bulkhead`, `cache`, `config`, `logging` | T3-T4 | wave 5 |
| 7 | `lock`, `txn`, `page` | T4-T5 | wave 6; `page` on calcbench request 2 |
| 8 | `store`, `jobs`, `outbox`, `idempotency` | T5 | wave 7 (`store` needs `page`) |
| 9 | `blob`, `mail`, `httpx`, `grpcx` | T5-T6 | wave 8; grpcx is HIGH risk (own go.mod, gRPC dependency) |
| 10+ | `adapters/pgx`, `adapters/redis`, then the rest by USED TODAY | T6 | wave 9; each adapter dependency is Devin's yes |

Wave 1: `panics` (ErrPanic, PanicError, Try, TryValue), `clock` (Clock, Timer, Real, Fake), `ctxkey` (Key[T]).
Wave 2: `group` (Group, WithContext, Results), `flow` (FromSeq, ToSeq, Stage, FanOut, Merge, OrDone, Tee, Bridge,
Priority, Weighted), `semaphore` (Weighted), `guard` (Guarded, Confine).

## 7. Outdated in the repos (do not port)

- `go func(v T){...}(v)` and `x := x` loop-variable copies: per-iteration variables since go1.22.
- `for i := 0; i < n; i++` counters: `for i := range n` (go1.22).
- `interface{}` payloads and type assertions (go_42, go_54, go_66): generics and `any`.
- reflect.Call in a cache helper (go_45): generics.
- channel generators for synchronous producers (go_56): `iter.Seq` (go1.23).
- hand-rolled signal goroutine + `os.Exit` (go_48, go_49, go_50, go_58): `signal.NotifyContext` (cause names the signal since go1.26).
- `for {}` to block main (go_49, go_50): `<-ctx.Done()`.
- `time.Sleep` coordination and polling (go_51, go_62, go_66, go_11): channels, timers, `testing/synctest`.
- one goroutine per cache entry sleeping for TTL (go_11): one timer plus a heap.
- `log.Logger` per level built in `init`, global loggers (go_19, go_79): `log/slog`, `slog.DiscardHandler`, no globals.
- `io.MultiWriter` log fan-out (go_26): `slog.NewMultiHandler` (go1.26).
- `r.Method != "POST"` checks and `strings.TrimPrefix` path parsing (go_38, go_44, go_47, go_53): ServeMux method and wildcard patterns, `Request.PathValue` (go1.22).
- context-less `DB.Begin`, `context.Background()`/`context.TODO()` inside handlers (go_38, go_65, go_74, go_76): `BeginTx(ctx)`, `r.Context()`.
- `math.Ceil(float64)` page counts (go_9, go_30): integer ceiling division.
- string context keys (go_51): unexported key type or `ctxkey.Key[T]`.
- `github.com/google/uuid` (go_63): standard library `uuid` (go1.27).
- go-redis under github.com/go-redis (socket_io_emitter): github.com/redis/go-redis/v9.
- mongo-driver v1 (go_65): go.mongodb.org/mongo-driver/v2.
- amqp091 `Publish`: `PublishWithContext` plus confirms (go_58).
- `errors.As` with a pointer variable: `errors.AsType[E]` (go1.26) in new code.
- gocraft/work (go_37): unmaintained since 2019.
- WebSocket origin checks disabled (go_12 `CheckOrigin` true, go_75 `InsecureSkipVerify`): keep the origin check; `http.CrossOriginProtection` (go1.25) for cookie-authenticated routes.

Defects seen in the repos, kept as test cases for ours: go_11 cache hit writes twice; go_8 unlock deletes another
holder's lock; go_14 builder reuse leaks state; go_20 confinement owner never exits; go_21 stages leak on early
consumer exit; go_51 timeout leaks its goroutine; go_62 tee sends one value twice to one side; go_66 stateful worker
with a value receiver; go_77 data race on the limiter map; go_80 handler spawned with a nil conn; go_58 one AMQP
channel shared across goroutines; go_60 iterator not restartable.

## 8. SKIPPED and FRAMEWORK

SKIPPED (library usage or out of scope):
- go_2, go_3, go_6, go_10, go_13, go_34, go_35: GORM usage; the projects use sqlc + pgx.
- go_4: Query Object over GORM scopes; sqlc queries are static.
- go_5: encoding/xml usage (and not valid RSS 2.0).
- go_15: GraphQL; not in scope.
- go_39: excelize usage (ingest is `batch.Run`).
- go_40: language tutorial; `cmp.Ordered` + `slices.Sort`, and calcbench algo/sorting.
- go_46: Elasticsearch usage (a rebuild would be `batch.Run` + an adapter).
- go_48: CLI application (cobra + langchaingo).
- go_52: protobuf usage.
- go_57: build tags setting globals from `init`.
- go_59: PDF generation (maroto).
- go_60: `iter` itself; calcbench algo/seq owns the sequential forms.
- go_63: cobra CLI.
- go_71 (struct scan, named params): reflection; sqlc and pgx `CollectRows` cover it.
- go_81: Grafana Alloy configuration.
- go_83: httptest convention, covered by `.claude/rules/go-testing.md`.

FRAMEWORK (the later net/http server framework):
- go_12, go_75 server side: WebSocket upgrade and hub (the per-connection loop is `adapters/websocket`).
- go_23: content negotiation (`Negotiate`, RFC 9110 §12).
- go_28, go_33: htmx fragment rendering and infinite scroll.
- go_30: session authentication middleware.
- go_36: bearer-token (JWT) middleware.
- go_53: handler tests against a database.
- go_64 (upload): multipart upload confined with `os.Root`.
- go_80: TCP accept loop (decision 9.9; not core under the current layer table).

## 9. Decisions for Devin

Each: recommendation · the alternative · why it lost.
1. `panics` · `safe` · `panics` names what it handles and matches conc/panics; `safe` says nothing.
2. `ratelimit` · `limit` · `limit` also reads as a concurrency limit (bulkhead, semaphore); toolbench imports
   calcbench `algo/ratelimit` under an alias, which only toolbench sees.
3. `recycle` (typed sync.Pool) · `objpool` · `pool` is the worker pool; `objpool` is a compound name.
4. `jobs` · `taskq` / `queue` · `queue` collides with calcbench `algo/queue` in callers that use both; `taskq` is
   an abbreviation.
5. `txn` · `tx` · `tx` is the variable name every caller already uses for the transaction.
6. `store` (Repository[K, V]) · `repository` · one short word, the standard-library style.
7. `logging` · `slogx` · `logging` says what it is for; the x-suffix is for transport layers here (httpx, grpcx).
8. `singleflight` · `flight` · familiar to every Go developer and it replaces x/sync's; a caller never imports both.
9. `netx` TCP serve loop (go_80): leave it out · add a `netx` layer beside httpx · the layer table has no raw-socket
   layer and no project needs one today.
10. Must: do not ship · `panics.Must[T]` · CLAUDE.md "a library function never panics on input"; stdlib keeps
    Must per package (`regexp.MustCompile`).
11. Timeout: an option of `retry` (`WithAttemptTimeout`) plus `context.WithTimeoutCause` · a `timeout` package ·
    a one-function package that wraps `WithTimeoutCause`.
12. Cron next-fire time: request it from calcbench (date arithmetic) · write it in `schedule` with `time` only ·
    CLAUDE.md sends missing math to calcbench; if Devin rules calendar logic is not math, `schedule` keeps it.
13. Core dependencies beyond the standard library and calcbench: NONE proposed. YAML (go_31, go_72) and every
    driver live in adapters, each needing Devin's yes at its wave.
14. Check before wave 2: whether the current golang.org/x/sync errgroup re-panics from `Wait` (R3 note). `group`
    returns `ErrPanic` either way; only the comparison test in `compare/` changes.

## Note (2026-11-13, align run)

Both reading lists (this one and `doc/ladder2_2026-11-13/LADDER2.md`) were cross-referenced against the code as
built in `doc/align_2026-11-13/MATRIX.md` (every pattern once by measured REPEAT; lists (a) delivered, (b) build,
(c) missing, (d) one-offs). What that run built is in `doc/CHANGELOG.md` (align); what it deferred and where each
item lands is `doc/align_2026-11-13/DEFERRED.md`; new calcbench requests R-6..R-9 are in
`doc/align_2026-11-13/calcbench_requests.md`. Waves 7+ of section 6 stay as planned.
