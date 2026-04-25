package planner

import (
	"context"
	"testing"

	"admission-api/internal/platform/web"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockMerchantStore struct {
	mock.Mock
}

func (m *mockMerchantStore) CreateMerchant(ctx context.Context, input *CreateMerchantInput) (*PlannerMerchant, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerMerchant), args.Error(1)
}

func (m *mockMerchantStore) GetMerchant(ctx context.Context, id int64) (*PlannerMerchant, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerMerchant), args.Error(1)
}

func (m *mockMerchantStore) ListMerchants(ctx context.Context, status, merchantName, serviceRegion string, page, pageSize int) ([]*PlannerMerchant, int64, error) {
	args := m.Called(ctx, status, merchantName, serviceRegion, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*PlannerMerchant), args.Get(1).(int64), args.Error(2)
}

func (m *mockMerchantStore) UpdateMerchant(ctx context.Context, id int64, input *UpdateMerchantInput) (*PlannerMerchant, error) {
	args := m.Called(ctx, id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerMerchant), args.Error(1)
}

func (m *mockMerchantStore) UserExists(ctx context.Context, userID int64) (bool, error) {
	args := m.Called(ctx, userID)
	return args.Bool(0), args.Error(1)
}

func TestMerchantService_CreateMerchant(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("UserExists", mock.Anything, int64(5)).Return(true, nil)
	store.On("CreateMerchant", mock.Anything, mock.Anything).Return(
		&PlannerMerchant{ID: 1, MerchantName: "Test Org", Status: "active"},
		nil,
	)

	m, err := svc.CreateMerchant(context.Background(), CreateMerchantRequest{
		MerchantName: "Test Org",
		OwnerID:      ptrInt64(5),
	})
	assert.NoError(t, err)
	assert.Equal(t, "Test Org", m.MerchantName)
	assert.Equal(t, "active", m.Status)
}

func TestMerchantService_CreateMerchant_OwnerNotFound(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("UserExists", mock.Anything, int64(99)).Return(false, nil)

	_, err := svc.CreateMerchant(context.Background(), CreateMerchantRequest{
		MerchantName: "Test Org",
		OwnerID:      ptrInt64(99),
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestMerchantService_CreateMerchant_NameConflict(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("CreateMerchant", mock.Anything, mock.Anything).Return(nil, &pgconn.PgError{Code: "23505"})

	_, err := svc.CreateMerchant(context.Background(), CreateMerchantRequest{
		MerchantName: "Duplicate",
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeConflict, appErr.Code)
}

func TestMerchantService_GetMerchant(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("GetMerchant", mock.Anything, int64(7)).Return(
		&PlannerMerchant{ID: 7, MerchantName: "Org A", Status: "active"},
		nil,
	)

	m, err := svc.GetMerchant(context.Background(), 7)
	assert.NoError(t, err)
	assert.Equal(t, "Org A", m.MerchantName)
}

func TestMerchantService_GetMerchant_NotFound(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("GetMerchant", mock.Anything, int64(1)).Return(nil, pgx.ErrNoRows)

	_, err := svc.GetMerchant(context.Background(), 1)
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestMerchantService_ListMerchants(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("ListMerchants", mock.Anything, "active", "", "", 1, 20).Return([]*PlannerMerchant{
		{ID: 1, MerchantName: "Org A", Status: "active"},
	}, int64(1), nil)

	result, err := svc.ListMerchants(context.Background(), "active", "", "", 1, 20)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), result.Total)
	assert.Len(t, result.Merchants, 1)
}

func TestMerchantService_UpdateMerchant(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("GetMerchant", mock.Anything, int64(7)).Return(
		&PlannerMerchant{ID: 7, MerchantName: "Org A", Status: "active"},
		nil,
	)
	store.On("UpdateMerchant", mock.Anything, int64(7), mock.Anything).Return(
		&PlannerMerchant{ID: 7, MerchantName: "Updated Org", Status: "active"},
		nil,
	)

	m, err := svc.UpdateMerchant(context.Background(), 7, UpdateMerchantRequest{
		MerchantName: strPtr("Updated Org"),
	})
	assert.NoError(t, err)
	assert.Equal(t, "Updated Org", m.MerchantName)
}

func TestMerchantService_UpdateMerchant_NotFound(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("GetMerchant", mock.Anything, int64(1)).Return(nil, pgx.ErrNoRows)

	_, err := svc.UpdateMerchant(context.Background(), 1, UpdateMerchantRequest{
		MerchantName: strPtr("Updated"),
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestMerchantService_UpdateMerchant_OwnerNotFound(t *testing.T) {
	store := new(mockMerchantStore)
	svc := NewMerchantService(store)

	store.On("GetMerchant", mock.Anything, int64(7)).Return(
		&PlannerMerchant{ID: 7, MerchantName: "Org A", Status: "active"},
		nil,
	)
	store.On("UserExists", mock.Anything, int64(99)).Return(false, nil)

	_, err := svc.UpdateMerchant(context.Background(), 7, UpdateMerchantRequest{
		OwnerID: ptrInt64(99),
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func ptrInt64(v int64) *int64 {
	return &v
}
