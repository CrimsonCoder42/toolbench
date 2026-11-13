package cache_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/cache"
	"github.com/CrimsonCoder42/toolbench/clock"
)

func ExampleNew() {
	m, err := cache.New(cache.WithCapacity[string, int](2))
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx := context.Background()
	_ = m.Set(ctx, "a", 1, 0)
	_ = m.Set(ctx, "b", 2, 0)
	_ = m.Set(ctx, "c", 3, 0) // evicts a, the least recently used
	_, ok, _ := m.Get(ctx, "a")
	fmt.Println(ok, m.Len())
	// Output: false 2
}

func ExampleMemory() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m, _ := cache.New(cache.WithClock[string, string](f), cache.WithTTL[string, string](time.Minute))
	ctx := context.Background()
	_ = m.Set(ctx, "session", "alice", 0)
	v, ok, _ := m.Get(ctx, "session")
	fmt.Println(v, ok)
	f.Advance(time.Minute)
	_, ok, _ = m.Get(ctx, "session")
	fmt.Println(ok)
	// Output:
	// alice true
	// false
}

func ExampleMemory_GetOrLoad() {
	m, _ := cache.New[int, string]()
	loads := 0
	load := func(context.Context) (string, error) { loads++; return "row 7", nil }
	for range 3 {
		v, _ := m.GetOrLoad(context.Background(), 7, load)
		fmt.Println(v)
	}
	fmt.Println("loads:", loads)
	// Output:
	// row 7
	// row 7
	// row 7
	// loads: 1
}

func ExampleCache() {
	var c cache.Cache[string, int]
	m, _ := cache.New[string, int]()
	c = m // any implementation: Memory here, an adapter over Redis elsewhere
	_ = c.Set(context.Background(), "n", 1, time.Hour)
	v, ok, err := c.Get(context.Background(), "n")
	fmt.Println(v, ok, err)
	// Output: 1 true <nil>
}

func ExampleAside() {
	m, _ := cache.New[string, int]()
	load := func(context.Context) (int, error) { fmt.Println("loading"); return 10, nil }
	for range 2 {
		v, err := cache.Aside[string, int](context.Background(), m, "price", time.Minute, load)
		fmt.Println(v, err)
	}
	// Output:
	// loading
	// 10 <nil>
	// 10 <nil>
}

func ExampleWithOnEvict() {
	m, _ := cache.New(cache.WithCapacity[string, int](1),
		cache.WithOnEvict(func(k string, v int, r cache.Reason) { fmt.Println(k, v, r) }))
	ctx := context.Background()
	_ = m.Set(ctx, "a", 1, 0)
	_ = m.Set(ctx, "a", 2, 0)
	_ = m.Set(ctx, "b", 3, 0)
	_ = m.Delete(ctx, "b")
	// Output:
	// a 1 replaced
	// a 2 evicted
	// b 3 deleted
}

func ExampleWithBloom() {
	m, _ := cache.New(cache.WithBloom[string, string](1000, 0.001, func(k string) []byte { return []byte(k) }))
	_ = m.AddKnown("user:1")
	load := func(context.Context) (string, error) { return "found in store", nil }
	v, err := m.GetOrLoad(context.Background(), "user:1", load)
	fmt.Println(v, err)
	_, err = m.GetOrLoad(context.Background(), "user:404", load)
	fmt.Println(errors.Is(err, cache.ErrNotFound))
	// Output:
	// found in store <nil>
	// true
}

func ExampleWithCapacity() {
	m, _ := cache.New(cache.WithCapacity[int, int](3))
	for i := range 10 {
		_ = m.Set(context.Background(), i, i, 0)
	}
	fmt.Println(m.Len())
	// Output: 3
}

func ExampleWithTTL() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m, _ := cache.New(cache.WithClock[string, int](f), cache.WithTTL[string, int](time.Second))
	_ = m.Set(context.Background(), "default", 1, 0)
	_ = m.Set(context.Background(), "longer", 2, time.Hour)
	f.Advance(time.Second)
	_, a, _ := m.Get(context.Background(), "default")
	_, b, _ := m.Get(context.Background(), "longer")
	fmt.Println(a, b)
	// Output: false true
}

func ExampleWithClock() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m, _ := cache.New(cache.WithClock[string, int](f))
	_ = m.Set(context.Background(), "k", 1, time.Minute)
	f.Advance(59 * time.Second)
	_, ok, _ := m.Get(context.Background(), "k")
	fmt.Println(ok)
	// Output: true
}

func ExampleReason() {
	fmt.Println(cache.Evicted, cache.Expired, cache.Deleted, cache.Replaced)
	// Output: evicted expired deleted replaced
}

func ExampleErrConfig() {
	_, err := cache.New(cache.WithCapacity[string, int](0))
	fmt.Println(errors.Is(err, cache.ErrConfig))
	// Output: true
}

func ExampleErrTTL() {
	m, _ := cache.New[string, int]()
	fmt.Println(errors.Is(m.Set(context.Background(), "k", 1, -time.Second), cache.ErrTTL))
	// Output: true
}

func ExampleErrNotFound() {
	m, _ := cache.New(cache.WithBloom[string, int](10, 0.01, func(k string) []byte { return []byte(k) }))
	_, err := m.GetOrLoad(context.Background(), "nobody", func(context.Context) (int, error) { return 0, nil })
	fmt.Println(errors.Is(err, cache.ErrNotFound))
	// Output: true
}

type downCache struct{}

func (downCache) Get(context.Context, string) (int, bool, error)        { return 0, false, errors.New("down") }
func (downCache) Set(context.Context, string, int, time.Duration) error { return errors.New("down") }
func (downCache) Delete(context.Context, string) error                  { return errors.New("down") }

func ExampleErrCache() {
	v, err := cache.Aside[string, int](context.Background(), downCache{}, "k", 0,
		func(context.Context) (int, error) { return 5, nil })
	fmt.Println(v, errors.Is(err, cache.ErrCache))
	// Output: 5 true
}

func ExampleDefaultCapacity() {
	fmt.Println(cache.DefaultCapacity)
	// Output: 10000
}

func ExampleOption() {
	opts := []cache.Option[string, int]{cache.WithCapacity[string, int](100), cache.WithTTL[string, int](time.Minute)}
	_, err := cache.New(opts...)
	fmt.Println(err)
	// Output: <nil>
}
