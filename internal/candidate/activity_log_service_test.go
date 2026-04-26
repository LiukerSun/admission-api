package candidate

import (
	"context"
	"testing"
	"time"

	"admission-api/internal/platform/web"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockActivityLogStore struct {
	mock.Mock
}

func (m *mockActivityLogStore) Create(ctx context.Context, input *CreateActivityInput) (*ActivityLog, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ActivityLog), args.Error(1)
}

func (m *mockActivityLogStore) BatchCreate(ctx context.Context, inputs []*CreateActivityInput) error {
	args := m.Called(ctx, inputs)
	return args.Error(0)
}

func (m *mockActivityLogStore) List(ctx context.Context, filter ActivityFilter, page, pageSize int) ([]*ActivityLog, int64, error) {
	args := m.Called(ctx, filter, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*ActivityLog), args.Get(1).(int64), args.Error(2)
}

func (m *mockActivityLogStore) GetStats(ctx context.Context, targetType string, targetID int64) (int64, error) {
	args := m.Called(ctx, targetType, targetID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockActivityLogStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	args := m.Called(ctx, ids)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockActivityLogStore) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	args := m.Called(ctx, before)
	return args.Get(0).(int64), args.Error(1)
}

func setupMiniredis(t *testing.T) *redis.Client {
	s := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: s.Addr()})
}

func TestActivityLogService_LogActivity(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	input := CreateActivityInput{
		UserID:       1,
		ActivityType: "view_school",
		TargetType:   "school",
		TargetID:     123,
		Metadata:     map[string]any{"school_name": "Test School"},
	}

	err := svc.LogActivity(context.Background(), input)
	assert.NoError(t, err)
}

func TestActivityLogService_ListActivities(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	store.On("List", mock.Anything, ActivityFilter{ActivityType: "view_school"}, 1, 20).Return(
		[]*ActivityLog{
			{ID: 1, UserID: 1, ActivityType: "view_school", TargetType: strPtr("school"), TargetID: int64Ptr(123)},
		},
		int64(1),
		nil,
	)

	result, err := svc.ListActivities(context.Background(), ActivityFilter{ActivityType: "view_school"}, 1, 20)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), result.Total)
	assert.Len(t, result.Logs, 1)
}

func TestActivityLogService_GetMyActivities(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	store.On("List", mock.Anything, ActivityFilter{UserID: 1}, 1, 20).Return(
		[]*ActivityLog{
			{ID: 1, UserID: 1, ActivityType: "view_major"},
		},
		int64(1),
		nil,
	)

	result, err := svc.GetMyActivities(context.Background(), 1, 1, 20)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), result.Total)
}

func TestActivityLogService_GetStats(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	store.On("GetStats", mock.Anything, "school", int64(123)).Return(int64(42), nil)

	result, err := svc.GetStats(context.Background(), "school", 123)
	assert.NoError(t, err)
	assert.Equal(t, int64(42), result.Count)
	assert.Equal(t, "school", result.TargetType)
	assert.Equal(t, int64(123), result.TargetID)
}

func TestActivityLogService_GetStats_InvalidParams(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	_, err := svc.GetStats(context.Background(), "", 123)
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestActivityLogService_DeleteByIDs(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	store.On("DeleteByIDs", mock.Anything, []int64{1, 2, 3}).Return(int64(3), nil)

	deleted, err := svc.DeleteByIDs(context.Background(), []int64{1, 2, 3})
	assert.NoError(t, err)
	assert.Equal(t, int64(3), deleted)
}

func TestActivityLogService_DeleteByIDs_Empty(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	_, err := svc.DeleteByIDs(context.Background(), []int64{})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestActivityLogService_DeleteBefore(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	before := time.Now().Add(-24 * time.Hour)
	store.On("DeleteBefore", mock.Anything, before).Return(int64(100), nil)

	deleted, err := svc.DeleteBefore(context.Background(), before)
	assert.NoError(t, err)
	assert.Equal(t, int64(100), deleted)
}

func TestActivityLogService_DeleteBefore_ZeroTime(t *testing.T) {
	rdb := setupMiniredis(t)
	store := new(mockActivityLogStore)
	svc := NewActivityLogService(store, rdb)

	_, err := svc.DeleteBefore(context.Background(), time.Time{})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func strPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}
