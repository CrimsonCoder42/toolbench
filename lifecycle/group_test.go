package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/lifecycle"
	"github.com/CrimsonCoder42/toolbench/panics"
)

var (
	errBoom = errors.New("boom")
	errStop = errors.New("stop failed")
)

func TestGroup_startOrderReverseStop(t *testing.T) {
	for _, n := range []int{0, 1, 2, 5} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := context.WithCancel(bounded(t))
			defer cancel()
			var r recorder
			var g lifecycle.Group
			var want []string
			for i := range n {
				name := fmt.Sprint("c", i)
				if err := g.Add(comp(&r, name)); err != nil {
					t.Fatal(err)
				}
				want = append(want, "start "+name)
			}
			for i := n - 1; i >= 0; i-- {
				want = append(want, fmt.Sprint("stop c", i))
			}
			if err := g.Add(trigger(cancel)); err != nil {
				t.Fatal(err)
			}
			if err := await(t, bounded(t), runAsync(ctx, &g)); err != nil {
				t.Fatalf("Run = %v, want nil", err)
			}
			if got := r.get(); !slices.Equal(got, want) {
				t.Fatalf("order = %v, want %v", got, want)
			}
		})
	}
}

func TestGroup_startFailureStopsEarlierInReverse(t *testing.T) {
	const n = 4
	for k := 1; k <= n; k++ {
		t.Run(fmt.Sprint("k=", k), func(t *testing.T) {
			leak.Verify(t)
			var r recorder
			var g lifecycle.Group
			for i := 1; i <= n; i++ {
				c := comp(&r, fmt.Sprint(i))
				if i == k {
					c.Start = func(context.Context) error { r.add(fmt.Sprint("fail ", i)); return errBoom }
				}
				_ = g.Add(c)
			}
			err := g.Run(bounded(t)) // never waits on ctx: start fails
			if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), fmt.Sprintf("start %d", k)) {
				t.Fatalf("Run = %v, want errBoom naming start %d", err, k)
			}
			var want []string
			for i := 1; i < k; i++ {
				want = append(want, fmt.Sprint("start ", i))
			}
			want = append(want, fmt.Sprint("fail ", k))
			for i := k - 1; i >= 1; i-- {
				want = append(want, fmt.Sprint("stop ", i))
			}
			if got := r.get(); !slices.Equal(got, want) {
				t.Fatalf("order = %v, want %v", got, want)
			}
		})
	}
}

func TestGroup_startPanicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	var r recorder
	var g lifecycle.Group
	_ = g.Add(comp(&r, "a"))
	_ = g.Add(lifecycle.Component{Name: "b", Start: func(context.Context) error { panic("start b") }})
	_ = g.Add(comp(&r, "c"))
	err := g.Run(bounded(t))
	if !errors.Is(err, panics.ErrPanic) || !strings.Contains(err.Error(), "start b") {
		t.Fatalf("Run = %v, want ErrPanic naming b", err)
	}
	if got, want := r.get(), []string{"start a", "stop a"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestGroup_stopErrorsJoinedAndPanicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancel(bounded(t))
	defer cancel()
	var r recorder
	var g lifecycle.Group
	a := comp(&r, "a")
	a.Stop = func(context.Context) error { r.add("stop a"); return errStop }
	b := comp(&r, "b")
	b.Stop = func(context.Context) error { r.add("stop b"); panic("stop b") }
	_ = g.Add(a)
	_ = g.Add(b)
	_ = g.Add(comp(&r, "c"))
	_ = g.Add(trigger(cancel))
	err := await(t, bounded(t), runAsync(ctx, &g))
	if !errors.Is(err, errStop) || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Run = %v, want errStop and ErrPanic joined", err)
	}
	if !strings.Contains(err.Error(), "stop a") || !strings.Contains(err.Error(), "stop b") {
		t.Fatalf("Run = %v, want both components named", err)
	}
	if got, want := r.get(), []string{"start a", "start b", "start c", "stop c", "stop b", "stop a"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v (a failing stop does not stop the others)", got, want)
	}
}

