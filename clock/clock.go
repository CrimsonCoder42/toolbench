package clock

import "time"

// Clock is a source of time: the current time, one-shot timers, callbacks after a delay and periodic
// tickers. Patterns take one through WithClock and default a nil Clock to Real(). Implementations are safe for
// concurrent use. A Clock is an interface; its zero value (nil) is not a clock.
//
// ID:          TB-CLOCK-001
// Pattern:     test double seam (injected time source)
// Authority:   pkg.go.dev time, "func Now", "func NewTimer", "func AfterFunc", "func NewTicker"
// Authority:   Fowler, "TestDouble", martinfowler.com, 2006 (after Meszaros, xUnit Test Patterns, 2007)
// Tags:        time, clock, timer, ticker, testing
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// NewTimer returns a Timer that sends the time on its channel once d has elapsed. A d <= 0 fires as soon
	// as the clock next runs its timers.
	NewTimer(d time.Duration) Timer
	// AfterFunc calls f on its own goroutine once d has elapsed and returns a Timer whose Stop cancels the
	// call. The Timer's C is nil.
	AfterFunc(d time.Duration, f func()) Timer
	// NewTicker returns a Ticker that sends the time every d. A d <= 0 returns a stopped ticker that sends
	// nothing until Reset with a positive period (package time panics instead).
	NewTicker(d time.Duration) Ticker
}

// Timer is a one-shot timer with the semantics of *time.Timer as of go1.23: after Stop or Reset returns, no
// value from before the call is received on C. The zero value (nil) is not a timer; get one from a Clock.
//
// ID:          TB-CLOCK-002
// Pattern:     timer (one-shot)
// Authority:   pkg.go.dev time, "func (*Timer) Stop", "func (*Timer) Reset"
// Authority:   Go 1.23 Release Notes, go.dev, 2024, "Timer changes"
// Tags:        time, timer, stop, reset
type Timer interface {
	// C returns the channel the time is sent on; nil for a timer made by AfterFunc.
	C() <-chan time.Time
	// Stop prevents the timer from firing. It reports true if the call stopped the timer, false if it had
	// already expired (and its value was received, or its function started) or been stopped. An AfterFunc
	// function that has started is not waited for.
	Stop() bool
	// Reset makes the timer expire after d, as if just created. It reports true if the timer had been active.
	Reset(d time.Duration) bool
}

// Ticker sends the time on its channel once per period, with the semantics of *time.Ticker as of go1.23: it
// drops ticks for a slow receiver and no stale tick is received after Stop or Reset returns. The zero value
// (nil) is not a ticker; get one from a Clock.
//
// ID:          TB-CLOCK-003
// Pattern:     ticker (periodic timer)
// Authority:   pkg.go.dev time, "type Ticker", "func (*Ticker) Stop", "func (*Ticker) Reset"
// Authority:   Go 1.23 Release Notes, go.dev, 2024, "Timer changes"
// Tags:        time, ticker, periodic
type Ticker interface {
	// C returns the channel the ticks are sent on.
	C() <-chan time.Time
	// Stop turns the ticker off; no tick is sent after it returns.
	Stop()
	// Reset stops the ticker and restarts it with period d; a d <= 0 leaves it stopped.
	Reset(d time.Duration)
}
