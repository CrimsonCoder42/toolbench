// Package ratelimit_test compares toolbench/ratelimit.Limiter with golang.org/x/time/rate.Limiter: the same
// script of AllowN, ReserveN and Cancel calls at the same instants gives the same results. x/time/rate is
// driven at explicit times (AllowN(t, n), ReserveN(t, n), DelayFrom(t), CancelAt(t)); toolbench with a
// clock.Fake moved to the same instants. Rates are chosen so x/time/rate's float arithmetic is exact (1, 2 and
// 4 events per second), so any difference is a difference of behaviour, not of rounding.
//
// Known differences, by design (each asserted below or in toolbench's own tests):
//   - rate type: x/time/rate Limit is float64 events per second; toolbench Limit is {Events, Per} integers.
//   - NewLimiter: toolbench returns an error for an invalid rate or burst; x/time/rate accepts any (Inf, 0).
//   - Cancel (wave 6): both give back n minus the tokens reserved after it (CancelAt's rule; toolbench uses
//     calcbench TokenBucket.Restore). toolbench counts whole tokens and a second Cancel gives back nothing
//     (CancelAt can give back twice); neither appears in these scripts.
//   - WaitN does not pre-check the ctx deadline against the wait (toolbench waits until ctx is done).
package ratelimit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/ratelimit"
)

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type op int

const (
	opAllow op = iota
	opReserve
	opCancelLast
)

type step struct {
	at time.Duration // since start, non-decreasing
	op op
	n  int
}

type pair struct {
	x    *rate.Limiter
	ours *ratelimit.Limiter
	f    *clock.Fake
}

func newPair(t *testing.T, perSecond int64, burst int) pair {
	t.Helper()
	f := clock.NewFake(start)
	ours, err := ratelimit.NewLimiter(ratelimit.Limit{Events: perSecond, Per: time.Second}, burst, ratelimit.WithClock(f))
	if err != nil {
		t.Fatal(err)
	}
	return pair{x: rate.NewLimiter(rate.Limit(perSecond), burst), ours: ours, f: f}
}

func run(t *testing.T, p pair, script []step) {
	t.Helper()
	var xr *rate.Reservation
	var or *ratelimit.Reservation
	for i, s := range script {
		now := start.Add(s.at)
		if d := now.Sub(p.f.Now()); d > 0 {
			p.f.Advance(d)
		}
		switch s.op {
		case opAllow:
			want, got := p.x.AllowN(now, s.n), p.ours.AllowN(s.n)
			if want != got {
				t.Fatalf("step %d: AllowN(+%v, %d): x/time/rate %v, toolbench %v", i, s.at, s.n, want, got)
			}
		case opReserve:
			xr, or = p.x.ReserveN(now, s.n), p.ours.ReserveN(s.n)
			wantOK, gotOK := xr.OK(), or.OK()
			wantD, gotD := xr.DelayFrom(now), or.DelayFrom(now)
			if wantOK != gotOK || wantD != gotD {
				t.Fatalf("step %d: ReserveN(+%v, %d): x/time/rate (%v, %v), toolbench (%v, %v)", i, s.at, s.n, wantOK, wantD, gotOK, gotD)
			}
		case opCancelLast:
			xr.CancelAt(now)
			or.Cancel()
		}
	}
}

func TestCompare_Allow(t *testing.T) {
	p := newPair(t, 2, 3)
	run(t, p, []step{
		{0, opAllow, 3}, {0, opAllow, 1},
		{499 * time.Millisecond, opAllow, 1}, {500 * time.Millisecond, opAllow, 1},
		{time.Second, opAllow, 2}, {1500 * time.Millisecond, opAllow, 2},
		{20 * time.Second, opAllow, 4}, {20 * time.Second, opAllow, 3}, {20 * time.Second, opAllow, 0},
		{20*time.Second + 250*time.Millisecond, opAllow, 1}, {20*time.Second + 500*time.Millisecond, opAllow, 1},
	})
}

func TestCompare_Reserve(t *testing.T) {
	p := newPair(t, 4, 2)
	run(t, p, []step{
		{0, opReserve, 2}, {0, opReserve, 1}, {0, opReserve, 2}, {0, opReserve, 3},
		{100 * time.Millisecond, opAllow, 1}, {750 * time.Millisecond, opAllow, 1},
		{time.Second, opAllow, 1}, {time.Second, opReserve, 1},
		{5 * time.Second, opReserve, 2}, {5 * time.Second, opReserve, 1},
	})
}

func TestCompare_Cancel(t *testing.T) {
	p := newPair(t, 2, 3)
	run(t, p, []step{
		{0, opAllow, 3},
		{0, opReserve, 1}, // +500 ms
		{200 * time.Millisecond, opCancelLast, 0}, // gives the token back
		{200 * time.Millisecond, opReserve, 1},    // 300 ms in both
		{200 * time.Millisecond, opReserve, 1},    // waits behind it
		{300 * time.Millisecond, opCancelLast, 0}, // the latest: given back
		{300 * time.Millisecond, opAllow, 1},
		{500 * time.Millisecond, opAllow, 1},
		{time.Second, opAllow, 1},
		{time.Second, opReserve, 1},
		{time.Second, opCancelLast, 0}, // cancelled before it acts: given back
		{time.Second, opAllow, 1},
		{2 * time.Second, opReserve, 1},
		{3 * time.Second, opCancelLast, 0}, // after the time to act: nothing
		{3 * time.Second, opAllow, 3},
	})
}

