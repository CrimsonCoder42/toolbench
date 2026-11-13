package guard_test

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/CrimsonCoder42/toolbench/guard"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

func ExampleGuarded() {
	var hits guard.Guarded[map[string]int] // the zero value is ready: the map is nil until Do sets it

	hits.Do(func(m *map[string]int) {
		if *m == nil {
			*m = map[string]int{}
		}
		(*m)["/home"]++
		(*m)["/home"]++
	})

	var n int
	hits.Do(func(m *map[string]int) { n = (*m)["/home"] }) // read a map inside Do: Load is a shallow copy
	fmt.Println("/home", n)
	// Output:
	// /home 2
}

func TestGuarded_concurrentDo(t *testing.T) {
	leak.Verify(t)
	const n = 1000
	var g guard.Guarded[int]
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			g.Do(func(v *int) { *v++ })
			_ = g.Load()
		})
	}
	wg.Wait()
	if got := g.Load(); got != n {
		t.Fatalf("Load() = %d, want %d", got, n)
	}
}

// TestGuarded_go20UnguardedCounter is the go_20 "before" program (2000 buyers, 500 tickets, a shared counter
// with no guard: a data race and a wrong count) with the counter in a Guarded: exactly 500 are sold and -race
// is clean.
func TestGuarded_go20UnguardedCounter(t *testing.T) {
	leak.Verify(t)
	type stock struct{ remaining, sold, refused int }
	g := &guard.Guarded[stock]{}
	g.Do(func(s *stock) { s.remaining = 500 })

	var wg sync.WaitGroup
	for range 2000 {
		wg.Go(func() {
			g.Do(func(s *stock) {
				if s.remaining > 0 {
					s.remaining--
					s.sold++
					return
				}
				s.refused++
			})
		})
	}
	wg.Wait()
	if got := g.Load(); got != (stock{remaining: 0, sold: 500, refused: 1500}) {
		t.Fatalf("Load() = %+v, want 0 remaining, 500 sold, 1500 refused", got)
	}
}

func TestGuarded_zeroValue(t *testing.T) {
	var g guard.Guarded[string]
	if got := g.Load(); got != "" {
		t.Fatalf("zero Guarded Load() = %q, want empty", got)
	}
	g.Do(func(s *string) { *s = "set" })
	if got := g.Load(); got != "set" {
		t.Fatalf("Load() = %q, want %q", got, "set")
	}
}

func TestGuarded_nilFn(t *testing.T) {
	var g guard.Guarded[int]
	g.Do(nil) // no panic, no change
	g.Do(func(v *int) { *v = 7 })
	g.Do(nil)
	if got := g.Load(); got != 7 {
		t.Fatalf("Load() = %d, want 7", got)
	}
}

// TestGuarded_LoadShallow pins the documented behaviour: Load copies T, not what T points to.
func TestGuarded_LoadShallow(t *testing.T) {
	var g guard.Guarded[[]int]
	g.Do(func(s *[]int) { *s = []int{1, 2, 3} })
	c := g.Load()
	c[0] = 9 // shares the backing array with the guarded slice (single goroutine here, so no race)
	if got := g.Load()[0]; got != 9 {
		t.Fatalf("Load()[0] = %d after writing through the copy, want 9 (shallow copy)", got)
	}
	c = append(c[:0:0], c...) // a deep copy breaks the sharing
	c[0] = 1
	if got := g.Load()[0]; got != 9 {
		t.Fatalf("Load()[0] = %d after writing a deep copy, want 9", got)
	}
}

func BenchmarkGuarded_Do(b *testing.B) {
	b.ReportAllocs()
	var g guard.Guarded[int]
	inc := func(v *int) { *v++ }
	for b.Loop() {
		g.Do(inc)
	}
}

func BenchmarkGuarded_DoParallel(b *testing.B) {
	b.ReportAllocs()
	var g guard.Guarded[int]
	inc := func(v *int) { *v++ }
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			g.Do(inc)
		}
	})
}

// ExampleGuarded_map: a map written by many goroutines through one Guarded; the total is exact.
func ExampleGuarded_map() {
	var hits guard.Guarded[map[string]int]
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			path := "/a"
			if i%2 == 1 {
				path = "/b"
			}
			hits.Do(func(m *map[string]int) {
				if *m == nil {
					*m = map[string]int{}
				}
				(*m)[path]++
			})
		})
	}
	wg.Wait()
	var a, b int
	hits.Do(func(m *map[string]int) { a, b = (*m)["/a"], (*m)["/b"] })
	fmt.Println("/a", a, "/b", b, "total", a+b)
	// Output:
	// /a 50 /b 50 total 100
}

// ExampleGuarded_slice: append from many goroutines through one Guarded; the length is exact.
func ExampleGuarded_slice() {
	var log guard.Guarded[[]int]
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			log.Do(func(s *[]int) { *s = append(*s, i) })
		})
	}
	wg.Wait()
	var n, sum int
	log.Do(func(s *[]int) {
		n = len(*s)
		for _, v := range *s {
			sum += v
		}
	})
	fmt.Println("entries", n, "sum", sum)
	// Output:
	// entries 100 sum 4950
}

// TestGuarded_mapManyWriters: thirteen reference repos write a package-level map or slice from handler
// goroutines with no guard (A-Blockchain-With-Golang main.go:74, go-chi-CRUD controllers/article.go:46,
// Complete_GO_Blockchain core/server.go:314, CSRF-Golang-Killer-Project db/db.go:21, sitemap-crawler
// main.go:126, langchain-GO chains/vacation.go:69, Videocall-Chat-GO-Project internal/server/server.go:62;
// code-heim go_20 before/main.go): a data race ("concurrent map writes" fatal error, or a lost append). Here the
// same shape runs through Guarded under -race: every write lands, the totals are exact.
func TestGuarded_mapManyWriters(t *testing.T) {
	leak.Verify(t)
	const writers, perWriter = 64, 200
	var index guard.Guarded[map[string]int] // the repos' package-level map
	var articles guard.Guarded[[]string]    // the repos' package-level slice
	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			for i := range perWriter {
				key := "k" + fmt.Sprint(i%10)
				index.Do(func(m *map[string]int) {
					if *m == nil {
						*m = map[string]int{}
					}
					(*m)[key]++
				})
				articles.Do(func(s *[]string) { *s = append(*s, fmt.Sprintf("%d/%d", w, i)) })
			}
		})
	}
	wg.Wait()
	total := 0
	index.Do(func(m *map[string]int) {
		if len(*m) != 10 {
			t.Errorf("map has %d keys, want 10", len(*m))
		}
		for _, n := range *m {
			total += n
		}
	})
	if total != writers*perWriter {
		t.Fatalf("map total = %d, want %d", total, writers*perWriter)
	}
	var n int
	articles.Do(func(s *[]string) { n = len(*s) })
	if n != writers*perWriter {
		t.Fatalf("slice len = %d, want %d", n, writers*perWriter)
	}
}
