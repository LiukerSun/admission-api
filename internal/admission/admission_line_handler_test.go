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

type stubAdmissionLineService struct {
	lines []AdmissionLineResponse
	err   error
}

func (s stubAdmissionLineService) ListAdmissionLines(ctx context.Context, filter *AdmissionLineFilter) ([]AdmissionLineResponse, error) {
	return s.lines, s.err
}

type capturingAdmissionLineService struct {
	lines   []AdmissionLineResponse
	filters []AdmissionLineFilter
}

func (s *capturingAdmissionLineService) ListAdmissionLines(ctx context.Context, filter *AdmissionLineFilter) ([]AdmissionLineResponse, error) {
	s.filters = append(s.filters, *filter)
	return s.lines, nil
}

func TestListAdmissionLinesReturnsGroupMajorScores(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAdmissionLineHandler(stubAdmissionLineService{lines: []AdmissionLineResponse{
		{
			UniversityID:    1,
			UniversityCode:  "1003",
			UniversityName:  "清华大学",
			GroupCode:       "008",
			LocalMajorCode:  "25",
			LocalMajorName:  "计算机类",
			PlanCount:       intPtr(1),
			MinScore:        intPtr(721),
			MinRank:         intPtr(45),
			AdmissionYear:   2025,
			RegionCode:      "230000",
			SubjectCategory: "physics",
		},
	}})

	router := gin.New()
	router.GET("/api/v1/admission/admission-lines", handler.ListAdmissionLines)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/admission-lines?admission_year=2025&region_code=230000&subject_category_code=physics&university_ids=1&group_codes=008", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope web.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	data, ok := envelope.Data.([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
}

func TestListAdmissionLinesAcceptsSchoolCodesGroupsAndUsesLatestYearWhenOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingAdmissionLineService{lines: []AdmissionLineResponse{
		{
			UniversityID:           1,
			UniversityCode:         "1003",
			UniversityName:         "清华大学",
			GroupCode:              "008",
			SubjectRequirementCode: "physics_chemistry",
			LocalMajorCode:         "25",
			LocalMajorName:         "计算机类",
			PlanCount:              intPtr(2),
			MinScore:               intPtr(721),
			MinRank:                intPtr(45),
			Tuition:                intPtr(5000),
			Duration:               "四年",
			AdmissionRemark:        "含人工智能方向",
			AdmissionYear:          2025,
			RegionCode:             "230000",
			SubjectCategory:        "physics",
		},
		{
			UniversityID:    2,
			UniversityCode:  "1001",
			UniversityName:  "北京大学",
			GroupCode:       "009",
			LocalMajorCode:  "31",
			LocalMajorName:  "理科试验班类",
			MinScore:        intPtr(715),
			MinRank:         intPtr(80),
			AdmissionYear:   2025,
			RegionCode:      "230000",
			SubjectCategory: "physics",
		},
	}}
	handler := NewAdmissionLineHandler(service)

	router := gin.New()
	router.GET("/api/v1/admission/admission-lines", handler.ListAdmissionLines)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/admission-lines?region_code=230000&subject_category_code=physics&university_codes=1003,%201001&group_codes=008,%20009", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, service.filters, 1)
	require.Nil(t, service.filters[0].AdmissionYear)
	require.Equal(t, "230000", service.filters[0].RegionCode)
	require.Equal(t, "physics", service.filters[0].SubjectCategoryCode)
	require.Equal(t, []string{"1003", "1001"}, service.filters[0].UniversityCodes)
	require.Equal(t, []string{"008", "009"}, service.filters[0].GroupCodes)

	var envelope web.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	data, ok := envelope.Data.([]any)
	require.True(t, ok)
	require.Len(t, data, 2)

	first, ok := data[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "1003", first["university_code"])
	require.Equal(t, "008", first["group_code"])
	require.Equal(t, "25", first["local_major_code"])
	require.Equal(t, "计算机类", first["local_major_name"])
	require.Equal(t, float64(721), first["min_score"])
	require.Equal(t, float64(45), first["min_rank"])
	require.Equal(t, "physics_chemistry", first["subject_requirement_code"])
	require.Equal(t, float64(5000), first["tuition"])
	require.Equal(t, "四年", first["duration"])
	require.Equal(t, "含人工智能方向", first["admission_remark"])
}

