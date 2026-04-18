package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) Create(ctx context.Context, email, passwordHash, role string) (*User, error) {
	args := m.Called(ctx, email, passwordHash, role)
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

func TestAuthService_Register(t *testing.T) {
	store := new(mockStore)
	svc := NewAuthService(store, nil, nil)

	store.On("Create", mock.Anything, "test@example.com", mock.AnythingOfType("string"), "user").
		Return(&User{ID: 1, Email: "test@example.com", Role: "user"}, nil)

	u, err := svc.Register(context.Background(), "test@example.com", "password123")

	assert.NoError(t, err)
	assert.Equal(t, int64(1), u.ID)
	assert.Equal(t, "test@example.com", u.Email)
	store.AssertExpectations(t)
}

func TestAuthService_Me(t *testing.T) {
	store := new(mockStore)
	svc := NewAuthService(store, nil, nil)

	store.On("GetByID", mock.Anything, int64(1)).
		Return(&User{ID: 1, Email: "test@example.com", Role: "user"}, nil)

	u, err := svc.Me(context.Background(), 1)

	assert.NoError(t, err)
	assert.Equal(t, int64(1), u.ID)
	store.AssertExpectations(t)
}
