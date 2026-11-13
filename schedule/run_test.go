package schedule_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/calcbench/num/prob"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/schedule"
)

func ExampleRun() {
	f := clock.NewFake(start)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0
	task := func(context.Context) error {
		fmt.Println(f.Now().Format(time.TimeOnly))
		if runs++; runs == 3 {
			cancel()
		}
		return nil
	}
	errc := make(chan error, 1)
	go func() { errc <- schedule.Run(ctx, schedule.Every(time.Minute), task, schedule.WithClock(f)) }()
	for range 3 {
		_ = f.BlockUntil(ctx, 1) // Run is waiting on its timer
		f.Advance(time.Minute)
	}
	fmt.Println(<-errc)
	// Output:
	// 00:01:00
	// 00:02:00
	// 00:03:00
	// context canceled
}

func TestRun_everyFiresEachPeriod(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	defer cancel()
	rc := newRec()
	ran := make(chan time.Time)
	errc := goRun(ctx, schedule.Every(time.Second), reporter(rc, ran), schedule.WithClock(rc))
	for i := 1; i <= 5; i++ {
		if d := recv(t, bound, rc.waits); d != time.Second {
			t.Fatalf("wait %d = %v, want 1s", i, d)
		}
		if err := rc.AdvanceContext(bound, time.Second); err != nil {
			t.Fatal(err)
		}
		if got := recv(t, bound, ran); !got.Equal(start.Add(time.Duration(i) * time.Second)) {
			t.Fatalf("run %d at %v, want start+%ds", i, got, i)
		}
	}
	cancel()
	if err := recv(t, bound, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
}

func TestRun_atFiresOnce(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	rc := newRec()
	ran := make(chan time.Time)
	errc := goRun(bound, schedule.At(start.Add(time.Hour)), reporter(rc, ran), schedule.WithClock(rc))
	if d := recv(t, bound, rc.waits); d != time.Hour {
		t.Fatalf("wait = %v, want 1h", d)
	}
	if err := rc.AdvanceContext(bound, time.Hour); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, bound, ran); !got.Equal(start.Add(time.Hour)) {
		t.Fatalf("ran at %v, want start+1h", got)
	}
	if err := recv(t, bound, errc); err != nil {
		t.Fatalf("Run = %v, want nil (the schedule ended)", err)
	}
	none(t, rc.waits, "a wait after the one-shot")
}

func TestRun_noRuns(t *testing.T) {
	for name, s := range map[string]schedule.Schedule{
		"Every(0)":   schedule.Every(0),
		"Every(-1s)": schedule.Every(-time.Second),
		"At(past)":   schedule.At(start.Add(-time.Second)),
		"At(now)":    schedule.At(start),
		"At(zero)":   schedule.At(time.Time{}),
		"nil":        nil,
	} {
		t.Run(name, func(t *testing.T) {
			rc := newRec()
			var calls atomic.Int32
			err := schedule.Run(t.Context(), s, func(context.Context) error { calls.Add(1); return nil }, schedule.WithClock(rc))
			if err != nil || calls.Load() != 0 {
				t.Fatalf("Run = %v after %d runs, want nil after 0", err, calls.Load())
			}
			none(t, rc.waits, "a timer for a schedule with no runs")
		})
	}
}

