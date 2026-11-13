package singleflight_test

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/singleflight"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var (
	errA = errors.New("a failed")
	errB = errors.New("b cancelled")
)

// testCtx bounds a test that does not run in a synctest bubble.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// recv receives one value from ch or fails the test when ctx ends first.
func recv[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-ctx.Done():
		t.Fatalf("timed out waiting: %v", context.Cause(ctx))
		panic("unreachable")
	}
}

type outcome struct {
	v      int
	shared bool
	err    error
}

// N concurrent Do on one key run fn once; every caller gets the value and shared is true. synctest.Wait returns
// only when every caller is durably blocked inside Do, so "all joined before release" is a fact, not a timing.
func TestGroup_Do_suppressesDuplicates(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		const n = 8
		var g singleflight.Group[string, int]
		var runs atomic.Int64
		release := make(chan struct{})
		fn := func(context.Context) (int, error) {
			runs.Add(1)
			<-release
			return 42, nil
		}
		out := make(chan outcome, n)
		for range n {
			go func() {
				v, shared, err := g.Do(t.Context(), "k", fn)
				out <- outcome{v, shared, err}
			}()
		}
		synctest.Wait()
		if got := runs.Load(); got != 1 {
			t.Fatalf("fn started %d times while in flight, want 1", got)
		}
		close(release)
		for range n {
			o := <-out
			if o.v != 42 || !o.shared || o.err != nil {
				t.Errorf("Do = %d, %t, %v; want 42, true, nil", o.v, o.shared, o.err)
			}
		}
		if got := runs.Load(); got != 1 {
			t.Fatalf("fn ran %d times, want 1", got)
		}
	})
}

// One caller alone: shared is false; a second Do after the first has returned runs fn again.
func TestGroup_Do_sequentialCallsRunAgain(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	var g singleflight.Group[string, int]
	var runs atomic.Int64
	fn := func(context.Context) (int, error) { return int(runs.Add(1)), nil }
	for want := 1; want <= 2; want++ {
		v, shared, err := g.Do(ctx, "k", fn)
		if v != want || shared || err != nil {
			t.Fatalf("Do #%d = %d, %t, %v; want %d, false, nil", want, v, shared, err, want)
		}
	}
}

// Different keys never wait for each other: both fns are in flight at once.
func TestGroup_Do_differentKeysRunSeparately(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		var g singleflight.Group[string, string]
		var runs atomic.Int64
		release := make(chan struct{})
		out := make(chan outcome, 2)
		for _, k := range []string{"a", "b"} {
			go func() {
				v, shared, err := g.Do(t.Context(), k, func(context.Context) (string, error) {
					runs.Add(1)
					<-release
					return k, nil
				})
				if v != k {
					err = errors.Join(err, errors.New("wrong value "+v+" for key "+k))
				}
				out <- outcome{shared: shared, err: err}
			}()
		}
		synctest.Wait()
		if got := runs.Load(); got != 2 {
			t.Fatalf("%d fns in flight for two keys, want 2", got)
		}
		close(release)
		for range 2 {
			if o := <-out; o.shared || o.err != nil {
				t.Errorf("Do: shared %t, err %v; want false, nil", o.shared, o.err)
			}
		}
	})
}

// fn's error reaches every caller unchanged.
func TestGroup_Do_errorShared(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	var g singleflight.Group[string, int]
	release := make(chan struct{})
	first := g.DoChan(ctx, "k", func(context.Context) (int, error) {
		<-release
		return 0, errA
	})
	second := g.DoChan(ctx, "k", func(context.Context) (int, error) { return 1, nil })
	close(release)
	for _, ch := range []<-chan singleflight.Result[int]{first, second} {
		if r := recv(t, ctx, ch); !errors.Is(r.Err, errA) || !r.Shared {
			t.Errorf("Result = %+v, want errA, shared", r)
		}
	}
}

// A ctx done before Do: fn is not run and the cause is returned.
func TestGroup_Do_ctxDoneBefore(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancelCause(testCtx(t))
	cancel(errB)
	var g singleflight.Group[string, int]
	var runs atomic.Int64
	_, shared, err := g.Do(ctx, "k", func(context.Context) (int, error) { runs.Add(1); return 1, nil })
	if !errors.Is(err, errB) || shared || runs.Load() != 0 {
		t.Fatalf("Do = %t, %v (runs %d); want false, errB, 0 runs", shared, err, runs.Load())
	}
}

