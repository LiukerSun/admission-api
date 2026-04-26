package planner

import (
	"context"
	"testing"
	"time"

	"admission-api/internal/platform/web"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockProfileStore struct {
	mock.Mock
}

func (m *mockProfileStore) CreateUserAndProfile(ctx context.Context, email, passwordHash, role, userType string, input *CreateProfileInput) (*PlannerProfile, error) {
	args := m.Called(ctx, email, passwordHash, role, userType, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerProfile), args.Error(1)
}

func (m *mockProfileStore) GetProfileByUserID(ctx context.Context, userID int64) (*PlannerProfile, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerProfile), args.Error(1)
}

func (m *mockProfileStore) GetProfile(ctx context.Context, id int64) (*PlannerProfile, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerProfile), args.Error(1)
}

func (m *mockProfileStore) UpdateProfile(ctx context.Context, userID int64, input *UpdateProfileInput) (*PlannerProfile, error) {
	args := m.Called(ctx, userID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerProfile), args.Error(1)
}

func (m *mockProfileStore) ListProfiles(ctx context.Context, filter ProfileFilter, page, pageSize int, sortField, sortOrder string) ([]*PlannerProfile, int64, error) {
	args := m.Called(ctx, filter, page, pageSize, sortField, sortOrder)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*PlannerProfile), args.Get(1).(int64), args.Error(2)
}

func (m *mockProfileStore) UserExists(ctx context.Context, userID int64) (bool, error) {
	args := m.Called(ctx, userID)
	return args.Bool(0), args.Error(1)
}

func (m *mockProfileStore) EmailExists(ctx context.Context, email string) (bool, error) {
	args := m.Called(ctx, email)
	return args.Bool(0), args.Error(1)
}

func TestProfileService_CreateProfile(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	merchantStore.On("GetMerchant", mock.Anything, int64(5)).Return(
		&PlannerMerchant{ID: 5, MerchantName: "Test Org", ServiceRegions: []string{"11", "12"}},
		nil,
	)
	profileStore.On("EmailExists", mock.Anything, "test@example.com").Return(false, nil)
	profileStore.On("CreateUserAndProfile", mock.Anything, "test@example.com", mock.AnythingOfType("string"), "planner", "student", mock.Anything).Return(
		&PlannerProfile{ID: 1, UserID: 10, RealName: "Test User", Level: "junior", Status: "active", MerchantID: ptrInt64(5), MerchantName: strPtr("Test Org")},
		nil,
	)

	p, err := svc.CreateProfile(context.Background(), CreateProfileRequest{
		Email:      "test@example.com",
		Password:   "password123",
		RealName:   "Test User",
		MerchantID: ptrInt64(5),
	})
	assert.NoError(t, err)
	assert.Equal(t, "Test User", p.RealName)
	assert.Equal(t, "junior", p.Level)
	assert.Equal(t, int64(5), *p.MerchantID)
}

func TestProfileService_CreateProfile_ServiceRegionInheritance(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	merchantStore.On("GetMerchant", mock.Anything, int64(5)).Return(
		&PlannerMerchant{ID: 5, MerchantName: "Test Org", ServiceRegions: []string{"11", "12"}},
		nil,
	)
	profileStore.On("EmailExists", mock.Anything, "test@example.com").Return(false, nil)
	profileStore.On("CreateUserAndProfile", mock.Anything, "test@example.com", mock.AnythingOfType("string"), "planner", "student", mock.Anything).Return(
		&PlannerProfile{ID: 1, UserID: 10, RealName: "Test User", Level: "junior", Status: "active", ServiceRegion: []string{"11", "12"}, MerchantID: ptrInt64(5)},
		nil,
	).Run(func(args mock.Arguments) {
		input := args.Get(5).(*CreateProfileInput)
		assert.Equal(t, []string{"11", "12"}, input.ServiceRegion)
	})

	p, err := svc.CreateProfile(context.Background(), CreateProfileRequest{
		Email:      "test@example.com",
		Password:   "password123",
		RealName:   "Test User",
		MerchantID: ptrInt64(5),
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"11", "12"}, p.ServiceRegion)
}

