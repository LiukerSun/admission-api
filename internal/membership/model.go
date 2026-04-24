package membership

import "time"

const (
	LevelNone    = "none"
	LevelPremium = "premium"

	PlanStatusActive   = "active"
	PlanStatusInactive = "inactive"

	MembershipStatusInactive = "inactive"
	MembershipStatusActive   = "active"
	MembershipStatusExpired  = "expired"

	GrantActionActivate = "activate"
	GrantActionRenew    = "renew"
	GrantActionRestore  = "restore"
)

// Plan is a purchasable premium membership offering.
type Plan struct {
	ID              int64     `json:"id"`
	PlanCode        string    `json:"plan_code"`
	PlanName        string    `json:"plan_name"`
	MembershipLevel string    `json:"membership_level"`
	DurationDays    int       `json:"duration_days"`
	PriceAmount     int       `json:"price_amount"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UserMembership is the current membership state for a user.
type UserMembership struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	MembershipLevel string     `json:"membership_level"`
	Status          string     `json:"status"`
	StartedAt       *time.Time `json:"started_at"`
	EndsAt          *time.Time `json:"ends_at"`
	LastOrderID     *int64     `json:"last_order_id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Grant records a membership entitlement grant event.
type Grant struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	PaymentOrderID int64     `json:"payment_order_id"`
	SourceType     string    `json:"source_type"`
	Action         string    `json:"action"`
	DurationDays   int       `json:"duration_days"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// GrantRequest contains the payment context needed to grant membership.
type GrantRequest struct {
	UserID         int64
	PaymentOrderID int64
	DurationDays   int
	IdempotencyKey string
	Now            time.Time
}

type PlanResponse struct {
	ID              int64  `json:"id" example:"1"`
	PlanCode        string `json:"plan_code" example:"monthly"`
	PlanName        string `json:"plan_name" example:"月度会员"`
	MembershipLevel string `json:"membership_level" example:"premium"`
	DurationDays    int    `json:"duration_days" example:"30"`
	PriceAmount     int    `json:"price_amount" example:"990"`
	Currency        string `json:"currency" example:"CNY"`
	Status          string `json:"status" example:"active"`
}

type CurrentMembershipResponse struct {
	MembershipLevel string     `json:"membership_level" example:"premium"`
	Status          string     `json:"status" example:"active"`
	StartedAt       *time.Time `json:"started_at,omitempty" example:"2026-04-23T10:00:00Z"`
	EndsAt          *time.Time `json:"ends_at,omitempty" example:"2026-05-23T10:00:00Z"`
	Active          bool       `json:"active" example:"true"`
}

func ToPlanResponse(p *Plan) PlanResponse {
	return PlanResponse{
		ID:              p.ID,
		PlanCode:        p.PlanCode,
		PlanName:        p.PlanName,
		MembershipLevel: p.MembershipLevel,
		DurationDays:    p.DurationDays,
		PriceAmount:     p.PriceAmount,
		Currency:        p.Currency,
		Status:          p.Status,
	}
}
