package cache_test

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/list"

	"github.com/CrimsonCoder42/toolbench/cache"
)

// loadInFlight starts GetOrLoad(k) with a load that blocks until release is closed, and returns once the
// load is running. The result arrives on the returned channel.
func loadInFlight(t *testing.T, ctx context.Context, m *cache.Memory[string, string], k, loaded string) (release chan struct{}, res <-chan string) {
	t.Helper()
	started := make(chan struct{})
	release = make(chan struct{})
	out := make(chan string, 1)
	go func() {
		v, err := m.GetOrLoad(ctx, k, func(context.Context) (string, error) {
			close(started)
			<-release
			return loaded, nil
		})
		if err != nil {
			t.Errorf("GetOrLoad: %v", err)
		}
		out <- v
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("load never started")
	}
	return release, out
}

// Azure "update the store, then invalidate": a Delete while the old value is being loaded must win.
func TestGetOrLoad_deleteDuringLoadNotOverwritten(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	m, _ := newMem[string, string](t)
	release, res := loadInFlight(t, ctx, m, "k", "old-from-db")
	if err := m.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if v := <-res; v != "old-from-db" {
		t.Fatalf("caller got %q, want the loaded value", v)
	}
	if v, ok := mustGet(t, m, "k"); ok {
		t.Fatalf("Get after Delete during load = %q, want a miss (stale load stored)", v)
	}
}

func TestGetOrLoad_setDuringLoadNotOverwritten(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	m, _ := newMem[string, string](t)
	release, res := loadInFlight(t, ctx, m, "k", "old-from-db")
	mustSet(t, m, "k", "new-explicit", 0)
	close(release)
	if v := <-res; v != "old-from-db" {
		t.Fatalf("caller got %q, want the loaded value", v)
	}
	if v, _ := mustGet(t, m, "k"); v != "new-explicit" {
		t.Fatalf("Get = %q, want new-explicit (the load overwrote a newer Set)", v)
	}
}

// A write to another key does not stop the load from being stored.
func TestGetOrLoad_writeToOtherKeyStillStores(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	m, _ := newMem[string, string](t)
	release, res := loadInFlight(t, ctx, m, "k", "v")
	mustSet(t, m, "other", "x", 0)
	close(release)
	<-res
	if v, ok := mustGet(t, m, "k"); !ok || v != "v" {
		t.Fatalf("Get = %q, %v; want the load stored", v, ok)
	}
}

// A NaN key is refused, and the expired entries the sweep removed first are still reported to the hook.
func TestMemory_NaNKeyStillReportsSweep(t *testing.T) {
	var got []string
	m, f := newMem(t, cache.WithOnEvict(func(k float64, _ int, r cache.Reason) { got = append(got, r.String()) }))
	mustSet(t, m, 1, 1, time.Second)
	f.Advance(time.Second)
	err := m.Set(t.Context(), math.NaN(), 1, 0)
	if !errors.Is(err, list.ErrKeyNotComparable) {
		t.Fatalf("Set(NaN) = %v, want ErrKeyNotComparable", err)
	}
	if !slices.Equal(got, []string{"expired"}) || m.Len() != 0 {
		t.Fatalf("hook %v, Len %d; want [expired], 0", got, m.Len())
	}
}

// A Memory not made by New (zero value, nil pointer) refuses every call with ErrConfig instead of panicking.
func TestMemory_notFromNewIsErrConfig(t *testing.T) {
	var zero cache.Memory[string, int]
	var nilp *cache.Memory[string, int]
	for name, m := range map[string]*cache.Memory[string, int]{"zero": &zero, "nil": nilp} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := m.Get(t.Context(), "k"); !errors.Is(err, cache.ErrConfig) {
				t.Fatalf("Get = %v", err)
			}
			if err := m.Set(t.Context(), "k", 1, 0); !errors.Is(err, cache.ErrConfig) {
				t.Fatalf("Set = %v", err)
			}
			if err := m.Delete(t.Context(), "k"); !errors.Is(err, cache.ErrConfig) {
				t.Fatalf("Delete = %v", err)
			}
			if err := m.AddKnown("k"); !errors.Is(err, cache.ErrConfig) {
				t.Fatalf("AddKnown = %v", err)
			}
			if _, err := m.GetOrLoad(t.Context(), "k", func(context.Context) (int, error) {
				t.Error("load ran")
				return 0, nil
			}); !errors.Is(err, cache.ErrConfig) {
				t.Fatalf("GetOrLoad = %v", err)
			}
			if m.Len() != 0 {
				t.Fatal("Len != 0")
			}
		})
	}
}

// Aside over a typed-nil *Memory: the cache fails (ErrConfig), the store answers (ErrCache with the value).
func TestAside_typedNilMemory(t *testing.T) {
	var m *cache.Memory[string, int]
	v, err := cache.Aside[string, int](t.Context(), m, "k", 0, func(context.Context) (int, error) { return 6, nil })
	if v != 6 || !errors.Is(err, cache.ErrCache) || !errors.Is(err, cache.ErrConfig) {
		t.Fatalf("= %d, %v; want 6 with ErrCache and ErrConfig", v, err)
	}
}

func TestGetOrLoad_errNotFoundOmitsKey(t *testing.T) {
	m, _ := newMem(t, cache.WithBloom[string, int](10, 0.01, hashString))
	_, err := m.GetOrLoad(t.Context(), "secret-key-123", func(context.Context) (int, error) { return 0, nil })
	if !errors.Is(err, cache.ErrNotFound) || strings.Contains(err.Error(), "secret-key-123") {
		t.Fatalf("err = %v; want ErrNotFound without the key", err)
	}
}
