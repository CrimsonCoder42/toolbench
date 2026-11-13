# LADDER2: what the AkhilSharma90 reading list adds to toolbench (2026-11-13)

Merged by the lead from A1-A4 (155 reader rows over 92 repos). Same rules as `doc/ladder_2026-11-13/LADDER.md`:
the repos tell us WHICH patterns matter; nothing is copied and nothing cites them. Every TODAY'S GO cell was
written against `GO_RELEASES.md` (go1.27). This file is a PROPOSAL: nothing here changes `INDEX.md`,
`doc/taxonomy.md`, `.claude/` or any package until Devin approves (section 9). Every `file:line` below is a
repo path under `_reference/AkhilSharma90/<repo>/`, cited so the reader can open it, never to adapt it.

Where code-heim was one lesson per repo, this account is mostly whole applications (CRUD APIs over six
routers, blockchains, scrapers, bots, LLM clients, two microservice sets, a 600-file course). The yield is
therefore different: few new patterns, many concrete failure modes for patterns toolbench already has or
plans, and a clear map of the specialist packages that sit above the foundation.

## 1. Counts

| what | count |
|---|---|
| repos listed (API + profile listing) | 192 |
| Go: language Go 79, name/description says Go 19 | 98 |
| dropped before clone: forks 9, not Go 85 | 94 |
| cloned (`--depth 1`, 0 failed, 0 oversize) | 98 |
| dropped at clone (no `*.go`) | 6 |
| read by A1-A4 | 92 |
| rows | 155 |

Rows per VERDICT: COVERED 42 · PLANNED 44 · NEW-GENERAL 2 · SPECIALIST 16 · FRAMEWORK 11 · SKIP 40.
Per reader: A1 39 (8/9/2/4/2/14+1 shifted by a `\|` in a path cell), A2 45 (13/17/0/7/2/6), A3 28 (4/5/0/2/4/13),
A4 42 (17/13/0/3/3/6). Licences: 13 MIT, 1 Apache-2.0 (go-full-text-search), 78 none declared.

## 2. NEW-GENERAL (merged across repos, by tier)

Two patterns the readers found that toolbench neither has nor plans, plus one borderline the lead adds from
two COVERED rows (2.3). Each: authority, package, API in the shape of `.claude/rules/go-functions.md`,
calcbench, compare-against, repos, and whether it fits an existing package.

### 2.1 Keyed mutex (lock striping per key) · T1 · fits existing `guard`

- Shown by: A-Database-Built-With-Golang main.go:173-185 (`getOrCreateMutex` per collection, unbounded map,
  readers take no lock). The same need appears as check-then-act races in Golang-JWT-Gin-Authentication-project
  controllers/userController.go:62-80 (count then insert) and Golang-Serverless-Project pkg/user/user.go:83-99
  (get then put): an in-process keyed lock is the first fix, a unique index or conditional write the real one.
- Authority: Cox-Buday, Concurrency in Go, 2017, ch.3 "The sync Package: Mutex and RWMutex"; The Go Memory
  Model, "Locks"; pkg.go.dev `sync.Mutex` ("TryLock", go1.18).
- Package: `guard` (next IDs TB-GUARD-003..). Alternative: an in-memory `Locker` inside planned `lock` (T4);
  it lost because `lock` carries lease, TTL and fencing semantics that an in-process lock does not need, and
  `guard` is where "a mutex beside the value" already lives (decision 9.2).
- API:
  ```go
  // Keyed serialises callers per key: Do(ctx, k, fn) runs fn while no other
  // Do with the same key runs; distinct keys never wait on each other.
  // The zero value is ready to use. Idle entries are dropped on Unlock
  // (no table growth); WithMaxKeys bounds the table (ErrKeys when full).
  type Keyed[K comparable] struct{ /* mu beside map[K]*entry */ }
  func NewKeyed[K comparable](opts ...KeyedOption) *Keyed[K]
  func (g *Keyed[K]) Do(ctx context.Context, key K, fn func() error) error   // waits for the key or ctx
  func (g *Keyed[K]) TryDo(key K, fn func() error) (ran bool, err error)     // fail fast
  func WithMaxKeys(n int) KeyedOption
  ```
- calcbench: none (the table is a map under one mutex; entries are reference-counted and removed at zero, so
  no eviction structure is needed).
- Compare against: github.com/puzpuzpuz/xsync (`MapOf`-based striping), golang.org/x/sync/singleflight (the
  keyed-table shape); none implements exactly this.
- Tests from the repo's defects: distinct keys run concurrently; same key serialises; table empty after every
  Do returns; ctx cancel while waiting returns ctx error and leaves no entry; `-race` with N writers.

### 2.2 Atomic file replace (write temp, fsync, rename) · T2 · needs a home (decision 9.1)

