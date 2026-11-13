package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/lifecycle"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// TestGroup_runFailureBeginsShutdown: GO-Kafka-MongoDB server.go:156-162 runs gRPC, consumers, metrics and HTTP
// each on a goroutine and any one failing cancels the rest; the one reference that has the run-time seam. Here
// a Run that fails after its Start returned ends the group: every started component is stopped in reverse,
// within the grace period (the fake clock never moves), and the failure is reported under the component's name.
func TestGroup_runFailureBeginsShutdown(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	fc := clock.NewFake(time.Unix(0, 0)) // never advanced: shutdown must finish without the deadline
	var r recorder
	var g lifecycle.Group
	_ = g.Add(comp(&r, "db"))
	grpc := comp(&r, "grpc")
	grpc.Run = func(context.Context) error { return errBoom } // Serve failed after Start
	_ = g.Add(grpc)
	web := comp(&r, "http")
	web.Run = func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() } // honours the run ctx
	_ = g.Add(web)
	err := await(t, tctx, runAsync(tctx, &g, lifecycle.WithClock(fc)))
	if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), "run grpc") {
		t.Fatalf("Run = %v, want errBoom naming run grpc", err)
	}
	if strings.Contains(err.Error(), "run http") {
		t.Fatalf("Run = %v: http's own cancellation error must not be reported", err)
	}
	want := []string{"start db", "start grpc", "start http", "stop http", "stop grpc", "stop db"}
	if got := r.get(); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// TestGroup_concurrentRunFailuresJoinedNoLeak: GO-CQRS internal/.../workers_runner.go:16 makes an unbuffered
// errChan and :30 sends on it from every worker, so the second worker to fail blocks forever. Here two Runs fail
// at the same instant: both errors are in Run's result and no goroutine is left behind.
func TestGroup_concurrentRunFailuresJoinedNoLeak(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	gate := make(chan struct{})
	running := make(chan struct{}, 2)
	errA, errB := errors.New("a failed"), errors.New("b failed")
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "a", Run: func(context.Context) error { running <- struct{}{}; <-gate; return errA }})
	_ = g.Add(lifecycle.Component{Name: "b", Run: func(context.Context) error { running <- struct{}{}; <-gate; return errB }})
	done := runAsync(tctx, &g)
	for range 2 {
		select {
		case <-running:
		case <-tctx.Done():
			t.Fatal("not every Run started")
		}
	}
	close(gate)
	err := await(t, tctx, done)
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("Run = %v, want both errA and errB", err)
	}
	for _, s := range []string{"run a: a failed", "run b: b failed"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("Run = %q, want it to contain %q", err, s)
		}
	}
}

func TestGroup_runReturningNilEndsGroup(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	var r recorder
	var g lifecycle.Group
	_ = g.Add(comp(&r, "db"))
	job := comp(&r, "job")
	job.Run = func(context.Context) error { r.add("run job"); return nil } // a one-shot job: done is done
	_ = g.Add(job)
	if err := await(t, tctx, runAsync(tctx, &g)); err != nil { // the ctx is never cancelled
		t.Fatalf("Run = %v, want nil", err)
	}
	want := []string{"start db", "start job", "run job", "stop job", "stop db"}
	if got := r.get(); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestGroup_runPanicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	var r recorder
	var g lifecycle.Group
	_ = g.Add(comp(&r, "db"))
	w := comp(&r, "worker")
	w.Run = func(context.Context) error { panic("worker") }
	_ = g.Add(w)
	err := await(t, tctx, runAsync(tctx, &g))
	if !errors.Is(err, panics.ErrPanic) || !strings.Contains(err.Error(), "run worker") {
		t.Fatalf("Run = %v, want ErrPanic naming run worker", err)
	}
	if got, want := r.get(), []string{"start db", "start worker", "stop worker", "stop db"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v (a panicking Run is still stopped)", got, want)
	}
}

