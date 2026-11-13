package semaphore

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// testCtx bounds every waiting test at 10 s.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// watchQueue makes s report every waiter that joins its queue on the returned channel, so a test knows a
// goroutine is blocked in Acquire without sleeping. Set before s is shared.
func watchQueue(s *Weighted) <-chan struct{} {
	ch := make(chan struct{}, 1024) // buffered: queued is called under s.mu and must not block
	s.queued = func() { ch <- struct{}{} }
	return ch
}

// waitQueued waits until one more waiter has joined the queue.
func waitQueued(t *testing.T, ctx context.Context, q <-chan struct{}) {
	t.Helper()
	select {
	case <-q:
	case <-ctx.Done():
		t.Fatal("timed out waiting for a waiter to queue")
	}
}

func recvErr(t *testing.T, ctx context.Context, errc <-chan error) error {
	t.Helper()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		t.Fatal("timed out waiting for Acquire to return")
		return nil
	}
}

func TestNew_negativeIsSizeZero(t *testing.T) {
	s := New(-5)
	if !s.TryAcquire(0) {
		t.Fatal("TryAcquire(0) on size 0 = false, want true")
	}
	if s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) on size 0 = true, want false")
	}
	if err := s.Acquire(testCtx(t), 1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Acquire(1) on size 0 = %v, want ErrTooLarge", err)
	}
}

func TestWeighted_zeroValue(t *testing.T) {
	var s Weighted
	if err := s.Acquire(testCtx(t), 0); err != nil {
		t.Fatalf("zero value Acquire(0) = %v, want nil", err)
	}
	if err := s.Acquire(testCtx(t), 1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("zero value Acquire(1) = %v, want ErrTooLarge", err)
	}
	s.Release(1) // more than held: clamps, no panic
	if s.TryAcquire(1) {
		t.Fatal("zero value TryAcquire(1) = true after over-release, want false")
	}
}

func TestWeighted_TryAcquire(t *testing.T) {
	s := New(4)
	steps := []struct {
		n    int64
		want bool
	}{
		{-1, false}, // negative: false, unchanged
		{5, false},  // larger than size
		{3, true},   // 1 left
		{2, false},  // does not fit
		{0, true},   // weight 0 with no waiters
		{1, true},   // 0 left
		{1, false},
	}
	for i, st := range steps {
		if got := s.TryAcquire(st.n); got != st.want {
			t.Fatalf("step %d: TryAcquire(%d) = %v, want %v", i, st.n, got, st.want)
		}
	}
	s.Release(4)
	if !s.TryAcquire(4) {
		t.Fatal("TryAcquire(4) after Release(4) = false, want true")
	}
}

func TestWeighted_TryAcquire_respectsQueue(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	s := New(2)
	q := watchQueue(s)
	if err := s.Acquire(ctx, 1); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { errc <- s.Acquire(ctx, 2) }() // queued: only 1 free
	waitQueued(t, ctx, q)
	if s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) jumped ahead of a queued waiter")
	}
	if s.TryAcquire(0) {
		t.Fatal("TryAcquire(0) jumped ahead of a queued waiter")
	}
	s.Release(1)
	if err := recvErr(t, ctx, errc); err != nil {
		t.Fatalf("queued Acquire(2) = %v, want nil", err)
	}
	s.Release(2)
}

func TestWeighted_Acquire_invalidInputs(t *testing.T) {
	ctx := testCtx(t)
	done, cancel := context.WithCancel(ctx)
	cancel()
	s := New(3)
	cases := []struct {
		name string
		ctx  context.Context
		n    int64
		want error
	}{
		{"negative", ctx, -1, ErrNegativeWeight},
		{"tooLarge", ctx, 4, ErrTooLarge},
		{"negativeCtxDone", done, -1, ErrNegativeWeight}, // input errors are reported whatever the ctx
		{"tooLargeCtxDone", done, 4, ErrTooLarge},
		{"ctxDoneBefore", done, 1, context.Canceled}, // fails even though the permits are free
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := s.Acquire(c.ctx, c.n); !errors.Is(err, c.want) {
				t.Fatalf("Acquire(%d) = %v, want %v", c.n, err, c.want)
			}
		})
	}
	if !s.TryAcquire(3) {
		t.Fatal("a failed Acquire changed the semaphore")
	}
}

func TestWeighted_Acquire_nilCtx(t *testing.T) {
	s := New(1)
	var nilCtx context.Context
	if err := s.Acquire(nilCtx, 1); err != nil {
		t.Fatalf("Acquire(nil ctx) = %v, want nil", err)
	}
	s.Release(1)
}

