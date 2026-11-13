package ratelimit

import (
	"math"
	"time"
)

// InfDuration is the Delay of a Reservation that is not OK, as x/time/rate's InfDuration.
//
// ID:          TB-RATE-014
// Pattern:     token bucket reservation (never-conforming delay)
// Authority:   pkg.go.dev golang.org/x/time/rate, "const InfDuration"
// Tags:        rate-limiting, reservation, duration
const InfDuration = time.Duration(math.MaxInt64)

// Reservation holds tokens a Limiter has taken for events that may happen at a later time: wait Delay,
// then act; or Cancel to give the tokens back. Methods are safe for concurrent use. Returned by Reserve and
// ReserveN; the zero value is a Reservation that is not OK.
//
// ID:          TB-RATE-004
// Pattern:     token bucket reservation
// Authority:   pkg.go.dev golang.org/x/time/rate, "type Reservation", "func (*Reservation) Delay", "func (*Reservation) CancelAt" (and its source, rate.go)
// Goroutines:  none
// Blocking:    never
// Math:        ALG-RATELM-001, ALG-RATELM-005
// Tags:        concurrency, rate-limiting, token-bucket, reservation
type Reservation struct {
	ok        bool
	lim       *Limiter
	tokens    int
	timeToAct time.Time
	cancelled bool // set by the Cancel that gave tokens back; guarded by lim.mu
}

// OK reports whether the limiter can grant the tokens (false when n > burst).
func (r *Reservation) OK() bool { return r.ok }

// Delay is DelayFrom(now) on the limiter's clock.
func (r *Reservation) Delay() time.Duration {
	if !r.ok {
		return InfDuration
	}
	return r.DelayFrom(r.lim.clock.Now())
}

// DelayFrom returns how long after t the reserved events may happen: 0 when they may happen at t,
// InfDuration when the Reservation is not OK.
func (r *Reservation) DelayFrom(t time.Time) time.Duration {
	if !r.ok {
		return InfDuration
	}
	return max(r.timeToAct.Sub(t), 0)
}

// Cancel gives the reserved tokens back when the events have not happened yet, by x/time/rate's CancelAt
// rule at the limiter's clock: nothing once the clock is after the time to act; otherwise n minus the
// tokens reserved after this one (measured, as x/time/rate does, by the time from this time to act to the
// latest one), never below 0 and capped at the burst. When this was the latest take the limiter is as if it
// had never been made. Differences from CancelAt: the count is in whole tokens (the fraction of a token
// x/time/rate would subtract is not subtracted), it is never more than n, and a second Cancel after one that
// gave tokens back does nothing (CancelAt can give back twice).
//
// Not in debt at the clock, the tokens go back with calcbench ALG-RATELM-005 TokenBucket.Restore. In debt
// (later reservations wait in line), the meter sits empty at the latest time to act, which is in the
// future; giving back k tokens moves that instant k token-times earlier (x/time/rate's balance rises by k),
// which Restore cannot express at a future instant, so the meter is rebuilt empty at the earlier instant.
func (r *Reservation) Cancel() {
	if !r.ok || r.tokens == 0 || r.lim == nil {
		return
	}
	l := r.lim
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.cancelled || r.timeToAct.Before(now) {
		return
	}
	n := int64(r.tokens)
	k := n - l.tokensBetween(r.timeToAct, l.st.lastEvent) // x/time/rate: tokens − tokensFromDuration(lastEvent − timeToAct)
	if k <= 0 {
		return
	}
	r.cancelled = true
	if l.st.debt.After(now) && !l.st.last.After(l.st.debt) {
		// In debt with st.last == st.debt: the meter holds less than a nanosecond's refill there (the take
		// that set the debt left it so); x/time/rate's balance after +k is zero at debt − timeFor(k). A stale
		// reading (now before st.debt while a later call already moved st.last past it) is not this case: the
		// meter is no longer empty at st.last, so it takes Restore below (wave 6 reviewer NIT).
		at := l.st.last.Add(-l.timeFor(k))
		b := l.fresh
		b.Allow(at, int64(l.burst)) // empty at at
		l.st.b, l.st.last, l.st.debt = b, at, at
	} else {
		l.st.b.Restore(k, now) // refills to now (a reading before st.last counts as st.last), adds k, caps at burst
		if now.After(l.st.last) {
			l.st.last = now
		}
	}
	if r.timeToAct.Equal(l.st.lastEvent) { // x/time/rate: the latest take is gone, lastEvent moves back
		if prev := r.timeToAct.Add(-l.timeFor(n)); !prev.Before(now) {
			l.st.lastEvent = prev
		}
	}
}
