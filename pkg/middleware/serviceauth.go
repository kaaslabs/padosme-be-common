package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pkgauth "github.com/kaaslabs/padosme-be-common/v4/pkg/auth"
)

const HeaderServiceToken = "X-Service-Token"

// ServiceAuth validates the static X-Service-Token header against expectedToken.
// Use this for simple service-to-service authentication with a shared secret.
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

// ServiceAuthJWT validates a Bearer JWT in the Authorization header.
// On success it populates user_id, user_type, lang, and claims into the Gin
// context — identical to pkg/auth.RequireAuth but named for service-to-service use.
func ServiceAuthJWT(secret string) gin.HandlerFunc {
	return pkgauth.RequireAuth(secret)
}

// RequireRole returns a middleware that permits only the specified user types.
// Must be chained after ServiceAuthJWT or pkg/auth.RequireAuth.
// Aborts with 403 if the caller's user_type is not in the allowed list.
func RequireRole(roles ...string) gin.HandlerFunc {
	return pkgauth.RequireUserType(roles...)
}
