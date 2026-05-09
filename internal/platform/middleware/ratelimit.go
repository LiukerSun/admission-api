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
	return func(c *gin.Context) {
		ip := extractIP(c.Request)
		key := "ratelimit:" + ip
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

// RateLimitByUser is a Redis-backed sliding-counter rate limiter keyed
// by authenticated user ID. Compared to RateLimitMiddleware (keyed by
// IP), this one survives users behind shared NATs / VPNs / corporate
// proxies that all present the same IP, and lets us bound the cost of
// expensive per-user endpoints — most importantly the AI chat
// endpoints, which can issue many LLM calls per request.
//
// If the request is anonymous (no user ID in the gin context) the
// limiter falls back to keying by IP so a misconfigured route still
// bounds traffic instead of silently allowing unbounded requests.
func RateLimitByUser(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var bucket string
		if uid, ok := GetUserID(c); ok {
			bucket = "user:" + strconv.FormatInt(uid, 10)
		} else {
			bucket = "ip:" + extractIP(c.Request)
		}

		key := "ratelimit:" + bucket
		ctx := c.Request.Context()

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			// Don't fail the request if Redis is down — rate limiting is
			// a best-effort safeguard, not a correctness requirement.
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
			c.JSON(http.StatusTooManyRequests, gin.H{"code": 1001, "message": "too many requests"})
			c.Abort()
			return
		}

		c.Next()
	}
}
