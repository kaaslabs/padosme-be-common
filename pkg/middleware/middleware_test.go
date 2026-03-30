package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kaaslabs/padosme-be-common/v4/pkg/middleware"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── RequestID ────────────────────────────────────────────────────────────────

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, middleware.GetRequestID(c))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())
	assert.NotEmpty(t, w.Header().Get(middleware.HeaderRequestID))
}

func TestRequestID_ReusesClientProvided(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, middleware.GetRequestID(c))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(middleware.HeaderRequestID, "my-request-id")
	r.ServeHTTP(w, req)

	assert.Equal(t, "my-request-id", w.Body.String())
	assert.Equal(t, "my-request-id", w.Header().Get(middleware.HeaderRequestID))
}

// ── ServiceAuth ───────────────────────────────────────────────────────────────

func TestServiceAuth_AllowsValidToken(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ServiceAuth("secret-token"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(middleware.HeaderServiceToken, "secret-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestServiceAuth_RejectsMissingToken(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ServiceAuth("secret-token"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestServiceAuth_RejectsWrongToken(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ServiceAuth("secret-token"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(middleware.HeaderServiceToken, "wrong-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
