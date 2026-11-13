# Brief for the ladder readers (2026-11-13)

Request: `doc/requests/foundation_2026-11-13.md`, Track B. Read `.claude/CLAUDE.md` and `.claude/rules/*.md`
first; they define what toolbench is. This run is READ-ONLY research: the only file a reader writes is its own
`doc/ladder_2026-11-13/<R#>.md`.

## The two rules

1. **github.com/code-heim is the true north for ORGANISING**: it tells us which patterns matter and in what
   order. It is never copied and never cited. The repos are 1-3 years old (last pushes 2023-11 to 2025-04), so
   every pattern is RE-RESEARCHED against today's Go (go1.27 is installed and is the module's `go` line) and
   cited to its authority (`.claude/rules/sources.md`), to calcbench's standard.
2. **calcbench is used throughout** (`github.com/CrimsonCoder42/calcbench` v0.1.0, local copy
   `/Users/devinanderson/Desktop/calcbench`, read-only; its `INDEX.md` is the lookup: one row per ID). For
   every pattern, search calcbench's INDEX.md BY MEANING first: any data structure, algorithm or formula it
   already has is called by ID, not rewritten. Packages most likely to matter: `algo/seq` (Map, Filter, Take,
   Chunk, Zip, Flatten, Batch, Window: iter.Seq), `algo/queue` (CircularBuffer = ring buffer), `algo/heap`
   (Heap, TopK, MedianStream), `algo/list`, `algo/stack`, `algo/window`, `algo/stream`, `algo/ratelimit`,
   `algo/bloom` (BloomFilter, OptimalParams, FalsePositiveRate), `algo/hashing`, `algo/sample`, `num/stats`
   (means, percentiles), `num/prob`, `algo/reduce`. A pattern re-implements something calcbench has only when it
   cannot be done correctly otherwise (lock-free, channel-based, must be safe for concurrent use) and the row
   says why. Math calcbench lacks goes in the row as "MISSING: <what>".

## Hard limits

- Read code with Read/Grep/Glob/cat/sed. Never run the repos' code or tests, never `go mod download`, never
  `go build` inside `_reference/`. No git commands. No edits outside your own `<R#>.md`.
- Never open `.env`, `.env.*`, `app.env`, `accounts.json`, `vendor/`, `node_modules/`, `.redlite/` anywhere.
- USED TODAY greps are read-only, in `/Users/devinanderson/Desktop/advo_production/advomedix-v2/backendAdvo`
  (exclude vendor/ and any env file) and `/Users/devinanderson/Desktop/red_lite_cache` (exclude .redlite/).
  Use Grep with a glob of `*.go`.
- No background processes, no `sleep`.
- **Time cap: 25 minutes.** If you are not done, write what you have and end the file with
  `REMAINING: <repo names>` so the lead can relaunch on them.
- Read the code, never guess from the repo name. A repo can yield several patterns (one row each) or none
  (one SKIP row saying why).

## Row format (one markdown table, these 13 columns, in this order)

`repo | pattern (textbook name) | what the repo does, one line | LAYER | TIER | builds on | TODAY'S GO | authority | proposed package and API | CALCBENCH | compare against | licence | USED TODAY`

- **LAYER**: `core` / `httpx` / `grpcx` / `adapters/<driver>` / `FRAMEWORK` (belongs to the later net/http
  server framework: routing, templates, auth flows, htmx...) / `SKIP` (library usage only, say why).
- **TIER**: T0..T6, below. The lowest tier whose dependencies it has.
- **builds on**: other patterns it needs first, by textbook name, or `-`.
- **TODAY'S GO**: what the current standard library and golang.org/x give for this, checked against the Go
  release notes on go.dev (https://go.dev/doc/devel/release and go.dev/doc/go1.NN for every release since the
  repo was last pushed: iter, slices, maps, errors.Join, context.WithCancelCause/AfterFunc/WithoutCancel,
  sync.OnceFunc/OnceValue, sync.WaitGroup.Go, log/slog, net/http method+wildcard routing, testing/synctest,
  min/max builtins, range-over-int, range-over-func, weak, unique, os.Root, etc.) and what in the repo is
  therefore outdated. Fetch the release notes once and reuse them; do not fetch per row.
- **authority**: to the chapter or section, per `.claude/rules/sources.md` (Go Blog article and heading,
  pkg.go.dev symbol, Cox-Buday chapter, Hohpe & Woolf pattern page, Nygard chapter, Azure pattern page...).
- **proposed package and API**: package name per `.claude/rules/go-functions.md` (one short lowercase word,
  never a stdlib name a caller imports beside ours), plus 1-4 Go signatures: ctx first on anything that blocks,
  directional channels, error last, functional options `With<Thing>`, generics not `any`.
- **CALCBENCH**: every calcbench ID it should call (`ALG-SEQ-004 Chunk`), or `none`, or
  `re-implemented because <reason>`, plus `MISSING: <what>` when calcbench lacks needed math.
- **compare against**: the library it replaces, by module path (`golang.org/x/sync/errgroup`,
  `github.com/sourcegraph/conc`...), or `none`.
- **licence**: the repo's licence (from its LICENSE file).
- **USED TODAY**: up to three `file:line` per project where Devin hand-rolls it, or `not found`.

Escape a literal `|` inside a cell as `\|`.

## Tiers (lowest first)

- **T0 FOUNDATION**: what every other pattern leans on (panic-to-error, error joining, functional options,
  clock, context helpers, must, first-class function helpers).
- **T1 BASE CONCURRENCY**: goroutine and channel primitives (generator, pipeline stage, fan-out, fan-in/merge,
  or-done, tee, bridge, buffered vs unbuffered semantics, mutex/confinement, errgroup, semaphore, sync.Pool).
- **T2 COMPOSED CONCURRENCY**: worker pool, bounded parallel map/filter (the concurrent twins of calcbench
  algo/seq), batching, background work, task scheduling, in-process pub/sub.
- **T3 RESILIENCE**: retry, backoff with jitter, timeout, rate limit, circuit breaker, bulkhead, singleflight.
- **T4 STATE AND STRUCTURE**: cache (TTL, cache-aside), lock with lease, bloom filter use, builder, decorator,
  dependency injection, config layering, structured logging.
- **T5 INTEGRATION INTERFACES**: publisher/consumer, queue with retry and dead-letter, repository,
  transaction, pagination, outbox, idempotency key (core interfaces only).
- **T6 TRANSPORT AND DRIVERS**: httpx, grpcx, adapters/<driver>.

## The split

| Reader | Repos (under `/Users/devinanderson/Desktop/toolbench/_reference/code-heim/`) | Output |
|---|---|---|
| R1 | socket_io_emitter, go_2 .. go_25 (22 repos) | `R1.md` |
| R2 | go_26 .. go_49 (23 repos) | `R2.md` |
| R3 | go_50 .. go_70 (21 repos) | `R3.md` |
| R4 | go_71 .. go_85 (14 repos) | `R4.md` |

## Output file shape

```
# R<n>: <repos>
Read: <n repos> · Rows: <n> · Time: <start-end>

| repo | pattern | ... 13 columns ... |
|---|---|...|

## Notes
Anything the lead needs for the merge: standard patterns you noticed missing from code-heim, outdated idioms
(one line each), calcbench gaps.
REMAINING: <repo names, or none>
```