- Shown by: A-Database-Built-With-Golang main.go:80-98 (temp + `os.Rename`, no `Sync`, fixed `.tmp` name,
  `resource` joined into the path unescaped: `../` traversal), and by omission in every repo that writes files
  in place (go-file-encrypt filecrypt.go:54 overwrites the source; GenAI-Code-Committer savekey.go:37).
- Authority: pkg.go.dev `os.Root` (go1.24; confines every path under one directory), `os.CreateTemp`,
  `os.Rename`, `File.Sync`; IEEE Std 1003.1-2024, `rename()` ("the new name shall be atomically replaced").
- Package: recommended inside planned `blob` as the local-filesystem `Store` whose `Put` is atomic, with the
  helper exported there (wave 9). Alternative: a new core package `atomicfile` (the shape of
  github.com/natefinch/atomic). It lost under decision 9.11's precedent: a one-function package. Devin decides.
- API:
  ```go
  // WriteAtomic writes r to name under root so that readers see either the
  // old file or the complete new one, never a partial write: temp file in
  // the same directory, Sync, Rename, directory Sync; the temp file is
  // removed on any error. name must not escape root (os.Root enforces it).
  func WriteAtomic(ctx context.Context, root *os.Root, name string, r io.Reader, perm os.FileMode) error
  ```
- calcbench: none.
- Compare against: github.com/natefinch/atomic, github.com/google/renameio/v2.
- Tests: crash-after-write simulation leaves the old content; temp removed on write error; a name with a
  separator or `..` is rejected (`os.Root` error); existing permissions preserved or `perm` applied, documented.

### 2.3 Fault-injection decorator (saboteur test double) · T2 · borderline; fits `testing/` helpers

- Shown by: GO-Fafka-Grafana-Prometheus-Watermill main.go:97-101, :113-126 (`RandomFail(p)`, `RandomPanic(p)`
  over a publisher) and Kafka-GO-Real-Time-Feed consumer/main.go:44-75 (same middlewares). Not a runtime
  pattern: a test helper that makes retry, breaker, bulkhead, dead-letter and recover tests deterministic.
- Authority: Meszaros, xUnit Test Patterns, 2007, "Test Stub" (variation "Saboteur"); Nygard, Release It!,
  2nd ed., 2018, ch.17 "Chaos Engineering".
- Package: toolbench's `testing/` (no IDs today: decision 9.5 on whether helpers carry IDs). API:
  ```go
  // Saboteur wraps fn: every call fails with err when fail(n) is true for
  // the call's ordinal n, panics when boom(n) is true; otherwise calls fn.
  func Saboteur[In, Out any](fn func(context.Context, In) (Out, error), fail func(n int) bool, err error, boom func(n int) bool) func(context.Context, In) (Out, error)
  ```
- calcbench: none (deterministic schedules, no random; `flow.WithRand`-style injection if randomness is wanted).
- Compare against: github.com/ThreeDotsLabs/watermill `middleware.RandomFail`; none in stdlib.

Every other "new" thing the readers flagged is an option, an edge case or a test for a package that exists
(section 3) or is planned (section 4).

## 3. Improvements to what exists (per built package; ID; one line each)

**panics / httpx.Recover (TB-PANIC-002, planned Recover)**: never re-panic inside the deferred recover
(CSRF-Golang-Killer-Project middleware.go:24 calls `log.Panic` there, so the 500 is never written); write 500
only when nothing was written yet; report through a hook.

**group (TB-GROUP-001/003)**: test that collects ALL errors with `errors.Join` versus first-error-cancels,
documented per entry point (AI-Code-Bundler reading.go:107-110 drops all but the first); result order kept
(LLM-Projects thinktank.go:47-49 loses it); add a doc line: a WaitGroup passed by value or `Add` inside the
goroutine is the misuse `WaitGroup.Go` (go1.25) and vet copylocks remove (monolith intraprocess/orders.go:38-40,
bootcamp advanced.go:36, waitgroups/error.go:8).

