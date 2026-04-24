package membership

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) ListActivePlans(ctx context.Context) ([]*Plan, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*Plan), args.Error(1)
}

func (m *mockStore) GetActivePlanByCode(ctx context.Context, planCode string) (*Plan, error) {
	args := m.Called(ctx, planCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Plan), args.Error(1)
}

func (m *mockStore) GetCurrentMembership(ctx context.Context, userID int64) (*UserMembership, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserMembership), args.Error(1)
}

func (m *mockStore) HasActiveMembership(ctx context.Context, userID int64, now time.Time) (bool, error) {
	args := m.Called(ctx, userID, now)
	return args.Bool(0), args.Error(1)
}

func (m *mockStore) GrantMembership(ctx context.Context, req GrantRequest) (*UserMembership, *Grant, bool, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, nil, args.Bool(2), args.Error(3)
	}
	return args.Get(0).(*UserMembership), args.Get(1).(*Grant), args.Bool(2), args.Error(3)
}

func TestListPlansReturnsActivePlans(t *testing.T) {
	store := new(mockStore)
	svc := NewService(store)
	store.On("ListActivePlans", mock.Anything).Return([]*Plan{
		{ID: 1, PlanCode: "monthly", PlanName: "月度会员", MembershipLevel: LevelPremium, DurationDays: 30, PriceAmount: 990, Currency: "CNY", Status: PlanStatusActive},
	}, nil)

	resp, err := svc.ListPlans(context.Background())

	require.NoError(t, err)
	require.Len(t, resp, 1)
	assert.Equal(t, "monthly", resp[0].PlanCode)
	assert.Equal(t, 30, resp[0].DurationDays)
}

func TestGetPurchasablePlanRejectsMissingPlanCode(t *testing.T) {
	svc := NewService(new(mockStore))

	plan, err := svc.GetPurchasablePlan(context.Background(), "")

	require.ErrorIs(t, err, ErrPlanNotFound)
	assert.Nil(t, plan)
}

func TestGrantFromPaidOrderRequiresIdempotencyKey(t *testing.T) {
	svc := NewService(new(mockStore))

	membership, grant, created, err := svc.GrantFromPaidOrder(context.Background(), GrantRequest{
		UserID:         1,
		PaymentOrderID: 2,
		DurationDays:   30,
	})

	require.Error(t, err)
	assert.Nil(t, membership)
	assert.Nil(t, grant)
	assert.False(t, created)
}

func TestGetCurrentReturnsNoneLevelWhenMembershipMissing(t *testing.T) {
	store := new(mockStore)
	svc := NewService(store)

	store.On("GetCurrentMembership", mock.Anything, int64(7)).Return(&UserMembership{
		UserID:          7,
		MembershipLevel: LevelNone,
		Status:          MembershipStatusInactive,
	}, nil)

	resp, err := svc.GetCurrent(context.Background(), 7)

	require.NoError(t, err)
	assert.Equal(t, LevelNone, resp.MembershipLevel)
	assert.Equal(t, MembershipStatusInactive, resp.Status)
	assert.False(t, resp.Active)
}

func TestGetCurrentMarksExpiredMembershipInactiveForAccess(t *testing.T) {
	store := new(mockStore)
	svc := NewService(store)
	expiredAt := time.Now().Add(-time.Hour)
	startedAt := time.Now().Add(-30 * 24 * time.Hour)

	store.On("GetCurrentMembership", mock.Anything, int64(7)).Return(&UserMembership{
		UserID:          7,
		MembershipLevel: LevelPremium,
		Status:          MembershipStatusExpired,
		StartedAt:       &startedAt,
		EndsAt:          &expiredAt,
	}, nil)

	resp, err := svc.GetCurrent(context.Background(), 7)

	require.NoError(t, err)
	assert.Equal(t, LevelPremium, resp.MembershipLevel)
	assert.Equal(t, MembershipStatusExpired, resp.Status)
	assert.False(t, resp.Active)
}

func TestHasActiveMembershipDelegatesToStore(t *testing.T) {
	store := new(mockStore)
	svc := NewService(store)

	store.On("HasActiveMembership", mock.Anything, int64(7), mock.AnythingOfType("time.Time")).Return(true, nil)

	active, err := svc.HasActiveMembership(context.Background(), 7)

	require.NoError(t, err)
	assert.True(t, active)
}

func TestGrantFromPaidOrderCreatesActiveMembership(t *testing.T) {
	store := new(mockStore)
	svc := NewService(store)
	now := time.Now().UTC()
	endsAt := now.Add(30 * 24 * time.Hour)

	store.On("GrantMembership", mock.Anything, mock.MatchedBy(func(req GrantRequest) bool {
		return req.UserID == 7 && req.PaymentOrderID == 100 && req.DurationDays == 30 && req.IdempotencyKey == "payment-order:100"
	})).Return(&UserMembership{
		UserID:          7,
		MembershipLevel: LevelPremium,
		Status:          MembershipStatusActive,
		StartedAt:       &now,
		EndsAt:          &endsAt,
	}, &Grant{
		UserID:         7,
		PaymentOrderID: 100,
		Action:         GrantActionActivate,
		StartsAt:       now,
		EndsAt:         endsAt,
		IdempotencyKey: "payment-order:100",
	}, true, nil)

	membership, grant, created, err := svc.GrantFromPaidOrder(context.Background(), GrantRequest{
		UserID:         7,
		PaymentOrderID: 100,
		DurationDays:   30,
		IdempotencyKey: "payment-order:100",
		Now:            now,
	})

	require.NoError(t, err)
	require.NotNil(t, membership)
	require.NotNil(t, grant)
	assert.True(t, created)
	assert.Equal(t, MembershipStatusActive, membership.Status)
	assert.Equal(t, LevelPremium, membership.MembershipLevel)
	assert.Equal(t, GrantActionActivate, grant.Action)
}