// TestWeighted_Acquire_FIFO queues five waiters one at a time and asserts they are granted in arrival order.
func TestWeighted_Acquire_FIFO(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	const n = 5
	s := New(1)
	q := watchQueue(s)
	if err := s.Acquire(ctx, 1); err != nil {
		t.Fatal(err)
	}
	order := make(chan int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			if err := s.Acquire(ctx, 1); err != nil {
				t.Errorf("waiter %d: %v", i, err)
				return
			}
			order <- i // sent before Release, so the next grant follows this send
			s.Release(1)
		})
		waitQueued(t, ctx, q)
	}
	s.Release(1)
	for want := range n {
		select {
		case got := <-order:
			if got != want {
				t.Fatalf("granted waiter %d, want %d (FIFO)", got, want)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for grants")
		}
	}
	wg.Wait()
}

// TestWeighted_Acquire_largeWaiterBlocksSmall: a large waiter at the front blocks a later small one even
// though the small one would fit (x/sync notifyWaiters: no starvation of large requests).
func TestWeighted_Acquire_largeWaiterBlocksSmall(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	s := New(4)
	q := watchQueue(s)
	if err := s.Acquire(ctx, 3); err != nil { // 1 free
		t.Fatal(err)
	}
	order := make(chan string, 2)
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := s.Acquire(ctx, 3); err != nil {
			t.Errorf("large: %v", err)
			return
		}
		order <- "large"
		s.Release(3)
	})
	waitQueued(t, ctx, q)
	wg.Go(func() {
		if err := s.Acquire(ctx, 1); err != nil { // fits in the free permit, but must wait its turn
			t.Errorf("small: %v", err)
			return
		}
		order <- "small"
		s.Release(1)
	})
	waitQueued(t, ctx, q)
	// Release(3) here would free 4: large (3) and small (1) both fit and are granted in the same call, so
	// their sends would race. Release one permit at a time instead, keeping 1 held, so small can only be
	// granted after large releases.
	s.Release(1) // 2 free: large (3) does not fit, so small (1) must not jump ahead
	s.Release(1) // 3 free: large is granted and holds the rest; small waits for large's Release
	for _, want := range []string{"large", "small"} {
		select {
		case got := <-order:
			if got != want {
				t.Fatalf("granted %s, want %s", got, want)
			}
		case <-ctx.Done():
			t.Fatal("timed out")
		}
	}
	wg.Wait()
	s.Release(1) // the permit this test kept
	if !s.TryAcquire(4) {
		t.Fatal("permits leaked")
	}
}

// TestWeighted_Acquire_cancelledFrontUnblocksSmall: when the large front waiter gives up, the smaller waiter
// behind it that fits is granted at once.
func TestWeighted_Acquire_cancelledFrontUnblocksSmall(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	s := New(4)
	q := watchQueue(s)
	if err := s.Acquire(ctx, 3); err != nil {
		t.Fatal(err)
	}
	largeCtx, cancelLarge := context.WithCancel(ctx)
	defer cancelLarge()
	largeErr := make(chan error, 1)
	go func() { largeErr <- s.Acquire(largeCtx, 3) }()
	waitQueued(t, ctx, q)
	smallErr := make(chan error, 1)
	go func() { smallErr <- s.Acquire(ctx, 1) }()
	waitQueued(t, ctx, q)

	cancelLarge()
	if err := recvErr(t, ctx, largeErr); !errors.Is(err, context.Canceled) {
		t.Fatalf("large = %v, want context.Canceled", err)
	}
	if err := recvErr(t, ctx, smallErr); err != nil {
		t.Fatalf("small = %v, want nil", err)
	}
	s.Release(4)
	if !s.TryAcquire(4) {
		t.Fatal("permits leaked")
	}
}

func TestWeighted_Acquire_cancelWhileWaiting(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	s := New(2)
	q := watchQueue(s)
	if err := s.Acquire(ctx, 2); err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithCancel(ctx)
	errc := make(chan error, 1)
	go func() { errc <- s.Acquire(wctx, 1) }()
	waitQueued(t, ctx, q)
	cancel()
	if err := recvErr(t, ctx, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire = %v, want context.Canceled", err)
	}
	s.Release(2)
	if !s.TryAcquire(2) {
		t.Fatal("a cancelled waiter leaked a permit or stayed queued")
	}
}

// TestWeighted_Acquire_cancelledAtGrantLeaksNoPermit cancels a queued waiter and then releases the permit it
// waits for before it wakes: it sees both ready and done, returns ctx.Err() and holds nothing.
func TestWeighted_Acquire_cancelledAtGrantLeaksNoPermit(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	for range 200 {
		s := New(1)
		q := watchQueue(s)
		if err := s.Acquire(ctx, 1); err != nil {
			t.Fatal(err)
		}
		wctx, cancel := context.WithCancel(ctx)
		errc := make(chan error, 1)
		go func() { errc <- s.Acquire(wctx, 1) }()
		waitQueued(t, ctx, q)
		cancel()
		s.Release(1)
		if err := recvErr(t, ctx, errc); !errors.Is(err, context.Canceled) {
			t.Fatalf("Acquire = %v, want context.Canceled", err)
		}
		if !s.TryAcquire(1) {
			t.Fatal("a waiter cancelled at the moment of grant leaked its permit")
		}
	}
}

