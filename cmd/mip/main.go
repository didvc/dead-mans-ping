// Command mip (dead-mans-ping) sends HTTP GET requests to one or more
// endpoints based on mouse-movement activity. See README.md for details.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/didvc/dead-mans-ping/internal/engine"
	"github.com/didvc/dead-mans-ping/internal/extend"
	"github.com/didvc/dead-mans-ping/internal/heartbeat"
	"github.com/didvc/dead-mans-ping/internal/pinger"
	"github.com/didvc/dead-mans-ping/internal/pointer"
	"github.com/didvc/dead-mans-ping/internal/server"
	"github.com/didvc/dead-mans-ping/internal/uilog"
)

// stringList collects a repeatable string flag (used for --endpoint).
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("endpoint must not be empty")
	}
	*s = append(*s, v)
	return nil
}

// dwPattern matches a number followed by a day (d) or week (w) unit so that
// friendly values like "3d" or "1w" work; Go's own parser only knows up to
// hours.
var dwPattern = regexp.MustCompile(`([0-9]*\.?[0-9]+)([dw])`)

func expandDaysWeeks(s string) string {
	return dwPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := dwPattern.FindStringSubmatch(m)
		val, _ := strconv.ParseFloat(sub[1], 64)
		hours := val * 24
		if sub[2] == "w" {
			hours = val * 24 * 7
		}
		return strconv.FormatFloat(hours, 'f', -1, 64) + "h"
	})
}

func parseDuration(name, s string) (time.Duration, error) {
	d, err := time.ParseDuration(expandDaysWeeks(strings.TrimSpace(s)))
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q (try e.g. 3d, 12h, 30s)", name, s)
	}
	if d < 0 {
		return 0, fmt.Errorf("%s: must not be negative", name)
	}
	return d, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var endpoints stringList
	flag.Var(&endpoints, "endpoint", "URL to send a GET request to; repeat for multiple endpoints")

	activePing := flag.Bool("active-ping", false, "ping instantly on mouse movement (overrides --inactive-ping; --inactive-period ignored)")
	flag.Bool("inactive-ping", true, "ping when the mouse is inactive for --inactive-period (default)")
	inactivePeriod := flag.String("inactive-period", "3d", "inactivity threshold before pinging (e.g. 3d, 12h)")

	onetime := flag.Bool("onetime", true, "exit after the first ping / after continuous pings stop (default)")
	coldPeriod := flag.String("cold-period", "", "minimum wait between pings; keeps running instead of exiting (overrides --onetime)")

	pingContinuous := flag.Bool("ping-continuous", false, "keep pinging every --ping-interval while flagged; stops when the flag clears")
	pingInterval := flag.String("ping-interval", "30s", "interval between pings for --ping-continuous")
	flag.Bool("ping-once", true, "send a single ping when flagged (default)")

	heartbeatEndpoint := flag.String("heartbeat-endpoint", "", "URL to GET on a fixed interval as a liveness check (independent of activity)")
	heartbeatInterval := flag.String("heartbeat-interval", "60s", "interval for --heartbeat-endpoint")

	serverEnabled := flag.Bool("server", false, "run the control HTTP server (endpoints: /extend, /help)")
	serverAddr := flag.String("server-addr", "127.0.0.1:8080", "listen address for --server")

	pollInterval := flag.String("poll-interval", "1s", "cursor sampling interval")
	moveThreshold := flag.Float64("move-threshold", 1.0, "minimum pixel distance counted as movement")
	timeout := flag.String("timeout", "10s", "per-request HTTP timeout")
	noLog := flag.Bool("no-log", false, "disable status and log output")

	flag.Parse()

	inactiveD, err := parseDuration("--inactive-period", *inactivePeriod)
	if err != nil {
		return err
	}
	pingIntervalD, err := parseDuration("--ping-interval", *pingInterval)
	if err != nil {
		return err
	}
	pollD, err := parseDuration("--poll-interval", *pollInterval)
	if err != nil {
		return err
	}
	if pollD <= 0 {
		return errors.New("--poll-interval must be greater than zero")
	}
	timeoutD, err := parseDuration("--timeout", *timeout)
	if err != nil {
		return err
	}
	heartbeatD, err := parseDuration("--heartbeat-interval", *heartbeatInterval)
	if err != nil {
		return err
	}

	cfg := engine.Config{
		ActivePing:     *activePing,
		InactivePeriod: inactiveD,
		PingContinuous: *pingContinuous,
		PingInterval:   pingIntervalD,
		OneTime:        *onetime,
		PollInterval:   pollD,
		MoveThreshold:  *moveThreshold,
	}
	// --cold-period, when set, replaces the onetime lifecycle with a cooldown.
	if strings.TrimSpace(*coldPeriod) != "" {
		coldD, err := parseDuration("--cold-period", *coldPeriod)
		if err != nil {
			return err
		}
		cfg.ColdPeriod = coldD
		cfg.OneTime = false
	}

	activityPinger, err := pinger.New(endpoints, timeoutD)
	if err != nil {
		return err
	}

	// Optional heartbeat pinger (separate endpoint, separate timing).
	var heartbeatPinger *pinger.Pinger
	if strings.TrimSpace(*heartbeatEndpoint) != "" {
		if heartbeatD <= 0 {
			return errors.New("--heartbeat-interval must be greater than zero")
		}
		heartbeatPinger, err = pinger.New([]string{*heartbeatEndpoint}, timeoutD)
		if err != nil {
			return err
		}
	}

	log := uilog.New(os.Stderr, !*noLog)
	ext := extend.New()

	// Bind the control server up front so a bad address fails fast, before any
	// goroutine or the cursor reader is started.
	var ctrl *server.Server
	if *serverEnabled {
		ctrl = server.New(*serverAddr, ext, log)
		if err := ctrl.Listen(); err != nil {
			return err
		}
	}

	reader, err := pointer.New()
	if err != nil {
		return err
	}
	defer reader.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	if ctrl != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ctrl.Serve(ctx); err != nil {
				log.Event("control server error: %v", err)
			}
		}()
	}
	if heartbeatPinger != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			heartbeat.Run(ctx, heartbeatPinger, heartbeatD, log)
		}()
	}

	runErr := engine.New(cfg, reader, activityPinger, ext, log).Run(ctx)

	stop()    // tell the server and heartbeat to wind down
	wg.Wait() // wait for them before returning so shutdown logs flush

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	return nil
}
