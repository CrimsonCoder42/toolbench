# <package>: <what it provides>

One page per package, written and approved before its code (model: calcbench `doc/specs/TEMPLATE.md`).
Copy to `doc/specs/<package>.md`.

Status: SPEC | BUILDING | VERIFYING | SHIPPED (date)
Package: `<package>` · SHORT: `<SHORT>` · Layer: core / httpx / grpcx / adapters/<driver> · Tier: T0..T6
Work folder: `doc/<topic>_<YYYY-MM-DD>/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #<n>
Builds on: <toolbench packages it imports, lower tiers only> · Decisions: D-… · Assumptions: …

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-<SHORT>-<NNN>` | `Name` | `func Name[T any](ctx context.Context, …, opts ...Option) (…, error)` |

Options (one style, functional, `With<Thing>`), each with its documented default:

| Option | Default | Bound it sets |
|---|---|---|
| `WithWorkers(n int)` | `runtime.GOMAXPROCS(0)` | goroutines started |

Sentinel errors: `ErrX` (when it is returned) · `ErrPanic` (wraps a recovered panic with its value and stack).

## Per export

### `TB-<SHORT>-<NNN>` `Name`
- **Pattern**: the textbook name (the `Pattern:` line).
- **Authority**: author, title, edition or site, year, then the narrowest locator (chapter, section heading,
  pkg.go.dev symbol, RFC section). `.claude/rules/sources.md`; recorded in `doc/sources/<source>.md`.
- **Familiar shape**: the standard-library or best-known API it mirrors (`errgroup.Group.Go`, `rate.Limiter.Wait`)
  and any deliberate difference, with why.
- **Goroutines**: how many are started, and what stops each; all have exited when <the call / Wait / Close> returns.
- **Cancellation**: what happens when ctx is cancelled before start, mid-stream and after completion.
- **Errors**: first error wins (and cancels the rest) or all are collected (`errors.Join`); each sentinel and the
  input that produces it; a panic in caller code is `ErrPanic`.
- **Blocking**: which calls block, on what, and the backpressure rule (no silent drops).
- **Ordering and counts**: input order or completion order; exactly-once inside the process or not.
- **Zero value**: ready to use, or create with `New…`.
- **Clock**: `WithClock` where it waits or measures, so tests use a fake clock or `testing/synctest`.
- **calcbench IDs called**: `ALG-…`, `NUM-…` (the `Math:` line), or `none`; if it re-implements something
  calcbench has, why (lock-free, channel-based, concurrent-safe).
- **Compared against**: the library it replaces (`golang.org/x/sync/errgroup`, …) and the comparison test in
  `compare/`, or `none`.
- **Worked example**: the `Example<Name>` function and its `// Output:`.

## Tests (`.claude/rules/go-testing.md`)

Example · happy path · nil and closed channels, zero / one / many items, workers = 1, slow consumer, slow
producer · cancel before, during, after · every sentinel and `ErrPanic` · leak check (`internal/leak`, in
`TestMain`) · ordering and count guarantees · benchmark with `b.ReportAllocs()` if on a hot path.

## Out of scope

What this package does not do, and where it lives if it is ever done (another package, an adapter, the
caller).