// TestWeighted_Acquire_cancelRacesRelease runs cancel and Release on different goroutines: whichever wins,
// a nil result holds the permit and an error holds nothing.
func TestWeighted_Acquire_cancelRacesRelease(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	for range 500 {
		s := New(1)
		q := watchQueue(s)
		if err := s.Acquire(ctx, 1); err != nil {
			t.Fatal(err)
		}
		wctx, cancel := context.WithCancel(ctx)
		errc := make(chan error, 1)
		go func() { errc <- s.Acquire(wctx, 1) }()
		waitQueued(t, ctx, q)
		var wg sync.WaitGroup
		wg.Go(cancel)
		s.Release(1)
		err := recvErr(t, ctx, errc)
		wg.Wait()
		switch {
		case err == nil:
			s.Release(1)
		case !errors.Is(err, context.Canceled):
			t.Fatalf("Acquire = %v, want nil or context.Canceled", err)
		}
		if !s.TryAcquire(1) {
			t.Fatal("permit leaked when cancel raced Release")
		}
	}
}

func TestWeighted_Acquire_zeroWeightWaitsItsTurn(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	s := New(1)
	q := watchQueue(s)
	if err := s.Acquire(ctx, 1); err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { first <- s.Acquire(ctx, 1) }()
	waitQueued(t, ctx, q)
	zero := make(chan error, 1)
	go func() { zero <- s.Acquire(ctx, 0) }()
	waitQueued(t, ctx, q) // weight 0 queued behind a waiter, not granted at once
	s.Release(1)
	if err := recvErr(t, ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := recvErr(t, ctx, zero); err != nil {
		t.Fatal(err)
	}
	s.Release(1)
}

func TestWeighted_Release_overHeldClamps(t *testing.T) {
	s := New(3)
	if !s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) = false")
	}
	s.Release(5) // more than held: releases the 1 held, no panic
	if !s.TryAcquire(3) {
		t.Fatal("TryAcquire(3) after over-release = false, want true")
	}
	if s.TryAcquire(1) {
		t.Fatal("over-release grew the semaphore beyond its size")
	}
}

func TestWeighted_Release_negativeNoop(t *testing.T) {
	s := New(2)
	if !s.TryAcquire(2) {
		t.Fatal("TryAcquire(2) = false")
	}
	s.Release(-1)
	if s.TryAcquire(1) {
		t.Fatal("Release(-1) freed a permit")
	}
}

func TestWeighted_Release_grantsSeveral(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	s := New(3)
	q := watchQueue(s)
	if err := s.Acquire(ctx, 3); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 3)
	for range 3 {
		go func() { errc <- s.Acquire(ctx, 1) }()
		waitQueued(t, ctx, q)
	}
	s.Release(3) // one Release grants all three
	for range 3 {
		if err := recvErr(t, ctx, errc); err != nil {
			t.Fatal(err)
		}
	}
	if s.TryAcquire(1) {
		t.Fatal("more permits granted than the size")
	}
	s.Release(3)
}

// TestWeighted_stress runs many goroutines taking weights 1..3 of a size-5 semaphore and checks, with
// atomics, that the weight in flight never exceeds the size.
func TestWeighted_stress(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	const (
		size       = 5
		goroutines = 32
		iterations = 200
	)
	s := New(size)
	var inFlight, maxInFlight atomic.Int64
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			for i := range iterations {
				n := int64((g+i)%3 + 1)
				if err := s.Acquire(ctx, n); err != nil {
					t.Errorf("Acquire(%d) = %v", n, err)
					return
				}
				cur := inFlight.Add(n)
				for {
					m := maxInFlight.Load()
					if cur <= m || maxInFlight.CompareAndSwap(m, cur) {
						break
					}
				}
				inFlight.Add(-n)
				s.Release(n)
			}
		})
	}
	wg.Wait()
	if m := maxInFlight.Load(); m > size {
		t.Fatalf("max weight in flight = %d, want <= %d", m, size)
	}
	if !s.TryAcquire(size) {
		t.Fatal("permits leaked after the stress run")
	}
}

func BenchmarkWeighted_AcquireRelease(b *testing.B) {
	ctx := context.Background()
	s := New(1)
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Acquire(ctx, 1); err != nil {
			b.Fatal(err)
		}
		s.Release(1)
	}
}

func BenchmarkWeighted_Contended(b *testing.B) {
	ctx := context.Background()
	s := New(4)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := s.Acquire(ctx, 1); err != nil {
				b.Error(err)
				return
			}
			s.Release(1)
		}
	})
}
