package model

import "time"

// User represents a registered user.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	UserType     string    `json:"user_type"`
	ProvinceCode string    `json:"province_code"`
	SubjectType  string    `json:"subject_type"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SafeUser returns a user without sensitive fields.
func (u *User) SafeUser() User {
	return User{
		ID:           u.ID,
		Email:        u.Email,
		Role:         u.Role,
		UserType:     u.UserType,
		ProvinceCode: u.ProvinceCode,
		SubjectType:  u.SubjectType,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}
