package ratelimit_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/ratelimit"
)

func TestNewLimiter_errors(t *testing.T) {
	tests := []struct {
		name  string
		r     ratelimit.Limit
		burst int
		want  error
	}{
		{"events zero", ratelimit.Limit{Events: 0, Per: time.Second}, 1, ratelimit.ErrLimit},
		{"events negative", ratelimit.Limit{Events: -1, Per: time.Second}, 1, ratelimit.ErrLimit},
		{"per zero", ratelimit.Limit{Events: 1}, 1, ratelimit.ErrLimit},
		{"every zero", ratelimit.Every(0), 1, ratelimit.ErrLimit},
		{"every negative", ratelimit.Every(-time.Second), 1, ratelimit.ErrLimit},
		{"burst zero", perSecond(1), 0, ratelimit.ErrBurst},
		{"burst negative", perSecond(1), -3, ratelimit.ErrBurst},
		{"burst × per overflows", ratelimit.Limit{Events: 1, Per: 365 * 24 * time.Hour}, 1_000_000_000, ratelimit.ErrLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := ratelimit.NewLimiter(tt.r, tt.burst)
			if l != nil || !errors.Is(err, tt.want) {
				t.Errorf("NewLimiter(%+v, %d) = (%v, %v), want (nil, %v)", tt.r, tt.burst, l, err, tt.want)
			}
			k, err := ratelimit.NewKeyed[string](tt.r, tt.burst)
			if k != nil || !errors.Is(err, tt.want) {
				t.Errorf("NewKeyed(%+v, %d) = (%v, %v), want (nil, %v)", tt.r, tt.burst, k, err, tt.want)
			}
		})
	}
}

func TestLimiter_LimitBurst(t *testing.T) {
	l := mustLimiter(t, ratelimit.Every(250*time.Millisecond), 4)
	if got := l.Limit(); got != (ratelimit.Limit{Events: 1, Per: 250 * time.Millisecond}) {
		t.Errorf("Limit = %+v", got)
	}
	if got := l.Burst(); got != 4 {
		t.Errorf("Burst = %d, want 4", got)
	}
}

// Two tokens per second, burst 3 (RFC 2697 §3: starts full, refills at the rate up to the burst).
func TestLimiter_AllowN_refillsExactlyPerClock(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(2), 3, ratelimit.WithClock(f))
	steps := []struct {
		advance time.Duration
		n       int
		want    bool
	}{
		{0, 3, true},  // starts full
		{0, 1, false}, // empty
		{499 * time.Millisecond, 1, false},
		{time.Millisecond, 1, true}, // one token every 500 ms
		{500 * time.Millisecond, 2, false},
		{500 * time.Millisecond, 2, true},
		{10 * time.Second, 3, true}, // capped at the burst
		{0, 1, false},
	}
	for i, s := range steps {
		f.Advance(s.advance)
		if got := l.AllowN(s.n); got != s.want {
			t.Fatalf("step %d: AllowN(%d) at +%v = %v, want %v", i, s.n, f.Now().Sub(start), got, s.want)
		}
	}
}

func TestLimiter_Allow_burstThenRate(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, ratelimit.Every(100*time.Millisecond), 5, ratelimit.WithClock(f))
	allowed := 0
	for range 10 {
		if l.Allow() {
			allowed++
		}
	}
	if allowed != 5 {
		t.Fatalf("allowed %d at one instant, want the burst 5", allowed)
	}
	for i := range 20 {
		f.Advance(100 * time.Millisecond)
		if !l.Allow() {
			t.Fatalf("tick %d refused", i)
		}
		if l.Allow() {
			t.Fatalf("tick %d allowed a second event", i)
		}
	}
}

