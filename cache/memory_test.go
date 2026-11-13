package cache_test

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/list"

	"github.com/CrimsonCoder42/toolbench/cache"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestMemory_GetSetDelete(t *testing.T) {
	m, _ := newMem[string, int](t)
	if _, ok := mustGet(t, m, "a"); ok {
		t.Fatal("empty cache: hit")
	}
	mustSet(t, m, "a", 1, 0)
	if v, ok := mustGet(t, m, "a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %d, %v; want 1, true", v, ok)
	}
	mustSet(t, m, "a", 2, 0)
	if v, _ := mustGet(t, m, "a"); v != 2 {
		t.Fatalf("after replace Get(a) = %d, want 2", v)
	}
	if err := m.Delete(t.Context(), "a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := mustGet(t, m, "a"); ok {
		t.Fatal("after Delete: hit")
	}
	if err := m.Delete(t.Context(), "absent"); err != nil {
		t.Fatalf("Delete(absent) = %v, want nil", err)
	}
	if m.Len() != 0 {
		t.Fatalf("Len = %d, want 0", m.Len())
	}
}

func TestMemory_nilCtx(t *testing.T) {
	m, _ := newMem[string, int](t)
	//lint:ignore SA1012 a nil ctx is documented as allowed
	if err := m.Set(nil, "a", 1, 0); err != nil { //nolint:staticcheck
		t.Fatal(err)
	}
	if v, ok, err := m.Get(nil, "a"); err != nil || !ok || v != 1 { //nolint:staticcheck
		t.Fatalf("Get(nil ctx) = %d, %v, %v", v, ok, err)
	}
}

func TestMemory_LRUEvictionOrderAtCapacity(t *testing.T) {
	log := newEvictLog()
	m, _ := newMem(t, cache.WithCapacity[string, int](3), cache.WithOnEvict(log.hook))
	mustSet(t, m, "a", 1, 0)
	mustSet(t, m, "b", 2, 0)
	mustSet(t, m, "c", 3, 0)
	mustGet(t, m, "a")       // a becomes most recently used: order (old→new) b, c, a
	mustSet(t, m, "d", 4, 0) // evicts b
	mustSet(t, m, "e", 5, 0) // evicts c
	if got, want := log.list(), []string{"b:evicted", "c:evicted"}; !slices.Equal(got, want) {
		t.Fatalf("evictions = %v, want %v", got, want)
	}
	for _, k := range []string{"a", "d", "e"} {
		if _, ok := mustGet(t, m, k); !ok {
			t.Fatalf("%s evicted, want kept", k)
		}
	}
	if m.Len() != 3 {
		t.Fatalf("Len = %d, want capacity 3", m.Len())
	}
}

func TestMemory_TTLExpiryOnFakeClock(t *testing.T) {
	log := newEvictLog()
	m, f := newMem(t, cache.WithTTL[string, int](time.Minute), cache.WithOnEvict(log.hook))
	mustSet(t, m, "default", 1, 0)            // default TTL: 1m
	mustSet(t, m, "short", 2, 10*time.Second) // explicit TTL
	f.Advance(10*time.Second - time.Nanosecond)
	if _, ok := mustGet(t, m, "short"); !ok {
		t.Fatal("short expired early")
	}
	f.Advance(time.Nanosecond) // exactly at the expiry instant: expired
	if _, ok := mustGet(t, m, "short"); ok {
		t.Fatal("short not expired at its TTL")
	}
	if _, ok := mustGet(t, m, "default"); !ok {
		t.Fatal("default expired early")
	}
	f.Advance(50 * time.Second)
	if _, ok := mustGet(t, m, "default"); ok {
		t.Fatal("default not expired at the cache TTL")
	}
	if got, want := log.list(), []string{"short:expired", "default:expired"}; !slices.Equal(got, want) {
		t.Fatalf("hook = %v, want %v", got, want)
	}
}

func TestMemory_noTTLNeverExpires(t *testing.T) {
	m, f := newMem[string, int](t)
	mustSet(t, m, "a", 1, 0)
	f.Advance(1000 * time.Hour)
	if _, ok := mustGet(t, m, "a"); !ok {
		t.Fatal("entry with no TTL expired")
	}
}

