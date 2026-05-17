package userprofile

import "time"

// Subject category codes mirror agent.go.
const (
	SubjectPhysics = "physics"
	SubjectHistory = "history"
)

// Profile 是 user_profiles 表的瘦身后形态（migration 008 之后）。
// 只保留 4 项核心信息 + 系统/状态字段。其余偏好（单科分/策略/家庭背景/专业偏好等）
// 由 AI 在对话中现问，通过 RecommendationRequest 传给推荐算法，不再持久化到 DB。
type Profile struct {
	UserID int64 `json:"user_id"`

	RegionCode          *string  `json:"region_code,omitempty"`
	SubjectCategoryCode *string  `json:"subject_category_code,omitempty"`
	ElectiveSubjects    []string `json:"elective_subjects,omitempty"` // 再选科目 4 选 2：biology/chemistry/geography/politics
	TotalScore          *int     `json:"total_score,omitempty"`

	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// UpsertRequest is the inbound payload for PUT /me/profile. Every field is
// optional; missing fields leave existing data untouched only if the request
// explicitly omits them — by convention we treat this as a full replacement
// (PUT semantics), so the frontend MUST send the complete current state.
type UpsertRequest struct {
	RegionCode          *string  `json:"region_code" example:"230000"`
	SubjectCategoryCode *string  `json:"subject_category_code" example:"physics"`
	ElectiveSubjects    []string `json:"elective_subjects" example:"biology,chemistry"`
	TotalScore          *int     `json:"total_score" example:"620"`
}

// ProfileResponse is the standard envelope returned by GET / PUT. It mirrors
// Profile plus a derived `completed` flag the frontend uses for status badges.
type ProfileResponse struct {
	Profile
	Completed bool `json:"completed" example:"true"`
}

// ToResponse builds the API response from a Profile.
func ToResponse(p *Profile) ProfileResponse {
	return ProfileResponse{
		Profile:   *p,
		Completed: p.CompletedAt != nil,
	}
}

// EmptyProfileFor returns the zero-state Profile we hand back when a user has
// never filled the questionnaire. The frontend uses this to avoid 404
// branches; everything is nil/empty and `completed` is false.
func EmptyProfileFor(userID int64) *Profile {
	return &Profile{
		UserID: userID,
	}
}
