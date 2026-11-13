package bulkhead

import (
	"context"
	"errors"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestNew_maxConcurrentInvalid(t *testing.T) {
	for _, n := range []int{0, -1, math.MinInt} {
		b, err := New(n)
		if b != nil || !errors.Is(err, ErrMaxConcurrent) {
			t.Fatalf("New(%d) = %v, %v; want nil, ErrMaxConcurrent", n, b, err)
		}
	}
}

func TestNew_optionDefaults(t *testing.T) {
	b := mustNew(t, 2, nil, WithQueue(-1), WithQueueTimeout(-time.Second), WithClock(nil), WithOnEvent(nil))
	if b.queue != 0 || b.timeout != 0 || b.clk == nil || b.onEvent != nil {
		t.Fatalf("defaults: queue=%d timeout=%v clk=%v hook=%v", b.queue, b.timeout, b.clk, b.onEvent != nil)
	}
	big := mustNew(t, 3, WithQueue(math.MaxInt))
	if want := int(min(int64(math.MaxInt), math.MaxInt64-3)); big.queue != want {
		t.Fatalf("queue = %d, want %d (capped so maxConcurrent+queue fits int64)", big.queue, want)
	}
	if err := big.Do(context.Background(), ok); err != nil {
		t.Fatalf("Do on the capped bulkhead = %v", err)
	}
}

func TestDo_maxConcurrentAsserted(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	const slots, queued = 3, 10
	ev := newEvents()
	b := mustNew(t, slots, WithQueue(queued), WithOnEvent(ev.hook))

	var inFlight, peak atomic.Int64
	release := make(chan struct{})
	entered := make(chan struct{}, slots+queued)
	fn := func(ctx context.Context) error {
		n := inFlight.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		defer inFlight.Add(-1)
		entered <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	var dones []<-chan error
	for range slots + queued {
		dones = append(dones, call(ctx, b, fn))
	}
	var admitted, waiting int
	for admitted+waiting < slots+queued {
		switch k := ev.next(t, ctx).Kind; k {
		case EventAdmitted:
			admitted++
		case EventQueued:
			waiting++
		default:
			t.Fatalf("unexpected event %v", k)
		}
	}
	if admitted != slots || waiting != queued {
		t.Fatalf("admitted=%d queued=%d, want %d and %d", admitted, waiting, slots, queued)
	}
	for range slots { // Admitted is emitted just before fn runs: wait until the admitted fns are inside
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(context.Cause(ctx))
		}
	}
	if got := inFlight.Load(); got != slots {
		t.Fatalf("in flight = %d, want %d", got, slots)
	}
	if err := b.Do(ctx, fn); !errors.Is(err, ErrFull) {
		t.Fatalf("Do beyond slots+queue = %v, want ErrFull", err)
	}
	ev.expect(t, ctx, EventRejected)
	close(release)
	for _, d := range dones {
		if err := result(t, ctx, d); err != nil {
			t.Fatalf("Do = %v", err)
		}
	}
	if got := peak.Load(); got != slots {
		t.Fatalf("peak concurrency = %d, want %d", got, slots)
	}
	assertIdle(t, b)
}

func TestDo_failFastWithoutQueue(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	ev := newEvents()
	b := mustNew(t, 1, WithOnEvent(ev.hook))
	h := hold(t, ctx, b)
	ev.expect(t, ctx, EventAdmitted)

	called := false
	err := b.Do(ctx, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, ErrFull) || errors.Is(err, ErrQueueTimeout) || called {
		t.Fatalf("Do = %v (called %v), want ErrFull without calling fn", err, called)
	}
	ev.expect(t, ctx, EventRejected)
	h.release()
	if err := result(t, ctx, h.done); err != nil {
		t.Fatalf("holder = %v", err)
	}
	ev.expect(t, ctx, EventReleased)
	assertIdle(t, b)
}

func TestDo_queueFull(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	ev := newEvents()
	b := mustNew(t, 1, WithQueue(1), WithOnEvent(ev.hook))
	h := hold(t, ctx, b)
	ev.expect(t, ctx, EventAdmitted)
	waiter := call(ctx, b, ok)
	ev.expect(t, ctx, EventQueued)

	if err := b.Do(ctx, ok); !errors.Is(err, ErrFull) {
		t.Fatalf("Do with the queue full = %v, want ErrFull", err)
	}
	ev.expect(t, ctx, EventRejected)
	h.release()
	for _, d := range []<-chan error{h.done, waiter} {
		if err := result(t, ctx, d); err != nil {
			t.Fatalf("Do = %v", err)
		}
	}
	assertIdle(t, b)
}

func TestDo_queueAdmitsInOrder(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	const n = 5
	ev := newEvents()
	b := mustNew(t, 1, WithQueue(n), WithOnEvent(ev.hook))
	h := hold(t, ctx, b)
	ev.expect(t, ctx, EventAdmitted)

	var mu sync.Mutex // guards order
	var order []int
	var dones []<-chan error
	for i := range n {
		dones = append(dones, call(ctx, b, func(context.Context) error {
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			return nil
		}))
		ev.expect(t, ctx, EventQueued) // call i is in the FIFO queue before call i+1 starts
	}
	h.release()
	for _, d := range append(dones, h.done) {
		if err := result(t, ctx, d); err != nil {
			t.Fatalf("Do = %v", err)
		}
	}
	for i, got := range order {
		if got != i {
			t.Fatalf("admission order = %v, want 0..%d", order, n-1)
		}
	}
	if len(order) != n {
		t.Fatalf("ran %d calls, want %d", len(order), n)
	}
	assertIdle(t, b)
}

func TestDo_queueTimeoutOnFakeClock(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	const wait = 5 * time.Second
	f := clock.NewFake(start)
	ev := newEvents()
	b := mustNew(t, 1, WithQueue(1), WithQueueTimeout(wait), WithClock(f), WithOnEvent(ev.hook))
	h := hold(t, ctx, b)
	ev.expect(t, ctx, EventAdmitted)

	called := false
	waiter := call(ctx, b, func(context.Context) error { called = true; return nil })
	ev.expect(t, ctx, EventQueued)
	if err := f.BlockUntil(ctx, 1); err != nil {
		t.Fatalf("BlockUntil: %v", err)
	}
	if err := f.AdvanceContext(ctx, wait-time.Nanosecond); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	select {
	case err := <-waiter:
		t.Fatalf("queued call returned %v before its timeout", err)
	default:
	}
	if err := f.AdvanceContext(ctx, time.Nanosecond); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	err := result(t, ctx, waiter)
	if !errors.Is(err, ErrQueueTimeout) || !errors.Is(err, ErrFull) || called {
		t.Fatalf("Do = %v (called %v), want ErrQueueTimeout (and ErrFull) without calling fn", err, called)
	}
	if e := ev.next(t, ctx); e.Kind != EventTimedOut || e.Waited != wait {
		t.Fatalf("event = %+v, want TimedOut after %v", e, wait)
	}

	// The timed-out call gave its queue place back: another call can queue, and is admitted on release.
	next := call(ctx, b, ok)
	ev.expect(t, ctx, EventQueued)
	h.release()
	for _, d := range []<-chan error{h.done, next} {
		if err := result(t, ctx, d); err != nil {
			t.Fatalf("Do = %v", err)
		}
	}
	ev.expectAnyOrder(t, ctx, EventReleased, EventAdmitted, EventReleased)
	assertIdle(t, b)
}

func TestDo_queuedThenAdmittedStopsTimer(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	f := clock.NewFake(start)
	ev := newEvents()
	b := mustNew(t, 1, WithQueue(1), WithQueueTimeout(time.Minute), WithClock(f), WithOnEvent(ev.hook))
	h := hold(t, ctx, b)
	ev.expect(t, ctx, EventAdmitted)
	waiter := call(ctx, b, ok)
	ev.expect(t, ctx, EventQueued)
	if err := f.AdvanceContext(ctx, 2*time.Second); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	h.release()
	if err := result(t, ctx, h.done); err != nil {
		t.Fatalf("holder = %v", err)
	}
	if err := result(t, ctx, waiter); err != nil {
		t.Fatalf("waiter = %v", err)
	}
	// Holder Released and waiter Admitted race; the waiter's Released comes after its Admitted.
	var kinds []EventKind
	for range 3 {
		e := ev.next(t, ctx)
		kinds = append(kinds, e.Kind)
		if e.Kind == EventAdmitted && e.Waited != 2*time.Second {
			t.Fatalf("Admitted waited %v, want 2s", e.Waited)
		}
	}
	if n := slices.Index(kinds, EventAdmitted); n < 0 || slices.Index(kinds[n:], EventReleased) < 0 {
		t.Fatalf("events = %v, want an Admitted followed by its Released", kinds)
	}
	// The admitted call's timer was stopped: advancing past it times nothing out.
	if err := f.AdvanceContext(ctx, 2*time.Minute); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	ev.none(t)
	assertIdle(t, b)
}

func TestDo_cancelWhileQueuedHoldsNothing(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	ev := newEvents()
	b := mustNew(t, 1, WithQueue(1), WithOnEvent(ev.hook))
	h := hold(t, ctx, b)
	ev.expect(t, ctx, EventAdmitted)

	errGone := errors.New("client went away")
	qctx, cancel := context.WithCancelCause(ctx)
	called := false
	waiter := call(qctx, b, func(context.Context) error { called = true; return nil })
	ev.expect(t, ctx, EventQueued)
	cancel(errGone)
	if err := result(t, ctx, waiter); !errors.Is(err, errGone) || called {
		t.Fatalf("Do = %v (called %v), want the cause without calling fn", err, called)
	}
	ev.expect(t, ctx, EventCancelled)

	// Nothing it did not hold was released: the holder still owns the only slot, so one call queues and the
	// next is rejected, exactly as before the cancelled call.
	next := call(ctx, b, ok)
	ev.expect(t, ctx, EventQueued)
	if err := b.Do(ctx, ok); !errors.Is(err, ErrFull) {
		t.Fatalf("Do = %v, want ErrFull", err)
	}
	ev.expect(t, ctx, EventRejected)
	h.release()
	for _, d := range []<-chan error{h.done, next} {
		if err := result(t, ctx, d); err != nil {
			t.Fatalf("Do = %v", err)
		}
	}
	assertIdle(t, b)
}

func TestDo_ctxDoneBefore(t *testing.T) {
	ev := newEvents()
	b := mustNew(t, 1, WithOnEvent(ev.hook))
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errBoom)
	called := false
	if err := b.Do(ctx, func(context.Context) error { called = true; return nil }); !errors.Is(err, errBoom) || called {
		t.Fatalf("Do = %v (called %v), want the cause without calling fn", err, called)
	}
	ev.expect(t, testCtx(t), EventCancelled)
	assertIdle(t, b)
}

