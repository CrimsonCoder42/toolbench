package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/lifecycle"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// stopper records its name.
func stopper(r *recorder, name string) lifecycle.Stopper {
	return func(context.Context) error { r.add(name); return nil }
}

// runAsyncFn runs lifecycle.Run on a goroutine.
func runAsyncFn(ctx context.Context, grace time.Duration, start func(context.Context) error, stops ...lifecycle.Stopper) <-chan error {
	done := make(chan error, 1)
	go func() { done <- lifecycle.Run(ctx, grace, start, stops...) }()
	return done
}

func TestRun_stopsInReverseAfterCtxDone(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	ctx, cancel := context.WithCancel(tctx)
	var r recorder
	started := make(chan struct{})
	start := func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		r.add("start returned")
		return ctx.Err() // a cancelled start is a clean stop
	}
	done := runAsyncFn(ctx, time.Minute, start, stopper(&r, "s1"), nil, stopper(&r, "s3"))
	select {
	case <-started:
	case <-tctx.Done():
		t.Fatal("start not called")
	}
	cancel()
	if err := await(t, tctx, done); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	got := r.get()
	// start returns concurrently with the stops; the stops themselves are in reverse.
	stops := slices.DeleteFunc(slices.Clone(got), func(s string) bool { return s == "start returned" })
	if !slices.Equal(stops, []string{"s3", "s1"}) || len(got) != 3 {
		t.Fatalf("calls = %v, want s3, s1 and start returned", got)
	}
}

func TestRun_startReturnsEarly(t *testing.T) {
	leak.Verify(t)
	for _, tc := range []struct {
		name string
		err  error
	}{{"nil", nil}, {"error", errBoom}} {
		t.Run(tc.name, func(t *testing.T) {
			var r recorder
			err := lifecycle.Run(bounded(t), time.Minute, func(context.Context) error { return tc.err },
				stopper(&r, "s1"), stopper(&r, "s2"))
			if !errors.Is(err, tc.err) || (tc.err == nil) != (err == nil) {
				t.Fatalf("Run = %v, want %v", err, tc.err)
			}
			if tc.err != nil && !strings.Contains(err.Error(), "lifecycle: start:") {
				t.Fatalf("Run = %v, want it named start", err)
			}
			if got, want := r.get(), []string{"s2", "s1"}; !slices.Equal(got, want) {
				t.Fatalf("stops = %v, want %v", got, want)
			}
		})
	}
}

func TestRun_panicsAndStopErrorsJoined(t *testing.T) {
	leak.Verify(t)
	var r recorder
	err := lifecycle.Run(bounded(t), time.Minute,
		func(context.Context) error { panic("start") },
		func(context.Context) error { r.add("s1"); return errStop },
		func(context.Context) error { r.add("s2"); panic("s2") })
	if !errors.Is(err, panics.ErrPanic) || !errors.Is(err, errStop) {
		t.Fatalf("Run = %v, want ErrPanic and errStop", err)
	}
	for _, s := range []string{"lifecycle: start:", "lifecycle: stop 1:", "lifecycle: stop 2:"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("Run = %q, want %q", err, s)
		}
	}
	if got, want := r.get(), []string{"s2", "s1"}; !slices.Equal(got, want) {
		t.Fatalf("stops = %v, want %v", got, want)
	}
}

func TestRun_startErrorAfterCtxDoneIsReported(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancel(bounded(t))
	started := make(chan struct{})
	cancelled := make(chan struct{})
	done := runAsyncFn(ctx, time.Minute, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return errBoom // not ctx's error: reported
	})
	<-started // cancel only once Run is running: a ctx done before the call is the "cancel before" case
	cancel()
	if err := await(t, bounded(t), done); !errors.Is(err, errBoom) {
		t.Fatalf("Run = %v, want errBoom", err)
	}
	<-cancelled
}

// The grace deadline of Run is on the real clock; a Stop and a start that never return until released make the
// outcome independent of how long it takes.
func TestRun_graceNamesStuckStopAndStart(t *testing.T) {
	leak.Verify(t)
	release := make(chan struct{})
	defer close(release)
	ctx, cancel := context.WithCancel(bounded(t))
	var r recorder
	started := make(chan struct{})
	done := runAsyncFn(ctx, time.Nanosecond, func(context.Context) error { close(started); <-release; return nil },
		stopper(&r, "s1"), func(context.Context) error { <-release; return nil })
	<-started
	cancel()
	err := await(t, bounded(t), done)
	if !errors.Is(err, lifecycle.ErrStopTimeout) || !strings.Contains(err.Error(), "stop 2 still stopping") ||
		!strings.Contains(err.Error(), "not stopped: stop 1, start") {
		t.Fatalf("Run = %v", err)
	}
	if got := r.get(); len(got) != 0 {
		t.Fatalf("stops = %v, want none (abandoned after grace)", got)
	}
}

func TestRun_graceStartIgnoresCtx(t *testing.T) {
	leak.Verify(t)
	release := make(chan struct{})
	defer close(release)
	ctx, cancel := context.WithCancel(bounded(t))
	started := make(chan struct{})
	done := runAsyncFn(ctx, time.Nanosecond, func(context.Context) error { close(started); <-release; return nil })
	<-started
	cancel()
	if err := await(t, bounded(t), done); !errors.Is(err, lifecycle.ErrStopTimeout) || !strings.Contains(err.Error(), "start still stopping") {
		t.Fatalf("Run = %v, want ErrStopTimeout naming start", err)
	}
}

func TestRun_cancelBeforeNilStartNilCtx(t *testing.T) {
	leak.Verify(t)
	t.Run("cancel before", func(t *testing.T) {
		cause := errors.New("before")
		ctx, cancel := context.WithCancelCause(bounded(t))
		cancel(cause)
		var r recorder
		calledStart := false
		err := lifecycle.Run(ctx, 0, func(context.Context) error { calledStart = true; return nil }, stopper(&r, "s1"))
		if !errors.Is(err, cause) || calledStart || len(r.get()) != 0 {
			t.Fatalf("Run = %v, start called %v, stops %v", err, calledStart, r.get())
		}
	})
	t.Run("nil start waits for ctx", func(t *testing.T) {
		parent, cancel := context.WithCancel(bounded(t))
		ctx := &doneWatch{Context: parent, called: make(chan struct{})}
		var r recorder
		done := runAsyncFn(ctx, -1, nil, stopper(&r, "s1"))
		<-ctx.called // Run is past its "done before the call" check and waiting
		cancel()
		if err := await(t, bounded(t), done); err != nil || !slices.Equal(r.get(), []string{"s1"}) {
			t.Fatalf("Run = %v, stops %v", err, r.get())
		}
	})
	t.Run("nil ctx", func(t *testing.T) {
		var nilCtx context.Context
		if err := lifecycle.Run(nilCtx, 0, func(context.Context) error { return errBoom }); !errors.Is(err, errBoom) {
			t.Fatalf("Run = %v, want errBoom", err)
		}
	})
}

// doneWatch closes called the first time Done is called: Run calls Done only after its "ctx done before the
// call" check, so a test that waits on called and then cancels exercises the cancel-while-running path.
type doneWatch struct {
	context.Context
	once   sync.Once
	called chan struct{}
}

func (c *doneWatch) Done() <-chan struct{} {
	c.once.Do(func() { close(c.called) })
	return c.Context.Done()
}
