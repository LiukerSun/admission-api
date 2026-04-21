package admin

import "time"

// UserResponse represents a single user detail payload for admin operations.
type UserResponse struct {
	ID        int64     `json:"id" example:"1"`
	Email     string    `json:"email" example:"user@example.com"`
	Username  string    `json:"username" example:"johndoe"`
	Role      string    `json:"role" example:"user"`
	UserType  string    `json:"user_type" example:"student"`
	Status    string    `json:"status" example:"active"`
	CreatedAt time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt time.Time `json:"updated_at" example:"2024-01-01T00:00:00Z"`
}

// UserListItem represents a user in the admin list response.
type UserListItem struct {
	ID        int64     `json:"id" example:"1"`
	Email     string    `json:"email" example:"user@example.com"`
	Username  string    `json:"username" example:"johndoe"`
	Role      string    `json:"role" example:"user"`
	UserType  string    `json:"user_type" example:"student"`
	Status    string    `json:"status" example:"active"`
	CreatedAt time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
}

// UserListResponse is the response for listing users.
type UserListResponse struct {
	Users    []*UserListItem `json:"users"`
	Total    int64           `json:"total" example:"100"`
	Page     int             `json:"page" example:"1"`
	PageSize int             `json:"page_size" example:"20"`
}

// UpdateRoleRequest is the request body for updating a user's role.
type UpdateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user premium admin" example:"premium"`
}

// UpdateUserRequest is the request body for updating a user by admin.
type UpdateUserRequest struct {
	Email    *string `json:"email" validate:"omitempty,email" example:"user@example.com"`
	Username *string `json:"username" validate:"omitempty,min=1,max=50" example:"johndoe"`
	Role     *string `json:"role" validate:"omitempty,oneof=user premium admin" example:"premium"`
	UserType *string `json:"user_type" validate:"omitempty,oneof=parent student" example:"student"`
	Status   *string `json:"status" validate:"omitempty,oneof=active banned" example:"active"`
}

// BindingListItem represents a binding in the admin list response.
type BindingListItem struct {
	ID        int64        `json:"id" example:"1"`
	Parent    SafeUserInfo `json:"parent"`
	Student   SafeUserInfo `json:"student"`
	CreatedAt time.Time    `json:"created_at" example:"2024-01-01T00:00:00Z"`
}

// SafeUserInfo is a minimal user representation for admin responses.
type SafeUserInfo struct {
	ID    int64  `json:"id" example:"1"`
	Email string `json:"email" example:"user@example.com"`
}

// BindingListResponse is the response for listing bindings.
type BindingListResponse struct {
	Bindings []*BindingListItem `json:"bindings"`
	Total    int64              `json:"total" example:"100"`
	Page     int                `json:"page" example:"1"`
	PageSize int                `json:"page_size" example:"20"`
}

// StatsResponse is the response for system statistics.
type StatsResponse struct {
	TotalUsers    int64            `json:"total_users" example:"1000"`
	UsersByRole   map[string]int64 `json:"users_by_role"`
	TotalBindings int64            `json:"total_bindings" example:"500"`
	ActiveUsers   int64            `json:"active_users" example:"950"`
	BannedUsers   int64            `json:"banned_users" example:"50"`
}

// ListUsersFilter defines filters for listing users.
type ListUsersFilter struct {
	Email    string
	Username string
	Role     string
	Status   string
}
