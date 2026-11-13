package singleflight_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/singleflight"
)

// DoChan joins before it returns: a second DoChan on the key is a duplicate, fn runs once, both Results are
// shared, and each channel is closed after its one Result.
func TestGroup_DoChan_joinsBeforeReturning(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	var g singleflight.Group[string, int]
	var runs atomic.Int64
	release := make(chan struct{})
	fn := func(context.Context) (int, error) {
		runs.Add(1)
		<-release
		return 9, nil
	}
	chans := []<-chan singleflight.Result[int]{g.DoChan(ctx, "k", fn), g.DoChan(ctx, "k", fn)}
	close(release)
	for _, ch := range chans {
		if r := recv(t, ctx, ch); r.Val != 9 || !r.Shared || r.Err != nil {
			t.Errorf("Result = %+v, want 9, shared, nil", r)
		}
		if _, open := <-ch; open {
			t.Error("channel not closed after its Result")
		}
	}
	if runs.Load() != 1 {
		t.Fatalf("fn ran %d times, want 1", runs.Load())
	}
}

// Do and DoChan share calls: a Do on a key in flight from DoChan waits for it.
func TestGroup_DoChan_sharedWithDo(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		var g singleflight.Group[string, int]
		release := make(chan struct{})
		ch := g.DoChan(t.Context(), "k", func(context.Context) (int, error) { <-release; return 1, nil })
		out := make(chan outcome, 1)
		go func() {
			v, shared, err := g.Do(t.Context(), "k", func(context.Context) (int, error) { return 2, nil })
			out <- outcome{v, shared, err}
		}()
		synctest.Wait()
		close(release)
		if o := <-out; o.v != 1 || !o.shared || o.err != nil {
			t.Fatalf("Do = %+v, want the DoChan call's 1, shared", o)
		}
		if r := <-ch; r.Val != 1 || !r.Shared {
			t.Fatalf("DoChan = %+v, want 1, shared", r)
		}
	})
}

// A ctx done before DoChan: the channel already holds the cause and is closed; fn is not run.
func TestGroup_DoChan_ctxDoneBefore(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancelCause(testCtx(t))
	cancel(errB)
	var g singleflight.Group[string, int]
	var runs atomic.Int64
	ch := g.DoChan(ctx, "k", func(context.Context) (int, error) { runs.Add(1); return 1, nil })
	if r := <-ch; !errors.Is(r.Err, errB) || r.Shared {
		t.Fatalf("Result = %+v, want errB", r)
	}
	if _, open := <-ch; open || runs.Load() != 0 {
		t.Fatalf("channel open %t, runs %d; want closed, 0", open, runs.Load())
	}
}

// A DoChan caller whose ctx ends mid-call gets its cause; the channel is closed.
func TestGroup_DoChan_cancelDuringCall(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		var g singleflight.Group[string, int]
		ctx, cancel := context.WithCancelCause(t.Context())
		ch := g.DoChan(ctx, "k", func(ctx context.Context) (int, error) {
			<-ctx.Done()
			return 0, context.Cause(ctx)
		})
		synctest.Wait()
		cancel(errB)
		if r := <-ch; !errors.Is(r.Err, errB) {
			t.Fatalf("Result = %+v, want errB", r)
		}
		if _, open := <-ch; open {
			t.Fatal("channel not closed")
		}
	})
}

// Forget during a call: the next DoChan starts a new call; the old call still delivers to its callers.
func TestGroup_Forget_duringCallStartsNew(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	var g singleflight.Group[string, int]
	var runs atomic.Int64
	release := make(chan struct{})
	started := make(chan struct{})
	old := g.DoChan(ctx, "k", func(context.Context) (int, error) {
		runs.Add(1)
		close(started)
		<-release
		return 1, nil
	})
	recv(t, ctx, started)
	g.Forget("k")
	fresh := g.DoChan(ctx, "k", func(context.Context) (int, error) { runs.Add(1); return 2, nil })
	if r := recv(t, ctx, fresh); r.Val != 2 || r.Shared || r.Err != nil {
		t.Fatalf("new call = %+v, want 2, not shared", r)
	}
	close(release)
	if r := recv(t, ctx, old); r.Val != 1 || r.Shared || r.Err != nil {
		t.Fatalf("old call = %+v, want 1, not shared", r)
	}
	if runs.Load() != 2 {
		t.Fatalf("fn ran %d times, want 2", runs.Load())
	}
	g.Forget("absent") // forgetting a key with no call is a no-op
}

// A panic in fn through DoChan is ErrPanic in the Result, never a crash (x/sync re-panics on a new goroutine).
func TestGroup_DoChan_panic(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	var g singleflight.Group[string, int]
	r := recv(t, ctx, g.DoChan(ctx, "k", func(context.Context) (int, error) { panic(errA) }))
	if !errors.Is(r.Err, panics.ErrPanic) || !errors.Is(r.Err, errA) {
		t.Fatalf("Result = %+v, want ErrPanic wrapping errA", r)
	}
}

func BenchmarkGroup_Do(b *testing.B) {
	var g singleflight.Group[int, int]
	ctx := context.Background()
	fn := func(context.Context) (int, error) { return 1, nil }
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := g.Do(ctx, 1, fn); err != nil {
			b.Fatal(err)
		}
	}
}
