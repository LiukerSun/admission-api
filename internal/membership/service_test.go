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
	args := m.Called(ctx, userID, mock.AnythingOfType("time.Time"))
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
