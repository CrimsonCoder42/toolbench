package breaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// Breaker is a circuit breaker with the states Closed, Open and HalfOpen. Do runs a call through it. The zero
// value is ready to use with the defaults (threshold 5, OpenFor 60 s, 1 probe, clock.Real, no hook); create a
// configured one with New. A Breaker is safe for concurrent use and must not be copied after first use. A nil
// *Breaker is always closed: Do runs fn (panics recovered) and counts nothing.
//
// ID:          TB-BRK-001
// Pattern:     circuit breaker
// Authority:   Nygard, Release It!, 2nd ed., Pragmatic Bookshelf, 2018, ch.5 "Stability Patterns", "Circuit Breaker" (book, not opened)
// Authority:   Fowler, "CircuitBreaker", martinfowler.com, 2014
// Authority:   Microsoft, "Circuit Breaker pattern", Azure Architecture Center, "Solution"
// Goroutines:  none; fn and the hook run on the caller's goroutine
// Tags:        breaker, circuit-breaker, resilience, fail-fast, concurrency
type Breaker struct {
	// Set by New before first use, read-only afterwards; zero means the default.
	threshold int
	openFor   time.Duration
	probes    int
	clock     clock.Clock
	onChange  func(from, to State)

	mu        sync.Mutex // guards every field below
	state     State
	gen       uint64       // bumped on every transition; a result from an older generation is ignored
	failures  int          // consecutive failures in Closed
	openedAt  time.Time    // when the breaker last opened
	inflight  int          // probes running in HalfOpen
	successes int          // probes that succeeded in HalfOpen
	pending   []transition // transitions not yet given to the hook (only when onChange != nil)
	notifying bool         // a goroutine is delivering pending to the hook
}

type transition struct{ from, to State }

// maxPending bounds the transitions queued for a hook that has not returned yet. Beyond it a transition is not
// queued: the hook sees the gap as a from that differs from the previous to.
const maxPending = 64

// New returns a Breaker configured by opts, starting Closed. Options with invalid values fall back to the
// documented defaults; nil options are ignored.
//
// ID:          TB-BRK-002
// Pattern:     circuit breaker (constructor with functional options)
// Authority:   Fowler, "CircuitBreaker", martinfowler.com, 2014
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Goroutines:  none
// Tags:        breaker, constructor, options
func New(opts ...Option) *Breaker {
	b := &Breaker{}
	for _, o := range opts {
		if o != nil {
			o(b)
		}
	}
	return b
}

func (b *Breaker) cfgThreshold() int {
	if b.threshold <= 0 {
		return defaultThreshold
	}
	return b.threshold
}

func (b *Breaker) cfgOpenFor() time.Duration {
	if b.openFor <= 0 {
		return defaultOpenFor
	}
	return b.openFor
}

func (b *Breaker) cfgProbes() int {
	if b.probes <= 0 {
		return defaultProbes
	}
	return b.probes
}

func (b *Breaker) now() time.Time {
	if b.clock == nil {
		return time.Now()
	}
	return b.clock.Now()
}

// Do runs fn through the breaker, on the caller's goroutine, and returns fn's error unchanged. Closed: fn runs
// and its result is counted. Open: Do returns ErrOpen without calling fn until OpenFor has passed; then the call
// becomes a probe (HalfOpen). HalfOpen: fn runs if fewer than the probe limit are running or have succeeded,
// else ErrOpen. A non-nil error from fn is a failure; a panic in fn is a failure returned as *panics.PanicError
// (errors.Is panics.ErrPanic). A ctx already done returns context.Cause(ctx) without calling fn; an error from fn
// that matches ctx.Err() or context.Cause(ctx) once ctx is done is the caller's own cancellation and is not
// counted. A panic in the WithOnStateChange hook run by this call is joined (errors.Join) to the result. Do
// blocks only while fn and the hook run, never waiting for the breaker. A nil ctx is context.Background().
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if b == nil {
		return panics.Try(func() error { return fn(ctx) })
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	gen, err := b.admit()
	hookErr := b.notify()
	if err != nil {
		return joinHook(err, hookErr)
	}
	err = panics.Try(func() error { return fn(ctx) })
	b.record(gen, classify(ctx, err))
	if h := b.notify(); hookErr == nil {
		hookErr = h
	}
	return joinHook(err, hookErr)
}

