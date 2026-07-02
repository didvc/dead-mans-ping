package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/didvc/dead-mans-ping/internal/pinger"
	"github.com/didvc/dead-mans-ping/internal/pointer"
	"github.com/didvc/dead-mans-ping/internal/uilog"
)

// scriptReader returns a fixed sequence of positions, repeating the last one.
type scriptReader struct {
	pts []pointer.Point
	i   int
}

func (r *scriptReader) Position() (pointer.Point, error) {
	p := r.pts[r.i]
	if r.i < len(r.pts)-1 {
		r.i++
	}
	return p, nil
}
func (r *scriptReader) Close() error { return nil }

// newTestEngine wires an engine to a counting HTTP server.
func newTestEngine(t *testing.T, cfg Config, reader pointer.Reader) (*Engine, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	p, err := pinger.New([]string{srv.URL}, 2*time.Second)
	if err != nil {
		t.Fatalf("pinger.New: %v", err)
	}
	return New(cfg, reader, p, nil, uilog.New(io.Discard, false)), &hits
}

// fakeDeadline is a static extension deadline.
type fakeDeadline struct{ until time.Time }

func (f fakeDeadline) Until() time.Time { return f.until }

// TestFlaggedRespectsDeadline: in inactive mode, an extension deadline in the
// future keeps the flag down even once the raw idle period has elapsed.
func TestFlaggedRespectsDeadline(t *testing.T) {
	now := time.Unix(10_000, 0)
	lastMove := now.Add(-2 * time.Hour) // idle well past the period
	cfg := Config{InactivePeriod: time.Hour}

	base := &Engine{cfg: cfg, now: func() time.Time { return now }}
	if !base.flagged(false, lastMove, now) {
		t.Fatal("without extension, should be flagged after idle > period")
	}

	extended := &Engine{cfg: cfg, ext: fakeDeadline{until: now.Add(time.Hour)}, now: func() time.Time { return now }}
	if extended.flagged(false, lastMove, now) {
		t.Fatal("extension into the future must suppress the flag")
	}

	// Past-dated extension has no effect.
	stale := &Engine{cfg: cfg, ext: fakeDeadline{until: now.Add(-time.Minute)}, now: func() time.Time { return now }}
	if !stale.flagged(false, lastMove, now) {
		t.Fatal("stale extension must not suppress the flag")
	}
}

// TestStepOnceOnetime: one ping on the rising edge, then done.
func TestStepOnceOnetime(t *testing.T) {
	e, hits := newTestEngine(t, Config{OneTime: true}, nil)
	canPing := func(time.Time) bool { return true }
	var last time.Time
	pinged := false
	now := time.Unix(1000, 0)

	if done := e.stepOnce(context.Background(), false, now, &pinged, &last, canPing); done {
		t.Fatal("should not be done while flag is false")
	}
	if got := atomic.LoadInt64(hits); got != 0 {
		t.Fatalf("unexpected ping while inactive: %d", got)
	}
	if done := e.stepOnce(context.Background(), true, now, &pinged, &last, canPing); !done {
		t.Fatal("onetime should be done after first ping")
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}

// TestStepOnceColdPeriod: one ping per episode, gated by the cooldown, no exit.
func TestStepOnceColdPeriod(t *testing.T) {
	cold := time.Minute
	e, hits := newTestEngine(t, Config{OneTime: false, ColdPeriod: cold}, nil)
	var last time.Time
	canPing := func(now time.Time) bool { return last.IsZero() || now.Sub(last) >= cold }

	pinged := false
	t0 := time.Unix(1000, 0)

	// First episode: pings once, does not exit.
	if done := e.stepOnce(context.Background(), true, t0, &pinged, &last, canPing); done {
		t.Fatal("cold-period must not exit")
	}
	// Same episode, still flagged: no second ping.
	e.stepOnce(context.Background(), true, t0.Add(10*time.Second), &pinged, &last, canPing)
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("hits = %d, want 1 within one episode", got)
	}

	// Flag clears, then a new episode starts before cooldown elapses: blocked.
	e.stepOnce(context.Background(), false, t0.Add(20*time.Second), &pinged, &last, canPing)
	e.stepOnce(context.Background(), true, t0.Add(30*time.Second), &pinged, &last, canPing)
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("hits = %d, want 1 (cooldown active)", got)
	}

	// New episode after cooldown: pings again.
	e.stepOnce(context.Background(), false, t0.Add(50*time.Second), &pinged, &last, canPing)
	e.stepOnce(context.Background(), true, t0.Add(2*time.Minute), &pinged, &last, canPing)
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("hits = %d, want 2 after cooldown", got)
	}
}

// TestStepContinuous: repeats on interval while flagged, exits on falling edge
// under the onetime lifecycle.
func TestStepContinuous(t *testing.T) {
	e, hits := newTestEngine(t, Config{PingContinuous: true, PingInterval: 10 * time.Second, OneTime: true}, nil)
	canPing := func(time.Time) bool { return true }
	var last time.Time
	burst := false
	t0 := time.Unix(1000, 0)

	// Rising edge: first ping.
	e.stepContinuous(context.Background(), true, t0, &burst, &last, canPing)
	// Before interval elapses: no extra ping.
	e.stepContinuous(context.Background(), true, t0.Add(3*time.Second), &burst, &last, canPing)
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("hits = %d, want 1 before interval", got)
	}
	// Interval elapsed: second ping.
	e.stepContinuous(context.Background(), true, t0.Add(11*time.Second), &burst, &last, canPing)
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("hits = %d, want 2 after interval", got)
	}
	// Falling edge under onetime: burst stops and loop should exit.
	if done := e.stepContinuous(context.Background(), false, t0.Add(15*time.Second), &burst, &last, canPing); !done {
		t.Fatal("continuous+onetime should exit when the burst ends")
	}
}

// TestRunActivePingExits is an end-to-end check that the run loop fires and
// returns under active-ping + ping-once + onetime.
func TestRunActivePingExits(t *testing.T) {
	reader := &scriptReader{pts: []pointer.Point{{X: 0, Y: 0}, {X: 50, Y: 0}}}
	cfg := Config{
		ActivePing:    true,
		OneTime:       true,
		PollInterval:  2 * time.Millisecond,
		MoveThreshold: 1.0,
	}
	e, hits := newTestEngine(t, cfg, reader)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := e.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("hits = %d, want exactly 1", got)
	}
}