// graceRun runs g with a fake clock, cancels it once every component has started, and fires the grace timer.
func graceRun(t *testing.T, g *lifecycle.Group, grace time.Duration) error {
	t.Helper()
	tctx := bounded(t)
	ctx, cancel := context.WithCancel(tctx)
	defer cancel()
	fc := clock.NewFake(time.Unix(0, 0))
	if err := g.Add(trigger(cancel)); err != nil { // cancel only once every component has started
		t.Fatal(err)
	}
	done := runAsync(ctx, g, lifecycle.WithClock(fc), lifecycle.WithGrace(grace))
	if err := fc.BlockUntil(tctx, 1); err != nil { // the grace timer is armed when shutdown begins
		t.Fatal(err)
	}
	if err := fc.AdvanceContext(tctx, grace); err != nil {
		t.Fatal(err)
	}
	return await(t, tctx, done)
}

func TestGroup_graceDeadlineNamesWhoDidNotStop(t *testing.T) {
	leak.Verify(t)
	var r recorder
	var g lifecycle.Group
	cause := make(chan error, 1)
	_ = g.Add(comp(&r, "db"))
	_ = g.Add(comp(&r, "cache"))
	_ = g.Add(lifecycle.Component{Name: "server", Stop: func(ctx context.Context) error {
		<-ctx.Done() // honours the stop ctx: returns when the grace period ends
		cause <- context.Cause(ctx)
		return ctx.Err()
	}})
	err := graceRun(t, &g, 5*time.Second)
	if !errors.Is(err, lifecycle.ErrStopTimeout) {
		t.Fatalf("Run = %v, want ErrStopTimeout", err)
	}
	for _, s := range []string{"stop server still stopping", "not stopped: stop cache, stop db"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("Run = %q, want it to contain %q", err, s)
		}
	}
	select {
	case c := <-cause:
		if !errors.Is(c, lifecycle.ErrStopTimeout) {
			t.Fatalf("stop ctx cause = %v, want ErrStopTimeout", c)
		}
	case <-bounded(t).Done():
		t.Fatal("the stop ctx was not cancelled")
	}
	if got, want := r.get(), []string{"start db", "start cache"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestGroup_graceStopIgnoringCtxIsAbandoned(t *testing.T) {
	leak.Verify(t)
	release := make(chan struct{})
	defer close(release) // the abandoned Stop returns after Run did; leak.Verify waits for it
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "stuck", Stop: func(context.Context) error { <-release; return nil }})
	err := graceRun(t, &g, time.Second)
	if !errors.Is(err, lifecycle.ErrStopTimeout) || !strings.Contains(err.Error(), "stop stuck") {
		t.Fatalf("Run = %v, want ErrStopTimeout naming stuck", err)
	}
}

func TestGroup_graceNotReachedWhenStopsReturn(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancel(bounded(t))
	defer cancel()
	fc := clock.NewFake(time.Unix(0, 0)) // never advanced: the deadline cannot pass
	var r recorder
	var g lifecycle.Group
	_ = g.Add(comp(&r, "a"))
	_ = g.Add(trigger(cancel))
	if err := await(t, bounded(t), runAsync(ctx, &g, lifecycle.WithClock(fc), lifecycle.WithGrace(time.Hour))); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

func TestGroup_stopCtxKeepsValuesAndIsNotCancelledByRunCtx(t *testing.T) {
	leak.Verify(t)
	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(bounded(t), key{}, "v"))
	defer cancel()
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Stop: func(sctx context.Context) error {
		if sctx.Value(key{}) != "v" {
			return errors.New("value lost")
		}
		if sctx.Err() != nil {
			return errors.New("stop ctx already done")
		}
		return nil
	}})
	_ = g.Add(trigger(cancel))
	if err := await(t, bounded(t), runAsync(ctx, &g)); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

