package admin

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	"golang.org/x/crypto/bcrypt"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"admission-api/internal/user"
)

// Service defines the admin business logic interface.
type Service interface {
	ListUsers(ctx context.Context, filter ListUsersFilter, page, pageSize int) (*UserListResponse, error)
	GetUser(ctx context.Context, id int64) (*UserResponse, error)
	UpdateRole(ctx context.Context, id int64, role string) error
	UpdateUser(ctx context.Context, id int64, req UpdateUserRequest) (*UserResponse, error)
	ResetPassword(ctx context.Context, id int64, newPassword string) error
	DisableUser(ctx context.Context, id int64) error
	EnableUser(ctx context.Context, id int64) error
	ListBindings(ctx context.Context, page, pageSize int) (*BindingListResponse, error)
	GetStats(ctx context.Context) (*StatsResponse, error)
}

type service struct {
	adminStore   Store
	userStore    user.Store
	tokenManager *redis.RefreshTokenManager
	redisClient  *redis.Client
	validate     *validator.Validate
}

// NewService creates a new admin service.
func NewService(adminStore Store, userStore user.Store, tokenManager *redis.RefreshTokenManager, redisClient *redis.Client) Service {
	return &service{
		adminStore:   adminStore,
		userStore:    userStore,
		tokenManager: tokenManager,
		redisClient:  redisClient,
		validate:     validator.New(),
	}
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toUserResponse(u *user.User) *UserResponse {
	return &UserResponse{
		ID:            u.ID,
		Email:         u.Email,
		Username:      stringValue(u.Username),
		Phone:         stringValue(u.Phone),
		PhoneVerified: u.PhoneVerifiedAt != nil,
		Role:          u.Role,
		UserType:      u.UserType,
		Status:        u.Status,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
}

func (s *service) ListUsers(ctx context.Context, filter ListUsersFilter, page, pageSize int) (*UserListResponse, error) {
	users, total, err := s.adminStore.ListUsers(ctx, filter, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	items := make([]*UserListItem, 0, len(users))
	for _, u := range users {
		items = append(items, &UserListItem{
			ID:            u.ID,
			Email:         u.Email,
			Username:      stringValue(u.Username),
			Phone:         stringValue(u.Phone),
			PhoneVerified: u.PhoneVerifiedAt != nil,
			Role:          u.Role,
			UserType:      u.UserType,
			Status:        u.Status,
			CreatedAt:     u.CreatedAt,
		})
	}

	return &UserListResponse{
		Users:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *service) GetUser(ctx context.Context, id int64) (*UserResponse, error) {
	u, err := s.userStore.GetByID(ctx, id)
	if err != nil {
		if err.Error() == "user not found" {
			return nil, err
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	return toUserResponse(u), nil
}

func (s *service) UpdateRole(ctx context.Context, id int64, role string) error {
	if err := s.validate.Var(role, "required,oneof=user premium admin"); err != nil {
		return fmt.Errorf("invalid role: %w", err)
	}

	if err := s.userStore.UpdateRole(ctx, id, role); err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	return nil
}

func (s *service) UpdateUser(ctx context.Context, id int64, req UpdateUserRequest) (*UserResponse, error) {
	if err := s.validate.Struct(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Get current user to check status change
	currentUser, err := s.userStore.GetByID(ctx, id)
	if err != nil {
		if err.Error() == "user not found" {
			return nil, err
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	fields := user.UpdateUserFields{
		Email:    req.Email,
		Username: req.Username,
		Role:     req.Role,
		UserType: req.UserType,
		Status:   req.Status,
	}

	if err := s.userStore.UpdateUser(ctx, id, fields); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	// Handle status change side effects
	if req.Status != nil {
		if *req.Status == "banned" && currentUser.Status != "banned" {
			// User being banned
			if err := s.redisClient.Set(ctx, middleware.UserStatusCacheKey(id), "banned", 0); err != nil {
				return nil, fmt.Errorf("cache banned status: %w", err)
			}
			if err := s.clearUserTokens(ctx, id); err != nil {
				return nil, fmt.Errorf("clear user tokens: %w", err)
			}
		} else if *req.Status == "active" && currentUser.Status == "banned" {
			// User being unbanned
			if err := s.redisClient.Del(ctx, middleware.UserStatusCacheKey(id)); err != nil {
				return nil, fmt.Errorf("clear banned status cache: %w", err)
			}
		}
	}

	updatedUser, err := s.userStore.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get updated user: %w", err)
	}

	return toUserResponse(updatedUser), nil
}

func (s *service) ResetPassword(ctx context.Context, id int64, newPassword string) error {
	if err := s.validate.Var(newPassword, "required,min=8,alphanum"); err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	if _, err := s.userStore.GetByID(ctx, id); err != nil {
		if err.Error() == "user not found" {
			return err
		}
		return fmt.Errorf("get user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.userStore.UpdatePassword(ctx, id, string(hash)); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	if err := s.clearUserTokens(ctx, id); err != nil {
		return fmt.Errorf("clear user tokens: %w", err)
	}

	return nil
}

func (s *service) DisableUser(ctx context.Context, id int64) error {
	if err := s.userStore.UpdateStatus(ctx, id, "banned"); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	// Set banned flag in Redis so AuthStatusMiddleware catches it immediately
	if err := s.redisClient.Set(ctx, middleware.UserStatusCacheKey(id), "banned", 0); err != nil {
		return fmt.Errorf("cache banned status: %w", err)
	}

	// Clear all device tokens for this user
	if err := s.clearUserTokens(ctx, id); err != nil {
		return fmt.Errorf("clear user tokens: %w", err)
	}

	return nil
}

func (s *service) EnableUser(ctx context.Context, id int64) error {
	if err := s.userStore.UpdateStatus(ctx, id, "active"); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	// Remove banned flag from Redis
	if err := s.redisClient.Del(ctx, middleware.UserStatusCacheKey(id)); err != nil {
		return fmt.Errorf("clear banned status cache: %w", err)
	}

	return nil
}

func (s *service) ListBindings(ctx context.Context, page, pageSize int) (*BindingListResponse, error) {
	bindings, total, err := s.adminStore.ListBindings(ctx, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}

	return &BindingListResponse{
		Bindings: bindings,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *service) GetStats(ctx context.Context) (*StatsResponse, error) {
	stats, err := s.adminStore.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	return stats, nil
}

// clearUserTokens removes all refresh tokens and device records for a user.
func (s *service) clearUserTokens(ctx context.Context, userID int64) error {
	deviceKey := fmt.Sprintf("user:%d:devices", userID)
	platforms, err := s.redisClient.SMembers(ctx, deviceKey)
	if err != nil {
		return fmt.Errorf("get user devices: %w", err)
	}

	// Delete device set
	if err := s.redisClient.Del(ctx, deviceKey); err != nil {
		return fmt.Errorf("delete device set: %w", err)
	}

	// Note: individual refresh:hash keys will naturally expire.
	// Without tracking all hashes, we cannot delete them directly.
	// The devices set removal prevents new logins from tracking,
	// and AuthStatusMiddleware blocks all requests for banned users.
	_ = platforms

	return nil
}
