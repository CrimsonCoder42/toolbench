// Package schedule runs a task on a schedule and lets a worker report that it is alive. Run calls a task at
// the times a Schedule gives (Every for a fixed period, At for one instant), one run at a time, skipping the
// run times a slow run missed, with optional jitter, returning the first task error unless WithOnError is
// passed, and turning a panic into panics.ErrPanic. Heartbeat gives a worker a beat function that never
// blocks and a monitor a channel of pulses. Every wait uses a clock.Clock (WithClock), so tests use
// clock.Fake. Jitter is calcbench NUM-PROB-059 UniformQuantile. ParseCron turns a five-field POSIX crontab
// specification into a Schedule read on the wall clock of a location (nil is UTC); parsing and next-fire are
// calcbench ALG-CRON-001 and ALG-CRON-002, with their daylight-saving rule (a gap time fires once at the
// transition instant, an overlap time at its first occurrence only).
//
// Heartbeat's goroutine exits only when its ctx is done; with a nil or context.Background() ctx it
// runs for the life of the process (as time.Tick's ticker does). Pass a ctx that is cancelled.
//
// ID short form: SCHED (IDs are TB-SCHED-NNN).
//
// Exports:
//
//	TB-SCHED-001  Schedule     gives the next run time after a time; the zero Time means no further runs
//	TB-SCHED-002  Every        a Schedule with a fixed period; a period <= 0 never fires
//	TB-SCHED-003  At           a Schedule with one run at a given instant; an instant in the past never fires
//	TB-SCHED-004  Run          runs a task at each time of a Schedule, one run at a time, skipping missed run times
//	TB-SCHED-005  Heartbeat    a non-blocking beat for a worker and a channel of pulses for its monitor
//	TB-SCHED-006  Option       configures Run and Heartbeat
//	TB-SCHED-007  WithClock    sets the clock Run and Heartbeat wait on (default clock.Real)
//	TB-SCHED-008  WithJitter   shifts each wait of Run by a uniform random offset within ±frac of the interval
//	TB-SCHED-009  WithOnError  passes task errors to a function and keeps Run going (default: Run returns the first)
//	TB-SCHED-010  WithRand     sets the random source of WithJitter (default math/rand/v2's top-level source)
//	TB-SCHED-011  ErrSchedule  the sentinel Run returns when a Schedule's Next does not move forward
//	TB-SCHED-012  ParseCron    a Schedule from a five-field POSIX crontab specification in a location (nil is UTC)
package schedule
