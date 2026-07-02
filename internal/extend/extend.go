// Package extend holds the runtime-adjustable inactivity deadline extension.
//
// The control server pushes this deadline forward (via /extend); the activity
// engine reads it when deciding whether the "inactive" flag should be raised.
// This lets an operator remotely postpone a dead-man's-switch ping without
// touching the mouse.
package extend

import (
	"sync"
	"time"
)

// State is a concurrency-safe extension deadline. The zero deadline means "no
// extension". The deadline only ever moves forward.
type State struct {
	mu    sync.Mutex
	until time.Time
	now   func() time.Time
}

// New returns an empty State (no extension).
func New() *State { return &State{now: time.Now} }

// Until reports the current extension deadline; a zero time means none.
func (s *State) Until() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.until
}

// ExtendUntil moves the deadline to t if t is later than the current value,
// then returns the effective deadline. Earlier values are ignored so a request
// can never shorten the deadline.
func (s *State) ExtendUntil(t time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.After(s.until) {
		s.until = t
	}
	return s.until
}

// ExtendBy adds d on top of the later of now and the current deadline, so
// repeated calls accumulate. Returns the effective deadline.
func (s *State) ExtendBy(d time.Duration) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	base := s.now()
	if s.until.After(base) {
		base = s.until
	}
	s.until = base.Add(d)
	return s.until
}
