# schedule: run a task on a schedule (Every, At), and a non-blocking heartbeat

Status: SHIPPED (2026-11-13; ParseCron TB-SCHED-012 shipped in wave 5, `doc/wave5_2026-11-13/`)
Package: `schedule` · SHORT: `SCHED` · Layer: core · Tier: T2
Work folder: `doc/wave4_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #20
Builds on: `clock` (Clock, Timer; tests with Fake), `panics` (Try, ErrPanic), calcbench `num/prob` (NUM-PROB-059),
calcbench v0.2.0 `algo/cron` (ALG-CRON-001, ALG-CRON-002) ·
Decisions: see "For the lead" · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; a reading list, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `go_49_task_scheduling/main.go` | gocron v2 scheduler: DurationJob, CronJob, DailyJob, OneTimeJob, DurationRandomJob; event listeners log. | **Busy spin**: `for {}` blocks main (`:128-130`), burning a core. Test `TestRun_go49_noBusySpin` (Run blocks on the clock: exactly one timer arm per run; `Every(0)` returns at once, no timer, no task). **Signal goroutine + os.Exit** (`:118-126`): Run returns `context.Cause(ctx)` on cancel instead; `TestRun_go49_returnsOnCancel`. Errors of `NewJob` ignored (`:51-53`, `:58`). `DurationRandomJob` (`:105`) is our `WithJitter`. |
| `go_50_task_scheduling_part_2/goroutines_task_scheduling/main.go` | `runAtSpecificTime`: a goroutine on `<-time.After(time.Until(t))` (`:10`), signals on a `chan bool`. | **Uncancellable At goroutine**: no ctx; the goroutine and its timer live until t whatever the caller does. Test `TestRun_go50_atIsCancellable` (cancel while waiting for At: Run returns the cause, task not run, no goroutine left, `leak.Verify`). |
| `go_50_task_scheduling_part_2/gocron_faktory/main.go` | CronJob "0 10 * * *" pushing a Faktory job. | Same `for {}` and `os.Exit` (`:50-58`); `log.Fatal` inside the task (`:30`) kills the process: our task returns an error, a panic becomes ErrPanic (`TestRun_panicIsErrPanic`). Cron is ParseCron TB-SCHED-012 (below). |
| `go_37_background_work/checkin/process_jobs/worker.go` | gocraft/work `Report`: `time.Sleep(10s)` then `job.Checkin(...)` 360 times (`:96-99`). | **Sleep loop** for work pacing; the check-in is a heartbeat bound to a Redis client. Our `Heartbeat`'s `beat()` is the check-in; it never blocks the worker even when nobody reads (`TestHeartbeat_go37_beatNeverBlocks`). gocraft/work unmaintained since 2019 (LADDER §7). Global `redisPool`/`enqueuer` (`:16`, `:56`). |

### b. The authority

- Go standard library, package `time`. Opened: https://pkg.go.dev/time, 2026-11-13. Relied on: "type Ticker":
  "A Ticker holds a channel that delivers 'ticks' of a clock at intervals"; "func NewTicker": "The ticker will
  adjust the time interval or drop ticks to make up for slow receivers. The duration d must be greater than
  zero; if not, NewTicker will panic"; "func (*Ticker) Stop": "Stop does not close the channel". Our mapping:
  Run is a ticker that drops the instants a slow task missed (the "drop ticks" rule) and never panics on
  `d <= 0` (D-011: no panic on input; `Every(d <= 0)` never fires); Heartbeat drops pulses for a slow receiver
  as the Ticker does.
- Go 1.23 release notes, "Timer changes". Opened: https://go.dev/doc/go1.23, 2026-11-13. Relied on: "for any
  call to a Reset or Stop method, no stale values prepared before that call will be sent or received after the
  call", and unstopped timers are collectable. Run and Heartbeat use one `clock.Timer` each (go1.23 semantics,
  D-009) and `Reset` it between waits; a stale fire can never start an extra run.
- Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.5 "Heartbeats". NOT ONLINE: cited, not opened. The two
  heartbeat forms it names (a pulse at an interval, a pulse per unit of work) and the rule that sending a pulse
  uses `select` with `default` so the worker never blocks on a monitor that is not listening. Heartbeat
  implements both forms (interval > 0: at most one pulse per interval, only if `beat` was called in it;
  interval <= 0: a pulse per `beat`).
- IEEE Std 1003.1-2024 (POSIX.1-2024), Shell & Utilities, "crontab", INPUT FILES. Opened:
  https://pubs.opengroup.org/onlinepubs/9799919799/utilities/crontab.html, 2026-11-13 (wave 5). Relied on: "The
  first five fields shall be integer patterns that specify the following: 1. Minute [0,59] 2. Hour [0,23] 3. Day
  of the month [1,31] 4. Month of the year [1,12] 5. Day of the week ([0,6] with 0=Sunday)"; "Each of these
  patterns can be either an <asterisk> (meaning all valid values), an element, or a list of elements separated by
  <comma> characters. An element shall be either a number or two numbers separated by a <hyphen-minus> (meaning
  an inclusive range)"; and the day rule "if either the month or day of month is specified as an element or
  list, and the day of week is also specified as an element or list, then any day matching either the month and
  day of month, or the day of week, shall be matched". The page says nothing about the time zone. ParseCron
  (TB-SCHED-012) takes this syntax from calcbench ALG-CRON-001 unchanged; the day rule is calcbench's A-068
  (common cron: the month always restricts; dom and dow ORed only when both are not `*`), which agrees with the
  POSIX example but not its literal wording when the month is also restricted.

### c. Today's Go

`time.Ticker` fires at a fixed rate with no ctx, no error, no jitter, no panic recovery, panics on `d <= 0`,
and a slow receiver gets the ticks back to back (one buffered tick fires as soon as the slow run ends). A
one-shot is `time.AfterFunc`, which runs `f` on its own goroutine with no ctx and no way to wait for it. No
standard export runs a task with "one run at a time, skip missed, return the first error, recover panics,
cancel by ctx", nor a heartbeat that cannot block its worker. Hence the package. `iter` and `synctest` do not
change this; tests use `clock.Fake` (wave 1) so they need no bubble.

### d. calcbench (confirmed in calcbench INDEX.md, v0.1.0 pinned)

- **NUM-PROB-059 UniformQuantile** (`num/prob/uniform_quantile.go:24`): CALLED by the jitter of Run
  (`schedule/option.go:42`, `config.offset`): offset = UniformQuantile(u, −frac·interval, +frac·interval) with u from
  math/rand/v2 (`WithRand` injects the source; tests use a fixed PCG seed). `Math:` on WithJitter and Run.
- **ALG-HEAP-002/004/005**: NOT called. One Run serves one Schedule, so there is only ever one next-run time;
  a heap of next-run times belongs to a multi-job scheduler (out of scope; `jobs`, wave 8).
- **ALG-CRON-001 ParseCron, ALG-CRON-002 CronSpec.Next** (calcbench v0.2.0 `algo/cron`, the reply
  `calcbench/doc/requests/toolbench_reply_2026-11-13.md` to R-3): CALLED by TB-SCHED-012 (`schedule/cron.go`):
  ParseCron calls ALG-CRON-001 once; the Schedule's Next calls ALG-CRON-002 with `after.In(loc)`. Errors
  `cron.ErrSyntax`, `cron.ErrRange`, `cron.ErrNeverFires` are wrapped with `%w`, never re-declared.

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-SCHED-001` | `Schedule` | `type Schedule interface{ Next(after time.Time) time.Time }` |
| `TB-SCHED-002` | `Every` | `func Every(d time.Duration) Schedule` |
| `TB-SCHED-003` | `At` | `func At(t time.Time) Schedule` |
| `TB-SCHED-004` | `Run` | `func Run(ctx context.Context, s Schedule, task func(context.Context) error, opts ...Option) error` |
| `TB-SCHED-005` | `Heartbeat` | `func Heartbeat(ctx context.Context, interval time.Duration, opts ...Option) (beat func(), pulses <-chan time.Time)` |
| `TB-SCHED-006` | `Option` | `type Option func(*config)` |
| `TB-SCHED-007` | `WithClock` | `func WithClock(c clock.Clock) Option` |
| `TB-SCHED-008` | `WithJitter` | `func WithJitter(frac float64) Option` |
| `TB-SCHED-009` | `WithOnError` | `func WithOnError(f func(error)) Option` |
| `TB-SCHED-010` | `WithRand` | `func WithRand(r *rand.Rand) Option` (math/rand/v2) |
| `TB-SCHED-011` | `ErrSchedule` | `var ErrSchedule = errors.New("schedule: Next did not move forward")` |
| `TB-SCHED-012` | `ParseCron` | `func ParseCron(spec string, loc *time.Location) (Schedule, error)` |

