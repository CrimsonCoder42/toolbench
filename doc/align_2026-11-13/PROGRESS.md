# PROGRESS (align_2026-11-13)

Re-run from the first unchecked step. Times are local, 2026-11-13.

- [x] Work folder, BRIEF.md, README.md, PROGRESS.md
- [x] PART 1: readers M1-M4 done -> M1.md..M4.md (93 rows, none REMAINING)
- [x] PART 1: no relaunch needed
- [x] PART 1: lead merge -> MATRIX.md (R-9 added to calcbench_requests.md; LADDER2 corrected)
- [x] PART 2: api_before.txt (guard, lifecycle, pool, retry, pubsub, ctxkey, group)
- [x] PART 2: builders D1 guard, D2 lifecycle, D3 pool, D4 retry (batch 1)
- [x] PART 2: builders D5 pubsub, D6 ctxkey, group (batch 2)
- [x] PART 2: DEFERRED.md, calcbench_requests.md, BUILD_BRIEF.md
- [x] VERIFY: gofmt/build/vet; INDEX.md regenerated (242 rows); go test -race ./... 26 ok (after 3 lead fixes: ExampleKeyed race, pool ICE workarounds, lifecycle doc.go method names); compare 14 ok; API diff 0 changed, 36 added; benchmarks in out/
- [x] VERIFY: reviewer: 0 BLOCKER, 2 MUST (group error compare, guard TryDo mutex leak) + 8 SHOULD, all fixed by the owning builders once; final run 26 ok + compare 14 ok under -race; INDEX regenerated; benchmarks re-run (out/bench_final.txt)
- [x] CLOSE: CHANGELOG, DECISIONS D-041..052, ASSUMPTIONS, TODO, sources written
- [x] CLOSE: /cleanup (index 241|241|241, taxonomy ok, sources cover every new ID, scratch moved to _delete/)
- [x] FINAL REPORT
