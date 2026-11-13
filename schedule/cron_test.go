package schedule_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // zone rules embedded so America/New_York loads on any host, without the host's zoneinfo

	"github.com/CrimsonCoder42/calcbench/algo/cron"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/schedule"
)

func ExampleParseCron() {
	s, err := schedule.ParseCron("0 10 * * 1-5", nil) // 10:00 Monday to Friday, UTC
	if err != nil {
		fmt.Println(err)
		return
	}
	t := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC) // a Friday, 10:00
	for range 2 {
		t = s.Next(t)
		fmt.Println(t.Format("Mon 2006-01-02 15:04 MST"))
	}
	_, err = schedule.ParseCron("*/5 * * * *", nil) // steps are not POSIX
	fmt.Println(errors.Is(err, cron.ErrSyntax))
	// Output:
	// Mon 2026-11-16 10:00 UTC
	// Tue 2026-11-17 10:00 UTC
	// true
}

func mustCron(t *testing.T, spec string, loc *time.Location) schedule.Schedule {
	t.Helper()
	s, err := schedule.ParseCron(spec, loc)
	if err != nil {
		t.Fatalf("ParseCron(%q): %v", spec, err)
	}
	return s
}

func newYork(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("America/New_York: %v (time/tzdata is embedded)", err)
	}
	return loc
}

func utcAt(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

// checkCron asserts s.Next(after) is the instant want, in loc.
func checkCron(t *testing.T, s schedule.Schedule, after, want time.Time, loc *time.Location) {
	t.Helper()
	got := s.Next(after)
	if !got.Equal(want) {
		t.Fatalf("Next(%v) = %v, want %v", after, got, want.In(loc))
	}
	if got.Location().String() != loc.String() {
		t.Fatalf("Next(%v) = %v is in %v, want %v", after, got, got.Location(), loc)
	}
}

func TestParseCron_errors(t *testing.T) {
	for _, tc := range []struct {
		spec string
		want error
	}{
		{"*/5 * * * *", cron.ErrSyntax}, // steps are not POSIX
		{"0 0 * * MON", cron.ErrSyntax}, // names
		{"0 0 1 JAN *", cron.ErrSyntax},
		{"@daily", cron.ErrSyntax}, // macros
		{"", cron.ErrSyntax},
		{"* * * *", cron.ErrSyntax},        // four fields
		{"0 * * * * *", cron.ErrSyntax},    // a seconds field
		{"0 0 * * 7", cron.ErrRange},       // 7 for Sunday is not POSIX
		{"60 * * * *", cron.ErrRange},      // minute 0-59
		{"0 0 5-3 * *", cron.ErrRange},     // lo > hi
		{"0 0 31 2 *", cron.ErrNeverFires}, // February has no 31st
		{"0 0 30 2 *", cron.ErrNeverFires},
		{"0 0 31 4,6,9,11 *", cron.ErrNeverFires},
	} {
		s, err := schedule.ParseCron(tc.spec, time.UTC)
		if !errors.Is(err, tc.want) {
			t.Errorf("ParseCron(%q) error = %v, want errors.Is %v", tc.spec, err, tc.want)
		}
		if s != nil {
			t.Errorf("ParseCron(%q) Schedule = %v, want nil with an error", tc.spec, s)
		}
		if err != nil && !strings.HasPrefix(err.Error(), "schedule: ParseCron(") {
			t.Errorf("ParseCron(%q) error = %q, want the schedule: prefix", tc.spec, err)
		}
	}
}

func TestParseCron_nilLocIsUTC(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+1800)
	s := mustCron(t, "0 9 * * *", nil)
	// The same instant given in another zone: the wall clock of UTC decides, and the result is in UTC.
	checkCron(t, s, start.In(ist), utcAt(2026, 1, 1, 9, 0), time.UTC)
	checkCron(t, s, utcAt(2026, 1, 1, 9, 0), utcAt(2026, 1, 2, 9, 0), time.UTC)
	// An explicit loc: 09:00 IST is 03:30Z, whatever the location of after.
	checkCron(t, mustCron(t, "0 9 * * *", ist), start, time.Date(2026, 1, 1, 9, 0, 0, 0, ist), ist)
}

