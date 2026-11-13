package clock_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func ExampleFake() {
	f := clock.NewFake(start)
	t := f.NewTimer(2 * time.Second)

	f.Advance(time.Second)
	select {
	case <-t.C():
		fmt.Println("fired early")
	default:
		fmt.Println("not yet at", f.Now().Format(time.TimeOnly))
	}

	f.Advance(time.Second)
	fmt.Println("fired at", (<-t.C()).Format(time.TimeOnly))
	// Output:
	// not yet at 00:00:01
	// fired at 00:00:02
}

func ExampleFake_BlockUntil() {
	f := clock.NewFake(start)
	done := make(chan string)
	go func() { // the code under test: waits 5 s on the clock it was given
		t := f.NewTimer(5 * time.Second)
		done <- (<-t.C()).Format(time.TimeOnly)
	}()

	// Advancing before the goroutine has armed its timer would fire nothing; wait for it instead of sleeping.
	if err := f.BlockUntil(context.Background(), 1); err != nil {
		fmt.Println(err)
		return
	}
	f.Advance(5 * time.Second)
	fmt.Println("woke at", <-done)
	// Output:
	// woke at 00:00:05
}

func ExampleFake_AfterFunc() {
	f := clock.NewFake(start)
	f.AfterFunc(time.Minute, func() { fmt.Println("callback at", f.Now().Format(time.TimeOnly)) })
	f.Advance(time.Hour) // returns after the callback has returned
	fmt.Println("now", f.Now().Format(time.TimeOnly))
	// Output:
	// callback at 00:01:00
	// now 01:00:00
}

func ExampleFake_NewTicker() {
	f := clock.NewFake(start)
	tk := f.NewTicker(time.Second)
	defer tk.Stop()
	for range 3 {
		f.Advance(time.Second)
		fmt.Println((<-tk.C()).Format(time.TimeOnly))
	}
	// Output:
	// 00:00:01
	// 00:00:02
	// 00:00:03
}

// expired is code under test: it takes a Clock instead of calling time.Now.
func expired(c clock.Clock, issued time.Time, ttl time.Duration) bool {
	return c.Now().Sub(issued) >= ttl
}

func ExampleClock() {
	f := clock.NewFake(start)
	issued := f.Now()
	fmt.Println(expired(f, issued, time.Minute))
	f.Advance(time.Minute)
	fmt.Println(expired(f, issued, time.Minute))
	fmt.Println(expired(clock.Real(), time.Now(), time.Hour)) // production passes clock.Real()
	// Output:
	// false
	// true
	// false
}

func ExampleTimer() {
	f := clock.NewFake(start)
	t := f.NewTimer(time.Second)
	f.Advance(time.Second)                    // fired; the value waits on C
	fmt.Println("Reset:", t.Reset(time.Hour)) // drains it: true, as go1.23 time.Timer
	select {
	case <-t.C():
		fmt.Println("stale value")
	default:
		fmt.Println("no stale value")
	}
	fmt.Println("Stop:", t.Stop())
	// Output:
	// Reset: true
	// no stale value
	// Stop: true
}

// recv is a non-blocking receive: the value and true when one is waiting.
func recv(c <-chan time.Time) (time.Time, bool) {
	select {
	case v := <-c:
		return v, true
	default:
		return time.Time{}, false
	}
}

func TestFake_Advance_deadlineOrder(t *testing.T) {
	leak.Verify(t)
	f := clock.NewFake(start)
	var got []string
	at := func(name string, d time.Duration) {
		f.AfterFunc(d, func() { got = append(got, fmt.Sprintf("%s@%v", name, f.Now().Sub(start))) })
	}
	at("e", 5*time.Second)
	at("a", 1*time.Second)
	at("c1", 3*time.Second)
	at("c2", 3*time.Second) // same deadline as c1, armed after it
	at("z", 10*time.Second)
	at("b", 2*time.Second)
	f.Advance(6 * time.Second)
	want := []string{"a@1s", "b@2s", "c1@3s", "c2@3s", "e@5s"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("fired %v, want %v", got, want)
	}
	if n := f.Now().Sub(start); n != 6*time.Second {
		t.Fatalf("Now after Advance = start+%v, want start+6s", n)
	}
	f.Advance(4 * time.Second)
	if got[len(got)-1] != "z@10s" {
		t.Fatalf("last fired %q, want z@10s", got[len(got)-1])
	}
}

