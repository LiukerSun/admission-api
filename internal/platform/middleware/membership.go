package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ActiveMembershipChecker interface {
	HasActiveMembership(ctx context.Context, userID int64) (bool, error)
}

func RequireActiveMembership(checker ActiveMembershipChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDRaw, exists := c.Get(ContextUserIDKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 1002, "message": "unauthorized"})
			c.Abort()
			return
		}

		userID, ok := userIDRaw.(int64)
		if !ok || userID <= 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 1002, "message": "unauthorized"})
			c.Abort()
			return
		}

		active, err := checker.HasActiveMembership(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 5000, "message": "membership check failed"})
			c.Abort()
			return
		}
		if !active {
			c.JSON(http.StatusForbidden, gin.H{"code": 1003, "message": "active membership required"})
			c.Abort()
			return
		}

		c.Next()
	}
}