// A waiter whose ctx ends returns its cause at once while the call continues for the others, whether the waiter
// that leaves is the one that started the call or one that joined it. fn's ctx is not cancelled.
func TestGroup_Do_waiterCancelReturnsCause(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cancel int // which caller leaves: 0 started the call, 1 joined it
	}{{"starter leaves", 0}, {"joiner leaves", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			leak.Verify(t)
			synctest.Test(t, func(t *testing.T) {
				var g singleflight.Group[string, int]
				release := make(chan struct{})
				fnCtx := make(chan context.Context, 1)
				fn := func(ctx context.Context) (int, error) {
					fnCtx <- ctx
					<-release
					return 42, nil
				}
				ctxs := [2]context.Context{t.Context(), t.Context()}
				var cancel context.CancelCauseFunc
				ctxs[tc.cancel], cancel = context.WithCancelCause(t.Context())
				out := [2]chan outcome{make(chan outcome, 1), make(chan outcome, 1)}
				for i := range 2 {
					go func() {
						v, shared, err := g.Do(ctxs[i], "k", fn)
						out[i] <- outcome{v, shared, err}
					}()
					synctest.Wait() // caller 0 starts the call before caller 1 joins
				}
				cancel(errB)
				left := <-out[tc.cancel] // returns while fn is still blocked on release
				if !errors.Is(left.err, errB) || left.v != 0 || left.shared {
					t.Fatalf("leaving caller got %+v, want the cause errB", left)
				}
				fctx := <-fnCtx
				if fctx.Err() != nil {
					t.Fatalf("fn's ctx cancelled while a caller still waits: %v", context.Cause(fctx))
				}
				close(release)
				stayed := <-out[1-tc.cancel]
				if stayed.v != 42 || !stayed.shared || stayed.err != nil {
					t.Fatalf("remaining caller got %+v, want 42, shared, nil", stayed)
				}
			})
		})
	}
}

// When every caller has left, fn's ctx is cancelled with the last leaver's cause, the last caller returns only
// after fn has returned, and the next Do starts a new call.
func TestGroup_Do_allLeaveCancelsFn(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		var g singleflight.Group[string, int]
		var runs atomic.Int64
		var fnReturned atomic.Bool
		var seen atomic.Value
		fn := func(ctx context.Context) (int, error) {
			runs.Add(1)
			<-ctx.Done()
			seen.Store(context.Cause(ctx))
			fnReturned.Store(true)
			return 0, context.Cause(ctx)
		}
		ctx, cancel := context.WithCancelCause(t.Context())
		out := make(chan outcome, 1)
		go func() {
			v, shared, err := g.Do(ctx, "k", fn)
			out <- outcome{v, shared, err}
		}()
		synctest.Wait()
		cancel(errB)
		o := <-out
		if !errors.Is(o.err, errB) {
			t.Fatalf("Do = %v, want errB", o.err)
		}
		if !fnReturned.Load() {
			t.Fatal("the last caller returned before fn had returned")
		}
		if c, _ := seen.Load().(error); !errors.Is(c, errB) {
			t.Fatalf("fn's ctx cause = %v, want errB", c)
		}
		v, _, err := g.Do(t.Context(), "k", func(context.Context) (int, error) { runs.Add(1); return 7, nil })
		if v != 7 || err != nil || runs.Load() != 2 {
			t.Fatalf("Do after all left = %d, %v (runs %d); want 7, nil, 2 runs", v, err, runs.Load())
		}
	})
}

// The last caller leaves and waits for an fn that ignores its ctx; meanwhile a new caller does not join the
// cancelled call: it starts a new one.
func TestGroup_Do_newCallerAfterAllLeftStartsNewCall(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		var g singleflight.Group[string, int]
		var runs atomic.Int64
		release := make(chan struct{})
		fn := func(context.Context) (int, error) {
			n := runs.Add(1)
			<-release // ignores its ctx
			return int(n), nil
		}
		ctx, cancel := context.WithCancelCause(t.Context())
		left := make(chan error, 1)
		go func() { _, _, err := g.Do(ctx, "k", fn); left <- err }()
		synctest.Wait()
		cancel(errB)
		synctest.Wait() // the leaver is now blocked until fn returns
		fresh := make(chan outcome, 1)
		go func() {
			v, shared, err := g.Do(t.Context(), "k", fn)
			fresh <- outcome{v, shared, err}
		}()
		synctest.Wait()
		if got := runs.Load(); got != 2 {
			t.Fatalf("%d calls started, want 2 (a new caller must not join a cancelled call)", got)
		}
		close(release)
		if err := <-left; !errors.Is(err, errB) {
			t.Fatalf("leaver got %v, want errB", err)
		}
		if o := <-fresh; o.v != 2 || o.shared || o.err != nil {
			t.Fatalf("new caller got %+v, want 2, false, nil", o)
		}
	})
}