func TestFake_Advance_valueIsDeadline(t *testing.T) {
	f := clock.NewFake(start)
	a, b := f.NewTimer(3*time.Second), f.NewTimer(time.Second)
	f.Advance(time.Minute)
	if v, ok := recv(a.C()); !ok || !v.Equal(start.Add(3*time.Second)) {
		t.Fatalf("a = %v, %v; want start+3s", v, ok)
	}
	if v, ok := recv(b.C()); !ok || !v.Equal(start.Add(time.Second)) {
		t.Fatalf("b = %v, %v; want start+1s", v, ok)
	}
}

func TestFake_Advance_zeroAndNegative(t *testing.T) {
	f := clock.NewFake(start)
	tm := f.NewTimer(0)
	if _, ok := recv(tm.C()); ok {
		t.Fatal("NewTimer(0) fired before any Advance")
	}
	f.Advance(-time.Hour)
	if !f.Now().Equal(start) {
		t.Fatalf("Advance(-1h) moved time to %v", f.Now())
	}
	if v, ok := recv(tm.C()); !ok || !v.Equal(start) {
		t.Fatalf("NewTimer(0) after Advance(<=0) = %v, %v; want start", v, ok)
	}
	neg := f.NewTimer(-time.Second) // a negative d is due now, not in the past
	f.Advance(0)
	if v, ok := recv(neg.C()); !ok || !v.Equal(start) {
		t.Fatalf("NewTimer(-1s) = %v, %v; want start", v, ok)
	}
}

func TestFake_zeroValue(t *testing.T) {
	var f clock.Fake
	if !f.Now().IsZero() {
		t.Fatalf("zero Fake Now = %v, want the zero time", f.Now())
	}
	tm := f.NewTimer(time.Second)
	f.Advance(time.Second)
	if v, ok := recv(tm.C()); !ok || !v.Equal(time.Time{}.Add(time.Second)) {
		t.Fatalf("zero Fake timer = %v, %v", v, ok)
	}
}

// TestTimer_Stop_noStaleValue: go1.23, "any receive from t.C after Stop has returned is guaranteed to block".
func TestTimer_Stop_noStaleValue(t *testing.T) {
	f := clock.NewFake(start)
	tm := f.NewTimer(time.Second)
	f.Advance(time.Second) // fired, value not received
	if !tm.Stop() {
		t.Fatal("Stop of a fired, unreceived timer = false; go1.23 reports it active (true)")
	}
	if v, ok := recv(tm.C()); ok {
		t.Fatalf("stale value %v received after Stop", v)
	}
	if tm.Stop() {
		t.Fatal("second Stop = true, want false")
	}
}

func TestTimer_Stop_beforeFire(t *testing.T) {
	f := clock.NewFake(start)
	tm := f.NewTimer(time.Second)
	if !tm.Stop() {
		t.Fatal("Stop of an armed timer = false, want true")
	}
	f.Advance(time.Hour)
	if v, ok := recv(tm.C()); ok {
		t.Fatalf("stopped timer sent %v", v)
	}
}

