package pinger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewRejectsBadEndpoints(t *testing.T) {
	bad := [][]string{
		{},                          // none
		{"ftp://example.com"},       // wrong scheme
		{"file:///etc/passwd"},      // wrong scheme
		{"http://"},                 // no host
		{"://nope"},                 // unparseable
		{"http://ok", "gopher://x"}, // one good, one bad
	}
	for _, eps := range bad {
		if _, err := New(eps, time.Second); err == nil {
			t.Errorf("New(%v) = nil error, want rejection", eps)
		}
	}
}

func TestPingHitsAllEndpoints(t *testing.T) {
	var a, b int64
	sa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { atomic.AddInt64(&a, 1) }))
	sb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { atomic.AddInt64(&b, 1) }))
	defer sa.Close()
	defer sb.Close()

	p, err := New([]string{sa.URL, sb.URL}, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	results := p.Ping(context.Background())
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Err != nil || r.Status != http.StatusOK {
			t.Errorf("result %+v", r)
		}
	}
	if atomic.LoadInt64(&a) != 1 || atomic.LoadInt64(&b) != 1 {
		t.Errorf("endpoint hits a=%d b=%d, want 1 each", a, b)
	}
}
