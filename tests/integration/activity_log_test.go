//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"admission-api/internal/candidate"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestActivityLogIntegration(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "integration-test-secret")
	}

	ctx := context.Background()
	cfg, err := config.Load()
	require.NoError(t, err)

	database, err := db.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer database.Close()

	redisClient, err := redis.New(cfg.RedisAddr)
	require.NoError(t, err)
	defer redisClient.Close()

	// Clean up
	// cleanupActivityLogs(t, database)
	_, _ = database.Pool().Exec(ctx, "DELETE FROM users WHERE email IN ($1, $2)", "actlog-admin@example.com", "actlog-user@example.com")

	adminID := createIntegrationUser(t, database, "actlog-admin@example.com", "student")
	userID := createIntegrationUser(t, database, "actlog-user@example.com", "student")

	// Seed activity logs directly into DB
	seedActivityLogs(t, database, userID)

	router := newActivityLogIntegrationRouter(t, database, redisClient, cfg)
	adminToken := issueAccessToken(t, cfg, adminID, "admin", "student")
	userToken := issueAccessToken(t, cfg, userID, "user", "student")

	// 1. Admin list activities with filter
	listResp := performActivityLogRequest(t, router, http.MethodGet, "/api/v1/admin/candidate/activities?activity_type=view_school&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, listResp.Code)

	var listEnvelope web.Response
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listEnvelope))
	require.Equal(t, 0, listEnvelope.Code)

	var listResult candidate.ActivityLogListResponse
	listData, _ := json.Marshal(listEnvelope.Data)
	require.NoError(t, json.Unmarshal(listData, &listResult))
	require.GreaterOrEqual(t, listResult.Total, int64(2))

	// 2. Admin list with user_id filter
	userFilterResp := performActivityLogRequest(t, router, http.MethodGet, "/api/v1/admin/candidate/activities?user_id="+itoa64(userID)+"&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, userFilterResp.Code)

	var userFilterEnvelope web.Response
	require.NoError(t, json.Unmarshal(userFilterResp.Body.Bytes(), &userFilterEnvelope))
	require.Equal(t, 0, userFilterEnvelope.Code)

	var userFilterResult candidate.ActivityLogListResponse
	userFilterData, _ := json.Marshal(userFilterEnvelope.Data)
	require.NoError(t, json.Unmarshal(userFilterData, &userFilterResult))
	require.GreaterOrEqual(t, userFilterResult.Total, int64(2))

	// 3. Admin list with target_type filter
	targetFilterResp := performActivityLogRequest(t, router, http.MethodGet, "/api/v1/admin/candidate/activities?target_type=school&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, targetFilterResp.Code)

	var targetFilterEnvelope web.Response
	require.NoError(t, json.Unmarshal(targetFilterResp.Body.Bytes(), &targetFilterEnvelope))
	require.Equal(t, 0, targetFilterEnvelope.Code)

	var targetFilterResult candidate.ActivityLogListResponse
	targetFilterData, _ := json.Marshal(targetFilterEnvelope.Data)
	require.NoError(t, json.Unmarshal(targetFilterData, &targetFilterResult))
	require.GreaterOrEqual(t, targetFilterResult.Total, int64(1))

	// 4. User get my activities
	myResp := performActivityLogRequest(t, router, http.MethodGet, "/api/v1/me/activities?page=1&page_size=20", nil, userToken)
	require.Equal(t, http.StatusOK, myResp.Code)

	var myEnvelope web.Response
	require.NoError(t, json.Unmarshal(myResp.Body.Bytes(), &myEnvelope))
	require.Equal(t, 0, myEnvelope.Code)

	var myResult candidate.ActivityLogListResponse
	myData, _ := json.Marshal(myEnvelope.Data)
	require.NoError(t, json.Unmarshal(myData, &myResult))
	require.GreaterOrEqual(t, myResult.Total, int64(2))

	// 5. Get stats
	statsResp := performActivityLogRequest(t, router, http.MethodGet, "/api/v1/admin/candidate/activities/stats?target_type=school&target_id=100", nil, adminToken)
	require.Equal(t, http.StatusOK, statsResp.Code)

	var statsEnvelope web.Response
	require.NoError(t, json.Unmarshal(statsResp.Body.Bytes(), &statsEnvelope))
	require.Equal(t, 0, statsEnvelope.Code)

	var statsResult candidate.ActivityStatsResponse
	statsData, _ := json.Marshal(statsEnvelope.Data)
	require.NoError(t, json.Unmarshal(statsData, &statsResult))
	require.Equal(t, "school", statsResult.TargetType)
	require.Equal(t, int64(100), statsResult.TargetID)
	require.Equal(t, int64(2), statsResult.Count)

	// 6. Get stats - invalid params
	invalidStatsResp := performActivityLogRequest(t, router, http.MethodGet, "/api/v1/admin/candidate/activities/stats?target_type=school&target_id=abc", nil, adminToken)
	require.Equal(t, http.StatusBadRequest, invalidStatsResp.Code)

	// 7. Delete by IDs
	var logIDs []int64
	for _, log := range listResult.Logs {
		logIDs = append(logIDs, log.ID)
	}
	require.GreaterOrEqual(t, len(logIDs), 1)

	deleteIDsPayload := map[string][]int64{"ids": logIDs[:1]}
	deleteResp := performActivityLogRequest(t, router, http.MethodDelete, "/api/v1/admin/candidate/activities", deleteIDsPayload, adminToken)
	require.Equal(t, http.StatusOK, deleteResp.Code)

	var deleteEnvelope web.Response
	require.NoError(t, json.Unmarshal(deleteResp.Body.Bytes(), &deleteEnvelope))
	require.Equal(t, 0, deleteEnvelope.Code)

	// 8. Delete before time
	beforePayload := map[string]string{"before": time.Now().Add(24 * time.Hour).Format(time.RFC3339)}
	beforeResp := performActivityLogRequest(t, router, http.MethodDelete, "/api/v1/admin/candidate/activities/before", beforePayload, adminToken)
	require.Equal(t, http.StatusOK, beforeResp.Code)

	var beforeEnvelope web.Response
	require.NoError(t, json.Unmarshal(beforeResp.Body.Bytes(), &beforeEnvelope))
	require.Equal(t, 0, beforeEnvelope.Code)

	// 9. List after deletion should be empty
	finalListResp := performActivityLogRequest(t, router, http.MethodGet, "/api/v1/admin/candidate/activities?page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, finalListResp.Code)

	var finalListEnvelope web.Response
	require.NoError(t, json.Unmarshal(finalListResp.Body.Bytes(), &finalListEnvelope))
	require.Equal(t, 0, finalListEnvelope.Code)

	var finalListResult candidate.ActivityLogListResponse
	finalListData, _ := json.Marshal(finalListEnvelope.Data)
	require.NoError(t, json.Unmarshal(finalListData, &finalListResult))
	require.Equal(t, int64(0), finalListResult.Total)
}

func newActivityLogIntegrationRouter(t *testing.T, database *db.DB, redisClient *redis.Client, cfg *config.Config) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  0,
		RefreshTTL: 0,
	}

	activityLogStore := candidate.NewActivityLogStore(database.Pool())
	activityLogService := candidate.NewActivityLogService(activityLogStore, redisClient.RDB())
	activityLogHandler := candidate.NewActivityLogHandler(activityLogService)

	userStore := userStoreForActivityLog(database)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Platform)

	api := r.Group("/api/v1")
	authorized := api.Group("")
	authorized.Use(middleware.JWTMiddleware(jwtConfig))
	authorized.Use(middleware.AuthStatusMiddleware(redisClient, func(ctx context.Context, userID int64) (string, error) {
		u, err := userStore.GetByID(ctx, userID)
		if err != nil {
			return "", err
		}
		return u.Status, nil
	}))

	authorized.GET("/me/activities", activityLogHandler.GetMyActivities)

	adminRoutes := authorized.Group("/admin")
	adminRoutes.Use(middleware.RequireRole("admin"))
	adminRoutes.GET("/candidate/activities", activityLogHandler.ListActivities)
	adminRoutes.GET("/candidate/activities/stats", activityLogHandler.GetStats)
	adminRoutes.DELETE("/candidate/activities", activityLogHandler.DeleteByIDs)
	adminRoutes.DELETE("/candidate/activities/before", activityLogHandler.DeleteBefore)

	return r
}

