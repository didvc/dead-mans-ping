// Package engine wires cursor polling, movement metrics and endpoint pinging
// into a single run loop driven by the configured options.
//
// Three orthogonal option pairs shape behaviour:
//
//   - trigger:   inactive-ping (flag = idle >= InactivePeriod) vs
//     active-ping   (flag = movement just happened)
//   - ping style: ping-once   (one ping per flagged episode) vs
//     ping-continuous (repeat every PingInterval while flagged)
//   - lifecycle: onetime      (exit after the first ping / burst) vs
//     cold-period  (stay running, enforce a minimum gap between
//     pings; onetime is ignored)
//
// In inactive-ping mode an optional Deadline may postpone the moment the flag
// is raised (see package extend and the control server).
package engine

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/didvc/dead-mans-ping/internal/metrics"
	"github.com/didvc/dead-mans-ping/internal/pinger"
	"github.com/didvc/dead-mans-ping/internal/pointer"
	"github.com/didvc/dead-mans-ping/internal/uilog"
)

// Config holds the fully resolved runtime options.
type Config struct {
	ActivePing     bool          // true = active-ping, false = inactive-ping
	InactivePeriod time.Duration // idle threshold for inactive-ping
	PingContinuous bool          // true = ping-continuous, false = ping-once
	PingInterval   time.Duration // repeat interval for ping-continuous
	OneTime        bool          // exit after first ping/burst (ignored if ColdPeriod > 0)
	ColdPeriod     time.Duration // minimum gap between pings; > 0 disables OneTime
	PollInterval   time.Duration // cursor sampling interval
	MoveThreshold  float64       // min pixel distance counted as movement
}

// Deadline optionally postpones the inactivity trigger. Until returns the
// point in time before which the inactive flag must not be raised; a zero time
// means no extension. It is consulted only in inactive-ping mode.
type Deadline interface {
	Until() time.Time
}

// Engine runs the monitoring loop.
type Engine struct {
	cfg     Config
	reader  pointer.Reader
	metrics *metrics.Recorder
	pinger  *pinger.Pinger
	ext     Deadline
	log     *uilog.Logger
	now     func() time.Time
}

// New builds an Engine. ext may be nil (no deadline extension).
func New(cfg Config, reader pointer.Reader, p *pinger.Pinger, ext Deadline, log *uilog.Logger) *Engine {
	return &Engine{
		cfg:     cfg,
		reader:  reader,
		metrics: metrics.New(),
		pinger:  p,
		ext:     ext,
		log:     log,
		now:     time.Now,
	}
}

// Run blocks until the lifecycle completes (onetime) or ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	poll := time.NewTicker(e.cfg.PollInterval)
	defer poll.Stop()
	status := time.NewTicker(time.Second)
	defer status.Stop()

	last, err := e.reader.Position()
	if err != nil {
		return fmt.Errorf("read initial cursor position: %w", err)
	}
	lastMove := e.now()

	var (
		flag          bool
		pingedEpisode bool      // ping-once: already pinged this flagged episode
		burstActive   bool      // ping-continuous: a burst is in progress
		lastPingAt    time.Time // for cold-period and continuous interval
	)

	// canPing enforces the cold-period floor between pings. When ColdPeriod is
	// unset (onetime lifecycle) it always allows.
	canPing := func(t time.Time) bool {
		if e.cfg.ColdPeriod <= 0 {
			return true
		}
		return lastPingAt.IsZero() || t.Sub(lastPingAt) >= e.cfg.ColdPeriod
	}

	for {
		select {
		case <-ctx.Done():
			e.log.Finish()
			return ctx.Err()
		case <-status.C:
			e.printStatus(lastMove, flag)
			continue
		case <-poll.C:
		}

		cur, err := e.reader.Position()
		if err != nil {
			e.log.Event("cursor read error: %v", err)
			continue
		}
		dist := math.Hypot(float64(cur.X-last.X), float64(cur.Y-last.Y))
		last = cur
		now := e.now()
		moved := dist >= e.cfg.MoveThreshold
		if moved {
			e.metrics.Record(dist)
			lastMove = now
		}

		flag = e.flagged(moved, lastMove, now)

		var done bool
		if e.cfg.PingContinuous {
			done = e.stepContinuous(ctx, flag, now, &burstActive, &lastPingAt, canPing)
		} else {
			done = e.stepOnce(ctx, flag, now, &pingedEpisode, &lastPingAt, canPing)
		}
		if done {
			e.log.Finish()
			return nil
		}
	}
}

