package volunteerplan

import (
	"encoding/json"
	"time"
)

type DraftStatus string

const (
	DraftStatusGenerating DraftStatus = "generating"
	DraftStatusReady      DraftStatus = "ready"
	DraftStatusFailed     DraftStatus = "failed"
	DraftStatusAdopted    DraftStatus = "adopted"
)

type Draft struct {
	ID               int64           `json:"id"`
	UserID           int64           `json:"user_id"`
	ConversationID   int64           `json:"conversation_id"`
	Status           DraftStatus     `json:"status"`
	InputJSON        json.RawMessage `json:"input_json"`
	PlanJSON         json.RawMessage `json:"plan_json,omitempty"`
	AlgorithmVersion string          `json:"algorithm_version"`
	Error            string          `json:"error,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}
