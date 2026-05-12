package admission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// CHSI category codes used by tests.
const (
	testCatElectronic = "0807" // 电子信息类
	testCatChinese    = "0501" // 中国语言文学类
	testCatCS         = "0809" // 计算机类
)

// fixtureMetadata mirrors the seed data the migration installs in
// city_groups / recommendation_*_keywords / recommendation_major_ability_rules.
// Keeps the unit tests independent of the database while still exercising the
// metadata-driven code paths.
func fixtureMetadata() *RecommendationMetadata {
	return &RecommendationMetadata{
		CityToGroupCode: map[string]string{
			"北京": "jjj", "天津": "jjj", "雄安": "jjj",
			"上海": "yrd", "南京": "yrd", "杭州": "yrd",
			"广州": "gba", "深圳": "gba",
			"成都": "cd", "重庆": "cd",
			"西安":  "gz",
			"哈尔滨": "ne", "长春": "ne", "沈阳": "ne", "大连": "ne",
		},
		GroupCodeToName: map[string]string{
			"jjj": "京津冀城市群", "yrd": "长三角城市群", "gba": "粤港澳大湾区",
			"cd": "成渝城市群", "gz": "关中平原城市群", "ne": "东北城市群",
		},
		FamilyResourceKeywords: map[string][]KeywordWeight{
			"公检法": {{Keyword: "法学", Weight: 1.4}, {Keyword: "法律", Weight: 1.3}},
			"金融":  {{Keyword: "金融", Weight: 1.3}, {Keyword: "经济", Weight: 1.2}},
			"医疗":  {{Keyword: "临床", Weight: 1.4}, {Keyword: "医学", Weight: 1.2}},
			"教育":  {{Keyword: "教育", Weight: 1.3}, {Keyword: "师范", Weight: 1.3}},
			"电网":  {{Keyword: "电气", Weight: 1.4}, {Keyword: "能动", Weight: 1.2}},
			"商业":  {{Keyword: "工商管理", Weight: 1.2}, {Keyword: "会计", Weight: 1.3}},
		},
		HollandKeywords: map[string][]KeywordWeight{
			"R": {{Keyword: "工学", Weight: 1.2}},
			"I": {{Keyword: "理学", Weight: 1.2}, {Keyword: "医学", Weight: 1.1}},
			"S": {{Keyword: "教育", Weight: 1.2}},
			"E": {{Keyword: "金融", Weight: 1.2}, {Keyword: "管理", Weight: 1.2}},
		},
		AbilityRules: map[string][]AbilityRule{
			testCatElectronic: {
				{ChsiCategoryCode: testCatElectronic, Subject: "physics", ExcludeBelowScore: 40, WarnBelowScore: 50, Note: "电子信息类大学物理强度高"},
			},
			testCatCS: {
				{ChsiCategoryCode: testCatCS, Subject: "math", ExcludeBelowScore: 50, WarnBelowScore: 70, Note: "计算机类大学数学强度高"},
			},
		},
	}
}

type stubMetadataStore struct{ md *RecommendationMetadata }

func (s *stubMetadataStore) Load(_ context.Context) (*RecommendationMetadata, error) {
	return s.md, nil
}

func newStubMetadataStore() *stubMetadataStore {
	return &stubMetadataStore{md: fixtureMetadata()}
}

func TestClampBucketWindow(t *testing.T) {
	w := clampBucketWindow(8000, 10000)
	require.Equal(t, 8000, w.Min)
	require.Equal(t, 10000, w.Max)
}

func TestClampBucketWindowClampsAtOne(t *testing.T) {
	w := clampBucketWindow(-2000, 500)
	require.Equal(t, 1, w.Min)
	require.Equal(t, 500, w.Max)
}

func TestSplitTierQuota(t *testing.T) {
	rush, match, safe := splitTierQuota(40)
	require.Equal(t, 10, rush)
	require.Equal(t, 20, match)
	require.Equal(t, 10, safe)
	require.Equal(t, 40, rush+match+safe)

	rush, match, safe = splitTierQuota(10)
	require.Equal(t, 10, rush+match+safe, "余数应全部进 match")
	require.Equal(t, 2, rush)
	require.Equal(t, 6, match)
	require.Equal(t, 2, safe)
}

