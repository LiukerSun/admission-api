package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildVolunteerPlanFromItems 是 plan_json 落盘流程的 schema 契约测试。
// 前端 VolunteerPlansPage 在导出 Excel / 渲染表格时读取
// plan_json.groups[i].{universityCode, universityName, groupCode, groupName,
// majors[j].{majorOrder, majorCode, majorName}} 等 camelCase 字段；
// 后端任何 refactor 误改字段名都会让导出退化成空内容（历史教训：
// 曾经把 items[] 直接当 plan_json 落库，前端只能拿到 undefined 的 groups）。
// 这条测试通过实际 JSON marshal 后再 unmarshal 到一个紧贴前端契约的
// 结构体，捕获字段名漂移、空 groups、空 majors 三类回归。
func TestBuildVolunteerPlanFromItems(t *testing.T) {
	items := []RecommendationItem{
		{
			Order:          1,
			Tier:           "rush",
			UniversityCode: "10001",
			UniversityName: "北京大学",
			GroupCode:      "01",
			LocalMajorCode: "080901",
			LocalMajorName: "计算机科学与技术",
		},
		{
			Order:          2,
			Tier:           "match",
			UniversityCode: "10002",
			UniversityName: "清华大学",
			GroupCode:      "02",
			LocalMajorCode: "080902",
			LocalMajorName: "软件工程",
		},
	}

	plan := buildVolunteerPlanFromItems(items)
	raw, err := json.Marshal(plan)
	require.NoError(t, err)

	// 用 only-camelCase 字段的极简类型反序列化，验证 marshaller 真的产出
	// 了前端期望的字段名；任何 snake_case 漂移（universityCode →
	// university_code）都会让以下字段读到零值。
	type majorView struct {
		MajorOrder int    `json:"majorOrder"`
		MajorCode  string `json:"majorCode"`
		MajorName  string `json:"majorName"`
	}
	type groupView struct {
		OrderNo          int         `json:"orderNo"`
		UniversityCode   string      `json:"universityCode"`
		UniversityName   string      `json:"universityName"`
		GroupCode        string      `json:"groupCode"`
		GroupName        string      `json:"groupName"`
		IsObeyAdjustment bool        `json:"isObeyAdjustment"`
		Remark           string      `json:"remark"`
		Majors           []majorView `json:"majors"`
	}
	type statsView struct {
		SchoolCount int `json:"schoolCount"`
		GroupCount  int `json:"groupCount"`
		RecordCount int `json:"recordCount"`
	}
	type planView struct {
		Groups []groupView `json:"groups"`
		Stats  statsView   `json:"stats"`
	}

	var got planView
	require.NoError(t, json.Unmarshal(raw, &got))

	require.Len(t, got.Groups, 2, "每个 item 应该折叠成一个 group")
	require.Equal(t, "北京大学", got.Groups[0].UniversityName)
	require.Equal(t, "10001", got.Groups[0].UniversityCode)
	require.Equal(t, "01", got.Groups[0].GroupCode)
	require.Equal(t, "冲", got.Groups[0].Remark)
	require.Len(t, got.Groups[0].Majors, 1)
	require.Equal(t, 1, got.Groups[0].Majors[0].MajorOrder)
	require.Equal(t, "计算机科学与技术", got.Groups[0].Majors[0].MajorName)
	require.Equal(t, "080901", got.Groups[0].Majors[0].MajorCode)

	require.Equal(t, 2, got.Stats.SchoolCount)
	require.Equal(t, 2, got.Stats.GroupCount)
	require.Equal(t, 2, got.Stats.RecordCount)
}

