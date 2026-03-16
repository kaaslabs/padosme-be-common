package errors

import (
	"fmt"
	"net/http"
)

// Standard error codes used across all PadosMe services.
const (
	ErrInvalidRequest  = "INVALID_REQUEST"
	ErrUnauthorized    = "UNAUTHORIZED"
	ErrRateLimited     = "RATE_LIMITED"
	ErrIntentFailed    = "INTENT_FAILED"
	ErrSearchFailed    = "SEARCH_FAILED"
	ErrTranslateFailed = "TRANSLATE_FAILED"
	ErrTimeout         = "TIMEOUT"
	ErrInternalError   = "INTERNAL_ERROR"
)

// AppError is the standard error type returned by all PadosMe services.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"error"`
	RequestID  string `json:"request_id"`
	StatusCode int    `json:"-"`
}

func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s (request_id=%s)", e.Code, e.Message, e.RequestID)
}

// New creates an AppError with the given HTTP status code, error code, and message.
func New(statusCode int, code, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
	}
}

// WithRequestID attaches a request ID to an existing AppError (returns a copy).
func (e *AppError) WithRequestID(requestID string) *AppError {
	return &AppError{
		Code:       e.Code,
		Message:    e.Message,
		RequestID:  requestID,
		StatusCode: e.StatusCode,
	}
}

// Predefined constructors for common errors.

func InvalidRequest(message, requestID string) *AppError {
	return &AppError{
		Code:       ErrInvalidRequest,
		Message:    message,
		RequestID:  requestID,
		StatusCode: http.StatusBadRequest,
	}
}

func Unauthorized(message, requestID string) *AppError {
	return &AppError{
		Code:       ErrUnauthorized,
		Message:    message,
		RequestID:  requestID,
		StatusCode: http.StatusUnauthorized,
	}
}

func RateLimited(message, requestID string) *AppError {
	return &AppError{
		Code:       ErrRateLimited,
		Message:    message,
		RequestID:  requestID,
		StatusCode: http.StatusTooManyRequests,
	}
}

func IntentFailed(message, requestID string) *AppError {
	return &AppError{
		Code:       ErrIntentFailed,
		Message:    message,
		RequestID:  requestID,
		StatusCode: http.StatusBadGateway,
	}
}

func SearchFailed(message, requestID string) *AppError {
	return &AppError{
		Code:       ErrSearchFailed,
		Message:    message,
		RequestID:  requestID,
		StatusCode: http.StatusBadGateway,
	}
}

func TranslateFailed(message, requestID string) *AppError {
	return &AppError{
		Code:       ErrTranslateFailed,
		Message:    message,
		RequestID:  requestID,
		StatusCode: http.StatusBadGateway,
	}
}

func Timeout(message, requestID string) *AppError {
	return &AppError{
		Code:       ErrTimeout,
		Message:    message,
		RequestID:  requestID,
		StatusCode: http.StatusGatewayTimeout,
	}
}

func Internal(message, requestID string) *AppError {
	return &AppError{
		Code:       ErrInternalError,
		Message:    message,
		RequestID:  requestID,
		StatusCode: http.StatusInternalServerError,
	}
}
