# Request: the foundation of toolbench + the pattern ladder (2026-11-13)

EDITS CODE (the module's skeleton and its index tooling only; no pattern is built in this run). Risk: NORMAL.
Repo root: /Users/devinanderson/Desktop/toolbench. Read .claude/CLAUDE.md and .claude/rules/*.md first; they are
the authority for what toolbench is. Edit nothing outside this repo. Never open any .env, app.env or accounts.json
anywhere. No Co-Authored-By, session or attribution lines. No git init, commit or push. No background processes;
every go test carries -timeout; stop any PID you start. One shell command per call. Do not ask me anything; a
choice nobody made goes to the nearest existing calcbench pattern and into doc/ASSUMPTIONS.md.

THE TWO RULES THIS WHOLE PROJECT RUNS ON
1. github.com/code-heim is our TRUE NORTH FOR ORGANISING: it tells us which patterns matter and in what order.
   It is never copied and never cited. Those repos are 1-3 years old, so every pattern is RE-RESEARCHED against
   today's Go and cited to its authority, to the same standard as calcbench (.claude/rules/sources.md).
2. calcbench (github.com/CrimsonCoder42/calcbench v0.1.0, local copy /Users/devinanderson/Desktop/calcbench,
   read-only) is USED THROUGHOUT. For every pattern, search calcbench's INDEX.md first: any data structure,
   algorithm or formula it already has (ring buffer, heap, deque, iterators in algo/seq, running statistics,
   windows, backoff or bucket arithmetic, hashing, percentiles...) is called by ID, not rewritten. A pattern
   re-implements something calcbench has only when it cannot be done correctly otherwise (for example it must be
   lock-free or channel-based), and the row says why.

TWO TRACKS RUN AT THE SAME TIME. You are the LEAD. Track A is yours. Track B is four background agents you launch
first, before starting Track A, so nothing waits.

TRACK A -- THE GUTS OF THE PACKAGE (the lead builds this; model every file on calcbench's equivalent)
A1. go.mod: `module github.com/CrimsonCoder42/toolbench`, the same `go` line as calcbench's go.mod, and
    `require github.com/CrimsonCoder42/calcbench v0.1.0`. Written with the Write tool; no `go mod` or `go get`
    command. go.sum is produced by `go build -mod=mod ./...` once A4 imports calcbench.
A2. doc.go: `package toolbench`, a package comment taken from CLAUDE.md's opening and layer table.
A3. .gitignore: keep what is there; make sure it holds, once each: _delete/  _reference/  .claude/  .env  .env.*
    !.env.example  *.test  *.out  /bin/  .DS_Store  *.swp  doc/**/out/  compare/vendor/  **/node_modules/ .
A4. internal/calccheck: one test that imports a calcbench package (num/stats ArithmeticMean of 1,2,3 = 2) so the
    link to calcbench is proven by `go test` from day one. This is the only place calcbench is imported until
    the first pattern lands.
A5. internal/leak: the goroutine-leak test helper .claude/rules/go-testing.md names (snapshot before, bounded
    settle after, fail with the stacks of the extra goroutines; standard library only: runtime, runtime/pprof).
    With its own tests. This is test tooling, not a pattern; it carries no ID.
A6. The index, exactly as calcbench does it (calcbench doc/DECISIONS.md D-070, D-071, D-072 and
    /Users/devinanderson/Desktop/calcbench/testing/index_test.go are the model; read them, then write ours):
    - INDEX.md at the root with the header row from CLAUDE.md ("Finding things") and no rows yet. It is
      GENERATED from the doc comments and each package's doc.go, never typed by hand.
    - testing/index_test.go: doc-comment shape (the labels in .claude/rules/go-functions.md: ID, Pattern,
      Authority, Tags always; Goroutines, Cancel, Errors, Blocking, Math where they apply), no duplicate IDs,
      INDEX.md agrees with the code, every doc.go agrees with the code, doc/taxonomy.md agrees with the tree
      (a built package has a row with Status built; no folder without a row), and a counts test that prints
      exports | INDEX rows | doc.go entries. IDs are `TB-<SHORT>-<NNN>`; SHORT comes from doc/taxonomy.md.
      internal/ and compare/ are outside the index.
    - The INDEX.md generator in the same place calcbench keeps its own (same mechanism, adapted labels).
    All of it must pass on a tree with zero patterns.
A7. doc/specs/TEMPLATE.md: one page per package, written before its code: exports with ID and signature,
    pattern name, authority, goroutines and what stops them, cancellation, errors, blocking, calcbench IDs
    called, the library it is compared against, out of scope.
A8. doc/sources/code-heim.md (what the org is, licence per repo, "reading list only") and
    doc/sources/calcbench.md (version pinned, how to look up an ID).
A9. Checks for Track A: gofmt -l prints nothing · `go build -mod=mod ./...` · `go vet ./...` ·
    `go test -race -timeout 5m ./...`. Paste the lines.

TRACK B -- THE PATTERN LADDER (read-only research; four agents, all at once, each capped at 25 minutes; a reader
that is not done hands back its remaining repos and you relaunch it on those)
B0. LEAD, before launching: get the repo list (WebFetch https://github.com/orgs/code-heim/repositories, all
    pages; expect 81; the names at the end of this file are the fallback) and clone each, one at a time, with
    `sleep 3` between: `git clone --depth 1 https://github.com/code-heim/<name>.git _reference/code-heim/<name>`.
    Never run their code, their tests or `go mod download`. Then write doc/ladder_2026-11-13/README.md (index)
    and BRIEF.md: the two rules above, the row format, the tier definitions, the split.
B1. Readers, by repo number: R1 go_2..go_25 + socket_io_emitter · R2 go_26..go_49 · R3 go_50..go_70 ·
    R4 go_71..go_85. Each writes doc/ladder_2026-11-13/<R#>.md, one row per PATTERN it finds (a repo can yield
    several; read the code, never guess from the repo name):
    repo | pattern (textbook name) | what the repo does with it, one line | LAYER (core / httpx / grpcx /
    adapters/<driver> / FRAMEWORK = the later net/http server framework / SKIP = library usage only, say why)
    | TIER (below) | builds on (other patterns it needs first)
    | TODAY'S GO: what the current standard library and golang.org/x already give for this, checked against the
      Go release notes on go.dev for every release since the repo was last updated (iter, slices, maps,
      errors.Join, context causes and AfterFunc, sync.OnceFunc/OnceValue, WaitGroup.Go, log/slog, net/http
      routing, testing/synctest and whatever else applies) and what in the repo is therefore outdated
    | authority, to the chapter or section (.claude/rules/sources.md)
    | proposed package and API: 1-4 Go signatures in the shape of .claude/rules/go-functions.md
    | CALCBENCH: every calcbench ID this pattern should call (search INDEX.md by meaning, name the ID), or
      "none", or "re-implemented because <reason>"
    | compare against (the library this replaces, module path, or none) | licence
    | USED TODAY: where Devin's projects hand-roll this pattern now: grep, read-only, in
      /Users/devinanderson/Desktop/advo_production/advomedix-v2/backendAdvo (never vendor/, never *.env) and
      /Users/devinanderson/Desktop/red_lite_cache (never .redlite/); up to three file:line each, or "not found"
    TIERS, lowest first; a pattern goes in the lowest tier whose dependencies it has:
    T0 FOUNDATION: what every other pattern leans on (panic-to-error, error joining, functional options, clock,
       context helpers, must, first-class function helpers).
    T1 BASE CONCURRENCY: goroutine and channel primitives (generator, pipeline stage, fan-out, fan-in/merge,
       or-done, tee, bridge, buffered vs unbuffered semantics, mutex/confinement, errgroup, semaphore, sync.Pool).
    T2 COMPOSED CONCURRENCY: worker pool, bounded parallel map/filter (the concurrent twins of calcbench
       algo/seq), batching, background work, task scheduling, in-process pub/sub.
    T3 RESILIENCE: retry, backoff with jitter, timeout, rate limit, circuit breaker, bulkhead, singleflight.
    T4 STATE AND STRUCTURE: cache (TTL, cache-aside), lock with lease, bloom filter use, builder, decorator,
       dependency injection, config layering, structured logging.
    T5 INTEGRATION INTERFACES: publisher/consumer, queue with retry and dead-letter, repository, transaction,
       pagination, outbox, idempotency key (core interfaces only).
    T6 TRANSPORT AND DRIVERS: httpx, grpcx, adapters/<driver>.
