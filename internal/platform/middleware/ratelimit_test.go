package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// newTestRedis spins up an in-process Redis (via miniredis) and returns
// a real *redis.Client pointed at it. We don't use a fake — exercising
// the actual go-redis client preserves command behavior (Incr/Expire),
// which is the part of the middleware we need to verify.
func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, srv
}

// withFixedUser plants a user ID in the gin context just like the JWT
// middleware would in production, so RateLimitByUser can read it.
func withFixedUser(userID int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ContextUserIDKey, userID)
		c.Next()
	}
}

// TestRateLimitByUserAllowsRequestsUnderLimit confirms the happy path:
// a single user can fire `limit` requests in a window and they all get
// through with 200.
func TestRateLimitByUserAllowsRequestsUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, _ := newTestRedis(t)

	router := gin.New()
	router.Use(withFixedUser(7))
	router.Use(RateLimitByUser(client, 3, time.Minute))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equalf(t, http.StatusOK, rec.Code, "request %d should pass", i+1)
	}
}

// TestRateLimitByUserRejectsAfterLimit confirms the (limit+1)th request
// from the same user is blocked with 429 — this is the actual safety
// guarantee the middleware exists for.
func TestRateLimitByUserRejectsAfterLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, _ := newTestRedis(t)

	router := gin.New()
	router.Use(withFixedUser(7))
	router.Use(RateLimitByUser(client, 2, time.Minute))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equalf(t, http.StatusOK, rec.Code, "request %d should pass", i+1)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTooManyRequests, rec.Code, "third request should be rate-limited")
}

// TestRateLimitByUserScopesPerUser proves the limiter keys by user ID,
// not by IP. User A burning through their quota must NOT cause user B's
// requests to fail. Without this the middleware would let one heavy
// user starve everyone behind the same load balancer.
func TestRateLimitByUserScopesPerUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, _ := newTestRedis(t)

	mwA := RateLimitByUser(client, 1, time.Minute)
	mwB := RateLimitByUser(client, 1, time.Minute)

	routerA := gin.New()
	routerA.Use(withFixedUser(7))
	routerA.Use(mwA)
	routerA.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	routerB := gin.New()
	routerB.Use(withFixedUser(8))
	routerB.Use(mwB)
	routerB.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	// User A spends their single quota.
	{
		req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
		rec := httptest.NewRecorder()
		routerA.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}
	// User A is now blocked.
	{
		req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
		rec := httptest.NewRecorder()
		routerA.ServeHTTP(rec, req)
		require.Equal(t, http.StatusTooManyRequests, rec.Code, "user 7 should be rate-limited")
	}
	// User B must still be able to make their first request.
	{
		req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
		rec := httptest.NewRecorder()
		routerB.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "user 8 must not be affected by user 7's quota")
	}
}

// TestRateLimitByUserEmitsHeaders proves the middleware sets the
// standard X-RateLimit-Limit / X-RateLimit-Remaining headers so clients
// can self-pace before they get a 429.
func TestRateLimitByUserEmitsHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, _ := newTestRedis(t)

	router := gin.New()
	router.Use(withFixedUser(7))
	router.Use(RateLimitByUser(client, 5, time.Minute))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "5", rec.Header().Get("X-RateLimit-Limit"))
	remaining, err := strconv.Atoi(rec.Header().Get("X-RateLimit-Remaining"))
	require.NoError(t, err)
	require.Equal(t, 4, remaining, "first request should leave 4 remaining of 5")
}

// TestRateLimitByUserFallsBackToIPWhenAnonymous documents the defense-
// in-depth behavior: if the middleware is mounted on a route that
// somehow lacks a user (misconfiguration), it must STILL rate-limit
// instead of silently letting unbounded traffic through. We key by
// remote IP in that case.
func TestRateLimitByUserFallsBackToIPWhenAnonymous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, _ := newTestRedis(t)

	router := gin.New()
	// Note: no withFixedUser — anonymous request.
	router.Use(RateLimitByUser(client, 1, time.Minute))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	{
		req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
		req.RemoteAddr = "203.0.113.10:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}
	{
		req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
		req.RemoteAddr = "203.0.113.10:12346"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusTooManyRequests, rec.Code, "second anonymous hit from same IP should be limited")
	}
}
