package userprofile

import "time"

// Subject category codes mirror agent.go.
const (
	SubjectPhysics = "physics"
	SubjectHistory = "history"
)

// Priority strategy codes mirror agent.go.
const (
	StrategyAuto   = "auto"
	StrategySchool = "school"
	StrategyMajor  = "major"
)

// Profile is the persisted user questionnaire row. Pointers signify "not yet
// filled" so the API can distinguish missing from zero (e.g. total_score 0 is
// a legitimate but rare answer, while nil means "user has not answered yet").
type Profile struct {
	UserID int64 `json:"user_id"`

	RegionCode          *string `json:"region_code,omitempty"`
	SubjectCategoryCode *string `json:"subject_category_code,omitempty"`
	TotalScore          *int    `json:"total_score,omitempty"`
	ProvincialRank      *int    `json:"provincial_rank,omitempty"`
	PlanSize            *int    `json:"plan_size,omitempty"`
	PriorityStrategy    *string `json:"priority_strategy,omitempty"`

	MathScore    *int `json:"math_score,omitempty"`
	PhysicsScore *int `json:"physics_score,omitempty"`
	ChineseScore *int `json:"chinese_score,omitempty"`
	EnglishScore *int `json:"english_score,omitempty"`

	Preferences Preferences `json:"preferences"`

	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Preferences holds the flexible bag of optional fields stored as JSONB.
// All fields are optional. Arrays are nullable (nil = not specified). The
// service layer enforces per-array length caps and per-entry length caps so
// they cannot bloat the LLM context window.
type Preferences struct {
	RequiredMajors     []string `json:"required_majors,omitempty"`
	PreferredMajors    []string `json:"preferred_majors,omitempty"`
	ExcludedMajors     []string `json:"excluded_majors,omitempty"`
	ExcludedKeywords   []string `json:"excluded_keywords,omitempty"`
	PreferredCities    []string `json:"preferred_cities,omitempty"`
	ExcludedCities     []string `json:"excluded_cities,omitempty"`
	PreferredProvinces []string `json:"preferred_provinces,omitempty"`
	ExcludedProvinces  []string `json:"excluded_provinces,omitempty"`

	HollandCode      string `json:"holland_code,omitempty"`
	FamilyResources  string `json:"family_resources,omitempty"`
	FamilyEconomy    string `json:"family_economy,omitempty"`
	CareerPlans      string `json:"career_plans,omitempty"`
	BudgetTuitionMax *int   `json:"budget_tuition_max,omitempty"`
}

// UpsertRequest is the inbound payload for PUT /me/profile. Every field is
// optional; missing fields leave existing data untouched only if the request
// explicitly omits them — by convention we treat this as a full replacement
// (PUT semantics), so the frontend MUST send the complete current state.
type UpsertRequest struct {
	RegionCode          *string `json:"region_code" example:"230000"`
	SubjectCategoryCode *string `json:"subject_category_code" example:"physics"`
	TotalScore          *int    `json:"total_score" example:"620"`
	ProvincialRank      *int    `json:"provincial_rank" example:"4500"`
	PlanSize            *int    `json:"plan_size" example:"40"`
	PriorityStrategy    *string `json:"priority_strategy" example:"auto"`

	MathScore    *int `json:"math_score" example:"135"`
	PhysicsScore *int `json:"physics_score" example:"92"`
	ChineseScore *int `json:"chinese_score" example:"120"`
	EnglishScore *int `json:"english_score" example:"130"`

	Preferences *Preferences `json:"preferences,omitempty"`
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
		UserID:      userID,
		Preferences: Preferences{},
	}
}
