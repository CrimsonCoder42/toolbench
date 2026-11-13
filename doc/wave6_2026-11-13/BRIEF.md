# Brief for the wave 6 builders (ratelimit switch + T3-T4, 2026-11-13)

Request: `doc/requests/wave6_2026-11-13.md`. The standard, mechanics and hard limits are those of
`doc/wave3_2026-11-13/BRIEF.md` (read it first), plus "Built on" and "Lessons" in `doc/wave4_2026-11-13/BRIEF.md`
and `doc/wave5_2026-11-13/BRIEF.md`. Folder: `doc/wave6_2026-11-13/`. Builders never `go test`, never `go mod`;
compare/ files are written, the lead tidies once after the wave.

## STEP 0 is done

`go.mod` requires `github.com/CrimsonCoder42/calcbench v0.3.0` (main go.mod sha256 c889b165…, go.sum c7cc2938…;
these must not change again). New in v0.3.0 (calcbench doc/requests/toolbench_reply_2026-11-13.md "## v0.3.0",
confirmed in calcbench INDEX.md and the pinned source `$(go env GOMODCACHE)/github.com/!crimson!coder42/calcbench@v0.3.0/`):
- ALG-RATELM-005 `func (b *TokenBucket) Restore(n int64, now time.Time)`: give back n tokens taken earlier, capped
  at the burst.
- ALG-LIST-018 `func (c *LRUCache[K, V]) Oldest() (key K, value V, ok bool)`: the least recently used entry, not
  removed, recency unchanged.
Also available: ALG-LIST-015 LRUCache, ALG-LIST-016 LFUCache, ALG-BLOOM-001 BloomFilter (NewBloomFilter, Add,
MayContain), -002 FalsePositiveRate, -003 OptimalParams. calcbench types are NOT safe for concurrent use: guard
them with a mutex named beside them (rule 8).

## Wave 6 extra duties (the reviewer checks both)

1. **No process environment, file or real clock is touched in core tests.** Environment comes through an
   injected lookup func (`func(string) (string, bool)`); files through an `fs.FS` (`fstest.MapFS` in tests) or an
   `io.Reader`; time through `clock.Clock` (`clock.Fake` in tests). No `os.Getenv`, `os.LookupEnv`, `os.Open`,
   `os.ReadFile`, `time.Now`, `time.Sleep` in core code paths or tests. Callers pass `os.LookupEnv`/`os.DirFS`.
2. **No secret value given to the logging or config tests appears in any output**: not in a log record, an error
   message, `String()`, `%v`/`%+v`/`%#v` of a config value, a panic message, an Example's `// Output:`. Tests
   assert this by searching every captured output for the secret.

## Built on (never duplicated)

`clock`, `panics`, `semaphore` (bulkhead), `singleflight` (cache GetOrLoad), `ctxkey` (logging context
attributes), `group`, `guard` where they fit; calcbench as above.

## The packages

**B0 `ratelimit` switch** (owner of `ratelimit/`, `doc/specs/ratelimit.md`, `compare/ratelimit/` only). Released
signatures unchanged (TB-RATE-001..016 keep name, parameters, results). `Reservation.Cancel` uses
ALG-RATELM-005 TokenBucket.Restore instead of restoring a saved bucket copy, so it matches x/time/rate's
CancelAt exactly (restore n minus the tokens later reservations took — read x/time/rate's CancelAt source and
reproduce its rule with Restore). `Keyed` drops its own map + container/list and uses ALG-LIST-015 LRUCache with
ALG-LIST-018 Oldest for idle eviction (peek the oldest; evict while idle). The spec's workaround notes become
`Math:` IDs; D-031's Cancel deviation ends (the lead writes the follow-up D). Removed code: `mv -n` to
`_delete/<same repo-relative path>` (whole files), or, for removed functions inside a kept file, save the removed
text as `_delete/ratelimit/<file>.removed.go.txt` before editing; tell the lead each path for the ledger.
compare/ratelimit gains the cancel-after-later-reservation case (both libraries must agree). Keep the
out-of-order-clock regression test (wave 5 blocker).

**B1 `bulkhead`, SHORT BULK** (LADDER rank 28, T3). No code-heim repo. Authority: Nygard, Release It!, 2nd ed.,
2018, ch.5 "Bulkheads" (book, not opened); Azure Architecture Center, "Bulkhead pattern"
(https://learn.microsoft.com/azure/architecture/patterns/bulkhead). Built on `semaphore` (TB-SEMA-001; do not
re-implement admission) and `clock` (queue timeout), `panics`. Compare: none named in LADDER (say so).
```go
func New(maxConcurrent int, opts ...Option) (*Bulkhead, error)   // maxConcurrent <= 0: error (D-023, long-lived)
func (b *Bulkhead) Do(ctx context.Context, fn func(context.Context) error) error
func DoValue[T any](ctx context.Context, b *Bulkhead, fn func(context.Context) (T, error)) (T, error)
type Option func(*config)
func WithQueue(n int) Option                  // default 0: fail fast with ErrFull when all slots are busy
func WithQueueTimeout(d time.Duration) Option // a queued call gives up after d on the clock → ErrFull (or ErrQueueTimeout: decide)
func WithClock(c clock.Clock) Option
func WithOnEvent(f func(Event)) Option        // metrics hook: admitted, rejected, queued, timed out (shape: builder's call)
var ErrFull
```

