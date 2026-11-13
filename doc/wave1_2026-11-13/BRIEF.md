# Brief for the wave builders (waves 1 and 2, 2026-11-13)

Request: `doc/requests/wave1_wave2_2026-11-13.md` (amended by Devin: steps 1f and 3 deleted). Plan: `doc/ladder_2026-11-13/LADDER.md`. Authority for how code is
written: `.claude/CLAUDE.md`, `.claude/rules/go-concurrency.md`, `go-functions.md`, `go-testing.md`, `sources.md`.
Read THIS file, `doc/specs/TEMPLATE.md`, the four rules files and your LADDER.md rows; you do not need the whole
repo. Devin approved every LADDER.md §9 recommendation (D-006).

## THE STANDARD FOR EVERY PACKAGE (verbatim from the request)

Piece by piece, in this order, nothing skipped:
1. RESEARCH, written before any code, as doc/specs/<package>.md from doc/specs/TEMPLATE.md:
   a. The reference: which _reference/code-heim repos show the pattern (LADDER.md "repos" column), what each
      does, what is outdated in it (LADDER.md §7) and the defects found there; those defects become our tests.
   b. The authority: the Go Blog post, standard-library source or book section LADDER.md names, OPENED and read,
      cited to the section (.claude/rules/sources.md). Where authority and repo disagree, the authority wins.
   c. Today's Go: what the standard library and golang.org/x give for this now
      (doc/ladder_2026-11-13/GO_RELEASES.md) and why toolbench still needs the export (or does not: then it is
      a doc line, not code).
   d. calcbench: the IDs this package calls (LADDER.md §5), confirmed in
      /Users/devinanderson/Desktop/calcbench/INDEX.md; anything re-implemented, with the reason.
   e. Every export: ID, signature (LADDER.md §4 is the agreed shape; a deviation is written down with why),
      pattern name, goroutines and what stops them, cancellation, errors, blocking, zero-value behaviour.
   g. The comparison: which library in compare/ it is checked against and on what behaviour.
   h. Out of scope.
2. BUILD, from the spec only: doc.go (SHORT, export list), one file per concept, the doc comment in the exact
   shape of .claude/rules/go-functions.md, tests per .claude/rules/go-testing.md (Example with // Output:,
   edge cases, cancellation, panic, leak.Verify per test and leak.Main in TestMain, the repo defects as cases),
   compare/ tests where LADDER.md names a library. Checks while building: gofmt -l, go build ./..., go vet on the
   package. Builders never run go test.

Amended by Devin (2026-11-13): step 1f and step 3 (CONSUMER NOTE) are deleted. The only read-only reference
outside this repo is /Users/devinanderson/Desktop/calcbench. advo_production and red_lite_cache are not opened in
this run. (Step letters keep their original names, so 1g follows 1e.)

## Mechanics every builder follows

- **Owned paths** (write nothing else): `<package>/`, `doc/specs/<package>.md`,
  `compare/<package>/` (your comparison tests, if LADDER.md names a library). INDEX.md, doc/taxonomy.md,
  doc/DECISIONS.md, doc/ASSUMPTIONS.md, doc/CHANGELOG.md, doc/sources/, go.mod, compare/go.mod belong to the lead:
  put anything they need in the "For the lead" section at the end of your spec.
- **doc.go** shape (testing/index_test.go reads it):
  ```go
  // Package panics turns a panic in caller-supplied code into an error ...
  //
  // ID short form: PANIC (IDs are TB-PANIC-NNN).
  //
  // Exports:
  //
  //	TB-PANIC-001  ErrPanic    <one line: what it does>
  //	TB-PANIC-002  PanicError  <one line>
  package panics
  ```
  Each entry: a TAB, the ID, spaces, the exact Go name (`Type.Method` for a method), spaces, the "Does" line.
- **What carries an ID**: every exported function and type. A method needs none when its receiver type has an
  ID; `New<T>` needs none when T has one (calcbench D-071), but MAY have one. Exported constants and variables
  (sentinel errors) may carry one; give `ErrX` an ID when callers match on it. Number from 001 in the order of
  the spec table; never reuse.
- **Doc comment** exactly as `.claude/rules/go-functions.md`: line 1 starts with the name; then `ID`, `Pattern`,
  `Authority` (repeat the label for a second citation), `Goroutines`, `Cancel`, `Errors`, `Blocking` (where they
  apply), `Math` (calcbench IDs, just before Tags), `Tags`. Labels align with spaces after the colon. An unknown
  label fails the index test. Never cite code-heim. A type states its zero-value behaviour.