| Option | Default | Bound it sets | Used by |
|---|---|---|---|
| `WithClock(c)` | `clock.Real()` (also for nil) | the time source of every wait | Run, Heartbeat |
| `WithJitter(frac)` | 0 (no jitter); NaN or < 0 is 0; > 1 is 1 | each wait shifted by at most ±frac·interval | Run |
| `WithOnError(f)` | nil: Run returns the first task error | f nil: the default | Run |
| `WithRand(r)` | nil: math/rand/v2 top-level `Float64` | the source of jitter's u | Run |

Sentinel errors: calcbench `cron.ErrSyntax`, `cron.ErrRange`, `cron.ErrNeverFires` (ParseCron, wrapped) ·
`ErrSchedule` (Run: a Schedule's Next returned a non-zero time not after the time it was
given; prevents a busy loop on a broken custom Schedule) · `panics.ErrPanic` (task or OnError panicked).

Deviations from LADDER.md §4: (1) **At** is new (the request adds it; §4 had only Every and ParseCron). (2)
**WithRand** and **ErrSchedule** are new, additive: the brief asks for an injectable jitter source, and a
Next that does not move forward must be an error, not a spin. (3) **ParseCron** is built as §4 has it (wave 5); its errors are calcbench's
sentinels wrapped, not new toolbench sentinels, and a nil loc is UTC (§4 left it open).

