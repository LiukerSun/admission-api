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
	// DraftStatusSuperseded 表示草稿因为用户后续修改了偏好而被新草稿顶替。
	// 与 failed 区分：算法本身没出问题；只是不再代表用户最新意图。
	// 仍然可读，但 listByConversation 复用查找会跳过；MarkAdopted 也拒绝采纳。
	DraftStatusSuperseded DraftStatus = "superseded"
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
