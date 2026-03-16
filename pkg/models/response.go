package models

// ErrorResponse is the standard error response format for all PadosMe services.
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id"`
}

// SuccessResponse wraps any successful response payload with a request ID.
type SuccessResponse struct {
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id"`
}
