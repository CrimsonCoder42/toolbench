// Package clock_test compares toolbench/clock.Fake with github.com/benbjohnson/clock.Mock: the same timers and
// ticker, advanced by the same amount (Mock.Add vs Fake.Advance), fire the same timers at the same times in the
// same order (deadline, then arming order).
//
// Known differences, by design:
//   - benbjohnson keeps a fired, unreceived value in its 1-slot channel after Timer.Stop and Timer.Reset (the
//     pre-go1.23 behaviour of package time); toolbench drains it, as go1.23 does. TestFake_StopNoStaleValue
//     asserts the difference.
//   - benbjohnson starts each AfterFunc function with go and does not wait for it, so the order of callbacks
//     is not deterministic there; toolbench's Advance waits for each before firing the next. Not compared.
package clock_test

import (
	"cmp"
	"fmt"
	"slices"
	"testing"
	"time"

	bjclock "github.com/benbjohnson/clock"

	"github.com/CrimsonCoder42/toolbench/clock"
)

var durations = []time.Duration{5 * time.Second, 1 * time.Second, 3 * time.Second, 3 * time.Second, 10 * time.Second}

const (
	advance = 6 * time.Second
	period  = 2 * time.Second
)

// fired is one received value: which timer, and the offset from the start it carried.
type fired struct {
	name string
	at   time.Duration
}

func poll(name string, c <-chan time.Time, start time.Time, out *[]fired) {
	select {
	case v := <-c:
		*out = append(*out, fired{name, v.Sub(start)})
	default:
	}
}

// sorted orders by the carried time only, keeping equal times in polling order, which is arming order: the
// firing order (deadline, then arming order) that a channel timer's value records.
func sorted(fs []fired) []fired {
	slices.SortStableFunc(fs, func(a, b fired) int { return cmp.Compare(a.at, b.at) })
	return fs
}

func TestFake_AdvanceMatchesMockAdd(t *testing.T) {
	// benbjohnson/clock.
	mock := bjclock.NewMock()
	mstart := mock.Now()
	var mtimers []*bjclock.Timer
	for _, d := range durations {
		mtimers = append(mtimers, mock.Timer(d))
	}
	mtick := mock.Ticker(period)
	defer mtick.Stop()
	mock.Add(advance)
	var theirs []fired
	for i, tm := range mtimers {
		poll(fmt.Sprint("t", i), tm.C, mstart, &theirs)
	}
	poll("tick", mtick.C, mstart, &theirs)

	// toolbench/clock.
	fake := clock.NewFake(time.Unix(0, 0).UTC())
	fstart := fake.Now()
	var ftimers []clock.Timer
	for _, d := range durations {
		ftimers = append(ftimers, fake.NewTimer(d))
	}
	ftick := fake.NewTicker(period)
	defer ftick.Stop()
	fake.Advance(advance)
	var ours []fired
	for i, tm := range ftimers {
		poll(fmt.Sprint("t", i), tm.C(), fstart, &ours)
	}
	poll("tick", ftick.C(), fstart, &ours)

	theirs, ours = sorted(theirs), sorted(ours)
	if fmt.Sprint(theirs) != fmt.Sprint(ours) {
		t.Fatalf("firing differs:\nbenbjohnson %v\ntoolbench   %v", theirs, ours)
	}
	// fired.at is unexported, so fmt prints it in nanoseconds; build want with the same printer.
	want := fmt.Sprint([]fired{{"t1", time.Second}, {"tick", 2 * time.Second}, {"t2", 3 * time.Second},
		{"t3", 3 * time.Second}, {"t0", 5 * time.Second}})
	if fmt.Sprint(ours) != want {
		t.Fatalf("toolbench fired %v, want %s", ours, want)
	}
	if mock.Now().Sub(mstart) != fake.Now().Sub(fstart) {
		t.Fatalf("Now after advance: benbjohnson +%v, toolbench +%v", mock.Now().Sub(mstart), fake.Now().Sub(fstart))
	}
}

func TestFake_StopNoStaleValue(t *testing.T) {
	stale := func(c <-chan time.Time) bool {
		select {
		case <-c:
			return true
		default:
			return false
		}
	}

	mock := bjclock.NewMock()
	mt := mock.Timer(time.Second)
	mock.Add(time.Second) // fired, not received
	mt.Stop()
	if !stale(mt.C) {
		t.Fatal("benbjohnson: no value after Stop; the documented difference is gone, update the header")
	}

	fake := clock.NewFake(time.Unix(0, 0).UTC())
	ft := fake.NewTimer(time.Second)
	fake.Advance(time.Second)
	if !ft.Stop() {
		t.Fatal("toolbench: Stop of a fired, unreceived timer = false, want true (go1.23)")
	}
	if stale(ft.C()) {
		t.Fatal("toolbench: stale value received after Stop")
	}
}
