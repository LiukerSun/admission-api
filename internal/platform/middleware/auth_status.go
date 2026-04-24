package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type userStatusCache interface {
	Get(ctx context.Context, key string) (string, error)
}

// UserStatusCacheKey returns the Redis key for cached user status.
func UserStatusCacheKey(userID int64) string {
	return fmt.Sprintf("user_status:%d", userID)
}

// AuthStatusMiddleware checks whether the authenticated user is banned.
// Redis remains the fast path, while persistent lookup covers cache misses
// and Redis availability issues.
func AuthStatusMiddleware(rdb userStatusCache, lookupStatus func(context.Context, int64) (string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDRaw, exists := c.Get(ContextUserIDKey)
		if !exists {
			c.Next()
			return
		}

		userID, ok := userIDRaw.(int64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 1002, "message": "unauthorized"})
			c.Abort()
			return
		}

		status, err := rdb.Get(c.Request.Context(), UserStatusCacheKey(userID))
		if err == nil {
			if status == "banned" {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 1002, "message": "account has been banned"})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		if lookupStatus == nil {
			c.Next()
			return
		}

		status, err = lookupStatus(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1000, "message": "failed to verify account status"})
			c.Abort()
			return
		}
		if status == "banned" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 1002, "message": "account has been banned"})
			c.Abort()
			return
		}

		c.Next()
	}
}
