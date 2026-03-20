package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter(h *Handler) *gin.Engine {
	r := gin.New()
	r.GET("/health", h.HealthHandler())
	r.GET("/ready", h.ReadyHandler())
	r.GET("/live", h.LiveHandler())
	return r
}

func healthy(_ context.Context) error  { return nil }
func unhealthy(_ context.Context) error { return errors.New("connection refused") }

// --- HealthHandler ---

func TestHealthHandler_AllHealthy(t *testing.T) {
	h := NewHandler("1.0.0")
	h.Register("postgres", healthy, true)
	h.Register("redis", healthy, true)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	newRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "healthy", resp.Status)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.Equal(t, "healthy", resp.Probes["postgres"].Status)
	assert.Equal(t, "healthy", resp.Probes["redis"].Status)
}

func TestHealthHandler_CriticalFail_StillReturns200(t *testing.T) {
	h := NewHandler("1.0.0")
	h.Register("postgres", unhealthy, true)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	newRouter(h).ServeHTTP(w, req)

	// /health always 200, status in body
	assert.Equal(t, http.StatusOK, w.Code)
	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "unhealthy", resp.Status)
	assert.Equal(t, "unhealthy", resp.Probes["postgres"].Status)
	assert.Equal(t, "connection refused", resp.Probes["postgres"].Error)
}

func TestHealthHandler_NonCriticalFail_Degraded(t *testing.T) {
	h := NewHandler("1.0.0")
	h.Register("postgres", healthy, true)
	h.Register("rabbitmq", unhealthy, false)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	newRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "degraded", resp.Status)
}

func TestHealthHandler_NoProbes(t *testing.T) {
	h := NewHandler("2.0.0")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	newRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "healthy", resp.Status)
	assert.Equal(t, "2.0.0", resp.Version)
}

// --- ReadyHandler ---

func TestReadyHandler_AllHealthy(t *testing.T) {
	h := NewHandler("1.0.0")
	h.Register("postgres", healthy, true)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	newRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestReadyHandler_CriticalFail_503(t *testing.T) {
	h := NewHandler("1.0.0")
	h.Register("postgres", unhealthy, true)
	h.Register("redis", healthy, true)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	newRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestReadyHandler_NonCriticalFail_StillReady(t *testing.T) {
	h := NewHandler("1.0.0")
	h.Register("postgres", healthy, true)
	h.Register("rabbitmq", unhealthy, false) // non-critical

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	newRouter(h).ServeHTTP(w, req)

	// Non-critical failure does not block readiness
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- LiveHandler ---

func TestLiveHandler_AlwaysOK(t *testing.T) {
	h := NewHandler("1.0.0")
	// Even with a failing critical probe, /live returns 200
	h.Register("postgres", unhealthy, true)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/live", nil)
	newRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLiveHandler_Body(t *testing.T) {
	h := NewHandler("1.1.0")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/live", nil)
	newRouter(h).ServeHTTP(w, req)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "alive", body["status"])
	assert.Equal(t, "1.1.0", body["version"])
}

// --- ProbeResult critical flag ---

func TestHealthHandler_ProbeResult_CriticalFlag(t *testing.T) {
	h := NewHandler("1.0.0")
	h.Register("db", healthy, true)
	h.Register("mq", healthy, false)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	newRouter(h).ServeHTTP(w, req)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Probes["db"].Critical)
	assert.False(t, resp.Probes["mq"].Critical)
}
