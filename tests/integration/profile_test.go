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

	"admission-api/internal/planner"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProfileCRUD(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "integration-test-secret")
	}

	ctx := context.Background()
	cfg, err := config.Load()
	require.NoError(t, err)

	database, err := db.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer database.Close()

	cleanupProfiles(t, database)
	cleanupMerchants(t, database)

	// Clean up test admin user if exists from previous run
	_, _ = database.Pool().Exec(ctx, "DELETE FROM users WHERE email = $1", "profile-admin@example.com")

	adminID := createIntegrationUser(t, database, "profile-admin@example.com", "student")

	router := newProfileIntegrationRouter(t, database, cfg)
	adminToken := issueAccessToken(t, cfg, adminID, "admin", "admin")

	// 1. Create a merchant first (for merchant-related tests)
	merchantPayload := map[string]interface{}{
		"merchant_name":   "Profile Test Org",
		"service_regions": []string{"11", "12", "13"},
		"status":          "active",
	}
	merchantResp := performProfileRequest(t, router, http.MethodPost, "/api/v1/admin/planner/merchants", merchantPayload, adminToken)
	require.Equal(t, http.StatusOK, merchantResp.Code)

	var merchantEnvelope web.Response
	require.NoError(t, json.Unmarshal(merchantResp.Body.Bytes(), &merchantEnvelope))
	require.Equal(t, 0, merchantEnvelope.Code)

	var merchant planner.PlannerMerchant
	merchantData, _ := json.Marshal(merchantEnvelope.Data)
	require.NoError(t, json.Unmarshal(merchantData, &merchant))
	require.NotZero(t, merchant.ID)
	merchantID := merchant.ID

	// 2. Create planner profile
	createPayload := map[string]interface{}{
		"email":            "planner1@example.com",
		"password":         "password123",
		"real_name":        "Planner One",
		"phone":            "13800138001",
		"title":            "Senior Consultant",
		"introduction":     "Experienced planner",
		"specialty_tags":   []string{"science", "engineering"},
		"service_region":   []string{"11", "12"},
		"service_price":    999.99,
		"level":            "senior",
		"certification_no": "CERT-001",
		"merchant_id":      merchantID,
		"status":           "active",
	}
	createResp := performProfileRequest(t, router, http.MethodPost, "/api/v1/admin/planner/profiles", createPayload, adminToken)
	require.Equal(t, http.StatusOK, createResp.Code)

	var createEnvelope web.Response
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &createEnvelope))
	require.Equal(t, 0, createEnvelope.Code)

	var createdProfile planner.PlannerProfileResponse
	createdData, _ := json.Marshal(createEnvelope.Data)
	require.NoError(t, json.Unmarshal(createdData, &createdProfile))
	require.NotZero(t, createdProfile.ID)
	require.NotZero(t, createdProfile.UserID)
	require.Equal(t, "Planner One", createdProfile.RealName)
	require.Equal(t, "13800138001", *createdProfile.Phone)
	require.Equal(t, "Senior Consultant", *createdProfile.Title)
	require.Equal(t, "Experienced planner", *createdProfile.Introduction)
	require.Equal(t, []string{"science", "engineering"}, createdProfile.SpecialtyTags)
	require.Equal(t, []string{"11", "12"}, createdProfile.ServiceRegion)
	require.Equal(t, 999.99, *createdProfile.ServicePrice)
	require.Equal(t, "senior", createdProfile.Level)
	require.Equal(t, "CERT-001", *createdProfile.CertificationNo)
	require.Equal(t, merchantID, *createdProfile.MerchantID)
	require.Equal(t, "active", createdProfile.Status)
	require.Equal(t, 0, createdProfile.TotalServiceCount)
	require.Equal(t, 5.0, createdProfile.RatingAvg)

	profileID := createdProfile.ID
	plannerUserID := createdProfile.UserID

	// 3. Create profile with service region inheritance (empty regions)
	createPayload2 := map[string]interface{}{
		"email":        "planner2@example.com",
		"password":     "password123",
		"real_name":    "Planner Two",
		"merchant_id":  merchantID,
		"status":       "active",
	}
	createResp2 := performProfileRequest(t, router, http.MethodPost, "/api/v1/admin/planner/profiles", createPayload2, adminToken)
	require.Equal(t, http.StatusOK, createResp2.Code)

	var createEnvelope2 web.Response
	require.NoError(t, json.Unmarshal(createResp2.Body.Bytes(), &createEnvelope2))
	require.Equal(t, 0, createEnvelope2.Code)

	var createdProfile2 planner.PlannerProfileResponse
	createdData2, _ := json.Marshal(createEnvelope2.Data)
	require.NoError(t, json.Unmarshal(createdData2, &createdProfile2))
	require.Equal(t, []string{"11", "12", "13"}, createdProfile2.ServiceRegion)

	// 4. Get profile by ID
	getResp := performProfileRequest(t, router, http.MethodGet, "/api/v1/planner/profiles/"+itoa64(profileID), nil, adminToken)
	require.Equal(t, http.StatusOK, getResp.Code)

	var getEnvelope web.Response
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getEnvelope))
	require.Equal(t, 0, getEnvelope.Code)

	var getProfile planner.PlannerProfileResponse
	getData, _ := json.Marshal(getEnvelope.Data)
	require.NoError(t, json.Unmarshal(getData, &getProfile))
	require.Equal(t, profileID, getProfile.ID)
	require.Equal(t, "Planner One", getProfile.RealName)

	// 5. Get my profile (as planner)
	plannerToken := issueAccessToken(t, cfg, plannerUserID, "planner", "student")
	myResp := performProfileRequest(t, router, http.MethodGet, "/api/v1/planner/profiles/me", nil, plannerToken)
	require.Equal(t, http.StatusOK, myResp.Code)

	var myEnvelope web.Response
	require.NoError(t, json.Unmarshal(myResp.Body.Bytes(), &myEnvelope))
	require.Equal(t, 0, myEnvelope.Code)

	var myProfile planner.PlannerProfileResponse
	myData, _ := json.Marshal(myEnvelope.Data)
	require.NoError(t, json.Unmarshal(myData, &myProfile))
	require.Equal(t, profileID, myProfile.ID)
	require.Equal(t, "Planner One", myProfile.RealName)

	// 6. Update my profile
	updatePayload := map[string]interface{}{
		"real_name":      "Updated Planner",
		"title":          "Expert Consultant",
		"service_region": []string{"11"},
	}
	updateResp := performProfileRequest(t, router, http.MethodPut, "/api/v1/planner/profiles/me", updatePayload, plannerToken)
	require.Equal(t, http.StatusOK, updateResp.Code)

	var updateEnvelope web.Response
	require.NoError(t, json.Unmarshal(updateResp.Body.Bytes(), &updateEnvelope))
	require.Equal(t, 0, updateEnvelope.Code)

	var updatedProfile planner.PlannerProfileResponse
	updatedData, _ := json.Marshal(updateEnvelope.Data)
	require.NoError(t, json.Unmarshal(updatedData, &updatedProfile))
	require.Equal(t, "Updated Planner", updatedProfile.RealName)
	require.Equal(t, "Expert Consultant", *updatedProfile.Title)
	require.Equal(t, []string{"11"}, updatedProfile.ServiceRegion)
	// Fields not in update payload should remain
	require.Equal(t, "13800138001", *updatedProfile.Phone)

	// 7. List profiles with status filter
	listResp := performProfileRequest(t, router, http.MethodGet, "/api/v1/planner/profiles?status=active&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, listResp.Code)

	var listEnvelope web.Response
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listEnvelope))
	require.Equal(t, 0, listEnvelope.Code)

	var listResult planner.ProfileListResponse
	listData, _ := json.Marshal(listEnvelope.Data)
	require.NoError(t, json.Unmarshal(listData, &listResult))
	require.GreaterOrEqual(t, listResult.Total, int64(2))

	// 8. List profiles with merchant_id filter
	merchantFilterResp := performProfileRequest(t, router, http.MethodGet, "/api/v1/planner/profiles?merchant_id="+itoa64(merchantID)+"&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, merchantFilterResp.Code)

	var merchantFilterEnvelope web.Response
	require.NoError(t, json.Unmarshal(merchantFilterResp.Body.Bytes(), &merchantFilterEnvelope))
	require.Equal(t, 0, merchantFilterEnvelope.Code)

	var merchantFilterResult planner.ProfileListResponse
	merchantFilterData, _ := json.Marshal(merchantFilterEnvelope.Data)
	require.NoError(t, json.Unmarshal(merchantFilterData, &merchantFilterResult))
	require.GreaterOrEqual(t, merchantFilterResult.Total, int64(2))

	// 9. List profiles with real_name filter
	nameFilterResp := performProfileRequest(t, router, http.MethodGet, "/api/v1/planner/profiles?real_name=Planner&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, nameFilterResp.Code)

	var nameFilterEnvelope web.Response
	require.NoError(t, json.Unmarshal(nameFilterResp.Body.Bytes(), &nameFilterEnvelope))
	require.Equal(t, 0, nameFilterEnvelope.Code)

	var nameFilterResult planner.ProfileListResponse
	nameFilterData, _ := json.Marshal(nameFilterEnvelope.Data)
	require.NoError(t, json.Unmarshal(nameFilterData, &nameFilterResult))
	require.GreaterOrEqual(t, nameFilterResult.Total, int64(1))

	// 10. List profiles with phone filter
	phoneFilterResp := performProfileRequest(t, router, http.MethodGet, "/api/v1/planner/profiles?phone=13800138001&page=1&page_size=20", nil, adminToken)
	require.Equal(t, http.StatusOK, phoneFilterResp.Code)

	var phoneFilterEnvelope web.Response
	require.NoError(t, json.Unmarshal(phoneFilterResp.Body.Bytes(), &phoneFilterEnvelope))
	require.Equal(t, 0, phoneFilterEnvelope.Code)

	var phoneFilterResult planner.ProfileListResponse
	phoneFilterData, _ := json.Marshal(phoneFilterEnvelope.Data)
	require.NoError(t, json.Unmarshal(phoneFilterData, &phoneFilterResult))
	require.GreaterOrEqual(t, phoneFilterResult.Total, int64(1))

	// 11. Create duplicate email should fail
	duplicatePayload := map[string]interface{}{
		"email":     "planner1@example.com",
		"password":  "password123",
		"real_name": "Duplicate",
	}
	dupResp := performProfileRequest(t, router, http.MethodPost, "/api/v1/admin/planner/profiles", duplicatePayload, adminToken)
	require.Equal(t, http.StatusConflict, dupResp.Code)

	// 11. Create with invalid merchant_id
	badPayload := map[string]interface{}{
		"email":       "planner3@example.com",
		"password":    "password123",
		"real_name":   "Bad Merchant",
		"merchant_id": 999999,
	}
	badResp := performProfileRequest(t, router, http.MethodPost, "/api/v1/admin/planner/profiles", badPayload, adminToken)
	require.Equal(t, http.StatusBadRequest, badResp.Code)

	// 12. Create with invalid service_region
	invalidRegionPayload := map[string]interface{}{
		"email":          "planner3@example.com",
		"password":       "password123",
		"real_name":      "Bad Region",
		"merchant_id":    merchantID,
		"service_region": []string{"99"},
	}
	invalidRegionResp := performProfileRequest(t, router, http.MethodPost, "/api/v1/admin/planner/profiles", invalidRegionPayload, adminToken)
	require.Equal(t, http.StatusBadRequest, invalidRegionResp.Code)

	// 13. Get non-existent profile
	notFoundResp := performProfileRequest(t, router, http.MethodGet, "/api/v1/planner/profiles/999999", nil, adminToken)
	require.Equal(t, http.StatusNotFound, notFoundResp.Code)

	// 14. Update with invalid service_region
	badUpdatePayload := map[string]interface{}{
		"service_region": []string{"99"},
	}
	badUpdateResp := performProfileRequest(t, router, http.MethodPut, "/api/v1/planner/profiles/me", badUpdatePayload, plannerToken)
	require.Equal(t, http.StatusBadRequest, badUpdateResp.Code)
}

func newProfileIntegrationRouter(t *testing.T, database *db.DB, cfg *config.Config) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  0,
		RefreshTTL: 0,
	}

	merchantStore := planner.NewMerchantStore(database.Pool())
	merchantService := planner.NewMerchantService(merchantStore)
	merchantHandler := planner.NewMerchantHandler(merchantService)

	profileStore := planner.NewProfileStore(database.Pool())
	profileService := planner.NewProfileService(profileStore, merchantStore)
	profileHandler := planner.NewProfileHandler(profileService)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Platform)

	api := r.Group("/api/v1")
	authorized := api.Group("")
	authorized.Use(middleware.JWTMiddleware(jwtConfig))

	authorized.GET("/planner/merchants", merchantHandler.ListMerchants)
	authorized.GET("/planner/merchants/:id", merchantHandler.GetMerchant)
	authorized.GET("/planner/profiles", profileHandler.ListProfiles)
	authorized.GET("/planner/profiles/:id", profileHandler.GetProfile)
	authorized.GET("/planner/profiles/me", profileHandler.GetMyProfile)
	authorized.PUT("/planner/profiles/me", profileHandler.UpdateMyProfile)

	adminRoutes := authorized.Group("/admin")
	adminRoutes.Use(middleware.RequireRole("admin"))
	adminRoutes.POST("/planner/merchants", merchantHandler.CreateMerchant)
	adminRoutes.PUT("/planner/merchants/:id", merchantHandler.UpdateMerchant)
	adminRoutes.POST("/planner/profiles", profileHandler.CreateProfile)

	return r
}

func performProfileRequest(t *testing.T, router http.Handler, method, path string, payload any, accessToken string) *httptest.ResponseRecorder {
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

func cleanupProfiles(t *testing.T, database *db.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := database.Pool().Exec(ctx, "TRUNCATE TABLE planner_profiles RESTART IDENTITY CASCADE")
	require.NoError(t, err)
	_, err = database.Pool().Exec(ctx, "DELETE FROM users WHERE role = 'planner'")
	require.NoError(t, err)
}
