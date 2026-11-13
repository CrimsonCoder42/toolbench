package schedule

import (
	"fmt"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/cron"
)

// ParseCron returns a Schedule for a five-field POSIX crontab time specification (minute hour day-of-month
// month day-of-week, for example "30 2 * * 1-5"), read on the wall clock of loc. A nil loc is time.UTC (never
// time.Local: the package reads no environment). Next(after) converts after to loc and returns the first
// matching minute strictly after it, in loc; it never returns the zero Time for a Schedule ParseCron gave.
// Steps ("*/5"), names ("MON"), 7 for Sunday and "@daily" macros are not POSIX and are rejected. Day of month
// and day of week are ORed only when both are not "*"; the month always restricts. Daylight saving: a
// matching wall time in a spring-forward gap fires once, at the transition instant; one in a fall-back
// overlap fires at its first occurrence only.
//
// ID:          TB-SCHED-012
// Pattern:     cron schedule (crontab time specification)
// Authority:   IEEE Std 1003.1-2024 (POSIX.1-2024), Shell & Utilities, "crontab", INPUT FILES, https://pubs.opengroup.org/onlinepubs/9799919799/utilities/crontab.html
// Errors:      wrapped calcbench algo/cron sentinels (match with errors.Is): cron.ErrSyntax (not five POSIX fields; steps, names, macros), cron.ErrRange (a value outside its field, lo > hi, day of week 7), cron.ErrNeverFires (no day can match, e.g. "0 0 31 2 *"); the Schedule is nil when the error is not
// Math:        ALG-CRON-001, ALG-CRON-002
// Tags:        schedule, cron, crontab, posix, time-zone, daylight-saving
func ParseCron(spec string, loc *time.Location) (Schedule, error) {
	c, err := cron.ParseCron(spec)
	if err != nil {
		return nil, fmt.Errorf("schedule: ParseCron(%q): %w", spec, err)
	}
	if loc == nil {
		loc = time.UTC
	}
	return cronSchedule{spec: c, loc: loc}, nil
}

type cronSchedule struct {
	spec cron.CronSpec
	loc  *time.Location
}

// Next calls calcbench CronSpec.Next on after's wall clock in loc. Its only error, ErrNeverFires, cannot
// happen for a CronSpec that cron.ParseCron accepted (every accepted spec fires within nine years); were it
// to happen, the zero Time ends Run with nil rather than looping.
func (s cronSchedule) Next(after time.Time) time.Time {
	t, err := s.spec.Next(after.In(s.loc))
	if err != nil {
		return time.Time{}
	}
	return t
}
