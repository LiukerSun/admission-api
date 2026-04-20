package user

import (
	"context"
	"fmt"
)

// BindingService defines the binding business logic interface.
type BindingService interface {
	BindStudent(ctx context.Context, parentID int64, studentEmail string) (*Binding, error)
	GetMyBindings(ctx context.Context, userID int64, userType string) (*BindingListResult, error)
	RemoveBinding(ctx context.Context, id int64) error
}

// BindingListResult represents the binding query result with user perspective.
type BindingListResult struct {
	UserType string             `json:"user_type"`
	Bindings []*BindingWithUser `json:"bindings"`
}

// BindingWithUser augments a binding with the associated user info.
type BindingWithUser struct {
	ID        int64    `json:"id"`
	User      SafeUser `json:"user"`
	CreatedAt string   `json:"created_at"`
}

// SafeUser is a minimal user representation for binding responses.
type SafeUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type bindingService struct {
	userStore    Store
	bindingStore BindingStore
}

func NewBindingService(userStore Store, bindingStore BindingStore) BindingService {
	return &bindingService{
		userStore:    userStore,
		bindingStore: bindingStore,
	}
}

func (s *bindingService) BindStudent(ctx context.Context, parentID int64, studentEmail string) (*Binding, error) {
	student, err := s.userStore.GetByEmailAndType(ctx, studentEmail, "student")
	if err != nil {
		return nil, fmt.Errorf("student not found")
	}

	if student.ID == parentID {
		return nil, fmt.Errorf("cannot bind yourself")
	}

	exists, err := s.bindingStore.BindingExistsForStudent(ctx, student.ID)
	if err != nil {
		return nil, fmt.Errorf("check binding status: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("student already bound to another parent")
	}

	binding, err := s.bindingStore.CreateBinding(ctx, parentID, student.ID)
	if err != nil {
		return nil, fmt.Errorf("create binding: %w", err)
	}

	return binding, nil
}

func (s *bindingService) GetMyBindings(ctx context.Context, userID int64, userType string) (*BindingListResult, error) {
	result := &BindingListResult{
		UserType: userType,
		Bindings: []*BindingWithUser{},
	}

	if userType == "parent" {
		bindings, err := s.bindingStore.GetBindingsByParent(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get bindings: %w", err)
		}
		for _, b := range bindings {
			student, err := s.userStore.GetByID(ctx, b.StudentID)
			if err != nil {
				continue
			}
			result.Bindings = append(result.Bindings, &BindingWithUser{
				ID: b.ID,
				User: SafeUser{
					ID:    student.ID,
					Email: student.Email,
				},
				CreatedAt: b.CreatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}
	} else if userType == "student" {
		binding, err := s.bindingStore.GetBindingByStudent(ctx, userID)
		if err != nil {
			if err.Error() == "binding not found" {
				return result, nil
			}
			return nil, fmt.Errorf("get binding: %w", err)
		}
		parent, err := s.userStore.GetByID(ctx, binding.ParentID)
		if err != nil {
			return result, nil
		}
		result.Bindings = append(result.Bindings, &BindingWithUser{
			ID: binding.ID,
			User: SafeUser{
				ID:    parent.ID,
				Email: parent.Email,
			},
			CreatedAt: binding.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return result, nil
}

func (s *bindingService) RemoveBinding(ctx context.Context, id int64) error {
	if err := s.bindingStore.DeleteBinding(ctx, id); err != nil {
		return fmt.Errorf("remove binding: %w", err)
	}
	return nil
}
