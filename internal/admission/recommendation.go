package admission

import (
	"context"
	"encoding/json"
)

// RecommendationRequest is the full input to the volunteer recommendation algorithm.
// 包含 #32 契约的基础字段 + v2 算法新增的画像/资源/职业/硬门槛字段。
// ai/tools.go 里 generate_volunteer_plan_draft 工具调 LLM 时只填基础几个字段，
// 扩展字段保持零值即可，service 会按缺失值降级处理。
type RecommendationRequest struct { //nolint:revive // Keeps Swagger and API naming explicit.
	// 基础（必填）
	RegionCode          string `json:"region_code" binding:"required"`           // 考生所在省份，例如 230000 (黑龙江)
	SubjectCategoryCode string `json:"subject_category_code" binding:"required"` // 物理类 / 历史类
	TotalScore          int    `json:"total_score" binding:"required"`           // 高考总分
	ProvincialRank      int    `json:"provincial_rank" binding:"required"`       // 省内位次（来自一分一段）

	// 选科 / 年份
	SubjectRequirementCode string   `json:"subject_requirement_code,omitempty"` // 选科组合编码（用于专业组匹配）
	SelectedSubjects       []string `json:"selected_subjects,omitempty"`        // 选科列表，例如 ["物理","化学","生物"]
	AdmissionYear          *int     `json:"admission_year,omitempty"`           // 默认取数据库中最新年份

	// 单科成绩 (硬门槛 + 能力匹配)
	MathScore    *int `json:"math_score,omitempty"`
	PhysicsScore *int `json:"physics_score,omitempty"`
	ChineseScore *int `json:"chinese_score,omitempty"`
	EnglishScore *int `json:"english_score,omitempty"`

	// 个人画像
	HollandCode      string   `json:"holland_code,omitempty"`      // RIASEC，例如 "RIA"
	PreferredMajors  []string `json:"preferred_majors,omitempty"`  // CHSI 标准专业 / 大类 / 关键词
	ExcludedMajors   []string `json:"excluded_majors,omitempty"`   // 主观排除的专业关键词
	ExcludedKeywords []string `json:"excluded_keywords,omitempty"` // 张雪峰式避雷词

	// 家庭资源
	FamilyResources []string `json:"family_resources,omitempty"` // ["公检法","金融","医疗","教育","电网","商业","普通"]
	FamilyEconomy   string   `json:"family_economy,omitempty"`   // "充裕" / "普通" / "紧张"

	// 地域偏好
	PreferredCities    []string `json:"preferred_cities,omitempty"`
	ExcludedCities     []string `json:"excluded_cities,omitempty"`
	PreferredProvinces []string `json:"preferred_provinces,omitempty"` // region_code 列表
	ExcludedProvinces  []string `json:"excluded_provinces,omitempty"`

	// 职业规划
	CareerPlans []string `json:"career_plans,omitempty"` // ["考公","从医","电网","考研","留学"]

	// 学校 vs 专业 优先策略
	PriorityStrategy string `json:"priority_strategy,omitempty"` // "auto" | "school" | "major"

	// 体检/性别/语种 等一刀切硬门槛
	Gender   string   `json:"gender,omitempty"`
	Language string   `json:"language,omitempty"`
	Health   []string `json:"health,omitempty"` // 已确诊不可报考的限制

	// 经济
	BudgetTuitionMax *int `json:"budget_tuition_max,omitempty"` // 学费上限

	// 输出规模。HLJ 新高考一次 40 个院校专业组，所以 default=40；上限放宽到 500
	// 允许"批量分析"使用方式，常规用户不必关心。
	PlanSize int `json:"plan_size,omitempty"` // 每套志愿的条数；默认 40，上限 500

	// 可选: 是否调用大模型做最终调优
	EnableLLMTuning bool `json:"enable_llm_tuning,omitempty"`
}

// RecommendationResponse 同时满足两个消费者：
//   - /api/v1/admission/recommendations 直接调用方：用 Items + RankWindow + Notes 等结构化字段
//   - ai/tools.go generate_volunteer_plan_draft：只看 VolunteerPlan json.RawMessage（草稿载体）
//
// service 会在 Recommend 末尾把 Items 等序列化进 VolunteerPlan，保证两个通道都不会拿到空。
type RecommendationResponse struct {
	Strategy       string               `json:"strategy,omitempty"`        // "school" | "major" | "rank_window"
	StrategyReason string               `json:"strategy_reason,omitempty"` // 解释为什么走这个路线
	Items          []RecommendationItem `json:"items,omitempty"`           // 全部 ≤ planSize 条，按冲→稳→保顺序排列
	RushCount      int                  `json:"rush_count,omitempty"`
	MatchCount     int                  `json:"match_count,omitempty"`
	SafeCount      int                  `json:"safe_count,omitempty"`
	RankWindow     RankWindow           `json:"rank_window"`                                   // 冲/稳/保三段位次区间
	Notes          []string             `json:"notes,omitempty"`                               // 顶层提示
	VolunteerPlan  json.RawMessage      `json:"volunteer_plan,omitempty" swaggertype:"object"` // 草稿载体，给 toolExecutor 用
	LLMSummary     string               `json:"llm_summary,omitempty"`
}