// go_49 blocks main with `for {}`, a busy spin. Run waits on the clock: exactly one timer arm per run, and a
// schedule with no runs returns at once instead of looping.
func TestRun_go49_noBusySpin(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	defer cancel()
	rc := newRec()
	ran := make(chan time.Time)
	errc := goRun(ctx, schedule.Every(time.Second), reporter(rc, ran), schedule.WithClock(rc))
	for range 3 {
		d := recv(t, bound, rc.waits)
		if err := rc.AdvanceContext(bound, d); err != nil {
			t.Fatal(err)
		}
		recv(t, bound, ran)
	}
	recv(t, bound, rc.waits) // the 4th arm: Run is waiting again, not looping
	none(t, rc.waits, "a second arm without time moving")
	cancel()
	if err := recv(t, bound, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	none(t, rc.waits, "an arm after cancel")
	if err := schedule.Run(bound, schedule.Every(0), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Run(Every(0)) = %v, want nil at once", err)
	}
}

// go_49 and go_50 stop with a signal goroutine calling os.Exit. Run returns the ctx cause instead.
func TestRun_go49_returnsOnCancel(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancelCause(bound)
	rc := newRec()
	errc := goRun(ctx, schedule.Every(time.Minute), func(context.Context) error { return nil }, schedule.WithClock(rc))
	recv(t, bound, rc.waits)
	sigterm := errors.New("SIGTERM")
	cancel(sigterm)
	if err := recv(t, bound, errc); !errors.Is(err, sigterm) {
		t.Fatalf("Run = %v, want the cause %v", err, sigterm)
	}
}

// go_50 waits for an instant in a goroutine on time.After(time.Until(t)) with no way to cancel it. Run(At)
// stops waiting when ctx is cancelled, the task never runs, and no goroutine is left (leak.Verify).
func TestRun_go50_atIsCancellable(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	rc := newRec()
	var calls atomic.Int32
	errc := goRun(ctx, schedule.At(start.Add(24*time.Hour)), func(context.Context) error { calls.Add(1); return nil }, schedule.WithClock(rc))
	recv(t, bound, rc.waits)
	cancel()
	if err := recv(t, bound, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	rc.Advance(48 * time.Hour) // the instant passes after the cancel: nothing runs
	if calls.Load() != 0 {
		t.Fatalf("task ran %d times after cancel", calls.Load())
	}
}

func TestRun_cancelBefore(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("stopped")
	cancel(cause)
	rc := newRec()
	var calls atomic.Int32
	err := schedule.Run(ctx, schedule.Every(time.Second), func(context.Context) error { calls.Add(1); return nil }, schedule.WithClock(rc))
	if !errors.Is(err, cause) || calls.Load() != 0 {
		t.Fatalf("Run = %v after %d runs, want %v after 0", err, calls.Load(), cause)
	}
	none(t, rc.waits, "a timer after a cancel before the start")
}

// A ctx cancelled while the timer is due: whichever Run sees first, the task does not run.
func TestRun_cancelWinsOverDueTimer(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	rc := newRec()
	var calls atomic.Int32
	errc := goRun(ctx, schedule.Every(time.Second), func(context.Context) error { calls.Add(1); return nil }, schedule.WithClock(rc))
	d := recv(t, bound, rc.waits)
	cancel()
	if err := rc.AdvanceContext(bound, d); err != nil {
		t.Fatal(err)
	}
	if err := recv(t, bound, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if calls.Load() != 0 {
		t.Fatal("task ran after ctx was cancelled")
	}
}

func TestRun_cancelDuringTask(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancelCause(bound)
	rc := newRec()
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	task := func(ctx context.Context) error {
		calls.Add(1)
		started <- struct{}{}
		<-ctx.Done() // the task sees the cancel
		return nil
	}
	errc := goRun(ctx, schedule.Every(time.Second), task, schedule.WithClock(rc))
	if err := rc.AdvanceContext(bound, recv(t, bound, rc.waits)); err != nil {
		t.Fatal(err)
	}
	recv(t, bound, started)
	cause := errors.New("shutdown")
	cancel(cause)
	if err := recv(t, bound, errc); !errors.Is(err, cause) {
		t.Fatalf("Run = %v, want the cause %v", err, cause)
	}
	if calls.Load() != 1 {
		t.Fatalf("task ran %d times, want 1", calls.Load())
	}
}

// A task that returns ctx's own error after a cancel with a cause: Run returns the cause, with and without
// WithOnError, and the hook does not see the cancellation.
func TestRun_taskReturnsCtxErrIsCause(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	for _, withHook := range []bool{false, true} {
		ctx, cancel := context.WithCancelCause(bound)
		rc := newRec()
		started := make(chan struct{}, 1)
		hooked := make(chan error, 1)
		task := func(ctx context.Context) error {
			started <- struct{}{}
			<-ctx.Done()
			return fmt.Errorf("task stopped: %w", ctx.Err())
		}
		opts := []schedule.Option{schedule.WithClock(rc)}
		if withHook {
			opts = append(opts, schedule.WithOnError(func(err error) { hooked <- err }))
		}
		errc := goRun(ctx, schedule.Every(time.Second), task, opts...)
		if err := rc.AdvanceContext(bound, recv(t, bound, rc.waits)); err != nil {
			t.Fatal(err)
		}
		recv(t, bound, started)
		cause := errors.New("deploy")
		cancel(cause)
		if err := recv(t, bound, errc); err != cause {
			t.Fatalf("WithOnError=%v: Run = %v, want the cause %v", withHook, err, cause)
		}
		none(t, hooked, "the cancellation passed to WithOnError")
	}
}

func TestRun_cancelAfterEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	err := schedule.Run(ctx, schedule.At(time.Time{}), func(context.Context) error { return nil })
	cancel() // after Run returned: nothing to stop
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

func TestRun_nilCtx(t *testing.T) {
	var ctx context.Context // nil: Background
	if err := schedule.Run(ctx, schedule.Every(0), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Run(nil ctx) = %v, want nil", err)
	}
}

// A run longer than the period never overlaps the next one, and fixed rate holds when it is shorter.
func TestRun_noOverlapAndFixedRate(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	defer cancel()
	rc := newRec()
	var inflight, most atomic.Int32
	ran := make(chan time.Time)
	release := make(chan struct{})
	task := func(ctx context.Context) error {
		n := inflight.Add(1)
		defer inflight.Add(-1)
		if n > most.Load() {
			most.Store(n)
		}
		select {
		case ran <- rc.Now():
		case <-ctx.Done():
			return nil
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}
	errc := goRun(ctx, schedule.Every(time.Second), task, schedule.WithClock(rc))

	// Run 1 at 1s takes 400ms: the next is still at 2s (wait 600ms), not 1.4s + 1s.
	rc.Advance(recv(t, bound, rc.waits))
	recv(t, bound, ran)
	rc.Advance(400 * time.Millisecond)
	release <- struct{}{}
	if d := recv(t, bound, rc.waits); d != 600*time.Millisecond {
		t.Fatalf("wait after a short run = %v, want 600ms (fixed rate)", d)
	}
	rc.Advance(600 * time.Millisecond)
	if got := recv(t, bound, ran); !got.Equal(start.Add(2 * time.Second)) {
		t.Fatalf("run 2 at %v, want start+2s", got)
	}

	// Run 2 at 2s takes 2.5s: the run times 3s and 4s are skipped, the next is 4.5s + 1s.
	rc.Advance(2500 * time.Millisecond)
	release <- struct{}{}
	if d := recv(t, bound, rc.waits); d != time.Second {
		t.Fatalf("wait after an overrun = %v, want 1s (missed runs skipped, one period from the end)", d)
	}
	rc.Advance(time.Second)
	if got := recv(t, bound, ran); !got.Equal(start.Add(5500 * time.Millisecond)) {
		t.Fatalf("run 3 at %v, want start+5.5s", got)
	}
	release <- struct{}{}
	recv(t, bound, rc.waits)
	cancel()
	if err := recv(t, bound, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if most.Load() != 1 {
		t.Fatalf("%d runs at once, want 1", most.Load())
	}
}

func TestRun_firstErrorStops(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	rc := newRec()
	boom := errors.New("boom")
	var calls atomic.Int32
	errc := goRun(bound, schedule.Every(time.Second), func(context.Context) error { calls.Add(1); return boom }, schedule.WithClock(rc))
	rc.Advance(recv(t, bound, rc.waits))
	if err := recv(t, bound, errc); err != boom {
		t.Fatalf("Run = %v, want boom unchanged", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("task ran %d times, want 1", calls.Load())
	}
	none(t, rc.waits, "a wait after the error")
}

func TestRun_panicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	for name, task := range map[string]func(context.Context) error{
		"panic":   func(context.Context) error { panic("log.Fatal in a task") },
		"nilTask": nil,
	} {
		rc := newRec()
		errc := goRun(bound, schedule.Every(time.Second), task, schedule.WithClock(rc))
		rc.Advance(recv(t, bound, rc.waits))
		err := recv(t, bound, errc)
		var pe *panics.PanicError
		if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) {
			t.Fatalf("%s: Run = %v, want a *panics.PanicError", name, err)
		}
	}
}

func TestRun_errSchedule(t *testing.T) {
	for name, s := range map[string]schedule.Schedule{
		"same":     stuck{0},
		"backward": stuck{-time.Second},
	} {
		var calls atomic.Int32
		err := schedule.Run(t.Context(), s, func(context.Context) error { calls.Add(1); return nil }, schedule.WithClock(newRec()))
		if !errors.Is(err, schedule.ErrSchedule) || calls.Load() != 0 {
			t.Fatalf("%s: Run = %v after %d runs, want ErrSchedule after 0", name, err, calls.Load())
		}
	}
}

// stuck is a broken Schedule: Next returns after + d with d <= 0.
type stuck struct{ d time.Duration }

func (s stuck) Next(after time.Time) time.Time { return after.Add(s.d) }

func ExampleErrSchedule() {
	err := schedule.Run(context.Background(), stuck{0}, func(context.Context) error { return nil })
	fmt.Println(errors.Is(err, schedule.ErrSchedule))
	// Output: true
}

// The jitter of each wait is exactly NUM-PROB-059 of the seeded source's next u, within ±frac of the
// period, and the run times stay on the 1 s grid.
func TestRun_jitterBoundsFixedSeed(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	defer cancel()
	rc := newRec()
	ran := make(chan time.Time)
	const frac = 0.1
	errc := goRun(ctx, schedule.Every(time.Second), reporter(rc, ran),
		schedule.WithClock(rc), schedule.WithJitter(frac), schedule.WithRand(rand.New(rand.NewPCG(1, 2))))
	twin := rand.New(rand.NewPCG(1, 2))
	span := frac * float64(time.Second)
	now := start
	distinct := map[time.Duration]bool{}
	for k := 1; k <= 20; k++ {
		x, err := prob.UniformQuantile(twin.Float64(), -span, span)
		if err != nil {
			t.Fatal(err)
		}
		off := time.Duration(math.Round(x))
		sched := start.Add(time.Duration(k) * time.Second)
		d := recv(t, bound, rc.waits)
		if want := sched.Sub(now) + off; d != want {
			t.Fatalf("wait %d = %v, want %v (grid %v + jitter %v)", k, d, want, sched.Sub(now), off)
		}
		if off < -100*time.Millisecond || off >= 100*time.Millisecond {
			t.Fatalf("jitter %v outside ±10%% of 1s", off)
		}
		distinct[off] = true
		rc.Advance(d)
		now = now.Add(d)
		if got := recv(t, bound, ran); !got.Equal(now) {
			t.Fatalf("run %d at %v, want %v", k, got, now)
		}
	}
	if len(distinct) < 10 {
		t.Fatalf("only %d distinct jitters in 20 runs", len(distinct))
	}
	cancel()
	recv(t, bound, errc)
}

func TestRun_onErrorContinues(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	defer cancel()
	rc := newRec()
	seen := make(chan error, 8) // read only by this test after each run
	var k atomic.Int32
	task := func(context.Context) error {
		switch k.Add(1) {
		case 1:
			return errors.New("run 1")
		case 2:
			panic("run 2")
		default:
			return nil
		}
	}
	errc := goRun(ctx, schedule.Every(time.Second), task, schedule.WithClock(rc),
		schedule.WithOnError(func(err error) { seen <- err }))
	for range 3 {
		rc.Advance(recv(t, bound, rc.waits))
	}
	recv(t, bound, rc.waits) // run 3 has returned
	if e := recv(t, bound, seen); e.Error() != "run 1" {
		t.Fatalf("first error %v, want run 1", e)
	}
	if e := recv(t, bound, seen); !errors.Is(e, panics.ErrPanic) {
		t.Fatalf("second error %v, want ErrPanic", e)
	}
	none(t, seen, "an error from a run that returned nil")
	cancel()
	if err := recv(t, bound, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
}

func TestRun_onErrorPanics(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	rc := newRec()
	errc := goRun(bound, schedule.Every(time.Second), func(context.Context) error { return errors.New("x") },
		schedule.WithClock(rc), schedule.WithOnError(func(error) { panic("hook") }))
	rc.Advance(recv(t, bound, rc.waits))
	if err := recv(t, bound, errc); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Run = %v, want ErrPanic from the hook", err)
	}
}

func BenchmarkRun_fakeClock(b *testing.B) {
	b.ReportAllocs()
	rc := newRec()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n := 0
	task := func(context.Context) error {
		if n++; n == b.N {
			cancel()
		}
		return nil
	}
	errc := goRun(ctx, schedule.Every(time.Second), task, schedule.WithClock(rc))
	for range b.N {
		rc.Advance(<-rc.waits)
	}
	<-errc
}
