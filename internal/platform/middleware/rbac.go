package middleware

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

// Role ranking: user < premium < admin
var roleRank = map[string]int{
	"user":    1,
	"premium": 2,
	"admin":   3,
}

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

// RequireMinRole allows access if the user's role rank is >= the minimum required role.
// For example, RequireMinRole("premium") allows both "premium" and "admin".
func RequireMinRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(ContextRoleKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 1002, "message": "unauthorized"})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"code": 1003, "message": "forbidden"})
			c.Abort()
			return
		}

		userRank, userOk := roleRank[roleStr]
		minRank, minOk := roleRank[minRole]
		if !userOk || !minOk || userRank < minRank {
			c.JSON(http.StatusForbidden, gin.H{"code": 1003, "message": "forbidden"})
			c.Abort()
			return
		}

		c.Next()
	}
}
