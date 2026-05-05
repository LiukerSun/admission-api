package middleware

import "github.com/gin-gonic/gin"

const (
	ContextUserIDKey   = "user_id"
	ContextRoleKey     = "role"
	ContextIsAdminKey  = "is_admin"
	ContextPlatformKey = "platform"
)

// GetUserID extracts the authenticated user ID from the gin context.
func GetUserID(c *gin.Context) (int64, bool) {
	raw, exists := c.Get(ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := raw.(int64)
	return userID, ok && userID > 0
}
