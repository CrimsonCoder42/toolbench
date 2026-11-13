package bulkhead

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/semaphore"
)

// Bulkhead isolates one dependency: at most maxConcurrent calls of Do run at once, up to WithQueue more wait
// for a slot in first-in, first-out order (each for at most WithQueueTimeout), and any call beyond that gets
// ErrFull at once, without calling fn. A failing or slow dependency can then hold only its own slots, never the
// caller's other resources. Admission is two semaphore.Weighted (TB-SEMA-001): one of size
// maxConcurrent+queue that a call takes without blocking (TryAcquire, else ErrFull) and one of size
// maxConcurrent it waits on (Acquire, FIFO, cancellable). Safe for concurrent use. Create one with New; the
// zero value, and a nil *Bulkhead, reject every call with ErrFull. A Bulkhead must not be copied after first
// use.
//
// ID:          TB-BULK-001
// Pattern:     bulkhead (bounded concurrency, fail fast, bounded wait queue)
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns", "Bulkheads" (book, not opened)
// Authority:   Microsoft, "Bulkhead pattern", Azure Architecture Center, 2026, "Solution", "Problems and considerations" ("consider using processes, thread pools, and semaphores")
// Goroutines:  none; fn and the hook run on the caller's goroutine (a real-clock queue timeout fires on a runtime timer goroutine that only cancels the wait)
// Cancel:      ctx done before Do: context.Cause(ctx), fn not called; done while queued: the call leaves the queue holding nothing and returns context.Cause(ctx); during fn: fn sees ctx
// Errors:      ErrFull (no slot and no queue place), ErrQueueTimeout (also ErrFull), ctx cause, fn's error unchanged, *panics.PanicError for a panic in fn; a hook panic is joined
// Blocking:    Do waits only while queued (bounded by ctx and WithQueueTimeout) and while fn runs; with no queue it never waits for a slot
// Tags:        bulkhead, concurrency, limit, semaphore, load-shedding, resilience, queue
type Bulkhead struct {
	slots   *semaphore.Weighted // maxConcurrent permits: a holder is running fn
	room    *semaphore.Weighted // maxConcurrent+queue permits: a holder is running or queued
	maxConc int
	queue   int
	timeout time.Duration
	clk     clock.Clock
	onEvent func(Event)
}

// New returns a Bulkhead that runs at most maxConcurrent calls at once, configured by opts (nil options are
// ignored, later ones win). maxConcurrent <= 0 returns nil and an error wrapping ErrMaxConcurrent (D-023: a
// long-lived limit sized by accident is surfaced, not defaulted).
//
// ID:          TB-BULK-002
// Pattern:     bulkhead (constructor)
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Bulkheads" (book, not opened)
// Errors:      ErrMaxConcurrent (wrapped with the value) when maxConcurrent <= 0
// Tags:        bulkhead, constructor, options
func New(maxConcurrent int, opts ...Option) (*Bulkhead, error) {
	if maxConcurrent <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrMaxConcurrent, maxConcurrent)
	}
	var c config
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.clk == nil {
		c.clk = clock.Real()
	}
	// The room semaphore counts in int64; keep maxConcurrent+queue from overflowing it.
	c.queue = int(min(int64(c.queue), math.MaxInt64-int64(maxConcurrent)))
	return &Bulkhead{
		slots:   semaphore.New(int64(maxConcurrent)),
		room:    semaphore.New(int64(maxConcurrent) + int64(c.queue)),
		maxConc: maxConcurrent,
		queue:   c.queue,
		timeout: c.timeout,
		clk:     c.clk,
		onEvent: c.onEvent,
	}, nil
}

