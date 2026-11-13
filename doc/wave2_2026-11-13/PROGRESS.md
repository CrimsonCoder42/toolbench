# Wave 2 progress

| Step | Owner | Status |
|---|---|---|
| Wave folder, BRIEF, taxonomy rows (planned) | lead | done |
| B4 group: spec (settles LADDER §9 item 14), code, tests, compare | builder | done (TB-GROUP-001..007; D-016, D-017: errgroup v0.25.0 does not re-panic) |
| B5 flow: spec, code, tests, compare | builder | done (TB-FLOW-001..015, 014 REMOVED; D-018..D-020) |
| B6 semaphore: spec, code, tests, compare | builder | done (TB-SEMA-001 Weighted, -002 ErrTooLarge, -003 ErrNegativeWeight; D-015) |
| B7 guard: spec, code, tests | builder | done (TB-GUARD-001 Guarded, -002 Confine; D-013, D-014) |
| Verify: INDEX regenerated, taxonomy built, go test -race | lead | done (38 \| 38 +1 REMOVED \| 38; all packages ok; wave 2 -count=5 ok) |
| compare/: go test -race -timeout 10m ./... | lead | done: tidy authorised once by Devin; 5 packages ok after two test-script fixes (no code bug) |
| Reviewer on group, flow, semaphore, guard | reviewer | done: 2 blockers (guard Confine dropped request; flow Priority order), 4 should-fix, 3 nits |
| Fixes | builders + lead | done: all fixed; semaphore test + New ID; guard -count=200 ok; flow -count=50 ok; D-018..D-020 |
