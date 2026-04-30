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

// mockExamRecordServiceForHandler
type mockExamRecordServiceForHandler struct{ mock.Mock }

func (m *mockExamRecordServiceForHandler) ListByProfile(ctx context.Context, userID, profileID int64) ([]*ExamRecordResponse, error) {
	args := m.Called(ctx, userID, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ExamRecordResponse), args.Error(1)
}

func (m *mockExamRecordServiceForHandler) GetByID(ctx context.Context, userID, recordID int64) (*ExamRecordResponse, error) {
	args := m.Called(ctx, userID, recordID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ExamRecordResponse), args.Error(1)
}

func (m *mockExamRecordServiceForHandler) Create(ctx context.Context, userID, profileID int64, req CreateExamRecordRequest) (*ExamRecordResponse, error) {
	args := m.Called(ctx, userID, profileID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ExamRecordResponse), args.Error(1)
}

func (m *mockExamRecordServiceForHandler) Update(ctx context.Context, userID, recordID int64, req UpdateExamRecordRequest) (*ExamRecordResponse, error) {
	args := m.Called(ctx, userID, recordID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ExamRecordResponse), args.Error(1)
}

func (m *mockExamRecordServiceForHandler) Void(ctx context.Context, userID, recordID int64) error {
	return m.Called(ctx, userID, recordID).Error(0)
}

func (m *mockExamRecordServiceForHandler) ListScoreHistories(ctx context.Context, userID, recordID int64) ([]*ScoreHistoryResponse, error) {
	args := m.Called(ctx, userID, recordID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ScoreHistoryResponse), args.Error(1)
}

func setupExamRecordHandlerRouter(t *testing.T, withAuth bool) (*gin.Engine, *mockExamRecordServiceForHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	svc := &mockExamRecordServiceForHandler{}
	h := NewExamRecordHandler(svc)

	if withAuth {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ContextUserIDKey, int64(10))
			c.Next()
		})
	}

	r.GET("/candidate/exam-records/by_profile_id/:profile_id", h.ListByProfile)
	r.POST("/candidate/exam-records/by_profile_id/:profile_id", h.Create)
	r.GET("/candidate/exam-records/:id", h.GetByID)
	r.PUT("/candidate/exam-records/:id", h.Update)
	r.DELETE("/candidate/exam-records/:id", h.Void)
	r.GET("/candidate/exam-records/:id/score-histories", h.ListScoreHistories)

	return r, svc
}

// --- Auth ---

