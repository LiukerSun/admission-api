package user

import "time"

// User is the domain model for a user account.
type User struct {
	ID              int64      `json:"id"`
	Email           string     `json:"email"`
	Username        *string    `json:"username"`
	Phone           *string    `json:"phone"`
	PhoneVerifiedAt *time.Time `json:"phone_verified_at"`
	PasswordHash    string     `json:"-"`
	Role            string     `json:"role"`
	UserType        string     `json:"user_type"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
