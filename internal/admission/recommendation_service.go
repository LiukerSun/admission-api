package admission

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// RecommendationService is the orchestrator for the volunteer recommendation algorithm.
// It pulls candidates from the store, runs the seven-step pipeline (strategy → rank window →
// hard filter → ability/preference → geo → tier split → composite scoring), and assembles
// the response. Step seven (LLM tuning) is delegated to an optional Tuner.
type RecommendationService interface {
	Recommend(ctx context.Context, req *RecommendationRequest) (*RecommendationResponse, error)
}

// RecommendationTuner is the optional final-pass tuner (LLM or human).
// Implementations should mutate the response or return a new one with reordered/edited items.
type RecommendationTuner interface {
	Tune(ctx context.Context, req *RecommendationRequest, resp *RecommendationResponse) (*RecommendationResponse, error)
}

type recommendationService struct {
	store    RecommendationStore
	metadata RecommendationMetadataStore
	tuner    RecommendationTuner // optional, may be nil
}

func NewRecommendationService(store RecommendationStore, metadata RecommendationMetadataStore, tuner RecommendationTuner) RecommendationService {
	return &recommendationService{store: store, metadata: metadata, tuner: tuner}
}

// 张雪峰式生化环材避雷需要的 CHSI 大类码常量。这些是国标专业目录里
// 客观存在的代码，不是业务策略，所以保留为常量；其余阈值/映射全部 DB 化。
const (
	chsiCategoryChemistry     = "0703" // 化学类
	chsiCategoryChemicalEng   = "0813" // 化工与制药类
	chsiCategoryMaterials     = "0804" // 材料类
	chsiCategoryEnvironmental = "0825" // 环境科学与工程类
	chsiCategoryBio           = "0710" // 生物科学类
)

const (
	// 黑龙江新高考一次报 40 个院校专业组志愿。一张表里混合冲/稳/保。
	planSizeCap     = 40
	defaultPlanSize = 40

	// 一张表里冲/稳/保的位次窗口（相对学生位次 R）：
	//   冲 [R-15000, R-2000]   — 比 R 靠前的院校专业组（更难录），激进
	//   稳 [R-3000,  R+3000]   — 围绕 R，按实际位次稳取
	//   保 [R+2000,  R+15000]  — 比 R 靠后的院校专业组（基本能录），兜底
	rushPlanMinGapBelow = 15000 // R - 15000
	rushPlanMaxGapBelow = 2000  // R - 2000
	matchPlanGap        = 3000  // R ± 3000
	safePlanMinGapAbove = 2000  // R + 2000
	safePlanMaxGapAbove = 15000 // R + 15000

	// 黑龙江志愿单位是"院校专业组"——同一 (university, group_code) 在真实志愿表里就是
	// 一个志愿位。所以这里按 group 去重；同一学校多个 group 算多个志愿位。
	// 每校最多 maxGroupsPerSchool 个组，仍然保留对大体量学校的多样性约束。
	maxGroupsPerSchool = 4
)

// bucketWindow 是单个档位（冲/稳/保）的位次范围，仅在 service 内部使用，不暴露到 JSON。
type bucketWindow struct {
	Min int
	Max int
}