// DoValue is Breaker.Do for a function that returns a value: it returns fn's value and error, or T's zero
// value and ErrOpen when the breaker rejects the call. A nil b runs fn and counts nothing.
//
// ID:          TB-BRK-003
// Pattern:     circuit breaker
// Authority:   Fowler, "CircuitBreaker", martinfowler.com, 2014
// Goroutines:  none; fn runs on the caller's goroutine
// Cancel:      as Breaker.Do
// Errors:      as Breaker.Do; a panic in fn gives T's zero value and *panics.PanicError
// Blocking:    for as long as fn runs
// Tags:        breaker, circuit-breaker, resilience, generic
func DoValue[T any](ctx context.Context, b *Breaker, fn func(context.Context) (T, error)) (T, error) {
	var v T
	err := b.Do(ctx, func(ctx context.Context) error {
		var err error
		v, err = fn(ctx)
		return err
	})
	return v, err
}

// State returns the breaker's state now. An Open breaker whose OpenFor has passed reports HalfOpen; the
// transition itself (and its hook call) happens on the next Do. A nil *Breaker reports Closed.
func (b *Breaker) State() State {
	if b == nil {
		return Closed
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == Open && b.openForPassed() {
		return HalfOpen
	}
	return b.state
}

type outcome int

const (
	success outcome = iota
	failure
	ignored // the caller's own cancellation
)

func classify(ctx context.Context, err error) outcome {
	switch {
	case err == nil:
		return success
	case ctx.Err() != nil && (errors.Is(err, ctx.Err()) || errors.Is(err, context.Cause(ctx))):
		return ignored
	}
	return failure
}

// openForPassed reports whether OpenFor has passed since the breaker opened. b.mu is held.
func (b *Breaker) openForPassed() bool {
	return !b.now().Before(b.openedAt.Add(b.cfgOpenFor()))
}

// admit decides whether a call may run and returns the generation it runs in.
func (b *Breaker) admit() (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == Open && b.openForPassed() {
		b.setState(HalfOpen)
	}
	switch b.state {
	case Closed:
		return b.gen, nil
	case HalfOpen:
		if b.inflight+b.successes >= b.cfgProbes() {
			return 0, ErrOpen
		}
		b.inflight++
		return b.gen, nil
	}
	return 0, ErrOpen
}

// record counts the outcome of a call admitted in generation gen; a stale generation is ignored.
func (b *Breaker) record(gen uint64, o outcome) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if gen != b.gen {
		return
	}
	switch b.state {
	case Closed:
		switch o {
		case success:
			b.failures = 0
		case failure:
			b.failures++
			if b.failures >= b.cfgThreshold() {
				b.setState(Open)
			}
		}
	case HalfOpen:
		b.inflight--
		switch o {
		case success:
			b.successes++
			if b.successes >= b.cfgProbes() {
				b.setState(Closed)
			}
		case failure:
			b.setState(Open)
		}
	}
}

// setState moves to a new state, resets the counters and queues the transition for the hook. b.mu is held.
func (b *Breaker) setState(to State) {
	from := b.state
	b.state = to
	b.gen++
	b.failures, b.inflight, b.successes = 0, 0, 0
	if to == Open {
		b.openedAt = b.now()
	}
	if b.onChange != nil && len(b.pending) < maxPending {
		b.pending = append(b.pending, transition{from, to})
	}
}

// notify gives the pending transitions to the hook, in order, one call at a time, outside the lock. If another
// goroutine is already delivering, it returns at once: that goroutine delivers these too. It returns the first
// hook panic as a *panics.PanicError.
func (b *Breaker) notify() error {
	b.mu.Lock()
	if b.notifying || len(b.pending) == 0 {
		b.mu.Unlock()
		return nil
	}
	b.notifying = true
	var first error
	for len(b.pending) > 0 {
		t := b.pending[0]
		b.pending = b.pending[1:]
		if len(b.pending) == 0 {
			b.pending = nil // release the backing array
		}
		b.mu.Unlock()
		if err := panics.Try(func() error { b.onChange(t.from, t.to); return nil }); err != nil && first == nil {
			first = err
		}
		b.mu.Lock()
	}
	b.notifying = false
	b.mu.Unlock()
	return first
}

func joinHook(err, hookErr error) error {
	if hookErr == nil {
		return err
	}
	return errors.Join(err, hookErr)
}
