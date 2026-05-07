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

type stubMajorCatalogService struct {
	latestYear int
	majors     []StandardMajorResponse
	err        error
}

func (s stubMajorCatalogService) LatestCatalogYear(ctx context.Context) (int, error) {
	return s.latestYear, s.err
}

func (s stubMajorCatalogService) ListStandardMajors(ctx context.Context, filter StandardMajorFilter) ([]StandardMajorResponse, error) {
	return s.majors, s.err
}

func TestListStandardMajorsUsesLatestYearWhenYearIsOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewMajorCatalogHandler(stubMajorCatalogService{majors: []StandardMajorResponse{
		{CatalogYear: 2025, MajorCode: "080901", Name: "计算机科学与技术"},
	}})

	router := gin.New()
	router.GET("/api/v1/admission/standard-majors", handler.ListStandardMajors)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/standard-majors?q=计算机", http.NoBody)
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

func TestLatestCatalogYearReturnsYear(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewMajorCatalogHandler(stubMajorCatalogService{latestYear: 2025})

	router := gin.New()
	router.GET("/api/v1/admission/major-catalog/latest-year", handler.LatestCatalogYear)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/major-catalog/latest-year", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope web.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)

	data, ok := envelope.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(2025), data["catalog_year"])
}
