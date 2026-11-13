# Go 1.23 Release Notes, go.dev, 2024

**Citation**: Go 1.23 Release Notes, go.dev, 2024.

**Opened**: https://go.dev/doc/go1.23#timer-changes (opened 2026-11-13 by the clock builder).

**Used for**: Section "Timer changes": unstopped timers are collectable; timer channels are synchronous (no stale value after Stop or Reset). clock.Fake matches the observable Stop/Reset results (D-010).

**Also** (opened 2026-11-13 by the schedule builder): no stale value after Reset, behind schedule.Run's single reused timer.

**Used by (IDs)**: TB-CLOCK-002, TB-CLOCK-003, TB-CLOCK-005, TB-SCHED-004.