func TestDecideStrategyExplicit(t *testing.T) {
	s, _ := decideStrategy(&RecommendationRequest{PriorityStrategy: "school"})
	require.Equal(t, "school", s)
	s, _ = decideStrategy(&RecommendationRequest{PriorityStrategy: "major"})
	require.Equal(t, "major", s)
}

func TestDecideStrategyByMajorIntent(t *testing.T) {
	s, _ := decideStrategy(&RecommendationRequest{PriorityStrategy: "auto", PreferredMajors: []string{"计算机科学与技术"}})
	require.Equal(t, "major", s, "STEM 偏好应回落到专业优先")

	s, _ = decideStrategy(&RecommendationRequest{PriorityStrategy: "auto", PreferredMajors: []string{"汉语言文学"}})
	require.Equal(t, "school", s, "纯文管偏好应走学校优先")
}

func TestFilterByPreferenceAbilityGate(t *testing.T) {
	candidates := []RecommendationCandidate{
		{LocalMajorName: "电子信息工程", TagCategoryCodes: testCatElectronic},
		{LocalMajorName: "汉语言文学", TagCategoryCodes: testCatChinese},
		{LocalMajorName: "计算机科学", TagCategoryCodes: testCatCS},
	}
	out, notes := filterByPreference(candidates, &RecommendationRequest{
		PhysicsScore: intPtr(30),
		MathScore:    intPtr(40),
	}, fixtureMetadata())
	require.Len(t, out, 1, "物理 30 + 数学 40 应只剩文科")
	require.Equal(t, "汉语言文学", out[0].LocalMajorName)
	require.NotEmpty(t, notes)
}

func TestFilterByPreferenceSubjectiveExcludes(t *testing.T) {
	candidates := []RecommendationCandidate{
		{LocalMajorName: "材料科学与工程", DisciplineCategory: "工学"},
		{LocalMajorName: "土木工程", DisciplineCategory: "工学"},
	}
	out, _ := filterByPreference(candidates, &RecommendationRequest{
		ExcludedKeywords: []string{"材料"},
	}, fixtureMetadata())
	require.Len(t, out, 1)
	require.Equal(t, "土木工程", out[0].LocalMajorName)
}

func TestPickBucketFiltersByWindowAndTagsTier(t *testing.T) {
	cands := []RecommendationCandidate{
		{UniversityCode: "U1", GroupCode: "01", LocalMajorName: "in-window-1", MinRank: intPtr(8000)},
		{UniversityCode: "U2", GroupCode: "02", LocalMajorName: "in-window-2", MinRank: intPtr(9000)},
		{UniversityCode: "U3", GroupCode: "03", LocalMajorName: "out-of-window", MinRank: intPtr(50000)},
		{UniversityCode: "U4", GroupCode: "04", LocalMajorName: "missing-rank"},
	}
	seen := map[string]struct{}{}
	used := map[string]int{}
	items := pickBucket(cands, &RecommendationRequest{PlanSize: 40}, fixtureMetadata(),
		"major", "rush", bucketWindow{Min: 7000, Max: 10000}, 10, seen, used)
	require.Len(t, items, 2)
	for _, it := range items {
		require.Equal(t, "rush", it.Tier)
	}
}

func TestPickBucketSharedDedupeAcrossTiers(t *testing.T) {
	// 同一院校专业组在 rush 和 match 都符合窗口，但跨档共享 seenGroups 保证它只进表一次。
	cands := []RecommendationCandidate{
		{UniversityCode: "U1", GroupCode: "01", LocalMajorName: "shared", MinRank: intPtr(9000)},
	}
	seen := map[string]struct{}{}
	used := map[string]int{}
	rush := pickBucket(cands, &RecommendationRequest{PlanSize: 40}, fixtureMetadata(),
		"major", "rush", bucketWindow{Min: 8000, Max: 9500}, 5, seen, used)
	match := pickBucket(cands, &RecommendationRequest{PlanSize: 40}, fixtureMetadata(),
		"major", "match", bucketWindow{Min: 8500, Max: 9500}, 5, seen, used)
	require.Len(t, rush, 1)
	require.Len(t, match, 0, "重复组应被跨档去重")
}