func (s *recommendationService) Recommend(ctx context.Context, req *RecommendationRequest) (*RecommendationResponse, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	applyDefaultCounts(req)

	year, err := s.resolveAdmissionYear(ctx, req)
	if err != nil {
		return nil, err
	}

	md, err := s.metadata.Load(ctx)
	if err != nil {
		return nil, err
	}

	// 一张志愿表（≤40 条）按位次轴分成冲/稳/保三段，每段独立取候选再合并。
	rushW := clampBucketWindow(req.ProvincialRank-rushPlanMinGapBelow, req.ProvincialRank-rushPlanMaxGapBelow)
	matchW := clampBucketWindow(req.ProvincialRank-matchPlanGap, req.ProvincialRank+matchPlanGap)
	safeW := clampBucketWindow(req.ProvincialRank+safePlanMinGapAbove, req.ProvincialRank+safePlanMaxGapAbove)

	// 一次性把覆盖三段窗口的候选池都捞下来，按桶在内存里打分挑选
	candidates, err := s.store.FetchCandidates(ctx, &CandidateQuery{
		AdmissionYear:          year,
		RegionCode:             req.RegionCode,
		SubjectCategoryCode:    req.SubjectCategoryCode,
		SubjectRequirementCode: req.SubjectRequirementCode,
		RankMin:                rushW.Min,
		RankMax:                safeW.Max,
		BudgetTuitionMax:       req.BudgetTuitionMax,
		ExcludedProvinces:      req.ExcludedProvinces,
		ExcludedCities:         req.ExcludedCities,
	})
	if err != nil {
		return nil, err
	}

	strategy, strategyReason := decideStrategy(req, md)

	filtered, filterNotes := filterByPreference(candidates, req, md)
	notes := make([]string, 0, len(filterNotes))
	notes = append(notes, filterNotes...)

	rushQuota, matchQuota, safeQuota := splitTierQuota(req.PlanSize)

	// 跨档共享去重——同一院校专业组只能进表一次，同一学校最多 maxGroupsPerSchool 个 group
	seenGroups := map[string]struct{}{}
	usedSchools := map[string]int{}

	// 配额下溢回填：如果上一档取不满（如学生位次太靠前，rush 窗口 clamp 成 [1,1]
	// 几乎没候选），把剩余名额加到下一档，保证总条数仍尽量靠近 planSize。
	rushItems := pickBucket(filtered, req, md, strategy, "rush", rushW, rushQuota, seenGroups, usedSchools)
	matchQuota += rushQuota - len(rushItems)
	matchItems := pickBucket(filtered, req, md, strategy, "match", matchW, matchQuota, seenGroups, usedSchools)
	safeQuota += matchQuota - len(matchItems)
	safeItems := pickBucket(filtered, req, md, strategy, "safe", safeW, safeQuota, seenGroups, usedSchools)
	safeItems = ensureSafeQuality(safeItems, req)

	if rushQuota > 0 && len(rushItems) == 0 {
		notes = append(notes, "你的位次太靠前，'冲'档已无更高目标可推荐，名额已转给'稳'档")
	}

	items := make([]RecommendationItem, 0, len(rushItems)+len(matchItems)+len(safeItems))
	items = append(items, rushItems...)
	items = append(items, matchItems...)
	items = append(items, safeItems...)
	for i := range items {
		items[i].Order = i + 1
	}

	resp := &RecommendationResponse{
		Strategy:       strategy,
		StrategyReason: strategyReason,
		Items:          items,
		RushCount:      len(rushItems),
		MatchCount:     len(matchItems),
		SafeCount:      len(safeItems),
		RankWindow: RankWindow{
			RushMin:  rushW.Min,
			RushMax:  rushW.Max,
			MatchMin: matchW.Min,
			MatchMax: matchW.Max,
			SafeMin:  safeW.Min,
			SafeMax:  safeW.Max,
		},
		Notes:         notes,
		VolunteerPlan: buildVolunteerPlan(items, "智能推荐"),
	}

	if req.EnableLLMTuning && s.tuner != nil {
		tuned, err := s.tuner.Tune(ctx, req, resp)
		if err == nil && tuned != nil {
			resp = tuned
		}
	}
	return resp, nil
}

// pickBucket 从全量候选中筛出位次落在 window 内的，按 strategy 打分，挑出 quota 条带上 tier 标签。
// 跨档去重通过共享的 seenGroups / usedSchools 实现——靠前的档位（冲）先挑，避免稳/保截到同一个组。
func pickBucket(
	candidates []RecommendationCandidate,
	req *RecommendationRequest,
	md *RecommendationMetadata,
	strategy, tier string,
	window bucketWindow,
	quota int,
	seenGroups map[string]struct{},
	usedSchools map[string]int,
) []RecommendationItem {
	if quota <= 0 {
		return nil
	}
	inWindow := make([]RecommendationCandidate, 0, len(candidates))
	for i := range candidates {
		c := &candidates[i]
		if c.MinRank == nil {
			continue
		}
		r := *c.MinRank
		if r >= window.Min && r <= window.Max {
			inWindow = append(inWindow, candidates[i])
		}
	}
	scored := scoreCandidates(inWindow, req, md, tier, strategy)
	out := make([]RecommendationItem, 0, quota)
	for i := range scored {
		it := &scored[i].item
		groupKey := it.UniversityCode + "|" + it.GroupCode
		if _, dup := seenGroups[groupKey]; dup {
			continue
		}
		if usedSchools[it.UniversityCode] >= maxGroupsPerSchool {
			continue
		}
		it.Tier = tier
		out = append(out, *it)
		seenGroups[groupKey] = struct{}{}
		usedSchools[it.UniversityCode]++
		if len(out) >= quota {
			break
		}
	}
	return out
}

func clampBucketWindow(minRank, maxRank int) bucketWindow {
	if minRank < 1 {
		minRank = 1
	}
	if maxRank < minRank {
		maxRank = minRank
	}
	return bucketWindow{Min: minRank, Max: maxRank}
}

// splitTierQuota 按 1:2:1 把总志愿表条数拆给冲/稳/保。
// 例：planSize=40 → 10/20/10。下取整后剩余余数全归到稳，保证总和等于 planSize。
func splitTierQuota(planSize int) (rush, match, safe int) {
	if planSize <= 0 {
		return 0, 0, 0
	}
	rush = planSize / 4
	safe = planSize / 4
	match = planSize - rush - safe
	return rush, match, safe
}

