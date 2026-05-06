package candidate

import (
	"context"
	"database/sql"
	"testing"

	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockExamRecordStore

type mockExamRecordStore struct{ mock.Mock }

func (m *mockExamRecordStore) ListByProfile(ctx context.Context, profileID int64) ([]*ExamRecord, error) {
	args := m.Called(ctx, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ExamRecord), args.Error(1)
}

func (m *mockExamRecordStore) GetByID(ctx context.Context, id int64) (*ExamRecord, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ExamRecord), args.Error(1)
}

func (m *mockExamRecordStore) Create(ctx context.Context, input *createExamRecordInput) (*ExamRecord, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ExamRecord), args.Error(1)
}

func (m *mockExamRecordStore) Update(ctx context.Context, id int64, input *updateExamRecordInput) (*ExamRecord, error) {
	args := m.Called(ctx, id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ExamRecord), args.Error(1)
}


func (m *mockExamRecordStore) SetOtherRecordsNotCurrent(ctx context.Context, profileID int64, excludeID int64) error {
	return m.Called(ctx, profileID, excludeID).Error(0)
}

func (m *mockExamRecordStore) Void(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

// mockScoreHistoryStore

type mockScoreHistoryStore struct{ mock.Mock }

func (m *mockScoreHistoryStore) ListByExamRecord(ctx context.Context, examRecordID int64) ([]*ScoreHistory, error) {
	args := m.Called(ctx, examRecordID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ScoreHistory), args.Error(1)
}

func (m *mockScoreHistoryStore) Create(ctx context.Context, input *createScoreHistoryInput) (*ScoreHistory, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ScoreHistory), args.Error(1)
}

func newExamRecordTestService(t *testing.T) (*examRecordService, *mockExamRecordStore, *mockScoreHistoryStore, *mockProfileStore, *mockBindingStore, *mockActivityLogService) {
	t.Helper()
	store := &mockExamRecordStore{}
	historyStore := &mockScoreHistoryStore{}
	profileStore := &mockProfileStore{}
	bindStore := &mockBindingStore{}
	logSvc := &mockActivityLogService{}
	logSvc.On("LogActivity", mock.Anything, mock.Anything).Return(nil).Maybe()

	return &examRecordService{
		store:        store,
		historyStore: historyStore,
		profileStore: profileStore,
		bindingStore: bindStore,
		activityLog:  logSvc,
	}, store, historyStore, profileStore, bindStore, logSvc
}

// --- ListByProfile ---

func TestExamRecordService_ListByProfile_Success(t *testing.T) {
	svc, store, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("ListByProfile", ctx, int64(1)).Return([]*ExamRecord{
		{ID: 1, ProfileID: 1, ExamYear: 2026, ExamModel: "3+1+2", IsCurrent: true, Status: "active"},
		{ID: 2, ProfileID: 1, ExamYear: 2025, ExamModel: "wenli", IsCurrent: false, Status: "active"},
	}, nil)

	resp, err := svc.ListByProfile(ctx, 10, 1)
	require.NoError(t, err)
	require.Len(t, resp, 2)
	assert.Equal(t, int16(2026), resp[0].ExamYear)
	assert.True(t, resp[0].CanWrite)
}

func TestExamRecordService_ListByProfile_BoundParentCanAccess(t *testing.T) {
	svc, store, _, profileStore, bindStore, _ := newExamRecordTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(20), nil)
	bindStore.On("GetBindingByStudent", ctx, int64(10)).Return(nil, user.ErrBindingNotFound)
	bindStore.On("GetBindingByStudent", ctx, int64(20)).Return(&user.Binding{ID: 1, ParentID: 10, StudentID: 20}, nil)
	store.On("ListByProfile", ctx, int64(1)).Return([]*ExamRecord{
		{ID: 1, ProfileID: 1, ExamYear: 2026, ExamModel: "3+1+2", IsCurrent: true, Status: "active"},
	}, nil)

	resp, err := svc.ListByProfile(ctx, 10, 1)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	assert.False(t, resp[0].CanWrite)
}

