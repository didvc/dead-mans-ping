package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/didvc/dead-mans-ping/internal/extend"
	"github.com/didvc/dead-mans-ping/internal/uilog"
)

func newTestServer() (*Server, *extend.State) {
	st := extend.New()
	return New("", st, uilog.New(io.Discard, false)), st
}

func TestExtendBySeconds(t *testing.T) {
	s, st := newTestServer()
	rec := httptest.NewRecorder()
	s.handleExtend(rec, httptest.NewRequest(http.MethodGet, "/extend?seconds=120", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %q", rec.Code, rec.Body.String())
	}
	if st.Until().IsZero() {
		t.Fatal("deadline was not extended")
	}
	if remaining := time.Until(st.Until()); remaining < 110*time.Second || remaining > 130*time.Second {
		t.Fatalf("remaining = %v, want ~120s", remaining)
	}
}

func TestExtendUntilAbsolute(t *testing.T) {
	s, st := newTestServer()
	target := time.Now().Add(time.Hour).Unix()
	rec := httptest.NewRecorder()
	s.handleExtend(rec, httptest.NewRequest(http.MethodGet, "/extend?until="+strconv.FormatInt(target, 10), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %q", rec.Code, rec.Body.String())
	}
	if st.Until().Unix() != target {
		t.Fatalf("deadline = %d, want %d", st.Until().Unix(), target)
	}
}

func TestExtendBadRequests(t *testing.T) {
	cases := []string{
		"/extend",                    // neither param
		"/extend?seconds=1&until=99", // both params
		"/extend?seconds=abc",        // non-numeric
		"/extend?seconds=-5",         // non-positive
		"/extend?until=nope",         // bad timestamp
	}
	for _, target := range cases {
		s, _ := newTestServer()
		rec := httptest.NewRecorder()
		s.handleExtend(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", target, rec.Code)
		}
	}
}

func TestExtendRejectsNonGet(t *testing.T) {
	s, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.handleExtend(rec, httptest.NewRequest(http.MethodPost, "/extend?seconds=1", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code = %d, want 405", rec.Code)
	}
}

func TestHelp(t *testing.T) {
	s, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.handleHelp(rec, httptest.NewRequest(http.MethodGet, "/help", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/extend") {
		t.Fatalf("help response: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestServeLifecycle exercises the real Listen/Serve/Shutdown path over a live
// socket on an ephemeral port.
func TestServeLifecycle(t *testing.T) {
	st := extend.New()
	s := New("127.0.0.1:0", st, uilog.New(io.Discard, false))
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()

	base := "http://" + s.ln.Addr().String()
	resp, err := http.Get(base + "/extend?seconds=60")
	if err != nil {
		t.Fatalf("GET /extend: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if time.Until(st.Until()) < 30*time.Second {
		t.Fatalf("deadline not extended: until=%v", st.Until())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not shut down")
	}
}
