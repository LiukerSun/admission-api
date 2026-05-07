package admission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubMajorCatalogStore struct {
	latestYear int
	majors     []StandardMajorResponse
	err        error
}

func (s stubMajorCatalogStore) LatestCatalogYear(ctx context.Context) (int, error) {
	return s.latestYear, s.err
}

func (s stubMajorCatalogStore) ListStandardMajors(ctx context.Context, filter StandardMajorFilter) ([]StandardMajorResponse, error) {
	return s.majors, s.err
}

func TestMajorCatalogServiceReturnsLatestCatalogYear(t *testing.T) {
	service := NewMajorCatalogService(stubMajorCatalogStore{latestYear: 2025})

	year, err := service.LatestCatalogYear(context.Background())

	require.NoError(t, err)
	require.Equal(t, 2025, year)
}

func TestMajorCatalogServiceListsStandardMajors(t *testing.T) {
	service := NewMajorCatalogService(stubMajorCatalogStore{majors: []StandardMajorResponse{
		{CatalogYear: 2025, MajorCode: "080901", Name: "计算机科学与技术"},
	}})

	majors, err := service.ListStandardMajors(context.Background(), StandardMajorFilter{Query: "计算机"})

	require.NoError(t, err)
	require.Len(t, majors, 1)
	require.Equal(t, "080901", majors[0].MajorCode)
}
