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

func (m *mockService) GetDatasetOverview(ctx context.Context, query *DatasetOverviewQuery) (*DatasetOverviewResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DatasetOverviewResponse), args.Error(1)
}

func (m *mockService) GetFacets(ctx context.Context, query *FacetsQuery) (*FacetsResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FacetsResponse), args.Error(1)
}

func (m *mockService) ListSchools(ctx context.Context, query *SchoolListQuery) (*ListResponse[School], error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ListResponse[School]), args.Error(1)
}

func (m *mockService) GetSchool(ctx context.Context, schoolID int64, query *SchoolDetailQuery) (*School, error) {
	args := m.Called(ctx, schoolID, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*School), args.Error(1)
}

func (m *mockService) CompareSchools(ctx context.Context, query *SchoolCompareQuery) (map[string]any, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *mockService) ListMajors(ctx context.Context, query *MajorListQuery) (*ListResponse[Major], error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ListResponse[Major]), args.Error(1)
}

func (m *mockService) GetMajor(ctx context.Context, majorID int64, query *MajorDetailQuery) (*Major, error) {
	args := m.Called(ctx, majorID, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Major), args.Error(1)
}

func (m *mockService) ListSchoolMajors(ctx context.Context, schoolID int64, query *SchoolMajorsQuery) (*ListResponse[SchoolMajorItem], error) {
	args := m.Called(ctx, schoolID, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ListResponse[SchoolMajorItem]), args.Error(1)
}

func (m *mockService) GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) (*EnrollmentPlanResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*EnrollmentPlanResponse), args.Error(1)
}

func (m *mockService) ListProvinceBatchLines(ctx context.Context, query *BatchLineQuery) (*ListResponse[ProvinceBatchLine], error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ListResponse[ProvinceBatchLine]), args.Error(1)
}

func (m *mockService) GetProvinceBatchLineTrend(ctx context.Context, query *BatchLineTrendQuery) (*BatchLineTrendResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BatchLineTrendResponse), args.Error(1)
}

func (m *mockService) ListSchoolAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[SchoolAdmissionScore], error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ListResponse[SchoolAdmissionScore]), args.Error(1)
}

func (m *mockService) ListMajorAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[MajorAdmissionScore], error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ListResponse[MajorAdmissionScore]), args.Error(1)
}

func (m *mockService) GetAdmissionScoreTrend(ctx context.Context, query *ScoreTrendQuery) (*ScoreTrendResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ScoreTrendResponse), args.Error(1)
}

func (m *mockService) GetScoreMatch(ctx context.Context, query *ScoreMatchQuery) (*ScoreMatchResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ScoreMatchResponse), args.Error(1)
}

func (m *mockService) GetEmploymentData(ctx context.Context, query *EmploymentDataQuery) (*EmploymentDataResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*EmploymentDataResponse), args.Error(1)
}

func TestHandler_GetDatasetOverview_Success(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)
	mockResponse := &DatasetOverviewResponse{Summary: DatasetSummary{SchoolCount: 2964}}
	svc.On("GetDatasetOverview", mock.Anything, mock.MatchedBy(func(q *DatasetOverviewQuery) bool {
		return q.IncludeCoverage
	})).Return(mockResponse, nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/dataset-overview?include_coverage=true", http.NoBody)
	h.GetDatasetOverview(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestHandler_GetEnrollmentPlans_Success(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	planCount := 50
	majorCode := "080901"
	schoolCode := "10001"
	mockResponse := &EnrollmentPlanResponse{
		Total: 100,
		Items: []EnrollmentPlan{
			{
				ID:         1,
				SchoolName: "北京大学",
				MajorName:  "计算机科学与技术",
				Province:   "北京",
				Year:       2024,
				PlanCount:  &planCount,
				Batch:      "本科批",
				MajorCode:  &majorCode,
				SchoolCode: &schoolCode,
			},
		},
		Page:    1,
		PerPage: 10,
	}
	mockResponse.Data = mockResponse.Items

	svc.On("GetEnrollmentPlans", mock.Anything, mock.Anything).Return(mockResponse, nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/enrollment-plans?page=1&per_page=10", http.NoBody)
	h.GetEnrollmentPlans(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)

	data, _ := json.Marshal(resp.Data)
	var result EnrollmentPlanResponse
	_ = json.Unmarshal(data, &result)
	assert.Equal(t, int64(100), result.Total)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, "北京大学", result.Items[0].SchoolName)
}

func TestHandler_GetEnrollmentPlans_WithFilter(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	mockResponse := &EnrollmentPlanResponse{
		Total:   10,
		Items:   []EnrollmentPlan{{ID: 1, SchoolName: "清华大学", MajorName: "软件工程", Province: "北京", Year: 2024}},
		Page:    1,
		PerPage: 10,
	}
	mockResponse.Data = mockResponse.Items

	svc.On("GetEnrollmentPlans", mock.Anything, mock.MatchedBy(func(q *EnrollmentPlanQuery) bool {
		return q.SchoolName == "清华" && q.Province == "北京"
	})).Return(mockResponse, nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/enrollment-plans?school_name=清华&province=北京", http.NoBody)
	h.GetEnrollmentPlans(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_QueryError(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	svc.On("GetEnrollmentPlans", mock.Anything, mock.Anything).Return(nil, &QueryError{Message: "bad query"})

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/enrollment-plans", http.NoBody)
	h.GetEnrollmentPlans(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, web.ErrCodeBadRequest, resp.Code)
}

func TestHandler_GetEmploymentData_Success(t *testing.T) {
	svc := new(mockService)
	h := NewHandler(svc)

	mockResponse := &EmploymentDataResponse{
		Total: 150,
		Data:  []EmploymentData{{ID: 1, MajorName: "计算机科学与技术", AverageSalary: 12000}},
		Page:  1, PerPage: 10,
	}
	svc.On("GetEmploymentData", mock.Anything, mock.Anything).Return(mockResponse, nil)

	c, w := setupTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analysis/employment-data?page=1&per_page=10", http.NoBody)
	h.GetEmploymentData(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}
