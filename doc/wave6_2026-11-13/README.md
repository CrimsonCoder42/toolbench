# Wave 6 (2026-11-13): calcbench v0.3.0, ratelimit onto calcbench, bulkhead, cache, config, logging

STEP 0 bumps calcbench to v0.3.0 (ALG-RATELM-005 TokenBucket.Restore, ALG-LIST-018 LRUCache.Oldest, answering
toolbench requests R-4 and R-5). Then `ratelimit` moves onto those methods (signatures unchanged), and the T3-T4
packages `bulkhead`, `cache`, `config`, `logging` are built under the package standard.

| File | What it is |
|---|---|
| `BRIEF.md` | STEP 0 result, extra duties, the agreed APIs for wave 6 |
| `PROGRESS.md` | What is finished (a rerun skips it) |
| `out/` | Raw test output (gitignored) |

Specs: `doc/specs/ratelimit.md` (updated), `doc/specs/bulkhead.md`, `doc/specs/cache.md`, `doc/specs/config.md`,
`doc/specs/logging.md`.
