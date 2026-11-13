package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	meter "github.com/CrimsonCoder42/calcbench/algo/ratelimit"
	"github.com/CrimsonCoder42/toolbench/clock"
)

// Limiter is a token bucket shared by goroutines: it holds at most burst tokens, gains Limit.Events tokens
// every Limit.Per, starts full, and each event takes a token. Allow and AllowN decide at once; Reserve and
// ReserveN take tokens now for an event at a later time (the bucket goes into debt, as x/time/rate's);
// Wait and WaitN block on the clock until the tokens are there. The bucket arithmetic is calcbench
// ALG-RATELM-001 (exact integer refill), guarded by one mutex. Safe for concurrent use. Create with
// NewLimiter; the zero value is not usable (its methods refuse every event), and a Limiter must not be
// copied after first use.
//
// Debt is expressed with the calcbench meter by time, not by negative tokens: a reservation that must wait
// d is taken at now+d and that instant is recorded as the debt, so until then no event conforms, and from
// then on the meter refills from an empty-for-n level exactly as x/time/rate's negative balance does. A
// clock reading that reaches the mutex after a later one (two goroutines) is decided as no time passed; it
// is refused only by a recorded debt. Reservation.Cancel gives back tokens by x/time/rate's CancelAt rule
// with calcbench ALG-RATELM-005 (TokenBucket.Restore).
//
// ID:          TB-RATE-003
// Pattern:     token bucket (rate limiter)
// Authority:   pkg.go.dev golang.org/x/time/rate, "type Limiter", "func (*Limiter) AllowN", "func (*Limiter) ReserveN", "func (*Limiter) WaitN"
// Authority:   Heinanen and Guérin, "A Single Rate Three Color Marker", RFC 2697, IETF, 1999, §3 Metering
// Goroutines:  none
// Cancel:      WaitN: ctx done before the tokens are there: the reservation is cancelled and context.Cause(ctx) is returned
// Errors:      NewLimiter: ErrLimit, ErrBurst; WaitN: ErrBurst when n > burst, context.Cause(ctx)
// Blocking:    Allow, AllowN, Reserve, ReserveN never block; WaitN blocks on one clock timer (WithClock)
// Math:        ALG-RATELM-001, ALG-RATELM-005
// Tags:        concurrency, rate-limiting, token-bucket, throttling, clock
type Limiter struct {
	clock clock.Clock
	limit Limit
	burst int

	fresh meter.TokenBucket // an unused meter for this rate and burst, copied to measure tokens and times; never changed

	mu sync.Mutex // guards st and every Reservation's cancelled flag
	st state
}

// state is everything a take or a Cancel changes. Guarded by Limiter.mu.
type state struct {
	b         meter.TokenBucket // the calcbench meter
	last      time.Time         // the latest instant given to b
	debt      time.Time         // the time to act of the latest delayed reservation: before it, no event conforms
	lastEvent time.Time         // the time to act of the latest take (x/time/rate's lastEvent), read by Cancel
	started   bool              // b has been given an instant
}

// NewLimiter returns a full Limiter that allows events at rate r with bursts of at most burst events.
// x/time/rate's NewLimiter returns no error; this one does, so a bad rate is never silent. Options:
// WithClock (WithMaxKeys and WithIdleTTL are ignored).
//
// ID:          TB-RATE-015
// Pattern:     token bucket rate limiter (constructor)
// Authority:   pkg.go.dev golang.org/x/time/rate, "func NewLimiter"
// Errors:      ErrLimit when r is not valid (Events < 1, Per <= 0, or burst × Per overflows int64); ErrBurst when burst < 1
// Math:        ALG-RATELM-001
// Tags:        rate-limiting, token-bucket, constructor
func NewLimiter(r Limit, burst int, opts ...Option) (*Limiter, error) {
	b, err := newBucket(r, burst)
	if err != nil {
		return nil, err
	}
	c := newConfig(opts)
	return &Limiter{clock: c.clock, limit: r, burst: burst, fresh: *b, st: state{b: *b}}, nil
}

// newBucket validates r and burst and returns the calcbench meter for them.
func newBucket(r Limit, burst int) (*meter.TokenBucket, error) {
	if r.Events < 1 || r.Per <= 0 {
		return nil, fmt.Errorf("%w: %d per %v", ErrLimit, r.Events, r.Per)
	}
	if burst < 1 {
		return nil, fmt.Errorf("%w: burst = %d must be at least 1", ErrBurst, burst)
	}
	b, err := meter.NewTokenBucket(r.Events, r.Per, int64(burst))
	if err != nil { // only ErrTooLarge remains: burst × Per overflows
		return nil, fmt.Errorf("%w: %w", ErrLimit, err)
	}
	return b, nil
}

// Limit returns the rate the Limiter was created with.
func (l *Limiter) Limit() Limit { return l.limit }

// Burst returns the most events the Limiter allows at one instant.
func (l *Limiter) Burst() int { return l.burst }

// Allow is AllowN(1).
func (l *Limiter) Allow() bool { return l.AllowN(1) }

