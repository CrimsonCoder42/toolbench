package clock_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var (
	_ clock.Clock = clock.Real()
	_ clock.Clock = (*clock.Fake)(nil)
	_ clock.Clock = clock.NewFake(time.Time{})
)

func ExampleReal() {
	c := clock.Real()
	t := c.NewTimer(time.Millisecond)
	<-t.C()
	fmt.Println("fired; Stop after the receive:", t.Stop())

	done := make(chan struct{})
	c.AfterFunc(time.Millisecond, func() { close(done) })
	<-done
	fmt.Println("AfterFunc ran")
	// Output:
	// fired; Stop after the receive: false
	// AfterFunc ran
}

func TestReal_Now(t *testing.T) {
	before := time.Now()
	got := clock.Real().Now()
	if got.Before(before) || got.After(time.Now()) {
		t.Fatalf("Real().Now() = %v, not between the surrounding time.Now calls", got)
	}
}

func TestReal_timerStopReset(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	c := clock.Real()

	tm := c.NewTimer(time.Hour)
	if !tm.Stop() {
		t.Fatal("Stop on an armed timer = false, want true")
	}
	if tm.Stop() {
		t.Fatal("second Stop = true, want false")
	}
	if tm.Reset(time.Millisecond) {
		t.Fatal("Reset of a stopped timer = true, want false")
	}
	select {
	case <-tm.C():
	case <-ctx.Done():
		t.Fatal("timer did not fire after Reset")
	}
}

func TestReal_AfterFunc_nilFunc(t *testing.T) {
	leak.Verify(t)
	tm := clock.Real().AfterFunc(0, nil)
	if tm.C() != nil {
		t.Fatal("C() of an AfterFunc timer is not nil")
	}
	tm.Stop()
}

func TestReal_NewTicker_nonPositive(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tk := clock.Real().NewTicker(0) // time.NewTicker(0) panics; ours returns a stopped ticker
	select {
	case <-tk.C():
		t.Fatal("stopped ticker ticked")
	default:
	}
	tk.Reset(-time.Second) // stays stopped, no panic
	tk.Reset(time.Millisecond)
	select {
	case <-tk.C():
	case <-ctx.Done():
		t.Fatal("ticker did not tick after Reset with a positive period")
	}
	tk.Stop()
}
