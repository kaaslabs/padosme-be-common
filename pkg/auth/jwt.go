package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Context keys set by RequireAuth into the Gin context.
const (
	ContextKeyUserID   = "user_id"
	ContextKeyUserType = "user_type"
	ContextKeyLang     = "lang"
	ContextKeyClaims   = "claims"
)

// User type constants matching the auth-service domain.
const (
	UserTypeUser   = "user"
	UserTypeSeller = "seller"
	UserTypeAdmin  = "admin"
)

// Sentinel errors returned by ParseToken.
var (
	ErrTokenMissing = errors.New("authorization token missing")
	ErrTokenInvalid = errors.New("authorization token invalid")
	ErrTokenExpired = errors.New("authorization token expired")
	ErrForbidden    = errors.New("insufficient permissions")
)

// Claims is the standard JWT payload issued by padosme-auth-service and read by
// all downstream services. The field names match the JWT claim keys exactly.
type Claims struct {
	UserID             uuid.UUID  `json:"user_id"`
	PublicUserID       string     `json:"public_user_id,omitempty"`
	DeviceID           *uuid.UUID `json:"device_id,omitempty"`
	Phone              string     `json:"phone,omitempty"`
	UserType           string     `json:"user_type,omitempty"`
	LanguagePreference string     `json:"lang,omitempty"`
	jwt.RegisteredClaims
}

// ParseToken validates the signed JWT string and returns the decoded Claims.
// Returns ErrTokenExpired if past expiry, ErrTokenInvalid for any other failure.
func ParseToken(tokenString, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalid
		}
		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	if !token.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// RequireAuth returns a Gin middleware that validates the Bearer JWT in the
// Authorization header. On success it sets user_id, user_type, lang, and claims
// into the Gin context. Aborts with 401 on any failure.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			abortUnauthorized(c, "missing authorization header")
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			abortUnauthorized(c, "invalid authorization header format")
			return
		}
		claims, err := ParseToken(parts[1], secret)
		if err != nil {
			if errors.Is(err, ErrTokenExpired) {
				abortUnauthorized(c, "token expired")
			} else {
				abortUnauthorized(c, "invalid token")
			}
			return
		}
		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUserType, claims.UserType)
		c.Set(ContextKeyLang, claims.LanguagePreference)
		c.Set(ContextKeyClaims, claims)
		c.Next()
	}
}

// RequireUserType returns a middleware that permits only the specified user types.
// Must be chained after RequireAuth (or ServiceAuthJWT) so claims are in context.
// Aborts with 403 if the caller's user_type is not in the allowed list.
func RequireUserType(allowedTypes ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedTypes))
	for _, t := range allowedTypes {
		allowed[t] = struct{}{}
	}
	return func(c *gin.Context) {
		ut, _ := c.Get(ContextKeyUserType)
		userType, _ := ut.(string)
		if _, ok := allowed[userType]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":      "insufficient permissions",
				"code":       "FORBIDDEN",
				"request_id": contextRequestID(c),
			})
			return
		}
		c.Next()
	}
}

// GetClaims retrieves the parsed *Claims from the Gin context.
// Returns nil, false if RequireAuth was not applied to this route.
func GetClaims(c *gin.Context) (*Claims, bool) {
	v, exists := c.Get(ContextKeyClaims)
	if !exists {
		return nil, false
	}
	claims, ok := v.(*Claims)
	return claims, ok
}

// GetUserID retrieves the user UUID from the Gin context.
func GetUserID(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get(ContextKeyUserID)
	if !exists {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

func abortUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error":      msg,
		"code":       "UNAUTHORIZED",
		"request_id": contextRequestID(c),
	})
}

func contextRequestID(c *gin.Context) string {
	if id, exists := c.Get("request_id"); exists {
		if s, ok := id.(string); ok {
			return s
		}
	}
	return ""
}