func TestGroup_withNotify(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	var hookCtx context.Context
	var signal context.CancelCauseFunc
	hookStopped := make(chan struct{})
	notify := func(ctx context.Context) (context.Context, context.CancelFunc) {
		hookCtx, signal = context.WithCancelCause(ctx)
		return hookCtx, func() { signal(nil); close(hookStopped) }
	}
	var r recorder
	var g lifecycle.Group
	started := make(chan struct{})
	_ = g.Add(comp(&r, "a"))
	_ = g.Add(lifecycle.Component{Name: "b", Start: func(ctx context.Context) error {
		if ctx != hookCtx {
			return errors.New("Start did not get the notify context")
		}
		close(started)
		return nil
	}})
	done := runAsync(tctx, &g, lifecycle.WithNotify(notify))
	select {
	case <-started:
	case <-tctx.Done():
		t.Fatal("not started")
	}
	signal(errors.New("signal: terminated")) // what a test hook does in place of SIGTERM
	if err := await(t, tctx, done); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	select {
	case <-hookStopped:
	default:
		t.Fatal("the notify CancelFunc was not called before Run returned")
	}
	if got, want := r.get(), []string{"start a", "stop a"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestGroup_withNotifyNilResultsAndPanic(t *testing.T) {
	leak.Verify(t)
	t.Run("nil ctx and stop", func(t *testing.T) {
		ctx, cancel := context.WithCancel(bounded(t))
		defer cancel()
		var g lifecycle.Group
		_ = g.Add(trigger(cancel))
		nilHook := func(context.Context) (context.Context, context.CancelFunc) { return nil, nil }
		if err := await(t, bounded(t), runAsync(ctx, &g, lifecycle.WithNotify(nilHook), nil)); err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	})
	t.Run("panic", func(t *testing.T) {
		var r recorder
		var g lifecycle.Group
		_ = g.Add(comp(&r, "a"))
		hook := func(context.Context) (context.Context, context.CancelFunc) { panic("hook") }
		err := g.Run(bounded(t), lifecycle.WithNotify(hook))
		if !errors.Is(err, panics.ErrPanic) {
			t.Fatalf("Run = %v, want ErrPanic", err)
		}
		if got := r.get(); len(got) != 0 {
			t.Fatalf("calls = %v, want none", got)
		}
	})
}

func TestGroup_cancelBefore(t *testing.T) {
	leak.Verify(t)
	cause := errors.New("cancelled before")
	ctx, cancel := context.WithCancelCause(bounded(t))
	cancel(cause)
	var r recorder
	var g lifecycle.Group
	_ = g.Add(comp(&r, "a"))
	if err := g.Run(ctx); !errors.Is(err, cause) {
		t.Fatalf("Run = %v, want the cause", err)
	}
	if got := r.get(); len(got) != 0 {
		t.Fatalf("calls = %v, want none", got)
	}
}

func TestGroup_cancelMidStart(t *testing.T) {
	leak.Verify(t)
	cause := errors.New("cancelled mid-start")
	t.Run("Start ignores ctx", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(bounded(t))
		defer cancel(nil)
		var r recorder
		var g lifecycle.Group
		_ = g.Add(comp(&r, "a"))
		b := comp(&r, "b")
		b.Start = func(context.Context) error { r.add("start b"); cancel(cause); return nil }
		_ = g.Add(b)
		_ = g.Add(comp(&r, "c"))
		if err := g.Run(ctx); !errors.Is(err, cause) {
			t.Fatalf("Run = %v, want the cause", err)
		}
		if got, want := r.get(), []string{"start a", "start b", "stop b", "stop a"}; !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
	t.Run("Start honours ctx", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(bounded(t))
		defer cancel(nil)
		var r recorder
		var g lifecycle.Group
		_ = g.Add(comp(&r, "a"))
		_ = g.Add(lifecycle.Component{Name: "b", Start: func(ctx context.Context) error {
			cancel(cause)
			return context.Cause(ctx)
		}, Stop: func(context.Context) error { r.add("stop b"); return nil }})
		err := g.Run(ctx)
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), "start b") {
			t.Fatalf("Run = %v, want the cause naming b", err)
		}
		if got, want := r.get(), []string{"start a", "stop a"}; !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v (b failed: not stopped)", got, want)
		}
	})
}

