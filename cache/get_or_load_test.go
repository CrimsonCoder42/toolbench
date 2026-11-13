package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/cache"
	"github.com/CrimsonCoder42/toolbench/panics"
)

var errLoad = errors.New("store down")

func TestGetOrLoad_missLoadsAndStores(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	m, f := newMem(t, cache.WithTTL[string, int](time.Minute))
	var loads atomic.Int32
	load := func(context.Context) (int, error) { loads.Add(1); return 42, nil }
	for range 3 {
		v, err := m.GetOrLoad(ctx, "k", load)
		if err != nil || v != 42 {
			t.Fatalf("GetOrLoad = %d, %v; want 42, nil", v, err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1 (then hits)", loads.Load())
	}
	f.Advance(time.Minute) // the stored entry got the default TTL
	if _, err := m.GetOrLoad(ctx, "k", load); err != nil || loads.Load() != 2 {
		t.Fatalf("after TTL: err %v, loads %d; want reload (2)", err, loads.Load())
	}
}

// N concurrent misses on one key run load once; every caller gets its value.
func TestGetOrLoad_concurrentMissesLoadOnce(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	m, _ := newMem[string, int](t)
	const n = 64
	var loads atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	load := func(context.Context) (int, error) {
		if loads.Add(1) == 1 {
			close(started)
		}
		<-release
		return 7, nil
	}
	var wg sync.WaitGroup
	results := make(chan int, n)
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			v, err := m.GetOrLoad(ctx, "hot", load)
			results <- v
			errs <- err
		})
	}
	<-started
	close(release)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for v := range results {
		if v != 7 {
			t.Fatalf("a caller got %d, want 7", v)
		}
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("loads = %d, want 1 (stampede suppressed)", got)
	}
}

// A load error is returned, not cached, and is not a miss: the next call loads again (go_45: every error was a
// miss, and the failure was stored or panicked).
func TestGetOrLoad_errorNotCachedAndDistinctFromMiss(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	m, _ := newMem[string, int](t)
	var loads atomic.Int32
	_, err := m.GetOrLoad(ctx, "k", func(context.Context) (int, error) { loads.Add(1); return 99, errLoad })
	if !errors.Is(err, errLoad) {
		t.Fatalf("err = %v, want errLoad", err)
	}
	if _, ok := mustGet(t, m, "k"); ok || m.Len() != 0 {
		t.Fatal("a failed load was cached")
	}
	v, err := m.GetOrLoad(ctx, "k", func(context.Context) (int, error) { loads.Add(1); return 5, nil })
	if err != nil || v != 5 || loads.Load() != 2 {
		t.Fatalf("retry = %d, %v, loads %d; want 5, nil, 2", v, err, loads.Load())
	}
}

func TestGetOrLoad_errorValueIsZero(t *testing.T) {
	leakCheck(t)
	m, _ := newMem[string, int](t)
	v, err := m.GetOrLoad(t.Context(), "k", func(context.Context) (int, error) { return 99, errLoad })
	if v != 0 || err == nil {
		t.Fatalf("= %d, %v; want 0 and the error", v, err)
	}
}

func TestGetOrLoad_panicIsErrPanic(t *testing.T) {
	leakCheck(t)
	m, _ := newMem[string, int](t)
	_, err := m.GetOrLoad(t.Context(), "k", func(context.Context) (int, error) { panic("load blew up") })
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "load blew up" {
		t.Fatalf("err = %v, want *panics.PanicError(load blew up)", err)
	}
	if m.Len() != 0 {
		t.Fatal("a panicked load stored a value")
	}
}

func TestGetOrLoad_nilLoadIsErrPanic(t *testing.T) {
	leakCheck(t)
	m, _ := newMem[string, int](t)
	if _, err := m.GetOrLoad(t.Context(), "k", nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("nil load: %v, want ErrPanic", err)
	}
}

func TestGetOrLoad_hitSkipsLoad(t *testing.T) {
	m, _ := newMem[string, int](t)
	mustSet(t, m, "k", 1, 0)
	v, err := m.GetOrLoad(t.Context(), "k", func(context.Context) (int, error) {
		t.Error("load called on a hit")
		return 0, nil
	})
	if err != nil || v != 1 {
		t.Fatalf("= %d, %v", v, err)
	}
}

