package schedule

import "time"

// Schedule gives the run times of a task. Next returns the first run time strictly after after, or the zero
// Time when there are no further runs. Run calls Next from one goroutine only. A nil Schedule passed to Run has
// no runs. Implement it for any calendar (the shape of robfig/cron's Schedule).
//
// ID:          TB-SCHED-001
// Pattern:     schedule (next-fire-time function)
// Authority:   pkg.go.dev time, "type Ticker" and "func NewTicker"
// Tags:        schedule, time, interface
type Schedule interface {
	Next(after time.Time) time.Time
}

// Every returns a Schedule whose run times are a fixed period d apart: Next(after) is after + d. A d <= 0
// never fires (Next returns the zero Time), so Run returns nil at once instead of spinning; it never panics
// as time.NewTicker does.
//
// ID:          TB-SCHED-002
// Pattern:     fixed-rate schedule (ticker)
// Authority:   pkg.go.dev time, "func NewTicker"
// Tags:        schedule, time, interval, ticker
func Every(d time.Duration) Schedule { return every(d) }

type every time.Duration

func (d every) Next(after time.Time) time.Time {
	if d <= 0 {
		return time.Time{}
	}
	return after.Add(time.Duration(d))
}

// At returns a Schedule with one run time, t: Next(after) is t when t is after after, else the zero Time. A t
// in the past (or the zero Time) never fires: Run returns nil without running the task, as a missed one-shot
// cron entry does not fire late.
//
// ID:          TB-SCHED-003
// Pattern:     one-shot schedule (timer)
// Authority:   pkg.go.dev time, "func NewTimer"
// Tags:        schedule, time, one-shot, timer
func At(t time.Time) Schedule { return at{t} }

type at struct{ t time.Time }

func (a at) Next(after time.Time) time.Time {
	if a.t.After(after) {
		return a.t
	}
	return time.Time{}
}