func TestGroup_runTwiceAndAddAfterRun(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancel(bounded(t))
	defer cancel()
	var g lifecycle.Group
	started := make(chan struct{})
	_ = g.Add(lifecycle.Component{Start: func(context.Context) error { close(started); return nil }})
	done := runAsync(ctx, &g)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("not started")
	}
	if err := g.Run(ctx); !errors.Is(err, lifecycle.ErrClosed) {
		t.Fatalf("second Run while running = %v, want ErrClosed", err)
	}
	if err := g.Add(lifecycle.Component{}); !errors.Is(err, lifecycle.ErrClosed) {
		t.Fatalf("Add while running = %v, want ErrClosed", err)
	}
	cancel()
	if err := await(t, bounded(t), done); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	cancel() // cancel after completion: nothing left to stop
	if err := g.Run(ctx); !errors.Is(err, lifecycle.ErrClosed) {
		t.Fatalf("Run after Run = %v, want ErrClosed", err)
	}
}

func TestGroup_nilCtxNilStartNilStopDefaultNames(t *testing.T) {
	leak.Verify(t)
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{})                                                      // does nothing
	_ = g.Add(lifecycle.Component{Start: func(context.Context) error { return errBoom }}) // unnamed, fails
	var nilCtx context.Context                                                            // documented as context.Background()
	err := g.Run(nilCtx)
	if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), "start component 2") {
		t.Fatalf("Run = %v, want errBoom naming component 2", err)
	}
}

