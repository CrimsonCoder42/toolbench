package ratelimit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/ratelimit"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// testCtx bounds a test that waits (go-testing.md: at most 10 s).
func testCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}

// perSecond is x/time/rate's rate.Limit(n) as an exact Limit.
func perSecond(n int64) ratelimit.Limit { return ratelimit.Limit{Events: n, Per: time.Second} }

func mustLimiter(t testing.TB, r ratelimit.Limit, burst int, opts ...ratelimit.Option) *ratelimit.Limiter {
	t.Helper()
	l, err := ratelimit.NewLimiter(r, burst, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func mustKeyed[K comparable](t testing.TB, r ratelimit.Limit, burst int, opts ...ratelimit.Option) *ratelimit.Keyed[K] {
	t.Helper()
	k, err := ratelimit.NewKeyed[K](r, burst, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// advance moves f forward by d, bounded by ctx.
func advance(t *testing.T, ctx context.Context, f *clock.Fake, d time.Duration) {
	t.Helper()
	if err := f.AdvanceContext(ctx, d); err != nil {
		t.Fatalf("AdvanceContext(%v): %v", d, err)
	}
}
