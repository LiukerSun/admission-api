package volunteerplan

import (
	"encoding/json"
	"time"
)

type UserVolunteerPlan struct {
	ID            int64           `json:"id"`
	UserID        int64           `json:"user_id"`
	Title         string          `json:"title"`
	SourceDraftID *int64          `json:"source_draft_id,omitempty"`
	PlanJSON      json.RawMessage `json:"plan_json"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
