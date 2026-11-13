package schedule_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/schedule"
)

// hourly runs on the hour: a custom Schedule.
type hourly struct{}

func (hourly) Next(after time.Time) time.Time { return after.Truncate(time.Hour).Add(time.Hour) }

func ExampleSchedule() {
	var s schedule.Schedule = hourly{}
	t := time.Date(2026, 1, 1, 9, 41, 0, 0, time.UTC)
	for range 3 {
		t = s.Next(t)
		fmt.Println(t.Format(time.TimeOnly))
	}
	// Output:
	// 10:00:00
	// 11:00:00
	// 12:00:00
}

func ExampleEvery() {
	s := schedule.Every(15 * time.Minute)
	fmt.Println(s.Next(start).Format(time.TimeOnly))
	fmt.Println(schedule.Every(0).Next(start).IsZero()) // never fires
	// Output:
	// 00:15:00
	// true
}

func ExampleAt() {
	s := schedule.At(start.Add(time.Hour))
	fmt.Println(s.Next(start).Format(time.TimeOnly))
	fmt.Println(s.Next(start.Add(time.Hour)).IsZero()) // one shot: nothing after it
	// Output:
	// 01:00:00
	// true
}

func TestEvery(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want time.Time
	}{
		{time.Second, start.Add(time.Second)},
		{time.Nanosecond, start.Add(time.Nanosecond)},
		{24 * time.Hour, start.Add(24 * time.Hour)},
		{0, time.Time{}},
		{-time.Second, time.Time{}},
	} {
		if got := schedule.Every(tc.d).Next(start); !got.Equal(tc.want) {
			t.Errorf("Every(%v).Next(start) = %v, want %v", tc.d, got, tc.want)
		}
	}
}

func TestAt(t *testing.T) {
	at := start.Add(time.Minute)
	s := schedule.At(at)
	for _, tc := range []struct {
		after time.Time
		want  time.Time
	}{
		{start, at},
		{at.Add(-time.Nanosecond), at},
		{at, time.Time{}},
		{at.Add(time.Hour), time.Time{}},
	} {
		if got := s.Next(tc.after); !got.Equal(tc.want) {
			t.Errorf("At(start+1m).Next(%v) = %v, want %v", tc.after, got, tc.want)
		}
	}
	if !schedule.At(time.Time{}).Next(start).IsZero() {
		t.Error("At(zero time) fires")
	}
}