// fn runs with the first caller's values but without its deadline or cancellation.
func TestGroup_Do_fnCtxKeepsValuesNotDeadline(t *testing.T) {
	leak.Verify(t)
	type key struct{}
	parent, cancel := context.WithTimeout(context.WithValue(testCtx(t), key{}, "v"), time.Hour)
	defer cancel()
	var g singleflight.Group[string, bool]
	ok, _, err := g.Do(parent, "k", func(ctx context.Context) (bool, error) {
		_, hasDeadline := ctx.Deadline()
		return ctx.Value(key{}) == "v" && !hasDeadline && ctx.Done() != nil, nil
	})
	if !ok || err != nil {
		t.Fatalf("fn ctx: values kept, no deadline, cancellable = %t (err %v), want true", ok, err)
	}
}

// A panic in fn reaches every caller as the same *panics.PanicError; the process keeps running.
func TestGroup_Do_panicToEveryWaiter(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		const n = 4
		var g singleflight.Group[string, int]
		release := make(chan struct{})
		fn := func(context.Context) (int, error) {
			<-release
			panic("boom")
		}
		errs := make(chan error, n)
		for range n {
			go func() { _, _, err := g.Do(t.Context(), "k", fn); errs <- err }()
		}
		synctest.Wait()
		close(release)
		var first *panics.PanicError
		for range n {
			err := <-errs
			var pe *panics.PanicError
			if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "boom" {
				t.Fatalf("Do = %v, want ErrPanic with value boom", err)
			}
			if first == nil {
				first = pe
			} else if pe != first {
				t.Fatal("callers got different panic errors for one call")
			}
		}
	})
}

// A nil fn and a nil ctx do not panic the caller: nil ctx is Background, nil fn is ErrPanic.
func TestGroup_Do_nilCtxNilFn(t *testing.T) {
	leak.Verify(t)
	var g singleflight.Group[string, int]
	var nilCtx context.Context
	v, _, err := g.Do(nilCtx, "k", func(context.Context) (int, error) { return 3, nil })
	if v != 3 || err != nil {
		t.Fatalf("Do(nil ctx) = %d, %v; want 3, nil", v, err)
	}
	if _, _, err := g.Do(testCtx(t), "k", nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Do(nil fn) = %v, want ErrPanic", err)
	}
}

// runtime.Goexit in fn: every caller gets ErrGoexit; no caller's goroutine is ended.
func TestGroup_Do_goexit(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	var g singleflight.Group[string, int]
	release := make(chan struct{})
	fn := func(context.Context) (int, error) {
		<-release
		runtime.Goexit()
		return 1, nil
	}
	first := g.DoChan(ctx, "k", fn)
	second := g.DoChan(ctx, "k", fn)
	close(release)
	for _, ch := range []<-chan singleflight.Result[int]{first, second} {
		if r := recv(t, ctx, ch); !errors.Is(r.Err, singleflight.ErrGoexit) || r.Val != 0 {
			t.Errorf("Result = %+v, want ErrGoexit and a zero value", r)
		}
	}
	if v, _, err := g.Do(ctx, "k", func(context.Context) (int, error) { return 5, nil }); v != 5 || err != nil {
		t.Fatalf("Do after Goexit = %d, %v; want 5, nil (the key must be free)", v, err)
	}
}

// The zero value and new(Group) are ready; many keys and callers at once are race-free.
func TestGroup_zeroValueReady(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	g := new(singleflight.Group[int, int])
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Go(func() {
			k := i % 4
			if v, _, err := g.Do(ctx, k, func(context.Context) (int, error) { return k * 10, nil }); v != k*10 || err != nil {
				t.Errorf("Do(%d) = %d, %v", k, v, err)
			}
		})
	}
	wg.Wait()
}