func TestProfileService_CreateProfile_EmailExists(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("EmailExists", mock.Anything, "test@example.com").Return(true, nil)

	_, err := svc.CreateProfile(context.Background(), CreateProfileRequest{
		Email:    "test@example.com",
		Password: "password123",
		RealName: "Test User",
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeConflict, appErr.Code)
}

func TestProfileService_CreateProfile_MerchantNotFound(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("EmailExists", mock.Anything, "test@example.com").Return(false, nil)
	merchantStore.On("GetMerchant", mock.Anything, int64(99)).Return(nil, pgx.ErrNoRows)

	_, err := svc.CreateProfile(context.Background(), CreateProfileRequest{
		Email:      "test@example.com",
		Password:   "password123",
		RealName:   "Test User",
		MerchantID: ptrInt64(99),
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestProfileService_CreateProfile_InvalidServiceRegion(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("EmailExists", mock.Anything, "test@example.com").Return(false, nil)
	merchantStore.On("GetMerchant", mock.Anything, int64(5)).Return(
		&PlannerMerchant{ID: 5, MerchantName: "Test Org", ServiceRegions: []string{"11", "12"}},
		nil,
	)

	_, err := svc.CreateProfile(context.Background(), CreateProfileRequest{
		Email:         "test@example.com",
		Password:      "password123",
		RealName:      "Test User",
		MerchantID:    ptrInt64(5),
		ServiceRegion: []string{"99"},
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestProfileService_CreateProfile_InvalidLevelExpireAt(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("EmailExists", mock.Anything, "test@example.com").Return(false, nil)

	past := time.Now().Add(-24 * time.Hour)
	_, err := svc.CreateProfile(context.Background(), CreateProfileRequest{
		Email:         "test@example.com",
		Password:      "password123",
		RealName:      "Test User",
		LevelExpireAt: &past,
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestProfileService_CreateProfile_Conflict(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("EmailExists", mock.Anything, "test@example.com").Return(false, nil)
	profileStore.On("CreateUserAndProfile", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		nil, &pgconn.PgError{Code: "23505"},
	)

	_, err := svc.CreateProfile(context.Background(), CreateProfileRequest{
		Email:    "test@example.com",
		Password: "password123",
		RealName: "Test User",
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeConflict, appErr.Code)
}

func TestProfileService_GetMyProfile(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("GetProfileByUserID", mock.Anything, int64(10)).Return(
		&PlannerProfile{ID: 1, UserID: 10, RealName: "Test User", Level: "senior", Status: "active"},
		nil,
	)

	p, err := svc.GetMyProfile(context.Background(), 10)
	assert.NoError(t, err)
	assert.Equal(t, "Test User", p.RealName)
	assert.Equal(t, "senior", p.Level)
}

func TestProfileService_GetMyProfile_NotFound(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("GetProfileByUserID", mock.Anything, int64(10)).Return(nil, pgx.ErrNoRows)

	_, err := svc.GetMyProfile(context.Background(), 10)
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestProfileService_UpdateMyProfile(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("GetProfileByUserID", mock.Anything, int64(10)).Return(
		&PlannerProfile{ID: 1, UserID: 10, RealName: "Test User", Level: "junior", Status: "active"},
		nil,
	)
	profileStore.On("UpdateProfile", mock.Anything, int64(10), mock.Anything).Return(
		&PlannerProfile{ID: 1, UserID: 10, RealName: "Updated User", Level: "senior", Status: "active"},
		nil,
	)

	p, err := svc.UpdateMyProfile(context.Background(), 10, UpdateMyProfileRequest{
		RealName: strPtr("Updated User"),
		Level:    strPtr("senior"),
	})
	assert.NoError(t, err)
	assert.Equal(t, "Updated User", p.RealName)
	assert.Equal(t, "senior", p.Level)
}

func TestProfileService_UpdateMyProfile_NotFound(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("GetProfileByUserID", mock.Anything, int64(10)).Return(nil, pgx.ErrNoRows)

	_, err := svc.UpdateMyProfile(context.Background(), 10, UpdateMyProfileRequest{
		RealName: strPtr("Updated User"),
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestProfileService_UpdateMyProfile_MerchantNotFound(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("GetProfileByUserID", mock.Anything, int64(10)).Return(
		&PlannerProfile{ID: 1, UserID: 10, RealName: "Test User", Level: "junior", Status: "active"},
		nil,
	)
	merchantStore.On("GetMerchant", mock.Anything, int64(99)).Return(nil, pgx.ErrNoRows)

	_, err := svc.UpdateMyProfile(context.Background(), 10, UpdateMyProfileRequest{
		MerchantID: ptrInt64(99),
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestProfileService_UpdateMyProfile_InvalidServiceRegion(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("GetProfileByUserID", mock.Anything, int64(10)).Return(
		&PlannerProfile{ID: 1, UserID: 10, RealName: "Test User", Level: "junior", Status: "active", MerchantID: ptrInt64(5)},
		nil,
	)
	merchantStore.On("GetMerchant", mock.Anything, int64(5)).Return(
		&PlannerMerchant{ID: 5, MerchantName: "Test Org", ServiceRegions: []string{"11", "12"}},
		nil,
	)

	_, err := svc.UpdateMyProfile(context.Background(), 10, UpdateMyProfileRequest{
		ServiceRegion: []string{"99"},
	})
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeBadRequest, appErr.Code)
}

func TestProfileService_GetProfile(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("GetProfile", mock.Anything, int64(1)).Return(
		&PlannerProfile{ID: 1, UserID: 10, RealName: "Test User", Level: "expert", Status: "active"},
		nil,
	)

	p, err := svc.GetProfile(context.Background(), 1)
	assert.NoError(t, err)
	assert.Equal(t, "Test User", p.RealName)
}

func TestProfileService_GetProfile_NotFound(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("GetProfile", mock.Anything, int64(1)).Return(nil, pgx.ErrNoRows)

	_, err := svc.GetProfile(context.Background(), 1)
	assert.Error(t, err)
	appErr, ok := err.(*web.AppError)
	assert.True(t, ok)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestProfileService_ListProfiles(t *testing.T) {
	profileStore := new(mockProfileStore)
	merchantStore := new(mockMerchantStore)
	svc := NewProfileService(profileStore, merchantStore)

	profileStore.On("ListProfiles", mock.Anything, ProfileFilter{Status: "active"}, 1, 20, "", "").Return(
		[]*PlannerProfile{
			{ID: 1, UserID: 10, RealName: "User A", Level: "junior", Status: "active"},
			{ID: 2, UserID: 11, RealName: "User B", Level: "senior", Status: "active"},
		},
		int64(2),
		nil,
	)

	result, err := svc.ListProfiles(context.Background(), ProfileFilter{Status: "active"}, 1, 20, "", "")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)
	assert.Len(t, result.Profiles, 2)
}
