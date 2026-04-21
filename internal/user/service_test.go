package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	svc := NewAuthService(store, nil, nil)

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
