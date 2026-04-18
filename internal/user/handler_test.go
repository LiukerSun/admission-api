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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockService struct {
	mock.Mock
}

func (m *mockService) Register(ctx context.Context, email, password string) (*User, error) {
	args := m.Called(ctx, email, password)
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

func TestHandler_Register(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc, nil)

	svc.On("Register", mock.Anything, "new@example.com", "password123").
		Return(&User{ID: 1, Email: "new@example.com", Role: "user"}, nil)

	body, _ := json.Marshal(RegisterRequest{Email: "new@example.com", Password: "password123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp web.Response
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestHandler_Register_InvalidBody(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader([]byte("invalid")))
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_Me_Unauthorized(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", http.NoBody)
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
