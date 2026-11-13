# Wave 1 progress

| Step | Owner | Status |
|---|---|---|
| Wave folder, BRIEF, taxonomy rows (planned) | lead | done |
| calcbench_requests.md | lead | done (R-1 backoff, R-2 CeilDiv, R-3 cron) |
| B1 panics: spec, code, tests, compare | builder | done (TB-PANIC-001..004; consumer note dropped per Devin) |
| B2 clock: spec, code, tests, compare | builder | done (TB-CLOCK-001..005) |
| B3 ctxkey: spec, code, tests | builder | done (TB-CTXKEY-001 Key, -002 New; consumer note dropped per Devin) |
| compare/ module (go.mod, replace ../) | lead | go.mod written; dependencies blocked (`go mod tidy` not permitted) |
| Verify: INDEX regenerated, taxonomy built, go test -race | lead | done (11 IDs; all packages ok) |
| Reviewer on panics, clock, ctxkey | reviewer | done: 0 blockers, 3 should-fix, 7 nits |
| Fixes | builders + lead | done: all 10 fixed (clock AdvanceContext D-012; panicnil test; DECISIONS D-006..D-012; 6 source records); retest clock, panics, ctxkey ok; index 11 \| 11 \| 11 |
