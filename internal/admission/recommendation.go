package admission

import (
	"context"
	"encoding/json"
)

type RecommendationRequest struct {
	RegionCode          string   `json:"region_code"`
	SubjectCategoryCode string   `json:"subject_category_code"`
	TotalScore          int      `json:"total_score"`
	ProvincialRank      int      `json:"provincial_rank"`
	PreferredCities     []string `json:"preferred_cities,omitempty"`
	PreferredMajors     []string `json:"preferred_majors,omitempty"`
	PriorityStrategy    string   `json:"priority_strategy,omitempty"`
	PlanSize            int      `json:"plan_size,omitempty"`
	EnableLLMTuning     bool     `json:"enable_llm_tuning,omitempty"`
}

type RecommendationResponse struct {
	Strategy      string          `json:"strategy,omitempty"`
	RushCount     int             `json:"rush_count,omitempty"`
	MatchCount    int             `json:"match_count,omitempty"`
	SafeCount     int             `json:"safe_count,omitempty"`
	RankWindow    any             `json:"rank_window,omitempty"`
	VolunteerPlan json.RawMessage `json:"volunteer_plan,omitempty"`
}

type RecommendationService interface {
	Recommend(ctx context.Context, req *RecommendationRequest) (*RecommendationResponse, error)
}

type noopRecommendationService struct{}

func NewNoopRecommendationService() RecommendationService {
	return noopRecommendationService{}
}

func (noopRecommendationService) Recommend(ctx context.Context, req *RecommendationRequest) (*RecommendationResponse, error) {
	plan := json.RawMessage(`{"items":[]}`)
	return &RecommendationResponse{
		Strategy:      "",
		RushCount:     0,
		MatchCount:    0,
		SafeCount:     0,
		RankWindow:    nil,
		VolunteerPlan: plan,
	}, nil
}
