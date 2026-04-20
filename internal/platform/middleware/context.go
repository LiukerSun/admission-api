package middleware

const (
	ContextUserIDKey    = "user_id"
	ContextRoleKey      = "role"
	ContextUserTypeKey  = "user_type"
	ContextPlatformKey  = "platform"
)

func UserFromContext(c any) (userID int64, role string, ok bool) {
	// This helper is kept for backward compatibility.
	// Handlers should use c.Get(ContextUserIDKey) directly with gin.
	return 0, "", false
}
