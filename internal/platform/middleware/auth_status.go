package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"admission-api/internal/platform/redis"
)

// UserStatusCacheKey returns the Redis key for cached user status.
func UserStatusCacheKey(userID int64) string {
	return fmt.Sprintf("user_status:%d", userID)
}

// AuthStatusMiddleware checks if the user is banned via Redis cache.
// The admin service sets "banned" in Redis when disabling a user.
// If the key does not exist, the user is assumed active.
func AuthStatusMiddleware(rdb *redis.Client) gin.HandlerFunc {
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
		if err == nil && status == "banned" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 1002, "message": "account has been banned"})
			c.Abort()
			return
		}

		c.Next()
	}
}
