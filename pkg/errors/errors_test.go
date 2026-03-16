package errors_test

import (
	"net/http"
	"testing"

	"github.com/kaaslabs/padosme-be-common/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	err := errors.New(http.StatusBadRequest, errors.ErrInvalidRequest, "bad field")
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, errors.ErrInvalidRequest, err.Code)
	assert.Equal(t, "bad field", err.Message)
	assert.Empty(t, err.RequestID)
}

func TestWithRequestID(t *testing.T) {
	err := errors.New(http.StatusBadRequest, errors.ErrInvalidRequest, "bad field")
	withID := err.WithRequestID("req-123")
	assert.Equal(t, "req-123", withID.RequestID)
	// original unchanged
	assert.Empty(t, err.RequestID)
}

func TestError_String(t *testing.T) {
	err := errors.Unauthorized("token expired", "req-abc")
	assert.Contains(t, err.Error(), "UNAUTHORIZED")
	assert.Contains(t, err.Error(), "req-abc")
}

func TestPredefinedConstructors(t *testing.T) {
	tests := []struct {
		name       string
		err        *errors.AppError
		code       string
		httpStatus int
	}{
		{"InvalidRequest", errors.InvalidRequest("msg", "r1"), errors.ErrInvalidRequest, http.StatusBadRequest},
		{"Unauthorized", errors.Unauthorized("msg", "r1"), errors.ErrUnauthorized, http.StatusUnauthorized},
		{"RateLimited", errors.RateLimited("msg", "r1"), errors.ErrRateLimited, http.StatusTooManyRequests},
		{"IntentFailed", errors.IntentFailed("msg", "r1"), errors.ErrIntentFailed, http.StatusBadGateway},
		{"SearchFailed", errors.SearchFailed("msg", "r1"), errors.ErrSearchFailed, http.StatusBadGateway},
		{"TranslateFailed", errors.TranslateFailed("msg", "r1"), errors.ErrTranslateFailed, http.StatusBadGateway},
		{"Timeout", errors.Timeout("msg", "r1"), errors.ErrTimeout, http.StatusGatewayTimeout},
		{"Internal", errors.Internal("msg", "r1"), errors.ErrInternalError, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.StatusCode)
			assert.Equal(t, "r1", tt.err.RequestID)
		})
	}
}
