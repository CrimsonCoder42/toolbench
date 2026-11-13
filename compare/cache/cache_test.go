// Package cache_test compares toolbench/cache.Memory with github.com/hashicorp/golang-lru/v2/expirable v2.0.7
// (LADDER.md rank 30) where the shapes meet: least-recently-used order, the capacity bound, what Get returns,
// and which keys leave and in what order, under the same fixed-seed sequence of Set/Add, Get and
// Delete/Remove. TTL is never compared: expirable reads time.Now and, with a TTL, starts a cleanup goroutine
// that never exits (expirable_lru.go:79-81, "done channel is never closed"); every TTL case is tested in the
// cache package on clock.Fake. Both caches here are built with no TTL, so expirable starts no goroutine.
//
// Known differences, by design (doc/specs/cache.md, "Compared against"):
//   - Size 0: expirable is unbounded (Add evicts only when size > 0); cache.New returns ErrConfig.
//   - The eviction callback: expirable calls it under its own lock during Add/Remove; ours runs after the
//     lock is released and carries a Reason. Ours also reports Replaced on a Set of an existing key, which
//     expirable updates in place silently; the comparison below filters Replaced out.
//   - Get/Set take a ctx and Get returns (v, ok, err) (the Cache interface); expirable has no ctx and no error.
package cache_test

import (
	"context"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/CrimsonCoder42/toolbench/cache"
)

type op struct {
	kind int // 0 set, 1 get, 2 delete
	key  string
	val  int
}

func script(seed uint64, n, keys int) []op {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	ops := make([]op, n)
	for i := range ops {
		ops[i] = op{kind: r.IntN(3), key: "k" + strconv.Itoa(r.IntN(keys)), val: i}
	}
	return ops
}

func TestCompare_LRUOrderAndCapacity(t *testing.T) {
	ctx := context.Background()
	for _, size := range []int{1, 2, 5, 16} {
		for seed := range uint64(20) {
			t.Run(strconv.Itoa(size)+"/"+strconv.FormatUint(seed, 10), func(t *testing.T) {
				var theirsGone, oursGone []string
				theirs := expirable.NewLRU[string, int](size, func(k string, v int) {
					theirsGone = append(theirsGone, k+"="+strconv.Itoa(v))
				}, 0)
				ours, err := cache.New(cache.WithCapacity[string, int](size),
					cache.WithOnEvict(func(k string, v int, r cache.Reason) {
						if r != cache.Replaced { // expirable updates in place without a callback
							oursGone = append(oursGone, k+"="+strconv.Itoa(v))
						}
					}))
				if err != nil {
					t.Fatal(err)
				}
				for i, o := range script(seed, 400, 3*size+1) {
					switch o.kind {
					case 0:
						theirs.Add(o.key, o.val)
						if err := ours.Set(ctx, o.key, o.val, 0); err != nil {
							t.Fatal(err)
						}
					case 1:
						tv, tok := theirs.Get(o.key)
						ov, ook, err := ours.Get(ctx, o.key)
						if err != nil || tv != ov || tok != ook {
							t.Fatalf("op %d Get(%s): theirs %d,%v ours %d,%v,%v", i, o.key, tv, tok, ov, ook, err)
						}
					case 2:
						theirs.Remove(o.key)
						if err := ours.Delete(ctx, o.key); err != nil {
							t.Fatal(err)
						}
					}
					if theirs.Len() != ours.Len() || ours.Len() > size {
						t.Fatalf("op %d: Len theirs %d ours %d (cap %d)", i, theirs.Len(), ours.Len(), size)
					}
				}
				if !slices.Equal(theirsGone, oursGone) {
					t.Fatalf("removals differ:\ntheirs %v\nours   %v", theirsGone, oursGone)
				}
				// The survivors, oldest first, agree: expirable's Keys is oldest-to-newest.
				for _, k := range theirs.Keys() {
					if _, ok, _ := ours.Get(ctx, k); !ok {
						t.Fatalf("theirs kept %s, ours did not", k)
					}
				}
			})
		}
	}
}

// Size 0: expirable keeps everything (unbounded); toolbench refuses the configuration.
func TestCompare_sizeZero(t *testing.T) {
	theirs := expirable.NewLRU[int, int](0, nil, 0)
	for i := range 100 {
		theirs.Add(i, i)
	}
	if theirs.Len() != 100 {
		t.Fatalf("expirable size 0 Len = %d, want 100 (unbounded)", theirs.Len())
	}
	if _, err := cache.New(cache.WithCapacity[int, int](0)); err == nil {
		t.Fatal("cache.New with capacity 0 succeeded, want ErrConfig")
	}
}
