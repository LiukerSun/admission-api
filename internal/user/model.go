package user

import "time"

// StringValue dereferences a string pointer, returning "" for nil.
func StringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// User is the domain model for a user account.
type User struct {
	ID              int64      `json:"id"`
	Email           *string    `json:"email"`
	Username        *string    `json:"username"`
	Phone           *string    `json:"phone"`
	PhoneVerifiedAt *time.Time `json:"phone_verified_at"`
	PasswordHash    string     `json:"-"`
	Role            string     `json:"role"`
	IsAdmin         bool       `json:"is_admin"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