// TestTimer_Reset_noStaleValue: go1.23, "any receive from t.C after Reset has returned is guaranteed not to
// receive a time value corresponding to the previous timer settings".
func TestTimer_Reset_noStaleValue(t *testing.T) {
	f := clock.NewFake(start)
	tm := f.NewTimer(time.Second)
	f.Advance(time.Second) // fired at start+1s, not received
	if !tm.Reset(2 * time.Second) {
		t.Fatal("Reset of a fired, unreceived timer = false; go1.23 reports it active (true)")
	}
	if v, ok := recv(tm.C()); ok {
		t.Fatalf("stale value %v received after Reset", v)
	}
	f.Advance(time.Second)
	if v, ok := recv(tm.C()); ok {
		t.Fatalf("fired early at %v", v)
	}
	f.Advance(time.Second)
	if v, ok := recv(tm.C()); !ok || !v.Equal(start.Add(3*time.Second)) {
		t.Fatalf("after Reset got %v, %v; want start+3s", v, ok)
	}
	if tm.Reset(time.Second) {
		t.Fatal("Reset after the value was received = true, want false")
	}
}

func TestTimer_Reset_armedMovesDeadline(t *testing.T) {
	f := clock.NewFake(start)
	tm := f.NewTimer(time.Second)
	if !tm.Reset(5 * time.Second) {
		t.Fatal("Reset of an armed timer = false, want true")
	}
	f.Advance(4 * time.Second)
	if _, ok := recv(tm.C()); ok {
		t.Fatal("fired at the old deadline")
	}
	f.Advance(time.Second)
	if v, ok := recv(tm.C()); !ok || !v.Equal(start.Add(5*time.Second)) {
		t.Fatalf("got %v, %v; want start+5s", v, ok)
	}
}

// goid returns the calling goroutine's id from its stack header.
func goid() uint64 {
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	buf = bytes.TrimPrefix(buf, []byte("goroutine "))
	id, _ := strconv.ParseUint(string(buf[:bytes.IndexByte(buf, ' ')]), 10, 64)
	return id
}

func TestFake_AfterFunc_ownGoroutineWaited(t *testing.T) {
	leak.Verify(t)
	f := clock.NewFake(start)
	var (
		ran      bool
		callerID = goid()
		cbID     uint64
	)
	tm := f.AfterFunc(time.Second, func() { cbID = goid(); ran = true })
	if tm.C() != nil {
		t.Fatal("C() of an AfterFunc timer is not nil")
	}
	f.Advance(time.Second)
	if !ran { // read without a lock: Advance waited, so -race sees the happens-before edge
		t.Fatal("Advance returned before the callback ran")
	}
	if cbID == callerID {
		t.Fatal("callback ran on the goroutine that called Advance; want its own goroutine")
	}
	if tm.Stop() {
		t.Fatal("Stop after the callback ran = true, want false")
	}
}

func TestFake_AfterFunc_stopBeforeFire(t *testing.T) {
	leak.Verify(t)
	f := clock.NewFake(start)
	ran := false
	tm := f.AfterFunc(time.Second, func() { ran = true })
	if !tm.Stop() {
		t.Fatal("Stop before fire = false, want true")
	}
	f.Advance(time.Hour)
	if ran {
		t.Fatal("stopped callback ran")
	}
	if tm.Reset(time.Second) {
		t.Fatal("Reset of a stopped AfterFunc timer = true, want false")
	}
	f.Advance(time.Second)
	if !ran {
		t.Fatal("callback did not run after Reset")
	}
}

func TestFake_AfterFunc_rearmInsideWindow(t *testing.T) {
	leak.Verify(t)
	f := clock.NewFake(start)
	var fired []time.Duration
	var tm clock.Timer
	tm = f.AfterFunc(time.Second, func() {
		fired = append(fired, f.Now().Sub(start))
		tm.Reset(time.Second) // the callback may call any method but Advance
	})
	f.Advance(3500 * time.Millisecond)
	if fmt.Sprint(fired) != "[1s 2s 3s]" {
		t.Fatalf("fired at %v, want [1s 2s 3s]", fired)
	}
	tm.Stop()
}

func TestFake_AfterFunc_nilFunc(t *testing.T) {
	f := clock.NewFake(start)
	f.AfterFunc(time.Second, nil)
	f.Advance(time.Second) // does nothing, does not panic
}