## Per export

### `TB-SCHED-001` `Schedule`
- **Pattern**: schedule (next-fire-time function), the shape of robfig/cron `Schedule.Next(time.Time) time.Time`.
- **Contract**: `Next(after)` returns the first run time strictly after `after`, or the zero Time for "no
  further runs". Must be safe to call from the Run goroutine (Run calls it from one goroutine only).
- **Zero value**: an interface; a nil Schedule passed to Run is a schedule with no runs (Run returns nil).
- **Example**: `ExampleSchedule` (a custom "every hour on the hour" schedule).

### `TB-SCHED-002` `Every`
- `Next(after) = after + d`. **d <= 0 (decided)**: never fires (`Next` returns zero), so Run returns nil at once
  without calling the task. Alternatives that lost: a minimum interval (a near-busy loop, the go_49 defect)
  and a panic like `time.NewTicker` (D-011 forbids a panic on input). Pure value, no goroutines.

### `TB-SCHED-003` `At`
- `Next(after) = t` when `t.After(after)`, else zero. **A t in the past (decided)**: Run never runs it and
  returns nil (cron semantics: a missed one-shot does not fire later). Alternative that lost: run at once
  (`time.After(time.Until(t))` with a negative duration, go_50), which makes "At yesterday" a silent "now".

### `TB-SCHED-004` `Run`
- **Pattern**: fixed-rate scheduler, one run at a time; an overrun re-phases from its end (unlike time.Ticker,
  which keeps its phase).
- **Authority**: pkg.go.dev time, "NewTicker" (drop ticks for slow receivers), "func (*Timer) Reset"; Go 1.23
  release notes, "Timer changes".
- **Familiar shape**: `for range ticker.C { task() }` with ctx, errors and panics; gocron/robfig `Schedule.Next`.
- **Goroutines**: none. The task runs on the caller's goroutine, so runs never overlap.
- **Time line (decided)**: first run at `s.Next(now)`. After a run scheduled at T ends at E:
  next = `s.Next(T)` (fixed rate, no drift) if it is after E; otherwise the run overran and the missed run
  times are SKIPPED: next = `s.Next(E)` (late, phase restarts at E, never a burst of catch-up runs). A
  zero next ends Run with nil.
