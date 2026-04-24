package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubUserStatusCache struct {
	value string
	err   error
}

func (s stubUserStatusCache) Get(ctx context.Context, key string) (string, error) {
	return s.value, s.err
}

func TestAuthStatusMiddlewareRejectsBannedUserFromCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	lookupCalled := false
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextUserIDKey, int64(7))
		c.Next()
	})
	r.GET("/me", AuthStatusMiddleware(stubUserStatusCache{value: "banned"}, func(ctx context.Context, userID int64) (string, error) {
		lookupCalled = true
		return "active", nil
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", http.NoBody)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "account has been banned")
	assert.False(t, lookupCalled)
}

func TestAuthStatusMiddlewareFallsBackToPersistentStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextUserIDKey, int64(7))
		c.Next()
	})
	r.GET("/me", AuthStatusMiddleware(stubUserStatusCache{err: errors.New("redis unavailable")}, func(ctx context.Context, userID int64) (string, error) {
		return "banned", nil
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", http.NoBody)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "account has been banned")
}

func TestAuthStatusMiddlewareReturnsServerErrorWhenFallbackFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextUserIDKey, int64(7))
		c.Next()
	})
	r.GET("/me", AuthStatusMiddleware(stubUserStatusCache{err: errors.New("redis unavailable")}, func(ctx context.Context, userID int64) (string, error) {
		return "", errors.New("db unavailable")
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", http.NoBody)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to verify account status")
}