func TestFake_AfterFunc_panic(t *testing.T) {
	leak.Verify(t)
	f := clock.NewFake(start)
	boom := errors.New("boom")
	f.AfterFunc(time.Second, func() { panic(boom) })
	later := f.NewTimer(2 * time.Second)
	func() {
		defer func() {
			err, ok := recover().(error)
			if !ok || !errors.Is(err, boom) {
				t.Fatalf("Advance re-panicked with %v, want an error wrapping boom", err)
			}
			if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "TestFake_AfterFunc_panic") {
				t.Fatalf("re-panic message lacks the value or the callback's stack:\n%s", err)
			}
		}()
		f.Advance(3 * time.Second)
		t.Fatal("Advance returned normally after a panicking callback")
	}()
	// Documented state: Now at the callback's deadline, the later timer not fired, the fake usable.
	if got := f.Now().Sub(start); got != time.Second {
		t.Fatalf("Now after the panic = start+%v, want start+1s", got)
	}
	if _, ok := recv(later.C()); ok {
		t.Fatal("timer due after the panicking callback fired")
	}
	f.Advance(time.Second)
	if v, ok := recv(later.C()); !ok || !v.Equal(start.Add(2*time.Second)) {
		t.Fatalf("fake unusable after a callback panic: %v, %v", v, ok)
	}
}

func TestFake_AfterFunc_goexit(t *testing.T) {
	leak.Verify(t)
	f := clock.NewFake(start)
	f.AfterFunc(time.Second, runtime.Goexit) // as t.FailNow inside a callback would
	f.Advance(time.Second)                   // returns, does not hang
}

func TestTicker_ticksEveryPeriod(t *testing.T) {
	f := clock.NewFake(start)
	tk := f.NewTicker(time.Second)
	defer tk.Stop()
	for i := 1; i <= 5; i++ {
		f.Advance(time.Second)
		if v, ok := recv(tk.C()); !ok || !v.Equal(start.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("tick %d = %v, %v", i, v, ok)
		}
	}
}

func TestTicker_dropsForSlowReceiver(t *testing.T) {
	f := clock.NewFake(start)
	tk := f.NewTicker(time.Second)
	defer tk.Stop()
	f.Advance(time.Hour) // 3600 ticks due, nobody reading: one kept, the rest dropped in one step
	if v, ok := recv(tk.C()); !ok || !v.Equal(start.Add(time.Second)) {
		t.Fatalf("first tick = %v, %v; want start+1s", v, ok)
	}
	if _, ok := recv(tk.C()); ok {
		t.Fatal("more than one tick buffered")
	}
	f.Advance(time.Second)
	if v, ok := recv(tk.C()); !ok || !v.Equal(start.Add(time.Hour+time.Second)) {
		t.Fatalf("next tick = %v, %v; want start+1h1s", v, ok)
	}
}

func TestTicker_StopReset_noStaleValue(t *testing.T) {
	f := clock.NewFake(start)
	tk := f.NewTicker(time.Second)
	f.Advance(time.Second)
	tk.Stop()
	if v, ok := recv(tk.C()); ok {
		t.Fatalf("stale tick %v after Stop", v)
	}
	f.Advance(time.Hour)
	if _, ok := recv(tk.C()); ok {
		t.Fatal("stopped ticker ticked")
	}
	tk.Reset(2 * time.Second)
	f.Advance(time.Second)
	if _, ok := recv(tk.C()); ok {
		t.Fatal("ticked before the new period")
	}
	f.Advance(time.Second)
	if v, ok := recv(tk.C()); !ok || !v.Equal(start.Add(time.Hour+3*time.Second)) {
		t.Fatalf("after Reset = %v, %v; want start+1h3s", v, ok)
	}
	f.Advance(2 * time.Second) // fired, not received
	tk.Reset(time.Second)
	if v, ok := recv(tk.C()); ok {
		t.Fatalf("stale tick %v after Reset", v)
	}
	tk.Stop()
}

