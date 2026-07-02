// Package server exposes a small HTTP control surface for the tool.
//
// Security note: /extend can postpone a dead-man's-switch ping, so the server
// binds to 127.0.0.1 by default. Only expose it more widely behind your own
// authentication/proxy.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/didvc/dead-mans-ping/internal/extend"
	"github.com/didvc/dead-mans-ping/internal/uilog"
)

const helpText = `dead-mans-ping — control server

GET /extend?seconds=<N>    extend the inactivity deadline by N seconds (accumulates)
GET /extend?until=<unix>   extend the inactivity deadline to an absolute unix timestamp (seconds)
GET /help                  this text

Provide exactly one of seconds/until per /extend request.
`

// Server serves the control endpoints backed by an extension State.
type Server struct {
	ext *extend.State
	log *uilog.Logger
	srv *http.Server
	ln  net.Listener
}

// New builds the server bound (later, via Listen) to addr.
func New(addr string, ext *extend.State, log *uilog.Logger) *Server {
	s := &Server{ext: ext, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("/extend", s.handleExtend)
	mux.HandleFunc("/help", s.handleHelp)
	mux.HandleFunc("/", s.handleHelp)
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Listen opens the socket. It is separate from Serve so bind failures surface
// synchronously to the caller (before background goroutines start).
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("control server listen on %s: %w", s.srv.Addr, err)
	}
	s.ln = ln
	return nil
}

// Serve blocks until ctx is cancelled, then shuts down gracefully. Listen must
// have been called first.
func (s *Server) Serve(ctx context.Context) error {
	s.log.Event("control server listening on http://%s (endpoints: /extend, /help)", s.ln.Addr())
	errc := make(chan error, 1)
	go func() { errc <- s.srv.Serve(s.ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return s.srv.Shutdown(shutdownCtx)
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleExtend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	until, seconds := q.Get("until"), q.Get("seconds")
	if (until == "") == (seconds == "") {
		http.Error(w, "provide exactly one of ?seconds= or ?until=", http.StatusBadRequest)
		return
	}

	var deadline time.Time
	if seconds != "" {
		secs, err := strconv.ParseFloat(seconds, 64)
		if err != nil || secs <= 0 {
			http.Error(w, "seconds must be a positive number", http.StatusBadRequest)
			return
		}
		deadline = s.ext.ExtendBy(time.Duration(secs * float64(time.Second)))
	} else {
		ts, err := strconv.ParseInt(until, 10, 64)
		if err != nil {
			http.Error(w, "until must be a unix timestamp in seconds", http.StatusBadRequest)
			return
		}
		deadline = s.ext.ExtendUntil(time.Unix(ts, 0))
	}

	s.log.Event("inactivity deadline extended to %s (from %s)", deadline.Format(time.RFC3339), r.RemoteAddr)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "extended until %d (%s)\n", deadline.Unix(), deadline.Format(time.RFC3339))
}

func (s *Server) handleHelp(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, helpText)
}
