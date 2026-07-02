// Package pinger sends HTTP GET requests to the configured endpoints.
//
// Security posture: endpoints are validated up front to be http/https with a
// host; requests carry a fixed timeout; redirect chains are capped; response
// bodies are read but discarded under a size limit so a hostile endpoint
// cannot exhaust memory; and TLS verification is left at its secure default.
package pinger

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	maxRedirects = 5
	maxBodyRead  = 64 << 10 // 64 KiB, read and discarded
	userAgent    = "dead-mans-ping/1.0"
)

// Pinger sends GETs to a fixed set of validated endpoints.
type Pinger struct {
	client    *http.Client
	endpoints []string
}

// New validates the endpoints and builds a hardened HTTP client.
func New(endpoints []string, timeout time.Duration) (*Pinger, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("at least one --endpoint is required")
	}
	for _, e := range endpoints {
		if err := validate(e); err != nil {
			return nil, err
		}
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
	return &Pinger{client: client, endpoints: endpoints}, nil
}

func validate(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid endpoint %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("endpoint %q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint %q: missing host", raw)
	}
	return nil
}

// Result reports the outcome of pinging one endpoint.
type Result struct {
	Endpoint string
	Status   int
	Err      error
}

// Ping sends a GET to every endpoint and returns a result per endpoint.
func (p *Pinger) Ping(ctx context.Context) []Result {
	results := make([]Result, 0, len(p.endpoints))
	for _, e := range p.endpoints {
		results = append(results, p.pingOne(ctx, e))
	}
	return results
}

func (p *Pinger) pingOne(ctx context.Context, endpoint string) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Result{Endpoint: endpoint, Err: err}
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return Result{Endpoint: endpoint, Err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyRead))
	return Result{Endpoint: endpoint, Status: resp.StatusCode}
}
