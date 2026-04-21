package analysis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func (m *mockService) GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) (*EnrollmentPlanResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*EnrollmentPlanResponse), args.Error(1)
}

func TestHandler_GetEnrollmentPlans_Success(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	mockResponse := &EnrollmentPlanResponse{
		Total: 100,
		Plans: []EnrollmentPlan{
			{
				ID:           1,
				SchoolName:   "北京大学",
				MajorName:    "计算机科学与技术",
				Province:     "北京",
				Year:         2024,
				PlanCount:    50,
				ActualCount:  48,
				MinScore:     680,
				AverageScore: 690,
				MaxScore:     700,
				Batch:        "一本",
				MajorCode:    "080901",
				SchoolCode:   "10001",
			},
		},
		Page:    1,
		PerPage: 10,
	}

	svc.On("GetEnrollmentPlans", mock.Anything, mock.Anything).Return(mockResponse, nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/enrollment-plans?page=1&per_page=10", http.NoBody)

	h.GetEnrollmentPlans(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)

	// 验证返回的数据
	data, _ := json.Marshal(resp.Data)
	var result EnrollmentPlanResponse
	_ = json.Unmarshal(data, &result)
	assert.Equal(t, 100, result.Total)
	assert.Equal(t, 1, len(result.Plans))
	assert.Equal(t, "北京大学", result.Plans[0].SchoolName)
}

func TestHandler_GetEnrollmentPlans_WithFilter(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	mockResponse := &EnrollmentPlanResponse{
		Total: 10,
		Plans: []EnrollmentPlan{
			{
				ID:         1,
				SchoolName: "清华大学",
				MajorName:  "软件工程",
				Province:   "北京",
				Year:       2024,
			},
		},
		Page:    1,
		PerPage: 10,
	}

	svc.On("GetEnrollmentPlans", mock.Anything, mock.MatchedBy(func(q *EnrollmentPlanQuery) bool {
		return q.SchoolName == "清华" && q.Province == "北京"
	})).Return(mockResponse, nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/enrollment-plans?school_name=清华&province=北京", http.NoBody)

	h.GetEnrollmentPlans(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestHandler_GetEnrollmentPlans_ServiceError(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	svc.On("GetEnrollmentPlans", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/enrollment-plans", http.NoBody)

	h.GetEnrollmentPlans(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, web.ErrCodeInternal, resp.Code)
}

func TestHandler_GetEnrollmentPlans_DefaultPagination(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	mockResponse := &EnrollmentPlanResponse{
		Total:   100,
		Plans:   []EnrollmentPlan{},
		Page:    1,
		PerPage: 10,
	}

	svc.On("GetEnrollmentPlans", mock.Anything, mock.MatchedBy(func(q *EnrollmentPlanQuery) bool {
		return q.Page == 0 && q.PerPage == 0 // 默认值
	})).Return(mockResponse, nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/enrollment-plans", http.NoBody)

	h.GetEnrollmentPlans(c)

	assert.Equal(t, http.StatusOK, w.Code)
}
