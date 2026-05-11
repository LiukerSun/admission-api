package middleware

import (
	"net"
	"strconv"
	"strings"
	"time"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RateLimitMiddleware(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return RateLimitWithKey(rdb, "ratelimit", limit, window, func(c *gin.Context) string {
		return extractIP(c.Request)
	})
}

// RateLimitWithKey is a configurable rate limiter that delegates to a caller-
// supplied key extractor. The Redis key format is "<prefix>:<extractor()>".
// Empty keys are treated as no-op (caller-provided key extractor decides
// fallback semantics).
func RateLimitWithKey(rdb *redis.Client, prefix string, limit int, window time.Duration, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := keyFn(c)
		if raw == "" {
			c.Next()
			return
		}
		key := prefix + ":" + raw
		ctx := c.Request.Context()

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			c.Next()
			return
		}

		if count == 1 {
			_ = rdb.Expire(ctx, key, window).Err()
		}

		remaining := int64(limit) - count
		if remaining < 0 {
			remaining = 0
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if count > int64(limit) {
			c.JSON(429, gin.H{"code": 1001, "message": "too many requests"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// UserRateLimitMiddleware enforces a per-user, shared-bucket rate limit. All
// middlewares constructed with the same `prefix` share a single Redis counter
// per user, so multiple AI endpoints can be grouped under one quota by passing
// the same prefix (e.g. "ratelimit:ai"). Falls back to the client IP when the
// JWT middleware has not populated the user ID (defensive — should not happen
// if mounted after JWTMiddleware).
func UserRateLimitMiddleware(rdb *redis.Client, prefix string, limit int, window time.Duration) gin.HandlerFunc {
	return RateLimitWithKey(rdb, prefix, limit, window, func(c *gin.Context) string {
		raw, ok := c.Get(ContextUserIDKey)
		if !ok {
			return "anon:" + extractIP(c.Request)
		}
		uid, ok := raw.(int64)
		if !ok || uid <= 0 {
			return "anon:" + extractIP(c.Request)
		}
		return "user:" + strconv.FormatInt(uid, 10)
	})
}

func extractIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	xri := r.Header.Get("X-Real-Ip")
	if xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