**flow (TB-FLOW-001..008)**: doc states who closes each channel ("never close a channel the producer may still
write to", BSIDES tui/run.go:47-49; monolith main.go:50 closes under a live sender); an Example of
"interrupt a stream mid-flight and emit a last item" via ctx (go-openai-terminal-assistant engine.go:104-134);
a test that an early-exiting consumer leaves no stage goroutine (sitemap-crawler, guardian-scraper).

**semaphore (TB-SEMA-001)**: Example showing the token held only around the network call, released before
parsing (guardian-scraper main.go:85-87); leak check on early return.

**guard (TB-GUARD-001/002)**: `Guarded[map[K]V]` and `Guarded[[]T]` Examples under `-race` with N writers; 13
repos write a package-level map or slice from handler goroutines with no lock (A-Blockchain main.go:74,
go-chi-CRUD article.go:46, Complete_GO_Blockchain server.go:314, CSRF db.go:21, sitemap-crawler main.go:126,
go-echo/fiber/gorilla/gin-CRUD, GO-Loadbalancer main.go:72, langchain-GO vacation.go:69, Videocall server.go:62,
GO-Proof-Of-Stake main.go:177, monolith memory.go:13); a reviewer grep for `^var .* = make(map` in handler
packages is worth a line in `.claude/rules`.

**recycle (TB-RECYC-004)**: pooled objects without reset (bootcamp sync_pool.go:9) is the defect `WithReset`
exists for; keep as the doc's motivating example.

**pool (TB-POOL-001..006)**: `Wait`/`Close` returns only after every worker exited (Git-Secrets-Scanner
task.go:118-157 polls with `time.Sleep` and returns early); Submit after Close returns `ErrClosed`; Submit from
INSIDE a worker with a full queue must not deadlock (crawler frontier shape, any-website-sitemap-crawler
main.go:62-92); candidate option `WithOnDone(func(done, total int))` progress hook (task.go:24, progressbar.go:58),
the pool twin of `bulkhead.WithOnEvent` (TB-BULK-011).

**parallel (TB-PAR-001/003)**: input order kept and all errors reported, versus the token-channel + shared
slice rewrite seen in three repos (AI-Code-Bundler reading.go:74, sitemap-crawler main.go:111, scanner task.go:83).

**batch (TB-BATCH-002)**: `time.After` inside a select loop allocates a timer per iteration (bootcamp
advanced.go:62): our one-timer implementation is the doc's contrast; no code change.

**schedule (TB-SCHED-002/004/005)**: Heartbeat variant that beats while `func(ctx) error` runs and stops when
it returns or panics (go-openai-discord-bot message_handler.go:190-208 leaks on panic); test the goroutine exits
on ctx cancel; doc note that `time.Second / n` truncates for n > 1000 (Kafka producer main.go:67).

**lifecycle (TB-LIFE-002/004/006/008)**: Shutdown must get a FRESH deadline context, never the already-cancelled
one (GO-CQRS echo-server.go:42) or `context.Background()` (GO-Kafka-MongoDB server.go:195, golang-todo); stop
ORDER test: consumers before the store, "stop accepting, drain, then close the channel handlers publish to"
(monolith main.go:46-50); a start error unblocks Run before any signal (golang-todo main.go:209-213); `Stop`
called exactly once per component even when N goroutines report (GO-CQRS workers_runner.go:33); second signal
forces exit (monolith signals.go). [CORRECTED 2026-11-13 by doc/align_2026-11-13/MATRIX.md: monolith signals.go swallows the second signal; no reference implements force-exit]

**pubsub (TB-PUBSUB-003/006/007)**: a "drop the slow subscriber" policy beside drop-oldest (LLM-Projects
hub.go:31-52, Videocall hub.go); an `OnDelivered(topic, dur, err)` observer hook, the general form of the
per-handler metrics in GO-Fafka main.go:90-95; a Message envelope with `Headers map[string]string` so adapters
propagate correlation and trace ids without a tracing dependency in core (GO-CQRS otel/utils.go:38-47,
Kafka-GO-Real-Time-Feed consumer/main.go:68-70); test: one publish reaches every subscriber exactly once
(GO-Proof-Of-Stake main.go:140-142 sends once per validator to whoever reads).

**retry (TB-RETRY-001/012/015)**: option `WithMaxElapsed(d)` (total cap beside attempts; GO-CQRS rabbitmq.go:31-47
combines both); a decision on "retry forever at startup" (go-grpc-graphql-microservices main.go:24-30,
monolith wait.go:9-22: both uncancellable, no jitter): recommend NO, bounded attempts + elapsed cap, then exit
non-zero (9.6); Example for `WithRetryIf` classifying a 429 with `errors.AsType` (kubernetes-assistant-go
completion.go:101-138, 10 uncapped doublings from 1 s, no jitter); ctx cancel stops a long retry; a Retry-After
hint is the planned `httpx.RetryTransport`'s, not core's; error text must never carry a DSN (GO-CQRS db.go:42).

**ratelimit (TB-RATE-005/008/009)**: [CORRECTED 2026-11-13 by doc/align_2026-11-13/MATRIX.md: there is no `Keyed.Run`; eviction is inline on each call, D-031] doc that idle eviction is inline
(go-ratelimit per-client-rate-limit/main.go:28-40 starts an unstoppable one per construction); test MaxKeys under
a burst of distinct keys and idle eviction via the fake clock; doc example "per-worker rate versus global rate"
(Kafka producer: N tickers = N x rate); the distributed keyed limiter is `adapters/redis` with ONE atomic
INCR+EXPIRE script (Redis-GO-URL-Shortener shorten.go:45-60 does GET then DECR).

**breaker / bulkhead**: no gaps found; both are exercised by the saboteur helper (2.3).

**cache (TB-CACHE-002/011/015)**: concurrent Get/Set `-race` test (go-openai-discord-bot cache.go:8 shares a bare
map); doc that cache errors are non-fatal fall-through (`ErrCache`, matches GO-Kafka-MongoDB usecase.go:64-86);
`WithKeyPrefix` belongs to `adapters/redis` (redis_repository.go:68); a miss without singleflight is the
stampede `Aside` prevents (keep as the doc's example).

**config (TB-CONFIG-006/012/013/016/018)**: a bad integer must surface as `ErrParse`, never become 0
(BulletProof config.go:27-28); `Required` turns silent empty API keys into one startup error (LLM-Projects
config.go:24-28); `Values` must redact keys named Token (go-simple-discordbot config.go:31 prints the bot token);
doc that the caller passes `os.LookupEnv` and nothing reads env at package init (7 repos dial a DB in `init`
with `log.Fatal`: JWT, restaurant, calorie tracker, React TODO, MySQL bookstore, golang-todo, go-twilio-verify).

**logging (TB-LOG-004)**: `WithPatterns` should ship patterns for Slack (`xoxb-`, `xapp-`), bearer tokens and
DSN passwords (GO-Slackbot-Calculates-Age main.go:25-26 and GO-SlackBot-Uploads-File main.go:12 hold live-looking
tokens; GO-Kafka-MongoDB interceptors/manager.go:45 logs auth metadata).

**clock, ctxkey, singleflight**: nothing new; ctxkey is confirmed by GO-CQRS correlation_id_middleware.go:20
(a string key, what `context.WithValue` forbids).

## 4. Additions to the plan (per planned package, waves 7+)

**lock (T4)**: in-process `guard.Keyed` (2.1) is the T1 twin; the distributed claim must be SETNX / conditional
write, never GET-then-SET (Redis-GO-URL-Shortener shorten.go:97-107).

**txn (T5)**: tests from go-grpc-graphql-microservices order/repository.go:36-78: Commit error returned,
`stmt.Close` on early return, rollback on panic (`panics.ErrPanic`), ctx cancel rolls back; the closure shape
`Update(func(tx) error)` (bbolt, Complete_GO_Blockchain blockchain.go:50) is the familiar one `txn.Run` mirrors;
a multi-document write with no transaction is the motivating case (restaurant orderItemsController.go:194-240).

**page (T5)**: `Size` clamped to `[1, MaxSize]`, `Number < 1` becomes 1, size 0 rejected, empty result is an
empty page not a panic (JWT userController.go:160-183, GO-Kafka-MongoDB pagination.go:33,:96); `HasMore` exact
(not integer division); `Count` via NUM-INTEG-013 CeilDiv (both pagination.go files use float ceil); order and
filter fields whitelisted, never interpolated (GO-CQRS db.go:147-155 Sprintf into SQL); a CONSUMER-side
`page.All(ctx, fetch func(ctx, cursor) (Page[T], error)) iter.Seq2[T, error]` that walks NextPage until empty
(Git-Secrets-Scanner scm/github.go:48-64); `Page[T]` carries `HasMore` beside `Total`.

**store (T5)**: sentinels `ErrNotFound` AND `ErrExists` (go-gin-CRUD repository.go:41-66 makes per-call errors);
`GetMany(ctx, keys)` with documented missing-key behaviour (go-grpc-graphql catalog/repository.go:87-146); a
per-adapter "translate driver error to sentinel" seam (Mini-LocalAI-Clone client.go:53-55 is an identity stub);
keep `Health`/`Ping` OFF the Repository interface, a separate `Pinger` (BulletProof adapter.go:16); the in-memory
implementation has a mutex (monolith memory.go:13-33; go-echo-CRUD main.go:105); doc: `*sql.DB` is opened once per
process (postgres-with-go handlers.go:186-318 opens and closes one per query); `ReadAll` is `iter.Seq2`
(A-Database main.go:146).

**jobs (T5)**: the at-least-once sequence fetch -> decode -> validate -> retry(handler) -> dead-letter -> commit
(GO-Kafka-MongoDB workers.go:20-92) and its failure modes as tests: undecodable messages go to an INVALID
channel and are committed (never re-fetched forever, workers.go:50-54), one worker's error does not cancel the
group (workers.go:29), exactly N workers (workers.go:110 starts N+1); dead-letter sits OUTSIDE retry in the
middleware order (Kafka-GO-Real-Time-Feed consumer/main.go:44-75); three options from three brokers: an ACK
DEADLINE after which the job is redelivered (NATS AckWait), a MAX-DELIVER count before dead-letter (NATS
MaxDeliver; torrent p2p.go:160-163 re-enqueues with no cap and loops forever), a MAX-IN-FLIGHT bound separate
from worker count (NATS MaxAckPending, RabbitMQ Qos prefetch, torrent MaxBacklog=5); a job STATUS visible to a
poller with stored result and failure (asynchronous request-reply, langchain-GO vacation.go:69-146); a per-job
deadline separate from the attempt timeout; at-least-once (ack after work) versus at-most-once (auto-ack) as an
option (RabbitMQ worker.go:40-48).

**outbox (T5)**: the dual write confirmed (GO-CQRS create_product_handler.go:44-55): event row in the same txn,
relay at-least-once with an id, so `idempotency` on the consumer side.

**idempotency (T5)**: the check-then-act signups (JWT, Serverless) are the use case; the store adapter does it
with a unique index or a conditional write, the core `Guard` with `lock`.

**blob (T5)**: `WriteAtomic` (2.2) as the local Store's `Put`.

**mail (T5)**: nothing new.

**httpx (T6)**: `Chain` documents and tests one canonical order (recover outermost, request id, real IP, log,
timeout; the three repos each differ: BulletProof routes.go:33-40, CSRF middleware.go:16, GO-Kafka-MongoDB
http.go:48-67); `RequestID` middleware (reuse inbound header, generate otherwise, echo on the response, stash
via `ctxkey`; GO-CQRS correlation_id_middleware.go:9-25) is an ADDITION to the API list; `Serve` sets
ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout, MaxHeaderBytes by default (GO-proof-of-work main.go:70,
golang-todo main.go:200, both lack ReadHeaderTimeout) and `Shutdown` with a fresh deadline; `Recover` never
re-panics; `WriteProblem` carries an application `code` field (monolith error.go:9-36) and never echoes
`err.Error()` on 500; `StatusOf` maps 409 for exists, 503 for a failed dependency, never 500 for a missing token
(three repos); `MaxBytes` on every JSON body; `RemoteIP` honours X-Forwarded-For only from trusted prefixes
(go-ratelimit tollbooth/main.go:36-40 trusts headers by default); `RateLimit` sends Retry-After and Content-Type
and 429 not 503 (go-ratelimit token-bucket/limit.go:10-26, Redis-GO-URL-Shortener shorten.go:55); `Health`
splits liveness from readiness, bounds the probe by a deadline, caches it, answers 503 (BulletProof
health.go:22-29 lists tables on every GET), accepts `func(ctx) error` checks with a `TCPCheck(addr)` helper
(go-health-checker check.go:9-22 leaks the conn); `DoJSON` checks status before decode, returns a typed status
error with a BOUNDED body excerpt (`errors.AsType[*StatusError]`), takes ctx, never reads env or writes stderr,
leaves Accept-Encoding to the Transport, allows per-request options (go-weather-tracker main.go:41-59,
go-whisper-api client.go:105-187, go-terraform-assistant-ai gpt3.go:276-320, Consuming-Pexels main.go:143-159:
body closed before the error check, AI-Code-Bundler review.go:50); `RetryTransport` retries idempotent methods
only, reads Retry-After, jitters (GO-CQRS http_client.go:11-43 retries POST with a fixed 300 ms), and ships the
transport-timeout defaults (dial, TLS handshake, response header, idle, MaxConnsPerHost) as documented
constants; handlers derive from `r.Context()`, never `context.WithTimeout(Background, 100s)` (JWT, restaurant,
calorie tracker); doc: handlers return errors, never `log.Fatal` (four repos).

**grpcx (T6)**: status table from the grpc.io mapping, no string matching, AlreadyExists -> 409, Canceled ->
499-class not 408 (GO-Kafka-MongoDB grpc_errors.go:23-70, http_errors.go:53-123); interceptor order (tags ->
tracing -> metrics -> recover -> log; interceptors/manager.go:35-48); keepalive ServerParameters as documented
defaults (both microservice repos); `GracefulStop` first, `Stop` after the deadline (GO-CQRS grpc-server.go:113);
stream handler stops on `stream.Context()` (GO-grpc-demo server_stream.go:20).

**adapters/redis**: keyed limiter in one atomic script (ALG-RATELM-003 window maths), SETNX claim, `WithKeyPrefix`,
one client per process not per request (Redis-GO-URL-Shortener shorten.go:45,:94).
**adapters/rabbitmq**: prefetch as the in-flight bound, manual ack after work, `PublishWithContext` with a
deadline, Nack/requeue path, `ok` on channel receive (monolith amqp/orders.go:81-91 spins on a closed channel).
**adapters/kafka**: producer acks/retry knobs exposed (GO-Kafka-Example producer.go:28-43), consumer group with
commit after handling, `IBM/sarama` not `Shopify/sarama`, or kafka-go / franz-go (decision at its wave).
**adapters/websocket**: read limit, pongWait with pingPeriod = 9/10 pongWait, writeWait, coalesced writes as
options with those defaults (Videocall client.go:11-99); origin check kept.
**adapters/lambda**: both forms, Function-URL-to-`http.Handler` and typed `func(ctx, E) (R, error)`; the Lambda
deadline reaches the handler ctx; 5xx for driver errors (Golang-Serverless-Project handlers.go:26-33 answers 400
for outages); API Gateway proxy response needs base64 for binary bodies.
**adapters/pgx**: `BeginFunc` is the compare target for `txn`; advisory lock for `lock`.

## 5. Specialists (the map for later; nothing is built)

| specialist package | what it would hold | repos | built on (toolbench) |
|---|---|---|---|
| `llm` client | provider clients (OpenAI, Anthropic, Gemini, Ollama, Groq) behind one tiny `Client`; streaming as `iter.Seq2[[]byte, error]`; SSE "data:" reader; mock provider; token-budget truncation; conversation memory; tool-call loop | BSIDES-Kerala-Workshop-Demo, LLM-Projects (15), go-terraform-assistant-ai, kubernetes-assistant-go, GenAI-Code-Committer, GO-Chat-GPT-*, go-whisper-api, go-telegram-gpt, go-openai-*, langchain-GO, Mini-LocalAI-Clone | httpx.DoJSON/RetryTransport, retry, ratelimit, cache (Aside), flow (ToSeq), jobs (async request-reply) |
| `crawl` | frontier with visited set, per-host politeness, robots, user-agent rotation, proxy client, sitemap walk | any-website-sitemap-crawler, sitemap-crawler, guardian-scraper, Golang-Bing.com-Scraper, google-scraper, golang-goquery-crawler, g-stock-scraper | pool (submit-from-worker), semaphore, ratelimit.Keyed (per host), retry (429/403), httpx.DoJSON |
| `auth` (framework side) | bearer/JWT middleware with algorithm pinning, refresh-token rotation and revocation store, CSRF (or `http.CrossOriginProtection`), password hashing, role check | CSRF-Golang-Killer-Project, Golang-JWT-Gin-Authentication-project, golang-restaurant-management-backend, simple-microservice-with-go, Mantil-trial-project, GO-CQRS (validate_token) | httpx.Chain/WriteProblem, store (revocation), cache (TTL), idempotency, ctxkey |
| `blockchain` | hash chain, proof of work (cancellable), stake lottery, Merkle tree, Base58, UTXO/wallet, gossip node | A-Blockchain-With-Golang, Complete_GO_Blockchain, GO-P2P-Blockchain, GO-Proof-Of-Stake-Blockchain, GO-proof-of-work-blockchain, Merkle-tree-go | guard, pubsub, schedule, lifecycle, txn; calcbench ALG-SAMPLE-003/004 + requests in section 7 |
| `search` | tokenizer, stop words, stemming, inverted index, sorted-postings intersection | go-full-text-search | calcbench ALG-HASH-006 (or ALG-INTVL-003, section 7); batch.Run for ingest |
| `filecrypt` | AES-GCM file encryption with a KDF, streaming, atomic output | go-file-encrypt, simple-encryption-decryption-GO | blob.WriteAtomic (2.2) |
| `proxy` / load balancer | reverse proxy over `httputil.ReverseProxy`, round-robin and weighted backend pick, health-aware, Forwarded headers | GO-Loadbalancer | httpx.Health, breaker (per backend), ratelimit; calcbench ALG-SAMPLE-003/004 |
| `operator` (Kubernetes) | reconcile loop under controller-runtime | Devops-GO (ping-operator), kubernetes-assistant-go | retry, schedule (requeue is the framework's) |
| `cli` / `tui` | bubbletea shells, spinner, input history (ring buffer), `Run(args) int` with injected streams, config dir + 0600 key file | go-openai-terminal-assistant, Mini-LocalAI-Clone, BSIDES tui, google-text-to-speech, GenAI-Code-Committer, GO-Chat-GPT-CLI | config.FromFlags, lifecycle (signal), calcbench ALG-QUEUE-002 |
| `sse` | server-sent events reader/writer (WHATWG event-stream parsing) | go-terraform-assistant-ai gpt3.go:141-198 | flow.ToSeq, httpx |
| `secretscan` | regex pattern tables over files and image layers, baseline/ignore sets | Git-Secrets-Scanner, Devops-GO imageinfo | pool (+WithOnDone), page.All, calcbench ALG-HASH-006/007/008 |

## 6. FRAMEWORK and SKIP

FRAMEWORK (the later net/http server framework), one line each:
- WebSocket hubs: GO-React-Fullstack-Chat pool.go:21-49 (CheckOrigin true; one write error kills the hub),
  LLM-Projects hard-resume-agent hub.go:31-52, Videocall-Chat-GO-Project hub.go (never stops); the per-connection
  pump is `adapters/websocket`.
- JWT bearer middleware: CSRF-Golang-Killer-Project, Golang-JWT-Gin-Authentication-project (500 for a missing
  token, nil-err `err.Error()` panic), golang-restaurant-management-backend, simple-microservice-with-go
  (nil token dereferenced), Mantil-trial-project (secret "secret"); all on archived dgrijalva/jwt-go.
- Refresh-token rotation with a server-side jti store and CSRF double-submit: CSRF-Golang-Killer-Project.
- Password hashing and check-then-act signup: Golang-JWT-Gin-Authentication-project userController.go:25-80.
- CQRS wiring (mediator, fx, otel span per request, problem-details library): GO-CQRS-Microservices.
- Chain-of-responsibility handler slice with `Next()`: go-openai-discord-bot context.go:58-66 (a handler
  calling Next twice silently does nothing).
- TCP accept loop with gob command framing: Complete_GO_Blockchain core/server.go:424-446 (unbounded ReadAll,
  no deadlines; LADDER decision 9.9).
- Per-handler JSON envelope and CORS set by hand: BulletProof response.go, Golang-React-Fullstack-TODO.
- Content/Lambda event routing by method switch: Golang-Serverless-Project (becomes `adapters/lambda` + ServeMux).

SKIP (library usage or application code with nothing reusable), one line each:
- CRUD over a global map or slice with no lock: go-simple-CRUD, go-chi-CRUD, go-echo-CRUD, go-fiber-CRUD,
  go-gorilla-mux-CRUD, Golang-MongoDB-CRUD-API (mgo), golang-react-calorie-tracker-app, Golang-React-Fullstack-TODO.
- GORM usage (LADDER §8): go-beginner-CRM-project, go-postgres-gorm, Golang-MySQL-CRUD-Bookstore-Management-API
  (credentials in source), go-beginner-fiber-HRMS-project (mongo).
- Bots and API clients that only call a library: AI-Bot-GOlang-Wit.ai-Wolfram-Slack, go-simple-slackbot,
  GO-Slackbot-Calculates-Age, GO-SlackBot-Uploads-File, go-simple-discordbot (config row COVERED), go-telegram-gpt,
  go-twilio-verify, GO-Chat-GPT-Basic, GO-Chat-GPT-CLI, GO-Chat-GPT-Intermediate, google-text-to-speech.
- Scraping with colly/goquery only: g-stock-scraper, golang-goquery-crawler.
- Duplicates: kubernetes-ai-test (= kubernetes-assistant-go), Serverless-GO-In-5Mins (= Mantil-trial-project).
- Mathematics that is calcbench's: Dijkstra-s-golang (ALG-GRAPH-012), A-Blockchain-With-Golang (hash chain),
  Git-Secrets-Scanner set algebra (ALG-HASH-006/007/008), simple-encryption-decryption-GO (toy cipher).
- Tests against a live server, GraphQL and gRPC demos: API-using-go-Echo-with-test, GO-GraphQL-MongoDB-CRUD-Project,
  GO-grpc-demo (the Recv-until-EOF loop is grpcx's doc example).
- One-file stdlib demos: simple-http-server-GO (404 for a wrong method), CSRF randomstrings.go
  (`crypto/rand.Text`), GenAI-Code-Committer savekey.go (0600 key file: a convention line, not a package).
- Course lessons that are sequential library usage: ULTIMATE-GO-Bootcamp-Course Networking,
  Slightly_Bigger_Projects, DNS, godolint, ssh-chat, log/time/flag/io/reflect/template; two generics files and two
  concurrency lessons do not compile (WaitGroup and Mutex passed by value).
- Devops-GO imageinfo scanner (docker SDK usage, pattern table as package globals).

## 7. Missing from calcbench (requests; nothing is built here)

First, the three LADDER requests are now PRESENT: exponential backoff with jitter (ALG-BACKOF-001 Exponential,
ALG-BACKOF-002 FullJitter, called by TB-RETRY-006..010), `CeilDiv` (NUM-INTEG-013, for `page.Count`), and cron
next-fire (`algo/cron`, called by TB-SCHED-012). New requests from this reading list:
1. Merkle root and inclusion (audit) proof over `[][]byte` with RFC 6962 §2.1 leaf/node prefixes and the odd-leaf
   rule, in `algo/tree` (Merkle-tree-go, Complete_GO_Blockchain merkle_tree.go). Blockchain-only; low priority.
2. Base58 encode/decode (Complete_GO_Blockchain utils/utils.go:31-70). Blockchain-only; low priority.
3. Sorted-postings intersection (merge of two ascending int slices, O(n+m)) for a search specialist
   (go-full-text-search utils/index.go:44-59). ALG-HASH-006 is the unsorted form; ALG-INTVL-003 IntersectLists
   ("two sorted, disjoint lists") may already be it: the search specialist's builder should check before asking.
Everything else maps to existing IDs: ALG-RATELM-001/003, ALG-LIST-015/018, ALG-SAMPLE-003/004, ALG-QUEUE-002,
ALG-SEQ-001/002, ALG-HASH-006/007/008, ALG-GRAPH-012, NUM-ALGEB-037 RoundDecimalsMode (restaurant
foodController.go:123-130 hand-rolls `toFixed`), NUM-PROB-059.

## 8. Agreement with code-heim

Both reading lists show (the strongest signal; all built or planned): worker pool and bounded parallelism (18 of
92 repos here), fan-out/fan-in and pipelines, confinement and mutex-beside-the-value, cache (LRU, TTL,
cache-aside), config from environment, graceful shutdown on a signal, retry with backoff, token-bucket and keyed
rate limiting, offset pagination (with the same float-ceil and no-max-size defects), repository, transaction
closure, publish-subscribe over Kafka and RabbitMQ, middleware chain with recover and logger, HTTP error bodies,
WebSocket hub (framework), JWT auth (framework), Lambda adapter, heartbeat and ticker loops, context tree,
sync.Pool, functional options, panic recovery. Code-heim shows them as one clean lesson each; this list shows
them inside applications with the production defects attached, which is why section 3 is the longer part.

Only this list shows: keyed mutex and atomic file replace (2.1, 2.2), the saboteur test double (2.3), jobs
semantics from three brokers (ack deadline, max-deliver, max-in-flight, invalid versus dead letter, async
request-reply with status), correlation/request-id propagation through HTTP and message headers,
consumer-side page iteration, health liveness/readiness split, gRPC keepalive and status-table details,
retry total-elapsed cap and the startup-retry question, load balancer, scatter-gather (covered by
`group.Results`), NATS and DynamoDB, WebSocket pump defaults, multi-provider LLM clients, crawlers.

Only code-heim shows: or-done, tee and bridge channels, singleflight, bloom guard, CSV/Excel batching, cron
scheduling, slog handler design, htmx, GORM internals, build tags, Faktory job queue, memcache, Elasticsearch.

## 9. Decisions for Devin (recommendation · alternative · why it lost)

1. Home of atomic file replace (2.2): inside planned `blob` as the local Store's `Put` plus an exported
   `WriteAtomic` · a new core package `atomicfile` · a one-function package (decision 9.11 precedent); if Devin
   wants it before wave 9, `atomicfile` is the name (one compound word, the shape every Go developer knows from
   natefinch/atomic; `fsx` loses because the x-suffix is for transport layers, LADDER 9.7).
2. Keyed mutex (2.1): `guard.Keyed` · an in-memory `Locker` in planned `lock` · lease, TTL and fencing are
   distributed-lock concerns an in-process lock does not have; `guard` is T1 and already holds Guarded/Confine.
3. `adapters/nats` (NATS-go-example): add beside rabbitmq/kafka ONLY when a project needs NATS · fold into a
   generic `adapters/watermill` · watermill would be a second messaging framework in the dependency tree.
4. `adapters/dynamodb` (BulletProof-DynamoDB-GO, Golang-Serverless-Project): not planned until a consumer
   needs it (aws-sdk-go v1 is end-of-support; v2 would be the dependency) · plan it now · no project uses it.
5. Saboteur test helper (2.3): in `testing/` without an ID, as the leak helper is today · an exported package
   with IDs · test helpers are not public API (additive-API rule applies only to exports consumers import).
6. Startup retry policy: bounded attempts plus `retry.WithMaxElapsed`, then exit non-zero, documented in
   `lifecycle` · allow `WithAttempts(0)` meaning forever · "Bounded" rule in CLAUDE.md; an unbounded retry hides
   a dead dependency from the orchestrator that would restart the process.
7. `httpx.RequestID` as an addition to the T6 API list (correlation id reuse/generate/echo/stash) · leave it
   to the framework · it is a decorator over `ctxkey` with no framework dependency and every access log needs it.
8. Core dependencies: NONE proposed. Every driver named here (NATS, DynamoDB, Redis, AMQP, Kafka, bbolt,
   websocket) stays in an adapter with its own go.mod and needs Devin's yes at its wave.
