package domain

import "time"

// DateLayout is the YYYY-MM-DD format used for all planning dates.
const DateLayout = "2006-01-02"

// ParseDate parses a YYYY-MM-DD string. The zero Time is returned when empty or
// malformed, so callers can treat both as "no date".
func ParseDate(s string) time.Time {
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// AddWorkingDays returns the date n working days (Mon–Fri, weekends skipped)
// after the given start date, formatted as YYYY-MM-DD. Holidays are not modelled.
func AddWorkingDays(start string, n int) string {
	t := ParseDate(start)
	if t.IsZero() {
		return ""
	}
	added := 0
	for added < n {
		t = t.AddDate(0, 0, 1)
		if wd := t.Weekday(); wd != time.Saturday && wd != time.Sunday {
			added++
		}
	}
	return t.Format(DateLayout)
}

// WorkingDaysBetween counts working days from a to b (negative if b precedes a).
func WorkingDaysBetween(a, b string) int {
	from, to := ParseDate(a), ParseDate(b)
	if from.IsZero() || to.IsZero() {
		return 0
	}
	sign := 1
	if to.Before(from) {
		from, to = to, from
		sign = -1
	}
	days := 0
	for from.Before(to) {
		from = from.AddDate(0, 0, 1)
		if wd := from.Weekday(); wd != time.Saturday && wd != time.Sunday {
			days++
		}
	}
	return days * sign
}
