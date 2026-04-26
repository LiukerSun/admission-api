package candidate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockActivityLogService struct {
	mock.Mock
}

func (m *mockActivityLogService) LogActivity(ctx context.Context, input CreateActivityInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *mockActivityLogService) ListActivities(ctx context.Context, filter ActivityFilter, page, pageSize int) (*ActivityLogListResponse, error) {
	args := m.Called(ctx, filter, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ActivityLogListResponse), args.Error(1)
}

func (m *mockActivityLogService) GetMyActivities(ctx context.Context, userID int64, page, pageSize int) (*ActivityLogListResponse, error) {
	args := m.Called(ctx, userID, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ActivityLogListResponse), args.Error(1)
}

func (m *mockActivityLogService) GetStats(ctx context.Context, targetType string, targetID int64) (*ActivityStatsResponse, error) {
	args := m.Called(ctx, targetType, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ActivityStatsResponse), args.Error(1)
}

func (m *mockActivityLogService) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	args := m.Called(ctx, ids)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockActivityLogService) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	args := m.Called(ctx, before)
	return args.Get(0).(int64), args.Error(1)
}

func setupHandlerTest() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestActivityLogHandler_ListActivities_Success(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	targetType := "school"
	mockResponse := &ActivityLogListResponse{
		Logs: []*ActivityLog{
			{ID: 1, UserID: 1, ActivityType: "view_school", TargetType: &targetType, TargetID: int64Ptr(123)},
		},
		Total: 1,
	}

	svc.On("ListActivities", mock.Anything, mock.MatchedBy(func(f ActivityFilter) bool {
		return f.ActivityType == "view_school" && f.UserID == 1
	}), 1, 20).Return(mockResponse, nil)

	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/candidate/activities?user_id=1&activity_type=view_school", http.NoBody)
	h.ListActivities(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestActivityLogHandler_ListActivities_WithTimeRange(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	mockResponse := &ActivityLogListResponse{Logs: []*ActivityLog{}, Total: 0}
	svc.On("ListActivities", mock.Anything, mock.Anything, 1, 20).Return(mockResponse, nil)

	c, w := setupHandlerTest()
	start := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	end := time.Now().Format(time.RFC3339)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/candidate/activities?start_time="+start+"&end_time="+end, http.NoBody)
	h.ListActivities(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestActivityLogHandler_GetMyActivities_Success(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	mockResponse := &ActivityLogListResponse{
		Logs:  []*ActivityLog{{ID: 1, UserID: 1, ActivityType: "view_major"}},
		Total: 1,
	}
	svc.On("GetMyActivities", mock.Anything, int64(1), 1, 20).Return(mockResponse, nil)

	c, w := setupHandlerTest()
	c.Set(middleware.ContextUserIDKey, int64(1))
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/me/activities", http.NoBody)
	h.GetMyActivities(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestActivityLogHandler_GetMyActivities_Unauthorized(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/me/activities", http.NoBody)
	h.GetMyActivities(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, web.ErrCodeUnauthorized, resp.Code)
}

func TestActivityLogHandler_GetStats_Success(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	mockResponse := &ActivityStatsResponse{TargetType: "school", TargetID: 123, Count: 42}
	svc.On("GetStats", mock.Anything, "school", int64(123)).Return(mockResponse, nil)

	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/candidate/activities/stats?target_type=school&target_id=123", http.NoBody)
	h.GetStats(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestActivityLogHandler_GetStats_InvalidParams(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/candidate/activities/stats?target_type=school&target_id=abc", http.NoBody)
	h.GetStats(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, web.ErrCodeBadRequest, resp.Code)
}

func TestActivityLogHandler_GetStats_ServiceError(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	svc.On("GetStats", mock.Anything, "school", int64(123)).Return(nil, web.NewError(web.ErrCodeBadRequest, "invalid target"))

	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/candidate/activities/stats?target_type=school&target_id=123", http.NoBody)
	h.GetStats(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, web.ErrCodeBadRequest, resp.Code)
}

func TestActivityLogHandler_DeleteByIDs_Success(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	svc.On("DeleteByIDs", mock.Anything, []int64{1, 2, 3}).Return(int64(3), nil)

	body, _ := json.Marshal(map[string][]int64{"ids": {1, 2, 3}})
	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/candidate/activities", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.DeleteByIDs(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestActivityLogHandler_DeleteByIDs_ServiceError(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	svc.On("DeleteByIDs", mock.Anything, []int64{1, 2, 3}).Return(int64(0), web.NewError(web.ErrCodeBadRequest, "invalid ids"))

	body, _ := json.Marshal(map[string][]int64{"ids": {1, 2, 3}})
	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/candidate/activities", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.DeleteByIDs(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestActivityLogHandler_DeleteBefore_Success(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	before := time.Now().Add(-24 * time.Hour)
	svc.On("DeleteBefore", mock.Anything, mock.MatchedBy(func(t time.Time) bool {
		return t.Before(time.Now().Add(-1*time.Hour))
	})).Return(int64(100), nil)

	body, _ := json.Marshal(map[string]string{"before": before.Format(time.RFC3339)})
	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/candidate/activities/before", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.DeleteBefore(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestActivityLogHandler_DeleteBefore_InvalidTime(t *testing.T) {
	svc := new(mockActivityLogService)
	h := NewActivityLogHandler(svc)

	body, _ := json.Marshal(map[string]string{"before": "not-a-time"})
	c, w := setupHandlerTest()
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/candidate/activities/before", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.DeleteBefore(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, web.ErrCodeBadRequest, resp.Code)
}
