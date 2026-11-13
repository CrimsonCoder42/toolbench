# Foundation (2026-11-13)

Track A of `doc/requests/foundation_2026-11-13.md`: the module's skeleton and its index tooling. No pattern is
built and no ID is given. Track B (the pattern ladder) is `doc/ladder_2026-11-13/`.

| Step | File | Model |
|---|---|---|
| A1 | `go.mod` (`go 1.27`, `require github.com/CrimsonCoder42/calcbench v0.1.0`), `go.sum` | calcbench `go.mod` |
| A2 | `doc.go` (`package toolbench`, layer table) | CLAUDE.md opening |
| A3 | `.gitignore` (added `compare/vendor/`, `**/node_modules/`) | request list |
| A4 | `internal/calccheck/calccheck_test.go` (NUM-STATS-001 on 1, 2, 3 = 2) | - |
| A5 | `internal/leak/leak.go`, `leak_test.go` (Take, Extra, Check, Verify, Main; runtime/pprof debug=2) | go-testing.md rule 5 |
| A6 | `INDEX.md` (header only), `testing/index_test.go`, `gen_index.go` (this folder) | calcbench `testing/index_test.go`, D-070..D-072, `doc/calculator_net_check_2026-11-13/gen_index.go` |
| A7 | `doc/specs/TEMPLATE.md` | calcbench `doc/specs/TEMPLATE.md` |
| A8 | `doc/sources/code-heim.md`, `doc/sources/calcbench.md` | calcbench `doc/sources/*.md` |

## The index tooling

`testing/index_test.go` reads files only and checks, on every `go test`:
- `TestIndex_docCommentShape`: every exported function and type has an `ID:` (methods and `New<T>` covered as in
  calcbench D-071), `ID`, `Pattern`, `Authority`, `Tags` present, labels in the go-functions.md order (ID, Pattern,
  Authority, Goroutines, Cancel, Errors, Blocking, Math, Tags), SHORT equal to the package doc.go
  `ID short form:` line; the root package exports nothing.
- `TestIndex_noDuplicateIDs`.
- `TestIndex_indexAgreesWithCode`: INDEX.md has the CLAUDE.md header, one row per ID, sorted, every cell equal to
  the doc comment (Pattern, Goroutines, Authority, Tags; "-" when absent) and the doc.go line (Does).
- `TestIndex_docGoAgreesWithCode`: every ID listed once in its own package's doc.go under the same name.
- `TestIndex_taxonomyAgreesWithTree`: every package on disk is a built row with the same SHORT, every built row
  exists, Status is built or planned, and no top-level folder (or `adapters/<driver>`) exists without a row.
- `TestIndex_counts`: logs `exports | INDEX rows | doc.go entries | packages`.
Outside the index: `internal/`, `compare/`, `testing/`, `doc/`, folders starting with `_` or `.`.

Regenerate INDEX.md after adding exports: `go run doc/foundation_2026-11-13/gen_index.go <out>`, compare `<out>`
with INDEX.md, then move it into place. (Not executed in this run: `go run` is outside the session's
permissions; the file was vetted with `go vet doc/foundation_2026-11-13/gen_index.go`.)

## Checks (2026-11-13, go1.27.3 darwin/arm64)

```
gofmt -l doc.go internal testing doc/foundation_2026-11-13   (no output)
go build -mod=mod ./...                                      ok
go vet ./...                                                 ok
go test -race -timeout 5m ./...
?   	github.com/CrimsonCoder42/toolbench	[no test files]
ok  	github.com/CrimsonCoder42/toolbench/internal/calccheck	1.462s
ok  	github.com/CrimsonCoder42/toolbench/internal/leak	1.926s
ok  	github.com/CrimsonCoder42/toolbench/testing	1.634s
TestIndex_counts: exports 0 | INDEX rows 0 (+0 REMOVED) | doc.go entries 0 | packages 0
```

`gofmt -l .` also lists files under `_reference/code-heim/` (the read-only clones); they are outside the module
(`_` prefix) and never edited.
