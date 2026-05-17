package volunteerplan

import (
	"encoding/json"
	"time"
)

// UserVolunteerPlan 是完整方案对象，含 plan_json（数据量大，~30-60 KB 一份）。
// 仅用于详情查询（GetPlan）。列表查询走 UserVolunteerPlanSummary。
type UserVolunteerPlan struct {
	ID            int64           `json:"id"`
	UserID        int64           `json:"user_id"`
	Title         string          `json:"title"`
	Description   string          `json:"description"` // 方案备注（用户自由文本）
	SourceDraftID *int64          `json:"source_draft_id,omitempty"`
	PlanJSON      json.RawMessage `json:"plan_json"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// UserVolunteerPlanSummary 是列表用的轻量摘要。SchoolCount / GroupCount 直接
// 从 plan_json->stats 取，没有的话（老数据 schema）回退 0；前端可据此显示概览
// 而不必拉全量 plan_json。
type UserVolunteerPlanSummary struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"` // 方案备注摘要，列表里展示
	SchoolCount int       `json:"school_count"`
	GroupCount  int       `json:"group_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