// Do runs fn on the caller's goroutine once a slot is free and returns fn's error unchanged. A free slot (and
// no one queued ahead) runs fn at once. When every slot is busy the call joins the FIFO queue if WithQueue has a
// free place, else returns ErrFull without calling fn. A queued call returns ErrQueueTimeout after
// WithQueueTimeout, or context.Cause(ctx) when ctx ends first; either way it leaves the queue holding nothing
// and fn is not called. A ctx already done returns context.Cause(ctx) at once, even if a slot is free. fn
// receives ctx itself (the queue timeout never reaches fn). A panic in fn is returned as *panics.PanicError
// (errors.Is panics.ErrPanic), as is a nil fn; the slot is given back however fn ends. A panic in the
// WithOnEvent hook is joined (errors.Join) to the result. A nil ctx is context.Background().
func (b *Bulkhead) Do(ctx context.Context, fn func(context.Context) error) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if b == nil || b.slots == nil {
		return ErrFull
	}
	var hookErr error
	emit := func(e Event) { hookErr = errors.Join(hookErr, b.emit(e)) }
	defer func() { err = joinHook(err, hookErr) }()

	if ctx.Err() != nil {
		emit(Event{Kind: EventCancelled})
		return context.Cause(ctx)
	}
	if !b.room.TryAcquire(1) {
		emit(Event{Kind: EventRejected})
		return ErrFull
	}
	waited, err := b.admit(ctx, emit)
	if err != nil {
		b.room.Release(1)
		return err
	}
	// Deferred so that the slot is given back even when fn calls runtime.Goexit (t.FailNow). The slot goes
	// back before the queue place, so a call holding a queue place always finds the slots it counts on.
	defer func() {
		b.slots.Release(1)
		b.room.Release(1)
		emit(Event{Kind: EventReleased})
	}()
	emit(Event{Kind: EventAdmitted, Waited: waited})
	return panics.Try(func() error { return fn(ctx) })
}

// admit takes one slot for a call that already holds a place in room: at once when one is free and no one is
// queued, else by waiting in the slot semaphore's FIFO queue, bounded by ctx and the queue timeout. On error the
// call holds no slot. emit runs on this goroutine.
func (b *Bulkhead) admit(ctx context.Context, emit func(Event)) (time.Duration, error) {
	if b.slots.TryAcquire(1) {
		return 0, nil
	}
	wctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if b.timeout > 0 {
		t := b.clk.AfterFunc(b.timeout, func() { cancel(ErrQueueTimeout) })
		defer t.Stop()
	}
	start := b.clk.Now()
	q := &queuedCtx{Context: wctx, onQueued: func() { emit(Event{Kind: EventQueued}) }}
	err := b.slots.Acquire(q, 1)
	waited := b.clk.Now().Sub(start)
	if err == nil {
		return waited, nil
	}
	if ctx.Err() != nil {
		emit(Event{Kind: EventCancelled, Waited: waited})
		return waited, context.Cause(ctx)
	}
	emit(Event{Kind: EventTimedOut, Waited: waited})
	return waited, ErrQueueTimeout
}

// queuedCtx is the context a waiting call hands to semaphore.Weighted.Acquire. Acquire reads Done only in its
// final select, after the waiter is linked into the FIFO queue and the lock is released (semaphore/weighted.go,
// Acquire), and never on its fast path; so the first Done call is the moment the call is queued, and the
// Queued event is emitted then, on the caller's goroutine, with no lock held. Emitting it before Acquire instead
// would let an observer start a later call that reaches the queue first. Done returns the same channel on every
// call, as the context.Context contract requires (D-036). TestDo_queuedEventMeansEnqueued pins this coupling in
// one direction only: it reliably catches Done never being called, but catches Done being called before the
// push only by chance (the hook runs on the waiter's goroutine, racing the test's TryAcquire(0)).
type queuedCtx struct {
	context.Context
	once     sync.Once
	onQueued func()
}

func (q *queuedCtx) Done() <-chan struct{} {
	q.once.Do(q.onQueued)
	return q.Context.Done()
}

// emit calls the hook, if any, and returns a *panics.PanicError if it panicked.
func (b *Bulkhead) emit(e Event) error {
	if b.onEvent == nil {
		return nil
	}
	return panics.Try(func() error { b.onEvent(e); return nil })
}

// joinHook adds hook panics to a call's result; a call whose hook did not panic keeps its error unchanged.
func joinHook(err, hookErr error) error {
	if hookErr == nil {
		return err
	}
	return errors.Join(err, hookErr)
}

// DoValue is Bulkhead.Do for a function that returns a value: fn's value and error, or T's zero value and the
// error when the call is rejected (ErrFull, ErrQueueTimeout, ctx cause) or fn panics.
//
// ID:          TB-BULK-003
// Pattern:     bulkhead
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Bulkheads" (book, not opened)
// Goroutines:  none; fn runs on the caller's goroutine
// Cancel:      as Bulkhead.Do
// Errors:      as Bulkhead.Do; a panic in fn gives T's zero value and *panics.PanicError
// Blocking:    as Bulkhead.Do
// Tags:        bulkhead, concurrency, limit, resilience, generic
func DoValue[T any](ctx context.Context, b *Bulkhead, fn func(context.Context) (T, error)) (T, error) {
	var v T
	err := b.Do(ctx, func(ctx context.Context) error {
		var err error
		v, err = fn(ctx)
		return err
	})
	return v, err
}