func validateRequest(req *RecommendationRequest) error {
	if req == nil {
		return fmt.Errorf("nil request")
	}
	if req.RegionCode == "" {
		return fmt.Errorf("region_code is required")
	}
	if req.SubjectCategoryCode == "" {
		return fmt.Errorf("subject_category_code is required")
	}
	if req.TotalScore <= 0 {
		return fmt.Errorf("total_score must be positive")
	}
	if req.ProvincialRank <= 0 {
		return fmt.Errorf("provincial_rank must be positive")
	}
	return nil
}

func applyDefaultCounts(req *RecommendationRequest) {
	if req.PlanSize <= 0 {
		req.PlanSize = defaultPlanSize
	}
	if req.PlanSize > planSizeCap {
		req.PlanSize = planSizeCap
	}
	if req.PriorityStrategy == "" {
		req.PriorityStrategy = "auto"
	}
}

func (s *recommendationService) resolveAdmissionYear(ctx context.Context, req *RecommendationRequest) (int, error) {
	if req.AdmissionYear != nil && *req.AdmissionYear > 0 {
		return *req.AdmissionYear, nil
	}
	year, err := s.store.LatestAdmissionYear(ctx, req.RegionCode, req.SubjectCategoryCode)
	if err != nil {
		return 0, err
	}
	if year == 0 {
		return 0, fmt.Errorf("no admission data for region=%s category=%s", req.RegionCode, req.SubjectCategoryCode)
	}
	return year, nil
}

// decideStrategy implements 逻辑二: STEM intent → major-first, humanities → school-first.
// 关键字来自 recommendation_strategy_keywords 表（migration 010），便于业务运营调整无需发版。
// 用户显式指定 PriorityStrategy 时直接采用。
func decideStrategy(req *RecommendationRequest, md *RecommendationMetadata) (strategy, reason string) {
	if req.PriorityStrategy == "school" || req.PriorityStrategy == "major" {
		return req.PriorityStrategy, "用户显式指定"
	}
	var stemKeywords, humanitiesKeywords []string
	if md != nil {
		stemKeywords = md.StrategyKeywords["stem"]
		humanitiesKeywords = md.StrategyKeywords["humanities"]
	}
	stem := containsAny(req.PreferredMajors, stemKeywords...)
	humanities := containsAny(req.PreferredMajors, humanitiesKeywords...)
	switch {
	case stem && !humanities:
		return "major", "理工类专业有专业壁垒，专业重要性 > 学校牌子"
	case humanities && !stem:
		return "school", "文管类无强专业壁垒，学校牌子（圈层、城市、平台）权重更高"
	default:
		return "major", "未指明强偏好，默认按专业优先（保留‘卡档边缘’时再切换到学校优先）"
	}
}

// filterByPreference applies in-memory排除法 + 匹配法.
// 返回过滤后的候选集 + 顶层 notes（向用户解释做了什么排除）。
//
// 能力门槛、张雪峰式避雷的关键字/阈值都来自 metadata（DB 表
// recommendation_major_ability_rules），不再硬编码。
func filterByPreference(candidates []RecommendationCandidate, req *RecommendationRequest, md *RecommendationMetadata) (filtered []RecommendationCandidate, notes []string) {
	notes = []string{}

	// 把 ability rules 折叠成 "(category, subject) → exclude_below"
	excludeRules := map[string]map[string]int{}
	for cat, rules := range md.AbilityRules {
		for i := range rules {
			r := rules[i]
			if _, ok := excludeRules[cat]; !ok {
				excludeRules[cat] = map[string]int{}
			}
			excludeRules[cat][r.Subject] = r.ExcludeBelowScore
		}
	}
	subjectScore := func(subject string) (int, bool) {
		switch subject {
		case "physics":
			if req.PhysicsScore != nil {
				return *req.PhysicsScore, true
			}
		case "math":
			if req.MathScore != nil {
				return *req.MathScore, true
			}
		case "chinese":
			if req.ChineseScore != nil {
				return *req.ChineseScore, true
			}
		case "english":
			if req.EnglishScore != nil {
				return *req.EnglishScore, true
			}
		}
		return 0, false
	}

	noteSubjects := map[string]bool{}

	// 张雪峰式避雷：把用户给的关键字精确映射到对应的 CHSI 大类，避免一刀切。
	//   "生化环材" → 五大类全部
	//   "化学"     → 化学 + 化工
	//   "生物"     → 生物科学类
	//   "材料"     → 材料类
	//   "环境"     → 环境科学与工程类
	// 单关键字粒度匹配后只屏蔽用户真正点名的方向。
	keywordToCats := map[string][]string{
		"生化环材": {chsiCategoryChemistry, chsiCategoryChemicalEng, chsiCategoryMaterials, chsiCategoryEnvironmental, chsiCategoryBio},
		"化学":   {chsiCategoryChemistry, chsiCategoryChemicalEng},
		"生物":   {chsiCategoryBio},
		"材料":   {chsiCategoryMaterials},
		"环境":   {chsiCategoryEnvironmental},
	}
	zhangAvoid := map[string]struct{}{}
	for _, kw := range req.ExcludedKeywords {
		for trigger, cats := range keywordToCats {
			if strings.Contains(kw, trigger) {
				for _, c := range cats {
					zhangAvoid[c] = struct{}{}
				}
			}
		}
	}

	out := candidates[:0]
	for i := range candidates {
		c := &candidates[i]
		cat := firstNonEmpty(c.TagCategoryCodes)

		// 能力门槛
		excludedHere := false
		if rules, ok := excludeRules[cat]; ok {
			for subject, threshold := range rules {
				if score, present := subjectScore(subject); present && score < threshold {
					excludedHere = true
					if !noteSubjects[subject] {
						notes = append(notes, fmt.Sprintf("已根据%s分数排除强依赖专业", subjectChineseName(subject)))
						noteSubjects[subject] = true
					}
					break
				}
			}
		}
		if excludedHere {
			continue
		}
		if _, ok := zhangAvoid[cat]; ok {
			continue
		}
		if hasExcludedKeyword(c, req.ExcludedMajors) {
			continue
		}
		if hasExcludedKeyword(c, req.ExcludedKeywords) {
			continue
		}
		out = append(out, *c)
	}
	return out, notes
}

