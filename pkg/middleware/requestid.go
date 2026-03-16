package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const HeaderRequestID = "X-Request-ID"

// RequestID middleware ensures every request has a unique X-Request-ID header.
// If the client provides one it is reused; otherwise a new UUID is generated.
// The value is stored in the Gin context under the key "request_id".
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(HeaderRequestID)
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("request_id", requestID)
		c.Header(HeaderRequestID, requestID)
		c.Next()
	}
}

// GetRequestID retrieves the request ID from the Gin context.
// Returns an empty string if not set.
func GetRequestID(c *gin.Context) string {
	if v, exists := c.Get("request_id"); exists {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}
