// Package semaphore_test compares toolbench/semaphore.Weighted with golang.org/x/sync/semaphore.Weighted
// (written against v0.25.0): the same script of TryAcquire and Release calls gives the same results, a ctx
// already done fails Acquire even when the permits are free, and a waiter whose ctx is cancelled returns
// ctx.Err() and leaves the permits free.
//
// Known differences, by design (each asserted below):
//   - constructor: toolbench New, x/sync NewWeighted (the type is Weighted in both).
//   - Acquire(n > size): x/sync blocks until ctx is done and returns ctx.Err(); toolbench returns an error
//     wrapping ErrTooLarge at once, whatever ctx.
//   - negative weights and NewWeighted(n < 0): x/sync panics; toolbench returns ErrNegativeWeight from Acquire,
//     false from TryAcquire, a no-op from Release, and size 0 from New.
//   - Release of more than is held: x/sync panics "released more than held"; toolbench releases what is held.
//
// FIFO order is asserted in toolbench's own tests (semaphore/weighted_test.go) with a test seam that reports
// a queued waiter; x/sync has no such seam, and detecting its queue would need polling, so it is not compared
// here. Its FIFO rule is cited from its source (semaphore.go L58, L123, L146-172) in doc/specs/semaphore.md.
package semaphore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	xsem "golang.org/x/sync/semaphore"

	"github.com/CrimsonCoder42/toolbench/semaphore"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// sem is the method set the two share.
type sem interface {
	Acquire(ctx context.Context, n int64) error
	TryAcquire(n int64) bool
	Release(n int64)
}

func both(size int64) map[string]sem {
	return map[string]sem{"toolbench": semaphore.New(size), "xsync": xsem.NewWeighted(size)}
}

func TestCompare_TryAcquireReleaseScript(t *testing.T) {
	type step struct {
		try     int64 // TryAcquire(try) when release == 0
		release int64
		want    bool
	}
	script := []step{
		// size 5: 3 held, then 5 (full), a zero-weight try always succeeds, nothing more fits; release 2 (3
		// held), 1 more fits (4 held), 2 do not; release 4 (0 held), all 5 fit.
		{try: 3, want: true}, {try: 2, want: true}, {try: 0, want: true}, {try: 2, want: false},
		{try: 1, want: false}, {release: 2}, {try: 1, want: true}, {try: 2, want: false},
		{release: 4}, {try: 5, want: true},
	}
	for name, s := range both(5) {
		for i, st := range script {
			if st.release > 0 {
				s.Release(st.release)
				continue
			}
			if got := s.TryAcquire(st.try); got != st.want {
				t.Fatalf("%s step %d: TryAcquire(%d) = %v, want %v", name, i, st.try, got, st.want)
			}
		}
	}
}

func TestCompare_ctxDoneBeforeAcquire(t *testing.T) {
	done, cancel := context.WithCancel(testCtx(t))
	cancel()
	for name, s := range both(2) {
		if err := s.Acquire(done, 1); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: Acquire with done ctx = %v, want context.Canceled", name, err)
		}
		if !s.TryAcquire(2) {
			t.Fatalf("%s: a failed Acquire changed the semaphore", name)
		}
	}
}

// TestCompare_cancelWaiter does not depend on whether the waiter queued before the cancel: either way the
// result is ctx.Err() and the permits are free afterwards.
func TestCompare_cancelWaiter(t *testing.T) {
	ctx := testCtx(t)
	for name, s := range both(1) {
		if err := s.Acquire(ctx, 1); err != nil {
			t.Fatal(err)
		}
		wctx, cancel := context.WithCancel(ctx)
		errc := make(chan error, 1)
		go func() { errc <- s.Acquire(wctx, 1) }()
		cancel()
		select {
		case err := <-errc:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s: cancelled Acquire = %v, want context.Canceled", name, err)
			}
		case <-ctx.Done():
			t.Fatalf("%s: timed out", name)
		}
		s.Release(1)
		if !s.TryAcquire(1) {
			t.Fatalf("%s: a cancelled waiter leaked a permit", name)
		}
	}
}

// TestCompare_tooLarge_differs: x/sync waits for ctx (here already done, so ctx.Err()); toolbench returns
// ErrTooLarge without looking at ctx.
func TestCompare_tooLarge_differs(t *testing.T) {
	done, cancel := context.WithCancel(testCtx(t))
	cancel()
	if err := xsem.NewWeighted(2).Acquire(done, 3); !errors.Is(err, context.Canceled) {
		t.Fatalf("xsync Acquire(3) on size 2 = %v, want context.Canceled", err)
	}
	if err := semaphore.New(2).Acquire(done, 3); !errors.Is(err, semaphore.ErrTooLarge) {
		t.Fatalf("toolbench Acquire(3) on size 2 = %v, want ErrTooLarge", err)
	}
}

func panics(fn func()) (p bool) {
	defer func() { p = recover() != nil }()
	fn()
	return false
}

func TestCompare_invalidInput_differs(t *testing.T) {
	ctx := testCtx(t)
	cases := []struct {
		name      string
		xsync, tb func()
	}{
		{"NewNegative", func() { xsem.NewWeighted(-1) }, func() { semaphore.New(-1) }},
		{"AcquireNegative", func() { _ = xsem.NewWeighted(1).Acquire(ctx, -1) }, func() { _ = semaphore.New(1).Acquire(ctx, -1) }},
		{"TryAcquireNegative", func() { xsem.NewWeighted(1).TryAcquire(-1) }, func() { semaphore.New(1).TryAcquire(-1) }},
		{"ReleaseNegative", func() { xsem.NewWeighted(1).Release(-1) }, func() { semaphore.New(1).Release(-1) }},
		{"ReleaseMoreThanHeld", func() { xsem.NewWeighted(1).Release(1) }, func() { semaphore.New(1).Release(1) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !panics(c.xsync) {
				t.Error("xsync did not panic (behaviour changed upstream?)")
			}
			if panics(c.tb) {
				t.Error("toolbench panicked on input")
			}
		})
	}
	if err := semaphore.New(1).Acquire(ctx, -1); !errors.Is(err, semaphore.ErrNegativeWeight) {
		t.Fatalf("toolbench Acquire(-1) = %v, want ErrNegativeWeight", err)
	}
}
