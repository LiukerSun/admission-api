package candidate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockIntentionService

type mockIntentionService struct{ mock.Mock }

func (m *mockIntentionService) GetIntentions(ctx context.Context, userID int64, profileID int64) (*IntentionGroupResponse, error) {
	args := m.Called(ctx, userID, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*IntentionGroupResponse), args.Error(1)
}

func (m *mockIntentionService) SaveIntentions(ctx context.Context, userID int64, profileID int64, intentionType string, req *SaveIntentionsRequest) error {
	return m.Called(ctx, userID, profileID, intentionType, req).Error(0)
}

func (m *mockIntentionService) RemoveIntention(ctx context.Context, userID int64, intentionID int64) error {
	return m.Called(ctx, userID, intentionID).Error(0)
}

func (m *mockIntentionService) ClearIntentions(ctx context.Context, userID int64, profileID int64, intentionType string) error {
	return m.Called(ctx, userID, profileID, intentionType).Error(0)
}

func setupIntentionHandlerRouter(t *testing.T, withAuth bool) (*gin.Engine, *mockIntentionService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	svc := &mockIntentionService{}
	h := NewIntentionHandler(svc)

	if withAuth {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ContextUserIDKey, int64(10))
			c.Next()
		})
	}

	r.GET("/candidate/intentions/:profile_id", h.GetIntentions)
	r.PUT("/candidate/intentions/:profile_id/:type", h.SaveIntentions)
	r.DELETE("/candidate/intentions/by_profile_id/:profile_id/:type", h.ClearIntentions)
	r.DELETE("/candidate/intentions/:id", h.RemoveIntention)

	return r, svc
}

func TestIntentionHandler_RequiresAuth(t *testing.T) {
	r, _ := setupIntentionHandlerRouter(t, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/candidate/intentions/1", http.NoBody)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestIntentionHandler_GetIntentions_Success(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("GetIntentions", mock.Anything, int64(10), int64(1)).Return(&IntentionGroupResponse{
		School: []*Intention{{ID: 1, TargetID: "100", TargetName: strPtr("清华")}},
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/intentions/1", http.NoBody))

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResp(t, w)
	assert.Equal(t, 0, resp.Code)
}

func TestIntentionHandler_GetIntentions_InvalidProfileID(t *testing.T) {
	r, _ := setupIntentionHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/intentions/abc", http.NoBody))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestIntentionHandler_GetIntentions_ServiceError(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("GetIntentions", mock.Anything, int64(10), int64(1)).Return(nil, web.NewError(web.ErrCodeForbidden, "无权访问"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/intentions/1", http.NoBody))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestIntentionHandler_SaveIntentions_Success(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("SaveIntentions", mock.Anything, int64(10), int64(1), "school", mock.Anything).Return(nil)

	body, _ := json.Marshal(SaveIntentionsRequest{
		Items: []IntentionItemInput{
			{TargetID: "100", TargetName: "清华", Priority: 0},
			{TargetID: "101", TargetName: "北大", Priority: 1},
		},
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/intentions/1/school", bytes.NewReader(body)))

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIntentionHandler_SaveIntentions_BadJSON(t *testing.T) {
	r, _ := setupIntentionHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/intentions/1/school", strings.NewReader("not-json")))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestIntentionHandler_SaveIntentions_InvalidProfileID(t *testing.T) {
	r, _ := setupIntentionHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/intentions/abc/school", http.NoBody))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestIntentionHandler_SaveIntentions_ServiceForbidden(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("SaveIntentions", mock.Anything, int64(10), int64(1), "school", mock.Anything).Return(web.NewError(web.ErrCodeForbidden, "无权访问"))

	body, _ := json.Marshal(SaveIntentionsRequest{
		Items: []IntentionItemInput{{TargetID: "100", TargetName: "清华"}},
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/intentions/1/school", bytes.NewReader(body)))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestIntentionHandler_SaveIntentions_ServiceNotFound(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("SaveIntentions", mock.Anything, int64(10), int64(1), "school", mock.Anything).Return(web.NewError(web.ErrCodeNotFound, "档案不存在"))

	body, _ := json.Marshal(SaveIntentionsRequest{
		Items: []IntentionItemInput{{TargetID: "100", TargetName: "清华"}},
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/intentions/1/school", bytes.NewReader(body)))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestIntentionHandler_RemoveIntention_Success(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("RemoveIntention", mock.Anything, int64(10), int64(5)).Return(nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/intentions/5", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIntentionHandler_RemoveIntention_InvalidID(t *testing.T) {
	r, _ := setupIntentionHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/intentions/abc", http.NoBody))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestIntentionHandler_RemoveIntention_NotFound(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("RemoveIntention", mock.Anything, int64(10), int64(5)).Return(web.NewError(web.ErrCodeNotFound, "意向不存在"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/intentions/5", http.NoBody))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestIntentionHandler_ClearIntentions_Success(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("ClearIntentions", mock.Anything, int64(10), int64(1), "school").Return(nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/intentions/by_profile_id/1/school", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIntentionHandler_ClearIntentions_InvalidProfileID(t *testing.T) {
	r, _ := setupIntentionHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/intentions/by_profile_id/abc/school", http.NoBody))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestIntentionHandler_ClearIntentions_Forbidden(t *testing.T) {
	r, svc := setupIntentionHandlerRouter(t, true)
	svc.On("ClearIntentions", mock.Anything, int64(10), int64(1), "school").Return(web.NewError(web.ErrCodeForbidden, "无权访问"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/intentions/1/school", http.NoBody))
	assert.Equal(t, http.StatusForbidden, w.Code)
}
