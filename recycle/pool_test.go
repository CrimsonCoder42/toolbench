package recycle_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/recycle"
)

// item is a pooled value that records who holds it, so a value handed to two callers at once is caught.
type item struct {
	held  atomic.Bool
	dirty int
}

// TestPool_Get_newFnWhenEmpty: an empty pool calls newFn, once per Get.
func TestPool_Get_newFnWhenEmpty(t *testing.T) {
	var calls atomic.Int64
	p := recycle.New(func() *item { calls.Add(1); return new(item) })
	a, b := p.Get(), p.Get()
	if a == nil || b == nil || a == b {
		t.Fatalf("Get on an empty pool: got %p and %p, want two distinct new items", a, b)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("newFn calls = %d, want 2", n)
	}
}

// TestPool_Get_nilNewFn: a nil newFn returns the zero T on an empty pool, for value, pointer, slice and
// interface types, and never panics.
func TestPool_Get_nilNewFn(t *testing.T) {
	if got := recycle.New[int](nil).Get(); got != 0 {
		t.Errorf("int: got %d, want 0", got)
	}
	if got := recycle.New[*item](nil).Get(); got != nil {
		t.Errorf("*item: got %p, want nil", got)
	}
	if got := recycle.New[[]byte](nil).Get(); got != nil {
		t.Errorf("[]byte: got %v, want nil", got)
	}
	if got := recycle.New[error](nil).Get(); got != nil {
		t.Errorf("error: got %v, want nil", got)
	}
}

// TestPool_zeroValue: the zero Pool is ready to use: Get returns the zero T, Put pools values unchanged.
func TestPool_zeroValue(t *testing.T) {
	var p recycle.Pool[*item]
	if got := p.Get(); got != nil {
		t.Fatalf("zero Pool Get: got %p, want nil", got)
	}
	it := &item{dirty: 7}
	p.Put(it)
	if got := p.Get(); got != nil && (got != it || got.dirty != 7) {
		t.Fatalf("zero Pool: got %+v, want the value put (unchanged) or nil", got)
	}
}

// TestPool_Get_nilInterfaceFromNewFn: a newFn that returns a nil interface value does not panic Get.
func TestPool_Get_nilInterfaceFromNewFn(t *testing.T) {
	p := recycle.New(func() error { return nil })
	if got := p.Get(); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
	p.Put(nil) // sync.Pool ignores a nil interface; must not panic
	if got := p.Get(); got != nil {
		t.Fatalf("after Put(nil): got %v, want nil", got)
	}
}

// TestPool_Put_resetApplied: every kept Put calls reset exactly once, and a value that comes back from Get has
// been reset. Reuse itself is not asserted (the GC, and -race, may empty the pool at any time).
func TestPool_Put_resetApplied(t *testing.T) {
	var resets atomic.Int64
	p := recycle.New(
		func() *item { return new(item) },
		recycle.WithReset(func(it *item) *item { resets.Add(1); it.dirty = 0; return it }),
	)
	put := map[*item]bool{}
	for i := range 10 {
		it := p.Get()
		it.dirty = i + 1
		put[it] = true
		p.Put(it)
	}
	if n := resets.Load(); n != 10 {
		t.Fatalf("reset calls = %d, want 10", n)
	}
	for range 10 {
		it := p.Get()
		if put[it] && it.dirty != 0 {
			t.Fatalf("a reused value was not reset: dirty = %d", it.dirty)
		}
	}
}

// TestPool_Put_resetResultIsPooled: the value reset returns is what is pooled, not the argument.
func TestPool_Put_resetResultIsPooled(t *testing.T) {
	replacement := &item{dirty: -1}
	p := recycle.New[*item](nil, recycle.WithReset(func(*item) *item { return replacement }))
	p.Put(&item{dirty: 5})
	if got := p.Get(); got != nil && got != replacement {
		t.Fatalf("got %+v, want the value reset returned (or nil if the pool was cleared)", got)
	}
}

