package ratelimit_test

import (
	"sync"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/ratelimit"
)

// seqClock returns the given readings from Now in order (then the last one again): two goroutines whose
// clock readings reach the limiter's mutex out of order, replayed on one goroutine. Timers come from Fake.
type seqClock struct {
	*clock.Fake
	mu       sync.Mutex
	readings []time.Time
}

func (c *seqClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.readings[0]
	if len(c.readings) > 1 {
		c.readings = c.readings[1:]
	}
	return t
}

func outOfOrder(readings ...time.Duration) *seqClock {
	c := &seqClock{Fake: clock.NewFake(start)}
	for _, d := range readings {
		c.readings = append(c.readings, start.Add(d))
	}
	return c
}

// Reviewer blocker: G1 reads t0+1ns, G2 reads t0, G1 locks first; G2 must not be refused as if in debt.
func TestLimiter_outOfOrderClockReadings(t *testing.T) {
	t.Run("AllowN", func(t *testing.T) {
		l := mustLimiter(t, perSecond(1), 10, ratelimit.WithClock(outOfOrder(time.Nanosecond, 0)))
		if !l.Allow() {
			t.Fatal("first Allow refused")
		}
		if !l.Allow() {
			t.Fatal("Allow with an earlier clock reading refused with 9 tokens left")
		}
	})
	t.Run("ReserveN", func(t *testing.T) {
		l := mustLimiter(t, perSecond(1), 10, ratelimit.WithClock(outOfOrder(time.Nanosecond, 0, 0)))
		l.Reserve()
		if r := l.Reserve(); !r.OK() || r.DelayFrom(start) != 0 {
			t.Fatalf("Reserve with an earlier clock reading: OK %v, Delay %v, want 0", r.OK(), r.DelayFrom(start))
		}
	})
	t.Run("WaitN", func(t *testing.T) {
		l := mustLimiter(t, perSecond(1), 10, ratelimit.WithClock(outOfOrder(time.Nanosecond, 0)))
		l.Allow()
		ctx, cancel := testCtx(t)
		defer cancel()
		if err := l.Wait(ctx); err != nil { // tokens are there: returns at once, no timer
			t.Fatalf("Wait = %v", err)
		}
	})
	t.Run("still in debt before the time to act", func(t *testing.T) {
		l := mustLimiter(t, perSecond(1), 1, ratelimit.WithClock(outOfOrder(0, 0, time.Millisecond)))
		l.Allow()
		if r := l.Reserve(); r.DelayFrom(start) != time.Second {
			t.Fatalf("Reserve Delay %v, want 1s", r.DelayFrom(start))
		}
		if l.Allow() {
			t.Fatal("Allow during debt = true")
		}
	})
}

// Wave 6 reviewer NIT: Cancel with a stale reading (before the debt, after a later call moved the meter past
// it) must give the token back with Restore, not rebuild the meter from its latest instant. x/time/rate:
// balance -1 at 0, 0.5 at 1.5 s (AllowN(2) refused), +1 by Cancel = 1.5, so AllowN(2) at 2 s is allowed.
func TestReservation_Cancel_outOfOrderClockReading(t *testing.T) {
	// Readings: AllowN(2), Reserve, AllowN(2), Cancel (stale), AllowN(2).
	l := mustLimiter(t, perSecond(1), 2, ratelimit.WithClock(outOfOrder(0, 0, 1500*time.Millisecond, 500*time.Millisecond, 2*time.Second)))
	if !l.AllowN(2) {
		t.Fatal("AllowN(2) at 0 refused")
	}
	r := l.Reserve()
	if d := r.DelayFrom(start); d != time.Second {
		t.Fatalf("Reserve Delay %v, want 1s", d)
	}
	if l.AllowN(2) {
		t.Fatal("AllowN(2) at 1.5 s = true, want false (0.5 tokens)")
	}
	r.Cancel() // reading 0.5 s: before the time to act, so the token goes back
	if !l.AllowN(2) {
		t.Fatal("AllowN(2) at 2 s after Cancel with a stale reading = false, want true (tokens thrown away)")
	}
}

func TestKeyed_outOfOrderClockReadings(t *testing.T) {
	k := mustKeyed[string](t, perSecond(1), 10, ratelimit.WithClock(outOfOrder(time.Nanosecond, 0)))
	if ok, _ := k.Allow("a"); !ok {
		t.Fatal("first Allow refused")
	}
	if ok, retry := k.Allow("a"); !ok {
		t.Fatalf("Allow with an earlier clock reading refused (retry %v) with 9 tokens left", retry)
	}
}