func TestTicker_nonPositive(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	f := clock.NewFake(start)
	tk := f.NewTicker(0)
	f.Advance(time.Hour)
	if _, ok := recv(tk.C()); ok {
		t.Fatal("NewTicker(0) ticked")
	}
	tk.Reset(time.Second)
	if err := f.BlockUntil(ctx, 1); err != nil {
		t.Fatal(err)
	}
	tk.Reset(-time.Second) // stops it
	f.Advance(time.Hour)
	if _, ok := recv(tk.C()); ok {
		t.Fatal("ticker reset to a negative period ticked")
	}
}

func TestFake_BlockUntil_armed(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	f := clock.NewFake(start)
	const n = 3
	var wg sync.WaitGroup
	got := make(chan time.Time, n)
	for i := range n {
		wg.Go(func() {
			tm := f.NewTimer(time.Duration(i+1) * time.Second)
			select {
			case v := <-tm.C():
				got <- v
			case <-ctx.Done():
			}
		})
	}
	if err := f.BlockUntil(ctx, n); err != nil {
		t.Fatalf("BlockUntil(%d) = %v", n, err)
	}
	f.Advance(n * time.Second)
	wg.Wait()
	close(got)
	count := 0
	for range got {
		count++
	}
	if count != n {
		t.Fatalf("%d timers fired, want %d", count, n)
	}
}

func TestFake_BlockUntil_alreadySatisfied(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // even a done ctx: the count is checked first
	f := clock.NewFake(start)
	if err := f.BlockUntil(ctx, 0); err != nil {
		t.Fatalf("BlockUntil(0) = %v, want nil", err)
	}
	f.NewTimer(time.Second)
	if err := f.BlockUntil(ctx, 1); err != nil {
		t.Fatalf("BlockUntil(1) with one armed = %v, want nil", err)
	}
}

func TestFake_BlockUntil_cancelBefore(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("gave up")
	cancel(cause)
	f := clock.NewFake(start)
	if err := f.BlockUntil(ctx, 1); !errors.Is(err, cause) {
		t.Fatalf("BlockUntil on a cancelled ctx = %v, want %v", err, cause)
	}
}

func TestFake_BlockUntil_cancelDuring(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	waitCtx, stop := context.WithCancelCause(ctx)
	cause := errors.New("stopped")
	f := clock.NewFake(start)
	res := make(chan error, 1)
	go func() { res <- f.BlockUntil(waitCtx, 2) }()
	f.NewTimer(time.Second) // one armed: not enough
	stop(cause)
	select {
	case err := <-res:
		if !errors.Is(err, cause) {
			t.Fatalf("BlockUntil = %v, want %v", err, cause)
		}
	case <-ctx.Done():
		t.Fatal("BlockUntil did not return after cancel")
	}
	f.NewTimer(time.Second) // a later count change finds no waiter left behind
}

func TestFake_BlockUntil_stopReducesCount(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f := clock.NewFake(start)
	tm := f.NewTimer(time.Second)
	tm.Stop()
	if err := f.BlockUntil(ctx, 1); err == nil {
		t.Fatal("BlockUntil(1) = nil with no timer armed")
	}
}

func TestFake_concurrent(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	f := clock.NewFake(start)
	const workers, rounds = 8, 200
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range rounds {
				tm := f.NewTimer(time.Duration(i%5) * time.Millisecond)
				tk := f.NewTicker(time.Millisecond)
				af := f.AfterFunc(time.Millisecond, func() { _ = f.Now() })
				recv(tm.C())
				recv(tk.C())
				tm.Reset(time.Millisecond)
				tm.Stop()
				tk.Reset(2 * time.Millisecond)
				tk.Stop()
				af.Stop()
				_ = f.Now()
				if ctx.Err() != nil {
					return
				}
			}
		})
	}
	for range workers * rounds { // a fixed number of steps, concurrent with the workers; no waiting loop
		f.Advance(time.Millisecond)
	}
	wg.Wait() // the workers run a bounded number of rounds and never block
	f.Advance(time.Second)
}

