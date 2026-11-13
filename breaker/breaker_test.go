package breaker_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/breaker"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestDo_closedToOpenAtThreshold(t *testing.T) {
	b, _ := newFake(3, time.Minute, 1)
	trip(t, b, 2)
	wantState(t, b, breaker.Closed)
	if err := b.Do(context.Background(), ok); err != nil { // a success resets the count
		t.Fatalf("Do(ok) = %v", err)
	}
	trip(t, b, 2)
	wantState(t, b, breaker.Closed)
	if err := b.Do(context.Background(), fail); !errors.Is(err, errBoom) {
		t.Fatalf("third failure = %v, want fn's error unchanged", err)
	}
	wantState(t, b, breaker.Open)
}

func TestDo_openRejectsWithoutCallingFn(t *testing.T) {
	b, _ := newFake(1, time.Minute, 1)
	trip(t, b, 1)
	called := false
	err := b.Do(context.Background(), func(context.Context) error { called = true; return nil })
	if !errors.Is(err, breaker.ErrOpen) || called {
		t.Fatalf("Do on open = %v, called %v; want ErrOpen, not called", err, called)
	}
}

func TestDo_openToHalfOpenAfterOpenFor(t *testing.T) {
	b, f := newFake(1, time.Minute, 1)
	trip(t, b, 1)
	f.Advance(time.Minute - time.Nanosecond)
	wantState(t, b, breaker.Open)
	if err := b.Do(context.Background(), ok); !errors.Is(err, breaker.ErrOpen) {
		t.Fatalf("Do 1ns before OpenFor = %v, want ErrOpen", err)
	}
	f.Advance(time.Nanosecond)
	wantState(t, b, breaker.HalfOpen)
	if err := b.Do(context.Background(), ok); err != nil {
		t.Fatalf("probe = %v", err)
	}
	wantState(t, b, breaker.Closed)
}

func TestDo_probesAllSucceedClose(t *testing.T) {
	b, f := newFake(1, time.Minute, 3)
	trip(t, b, 1)
	f.Advance(time.Minute)
	for i := range 3 {
		wantState(t, b, breaker.HalfOpen)
		if err := b.Do(context.Background(), ok); err != nil {
			t.Fatalf("probe %d = %v", i, err)
		}
	}
	wantState(t, b, breaker.Closed)
}

func TestDo_probeFailureReopens(t *testing.T) {
	b, f := newFake(1, time.Minute, 3)
	trip(t, b, 1)
	f.Advance(time.Minute)
	if err := b.Do(context.Background(), ok); err != nil {
		t.Fatal(err)
	}
	if err := b.Do(context.Background(), fail); !errors.Is(err, errBoom) {
		t.Fatalf("failing probe = %v", err)
	}
	wantState(t, b, breaker.Open)
	f.Advance(time.Minute - time.Nanosecond) // a full OpenFor again, from the reopen
	wantState(t, b, breaker.Open)
	f.Advance(time.Nanosecond)
	wantState(t, b, breaker.HalfOpen)
	for range 3 { // the earlier success does not carry over
		if err := b.Do(context.Background(), ok); err != nil {
			t.Fatal(err)
		}
	}
	wantState(t, b, breaker.Closed)
}

