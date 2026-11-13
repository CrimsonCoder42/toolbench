# Today's Go: release notes go1.22 to go1.27 (checked 2026-11-13)

The code-heim repos were last pushed between 2023-11-27 and 2025-04-23, so the releases that matter are go1.22
onward. Read from https://go.dev/doc/go1.22 ... https://go.dev/doc/go1.27 and https://go.dev/doc/devel/release
(read 2026-11-13). Readers R2 and R3 fetched go1.26 and go1.27 themselves; R1 and R4 wrote TODAY'S GO from
knowledge and the lead checked those cells against this list at merge. Installed: go1.27.3 darwin/arm64.

| Release | Date | What a pattern library uses from it |
|---|---|---|
| go1.22 | 2024-02-06 | per-iteration loop variables (the `v := v` copy is dead); `for i := range n`; `math/rand/v2` (`N`, `IntN`, seeded); `net/http.ServeMux` method + wildcard patterns, `Request.PathValue`; `cmp.Or`; `slices.Concat` |
| go1.23 | 2024-08-13 | range-over-func; package `iter` (`Seq`, `Seq2`, `Pull`); `slices.All/Values/Collect/Sorted/Chunk`, `maps.Keys/Values/All/Collect`; `time.Timer`/`Ticker` unbuffered channels, no stale value after `Stop`/`Reset`, collectable when unreferenced; `unique.Make`; `sync.Map.Clear`; `atomic.And/Or` |
| go1.24 | 2025-02-11 | generic type aliases; `os.Root`; `testing.B.Loop`, `T.Context`; `runtime.AddCleanup`; package `weak`; `testing/synctest` (experiment); `crypto/rand.Text`; `encoding/json` `omitzero`; `slog.DiscardHandler`; faster `sync.Map`; `strings.SplitSeq/Lines` |
| go1.25 | 2025-08-12 | `sync.WaitGroup.Go`; vet `waitgroup`; `testing/synctest` GA (`synctest.Test`, `synctest.Wait`); `net/http.CrossOriginProtection`; `slog.GroupAttrs`; container-aware `GOMAXPROCS`; `runtime/trace.FlightRecorder` |
| go1.26 | 2026-02-10 | `errors.AsType[E]`; `new(expr)`; `slog.NewMultiHandler`; `signal.NotifyContext` cancels with a cause naming the signal; goroutine-leak profile (experiment); `go fix` modernizers; ServeMux trailing-slash redirect 307 |
| go1.27 | 2026-08-19 | generic methods; goroutine-leak profile GA (`runtime/pprof` `goroutineleak`); `asynctimerchan` removed (timer channels always synchronous); `encoding/json/v2` GA; `synctest.Sleep`; `httptest.NewTestServer`; package `uuid`; `strings.CutLast`; vet `printf` `%w` on `*E` |

Consequences for toolbench, applied in LADDER.md section 7:
- `golang.org/x/sync/errgroup` shape stays the reference API, but `sync.WaitGroup.Go` (1.25) covers the
  no-error case; toolbench wraps it, never re-implements it.
- Every waiting pattern is tested with `testing/synctest` (1.25 GA, `Sleep` in 1.27) or a clock option; the
  internal/leak helper stays (it checks after every test), and the 1.27 `goroutineleak` profile is a second
  opinion for the reviewer, not a replacement (it needs the leaked goroutine to be unreachable).
- Generators and pipeline stages over finite data are `iter.Seq`; channels stay for concurrency only.
- `errors.AsType` replaces `errors.As` with a pointer variable in new code.
- `math/rand/v2` for jitter (no seeding); a uuid comes from the standard library `uuid` package.
