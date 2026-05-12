package admission

// RecommendationRequest is the full input to the volunteer recommendation algorithm.
// All knowledge that the algorithm needs about the student travels through this struct.
type RecommendationRequest struct { //nolint:revive // Keeps Swagger and API naming explicit.
	// 基础信息
	RegionCode             string   `json:"region_code" binding:"required"`           // 考生所在省份，例如 230000 (黑龙江)
	SubjectCategoryCode    string   `json:"subject_category_code" binding:"required"` // 物理类 / 历史类
	SubjectRequirementCode string   `json:"subject_requirement_code,omitempty"`       // 选科组合编码（用于专业组匹配）
	SelectedSubjects       []string `json:"selected_subjects,omitempty"`              // 选科列表，例如 ["物理","化学","生物"]
	TotalScore             int      `json:"total_score" binding:"required"`           // 高考总分
	ProvincialRank         int      `json:"provincial_rank" binding:"required"`       // 省内位次（来自一分一段）
	AdmissionYear          *int     `json:"admission_year,omitempty"`                 // 默认取数据库中最新年份

	// 单科成绩 (硬门槛 + 能力匹配)
	MathScore    *int `json:"math_score,omitempty"`
	PhysicsScore *int `json:"physics_score,omitempty"`
	ChineseScore *int `json:"chinese_score,omitempty"`
	EnglishScore *int `json:"english_score,omitempty"`

	// 个人画像
	HollandCode      string   `json:"holland_code,omitempty"`      // RIASEC，例如 "RIA"
	PreferredMajors  []string `json:"preferred_majors,omitempty"`  // CHSI 标准专业 / 大类 / 关键词
	ExcludedMajors   []string `json:"excluded_majors,omitempty"`   // 主观排除的专业关键词
	ExcludedKeywords []string `json:"excluded_keywords,omitempty"` // 张雪峰式避雷词，如 ["生物","环境","材料"]

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

	// 输出规模——黑龙江新高考一次最多 40 个院校专业组
	PlanSize int `json:"plan_size,omitempty"` // 每套志愿的条数；默认 40，上限 40

	// 可选: 是否调用大模型做最终调优
	EnableLLMTuning bool `json:"enable_llm_tuning,omitempty"`
}

// RecommendationItem is one slot in the final recommendation list.
type RecommendationItem struct {
	Order              int            `json:"order"`       // 在志愿表里的顺序（1 起，跨冲/稳/保连续编号）
	Tier               string         `json:"tier"`        // "rush" | "match" | "safe" — 该条所属档位
	Probability        float64        `json:"probability"` // 估算的录取概率 [0,1]
	CompositeScore     float64        `json:"composite_score"`
	Reason             string         `json:"reason"` // 为什么推荐
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

	Warnings []string `json:"warnings,omitempty"` // 如：物理成绩偏低，选此专业风险大
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

	// Base values pulled straight from recommendation_precomputed_scores.
	// Useful for the frontend to show "本校该专业基础得分 vs 你的偏好加成".
	CityBase                  float64 `json:"city_base,omitempty"`
	SchoolBase                float64 `json:"school_base,omitempty"`
	MajorBase                 float64 `json:"major_base,omitempty"`
	AbilityImprovementBase    float64 `json:"ability_improvement_base,omitempty"`
	FutureCompetitivenessBase float64 `json:"future_competitiveness_base,omitempty"`

	// Reasons captured at evaluation time (LLM / manual / algorithm).
	CityReason                  string `json:"city_reason,omitempty"`
	SchoolReason                string `json:"school_reason,omitempty"`
	MajorReason                 string `json:"major_reason,omitempty"`
	AbilityImprovementReason    string `json:"ability_improvement_reason,omitempty"`
	FutureCompetitivenessReason string `json:"future_competitiveness_reason,omitempty"`

	EvaluatedBy    string `json:"evaluated_by,omitempty"`    // 'algorithm' | 'llm' | 'manual'
	EvaluatorModel string `json:"evaluator_model,omitempty"` // e.g. claude-opus-4-7
}

// RankWindow describes the three rank-axis buckets the algorithm pulled candidates from.
// 一份志愿表 40 个院校专业组里，冲/稳/保各占一段位次区间，前端用这个解释为什么某条进了某档。
type RankWindow struct {
	RushMin  int `json:"rush_min"`
	RushMax  int `json:"rush_max"`
	MatchMin int `json:"match_min"`
	MatchMax int `json:"match_max"`
	SafeMin  int `json:"safe_min"`
	SafeMax  int `json:"safe_max"`
}

// RecommendationResponse is what the handler returns.
// 黑龙江新高考一次报 40 个院校专业组——所以算法一次就生成一张表，里面混合冲/稳/保。
type RecommendationResponse struct {
	Strategy       string               `json:"strategy"`        // "school" | "major"
	StrategyReason string               `json:"strategy_reason"` // 解释为什么走这个路线
	Items          []RecommendationItem `json:"items"`           // 全部 ≤ 40 条，按冲→稳→保顺序排列
	RushCount      int                  `json:"rush_count"`
	MatchCount     int                  `json:"match_count"`
	SafeCount      int                  `json:"safe_count"`
	RankWindow     RankWindow           `json:"rank_window"`
	Notes          []string             `json:"notes,omitempty"` // 顶层提示，例如 "已根据物理 32 分排除电子信息类"
	VolunteerPlan  *VolunteerPlan       `json:"volunteer_plan,omitempty"`
	LLMSummary     string               `json:"llm_summary,omitempty"`
}
