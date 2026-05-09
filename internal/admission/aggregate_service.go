package admission

import "context"

type AggregateService interface {
	Aggregate(ctx context.Context, filter *AggregateFilter) (*AggregateResponse, error)
}

type aggregateService struct {
	store AggregateStore
}

func NewAggregateService(store AggregateStore) AggregateService {
	return &aggregateService{store: store}
}

func (s *aggregateService) Aggregate(ctx context.Context, filter *AggregateFilter) (*AggregateResponse, error) {
	return s.store.Aggregate(ctx, filter)
}
