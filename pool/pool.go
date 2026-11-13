package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/CrimsonCoder42/toolbench/group"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// workerKey is the context key under which every worker's ctx carries the identity of its pool (*poolID), so
// Submit can tell a call from inside one of its own workers (non-blocking) from a call from outside
// (blocking). An unexported key type, as the context documentation asks, so no other package can read or
// shadow it. (A ctxkey.Key[*poolID] was the first shape; go1.27.3 crashes with an internal compiler error
// compiling this package's test binary with that generic key instantiated on an unexported pointer type,
// align run 2026-11-13, so the plain key type is used here.)
type workerKey struct{}

// poolID is one pool's identity, compared by pointer. It has a byte so that two pools never share an address
// (zero-size allocations may).
type poolID struct{ _ byte }

// Pool is a worker pool: a fixed set of workers, started by New, competing for values on one bounded queue
// and calling fn on each. Submit adds a value and blocks while the queue is full; Close stops accepting,
// waits until every accepted value has been run (or the pool's ctx has ended) and returns the errors. Each
// accepted value is given to exactly one worker, at most once. A worker may feed its own pool: Submit called
// with the ctx fn received (or a child of it) never blocks and returns ErrQueueFull when there is no room,
// and TrySubmit is the same non-blocking form from anywhere. WithOnDone reports progress; WithMaxErrors
// bounds the errors kept. Create it with New; the zero value is not usable (Submit returns ErrClosed,
// TrySubmit false and ErrClosed, Close returns nil). A Pool must not be copied.
//
// ID:          TB-POOL-001
// Pattern:     worker pool (competing consumers, bounded parallelism)
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Bounded parallelism"
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Competing Consumers"
// Goroutines:  workers, started by New, run in a group.Group; all have exited when Close returns
// Cancel:      the pool's ctx (New's ctx, or the first error with WithFailFast) ends: workers stop after their current fn call (at most one queued value per worker may still start, with a done ctx); the rest of the queue is not run and Close reports how many
// Errors:      Close: every fn error and recovered panic joined (default) or the first one (WithFailFast), joined with a count of values left unrun that wraps context.Cause
// Blocking:    Submit blocks while every worker is busy and the queue is full, except from inside a worker of this pool (then ErrQueueFull); Close blocks until the workers have exited
// Tags:        concurrency, workers, pool, backpressure, queue, panic
type Pool[T any] struct {
	ctx      context.Context // the pool's ctx: New's ctx, cancelled on the first error with WithFailFast
	wctx     context.Context // ctx, carrying id under workerKey; what every worker passes to fn
	id       *poolID         // this pool's identity, compared by pointer in Submit
	fn       func(context.Context, T) error
	failFast bool
	onDone   func(done, total int) // progress hook (WithOnDone); nil = none
	maxErr   int                   // fn errors kept in collect mode (WithMaxErrors); 0 = all
	errFull  error                 // ErrQueueFull wrapped with the queue size, made once
	g        *group.Group          // runs the workers
	tasks    chan T                // the queue; closed by Close, under mu, after quit
	quit     chan struct{}         // closed first by Close: wakes every Submit blocked on a full queue

	done     atomic.Int64 // values whose fn has returned
	accepted atomic.Int64 // values accepted by Submit or TrySubmit (counted before the send, undone when it fails)

	mu     sync.RWMutex // Submit holds it for reading while it may send on tasks; Close takes it to close tasks
	closed bool         // guarded by mu

	errMu       sync.Mutex // guards errs, dropped, lastDropped
	errs        []error    // collected fn errors (default mode), in completion order, at most maxErr when set
	dropped     int        // fn errors not kept because of maxErr
	lastDropped error      // the last error not kept

	closeOnce sync.Once
	closeErr  error // set once by Close
}

