# Wave 4 progress

| Step | Owner | Status |
|---|---|---|
| Wave folder, BRIEF, taxonomy rows (planned) | lead | done |
| B5 schedule: spec (Cron PENDING calcbench R-3), code (Every, At, Run, Heartbeat), tests | builder | done (TB-SCHED-001..011; D-024) |
| B6 lifecycle: spec, code, tests, compare | builder | done (TB-LIFE-001..011; D-025) |
| B7 pubsub: spec, code, tests, compare | builder | done (TB-PUBSUB-001..010; D-026; no compare: watermill too heavy) |
| Verify: INDEX regenerated, taxonomy built, go test -race | lead | done (90 \| 90 +1 REMOVED \| 90; 14 packages ok; wave 4 -count=50 ok) |
| compare/: go mod tidy (compare only), go test -race | lead | done (+oklog/run v1.2.0; main go.mod/go.sum sha256 unchanged; 9 packages ok) |
| Reviewer on schedule, lifecycle, pubsub | reviewer | done: 0 blockers, 3 should-fix, 9 nits |
| Fixes | builders + lead | done: pubsub Option[M]; Heartbeat keeps newest; Run returns Cause; drift; docs; D-024, D-026, D-027 |
