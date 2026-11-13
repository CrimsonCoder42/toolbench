package recycle

import "sync"

// Pool is a set of temporary values of type T that may be saved with Put and retrieved with Get, built on
// sync.Pool. Get returns a T directly (no type assertion). Put first asks keep (WithKeep) whether the value may
// be pooled and drops it if not, then applies reset (WithReset) and pools the result. Any pooled value may be
// removed automatically at any time without notification, so Get may call newFn even right after a Put.
// Safe for concurrent use. The zero value is ready to use: Get returns the zero T when the pool is empty and
// Put pools values unchanged; create one with New to set the constructor and options. A Pool must not be
// copied after first use (it holds a sync.Pool; vet copylocks reports a copy).
//
// ID:          TB-RECYC-001
// Pattern:     object pool (free list)
// Authority:   pkg.go.dev sync, "type Pool", "func (*Pool) Get", "func (*Pool) Put"
// Authority:   golang/go#23199, "sync: Pool example suggests incorrect usage", 2017 (oversized-buffer retention)
// Goroutines:  none
// Blocking:    never blocks; Get may call newFn and Put may call keep and reset on the caller's goroutine
// Tags:        pool, sync-pool, allocation, buffer, reuse
type Pool[T any] struct {
	p     sync.Pool
	newFn func() T
	reset func(T) T
	keep  func(T) bool
}

// New returns a Pool whose Get calls newFn when the pool has nothing to give. A nil newFn is allowed: Get then
// returns the zero T when the pool is empty (sync.Pool's Get returns nil in the same case). newFn, and the
// functions passed with WithReset and WithKeep, are called from any goroutine that calls Get or Put and must be
// safe for concurrent use. A nil Option is ignored.
//
// ID:          TB-RECYC-002
// Pattern:     object pool (constructor)
// Authority:   pkg.go.dev sync, "type Pool" (the New field)
// Tags:        pool, sync-pool, constructor
func New[T any](newFn func() T, opts ...Option[T]) *Pool[T] {
	p := &Pool[T]{newFn: newFn}
	var c config[T]
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	p.reset, p.keep = c.reset, c.keep
	return p
}

// Get removes an arbitrary value from the pool and returns it. When the pool is empty it returns newFn() or,
// with a nil newFn (or the zero Pool), the zero T. The caller owns the value until it Puts it back. A panic in
// newFn is not recovered: it reaches the caller of Get, as with sync.Pool.
func (p *Pool[T]) Get() T {
	if v, ok := p.p.Get().(T); ok {
		return v
	}
	if p.newFn != nil {
		return p.newFn()
	}
	var zero T
	return zero
}

// Put returns v to the pool. If keep is set and keep(v) is false, v is dropped (reset is not called) and left
// to the garbage collector. Otherwise reset, if set, is applied and its result is pooled. The caller must not
// use v after Put. A panic in keep or reset is not recovered: it reaches the caller of Put, and v is not
// pooled.
func (p *Pool[T]) Put(v T) {
	if p.keep != nil && !p.keep(v) {
		return
	}
	if p.reset != nil {
		v = p.reset(v)
	}
	p.p.Put(v)
}