func TestGroup_runIgnoringCtxIsAbandonedAndNamed(t *testing.T) {
	leak.Verify(t)
	release := make(chan struct{})
	defer close(release) // the abandoned Run returns after Run did; leak.Verify waits for it
	var g lifecycle.Group
	// Nothing has a Stop, so the Run wait is the only shutdown step and the deadline names it.
	_ = g.Add(lifecycle.Component{Name: "db"})
	// stuck ignores the run ctx (and has no Stop to end it).
	_ = g.Add(lifecycle.Component{Name: "stuck", Run: func(context.Context) error { <-release; return nil }})
	// graceRun cancels once every component has started, then fires the grace timer.
	err := graceRun(t, &g, time.Second)
	if !errors.Is(err, lifecycle.ErrStopTimeout) || !strings.Contains(err.Error(), "run stuck still running after 1s") {
		t.Fatalf("Run = %v, want ErrStopTimeout naming run stuck", err)
	}
	if strings.Contains(err.Error(), "not stopped") {
		t.Fatalf("Run = %v: nothing else was outstanding", err)
	}
}

func TestGroup_runCtxCancelledOnShutdownWithCause(t *testing.T) {
	errSignal := errors.New("SIGTERM")
	t.Run("a Run ended the group", func(t *testing.T) {
		leak.Verify(t)
		tctx := bounded(t)
		cause := make(chan error, 1)
		var g lifecycle.Group
		_ = g.Add(lifecycle.Component{Name: "consumer", Run: func(ctx context.Context) error {
			<-ctx.Done()
			cause <- context.Cause(ctx)
			return ctx.Err()
		}})
		_ = g.Add(lifecycle.Component{Name: "failer", Run: func(context.Context) error { return errBoom }})
		err := await(t, tctx, runAsync(tctx, &g))
		if !errors.Is(err, errBoom) || strings.Contains(err.Error(), "run consumer") {
			t.Fatalf("Run = %v, want errBoom from failer only", err)
		}
		select {
		case c := <-cause:
			if !errors.Is(c, errBoom) || !strings.Contains(c.Error(), "run failer ended the group") {
				t.Fatalf("run ctx cause = %v, want it to name failer and wrap errBoom", c)
			}
		case <-tctx.Done():
			t.Fatal("the run ctx was not cancelled")
		}
	})
	t.Run("ctx ended the group", func(t *testing.T) {
		leak.Verify(t)
		tctx := bounded(t)
		ctx, cancel := context.WithCancelCause(tctx)
		defer cancel(nil)
		cause := make(chan error, 1)
		var g lifecycle.Group
		_ = g.Add(lifecycle.Component{Name: "consumer", Run: func(ctx context.Context) error {
			<-ctx.Done()
			cause <- context.Cause(ctx)
			return context.Cause(ctx) // its own cancellation, by cause: not reported
		}})
		_ = g.Add(lifecycle.Component{Name: "signal", Run: func(ctx context.Context) error {
			cancel(errSignal) // a signal arrives while the consumer runs
			<-ctx.Done()      // the run ctx, a child of the signalled one, is done at once
			return nil
		}})
		if err := await(t, tctx, runAsync(ctx, &g)); err != nil {
			t.Fatalf("Run = %v, want nil (a signal is a clean stop)", err)
		}
		select {
		case c := <-cause:
			if !errors.Is(c, errSignal) {
				t.Fatalf("run ctx cause = %v, want errSignal", c)
			}
		case <-tctx.Done():
			t.Fatal("the run ctx was not cancelled")
		}
	})
}

func TestGroup_runCancelledBeforeRunsStart(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	ctx, cancel := context.WithCancel(tctx)
	defer cancel()
	ran := make(chan error, 1) // what the Run saw of its ctx when it was called
	var r recorder
	var g lifecycle.Group
	c := comp(&r, "consumer")
	c.Run = func(ctx context.Context) error { ran <- ctx.Err(); <-ctx.Done(); return ctx.Err() }
	_ = g.Add(c)
	_ = g.Add(trigger(cancel)) // ctx is done before any Run is called: each sees a cancelled ctx at once
	if err := await(t, tctx, runAsync(ctx, &g)); err != nil {
		t.Fatalf("Run = %v, want nil (the Run's own cancellation error is not reported)", err)
	}
	select {
	case err := <-ran: // Run waited for the Run goroutine, so this was sent
		if err == nil {
			t.Fatal("the Run was called with a live ctx, want one already done")
		}
	default:
		t.Fatal("the Run was not called")
	}
	if got, want := r.get(), []string{"start consumer", "stop consumer"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
