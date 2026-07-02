// Package heartbeat periodically GETs a health endpoint to prove this process
// itself is alive. It is completely independent of mouse activity and of the
// activity pings; the two never share endpoints or timing.
package heartbeat

import (
	"context"
	"time"

	"github.com/didvc/dead-mans-ping/internal/pinger"
	"github.com/didvc/dead-mans-ping/internal/uilog"
)

// Run sends one heartbeat immediately, then one every interval, until ctx is
// cancelled. p should be configured with the single heartbeat endpoint.
func Run(ctx context.Context, p *pinger.Pinger, interval time.Duration, log *uilog.Logger) {
	beat := func() {
		for _, r := range p.Ping(ctx) {
			if r.Err != nil {
				log.Event("heartbeat %s -> error: %v", r.Endpoint, r.Err)
			} else {
				log.Event("heartbeat %s -> %d", r.Endpoint, r.Status)
			}
		}
	}

	beat()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			beat()
		}
	}
}
