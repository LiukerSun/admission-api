package admission

import "context"

type MajorCatalogService interface {
	LatestCatalogYear(ctx context.Context) (int, error)
	ListStandardMajors(ctx context.Context, filter StandardMajorFilter) ([]StandardMajorResponse, error)
}

type majorCatalogService struct {
	store MajorCatalogStore
}

func NewMajorCatalogService(store MajorCatalogStore) MajorCatalogService {
	return &majorCatalogService{store: store}
}

func (s *majorCatalogService) LatestCatalogYear(ctx context.Context) (int, error) {
	return s.store.LatestCatalogYear(ctx)
}

func (s *majorCatalogService) ListStandardMajors(ctx context.Context, filter StandardMajorFilter) ([]StandardMajorResponse, error) {
	return s.store.ListStandardMajors(ctx, filter)
}
