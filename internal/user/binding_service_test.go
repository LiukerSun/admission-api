package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockBindingStore struct {
	mock.Mock
}

func (m *mockBindingStore) CreateBinding(ctx context.Context, parentID, studentID int64) (*Binding, error) {
	args := m.Called(ctx, parentID, studentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Binding), args.Error(1)
}

func (m *mockBindingStore) GetBindingsByParent(ctx context.Context, parentID int64) ([]*Binding, error) {
	args := m.Called(ctx, parentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Binding), args.Error(1)
}

func (m *mockBindingStore) GetBindingByStudent(ctx context.Context, studentID int64) (*Binding, error) {
	args := m.Called(ctx, studentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Binding), args.Error(1)
}

func (m *mockBindingStore) DeleteBinding(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockBindingStore) BindingExistsForStudent(ctx context.Context, studentID int64) (bool, error) {
	args := m.Called(ctx, studentID)
	return args.Bool(0), args.Error(1)
}

func TestBindingService_BindStudent_Success(t *testing.T) {
	userStore := new(mockStore)
	bindingStore := new(mockBindingStore)
	svc := NewBindingService(userStore, bindingStore)

	userStore.On("GetByEmailAndType", mock.Anything, "student@test.com", "student").
		Return(&User{ID: 5, Email: "student@test.com", UserType: "student"}, nil)
	bindingStore.On("BindingExistsForStudent", mock.Anything, int64(5)).
		Return(false, nil)
	bindingStore.On("CreateBinding", mock.Anything, int64(1), int64(5)).
		Return(&Binding{ID: 10, ParentID: 1, StudentID: 5}, nil)

	b, err := svc.BindStudent(context.Background(), 1, "student@test.com")

	assert.NoError(t, err)
	assert.Equal(t, int64(10), b.ID)
	assert.Equal(t, int64(1), b.ParentID)
	assert.Equal(t, int64(5), b.StudentID)
	userStore.AssertExpectations(t)
	bindingStore.AssertExpectations(t)
}

func TestBindingService_BindStudent_StudentNotFound(t *testing.T) {
	userStore := new(mockStore)
	bindingStore := new(mockBindingStore)
	svc := NewBindingService(userStore, bindingStore)

	userStore.On("GetByEmailAndType", mock.Anything, "notfound@test.com", "student").
		Return(nil, assert.AnError)

	_, err := svc.BindStudent(context.Background(), 1, "notfound@test.com")

	assert.Error(t, err)
	assert.Equal(t, "student not found", err.Error())
}

func TestBindingService_BindStudent_AlreadyBound(t *testing.T) {
	userStore := new(mockStore)
	bindingStore := new(mockBindingStore)
	svc := NewBindingService(userStore, bindingStore)

	userStore.On("GetByEmailAndType", mock.Anything, "student@test.com", "student").
		Return(&User{ID: 5, Email: "student@test.com", UserType: "student"}, nil)
	bindingStore.On("BindingExistsForStudent", mock.Anything, int64(5)).
		Return(true, nil)

	_, err := svc.BindStudent(context.Background(), 1, "student@test.com")

	assert.Error(t, err)
	assert.Equal(t, "student already bound to another parent", err.Error())
}

func TestBindingService_GetMyBindings_Parent(t *testing.T) {
	userStore := new(mockStore)
	bindingStore := new(mockBindingStore)
	svc := NewBindingService(userStore, bindingStore)

	bindingStore.On("GetBindingsByParent", mock.Anything, int64(1)).
		Return([]*Binding{
			{ID: 10, ParentID: 1, StudentID: 5},
		}, nil)
	userStore.On("GetByID", mock.Anything, int64(5)).
		Return(&User{ID: 5, Email: "student@test.com", UserType: "student"}, nil)

	result, err := svc.GetMyBindings(context.Background(), 1, "parent")

	assert.NoError(t, err)
	assert.Equal(t, "parent", result.UserType)
	assert.Len(t, result.Bindings, 1)
	assert.Equal(t, int64(5), result.Bindings[0].User.ID)
}

func TestBindingService_GetMyBindings_Student(t *testing.T) {
	userStore := new(mockStore)
	bindingStore := new(mockBindingStore)
	svc := NewBindingService(userStore, bindingStore)

	bindingStore.On("GetBindingByStudent", mock.Anything, int64(5)).
		Return(&Binding{ID: 10, ParentID: 1, StudentID: 5}, nil)
	userStore.On("GetByID", mock.Anything, int64(1)).
		Return(&User{ID: 1, Email: "parent@test.com", UserType: "parent"}, nil)

	result, err := svc.GetMyBindings(context.Background(), 5, "student")

	assert.NoError(t, err)
	assert.Equal(t, "student", result.UserType)
	assert.Len(t, result.Bindings, 1)
	assert.Equal(t, int64(1), result.Bindings[0].User.ID)
}

func TestBindingService_RemoveBinding_Success(t *testing.T) {
	userStore := new(mockStore)
	bindingStore := new(mockBindingStore)
	svc := NewBindingService(userStore, bindingStore)

	bindingStore.On("DeleteBinding", mock.Anything, int64(10)).Return(nil)

	err := svc.RemoveBinding(context.Background(), 10)

	assert.NoError(t, err)
	bindingStore.AssertExpectations(t)
}