// 2026-03-08, America/New_York: 02:00-02:59 EST do not exist; 03:00 EDT is 07:00Z.
func TestParseCron_springForwardGap(t *testing.T) {
	ny := newYork(t)
	gapEnd := utcAt(2026, 3, 8, 7, 0)
	s := mustCron(t, "30 2 * * *", ny)
	checkCron(t, s, utcAt(2026, 3, 7, 8, 0), gapEnd, ny)                                               // 02:30 is in the gap: the transition instant
	checkCron(t, s, gapEnd, utcAt(2026, 3, 9, 6, 30), ny)                                              // once: next is 02:30 EDT the day after
	every := mustCron(t, "* 2 * * *", ny)                                                              // sixty matching wall times in the gap
	checkCron(t, every, utcAt(2026, 3, 8, 6, 59), gapEnd, ny)                                          // after 01:59 EST: fire once at 03:00 EDT
	checkCron(t, every, gapEnd, utcAt(2026, 3, 9, 6, 0), ny)                                           // then not again until 02:00 EDT next day
	checkCron(t, mustCron(t, "59 1 * * *", ny), utcAt(2026, 3, 8, 5, 0), utcAt(2026, 3, 8, 6, 59), ny) // 01:59 EST fires normally
}

// 2026-11-01, America/New_York: 01:00-01:59 occurs at -04:00 (05:00Z-05:59Z) and again at -05:00 (06:00Z-06:59Z).
func TestParseCron_fallBackOverlap(t *testing.T) {
	ny := newYork(t)
	s := mustCron(t, "30 1 * * *", ny)
	checkCron(t, s, utcAt(2026, 11, 1, 4, 0), utcAt(2026, 11, 1, 5, 30), ny)  // first occurrence, 01:30 EDT
	checkCron(t, s, utcAt(2026, 11, 1, 5, 30), utcAt(2026, 11, 2, 6, 30), ny) // 01:30 EST never fires
	checkCron(t, s, utcAt(2026, 11, 1, 6, 10), utcAt(2026, 11, 2, 6, 30), ny)
	every := mustCron(t, "* * * * *", ny)
	checkCron(t, every, utcAt(2026, 11, 1, 5, 59), utcAt(2026, 11, 1, 7, 0), ny) // 01:59 EDT → 02:00 EST
}

// Day rule (calcbench A-068): the month always restricts; day of month and day of week are ORed only when
// both are not "*". 2026-11-13 is a Friday; 2026-12-13 is a Sunday; 2027-02-05 is the first Friday of February.
func TestParseCron_domDowOr(t *testing.T) {
	for _, tc := range []struct {
		name, spec  string
		after, want time.Time
	}{
		{"13th or Friday: a Friday", "0 0 13 * 5", utcAt(2026, 11, 13, 0, 0), utcAt(2026, 11, 20, 0, 0)},
		{"13th or Friday: a Sunday the 13th", "0 0 13 * 5", utcAt(2026, 12, 11, 0, 0), utcAt(2026, 12, 13, 0, 0)},
		{"dom only", "0 0 13 * *", utcAt(2026, 11, 13, 0, 0), utcAt(2026, 12, 13, 0, 0)},
		{"dow only", "0 0 * * 5", utcAt(2026, 11, 13, 0, 0), utcAt(2026, 11, 20, 0, 0)},
		{"month restricts the OR", "0 0 13 2 5", utcAt(2026, 11, 13, 0, 0), utcAt(2027, 2, 5, 0, 0)},
		{"Feb 31 or any weekday", "0 0 31 2 0-6", utcAt(2026, 1, 15, 0, 0), utcAt(2026, 2, 1, 0, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) { checkCron(t, mustCron(t, tc.spec, nil), tc.after, tc.want, time.UTC) })
	}
}

