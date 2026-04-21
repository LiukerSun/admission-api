package user

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

func setupTest() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

type mockService struct {
	mock.Mock
}

func (m *mockService) Register(ctx context.Context, email, password, userType string) (*User, error) {
	args := m.Called(ctx, email, password, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *mockService) Login(ctx context.Context, email, password, platform string) (*middleware.TokenPair, error) {
	args := m.Called(ctx, email, password, platform)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*middleware.TokenPair), args.Error(1)
}

func (m *mockService) Refresh(ctx context.Context, refreshToken string) (*middleware.TokenPair, error) {
	args := m.Called(ctx, refreshToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*middleware.TokenPair), args.Error(1)
}

func (m *mockService) Me(ctx context.Context, userID int64) (*User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *mockService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	args := m.Called(ctx, userID, currentPassword, newPassword)
	return args.Error(0)
}

type mockPhoneVerificationService struct {
	mock.Mock
}

func (m *mockPhoneVerificationService) SendPhoneVerificationCode(ctx context.Context, userID int64, phone string) error {
	args := m.Called(ctx, userID, phone)
	return args.Error(0)
}

func (m *mockPhoneVerificationService) VerifyPhoneCode(ctx context.Context, userID int64, phone, code string) error {
	args := m.Called(ctx, userID, phone, code)
	return args.Error(0)
}

func TestHandler_Register(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc, new(mockPhoneVerificationService), nil)

	svc.On("Register", mock.Anything, "new@example.com", "password123", "student").
		Return(&User{ID: 1, Email: "new@example.com", Role: "user", UserType: "student"}, nil)

	body, _ := json.Marshal(RegisterRequest{Email: "new@example.com", Password: "password123", UserType: "student"})
	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Register(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestHandler_Register_InvalidBody(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc, new(mockPhoneVerificationService), nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader([]byte("invalid")))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Register(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Me_Unauthorized(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc, new(mockPhoneVerificationService), nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/me", http.NoBody)

	h.Me(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_ChangePassword(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc, new(mockPhoneVerificationService), nil)

	svc.On("ChangePassword", mock.Anything, int64(1), "oldpass123", "newpass123").Return(nil)

	body, _ := json.Marshal(ChangePasswordRequest{
		CurrentPassword: "oldpass123",
		NewPassword:     "newpass123",
	})
	c, w := setupTest()
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/me/password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ChangePassword(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_SendPhoneVerificationCode(t *testing.T) {
	svc := new(mockService)
	phoneSvc := new(mockPhoneVerificationService)
	h := NewHandler(svc, phoneSvc, nil)

	phoneSvc.On("SendPhoneVerificationCode", mock.Anything, int64(1), "13800138000").Return(nil)

	body, _ := json.Marshal(SendPhoneCodeRequest{Phone: "13800138000"})
	c, w := setupTest()
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/me/phone/send-code", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.SendPhoneVerificationCode(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_VerifyPhone(t *testing.T) {
	svc := new(mockService)
	phoneSvc := new(mockPhoneVerificationService)
	h := NewHandler(svc, phoneSvc, nil)

	phoneSvc.On("VerifyPhoneCode", mock.Anything, int64(1), "13800138000", "123456").Return(nil)

	body, _ := json.Marshal(VerifyPhoneRequest{Phone: "13800138000", Code: "123456"})
	c, w := setupTest()
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/me/phone/verify", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.VerifyPhone(c)

	assert.Equal(t, http.StatusOK, w.Code)
}