// TestFake_AdvanceContext_callbackBlocksOnCaller: the callback sends on a channel the test reads only after
// Advance returns, which deadlocks Advance; AdvanceContext returns when ctx is cancelled.
func TestFake_AdvanceContext_callbackBlocksOnCaller(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	f := clock.NewFake(start)
	started, ch := make(chan struct{}), make(chan int)
	f.AfterFunc(time.Second, func() {
		close(started)
		select {
		case ch <- 1:
		case <-bound.Done():
		}
	})
	go func() { <-started; cancel() }()
	if err := f.AdvanceContext(ctx, 2*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("AdvanceContext = %v, want context.Canceled", err)
	}
	if got := f.Now().Sub(start); got != time.Second {
		t.Fatalf("Now = start+%v, want start+1s (the callback's deadline)", got)
	}
	select {
	case <-ch: // the callback can now finish
	case <-bound.Done():
		t.Fatal("callback never sent")
	}
	if err := f.AdvanceContext(bound, time.Second); err != nil { // waits for the left-over callback first
		t.Fatalf("next AdvanceContext = %v", err)
	}
	if got := f.Now().Sub(start); got != 2*time.Second {
		t.Fatalf("Now = start+%v, want start+2s", got)
	}
}

// TestFake_AdvanceContext_callbackWaitsOnLaterTimer: the callback waits on a fake timer due later in the same
// window, which only the blocked Advance could fire.
func TestFake_AdvanceContext_callbackWaitsOnLaterTimer(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	f := clock.NewFake(start)
	started, release := make(chan struct{}), make(chan struct{})
	f.AfterFunc(time.Second, func() {
		tm := f.NewTimer(500 * time.Millisecond)
		close(started)
		select {
		case <-tm.C():
		case <-release:
		case <-bound.Done():
		}
	})
	go func() { <-started; cancel() }()
	if err := f.AdvanceContext(ctx, 3*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("AdvanceContext = %v, want context.Canceled", err)
	}
	close(release)
	if err := f.AdvanceContext(bound, 0); err != nil {
		t.Fatalf("AdvanceContext after release = %v", err)
	}
	if got := f.Now().Sub(start); got != time.Second {
		t.Fatalf("Now = start+%v, want start+1s", got)
	}
}

func TestFake_AdvanceContext_cancelBefore(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("not now")
	cancel(cause)
	f := clock.NewFake(start)
	tm := f.NewTimer(0)
	if err := f.AdvanceContext(ctx, time.Hour); !errors.Is(err, cause) {
		t.Fatalf("AdvanceContext = %v, want %v", err, cause)
	}
	if !f.Now().Equal(start) {
		t.Fatal("time moved on a cancelled ctx")
	}
	if _, ok := recv(tm.C()); ok {
		t.Fatal("a timer fired on a cancelled ctx")
	}
}

func TestFake_nilContext(t *testing.T) {
	f := clock.NewFake(start)
	f.NewTimer(time.Second)
	//lint:ignore SA1012 a nil ctx is documented as context.Background()
	if err := f.BlockUntil(nil, 1); err != nil { //nolint:staticcheck
		t.Fatalf("BlockUntil(nil, 1) = %v", err)
	}
	if err := f.AdvanceContext(nil, time.Second); err != nil { //nolint:staticcheck
		t.Fatalf("AdvanceContext(nil, 1s) = %v", err)
	}
}

func BenchmarkFake_Advance(b *testing.B) {
	b.ReportAllocs()
	f := clock.NewFake(start)
	for b.Loop() {
		tm := f.NewTimer(time.Second)
		f.Advance(time.Second)
		<-tm.C()
	}
}
