// Package internalclient provides a small HTTP client wrapper for
// service-to-service calls inside the Padosme mesh. It auto-injects the
// X-Service-Token header, forwards the request ID, marshals JSON bodies, and
// returns a typed error for non-2xx responses.
package internalclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config configures a Client. BaseURL is the target service root
// (e.g. "http://padosme-wallet-service:8080"). ServiceToken is the bearer
// value sent in X-Service-Token. CallerName is sent as X-Caller-Service so
// the server can log who called it.
type Config struct {
	BaseURL      string
	ServiceToken string
	CallerName   string
	Timeout      time.Duration
}

// Client is a thin wrapper around *http.Client.
type Client struct {
	cfg Config
	hc  *http.Client
}

// New builds a Client. Timeout defaults to 10s.
func New(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Client{cfg: cfg, hc: &http.Client{Timeout: cfg.Timeout}}
}

// Error is returned for non-2xx responses.
type Error struct {
	Status int
	Body   string
	URL    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("internalclient: %s %d: %s", e.URL, e.Status, e.Body)
}

// DoJSON sends reqBody as JSON to METHOD path and decodes the response body
// into respBody if respBody is not nil and the response is 2xx.
func (c *Client) DoJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("internalclient: marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("internalclient: build request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.cfg.ServiceToken != "" {
		req.Header.Set("X-Service-Token", c.cfg.ServiceToken)
	}
	if c.cfg.CallerName != "" {
		req.Header.Set("X-Caller-Service", c.cfg.CallerName)
	}
	if rid, ok := ctx.Value(ctxKeyRequestID{}).(string); ok && rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("internalclient: do: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("internalclient: read body: %w", err)
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		if respBody == nil || len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, respBody); err != nil {
			return fmt.Errorf("internalclient: decode response: %w", err)
		}
		return nil
	}
	return &Error{Status: res.StatusCode, Body: string(raw), URL: c.cfg.BaseURL + path}
}

// WithRequestID stores a request-id on the context so DoJSON propagates it as
// X-Request-ID on outbound calls.
type ctxKeyRequestID struct{}

func WithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID{}, rid)
}