// userStoreForActivityLog creates a minimal user store for auth status middleware.
func userStoreForActivityLog(database *db.DB) interface {
	GetByID(ctx context.Context, userID int64) (struct{ Status string }, error)
} {
	return &simpleUserStore{database: database}
}

type simpleUserStore struct {
	database *db.DB
}

func (s *simpleUserStore) GetByID(ctx context.Context, userID int64) (struct{ Status string }, error) {
	var result struct{ Status string }
	err := s.database.Pool().QueryRow(ctx, "SELECT status FROM users WHERE id = $1", userID).Scan(&result.Status)
	return result, err
}

func performActivityLogRequest(t *testing.T, router http.Handler, method, path string, payload any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	if payload != nil {
		require.NoError(t, json.NewEncoder(&body).Encode(payload))
	}

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("X-Platform", "web")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func seedActivityLogs(t *testing.T, database *db.DB, userID int64) {
	t.Helper()

	ctx := context.Background()
	_, err := database.Pool().Exec(ctx, `
		INSERT INTO candidate_activity_logs (user_id, activity_type, target_type, target_id, metadata, created_at)
		VALUES
			($1, 'view_school', 'school', 100, '{"school_name": "Test School 1"}', NOW()),
			($1, 'view_school', 'school', 100, '{"school_name": "Test School 1"}', NOW()),
			($1, 'view_major', 'major', 200, '{"major_name": "CS"}', NOW())
	`, userID)
	require.NoError(t, err)
}

func TestActivityLogConsumer(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "integration-test-secret")
	}

	ctx := context.Background()
	cfg, err := config.Load()
	require.NoError(t, err)

	database, err := db.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer database.Close()

	redisClient, err := redis.New(cfg.RedisAddr)
	require.NoError(t, err)
	defer redisClient.Close()

	// Clean up
	// cleanupActivityLogs(t, database)
	_, _ = database.Pool().Exec(ctx, "DELETE FROM users WHERE email = $1", "actlog-consumer@example.com")

	userID := createIntegrationUser(t, database, "actlog-consumer@example.com", "student")

	// Clear Redis queue
	rdb := redisClient.RDB()
	_ = rdb.Del(ctx, "activity_log:queue").Err()

	store := candidate.NewActivityLogStore(database.Pool())
	service := candidate.NewActivityLogService(store, rdb)
	consumer := candidate.NewActivityLogConsumer(store, rdb)

	// Log two activities via service (pushes to Redis)
	err = service.LogActivity(ctx, candidate.CreateActivityInput{
		UserID:       userID,
		ActivityType: "view_school",
		TargetType:   "school",
		TargetID:     999,
		Metadata:     map[string]any{"school_name": "Test University"},
	})
	require.NoError(t, err)

	err = service.LogActivity(ctx, candidate.CreateActivityInput{
		UserID:       userID,
		ActivityType: "view_major",
		TargetType:   "major",
		TargetID:     888,
		Metadata:     map[string]any{"major_name": "Computer Science"},
	})
	require.NoError(t, err)

	// Start consumer and wait for flush
	consumerCtx, cancel := context.WithCancel(context.Background())
	consumerDone := consumer.Start(consumerCtx)

	// Consumer flush interval is 2s; wait a bit longer to ensure processing
	time.Sleep(3 * time.Second)

	cancel()
	select {
	case <-consumerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop in time")
	}

	// Verify logs were written to DB
	logs, total, err := store.List(ctx, candidate.ActivityFilter{UserID: userID}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, logs, 2)

	activityTypes := make(map[string]bool)
	for _, l := range logs {
		activityTypes[l.ActivityType] = true
		require.Equal(t, userID, l.UserID)
	}
	require.True(t, activityTypes["view_school"])
	require.True(t, activityTypes["view_major"])
}

func cleanupActivityLogs(t *testing.T, database *db.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := database.Pool().Exec(ctx, "TRUNCATE TABLE candidate_activity_logs RESTART IDENTITY CASCADE")
	require.NoError(t, err)
}
