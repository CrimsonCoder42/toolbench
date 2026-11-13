package singleflight

import (
	"context"
	"sync"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// Group is a namespace of keys in which a function call is suppressed while another call for the same key is
// in flight: the duplicate callers wait for the first call and receive the same result. The zero value is
// ready to use; a Group must not be copied after first use. A shared result is shared memory: every caller of
// one call receives the same V, which must not be mutated.
//
// ID:          TB-FLIGHT-001
// Pattern:     duplicate call suppression (singleflight)
// Authority:   pkg.go.dev golang.org/x/sync/singleflight v0.25.0, "Group", "Group.Do", "Group.DoChan", "Group.Forget"
// Goroutines:  one per call running fn (exited when its last caller returns), plus one per DoChan caller
// Cancel:      a caller whose ctx ends returns its cause; fn's ctx is cancelled when every caller has left
// Errors:      fn's error to every caller; a panic in fn is panics.ErrPanic, a Goexit in fn is ErrGoexit
// Blocking:    Do until fn returns or ctx ends; the last caller to leave waits for fn to honour its ctx
// Tags:        concurrency, singleflight, deduplication, stampede, generic
type Group[K comparable, V any] struct {
	mu sync.Mutex     // guards m and every call's waiters and finished fields
	m  map[K]*call[V] // the in-flight call per key; lazily made
}

// call is one in-flight or completed execution of fn for a key.
type call[V any] struct {
	done   chan struct{}           // closed once val and err are final
	cancel context.CancelCauseFunc // cancels fn's ctx

	// Written by the goroutine running fn before finished is set (under Group.mu) and before done is closed;
	// read only after either.
	val V
	err error

	// Guarded by Group.mu. joined only grows while the call is in the map, so it is final once done is closed.
	joined   int  // callers that ever joined this call
	waiters  int  // callers still waiting
	finished bool // fn has returned; val and err are final
}

// Do runs fn for key and returns its result, unless a call for key is already in flight: then Do waits for that
// call and returns its result. shared reports whether more than one caller joined the call (x/sync's rule).
// fn runs on its own goroutine with a ctx that carries the first caller's values but not its cancellation or
// deadline; that ctx is cancelled, with the last leaving caller's cause, only when every caller has left.
//
// If ctx is done before Do, fn is neither run nor joined and context.Cause(ctx) is returned. If ctx ends while
// Do waits, Do returns the cause at once and the call continues for the other callers; the last caller to
// leave cancels fn's ctx and waits for fn to return before it returns the cause, so the goroutine running fn
// never outlives every caller. A caller whose ctx ends after fn has returned gets the result. fn's error is
// returned unchanged; a panic in fn is a *panics.PanicError (panics.ErrPanic) to every caller; runtime.Goexit
// in fn is ErrGoexit to every caller. A nil ctx is context.Background(); a nil fn panics when called, so it
// returns panics.ErrPanic.
func (g *Group[K, V]) Do(ctx context.Context, key K, fn func(context.Context) (V, error)) (v V, shared bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		var zero V
		return zero, false, context.Cause(ctx)
	}
	c := g.join(ctx, key, fn)
	return g.wait(ctx, key, c)
}

// Forget tells the Group to forget key: the next Do or DoChan for key runs fn again instead of joining the call
// in flight. The call in flight is not cancelled; its callers still receive its result.
func (g *Group[K, V]) Forget(key K) {
	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
}

// join adds the caller to the call in flight for key, starting one if there is none.
func (g *Group[K, V]) join(ctx context.Context, key K, fn func(context.Context) (V, error)) *call[V] {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.m == nil {
		g.m = make(map[K]*call[V])
	}
	c, ok := g.m[key]
	if !ok {
		fctx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
		c = &call[V]{done: make(chan struct{}), cancel: cancel}
		g.m[key] = c
		go g.run(fctx, key, c, fn)
	}
	c.joined++
	c.waiters++
	return c
}

// run executes fn for c on its own goroutine. Panics go through panics.TryValue; a Goexit is told apart from a
// normal return by the flag, as x/sync's doCall does with its double defer.
func (g *Group[K, V]) run(ctx context.Context, key K, c *call[V], fn func(context.Context) (V, error)) {
	returned := false
	defer func() {
		if !returned { // fn called runtime.Goexit: TryValue did not return
			var zero V
			c.val, c.err = zero, ErrGoexit
		}
		g.mu.Lock()
		c.finished = true
		if g.m[key] == c {
			delete(g.m, key)
		}
		g.mu.Unlock()
		c.cancel(nil)
		close(c.done)
	}()
	c.val, c.err = panics.TryValue(func() (V, error) { return fn(ctx) })
	returned = true
}

// wait blocks until c completes or ctx ends, and returns what this caller gets.
func (g *Group[K, V]) wait(ctx context.Context, key K, c *call[V]) (V, bool, error) {
	select {
	case <-c.done:
		return c.val, c.joined > 1, c.err
	case <-ctx.Done():
	}
	cause := context.Cause(ctx)
	g.mu.Lock()
	if c.finished { // fn returned before this caller left: it gets the result
		g.mu.Unlock()
		<-c.done
		return c.val, c.joined > 1, c.err
	}
	c.waiters--
	last := c.waiters == 0
	if last && g.m[key] == c {
		delete(g.m, key) // a later caller starts a new call instead of joining a cancelled one
	}
	g.mu.Unlock()
	if last {
		c.cancel(cause)
		<-c.done
	}
	var zero V
	return zero, false, cause
}