- **Jitter**: each wait is shifted by UniformQuantile(u, −frac·I, +frac·I), I = next − (the time Next was
  given); the scheduled grid is not moved (jitter does not accumulate); a wait below 0 is 0.
- **Cancellation**: before: returns `context.Cause(ctx)`, task not run, Next not called. While waiting: the
  timer is stopped, returns the cause. During a task: the task sees ctx; a task error that is ctx's own
  error (errors.Is ctx.Err() or the cause) is not a task failure: Run returns `context.Cause(ctx)` and
  WithOnError does not see it (the lifecycle rule); any other task error is handled as below first. A cancelled ctx wins over a timer that fired at the same time.
- **Errors (decided)**: default (no WithOnError): the first task error stops Run and is returned unchanged.
  With WithOnError(f): f(err) is called on the Run goroutine and the schedule continues. A panic in task is a
  `*panics.PanicError` (errors.Is ErrPanic) handled the same way; a panic in f is returned as ErrPanic. A nil
  task panics when called → ErrPanic. A Next that does not move forward → `ErrSchedule` (wrapped with both
  times). Alternative that lost for the default: continue and drop errors silently (rule 7).
- **Blocking**: until ctx is done, the schedule ends, or an error stops it. One clock timer, Reset per wait.
- **Ordering and counts**: at most one run per scheduled time; never two at once; never a catch-up burst.
- **Clock**: WithClock; tests wrap `clock.Fake` to record each wait and advance exactly to it.
- **calcbench IDs called**: NUM-PROB-059 (`Math:`).
- **Worked example**: `ExampleRun` (`00:01:00 00:02:00 00:03:00 context canceled`).