func TestDo_panicReleasesSlot(t *testing.T) {
	ev := newEvents()
	b := mustNew(t, 1, WithOnEvent(ev.hook))
	err := b.Do(context.Background(), func(context.Context) error { panic("kaboom") })
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) {
		t.Fatalf("Do = %v, want *panics.PanicError", err)
	}
	if err := b.Do(context.Background(), nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Do(nil fn) = %v, want ErrPanic", err)
	}
	if err := b.Do(context.Background(), ok); err != nil {
		t.Fatalf("Do after panics = %v, want the slot free (no queue: ErrFull would mean a leaked slot)", err)
	}
	ctx := testCtx(t)
	ev.expect(t, ctx, EventAdmitted, EventReleased, EventAdmitted, EventReleased, EventAdmitted, EventReleased)
	assertIdle(t, b)
}

func TestDo_goexitReleasesSlot(t *testing.T) {
	leak.Verify(t)
	b := mustNew(t, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = b.Do(context.Background(), func(context.Context) error { runtime.Goexit(); return nil })
	}()
	<-done
	assertIdle(t, b)
}

func TestDo_fnErrorUnchangedAndCallerCtx(t *testing.T) {
	type key struct{}
	b := mustNew(t, 1, WithQueue(1), WithQueueTimeout(time.Second), WithClock(clock.NewFake(start)))
	ctx := context.WithValue(context.Background(), key{}, "v")
	err := b.Do(ctx, func(got context.Context) error {
		if got != ctx {
			t.Errorf("fn got a different ctx")
		}
		return errBoom
	})
	if err != errBoom {
		t.Fatalf("Do = %v, want fn's error unchanged", err)
	}
}

