# Logging house format progress

| Step | Owner | Status |
|---|---|---|
| Request saved, work folder | lead | done |
| Builder: spec "House format" (S1-S7, file:line evidence), Text, Trace, ParseLevel, New, tests, doc.go, doc/standards/logging.md | builder | done (TB-LOG-011..021; 22 golden tests) |
| Verify: INDEX regenerated, go test -race | lead | done (210 IDs; all ok) |
| Reviewer on logging (+ byte-for-byte sample lines vs util/logger.go) | reviewer | done: 0 blockers, 2 should-fix, 4 nits; the three sample lines match util/logger.go except colour-by-option and caller-vs-wrapper source |
| Fixes | builder | done: Trace caller PC; empty module clears; Trace Blocking/Errors lines; 24 golden tests |
| Close: DECISIONS, ASSUMPTIONS, CHANGELOG, doc/README, /cleanup | lead | done (D-040; A-1..A-11; 210 \| 210 \| 210) |
