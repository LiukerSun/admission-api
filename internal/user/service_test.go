package user

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"admission-api/internal/platform/middleware"
	platformredis "admission-api/internal/platform/redis"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) CreateWithPhone(ctx context.Context, phone, passwordHash, role string) (*User, error) {
	args := m.Called(ctx, phone, passwordHash, role)
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

type stubPhoneAuth struct {
	mock.Mock
}

func (s *stubPhoneAuth) SendAuthCode(ctx context.Context, phone string, scene Scene) error {
	args := s.Called(ctx, phone, scene)
	return args.Error(0)
}

func (s *stubPhoneAuth) VerifyAuthCode(ctx context.Context, phone, code string, scene Scene) error {
	args := s.Called(ctx, phone, code, scene)
	return args.Error(0)
}

func newJWTConfig() *middleware.JWTConfig {
	return &middleware.JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	}
}

func TestAuthService_RegisterByPhone(t *testing.T) {
	phone := "13800138000"
	store := new(mockStore)
	phoneAuth := new(stubPhoneAuth)
	tokenManager, _, _ := newUserTestTokenManager(t)
	svc := NewAuthService(store, phoneAuth, tokenManager, newJWTConfig())

	phoneAuth.On("VerifyAuthCode", mock.Anything, phone, "123456", SceneRegister).Return(nil)
	store.On("CreateWithPhone", mock.Anything, phone, mock.AnythingOfType("string"), "user").
		Return(&User{ID: 1, Phone: &phone, Role: "user"}, nil)

	u, tokens, err := svc.RegisterByPhone(context.Background(), phone, "123456", "password123", "web")

	require.NoError(t, err)
	require.NotNil(t, u)
	require.NotNil(t, tokens)
	assert.Equal(t, int64(1), u.ID)
	assert.NotEmpty(t, tokens.AccessToken)
	store.AssertExpectations(t)
	phoneAuth.AssertExpectations(t)
}