// AllowN reports whether n events may happen now, and if so takes n tokens. A refused call takes nothing.
// n <= 0 is allowed and takes nothing; n > burst is refused.
func (l *Limiter) AllowN(n int) bool {
	if l.clock == nil {
		return n <= 0
	}
	return l.reserve(l.clock.Now(), n, false).ok
}

// Reserve is ReserveN(1).
func (l *Limiter) Reserve() *Reservation { return l.ReserveN(1) }

// ReserveN takes n tokens now for n events that may happen after the returned Reservation's Delay. The
// Reservation is not OK (and takes nothing) when n > burst. The caller waits Delay, or calls Cancel to give
// the tokens back when it will not act.
func (l *Limiter) ReserveN(n int) *Reservation {
	if l.clock == nil {
		return &Reservation{}
	}
	r := l.reserve(l.clock.Now(), n, true)
	return &r
}

// Wait is WaitN(ctx, 1).
func (l *Limiter) Wait(ctx context.Context) error { return l.WaitN(ctx, 1) }

// WaitN blocks until n events may happen, then returns nil having taken n tokens. It returns ErrBurst
// (wrapped) at once when n > burst, and context.Cause(ctx) when ctx is done before the tokens are there; then
// the reservation is cancelled (Reservation.Cancel). Unlike x/time/rate it does not compare ctx's deadline
// with the wait in advance (the deadline is wall time, the wait is on the limiter's clock): it waits until
// ctx is done. A nil ctx is context.Background().
func (l *Limiter) WaitN(ctx context.Context, n int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if n > l.burst {
		return fmt.Errorf("%w: WaitN(n=%d) exceeds the limiter's burst %d", ErrBurst, n, l.burst)
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if n <= 0 {
		return nil
	}
	now := l.clock.Now()
	r := l.reserve(now, n, true)
	delay := r.DelayFrom(now)
	if delay == 0 {
		return nil
	}
	t := l.clock.NewTimer(delay)
	defer t.Stop()
	select {
	case <-t.C():
		return nil
	case <-ctx.Done():
		r.Cancel()
		return context.Cause(ctx)
	}
}

// reserve decides n events at now. With mayWait false it takes the tokens only when they are there; with
// mayWait true it takes them at the earliest instant they will be there (now or later).
//
// now is read before l.mu, so two callers' readings can reach the lock out of order. Only an explicit debt
// (st.debt, set by a delayed reservation) refuses before its time; a reading merely earlier than st.last is
// decided at st.last, as if no time had passed (reviewer blocker, wave 5: a stale reading refused an event
// with tokens left).
func (l *Limiter) reserve(now time.Time, n int, mayWait bool) Reservation {
	if n <= 0 {
		return Reservation{ok: true, lim: l, timeToAct: now}
	}
	if n > l.burst {
		return Reservation{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	prev := l.st // restored only on the unreachable path below
	var act time.Time
	if l.st.debt.After(now) {
		// In debt: tokens taken by delayed reservations are not there before st.debt (st.last >= st.debt).
		// Decide on a copy so a refusal changes nothing.
		if !mayWait {
			return Reservation{}
		}
		probe := l.st.b
		_, wait := probe.Allow(l.st.last, int64(n))
		act = l.st.last.Add(wait)
	} else {
		base := now
		if l.st.started && l.st.last.After(now) {
			base = l.st.last // an out-of-order reading: no time has passed since st.last
		}
		ok, wait := l.st.b.Allow(base, int64(n))
		l.st.last, l.st.started = base, true
		if ok {
			return l.taken(n, now)
		}
		if !mayWait {
			return Reservation{}
		}
		act = base.Add(wait)
	}
	if ok, _ := l.st.b.Allow(act, int64(n)); !ok { // unreachable: wait is the meter's own time to n tokens
		l.st = prev
		return Reservation{}
	}
	l.st.last, l.st.started = act, true
	if act.After(now) {
		l.st.debt = act
	}
	return l.taken(n, act)
}

// taken records a take of n tokens acting at act and returns its Reservation. Called with l.mu held.
func (l *Limiter) taken(n int, act time.Time) Reservation {
	l.st.lastEvent = act // x/time/rate's reserveN: lastEvent = timeToAct on every granted take
	return Reservation{ok: true, lim: l, tokens: n, timeToAct: act}
}

// tokensBetween returns the whole tokens the rate gives from an empty bucket at from to the instant to, at
// most burst; 0 when to is not after from. Measured on a copy of the unused meter (ALG-RATELM-001).
func (l *Limiter) tokensBetween(from, to time.Time) int64 {
	if !to.After(from) {
		return 0
	}
	p := l.fresh
	p.Allow(from, int64(l.burst)) // a new meter starts full: taking the burst leaves it empty at from
	return p.Available(to)
}

// timeFor returns how long the rate takes to give k tokens (1 <= k <= burst), rounded up to the nanosecond:
// the meter's own wait from empty (ALG-RATELM-001).
func (l *Limiter) timeFor(k int64) time.Duration {
	var t time.Time
	p := l.fresh
	p.Allow(t, int64(l.burst))
	_, wait := p.Allow(t, k)
	return wait
}
