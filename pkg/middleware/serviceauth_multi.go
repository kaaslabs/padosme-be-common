package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const contextKeyCaller = "service_caller"

// ServiceAuthMulti validates X-Service-Token against a map of token -> caller
// name. On match the caller name is stored on the Gin context (key
// "service_caller") for audit logging. Loaded tokens typically come from the
// config table rather than environment variables.
func ServiceAuthMulti(tokens map[string]string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader(HeaderServiceToken)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":      "missing service token",
				"code":       "UNAUTHORIZED",
				"request_id": GetRequestID(c),
			})
			return
		}
		caller, ok := tokens[token]
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":      "invalid service token",
				"code":       "UNAUTHORIZED",
				"request_id": GetRequestID(c),
			})
			return
		}
		c.Set(contextKeyCaller, caller)
		c.Next()
	}
}

// Caller returns the authenticated caller name set by ServiceAuthMulti.
func Caller(c *gin.Context) string {
	if v, ok := c.Get(contextKeyCaller); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