func subjectChineseName(subject string) string {
	switch subject {
	case "physics":
		return "物理"
	case "math":
		return "数学"
	case "chinese":
		return "语文"
	case "english":
		return "英语"
	default:
		return subject
	}
}

// scoredItem is the in-memory pair used during sorting.
type scoredItem struct {
	item RecommendationItem
}

func scoreCandidates(candidates []RecommendationCandidate, req *RecommendationRequest, md *RecommendationMetadata, planLabel, strategy string) []scoredItem {
	scored := make([]scoredItem, 0, len(candidates))
	for i := range candidates {
		c := &candidates[i]
		bd := scoreBreakdown(c, req, md)
		composite := bd.CityScore * bd.SchoolScore * bd.MajorScore * bd.AbilityImprovementScore * bd.FutureCompetitivenessScore
		// 学校优先时把学校权重再放大；专业优先时放大专业 & 未来竞争力
		if strategy == "school" {
			composite *= math.Pow(bd.SchoolScore, 0.5)
		} else {
			composite *= math.Pow(bd.MajorScore*bd.FutureCompetitivenessScore, 0.25)
		}

		item := RecommendationItem{
			CompositeScore:     round(composite, 4),
			ScoreBreakdown:     bd,
			Reason:             buildReason(c, req, md, &bd, planLabel),
			Probability:        estimateProbability(c, req, planLabel),
			UniversityID:       c.UniversityID,
			UniversityCode:     c.UniversityCode,
			UniversityName:     c.UniversityName,
			City:               c.City,
			ProvinceCode:       c.ProvinceCode,
			Is985:              c.Is985,
			Is211:              c.Is211,
			IsDoubleClass:      c.IsDoubleClass,
			SoftRank:           c.SoftRank,
			AdmissionGroupID:   c.AdmissionGroupID,
			GroupCode:          c.GroupCode,
			BatchCode:          c.BatchCode,
			LocalMajorCode:     c.LocalMajorCode,
			LocalMajorName:     c.LocalMajorName,
			DisciplineCategory: c.DisciplineCategory,
			MajorRank:          c.MajorRank,
			IsNationalFeature:  c.IsNationalFeature,
			HistoricalMinScore: c.MinScore,
			HistoricalMinRank:  c.MinRank,
			EquivalentMinScore: c.EquivalentMinScore,
			PlanCount:          c.PlanCount,
			Tuition:            c.Tuition,
			Warnings:           buildWarnings(c, req, md),
		}
		scored = append(scored, scoredItem{item: item})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].item.CompositeScore > scored[j].item.CompositeScore
	})
	return scored
}

// personalizationClampMin / personalizationClampMax bound each runtime personalization
// modifier before it multiplies the base. Without this, stacking 4 distinct ×1.2 bonuses
// (preferred city + preferred major + family resource + career plan) compounds to ~2.07x
// and a single ×1.2 bonus on a 985 could rank it above 清北 (school_base 2.0 vs 1.5).
// Clamping to [0.7, 1.3] caps each dimension's runtime effect at ±30%, keeping the base
// (which reflects objective tier / precomputed quality) as the dominant signal.
const (
	personalizationClampMin = 0.7
	personalizationClampMax = 1.3
)