func TestExamRecordHandler_RequiresAuth(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/candidate/exam-records/by_profile_id/1", http.NoBody)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- ListByProfile ---

func TestExamRecordHandler_ListByProfile_Success(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("ListByProfile", mock.Anything, int64(10), int64(1)).Return([]*ExamRecordResponse{
		{ID: 1, ProfileID: 1, ExamYear: 2026, ExamModel: "3+1+2", IsCurrent: true, Status: "active", CanWrite: true},
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/by_profile_id/1", http.NoBody))

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResp(t, w)
	assert.Equal(t, 0, resp.Code)
}

func TestExamRecordHandler_ListByProfile_InvalidProfileID(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/by_profile_id/abc", http.NoBody))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExamRecordHandler_ListByProfile_ServiceForbidden(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("ListByProfile", mock.Anything, int64(10), int64(1)).Return(nil, web.NewError(web.ErrCodeForbidden, "无权访问"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/by_profile_id/1", http.NoBody))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestExamRecordHandler_ListByProfile_ServiceNotFound(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("ListByProfile", mock.Anything, int64(10), int64(1)).Return(nil, web.NewError(web.ErrCodeNotFound, "档案不存在"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/by_profile_id/1", http.NoBody))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- Create ---

func TestExamRecordHandler_Create_Success(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("Create", mock.Anything, int64(10), int64(1), mock.Anything).Return(&ExamRecordResponse{
		ID: 3, ProfileID: 1, ExamYear: 2026, ExamModel: "3+1+2", IsCurrent: true, Status: "active",
	}, nil)

	body, _ := json.Marshal(CreateExamRecordRequest{ExamYear: 2026, ExamModel: "3+1+2", TotalScore: 650})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/candidate/exam-records/by_profile_id/1", bytes.NewReader(body)))

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResp(t, w)
	assert.Equal(t, 0, resp.Code)
}

func TestExamRecordHandler_Create_InvalidProfileID(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, true)
	body, _ := json.Marshal(CreateExamRecordRequest{ExamYear: 2026, ExamModel: "3+1+2"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/candidate/exam-records/by_profile_id/abc", bytes.NewReader(body)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExamRecordHandler_Create_BadJSON(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/candidate/exam-records/by_profile_id/1", strings.NewReader("not-json")))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExamRecordHandler_Create_ServiceForbidden(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("Create", mock.Anything, int64(10), int64(1), mock.Anything).Return(nil, web.NewError(web.ErrCodeForbidden, "无权访问"))

	body, _ := json.Marshal(CreateExamRecordRequest{ExamYear: 2026, ExamModel: "3+1+2"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/candidate/exam-records/by_profile_id/1", bytes.NewReader(body)))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- GetByID ---

func TestExamRecordHandler_GetByID_Success(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("GetByID", mock.Anything, int64(10), int64(1)).Return(&ExamRecordResponse{
		ID: 1, ProfileID: 1, ExamYear: 2026, ExamModel: "3+1+2", IsCurrent: true, Status: "active", CanWrite: true,
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/1", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestExamRecordHandler_GetByID_InvalidID(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/abc", http.NoBody))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExamRecordHandler_GetByID_NotFound(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("GetByID", mock.Anything, int64(10), int64(1)).Return(nil, web.NewError(web.ErrCodeNotFound, "记录不存在"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/1", http.NoBody))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- Update ---

func TestExamRecordHandler_Update_Success(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("Update", mock.Anything, int64(10), int64(1), mock.Anything).Return(&ExamRecordResponse{
		ID: 1, ProfileID: 1, ExamYear: 2027, ExamModel: "3+3", Status: "active",
	}, nil)

	body, _ := json.Marshal(UpdateExamRecordRequest{ExamYear: 2027, ExamModel: "3+3"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/exam-records/1", bytes.NewReader(body)))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestExamRecordHandler_Update_InvalidID(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, true)
	body, _ := json.Marshal(UpdateExamRecordRequest{ExamYear: 2027})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/exam-records/abc", bytes.NewReader(body)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExamRecordHandler_Update_BadJSON(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/exam-records/1", strings.NewReader("not-json")))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExamRecordHandler_Update_ServiceForbidden(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("Update", mock.Anything, int64(10), int64(1), mock.Anything).Return(nil, web.NewError(web.ErrCodeForbidden, "无权访问"))

	body, _ := json.Marshal(UpdateExamRecordRequest{ExamYear: 2027})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/candidate/exam-records/1", bytes.NewReader(body)))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- Void ---

func TestExamRecordHandler_Void_Success(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("Void", mock.Anything, int64(10), int64(1)).Return(nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/exam-records/1", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestExamRecordHandler_Void_InvalidID(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/exam-records/abc", http.NoBody))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExamRecordHandler_Void_NotFound(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("Void", mock.Anything, int64(10), int64(1)).Return(web.NewError(web.ErrCodeNotFound, "记录不存在"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/candidate/exam-records/1", http.NoBody))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- ListScoreHistories ---

func TestExamRecordHandler_ListScoreHistories_Success(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("ListScoreHistories", mock.Anything, int64(10), int64(1)).Return([]*ScoreHistoryResponse{
		{ID: 1, ExamRecordID: 1, NewTotalScore: 660, NewRankValue: 4800},
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/1/score-histories", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestExamRecordHandler_ListScoreHistories_InvalidID(t *testing.T) {
	r, _ := setupExamRecordHandlerRouter(t, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/abc/score-histories", http.NoBody))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExamRecordHandler_ListScoreHistories_Forbidden(t *testing.T) {
	r, svc := setupExamRecordHandlerRouter(t, true)
	svc.On("ListScoreHistories", mock.Anything, int64(10), int64(1)).Return(nil, web.NewError(web.ErrCodeForbidden, "无权访问"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/candidate/exam-records/1/score-histories", http.NoBody))
	assert.Equal(t, http.StatusForbidden, w.Code)
}