func TestMemory_lazyExpiryKeepsEntryUntilTouched(t *testing.T) {
	m, f := newMem[string, int](t)
	mustSet(t, m, "a", 1, time.Second)
	f.Advance(time.Hour)
	if m.Len() != 1 {
		t.Fatalf("Len = %d before any access, want 1 (lazy)", m.Len())
	}
	mustGet(t, m, "a")
	if m.Len() != 0 {
		t.Fatalf("Len = %d after Get met the expired entry, want 0", m.Len())
	}
}

// The sweep on a write removes expired entries from the least recently used end (ALG-LIST-018 Oldest), at
// most 16, and stops at the first one that has not expired.
func TestMemory_boundedSweepViaOldest(t *testing.T) {
	log := newEvictLog()
	m, f := newMem(t, cache.WithCapacity[string, int](100), cache.WithOnEvict(log.hook))
	for i := range 20 {
		mustSet(t, m, string(rune('A'+i)), i, time.Second) // A..T, oldest first
	}
	mustSet(t, m, "keep", 0, 0) // newest, no TTL
	f.Advance(time.Second)
	mustSet(t, m, "x", 1, 0) // a new key: sweeps 16 of the 20 expired
	if got := len(log.list()); got != 16 {
		t.Fatalf("swept %d, want 16 (the bound)", got)
	}
	if m.Len() != 22-16 {
		t.Fatalf("Len = %d, want %d", m.Len(), 22-16)
	}
	mustSet(t, m, "y", 2, 0) // sweeps the remaining 4, stops at "keep"
	if got := len(log.list()); got != 20 {
		t.Fatalf("swept %d in all, want 20", got)
	}
	for _, k := range []string{"keep", "x", "y"} {
		if _, ok := mustGet(t, m, k); !ok {
			t.Fatalf("%s swept, want kept", k)
		}
	}
	for _, g := range log.list() {
		if g[len(g)-len("expired"):] != "expired" {
			t.Fatalf("sweep reason %q, want expired", g)
		}
	}
}

// An expired entry at the LRU end is swept before a live one is evicted for capacity.
func TestMemory_sweepBeforeCapacityEviction(t *testing.T) {
	log := newEvictLog()
	m, f := newMem(t, cache.WithCapacity[string, int](2), cache.WithOnEvict(log.hook))
	mustSet(t, m, "old", 1, time.Second)
	mustSet(t, m, "live", 2, 0)
	f.Advance(time.Second)
	mustSet(t, m, "new", 3, 0)
	if got, want := log.list(), []string{"old:expired"}; !slices.Equal(got, want) {
		t.Fatalf("hook = %v, want %v", got, want)
	}
	if _, ok := mustGet(t, m, "live"); !ok {
		t.Fatal("live evicted although an expired entry was there")
	}
}

func TestMemory_hookReasons(t *testing.T) {
	log := newEvictLog()
	m, f := newMem(t, cache.WithCapacity[string, int](1), cache.WithOnEvict(log.hook))
	mustSet(t, m, "a", 1, 0)
	mustSet(t, m, "a", 2, 0)                           // Replaced
	if err := m.Delete(t.Context(), "a"); err != nil { // Deleted
		t.Fatal(err)
	}
	mustSet(t, m, "b", 1, 0)
	mustSet(t, m, "c", 1, 0) // Evicted b
	mustSet(t, m, "c", 2, time.Second)
	f.Advance(time.Second)
	mustSet(t, m, "c", 3, 0) // the replaced entry had expired: Expired
	want := []string{"a:replaced", "a:deleted", "b:evicted", "c:replaced", "c:expired"}
	if got := log.list(); !slices.Equal(got, want) {
		t.Fatalf("hook = %v, want %v", got, want)
	}
}

