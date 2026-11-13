package schedule

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// Heartbeat returns beat, for a worker to call as it makes progress, and pulses, for a monitor to read. With
// interval > 0 a goroutine wakes every interval and sends one pulse (the wake time) if beat was called since
// the last wake: a stalled worker stops the pulses, many beats make one pulse. With interval <= 0 every beat
// is sent as a pulse (the clock's Now), the per-unit-of-work heartbeat. beat never blocks and is safe for
// concurrent use; after ctx is done it does nothing. pulses holds one value, always the newest: a new pulse
// replaces an unread one. This is the explicit exception to rule 5 (lossy by design, as Cox-Buday's
// heartbeats are): a heartbeat must never block its worker, and no information is lost because the newest
// "alive at" survives. Interval wakes are on a fixed grid from the call (each deadline is the previous one
// plus interval, so they do not drift; wakes a slow goroutine missed are skipped). A nil ctx is
// context.Background(), whose goroutine never exits: pass a ctx that is cancelled. WithClock applies;
// WithJitter, WithOnError and WithRand are ignored.
//
// ID:          TB-SCHED-005
// Pattern:     heartbeat (interval and per-unit-of-work)
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.5 "Heartbeats" (not online; not opened)
// Authority:   pkg.go.dev time, "func NewTicker" (drops ticks for slow receivers)
// Goroutines:  1 (none if ctx is already done); it exits when ctx is done, after stopping its timer and closing pulses
// Cancel:      ctx done: pulses is closed; ctx done before the call: pulses is returned closed
// Blocking:    never: beat is an atomic store or a non-blocking send; a new pulse replaces an unread one (the goroutine is the only sender)
// Tags:        concurrency, heartbeat, liveness, monitoring, timer
func Heartbeat(ctx context.Context, interval time.Duration, opts ...Option) (beat func(), pulses <-chan time.Time) {
	if ctx == nil {
		ctx = context.Background()
	}
	c := newConfig(opts)
	out := make(chan time.Time, 1)
	var (
		alive  atomic.Bool              // interval > 0: beat was called since the last wake
		sig    = make(chan struct{}, 1) // interval <= 0: a beat not yet forwarded; never closed
		timer  clock.Timer
		timerC <-chan time.Time // nil when interval <= 0
	)
	if interval > 0 {
		beat = func() { alive.Store(true) }
	} else {
		beat = func() {
			select {
			case sig <- struct{}{}:
			default: // a beat is already waiting to be forwarded
			}
		}
	}
	if ctx.Err() != nil {
		close(out)
		return beat, out
	}
	if interval > 0 {
		timer = c.clock.NewTimer(interval)
		timerC = timer.C()
	}
	offer := func(t time.Time) {
		select {
		case <-out: // the named drop: an unread, older pulse is replaced by the newest
		default:
		}
		out <- t // cannot block: this goroutine is the only sender and the slot is now free
	}
	go func() {
		defer close(out)
		if timer != nil {
			defer timer.Stop()
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-sig:
				offer(c.clock.Now())
			case t := <-timerC:
				if alive.Swap(false) {
					offer(t)
				}
				// The next deadline is on the grid t + k·interval, the first one after now: no drift, and
				// wakes missed by a slow goroutine are skipped.
				next := t.Add(interval)
				if now := c.clock.Now(); !next.After(now) {
					next = next.Add((now.Sub(next)/interval + 1) * interval)
				}
				timer.Reset(next.Sub(c.clock.Now()))
			}
		}
	}()
	return beat, out
}