// runCron drives Run(ParseCron(spec, loc)) on a recording fake clock that starts at t0 and asserts each wait
// and each run instant.
func runCron(t *testing.T, spec string, loc *time.Location, t0 time.Time, runs []time.Time) {
	t.Helper()
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	defer cancel()
	rc := &recClock{Fake: clock.NewFake(t0), waits: make(chan time.Duration, 1024)}
	ran := make(chan time.Time)
	errc := goRun(ctx, mustCron(t, spec, loc), reporter(rc, ran), schedule.WithClock(rc))
	prev := t0
	for i, want := range runs {
		if d := recv(t, bound, rc.waits); d != want.Sub(prev) {
			t.Fatalf("wait %d = %v, want %v (to %v)", i, d, want.Sub(prev), want)
		}
		if err := rc.AdvanceContext(bound, want.Sub(prev)); err != nil {
			t.Fatal(err)
		}
		if got := recv(t, bound, ran); !got.Equal(want) {
			t.Fatalf("run %d at %v, want %v", i, got, want)
		}
		prev = want
	}
	recv(t, bound, rc.waits) // Run has re-armed its timer for the next run: no extra run is pending
	none(t, ran, "a run before its time")
	cancel()
	if err := recv(t, bound, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
}

func TestParseCron_runFiresAtCronInstants(t *testing.T) {
	runCron(t, "15,45 * * * *", nil, start,
		[]time.Time{utcAt(2026, 1, 1, 0, 15), utcAt(2026, 1, 1, 0, 45), utcAt(2026, 1, 1, 1, 15)})
}

func TestParseCron_runAcrossSpringForwardGap(t *testing.T) {
	// From 03:00 EST on 2026-03-07: the gap day fires once at 03:00 EDT, then 02:30 EDT the next day.
	runCron(t, "30 2 * * *", newYork(t), utcAt(2026, 3, 7, 8, 0),
		[]time.Time{utcAt(2026, 3, 8, 7, 0), utcAt(2026, 3, 9, 6, 30)})
}

func TestParseCron_runAcrossFallBackOverlap(t *testing.T) {
	// From midnight EDT on 2026-11-01: 01:30 EDT fires; 01:30 EST does not; next is 01:30 EST on the 2nd.
	runCron(t, "30 1 * * *", newYork(t), utcAt(2026, 11, 1, 4, 0),
		[]time.Time{utcAt(2026, 11, 1, 5, 30), utcAt(2026, 11, 2, 6, 30)})
}

// ErrNeverFires is a construction error only: a spec ParseCron accepts always fires, even years away, so Run
// waits for it rather than failing or ending.
func TestParseCron_neverFiresOnlyAtParse(t *testing.T) {
	if s, err := schedule.ParseCron("0 0 31 2 *", nil); !errors.Is(err, cron.ErrNeverFires) || s != nil {
		t.Fatalf("ParseCron(Feb 31) = %v, %v; want nil, ErrNeverFires", s, err)
	}
	leak.Verify(t)
	bound, stop := testCtx(t)
	defer stop()
	ctx, cancel := context.WithCancel(bound)
	defer cancel()
	rc := newRec()
	ran := make(chan time.Time)
	errc := goRun(ctx, mustCron(t, "0 0 29 2 *", nil), reporter(rc, ran), schedule.WithClock(rc))
	if d, want := recv(t, bound, rc.waits), utcAt(2028, 2, 29, 0, 0).Sub(start); d != want {
		t.Fatalf("wait = %v, want %v (to 2028-02-29)", d, want)
	}
	cancel()
	if err := recv(t, bound, errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled (never ErrNeverFires)", err)
	}
	none(t, ran, "a run of a schedule cancelled while waiting")
}