func TestLimiter_AllowN_zeroAndOverBurst(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(1), 2, ratelimit.WithClock(f))
	for _, n := range []int{0, -1} {
		if !l.AllowN(n) {
			t.Errorf("AllowN(%d) = false, want true", n)
		}
	}
	if l.AllowN(3) {
		t.Error("AllowN(3) over burst 2 = true")
	}
	if !l.AllowN(2) {
		t.Error("AllowN(2) after the refusals = false: a refusal took tokens")
	}
}

func TestLimiter_noDrift(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, ratelimit.Limit{Events: 7, Per: 3 * time.Nanosecond}, 7, ratelimit.WithClock(f))
	for i := range 10_000 {
		if !l.AllowN(7) {
			t.Fatalf("call %d refused", i)
		}
		f.Advance(3 * time.Nanosecond)
	}
}

func TestLimiter_Reserve_delayAndDebt(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(2), 3, ratelimit.WithClock(f))
	wantDelays := []time.Duration{0, 0, 0, 500 * time.Millisecond, time.Second, 1500 * time.Millisecond}
	for i, want := range wantDelays {
		r := l.Reserve()
		if !r.OK() || r.Delay() != want {
			t.Fatalf("Reserve %d: OK %v, Delay %v, want %v", i, r.OK(), r.Delay(), want)
		}
	}
	// In debt until +1.5 s: nothing is allowed before then, and exactly one token every 500 ms after.
	f.Advance(1499 * time.Millisecond)
	if l.Allow() {
		t.Fatal("Allow during debt = true")
	}
	f.Advance(time.Millisecond)
	if l.Allow() {
		t.Fatal("Allow at the last time to act = true (the tokens were reserved)")
	}
	f.Advance(500 * time.Millisecond)
	if !l.Allow() {
		t.Fatal("Allow 500 ms after the debt = false")
	}
}

func TestReservation_DelayFrom(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(1), 1, ratelimit.WithClock(f))
	l.Allow()
	r := l.Reserve()
	if got := r.DelayFrom(start); got != time.Second {
		t.Errorf("DelayFrom(start) = %v, want 1s", got)
	}
	if got := r.DelayFrom(start.Add(2 * time.Second)); got != 0 {
		t.Errorf("DelayFrom(after) = %v, want 0", got)
	}
}

func TestReservation_notOK(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(1), 2, ratelimit.WithClock(f))
	r := l.ReserveN(3)
	if r.OK() || r.Delay() != ratelimit.InfDuration || r.DelayFrom(start) != ratelimit.InfDuration {
		t.Fatalf("ReserveN(3) over burst 2: OK %v, Delay %v", r.OK(), r.Delay())
	}
	r.Cancel() // no effect
	if !l.AllowN(2) {
		t.Error("a refused reservation took tokens")
	}
	var zero ratelimit.Reservation
	if zero.OK() || zero.Delay() != ratelimit.InfDuration {
		t.Error("zero Reservation is OK")
	}
	zero.Cancel()
}