// TestDo_probesInFlightBeyondLimit: with 2 probes running, a third call is ErrOpen without calling fn; when both
// succeed the breaker closes.
func TestDo_probesInFlightBeyondLimit(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	b, f := newFake(1, time.Minute, 2)
	trip(t, b, 1)
	f.Advance(time.Minute)

	started, release := make(chan struct{}), make(chan struct{})
	errc := make(chan error, 2)
	for range 2 {
		go func() {
			errc <- b.Do(ctx, func(ctx context.Context) error {
				started <- struct{}{}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("probes did not start")
		}
	}
	called := false
	if err := b.Do(ctx, func(context.Context) error { called = true; return nil }); !errors.Is(err, breaker.ErrOpen) || called {
		t.Fatalf("third probe = %v, called %v; want ErrOpen, not called", err, called)
	}
	close(release)
	for range 2 {
		if err := <-errc; err != nil {
			t.Fatalf("probe = %v", err)
		}
	}
	wantState(t, b, breaker.Closed)
}

func TestDo_contextErrorsNotCounted(t *testing.T) {
	b, f := newFake(1, time.Minute, 1)

	done, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := b.Do(done, func(context.Context) error { called = true; return errBoom }); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("Do(done ctx) = %v, called %v; want Canceled, fn not called", err, called)
	}

	cause := errors.New("shutdown")
	ctx, cancelCause := context.WithCancelCause(context.Background())
	err := b.Do(ctx, func(ctx context.Context) error { cancelCause(cause); return context.Cause(ctx) })
	if !errors.Is(err, cause) {
		t.Fatalf("Do = %v, want the cause", err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	err = b.Do(ctx2, func(ctx context.Context) error { cancel2(); return fmt.Errorf("wrapped: %w", ctx.Err()) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do = %v, want Canceled", err)
	}
	wantState(t, b, breaker.Closed) // threshold 1, yet nothing counted

	// A DeadlineExceeded of fn's own (the caller's ctx is alive) is a failure: a slow dependency trips.
	if err := b.Do(context.Background(), func(context.Context) error { return context.DeadlineExceeded }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	wantState(t, b, breaker.Open)

	// HalfOpen: a cancelled probe frees its slot and decides nothing.
	f.Advance(time.Minute)
	ctx3, cancel3 := context.WithCancel(context.Background())
	if err := b.Do(ctx3, func(ctx context.Context) error { cancel3(); return ctx.Err() }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	wantState(t, b, breaker.HalfOpen)
	if err := b.Do(context.Background(), ok); err != nil {
		t.Fatalf("probe after a cancelled probe = %v, want the slot free", err)
	}
	wantState(t, b, breaker.Closed)
}

func TestDo_ignoredDoesNotResetFailures(t *testing.T) {
	b, _ := newFake(3, time.Minute, 1)
	trip(t, b, 2)
	ctx, cancel := context.WithCancel(context.Background())
	_ = b.Do(ctx, func(ctx context.Context) error { cancel(); return ctx.Err() })
	trip(t, b, 1)
	wantState(t, b, breaker.Open)
}

func TestDo_panicCountsAndIsErrPanic(t *testing.T) {
	b, _ := newFake(2, time.Minute, 1)
	err := b.Do(context.Background(), func(context.Context) error { panic("bad") })
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "bad" {
		t.Fatalf("Do(panic) = %v, want ErrPanic with value", err)
	}
	wantState(t, b, breaker.Closed)
	if err := b.Do(context.Background(), nil); !errors.Is(err, panics.ErrPanic) { // a nil fn panics when called
		t.Fatalf("Do(nil fn) = %v, want ErrPanic", err)
	}
	wantState(t, b, breaker.Open)
}

func TestDo_nilCtx(t *testing.T) {
	b, _ := newFake(1, time.Minute, 1)
	var got context.Context
	if err := b.Do(nil, func(ctx context.Context) error { got = ctx; return nil }); err != nil || got == nil {
		t.Fatalf("Do(nil ctx) = %v, ctx %v", err, got)
	}
}

// TestDo_staleResultIgnored: a call admitted while Closed that finishes after the breaker opened does not
// change the new state.
func TestDo_staleResultIgnored(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	b, _ := newFake(1, time.Minute, 1)
	started, release := make(chan struct{}), make(chan struct{})
	errc := make(chan error, 1)
	go func() {
		errc <- b.Do(ctx, func(context.Context) error {
			close(started)
			<-release
			return nil
		})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("call did not start")
	}
	trip(t, b, 1)
	wantState(t, b, breaker.Open)
	close(release)
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	wantState(t, b, breaker.Open)
}

func TestWithOnStateChange_everyTransitionInOrder(t *testing.T) {
	var r recorder
	b, f := newFake(2, time.Minute, 2, breaker.WithOnStateChange(r.hook))
	trip(t, b, 2) // closed>open
	f.Advance(time.Minute)
	_ = b.State()                        // reports half-open, no transition, no hook
	_ = b.Do(context.Background(), ok)   // open>half-open
	_ = b.Do(context.Background(), fail) // half-open>open
	f.Advance(time.Minute)
	_ = b.Do(context.Background(), ok) // open>half-open
	_ = b.Do(context.Background(), ok) // half-open>closed
	want := "closed>open open>half-open half-open>open open>half-open half-open>closed"
	if got := strings.Join(r.got, " "); got != want {
		t.Fatalf("transitions\n got %s\nwant %s", got, want)
	}
}

func TestWithOnStateChange_panicJoined(t *testing.T) {
	b, _ := newFake(1, time.Minute, 1, breaker.WithOnStateChange(func(from, to breaker.State) { panic("hook") }))
	err := b.Do(context.Background(), fail)
	if !errors.Is(err, errBoom) || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Do = %v, want fn's error joined with ErrPanic", err)
	}
	wantState(t, b, breaker.Open)
	if err := b.Do(context.Background(), ok); err != breaker.ErrOpen { // the breaker still works; no hook ran
		t.Fatalf("Do = %v, want ErrOpen exactly", err)
	}
}

func TestWithOnStateChange_reentrant(t *testing.T) {
	var b *breaker.Breaker
	var seen []string
	b, f := newFake(1, time.Minute, 1, breaker.WithOnStateChange(func(from, to breaker.State) {
		seen = append(seen, to.String()+"/"+b.State().String())
		if to == breaker.Open {
			_ = b.Do(context.Background(), ok) // rejected, no deadlock
		}
	}))
	trip(t, b, 1)
	f.Advance(time.Minute)
	_ = b.Do(context.Background(), ok)
	if got := strings.Join(seen, " "); got != "open/open half-open/half-open closed/closed" {
		t.Fatalf("hook saw %q", got)
	}
}

func TestDoValue(t *testing.T) {
	b, _ := newFake(1, time.Minute, 1)
	v, err := breaker.DoValue(context.Background(), b, func(context.Context) (int, error) { return 42, nil })
	if v != 42 || err != nil {
		t.Fatalf("DoValue = %v, %v", v, err)
	}
	v, err = breaker.DoValue(context.Background(), b, func(context.Context) (int, error) { return 7, errBoom })
	if v != 7 || !errors.Is(err, errBoom) {
		t.Fatalf("DoValue(fail) = %v, %v; want fn's value and error", v, err)
	}
	v, err = breaker.DoValue(context.Background(), b, func(context.Context) (int, error) { return 1, nil })
	if v != 0 || !errors.Is(err, breaker.ErrOpen) {
		t.Fatalf("DoValue(open) = %v, %v; want 0, ErrOpen", v, err)
	}
	s, err := breaker.DoValue(context.Background(), (*breaker.Breaker)(nil), func(context.Context) (string, error) { panic("x") })
	if s != "" || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("DoValue(nil b, panic) = %q, %v", s, err)
	}
}

func TestBreaker_zeroValue(t *testing.T) {
	var b breaker.Breaker // defaults: threshold 5, OpenFor 60 s on the real clock
	trip(t, &b, 4)
	wantState(t, &b, breaker.Closed)
	trip(t, &b, 1)
	wantState(t, &b, breaker.Open)
	if err := b.Do(context.Background(), ok); !errors.Is(err, breaker.ErrOpen) {
		t.Fatal(err)
	}
}

func TestBreaker_nil(t *testing.T) {
	var b *breaker.Breaker
	for range 10 {
		if err := b.Do(context.Background(), fail); !errors.Is(err, errBoom) {
			t.Fatal(err)
		}
	}
	wantState(t, b, breaker.Closed)
}

func TestNew_invalidOptionsAreDefaults(t *testing.T) {
	f := clock.NewFake(start)
	b := breaker.New(nil, breaker.WithFailureThreshold(0), breaker.WithOpenFor(-time.Second),
		breaker.WithHalfOpenProbes(-1), breaker.WithClock(f), breaker.WithOnStateChange(nil))
	trip(t, b, 4)
	wantState(t, b, breaker.Closed)
	trip(t, b, 1)
	wantState(t, b, breaker.Open)
	f.Advance(60*time.Second - time.Nanosecond)
	wantState(t, b, breaker.Open)
	f.Advance(time.Nanosecond)
	if err := b.Do(context.Background(), ok); err != nil { // one probe closes it
		t.Fatal(err)
	}
	wantState(t, b, breaker.Closed)
	if b := breaker.New(breaker.WithClock(nil)); b.State() != breaker.Closed { // nil clock is the real clock
		t.Fatal("New(WithClock(nil))")
	}
}

func TestState_String(t *testing.T) {
	for s, want := range map[breaker.State]string{
		breaker.Closed: "closed", breaker.Open: "open", breaker.HalfOpen: "half-open", 7: "State(7)", -1: "State(-1)",
	} {
		if got := s.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(s), got, want)
		}
	}
}

// TestDo_concurrentProbeLimit: 50 calls race into HalfOpen with 3 probes: exactly 3 run fn, 47 get ErrOpen.
func TestDo_concurrentProbeLimit(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	b, f := newFake(1, time.Minute, 3)
	trip(t, b, 1)
	f.Advance(time.Minute)

	const n = 50
	var running atomic.Int32
	release := make(chan struct{})
	errc := make(chan error, n)
	for range n {
		go func() {
			errc <- b.Do(ctx, func(ctx context.Context) error {
				running.Add(1)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}()
	}
	for range n - 3 {
		if err := <-errc; !errors.Is(err, breaker.ErrOpen) {
			t.Fatalf("rejected call = %v, want ErrOpen", err)
		}
	}
	close(release)
	for range 3 {
		if err := <-errc; err != nil {
			t.Fatalf("probe = %v", err)
		}
	}
	if r := running.Load(); r != 3 {
		t.Fatalf("fn ran %d times, want 3", r)
	}
	wantState(t, b, breaker.Closed)
}

// TestDo_concurrentHookChain: many goroutines call Do with mixed results while the clock moves; the hook sees a
// consistent chain of transitions (each from is the previous to), one call at a time.
func TestDo_concurrentHookChain(t *testing.T) {
	leak.Verify(t)
	var (
		mu      sync.Mutex
		chain   []breaker.State
		inHook  atomic.Int32
		overlap atomic.Bool
	)
	chain = append(chain, breaker.Closed)
	b, f := newFake(3, 3*time.Second, 2, breaker.WithOnStateChange(func(from, to breaker.State) {
		if inHook.Add(1) > 1 {
			overlap.Store(true)
		}
		mu.Lock()
		if last := chain[len(chain)-1]; last != from {
			t.Errorf("transition %v>%v after %v", from, to, last)
		}
		chain = append(chain, to)
		mu.Unlock()
		inHook.Add(-1)
	}))
	trip(t, b, 3) // closed>open, so the chain has at least one transition
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 200 {
				fn := ok
				if (g+i)%3 != 0 {
					fn = fail
				}
				if i%25 == 0 {
					f.Advance(time.Second)
				}
				_ = b.Do(context.Background(), fn)
			}
		})
	}
	wg.Wait()
	if overlap.Load() {
		t.Fatal("hook calls overlapped")
	}
	last, now := chain[len(chain)-1], b.State()
	if now != last && !(last == breaker.Open && now == breaker.HalfOpen) {
		t.Fatalf("State = %v, last transition to %v", now, last)
	}
}

func BenchmarkDo_closed(b *testing.B) {
	br := breaker.New()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_ = br.Do(ctx, ok)
	}
}

// TestWithOnStateChange_pendingBounded: while the hook is blocked, other goroutines' transitions queue up to
// 64; beyond that they are dropped, and the hook sees the gap as a broken chain, never an unbounded queue.
func TestWithOnStateChange_pendingBounded(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var got []string
	first := true
	b, f := newFake(1, time.Second, 1, breaker.WithOnStateChange(func(from, to breaker.State) {
		got = append(got, from.String()+">"+to.String())
		if first {
			first = false
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}))
	errc := make(chan error, 1)
	go func() { errc <- b.Do(ctx, fail) }() // closed>open; its hook call blocks
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("hook not entered")
	}
	for range 100 { // each round: open>half-open, half-open>open; queued, not delivered
		f.Advance(time.Second)
		_ = b.Do(ctx, fail)
	}
	close(release)
	<-errc
	if len(got) != 1+64 {
		t.Fatalf("hook calls = %d, want 65 (1 + the 64 queued)", len(got))
	}
	wantState(t, b, breaker.Open)
}
