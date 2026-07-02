package heartbeat

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/didvc/dead-mans-ping/internal/pinger"
	"github.com/didvc/dead-mans-ping/internal/uilog"
)

func TestRunBeatsRepeatedly(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
	}))
	defer srv.Close()

	p, err := pinger.New([]string{srv.URL}, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Run(ctx, p, 20*time.Millisecond, uilog.New(io.Discard, false))

	// Expect the immediate beat plus at least one interval beat.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt64(&hits) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt64(&hits); got < 2 {
		t.Fatalf("heartbeat hits = %d, want >= 2", got)
	}
}
