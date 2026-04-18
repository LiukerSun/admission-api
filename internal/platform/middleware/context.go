package middleware

import "context"

type contextKey string

const (
	ContextUserIDKey   contextKey = "user_id"
	ContextRoleKey     contextKey = "role"
	ContextPlatformKey contextKey = "platform"
)

func UserFromContext(ctx context.Context) (userID int64, role string, ok bool) {
	uid, ok1 := ctx.Value(ContextUserIDKey).(int64)
	r, ok2 := ctx.Value(ContextRoleKey).(string)
	return uid, r, ok1 && ok2
}

func PlatformFromContext(ctx context.Context) string {
	p, ok := ctx.Value(ContextPlatformKey).(string)
	if !ok || p == "" {
		return "web"
	}
	return p
}
