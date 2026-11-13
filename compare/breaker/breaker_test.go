// Package breaker_test compares toolbench/breaker.Breaker with github.com/sony/gobreaker/v2 (v2.4.0, as
// pinned in compare/go.mod; generic CircuitBreaker[T]): the same script of call results gives the same state after every call,
// the same rejections, and the same sequence of state changes.
//
// Settings that make gobreaker match the toolbench defaults' shape: ReadyToTrip is ConsecutiveFailures >=
// threshold (gobreaker's default is > 5), Interval 0 (closed counts are never cleared by time), MaxRequests =
// the half-open probe count, Timeout = OpenFor.
//
// Known differences, by design:
//   - gobreaker reads time.Now and has no clock seam, so its open timeout cannot be moved by hand. The script
//     uses Timeout = OpenFor = 1 s and, to leave Open, waits on a real timer of 1 s + 1 ms started after the call that
//     opened the breaker; toolbench advances a clock.Fake by exactly OpenFor. This wall-time wait is accepted
//     here only (compare/ is its own module). Both directions:
//   - leaving Open is deterministic: gobreaker sets its expiry (now + Timeout) before the timer starts, a
//     timer fires after at least its duration, and gobreaker leaves Open only when expiry is strictly Before
//     now, which the extra 1 ms guarantees, so the call after the wait always sees half-open;
//   - staying Open assumes less than 1 s of wall time passes between the call that (re)opens the breaker
//     and the next checks in the script (a stall of >= 1 s under -race would show half-open early).
//   - gobreaker's State() performs the open -> half-open transition (and calls OnStateChange); toolbench's
//     State() only reports HalfOpen, and the transition and hook happen on the next Do. The script calls Do or
//     Execute after each wait, so the transition sequences agree.
//   - gobreaker rejects with ErrOpenState (open) or ErrTooManyRequests (half-open, probes used up); toolbench
//     returns ErrOpen for both.
//   - gobreaker re-panics a panic in the request; toolbench returns panics.ErrPanic. Not scripted.
//   - gobreaker counts a context error from the request as a failure (unless IsSuccessful says otherwise);
//     toolbench ignores the caller's own cancellation. Not scripted.
package breaker_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/breaker"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/sony/gobreaker/v2"
)

const (
	threshold = 3
	probes    = 2
	openFor   = time.Second
)

var errDown = errors.New("down")

type pair struct {
	ours     *breaker.Breaker
	fake     *clock.Fake
	oursLog  []string
	theirs   *gobreaker.CircuitBreaker[struct{}]
	theirLog []string
}

func newPair() *pair {
	p := &pair{fake: clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))}
	p.ours = breaker.New(
		breaker.WithFailureThreshold(threshold),
		breaker.WithOpenFor(openFor),
		breaker.WithHalfOpenProbes(probes),
		breaker.WithClock(p.fake),
		breaker.WithOnStateChange(func(from, to breaker.State) {
			p.oursLog = append(p.oursLog, from.String()+">"+to.String())
		}),
	)
	p.theirs = gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        "compare",
		MaxRequests: probes,
		Timeout:     openFor,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= threshold },
		OnStateChange: func(_ string, from, to gobreaker.State) {
			p.theirLog = append(p.theirLog, from.String()+">"+to.String())
		},
	})
	return p
}

// call runs one scripted result through both and reports (ours rejected, theirs rejected).
func (p *pair) call(result error) (bool, bool) {
	err := p.ours.Do(context.Background(), func(context.Context) error { return result })
	_, gerr := p.theirs.Execute(func() (struct{}, error) { return struct{}{}, result })
	return errors.Is(err, breaker.ErrOpen),
		errors.Is(gerr, gobreaker.ErrOpenState) || errors.Is(gerr, gobreaker.ErrTooManyRequests)
}

// wait moves both past the open timeout.
func (p *pair) wait() {
	p.fake.Advance(openFor)
	<-time.NewTimer(openFor + time.Millisecond).C
}

func TestBreaker_scriptMatchesGobreaker(t *testing.T) {
	p := newPair()
	type step struct {
		wait   bool
		result error
	}
	script := []step{
		{false, errDown}, {false, errDown}, {false, nil}, // a success resets the count
		{false, errDown}, {false, errDown}, {false, errDown}, // closed -> open
		{false, nil},              // rejected
		{true, nil},               // open -> half-open, probe 1 succeeds
		{false, errDown},          // probe fails: half-open -> open
		{false, nil},              // rejected
		{true, nil}, {false, nil}, // two probes succeed: closed
		{false, errDown}, {false, nil}, // count reset
		{false, errDown}, {false, errDown}, {false, errDown}, // open again
	}
	for i, s := range script {
		if s.wait {
			p.wait()
		}
		or, tr := p.call(s.result)
		if or != tr {
			t.Fatalf("step %d: ours rejected %v, gobreaker rejected %v", i, or, tr)
		}
		if os, ts := p.ours.State().String(), p.theirs.State().String(); os != ts {
			t.Fatalf("step %d: ours %s, gobreaker %s", i, os, ts)
		}
	}
	if o, g := strings.Join(p.oursLog, " "), strings.Join(p.theirLog, " "); o != g {
		t.Fatalf("transitions\n  ours %s\ntheirs %s", o, g)
	}
}

// TestBreaker_probeLimitMatchesGobreaker: with every probe in flight, both reject the next call.
func TestBreaker_probeLimitMatchesGobreaker(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	p := newPair()
	for range threshold {
		p.call(errDown)
	}
	p.wait()

	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{}, 2*probes)
	block := func() {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	for range probes {
		go func() {
			_ = p.ours.Do(ctx, func(context.Context) error { block(); return nil })
			done <- struct{}{}
		}()
		go func() {
			_, _ = p.theirs.Execute(func() (struct{}, error) { block(); return struct{}{}, nil })
			done <- struct{}{}
		}()
	}
	for range 2 * probes {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("probes did not start")
		}
	}
	err := p.ours.Do(ctx, func(context.Context) error { return nil })
	_, gerr := p.theirs.Execute(func() (struct{}, error) { return struct{}{}, nil })
	if !errors.Is(err, breaker.ErrOpen) || !errors.Is(gerr, gobreaker.ErrTooManyRequests) {
		t.Fatalf("over the probe limit: ours %v, gobreaker %v", err, gerr)
	}
	close(release)
	for range 2 * probes {
		<-done
	}
	if os, ts := p.ours.State().String(), p.theirs.State().String(); os != "closed" || ts != "closed" {
		t.Fatalf("after the probes: ours %s, gobreaker %s", os, ts)
	}
}
