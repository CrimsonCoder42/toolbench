package ratelimit_test

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/list"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/ratelimit"
)

func TestKeyed_perKeyBuckets(t *testing.T) {
	f := clock.NewFake(start)
	k := mustKeyed[string](t, perSecond(1), 2, ratelimit.WithClock(f))
	for _, key := range []string{"a", "b"} {
		for i := range 2 {
			if ok, _ := k.Allow(key); !ok {
				t.Fatalf("%s call %d refused", key, i)
			}
		}
	}
	if ok, retry := k.Allow("a"); ok || retry != time.Second {
		t.Fatalf("a over its burst = (%v, %v), want (false, 1s)", ok, retry)
	}
	if ok, _ := k.Allow("c"); !ok {
		t.Fatal("a fresh key c refused: buckets are shared")
	}
	f.Advance(time.Second)
	if ok, _ := k.Allow("a"); !ok {
		t.Fatal("a refused after its refill")
	}
	if got := k.Limit(); got != perSecond(1) {
		t.Errorf("Limit = %+v", got)
	}
	if got := k.Burst(); got != 2 {
		t.Errorf("Burst = %d", got)
	}
}

func TestKeyed_AllowN(t *testing.T) {
	f := clock.NewFake(start)
	k := mustKeyed[int](t, perSecond(2), 4, ratelimit.WithClock(f))
	if ok, retry, err := k.AllowN(1, 0); !ok || retry != 0 || err != nil {
		t.Errorf("AllowN(n=0) = (%v, %v, %v)", ok, retry, err)
	}
	if got := k.Len(); got != 0 {
		t.Errorf("AllowN(n=0) added a key: Len %d", got)
	}
	ok, retry, err := k.AllowN(1, 5)
	if ok || retry != ratelimit.InfDuration || !errors.Is(err, ratelimit.ErrBurst) {
		t.Errorf("AllowN(n=5) over burst 4 = (%v, %v, %v), want ErrBurst", ok, retry, err)
	}
	if ok, _, err := k.AllowN(1, 4); !ok || err != nil {
		t.Errorf("AllowN(n=4) = (%v, %v)", ok, err)
	}
	if ok, retry, err := k.AllowN(1, 3); ok || retry != 1500*time.Millisecond || err != nil {
		t.Errorf("AllowN(n=3) on empty = (%v, %v, %v), want (false, 1.5s, nil)", ok, retry, err)
	}
}

// Default idle TTL is the full-refill time: burst 3 at 2 per second refills in 1.5 s.
func TestKeyed_idleEvictionOnTheClock(t *testing.T) {
	f := clock.NewFake(start)
	k := mustKeyed[string](t, perSecond(2), 3, ratelimit.WithClock(f))
	k.Allow("a")
	f.Advance(time.Second)
	k.Allow("b")
	if got := k.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2", got)
	}
	f.Advance(499 * time.Millisecond) // a unseen 1.499 s
	if got := k.Len(); got != 2 {
		t.Fatalf("Len before a's TTL = %d, want 2", got)
	}
	f.Advance(time.Millisecond) // a unseen 1.5 s: evicted
	if got := k.Len(); got != 1 {
		t.Fatalf("Len at a's TTL = %d, want 1", got)
	}
	f.Advance(time.Second) // b unseen 1.5 s
	if got := k.Len(); got != 0 {
		t.Fatalf("Len at b's TTL = %d, want 0", got)
	}
}

// Eviction at the default TTL never changes a decision: the evicted bucket was full.
func TestKeyed_defaultEvictionIsLossless(t *testing.T) {
	f := clock.NewFake(start)
	k := mustKeyed[string](t, perSecond(2), 3, ratelimit.WithClock(f))
	l := mustLimiter(t, perSecond(2), 3, ratelimit.WithClock(f))
	steps := []time.Duration{0, 0, 0, 0, 1500 * time.Millisecond, 0, 0, 0, 0, 3 * time.Second, 0, 250 * time.Millisecond, 250 * time.Millisecond}
	for i, d := range steps {
		f.Advance(d)
		got, _ := k.Allow("x")
		if want := l.Allow(); got != want {
			t.Fatalf("step %d: Keyed %v, Limiter %v", i, got, want)
		}
	}
}

func TestKeyed_WithIdleTTL(t *testing.T) {
	f := clock.NewFake(start)
	k := mustKeyed[string](t, perSecond(1), 1, ratelimit.WithClock(f), ratelimit.WithIdleTTL(10*time.Second))
	k.Allow("a")
	f.Advance(9 * time.Second)
	if got := k.Len(); got != 1 {
		t.Fatalf("Len at 9 s = %d, want 1", got)
	}
	f.Advance(time.Second)
	if got := k.Len(); got != 0 {
		t.Fatalf("Len at 10 s = %d, want 0", got)
	}
}

