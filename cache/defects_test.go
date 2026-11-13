package cache_test

// The defects of the code-heim repos that show this pattern (LADDER.md §7), as named tests.

import (
	"context"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/cache"
)

// go_11: on a hit the handler wrote the response and then fell through to compute and write it again. A hit
// returns once: load is not called and nothing is stored again (no Replaced in the hook).
func TestDefect_go11_hitWritesTwice(t *testing.T) {
	log := newEvictLog()
	m, _ := newMem(t, cache.WithOnEvict(log.hook))
	var loads atomic.Int32
	load := func(context.Context) (int, error) { loads.Add(1); return 1, nil }
	for range 5 {
		if _, err := m.GetOrLoad(t.Context(), "fib:10", load); err != nil {
			t.Fatal(err)
		}
	}
	if loads.Load() != 1 || len(log.list()) != 0 {
		t.Fatalf("loads %d, hook %v; want 1 load and no rewrite", loads.Load(), log.list())
	}
}

// go_11: one goroutine per entry sleeping for the TTL. Storing many entries with a TTL starts no goroutine,
// and expiry needs none (the fake clock has no timers armed by the cache).
func TestDefect_go11_goroutinePerEntry(t *testing.T) {
	leakCheck(t)
	m, f := newMem(t, cache.WithTTL[int, int](5*time.Minute), cache.WithCapacity[int, int](2000))
	before := runtime.NumGoroutine()
	for i := range 1000 {
		mustSet(t, m, i, i, 0)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines %d → %d after 1000 Sets with a TTL", before, after)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 0)
	defer cancel()
	if err := f.BlockUntil(ctx, 1); err == nil {
		t.Fatal("the cache armed a timer")
	}
	f.Advance(5 * time.Minute)
	if _, ok := mustGet(t, m, 0); ok {
		t.Fatal("entry not expired")
	}
}

// go_11: a global sync.Map that grows without bound. Memory holds at most its capacity.
func TestDefect_go11_unbounded(t *testing.T) {
	m, _ := newMem(t, cache.WithCapacity[string, int](100))
	for i := range 10_000 {
		mustSet(t, m, strconv.Itoa(i), i, 0)
	}
	if m.Len() != 100 {
		t.Fatalf("Len = %d, want the bound 100", m.Len())
	}
	d, err := cache.New[int, int]()
	if err != nil {
		t.Fatal(err)
	}
	for i := range cache.DefaultCapacity + 10 {
		_ = d.Set(t.Context(), i, i, 0)
	}
	if d.Len() != cache.DefaultCapacity {
		t.Fatalf("default Len = %d, want DefaultCapacity", d.Len())
	}
}

// go_45: every cache error was treated as a miss. Get reports a miss as ok == false with a nil error; Aside
// tells a cache failure (ErrCache) from a miss (tested in aside_test.go).
func TestDefect_go45_errorIsNotAMiss(t *testing.T) {
	m, _ := newMem[string, int](t)
	v, ok, err := m.Get(t.Context(), "absent")
	if ok || err != nil || v != 0 {
		t.Fatalf("miss = %d, %v, %v; want 0, false, nil", v, ok, err)
	}
}

// go_45: no stampede guard: N concurrent misses ran the 2-second load N times. GetOrLoad runs it once.
func TestDefect_go45_noStampedeGuard(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	m, _ := newMem[string, []byte](t)
	var loads atomic.Int32
	gate := make(chan struct{})
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			_, _ = m.GetOrLoad(ctx, "blog:1", func(context.Context) ([]byte, error) {
				loads.Add(1)
				<-gate
				return []byte("{}"), nil
			})
		})
	}
	close(gate)
	wg.Wait()
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
}

// go_45: reflection and a panic on a load that does not return []byte. Typed generics need neither; a load
// of any V works.
func TestDefect_go45_reflection(t *testing.T) {
	type blog struct{ ID int }
	m, _ := newMem[int, blog](t)
	v, err := m.GetOrLoad(t.Context(), 1, func(context.Context) (blog, error) { return blog{ID: 1}, nil })
	if err != nil || v.ID != 1 {
		t.Fatalf("= %+v, %v", v, err)
	}
}

// go_69: a global filter filled from init and read by handlers with no lock. Each Memory owns its filter
// under its mutex: concurrent AddKnown and GetOrLoad are race-free (-race), and two caches do not share one.
func TestDefect_go69_globalFilterNoLock(t *testing.T) {
	leakCheck(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	a, _ := newMem(t, cache.WithBloom[string, int](1000, 0.0001, hashString))
	b, _ := newMem(t, cache.WithBloom[string, int](1000, 0.0001, hashString))
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Go(func() {
			for i := range 100 {
				k := strconv.Itoa(w*100 + i)
				_ = a.AddKnown(k)
				_, _ = a.GetOrLoad(ctx, k, func(context.Context) (int, error) { return i, nil })
			}
		})
	}
	wg.Wait()
	if _, err := b.GetOrLoad(ctx, "1", func(context.Context) (int, error) { return 0, nil }); err == nil {
		t.Fatal("b's filter saw a's keys")
	}
}
