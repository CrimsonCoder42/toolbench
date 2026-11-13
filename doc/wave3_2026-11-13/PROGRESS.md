# Wave 3 progress

| Step | Owner | Status |
|---|---|---|
| Wave folder, BRIEF, taxonomy rows (planned) | lead | done |
| B1 recycle: spec, code, tests, compare | builder | done (TB-RECYC-001..005; no §4 deviation) |
| B2 pool: spec, code, tests, compare | builder | done (TB-POOL-001..007; D-022) |
| B3 parallel: spec, code, tests, compare | builder | done (TB-PAR-001..003; no §4 deviation) |
| B4 batch: spec, code, tests | builder | done (TB-BATCH-001..005; D-021) |
| Verify: INDEX regenerated, taxonomy built, go test -race | lead | done (58 \| 58 +1 REMOVED \| 58; all ok) |
| compare/: go mod tidy (compare only), go test -race | lead | done (+bytebufferpool v1.0.0; main go.mod/go.sum sha256 unchanged; 8 packages ok) |
| Reviewer on recycle, pool, parallel, batch | reviewer | done: 0 blockers, 4 should-fix, 3 nits |
| Fixes | builders + lead | done: pool fail-fast count + doc + citations; ExampleErrSize; parallel doc.go; D-023; recycle race-skip in ASSUMPTIONS |