// New starts workers goroutines that each take values from the pool's queue and call fn(ctx, v), where ctx is
// the pool's context: ctx itself, or with WithFailFast a child of it that is cancelled on the first error,
// carrying a private marker that lets Submit recognise a call from inside this pool (see Submit). A
// nil ctx is context.Background(). A nil fn panics on each value and so returns panics.ErrPanic through Close.
// A worker whose fn calls runtime.Goexit is replaced, so the pool keeps its worker count.
//
// ID:          TB-POOL-002
// Pattern:     worker pool (competing consumers, bounded parallelism)
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Bounded parallelism"
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Competing Consumers"
// Goroutines:  workers; each ends when the queue is closed and empty or the pool's ctx is done; all have exited when Close returns
// Cancel:      ctx done before New: the workers exit at once, Submit returns context.Cause(ctx)
// Errors:      ErrWorkers (wrapped with the count) when workers <= 0; no goroutine is started then
// Blocking:    never
// Tags:        concurrency, workers, pool, constructor
func New[T any](ctx context.Context, workers int, fn func(context.Context, T) error, opts ...Option) (*Pool[T], error) {
	if workers <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrWorkers, workers)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var c config
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	g, gctx := group.WithContext(ctx)
	id := &poolID{}
	p := &Pool[T]{
		ctx:      gctx,
		wctx:     context.WithValue(gctx, workerKey{}, id),
		id:       id,
		fn:       fn,
		failFast: c.failFast,
		onDone:   c.onDone,
		maxErr:   c.maxErrors,
		errFull:  fmt.Errorf("%w: queue size %d", ErrQueueFull, c.queue),
		g:        g,
		tasks:    make(chan T, c.queue),
		quit:     make(chan struct{}),
	}
	for range workers {
		g.Go(p.worker)
	}
	return p, nil
}

// Submit gives v to the pool. It returns nil once a worker or the queue has taken v; it blocks while every
// worker is busy and the queue is full (backpressure: nothing is dropped). It returns ErrClosed after Close has
// been called, context.Cause(ctx) when ctx is done, and context.Cause of the pool's ctx when the pool has
// stopped (New's ctx ended, or the first error with WithFailFast). A value for which Submit returned an error
// was not accepted. A nil ctx is context.Background(). Submit is safe for concurrent use, also with Close.
//
// From inside a worker of this pool, Submit does not block: called with the ctx fn received (or a child of it),
// it enqueues v when the queue has room (with no queue: when another worker is waiting) and otherwise returns
// ErrQueueFull, wrapped with the queue size; v was not accepted and is never run, so the caller handles the
// overflow (run it inline, keep it aside, or drop it knowingly). The two cases are told apart by the ctx: a
// worker that passes context.Background() (or any ctx not descended from its own) gets the blocking form, and
// then fn must not call Submit on its own pool without a queue to spare: with every worker inside fn it
// would wait until Close releases it with ErrClosed. TrySubmit is the non-blocking form from anywhere.
func (p *Pool[T]) Submit(ctx context.Context, v T) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if p == nil || p.tasks == nil {
		return ErrClosed
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch {
	case p.closed || isClosed(p.quit):
		return ErrClosed
	case ctx.Err() != nil:
		return context.Cause(ctx)
	case p.ctx.Err() != nil:
		return context.Cause(p.ctx)
	}
	if id, ok := ctx.Value(workerKey{}).(*poolID); ok && id == p.id {
		return p.trySend(v)
	}
	p.accepted.Add(1)
	select {
	case p.tasks <- v:
		return nil
	case <-p.quit:
		p.accepted.Add(-1)
		return ErrClosed
	case <-ctx.Done():
		p.accepted.Add(-1)
		return context.Cause(ctx)
	case <-p.ctx.Done():
		p.accepted.Add(-1)
		return context.Cause(p.ctx)
	}
}