func TestEstimateProbability(t *testing.T) {
	c := RecommendationCandidate{MinRank: intPtr(15000)}
	req := &RecommendationRequest{ProvincialRank: 10000}
	require.InDelta(t, 0.95, estimateProbability(&c, req, "safe"), 0.001)

	c2 := RecommendationCandidate{MinRank: intPtr(8500)}
	require.InDelta(t, 0.25, estimateProbability(&c2, req, "rush"), 0.001)
}

func TestScoreBreakdownFamilyResourceBoost(t *testing.T) {
	c := RecommendationCandidate{
		LocalMajorName:     "法学",
		DisciplineCategory: "法学",
	}
	md := fixtureMetadata()
	with := scoreBreakdown(&c, &RecommendationRequest{FamilyResources: []string{"公检法"}}, md)
	without := scoreBreakdown(&c, &RecommendationRequest{}, md)
	require.Greater(t, with.MajorScore, without.MajorScore, "公检法资源应让法学专业得分更高")
}

func TestScoreBreakdownTierFromDB(t *testing.T) {
	cTopTwo := RecommendationCandidate{UniversityName: "清华大学", UniversityTier: "top_2"}
	cHua5 := RecommendationCandidate{UniversityName: "复旦大学", UniversityTier: "hua_5"}
	c985 := RecommendationCandidate{UniversityName: "X大学", UniversityTier: "985_other"}
	cFallback := RecommendationCandidate{UniversityName: "Y大学", Is211: true} // 无 tier，走 fallback
	md := fixtureMetadata()

	require.Equal(t, 2.0, scoreBreakdown(&cTopTwo, &RecommendationRequest{}, md).SchoolScore)
	require.Equal(t, 1.8, scoreBreakdown(&cHua5, &RecommendationRequest{}, md).SchoolScore)
	require.Equal(t, 1.5, scoreBreakdown(&c985, &RecommendationRequest{}, md).SchoolScore)
	require.Equal(t, 1.3, scoreBreakdown(&cFallback, &RecommendationRequest{}, md).SchoolScore, "fallback: is_211 → 1.3")
}

func float64Ptr(v float64) *float64 { return &v }

func TestScoreBreakdownPrecomputedOverridesFallback(t *testing.T) {
	// 即便候选是 985，只要 PrecomputedSchoolScore=0.5，最终 school_score 应取这个 0.5（× personalization 1.0）
	c := RecommendationCandidate{
		UniversityName:                         "X大学",
		Is985:                                  true,
		PrecomputedSchoolScore:                 float64Ptr(0.5),
		PrecomputedFutureCompetitivenessScore:  float64Ptr(1.7),
		PrecomputedFutureCompetitivenessReason: "AI 时代核心赛道",
		PrecomputedEvaluatedBy:                 "llm",
		PrecomputedEvaluatorModel:              "claude-opus-4-7",
	}
	bd := scoreBreakdown(&c, &RecommendationRequest{}, fixtureMetadata())
	require.Equal(t, 0.5, bd.SchoolScore, "precomputed 0.5 应直接覆盖 fallback 的 1.5（is_985）")
	require.Equal(t, 0.5, bd.SchoolBase)
	require.Equal(t, 1.7, bd.FutureCompetitivenessScore)
	require.Equal(t, "AI 时代核心赛道", bd.FutureCompetitivenessReason)
	require.Equal(t, "llm", bd.EvaluatedBy)
	require.Equal(t, "claude-opus-4-7", bd.EvaluatorModel)
}

