package cache_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/cache"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// failing is a Cache whose Get and/or Set fail, standing in for an unreachable network cache.
type failing struct {
	getErr, setErr error
	sets           atomic.Int32
}

func (f *failing) Get(context.Context, string) (int, bool, error) { return 0, false, f.getErr }
func (f *failing) Set(context.Context, string, int, time.Duration) error {
	f.sets.Add(1)
	return f.setErr
}
func (f *failing) Delete(context.Context, string) error { return nil }

var _ cache.Cache[string, int] = (*cache.Memory[string, int])(nil)

func TestAside_overMemory(t *testing.T) {
	m, f := newMem[string, int](t)
	var loads atomic.Int32
	load := func(context.Context) (int, error) { loads.Add(1); return 3, nil }
	for range 2 {
		v, err := cache.Aside[string, int](t.Context(), m, "k", time.Second, load)
		if err != nil || v != 3 {
			t.Fatalf("Aside = %d, %v", v, err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
	f.Advance(time.Second) // the ttl given to Aside was used
	if _, err := cache.Aside[string, int](t.Context(), m, "k", time.Second, load); err != nil || loads.Load() != 2 {
		t.Fatalf("after ttl: %v, loads %d; want 2", err, loads.Load())
	}
}

func TestAside_loadErrorNotStored(t *testing.T) {
	c := &failing{}
	_, err := cache.Aside[string, int](t.Context(), c, "k", 0, func(context.Context) (int, error) { return 1, errLoad })
	if !errors.Is(err, errLoad) || c.sets.Load() != 0 {
		t.Fatalf("err %v, sets %d; want errLoad and no Set", err, c.sets.Load())
	}
}

func TestAside_getErrorFallsBackToLoad(t *testing.T) {
	getErr := errors.New("redis: connection refused")
	c := &failing{getErr: getErr}
	v, err := cache.Aside[string, int](t.Context(), c, "k", 0, func(context.Context) (int, error) { return 8, nil })
	if v != 8 || !errors.Is(err, cache.ErrCache) || !errors.Is(err, getErr) {
		t.Fatalf("= %d, %v; want 8 with ErrCache wrapping the get error", v, err)
	}
	if c.sets.Load() != 0 {
		t.Fatal("Set tried on a cache whose Get failed")
	}
}

// go_45 panicked when the cache Set failed; Aside returns the loaded value and ErrCache.
func TestAside_setErrorReportedWithValue(t *testing.T) {
	setErr := errors.New("memcache: server error")
	c := &failing{setErr: setErr}
	v, err := cache.Aside[string, int](t.Context(), c, "k", 0, func(context.Context) (int, error) { return 8, nil })
	if v != 8 || !errors.Is(err, cache.ErrCache) || !errors.Is(err, setErr) {
		t.Fatalf("= %d, %v; want 8 with ErrCache wrapping the set error", v, err)
	}
}

func TestAside_panicIsErrPanic(t *testing.T) {
	_, err := cache.Aside[string, int](t.Context(), &failing{}, "k", 0, func(context.Context) (int, error) { panic(1) })
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("err = %v, want ErrPanic", err)
	}
	if _, err := cache.Aside[string, int](t.Context(), &failing{}, "k", 0, nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("nil load: %v, want ErrPanic", err)
	}
}

func TestAside_cancelled(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("bye")
	cancel(cause)
	_, err := cache.Aside[string, int](ctx, &failing{}, "k", 0, func(context.Context) (int, error) {
		t.Error("load ran")
		return 0, nil
	})
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want cause", err)
	}
}

func TestAside_nilCacheAndCtx(t *testing.T) {
	//lint:ignore SA1012 a nil ctx is documented as allowed
	v, err := cache.Aside[string, int](nil, nil, "k", 0, func(context.Context) (int, error) { return 4, nil }) //nolint:staticcheck
	if err != nil || v != 4 {
		t.Fatalf("= %d, %v", v, err)
	}
}

func TestAside_negativeTTLIsCacheError(t *testing.T) {
	m, _ := newMem[string, int](t)
	v, err := cache.Aside[string, int](t.Context(), m, "k", -1, func(context.Context) (int, error) { return 4, nil })
	if v != 4 || !errors.Is(err, cache.ErrTTL) || !errors.Is(err, cache.ErrCache) {
		t.Fatalf("= %d, %v; want 4 with ErrCache and ErrTTL", v, err)
	}
}
