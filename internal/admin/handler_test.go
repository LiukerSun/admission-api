package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupTest() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

type mockService struct {
	mock.Mock
}

func (m *mockService) ListUsers(ctx context.Context, filter ListUsersFilter, page, pageSize int) (*UserListResponse, error) {
	args := m.Called(ctx, filter, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserListResponse), args.Error(1)
}

func (m *mockService) GetUser(ctx context.Context, id int64) (*UserResponse, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserResponse), args.Error(1)
}

func (m *mockService) UpdateRole(ctx context.Context, id int64, role string) error {
	args := m.Called(ctx, id, role)
	return args.Error(0)
}

func (m *mockService) UpdateUser(ctx context.Context, id int64, req UpdateUserRequest) (*UserResponse, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserResponse), args.Error(1)
}

func (m *mockService) ResetPassword(ctx context.Context, id int64, newPassword string) error {
	args := m.Called(ctx, id, newPassword)
	return args.Error(0)
}

func (m *mockService) DisableUser(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockService) EnableUser(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockService) ListBindings(ctx context.Context, page, pageSize int) (*BindingListResponse, error) {
	args := m.Called(ctx, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BindingListResponse), args.Error(1)
}

func (m *mockService) GetStats(ctx context.Context) (*StatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*StatsResponse), args.Error(1)
}

func TestHandler_GetUser(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	updatedAt := time.Now().UTC()
	svc.On("GetUser", mock.Anything, int64(7)).Return(&UserResponse{
		ID:        7,
		Email:     "user@example.com",
		Username:  "alice",
		Role:      "user",
		UserType:  "student",
		Status:    "active",
		CreatedAt: updatedAt,
		UpdatedAt: updatedAt,
	}, nil)

	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/7", http.NoBody)

	h.GetUser(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestHandler_GetUser_NotFound(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	svc.On("GetUser", mock.Anything, int64(7)).Return(nil, user.ErrUserNotFound)

	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/7", http.NoBody)

	h.GetUser(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_UpdateUser(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	email := "updated@example.com"
	username := "alice"
	role := "premium"
	userType := "student"
	status := "active"
	now := time.Now().UTC()

	req := UpdateUserRequest{
		Email:    &email,
		Username: &username,
		Role:     &role,
		UserType: &userType,
		Status:   &status,
	}

	svc.On("UpdateUser", mock.Anything, int64(7), req).Return(&UserResponse{
		ID:        7,
		Email:     email,
		Username:  username,
		Role:      role,
		UserType:  userType,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil)

	body, _ := json.Marshal(req)
	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateUser(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestHandler_ResetPassword(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	svc.On("ResetPassword", mock.Anything, int64(7), "newpass123").Return(nil)

	body, _ := json.Marshal(ResetPasswordRequest{NewPassword: "newpass123"})
	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ResetPassword(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_UpdateUser_Conflict(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	email := "updated@example.com"
	req := UpdateUserRequest{Email: &email}
	svc.On("UpdateUser", mock.Anything, int64(7), req).Return(nil, user.ErrEmailAlreadyExists)

	body, _ := json.Marshal(req)
	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateUser(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestHandler_UpdateRole_NotFound(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	svc.On("UpdateRole", mock.Anything, int64(7), "premium").Return(user.ErrUserNotFound)

	body, _ := json.Marshal(UpdateRoleRequest{Role: "premium"})
	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/role", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateRole(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_ResetPassword_InvalidPassword(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	svc.On("ResetPassword", mock.Anything, int64(7), "newpass123").Return(errors.New("invalid password: validation failed"))

	body, _ := json.Marshal(ResetPasswordRequest{NewPassword: "newpass123"})
	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ResetPassword(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_DisableUser_NotFound(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	svc.On("DisableUser", mock.Anything, int64(7)).Return(user.ErrUserNotFound)

	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/disable", http.NoBody)

	h.DisableUser(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_EnableUser_NotFound(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	svc.On("EnableUser", mock.Anything, int64(7)).Return(user.ErrUserNotFound)

	c, w := setupTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/enable", http.NoBody)

	h.EnableUser(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