// TrySubmit gives v to the pool without waiting: it returns (true, nil) once a worker or the queue has taken
// v, and (false, ErrQueueFull wrapped with the queue size) when there is no room (with no queue: no worker is
// waiting). It returns (false, ErrClosed) after Close has been called, for the zero value and for a nil *Pool,
// and (false, context.Cause of the pool's ctx) when the pool has stopped (New's ctx ended, or the first error
// with WithFailFast). A value for which ok is false was not accepted and is never run. TrySubmit is safe for
// concurrent use, also with Close, and may be called from inside fn.
//
// ID:          TB-POOL-009
// Pattern:     worker pool (non-blocking submit)
// Authority:   The Go Programming Language Specification, "Select statements" (a select with a default case never blocks), go1.27
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Competing Consumers"
// Goroutines:  none
// Cancel:      the pool's ctx done: returns (false, its cause)
// Errors:      ErrQueueFull (wrapped with the queue size) when full; ErrClosed after Close; the pool ctx's cause when it stopped
// Blocking:    never
// Tags:        concurrency, workers, pool, queue, non-blocking
func (p *Pool[T]) TrySubmit(v T) (ok bool, err error) {
	if p == nil || p.tasks == nil {
		return false, ErrClosed
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch {
	case p.closed || isClosed(p.quit):
		return false, ErrClosed
	case p.ctx.Err() != nil:
		return false, context.Cause(p.ctx)
	}
	if err := p.trySend(v); err != nil {
		return false, err
	}
	return true, nil
}

// trySend sends v without waiting; the caller holds mu for reading and has checked closed and the ctx. It
// returns errFull when nothing can take v now.
func (p *Pool[T]) trySend(v T) error {
	p.accepted.Add(1)
	select {
	case p.tasks <- v:
		return nil
	default:
		p.accepted.Add(-1)
		return p.errFull
	}
}

// Close stops the pool accepting values (a Submit blocked on a full queue returns ErrClosed), lets the workers
// run every value already accepted, waits until they have all exited and returns the errors. By default it
// returns every fn error and recovered panic joined with errors.Join (nil when there were none); with
// WithFailFast it returns the first. If the pool's ctx ended before the queue was empty, the values left
// unrun are reported as an error that wraps context.Cause of the pool's ctx, joined with the errors above (so
// errors.Is still matches the first error). With WithMaxErrors(n), when more than n errors occurred the kept
// ones are joined with one error "pool: N further errors not kept" that wraps the last dropped error. Close is
// idempotent: every call returns the first call's result. It must not be called from inside fn or the
// WithOnDone hook (it would wait for itself).
func (p *Pool[T]) Close() error {
	if p == nil || p.tasks == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		close(p.quit)
		p.mu.Lock() // waits for every Submit in flight; they return promptly because quit is closed
		p.closed = true
		close(p.tasks)
		p.mu.Unlock()

		first := p.g.Wait() // the workers have all exited
		var dropped error
		if n := len(p.tasks); n > 0 {
			for range p.tasks { // drain, so the values are not retained
			}
			dropped = fmt.Errorf("pool: %d submitted values not run: %w", n, context.Cause(p.ctx))
		}
		if p.failFast && first != nil {
			p.closeErr = first // the first error itself when nothing was left unrun
			if dropped != nil {
				p.closeErr = errors.Join(first, dropped)
			}
			return
		}
		p.errMu.Lock()
		var notKept error
		if p.dropped > 0 {
			notKept = fmt.Errorf("pool: %d further errors not kept: %w", p.dropped, p.lastDropped)
		}
		p.closeErr = errors.Join(append(p.errs, notKept, first, dropped)...)
		p.errs, p.lastDropped = nil, nil
		p.errMu.Unlock()
	})
	return p.closeErr
}

// worker is one competing consumer. It returns nil when the queue is closed and empty or the pool's ctx is
// done; with fail-fast it returns the first error, which the group records and uses to cancel the pool's ctx.
// If fn calls runtime.Goexit the loop does not return normally; the deferred call then starts a replacement
// worker (Go from inside a group goroutine is allowed, its count is still held).
func (p *Pool[T]) worker() error {
	normal := false
	defer func() {
		if !normal {
			p.g.Go(p.worker)
		}
	}()
	err := p.loop()
	normal = true
	return err
}

func (p *Pool[T]) loop() error {
	for {
		if p.ctx.Err() != nil {
			return nil
		}
		select {
		case <-p.ctx.Done():
			return nil
		case v, ok := <-p.tasks:
			if !ok {
				return nil
			}
			err := panics.Try(func() error { return p.fn(p.wctx, v) })
			done := p.done.Add(1)
			if p.onDone != nil {
				total := p.accepted.Load()
				if herr := panics.Try(func() error { p.onDone(int(done), int(total)); return nil }); herr != nil {
					err = errors.Join(err, herr) // nil err: Join returns herr alone (wrapped)
				}
			}
			if err == nil {
				continue
			}
			if p.failFast {
				return err
			}
			p.keep(err)
		}
	}
}

// keep records one fn error in collect mode: appended while fewer than maxErr are held (or always when maxErr
// is 0), otherwise counted and remembered as the last dropped one, for Close's summary.
func (p *Pool[T]) keep(err error) {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	if p.maxErr <= 0 || len(p.errs) < p.maxErr {
		p.errs = append(p.errs, err)
		return
	}
	p.dropped++
	p.lastDropped = err
}

// isClosed reports whether ch is closed, without blocking.
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