func TestReservation_Cancel(t *testing.T) {
	t.Run("restores the tokens", func(t *testing.T) {
		f := clock.NewFake(start)
		l := mustLimiter(t, perSecond(2), 3, ratelimit.WithClock(f))
		l.AllowN(3)
		r := l.Reserve() // waits 500 ms
		f.Advance(200 * time.Millisecond)
		r.Cancel()
		if got := l.Reserve().Delay(); got != 300*time.Millisecond {
			t.Errorf("Reserve after Cancel: Delay %v, want 300ms (as if the cancelled one never was)", got)
		}
	})
	t.Run("immediate reservation at the same instant", func(t *testing.T) {
		f := clock.NewFake(start)
		l := mustLimiter(t, perSecond(1), 1, ratelimit.WithClock(f))
		r := l.Reserve()
		r.Cancel()
		if !l.Allow() {
			t.Error("Allow after Cancel of an immediate reservation = false")
		}
	})
	t.Run("after the time to act does nothing", func(t *testing.T) {
		f := clock.NewFake(start)
		l := mustLimiter(t, perSecond(1), 1, ratelimit.WithClock(f))
		r := l.Reserve()
		f.Advance(time.Nanosecond)
		r.Cancel()
		if l.Allow() {
			t.Error("Cancel after the time to act gave tokens back")
		}
	})
	t.Run("after a later take of as many tokens gives back nothing", func(t *testing.T) {
		f := clock.NewFake(start)
		l := mustLimiter(t, perSecond(1), 1, ratelimit.WithClock(f))
		l.Allow()
		r1 := l.Reserve() // +1 s
		r2 := l.Reserve() // +2 s
		r1.Cancel()
		if got := r2.Delay(); got != 2*time.Second {
			t.Fatalf("r2 Delay %v", got)
		}
		if got := l.Reserve().Delay(); got != 3*time.Second {
			t.Errorf("Reserve after Cancel of an earlier reservation: Delay %v, want 3s (n − later = 0 given back)", got)
		}
	})
	t.Run("after a later smaller reservation gives back n minus its tokens", func(t *testing.T) {
		// x/time/rate's CancelAt: r1 took 3, r2 after it took 1, so 2 go back; the balance at start is then
		// -2, so the next token is there at +3 s, not +4 s (r2's time to act) and not +0 (all 3 back).
		f := clock.NewFake(start)
		l := mustLimiter(t, perSecond(1), 3, ratelimit.WithClock(f))
		l.AllowN(3)
		r1 := l.ReserveN(3) // +3 s
		r2 := l.ReserveN(1) // +4 s
		if r1.Delay() != 3*time.Second || r2.Delay() != 4*time.Second {
			t.Fatalf("delays %v, %v; want 3s, 4s", r1.Delay(), r2.Delay())
		}
		r1.Cancel()
		if l.Allow() {
			t.Fatal("Allow at +0 after Cancel = true, want false (2 of 3 given back, balance -2)")
		}
		f.Advance(2500 * time.Millisecond)
		if l.Allow() {
			t.Fatal("Allow at +2.5 s = true, want false (balance 0.5)")
		}
		f.Advance(500 * time.Millisecond)
		if !l.Allow() {
			t.Fatal("Allow at +3 s = false, want true (balance 1)")
		}
		if got := r2.Delay(); got != time.Second {
			t.Errorf("r2 Delay at +3 s = %v, want 1s (its time to act does not move)", got)
		}
	})
	t.Run("at its time to act gives back by Restore", func(t *testing.T) {
		f := clock.NewFake(start)
		l := mustLimiter(t, perSecond(1), 2, ratelimit.WithClock(f))
		l.AllowN(2)
		r := l.Reserve() // +1 s
		f.Advance(time.Second)
		r.Cancel() // not after the time to act: given back, the limiter no longer in debt
		if !l.Allow() {
			t.Fatal("Allow after Cancel at the time to act = false, want true")
		}
		if l.Allow() {
			t.Error("second Allow = true, want false (one token given back)")
		}
	})
	t.Run("twice gives back once", func(t *testing.T) {
		f := clock.NewFake(start)
		l := mustLimiter(t, perSecond(1), 1, ratelimit.WithClock(f))
		l.Allow()
		r := l.Reserve()
		r.Cancel()
		r.Cancel()
		if got := l.Reserve().Delay(); got != time.Second {
			t.Errorf("Delay after a double Cancel = %v, want 1s", got)
		}
	})
}

func TestLimiter_Wait_blocksUntilClockAdvances(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(2), 1, ratelimit.WithClock(f))
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("first Wait: %v", err)
	}
	errc := make(chan error, 1)
	go func() { errc <- l.Wait(ctx) }()
	if err := f.BlockUntil(ctx, 1); err != nil {
		t.Fatal(err)
	}
	advance(t, ctx, f, 499*time.Millisecond)
	// The Wait's timer is still armed on the fake clock (it has not fired, so Wait cannot have returned).
	// A done ctx makes BlockUntil answer from the armed count alone, without waiting.
	done, stop := context.WithCancel(ctx)
	stop()
	if err := f.BlockUntil(done, 1); err != nil {
		t.Fatalf("after 499 ms the Wait timer is not armed: %v", err)
	}
	advance(t, ctx, f, time.Millisecond)
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Wait = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Wait did not return after the clock advanced")
	}
}

