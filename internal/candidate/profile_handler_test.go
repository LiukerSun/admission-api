package candidate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockProfileService struct{ mock.Mock }

func (m *mockProfileService) GetMyProfiles(ctx context.Context, userID int64, userType string) ([]*ProfileResponse, error) {
	args := m.Called(ctx, userID, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ProfileResponse), args.Error(1)
}
func (m *mockProfileService) GetProfile(ctx context.Context, userID, profileID int64, userType string) (*ProfileResponse, error) {
	args := m.Called(ctx, userID, profileID, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProfileResponse), args.Error(1)
}
func (m *mockProfileService) CreateProfile(ctx context.Context, userID int64, req CreateProfileRequest) (*ProfileResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProfileResponse), args.Error(1)
}
func (m *mockProfileService) UpdateProfile(ctx context.Context, userID, profileID int64, req UpdateProfileRequest) (*ProfileResponse, error) {
	args := m.Called(ctx, userID, profileID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProfileResponse), args.Error(1)
}
func (m *mockProfileService) DeleteProfile(ctx context.Context, userID, profileID int64) error {
	return m.Called(ctx, userID, profileID).Error(0)
}
func (m *mockProfileService) LookupByIDCard(ctx context.Context, userType, idCard string) (*LookupResponse, error) {
	args := m.Called(ctx, userType, idCard)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LookupResponse), args.Error(1)
}
func (m *mockProfileService) LookupByPhone(ctx context.Context, userType, phone string) (*LookupResponse, error) {
	args := m.Called(ctx, userType, phone)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LookupResponse), args.Error(1)
}
func (m *mockProfileService) LookupByCode(ctx context.Context, userType, code string) (*LookupResponse, error) {
	args := m.Called(ctx, userType, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LookupResponse), args.Error(1)
}
func (m *mockProfileService) GenerateInviteCode(ctx context.Context, userID, profileID int64) (*InviteResponse, error) {
	args := m.Called(ctx, userID, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*InviteResponse), args.Error(1)
}

func setupHandlerRouter(t *testing.T, withAuth bool, userType string) (*gin.Engine, *mockProfileService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	svc := &mockProfileService{}
	h := NewProfileHandler(svc)

	if withAuth {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ContextUserIDKey, int64(10))
			c.Set(middleware.ContextUserTypeKey, userType)
			c.Next()
		})
	}

	r.GET("/profiles", h.GetMyProfiles)
	r.POST("/profiles", h.CreateProfile)
	r.GET("/profiles/:id", h.GetProfile)
	r.PUT("/profiles/:id", h.UpdateProfile)
	r.DELETE("/profiles/:id", h.DeleteProfile)
	r.POST("/profiles/lookup/idcard", h.LookupByIDCard)
	r.POST("/profiles/lookup/phone", h.LookupByPhone)
	r.POST("/profiles/lookup/code", h.LookupByCode)
	r.POST("/profiles/:id/invite-code", h.GenerateInviteCode)
	return r, svc
}

func decodeResp(t *testing.T, w *httptest.ResponseRecorder) web.Response {
	t.Helper()
	var resp web.Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func TestHandler_RequiresAuth(t *testing.T) {
	r, _ := setupHandlerRouter(t, false, "")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/profiles", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_GetMyProfiles_Success(t *testing.T) {
	r, svc := setupHandlerRouter(t, true, "parent")
	svc.On("GetMyProfiles", mock.Anything, int64(10), "parent").Return([]*ProfileResponse{
		{ID: 1, UserID: 10, RealName: "张三", ProvinceID: 11},
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/profiles", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResp(t, w)
	assert.Equal(t, 0, resp.Code)
}

func TestHandler_CreateProfile_BadJSON(t *testing.T) {
	r, _ := setupHandlerRouter(t, true, "parent")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/profiles", strings.NewReader("not-json")))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateProfile_ValidationFailure(t *testing.T) {
	r, _ := setupHandlerRouter(t, true, "parent")
	body, _ := json.Marshal(CreateProfileRequest{
		RealName:        "",
		CandidateIDCard: "too-short",
		ProvinceID:      0,
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/profiles", bytes.NewReader(body)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateProfile_Success(t *testing.T) {
	r, svc := setupHandlerRouter(t, true, "parent")
	svc.On("CreateProfile", mock.Anything, int64(10), mock.Anything).Return(&ProfileResponse{ID: 1, UserID: 10}, nil)

	body, _ := json.Marshal(CreateProfileRequest{
		RealName:        "张三",
		CandidateIDCard: "110105200001011234",
		ProvinceID:      11,
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/profiles", bytes.NewReader(body)))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_GetProfile_InvalidID(t *testing.T) {
	r, _ := setupHandlerRouter(t, true, "parent")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/profiles/abc", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetProfile_NotFoundMapped(t *testing.T) {
	r, svc := setupHandlerRouter(t, true, "parent")
	svc.On("GetProfile", mock.Anything, int64(10), int64(7), "parent").Return(nil, web.NewError(web.ErrCodeNotFound, "档案不存在"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/profiles/7", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_DeleteProfile_ForbiddenMapped(t *testing.T) {
	r, svc := setupHandlerRouter(t, true, "parent")
	svc.On("DeleteProfile", mock.Anything, int64(10), int64(1)).Return(web.NewError(web.ErrCodeForbidden, "仅档案所有者可删除"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/profiles/1", nil))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandler_LookupByIDCard_Success(t *testing.T) {
	r, svc := setupHandlerRouter(t, true, "student")
	svc.On("LookupByIDCard", mock.Anything, "student", "110105200001011234").Return(&LookupResponse{
		ProfileID: 1, OwnerUserID: 10, OwnerEmail: "p@example.com", OwnerUserType: "parent",
	}, nil)

	body, _ := json.Marshal(LookupByIDCardRequest{IDCard: "110105200001011234"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/profiles/lookup/idcard", bytes.NewReader(body)))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_LookupByCode_BadLength(t *testing.T) {
	r, _ := setupHandlerRouter(t, true, "student")
	body, _ := json.Marshal(LookupByCodeRequest{Code: "12"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/profiles/lookup/code", bytes.NewReader(body)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GenerateInviteCode_Success(t *testing.T) {
	r, svc := setupHandlerRouter(t, true, "parent")
	svc.On("GenerateInviteCode", mock.Anything, int64(10), int64(1)).Return(&InviteResponse{
		Code:      "246810",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/profiles/1/invite-code", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}
