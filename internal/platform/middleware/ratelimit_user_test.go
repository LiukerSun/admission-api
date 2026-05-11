package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	return rdb, func() { _ = rdb.Close() }
}

func TestUserRateLimitMiddleware_SharedBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, cleanup := newTestRedis(t)
	defer cleanup()

	mw := UserRateLimitMiddleware(rdb, "ratelimit:ai", 2, time.Minute)

	hit := func(uid int64) int {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(ContextUserIDKey, uid)
			c.Next()
		})
		r.Use(mw)
		r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })
		req := httptest.NewRequest("GET", "/x", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if c := hit(7); c != 200 {
		t.Fatalf("call 1 = %d", c)
	}
	if c := hit(7); c != 200 {
		t.Fatalf("call 2 = %d", c)
	}
	if c := hit(7); c != 429 {
		t.Fatalf("call 3 should be rate-limited, got %d", c)
	}
	// A different user has its own bucket.
	if c := hit(99); c != 200 {
		t.Fatalf("different user should pass: %d", c)
	}
}

func TestUserRateLimitMiddleware_FallsBackToIPWithoutUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, cleanup := newTestRedis(t)
	defer cleanup()

	mw := UserRateLimitMiddleware(rdb, "ratelimit:ai", 1, time.Minute)
	r := gin.New()
	r.Use(mw)
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "1.2.3.4:55555"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("first call expected 200, got %d", w.Code)
	}

	req2 := httptest.NewRequest("GET", "/x", nil)
	req2.RemoteAddr = "1.2.3.4:55556"
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 429 {
		t.Fatalf("second call expected 429 (same anon IP), got %d", w2.Code)
	}
}
