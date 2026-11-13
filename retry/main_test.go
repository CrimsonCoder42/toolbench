package retry_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// testCtx bounds a test that waits (go-testing.md: at most 10 s).
func testCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}

// instant is a clock.Fake whose timers fire as soon as they are made: NewTimer arms a fake timer and advances
// the fake by its duration on the calling goroutine, so a wait in Do returns at once and Now() moves by exactly
// the wait. No goroutine, no wall time.
type instant struct{ *clock.Fake }

func newInstant() instant { return instant{clock.NewFake(start)} }

func (i instant) NewTimer(d time.Duration) clock.Timer {
	t := i.Fake.NewTimer(d)
	i.Fake.Advance(d)
	return t
}

// elapsed is the fake time that has passed since start.
func (i instant) elapsed() time.Duration { return i.Now().Sub(start) }

// counter returns fn that fails with errs[k] on call k (nil after the list ends) and the call count.
func counter(errs ...error) (fn func(context.Context) error, calls *int) {
	n := 0
	return func(context.Context) error {
		n++
		if n <= len(errs) {
			return errs[n-1]
		}
		return nil
	}, &n
}
