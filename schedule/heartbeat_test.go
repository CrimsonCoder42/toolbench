package schedule_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/schedule"
)

func ExampleHeartbeat() {
	f := clock.NewFake(start)
	ctx, cancel := context.WithCancel(context.Background())
	beat, pulses := schedule.Heartbeat(ctx, time.Second, schedule.WithClock(f))

	beat()                 // the worker made progress
	f.Advance(time.Second) // the interval ends: one pulse
	fmt.Println("alive at", (<-pulses).Format(time.TimeOnly))

	cancel()
	for range pulses { // closed by Heartbeat's goroutine
	}
	fmt.Println("closed")
	// Output:
	// alive at 00:00:01
	// closed
}

// drain receives from ch until it is closed and returns how many values it had.
func drain[T any](t *testing.T, bound context.Context, ch <-chan T) int {
	t.Helper()
	n := 0
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return n
			}
			n++
		case <-bound.Done():
			t.Fatal("channel not closed before the test bound")
		}
	}
}

func TestHeartbeat_pulsesOnlyAfterBeat(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	f := clock.NewFake(start)
	beat, pulses := schedule.Heartbeat(ctx, time.Second, schedule.WithClock(f))

	beat()
	beat() // many beats in one interval: one pulse
	if err := f.AdvanceContext(bound, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, bound, pulses); !got.Equal(start.Add(time.Second)) {
		t.Fatalf("pulse %v, want start+1s", got)
	}
	if err := f.BlockUntil(bound, 1); err != nil { // the goroutine re-armed after the pulse
		t.Fatal(err)
	}

	// No beat in the second interval: no pulse (a stalled worker stops the heartbeat).
	if err := f.AdvanceContext(bound, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := f.BlockUntil(bound, 1); err != nil { // the wake at 2s has been handled
		t.Fatal(err)
	}
	none(t, pulses, "a pulse without a beat")

	beat()
	if err := f.AdvanceContext(bound, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, bound, pulses); !got.Equal(start.Add(3 * time.Second)) {
		t.Fatalf("pulse %v, want start+3s", got)
	}
	cancel()
	drain(t, bound, pulses)
	beat() // after cancel: a no-op
}

// The named drop: an unread pulse is replaced by the newer one, so a monitor reading late sees the latest
// "alive at", and the worker is never blocked.
func TestHeartbeat_unreadPulseReplacedByNewest(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	f := clock.NewFake(start)
	beat, pulses := schedule.Heartbeat(ctx, time.Second, schedule.WithClock(f))
	for range 2 {
		beat()
		if err := f.AdvanceContext(bound, time.Second); err != nil {
			t.Fatal(err)
		}
		if err := f.BlockUntil(bound, 1); err != nil {
			t.Fatal(err)
		}
	}
	if got := recv(t, bound, pulses); !got.Equal(start.Add(2 * time.Second)) {
		t.Fatalf("pulse %v, want the newest at start+2s", got)
	}
	none(t, pulses, "the replaced pulse at start+1s")
	cancel()
	drain(t, bound, pulses)
}

// go_37 checks in from a worker that sleeps between steps; here a worker beats as fast as it likes with no
// monitor reading, and beat never blocks.
func TestHeartbeat_go37_beatNeverBlocks(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	for _, interval := range []time.Duration{time.Second, 0} {
		ctx, cancel := context.WithCancel(bound)
		f := clock.NewFake(start)
		beat, pulses := schedule.Heartbeat(ctx, interval, schedule.WithClock(f))
		for range 10_000 {
			beat()
		}
		if interval > 0 {
			if err := f.AdvanceContext(bound, 5*interval); err != nil {
				t.Fatal(err)
			}
		}
		if c := cap(pulses); c != 1 { // the buffer is one slot; a pulse that finds it full is dropped
			t.Fatalf("interval %v: pulses capacity %d, want 1", interval, c)
		}
		cancel()
		drain(t, bound, pulses) // closed after cancel; while draining, a beat still pending may be forwarded
	}
}

func TestHeartbeat_perBeat(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	f := clock.NewFake(start)
	beat, pulses := schedule.Heartbeat(ctx, 0, schedule.WithClock(f))
	for i := range 3 {
		beat()
		if got := recv(t, bound, pulses); !got.Equal(start.Add(time.Duration(i) * time.Minute)) {
			t.Fatalf("pulse %d = %v", i, got)
		}
		f.Advance(time.Minute)
	}
	cancel()
	drain(t, bound, pulses)
}

func TestHeartbeat_cancelBefore(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	beat, pulses := schedule.Heartbeat(ctx, time.Second)
	beat()
	if n := drain(t, bound, pulses); n != 0 {
		t.Fatalf("%d pulses after a cancel before the start", n)
	}
}

func TestHeartbeat_realClockClosesOnCancel(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	beat, pulses := schedule.Heartbeat(ctx, time.Hour) // default clock.Real; nothing fires in the test
	beat()
	cancel()
	if n := drain(t, bound, pulses); n != 0 {
		t.Fatalf("%d pulses before the first hour", n)
	}
}

// Wakes stay on the grid start + k·interval: a wake handled late does not shift the next one, and wakes
// missed meanwhile are skipped.
func TestHeartbeat_wakesStayOnGrid(t *testing.T) {
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	f := clock.NewFake(start)
	beat, pulses := schedule.Heartbeat(ctx, time.Second, schedule.WithClock(f))
	beat()
	if err := f.AdvanceContext(bound, 5500*time.Millisecond); err != nil { // wake due at 1s, handled at 5.5s
		t.Fatal(err)
	}
	if got := recv(t, bound, pulses); !got.Equal(start.Add(time.Second)) {
		t.Fatalf("pulse %v, want start+1s", got)
	}
	if err := f.BlockUntil(bound, 1); err != nil {
		t.Fatal(err)
	}
	beat()
	if err := f.AdvanceContext(bound, 500*time.Millisecond); err != nil { // next grid point: 6s
		t.Fatal(err)
	}
	if got := recv(t, bound, pulses); !got.Equal(start.Add(6 * time.Second)) {
		t.Fatalf("pulse %v, want start+6s (on the grid, missed wakes skipped)", got)
	}
	cancel()
	drain(t, bound, pulses)
}