// scoreBreakdown computes the five sub-scores.
//
// Each dimension is `precomputed_base × clamp(personalization_modifier)`:
//   - Base lives in `recommendation_precomputed_scores` (city/school/major/
//     ability_improvement/future_competitiveness). When NULL, falls back to
//     the legacy on-the-fly formula so partial population is fine.
//   - Personalization is the runtime overlay: preferred cities, preferred
//     majors, family resources, holland, career plans, single-subject ability fit.
//     Clamped to [0.7, 1.3] per dimension to keep the runtime signal from
//     overwhelming the objective base.
func scoreBreakdown(c *RecommendationCandidate, req *RecommendationRequest, md *RecommendationMetadata) ScoreBreakdown {
	cityBase := defaultFloatPtr(c.PrecomputedCityScore, fallbackCityBase(c, md))
	schoolBase := defaultFloatPtr(c.PrecomputedSchoolScore, fallbackSchoolBase(c))
	majorBase := defaultFloatPtr(c.PrecomputedMajorScore, fallbackMajorBase(c))
	abilityBase := defaultFloatPtr(c.PrecomputedAbilityImprovementScore, 1.0)
	futureBase := defaultFloatPtr(c.PrecomputedFutureCompetitivenessScore, fallbackFutureBase(c))

	city := cityBase * clampPersonalization(cityPersonalization(c, req, md))
	school := schoolBase * clampPersonalization(schoolPersonalization(c))
	major := majorBase * clampPersonalization(majorPersonalization(c, req, md))
	ability := abilityBase * clampPersonalization(abilityFitScore(c, req, md))
	future := futureBase * clampPersonalization(futurePersonalization(c))

	return ScoreBreakdown{
		CityScore:                  round(city, 3),
		SchoolScore:                round(school, 3),
		MajorScore:                 round(major, 3),
		AbilityImprovementScore:    round(ability, 3),
		FutureCompetitivenessScore: round(future, 3),

		CityBase:                  round(cityBase, 3),
		SchoolBase:                round(schoolBase, 3),
		MajorBase:                 round(majorBase, 3),
		AbilityImprovementBase:    round(abilityBase, 3),
		FutureCompetitivenessBase: round(futureBase, 3),

		CityReason:                  c.PrecomputedCityReason,
		SchoolReason:                c.PrecomputedSchoolReason,
		MajorReason:                 c.PrecomputedMajorReason,
		AbilityImprovementReason:    c.PrecomputedAbilityImprovementReason,
		FutureCompetitivenessReason: c.PrecomputedFutureCompetitivenessReason,

		EvaluatedBy:    c.PrecomputedEvaluatedBy,
		EvaluatorModel: c.PrecomputedEvaluatorModel,
	}
}

// --- runtime personalization modifiers (multipliers on top of the base) ---

func cityPersonalization(c *RecommendationCandidate, req *RecommendationRequest, _ *RecommendationMetadata) float64 {
	mod := 1.0
	if containsString(req.PreferredCities, c.City) {
		mod *= 1.4
	}
	if containsString(req.PreferredProvinces, c.ProvinceCode) {
		mod *= 1.2
	}
	return mod
}

func schoolPersonalization(c *RecommendationCandidate) float64 {
	// 软科排名是 string（"100" / "100强" / "全国第 100 名"等），
	// 取首段数字尝试转 int；失败就返回 1.0。
	if c.SoftRank == nil || *c.SoftRank == "" {
		return 1.0
	}
	if rank, ok := parseLeadingInt(*c.SoftRank); ok && rank > 0 && rank <= 100 {
		return 1.05
	}
	return 1.0
}

func parseLeadingInt(s string) (int, bool) {
	out := 0
	matched := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			out = out*10 + int(r-'0')
			matched = true
		} else if matched {
			break
		}
	}
	return out, matched
}

func majorPersonalization(c *RecommendationCandidate, req *RecommendationRequest, md *RecommendationMetadata) float64 {
	mod := 1.0
	if matchesPreferredMajors(c, req.PreferredMajors) {
		mod *= 1.25
	}
	if w := familyResourceWeight(c, req.FamilyResources, md); w > 1.0 {
		mod *= w
	}
	if w := hollandWeight(c, req.HollandCode, md); w > 1.0 {
		mod *= w
	}
	if matchesCareerPlan(c, req.CareerPlans) {
		mod *= 1.15
	}
	return mod
}

func futurePersonalization(_ *RecommendationCandidate) float64 {
	// 未来竞争力是专业固有属性，没有 per-student 修饰
	return 1.0
}

// --- fallbacks used when no precomputed_score row exists ---

func fallbackCityBase(c *RecommendationCandidate, md *RecommendationMetadata) float64 {
	if _, ok := md.CityToGroupCode[c.City]; ok {
		return 1.1
	}
	return 1.0
}

func fallbackSchoolBase(c *RecommendationCandidate) float64 {
	return schoolScoreForCandidate(c)
}

func fallbackMajorBase(c *RecommendationCandidate) float64 {
	base := 1.0
	if c.IsNationalFeature != nil && *c.IsNationalFeature {
		base *= 1.2
	}
	if isStrongEval(c.SoftMajorGrade) {
		base *= 1.15
	}
	if c.MajorEvaluationScore != nil {
		base *= 1.0 + math.Min(*c.MajorEvaluationScore/200.0, 0.25)
	}
	return base
}