func TestScoreBreakdownPersonalizationStacksOnPrecomputedBase(t *testing.T) {
	c := RecommendationCandidate{
		City:                  "上海",
		PrecomputedCityScore:  float64Ptr(1.2), // base from DB
		LocalMajorName:        "金融学",
		PrecomputedMajorScore: float64Ptr(1.3),
	}
	md := fixtureMetadata()
	req := &RecommendationRequest{
		PreferredCities: []string{"上海"}, // 偏好命中 → city personalization ×1.4
		FamilyResources: []string{"金融"}, // 命中关键词 "金融" weight 1.3 → major personalization ×1.3
	}
	bd := scoreBreakdown(&c, req, md)
	require.InDelta(t, 1.2*1.4, bd.CityScore, 0.001, "1.2 base × 1.4 personalization")
	require.InDelta(t, 1.3*1.3, bd.MajorScore, 0.001, "1.3 base × 1.3 family-resource personalization")
}

func TestScoreBreakdownCityGroupBoost(t *testing.T) {
	cInGroup := RecommendationCandidate{City: "上海"}
	cNotInGroup := RecommendationCandidate{City: "齐齐哈尔"}
	md := fixtureMetadata()

	with := scoreBreakdown(&cInGroup, &RecommendationRequest{}, md)
	without := scoreBreakdown(&cNotInGroup, &RecommendationRequest{}, md)
	require.Greater(t, with.CityScore, without.CityScore, "上海属于长三角，城市分应被加权")
}

func TestEnsureSafeQualityFiltersTinyPlans(t *testing.T) {
	items := []RecommendationItem{
		{UniversityName: "A", PlanCount: intPtr(1)},
		{UniversityName: "B", PlanCount: intPtr(20)},
		{UniversityName: "C"}, // 无数据，放过
	}
	out := ensureSafeQuality(items, &RecommendationRequest{})
	require.Len(t, out, 2)
	require.Equal(t, "B", out[0].UniversityName)
	require.Equal(t, "C", out[1].UniversityName)
}

func TestPickTopKDedupesByGroupAndCapsPerSchool(t *testing.T) {
	// Dedup is now by (UniversityCode, GroupCode). Same group → only highest-scored major kept.
	// Per-school cap is maxGroupsPerSchool=4.
	scored := []scoredItem{
		// X 学校 group=01：两个专业，只保留最高分
		{item: RecommendationItem{UniversityCode: "X", GroupCode: "01", LocalMajorName: "A", CompositeScore: 5}},
		{item: RecommendationItem{UniversityCode: "X", GroupCode: "01", LocalMajorName: "B", CompositeScore: 4}},
		// X 学校 group=02：独立 slot
		{item: RecommendationItem{UniversityCode: "X", GroupCode: "02", LocalMajorName: "C", CompositeScore: 3}},
		// Y 学校
		{item: RecommendationItem{UniversityCode: "Y", GroupCode: "01", LocalMajorName: "D", CompositeScore: 2}},
	}
	out := pickTopK(scored, 4)
	require.Len(t, out, 3, "X|01 去重后 1 个 + X|02 + Y|01 = 3")
	require.Equal(t, "A", out[0].LocalMajorName, "X|01 应保留分数最高的 A")
	require.Equal(t, "C", out[1].LocalMajorName)
	require.Equal(t, "D", out[2].LocalMajorName)
}

func TestPickTopKCapsByGroupsPerSchool(t *testing.T) {
	scored := []scoredItem{}
	// 同一学校 5 个不同 group，maxGroupsPerSchool=4
	for i := 1; i <= 5; i++ {
		scored = append(scored, scoredItem{item: RecommendationItem{
			UniversityCode: "X",
			GroupCode:      string(rune('0' + i)),
			CompositeScore: float64(10 - i),
		}})
	}
	out := pickTopK(scored, 10)
	require.Len(t, out, maxGroupsPerSchool, "同一学校最多保留 maxGroupsPerSchool 个不同 group")
}

func TestBuildVolunteerPlanShape(t *testing.T) {
	items := []RecommendationItem{
		{
			Order: 1, Probability: 0.42,
			UniversityCode: "1001", UniversityName: "测试大学", City: "哈尔滨",
			GroupCode: "001", LocalMajorCode: "01", LocalMajorName: "计算机",
			Reason:    "test",
			PlanCount: intPtr(20),
		},
	}
	plan := buildVolunteerPlan(items, "稳")
	require.NotNil(t, plan)
	require.Equal(t, "recommended-稳", plan.ID)
	require.Equal(t, 1, plan.Stats.RecordCount)
	require.Len(t, plan.Rows, 1)
	require.Equal(t, "01", plan.Rows[0]["志愿顺序"])
	require.Equal(t, "42%", plan.Rows[0]["录取概率"])
}

