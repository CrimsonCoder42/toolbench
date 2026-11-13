// Package breaker is a circuit breaker: it stops calling an operation that keeps failing, gives it time to
// recover, then lets a few trial calls through before it trusts it again (Nygard, Release It!; Fowler,
// "CircuitBreaker"; Azure, "Circuit Breaker pattern").
//
// A Breaker starts Closed: Do calls fn and counts consecutive failures. When they reach the threshold
// (WithFailureThreshold, default 5) it opens: Do returns ErrOpen at once, without calling fn. After OpenFor
// (WithOpenFor, default 60 s, read from WithClock's clock) the next Do moves it to HalfOpen and up to n probe
// calls run at the same time (WithHalfOpenProbes, default 1); further calls get ErrOpen. When n probes have
// succeeded it closes; when any probe fails it opens again for another OpenFor.
//
// A failure is any non-nil error from fn, and a panic in fn (returned as panics.ErrPanic). An error that is the
// caller's own cancellation (ctx is done and the error matches ctx.Err() or context.Cause(ctx)) is neither a
// success nor a failure. No goroutine is started; time is read only when Do or State runs.
// WithOnStateChange observes every transition, in order, outside the lock.
//
// ID short form: BRK (IDs are TB-BRK-NNN).
//
// Exports:
//
//	TB-BRK-001  Breaker               a circuit breaker; the zero value is ready to use with the defaults
//	TB-BRK-002  New                   a Breaker configured by options
//	TB-BRK-003  DoValue               Breaker.Do for a function that returns a value
//	TB-BRK-004  State                 the state of a Breaker: Closed, Open or HalfOpen
//	TB-BRK-005  ErrOpen               returned without calling fn when the breaker is open or its probes are all in flight
//	TB-BRK-006  Option                configures New
//	TB-BRK-007  WithFailureThreshold  consecutive failures that open the breaker (default 5)
//	TB-BRK-008  WithOpenFor           how long the breaker stays open before it lets probes through (default 60 s)
//	TB-BRK-009  WithHalfOpenProbes    probe calls allowed at once in HalfOpen, and successes that close it (default 1)
//	TB-BRK-010  WithClock             sets the clock OpenFor is measured on (default clock.Real)
//	TB-BRK-011  WithOnStateChange     a hook called with every transition, in order, outside the lock
package breaker