func TestDo_nilCtx(t *testing.T) {
	b := mustNew(t, 1)
	var got context.Context
	if err := b.Do(nil, func(ctx context.Context) error { got = ctx; return nil }); err != nil || got == nil {
		t.Fatalf("Do(nil ctx) = %v, ctx %v", err, got)
	}
}

func TestDo_nilAndZeroBulkhead(t *testing.T) {
	var nilB *Bulkhead
	if err := nilB.Do(context.Background(), ok); !errors.Is(err, ErrFull) {
		t.Fatalf("nil Do = %v, want ErrFull", err)
	}
	if v, err := DoValue(context.Background(), nilB, func(context.Context) (int, error) { return 1, nil }); v != 0 || !errors.Is(err, ErrFull) {
		t.Fatalf("nil DoValue = %v, %v", v, err)
	}
	var zero Bulkhead
	if err := zero.Do(context.Background(), ok); !errors.Is(err, ErrFull) {
		t.Fatalf("zero Do = %v, want ErrFull", err)
	}
}

func TestDo_queuedEventMeansEnqueued(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	ev := newEvents()
	b := mustNew(t, 1, WithQueue(1), WithOnEvent(ev.hook))
	h := hold(t, ctx, b)
	ev.expect(t, ctx, EventAdmitted)
	qctx, cancel := context.WithCancel(ctx)
	waiter := call(qctx, b, ok)
	ev.expect(t, ctx, EventQueued)
	// TryAcquire(0) fails only while someone waits in the slot semaphore's queue (semaphore TB-SEMA-001).
	if b.slots.TryAcquire(0) {
		t.Fatalf("EventQueued was emitted before the call joined the semaphore's queue")
	}
	cancel()
	if err := result(t, ctx, waiter); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter = %v", err)
	}
	h.release()
	if err := result(t, ctx, h.done); err != nil {
		t.Fatalf("holder = %v", err)
	}
	assertIdle(t, b)
}

