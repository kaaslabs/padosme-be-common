// Package health provides a probe-based health check framework for PadosMe
// services, exposing standardized /health, /ready, and /live endpoints.
//
// Usage:
//
//	h := health.NewHandler("1.2.0")
//	h.Register("postgres", func(ctx context.Context) error { return db.PingContext(ctx) }, true)
//	h.Register("redis",    func(ctx context.Context) error { return rdb.Ping(ctx).Err() }, true)
//	h.Register("rabbitmq", func(ctx context.Context) error { ... }, false)
//
//	router.GET("/health", h.HealthHandler())
//	router.GET("/ready",  h.ReadyHandler())
//	router.GET("/live",   h.LiveHandler())
package health

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ProbeFunc checks a single dependency. Return nil if healthy, an error otherwise.
type ProbeFunc func(ctx context.Context) error

type probe struct {
	name     string
	fn       ProbeFunc
	critical bool
}

// Handler manages a set of named health probes and generates Gin handlers
// for the standard /health, /ready, and /live endpoints.
type Handler struct {
	version string
	mu      sync.RWMutex
	probes  []probe
}

// NewHandler creates a Handler that reports the given version in all responses.
func NewHandler(version string) *Handler {
	return &Handler{version: version}
}

// Register adds a named probe. Set critical=true for dependencies whose failure
// makes the service unable to serve requests (primary database, etc.).
// Register must not be called concurrently with request handling.
func (h *Handler) Register(name string, fn ProbeFunc, critical bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.probes = append(h.probes, probe{name: name, fn: fn, critical: critical})
}

// ProbeResult is the per-probe section of the health response body.
type ProbeResult struct {
	Status   string `json:"status"`
	Critical bool   `json:"critical,omitempty"`
	Error    string `json:"error,omitempty"`
}

// HealthResponse is the standardized JSON body returned by /health.
type HealthResponse struct {
	Status    string                 `json:"status"`
	Version   string                 `json:"version"`
	Timestamp time.Time              `json:"timestamp"`
	Probes    map[string]ProbeResult `json:"probes,omitempty"`
}

// HealthHandler returns a Gin handler for GET /health.
// Always responds with HTTP 200; the body status field carries the true health.
// Suitable for monitoring systems (Grafana, Prometheus scraping, etc.).
func (h *Handler) HealthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		results, critFail, degraded := h.runProbes(ctx)
		status := "healthy"
		if critFail {
			status = "unhealthy"
		} else if degraded {
			status = "degraded"
		}

		c.JSON(http.StatusOK, HealthResponse{
			Status:    status,
			Version:   h.version,
			Timestamp: time.Now().UTC(),
			Probes:    results,
		})
	}
}

// ReadyHandler returns a Gin handler for GET /ready.
// Returns HTTP 200 when all critical probes pass, HTTP 503 otherwise.
// Use this for Kubernetes readiness probes and load balancer health checks.
func (h *Handler) ReadyHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		results, critFail, _ := h.runProbes(ctx)
		httpCode := http.StatusOK
		status := "ready"
		if critFail {
			httpCode = http.StatusServiceUnavailable
			status = "not ready"
		}

		c.JSON(httpCode, gin.H{
			"status":    status,
			"version":   h.version,
			"timestamp": time.Now().UTC(),
			"probes":    results,
		})
	}
}

// LiveHandler returns a Gin handler for GET /live.
// Always returns HTTP 200 — if the process can handle this request it is alive.
// Use this for Kubernetes liveness probes.
func (h *Handler) LiveHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "alive",
			"version":   h.version,
			"timestamp": time.Now().UTC(),
		})
	}
}

func (h *Handler) runProbes(ctx context.Context) (results map[string]ProbeResult, critFail, degraded bool) {
	h.mu.RLock()
	probes := make([]probe, len(h.probes))
	copy(probes, h.probes)
	h.mu.RUnlock()

	results = make(map[string]ProbeResult, len(probes))
	for _, p := range probes {
		pr := ProbeResult{Status: "healthy", Critical: p.critical}
		if err := p.fn(ctx); err != nil {
			pr.Status = "unhealthy"
			pr.Error = err.Error()
			if p.critical {
				critFail = true
			} else {
				degraded = true
			}
		}
		results[p.name] = pr
	}
	return
}
