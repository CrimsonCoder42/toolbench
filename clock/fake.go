package clock

import (
	"context"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"time"
)

// Fake is a Clock for tests whose time moves only when Advance is called. Advance fires every timer and ticker
// that is due in deadline order (ties in arming order), with Now reading each deadline as it fires; nothing
// fires outside Advance, so a timer armed with d <= 0 fires on the next Advance, Advance(0) included.
// BlockUntil lets a test wait until the code under test has armed its timers. Timers and tickers follow the
// go1.23 rules: Stop and Reset drain an unreceived value and report the timer active, so no stale value is
// received after they return. One difference is visible: len and cap of a fake timer channel are up to 1,
// not 0. Safe for concurrent use. The zero value is ready to use and starts at the zero time.Time; NewFake
// sets the start. A Fake must not be copied after first use.
//
// Advance waits for each AfterFunc function before firing the next timer, so it never returns while a
// function that blocks runs. Two such cases deadlock Advance: a function that waits on something the caller
// of Advance provides only after Advance returns (a send on an unbuffered channel the caller reads later), and
// a function that waits on a fake timer due later in the same window. AdvanceContext bounds the wait with ctx.
//
// ID:          TB-CLOCK-005
// Pattern:     test double (fake object)
// Authority:   Fowler, "TestDouble", martinfowler.com, 2006 (after Meszaros, xUnit Test Patterns, 2007)
// Authority:   pkg.go.dev time, "func (*Timer) Stop", "func (*Timer) Reset", "func AfterFunc"
// Authority:   Go 1.23 Release Notes, go.dev, 2024, "Timer changes"
// Goroutines:  Advance starts one per AfterFunc function it fires and waits for it before firing the next
// Cancel:      AdvanceContext and BlockUntil return context.Cause(ctx) when ctx is done first
// Errors:      a panic in an AfterFunc function is recovered with its stack and re-panicked in Advance
// Blocking:    Advance blocks while AfterFunc functions run, one Advance at a time; BlockUntil waits on a channel
// Tags:        time, clock, fake, testing, timer, ticker
type Fake struct {
	// advancing is a one-slot semaphore (made lazily under mu) held by the running AdvanceContext; pending is
	// the done channel of an AfterFunc function a cancelled AdvanceContext left running, guarded by advancing.
	advancing chan struct{}
	pending   chan *callbackPanic

	mu      sync.Mutex // guards every field below and every fakeTimer field
	now     time.Time
	armed   []*fakeTimer // the timers and tickers that will fire, in no order
	seq     uint64       // arming order, the tie-break between equal deadlines
	waiters []waiter     // BlockUntil calls not yet satisfied
}

// NewFake returns a fake clock whose Now is start until Advance is called.
func NewFake(start time.Time) *Fake { return &Fake{now: start} }

type waiter struct {
	n     int
	ready chan struct{} // closed by notify when len(armed) >= n
}

type timerKind int

const (
	chanTimer timerKind = iota
	funcTimer
	tickerTimer
)

// fakeTimer is a timer, an AfterFunc timer or a ticker of one Fake; its fields are guarded by f.mu.
type fakeTimer struct {
	f      *Fake
	kind   timerKind
	ch     chan time.Time // one slot, drained by Stop and Reset; nil for funcTimer
	fn     func()         // funcTimer only; may be nil (does nothing)
	period time.Duration  // tickerTimer only
	when   time.Time
	seq    uint64
	armed  bool
}

// Now returns the fake's current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// NewTimer returns a timer that sends its deadline, Now()+d, on C when Advance reaches it.
func (f *Fake) NewTimer(d time.Duration) Timer {
	t := &fakeTimer{f: f, kind: chanTimer, ch: make(chan time.Time, 1)}
	f.mu.Lock()
	f.arm(t, d)
	f.mu.Unlock()
	return t
}

// AfterFunc returns a timer that, when Advance reaches Now()+d, calls fn on a new goroutine and waits for it
// to return. C of the returned timer is nil; a nil fn does nothing.
func (f *Fake) AfterFunc(d time.Duration, fn func()) Timer {
	t := &fakeTimer{f: f, kind: funcTimer, fn: fn}
	f.mu.Lock()
	f.arm(t, d)
	f.mu.Unlock()
	return t
}

