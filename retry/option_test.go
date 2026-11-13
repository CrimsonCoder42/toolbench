package retry_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/retry"
)

// printWaits runs a Do that always fails, with attempts attempts and the given options, and prints its waits.
func printWaits(attempts int, opts ...retry.Option) {
	var waits []time.Duration
	opts = append(opts, retry.WithClock(newInstant()), retry.WithAttempts(attempts),
		retry.WithOnRetry(func(_ int, _ error, w time.Duration) { waits = append(waits, w) }))
	_ = retry.Do(context.Background(), func(context.Context) error { return errFlaky }, opts...)
	fmt.Println(waits)
}

func ExampleOption() {
	opts := []retry.Option{
		retry.WithAttempts(4),
		retry.WithBackoff(retry.Exponential(time.Second, 4*time.Second)),
	}
	printWaits(4, opts...)
	// Output: [1s 2s 4s]
}

func ExampleWithAttempts() {
	calls := 0
	_ = retry.Do(context.Background(), func(context.Context) error { calls++; return errFlaky },
		retry.WithClock(newInstant()), retry.WithAttempts(5))
	fmt.Println(calls)
	// Output: 5
}

func ExampleWithBackoff() {
	printWaits(5, retry.WithBackoff(retry.Exponential(100*time.Millisecond, 500*time.Millisecond)))
	// Output: [100ms 200ms 400ms 500ms]
}

func ExampleBackoff() {
	var zero retry.Backoff // FullJitter(100ms, 10s)
	var waits []time.Duration
	_ = retry.Do(context.Background(), func(context.Context) error { return errFlaky },
		retry.WithClock(newInstant()), retry.WithBackoff(zero), retry.WithAttempts(4),
		retry.WithOnRetry(func(_ int, _ error, w time.Duration) { waits = append(waits, w) }))
	fmt.Println(waits[0] < 100*time.Millisecond, waits[1] < 200*time.Millisecond, waits[2] < 400*time.Millisecond)
	// Output: true true true
}

func ExampleExponential() {
	printWaits(6, retry.WithBackoff(retry.Exponential(time.Second, 10*time.Second)))
	// Output: [1s 2s 4s 8s 10s]
}

// jitteredInRange prints, for 200 retries of b, whether every wait was within [lo(k), hi(k)].
func jitteredInRange(b retry.Backoff, lo, hi func(k int, prev time.Duration) time.Duration) {
	ok := true
	k, prev := 0, time.Duration(0)
	_ = retry.Do(context.Background(), func(context.Context) error { return errFlaky },
		retry.WithClock(newInstant()), retry.WithBackoff(b), retry.WithAttempts(201),
		retry.WithRand(rand.New(rand.NewPCG(1, 2))),
		retry.WithOnRetry(func(_ int, _ error, w time.Duration) {
			if k == 0 {
				prev = time.Second
			}
			if w < lo(k, prev) || w > hi(k, prev) {
				ok = false
			}
			k, prev = k+1, w
		}))
	fmt.Println(ok)
}

func capped(k int) time.Duration { return min(time.Second<<min(k, 10), 8*time.Second) }

func ExampleFullJitter() {
	jitteredInRange(retry.FullJitter(time.Second, 8*time.Second),
		func(int, time.Duration) time.Duration { return 0 },
		func(k int, _ time.Duration) time.Duration { return capped(k) - 1 })
	// Output: true
}

func ExampleEqualJitter() {
	jitteredInRange(retry.EqualJitter(time.Second, 8*time.Second),
		func(k int, _ time.Duration) time.Duration { return capped(k) / 2 },
		func(k int, _ time.Duration) time.Duration { return capped(k) })
	// Output: true
}

func ExampleDecorrelatedJitter() {
	jitteredInRange(retry.DecorrelatedJitter(time.Second, 8*time.Second),
		func(int, time.Duration) time.Duration { return time.Second },
		func(_ int, prev time.Duration) time.Duration { return min(3*prev, 8*time.Second) })
	// Output: true
}

func ExamplePermanent() {
	errNotFound := errors.New("not found")
	calls := 0
	err := retry.Do(context.Background(), func(context.Context) error {
		calls++
		return retry.Permanent(errNotFound)
	}, retry.WithClock(newInstant()))
	fmt.Println(calls, errors.Is(err, errNotFound), err)
	// Output: 1 true not found
}

