package cache_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/cache"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// testCtx bounds a test (go-testing.md: at most 10 s).
func testCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}

// newMem is a Memory on a fresh fake clock.
func newMem[K comparable, V any](t testing.TB, opts ...cache.Option[K, V]) (*cache.Memory[K, V], *clock.Fake) {
	t.Helper()
	f := clock.NewFake(start)
	m, err := cache.New(append([]cache.Option[K, V]{cache.WithClock[K, V](f)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return m, f
}

// evictLog records hook calls; it is called outside the cache's lock but may be called from several
// goroutines, so it is guarded.
type evictLog struct {
	mu  chan struct{}
	got []string
}

func newEvictLog() *evictLog { l := &evictLog{mu: make(chan struct{}, 1)}; return l }

func (l *evictLog) hook(k string, v int, r cache.Reason) {
	l.mu <- struct{}{}
	l.got = append(l.got, k+":"+r.String())
	<-l.mu
}

func (l *evictLog) list() []string {
	l.mu <- struct{}{}
	defer func() { <-l.mu }()
	return append([]string(nil), l.got...)
}

func mustGet[K comparable, V any](t *testing.T, m *cache.Memory[K, V], k K) (V, bool) {
	t.Helper()
	v, ok, err := m.Get(t.Context(), k)
	if err != nil {
		t.Fatalf("Get(%v): %v", k, err)
	}
	return v, ok
}

func mustSet[K comparable, V any](t *testing.T, m *cache.Memory[K, V], k K, v V, ttl time.Duration) {
	t.Helper()
	if err := m.Set(t.Context(), k, v, ttl); err != nil {
		t.Fatalf("Set(%v): %v", k, err)
	}
}

// leakCheck is leak.Verify: every goroutine the test started has exited when it ends.
func leakCheck(t *testing.T) {
	t.Helper()
	leak.Verify(t)
}