func TestGroup_noComponentsWaitsForCtx(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancel(bounded(t))
	var g lifecycle.Group
	done := runAsync(ctx, &g)
	cancel()
	if err := await(t, bounded(t), done); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

// go_80_tcp_udp_sockets: the accept loop has no shutdown path and every connection goroutine outlives main.
// Here the handlers belong to a component whose Stop ends the accept loop and waits for every handler, so all
// connections are handled and leak.Verify finds no goroutine left.
func TestGroup_defectGo80_connectionGoroutinesEndedByStop(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancel(bounded(t))
	defer cancel()
	conns := make(chan int)
	quit := make(chan struct{})
	served := make(chan struct{})
	var handled atomic.Int64
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{
		Name: "server",
		Start: func(context.Context) error {
			go func() { // the accept loop
				defer close(served)
				var wg sync.WaitGroup
				defer wg.Wait()
				for {
					select {
					case <-conns:
						wg.Go(func() { handled.Add(1) })
					case <-quit:
						return
					}
				}
			}()
			return nil
		},
		Stop: func(ctx context.Context) error {
			close(quit)
			select {
			case <-served:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	_ = g.Add(lifecycle.Component{Name: "clients", Start: func(context.Context) error {
		for i := range 3 {
			conns <- i
		}
		cancel()
		return nil
	}})
	if err := await(t, bounded(t), runAsync(ctx, &g)); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if n := handled.Load(); n != 3 {
		t.Fatalf("handled = %d, want 3", n)
	}
}

// go_37_background_work: a raw signal channel, then pool.Stop() with no deadline. Here the signal arrives
// through the WithNotify hook (a test hook, never os/signal) and the Stop is bounded by the grace period.
func TestGroup_defectGo37_signalThenBoundedStop(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	fc := clock.NewFake(time.Unix(0, 0))
	sig := make(chan struct{})
	hook := func(ctx context.Context) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-sig:
				cancel()
			case <-ctx.Done():
			}
		}()
		return ctx, cancel
	}
	release := make(chan struct{})
	defer close(release)
	var g lifecycle.Group
	started := make(chan struct{})
	_ = g.Add(lifecycle.Component{
		Name:  "pool",
		Start: func(context.Context) error { close(started); return nil },
		Stop:  func(context.Context) error { <-release; return nil },
	})
	done := runAsync(tctx, &g, lifecycle.WithNotify(hook), lifecycle.WithClock(fc), lifecycle.WithGrace(time.Second))
	select {
	case <-started:
	case <-tctx.Done():
		t.Fatal("not started")
	}
	close(sig)
	if err := fc.BlockUntil(tctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := fc.AdvanceContext(tctx, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := await(t, tctx, done); !errors.Is(err, lifecycle.ErrStopTimeout) {
		t.Fatalf("Run = %v, want ErrStopTimeout (bounded, not a hang)", err)
	}
}

// go_42_faktory: ShutdownTimeout abandons in-flight jobs without saying which. Here the error names them.
func TestGroup_defectGo42_shutdownTimeoutNamesTheAbandoned(t *testing.T) {
	leak.Verify(t)
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "queue", Stop: func(context.Context) error { return nil }})
	_ = g.Add(lifecycle.Component{Name: "jobs", Stop: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})
	err := graceRun(t, &g, 25*time.Second)
	if !errors.Is(err, lifecycle.ErrStopTimeout) || !strings.Contains(err.Error(), "stop jobs still stopping after 25s; not stopped: stop queue") {
		t.Fatalf("Run = %v", err)
	}
}

// TestGroup_stopGetsFreshDeadlineNotTheCancelledCtx: GO-CQRS internal/pkg/http/echo/server/echo-server.go:42 calls
// Shutdown with the already-cancelled run ctx (it returns at once, nothing is drained); GO-Kafka-MongoDB
// server.go:195 calls it with context.Background() (no deadline at all). Here the Stop ctx is not done when Stop
// starts, keeps the run ctx's values, and is cancelled with cause ErrStopTimeout at the grace deadline.
func TestGroup_stopGetsFreshDeadlineNotTheCancelledCtx(t *testing.T) {
	leak.Verify(t)
	type key struct{}
	tctx := bounded(t)
	ctx, cancel := context.WithCancel(context.WithValue(tctx, key{}, "v"))
	defer cancel()
	fc := clock.NewFake(time.Unix(0, 0))
	const grace = 5 * time.Second
	atStart := make(chan error, 1) // what Stop saw when it began
	cause := make(chan error, 1)
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "server", Stop: func(sctx context.Context) error {
		switch {
		case sctx.Err() != nil:
			atStart <- errors.New("stop ctx already done: the cancelled run ctx was passed")
		case sctx.Value(key{}) != "v":
			atStart <- errors.New("stop ctx lost the run ctx's values")
		default:
			atStart <- nil
		}
		<-sctx.Done() // the deadline, not the signal, ends this Stop
		cause <- context.Cause(sctx)
		return sctx.Err()
	}})
	_ = g.Add(trigger(cancel)) // the run ctx is cancelled before Stop is called
	done := runAsync(ctx, &g, lifecycle.WithClock(fc), lifecycle.WithGrace(grace))
	if err := fc.BlockUntil(tctx, 1); err != nil { // the grace timer is armed when shutdown begins
		t.Fatal(err)
	}
	select {
	case err := <-atStart:
		if err != nil {
			t.Fatal(err)
		}
	case <-tctx.Done():
		t.Fatal("Stop was not called")
	}
	if err := fc.AdvanceContext(tctx, grace); err != nil {
		t.Fatal(err)
	}
	if err := await(t, tctx, done); !errors.Is(err, lifecycle.ErrStopTimeout) {
		t.Fatalf("Run = %v, want ErrStopTimeout", err)
	}
	select {
	case c := <-cause:
		if !errors.Is(c, lifecycle.ErrStopTimeout) {
			t.Fatalf("stop ctx cause = %v, want ErrStopTimeout", c)
		}
	case <-tctx.Done():
		t.Fatal("the stop ctx was not cancelled at the deadline")
	}
}