// RecommendationItem is one slot in the final recommendation list.
type RecommendationItem struct {
	Order              int            `json:"order"`       // 在志愿表里的顺序（1 起，跨冲/稳/保连续编号）
	Tier               string         `json:"tier"`        // "rush" | "match" | "safe"
	Probability        float64        `json:"probability"` // 估算的录取概率 [0,1]
	CompositeScore     float64        `json:"composite_score"`
	Reason             string         `json:"reason"`
	ScoreBreakdown     ScoreBreakdown `json:"score_breakdown"`
	MajorPriorityScore float64        `json:"major_priority_score,omitempty"`

	UniversityID   int64   `json:"university_id"`
	UniversityCode string  `json:"university_code"`
	UniversityName string  `json:"university_name"`
	City           string  `json:"city,omitempty"`
	ProvinceCode   string  `json:"province_code,omitempty"`
	Is985          bool    `json:"is_985"`
	Is211          bool    `json:"is_211"`
	IsDoubleClass  bool    `json:"is_double_first_class"`
	SoftRank       *string `json:"soft_rank,omitempty"`

	AdmissionGroupID int64  `json:"admission_group_id"`
	GroupCode        string `json:"group_code"`
	BatchCode        string `json:"batch_code"`

	LocalMajorCode     string `json:"local_major_code"`
	LocalMajorName     string `json:"local_major_name"`
	DisciplineCategory string `json:"discipline_category,omitempty"`
	MajorRank          string `json:"major_rank,omitempty"`
	IsNationalFeature  *bool  `json:"is_national_feature,omitempty"`

	HistoricalMinScore *int `json:"historical_min_score,omitempty"`
	HistoricalMinRank  *int `json:"historical_min_rank,omitempty"`
	EquivalentMinScore *int `json:"equivalent_min_score,omitempty"`
	PlanCount          *int `json:"plan_count,omitempty"`
	Tuition            *int `json:"tuition,omitempty"`

	Warnings []string `json:"warnings,omitempty"`
}

// ScoreBreakdown lets the frontend explain how the composite score was assembled.
// Each dimension is the product of a base score (precomputed in
// recommendation_precomputed_scores) and a runtime personalization modifier.
type ScoreBreakdown struct {
	CityScore                  float64 `json:"city_score"`
	SchoolScore                float64 `json:"school_score"`
	MajorScore                 float64 `json:"major_score"`
	AbilityImprovementScore    float64 `json:"ability_improvement_score"`
	FutureCompetitivenessScore float64 `json:"future_competitiveness_score"`

	CityBase                  float64 `json:"city_base,omitempty"`
	SchoolBase                float64 `json:"school_base,omitempty"`
	MajorBase                 float64 `json:"major_base,omitempty"`
	AbilityImprovementBase    float64 `json:"ability_improvement_base,omitempty"`
	FutureCompetitivenessBase float64 `json:"future_competitiveness_base,omitempty"`

	CityReason                  string `json:"city_reason,omitempty"`
	SchoolReason                string `json:"school_reason,omitempty"`
	MajorReason                 string `json:"major_reason,omitempty"`
	AbilityImprovementReason    string `json:"ability_improvement_reason,omitempty"`
	FutureCompetitivenessReason string `json:"future_competitiveness_reason,omitempty"`

	EvaluatedBy    string `json:"evaluated_by,omitempty"`    // 'algorithm' | 'llm' | 'manual'
	EvaluatorModel string `json:"evaluator_model,omitempty"` // e.g. claude-opus-4-7
}

// RankWindow 一份志愿表里 冲/稳/保 各占一段位次区间。
type RankWindow struct {
	RushMin  int `json:"rush_min"`
	RushMax  int `json:"rush_max"`
	MatchMin int `json:"match_min"`
	MatchMax int `json:"match_max"`
	SafeMin  int `json:"safe_min"`
	SafeMax  int `json:"safe_max"`
}

// RecommendationService is the orchestrator for the volunteer recommendation algorithm.
// 实现见 recommendation_service.go。noopRecommendationService 留作单元测试和 tools.go
// 接线时的最小兜底（在 service 还没初始化好时也不要 panic）。
type RecommendationService interface {
	Recommend(ctx context.Context, req *RecommendationRequest) (*RecommendationResponse, error)
}

// RecommendationTuner is the optional final-pass tuner (LLM or human).
type RecommendationTuner interface {
	Tune(ctx context.Context, req *RecommendationRequest, resp *RecommendationResponse) (*RecommendationResponse, error)
}

type noopRecommendationService struct{}

// NewNoopRecommendationService 返回空实现：用于尚未初始化真实 service 的兜底，
// 满足 toolExecutor 必须拿到 volunteer_plan 不为 nil 的契约（返回 {"items":[]}）。
func NewNoopRecommendationService() RecommendationService {
	return noopRecommendationService{}
}

func (noopRecommendationService) Recommend(_ context.Context, _ *RecommendationRequest) (*RecommendationResponse, error) {
	return &RecommendationResponse{
		VolunteerPlan: json.RawMessage(`{"items":[]}`),
	}, nil
}
