package admission

import "context"

type UniversityService interface {
	ListUniversities(ctx context.Context, filter *UniversityFilter) ([]UniversityResponse, error)
	GetUniversityProfile(ctx context.Context, universityID int64, profileYear *int) (*UniversityProfileResponse, error)
}

type universityService struct {
	store UniversityStore
}

func NewUniversityService(store UniversityStore) UniversityService {
	return &universityService{store: store}
}

func (s *universityService) ListUniversities(ctx context.Context, filter *UniversityFilter) ([]UniversityResponse, error) {
	return s.store.ListUniversities(ctx, filter)
}

func (s *universityService) GetUniversityProfile(ctx context.Context, universityID int64, profileYear *int) (*UniversityProfileResponse, error) {
	return s.store.GetUniversityProfile(ctx, universityID, profileYear)
}
