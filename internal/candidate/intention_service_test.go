package candidate

import (
	"context"
	"testing"

	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockIntentionStore

type mockIntentionStore struct{ mock.Mock }

func (m *mockIntentionStore) ListByProfile(ctx context.Context, profileID int64, intentionType string) ([]*Intention, error) {
	args := m.Called(ctx, profileID, intentionType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Intention), args.Error(1)
}

func (m *mockIntentionStore) GetByID(ctx context.Context, id int64) (*Intention, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Intention), args.Error(1)
}

func (m *mockIntentionStore) ReplaceByType(ctx context.Context, profileID int64, intentionType string, items []*CreateIntentionInput) error {
	return m.Called(ctx, profileID, intentionType, items).Error(0)
}

func (m *mockIntentionStore) DeleteByID(ctx context.Context, profileID int64, id int64) error {
	return m.Called(ctx, profileID, id).Error(0)
}

func (m *mockIntentionStore) DeleteByType(ctx context.Context, profileID int64, intentionType string) error {
	return m.Called(ctx, profileID, intentionType).Error(0)
}

func newIntentionTestService(t *testing.T) (*intentionService, *mockIntentionStore, *mockProfileStore, *mockBindingStore, *mockActivityLogService) {
	t.Helper()
	store := &mockIntentionStore{}
	profileStore := &mockProfileStore{}
	bindStore := &mockBindingStore{}
	logSvc := &mockActivityLogService{}
	logSvc.On("LogActivity", mock.Anything, mock.Anything).Return(nil).Maybe()

	return &intentionService{
		store:        store,
		profileStore: profileStore,
		bindingStore: bindStore,
		activityLog:  logSvc,
	}, store, profileStore, bindStore, logSvc
}

// --- GetIntentions ---

func TestIntentionService_GetIntentions_Success(t *testing.T) {
	svc, store, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("ListByProfile", ctx, int64(1), "").Return([]*Intention{
		{ID: 1, ProfileID: 1, IntentionType: "school", TargetID: "100", Priority: 0},
		{ID: 2, ProfileID: 1, IntentionType: "province", TargetID: "11", Priority: 1},
		{ID: 3, ProfileID: 1, IntentionType: "major", TargetID: "0809", Priority: 2},
	}, nil)

	resp, err := svc.GetIntentions(ctx, 10, 1)
	require.NoError(t, err)
	require.Len(t, resp.School, 1)
	require.Len(t, resp.Province, 1)
	require.Len(t, resp.Major, 1)
	assert.Equal(t, "100", resp.School[0].TargetID)
	assert.Equal(t, "11", resp.Province[0].TargetID)
}

func TestIntentionService_GetIntentions_ProfileNotFound(t *testing.T) {
	svc, _, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(0), pgx.ErrNoRows)

	_, err := svc.GetIntentions(ctx, 10, 1)
	require.Error(t, err)
	appErr, ok := err.(*web.AppError)
	require.True(t, ok)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestIntentionService_GetIntentions_Forbidden(t *testing.T) {
	svc, _, profileStore, bindStore, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	bindStore.On("GetBindingByStudent", ctx, int64(99)).Return(nil, user.ErrBindingNotFound)
	bindStore.On("GetBindingByStudent", ctx, int64(10)).Return(nil, user.ErrBindingNotFound)

	_, err := svc.GetIntentions(ctx, 99, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

func TestIntentionService_GetIntentions_BoundParentCanAccess(t *testing.T) {
	svc, store, profileStore, bindStore, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(20), nil)
	bindStore.On("GetBindingByStudent", ctx, int64(10)).Return(nil, user.ErrBindingNotFound)
	bindStore.On("GetBindingByStudent", ctx, int64(20)).Return(&user.Binding{ID: 1, ParentID: 10, StudentID: 20}, nil)
	store.On("ListByProfile", ctx, int64(1), "").Return([]*Intention{}, nil)

	resp, err := svc.GetIntentions(ctx, 10, 1)
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// --- SaveIntentions ---

func TestIntentionService_SaveIntentions_Success(t *testing.T) {
	svc, store, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("ReplaceByType", ctx, int64(1), "school", mock.Anything).Return(nil)

	err := svc.SaveIntentions(ctx, 10, 1, "school", &SaveIntentionsRequest{
		Items: []IntentionItemInput{
			{TargetID: "100", TargetName: "清华", Priority: 0},
			{TargetID: "101", TargetName: "北大", Priority: 1},
		},
	})
	require.NoError(t, err)
	store.AssertExpectations(t)
}

func TestIntentionService_SaveIntentions_EmptyItems(t *testing.T) {
	svc, store, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("ReplaceByType", ctx, int64(1), "school", []*CreateIntentionInput{}).Return(nil)

	err := svc.SaveIntentions(ctx, 10, 1, "school", &SaveIntentionsRequest{Items: []IntentionItemInput{}})
	require.NoError(t, err)
}

func TestIntentionService_SaveIntentions_InvalidType(t *testing.T) {
	svc, _, _, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	err := svc.SaveIntentions(ctx, 10, 1, "invalid", &SaveIntentionsRequest{
		Items: []IntentionItemInput{{TargetID: "100", TargetName: "Test"}},
	})
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestIntentionService_SaveIntentions_Forbidden(t *testing.T) {
	svc, _, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)

	err := svc.SaveIntentions(ctx, 99, 1, "school", &SaveIntentionsRequest{
		Items: []IntentionItemInput{{TargetID: "100", TargetName: "Test"}},
	})
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

// --- RemoveIntention ---

func TestIntentionService_RemoveIntention_Success(t *testing.T) {
	svc, store, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&Intention{
		ID: 1, ProfileID: 1, IntentionType: "school", TargetID: "100",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("DeleteByID", ctx, int64(1), int64(1)).Return(nil)

	err := svc.RemoveIntention(ctx, 10, 1)
	require.NoError(t, err)
	store.AssertExpectations(t)
}

func TestIntentionService_RemoveIntention_NotFound(t *testing.T) {
	svc, store, _, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(nil, pgx.ErrNoRows)

	err := svc.RemoveIntention(ctx, 10, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestIntentionService_RemoveIntention_Forbidden(t *testing.T) {
	svc, store, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&Intention{
		ID: 1, ProfileID: 1, IntentionType: "school", TargetID: "100",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)

	err := svc.RemoveIntention(ctx, 99, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

// --- ClearIntentions ---

func TestIntentionService_ClearIntentions_Success(t *testing.T) {
	svc, store, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("DeleteByType", ctx, int64(1), "school").Return(nil)

	err := svc.ClearIntentions(ctx, 10, 1, "school")
	require.NoError(t, err)
	store.AssertExpectations(t)
}

func TestIntentionService_ClearIntentions_InvalidType(t *testing.T) {
	svc, _, _, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	err := svc.ClearIntentions(ctx, 10, 1, "invalid")
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestIntentionService_ClearIntentions_Forbidden(t *testing.T) {
	svc, _, profileStore, _, _ := newIntentionTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)

	err := svc.ClearIntentions(ctx, 99, 1, "school")
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}