func TestExamRecordService_ListByProfile_Forbidden(t *testing.T) {
	svc, _, _, profileStore, bindStore, _ := newExamRecordTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	bindStore.On("GetBindingByStudent", ctx, int64(99)).Return(nil, user.ErrBindingNotFound)
	bindStore.On("GetBindingByStudent", ctx, int64(10)).Return(nil, user.ErrBindingNotFound)

	_, err := svc.ListByProfile(ctx, 99, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

func TestExamRecordService_ListByProfile_ProfileNotFound(t *testing.T) {
	svc, _, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(0), pgx.ErrNoRows)

	_, err := svc.ListByProfile(ctx, 10, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

// --- GetByID ---

func TestExamRecordService_GetByID_Success(t *testing.T) {
	svc, store, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026, ExamModel: "3+1+2", IsCurrent: true, Status: "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)

	resp, err := svc.GetByID(ctx, 10, 1)
	require.NoError(t, err)
	assert.Equal(t, int16(2026), resp.ExamYear)
	assert.True(t, resp.CanWrite)
}

func TestExamRecordService_GetByID_NotFound(t *testing.T) {
	svc, store, _, _, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(nil, pgx.ErrNoRows)

	_, err := svc.GetByID(ctx, 10, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

// --- Create ---

func TestExamRecordService_Create_Success(t *testing.T) {
	svc, store, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("Create", ctx, mock.Anything).Return(&ExamRecord{
		ID: 3, ProfileID: 1, ExamYear: 2026, ExamModel: "3+1+2", IsCurrent: true, Status: "active",
		TotalScore: float64Ptr(650),
	}, nil)

	resp, err := svc.Create(ctx, 10, 1, CreateExamRecordRequest{
		ExamYear: 2026, ExamModel: "3+1+2", TotalScore: 650,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), resp.ID)
	assert.True(t, resp.IsCurrent)
}

func TestExamRecordService_Create_InvalidExamModel(t *testing.T) {
	svc, _, _, _, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 10, 1, CreateExamRecordRequest{
		ExamYear: 2026, ExamModel: "invalid",
	})
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestExamRecordService_Create_Forbidden(t *testing.T) {
	svc, _, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)

	_, err := svc.Create(ctx, 99, 1, CreateExamRecordRequest{
		ExamYear: 2026, ExamModel: "3+1+2",
	})
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

// --- Update ---

func TestExamRecordService_Update_BasicFieldsOnly(t *testing.T) {
	svc, store, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026, ExamModel: "3+1+2", Status: "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("Update", ctx, int64(1), mock.Anything).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2027, ExamModel: "3+3", Status: "active",
	}, nil)

	resp, err := svc.Update(ctx, 10, 1, UpdateExamRecordRequest{ExamYear: 2027, ExamModel: "3+3"})
	require.NoError(t, err)
	assert.Equal(t, int16(2027), resp.ExamYear)
	assert.Equal(t, "3+3", resp.ExamModel)
}

func TestExamRecordService_Update_WithScoreChangesRecordsHistory(t *testing.T) {
	svc, store, historyStore, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026,
		TotalScore:    float64Ptr(650),
		RankValue:     int32Ptr(5000),
		SubjectScores: map[string]float64{"语文": 120},
		Status:        "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("Update", ctx, int64(1), mock.Anything).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026,
		TotalScore:    float64Ptr(660),
		RankValue:     int32Ptr(4800),
		SubjectScores: map[string]float64{"语文": 125},
		Status:        "active",
	}, nil)
	historyStore.On("Create", ctx, mock.Anything).Return(&ScoreHistory{ID: 1}, nil)

	totalScore := 660.0
	rankValue := int32(4800)
	resp, err := svc.Update(ctx, 10, 1, UpdateExamRecordRequest{
		TotalScore:    &totalScore,
		RankValue:     &rankValue,
		SubjectScores: map[string]float64{"语文": 125},
	})
	require.NoError(t, err)
	assert.Equal(t, 660.0, *resp.TotalScore)
	historyStore.AssertExpectations(t)
}

func TestExamRecordService_Update_NoScoreChangeSkipsHistory(t *testing.T) {
	svc, store, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026,
		TotalScore: float64Ptr(650),
		Status:     "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("Update", ctx, int64(1), mock.Anything).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026,
		TotalScore: float64Ptr(650),
		Status:     "active",
	}, nil)

	// historyStore should NOT be called
	totalScore := 650.0
	_, err := svc.Update(ctx, 10, 1, UpdateExamRecordRequest{
		TotalScore: &totalScore,
		ExamYear:   2027,
	})
	require.NoError(t, err)
}

func TestExamRecordService_Update_Forbidden(t *testing.T) {
	svc, store, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026, Status: "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)

	_, err := svc.Update(ctx, 99, 1, UpdateExamRecordRequest{ExamYear: 2027})
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

// --- Void ---

func TestExamRecordService_Void_Success(t *testing.T) {
	svc, store, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026, Status: "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("Void", ctx, int64(1)).Return(nil)

	err := svc.Void(ctx, 10, 1)
	require.NoError(t, err)
	store.AssertExpectations(t)
}

func TestExamRecordService_Void_Forbidden(t *testing.T) {
	svc, store, _, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026, Status: "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)

	err := svc.Void(ctx, 99, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

// --- ListScoreHistories ---

func TestExamRecordService_ListScoreHistories_Success(t *testing.T) {
	svc, store, historyStore, profileStore, _, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026, Status: "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	historyStore.On("ListByExamRecord", ctx, int64(1)).Return([]*ScoreHistory{
		{ID: 1, ExamRecordID: 1, NewTotalScore: sql.NullFloat64{Valid: true, Float64: 660}, NewRankValue: sql.NullInt32{Valid: true, Int32: 4800}},
	}, nil)

	resp, err := svc.ListScoreHistories(ctx, 10, 1)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	assert.Equal(t, 660.0, *resp[0].NewTotalScore)
}

func TestExamRecordService_ListScoreHistories_Forbidden(t *testing.T) {
	svc, store, _, profileStore, bindStore, _ := newExamRecordTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&ExamRecord{
		ID: 1, ProfileID: 1, ExamYear: 2026, Status: "active",
	}, nil)
	profileStore.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	bindStore.On("GetBindingByStudent", ctx, int64(99)).Return(nil, user.ErrBindingNotFound)
	bindStore.On("GetBindingByStudent", ctx, int64(10)).Return(nil, user.ErrBindingNotFound)

	_, err := svc.ListScoreHistories(ctx, 99, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

// --- helpers ---

func float64Ptr(v float64) sql.NullFloat64 {
	return sql.NullFloat64{Valid: true, Float64: v}
}

func int32Ptr(v int32) sql.NullInt32 {
	return sql.NullInt32{Valid: true, Int32: v}
}
