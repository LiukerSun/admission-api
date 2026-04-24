package user

import (
	"context"
	"sync"
	"testing"
	"time"

	"admission-api/internal/platform/middleware"
	platformredis "admission-api/internal/platform/redis"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) Create(ctx context.Context, email, passwordHash, role, userType string) (*User, error) {
	args := m.Called(ctx, email, passwordHash, role, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *mockStore) GetByEmailAndType(ctx context.Context, email, userType string) (*User, error) {
	args := m.Called(ctx, email, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *mockStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *mockStore) GetByID(ctx context.Context, id int64) (*User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *mockStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *mockStore) GetByPhone(ctx context.Context, phone string) (*User, error) {
	args := m.Called(ctx, phone)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *mockStore) ListUsers(ctx context.Context, filter Filter, page, pageSize int) ([]*User, int64, error) {
	args := m.Called(ctx, filter, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*User), args.Get(1).(int64), args.Error(2)
}

func (m *mockStore) UpdateRole(ctx context.Context, id int64, role string) error {
	args := m.Called(ctx, id, role)
	return args.Error(0)
}

func (m *mockStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *mockStore) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	args := m.Called(ctx, id, passwordHash)
	return args.Error(0)
}

func (m *mockStore) UpdatePhone(ctx context.Context, id int64, phone string) error {
	args := m.Called(ctx, id, phone)
	return args.Error(0)
}

func (m *mockStore) UpdateUser(ctx context.Context, id int64, fields UpdateUserFields) error {
	args := m.Called(ctx, id, fields)
	return args.Error(0)
}

func TestAuthService_Register(t *testing.T) {
	store := new(mockStore)
	svc := NewAuthService(store, nil, nil)

	store.On("Create", mock.Anything, "test@example.com", mock.AnythingOfType("string"), "user", "student").
		Return(&User{ID: 1, Email: "test@example.com", Role: "user", UserType: "student"}, nil)

	u, err := svc.Register(context.Background(), "test@example.com", "password123", "student")

	assert.NoError(t, err)
	assert.Equal(t, int64(1), u.ID)
	assert.Equal(t, "test@example.com", u.Email)
	assert.Equal(t, "student", u.UserType)
	store.AssertExpectations(t)
}

func TestAuthService_Register_InvalidUserType(t *testing.T) {
	store := new(mockStore)
	svc := NewAuthService(store, nil, nil)

	_, err := svc.Register(context.Background(), "test@example.com", "password123", "invalid")

	assert.Error(t, err)
}

func TestAuthService_Me(t *testing.T) {
	store := new(mockStore)
	svc := NewAuthService(store, nil, nil)

	store.On("GetByID", mock.Anything, int64(1)).
		Return(&User{ID: 1, Email: "test@example.com", Role: "user", UserType: "parent"}, nil)

	u, err := svc.Me(context.Background(), 1)

	assert.NoError(t, err)
	assert.Equal(t, int64(1), u.ID)
	assert.Equal(t, "parent", u.UserType)
	store.AssertExpectations(t)
}

func TestAuthService_ChangePassword(t *testing.T) {
	store := new(mockStore)
	tokenManager, _, _ := newUserTestTokenManager(t)
	svc := NewAuthService(store, tokenManager, nil)

	hash, err := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.DefaultCost)
	assert.NoError(t, err)

	store.On("GetByID", mock.Anything, int64(1)).
		Return(&User{ID: 1, Email: "test@example.com", PasswordHash: string(hash)}, nil)
	store.On("UpdatePassword", mock.Anything, int64(1), mock.AnythingOfType("string")).
		Return(nil)

	err = svc.ChangePassword(context.Background(), 1, "oldpass123", "newpass123")

	assert.NoError(t, err)
	store.AssertExpectations(t)
}

func TestAuthService_RefreshAllowsOnlySingleUseRotation(t *testing.T) {
	tokenManager, _, _ := newUserTestTokenManager(t)
	jwtConfig := &middleware.JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	}
	svc := NewAuthService(nil, tokenManager, jwtConfig)

	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, 7, "user", "parent", "ios")
	require.NoError(t, err)
	require.NoError(t, tokenManager.Save(context.Background(), middleware.HashRefreshToken(tokens.RefreshToken), 7, "ios"))

	results := make(chan *middleware.TokenPair, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, refreshErr := svc.Refresh(context.Background(), tokens.RefreshToken)
			results <- resp
			errs <- refreshErr
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	var responses []*middleware.TokenPair
	for resp := range results {
		require.NotNil(t, resp)
		responses = append(responses, resp)
	}
	require.Len(t, responses, 2)
	assert.Equal(t, responses[0].AccessToken, responses[1].AccessToken)
	assert.Equal(t, responses[0].RefreshToken, responses[1].RefreshToken)
}

func TestAuthService_RefreshReplayExpiresWithWindow(t *testing.T) {
	tokenManager, client, server := newUserTestTokenManager(t)
	jwtConfig := &middleware.JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	}
	svc := NewAuthService(nil, tokenManager, jwtConfig)

	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, 7, "user", "parent", "ios")
	require.NoError(t, err)
	oldHash := middleware.HashRefreshToken(tokens.RefreshToken)
	require.NoError(t, tokenManager.Save(context.Background(), oldHash, 7, "ios"))

	first, err := svc.Refresh(context.Background(), tokens.RefreshToken)
	require.NoError(t, err)
	require.NotNil(t, first)

	ttl, err := client.TTL(context.Background(), "refresh_rotation:"+oldHash)
	require.NoError(t, err)
	require.Positive(t, ttl)

	server.FastForward(ttl + time.Second)

	_, err = svc.Refresh(context.Background(), tokens.RefreshToken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired refresh token")
}

func TestAuthService_ChangePasswordRevokesRefreshSessions(t *testing.T) {
	store := new(mockStore)
	tokenManager, _, _ := newUserTestTokenManager(t)
	jwtConfig := &middleware.JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	}
	svc := NewAuthService(store, tokenManager, jwtConfig)

	oldPasswordHash, err := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.DefaultCost)
	require.NoError(t, err)
	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, 1, "user", "parent", "ios")
	require.NoError(t, err)
	require.NoError(t, tokenManager.Save(context.Background(), middleware.HashRefreshToken(tokens.RefreshToken), 1, "ios"))

	store.On("GetByID", mock.Anything, int64(1)).
		Return(&User{ID: 1, Email: "test@example.com", PasswordHash: string(oldPasswordHash)}, nil)
	store.On("UpdatePassword", mock.Anything, int64(1), mock.AnythingOfType("string")).
		Return(nil)

	err = svc.ChangePassword(context.Background(), 1, "oldpass123", "newpass123")
	require.NoError(t, err)

	_, err = svc.Refresh(context.Background(), tokens.RefreshToken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired refresh token")
	store.AssertExpectations(t)
}

func newUserTestTokenManager(t *testing.T) (*platformredis.RefreshTokenManager, *platformredis.Client, *miniredis.Miniredis) {
	t.Helper()

	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	client, err := platformredis.New(server.Addr())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
	})

	return platformredis.NewRefreshTokenManager(client, time.Hour), client, server
}
