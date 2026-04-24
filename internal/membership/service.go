package membership

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Service defines membership business operations.
type Service interface {
	ListPlans(ctx context.Context) ([]PlanResponse, error)
	GetPurchasablePlan(ctx context.Context, planCode string) (*Plan, error)
	GetCurrent(ctx context.Context, userID int64) (*CurrentMembershipResponse, error)
	HasActiveMembership(ctx context.Context, userID int64) (bool, error)
	GrantFromPaidOrder(ctx context.Context, req GrantRequest) (*UserMembership, *Grant, bool, error)
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

func (s *service) ListPlans(ctx context.Context) ([]PlanResponse, error) {
	plans, err := s.store.ListActivePlans(ctx)
	if err != nil {
		return nil, err
	}
	resp := make([]PlanResponse, 0, len(plans))
	for _, p := range plans {
		resp = append(resp, ToPlanResponse(p))
	}
	return resp, nil
}

func (s *service) GetPurchasablePlan(ctx context.Context, planCode string) (*Plan, error) {
	if planCode == "" {
		return nil, ErrPlanNotFound
	}
	p, err := s.store.GetActivePlanByCode(ctx, planCode)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) || errors.Is(err, ErrPlanNotPurchasable) {
			return nil, err
		}
		return nil, fmt.Errorf("get purchasable plan: %w", err)
	}
	return p, nil
}

func (s *service) GetCurrent(ctx context.Context, userID int64) (*CurrentMembershipResponse, error) {
	m, err := s.store.GetCurrentMembership(ctx, userID)
	if err != nil {
		return nil, err
	}
	active := m.EndsAt != nil && m.EndsAt.After(time.Now()) && m.Status == MembershipStatusActive
	return &CurrentMembershipResponse{
		MembershipLevel: m.MembershipLevel,
		Status:          m.Status,
		StartedAt:       m.StartedAt,
		EndsAt:          m.EndsAt,
		Active:          active,
	}, nil
}

func (s *service) HasActiveMembership(ctx context.Context, userID int64) (bool, error) {
	return s.store.HasActiveMembership(ctx, userID, time.Now())
}

func (s *service) GrantFromPaidOrder(ctx context.Context, req GrantRequest) (*UserMembership, *Grant, bool, error) {
	if req.UserID <= 0 || req.PaymentOrderID <= 0 || req.DurationDays <= 0 || req.IdempotencyKey == "" {
		return nil, nil, false, fmt.Errorf("invalid membership grant request")
	}
	if req.Now.IsZero() {
		req.Now = time.Now()
	}
	return s.store.GrantMembership(ctx, req)
}
