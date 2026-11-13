package schedule_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/schedule"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// testCtx bounds a test that waits (go-testing.md: at most 10 s).
func testCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}

// recClock is a clock.Fake that records the duration of every NewTimer and Timer.Reset, in order, after the
// timer is armed. A test receives the wait Run asked for and advances exactly to it, so no assertion depends on
// goroutine timing.
type recClock struct {
	*clock.Fake
	waits chan time.Duration
}

func newRec() *recClock {
	return &recClock{Fake: clock.NewFake(start), waits: make(chan time.Duration, 1024)}
}

func (r *recClock) NewTimer(d time.Duration) clock.Timer {
	t := r.Fake.NewTimer(d)
	r.waits <- d
	return recTimer{Timer: t, waits: r.waits}
}

type recTimer struct {
	clock.Timer
	waits chan time.Duration
}

func (t recTimer) Reset(d time.Duration) bool {
	ok := t.Timer.Reset(d)
	t.waits <- d
	return ok
}

// recv receives one value from ch; it fails the test if ch is closed or bound ends first.
func recv[T any](t *testing.T, bound context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("channel closed, want a value")
		}
		return v
	case <-bound.Done():
		t.Fatal("no value before the test bound")
	}
	var zero T
	return zero
}

// none fails the test if ch has a value ready now.
func none[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("%s: got %v, want nothing", what, v)
	default:
	}
}

// goRun starts Run on its own goroutine and returns the channel its result arrives on.
func goRun(ctx context.Context, s schedule.Schedule, task func(context.Context) error, opts ...schedule.Option) <-chan error {
	errc := make(chan error, 1)
	go func() { errc <- schedule.Run(ctx, s, task, opts...) }()
	return errc
}

// reporter returns a task that sends the clock's Now on ran for each run (or gives up when ctx is done) and
// returns nil.
func reporter(c clock.Clock, ran chan<- time.Time) func(context.Context) error {
	return func(ctx context.Context) error {
		select {
		case ran <- c.Now():
		case <-ctx.Done():
		}
		return nil
	}
}