func fallbackFutureBase(c *RecommendationCandidate) float64 {
	base := 1.0
	if c.PostgraduateRecommendationRate != nil && *c.PostgraduateRecommendationRate > 0 {
		base *= 1.0 + math.Min(*c.PostgraduateRecommendationRate/50.0, 0.3)
	}
	switch {
	case strings.Contains(c.DisciplineCategory, "工学"):
		base *= 1.1
	case strings.Contains(c.DisciplineCategory, "医学"):
		base *= 1.05
	case strings.Contains(c.DisciplineCategory, "法学"):
		base *= 1.05
	case strings.Contains(c.DisciplineCategory, "管理学"):
		base *= 1.0
	case strings.Contains(c.DisciplineCategory, "文学"):
		base *= 0.95
	case strings.Contains(c.DisciplineCategory, "农学"):
		base *= 0.9
	}
	return base
}

func defaultFloatPtr(p *float64, fallback float64) float64 {
	if p == nil {
		return fallback
	}
	return *p
}

// clampPersonalization keeps any single dimension's runtime modifier within
// [personalizationClampMin, personalizationClampMax]. See personalizationClampMin
// for the rationale.
func clampPersonalization(v float64) float64 {
	if v < personalizationClampMin {
		return personalizationClampMin
	}
	if v > personalizationClampMax {
		return personalizationClampMax
	}
	return v
}

// estimateProbability is a lightweight heuristic. With more data we'd train a logistic
// model; for now we use the gap between historical_min_rank and the student's rank.
// The planLabel argument is kept for future use (e.g. plan-specific risk modeling).
func estimateProbability(c *RecommendationCandidate, req *RecommendationRequest, _ string) float64 {
	if c.MinRank == nil {
		return 0.5
	}
	gap := *c.MinRank - req.ProvincialRank
	switch {
	case gap >= 4000:
		return 0.95
	case gap >= 2000:
		return 0.85
	case gap >= 500:
		return 0.7
	case gap >= 0:
		return 0.55
	case gap >= -1000:
		return 0.4
	case gap >= -2000:
		return 0.25
	default:
		return 0.1
	}
	// 注：tier 仅用于解释性，概率纯粹由 rank 差决定
}

func buildReason(c *RecommendationCandidate, req *RecommendationRequest, md *RecommendationMetadata, bd *ScoreBreakdown, _ string) string {
	parts := []string{}
	// 该 item 属于哪套方案由外层 plan 决定；reason 这里不再重复"冲/稳/保"，
	// 由 plan 自身的 description 表达整体取向。
	switch tierForCandidate(c) {
	case "top_2":
		parts = append(parts, "清北")
	case "hua_5":
		parts = append(parts, "华5")
	case "c9":
		parts = append(parts, "C9")
	case "985_other":
		parts = append(parts, "985 院校")
	case "211_double":
		parts = append(parts, "211 / 双一流")
	}
	if matchesPreferredMajors(c, req.PreferredMajors) {
		parts = append(parts, "命中你的专业偏好")
	}
	if familyResourceWeight(c, req.FamilyResources, md) > 1.0 {
		parts = append(parts, "与家庭资源方向匹配")
	}
	if c.IsNationalFeature != nil && *c.IsNationalFeature {
		parts = append(parts, "国家特色专业")
	}
	if bd.CityScore > 1.1 {
		if code, ok := md.CityToGroupCode[c.City]; ok {
			parts = append(parts, md.GroupCodeToName[code])
		} else {
			parts = append(parts, "在你的偏好城市/省份")
		}
	}
	return strings.Join(parts, " · ")
}

func buildWarnings(c *RecommendationCandidate, req *RecommendationRequest, md *RecommendationMetadata) []string {
	w := []string{}
	if req.PhysicsScore != nil {
		if rule, ok := hasAbilityRuleForSubject(c, "physics", md); ok && *req.PhysicsScore < rule.WarnBelowScore {
			w = append(w, fmt.Sprintf("物理 %d 分偏弱（建议 %d 分以上），%s", *req.PhysicsScore, rule.WarnBelowScore, rule.Note))
		}
	}
	if req.MathScore != nil {
		if rule, ok := hasAbilityRuleForSubject(c, "math", md); ok && *req.MathScore < rule.WarnBelowScore {
			w = append(w, fmt.Sprintf("数学 %d 分偏弱（建议 %d 分以上），%s", *req.MathScore, rule.WarnBelowScore, rule.Note))
		}
	}
	if isBioChemEnvMat(c) {
		w = append(w, "生化环材类，本科直接就业难，需做好读研准备")
	}
	if c.PlanCount != nil && *c.PlanCount <= 2 {
		w = append(w, "本省招生计划很少，录取分数波动大")
	}
	return w
}

