package middleware

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(ContextRoleKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 1002, "message": "unauthorized"})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || !slices.Contains(roles, roleStr) {
			c.JSON(http.StatusForbidden, gin.H{"code": 1003, "message": "forbidden"})
			c.Abort()
			return
		}

		c.Next()
	}
}