// ---- service end-to-end with stub stores ----

type stubRecommendationStore struct {
	candidates []RecommendationCandidate
	year       int
}

func (s *stubRecommendationStore) FetchCandidates(_ context.Context, _ *CandidateQuery) ([]RecommendationCandidate, error) {
	return s.candidates, nil
}
func (s *stubRecommendationStore) LatestAdmissionYear(_ context.Context, _, _ string) (int, error) {
	return s.year, nil
}

func TestRecommendMergesTiersIntoSingleList(t *testing.T) {
	// 学生位次 10000 → 三段窗口:
	//   rushW  = [R-15000, R-2000] = [-5000, 8000] → clamp [1, 8000]
	//   matchW = [R-3000, R+3000]  = [7000, 13000]
	//   safeW  = [R+2000, R+15000] = [12000, 25000]
	stub := &stubRecommendationStore{
		year: 2024,
		candidates: []RecommendationCandidate{
			// 仅 rush 窗口
			{UniversityID: 1, UniversityCode: "U1", UniversityName: "A大学", City: "上海",
				GroupCode: "001", LocalMajorCode: "01", LocalMajorName: "计算机科学",
				MinRank: intPtr(5000), MinScore: intPtr(640), PlanCount: intPtr(10),
				Is985: true, UniversityTier: "985_other", TagCategoryCodes: testCatCS},
			// 仅 match 窗口
			{UniversityID: 2, UniversityCode: "U2", UniversityName: "B大学", City: "南京",
				GroupCode: "002", LocalMajorCode: "02", LocalMajorName: "电子信息",
				MinRank: intPtr(10000), MinScore: intPtr(600), PlanCount: intPtr(15),
				Is211: true, UniversityTier: "211_double", TagCategoryCodes: testCatElectronic},
			// 仅 safe 窗口
			{UniversityID: 3, UniversityCode: "U3", UniversityName: "C大学", City: "西安",
				GroupCode: "003", LocalMajorCode: "03", LocalMajorName: "汉语言文学",
				MinRank: intPtr(20000), MinScore: intPtr(560), PlanCount: intPtr(20),
				TagCategoryCodes: testCatChinese},
			// 全部窗口外
			{UniversityID: 4, UniversityCode: "U4", UniversityName: "D大学",
				MinRank: intPtr(80000)},
		},
	}
	svc := NewRecommendationService(stub, newStubMetadataStore(), nil)
	resp, err := svc.Recommend(context.Background(), &RecommendationRequest{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		TotalScore:          580,
		ProvincialRank:      10000,
		MathScore:           intPtr(120),
		PhysicsScore:        intPtr(85),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "major", resp.Strategy)
	require.Len(t, resp.Items, 3, "三个候选分属冲/稳/保各一条")

	// 顺序应是 rush → match → safe，order 跨档连续编号
	require.Equal(t, "rush", resp.Items[0].Tier)
	require.Equal(t, 1, resp.Items[0].Order)
	require.Equal(t, "U1", resp.Items[0].UniversityCode)

	require.Equal(t, "match", resp.Items[1].Tier)
	require.Equal(t, 2, resp.Items[1].Order)
	require.Equal(t, "U2", resp.Items[1].UniversityCode)

	require.Equal(t, "safe", resp.Items[2].Tier)
	require.Equal(t, 3, resp.Items[2].Order)
	require.Equal(t, "U3", resp.Items[2].UniversityCode)

	require.Equal(t, 1, resp.RushCount)
	require.Equal(t, 1, resp.MatchCount)
	require.Equal(t, 1, resp.SafeCount)

	// 顶层 RankWindow 含三段位次窗口
	require.Equal(t, 1, resp.RankWindow.RushMin)
	require.Equal(t, 8000, resp.RankWindow.RushMax)
	require.Equal(t, 7000, resp.RankWindow.MatchMin)
	require.Equal(t, 13000, resp.RankWindow.MatchMax)
	require.Equal(t, 12000, resp.RankWindow.SafeMin)
	require.Equal(t, 25000, resp.RankWindow.SafeMax)

	require.NotNil(t, resp.VolunteerPlan)
	require.Equal(t, 3, resp.VolunteerPlan.Stats.RecordCount)
}

func TestRecommendAbilityGateExcludesElectronic(t *testing.T) {
	// 学生位次 10000 → 候选要在 rush [5000-10000] 范围才会出现
	stub := &stubRecommendationStore{
		year: 2024,
		candidates: []RecommendationCandidate{
			{UniversityID: 1, UniversityCode: "U1", UniversityName: "A", City: "北京",
				GroupCode: "001", LocalMajorName: "电子信息工程",
				MinRank: intPtr(9500), PlanCount: intPtr(10),
				TagCategoryCodes: testCatElectronic},
			{UniversityID: 2, UniversityCode: "U2", UniversityName: "B", City: "上海",
				GroupCode: "002", LocalMajorName: "汉语言文学",
				MinRank: intPtr(9700), PlanCount: intPtr(10),
				TagCategoryCodes: testCatChinese},
		},
	}
	svc := NewRecommendationService(stub, newStubMetadataStore(), nil)
	resp, err := svc.Recommend(context.Background(), &RecommendationRequest{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		TotalScore:          550,
		ProvincialRank:      10000,
		PhysicsScore:        intPtr(30),
	})
	require.NoError(t, err)
	for _, it := range resp.Items {
		require.NotEqual(t, "电子信息工程", it.LocalMajorName,
			"物理 30 应排除电子信息（tier: %s）", it.Tier)
	}
}

func TestRecommendRushUnderflowSpillsToMatch(t *testing.T) {
	// 学生位次 1500（极高分）→ rushW = [1500-15000, 1500-2000] = clamp [1, 1]
	// rush 桶里几乎不可能有候选；剩余配额（默认 10）应转给 match。
	stub := &stubRecommendationStore{
		year: 2024,
		candidates: []RecommendationCandidate{
			// 落在 match 窗口 [R-3000, R+3000] = [1, 4500]
			{UniversityID: 1, UniversityCode: "U1", UniversityName: "A", City: "上海",
				GroupCode: "001", LocalMajorName: "M1", MinRank: intPtr(2000), PlanCount: intPtr(20)},
			{UniversityID: 2, UniversityCode: "U2", UniversityName: "B", City: "南京",
				GroupCode: "002", LocalMajorName: "M2", MinRank: intPtr(2500), PlanCount: intPtr(20)},
			{UniversityID: 3, UniversityCode: "U3", UniversityName: "C", City: "西安",
				GroupCode: "003", LocalMajorName: "M3", MinRank: intPtr(3000), PlanCount: intPtr(20)},
			{UniversityID: 4, UniversityCode: "U4", UniversityName: "D", City: "成都",
				GroupCode: "004", LocalMajorName: "M4", MinRank: intPtr(3500), PlanCount: intPtr(20)},
		},
	}
	svc := NewRecommendationService(stub, newStubMetadataStore(), nil)
	resp, err := svc.Recommend(context.Background(), &RecommendationRequest{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		TotalScore:          680,
		ProvincialRank:      1500,
	})
	require.NoError(t, err)
	require.Equal(t, 0, resp.RushCount, "极高分位次 rush 窗口被 clamp 成空")
	require.Equal(t, 4, resp.MatchCount, "rush 名额应回填到 match")
	require.Contains(t, resp.Notes[0], "冲", "应在 notes 中提示用户冲档为空")
}

func TestRecommendValidatesRequest(t *testing.T) {
	svc := NewRecommendationService(&stubRecommendationStore{year: 2024}, newStubMetadataStore(), nil)
	_, err := svc.Recommend(context.Background(), &RecommendationRequest{})
	require.Error(t, err)
}
