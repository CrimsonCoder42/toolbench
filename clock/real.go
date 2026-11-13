package clock

import "time"

// Real returns the Clock backed by package time. Each method calls its package time twin; it keeps no state,
// so every call returns the same value and it is safe for concurrent use. Inside a testing/synctest bubble it
// runs on the bubble's fake time.
//
// ID:          TB-CLOCK-004
// Pattern:     adapter over package time
// Authority:   pkg.go.dev time, "func Now", "func NewTimer", "func AfterFunc", "func NewTicker"
// Goroutines:  none of its own; an AfterFunc function runs on the goroutine package time starts for it
// Errors:      none; a panic in an AfterFunc function is not recovered, as with time.AfterFunc
// Tags:        time, clock, real
func Real() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	if f == nil {
		f = func() {} // time.AfterFunc would panic when it fires; a nil f does nothing here.
	}
	return realTimer{time.AfterFunc(d, f)}
}

func (realClock) NewTicker(d time.Duration) Ticker {
	if d <= 0 {
		// time.NewTicker panics on d <= 0; a library never panics on input, so return it stopped.
		t := time.NewTicker(time.Hour)
		t.Stop()
		return realTicker{t}
	}
	return realTicker{time.NewTicker(d)}
}

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time        { return r.t.C }
func (r realTimer) Stop() bool                 { return r.t.Stop() }
func (r realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }

type realTicker struct{ t *time.Ticker }

func (r realTicker) C() <-chan time.Time { return r.t.C }
func (r realTicker) Stop()               { r.t.Stop() }

func (r realTicker) Reset(d time.Duration) {
	if d <= 0 {
		r.t.Stop()
		return
	}
	r.t.Reset(d)
}
