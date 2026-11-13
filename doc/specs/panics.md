# panics: a panic in caller-supplied code, returned as an error

Status: SHIPPED (2026-11-13)
Package: `panics` · SHORT: `PANIC` · Layer: core · Tier: T0
Work folder: `doc/wave1_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #1
Builds on: nothing (standard library only: `errors`, `fmt`, `runtime/debug`) · Decisions: D-006 · Assumptions: none new

Every later toolbench package that runs caller code (group, flow, pool, parallel, schedule, pubsub, retry,
singleflight, txn, httpx, grpcx) returns a recovered panic as `panics.ErrPanic` through this package: this is the
one shared type, in the lowest tier.

## 1a. The reference (reading list only, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `_reference/code-heim/go_27_panic_recover/repanicking.go` | `processInput` defers a recover that logs, prints "cleanup" and re-panics (`:14-24`); `main` recovers and prints (`:36-40`). | Re-panic-for-cleanup is an anti-pattern for library workers (cleanup belongs in a plain `defer`); the recovered value is printed, never turned into an error; logging inside the recover breaks the Quiet rule. Test: `TestTry_cleanupRunsWithoutRepanic` (a deferred cleanup inside fn runs and the panic still arrives as one error, not a second panic). |
| `_reference/code-heim/go_67_conc/group/main.go` | `conc.WaitGroup.Go` catches a task panic and re-raises it on `Wait` (`:16-28`). | Re-raising on `Wait` crashes the caller instead of returning a value; `time.Sleep` as fake work (`:24`). Our answer: the panic is an `error` (`ErrPanic`), never re-raised. |

Defects recorded in LADDER.md §7 and the R1/R2/R3 notes (not re-read here), also tests:
- backendAdvo `metabolic_reports.go:577,584`: `defer func() { recover() }()` swallows the panic silently; the
  result pointer stays nil and nobody knows why. Test: `TestTry_swallowedPanicSurfacesAsErrPanic`.
- A recover helper that treats a nil `recover()` as "no panic" misses `panic(nil)` under `GODEBUG=panicnil=1`.
  Test: `TestTry_panicNil` (default go1.21+ behaviour: `*runtime.PanicNilError`, matched with `errors.As`).

## 1b. The authority (opened 2026-11-13)

- Gerrand, "Defer, Panic, and Recover", The Go Blog, 4 Aug 2010, sections "Panic" and "Recover".
  Opened: https://go.dev/blog/defer-panic-and-recover, 2026-11-13. "Recover is only useful inside deferred
  functions. During normal execution, a call to recover will return nil and have no other effect. If the current
  goroutine is panicking, a call to recover will capture the value given to panic and resume normal execution."
- Effective Go, "Recover". Opened: https://go.dev/doc/effective_go#recover, 2026-11-13. "A call to recover stops
  the unwinding and returns the argument passed to panic ... only useful inside deferred functions"; the
  `safelyDo` server example (a failing goroutine shut down without killing the others) is exactly `Try`.
- pkg.go.dev `builtin`, `recover` and `panic`. Opened: https://pkg.go.dev/builtin#recover, 2026-11-13. "Executing
  a call to recover inside a deferred function (but not any function called by it) stops the panicking
  sequence"; "Starting in Go 1.21, calling panic with a nil interface value or an untyped nil causes a run-time
  error (a different panic). The GODEBUG setting panicnil=1 disables the run-time error."
- pkg.go.dev `runtime.PanicNilError` and `runtime.Goexit`. Opened: https://pkg.go.dev/runtime#PanicNilError,
  2026-11-13. "A PanicNilError happens when code calls panic(nil)" (go1.21). Goexit: "Goexit runs all deferred
  calls before terminating the goroutine. Because Goexit is not a panic, any recover calls in those deferred
  functions will return nil."
- The Go Programming Language Specification, "Handling panics": https://go.dev/ref/spec#Handling_panics was
  fetched twice on 2026-11-13 and the fetched text was truncated before that section both times. It is NOT
  cited in the code; the builtin docs above carry the same rule.

### Decisions taken from the authority

1. **`panic(nil)`.** With go 1.27 in go.mod, `panic(nil)` reaches `recover` as `*runtime.PanicNilError`, so it is
   an ordinary panic: `PanicError.Value` holds the `*runtime.PanicNilError` and `errors.As(err, &pne)` finds it.
   Under `GODEBUG=panicnil=1` recover returns nil. Try does not rely on the recover value to decide "did it
   panic": it uses a completion flag (set after fn returns) and a flag set after the recovering closure returns,
   the technique of `golang.org/x/sync/singleflight`. A nil-valued panic is therefore still `ErrPanic`, with
   `Value == nil`. Never "no panic".
2. **`runtime.Goexit` inside fn** (for example `t.FailNow` in a test). Goexit is not a panic and cannot be
   stopped by recover. Try lets it through: fn's and Try's deferred calls run, the goroutine terminates, Try does
   not return. It is not converted to an error, because turning it into a return would break `t.FailNow`/
   `t.SkipNow` and every caller that relies on Goexit. Documented on Try and TryValue; tested in
   `TestTry_goexitPassesThrough`.
3. **Re-panicking** (go_27): never. The value is returned.
4. **The stack** is taken with `runtime/debug.Stack()` inside the deferred function, so it shows the panic site.

## 1c. Today's Go

go1.27 has no panic-to-error helper: `sync.WaitGroup.Go` (1.25) does not recover; `golang.org/x/sync/errgroup`
does not turn a worker panic into an error. `errors.Join` (1.20), `errors.AsType` (1.26) help callers match
`*PanicError`. toolbench still needs the export: every later package needs one shared `ErrPanic`; LADDER.md rank 1 lists the hand-rolled recovers in the consumer projects.

## 1d. calcbench

No mathematics. `Math:` line omitted. Nothing re-implemented.

## 1e. Exports

| ID | Name | Signature |
|---|---|---|
| `TB-PANIC-001` | `ErrPanic` | `var ErrPanic = errors.New("panics: recovered panic")` |
| `TB-PANIC-002` | `PanicError` | `type PanicError struct{ Value any; Stack []byte }`; `Error() string`; `Unwrap() []error` |
| `TB-PANIC-003` | `Try` | `func Try(fn func() error) error` |
| `TB-PANIC-004` | `TryValue` | `func TryValue[T any](fn func() (T, error)) (T, error)` |

No options (nothing to bound: Try starts no goroutine and holds no buffer).

### `TB-PANIC-001` `ErrPanic`
Sentinel matched with `errors.Is(err, panics.ErrPanic)`. Every `*PanicError` unwraps to it.

### `TB-PANIC-002` `PanicError`
- **Pattern**: panic-to-error (recover in a deferred call).
- **Fields**: `Value` the value passed to panic (a `*runtime.PanicNilError` for `panic(nil)`, nil only under
  `GODEBUG=panicnil=1`); `Stack` the panicking goroutine's stack from `debug.Stack()`.
- **Error()**: `"panics: recovered panic: <Value with %v>"`, one line; the stack is not in the message (read
  `Stack`).
- **Unwrap() []error**: `[ErrPanic]`, plus `Value` when Value is an `error`. So `errors.Is(err, ErrPanic)` is
  always true and `errors.Is(err, io.EOF)` is true after `panic(io.EOF)`; `errors.As` finds a
  `runtime.Error` / `*runtime.PanicNilError`.
- **Zero value**: usable; `Error()` prints `<nil>` for the value and it still unwraps to ErrPanic.
- **Deviation from LADDER.md §4** ("`Unwrap() error` returns ErrPanic"): `Unwrap() []error` keeps that guarantee
  for `errors.Is/As` and adds the panic value when it is an error (what conc/panics `ErrRecovered.Unwrap` does).
  Cost: `errors.Unwrap(err)` returns nil (it only follows the single-error form). Recommendation kept; the
  alternative (`Unwrap() error` returning only ErrPanic) loses `errors.Is(err, io.EOF)` after `panic(io.EOF)`.

### `TB-PANIC-003` `Try`
- **Pattern**: panic-to-error. **Familiar shape**: conc/panics `Try(f func()) *Recovered`, but returning `error`
  (fn's own error passes through unchanged) so it drops into errgroup-style `func() error` code.
- **Goroutines**: none; fn runs on the caller's goroutine.
- **Cancel**: no ctx (fn is synchronous; ctx is fn's own business).
- **Errors**: fn's error unchanged; a panic is a `*PanicError` (errors.Is ErrPanic); a nil fn is a panic of the
  call itself, so it returns `*PanicError` (never panics on input).
- **Blocking**: exactly as long as fn.
- **Goexit**: passes through (decision 2).

### `TB-PANIC-004` `TryValue`
As Try, for `func() (T, error)`. On a panic the value is T's zero value.

## 1g. Comparison

`compare/panics/panics_test.go` against `github.com/sourcegraph/conc/panics` (v0.3.0 in the module cache):
same inputs (no panic, panic(string), panic(error), panic(nil), runtime error, nil fn), same observable result
(panicked or not, value, error chain includes the cause). Named differences: conc returns `*Recovered`
(nil when nothing panicked), we return `error`; conc's `Error()` includes the stack, ours is one line; conc's
`ErrRecovered.Unwrap` gives only the cause, ours gives `ErrPanic` and the cause; conc has `Catcher` (first panic
across goroutines) and `Repanic`, which we do not ship (re-panic is what we replace; multi-goroutine collection is
`group`'s job, wave 2).

## Tests

Example per export with `// Output:`; happy path; fn error passes through; panic(string), panic(error),
panic(nil) (`*runtime.PanicNilError`), runtime error (nil map write) matched with `errors.As(runtime.Error)`; nil
fn; Stack non-empty and names the panicking function; Goexit passes through; deferred cleanup inside fn runs;
the swallowed-panic defect; zero `PanicError`; `GODEBUG=panicnil=1` (Value nil, still `ErrPanic`) in the
test-only package `panics/internal/panicniltest/` whose `_test.go` carries `//go:debug panicnil=1` (it first
asserts that recover returns nil for panic(nil), so an ignored directive fails loudly); `leak.Main` in TestMain, `leak.Verify` in the test that starts a
goroutine. Benchmark `BenchmarkTry` with `b.ReportAllocs()` (on every later package's hot path).

## 1h. Out of scope

Collecting panics from several goroutines (`group`, wave 2); re-panicking (never); logging (the caller's hook);
HTTP/gRPC recovery middleware (`httpx.Recover`, `grpcx`, wave 6); `Must` (decision 9.10: not shipped).

## For the lead

- INDEX.md rows TB-PANIC-001..004; taxonomy row exists (`panics`, PANIC).
- doc/sources: go.dev blog defer-panic-and-recover, Effective Go "Recover", pkg.go.dev builtin/runtime.
- DECISIONS candidate: `PanicError.Unwrap() []error` (deviation above), panic(nil) and Goexit handling.
- compare/go.mod needs `github.com/sourcegraph/conc v0.3.0` (in the local module cache).
- Step 1f and the consumer note were dropped by Devin (2026-11-13). `doc/wave1_2026-11-13/consumers_panics.md`
  was written before that change arrived; it is left in place for the lead to move to `_delete/`.
- The spec URL `#Handling_panics` could not be read through WebFetch (truncated); not cited.
