# Brief for the matrix readers (align, 2026-11-13)

Request (Devin, 2026-11-13), PART 1 verbatim:

PART 1 -- THE MATRIX (read-only; four background readers at once, each capped at 30 minutes; a reader not done
hands back what remains and is relaunched)
Lead first: doc/align_2026-11-13/ with README.md, BRIEF.md (this section verbatim, the split), PROGRESS.md.
Split by toolbench area, so each reader judges OUR code against THEIR code for the same pattern:
  M1 panics, clock, ctxkey, group, flow, semaphore, guard, recycle
  M2 pool, parallel, batch, schedule, lifecycle, pubsub
  M3 retry, singleflight, ratelimit, breaker, bulkhead
  M4 cache, config, logging, and every PLANNED pattern (LADDER.md waves 7+: lock, txn, page, store, jobs, outbox,
     idempotency, blob, mail, httpx, grpcx)
For each pattern in its area a reader:
 1. Starts from the rows in LADDER.md and LADDER2 A1-A4.md, then GREPS BOTH clone trees itself for the pattern's
    fingerprints (e.g. `go func` + `sync.WaitGroup` + a jobs channel for worker pools; `time.Sleep` inside a
    retry loop; `signal.Notify`; `sync.Mutex` beside a map; `rate.NewLimiter`; `context.WithTimeout`), so the
    count is measured, not inherited. Records REPEAT = number of distinct repos per list and in total, with up
    to five paths.
 2. Reads the three most complete implementations and the three most broken ones across both lists.
 3. Reads OUR implementation (file:line) and gives one verdict:
    SAME (delivers everything the best reference does) · NEAR (differs only in naming or an option nobody needs;
    say what) · PARTIAL (a real capability, edge case or failure mode a reference handles and ours does not;
    name it) · MISSING (general pattern, not in toolbench, not planned) · PLANNED (waves 7+; list what the
    references add to the plan) · NOT-GENERAL (belongs to a specialist or the framework; one line).
 4. SCALE: how the pattern behaves under load in the references and in ours: bounded memory yes/no, contention
    point, per-call allocations, what breaks first at 10x and 100x the callers. Evidence from the code, and for
    ours from the existing benchmarks; no new measurement in Part 1.
 5. Lists the reference defects not already covered by one of our tests (they become tests in Part 2).
Rows go to doc/align_2026-11-13/<M#>.md:
pattern | toolbench IDs | REPEAT code-heim / Akhil / total | best references (paths) | VERDICT | what ours lacks
(or "nothing") | SCALE notes | uncovered defects -> tests | authority | calcbench IDs.
LEAD MERGE -> doc/align_2026-11-13/MATRIX.md: every pattern once, sorted by total REPEAT; then four lists:
(a) REPEATED AND DELIVERED (SAME/NEAR) -- the proof the base is right; (b) REPEATED AND PARTIAL -- the build
list, ranked by REPEAT then by scale impact; (c) MISSING general patterns; (d) one-off patterns (REPEAT 1-2)
that are NOT built, with the reason. A PARTIAL or MISSING item enters the build list only if REPEAT >= 3 across
the two lists OR an authority names it as standard OR it fixes a scale limit in what we ship.

## Hard limits for a reader

- Read-only. The only file you write is your own `doc/align_2026-11-13/<M#>.md`. No edits elsewhere, no go
  build/vet/test/run/mod in the clones, no git, no background processes, no `time.Sleep`.
- Clone trees: `_reference/code-heim/<repo>` (81 repos) and `_reference/AkhilSharma90/<repo>` (92 with Go code).
  Never open any `.env`, `.env.*`, `app.env`, `accounts.json`, `vendor/`, `node_modules/`. Never copy a line of
  their code or a secret into your file; cite `repo/path:line` only. LADDER2 flags live-looking tokens in
  GO-Slackbot-Calculates-Age main.go:25-26, GO-SlackBot-Uploads-File main.go:12 and
  Golang-MySQL-CRUD-Bookstore-Management-API pkg/config/app.go:13: do not open those lines.
- Our code: the package folders at the repo root, `INDEX.md`, `doc/specs/<package>.md` (the contract and the
  tests already written), `<package>/*_test.go` (what is already covered), `compare/` (comparison tests),
  benchmarks in the `_test.go` files (`Benchmark...`) for the scale notes.
- Authority per `.claude/rules/sources.md`; calcbench IDs from `/Users/devinanderson/Desktop/calcbench/INDEX.md`
  (read-only). Today's Go is go1.27 (`doc/ladder_2026-11-13/GO_RELEASES.md`).
- Grep counts are per DISTINCT REPO (a repo with five hits counts once). Use Grep with `*.go` globs and the
  exclusions above. Record the fingerprint you grepped so the lead can repeat it.
- Time cap: 30 minutes. Write rows incrementally. If not done, end the file with `REMAINING: <patterns>`.
- Final message to the lead (at most 15 lines): patterns done / remaining, verdict counts, the three findings
  that most change what Part 2 should build.

## The split

| reader | file | patterns (toolbench packages) |
|---|---|---|
| M1 | `M1.md` | panics, clock, ctxkey, group, flow, semaphore, guard, recycle |
| M2 | `M2.md` | pool, parallel, batch, schedule, lifecycle, pubsub |
| M3 | `M3.md` | retry, singleflight, ratelimit, breaker, bulkhead |
| M4 | `M4.md` | cache, config, logging, and every PLANNED pattern: lock, txn, page, store, jobs, outbox, idempotency, blob, mail, httpx, grpcx |

A "pattern" is one row: a package may hold several (flow: generator, stage, fan-out, merge, or-done, tee, bridge,
priority/weighted merge; lifecycle: run group, graceful stop, signal hook, health; retry: backoff, jitter,
per-attempt timeout, permanent errors; pubsub: bus, drop policy, headers; and so on). Give the sub-patterns
their own rows when the references treat them separately.
