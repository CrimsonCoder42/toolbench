# Wave 5 progress

| Step | Owner | Status |
|---|---|---|
| STEP 0: go.mod calcbench v0.2.0, go mod tidy (main + compare), build, vet, go test -race | lead | done (go.mod sha256 a12ebe1b…, go.sum e6b44175…; all ok; compare ok) |
| Wave folder, BRIEF, taxonomy rows (planned) | lead | done |
| B0 schedule.ParseCron (ALG-CRON-001/002) | builder | done (TB-SCHED-012 schedule/cron.go:25; D-028) |
| B1 retry: spec, code, tests, compare | builder | done (TB-RETRY-001..018; D-030) |
| B2 singleflight: spec, code, tests, compare | builder | done (TB-FLIGHT-001..003; D-029) |
| B3 ratelimit: spec, code, tests, compare | builder | done (TB-RATE-001..014; D-031; calcbench R-4, R-5) |
| B4 breaker: spec, code, tests, compare | builder | done (TB-BRK-001..011; D-032) |
| Verify: INDEX regenerated, taxonomy built, go test -race | lead | done (139 \| 139 +1 REMOVED \| 139; 18 packages ok; wave 5 -count=50 ok) |
| compare/: go mod tidy (compare only), go test -race | lead | done (+x/time v0.16.0, cenkalti/backoff/v4 v4.3.0, gobreaker/v2 v2.4.0; main hashes unchanged; 13 packages ok) |
| Reviewer (timing claims on the fake clock) | reviewer | done: 1 blocker (ratelimit out-of-order clock), 2 should-fix, 3 nits; one wall-time wait accepted in compare/breaker only |
| Fixes | builders + lead | done: ratelimit debt instant + TB-RATE-015/016; breaker hook cap + compare header; specs SHIPPED |