### `TB-SCHED-005` `Heartbeat`
- **Pattern**: heartbeat (Cox-Buday ch.5).
- **Shape**: `beat` is called by the worker (the check-in of go_37); `pulses` is read by a monitor.
  interval > 0: one goroutine wakes every interval and sends one pulse (the timer's time) if `beat` was called
  since the last wake: a stalled worker stops the pulses. interval <= 0: each `beat` is forwarded as a pulse
  (time = clock Now), the per-unit-of-work form.
- **The named drop (rule 5 exception, D-024)**: `pulses` has a buffer of 1 holding the NEWEST pulse: a new pulse
  replaces an unread one (`select { case <-out: default: }; out <- t`, which cannot block because the
  goroutine is the only sender). Lossy by design (Cox-Buday heartbeats); no information is lost, the newest
  "alive at" survives, and a monitor reading late sees the latest time.
- **No drift**: interval wakes are on the grid call time + k·interval (next deadline = previous deadline +
  interval; wakes missed by a slow goroutine are skipped), not Reset(interval) after each wake.
  `beat` never blocks (an atomic store, or a non-blocking send on a 1-slot signal channel).
- **Goroutines**: 1 (none if ctx is already done); it exits when ctx is done, stops its timer, closes `pulses`
  (the creator closes). A nil ctx is Background: then the goroutine never exits (stated in the doc comment).
- **Cancellation**: before: `pulses` returned closed. After: `beat` is a harmless no-op. With a nil or
  Background ctx the goroutine never exits (warning in doc.go, as time.Tick).
- **Errors**: none; no caller code runs on the goroutine.
- **Clock**: WithClock (one timer, Reset to the next grid point after each wake; WithJitter, WithOnError,
  WithRand are ignored).
- **Worked example**: `ExampleHeartbeat`.

### `TB-SCHED-006..011`
Option: functional options (Pike 2014), nil ignored. WithClock: nil = Real. WithJitter: Math NUM-PROB-059.
WithOnError: nil = default. WithRand: a *rand.Rand used only from the Run goroutine; not safe to share with
another Run running at the same time. ErrSchedule: matched with errors.Is.

### `TB-SCHED-012` `ParseCron`
- **Pattern**: cron schedule (crontab time specification). **Authority**: IEEE Std 1003.1-2024 "crontab", INPUT
  FILES (opened, above). **Familiar shape**: robfig/cron `ParseStandard(spec) (Schedule, error)` with an explicit
  location instead of robfig's `CRON_TZ=` prefix.
- **Syntax**: five blank-separated POSIX fields (minute hour day-of-month month day-of-week); `*`, numbers,
  `lo-hi` ranges and comma lists only. Steps (`*/5`), names (`MON`, `JAN`), `@daily` macros, 7 for Sunday and a
  seconds field are rejected (calcbench A-069: POSIX only).
- **loc (decided)**: the spec is read on the wall clock of loc; **nil loc is `time.UTC`**. `Next(after)` calls
  ALG-CRON-002 on `after.In(loc)` (calcbench works in `after.Location()`), so the location `after` carries never
  matters, and the result is in loc. Alternative that lost: nil = `time.Local` (robfig/cron's default), which
  makes a schedule depend on the host's TZ environment and zoneinfo, against "Quiet: no environment reads", and
  makes tests host-dependent.
- **Errors**: `fmt.Errorf("schedule: ParseCron(%q): %w", spec, err)` around calcbench's: `cron.ErrSyntax`,
  `cron.ErrRange` (out of range, lo > hi, dow 7), `cron.ErrNeverFires` (e.g. `0 0 31 2 *`); the Schedule is nil
  with an error. ErrNeverFires is a construction error only: every spec ALG-CRON-001 accepts fires within nine
  years (calcbench `searchYears`), so Next never returns the zero Time and Run never ends or fails because of it
  (defensively, a calcbench error from Next becomes the zero Time: Run returns nil, never a spin).
- **Day rule** (A-068): the month always restricts; dom and dow ORed only when both are not `*`; only one
  restricted: it alone decides.
- **DST** (calcbench ALG-CRON-002): a matching wall time in a spring-forward gap fires once, at the transition
  instant (several gap times fire once); one in a fall-back overlap fires at its first (earlier-offset)
  occurrence only.
- **Goroutines**: none; a pure value, safe for concurrent use (immutable). **Clock**: none read; Run supplies the time.
- **Math**: ALG-CRON-001, ALG-CRON-002.
- **Test cases (the reply file's rules)**, `schedule/cron_test.go`: `*/5`, `MON`, `JAN`, `@daily`, empty, four
  and six fields → ErrSyntax; `0 0 * * 7`, minute 60, `5-3` → ErrRange; `0 0 31 2 *`, `0 0 30 2 *`,
  `0 0 31 4,6,9,11 *` → ErrNeverFires at ParseCron (errors.Is through the wrap; nil Schedule); `0 0 29 2 *` under
  Run waits to 2028-02-29 and returns only the ctx cause; nil loc = UTC, after in another zone converted, an
  explicit fixed zone; America/New_York 2026-03-08 gap (`30 2 * * *` fires at 07:00Z once, then 2026-03-09
  06:30Z; `* 2 * * *` fires once) and 2026-11-01 overlap (`30 1 * * *` fires 05:30Z, not 06:30Z; next
  2026-11-02 06:30Z); dom/dow OR (`0 0 13 * 5`: Fridays and the 13th; dom only; dow only; month restricts the
  OR); Run(ParseCron(...)) on the recording fake clock: exact waits and run instants for `15,45 * * * *` and
  across both New York transitions. Zones come from the embedded `time/tzdata` (test binary only).
- **Worked example**: `ExampleParseCron` (`0 10 * * 1-5` after Friday 2026-11-13 10:00Z → Mon 16th, Tue 17th;
  `*/5` is ErrSyntax).

## Tests (written; not run by the builder)
`schedule/main_test.go` (`leak.Main`, a recording clock over `clock.Fake`), `schedule/schedule_test.go`,
`schedule/run_test.go`, `schedule/heartbeat_test.go`, `schedule/option_test.go`. Examples for every export ·
Every fires at each period (exact waits) · At fires once and Run returns nil · At in the past and Every(0):
nil, no task · no overlap with a slow task (atomic in-flight counter) · missed-tick policy (overrun of 2.5
periods: next run at E + period, one run) · jitter bounds and exact values with a fixed PCG seed, grid not
moved · WithOnError continues and sees each error · default returns first error · panic → ErrPanic (default
and via OnError), OnError panic · nil task, nil schedule, nil ctx · ErrSchedule · cancel before / while
waiting / inside the task / after · Heartbeat pulses once per interval only after a beat, pulse per beat for
interval <= 0, newest pulse replaces an unread one, wakes stay on the grid, never blocks with no reader, closes on cancel, a task returning ctx.Err() after a cancel with a cause returns the cause · go_49 and go_50 and go_37 defects as named
tests · leak.Verify in every goroutine test. No time.Sleep; every wait is bounded by a 10 s ctx; every "not
yet" is asserted after the Run or Heartbeat goroutine has provably re-armed its timer (deterministic under
-count=200).

## Out of scope
Cron extensions beyond POSIX (steps, names, macros, seconds; calcbench A-069); many jobs in one scheduler with a heap of next-run times (`jobs`, wave 8); distributed
"run once across replicas" (needs `lock`, wave 7); retry of a failed run (`retry`, wave 5); persistence of
missed runs; overlap allowed on purpose (compose with `pool`).

## For the lead
- Decisions to log: (1) Every(d <= 0) never fires (alt: panic like time.NewTicker; a minimum period);
  (2) At in the past never fires (alt: run at once); (3) missed run times are skipped, next = Next(end of the
  overrunning run) (alt: run late back-to-back / catch-up burst); fixed rate otherwise; (4) default error
  policy: first task error stops Run and is returned; WithOnError continues (alt: continue silently);
  (5) Heartbeat: interval > 0 coalesces beats into at most one pulse per interval; interval <= 0 pulse per
  beat; 1-slot buffer keeping the NEWEST pulse, the named rule-5 exception, D-024 (alt: keep the stale
  one; a heartbeat goroutine that pulses on its own, which says nothing about the worker); wakes on a fixed
  grid (alt: Reset(interval) after each wake, which drifts); (9) a task error that is ctx's own error after
  a cancel returns context.Cause(ctx), as lifecycle; (6) ErrSchedule TB-SCHED-011 and WithRand TB-SCHED-010 added beyond §4;
  (7) At added (request) — record as a §4 deviation; (8) jitter does not move the grid.
- `doc/sources/`: pkg.go.dev time (opened 2026-11-13) → 002, 004, 005, 007; go.dev/doc/go1.23 "Timer
  changes" (opened 2026-11-13) → 004, 005; Cox-Buday 2017 ch.5 "Heartbeats" (not opened) → 005;
  pike_2014_options → 006; calcbench.md: NUM-PROB-059 used by TB-SCHED-004, 008.
- code-heim.md: go_49, go_50, go_37 (checkin) → schedule (defects above).
- taxonomy row "Every, At, Cron, Heartbeat | clock, panics, calcbench algo/heap, num/prob": algo/heap is not
  used (one schedule per Run); Cron shipped wave 5 (TB-SCHED-012).
- calcbench requests: R-3 answered by v0.2.0 (ALG-CRON-001/002); nothing new.
- Wave 5 (ParseCron): decision to log: nil loc = UTC (alt: time.Local, robfig default; lost on "no environment
  reads"); errors are calcbench sentinels wrapped (alt: toolbench aliases `ErrCron...`, new exports for no new
  meaning). `doc/sources/`: IEEE 1003.1-2024 crontab (opened 2026-11-13) → 012; calcbench.md: ALG-CRON-001/002 →
  TB-SCHED-012. Taxonomy row: Cron now shipped (calcbench algo/cron).

Verify: `go test -race -timeout 5m -count=50 ./schedule/` → ok (2.002s), after the reviewer fixes (newest pulse
kept, ctx-error filter, heartbeat grid, Pattern wording, WithClock Pattern `injected clock (test double seam)`).

Wave 5: ParseCron TB-SCHED-012 built (schedule/cron.go); the lead runs verify.
