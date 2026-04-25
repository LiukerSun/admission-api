//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"admission-api/internal/planner"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMerchantCRUD(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "integration-test-secret")
	}

	ctx := context.Background()
	cfg, err := config.Load()
	require.NoError(t, err)

	database, err := db.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer database.Close()

	cleanupMerchants(t, database)

	adminID := createIntegrationUser(t, database, "merchant-admin@example.com", "student")
	ownerID := createIntegrationUser(t, database, "merchant-owner@example.com", "student")

	router := newMerchantIntegrationRouter(t, database, cfg)
	adminToken := issueAccessToken(t, cfg, adminID, "admin", "admin")

	// 1. Create merchant
	createPayload := map[string]interface{}{
		"merchant_name":         "Integration Org",
		"contact_person":        "Alice",
		"contact_phone":         "13800138000",
		"address":               "Beijing",
		"sort_order":            10,
		"owner_id":              ownerID,
		"service_regions":       []string{"11", "12"},
		"default_service_price": 999.99,
		"status":                "active",
	}
	createResp := performMerchantRequest(t, router, http.MethodPost, "/api/v1/admin/planner/merchants", createPayload, adminToken)
	require.Equal(t, http.StatusOK, createResp.Code)

	var createEnvelope web.Response
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &createEnvelope))
	require.Equal(t, 0, createEnvelope.Code)

	var createdMerchant planner.PlannerMerchant
	createdData, _ := json.Marshal(createEnvelope.Data)
	require.NoError(t, json.Unmarshal(createdData, &createdMerchant))
	require.NotZero(t, createdMerchant.ID)
	require.Equal(t, "Integration Org", createdMerchant.MerchantName)
	require.Equal(t, "Alice", *createdMerchant.ContactPerson)
	require.Equal(t, "13800138000", *createdMerchant.ContactPhone)
	require.Equal(t, "Beijing", *createdMerchant.Address)
	require.Equal(t, 10, createdMerchant.SortOrder)
	require.Equal(t, ownerID, *createdMerchant.OwnerID)
	require.Equal(t, []string{"11", "12"}, createdMerchant.ServiceRegions)
	require.Equal(t, 999.99, *createdMerchant.DefaultServicePrice)
	require.Equal(t, "active", createdMerchant.Status)

	merchantID := createdMerchant.ID

	// 2. Get merchant
	getResp := performMerchantRequest(t, router, http.MethodGet, "/api/v1/planner/merchants/"+itoa64(merchantID), nil, adminToken)
	require.Equal(t, http.StatusOK, getResp.Code)

	var getEnvelope web.Response
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getEnvelope))
	require.Equal(t, 0, getEnvelope.Code)

	// 3. List merchants with status filter
	listResp := performMerchantRequest(t, router, http.MethodGet, "/api/v1/planner/merchants?status=active&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, listResp.Code)

	var listEnvelope web.Response
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listEnvelope))
	require.Equal(t, 0, listEnvelope.Code)

	var listResult planner.MerchantListResponse
	listData, _ := json.Marshal(listEnvelope.Data)
	require.NoError(t, json.Unmarshal(listData, &listResult))
	require.GreaterOrEqual(t, listResult.Total, int64(1))

	// 3a. List merchants with merchant_name filter
	nameFilterResp := performMerchantRequest(t, router, http.MethodGet, "/api/v1/planner/merchants?merchant_name=Integration&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, nameFilterResp.Code)

	var nameFilterEnvelope web.Response
	require.NoError(t, json.Unmarshal(nameFilterResp.Body.Bytes(), &nameFilterEnvelope))
	require.Equal(t, 0, nameFilterEnvelope.Code)

	var nameFilterResult planner.MerchantListResponse
	nameFilterData, _ := json.Marshal(nameFilterEnvelope.Data)
	require.NoError(t, json.Unmarshal(nameFilterData, &nameFilterResult))
	require.GreaterOrEqual(t, nameFilterResult.Total, int64(1))

	// 3b. List merchants with service_region filter
	regionFilterResp := performMerchantRequest(t, router, http.MethodGet, "/api/v1/planner/merchants?service_region=11&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, regionFilterResp.Code)

	var regionFilterEnvelope web.Response
	require.NoError(t, json.Unmarshal(regionFilterResp.Body.Bytes(), &regionFilterEnvelope))
	require.Equal(t, 0, regionFilterEnvelope.Code)

	var regionFilterResult planner.MerchantListResponse
	regionFilterData, _ := json.Marshal(regionFilterEnvelope.Data)
	require.NoError(t, json.Unmarshal(regionFilterData, &regionFilterResult))
	require.GreaterOrEqual(t, regionFilterResult.Total, int64(1))

	// 3c. List merchants with non-matching service_region filter
	noMatchResp := performMerchantRequest(t, router, http.MethodGet, "/api/v1/planner/merchants?service_region=99&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, noMatchResp.Code)

	var noMatchEnvelope web.Response
	require.NoError(t, json.Unmarshal(noMatchResp.Body.Bytes(), &noMatchEnvelope))
	require.Equal(t, 0, noMatchEnvelope.Code)

	var noMatchResult planner.MerchantListResponse
	noMatchData, _ := json.Marshal(noMatchEnvelope.Data)
	require.NoError(t, json.Unmarshal(noMatchData, &noMatchResult))
	require.Equal(t, int64(0), noMatchResult.Total)

	// 4. Update merchant (dynamic field update)
	updatePayload := map[string]interface{}{
		"merchant_name":   "Updated Org Name",
		"contact_person":  "Bob",
		"sort_order":      20,
		"service_regions": []string{"11", "12", "13"},
	}
	updateResp := performMerchantRequest(t, router, http.MethodPut, "/api/v1/admin/planner/merchants/"+itoa64(merchantID), updatePayload, adminToken)
	require.Equal(t, http.StatusOK, updateResp.Code)

	var updateEnvelope web.Response
	require.NoError(t, json.Unmarshal(updateResp.Body.Bytes(), &updateEnvelope))
	require.Equal(t, 0, updateEnvelope.Code)

	var updatedMerchant planner.PlannerMerchant
	updatedData, _ := json.Marshal(updateEnvelope.Data)
	require.NoError(t, json.Unmarshal(updatedData, &updatedMerchant))
	require.Equal(t, "Updated Org Name", updatedMerchant.MerchantName)
	require.Equal(t, "Bob", *updatedMerchant.ContactPerson)
	require.Equal(t, 20, updatedMerchant.SortOrder)
	require.Equal(t, []string{"11", "12", "13"}, updatedMerchant.ServiceRegions)
	// Fields not in update payload should remain
	require.Equal(t, "13800138000", *updatedMerchant.ContactPhone)
	require.Equal(t, "Beijing", *updatedMerchant.Address)

	// 5. Update with invalid owner_id
	badUpdatePayload := map[string]interface{}{"owner_id": 999999}
	badUpdateResp := performMerchantRequest(t, router, http.MethodPut, "/api/v1/admin/planner/merchants/"+itoa64(merchantID), badUpdatePayload, adminToken)
	require.Equal(t, http.StatusBadRequest, badUpdateResp.Code)

	// 6. Create duplicate name should fail
	duplicatePayload := map[string]interface{}{
		"merchant_name": "Updated Org Name",
		"status":        "active",
	}
	dupResp := performMerchantRequest(t, router, http.MethodPost, "/api/v1/admin/planner/merchants", duplicatePayload, adminToken)
	require.Equal(t, http.StatusConflict, dupResp.Code)

	// 7. Get non-existent merchant
	notFoundResp := performMerchantRequest(t, router, http.MethodGet, "/api/v1/planner/merchants/999999", nil, adminToken)
	require.Equal(t, http.StatusNotFound, notFoundResp.Code)
}

func TestMerchantCreate_InvalidOwner(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "integration-test-secret")
	}

	ctx := context.Background()
	cfg, err := config.Load()
	require.NoError(t, err)

	database, err := db.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer database.Close()

	// cleanupMerchants(t, database)

	adminID := createIntegrationUser(t, database, "merchant-admin2@example.com", "student")
	router := newMerchantIntegrationRouter(t, database, cfg)
	adminToken := issueAccessToken(t, cfg, adminID, "admin", "admin")

	payload := map[string]interface{}{
		"merchant_name": "Invalid Owner Org",
		"owner_id":      999999,
	}
	resp := performMerchantRequest(t, router, http.MethodPost, "/api/v1/admin/planner/merchants", payload, adminToken)
	require.Equal(t, http.StatusBadRequest, resp.Code)

	var envelope web.Response
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &envelope))
	require.Equal(t, web.ErrCodeBadRequest, envelope.Code)
}

