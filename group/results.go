package group

import (
	"context"
	"slices"
	"sync"
)

// Option changes how NewResults builds a Results. A nil Option is ignored.
//
// ID:          TB-GROUP-005
// Pattern:     functional options
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "func (*Group) SetLimit"
// Tags:        concurrency, options
type Option func(*config)

type config struct {
	limit   int  // < 0: no limit
	collect bool // WithCollect: Wait returns partial values and every error joined
}

// WithLimit sets the most goroutines a Results runs at once, as Group.SetLimit before the first Go. A negative
// n (the default, -1) means no limit; zero means no goroutine can start and every Go records ErrZeroLimit.
//
// ID:          TB-GROUP-006
// Pattern:     bounded parallelism (limit option)
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "func (*Group) SetLimit"
// Tags:        concurrency, limit, options
func WithLimit(n int) Option {
	return func(c *config) { c.limit = n }
}

// Results runs functions that each produce one value on their own goroutines and returns the values in the
// order Go was called. It is a Group underneath: the first error (or recovered panic, panics.ErrPanic) wins and
// cancels the context passed to every function, and Wait returns only after every goroutine has returned. A
// function whose goroutine starts after the context is done is not called; context.Cause(ctx) is recorded
// instead, so a cancelled run is never reported as a success. The zero value is ready to use, with
// context.Background() and no limit; NewResults passes a context and options. A Results must not be copied
// after first use and is meant for one task.
//
// ID:          TB-GROUP-003
// Pattern:     result group (fork-join with ordered results)
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "type Group", "func WithContext"
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4 "Error Handling"
// Goroutines:  one per Go (at most the limit at once); all have exited when Wait returns
// Cancel:      the ctx given to each function is cancelled on the first error, when the parent is, or when Wait returns
// Errors:      the first of: a function's error, panics.ErrPanic, ErrZeroLimit, the context's cause; every one joined with WithCollect
// Blocking:    Go blocks while the active count is at the limit; Wait blocks until all return
// Tags:        concurrency, errgroup, results, ordered, generic
type Results[T any] struct {
	g       Group
	collect bool // WithCollect; set by NewResults before the Results is shared, read-only after

	mu   sync.Mutex      // guards ctx and vals
	ctx  context.Context // derived from the parent; nil until the first Go of a zero Results
	vals []T             // one slot per Go, in submission order
}

// NewResults returns a Results whose functions receive a context derived from ctx (a nil ctx is
// context.Background()), configured by opts. The default is no limit.
//
// ID:          TB-GROUP-004
// Pattern:     result group (fork-join with ordered results)
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "func WithContext", "func (*Group) SetLimit"
// Goroutines:  none
// Cancel:      the derived ctx is done when ctx is, on the first error, or when Wait returns
// Tags:        concurrency, errgroup, results, generic
func NewResults[T any](ctx context.Context, opts ...Option) *Results[T] {
	c := config{limit: -1}
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	r := &Results[T]{collect: c.collect}
	r.bind(ctx)
	r.g.SetLimit(c.limit)
	r.g.SetCollect(c.collect)
	return r
}

// bind derives the functions' context from ctx and gives its cancel to the Group. r.mu must be held or r not
// yet shared.
func (r *Results[T]) bind(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	cctx, cancel := context.WithCancelCause(ctx)
	r.ctx = cctx
	r.g.mu.Lock()
	r.g.cancel = cancel
	r.g.mu.Unlock()
}

// Go reserves the next result slot and calls fn on a new goroutine with the Results' context. It blocks while
// the active count is at the limit. fn's value is kept only if fn returns a nil error. If the context is done
// when the goroutine starts, fn is not called and the context's cause is recorded. A nil fn is returned as
// panics.ErrPanic. As with Group, the first call to Go must happen before Wait.
func (r *Results[T]) Go(fn func(ctx context.Context) (T, error)) {
	r.mu.Lock()
	if r.ctx == nil {
		r.bind(context.Background())
	}
	ctx := r.ctx
	i := len(r.vals)
	var zero T
	r.vals = append(r.vals, zero)
	r.mu.Unlock()

	r.g.Go(func() error {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		v, err := fn(ctx)
		if err != nil {
			return err
		}
		r.mu.Lock()
		r.vals[i] = v
		r.mu.Unlock()
		return nil
	})
}

// Wait blocks until every function has returned and cancels the context. With no error it returns one value
// per Go, in the order Go was called (nil if Go was never called); otherwise it returns nil and the first error.
// With WithCollect it returns the values in submission order, the zero value in every failed slot, and every
// error joined with errors.Join.
func (r *Results[T]) Wait() ([]T, error) {
	err := r.g.Wait()
	if err != nil && !r.collect {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.vals), err
}
