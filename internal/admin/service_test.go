package admin

import (
	"context"
	"testing"
	"time"

	"admission-api/internal/platform/redis"
	"admission-api/internal/user"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type mockAdminStore struct {
	mock.Mock
}

type mockUserStore struct {
	mock.Mock
}

func (m *mockAdminStore) ListUsers(ctx context.Context, filter ListUsersFilter, page, pageSize int) ([]*user.User, int64, error) {
	args := m.Called(ctx, filter, page, pageSize)
	return args.Get(0).([]*user.User), args.Get(1).(int64), args.Error(2)
}

func (m *mockAdminStore) ListBindings(ctx context.Context, page, pageSize int) ([]*BindingListItem, int64, error) {
	args := m.Called(ctx, page, pageSize)
	return args.Get(0).([]*BindingListItem), args.Get(1).(int64), args.Error(2)
}

func (m *mockAdminStore) GetStats(ctx context.Context) (*StatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*StatsResponse), args.Error(1)
}

func (m *mockUserStore) Create(ctx context.Context, email, passwordHash, role, userType string) (*user.User, error) {
	args := m.Called(ctx, email, passwordHash, role, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *mockUserStore) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *mockUserStore) GetByID(ctx context.Context, id int64) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *mockUserStore) GetByEmailAndType(ctx context.Context, email, userType string) (*user.User, error) {
	args := m.Called(ctx, email, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *mockUserStore) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *mockUserStore) GetByPhone(ctx context.Context, phone string) (*user.User, error) {
	args := m.Called(ctx, phone)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *mockUserStore) ListUsers(ctx context.Context, filter user.Filter, page, pageSize int) ([]*user.User, int64, error) {
	args := m.Called(ctx, filter, page, pageSize)
	return args.Get(0).([]*user.User), args.Get(1).(int64), args.Error(2)
}

func (m *mockUserStore) UpdateRole(ctx context.Context, id int64, role string) error {
	args := m.Called(ctx, id, role)
	return args.Error(0)
}

func (m *mockUserStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *mockUserStore) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	args := m.Called(ctx, id, passwordHash)
	return args.Error(0)
}

func (m *mockUserStore) UpdatePhone(ctx context.Context, id int64, phone string) error {
	args := m.Called(ctx, id, phone)
	return args.Error(0)
}

func (m *mockUserStore) UpdateUser(ctx context.Context, id int64, fields user.UpdateUserFields) error {
	args := m.Called(ctx, id, fields)
	return args.Error(0)
}

func newAdminTestDeps(t *testing.T) (*service, *mockUserStore, *redis.RefreshTokenManager, *redis.Client) {
	t.Helper()

	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	client, err := redis.New(server.Addr())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
	})

	userStore := new(mockUserStore)
	adminStore := new(mockAdminStore)
	tokenManager := redis.NewRefreshTokenManager(client, time.Hour)

	svc := NewService(adminStore, userStore, tokenManager, client).(*service)
	return svc, userStore, tokenManager, client
}

func TestServiceDisableUserRevokesRefreshSessions(t *testing.T) {
	svc, userStore, tokenManager, client := newAdminTestDeps(t)
	ctx := context.Background()

	require.NoError(t, tokenManager.Save(ctx, "hash-ios", 9, "ios"))
	userStore.On("GetByID", mock.Anything, int64(9)).Return(&user.User{ID: 9, Status: "active"}, nil)
	userStore.On("UpdateStatus", mock.Anything, int64(9), "banned").Return(nil)

	err := svc.DisableUser(ctx, 9)

	require.NoError(t, err)
	userStore.AssertExpectations(t)

	status, getErr := client.Get(ctx, "user_status:9")
	require.NoError(t, getErr)
	assert.Equal(t, "banned", status)

	ok, verifyErr := tokenManager.Verify(ctx, "hash-ios", 9, "ios")
	require.NoError(t, verifyErr)
	assert.False(t, ok)
}

func TestServiceResetPasswordRevokesRefreshSessions(t *testing.T) {
	svc, userStore, tokenManager, _ := newAdminTestDeps(t)
	ctx := context.Background()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.DefaultCost)
	require.NoError(t, err)
	require.NoError(t, tokenManager.Save(ctx, "hash-web", 12, "web"))

	userStore.On("GetByID", mock.Anything, int64(12)).Return(&user.User{
		ID:           12,
		Email:        "admin@example.com",
		PasswordHash: string(passwordHash),
		Status:       "active",
	}, nil)
	userStore.On("UpdatePassword", mock.Anything, int64(12), mock.AnythingOfType("string")).Return(nil)

	err = svc.ResetPassword(ctx, 12, "newpass123")

	require.NoError(t, err)
	userStore.AssertExpectations(t)

	ok, verifyErr := tokenManager.Verify(ctx, "hash-web", 12, "web")
	require.NoError(t, verifyErr)
	assert.False(t, ok)
}

func TestServiceEnableUserClearsBanCache(t *testing.T) {
	svc, userStore, _, client := newAdminTestDeps(t)
	ctx := context.Background()

	userStore.On("GetByID", mock.Anything, int64(15)).Return(&user.User{ID: 15, Status: "banned"}, nil)
	userStore.On("UpdateStatus", mock.Anything, int64(15), "active").Return(nil)
	require.NoError(t, client.Set(ctx, "user_status:15", "banned", 0))

	err := svc.EnableUser(ctx, 15)

	require.NoError(t, err)
	userStore.AssertExpectations(t)

	exists, existsErr := client.Exists(ctx, "user_status:15")
	require.NoError(t, existsErr)
	assert.Equal(t, int64(0), exists)
}

func TestServiceDisableUserReturnsNotFound(t *testing.T) {
	svc, userStore, _, _ := newAdminTestDeps(t)

	userStore.On("GetByID", mock.Anything, int64(99)).Return(nil, user.ErrUserNotFound)

	err := svc.DisableUser(context.Background(), 99)

	require.ErrorIs(t, err, user.ErrUserNotFound)
}
