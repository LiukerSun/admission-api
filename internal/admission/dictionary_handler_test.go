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

type stubDictionaryService struct {
	resp *DictionaryResponse
	err  error
}

func (s stubDictionaryService) ListDictionaries(ctx context.Context) (*DictionaryResponse, error) {
	return s.resp, s.err
}

func TestListDictionariesReturnsGroupedCodeNameValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewDictionaryHandler(stubDictionaryService{resp: &DictionaryResponse{
		Regions: []DictionaryItem{
			{Code: "230000", Name: "黑龙江省"},
		},
		SubjectCategories: []DictionaryItem{
			{Code: "physics", Name: "物理"},
		},
	}})

	router := gin.New()
	router.GET("/api/v1/admission/dictionaries", handler.ListDictionaries)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admission/dictionaries", http.NoBody)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope web.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)

	data, ok := envelope.Data.(map[string]any)
	require.True(t, ok)
	require.Len(t, data["regions"], 1)
	require.Len(t, data["subject_categories"], 1)
}