func ExampleWithRetryIf() {
	errBad := errors.New("400 bad request")
	calls := 0
	err := retry.Do(context.Background(), func(context.Context) error { calls++; return errBad },
		retry.WithClock(newInstant()),
		retry.WithRetryIf(func(err error) bool { return !errors.Is(err, errBad) }))
	fmt.Println(calls, err)
	// Output: 1 400 bad request
}

func ExampleWithOnRetry() {
	_ = retry.Do(context.Background(), func(context.Context) error { return errFlaky },
		retry.WithClock(newInstant()), retry.WithBackoff(retry.Exponential(time.Second, time.Minute)),
		retry.WithOnRetry(func(attempt int, err error, wait time.Duration) {
			fmt.Println(attempt, err, wait)
		}))
	// Output:
	// 1 flaky 1s
	// 2 flaky 2s
}

func ExampleWithClock() {
	c := newInstant()
	_ = retry.Do(context.Background(), func(context.Context) error { return errFlaky },
		retry.WithClock(c), retry.WithAttempts(4), retry.WithBackoff(retry.Exponential(time.Minute, time.Hour)))
	fmt.Println(c.elapsed())
	// Output: 7m0s
}

func ExampleWithRand() {
	run := func() []time.Duration {
		var waits []time.Duration
		_ = retry.Do(context.Background(), func(context.Context) error { return errFlaky },
			retry.WithClock(newInstant()), retry.WithAttempts(4), retry.WithRand(rand.New(rand.NewPCG(7, 7))),
			retry.WithOnRetry(func(_ int, _ error, w time.Duration) { waits = append(waits, w) }))
		return waits
	}
	fmt.Println(fmt.Sprint(run()) == fmt.Sprint(run()))
	// Output: true
}

func ExampleWithAttemptTimeout() {
	f := clock.NewFake(start)
	ctx := context.Background()
	errc := make(chan error, 1)
	go func() {
		errc <- retry.Do(ctx, func(ctx context.Context) error {
			<-ctx.Done() // a call that hangs until its attempt ctx ends
			return context.Cause(ctx)
		}, retry.WithClock(f), retry.WithAttempts(1), retry.WithAttemptTimeout(2*time.Second))
	}()
	_ = f.BlockUntil(ctx, 1) // the attempt's timeout timer is armed
	f.Advance(2 * time.Second)
	err := <-errc
	fmt.Println(errors.Is(err, retry.ErrAttemptTimeout))
	fmt.Println(err)
	// Output:
	// true
	// retry: attempts exhausted after 1 attempts: retry: attempt timed out after 2s
}

func ExampleErrAttemptTimeout() {
	f := clock.NewFake(start)
	ctx := context.Background()
	errc := make(chan error, 1)
	go func() {
		errc <- retry.Do(ctx, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
			retry.WithClock(f), retry.WithAttempts(1), retry.WithAttemptTimeout(time.Second))
	}()
	_ = f.BlockUntil(ctx, 1)
	f.Advance(time.Second)
	err := <-errc
	fmt.Println(errors.Is(err, retry.ErrAttemptTimeout), errors.Is(err, context.Canceled))
	// Output: true true
}

