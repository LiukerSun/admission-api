package analysis

import "context"

type AnalysisService interface {
	GetTrend(ctx context.Context, filter *TrendFilter) (*TrendResponse, error)
	GetGroupComparison(ctx context.Context, filter *GroupComparisonFilter) (*GroupComparisonResponse, error)
	GetMajorDistribution(ctx context.Context, filter *MajorDistributionFilter) (*MajorDistributionResponse, error)
	GetMajorComparison(ctx context.Context, filter *MajorComparisonFilter) (*MajorComparisonResponse, error)
}

type analysisService struct {
	store AnalysisStore
}

func NewService(store AnalysisStore) AnalysisService {
	return &analysisService{store: store}
}

func (s *analysisService) GetTrend(ctx context.Context, filter *TrendFilter) (*TrendResponse, error) {
	return s.store.GetTrend(ctx, filter)
}

func (s *analysisService) GetGroupComparison(ctx context.Context, filter *GroupComparisonFilter) (*GroupComparisonResponse, error) {
	return s.store.GetGroupComparison(ctx, filter)
}

func (s *analysisService) GetMajorDistribution(ctx context.Context, filter *MajorDistributionFilter) (*MajorDistributionResponse, error) {
	return s.store.GetMajorDistribution(ctx, filter)
}

func (s *analysisService) GetMajorComparison(ctx context.Context, filter *MajorComparisonFilter) (*MajorComparisonResponse, error) {
	return s.store.GetMajorComparison(ctx, filter)
}
