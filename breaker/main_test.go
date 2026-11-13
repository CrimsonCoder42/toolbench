package breaker_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/breaker"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var (
	start   = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	errBoom = errors.New("boom")
)

// testCtx bounds a test (go-testing.md: at most 10 s).
func testCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}

func ok(context.Context) error   { return nil }
func fail(context.Context) error { return errBoom }

// recorder is a WithOnStateChange hook that keeps every transition as "from>to".
type recorder struct{ got []string }

func (r *recorder) hook(from, to breaker.State) { r.got = append(r.got, from.String()+">"+to.String()) }

// newFake returns a breaker on a fake clock with the given threshold, OpenFor and probes, and its clock.
func newFake(threshold int, openFor time.Duration, probes int, opts ...breaker.Option) (*breaker.Breaker, *clock.Fake) {
	f := clock.NewFake(start)
	opts = append([]breaker.Option{
		breaker.WithFailureThreshold(threshold),
		breaker.WithOpenFor(openFor),
		breaker.WithHalfOpenProbes(probes),
		breaker.WithClock(f),
	}, opts...)
	return breaker.New(opts...), f
}

// trip makes n failing calls.
func trip(t *testing.T, b *breaker.Breaker, n int) {
	t.Helper()
	for range n {
		if err := b.Do(context.Background(), fail); !errors.Is(err, errBoom) && !errors.Is(err, breaker.ErrOpen) {
			t.Fatalf("trip: Do = %v", err)
		}
	}
}

func wantState(t *testing.T, b *breaker.Breaker, want breaker.State) {
	t.Helper()
	if got := b.State(); got != want {
		t.Fatalf("State = %v, want %v", got, want)
	}
}