**B2 `cache`, SHORT CACHE** (rank 30, T4). Repos go_11_cache_sync_map (hit writes the response twice; one
goroutine per entry sleeping for TTL; unbounded; global), go_45_memcache (every error is a miss; reflection;
panics on Set failure; no stampede guard), go_69_bloom_filter (global filter filled from init, no lock).
Authority: Azure Architecture Center, "Cache-Aside pattern"
(https://learn.microsoft.com/azure/architecture/patterns/cache-aside); Bloom, "Space/time trade-offs in hash coding
with allowable errors", CACM 13(7), 1970 (through calcbench ALG-BLOOM-001's citation). calcbench: ALG-LIST-015
LRUCache, ALG-LIST-018 Oldest (TTL sweep without eviction surprises), ALG-BLOOM-001/003 for the optional bloom guard
(it is in INDEX.md: use it). Built on `singleflight` (GetOrLoad: a miss loads once), `clock` (TTL; no goroutine per
entry; no background sweeper unless stated and stopped), `panics`. Compare: github.com/hashicorp/golang-lru/v2
(expirable) where shapes meet (LADDER rank 30); write compare/cache/.
```go
type Cache[K comparable, V any] interface {        // the cache-aside interface T5 and adapters implement
    Get(ctx context.Context, k K) (V, bool, error)
    Set(ctx context.Context, k K, v V, ttl time.Duration) error
    Delete(ctx context.Context, k K) error
}
func New[K comparable, V any](opts ...Option[K, V]) (*Memory[K, V], error)  // in-memory implementation
func (m *Memory[K, V]) GetOrLoad(ctx context.Context, k K, load func(context.Context) (V, error)) (V, error)
func WithCapacity[K comparable, V any](n int) Option[K, V]; WithTTL; WithClock; WithOnEvict(f func(K, V, Reason))
func WithBloom[K comparable, V any](expected uint64, fpr float64, hash func(K) []byte) Option[K, V]  // negative lookups
func Aside[K comparable, V any](ctx context.Context, c Cache[K, V], k K, ttl time.Duration, load func(context.Context) (V, error)) (V, error)
```
State: "size and count bounds" (count = capacity; size = an optional cost func with a byte budget: decide and
state), miss vs error distinguished (go_45), what a bloom guard does on a definite miss (skip the load? only for
Aside/GetOrLoad; document), TTL 0 = no expiry or default, expired entries evicted lazily on access plus a bounded
sweep using Oldest, eviction hook runs where (outside the lock).

**B3 `config`, SHORT CONFIG** (rank 31, T4). Repos go_17_env_variables, go_31_yaml_config, go_72_viper_config
(defects: errors ignored, global config, `log.Fatal` in a loader, secrets committed and logged). Authority: Wiggins,
The Twelve-Factor App, "III. Config" (https://12factor.net/config); pkg.go.dev `os.LookupEnv`, `encoding/json`,
`flag`, `io/fs`. Built on nothing heavy (stdlib); `errors.Join` for missing keys. Compare: none (viper is a
framework; say so). The request's precedence is explicit: defaults < file < environment < flags.
```go
type Lookup func(key string) (string, bool)                      // LADDER §4
func Layer(ls ...Lookup) Lookup                                   // later wins? first wins? — state it; precedence defaults < file < env < flags
func FromMap(m map[string]string) Lookup                          // defaults
func FromJSON(r io.Reader) (Lookup, error) / FromJSONFile(fsys fs.FS, name string) (Lookup, error)  // flattened keys ("db.host")
func FromEnv(lookup func(string) (string, bool), prefix string) Lookup  // caller passes os.LookupEnv; key mapping stated
func FromFlags(fs *flag.FlagSet) Lookup                           // only flags actually set
func String(l Lookup, key, def string) string; Int, Duration, Bool (… (T, error))   // typed, never panic
func Required(l Lookup, keys ...string) error                     // every missing key, errors.Join
// redaction: a Config (or Values) type whose String()/Format never prints a secret key's value
func WithSecrets(keys ...string) …                                // shape: builder's call; default secret patterns (password, secret, token, key) stated
```

**B4 `logging`, SHORT LOG** (rank 33, T4). Repos go_19_custom_logger (global logger built in init; SetLevel race),
go_26_slog (masks keys but a value can still carry a secret; multi-writer), go_79_loki_grafana (slog.SetDefault in a
library). Authority: pkg.go.dev `log/slog` (Handler, HandlerOptions.ReplaceAttr, LogValuer,
NewMultiHandler go1.26); Go team, "A Guide to Writing slog Handlers" (https://github.com/golang/example/blob/master/slog-handler-guide/README.md);
Amsterdam, "Structured Logging with slog", The Go Blog, 2023 (https://go.dev/blog/slog). Built on `ctxkey`
(context attributes come from ctxkey keys). Compare: none. Handlers only; never a logger of our own; nothing writes
files; no globals (`slog.SetDefault` never called).
```go
func Redact(h slog.Handler, opts ...RedactOption) slog.Handler   // keys (case-insensitive, nested groups) and value patterns (regexp)
func Context(h slog.Handler, extract func(context.Context) []slog.Attr) slog.Handler  // + a helper taking ctxkey keys
func Multi(hs ...slog.Handler) slog.Handler                       // go1.26 has slog.NewMultiHandler: wrap it or justify; state which
type Recorder struct{ /* ... */ }                                 // test handler capturing records; zero value ready
func (r *Recorder) Records() []slog.Record
```
State: WithAttrs/WithGroup correctness (the guide's hard part), Enabled delegation, a LogValuer that returns a
secret, groups, redaction of a secret inside an error message (value pattern), concurrency (Recorder under -race).
