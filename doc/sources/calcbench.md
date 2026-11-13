# calcbench (github.com/CrimsonCoder42/calcbench)

**What it is**: the mathematics module toolbench sits on: algorithms, data structures, numerical methods and
calculators, one export per ID. Every formula a toolbench pattern needs is CALLED from it by ID, never rewritten
here (decision D-003).

**Version pinned**: `require github.com/CrimsonCoder42/calcbench v0.3.0` in `go.mod`; hashes in `go.sum`
(`h1:t8HwlmB0uvMVl6yIS0V6Mbr307/6f4YioOsD2LRfkUY=`). The module's `go` line matches calcbench's (`go 1.27`).
A version bump is its own request; a toolbench pattern never relies on calcbench code that is not in the pinned
version.

**How to look up an ID**:
1. Open `/Users/devinanderson/Desktop/calcbench/INDEX.md` (read-only local copy; same content as the tag) and
   search by meaning: words, tags or the Does column. IDs are `<GROUP>-<SHORT>-<NNN>`, for example
   `NUM-STATS-001` (ArithmeticMean, package `num/stats`), `ALG-SEQ-004` (Chunk, `algo/seq`), `ALG-QUEUE-002`
   (CircularBuffer, `algo/queue`), `ALG-BLOOM-003` (OptimalParams, `algo/bloom`).
2. Find the code: `grep -rn "ID:      <the id>" /Users/devinanderson/Desktop/calcbench` (the pinned copy is in the
   module cache, `$(go env GOMODCACHE)/github.com/!crimson!coder42/calcbench@v0.1.0/`).
3. Import `github.com/CrimsonCoder42/calcbench/<package>` and cite the ID on the toolbench doc comment's `Math:`
   line.
4. Math calcbench lacks is NOT added to toolbench: it goes in the report as a request for calcbench (the current
   list: `doc/ladder_2026-11-13/LADDER.md` section 5).

**Concurrency**: calcbench types are documented as not safe for concurrent use (for example ALG-QUEUE-002
CircularBuffer, ALG-LIST-015 LRUCache, the algo/ratelimit meters). A toolbench pattern that shares one guards it
(a mutex named beside it, or confinement) and says so in its doc comment.

**Version history**: v0.1.0 from the foundation (2026-11-13); v0.2.0 from wave 5 STEP 0 (D-033: algo/backoff, algo/cron,
num/integer CeilDiv, answering R-1..R-3); **v0.3.0** since wave 6 STEP 0 (D-034: ALG-RATELM-005 TokenBucket.Restore,
ALG-LIST-018 LRUCache.Oldest, answering R-4, R-5). No open requests.

**Used by (IDs)**, every `Math:` line in toolbench (plus `internal/calccheck`, no ID, calling NUM-STATS-001):
- ALG-SAMPLE-003, ALG-SAMPLE-004 → TB-FLOW-010 Weighted.
- ALG-SEQ-004 → TB-BATCH-001 Run.
- NUM-PROB-059 → TB-SCHED-004 Run, TB-SCHED-008 WithJitter.
- ALG-CRON-001, ALG-CRON-002 → TB-SCHED-012 ParseCron.
- ALG-QUEUE-002 → TB-PUBSUB-003 Bus, TB-PUBSUB-007 WithDropOldest.
- ALG-BACKOF-001..004 → TB-RETRY-001 Do, TB-RETRY-002 DoValue, TB-RETRY-007..010 Exponential, FullJitter, EqualJitter,
  DecorrelatedJitter.
- ALG-RATELM-001 → TB-RATE-003 Limiter, TB-RATE-005 Keyed; NUM-INTEG-013 → TB-RATE-005 Keyed.
- ALG-RATELM-005 → TB-RATE-003 Limiter, TB-RATE-004 Reservation; ALG-LIST-015, ALG-LIST-018 → TB-RATE-005 Keyed (D-035).
- ALG-LIST-015, ALG-LIST-018 → TB-CACHE-002 Memory; ALG-BLOOM-001, ALG-BLOOM-003 → TB-CACHE-009 WithBloom, TB-CACHE-014.