func ensureSafeQuality(items []RecommendationItem, _ *RecommendationRequest) []RecommendationItem {
	out := make([]RecommendationItem, 0, len(items))
	for i := range items {
		// 保底要求招生计划 ≥ 5（"招生人数多"），缺数据则放过
		if items[i].PlanCount != nil && *items[i].PlanCount > 0 && *items[i].PlanCount < 5 {
			continue
		}
		out = append(out, items[i])
	}
	return out
}

// buildVolunteerPlan converts the merged 40-条 recommendation list into the VolunteerPlan
// shape the legacy frontend table can render. label 用作 VolunteerPlan ID/Name 后缀。
func buildVolunteerPlan(items []RecommendationItem, planLabel string) *VolunteerPlan {
	if len(items) == 0 {
		return nil
	}
	cols := []string{
		"志愿顺序", "录取概率",
		"院校代码", "院校名称", "城市",
		"专业组代号", "专业代号", "专业名称",
		"招生计划", "历史最低分", "历史最低位次",
		"推荐理由",
	}
	rows := make([]map[string]interface{}, 0, len(items))
	for i := range items {
		it := &items[i]
		row := map[string]interface{}{
			"志愿顺序":  fmt.Sprintf("%02d", it.Order),
			"录取概率":  fmt.Sprintf("%.0f%%", it.Probability*100),
			"院校代码":  it.UniversityCode,
			"院校名称":  it.UniversityName,
			"城市":    it.City,
			"专业组代号": it.GroupCode,
			"专业代号":  it.LocalMajorCode,
			"专业名称":  it.LocalMajorName,
			"推荐理由":  it.Reason,
		}
		if it.PlanCount != nil {
			row["招生计划"] = *it.PlanCount
		}
		if it.HistoricalMinScore != nil {
			row["历史最低分"] = *it.HistoricalMinScore
		}
		if it.HistoricalMinRank != nil {
			row["历史最低位次"] = *it.HistoricalMinRank
		}
		rows = append(rows, row)
	}
	return &VolunteerPlan{
		ID:          "recommended-" + planLabel,
		Name:        "智能推荐志愿表（" + planLabel + "）",
		Description: "由推荐算法基于你的分数、位次、偏好生成",
		Columns:     cols,
		Rows:        rows,
		Stats: VolunteerPlanStats{
			SchoolCount: countDistinct(items, func(i RecommendationItem) string { return i.UniversityCode }),
			GroupCount:  countDistinct(items, func(i RecommendationItem) string { return i.UniversityCode + "|" + i.GroupCode }),
			RecordCount: len(items),
		},
	}
}

// ---- helpers ----

