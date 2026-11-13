# Requests for calcbench (from toolbench, 2026-11-13)

Math toolbench needs that calcbench v0.1.0 lacks (doc/ladder_2026-11-13/LADDER.md §5, checked against
/Users/devinanderson/Desktop/calcbench/INDEX.md on 2026-11-13). Nothing is built for them in toolbench; each
blocks the toolbench package named. Shape: calcbench `doc/specs/TEMPLATE.md` (ID left for calcbench to assign,
signature, source, worked example, errors). Suggested homes follow calcbench's taxonomy; calcbench decides.

## R-1 Exponential backoff with jitter

| ID | Name | Signature | Units |
|---|---|---|---|
| ALG-?-? (suggested `algo/backoff`) | `Exponential` | `func Exponential(attempt int, base, max time.Duration) (time.Duration, error)` | attempt ≥ 0 (0 = first retry); durations in, duration out |
| ALG-?-? | `FullJitter` | `func FullJitter(attempt int, base, max time.Duration, u float64) (time.Duration, error)` | u ∈ [0, 1) supplied by the caller (no RNG inside) |
| ALG-?-? | `EqualJitter` | `func EqualJitter(attempt int, base, max time.Duration, u float64) (time.Duration, error)` | as above |
| ALG-?-? | `DecorrelatedJitter` | `func DecorrelatedJitter(prev, base, max time.Duration, u float64) (time.Duration, error)` | prev = the previous sleep (base on the first call) |

- **Source**: Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015-03-04 (the "Full Jitter",
  "Equal Jitter" and "Decorrelated Jitter" formulas: `sleep = min(cap, base·2^attempt)`; full:
  `random_between(0, sleep)`; equal: `sleep/2 + random_between(0, sleep/2)`; decorrelated:
  `min(cap, random_between(base, prev·3))`); gRPC, "Connection Backoff Protocol", grpc/grpc doc/connection-backoff.md
  (multiplier 1.6, jitter 0.2, the cap).
- **Approach**: integer nanoseconds; `base·2^attempt` saturates at `max` without overflow (shift checked against
  the leading-zero count, as ALG-BITS-003 does). The random draw is the caller's `u`, so the function is pure and
  testable; toolbench draws u from math/rand/v2.
- **Worked example**: base 100 ms, max 10 s, attempt 3 → Exponential 800 ms; FullJitter with u = 0.5 → 400 ms;
  EqualJitter with u = 0.5 → 600 ms; attempt 10 → Exponential 10 s (capped, 102.4 s uncapped);
  DecorrelatedJitter(prev 300 ms, base 100 ms, max 10 s, u 0.5) → 100 ms + 0.5·(900 ms − 100 ms) = 500 ms.
- **Errors**: attempt < 0, base ≤ 0, max < base, u ∉ [0, 1) → a calcbench sentinel each.
- **Needed by**: toolbench `retry` (wave 5), and through it `lock.Acquire`, `jobs`, `httpx.RetryTransport`.

## R-2 Overflow-safe integer ceiling division

| ID | Name | Signature | Units |
|---|---|---|---|
| NUM-INTEG-? (suggested `num/integer`) | `CeilDiv` | `func CeilDiv(a, b int64) (int64, error)` | integers |

- **Source**: Graham, Knuth, Patashnik, Concrete Mathematics, 2nd ed., 1994, §3.1–3.2 (⌈a/b⌉ = ⌊(a + b − 1)/b⌋ for
  b > 0, and the identity ⌈x⌉ = −⌊−x⌋); Go spec, "Integer operators" (truncated division).
- **Approach**: `q, r := a/b, a%b; if r != 0 && (r > 0) == (b > 0) { q++ }` — no `a + b − 1` overflow, exact for
  every int64 except the one overflowing case.
- **Worked example**: CeilDiv(10, 3) = 4; CeilDiv(9, 3) = 3; CeilDiv(0, 5) = 0; CeilDiv(−7, 2) = −3;
  CeilDiv(math.MaxInt64, 1) = MaxInt64; CeilDiv(2^53 + 1, 1) = 2^53 + 1 (the float `math.Ceil` route gives 2^53).
- **Errors**: b = 0 → ErrDivByZero; a = MinInt64, b = −1 → ErrOverflow.
- **Needed by**: toolbench `page.Count` (wave 7).

## R-3 Cron next-fire time

| ID | Name | Signature | Units |
|---|---|---|---|
| ALG-?-? (suggested a time/calendar package) | `ParseCron` | `func ParseCron(spec string) (CronSpec, error)` | five fields: minute hour day-of-month month day-of-week |
| ALG-?-? | `CronSpec.Next` | `func (c CronSpec) Next(after time.Time) (time.Time, error)` | wall time in `after.Location()` |

- **Source**: IEEE Std 1003.1-2024 (POSIX), "crontab" utility, INPUT FILES (field ranges, lists, ranges, `*`; when
  both day-of-month and day-of-week are restricted, either matching fires); Go `time` package for zone rules.
- **Approach**: field bitsets; step minute → hour → day → month with carries; DST: a wall time that does not
  exist (spring-forward gap) fires at the first instant after the gap; a wall time that occurs twice (fall-back)
  fires once, at the first occurrence. State which. No iteration over every minute.
- **Worked example**: "30 2 * * *" in America/New_York after 2026-03-07T03:00-05:00 → 2026-03-08T03:00-04:00
  (02:30 does not exist that day); "0 10 * * 1-5" after Friday 2026-11-13T10:00Z (UTC) → Monday
  2026-11-16T10:00Z; "0 0 31 2 *" → ErrNeverFires.
- **Errors**: malformed field, value out of range, a spec that can never fire (Feb 31) → sentinels.
- **Needed by**: toolbench `schedule.ParseCron` (wave 4). Devin decision 12 (D-006): requested from calcbench.