func TestMemory_hookPanicIsErrPanic(t *testing.T) {
	m, _ := newMem(t, cache.WithOnEvict(func(string, int, cache.Reason) { panic("hook") }))
	mustSet(t, m, "a", 1, 0)
	err := m.Set(t.Context(), "a", 2, 0)
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Set with panicking hook = %v, want ErrPanic", err)
	}
	if v, _ := mustGet(t, m, "a"); v != 2 {
		t.Fatalf("value after hook panic = %d, want 2 (stored)", v)
	}
}

// The hook may call back into the cache: it runs outside the lock (a hook under the lock would deadlock here).
func TestMemory_hookMayCallCache(t *testing.T) {
	var m *cache.Memory[string, int]
	lens := make(chan int, 4)
	m, _ = newMem(t, cache.WithCapacity[string, int](1), cache.WithOnEvict(func(k string, _ int, _ cache.Reason) {
		lens <- m.Len()
	}))
	mustSet(t, m, "a", 1, 0)
	mustSet(t, m, "b", 1, 0)
	if got := <-lens; got != 1 {
		t.Fatalf("Len inside hook = %d, want 1", got)
	}
}

func TestMemory_negativeTTL(t *testing.T) {
	m, _ := newMem[string, int](t)
	if err := m.Set(t.Context(), "a", 1, -time.Second); !errors.Is(err, cache.ErrTTL) {
		t.Fatalf("Set(ttl<0) = %v, want ErrTTL", err)
	}
	if m.Len() != 0 {
		t.Fatal("negative TTL stored the entry")
	}
}

func TestMemory_NaNKeyRefused(t *testing.T) {
	m, _ := newMem[float64, int](t)
	err := m.Set(t.Context(), math.NaN(), 1, 0)
	if !errors.Is(err, list.ErrKeyNotComparable) {
		t.Fatalf("Set(NaN) = %v, want list.ErrKeyNotComparable", err)
	}
}

func TestNew_invalidOptions(t *testing.T) {
	h := func(s string) []byte { return []byte(s) }
	cases := map[string][]cache.Option[string, int]{
		"capacity 0":    {cache.WithCapacity[string, int](0)},
		"capacity -1":   {cache.WithCapacity[string, int](-1)},
		"ttl -1":        {cache.WithTTL[string, int](-1)},
		"bloom nil":     {cache.WithBloom[string, int](10, 0.01, nil)},
		"bloom n 0":     {cache.WithBloom[string, int](0, 0.01, h)},
		"bloom fpr 0":   {cache.WithBloom[string, int](10, 0, h)},
		"bloom fpr 1":   {cache.WithBloom[string, int](10, 1, h)},
		"bloom fpr NaN": {cache.WithBloom[string, int](10, math.NaN(), h)},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := cache.New(opts...)
			if !errors.Is(err, cache.ErrConfig) || m != nil {
				t.Fatalf("New = %v, %v; want nil, ErrConfig", m, err)
			}
		})
	}
	if _, err := cache.New[string, int](nil, cache.WithClock[string, int](nil)); err != nil {
		t.Fatalf("nil option and nil clock: %v, want defaults", err)
	}
}

func TestReason_String(t *testing.T) {
	for r, want := range map[cache.Reason]string{cache.Evicted: "evicted", cache.Expired: "expired",
		cache.Deleted: "deleted", cache.Replaced: "replaced", cache.Reason(9): "Reason(9)"} {
		if r.String() != want {
			t.Fatalf("%d.String() = %q, want %q", int(r), r.String(), want)
		}
	}
}

func TestMemory_concurrentUseIsRaceFree(t *testing.T) {
	leakCheck(t)
	m, f := newMem(t, cache.WithCapacity[int, int](8), cache.WithTTL[int, int](time.Second))
	done := make(chan struct{})
	for w := range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := range 200 {
				k := (w*31 + i) % 20
				_ = m.Set(t.Context(), k, i, 0)
				_, _, _ = m.Get(t.Context(), k)
				if i%7 == 0 {
					_ = m.Delete(t.Context(), k)
				}
			}
		}()
	}
	f.Advance(2 * time.Second)
	for range 8 {
		<-done
	}
	if m.Len() > 8 {
		t.Fatalf("Len = %d over capacity 8", m.Len())
	}
}