// flagged computes the activity flag. In active-ping mode it is simply whether
// the mouse just moved. In inactive-ping mode it is whether the idle deadline
// (possibly postponed by an extension) has passed.
func (e *Engine) flagged(moved bool, lastMove, now time.Time) bool {
	if e.cfg.ActivePing {
		return moved
	}
	deadline := lastMove.Add(e.cfg.InactivePeriod)
	if e.ext != nil {
		if u := e.ext.Until(); u.After(deadline) {
			deadline = u
		}
	}
	return !now.Before(deadline)
}

// stepOnce sends a single ping per flagged episode. Returns true when the
// onetime lifecycle is satisfied and the loop should exit.
func (e *Engine) stepOnce(ctx context.Context, flag bool, now time.Time, pingedEpisode *bool, lastPingAt *time.Time, canPing func(time.Time) bool) bool {
	if !flag {
		*pingedEpisode = false
		return false
	}
	if *pingedEpisode || !canPing(now) {
		return false
	}
	e.doPing(ctx)
	*lastPingAt = now
	*pingedEpisode = true
	return e.cfg.OneTime
}

// stepContinuous repeats pings every PingInterval while flagged. A "burst"
// begins on the rising edge and ends when the flag clears. Returns true when
// the onetime lifecycle is satisfied (burst ended) and the loop should exit.
func (e *Engine) stepContinuous(ctx context.Context, flag bool, now time.Time, burstActive *bool, lastPingAt *time.Time, canPing func(time.Time) bool) bool {
	if flag {
		if !*burstActive {
			if !canPing(now) {
				return false // cooling down before a new burst may start
			}
			*burstActive = true
			e.log.Event("continuous pings started")
			e.doPing(ctx)
			*lastPingAt = now
			return false
		}
		if now.Sub(*lastPingAt) >= e.cfg.PingInterval {
			e.doPing(ctx)
			*lastPingAt = now
		}
		return false
	}
	if *burstActive {
		*burstActive = false
		e.log.Event("continuous pings stopped")
		return e.cfg.OneTime
	}
	return false
}

func (e *Engine) doPing(ctx context.Context) {
	for _, r := range e.pinger.Ping(ctx) {
		if r.Err != nil {
			e.log.Event("ping %s -> error: %v", r.Endpoint, r.Err)
		} else {
			e.log.Event("ping %s -> %d", r.Endpoint, r.Status)
		}
	}
}

// printStatus refreshes the in-place status line with the movement summary.
func (e *Engine) printStatus(lastMove time.Time, flagged bool) {
	if !e.log.Enabled() {
		return
	}
	s := e.metrics.Summary()
	mode := "inactive"
	if e.cfg.ActivePing {
		mode = "active"
	}
	now := e.now()
	ext := ""
	if e.ext != nil {
		if u := e.ext.Until(); u.After(now) {
			ext = fmt.Sprintf(" ext=+%s", u.Sub(now).Truncate(time.Second))
		}
	}
	e.log.Status(fmt.Sprintf(
		"[%s] mode=%s flag=%v idle=%s%s | 1h %.1fpx/%d 1d %.1fpx/%d 1w %.1fpx/%d",
		now.Format("15:04:05"), mode, flagged, now.Sub(lastMove).Truncate(time.Second), ext,
		s.Hour.Distance, s.Hour.Events,
		s.Day.Distance, s.Day.Events,
		s.Week.Distance, s.Week.Events,
	))
}
