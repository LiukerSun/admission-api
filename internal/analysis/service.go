package analysis

import (
	"context"
)

// Service 数据分析服务接口
type Service interface {
	GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) (*EnrollmentPlanResponse, error)
}

// service 数据分析服务实现
type service struct {
	store Store
}

// NewService 创建新的服务实例
func NewService(store Store) Service {
	return &service{
		store: store,
	}
}

// GetEnrollmentPlans 获取招生计划数据
func (s *service) GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) (*EnrollmentPlanResponse, error) {
	// 调用store获取数据
	plans, total, err := s.store.GetEnrollmentPlans(ctx, query)
	if err != nil {
		return nil, err
	}

	// 构建响应
	response := &EnrollmentPlanResponse{
		Total:   total,
		Plans:   plans,
		Page:    query.Page,
		PerPage: query.PerPage,
	}

	// 确保分页参数有效
	if response.Page <= 0 {
		response.Page = 1
	}

	if response.PerPage <= 0 {
		response.PerPage = 10
	}

	return response, nil
}