// The attempt timeout fires on the fake clock, fn sees ErrAttemptTimeout as its ctx's cause, the default
// WithRetryIf retries it, and the next attempt succeeds.
func TestWithAttemptTimeout_firesOnFakeClockAndRetries(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	f := clock.NewFake(start)
	type seen struct {
		attempt int
		err     error
		wait    time.Duration
	}
	retries := make(chan seen, 4)
	causes := make(chan error, 4)
	calls := 0
	errc := make(chan error, 1)
	go func() {
		errc <- retry.Do(bound, func(ctx context.Context) error {
			calls++
			// On a fake clock the attempt ctx adds no deadline of its own: it only inherits bound's.
			dl, ok := ctx.Deadline()
			if bdl, _ := bound.Deadline(); ok && !dl.Equal(bdl) {
				return retry.Permanent(errors.New("a fake-clock attempt ctx must carry no real deadline of its own"))
			}
			if calls == 1 {
				<-ctx.Done()
				causes <- context.Cause(ctx)
				return ctx.Err()
			}
			return nil
		}, retry.WithClock(f), retry.WithAttemptTimeout(5*time.Second),
			retry.WithBackoff(retry.Exponential(time.Second, time.Second)),
			retry.WithOnRetry(func(a int, err error, w time.Duration) { retries <- seen{a, err, w} }))
	}()
	if err := f.BlockUntil(bound, 1); err != nil { // the attempt's timeout
		t.Fatal(err)
	}
	if err := f.AdvanceContext(bound, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := f.BlockUntil(bound, 1); err != nil { // the 1 s wait
		t.Fatal(err)
	}
	if err := f.AdvanceContext(bound, time.Second); err != nil {
		t.Fatal(err)
	}
	var err error
	select {
	case err = <-errc:
	case <-bound.Done():
		t.Fatal("Do did not return")
	}
	if err != nil || calls != 2 {
		t.Fatalf("err %v calls %d", err, calls)
	}
	if c := <-causes; !errors.Is(c, retry.ErrAttemptTimeout) {
		t.Fatalf("cause %v", c)
	}
	r := <-retries
	if r.attempt != 1 || r.wait != time.Second || !errors.Is(r.err, retry.ErrAttemptTimeout) || !errors.Is(r.err, context.Canceled) {
		t.Fatalf("OnRetry saw %+v", r)
	}
	if !f.Now().Equal(start.Add(6 * time.Second)) {
		t.Fatalf("now %v", f.Now())
	}
}

// With clock.Real the attempt ctx is context.WithTimeoutCause: it carries a deadline. No wait happens.
func TestWithAttemptTimeout_realClockSetsDeadline(t *testing.T) {
	err := retry.Do(t.Context(), func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			return retry.Permanent(errors.New("no deadline"))
		}
		return nil
	}, retry.WithAttemptTimeout(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
}

// The outer ctx ending during an attempt wins over the attempt timeout: Do returns the outer cause.
func TestWithAttemptTimeout_outerCtxWins(t *testing.T) {
	outer := errors.New("outer")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	calls := 0
	err := retry.Do(ctx, func(ctx context.Context) error {
		calls++
		cancel(outer)
		<-ctx.Done()
		return ctx.Err()
	}, retry.WithClock(clock.NewFake(start)), retry.WithAttemptTimeout(time.Second))
	if calls != 1 || !errors.Is(err, outer) || errors.Is(err, retry.ErrAttemptTimeout) {
		t.Fatalf("calls %d err %v", calls, err)
	}
}

// d <= 0 is no per-attempt timeout: fn's ctx is the caller's.
func TestWithAttemptTimeout_nonPositiveIsNone(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		err := retry.Do(t.Context(), func(ctx context.Context) error {
			if ctx != t.Context() {
				return retry.Permanent(errors.New("ctx was wrapped"))
			}
			return nil
		}, retry.WithClock(clock.NewFake(start)), retry.WithAttemptTimeout(d))
		if err != nil {
			t.Fatalf("d %v: %v", d, err)
		}
	}
}

func TestWithOnRetry_seesAttemptErrWait(t *testing.T) {
	e1, e2, e3 := errors.New("e1"), errors.New("e2"), errors.New("e3")
	fn, _ := counter(e1, e2, e3)
	var got []string
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(3),
		retry.WithBackoff(retry.Exponential(100*time.Millisecond, time.Second)),
		retry.WithOnRetry(func(a int, err error, w time.Duration) { got = append(got, fmt.Sprint(a, err, w)) }))
	if want := "[1 e1 100ms 2 e2 200ms]"; fmt.Sprint(got) != want || !errors.Is(err, e3) {
		t.Fatalf("got %v err %v, want %s", got, err, want)
	}
}

func TestWithOnRetry_panicIsErrPanic(t *testing.T) {
	fn, calls := counter(errFlaky, errFlaky)
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()),
		retry.WithOnRetry(func(int, error, time.Duration) { panic("hook") }))
	if *calls != 1 || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

func TestWithRetryIf_panicIsErrPanic(t *testing.T) {
	fn, calls := counter(errFlaky, errFlaky)
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()),
		retry.WithRetryIf(func(error) bool { panic("predicate") }))
	if *calls != 1 || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

// WithRetryIf is not consulted for a Permanent error, and can retry what the default would not.
func TestWithRetryIf_notCalledForPermanent(t *testing.T) {
	asked := 0
	fn, calls := counter(context.Canceled, retry.Permanent(errFlaky))
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()),
		retry.WithRetryIf(func(error) bool { asked++; return true }))
	if *calls != 2 || asked != 1 || !errors.Is(err, errFlaky) {
		t.Fatalf("calls %d asked %d err %v", *calls, asked, err)
	}
}

func TestPermanent(t *testing.T) {
	if retry.Permanent(nil) != nil {
		t.Fatal("Permanent(nil) != nil")
	}
	p := retry.Permanent(errFlaky)
	if !errors.Is(p, errFlaky) || p.Error() != errFlaky.Error() || errors.Unwrap(p) != errFlaky {
		t.Fatalf("Permanent(errFlaky) = %v", p)
	}
}

