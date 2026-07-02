package metrics

import (
	"testing"
	"time"
)

func TestSummaryWindows(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	r := New()
	r.now = func() time.Time { return now }

	// One move now (in all windows).
	r.Record(10)
	// One move 90 minutes ago (in day/week, not hour).
	r.now = func() time.Time { return now.Add(-90 * time.Minute) }
	r.Record(20)
	// One move 3 days ago (week only).
	r.now = func() time.Time { return now.Add(-3 * 24 * time.Hour) }
	r.Record(30)
	// One move 11 days ago (outside all windows). 11d is used rather than a
	// value exactly N*7 days from an in-window sample, which would alias onto
	// the same ring slot (that aliasing is the intended weekly eviction).
	r.now = func() time.Time { return now.Add(-11 * 24 * time.Hour) }
	r.Record(40)

	r.now = func() time.Time { return now }
	s := r.Summary()

	if s.Hour.Events != 1 || s.Hour.Distance != 10 {
		t.Errorf("hour = %+v, want events=1 dist=10", s.Hour)
	}
	if s.Day.Events != 2 || s.Day.Distance != 30 {
		t.Errorf("day = %+v, want events=2 dist=30", s.Day)
	}
	if s.Week.Events != 3 || s.Week.Distance != 60 {
		t.Errorf("week = %+v, want events=3 dist=60", s.Week)
	}
}

func TestBucketReuseDiscardsOldWeek(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := New()
	r.now = func() time.Time { return base }
	r.Record(5)

	// Exactly one week later maps to the same ring slot; the old value must
	// not leak into the new week's counts.
	later := base.Add(weekMinutes * time.Minute)
	r.now = func() time.Time { return later }
	s := r.Summary()
	if s.Week.Events != 0 || s.Week.Distance != 0 {
		t.Errorf("stale bucket leaked: %+v", s.Week)
	}
}
