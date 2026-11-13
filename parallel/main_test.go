package parallel_test

import (
	"context"
	"iter"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// testCtx bounds a test that waits (go-testing.md: at most 10 s).
func testCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}

// upTo yields 0 .. n-1.
func upTo(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

// counted yields 0, 1, 2, ... forever, counting the values handed out in pulled and setting stopped when the
// sequence returns (its deferred cleanup ran).
func counted(pulled *atomic.Int64, stopped *atomic.Bool) iter.Seq[int] {
	return func(yield func(int) bool) {
		defer stopped.Store(true)
		for i := 0; ; i++ {
			pulled.Add(1)
			if !yield(i) {
				return
			}
		}
	}
}

// panicky yields 0 .. n-1 and then panics.
func panicky(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
		panic("seq broke")
	}
}

// maxTracker records the most calls in flight at once, with atomics only.
type maxTracker struct{ now, max atomic.Int64 }

func (m *maxTracker) enter() {
	n := m.now.Add(1)
	for {
		old := m.max.Load()
		if n <= old || m.max.CompareAndSwap(old, n) {
			return
		}
	}
}

func (m *maxTracker) leave() { m.now.Add(-1) }

// result is what a call returned, for tests that run it on another goroutine.
type result[T any] struct {
	v   T
	err error
}

// await receives one result or fails the test when bound ends first.
func await[T any](t *testing.T, bound context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-bound.Done():
		t.Fatal("no result before the test bound")
		var zero T
		return zero
	}
}