// NewTicker returns a ticker that sends each tick's deadline on C every d of fake time. A d <= 0 returns a
// stopped ticker that sends nothing until Reset with a positive period.
func (f *Fake) NewTicker(d time.Duration) Ticker {
	t := &fakeTimer{f: f, kind: tickerTimer, ch: make(chan time.Time, 1), period: d}
	if d > 0 {
		f.mu.Lock()
		f.arm(t, d)
		f.mu.Unlock()
	}
	return fakeTicker{t}
}

// Advance is AdvanceContext with context.Background(): it moves time forward by d (a negative d counts as 0)
// and fires, in deadline order, every timer and ticker due by then, including those armed by AfterFunc
// functions during the call. Each AfterFunc function runs on its own goroutine and has returned before the
// next timer fires and before Advance returns. It never returns, so the test hangs, if a function blocks on
// something the caller provides after Advance returns, or on a fake timer due later in the same window, or
// calls Advance itself; use AdvanceContext where that can happen. A panic in a function is re-panicked here.
func (f *Fake) Advance(d time.Duration) {
	_ = f.AdvanceContext(context.Background(), d) // Background is never done: the error is always nil
}

// AdvanceContext is Advance bounded by ctx (a nil ctx is context.Background()). It returns nil when every
// timer due by Now()+d has fired, or context.Cause(ctx) when ctx is done first: before it started (time does
// not move), while another AdvanceContext runs (time does not move), or while an AfterFunc function is still
// running. In the last case the clock is left at that function's deadline, the rest of the window is not
// advanced and later due timers have not fired; the function keeps running, and the next AdvanceContext first
// waits for it (and re-panics its panic) before moving time again.
//
// A panic in an AfterFunc function is re-panicked on this goroutine with an error whose message holds the
// panic value and the function's stack (errors.Unwrap gives the value when it is an error). The clock is then
// left at that function's deadline, later due timers have not fired, and it remains usable.
func (f *Fake) AdvanceContext(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	f.mu.Lock()
	if f.advancing == nil {
		f.advancing = make(chan struct{}, 1)
	}
	sem := f.advancing
	f.mu.Unlock()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	defer func() { <-sem }()

	if f.pending != nil {
		if err := f.await(ctx, f.pending); err != nil {
			return err
		}
	}
	d = max(d, 0)
	f.mu.Lock()
	target := f.now.Add(d)
	for {
		t := f.due(target)
		if t == nil {
			break
		}
		if t.when.After(f.now) {
			f.now = t.when
		}
		switch t.kind {
		case funcTimer:
			f.disarm(t)
			fn := t.fn
			f.mu.Unlock()
			if fn != nil {
				if err := f.await(ctx, start(fn)); err != nil {
					return err
				}
			}
			f.mu.Lock()
		case tickerTimer:
			select {
			case t.ch <- t.when:
				t.when = t.when.Add(t.period)
			default:
				// The slot is full: like time.Ticker, drop every tick due by target, in one step.
				skip := target.Sub(t.when) / t.period
				t.when = t.when.Add((skip + 1) * t.period)
			}
			f.seq++
			t.seq = f.seq
		default:
			f.disarm(t)
			select {
			case t.ch <- t.when:
			default: // unreachable: Stop and Reset drain the slot before the timer is armed again
			}
		}
	}
	f.now = target
	f.mu.Unlock()
	return nil
}

// await waits for an AfterFunc function's goroutine. When ctx is done first it records done in f.pending and
// returns the cause; a recovered panic is re-panicked. Called holding f.advancing, not f.mu.
func (f *Fake) await(ctx context.Context, done chan *callbackPanic) error {
	select {
	case p := <-done:
		f.pending = nil
		if p != nil {
			panic(p)
		}
		return nil
	case <-ctx.Done():
		f.pending = done
		return context.Cause(ctx)
	}
}

// BlockUntil returns nil once at least n timers and tickers are armed (created or reset, not yet fired or
// stopped), or context.Cause(ctx) if ctx is done first. It waits on a channel the fake closes when the count
// reaches n; it never polls. An n <= 0 returns nil at once; a nil ctx is context.Background().
func (f *Fake) BlockUntil(ctx context.Context, n int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	f.mu.Lock()
	if len(f.armed) >= n {
		f.mu.Unlock()
		return nil
	}
	w := waiter{n: n, ready: make(chan struct{})}
	f.waiters = append(f.waiters, w)
	f.mu.Unlock()

	select {
	case <-w.ready:
		return nil
	case <-ctx.Done():
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.waiters, func(x waiter) bool { return x.ready == w.ready })
	if i < 0 {
		return nil // satisfied while ctx was being cancelled: the count was reached
	}
	f.waiters = slices.Delete(f.waiters, i, i+1)
	return context.Cause(ctx)
}

