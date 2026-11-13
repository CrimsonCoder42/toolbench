# Brief for the builders (align_2026-11-13, PART 2)

You are one builder, one package, 30 minutes, additive changes only. Repo root /Users/devinanderson/Desktop/toolbench.
Read first, in this order: `.claude/CLAUDE.md`, `.claude/rules/go-functions.md`, `.claude/rules/go-concurrency.md`,
`.claude/rules/go-testing.md`, `.claude/rules/sources.md`, then your package's `doc/specs/<package>.md`, its
`doc.go`, every source and test file in it, and the rows for your package in `doc/align_2026-11-13/MATRIX.md`
(and the reader file it points to) plus the LADDER2 sections your task names.

## Hard rules
- You own ONLY: `<package>/` (new files and additive edits to existing ones) and `doc/specs/<package>.md`.
  Never edit INDEX.md, doc/taxonomy.md, doc/DECISIONS.md, doc/ASSUMPTIONS.md, doc/CHANGELOG.md, go.mod, `.claude/`,
  another package, or `compare/` unless your task says a compare test is wanted (then only `compare/<package>/`).
- RELEASED SIGNATURES NEVER CHANGE: no renamed export, no changed parameter or result, no removed export, no
  changed default behaviour that a test already pins. New capability = new export with the next free ID for
  your SHORT (highest NNN in INDEX.md + 1, never reused) or a new option.
- The ONLY commands you run: `gofmt -l <files>`, `go build ./<package>/...`, `go vet ./<package>/...`. You write
  tests and benchmarks and NEVER run them (`go test` is the lead's, once, with -race). No background processes,
  no git, no edits in `_reference/`, never open `.env`/`vendor`/`node_modules`, never copy reference code.
- Standard library and calcbench only in core. Time comes from `clock.Clock` where anything waits.
- Nothing panics on input. Every goroutine's end is stated. Every buffer is bounded. No `time.Sleep` in tests.

## The standard per change (in this order)
1. `doc/specs/<package>.md` gains a dated section `## Align (2026-11-13)` FIRST: the matrix row (pattern, REPEAT,
   verdict), the references read (repo/path:line; a reading list, never cited as authority), the authority
   opened and cited to the section (`.claude/rules/sources.md`; say "not opened" when a book is only named),
   today's Go (what go1.27 gives), calcbench IDs or none, each new export with its ID and contract (goroutines,
   cancel, errors, blocking, zero value), the scale note (bounded memory yes/no, contention point, per-call
   allocations, what breaks first at 10x and 100x callers) and how the change addresses it, a "For the lead"
   list (decisions D-needed, assumptions, sources files to add, INDEX rows).
2. The code: one concept per file; doc comments in the EXACT shape of `.claude/rules/go-functions.md` (ID,
   Pattern, Authority, Goroutines, Cancel, Errors, Blocking, Math, Tags; the labels the index generator reads);
   the package `doc.go` export list gains one line per new ID (same format as the existing lines).
3. Tests beside the code: `Example<Name>` with `// Output:`; happy path and edge cases; cancel before, during,
   after; every sentinel and ErrPanic; `leak.Verify(t)` (the package's TestMain already has `leak.Main`); fake
   clock where anything waits; the uncovered reference defects from your matrix rows, each as a named test
   `Test<Export>_<defect>` with a comment naming repo/path:line; a benchmark with `b.ReportAllocs()` for
   anything on a hot path. Every test bounded (`context.WithTimeout` <= 10 s).
4. `gofmt -l`, `go build`, `go vet` clean on your package.
5. Final message to the lead (at most 15 lines): exports added (ID, name, file:line), files touched, tests
   written (count, the reference-defect tests by name), decisions/assumptions for the ledgers, anything you
   could not finish.
