package planner

import "time"

// PlannerProfile represents a planner's professional profile.
type PlannerProfile struct {
	ID                int64     `db:"id" json:"id"`
	UserID            int64     `db:"user_id" json:"user_id"`
	RealName          string    `db:"real_name" json:"real_name"`
	Avatar            *string   `db:"avatar" json:"avatar,omitempty"`
	Phone             *string   `db:"phone" json:"phone,omitempty"`
	Title             *string   `db:"title" json:"title,omitempty"`
	Introduction      *string   `db:"introduction" json:"introduction,omitempty"`
	SpecialtyTags     []string  `db:"specialty_tags" json:"specialty_tags,omitempty"`
	ServiceRegion     []string  `db:"service_region" json:"service_region,omitempty"`
	ServicePrice      *float64  `db:"service_price" json:"service_price,omitempty"`
	Level             string    `db:"level" json:"level"`
	LevelExpireAt     *time.Time `db:"level_expire_at" json:"level_expire_at,omitempty"`
	CertificationNo   *string   `db:"certification_no" json:"certification_no,omitempty"`
	MerchantID        *int64    `db:"merchant_id" json:"merchant_id,omitempty"`
	MerchantName      *string   `db:"merchant_name" json:"merchant_name,omitempty"`
	TotalServiceCount int       `db:"total_service_count" json:"total_service_count"`
	RatingAvg         float64   `db:"rating_avg" json:"rating_avg"`
	Status            string    `db:"status" json:"status"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at" json:"updated_at"`
}

// PlannerProfileResponse is the response model with potential service_region inheritance.
type PlannerProfileResponse struct {
	ID                int64      `json:"id"`
	UserID            int64      `json:"user_id"`
	RealName          string     `json:"real_name"`
	Avatar            *string    `json:"avatar,omitempty"`
	Phone             *string    `json:"phone,omitempty"`
	Title             *string    `json:"title,omitempty"`
	Introduction      *string    `json:"introduction,omitempty"`
	SpecialtyTags     []string   `json:"specialty_tags,omitempty"`
	ServiceRegion     []string   `json:"service_region,omitempty"`
	ServicePrice      *float64   `json:"service_price,omitempty"`
	Level             string     `json:"level"`
	LevelExpireAt     *time.Time `json:"level_expire_at,omitempty"`
	CertificationNo   *string    `json:"certification_no,omitempty"`
	MerchantID        *int64     `json:"merchant_id,omitempty"`
	MerchantName      *string    `json:"merchant_name,omitempty"`
	TotalServiceCount int        `json:"total_service_count"`
	RatingAvg         float64    `json:"rating_avg"`
	Status            string     `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// CreateProfileRequest is the admin request to create a planner profile and user.
type CreateProfileRequest struct {
	Email           string     `json:"email" validate:"required,email"`
	Password        string     `json:"password" validate:"required,min=8,alphanum"`
	RealName        string     `json:"real_name" validate:"required,max=64"`
	Avatar          string     `json:"avatar,omitempty"`
	Phone           string     `json:"phone,omitempty"`
	Title           string     `json:"title,omitempty"`
	Introduction    string     `json:"introduction,omitempty"`
	SpecialtyTags   []string   `json:"specialty_tags,omitempty"`
	ServiceRegion   []string   `json:"service_region,omitempty"`
	ServicePrice    *float64   `json:"service_price,omitempty"`
	Level           string     `json:"level,omitempty" validate:"omitempty,oneof=junior intermediate senior expert"`
	LevelExpireAt   *time.Time `json:"level_expire_at,omitempty"`
	CertificationNo string     `json:"certification_no,omitempty"`
	MerchantID      *int64     `json:"merchant_id,omitempty"`
	Status          string     `json:"status,omitempty" validate:"omitempty,oneof=active inactive retired pending"`
}

// UpdateMyProfileRequest is the request for a planner to update their own profile.
type UpdateMyProfileRequest struct {
	RealName        *string    `json:"real_name,omitempty" validate:"omitempty,max=64"`
	Avatar          *string    `json:"avatar,omitempty"`
	Phone           *string    `json:"phone,omitempty"`
	Title           *string    `json:"title,omitempty"`
	Introduction    *string    `json:"introduction,omitempty"`
	SpecialtyTags   []string   `json:"specialty_tags,omitempty"`
	ServiceRegion   []string   `json:"service_region,omitempty"`
	ServicePrice    *float64   `json:"service_price,omitempty"`
	Level           *string    `json:"level,omitempty" validate:"omitempty,oneof=junior intermediate senior expert"`
	LevelExpireAt   *time.Time `json:"level_expire_at,omitempty"`
	CertificationNo *string    `json:"certification_no,omitempty"`
	MerchantID      *int64     `json:"merchant_id,omitempty"`
	Status          *string    `json:"status,omitempty" validate:"omitempty,oneof=active inactive retired pending"`
}

// ProfileFilter defines filters for listing profiles.
type ProfileFilter struct {
	Level      string
	Status     string
	MerchantID *int64
	RealName   string
	Phone      string
}

// ProfileListResponse is the response for listing profiles.
type ProfileListResponse struct {
	Profiles []*PlannerProfileResponse `json:"profiles"`
	Total    int64                     `json:"total"`
}

// CreateProfileInput is the store-layer input for creating a profile.
type CreateProfileInput struct {
	UserID          int64
	RealName        string
	Avatar          *string
	Phone           *string
	Title           *string
	Introduction    *string
	SpecialtyTags   []string
	ServiceRegion   []string
	ServicePrice    *float64
	Level           string
	LevelExpireAt   *time.Time
	CertificationNo *string
	MerchantID      *int64
	MerchantName    *string
	Status          string
}

// UpdateProfileInput is the store-layer input for updating a profile.
type UpdateProfileInput struct {
	RealName        *string
	Avatar          *string
	Phone           *string
	Title           *string
	Introduction    *string
	SpecialtyTags   []string
	ServiceRegion   []string
	ServicePrice    *float64
	Level           *string
	LevelExpireAt   *time.Time
	CertificationNo *string
	MerchantID      *int64
	MerchantName    *string
	Status          *string
}