B2. LEAD MERGE -> doc/ladder_2026-11-13/LADDER.md. This is the build plan for the whole package:
    1. THE LADDER: every pattern once (duplicates across repos merged), sorted by tier, then inside a tier by
       how many other patterns build on it, then by USED TODAY count. Columns: rank | pattern | package | tier |
       builds on | repos | used today | calcbench IDs | authority | compare against.
    2. STANDARD PATTERNS NOT IN code-heim that the authorities treat as standard and this library is expected
       to have, placed in their tier with their authority.
    3. PACKAGE MAP: every proposed package, SHORT, layer, tier, patterns, dependencies. Names per
       .claude/rules/go-functions.md. A proposal: do NOT write it into doc/taxonomy.md or .claude/.
    4. API LIST per package, made consistent across packages (context first, one option style, one error style).
    5. CALCBENCH USE: every call with its ID, grouped by calcbench package; then MISSING FROM CALCBENCH: math or
       data structures toolbench needs that calcbench lacks (a request list for calcbench, nothing built here).
    6. BUILD WAVES: the ladder cut into waves of at most 4 builders, one package per builder, lower tiers first,
       no wave depending on a later one. Wave 1 is the most basic and essential patterns only.
    7. OUTDATED IN THE REPOS: what today's Go replaced, one line each, so nobody ports it.
    8. SKIPPED and FRAMEWORK lists, one line each.
    9. DECISIONS FOR DEVIN: only package names that could reasonably go two ways, any core dependency beyond the
       standard library and calcbench, and anything that cannot use calcbench where one would expect it to.