func newMerchantIntegrationRouter(t *testing.T, database *db.DB, cfg *config.Config) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  0, // not used directly in tests
		RefreshTTL: 0,
	}

	merchantStore := planner.NewMerchantStore(database.Pool())
	merchantService := planner.NewMerchantService(merchantStore)
	merchantHandler := planner.NewMerchantHandler(merchantService)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Platform)

	api := r.Group("/api/v1")
	authorized := api.Group("")
	authorized.Use(middleware.JWTMiddleware(jwtConfig))

	authorized.GET("/planner/merchants", merchantHandler.ListMerchants)
	authorized.GET("/planner/merchants/:id", merchantHandler.GetMerchant)

	adminRoutes := authorized.Group("/admin")
	adminRoutes.Use(middleware.RequireRole("admin"))
	adminRoutes.POST("/planner/merchants", merchantHandler.CreateMerchant)
	adminRoutes.PUT("/planner/merchants/:id", merchantHandler.UpdateMerchant)

	return r
}

func performMerchantRequest(t *testing.T, router http.Handler, method, path string, payload any, accessToken string) *httptest.ResponseRecorder {
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

func cleanupMerchants(t *testing.T, database *db.DB) {
	t.Helper()
	_, err := database.Pool().Exec(context.Background(), "TRUNCATE TABLE planner_merchants RESTART IDENTITY CASCADE")
	require.NoError(t, err)
}

func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}
