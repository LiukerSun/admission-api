package admission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubAggregateService struct {
	resp *AggregateResponse
	err  error
}

func (s stubAggregateService) Aggregate(ctx context.Context, filter *AggregateFilter) (*AggregateResponse, error) {
	return s.resp, s.err
}

type capturingAggregateService struct {
	resp    *AggregateResponse
	filters []AggregateFilter
}

func (s *capturingAggregateService) Aggregate(ctx context.Context, filter *AggregateFilter) (*AggregateResponse, error) {
	s.filters = append(s.filters, *filter)
	return s.resp, nil
}

func TestAggregateHandlerReturnsGroupByCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAggregateHandler(stubAggregateService{
		resp: &AggregateResponse{
			GroupBy: "province",
			Total:   3,
			Items: []AggregateItem{
				{Code: "110000", Name: "北京", Count: 2},
				{Code: "310000", Name: "上海", Count: 1},
			},
		},
	})

	router := gin.New()
	router.GET("/api/v1/admission/aggregate", handler.Aggregate)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/aggregate?region_code=230000&subject_category_code=physics&group_by=province&metrics=count", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope web.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	data, ok := envelope.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "province", data["group_by"])
	require.Equal(t, float64(3), data["total"])
	items, ok := data["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)
}

func TestAggregateHandlerParsesAllFilterParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingAggregateService{resp: &AggregateResponse{}}
	handler := NewAggregateHandler(service)

	router := gin.New()
	router.GET("/api/v1/admission/aggregate", handler.Aggregate)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/aggregate?admission_year=2025&region_code=230000&subject_category_code=physics&university_ids=1,2&university_codes=1001,1003&group_codes=008,009&tag_catalog_year=2025&tag_query=计算机&tag_category_code=08&tag_class_code=0809&tag_major_code=080901&min_rank_from=1&min_rank_to=1000&min_score_from=650&min_score_to=750&is_985=true&is_211=false&is_double_first_class=true&group_by=province&metrics=count,avg_min_score", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, service.filters, 1)
	f := service.filters[0]
	require.Equal(t, 2025, *f.AdmissionYear)
	require.Equal(t, "230000", f.RegionCode)
	require.Equal(t, "physics", f.SubjectCategoryCode)
	require.Equal(t, []int64{1, 2}, f.UniversityIDs)
	require.Equal(t, []string{"1001", "1003"}, f.UniversityCodes)
	require.Equal(t, []string{"008", "009"}, f.GroupCodes)
	require.Equal(t, 2025, *f.TagCatalogYear)
	require.Equal(t, "计算机", f.TagQuery)
	require.Equal(t, "08", f.TagCategoryCode)
	require.Equal(t, "0809", f.TagClassCode)
	require.Equal(t, "080901", f.TagMajorCode)
	require.Equal(t, 1, *f.MinRankFrom)
	require.Equal(t, 1000, *f.MinRankTo)
	require.Equal(t, 650, *f.MinScoreFrom)
	require.Equal(t, 750, *f.MinScoreTo)
	require.NotNil(t, f.Is985)
	require.True(t, *f.Is985)
	require.NotNil(t, f.Is211)
	require.False(t, *f.Is211)
	require.NotNil(t, f.IsDoubleFirstClass)
	require.True(t, *f.IsDoubleFirstClass)
	require.Equal(t, "province", f.GroupBy)
	require.Equal(t, []string{"count", "avg_min_score"}, f.Metrics)
}

func TestAggregateHandlerDefaultsGroupByToProvince(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingAggregateService{resp: &AggregateResponse{}}
	handler := NewAggregateHandler(service)

	router := gin.New()
	router.GET("/api/v1/admission/aggregate", handler.Aggregate)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/aggregate?region_code=230000&subject_category_code=physics&metrics=count", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, service.filters, 1)
	require.Equal(t, "province", service.filters[0].GroupBy)
}

func TestAggregateHandlerReturnsErrorOnInvalidAdmissionYear(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAggregateHandler(stubAggregateService{})

	router := gin.New()
	router.GET("/api/v1/admission/aggregate", handler.Aggregate)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/aggregate?admission_year=abc&metrics=count", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
