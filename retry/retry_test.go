package retry_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/backoff"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/retry"
)

var errFlaky = errors.New("flaky")

func ExampleDo() {
	c := newInstant() // a fake clock whose waits pass at once
	calls := 0
	err := retry.Do(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return errFlaky
		}
		return nil
	}, retry.WithClock(c), retry.WithBackoff(retry.Exponential(100*time.Millisecond, time.Second)))
	fmt.Println(calls, err, c.elapsed())
	// Output: 3 <nil> 300ms
}

func ExampleDoValue() {
	calls := 0
	v, err := retry.DoValue(context.Background(), func(context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "", errFlaky
		}
		return "ok", nil
	}, retry.WithClock(newInstant()))
	fmt.Println(v, err, calls)
	// Output: ok <nil> 2
}

func ExampleErrExhausted() {
	err := retry.Do(context.Background(), func(context.Context) error { return errFlaky },
		retry.WithClock(newInstant()), retry.WithAttempts(2))
	fmt.Println(errors.Is(err, retry.ErrExhausted), errors.Is(err, errFlaky))
	fmt.Println(err)
	// Output:
	// true true
	// retry: attempts exhausted after 2 attempts: flaky
}

func TestDo_successFirstAttempt(t *testing.T) {
	c := newInstant()
	fn, calls := counter()
	if err := retry.Do(t.Context(), fn, retry.WithClock(c)); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || c.elapsed() != 0 {
		t.Fatalf("calls %d elapsed %v, want 1 and 0", *calls, c.elapsed())
	}
}

func TestDo_attemptsCount(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5, 10} {
		fn, calls := counter(errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky, errFlaky)
		err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(n))
		if *calls != n || !errors.Is(err, retry.ErrExhausted) || !errors.Is(err, errFlaky) {
			t.Fatalf("attempts %d: calls %d err %v", n, *calls, err)
		}
	}
}

func TestDo_defaultAttemptsIsThree(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		fn, calls := counter(errFlaky, errFlaky, errFlaky, errFlaky)
		opts := []retry.Option{retry.WithClock(newInstant())}
		if n != 0 {
			opts = append(opts, retry.WithAttempts(n))
		}
		_ = retry.Do(t.Context(), fn, opts...)
		if *calls != 3 {
			t.Fatalf("WithAttempts(%d): calls %d, want 3", n, *calls)
		}
	}
}

func TestDo_successAfterRetries(t *testing.T) {
	fn, calls := counter(errFlaky, errFlaky)
	if err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(5)); err != nil {
		t.Fatal(err)
	}
	if *calls != 3 {
		t.Fatalf("calls %d, want 3", *calls)
	}
}

// ErrExhausted wraps the LAST attempt's error, not the first.
func TestDo_exhaustedWrapsLastError(t *testing.T) {
	first, last := errors.New("first"), errors.New("last")
	fn, _ := counter(first, last)
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(2))
	if !errors.Is(err, retry.ErrExhausted) || !errors.Is(err, last) || errors.Is(err, first) {
		t.Fatalf("err = %v", err)
	}
}