func TestLimiter_Wait_ctxCancelReturnsCauseAndCancels(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(1), 1, ratelimit.WithClock(f))
	l.Allow()
	cause := errors.New("client went away")
	ctx, cancel := context.WithCancelCause(bound)
	errc := make(chan error, 1)
	go func() { errc <- l.Wait(ctx) }()
	if err := f.BlockUntil(bound, 1); err != nil {
		t.Fatal(err)
	}
	cancel(cause)
	select {
	case err := <-errc:
		if !errors.Is(err, cause) {
			t.Fatalf("Wait = %v, want the cause", err)
		}
	case <-bound.Done():
		t.Fatal("Wait did not return on cancel")
	}
	if got := l.Reserve().Delay(); got != time.Second {
		t.Errorf("Reserve after a cancelled Wait: Delay %v, want 1s (the Wait's tokens given back)", got)
	}
}

func TestLimiter_Wait_ctxDoneBefore(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(1), 1, ratelimit.WithClock(f))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := l.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait(done ctx) = %v, want context.Canceled", err)
	}
	if !l.Allow() {
		t.Error("Wait with a done ctx took a token")
	}
	if err := l.WaitN(nil, 0); err != nil { //nolint:staticcheck // a nil ctx is documented as Background
		t.Errorf("WaitN(nil, 0) = %v", err)
	}
}

// x/time/rate: "rate: Wait(n=%d) exceed limiter's burst %d", checked before ctx.
func TestLimiter_WaitN_exceedsBurst(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(1), 2, ratelimit.WithClock(f))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := l.WaitN(ctx, 3); !errors.Is(err, ratelimit.ErrBurst) {
		t.Fatalf("WaitN(3) over burst 2 = %v, want ErrBurst", err)
	}
	if err := l.WaitN(t.Context(), 3); !errors.Is(err, ratelimit.ErrBurst) {
		t.Fatalf("WaitN(3) over burst 2 = %v, want ErrBurst", err)
	}
}

func TestLimiter_zeroValue(t *testing.T) {
	var l ratelimit.Limiter
	if l.Allow() {
		t.Error("zero Limiter Allow = true")
	}
	if !l.AllowN(0) {
		t.Error("zero Limiter AllowN(0) = false")
	}
	if l.Reserve().OK() {
		t.Error("zero Limiter Reserve OK")
	}
	if err := l.Wait(t.Context()); !errors.Is(err, ratelimit.ErrBurst) {
		t.Errorf("zero Limiter Wait = %v, want ErrBurst", err)
	}
}

// Many goroutines at one instant: exactly burst are allowed (-race checks the mutex).
func TestLimiter_concurrentAllowExactlyBurst(t *testing.T) {
	leak.Verify(t)
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(1), 10, ratelimit.WithClock(f))
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			if l.Allow() {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if got := allowed.Load(); got != 10 {
		t.Fatalf("allowed %d, want 10", got)
	}
}

func TestLimiter_AllowNoAllocation(t *testing.T) {
	f := clock.NewFake(start)
	l := mustLimiter(t, perSecond(1000), 10, ratelimit.WithClock(f))
	allocs := testing.AllocsPerRun(100, func() { // allowed for the burst, then refused: both paths
		l.Allow()
	})
	if allocs != 0 {
		t.Errorf("Allow allocates %v times per call, want 0", allocs)
	}
}

func BenchmarkLimiter_Allow(b *testing.B) {
	f := clock.NewFake(start)
	l := mustLimiter(b, perSecond(1_000_000), 100, ratelimit.WithClock(f))
	b.ReportAllocs()
	for b.Loop() {
		l.Allow()
	}
}
