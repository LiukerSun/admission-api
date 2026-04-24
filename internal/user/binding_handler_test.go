package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockBindingSvc struct {
	mock.Mock
}

func (m *mockBindingSvc) BindStudent(ctx context.Context, parentID int64, studentEmail string) (*Binding, error) {
	args := m.Called(ctx, parentID, studentEmail)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Binding), args.Error(1)
}

func (m *mockBindingSvc) GetMyBindings(ctx context.Context, userID int64, userType string) (*BindingListResult, error) {
	args := m.Called(ctx, userID, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BindingListResult), args.Error(1)
}

func (m *mockBindingSvc) RemoveBinding(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestBindingHandler_CreateBinding_Success(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	svc.On("BindStudent", mock.Anything, int64(1), "student@test.com").
		Return(&Binding{ID: 10, ParentID: 1, StudentID: 5}, nil)

	body, _ := json.Marshal(CreateBindingRequest{StudentEmail: "student@test.com"})
	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/bindings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Set(middleware.ContextUserTypeKey, "parent")

	h.CreateBinding(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestBindingHandler_CreateBinding_NotParent(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	body, _ := json.Marshal(CreateBindingRequest{StudentEmail: "student@test.com"})
	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/bindings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, int64(5))
	c.Set(middleware.ContextUserTypeKey, "student")

	h.CreateBinding(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestBindingHandler_CreateBinding_StudentNotFound(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	svc.On("BindStudent", mock.Anything, int64(1), "notfound@test.com").
		Return(nil, ErrStudentNotFound)

	body, _ := json.Marshal(CreateBindingRequest{StudentEmail: "notfound@test.com"})
	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/bindings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Set(middleware.ContextUserTypeKey, "parent")

	h.CreateBinding(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBindingHandler_CreateBinding_AlreadyBound(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	svc.On("BindStudent", mock.Anything, int64(1), "student@test.com").
		Return(nil, ErrStudentAlreadyBound)

	body, _ := json.Marshal(CreateBindingRequest{StudentEmail: "student@test.com"})
	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/bindings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Set(middleware.ContextUserTypeKey, "parent")

	h.CreateBinding(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestBindingHandler_CreateBinding_InternalError(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	svc.On("BindStudent", mock.Anything, int64(1), "student@test.com").
		Return(nil, errors.New("db down"))

	body, _ := json.Marshal(CreateBindingRequest{StudentEmail: "student@test.com"})
	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/bindings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Set(middleware.ContextUserTypeKey, "parent")

	h.CreateBinding(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBindingHandler_GetMyBindings_Success(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	svc.On("GetMyBindings", mock.Anything, int64(1), "parent").
		Return(&BindingListResult{
			UserType: "parent",
			Bindings: []*BindingWithUser{
				{ID: 10, User: SafeUser{ID: 5, Email: "student@test.com"}, CreatedAt: "2026-04-20T10:00:00Z"},
			},
		}, nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/bindings", http.NoBody)
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Set(middleware.ContextUserTypeKey, "parent")

	h.GetMyBindings(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestBindingHandler_GetMyBindings_Unauthorized(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/bindings", http.NoBody)

	h.GetMyBindings(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBindingHandler_DeleteBinding_AdminSuccess(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	svc.On("RemoveBinding", mock.Anything, int64(10)).Return(nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bindings/10", http.NoBody)
	c.Set(middleware.ContextRoleKey, "admin")
	c.Params = gin.Params{{Key: "id", Value: "10"}}

	h.DeleteBinding(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestBindingHandler_DeleteBinding_NotAdmin(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bindings/10", http.NoBody)
	c.Set(middleware.ContextRoleKey, "user")
	c.Params = gin.Params{{Key: "id", Value: "10"}}

	h.DeleteBinding(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestBindingHandler_DeleteBinding_NotFound(t *testing.T) {
	svc := new(mockBindingSvc)
	h := NewBindingHandler(svc)

	svc.On("RemoveBinding", mock.Anything, int64(10)).Return(ErrBindingNotFound)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bindings/10", http.NoBody)
	c.Set(middleware.ContextRoleKey, "admin")
	c.Params = gin.Params{{Key: "id", Value: "10"}}

	h.DeleteBinding(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