func containsString(list []string, target string) bool {
	if target == "" {
		return false
	}
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func containsAny(list []string, needles ...string) bool {
	for _, v := range list {
		for _, n := range needles {
			if strings.Contains(v, n) {
				return true
			}
		}
	}
	return false
}

func firstNonEmpty(csv string) string {
	if csv == "" {
		return ""
	}
	if i := strings.Index(csv, ","); i >= 0 {
		return csv[:i]
	}
	return csv
}

func hasExcludedKeyword(c *RecommendationCandidate, keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}
	hay := strings.ToLower(c.LocalMajorName + " " + c.DisciplineCategory + " " + c.FirstLevelDiscipline + " " + c.TagNames)
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(hay, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func matchesPreferredMajors(c *RecommendationCandidate, prefs []string) bool {
	if len(prefs) == 0 {
		return false
	}
	hay := strings.ToLower(c.LocalMajorName + " " + c.DisciplineCategory + " " + c.FirstLevelDiscipline + " " + c.TagNames)
	for _, p := range prefs {
		if p == "" {
			continue
		}
		if strings.Contains(hay, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// familyResourceWeight returns a multiplicative bonus driven by the DB-backed
// `recommendation_family_resource_keywords` table. Returns 1.0 if no match.
func familyResourceWeight(c *RecommendationCandidate, resources []string, md *RecommendationMetadata) float64 {
	if len(resources) == 0 || len(md.FamilyResourceKeywords) == 0 {
		return 1.0
	}
	hay := c.LocalMajorName + " " + c.DisciplineCategory + " " + c.FirstLevelDiscipline + " " + c.TagNames
	best := 1.0
	for _, r := range resources {
		for _, kw := range md.FamilyResourceKeywords[r] {
			if kw.Keyword == "" {
				continue
			}
			if strings.Contains(hay, kw.Keyword) && kw.Weight > best {
				best = kw.Weight
			}
		}
	}
	return best
}

// hollandWeight returns a multiplicative bonus from the DB-backed RIASEC mapping.
// 仅做正向加分，不做硬过滤。
func hollandWeight(c *RecommendationCandidate, code string, md *RecommendationMetadata) float64 {
	if code == "" || len(md.HollandKeywords) == 0 {
		return 1.0
	}
	disc := c.DisciplineCategory + " " + c.LocalMajorName
	best := 1.0
	for _, ch := range strings.ToUpper(code) {
		for _, kw := range md.HollandKeywords[string(ch)] {
			if kw.Keyword == "" {
				continue
			}
			if strings.Contains(disc, kw.Keyword) && kw.Weight > best {
				best = kw.Weight
			}
		}
	}
	return best
}

func matchesCareerPlan(c *RecommendationCandidate, plans []string) bool {
	if len(plans) == 0 {
		return false
	}
	hay := c.LocalMajorName + " " + c.DisciplineCategory + " " + c.FirstLevelDiscipline
	for _, p := range plans {
		switch p {
		case "考公":
			if strings.Contains(hay, "法学") || strings.Contains(hay, "会计") || strings.Contains(hay, "财务") ||
				strings.Contains(hay, "汉语言") || strings.Contains(hay, "审计") {
				return true
			}
		case "从医":
			if strings.Contains(hay, "医学") {
				return true
			}
		case "电网":
			if strings.Contains(hay, "电气") || strings.Contains(hay, "能源") {
				return true
			}
		case "考研", "深造":
			if c.PostgraduateRecommendationRate != nil && *c.PostgraduateRecommendationRate >= 15 {
				return true
			}
		case "留学":
			if c.Is985 || c.Is211 {
				return true
			}
		}
	}
	return false
}

// hasAbilityRuleForSubject reports whether the candidate's CHSI category has
// any DB-backed ability rule for the given subject. Used by warning generation
// and the abilityFitScore weighting.
func hasAbilityRuleForSubject(c *RecommendationCandidate, subject string, md *RecommendationMetadata) (AbilityRule, bool) {
	cat := firstNonEmpty(c.TagCategoryCodes)
	if cat == "" {
		return AbilityRule{}, false
	}
	for _, r := range md.AbilityRules[cat] {
		if r.Subject == subject {
			return r, true
		}
	}
	return AbilityRule{}, false
}

func isBioChemEnvMat(c *RecommendationCandidate) bool {
	cats := c.TagCategoryCodes
	return strings.Contains(cats, chsiCategoryChemistry) ||
		strings.Contains(cats, chsiCategoryChemicalEng) ||
		strings.Contains(cats, chsiCategoryMaterials) ||
		strings.Contains(cats, chsiCategoryEnvironmental) ||
		strings.Contains(cats, chsiCategoryBio)
}

// --- 学校档次判断 (data-driven via university_profiles.university_tier) ---

// tierForCandidate reads university_profiles.university_tier first; if empty,
// falls back to is_985 / is_211 / is_double_first_class / is_national_key flags.
func tierForCandidate(c *RecommendationCandidate) string {
	if c.UniversityTier != "" {
		return c.UniversityTier
	}
	switch {
	case c.Is985:
		return "985_other"
	case c.Is211 || c.IsDoubleClass:
		return "211_double"
	case c.IsNationalKey:
		return "key"
	default:
		return "regular"
	}
}

// schoolScoreForCandidate maps a tier code to its multiplicative weight.
// Tier values come from university_profiles.university_tier (top_2 / hua_5 / c9 / 985_other / 211_double / key / regular / private / vocational).
func schoolScoreForCandidate(c *RecommendationCandidate) float64 {
	switch tierForCandidate(c) {
	case "top_2":
		return 2.0
	case "hua_5":
		return 1.8
	case "c9":
		return 1.7
	case "985_other":
		return 1.5
	case "211_double":
		return 1.3
	case "key":
		return 1.15
	case "private":
		return 0.85
	case "vocational":
		return 0.7
	default: // "regular" or unknown
		return 1.0
	}
}

// abilityFitScore boosts/penalizes based on the gap between the student's
// single-subject score and the DB-backed ability rule for the candidate's CHSI category.
func abilityFitScore(c *RecommendationCandidate, req *RecommendationRequest, md *RecommendationMetadata) float64 {
	fit := 1.0
	for _, subject := range []string{"physics", "math"} {
		rule, ok := hasAbilityRuleForSubject(c, subject, md)
		if !ok {
			continue
		}
		var score *int
		switch subject {
		case "physics":
			score = req.PhysicsScore
		case "math":
			score = req.MathScore
		}
		if score == nil {
			continue
		}
		switch {
		case *score >= rule.WarnBelowScore+30:
			fit *= 1.2
		case *score < rule.WarnBelowScore:
			fit *= 0.8
		}
	}
	return fit
}

func isStrongEval(grade string) bool {
	g := strings.TrimSpace(grade)
	return g == "A+" || g == "A" || g == "A-"
}

func round(v float64, digits int) float64 {
	p := math.Pow10(digits)
	return math.Round(v*p) / p
}

func countDistinct[T any](items []T, key func(T) string) int {
	seen := map[string]struct{}{}
	for _, it := range items {
		seen[key(it)] = struct{}{}
	}
	return len(seen)
}
