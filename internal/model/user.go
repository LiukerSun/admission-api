package model

import "time"

// User represents a registered user.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	Username     *string   `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	UserType     string    `json:"user_type"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SafeUser returns a user without sensitive fields.
func (u *User) SafeUser() User {
	return User{
		ID:        u.ID,
		Email:     u.Email,
		Username:  u.Username,
		Role:      u.Role,
		UserType:  u.UserType,
		Status:    u.Status,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
