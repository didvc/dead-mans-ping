package extend

import (
	"testing"
	"time"
)

func TestExtendUntilOnlyMovesForward(t *testing.T) {
	s := New()
	base := time.Unix(1_000_000, 0)

	if got := s.ExtendUntil(base.Add(time.Hour)); !got.Equal(base.Add(time.Hour)) {
		t.Fatalf("first extend = %v", got)
	}
	// An earlier deadline must be ignored.
	if got := s.ExtendUntil(base.Add(time.Minute)); !got.Equal(base.Add(time.Hour)) {
		t.Fatalf("earlier extend changed deadline: %v", got)
	}
	// A later deadline wins.
	if got := s.ExtendUntil(base.Add(2 * time.Hour)); !got.Equal(base.Add(2 * time.Hour)) {
		t.Fatalf("later extend = %v", got)
	}
}

func TestExtendByAccumulates(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	s := New()
	s.now = func() time.Time { return now }

	got := s.ExtendBy(time.Minute)
	if !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("first ExtendBy = %v", got)
	}
	// Second call, still before the current deadline, accumulates on top of it
	// rather than resetting from now.
	got = s.ExtendBy(time.Minute)
	if !got.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("accumulated ExtendBy = %v, want now+2m", got)
	}
}
