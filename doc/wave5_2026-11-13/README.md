# Wave 5 (2026-11-13): calcbench v0.2.0, schedule.ParseCron, resilience

STEP 0 bumps calcbench to v0.2.0 (algo/backoff, algo/cron, num/integer CeilDiv; contract:
`/Users/devinanderson/Desktop/calcbench/doc/requests/toolbench_reply_2026-11-13.md`). Then `schedule.ParseCron`
and the T3 packages `retry`, `singleflight`, `ratelimit`, `breaker`, built under the package standard
(`doc/wave3_2026-11-13/BRIEF.md`) from LADDER.md ranks 24-27 and 29.

| File | What it is |
|---|---|
| `BRIEF.md` | STEP 0 result, the calcbench contract, the agreed APIs for wave 5 |
| `PROGRESS.md` | What is finished (a rerun skips it) |
| `out/` | Raw test output (gitignored) |

Specs: `doc/specs/schedule.md` (ParseCron added), `doc/specs/retry.md`, `doc/specs/singleflight.md`,
`doc/specs/ratelimit.md`, `doc/specs/breaker.md`.
