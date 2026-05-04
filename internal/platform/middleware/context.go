package middleware

import "github.com/gin-gonic/gin"

const (
	ContextUserIDKey   = "user_id"
	ContextRoleKey     = "role"
	ContextIsAdminKey  = "is_admin"
	ContextUserTypeKey = "user_type"
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

// GetUserType extracts the user type from the gin context.
func GetUserType(c *gin.Context) (string, bool) {
	raw, exists := c.Get(ContextUserTypeKey)
	if !exists {
		return "", false
	}
	userType, ok := raw.(string)
	return userType, ok
}
