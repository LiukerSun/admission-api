package analysis

import (
	"context"
)

type Service interface {
	GetDatasetOverview(ctx context.Context, query *DatasetOverviewQuery) (*DatasetOverviewResponse, error)
	GetFacets(ctx context.Context, query *FacetsQuery) (*FacetsResponse, error)
	ListSchools(ctx context.Context, query *SchoolListQuery) (*ListResponse[School], error)
	GetSchool(ctx context.Context, schoolID int64, query *SchoolDetailQuery) (*School, error)
	CompareSchools(ctx context.Context, query *SchoolCompareQuery) (map[string]any, error)
	ListMajors(ctx context.Context, query *MajorListQuery) (*ListResponse[Major], error)
	GetMajor(ctx context.Context, majorID int64, query *MajorDetailQuery) (*Major, error)
	ListSchoolMajors(ctx context.Context, schoolID int64, query *SchoolMajorsQuery) (*ListResponse[SchoolMajorItem], error)
	GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) (*EnrollmentPlanResponse, error)
	ListProvinceBatchLines(ctx context.Context, query *BatchLineQuery) (*ListResponse[ProvinceBatchLine], error)
	GetProvinceBatchLineTrend(ctx context.Context, query *BatchLineTrendQuery) (*BatchLineTrendResponse, error)
	ListSchoolAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[SchoolAdmissionScore], error)
	ListMajorAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[MajorAdmissionScore], error)
	GetAdmissionScoreTrend(ctx context.Context, query *ScoreTrendQuery) (*ScoreTrendResponse, error)
	GetScoreMatch(ctx context.Context, query *ScoreMatchQuery) (*ScoreMatchResponse, error)
	GetEmploymentData(ctx context.Context, query *EmploymentDataQuery) (*EmploymentDataResponse, error)
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

func (s *service) GetDatasetOverview(ctx context.Context, query *DatasetOverviewQuery) (*DatasetOverviewResponse, error) {
	return s.store.GetDatasetOverview(ctx, query)
}

func (s *service) GetFacets(ctx context.Context, query *FacetsQuery) (*FacetsResponse, error) {
	return s.store.GetFacets(ctx, query)
}

func (s *service) ListSchools(ctx context.Context, query *SchoolListQuery) (*ListResponse[School], error) {
	return s.store.ListSchools(ctx, query)
}

func (s *service) GetSchool(ctx context.Context, schoolID int64, query *SchoolDetailQuery) (*School, error) {
	return s.store.GetSchool(ctx, schoolID, query)
}

func (s *service) CompareSchools(ctx context.Context, query *SchoolCompareQuery) (map[string]any, error) {
	items, missing, err := s.store.CompareSchools(ctx, query)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"items":              items,
		"missing_school_ids": missing,
	}, nil
}

func (s *service) ListMajors(ctx context.Context, query *MajorListQuery) (*ListResponse[Major], error) {
	return s.store.ListMajors(ctx, query)
}

func (s *service) GetMajor(ctx context.Context, majorID int64, query *MajorDetailQuery) (*Major, error) {
	return s.store.GetMajor(ctx, majorID, query)
}

func (s *service) ListSchoolMajors(ctx context.Context, schoolID int64, query *SchoolMajorsQuery) (*ListResponse[SchoolMajorItem], error) {
	return s.store.ListSchoolMajors(ctx, schoolID, query)
}

func (s *service) GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) (*EnrollmentPlanResponse, error) {
	return s.store.GetEnrollmentPlans(ctx, query)
}

func (s *service) ListProvinceBatchLines(ctx context.Context, query *BatchLineQuery) (*ListResponse[ProvinceBatchLine], error) {
	return s.store.ListProvinceBatchLines(ctx, query)
}

func (s *service) GetProvinceBatchLineTrend(ctx context.Context, query *BatchLineTrendQuery) (*BatchLineTrendResponse, error) {
	return s.store.GetProvinceBatchLineTrend(ctx, query)
}

func (s *service) ListSchoolAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[SchoolAdmissionScore], error) {
	return s.store.ListSchoolAdmissionScores(ctx, query)
}

func (s *service) ListMajorAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[MajorAdmissionScore], error) {
	return s.store.ListMajorAdmissionScores(ctx, query)
}

func (s *service) GetAdmissionScoreTrend(ctx context.Context, query *ScoreTrendQuery) (*ScoreTrendResponse, error) {
	return s.store.GetAdmissionScoreTrend(ctx, query)
}

func (s *service) GetScoreMatch(ctx context.Context, query *ScoreMatchQuery) (*ScoreMatchResponse, error) {
	return s.store.GetScoreMatch(ctx, query)
}

func (s *service) GetEmploymentData(ctx context.Context, query *EmploymentDataQuery) (*EmploymentDataResponse, error) {
	return s.store.GetEmploymentData(ctx, query)
}