// TestPool_Put_keepDropsOversized: a buffer keep rejects is never pooled and never handed out again, and reset
// is not called for it (golang/go#23199). Deterministic: the pool holds nothing else, so Get must call newFn.
func TestPool_Put_keepDropsOversized(t *testing.T) {
	const maxCap = 4 << 10
	var resets, news atomic.Int64
	p := recycle.New(
		func() *bytes.Buffer { news.Add(1); return bytes.NewBuffer(make([]byte, 0, 512)) },
		recycle.WithKeep(func(b *bytes.Buffer) bool { return b.Cap() <= maxCap }),
		recycle.WithReset(func(b *bytes.Buffer) *bytes.Buffer { resets.Add(1); b.Reset(); return b }),
	)
	big := p.Get()
	big.Grow(1 << 20)
	p.Put(big)
	if n := resets.Load(); n != 0 {
		t.Fatalf("reset called %d times for a dropped value, want 0", n)
	}
	got := p.Get()
	if got == big {
		t.Fatal("the oversized buffer came back from Get")
	}
	if got.Cap() > maxCap {
		t.Fatalf("Get returned cap %d, want <= %d", got.Cap(), maxCap)
	}
	if n := news.Load(); n != 2 {
		t.Fatalf("newFn calls = %d, want 2 (the dropped buffer was not pooled)", n)
	}
}

// TestPool_Put_keepAcceptsSmall: a value keep accepts is reset and pooled.
func TestPool_Put_keepAcceptsSmall(t *testing.T) {
	var resets atomic.Int64
	p := recycle.New(
		func() *bytes.Buffer { return new(bytes.Buffer) },
		recycle.WithKeep(func(b *bytes.Buffer) bool { return b.Cap() <= 1<<20 }),
		recycle.WithReset(func(b *bytes.Buffer) *bytes.Buffer { resets.Add(1); b.Reset(); return b }),
	)
	b := p.Get()
	b.WriteString("small")
	p.Put(b)
	if n := resets.Load(); n != 1 {
		t.Fatalf("reset calls = %d, want 1", n)
	}
	if got := p.Get(); got.Len() != 0 {
		t.Fatalf("Get after Put: Len = %d, want 0", got.Len())
	}
}

// TestNew_nilOptions: nil options, a nil reset and a nil keep are ignored.
func TestNew_nilOptions(t *testing.T) {
	p := recycle.New(func() *item { return new(item) },
		nil, recycle.WithReset[*item](nil), recycle.WithKeep[*item](nil))
	it := p.Get()
	it.dirty = 3
	p.Put(it)
	if got := p.Get(); got == it && got.dirty != 3 {
		t.Fatalf("a nil reset changed the value: dirty = %d", got.dirty)
	}
}

// TestPool_callbackPanicPropagates: a panic in newFn, keep or reset reaches the caller (as with sync.Pool),
// and a panicking Put pools nothing.
func TestPool_callbackPanicPropagates(t *testing.T) {
	boom := errors.New("boom")
	mustPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != boom {
				t.Errorf("%s: recovered %v, want %v", name, r, boom)
			}
		}()
		f()
	}
	mustPanic("newFn", func() { recycle.New(func() *item { panic(boom) }).Get() })

	keepP := recycle.New[*item](nil, recycle.WithKeep(func(*item) bool { panic(boom) }))
	mustPanic("keep", func() { keepP.Put(new(item)) })
	if got := keepP.Get(); got != nil {
		t.Errorf("keep panicked but a value was pooled: %p", got)
	}

	resetP := recycle.New[*item](nil, recycle.WithReset(func(*item) *item { panic(boom) }))
	mustPanic("reset", func() { resetP.Put(new(item)) })
	if got := resetP.Get(); got != nil {
		t.Errorf("reset panicked but a value was pooled: %p", got)
	}
}

// TestPool_concurrentGetPut: many goroutines Get and Put at once under -race; no value is ever held by two
// callers at the same time, every kept Put is reset, and newFn may be called concurrently (go_54's pool2
// counted New calls with a plain int, a data race; here newFn counts with an atomic).
func TestPool_concurrentGetPut(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	const workers, rounds = 16, 500
	var news, resets, doubles atomic.Int64
	p := recycle.New(
		func() *item { news.Add(1); return new(item) },
		recycle.WithReset(func(it *item) *item { resets.Add(1); it.dirty = 0; return it }),
	)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range rounds {
				if ctx.Err() != nil {
					return
				}
				it := p.Get()
				if !it.held.CompareAndSwap(false, true) {
					doubles.Add(1)
				}
				if it.dirty != 0 {
					doubles.Add(1) // a value came back without reset
				}
				it.dirty = w*rounds + i + 1
				it.held.Store(false)
				p.Put(it)
			}
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		t.Fatalf("test bound reached: %v", err)
	}
	if n := doubles.Load(); n != 0 {
		t.Fatalf("%d Gets returned a value held elsewhere or not reset", n)
	}
	if n := resets.Load(); n != workers*rounds {
		t.Fatalf("reset calls = %d, want %d", n, workers*rounds)
	}
	if n := news.Load(); n < 1 || n > workers*rounds {
		t.Fatalf("newFn calls = %d, want 1..%d", n, workers*rounds)
	}
}

