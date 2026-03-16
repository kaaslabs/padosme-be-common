package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const HeaderServiceToken = "X-Service-Token"

// ServiceAuth returns a Gin middleware that validates the X-Service-Token header
// against the expected token. Requests without a valid token receive 401.
//
// This is used by internal services (query-intelligence, search-engine, ai-service)
// to reject requests that did not originate from another PadosMe service.
func ServiceAuth(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader(HeaderServiceToken)
		if token == "" || token != expectedToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":      "missing or invalid service token",
				"code":       "UNAUTHORIZED",
				"request_id": GetRequestID(c),
			})
			return
		}
		c.Next()
	}
}