func TestDo_permanentStopsAtOnce(t *testing.T) {
	cause := errors.New("bad input")
	fn, calls := counter(errFlaky, retry.Permanent(cause), errFlaky)
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(10))
	if *calls != 2 || !errors.Is(err, cause) || errors.Is(err, retry.ErrExhausted) || err.Error() != "bad input" {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

// go_68_asynq marks a non-retryable error by wrapping asynq.SkipRetry with %w and the cause with %v: the cause
// is lost to errors.Is. Ours: Permanent wrapped inside another error still stops the retry, and the cause stays
// visible to errors.Is and errors.As.
func TestDo_go68SkipRetryWrappedKeepsCause(t *testing.T) {
	cause := &net{msg: "unmarshal"}
	fn, calls := counter(fmt.Errorf("decode payload: %w", retry.Permanent(cause)))
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()))
	var got *net
	if *calls != 1 || !errors.As(err, &got) || got != cause {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

type net struct{ msg string }

func (n *net) Error() string { return n.msg }

// go_37_background_work sets MaxFails: 1, so a job is never retried and the knob is hidden in a struct.
// Ours: WithAttempts(1) is exactly one call and the error says so.
func TestDo_go37MaxFailsOneCallsOnce(t *testing.T) {
	fn, calls := counter(errFlaky, errFlaky)
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(1))
	if *calls != 1 || !errors.Is(err, retry.ErrExhausted) {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

var errBadRequest = errors.New("status 400")

// go_78_resty treats only transport errors as failures and ignores the HTTP status. Ours: the caller classifies
// every error with WithRetryIf, and a non-retryable one stops at once, returned unchanged.
func TestDo_go78NonRetryableStopsAtOnce(t *testing.T) {
	fn, calls := counter(errBadRequest, errFlaky)
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(5),
		retry.WithRetryIf(func(err error) bool { return !errors.Is(err, errBadRequest) }))
	if *calls != 1 || err != errBadRequest {
		t.Fatalf("calls %d err %v", *calls, err)
	}
	// and a status the caller classifies as retryable is retried
	fn, calls = counter(errFlaky, errFlaky)
	if err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithAttempts(5)); err != nil || *calls != 3 {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

func TestDo_defaultSkipsContextErrors(t *testing.T) {
	for _, e := range []error{context.Canceled, context.DeadlineExceeded, fmt.Errorf("dial: %w", context.DeadlineExceeded)} {
		fn, calls := counter(e, e)
		err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()))
		if *calls != 1 || err != e {
			t.Fatalf("%v: calls %d err %v", e, *calls, err)
		}
	}
}

func TestDo_panicIsErrPanicNotRetried(t *testing.T) {
	calls := 0
	err := retry.Do(t.Context(), func(context.Context) error { calls++; panic("boom") }, retry.WithClock(newInstant()))
	if calls != 1 || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("calls %d err %v", calls, err)
	}
}

func TestDo_panicRetriedWhenRetryIfSaysSo(t *testing.T) {
	calls := 0
	err := retry.Do(t.Context(), func(context.Context) error { calls++; panic("boom") },
		retry.WithClock(newInstant()), retry.WithRetryIf(func(error) bool { return true }))
	if calls != 3 || !errors.Is(err, retry.ErrExhausted) || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("calls %d err %v", calls, err)
	}
}

func TestDo_nilFnIsErrPanic(t *testing.T) {
	if err := retry.Do(t.Context(), nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("err %v", err)
	}
	if _, err := retry.DoValue[int](t.Context(), nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("err %v", err)
	}
}