func TestKeyed_WithMaxKeys_ErrKeys(t *testing.T) {
	f := clock.NewFake(start)
	k := mustKeyed[string](t, perSecond(1), 1, ratelimit.WithClock(f), ratelimit.WithMaxKeys(2), ratelimit.WithIdleTTL(time.Minute))
	k.Allow("a")
	f.Advance(10 * time.Second)
	k.Allow("b")
	ok, retry, err := k.AllowN("c", 1)
	if ok || !errors.Is(err, ratelimit.ErrKeys) || retry != 50*time.Second {
		t.Fatalf("third key = (%v, %v, %v), want (false, 50s, ErrKeys)", ok, retry, err)
	}
	if ok, retry := k.Allow("c"); ok || retry != 50*time.Second {
		t.Fatalf("Allow third key = (%v, %v), want (false, 50s)", ok, retry)
	}
	if ok, _, err := k.AllowN("a", 1); !ok || err != nil {
		t.Fatalf("existing key a at a full table = (%v, %v)", ok, err) // a is now the most recent
	}
	f.Advance(50 * time.Second) // b unseen 50 s, a unseen 50 s: neither idle yet
	if _, _, err := k.AllowN("c", 1); !errors.Is(err, ratelimit.ErrKeys) {
		t.Fatalf("c before b idles: %v, want ErrKeys", err)
	}
	f.Advance(10 * time.Second) // b unseen 60 s: evicted
	if ok, _, err := k.AllowN("c", 1); !ok || err != nil {
		t.Fatalf("c after b idled = (%v, %v)", ok, err)
	}
	if got := k.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1 (a idled too)", got)
	}
}

// A NaN key is not equal to itself, so no table can find it again: it is refused (ErrKeys wrapping
// list.ErrKeyNotComparable) and never fills the table.
func TestKeyed_NaNKeyRefused(t *testing.T) {
	f := clock.NewFake(start)
	k := mustKeyed[float64](t, perSecond(1), 1, ratelimit.WithClock(f), ratelimit.WithMaxKeys(2))
	for range 3 {
		ok, retry, err := k.AllowN(math.NaN(), 1)
		if ok || retry != ratelimit.InfDuration || !errors.Is(err, ratelimit.ErrKeys) || !errors.Is(err, list.ErrKeyNotComparable) {
			t.Fatalf("AllowN(NaN) = (%v, %v, %v), want (false, InfDuration, ErrKeys wrapping ErrKeyNotComparable)", ok, retry, err)
		}
	}
	if got := k.Len(); got != 0 {
		t.Errorf("Len after NaN keys = %d, want 0", got)
	}
	if ok, _ := k.Allow(1.5); !ok {
		t.Error("Allow(1.5) after NaN keys = false, want true (the table did not fill)")
	}
}

func TestKeyed_zeroValue(t *testing.T) {
	var k ratelimit.Keyed[string]
	if ok, retry := k.Allow("a"); ok || retry != ratelimit.InfDuration {
		t.Errorf("zero Keyed Allow = (%v, %v)", ok, retry)
	}
	if got := k.Len(); got != 0 {
		t.Errorf("zero Keyed Len = %d", got)
	}
}

// go_77_rate_limit: a plain map of limiters read and written from every request goroutine with no lock (a
// data race the race detector reports). Here many goroutines on many keys at once; -race must stay clean
// and each key must get exactly its burst.
func TestKeyed_go77_concurrentKeysNoDataRace(t *testing.T) {
	leak.Verify(t)
	f := clock.NewFake(start)
	k := mustKeyed[int](t, perSecond(1), 3, ratelimit.WithClock(f))
	const keys, perKey = 16, 8
	var allowed [keys]atomic.Int64
	var wg sync.WaitGroup
	for key := range keys {
		for range perKey {
			wg.Go(func() {
				if ok, _ := k.Allow(key); ok {
					allowed[key].Add(1)
				}
			})
		}
	}
	wg.Wait()
	for key := range keys {
		if got := allowed[key].Load(); got != 3 {
			t.Errorf("key %d allowed %d, want the burst 3", key, got)
		}
	}
}

// go_82_rate_limiting_revised: the map of limiters is never evicted (it grows with every client ever seen)
// and the sync.Map variant builds a new limiter for every request (LoadOrStore with a fresh value). Here
// the table stays within WithMaxKeys whatever arrives, and a request for a known key allocates nothing.
func TestKeyed_go82_boundedAndNoAllocationPerRequest(t *testing.T) {
	f := clock.NewFake(start)
	k := mustKeyed[string](t, perSecond(100), 1, ratelimit.WithClock(f), ratelimit.WithMaxKeys(8))
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("client-%d", i)
	}
	for _, key := range keys {
		f.Advance(5 * time.Millisecond) // a key idles after 10 ms: two new keys arrive per TTL
		k.Allow(key)
		if got := k.Len(); got > 8 {
			t.Fatalf("Len = %d, want at most 8", got)
		}
	}
	k.Allow("known")
	allocs := testing.AllocsPerRun(100, func() { k.Allow("known") })
	if allocs != 0 {
		t.Errorf("Allow for a known key allocates %v times, want 0", allocs)
	}
}

func BenchmarkKeyed_Allow(b *testing.B) {
	f := clock.NewFake(start)
	k := mustKeyed[int](b, perSecond(1_000_000), 100, ratelimit.WithClock(f))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k.Allow(i % 64)
		i++
	}
}
