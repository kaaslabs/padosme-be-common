package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-for-unit-tests-padosme"

func makeToken(t *testing.T, claims *Claims, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func validClaims(userID uuid.UUID, userType string) *Claims {
	return &Claims{
		UserID:   userID,
		UserType: userType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
}

// --- ParseToken ---

func TestParseToken_Valid(t *testing.T) {
	userID := uuid.New()
	tokenStr := makeToken(t, validClaims(userID, UserTypeUser), testSecret)

	got, err := ParseToken(tokenStr, testSecret)
	require.NoError(t, err)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, UserTypeUser, got.UserType)
}

func TestParseToken_WithLangAndPublicID(t *testing.T) {
	userID := uuid.New()
	claims := &Claims{
		UserID:             userID,
		PublicUserID:       "Ab3k-Xy9z-Mn7p",
		UserType:           UserTypeSeller,
		LanguagePreference: "hi",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	tokenStr := makeToken(t, claims, testSecret)

	got, err := ParseToken(tokenStr, testSecret)
	require.NoError(t, err)
	assert.Equal(t, "Ab3k-Xy9z-Mn7p", got.PublicUserID)
	assert.Equal(t, "hi", got.LanguagePreference)
}

func TestParseToken_Expired(t *testing.T) {
	claims := &Claims{
		UserID: uuid.New(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
		},
	}
	tokenStr := makeToken(t, claims, testSecret)

	_, err := ParseToken(tokenStr, testSecret)
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestParseToken_WrongSecret(t *testing.T) {
	tokenStr := makeToken(t, validClaims(uuid.New(), UserTypeUser), testSecret)
	_, err := ParseToken(tokenStr, "wrong-secret")
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestParseToken_Malformed(t *testing.T) {
	_, err := ParseToken("not.a.jwt", testSecret)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestParseToken_WrongAlgorithm(t *testing.T) {
	// Build a token with RS256 header manually to trigger algorithm rejection.
	_, err := ParseToken("eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.e30.signature", testSecret)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

// --- RequireAuth middleware ---

func setupRouter(secret string, handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", handlers...)
	return r
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	r := setupRouter(testSecret, RequireAuth(testSecret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_InvalidFormat(t *testing.T) {
	r := setupRouter(testSecret, RequireAuth(testSecret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Token abc123")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_ValidToken_SetsContext(t *testing.T) {
	userID := uuid.New()
	tokenStr := makeToken(t, validClaims(userID, UserTypeSeller), testSecret)

	var (
		gotID   uuid.UUID
		gotType string
	)
	r := setupRouter(testSecret, RequireAuth(testSecret), func(c *gin.Context) {
		gotID, _ = GetUserID(c)
		ut, _ := c.Get(ContextKeyUserType)
		gotType, _ = ut.(string)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, userID, gotID)
	assert.Equal(t, UserTypeSeller, gotType)
}

func TestRequireAuth_CaseInsensitiveBearer(t *testing.T) {
	tokenStr := makeToken(t, validClaims(uuid.New(), UserTypeUser), testSecret)
	r := setupRouter(testSecret, RequireAuth(testSecret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "BEARER "+tokenStr)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	claims := &Claims{
		UserID: uuid.New(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
		},
	}
	tokenStr := makeToken(t, claims, testSecret)
	r := setupRouter(testSecret, RequireAuth(testSecret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- RequireUserType middleware ---

func TestRequireUserType_Allowed(t *testing.T) {
	tokenStr := makeToken(t, validClaims(uuid.New(), UserTypeAdmin), testSecret)
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.GET("/test", RequireAuth(testSecret), RequireUserType(UserTypeAdmin), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireUserType_MultipleAllowed(t *testing.T) {
	tokenStr := makeToken(t, validClaims(uuid.New(), UserTypeSeller), testSecret)
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.GET("/test", RequireAuth(testSecret), RequireUserType(UserTypeAdmin, UserTypeSeller), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireUserType_Forbidden(t *testing.T) {
	tokenStr := makeToken(t, validClaims(uuid.New(), UserTypeUser), testSecret)
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.GET("/test", RequireAuth(testSecret), RequireUserType(UserTypeAdmin), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- GetClaims / GetUserID helpers ---

func TestGetClaims_Present(t *testing.T) {
	userID := uuid.New()
	tokenStr := makeToken(t, validClaims(userID, UserTypeUser), testSecret)

	var gotClaims *Claims
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.GET("/test", RequireAuth(testSecret), func(c *gin.Context) {
		gotClaims, _ = GetClaims(c)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	r.ServeHTTP(w, req)

	require.NotNil(t, gotClaims)
	assert.Equal(t, userID, gotClaims.UserID)
}

func TestGetClaims_Absent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		claims, ok := GetClaims(c)
		assert.False(t, ok)
		assert.Nil(t, claims)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
}