func TestListAdmissionLinesReturnsEmptyArrayWhenNoGroupsMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAdmissionLineHandler(stubAdmissionLineService{lines: []AdmissionLineResponse{}})

	router := gin.New()
	router.GET("/api/v1/admission/admission-lines", handler.ListAdmissionLines)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/admission-lines?admission_year=2025&region_code=230000&subject_category_code=physics&university_codes=9999&group_codes=999", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope web.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	data, ok := envelope.Data.([]any)
	require.True(t, ok)
	require.Empty(t, data)
}

func TestListAdmissionLinesAcceptsTagAndScoreRankFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingAdmissionLineService{lines: []AdmissionLineResponse{}}
	handler := NewAdmissionLineHandler(service)

	router := gin.New()
	router.GET("/api/v1/admission/admission-lines", handler.ListAdmissionLines)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/admission-lines?region_code=230000&subject_category_code=physics&tag_catalog_year=2025&tag_query=计算机&tag_category_code=08&tag_class_code=0809&tag_major_code=080901&min_rank_from=1&min_rank_to=1000&min_score_from=650&min_score_to=750", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, service.filters, 1)
	require.Equal(t, 2025, *service.filters[0].TagCatalogYear)
	require.Equal(t, "计算机", service.filters[0].TagQuery)
	require.Equal(t, "08", service.filters[0].TagCategoryCode)
	require.Equal(t, "0809", service.filters[0].TagClassCode)
	require.Equal(t, "080901", service.filters[0].TagMajorCode)
	require.Equal(t, 1, *service.filters[0].MinRankFrom)
	require.Equal(t, 1000, *service.filters[0].MinRankTo)
	require.Equal(t, 650, *service.filters[0].MinScoreFrom)
	require.Equal(t, 750, *service.filters[0].MinScoreTo)
}

func TestListAdmissionLinesAcceptsProfileBooleanFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingAdmissionLineService{lines: []AdmissionLineResponse{}}
	handler := NewAdmissionLineHandler(service)

	router := gin.New()
	router.GET("/api/v1/admission/admission-lines", handler.ListAdmissionLines)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/admission-lines?region_code=230000&subject_category_code=physics&is_985=true&is_211=false&is_double_first_class=true", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, service.filters, 1)
	require.NotNil(t, service.filters[0].Is985)
	require.True(t, *service.filters[0].Is985)
	require.NotNil(t, service.filters[0].Is211)
	require.False(t, *service.filters[0].Is211)
	require.NotNil(t, service.filters[0].IsDoubleFirstClass)
	require.True(t, *service.filters[0].IsDoubleFirstClass)
}

func TestListAdmissionLinesAcceptsProfileLocationFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingAdmissionLineService{lines: []AdmissionLineResponse{}}
	handler := NewAdmissionLineHandler(service)

	router := gin.New()
	router.GET("/api/v1/admission/admission-lines", handler.ListAdmissionLines)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/admission-lines?cities=哈尔滨,上海&exclude_cities=北京&provinces=230000&exclude_provinces=110000&subject_categories=physics,history", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, service.filters, 1)
	require.Equal(t, []string{"哈尔滨", "上海"}, service.filters[0].Cities)
	require.Equal(t, []string{"北京"}, service.filters[0].ExcludeCities)
	require.Equal(t, []string{"230000"}, service.filters[0].Provinces)
	require.Equal(t, []string{"110000"}, service.filters[0].ExcludeProvinces)
	require.Equal(t, []string{"physics", "history"}, service.filters[0].SubjectCategories)
}

func intPtr(v int) *int {
	return &v
}