// TestPool_go54_typedNoAssertion (go_54 defect: `pool.Get().([]byte)`, an interface{} payload and an
// assertion that panics on a wrong type): Get returns the type parameter, checked by the compiler.
func TestPool_go54_typedNoAssertion(t *testing.T) {
	p := recycle.New(func() []byte { return make([]byte, 1024) })
	var b []byte = p.Get() // compiles only because Get returns []byte
	if len(b) != 1024 {
		t.Fatalf("len = %d, want 1024", len(b))
	}
}

// TestPool_go54_sliceValuePutAllocates (go_54 defect: Put of a non-pointer []byte): boxing a slice header into
// the pool's interface allocates on every Put, so pooling []byte by value does not save allocations. The
// boxing happens before sync.Pool sees the value, so the count is at least 1 with and without -race.
func TestPool_go54_sliceValuePutAllocates(t *testing.T) {
	p := recycle.New(func() []byte { return make([]byte, 0, 64) })
	b := make([]byte, 0, 64)
	allocs := testing.AllocsPerRun(100, func() { p.Put(b) })
	if allocs < 1 {
		t.Fatalf("Put([]byte) allocs = %v, want >= 1 (the slice header is boxed)", allocs)
	}
}

// TestPool_go54_pointerGetPutNoAlloc: the fix for the defect above: pool a pointer (*[]byte). A Get/Put pair
// then allocates nothing in steady state. Asserted without -race only: under -race sync.Pool drops a random
// quarter of Puts on purpose, and the next Get allocates through newFn.
func TestPool_go54_pointerGetPutNoAlloc(t *testing.T) {
	if raceEnabled {
		t.Skip("sync.Pool drops Puts at random under -race; the zero-allocation count is not deterministic there")
	}
	p := recycle.New(func() *[]byte { b := make([]byte, 0, 64); return &b })
	allocs := testing.AllocsPerRun(100, func() {
		b := p.Get()
		*b = append((*b)[:0], 'x')
		p.Put(b)
	})
	if allocs != 0 {
		t.Fatalf("Get/Put of *[]byte allocs = %v, want 0", allocs)
	}
}

// BenchmarkPool_GetPut: a Get/Put pair of a *bytes.Buffer with reset, serial.
func BenchmarkPool_GetPut(b *testing.B) {
	p := recycle.New(
		func() *bytes.Buffer { return new(bytes.Buffer) },
		recycle.WithReset(func(buf *bytes.Buffer) *bytes.Buffer { buf.Reset(); return buf }),
	)
	b.ReportAllocs()
	for b.Loop() {
		buf := p.Get()
		buf.WriteByte('x')
		p.Put(buf)
	}
}

// BenchmarkPool_GetPutParallel: the same pair from GOMAXPROCS goroutines.
func BenchmarkPool_GetPutParallel(b *testing.B) {
	p := recycle.New(
		func() *bytes.Buffer { return new(bytes.Buffer) },
		recycle.WithReset(func(buf *bytes.Buffer) *bytes.Buffer { buf.Reset(); return buf }),
	)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := p.Get()
			buf.WriteByte('x')
			p.Put(buf)
		}
	})
}

// BenchmarkSyncPool_GetPut: the bare sync.Pool baseline with an assertion, to show the wrapper's cost.
func BenchmarkSyncPool_GetPut(b *testing.B) {
	p := sync.Pool{New: func() any { return new(bytes.Buffer) }}
	b.ReportAllocs()
	for b.Loop() {
		buf := p.Get().(*bytes.Buffer)
		buf.WriteByte('x')
		buf.Reset()
		p.Put(buf)
	}
}
