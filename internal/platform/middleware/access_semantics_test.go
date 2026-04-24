package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubStatusCache struct {
	value string
	err   error
}

func (s stubStatusCache) Get(ctx context.Context, key string) (string, error) {
	return s.value, s.err
}

func TestPremiumRouteRejectsExpiredPremiumTokenWithoutActiveMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)

	jwtConfig := &JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	}
	tokenPair, _, err := GenerateTokenPair(jwtConfig, 7, "premium", "parent", "web")
	require.NoError(t, err)

	r := gin.New()
	r.Use(JWTMiddleware(jwtConfig))
	r.Use(AuthStatusMiddleware(stubStatusCache{}, func(ctx context.Context, userID int64) (string, error) {
		return "active", nil
	}))
	r.Use(RequireMinRole("premium"))
	r.Use(RequireActiveMembership(mockMembershipChecker{active: false}))
	r.GET("/premium", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/premium", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "active membership required")
}

func TestBannedUserIsDeniedEvenWithValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	jwtConfig := &JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	}
	tokenPair, _, err := GenerateTokenPair(jwtConfig, 7, "premium", "parent", "web")
	require.NoError(t, err)

	r := gin.New()
	r.Use(JWTMiddleware(jwtConfig))
	r.Use(AuthStatusMiddleware(stubStatusCache{value: "banned"}, func(ctx context.Context, userID int64) (string, error) {
		return "banned", nil
	}))
	r.GET("/me", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/me", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "account has been banned")
}
