# Wave 6 progress

| Step | Owner | Status |
|---|---|---|
| STEP 0: calcbench v0.3.0, go mod tidy (main + compare), build, vet, go test -race | lead | done (go.mod sha256 c889b165…, go.sum c7cc2938…; all ok; compare ok) |
| Wave folder, BRIEF, taxonomy rows (planned) | lead | done |
| B0 ratelimit switch (ALG-RATELM-005, ALG-LIST-015/018) | builder | done (D-035; 6 pre-edit files saved in _delete/) |
| B1 bulkhead | builder | done (TB-BULK-001..013; D-036) |
| B2 cache | builder | done (TB-CACHE-001..016; D-038) |
| B3 config | builder | done (TB-CONFIG-001..021; D-037) |
| B4 logging | builder | done (TB-LOG-001..010; D-039) |
| Verify: INDEX, taxonomy built, go test -race | lead | done (199 \| 199 +1 REMOVED \| 199; 22 packages ok; wave 6 -count=50 ok) |
| compare/: tidy (compare only), go test -race | lead | done (+golang-lru/v2 v2.0.7; main hashes unchanged; 14 packages ok) |
| Reviewer (no env/file/clock in core tests; no secret in any output) | reviewer | done: 1 blocker (cache stale load), 4 should-fix (duty 2 failure paths, D-039, NaN sweep, bulkhead pin), 8 nits; duty 1 clean |
| Fixes | builders + lead | done: cache per-key stale mark; failure messages masked; shared secret list; Recorder cap; ratelimit stale Cancel; bulkhead authority + pin note |