NOT IN THIS RUN: any pattern code, compare/ tests, httpx, grpcx, adapters, doc/taxonomy.md rows, .claude edits.

FINAL REPORT (<=40 lines): Track A files created and the four check lines · exports | INDEX rows | doc.go entries
(expect 0 | 0 | 0) · repos found / cloned / missing · patterns per tier · the package map, one line per package ·
Wave 1 and Wave 2 as lists · calcbench calls count, and the missing-from-calcbench list · decisions for me ·
anything refused or blocked · PIDs started and stopped.

Names (org page, 2026-11-13): socket_io_emitter, go_85_first_class_functions_closures, go_83_http_testing_2,
go_82_rate_limiting_revised, go_81_loki_grafana_alloy, go_80_tcp_udp_sockets, go_79_loki_grafana, go_78_resty,
go_77_rate_limit, go_76_aws_s3, go_75_coder_websocket, go_74_pgx, go_73_lambda, go_72_viper_config, go_71_sqlx,
go_70_must_pattern, go_69_bloom_filter, go_68_asynq, go_67_conc, go_66_tunny, go_65_mongo, go_64_multipart_upload,
go_63_cobra_cli, go_62_ordone_tee_channel, go_61_err_group, go_60_iter, go_59_pdf, go_58_rabbitmq,
go_57_build_tags, go_56_generator, go_55_grpc, go_54_sync_pool, go_53_http_testing, go_52_protobuf, go_51_context,
go_50_task_scheduling_part_2, go_49_task_scheduling, go_48_llm_cli, go_47_email, go_46_elasticsearch,
go_45_memcache, go_44_kafka, go_43_net_http_middleware, go_42_faktory, go_41_dependency_injection,
go_40_interfaces_generics, go_39_excel, go_38_csv, go_37_background_work, go_36_gin_jwt, go_35_gorm_polymorphism,
go_34_gorm_preload_vs_joins, go_33_endless_scrolling, go_32_http_error_code, go_31_yaml_config,
go_17_env_variables, go_30_gin_auth, go_28_htmx, go_27_panic_recover, go_26_slog, go_25_fan_out_fan_in,
go_24_unbuffered_buffered_channels, go_23_gin_html_json_response, go_21_goroutines_pipeline,
go_20_mutexes_and_confinement, go_19_custom_logger, go_18_worker_pool, go_15_graphql_mysql, go_14_builder_pattern,
go_13_gorm_custom_serializer, go_12_gorilla_websocket, go_11_cache_sync_map, go_10_gorm_serializer,
go_9_gin_gorm_pagination, go_8_redis_lock, go_7_go_decorator, go_6_gorm_filters, go_5_go_rss_feed,
go_4_gorm_scopes, go_3_gorm_many_to_many, go_2_gorm_association_one_to_many.