// TestBuildVolunteerPlanFromItemsEmpty 守住"候选池真的空"这种极端情况——
// 至少 groups 必须是 [] 而不是 nil，否则前端 (activePlan.plan_json?.groups ?? [])
// 兜底虽然能 work，但 JSON 里出现 "groups": null 是个明显 schema 异常信号。
func TestBuildVolunteerPlanFromItemsEmpty(t *testing.T) {
	plan := buildVolunteerPlanFromItems(nil)
	raw, err := json.Marshal(plan)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"groups":[]`)
	require.Contains(t, string(raw), `"schoolCount":0`)
}

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
		StrategyKeywords: map[string][]string{
			"stem":       {"计算机", "电子", "电气", "自动化", "机械", "通信", "软件", "人工智能", "数学", "物理", "土木", "航空", "材料"},
			"humanities": {"法学", "汉语言", "新闻", "金融", "会计", "经济", "管理", "外语", "教育", "心理"},
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
	md := fixtureMetadata()
	s, _ := decideStrategy(&RecommendationRequest{PriorityStrategy: "school"}, md)
	require.Equal(t, "school", s)
	s, _ = decideStrategy(&RecommendationRequest{PriorityStrategy: "major"}, md)
	require.Equal(t, "major", s)
}

func TestDecideStrategyByMajorIntent(t *testing.T) {
	md := fixtureMetadata()
	s, _ := decideStrategy(&RecommendationRequest{PriorityStrategy: "auto", PreferredMajors: []string{"计算机科学与技术"}}, md)
	require.Equal(t, "major", s, "STEM 偏好应回落到专业优先")

	s, _ = decideStrategy(&RecommendationRequest{PriorityStrategy: "auto", PreferredMajors: []string{"汉语言文学"}}, md)
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
	// 比例阈值（gap/R）：
	//   1.5+ → 0.95, 0.5+ → 0.85, 0.2+ → 0.7, 0+ → 0.55,
	//   -0.1+ → 0.4, -0.3+ → 0.25, else → 0.1
	req := &RecommendationRequest{ProvincialRank: 10000}
	// rank 25000 → ratio = 1.5 → 0.95
	c := RecommendationCandidate{MinRank: intPtr(25000)}
	require.InDelta(t, 0.95, estimateProbability(&c, req, "safe"), 0.001)
	// rank 8500 → ratio = -0.15 → 0.25
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
		PreferredCities: []string{"上海"}, // 偏好命中 → city personalization ×1.4 → clamp 1.3
		FamilyResources: []string{"金融"}, // 命中关键词 "金融" weight 1.3 → major personalization ×1.3
	}
	bd := scoreBreakdown(&c, req, md)
	require.InDelta(t, 1.2*personalizationClampMax, bd.CityScore, 0.001, "1.2 base × clamp(1.4)=1.3")
	require.InDelta(t, 1.3*1.3, bd.MajorScore, 0.001, "1.3 base × 1.3 family-resource personalization")
}

func TestScoreBreakdownPersonalizationClampPreventsOverstacking(t *testing.T) {
	// I1 回归：4 个独立 ×1.2 personalization 不应反超学校档差。
	// 一个 211 院校 (base 1.3) + 4 个加成 在未 clamp 时累乘到 2.07，会盖过清北 (base 2.0)。
	// 加 clamp 后，单维度最多 ×1.3，复合分由 base 主导。
	cTopTwo := RecommendationCandidate{UniversityName: "清华大学", UniversityTier: "top_2"}
	c211 := RecommendationCandidate{
		UniversityName:     "X 大学",
		UniversityTier:     "211_double",
		City:               "上海",
		LocalMajorName:     "金融学",
		DisciplineCategory: "经济学",
	}
	md := fixtureMetadata()
	plainReq := &RecommendationRequest{}
	stackedReq := &RecommendationRequest{
		PreferredCities: []string{"上海"}, // city ×1.4
		PreferredMajors: []string{"金融"}, // major ×1.25
		FamilyResources: []string{"金融"}, // major ×1.3
		HollandCode:     "E",            // major ×1.2
		CareerPlans:     []string{"考公"}, // 命中 "金融" → 实际未命中（关键词不含金融）
	}
	bdTop := scoreBreakdown(&cTopTwo, plainReq, md)
	bd211 := scoreBreakdown(&c211, stackedReq, md)
	require.Greater(t, bdTop.SchoolScore, bd211.SchoolScore,
		"clamp 后 211 院校无论怎么叠加 personalization 都不应反超 top_2 的 school 维度")
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
		{UniversityName: "A", AdmittedCount: intPtr(1)},
		{UniversityName: "B", AdmittedCount: intPtr(20)},
		{UniversityName: "C"}, // 无数据，放过
	}
	out := ensureSafeQuality(items, &RecommendationRequest{})
	require.Len(t, out, 2)
	require.Equal(t, "B", out[0].UniversityName)
	require.Equal(t, "C", out[1].UniversityName)
}

// ---- service end-to-end with stub stores ----

type stubRecommendationStore struct {
	candidates []RecommendationCandidate
	year       int
	// callCount tracks how many FetchCandidates calls were issued; helps verify
	// the service really does query each bucket window separately.
	callCount int
}

// FetchCandidates filters in-memory by RankMin/RankMax/Limit so each bucket
// query returns only the candidates that would actually come back from PG.
// Mirrors enough of the real SQL to keep service-level tests honest.
func (s *stubRecommendationStore) FetchCandidates(_ context.Context, q *CandidateQuery) ([]RecommendationCandidate, error) {
	s.callCount++
	out := make([]RecommendationCandidate, 0, len(s.candidates))
	for i := range s.candidates {
		c := &s.candidates[i]
		if c.MinRank == nil {
			continue
		}
		if q.RankMin > 0 && *c.MinRank < q.RankMin {
			continue
		}
		if q.RankMax > 0 && *c.MinRank > q.RankMax {
			continue
		}
		out = append(out, *c)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
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
			{UniversityMajorAdmissionID: 1001,
				UniversityID: 1, UniversityCode: "U1", UniversityName: "A大学", City: "上海",
				GroupCode: "001", LocalMajorCode: "01", LocalMajorName: "计算机科学",
				MinRank: intPtr(5000), MinScore: intPtr(640), AdmittedCount: intPtr(10),
				Is985: true, UniversityTier: "985_other", TagCategoryCodes: testCatCS},
			// 仅 match 窗口
			{UniversityMajorAdmissionID: 1002,
				UniversityID: 2, UniversityCode: "U2", UniversityName: "B大学", City: "南京",
				GroupCode: "002", LocalMajorCode: "02", LocalMajorName: "电子信息",
				MinRank: intPtr(10000), MinScore: intPtr(600), AdmittedCount: intPtr(15),
				Is211: true, UniversityTier: "211_double", TagCategoryCodes: testCatElectronic},
			// 仅 safe 窗口
			{UniversityMajorAdmissionID: 1003,
				UniversityID: 3, UniversityCode: "U3", UniversityName: "C大学", City: "西安",
				GroupCode: "003", LocalMajorCode: "03", LocalMajorName: "汉语言文学",
				MinRank: intPtr(20000), MinScore: intPtr(560), AdmittedCount: intPtr(20),
				TagCategoryCodes: testCatChinese},
			// 全部窗口外
			{UniversityMajorAdmissionID: 1004,
				UniversityID: 4, UniversityCode: "U4", UniversityName: "D大学",
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

	// 顶层 RankWindow 含三段位次窗口（按 R=10000 的比例：50%-85% / ±15% / 115%-200%）。
	require.Equal(t, 5000, resp.RankWindow.RushMin)
	require.Equal(t, 8500, resp.RankWindow.RushMax)
	require.Equal(t, 8500, resp.RankWindow.MatchMin)
	require.Equal(t, 11500, resp.RankWindow.MatchMax)
	require.Equal(t, 11500, resp.RankWindow.SafeMin)
	require.Equal(t, 20000, resp.RankWindow.SafeMax)

	require.NotNil(t, resp.VolunteerPlan)
}

func TestRecommendAbilityGateExcludesElectronic(t *testing.T) {
	// 学生位次 10000 → 候选要在 rush [5000-10000] 范围才会出现
	stub := &stubRecommendationStore{
		year: 2024,
		candidates: []RecommendationCandidate{
			{UniversityMajorAdmissionID: 2001,
				UniversityID: 1, UniversityCode: "U1", UniversityName: "A", City: "北京",
				GroupCode: "001", LocalMajorName: "电子信息工程",
				MinRank: intPtr(9500), AdmittedCount: intPtr(10),
				TagCategoryCodes: testCatElectronic},
			{UniversityMajorAdmissionID: 2002,
				UniversityID: 2, UniversityCode: "U2", UniversityName: "B", City: "上海",
				GroupCode: "002", LocalMajorName: "汉语言文学",
				MinRank: intPtr(9700), AdmittedCount: intPtr(10),
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

func TestRecommendRushUnderflowSpillsToLowerBuckets(t *testing.T) {
	// 学生位次 1500（高分段）→ 比例窗口：
	//   rush  = [1500*0.5, 1500*0.85] = [750, 1275]
	//   match = [1500*0.85, 1500*1.15] = [1275, 1725]
	//   safe  = [1500*1.15, 1500*2]   = [1725, 3000]
	// 候选 rank 都 ≥ 2000，落在 safe；rush/match 没候选，rush 名额回填给 match，
	// match 自己也空，再回填给 safe。验证名额单向下沉、且 notes 提示冲档为空。
	stub := &stubRecommendationStore{
		year: 2024,
		candidates: []RecommendationCandidate{
			{UniversityMajorAdmissionID: 3001,
				UniversityID: 1, UniversityCode: "U1", UniversityName: "A", City: "上海",
				GroupCode: "001", LocalMajorName: "M1", MinRank: intPtr(2000), AdmittedCount: intPtr(20)},
			{UniversityMajorAdmissionID: 3002,
				UniversityID: 2, UniversityCode: "U2", UniversityName: "B", City: "南京",
				GroupCode: "002", LocalMajorName: "M2", MinRank: intPtr(2500), AdmittedCount: intPtr(20)},
			{UniversityMajorAdmissionID: 3003,
				UniversityID: 3, UniversityCode: "U3", UniversityName: "C", City: "西安",
				GroupCode: "003", LocalMajorName: "M3", MinRank: intPtr(2800), AdmittedCount: intPtr(20)},
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
	require.Equal(t, 0, resp.RushCount, "rush 窗口没候选")
	require.Equal(t, 0, resp.MatchCount, "match 窗口也没候选")
	require.Equal(t, 3, resp.SafeCount, "rush+match 名额应全部下沉到 safe")
	require.Contains(t, resp.Notes[0], "冲", "应在 notes 中提示用户冲档为空")
}

func TestRecommendHighRankStudentDoesNotPullTopSchools(t *testing.T) {
	// N1 升级回归：R=2555 时，老逻辑会把 rush 窗口 clamp 到 [1, 555] 把清北 (rank≈45) 全卷进推荐。
	// 比例窗口下 rush 应该是 [1278, 2172]，绝对不允许把 rank<700 的院校放进来。
	stub := &stubRecommendationStore{
		year: 2024,
		candidates: []RecommendationCandidate{
			// 清北档：rank 45，远远超出学生水平 — 不应进任何桶
			{UniversityMajorAdmissionID: 9001,
				UniversityID: 90, UniversityCode: "PKU", UniversityName: "北京大学",
				GroupCode: "001", LocalMajorCode: "01", LocalMajorName: "计算机",
				MinRank: intPtr(45), AdmittedCount: intPtr(10), UniversityTier: "top_2"},
			// 偏鼓励：rank 1500，落在 rush 窗口
			{UniversityMajorAdmissionID: 9002,
				UniversityID: 91, UniversityCode: "BIT", UniversityName: "B 大学",
				GroupCode: "002", LocalMajorCode: "02", LocalMajorName: "通信",
				MinRank: intPtr(1500), AdmittedCount: intPtr(10), UniversityTier: "211_double"},
			// 稳：rank 2500，靠近学生位次
			{UniversityMajorAdmissionID: 9003,
				UniversityID: 92, UniversityCode: "USTC", UniversityName: "C 大学",
				GroupCode: "003", LocalMajorCode: "03", LocalMajorName: "电子",
				MinRank: intPtr(2500), AdmittedCount: intPtr(10), UniversityTier: "985_other"},
			// 保：rank 3500，下沉一档
			{UniversityMajorAdmissionID: 9004,
				UniversityID: 93, UniversityCode: "TJU", UniversityName: "D 大学",
				GroupCode: "004", LocalMajorCode: "04", LocalMajorName: "材料",
				MinRank: intPtr(3500), AdmittedCount: intPtr(10), UniversityTier: "985_other"},
		},
	}
	svc := NewRecommendationService(stub, newStubMetadataStore(), nil)
	resp, err := svc.Recommend(context.Background(), &RecommendationRequest{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		TotalScore:          680,
		ProvincialRank:      2555,
	})
	require.NoError(t, err)
	for _, it := range resp.Items {
		require.NotEqual(t, "北京大学", it.UniversityName,
			"R=2555 学生的推荐表里不应该出现 rank=45 的清北档")
	}
	// match 应大致围绕 R 居中：[R*0.85, R*1.15]，对 R=2555 ≈ [2171, 2938]。
	require.InDelta(t, 2171, resp.RankWindow.MatchMin, 1)
	require.InDelta(t, 2938, resp.RankWindow.MatchMax, 1)
}

func TestEstimateProbabilityScalesWithStudentRank(t *testing.T) {
	// 比例概率：同样 gap 在高分段是巨大相对差距，在低分段是微小波动。
	// R=2000，校 rank=4000 → ratio=1.0 → 0.85（相比之下学校录的人比你多两倍）
	// R=60000，校 rank=62000 → ratio≈0.03 → 0.55（基本是同档）
	gapHigh := RecommendationCandidate{MinRank: intPtr(4000)}
	require.InDelta(t, 0.85, estimateProbability(&gapHigh, &RecommendationRequest{ProvincialRank: 2000}, ""), 0.001,
		"高分段 (R=2000) 校 rank=4000 → ratio=1.0，应该相当稳")
	gapLow := RecommendationCandidate{MinRank: intPtr(62000)}
	require.InDelta(t, 0.55, estimateProbability(&gapLow, &RecommendationRequest{ProvincialRank: 60000}, ""), 0.001,
		"中分段 (R=60000) 校 rank=62000 → ratio≈0.03，仅算持平")
}

func TestRecommendValidatesRequest(t *testing.T) {
	svc := NewRecommendationService(&stubRecommendationStore{year: 2024}, newStubMetadataStore(), nil)
	_, err := svc.Recommend(context.Background(), &RecommendationRequest{})
	require.Error(t, err)
}

func TestRecommendKeepsSafeBucketWhenCandidatesExceedLimit(t *testing.T) {
	// N1 回归：当候选总数 > 5000 时，原来"一次大查询 LIMIT 5000 ORDER BY min_rank ASC"
	// 会把保档（高位次）整段砍掉。修复后改为分桶独立 LIMIT，保档桶必须能拿到候选。
	//
	// 学生位次 10000：
	//   rushW  = [1, 8000]      → 放 rushBucketLimit+200 条候选
	//   matchW = [7000, 13000]  → 放 100 条
	//   safeW  = [12000, 25000] → 放 100 条
	// 验证保档桶不会因为冲档候选爆量而被挤掉。
	cands := []RecommendationCandidate{}
	for i := 0; i < rushBucketLimit+200; i++ {
		cands = append(cands, RecommendationCandidate{
			UniversityMajorAdmissionID: int64(100000 + i),
			UniversityID:               int64(i),
			UniversityCode:             fmt.Sprintf("R%05d", i),
			GroupCode:                  "01",
			LocalMajorCode:             fmt.Sprintf("M%05d", i),
			LocalMajorName:             "占位",
			MinRank:                    intPtr(1 + i),
			AdmittedCount:              intPtr(10),
		})
	}
	for i := 0; i < 100; i++ {
		cands = append(cands, RecommendationCandidate{
			UniversityMajorAdmissionID: int64(200000 + i),
			UniversityID:               int64(20000 + i),
			UniversityCode:             fmt.Sprintf("M%05d", i),
			GroupCode:                  "01",
			LocalMajorCode:             fmt.Sprintf("MM%05d", i),
			LocalMajorName:             "match-占位",
			MinRank:                    intPtr(9000 + i*10),
			AdmittedCount:              intPtr(10),
		})
	}
	for i := 0; i < 100; i++ {
		cands = append(cands, RecommendationCandidate{
			UniversityMajorAdmissionID: int64(300000 + i),
			UniversityID:               int64(30000 + i),
			UniversityCode:             fmt.Sprintf("S%05d", i),
			GroupCode:                  "01",
			LocalMajorCode:             fmt.Sprintf("SS%05d", i),
			LocalMajorName:             "safe-占位",
			MinRank:                    intPtr(13000 + i*100),
			AdmittedCount:              intPtr(10),
		})
	}
	stub := &stubRecommendationStore{year: 2024, candidates: cands}
	svc := NewRecommendationService(stub, newStubMetadataStore(), nil)
	resp, err := svc.Recommend(context.Background(), &RecommendationRequest{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		TotalScore:          580,
		ProvincialRank:      10000,
		PlanSize:            40,
	})
	require.NoError(t, err)
	require.Equal(t, 3, stub.callCount, "应该按冲/稳/保三段各发一次 query")
	require.Greater(t, resp.SafeCount, 0, "保档桶必须非空——回归：原 LIMIT 5000 ASC 会把高位次保档全砍掉")
	require.Greater(t, resp.MatchCount, 0, "稳档桶也应有候选")
}

func TestFilterByPreferenceChemistryDoesNotExcludeBio(t *testing.T) {
	// I3 反例：用户排除"化学"应只影响化学和化工类，不应一刀切到生物类。
	candidates := []RecommendationCandidate{
		{LocalMajorName: "应用化学", TagCategoryCodes: "0703"},
		{LocalMajorName: "化学工程", TagCategoryCodes: "0813"},
		{LocalMajorName: "生物科学", TagCategoryCodes: "0710"},
		{LocalMajorName: "材料科学", TagCategoryCodes: "0804"},
	}
	out, _ := filterByPreference(candidates, &RecommendationRequest{
		ExcludedKeywords: []string{"化学"},
	}, fixtureMetadata())
	names := map[string]bool{}
	for _, c := range out {
		names[c.LocalMajorName] = true
	}
	require.False(t, names["应用化学"], "排除化学应屏蔽 0703")
	require.False(t, names["化学工程"], "排除化学应屏蔽 0813")
	require.True(t, names["生物科学"], "排除化学不应连带屏蔽生物")
	require.True(t, names["材料科学"], "排除化学不应连带屏蔽材料")
}

func TestFilterByPreferenceFiveChinesePreservesTraditionalAvoid(t *testing.T) {
	// I3 正面：用户传"生化环材"时仍然保持原有的一刀切语义。
	candidates := []RecommendationCandidate{
		{LocalMajorName: "应用化学", TagCategoryCodes: "0703"},
		{LocalMajorName: "生物科学", TagCategoryCodes: "0710"},
		{LocalMajorName: "材料科学", TagCategoryCodes: "0804"},
		{LocalMajorName: "环境工程", TagCategoryCodes: "0825"},
		{LocalMajorName: "土木工程", TagCategoryCodes: "0810"},
	}
	out, _ := filterByPreference(candidates, &RecommendationRequest{
		ExcludedKeywords: []string{"生化环材"},
	}, fixtureMetadata())
	require.Len(t, out, 1, "生化环材关键字应屏蔽 5 类")
	require.Equal(t, "土木工程", out[0].LocalMajorName)
}

func TestDecideStrategyFallbackWhenMetadataEmpty(t *testing.T) {
	// N2 回归：metadata 没装载策略关键字时（baseline 未跑或 seed 被清），不应静默退化到永远 "major"。
	// 应该回退到代码内 hardcoded 列表，仍能识别 STEM / humanities 偏好。
	emptyMD := &RecommendationMetadata{
		CityToGroupCode:  map[string]string{},
		StrategyKeywords: map[string][]string{}, // 空表
	}
	s, _ := decideStrategy(&RecommendationRequest{
		PriorityStrategy: "auto",
		PreferredMajors:  []string{"汉语言文学"},
	}, emptyMD)
	require.Equal(t, "school", s, "metadata 空时应回退到 hardcoded 列表识别文管偏好")

	s, _ = decideStrategy(&RecommendationRequest{
		PriorityStrategy: "auto",
		PreferredMajors:  []string{"计算机科学"},
	}, emptyMD)
	require.Equal(t, "major", s, "metadata 空时应回退到 hardcoded 列表识别 STEM 偏好")
}
