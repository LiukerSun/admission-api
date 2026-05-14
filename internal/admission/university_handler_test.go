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

type stubUniversityService struct {
	universities []UniversityResponse
	profile      *UniversityProfileResponse
	err          error
}

func (s stubUniversityService) ListUniversities(ctx context.Context, filter *UniversityFilter) ([]UniversityResponse, error) {
	return s.universities, s.err
}

func (s stubUniversityService) GetUniversityProfile(ctx context.Context, universityID int64, profileYear *int) (*UniversityProfileResponse, error) {
	return s.profile, s.err
}

func TestListUniversitiesReturnsUniversityIdentities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUniversityHandler(stubUniversityService{universities: []UniversityResponse{
		{ID: 1, UniversityCode: "1003", Name: "清华大学"},
	}})

	router := gin.New()
	router.GET("/api/v1/admission/universities", handler.ListUniversities)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/universities?q=清华", http.NoBody)
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

func TestGetUniversityProfileReturnsProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUniversityHandler(stubUniversityService{profile: &UniversityProfileResponse{
		UniversityID: 1,
		ProfileYear:  2025,
		RegionCode:   "110000",
		City:         "北京市",
	}})

	router := gin.New()
	router.GET("/api/v1/admission/universities/:id/profile", handler.GetUniversityProfile)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/universities/1/profile?profile_year=2025", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope web.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	data, ok := envelope.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(2025), data["profile_year"])
}