func ExampleWithMaxElapsed() {
	c := newInstant() // waits pass at once and move the fake clock
	calls := 0
	err := retry.Do(context.Background(), func(context.Context) error { calls++; return errFlaky },
		retry.WithClock(c), retry.WithAttempts(10),
		retry.WithBackoff(retry.Exponential(time.Second, time.Second)), // 1 s between attempts
		retry.WithMaxElapsed(2500*time.Millisecond))                    // the third wait would end at 3 s
	fmt.Println(calls, c.elapsed(), errors.Is(err, retry.ErrExhausted))
	fmt.Println(err)
	// Output:
	// 3 2s true
	// retry: attempts exhausted: 2s elapsed of the 2.5s cap after 3 attempts: flaky
}

// The cap stops Do BEFORE a wait that would end past it: attempts at 0, 1 s and 2 s; the wait to 3 s is not started,
// WithOnRetry is not called for it, and the error wraps the last attempt's error.
func TestWithMaxElapsed_stopsBeforeAWaitThatWouldPass(t *testing.T) {
	c := newInstant()
	fn, calls := counter(errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky)
	retries := 0
	err := retry.Do(t.Context(), fn, retry.WithClock(c), retry.WithAttempts(10),
		retry.WithBackoff(retry.Exponential(time.Second, time.Second)), retry.WithMaxElapsed(2500*time.Millisecond),
		retry.WithOnRetry(func(int, error, time.Duration) { retries++ }))
	if *calls != 3 || retries != 2 || c.elapsed() != 2*time.Second {
		t.Fatalf("calls %d retries %d elapsed %v", *calls, retries, c.elapsed())
	}
	if !errors.Is(err, retry.ErrExhausted) || !errors.Is(err, errFlaky) {
		t.Fatalf("err %v", err)
	}
	if want := "retry: attempts exhausted: 2s elapsed of the 2.5s cap after 3 attempts: flaky"; err.Error() != want {
		t.Fatalf("err %q, want %q", err, want)
	}
}

// An attempt that runs past the cap is never cut (that is the ctx deadline's job): its result is kept. A failure past
// the cap is ErrExhausted wrapping that failure with no further attempt; a success past the cap is a success.
func TestWithMaxElapsed_attemptInFlightNotCut(t *testing.T) {
	const limit = 5 * time.Second
	t.Run("failure", func(t *testing.T) {
		c := newInstant()
		calls := 0
		err := retry.Do(t.Context(), func(ctx context.Context) error {
			calls++
			c.Advance(10 * time.Second) // fn takes 10 s, twice the cap
			if ctx.Err() != nil {
				return retry.Permanent(errors.New("fn was cut"))
			}
			return errFlaky
		}, retry.WithClock(c), retry.WithAttempts(10), retry.WithMaxElapsed(limit),
			retry.WithBackoff(retry.Exponential(time.Millisecond, time.Millisecond)))
		if calls != 1 || !errors.Is(err, retry.ErrExhausted) || !errors.Is(err, errFlaky) || c.elapsed() != 10*time.Second {
			t.Fatalf("calls %d err %v elapsed %v", calls, err, c.elapsed())
		}
		if want := "retry: attempts exhausted: 10s elapsed of the 5s cap after 1 attempts: flaky"; err.Error() != want {
			t.Fatalf("err %q, want %q", err, want)
		}
	})
	t.Run("success", func(t *testing.T) {
		c := newInstant()
		calls := 0
		err := retry.Do(t.Context(), func(context.Context) error {
			calls++
			c.Advance(10 * time.Second)
			return nil
		}, retry.WithClock(c), retry.WithMaxElapsed(limit))
		if err != nil || calls != 1 {
			t.Fatalf("calls %d err %v", calls, err)
		}
	})
}

// d <= 0 is no cap: WithAttempts alone bounds the call (five attempts, four one-hour waits).
func TestWithMaxElapsed_zeroIsNoCap(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		c := newInstant()
		fn, calls := counter(errFlaky, errFlaky, errFlaky, errFlaky, errFlaky)
		err := retry.Do(t.Context(), fn, retry.WithClock(c), retry.WithAttempts(5), retry.WithMaxElapsed(d),
			retry.WithBackoff(retry.Exponential(time.Hour, time.Hour)))
		if *calls != 5 || c.elapsed() != 4*time.Hour || !errors.Is(err, retry.ErrExhausted) {
			t.Fatalf("d %v: calls %d elapsed %v err %v", d, *calls, c.elapsed(), err)
		}
		if want := "retry: attempts exhausted after 5 attempts: flaky"; err.Error() != want {
			t.Fatalf("d %v: err %q, want %q", d, err, want)
		}
	}
}