func TestDo_nilCtx(t *testing.T) {
	fn, calls := counter(errFlaky)
	//lint:ignore SA1012 a nil ctx is documented as context.Background()
	if err := retry.Do(nil, fn, retry.WithClock(newInstant())); err != nil || *calls != 2 { //nolint:staticcheck
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

func TestDo_nilOptionsIgnored(t *testing.T) {
	fn, calls := counter(errFlaky)
	if err := retry.Do(t.Context(), fn, nil, retry.WithClock(newInstant()), nil, retry.WithRetryIf(nil), retry.WithRand(nil)); err != nil || *calls != 2 {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

func TestDo_cancelBeforeStart(t *testing.T) {
	cause := errors.New("shutdown")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	fn, calls := counter()
	if err := retry.Do(ctx, fn, retry.WithClock(newInstant())); *calls != 0 || !errors.Is(err, cause) {
		t.Fatalf("calls %d err %v", *calls, err)
	}
}

// ctx cancelled while Do waits: Do returns at once with the cause (and the last error), the fake time has not
// moved, and fn is not called again.
func TestDo_cancelDuringWait(t *testing.T) {
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
		errc <- retry.Do(ctx, fn, retry.WithClock(f), retry.WithBackoff(retry.Exponential(time.Hour, time.Hour)))
	}()
	if err := f.BlockUntil(bound, 1); err != nil { // Do is waiting on its one-hour timer
		t.Fatal(err)
	}
	cancel(cause)
	var err error
	select {
	case err = <-errc:
	case <-bound.Done():
		t.Fatal("Do did not return after cancel")
	}
	if !errors.Is(err, cause) || !errors.Is(err, errFlaky) || *calls != 1 || !f.Now().Equal(start) {
		t.Fatalf("err %v calls %d now %v", err, *calls, f.Now())
	}
}

// ctx cancelled during fn: fn sees it, Do returns the cause and makes no further attempt.
func TestDo_cancelDuringAttempt(t *testing.T) {
	cause := errors.New("shutdown")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	calls := 0
	err := retry.Do(ctx, func(ctx context.Context) error {
		calls++
		cancel(cause)
		return ctx.Err()
	}, retry.WithClock(newInstant()))
	if calls != 1 || !errors.Is(err, cause) {
		t.Fatalf("calls %d err %v", calls, err)
	}
}

func TestDo_cancelAfterCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	fn, _ := counter()
	err := retry.Do(ctx, fn, retry.WithClock(newInstant()))
	cancel()
	if err != nil {
		t.Fatal(err)
	}
}

// The waits are exactly the calcbench results for the same seed, and the fake clock moved by their sum.
func TestDo_waitsEqualCalcbench(t *testing.T) {
	const n = 8
	base, maxD := 100*time.Millisecond, 3*time.Second
	cases := []struct {
		name string
		b    retry.Backoff
		want func(k int, prev time.Duration, r *rand.Rand) (time.Duration, error)
	}{
		{"Exponential", retry.Exponential(base, maxD), func(k int, _ time.Duration, _ *rand.Rand) (time.Duration, error) {
			return backoff.Exponential(k, base, maxD)
		}},
		{"FullJitter", retry.FullJitter(base, maxD), func(k int, _ time.Duration, r *rand.Rand) (time.Duration, error) {
			return backoff.FullJitter(k, base, maxD, r.Float64())
		}},
		{"EqualJitter", retry.EqualJitter(base, maxD), func(k int, _ time.Duration, r *rand.Rand) (time.Duration, error) {
			return backoff.EqualJitter(k, base, maxD, r.Float64())
		}},
		{"DecorrelatedJitter", retry.DecorrelatedJitter(base, maxD), func(_ int, prev time.Duration, r *rand.Rand) (time.Duration, error) {
			return backoff.DecorrelatedJitter(prev, base, maxD, r.Float64())
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newInstant()
			var got []time.Duration
			err := retry.Do(t.Context(), func(context.Context) error { return errFlaky },
				retry.WithClock(c), retry.WithAttempts(n), retry.WithBackoff(tc.b),
				retry.WithRand(rand.New(rand.NewPCG(1, 2))),
				retry.WithOnRetry(func(_ int, _ error, wait time.Duration) { got = append(got, wait) }))
			if !errors.Is(err, retry.ErrExhausted) {
				t.Fatal(err)
			}
			twin := rand.New(rand.NewPCG(1, 2))
			prev, sum := base, time.Duration(0)
			for k := range n - 1 {
				w, err := tc.want(k, prev, twin)
				if err != nil {
					t.Fatal(err)
				}
				if got[k] != w {
					t.Fatalf("wait %d = %v, want %v", k, got[k], w)
				}
				prev, sum = w, sum+w
			}
			if len(got) != n-1 || c.elapsed() != sum {
				t.Fatalf("waits %d elapsed %v, want %d and %v", len(got), c.elapsed(), n-1, sum)
			}
		})
	}
}

// The zero Backoff, and WithBackoff not passed, is FullJitter(100ms, 10s).
func TestDo_defaultBackoff(t *testing.T) {
	waits := func(opts ...retry.Option) []time.Duration {
		var got []time.Duration
		opts = append(opts, retry.WithClock(newInstant()), retry.WithAttempts(6), retry.WithRand(rand.New(rand.NewPCG(3, 4))),
			retry.WithOnRetry(func(_ int, _ error, w time.Duration) { got = append(got, w) }))
		_ = retry.Do(t.Context(), func(context.Context) error { return errFlaky }, opts...)
		return got
	}
	want := waits(retry.WithBackoff(retry.FullJitter(100*time.Millisecond, 10*time.Second)))
	for name, got := range map[string][]time.Duration{
		"none":     waits(),
		"zero":     waits(retry.WithBackoff(retry.Backoff{})),
		"invalid":  waits(retry.WithBackoff(retry.FullJitter(-1, 0))),
		"baseZero": waits(retry.WithBackoff(retry.FullJitter(0, 10*time.Second))),
	} {
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
}

// Exponential(d, d) is a constant d; a max below base is base.
func TestBackoff_constantAndMaxBelowBase(t *testing.T) {
	for _, b := range []retry.Backoff{retry.Exponential(time.Second, time.Second), retry.Exponential(time.Second, time.Millisecond)} {
		c := newInstant()
		var got []time.Duration
		_ = retry.Do(t.Context(), func(context.Context) error { return errFlaky }, retry.WithClock(c), retry.WithAttempts(5),
			retry.WithBackoff(b), retry.WithOnRetry(func(_ int, _ error, w time.Duration) { got = append(got, w) }))
		if fmt.Sprint(got) != "[1s 1s 1s 1s]" || c.elapsed() != 4*time.Second {
			t.Fatalf("waits %v elapsed %v", got, c.elapsed())
		}
	}
}

// A max <= 0 is the larger of 10 s and base.
func TestBackoff_maxDefault(t *testing.T) {
	for _, tc := range []struct {
		b    retry.Backoff
		want string
	}{
		{retry.Exponential(4*time.Second, 0), "[4s 8s 10s 10s]"},
		{retry.Exponential(20*time.Second, -1), "[20s 20s 20s 20s]"},
	} {
		var got []time.Duration
		_ = retry.Do(t.Context(), func(context.Context) error { return errFlaky }, retry.WithClock(newInstant()), retry.WithAttempts(5),
			retry.WithBackoff(tc.b), retry.WithOnRetry(func(_ int, _ error, w time.Duration) { got = append(got, w) }))
		if fmt.Sprint(got) != tc.want {
			t.Fatalf("waits %v, want %s", got, tc.want)
		}
	}
}

func TestDoValue_zeroValueOnError(t *testing.T) {
	calls := 0
	v, err := retry.DoValue(t.Context(), func(context.Context) (int, error) { calls++; return 42, errFlaky },
		retry.WithClock(newInstant()), retry.WithAttempts(2))
	if v != 0 || !errors.Is(err, retry.ErrExhausted) || calls != 2 {
		t.Fatalf("v %d err %v calls %d", v, err, calls)
	}
}

func TestDoValue_valueOfSuccessfulAttempt(t *testing.T) {
	calls := 0
	v, err := retry.DoValue(t.Context(), func(context.Context) (int, error) {
		calls++
		if calls < 3 {
			return -1, errFlaky
		}
		return calls * 10, nil
	}, retry.WithClock(newInstant()))
	if v != 30 || err != nil {
		t.Fatalf("v %d err %v", v, err)
	}
}

func BenchmarkDo_success(b *testing.B) {
	b.ReportAllocs()
	ctx := context.Background()
	fn := func(context.Context) error { return nil }
	for b.Loop() {
		_ = retry.Do(ctx, fn)
	}
}

// At process start: wait for a dependency (a database, a broker) with every bound set, then exit non-zero if it
// never came up. A deterministic stub stands in for the dial: it fails twice, then connects.
func ExampleDo_startupDependency() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // the overall deadline
	defer cancel()
	dials := 0
	dial := func(context.Context) error {
		dials++
		if dials < 3 {
			return errors.New("connection refused")
		}
		return nil
	}
	err := retry.Do(ctx, dial,
		retry.WithAttempts(5),                // at most five dials
		retry.WithMaxElapsed(10*time.Second), // and no wait that would pass 10 s
		retry.WithBackoff(retry.Exponential(500*time.Millisecond, 2*time.Second)),
		retry.WithClock(newInstant())) // the Example's clock; main uses the default clock.Real()
	if err != nil {
		// The dependency did not come up inside the bounds: in main, log the error and os.Exit(1) so the
		// orchestrator restarts the process (Twelve-Factor IX). Never loop forever here.
		fmt.Println("fatal:", err)
		return
	}
	fmt.Println("connected after", dials, "dials")
	// Output: connected after 3 dials
}

// go-openai-discord-bot pkg/commands/gpt/message_handler.go:68-82 retries with no delay: a hot loop on a failing
// API. Ours: a zero or invalid Backoff is normalised, so the waits are never zero by configuration
// (Exponential(0, 0) is Exponential(100ms, 10s); the zero Backoff is FullJitter(100ms, 10s)), and the default
// attempts (3) bound the loop.
func TestDo_zeroWaitBackoffStillBounded(t *testing.T) {
	c := newInstant()
	var waits []time.Duration
	fn, calls := counter(errFlaky, errFlaky, errFlaky, errFlaky)
	_ = retry.Do(t.Context(), fn, retry.WithClock(c), retry.WithAttempts(4), retry.WithBackoff(retry.Exponential(0, 0)),
		retry.WithOnRetry(func(_ int, _ error, w time.Duration) { waits = append(waits, w) }))
	if *calls != 4 || fmt.Sprint(waits) != "[100ms 200ms 400ms]" || c.elapsed() != 700*time.Millisecond {
		t.Fatalf("calls %d waits %v elapsed %v", *calls, waits, c.elapsed())
	}
	waits = nil
	fn, calls = counter(errFlaky, errFlaky, errFlaky, errFlaky)
	err := retry.Do(t.Context(), fn, retry.WithClock(newInstant()), retry.WithBackoff(retry.Backoff{}),
		retry.WithRand(rand.New(rand.NewPCG(5, 6))),
		retry.WithOnRetry(func(_ int, _ error, w time.Duration) { waits = append(waits, w) }))
	if *calls != 3 || len(waits) != 2 || !errors.Is(err, retry.ErrExhausted) {
		t.Fatalf("calls %d waits %v err %v", *calls, waits, err)
	}
	for k, w := range waits {
		if w < 0 || w >= 100*time.Millisecond<<k {
			t.Fatalf("wait %d = %v, want in [0, %v)", k, w, 100*time.Millisecond<<k)
		}
	}
}

// kubernetes-assistant-go cmd/cli/completion.go:101 doubles from 1 s with no cap. Ours: the cap holds for every
// doubling, including the shifts past 63 bits that calcbench saturates (ALG-BACKOF-001): 70 attempts of
// Exponential(1s, 10s) wait 1+2+4+8 s then 10 s sixty-five times, 665 s in all; FullJitter never exceeds the cap.
func TestBackoff_capHoldsAfter60Doublings(t *testing.T) {
	const attempts = 70
	c := newInstant()
	var waits []time.Duration
	err := retry.Do(t.Context(), func(context.Context) error { return errFlaky },
		retry.WithClock(c), retry.WithAttempts(attempts), retry.WithBackoff(retry.Exponential(time.Second, 10*time.Second)),
		retry.WithOnRetry(func(_ int, _ error, w time.Duration) { waits = append(waits, w) }))
	if !errors.Is(err, retry.ErrExhausted) || len(waits) != attempts-1 || c.elapsed() != 665*time.Second {
		t.Fatalf("err %v waits %d elapsed %v", err, len(waits), c.elapsed())
	}
	for k, w := range waits {
		want := min(time.Second<<min(k, 4), 10*time.Second)
		if w != want {
			t.Fatalf("wait %d = %v, want %v", k, w, want)
		}
	}
	for _, b := range []retry.Backoff{retry.FullJitter(time.Second, 10*time.Second), retry.EqualJitter(time.Second, 10*time.Second), retry.DecorrelatedJitter(time.Second, 10*time.Second)} {
		_ = retry.Do(t.Context(), func(context.Context) error { return errFlaky },
			retry.WithClock(newInstant()), retry.WithAttempts(attempts), retry.WithBackoff(b),
			retry.WithRand(rand.New(rand.NewPCG(9, 9))),
			retry.WithOnRetry(func(k int, _ error, w time.Duration) {
				if w > 10*time.Second {
					t.Errorf("%v: wait %d = %v exceeds the 10s cap", b, k, w)
				}
			}))
	}
}

// BenchmarkDo_success (above) is the success path with the defaults; this is the same path with WithMaxElapsed
// set, so the cost of the cap is one extra clock read.
func BenchmarkDo_withMaxElapsed(b *testing.B) {
	b.ReportAllocs()
	ctx := context.Background()
	fn := func(context.Context) error { return nil }
	opts := []retry.Option{retry.WithMaxElapsed(time.Minute)}
	for b.Loop() {
		_ = retry.Do(ctx, fn, opts...)
	}
}