// Cancel of an earlier reservation while a later one of as many tokens waits behind it: n minus the later
// tokens is 0, so both give back nothing.
func TestCompare_CancelEarlierInDebt(t *testing.T) {
	p := newPair(t, 1, 1)
	now := start
	p.x.AllowN(now, 1)
	p.ours.Allow()
	x1, o1 := p.x.ReserveN(now, 1), p.ours.Reserve()
	p.x.ReserveN(now, 1)
	p.ours.Reserve()
	x1.CancelAt(now)
	o1.Cancel()
	if want, got := p.x.ReserveN(now, 1).DelayFrom(now), p.ours.Reserve().DelayFrom(now); want != got {
		t.Fatalf("Reserve after Cancel of an earlier reservation: x/time/rate %v, toolbench %v", want, got)
	}
}

// Cancel after a later, smaller reservation (wave 6): r1 takes 3, r2 after it takes 1, r1 is cancelled; both
// give back 3 − 1 = 2 tokens, so the balance at start is -2 and a token is there again at +3 s. The same
// AllowN probes at the same instants agree, and r2's time to act is unchanged.
func TestCompare_CancelAfterLaterReservation(t *testing.T) {
	p := newPair(t, 1, 3)
	now := start
	if !p.x.AllowN(now, 3) || !p.ours.AllowN(3) {
		t.Fatal("AllowN(3) at start refused")
	}
	x1, o1 := p.x.ReserveN(now, 3), p.ours.ReserveN(3)
	x2, o2 := p.x.ReserveN(now, 1), p.ours.ReserveN(1)
	if x1.DelayFrom(now) != o1.DelayFrom(now) || x2.DelayFrom(now) != o2.DelayFrom(now) {
		t.Fatalf("delays: x/time/rate %v, %v; toolbench %v, %v", x1.DelayFrom(now), x2.DelayFrom(now), o1.DelayFrom(now), o2.DelayFrom(now))
	}
	x1.CancelAt(now)
	o1.Cancel()
	for _, at := range []time.Duration{0, time.Second, 2 * time.Second, 2500 * time.Millisecond, 3 * time.Second, 3 * time.Second} {
		now = start.Add(at)
		if d := now.Sub(p.f.Now()); d > 0 {
			p.f.Advance(d)
		}
		if want, got := p.x.AllowN(now, 1), p.ours.AllowN(1); want != got {
			t.Fatalf("AllowN(+%v, 1) after Cancel: x/time/rate %v, toolbench %v", at, want, got)
		}
	}
	if want, got := x2.DelayFrom(now), o2.DelayFrom(now); want != got {
		t.Fatalf("r2 DelayFrom(+3s): x/time/rate %v, toolbench %v", want, got)
	}
}

// Cancel at exactly the time to act, once the debt has run out: both give the token back (toolbench through
// TokenBucket.Restore), and a reservation made afterwards waits the same.
func TestCompare_CancelAtTimeToAct(t *testing.T) {
	p := newPair(t, 1, 2)
	now := start
	p.x.AllowN(now, 2)
	p.ours.AllowN(2)
	x1, o1 := p.x.ReserveN(now, 1), p.ours.ReserveN(1) // +1 s
	now = start.Add(time.Second)
	p.f.Advance(time.Second)
	x1.CancelAt(now)
	o1.Cancel()
	for i := range 2 {
		if want, got := p.x.AllowN(now, 1), p.ours.AllowN(1); want != got {
			t.Fatalf("AllowN #%d at +1s after Cancel: x/time/rate %v, toolbench %v", i, want, got)
		}
	}
	if want, got := p.x.ReserveN(now, 2).DelayFrom(now), p.ours.ReserveN(2).DelayFrom(now); want != got {
		t.Fatalf("ReserveN(2) at +1s: x/time/rate %v, toolbench %v", want, got)
	}
}

func TestCompare_WaitNExceedsBurst(t *testing.T) {
	p := newPair(t, 1, 2)
	ctx := context.Background()
	xErr := p.x.WaitN(ctx, 3)
	oErr := p.ours.WaitN(ctx, 3)
	if xErr == nil || !errors.Is(oErr, ratelimit.ErrBurst) {
		t.Fatalf("WaitN(3) over burst 2: x/time/rate %v, toolbench %v", xErr, oErr)
	}
}

func TestCompare_WaitDoneCtx(t *testing.T) {
	p := newPair(t, 1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	xErr, oErr := p.x.Wait(ctx), p.ours.Wait(ctx)
	if !errors.Is(xErr, context.Canceled) || !errors.Is(oErr, context.Canceled) {
		t.Fatalf("Wait(done ctx): x/time/rate %v, toolbench %v", xErr, oErr)
	}
}