func TestWithOnEvent_seesEverySequence(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	f := clock.NewFake(start)
	ev := newEvents()
	b := mustNew(t, 1, WithQueue(1), WithQueueTimeout(time.Second), WithClock(f), WithOnEvent(ev.hook))

	// Admitted, Released.
	if err := b.Do(ctx, ok); err != nil {
		t.Fatal(err)
	}
	ev.expect(t, ctx, EventAdmitted, EventReleased)

	h := hold(t, ctx, b)
	ev.expect(t, ctx, EventAdmitted)
	// Queued, TimedOut.
	timedOut := call(ctx, b, ok)
	ev.expect(t, ctx, EventQueued)
	if err := f.BlockUntil(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := f.AdvanceContext(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := result(t, ctx, timedOut); !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("Do = %v", err)
	}
	ev.expect(t, ctx, EventTimedOut)
	// Queued, Cancelled.
	qctx, cancel := context.WithCancel(ctx)
	cancelled := call(qctx, b, ok)
	ev.expect(t, ctx, EventQueued)
	cancel()
	if err := result(t, ctx, cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Do = %v", err)
	}
	ev.expect(t, ctx, EventCancelled)
	// Queued, then Rejected for the call that finds the queue full.
	queued := call(ctx, b, ok)
	ev.expect(t, ctx, EventQueued)
	if err := b.Do(ctx, ok); !errors.Is(err, ErrFull) {
		t.Fatalf("Do = %v", err)
	}
	ev.expect(t, ctx, EventRejected)
	// Cancelled before start.
	if err := b.Do(qctx, ok); !errors.Is(err, context.Canceled) {
		t.Fatalf("Do = %v", err)
	}
	ev.expect(t, ctx, EventCancelled)
	// Release the holder: holder Released; queued Admitted, Released.
	h.release()
	for _, d := range []<-chan error{h.done, queued} {
		if err := result(t, ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	ev.expectAnyOrder(t, ctx, EventReleased, EventAdmitted, EventReleased)
	ev.none(t)
	assertIdle(t, b)
}

func TestWithOnEvent_hookPanicJoined(t *testing.T) {
	ran := false
	b := mustNew(t, 1, WithOnEvent(func(e Event) {
		if e.Kind == EventAdmitted {
			panic("hook")
		}
	}))
	err := b.Do(context.Background(), func(context.Context) error { ran = true; return errBoom })
	if !ran || !errors.Is(err, errBoom) || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Do = %v (ran %v), want fn's error joined with the hook's ErrPanic", err, ran)
	}
	if err := b.Do(context.Background(), ok); !errors.Is(err, panics.ErrPanic) || errors.Is(err, ErrFull) {
		t.Fatalf("Do = %v, want only the hook's ErrPanic", err)
	}
	assertIdle(t, b)
}

func TestDoValue(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	b := mustNew(t, 1)
	v, err := DoValue(ctx, b, func(context.Context) (int, error) { return 42, nil })
	if v != 42 || err != nil {
		t.Fatalf("DoValue = %v, %v", v, err)
	}
	if _, err := DoValue(ctx, b, func(context.Context) (int, error) { return 7, errBoom }); err != errBoom {
		t.Fatalf("DoValue error = %v", err)
	}
	if v, err := DoValue(ctx, b, func(context.Context) (int, error) { panic("x") }); v != 0 || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("DoValue panic = %v, %v", v, err)
	}
	h := hold(t, ctx, b)
	if v, err := DoValue(ctx, b, func(context.Context) (int, error) { return 1, nil }); v != 0 || !errors.Is(err, ErrFull) {
		t.Fatalf("DoValue full = %v, %v", v, err)
	}
	h.release()
	if err := result(t, ctx, h.done); err != nil {
		t.Fatal(err)
	}
	assertIdle(t, b)
}

func TestEventKind_String(t *testing.T) {
	for k, want := range map[EventKind]string{
		0: "EventKind(0)", EventAdmitted: "admitted", EventRejected: "rejected", EventQueued: "queued",
		EventTimedOut: "timed-out", EventCancelled: "cancelled", EventReleased: "released", 99: "EventKind(99)",
	} {
		if got := k.String(); got != want {
			t.Errorf("EventKind(%d).String() = %q, want %q", int(k), got, want)
		}
	}
}

func BenchmarkDo_uncontended(b *testing.B) {
	bh, _ := New(4)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_ = bh.Do(ctx, ok)
	}
}