// due returns the armed timer with the earliest deadline not after target (ties: lowest seq), or nil.
// Called with f.mu held.
func (f *Fake) due(target time.Time) *fakeTimer {
	var best *fakeTimer
	for _, t := range f.armed {
		if t.when.After(target) {
			continue
		}
		if best == nil || t.when.Before(best.when) || (t.when.Equal(best.when) && t.seq < best.seq) {
			best = t
		}
	}
	return best
}

// arm schedules t to fire at Now()+d (Now() when d <= 0), last among equal deadlines. Called with f.mu held.
func (f *Fake) arm(t *fakeTimer, d time.Duration) {
	t.when = f.now.Add(max(d, 0))
	f.seq++
	t.seq = f.seq
	if !t.armed {
		t.armed = true
		f.armed = append(f.armed, t)
		f.notify()
	}
}

// disarm removes t from the armed list and reports whether it was there. Called with f.mu held.
func (f *Fake) disarm(t *fakeTimer) bool {
	if !t.armed {
		return false
	}
	t.armed = false
	if i := slices.Index(f.armed, t); i >= 0 {
		f.armed = slices.Delete(f.armed, i, i+1)
	}
	return true
}

// notify releases every BlockUntil waiter whose count is reached. Called with f.mu held.
func (f *Fake) notify() {
	n := len(f.armed)
	f.waiters = slices.DeleteFunc(f.waiters, func(w waiter) bool {
		if n >= w.n {
			close(w.ready)
			return true
		}
		return false
	})
}

// callbackPanic is the value Advance re-panics with when an AfterFunc function panicked: the panic value and
// the stack of the function's goroutine, taken where it was recovered.
type callbackPanic struct {
	value any
	stack []byte
}

func (p *callbackPanic) Error() string {
	return fmt.Sprintf("clock: AfterFunc function panicked: %v\n\n%s", p.value, p.stack)
}

// Unwrap returns the panic value when it is an error.
func (p *callbackPanic) Unwrap() error {
	if err, ok := p.value.(error); ok {
		return err
	}
	return nil
}

var _ error = (*callbackPanic)(nil)

// start calls fn on a new goroutine and returns the channel that receives, once, nil when fn returns or calls
// runtime.Goexit (t.FailNow), or the recovered panic with its stack.
func start(fn func()) chan *callbackPanic {
	done := make(chan *callbackPanic, 1)
	go func() {
		normal := false
		defer func() {
			if normal {
				done <- nil
				return
			}
			if r := recover(); r != nil {
				done <- &callbackPanic{value: r, stack: debug.Stack()}
				return
			}
			done <- nil // runtime.Goexit
		}()
		fn()
		normal = true
	}()
	return done
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch } // nil for an AfterFunc timer

// drain empties t's slot and reports whether it held a value. Called with f.mu held.
func (t *fakeTimer) drain() bool {
	select {
	case <-t.ch: // a nil ch (funcTimer) is never ready
		return true
	default:
		return false
	}
}

// Stop disarms the timer and drains an unreceived value; it reports true if either happened, as the go1.23
// runtime does for *time.Timer.
func (t *fakeTimer) Stop() bool {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	active := t.f.disarm(t)
	if t.drain() {
		active = true
	}
	return active
}

// Reset is Stop followed by arming the timer at Now()+d; it satisfies both Timer.Reset and Ticker.Reset.
// For a ticker, d <= 0 leaves it stopped.
func (t *fakeTimer) Reset(d time.Duration) bool {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	active := t.f.disarm(t)
	if t.drain() {
		active = true
	}
	if t.kind == tickerTimer {
		t.period = d
		if d <= 0 {
			return active
		}
	}
	t.f.arm(t, d)
	return active
}

// fakeTicker adapts a ticker's fakeTimer to Ticker, whose Stop and Reset return nothing.
type fakeTicker struct{ t *fakeTimer }

func (k fakeTicker) C() <-chan time.Time   { return k.t.ch }
func (k fakeTicker) Stop()                 { k.t.Stop() }
func (k fakeTicker) Reset(d time.Duration) { k.t.Reset(d) }
