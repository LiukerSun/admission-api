package planner

import "time"

// PlannerMerchant represents a planner institution/merchant.
type PlannerMerchant struct {
	ID                  int64     `db:"id" json:"id"`
	MerchantName        string    `db:"merchant_name" json:"merchant_name"`
	ContactPerson       *string   `db:"contact_person" json:"contact_person,omitempty"`
	ContactPhone        *string   `db:"contact_phone" json:"contact_phone,omitempty"`
	Address             *string   `db:"address" json:"address,omitempty"`
	Logo                *string   `db:"logo" json:"logo,omitempty"`
	Banner              *string   `db:"banner" json:"banner,omitempty"`
	Description         *string   `db:"description" json:"description,omitempty"`
	SortOrder           int       `db:"sort_order" json:"sort_order"`
	OwnerID             *int64    `db:"owner_id" json:"owner_id,omitempty"`
	ServiceRegions      []string  `db:"service_regions" json:"service_regions,omitempty"`
	DefaultServicePrice *float64  `db:"default_service_price" json:"default_service_price,omitempty"`
	Status              string    `db:"status" json:"status"`
	CreatedAt           time.Time `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time `db:"updated_at" json:"updated_at"`
}

// CreateMerchantRequest is the request to create a merchant.
type CreateMerchantRequest struct {
	MerchantName        string   `json:"merchant_name" validate:"required,max=128"`
	ContactPerson       string   `json:"contact_person,omitempty"`
	ContactPhone        string   `json:"contact_phone,omitempty"`
	Address             string   `json:"address,omitempty"`
	Logo                string   `json:"logo,omitempty"`
	Banner              string   `json:"banner,omitempty"`
	Description         string   `json:"description,omitempty"`
	SortOrder           int      `json:"sort_order"`
	OwnerID             *int64   `json:"owner_id,omitempty"`
	ServiceRegions      []string `json:"service_regions,omitempty"`
	DefaultServicePrice *float64 `json:"default_service_price,omitempty"`
	Status              string   `json:"status,omitempty" validate:"omitempty,oneof=active inactive"`
}

// UpdateMerchantRequest is the request to update a merchant (pointer fields = update only if non-nil).
type UpdateMerchantRequest struct {
	MerchantName        *string   `json:"merchant_name,omitempty" validate:"omitempty,max=128"`
	ContactPerson       *string   `json:"contact_person,omitempty"`
	ContactPhone        *string   `json:"contact_phone,omitempty"`
	Address             *string   `json:"address,omitempty"`
	Logo                *string   `json:"logo,omitempty"`
	Banner              *string   `json:"banner,omitempty"`
	Description         *string   `json:"description,omitempty"`
	SortOrder           *int      `json:"sort_order,omitempty"`
	OwnerID             *int64    `json:"owner_id,omitempty"`
	ServiceRegions      []string  `json:"service_regions,omitempty"`
	DefaultServicePrice *float64  `json:"default_service_price,omitempty"`
	Status              *string   `json:"status,omitempty" validate:"omitempty,oneof=active inactive"`
}

// MerchantListResponse is the response for listing merchants.
type MerchantListResponse struct {
	Merchants []*PlannerMerchant `json:"merchants"`
	Total     int64              `json:"total"`
}

// CreateMerchantInput is the store-layer input for creating a merchant.
type CreateMerchantInput struct {
	MerchantName        string
	ContactPerson       *string
	ContactPhone        *string
	Address             *string
	Logo                *string
	Banner              *string
	Description         *string
	SortOrder           int
	OwnerID             *int64
	ServiceRegions      []string
	DefaultServicePrice *float64
	Status              string
}

// UpdateMerchantInput is the store-layer input for updating a merchant.
type UpdateMerchantInput struct {
	MerchantName        *string
	ContactPerson       *string
	ContactPhone        *string
	Address             *string
	Logo                *string
	Banner              *string
	Description         *string
	SortOrder           *int
	OwnerID             *int64
	ServiceRegions      []string
	DefaultServicePrice *float64
	Status              *string
}
