package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockProfileService struct {
	mock.Mock
}

func (m *mockProfileService) CreateProfile(ctx context.Context, req CreateProfileRequest) (*PlannerProfileResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerProfileResponse), args.Error(1)
}

func (m *mockProfileService) GetMyProfile(ctx context.Context, userID int64) (*PlannerProfileResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerProfileResponse), args.Error(1)
}

func (m *mockProfileService) UpdateMyProfile(ctx context.Context, userID int64, req UpdateMyProfileRequest) (*PlannerProfileResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerProfileResponse), args.Error(1)
}

func (m *mockProfileService) GetProfile(ctx context.Context, id int64) (*PlannerProfileResponse, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerProfileResponse), args.Error(1)
}

func (m *mockProfileService) ListProfiles(ctx context.Context, filter ProfileFilter, page, pageSize int, sortField, sortOrder string) (*ProfileListResponse, error) {
	args := m.Called(ctx, filter, page, pageSize, sortField, sortOrder)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProfileListResponse), args.Error(1)
}

func setupProfileTest() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestProfileHandler_CreateProfile(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	svc.On("CreateProfile", mock.Anything, mock.Anything).Return(
		&PlannerProfileResponse{ID: 1, UserID: 10, RealName: "Test User", Level: "junior", Status: "active"},
		nil,
	)

	c, w := setupProfileTest()
	body := CreateProfileRequest{Email: "test@example.com", Password: "password123", RealName: "Test User"}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/planner/profiles", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateProfile(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestProfileHandler_CreateProfile_ValidationError(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	c, w := setupProfileTest()
	body := CreateProfileRequest{Email: "invalid", Password: "short", RealName: ""}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/planner/profiles", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateProfile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProfileHandler_GetMyProfile(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	svc.On("GetMyProfile", mock.Anything, int64(10)).Return(
		&PlannerProfileResponse{ID: 1, UserID: 10, RealName: "Test User", Level: "senior", Status: "active"},
		nil,
	)

	c, w := setupProfileTest()
	c.Set(middleware.ContextUserIDKey, int64(10))
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/planner/profiles/me", http.NoBody)

	h.GetMyProfile(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestProfileHandler_GetMyProfile_Unauthorized(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	c, w := setupProfileTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/planner/profiles/me", http.NoBody)

	h.GetMyProfile(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProfileHandler_UpdateMyProfile(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	svc.On("UpdateMyProfile", mock.Anything, int64(10), mock.Anything).Return(
		&PlannerProfileResponse{ID: 1, UserID: 10, RealName: "Updated User", Level: "expert", Status: "active"},
		nil,
	)

	c, w := setupProfileTest()
	c.Set(middleware.ContextUserIDKey, int64(10))
	body := UpdateMyProfileRequest{RealName: strPtr("Updated User"), Level: strPtr("expert")}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/planner/profiles/me", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateMyProfile(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestProfileHandler_UpdateMyProfile_Unauthorized(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	c, w := setupProfileTest()
	body := UpdateMyProfileRequest{RealName: strPtr("Updated User")}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/planner/profiles/me", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateMyProfile(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProfileHandler_GetProfile(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	svc.On("GetProfile", mock.Anything, int64(7)).Return(
		&PlannerProfileResponse{ID: 7, UserID: 10, RealName: "Test User", Level: "senior", Status: "active"},
		nil,
	)

	c, w := setupProfileTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/planner/profiles/7", http.NoBody)

	h.GetProfile(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestProfileHandler_GetProfile_InvalidID(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	c, w := setupProfileTest()
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/planner/profiles/invalid", http.NoBody)

	h.GetProfile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProfileHandler_ListProfiles(t *testing.T) {
	svc := new(mockProfileService)
	h := NewProfileHandler(svc)

	svc.On("ListProfiles", mock.Anything, ProfileFilter{Status: "active"}, 1, 20, "", "").Return(
		&ProfileListResponse{
			Profiles: []*PlannerProfileResponse{
				{ID: 1, UserID: 10, RealName: "User A", Level: "junior", Status: "active"},
			},
			Total: 1,
		},
		nil,
	)

	c, w := setupProfileTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/planner/profiles?status=active&page=1&page_size=20", http.NoBody)

	h.ListProfiles(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}