func TestGetOrLoad_cancelBeforeStart(t *testing.T) {
	leakCheck(t)
	m, _ := newMem[string, int](t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("gone")
	cancel(cause)
	_, err := m.GetOrLoad(ctx, "k", func(context.Context) (int, error) {
		t.Error("load ran after cancel")
		return 0, nil
	})
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want the cause", err)
	}
}

// A cancelled caller returns its cause; the load's ctx is cancelled (the only waiter left), and nothing is
// stored when the load gives up.
func TestGetOrLoad_cancelDuringLoad(t *testing.T) {
	leakCheck(t)
	m, _ := newMem[string, int](t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("client left")
	started := make(chan struct{})
	loadDone := make(chan struct{})
	go func() { <-started; cancel(cause) }()
	_, err := m.GetOrLoad(ctx, "k", func(lctx context.Context) (int, error) {
		defer close(loadDone)
		close(started)
		<-lctx.Done()
		return 0, context.Cause(lctx)
	})
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want the cause", err)
	}
	select {
	case <-loadDone: // the last caller waited for the load to return
	default:
		t.Fatal("GetOrLoad returned before the load did")
	}
	if m.Len() != 0 {
		t.Fatal("a cancelled load stored a value")
	}
}

func TestGetOrLoad_cancelAfterCompletion(t *testing.T) {
	leakCheck(t)
	m, _ := newMem[string, int](t)
	ctx, cancel := context.WithCancel(t.Context())
	if _, err := m.GetOrLoad(ctx, "k", func(context.Context) (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	cancel()
	if v, ok := mustGet(t, m, "k"); !ok || v != 1 {
		t.Fatal("value lost after the caller's ctx ended")
	}
}

func hashString(s string) []byte { return []byte(s) }

// Bloom guard: a key never declared is a definite miss and skips the load (ErrNotFound); a known key loads.
func TestGetOrLoad_bloomDefiniteMissSkipsLoad(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	m, _ := newMem(t, cache.WithBloom[string, int](1000, 0.0001, hashString))
	if err := m.AddKnown("user:1", "user:2"); err != nil {
		t.Fatal(err)
	}
	var loads atomic.Int32
	load := func(context.Context) (int, error) { loads.Add(1); return 1, nil }
	if _, err := m.GetOrLoad(ctx, "user:999", load); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("absent key: %v, want ErrNotFound", err)
	}
	if loads.Load() != 0 {
		t.Fatal("load ran for a definite miss")
	}
	if v, err := m.GetOrLoad(ctx, "user:1", load); err != nil || v != 1 || loads.Load() != 1 {
		t.Fatalf("known key: %d, %v, loads %d", v, err, loads.Load())
	}
}

func TestGetOrLoad_bloomSetAddsKey(t *testing.T) {
	m, _ := newMem(t, cache.WithBloom[string, int](100, 0.001, hashString))
	mustSet(t, m, "k", 1, 0)
	if err := m.Delete(t.Context(), "k"); err != nil {
		t.Fatal(err)
	}
	// Deleted from the cache, still "maybe" in the filter: the load runs.
	v, err := m.GetOrLoad(t.Context(), "k", func(context.Context) (int, error) { return 2, nil })
	if err != nil || v != 2 {
		t.Fatalf("= %d, %v; want the load to run", v, err)
	}
}

func TestGetOrLoad_bloomIgnoredByGet(t *testing.T) {
	m, _ := newMem(t, cache.WithBloom[string, int](100, 0.001, hashString))
	if _, ok, err := m.Get(t.Context(), "never"); ok || err != nil {
		t.Fatalf("Get of an unknown key = %v, %v; want a plain miss", ok, err)
	}
}

func TestGetOrLoad_bloomHashPanicIsErrPanic(t *testing.T) {
	m, _ := newMem(t, cache.WithBloom[string, int](100, 0.01, func(string) []byte { panic("hash") }))
	if err := m.AddKnown("a"); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("AddKnown = %v, want ErrPanic", err)
	}
	if _, err := m.GetOrLoad(t.Context(), "a", func(context.Context) (int, error) { return 0, nil }); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("GetOrLoad = %v, want ErrPanic", err)
	}
}

func TestAddKnown_withoutBloomIsNoop(t *testing.T) {
	m, _ := newMem[string, int](t)
	if err := m.AddKnown("a"); err != nil {
		t.Fatal(err)
	}
}
