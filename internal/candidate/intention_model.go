package candidate

import "time"

// Intention represents a candidate's intention (province / school / major / school_major).
type Intention struct {
	ID            int64     `db:"id" json:"id"`
	ProfileID     int64     `db:"profile_id" json:"profile_id"`
	IntentionType string    `db:"intention_type" json:"intention_type"`
	TargetID      string    `db:"target_id" json:"target_id"`
	TargetName    *string   `db:"target_name" json:"target_name,omitempty"`
	Priority      int       `db:"priority" json:"priority"`
	Notes         *string   `db:"notes" json:"notes,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// IntentionItemInput is a single intention item from client.
type IntentionItemInput struct {
	TargetID   string `json:"target_id" validate:"required,max=50"`
	TargetName string `json:"target_name" validate:"required,max=100"`
	Priority   int    `json:"priority"`
	Notes      string `json:"notes,omitempty" validate:"max=255"`
}

// SaveIntentionsRequest is the batch save request.
type SaveIntentionsRequest struct {
	Items []IntentionItemInput `json:"items" validate:"required,dive"`
}

// IntentionGroupResponse groups intentions by type.
type IntentionGroupResponse struct {
	Province    []*Intention `json:"province,omitempty"`
	School      []*Intention `json:"school,omitempty"`
	Major       []*Intention `json:"major,omitempty"`
	SchoolMajor []*Intention `json:"school_major,omitempty"`
}

// CreateIntentionInput is the store-layer write input.
type CreateIntentionInput struct {
	ProfileID     int64
	IntentionType string
	TargetID      string
	TargetName    *string
	Priority      int
	Notes         *string
}

var validIntentionTypes = map[string]struct{}{
	"province":     {},
	"school":       {},
	"major":        {},
	"school_major": {},
}

func isValidIntentionType(t string) bool {
	_, ok := validIntentionTypes[t]
	return ok
}