// Whichever bound is reached first stops the call: attempts first keeps the attempts message; the cap first keeps
// the elapsed message.
func TestWithMaxElapsed_attemptsBoundWinsWhenEarlier(t *testing.T) {
	c := newInstant()
	fn, calls := counter(errFlaky, errFlaky, errFlaky, errFlaky)
	err := retry.Do(t.Context(), fn, retry.WithClock(c), retry.WithAttempts(2), retry.WithMaxElapsed(time.Hour),
		retry.WithBackoff(retry.Exponential(time.Second, time.Second)))
	if *calls != 2 || c.elapsed() != time.Second || err.Error() != "retry: attempts exhausted after 2 attempts: flaky" {
		t.Fatalf("attempts first: calls %d elapsed %v err %v", *calls, c.elapsed(), err)
	}
	c = newInstant()
	fn, calls = counter(errFlaky, errFlaky, errFlaky, errFlaky)
	err = retry.Do(t.Context(), fn, retry.WithClock(c), retry.WithAttempts(10), retry.WithMaxElapsed(1500*time.Millisecond),
		retry.WithBackoff(retry.Exponential(time.Second, time.Second)))
	if *calls != 2 || c.elapsed() != time.Second || err.Error() != "retry: attempts exhausted: 1s elapsed of the 1.5s cap after 2 attempts: flaky" {
		t.Fatalf("cap first: calls %d elapsed %v err %v", *calls, c.elapsed(), err)
	}
}

// ctx cancelled during a wait that the cap allowed: the ctx cause wins, not ErrExhausted, and Do returns at once.
func TestWithMaxElapsed_cancelDuringWaitWins(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	f := clock.NewFake(start)
	cause := errors.New("shutdown")
	ctx, cancel := context.WithCancelCause(bound)
	defer cancel(nil)
	fn, calls := counter(errFlaky, errFlaky)
	errc := make(chan error, 1)
	go func() {
		errc <- retry.Do(ctx, fn, retry.WithClock(f), retry.WithMaxElapsed(time.Hour),
			retry.WithBackoff(retry.Exponential(time.Minute, time.Minute)))
	}()
	if err := f.BlockUntil(bound, 1); err != nil { // Do is waiting on its one-minute timer
		t.Fatal(err)
	}
	cancel(cause)
	var err error
	select {
	case err = <-errc:
	case <-bound.Done():
		t.Fatal("Do did not return after cancel")
	}
	if !errors.Is(err, cause) || errors.Is(err, retry.ErrExhausted) || *calls != 1 || !f.Now().Equal(start) {
		t.Fatalf("err %v calls %d now %v", err, *calls, f.Now())
	}
}

// statusError is a typed error a client wraps with %w, as kubernetes-assistant-go's request error is.
type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("status %d", e.code) }

// kubernetes-assistant-go cmd/cli/completion.go:117-131 classifies with errors.As on a wrapped typed error and
// retries only HTTP 429. Ours: an allow-list WithRetryIf with errors.AsType (go1.26) sees the typed error through
// the %w wrap, retries the 429s and returns the 500 unchanged at once, not as ErrExhausted.
func TestWithRetryIf_allowListTypedError(t *testing.T) {
	e429a := fmt.Errorf("chat completion: %w", &statusError{429})
	e429b := fmt.Errorf("chat completion: %w", &statusError{429})
	e500 := fmt.Errorf("chat completion: %w", &statusError{500})
	fn, calls := counter(e429a, e429b, e500, e429a)
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(10),
		retry.WithRetryIf(func(err error) bool {
			se, ok := errors.AsType[*statusError](err)
			return ok && se.code == 429
		}))
	if *calls != 3 || err != e500 || errors.Is(err, retry.ErrExhausted) {
		t.Fatalf("calls %d err %v", *calls, err)
	}
	// an untyped error is not on the allow-list: one call, returned unchanged
	fn, calls = counter(errFlaky, errFlaky)
	err = retry.Do(t.Context(), fn, retry.WithClock(newInstant()),
		retry.WithRetryIf(func(err error) bool { _, ok := errors.AsType[*statusError](err); return ok }))
	if *calls != 1 || err != errFlaky {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}