// TestGroup_stopOncePerComponentUnderConcurrentFailures: GO-CQRS internal/.../workers_runner.go:33 calls Stop
// from every worker's goroutine on ctx.Done, so Stop runs N times per component when N workers fail at once.
// Here N Runs fail at the same instant and every Stop is called exactly once, in reverse order.
func TestGroup_stopOncePerComponentUnderConcurrentFailures(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	const n = 4
	gate := make(chan struct{})       // closed: every Run fails at the same instant
	running := make(chan struct{}, n) // one send per Run that has started
	var stops [n]atomic.Int32         // Stop calls per component
	var r recorder
	var g lifecycle.Group
	for i := range n {
		name := fmt.Sprint("w", i)
		_ = g.Add(lifecycle.Component{
			Name: name,
			Run: func(context.Context) error {
				running <- struct{}{}
				<-gate
				return fmt.Errorf("%w: %s", errBoom, name)
			},
			Stop: func(context.Context) error { stops[i].Add(1); r.add("stop " + name); return nil },
		})
	}
	done := runAsync(tctx, &g)
	for range n {
		select {
		case <-running:
		case <-tctx.Done():
			t.Fatal("not every Run started")
		}
	}
	close(gate)
	err := await(t, tctx, done)
	if !errors.Is(err, errBoom) {
		t.Fatalf("Run = %v, want errBoom", err)
	}
	var want []string
	for i := n - 1; i >= 0; i-- {
		if c := stops[i].Load(); c != 1 {
			t.Fatalf("w%d stopped %d times, want exactly once", i, c)
		}
		if s := fmt.Sprintf("run w%d: boom: w%d", i, i); !strings.Contains(err.Error(), s) {
			t.Fatalf("Run = %q, want every failure reported, missing %q", err, s)
		}
		want = append(want, fmt.Sprint("stop w", i))
	}
	if got := r.get(); !slices.Equal(got, want) {
		t.Fatalf("stop order = %v, want %v", got, want)
	}
}

// TestGroup_stopOrderIsReverseOfStart: monolith-microservice cmd/monolith/main.go:46-50 closes the channel the
// handlers publish to before the handlers have stopped (a send on a closed channel panics). Here the bus is
// started first and stopped last, so a handler that publishes while stopping still finds it open.
func TestGroup_stopOrderIsReverseOfStart(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	ctx, cancel := context.WithCancel(tctx)
	defer cancel()
	events := make(chan string, 1) // the bus; its owner's Stop closes it
	var r recorder
	var g lifecycle.Group
	bus := comp(&r, "bus")
	bus.Stop = func(context.Context) error { r.add("stop bus"); close(events); return nil }
	handlers := comp(&r, "handlers")
	handlers.Stop = func(context.Context) error { events <- "flushed"; r.add("stop handlers"); return nil } // publishes while stopping
	_ = g.Add(bus)
	_ = g.Add(handlers)
	_ = g.Add(comp(&r, "server"))
	_ = g.Add(trigger(cancel))
	if err := await(t, tctx, runAsync(ctx, &g)); err != nil {
		t.Fatalf("Run = %v, want nil (a send on a closed bus would be ErrPanic)", err)
	}
	want := []string{"start bus", "start handlers", "start server", "stop server", "stop handlers", "stop bus"}
	if got := r.get(); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// TestGroup_startErrorUnblocksBeforeAnySignal: golang-todo main.go:209-213 waits for a signal even when the server
// failed to start, so the process hangs on a port in use. Here Start fails, no signal ever arrives (the notify
// context is never cancelled), and Run returns at once with the start error after stopping what had started.
func TestGroup_startErrorUnblocksBeforeAnySignal(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	var r recorder
	var g lifecycle.Group
	_ = g.Add(comp(&r, "db"))
	_ = g.Add(lifecycle.Component{Name: "server", Start: func(context.Context) error { return errBoom }}) // listen failed
	_ = g.Add(comp(&r, "metrics"))
	noSignal := func(ctx context.Context) (context.Context, context.CancelFunc) { return context.WithCancel(ctx) }
	err := await(t, tctx, runAsync(tctx, &g, lifecycle.WithNotify(noSignal)))
	if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), "start server") {
		t.Fatalf("Run = %v, want errBoom naming start server", err)
	}
	if got, want := r.get(), []string{"start db", "stop db"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
