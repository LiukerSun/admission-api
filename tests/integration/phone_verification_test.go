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

	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	platformredis "admission-api/internal/platform/redis"
	"admission-api/internal/platform/sms"
	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type integrationResponse struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

type meResponse struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	PhoneVerified bool   `json:"phone_verified"`
}

func TestPhoneVerificationFlow(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "integration-test-secret")
	}

	ctx := context.Background()
	cfg := config.Load()

	database, err := db.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer database.Close()

	redisClient, err := platformredis.New(cfg.RedisAddr)
	require.NoError(t, err)
	defer redisClient.Close()

	cleanupIntegrationState(t, database, redisClient)

	userID := createIntegrationUser(t, database, "phone-flow@example.com", "student")
	router := newIntegrationRouter(t, database, redisClient, cfg)
	accessToken := issueAccessToken(t, cfg, userID, "user", "student")

	sendPayload := map[string]string{"phone": "13800138000"}
	sendResp := performJSONRequest(t, router, http.MethodPost, "/api/v1/me/phone/send-code", sendPayload, accessToken)
	require.Equal(t, http.StatusOK, sendResp.Code)

	code, err := redisClient.Get(ctx, "sms:code:13800138000")
	require.NoError(t, err)
	require.Len(t, code, 6)

	verifyPayload := map[string]string{"phone": "13800138000", "code": code}
	verifyResp := performJSONRequest(t, router, http.MethodPost, "/api/v1/me/phone/verify", verifyPayload, accessToken)
	require.Equal(t, http.StatusOK, verifyResp.Code)

	meResp := performJSONRequest(t, router, http.MethodGet, "/api/v1/me", nil, accessToken)
	require.Equal(t, http.StatusOK, meResp.Code)

	var envelope integrationResponse
	require.NoError(t, json.Unmarshal(meResp.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)

	var me meResponse
	require.NoError(t, json.Unmarshal(envelope.Data, &me))
	require.Equal(t, userID, me.ID)
	require.Equal(t, "13800138000", me.Phone)
	require.True(t, me.PhoneVerified)

	var phone string
	var verifiedAt *time.Time
	err = database.Pool().QueryRow(ctx, "SELECT phone, phone_verified_at FROM users WHERE id = $1", userID).Scan(&phone, &verifiedAt)
	require.NoError(t, err)
	require.Equal(t, "13800138000", phone)
	require.NotNil(t, verifiedAt)

	_, err = redisClient.Get(ctx, "sms:code:13800138000")
	require.Error(t, err)
}

func TestPhoneVerificationSendCodeCooldown(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "integration-test-secret")
	}

	ctx := context.Background()
	cfg := config.Load()

	database, err := db.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer database.Close()

	redisClient, err := platformredis.New(cfg.RedisAddr)
	require.NoError(t, err)
	defer redisClient.Close()

	cleanupIntegrationState(t, database, redisClient)

	userID := createIntegrationUser(t, database, "phone-cooldown@example.com", "student")
	router := newIntegrationRouter(t, database, redisClient, cfg)
	accessToken := issueAccessToken(t, cfg, userID, "user", "student")

	payload := map[string]string{"phone": "13800138001"}
	firstResp := performJSONRequest(t, router, http.MethodPost, "/api/v1/me/phone/send-code", payload, accessToken)
	require.Equal(t, http.StatusOK, firstResp.Code)

	secondResp := performJSONRequest(t, router, http.MethodPost, "/api/v1/me/phone/send-code", payload, accessToken)
	require.Equal(t, http.StatusBadRequest, secondResp.Code)

	var envelope integrationResponse
	require.NoError(t, json.Unmarshal(secondResp.Body.Bytes(), &envelope))
	require.Equal(t, web.ErrCodeBadRequest, envelope.Code)
	require.Equal(t, "verification code sent too frequently", envelope.Message)
}

func newIntegrationRouter(t *testing.T, database *db.DB, redisClient *platformredis.Client, cfg *config.Config) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  time.Duration(cfg.JWTAccessTTLMinutes) * time.Minute,
		RefreshTTL: time.Duration(cfg.JWTRefreshTTLHours) * time.Hour,
	}

	userStore := user.NewStore(database.Pool())
	userService := user.NewAuthService(userStore, nil, jwtConfig)
	phoneVerificationService := user.NewPhoneVerificationService(userStore, redisClient, sms.NewMockClient(), user.PhoneVerificationConfig{
		CodeTTL:      time.Duration(cfg.SMSCodeTTLMinutes) * time.Minute,
		SendCooldown: time.Duration(cfg.SMSSendCooldownSeconds) * time.Second,
		DailyLimit:   cfg.SMSDailyLimit,
		MaxAttempts:  cfg.SMSMaxVerifyAttempts,
	})
	userHandler := user.NewHandler(userService, phoneVerificationService, jwtConfig)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Platform)

	api := r.Group("/api/v1")
	authorized := api.Group("")
	authorized.Use(middleware.JWTMiddleware(jwtConfig))
	authorized.Use(middleware.AuthStatusMiddleware(redisClient))
	authorized.GET("/me", userHandler.Me)
	authorized.POST("/me/phone/send-code", userHandler.SendPhoneVerificationCode)
	authorized.POST("/me/phone/verify", userHandler.VerifyPhone)

	return r
}

func performJSONRequest(t *testing.T, router http.Handler, method, path string, payload any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	if payload != nil {
		require.NoError(t, json.NewEncoder(&body).Encode(payload))
	}

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Platform", "web")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func issueAccessToken(t *testing.T, cfg *config.Config, userID int64, role, userType string) string {
	t.Helper()

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  time.Duration(cfg.JWTAccessTTLMinutes) * time.Minute,
		RefreshTTL: time.Duration(cfg.JWTRefreshTTLHours) * time.Hour,
	}

	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, userID, role, userType, "web")
	require.NoError(t, err)
	return tokens.AccessToken
}

func createIntegrationUser(t *testing.T, database *db.DB, email, userType string) int64 {
	t.Helper()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	var userID int64
	err = database.Pool().QueryRow(
		context.Background(),
		`INSERT INTO users (email, password_hash, role, user_type, status) VALUES ($1, $2, 'user', $3, 'active') RETURNING id`,
		email,
		string(passwordHash),
		userType,
	).Scan(&userID)
	require.NoError(t, err)

	return userID
}

func cleanupIntegrationState(t *testing.T, database *db.DB, redisClient *platformredis.Client) {
	t.Helper()

	ctx := context.Background()
	_, err := database.Pool().Exec(ctx, "TRUNCATE TABLE user_bindings, users RESTART IDENTITY CASCADE")
	require.NoError(t, err)

	err = flushRedisDB(ctx, redisClient)
	require.NoError(t, err)
}

func flushRedisDB(ctx context.Context, redisClient *platformredis.Client) error {
	status := redisClient.RDB().FlushDB(ctx)
	if status.Err() != nil {
		return status.Err()
	}
	return nil
}