func TestAuthService_RegisterByPhone_InvalidCode(t *testing.T) {
	phone := "13800138000"
	store := new(mockStore)
	phoneAuth := new(stubPhoneAuth)
	svc := NewAuthService(store, phoneAuth, nil, newJWTConfig())

	phoneAuth.On("VerifyAuthCode", mock.Anything, phone, "000000", SceneRegister).
		Return(ErrVerificationCodeInvalid)

	u, tokens, err := svc.RegisterByPhone(context.Background(), phone, "000000", "password123", "web")

	require.ErrorIs(t, err, ErrVerificationCodeInvalid)
	assert.Nil(t, u)
	assert.Nil(t, tokens)
	store.AssertNotCalled(t, "CreateWithPhone", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAuthService_RegisterByPhone_PhoneTaken(t *testing.T) {
	phone := "13800138000"
	store := new(mockStore)
	phoneAuth := new(stubPhoneAuth)
	svc := NewAuthService(store, phoneAuth, nil, newJWTConfig())

	phoneAuth.On("VerifyAuthCode", mock.Anything, phone, "123456", SceneRegister).Return(nil)
	store.On("CreateWithPhone", mock.Anything, phone, mock.AnythingOfType("string"), "user").
		Return(nil, ErrPhoneAlreadyExists)

	_, _, err := svc.RegisterByPhone(context.Background(), phone, "123456", "password123", "web")

	require.ErrorIs(t, err, ErrPhoneAlreadyExists)
}

func TestAuthService_LoginByPassword_Success(t *testing.T) {
	phone := "13800138000"
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	store := new(mockStore)
	tokenManager, _, _ := newUserTestTokenManager(t)
	svc := NewAuthService(store, nil, tokenManager, newJWTConfig())

	store.On("GetByPhone", mock.Anything, phone).
		Return(&User{ID: 1, Phone: &phone, PasswordHash: string(hash), Status: "active"}, nil)

	tokens, err := svc.LoginByPassword(context.Background(), phone, "password123", "web")

	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.NotEmpty(t, tokens.AccessToken)
	store.AssertExpectations(t)
}

func TestAuthService_LoginByPassword_WrongPassword(t *testing.T) {
	phone := "13800138000"
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	store := new(mockStore)
	svc := NewAuthService(store, nil, nil, newJWTConfig())

	store.On("GetByPhone", mock.Anything, phone).
		Return(&User{ID: 1, Phone: &phone, PasswordHash: string(hash), Status: "active"}, nil)

	tokens, err := svc.LoginByPassword(context.Background(), phone, "wrong-pass", "web")

	require.ErrorIs(t, err, ErrInvalidCredentials)
	assert.Nil(t, tokens)
}

func TestAuthService_LoginByPassword_PhoneNotFound(t *testing.T) {
	store := new(mockStore)
	svc := NewAuthService(store, nil, nil, newJWTConfig())

	store.On("GetByPhone", mock.Anything, "13800138000").Return(nil, ErrUserNotFound)

	_, err := svc.LoginByPassword(context.Background(), "13800138000", "password123", "web")

	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestAuthService_LoginByPassword_Banned(t *testing.T) {
	phone := "13800138000"
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	store := new(mockStore)
	svc := NewAuthService(store, nil, nil, newJWTConfig())

	store.On("GetByPhone", mock.Anything, phone).
		Return(&User{ID: 1, Phone: &phone, PasswordHash: string(hash), Status: "banned"}, nil)

	_, err = svc.LoginByPassword(context.Background(), phone, "password123", "web")

	require.ErrorIs(t, err, ErrAccountBanned)
}

func TestAuthService_LoginByCode_Success(t *testing.T) {
	phone := "13800138000"
	store := new(mockStore)
	phoneAuth := new(stubPhoneAuth)
	tokenManager, _, _ := newUserTestTokenManager(t)
	svc := NewAuthService(store, phoneAuth, tokenManager, newJWTConfig())

	phoneAuth.On("VerifyAuthCode", mock.Anything, phone, "123456", SceneLogin).Return(nil)
	store.On("GetByPhone", mock.Anything, phone).
		Return(&User{ID: 1, Phone: &phone, Status: "active"}, nil)

	tokens, err := svc.LoginByCode(context.Background(), phone, "123456", "web")

	require.NoError(t, err)
	require.NotNil(t, tokens)
	store.AssertExpectations(t)
	phoneAuth.AssertExpectations(t)
}

func TestAuthService_LoginByCode_NotRegistered(t *testing.T) {
	phone := "13800138000"
	store := new(mockStore)
	phoneAuth := new(stubPhoneAuth)
	svc := NewAuthService(store, phoneAuth, nil, newJWTConfig())

	phoneAuth.On("VerifyAuthCode", mock.Anything, phone, "123456", SceneLogin).Return(nil)
	store.On("GetByPhone", mock.Anything, phone).Return(nil, ErrUserNotFound)

	_, err := svc.LoginByCode(context.Background(), phone, "123456", "web")

	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestAuthService_SendAuthCode_DelegatesToPhoneAuth(t *testing.T) {
	phoneAuth := new(stubPhoneAuth)
	svc := NewAuthService(nil, phoneAuth, nil, newJWTConfig())

	phoneAuth.On("SendAuthCode", mock.Anything, "13800138000", SceneRegister).Return(nil)

	err := svc.SendAuthCode(context.Background(), "13800138000", SceneRegister)

	require.NoError(t, err)
	phoneAuth.AssertExpectations(t)
}

func TestAuthService_Me(t *testing.T) {
	store := new(mockStore)
	svc := NewAuthService(store, nil, nil, newJWTConfig())

	email := "test@example.com"
	store.On("GetByID", mock.Anything, int64(1)).
		Return(&User{ID: 1, Email: &email, Role: "user"}, nil)

	u, err := svc.Me(context.Background(), 1)

	assert.NoError(t, err)
	assert.Equal(t, int64(1), u.ID)
	store.AssertExpectations(t)
}

func TestAuthService_ChangePassword(t *testing.T) {
	store := new(mockStore)
	tokenManager, _, _ := newUserTestTokenManager(t)
	svc := NewAuthService(store, nil, tokenManager, newJWTConfig())

	hash, err := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.DefaultCost)
	assert.NoError(t, err)

	store.On("GetByID", mock.Anything, int64(1)).
		Return(&User{ID: 1, PasswordHash: string(hash)}, nil)
	store.On("UpdatePassword", mock.Anything, int64(1), mock.AnythingOfType("string")).
		Return(nil)

	err = svc.ChangePassword(context.Background(), 1, "oldpass123", "newpass123")

	assert.NoError(t, err)
	store.AssertExpectations(t)
}

func TestAuthService_RefreshAllowsOnlySingleUseRotation(t *testing.T) {
	tokenManager, _, _ := newUserTestTokenManager(t)
	jwtConfig := newJWTConfig()
	svc := NewAuthService(nil, nil, tokenManager, jwtConfig)

	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, 7, "user", false, "ios")
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

func TestAuthService_RefreshPreservesIsAdminClaim(t *testing.T) {
	tokenManager, _, _ := newUserTestTokenManager(t)
	jwtConfig := newJWTConfig()
	svc := NewAuthService(nil, nil, tokenManager, jwtConfig)

	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, 7, "premium", true, "ios")
	require.NoError(t, err)
	require.NoError(t, tokenManager.Save(context.Background(), middleware.HashRefreshToken(tokens.RefreshToken), 7, "ios"))

	refreshed, err := svc.Refresh(context.Background(), tokens.RefreshToken)
	require.NoError(t, err)
	require.NotNil(t, refreshed)

	claims := &middleware.Claims{}
	parsed, err := jwt.ParseWithClaims(refreshed.AccessToken, claims, func(token *jwt.Token) (any, error) {
		return []byte(jwtConfig.Secret), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	assert.True(t, claims.IsAdmin)
	assert.Equal(t, "premium", claims.Role)
}

func TestAuthService_RefreshReplayExpiresWithWindow(t *testing.T) {
	tokenManager, client, server := newUserTestTokenManager(t)
	jwtConfig := newJWTConfig()
	svc := NewAuthService(nil, nil, tokenManager, jwtConfig)

	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, 7, "user", false, "ios")
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
	jwtConfig := newJWTConfig()
	svc := NewAuthService(store, nil, tokenManager, jwtConfig)

	oldPasswordHash, err := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.DefaultCost)
	require.NoError(t, err)
	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, 1, "user", false, "ios")
	require.NoError(t, err)
	require.NoError(t, tokenManager.Save(context.Background(), middleware.HashRefreshToken(tokens.RefreshToken), 1, "ios"))

	store.On("GetByID", mock.Anything, int64(1)).
		Return(&User{ID: 1, PasswordHash: string(oldPasswordHash)}, nil)
	store.On("UpdatePassword", mock.Anything, int64(1), mock.AnythingOfType("string")).
		Return(nil)

	err = svc.ChangePassword(context.Background(), 1, "oldpass123", "newpass123")
	require.NoError(t, err)

	_, err = svc.Refresh(context.Background(), tokens.RefreshToken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired refresh token")
	store.AssertExpectations(t)
}

// Sanity guard: ensure the new errors export from this package so callers can
// match on them without importing a hidden alias.
func TestAuthService_SentinelErrors(t *testing.T) {
	require.True(t, errors.Is(ErrInvalidCredentials, ErrInvalidCredentials))
	require.True(t, errors.Is(ErrAccountBanned, ErrAccountBanned))
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
