# align (2026-11-13): the matrix of both reading lists against toolbench, and the gaps closed

Request: Devin, 2026-11-13 (PART 1 verbatim in `BRIEF.md`). Question answered: across code-heim (81 repos) and
AkhilSharma90 (92 repos read), which patterns REPEAT, which matter AT SCALE, and does toolbench deliver each the
same or as near as makes no difference; then close the general gaps.

| File | What it is |
|---|---|
| `BRIEF.md` | PART 1 of the request verbatim, the hard limits, the split M1-M4 |
| `PROGRESS.md` | Re-runnable state: what is done, what is running, what is next |
| `BUILD_BRIEF.md` | The brief every Part 2 builder followed (ownership, the standard per change, the no-test rule) |
| `M1.md` .. `M4.md` | Reader rows, one per pattern, by toolbench area |
| `MATRIX.md` | The merge: every pattern once by REPEAT; lists (a) delivered, (b) build, (c) missing, (d) one-offs |
| `DEFERRED.md` | What is NOT built in this run and where each item lands |
| `calcbench_requests.md` | Math and data structures calcbench lacks (from LADDER2 §7, checked) |
| `api_before.txt`, `api_after.txt` | `go doc -all` per touched package before and after (the API diff) |
| `out/` | raw test, benchmark and compare output, INDEX.before/final (gitignored) |

LEFT FOR DEVIN: two go1.27.3 internal compiler errors met while compiling pool's test binary (ASSUMPTIONS 2026-11-13, pool): (1) a `ctxkey.Key[*poolID]` field with an unexported pointer type argument, read from an external test package (`./ctxkey/key.go: internal compiler error: ... nil pointer dereference`); (2) escape analysis crashing (`iter.go:223`) on the `for b.Loop()` body of `BenchmarkPool_SubmitFromWorker` (a generic method call with an argument received from a channel, then `b.ReportMetric`). Both reproduce with `go test -c ./pool` at the pre-fix revisions described there; minimal programs are not yet isolated.
