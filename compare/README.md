# compare

Comparison tests: each toolbench package beside the library it replaces, same inputs, same observable behaviour,
every difference named in the test (decision D-005). This is its own module (`compare/go.mod`, toolbench through
`replace ../`), the only module allowed `golang.org/x/sync` and the libraries `doc/ladder_2026-11-13/LADDER.md`
names, so the main module's go.mod never grows because of a test.

| Folder | toolbench | Compared with |
|---|---|---|
| `breaker/` | `breaker` | github.com/sony/gobreaker/v2 v2.4.0 (one real 1 s wait: gobreaker has no clock seam) |
| `cache/` | `cache` | github.com/hashicorp/golang-lru/v2/expirable v2.0.7 (LRU order and capacity only; its TTL reads wall time) |
| `clock/` | `clock` | github.com/benbjohnson/clock v1.3.5 |
| `flow/` | `flow` | github.com/sourcegraph/conc/stream v0.3.0 (where shapes meet) |
| `group/` | `group` | golang.org/x/sync/errgroup v0.25.0 |
| `lifecycle/` | `lifecycle` | github.com/oklog/run v1.2.0 |
| `panics/` | `panics` | github.com/sourcegraph/conc/panics v0.3.0 |
| `parallel/` | `parallel` | github.com/sourcegraph/conc/iter v0.3.0, golang.org/x/sync/errgroup |
| `pool/` | `pool` | github.com/sourcegraph/conc/pool v0.3.0 |
| `ratelimit/` | `ratelimit` | golang.org/x/time/rate v0.16.0 |
| `recycle/` | `recycle` | github.com/valyala/bytebufferpool v1.0.0, sync.Pool |
| `retry/` | `retry` | github.com/cenkalti/backoff/v4 v4.3.0 |
| `semaphore/` | `semaphore` | golang.org/x/sync/semaphore v0.25.0 |
| `singleflight/` | `singleflight` | golang.org/x/sync/singleflight v0.25.0 |

No compare: `ctxkey`, `guard`, `batch`, `schedule`, `bulkhead` (no library named), `config` (viper is a framework), `logging` (handlers only), `pubsub` (watermill's tree is too heavy,
doc/specs/pubsub.md).

Run: `cd compare && go test -race -timeout 10m ./...`