- **Tests**: `func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }` and `leak.Verify(t)` first in every test that
  starts goroutines (`github.com/CrimsonCoder42/toolbench/internal/leak`); never with `t.Parallel`. Every test
  bounded by `context.WithTimeout(t.Context(), 10*time.Second)` where it waits. No `time.Sleep`. Example functions
  with `// Output:`. Names `Test<Export>_<case>`. The repo defects in LADDER.md §7 for your pattern become named
  test cases (`Test<Export>_<defect>`).
- **Checks while building** (one command per call): `gofmt -l <pkg>`, `go build ./...`, `go vet ./<pkg>/`.
  Do NOT run `go test`. Do NOT run `go run`, `go get`, `go mod` anything.
- **Authority**: open it (WebFetch the URL) and cite the section. In the spec record `Opened: <URL>, 2026-11-13`.
  If a page cannot be fetched, say so in the spec; do not cite what you did not open.
- **Outside this repo** only `/Users/devinanderson/Desktop/calcbench` is read (read-only). advo_production and
  red_lite_cache are never opened.
- **Hard limits**: edit nothing outside your owned paths; never open `.env`, `.env.*`, `app.env`,
  `accounts.json`, `vendor/`, `node_modules/`; no git, no rm (a delete is `mv -n` to `_delete/`, tell the lead), no
  background processes, no sleep. **30-minute cap**: if not done, stop and list what remains at the end of
  your spec under `REMAINING:`.
- **Hand back** (≤ 15 lines): spec path; exports with ID and file:line; authority opened yes/no (URLs);
  calcbench IDs called; deviations from LADDER.md §4 with why; "For the lead" items;
  REMAINING.

## Wave 1 (T0): owned packages and the agreed §4 APIs

Folder: `doc/wave1_2026-11-13/`. No package imports another toolbench package except `internal/leak` in tests.

**B1 `panics`, SHORT PANIC**: LADDER.md rank 1. Repos go_27_panic_recover, go_67_conc. Authority: Gerrand,
"Defer, Panic, and Recover", The Go Blog, 2010 (https://go.dev/blog/defer-panic-and-recover); Effective Go,
"Recover" (https://go.dev/doc/effective_go#recover); Go spec "Handling panics"; `runtime.PanicNilError`
(go1.21). Compare: github.com/sourcegraph/conc/panics (`compare/panics/`).
```go
var ErrPanic = errors.New("panics: recovered panic")
type PanicError struct{ Value any; Stack []byte }      // Error(); Unwrap() error returns ErrPanic
func Try(fn func() error) error
func TryValue[T any](fn func() (T, error)) (T, error)
```
Every later package returns `panics.ErrPanic` for a recovered panic: this is the one shared type.

**B2 `clock`, SHORT CLOCK**: rank 2. No repo (standard). Authority: pkg.go.dev `time` (`Timer`, `AfterFunc`,
`Ticker`, the go1.23 timer changes: https://pkg.go.dev/time#Timer, https://go.dev/doc/go1.23#timer-changes);
pkg.go.dev `testing/synctest`; Meszaros, xUnit Test Patterns, 2007, "Test Double". Compare:
github.com/benbjohnson/clock (`compare/clock/`).
```go
type Clock interface{ Now() time.Time; NewTimer(d time.Duration) Timer; AfterFunc(d time.Duration, f func()) Timer }
type Timer interface{ C() <-chan time.Time; Stop() bool; Reset(d time.Duration) bool }
func Real() Clock
func NewFake(start time.Time) *Fake                     // Advance(d), BlockUntil(ctx, n) — waiters, no sleep
```
Later waves need tickers (schedule, wave 4) and `Since`: adding `NewTicker`/`Since` now is allowed as a written
deviation if the spec argues it; the interface must stay small. The fake must match go1.23 semantics (no stale
value after Stop/Reset; unbuffered channel behaviour as far as observable), be safe for concurrent use, and let a
test wait for "n timers armed" on a channel or with ctx, never by polling with sleep.

**B3 `ctxkey`, SHORT CTXKEY**: rank 4. Repo go_51_context. Authority: pkg.go.dev `context.WithValue` ("The
provided key must be comparable and should not be of type string or any other built-in type"); Ajmani, "Go
Concurrency Patterns: Context", The Go Blog, 2014 (https://go.dev/blog/context). Compare: none.
```go
func New[T any](name string) *Key[T]
func (k *Key[T]) With(ctx context.Context, v T) context.Context
func (k *Key[T]) Value(ctx context.Context) (T, bool)
```

## Wave 2 (T1)

Written by the lead into this file's wave 2 twin, `doc/wave2_2026-11-13/BRIEF.md`, after wave 1 is green.
