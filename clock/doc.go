// Package clock is an injectable time source: the Clock interface every toolbench pattern that waits or
// measures takes through WithClock, the real clock that calls package time, and a fake clock that only moves
// when a test calls Advance. Timers and tickers follow the go1.23 rules of package time: no stale value is
// received after Stop or Reset returns.
//
// Inside a testing/synctest bubble the real clock already runs on fake time; Fake is for code that cannot run
// in a bubble (network, long mutex holds, goroutines started outside the test) and for step-by-step control.
//
// ID short form: CLOCK (IDs are TB-CLOCK-NNN).
//
// Exports:
//
//	TB-CLOCK-001  Clock   the time source: Now, NewTimer, AfterFunc, NewTicker
//	TB-CLOCK-002  Timer   a one-shot timer: C, Stop, Reset with go1.23 semantics
//	TB-CLOCK-003  Ticker  a periodic ticker: C, Stop, Reset; drops ticks for a slow receiver
//	TB-CLOCK-004  Real    the clock backed by package time, with no goroutines of its own
//	TB-CLOCK-005  Fake    a fake clock for tests: Advance fires timers in deadline order, BlockUntil waits for armed timers
package clock
