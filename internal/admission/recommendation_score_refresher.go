package admission

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ScoreEvaluation is the evaluator's output for one (university × major).
// All five base scores plus the per-dimension reason strings.
type ScoreEvaluation struct {
	CityScore                  float64
	SchoolScore                float64
	MajorScore                 float64
	AbilityImprovementScore    float64
	FutureCompetitivenessScore float64

	CityReason                  string
	SchoolReason                string
	MajorReason                 string
	AbilityImprovementReason    string
	FutureCompetitivenessReason string

	ModelID string // 'algorithm' / a model name like 'claude-opus-4-7'
}

// ScoreEvaluator produces the five base scores for one (university × major).
// Implementations: algorithmic baseline (default 1.0 + heuristics), LLM-backed,
// or admin-driven manual entry via the API.
type ScoreEvaluator interface {
	// Evaluate returns the five scores + reasons. Must be deterministic enough
	// that the refresher can call it repeatedly without consuming budget on
	// already-evaluated rows.
	Evaluate(ctx context.Context, row *PrecomputedScoreRow) (*ScoreEvaluation, error)
	// Source identifies the evaluator in the `evaluated_by` column.
	Source() string
}

// RecommendationScoreRefresher iterates pending rows and writes fresh scores.
type RecommendationScoreRefresher struct {
	store     RecommendationScoreStore
	evaluator ScoreEvaluator
}

func NewRecommendationScoreRefresher(store RecommendationScoreStore, evaluator ScoreEvaluator) *RecommendationScoreRefresher {
	return &RecommendationScoreRefresher{store: store, evaluator: evaluator}
}

// RefreshOptions control one batch run.
type RefreshOptions struct {
	MaxAge        time.Duration // rows older than this are eligible
	Limit         int           // batch size
	PerCallBudget time.Duration // per-row timeout for the evaluator call (0 → default)
}

// refresherLimits — keep refresh synchronous: gin server WriteTimeout is 15s and
// LLM evaluators take 5–15s per row, so we cap each call and the whole batch.
// Caller (handler) should also wrap ctx with an overall timeout so partial
// results are returned cleanly when the budget runs out.
const (
	refresherDefaultLimit         = 2               // ~2 × 6s ≈ 12s, comfortably under 15s gin WriteTimeout
	refresherMaxLimit             = 5               // hard ceiling to avoid runaway admin requests; rest must come from repeated calls
	refresherDefaultPerCallBudget = 6 * time.Second // per evaluator call timeout
)

// RefreshResult is what the admin endpoint / CLI returns.
type RefreshResult struct {
	Evaluated int      `json:"evaluated"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
	Source    string   `json:"source"`
}

func (r *RecommendationScoreRefresher) Refresh(ctx context.Context, opts RefreshOptions) (*RefreshResult, error) {
	if opts.MaxAge <= 0 {
		opts.MaxAge = 90 * 24 * time.Hour
	}
	if opts.Limit <= 0 {
		opts.Limit = refresherDefaultLimit
	}
	if opts.Limit > refresherMaxLimit {
		opts.Limit = refresherMaxLimit
	}
	if opts.PerCallBudget <= 0 {
		opts.PerCallBudget = refresherDefaultPerCallBudget
	}

	rows, err := r.store.PendingForRefresh(ctx, opts.MaxAge, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("fetch pending: %w", err)
	}
	res := &RefreshResult{Source: r.evaluator.Source()}

	for i := range rows {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		row := &rows[i]

		callCtx, cancel := context.WithTimeout(ctx, opts.PerCallBudget)
		eval, err := r.evaluator.Evaluate(callCtx, row)
		cancel()
		if err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("uni=%d major=%s: %v", row.UniversityID, row.LocalMajorCode, err))
			slog.Warn("score evaluator failed",
				"university_id", row.UniversityID,
				"local_major_code", row.LocalMajorCode,
				"error", err,
			)
			continue
		}
		applyEvaluation(row, eval, r.evaluator.Source())
		if err := r.store.Upsert(ctx, row); err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("uni=%d major=%s upsert: %v", row.UniversityID, row.LocalMajorCode, err))
			continue
		}
		res.Evaluated++
	}
	return res, nil
}

func applyEvaluation(row *PrecomputedScoreRow, eval *ScoreEvaluation, source string) {
	row.CityScore = eval.CityScore
	row.SchoolScore = eval.SchoolScore
	row.MajorScore = eval.MajorScore
	row.AbilityImprovementScore = eval.AbilityImprovementScore
	row.FutureCompetitivenessScore = eval.FutureCompetitivenessScore
	row.CityReason = eval.CityReason
	row.SchoolReason = eval.SchoolReason
	row.MajorReason = eval.MajorReason
	row.AbilityImprovementReason = eval.AbilityImprovementReason
	row.FutureCompetitivenessReason = eval.FutureCompetitivenessReason
	row.EvaluatedBy = source
	row.EvaluatorModel = eval.ModelID
}

// AlgorithmicScoreEvaluator is the no-LLM fallback evaluator. It reuses the
// same fallback formulas the recommendation service applies when a precomputed
// row is missing, so seeded rows match the on-the-fly behavior and the
// recommendation pipeline yields the same composite score whether the row is
// pre-seeded or computed live.
//
// Holds a metadata snapshot taken at construction; cheap (~500 rows total) and
// stable enough that re-loading per Evaluate isn't worth the DB cost.
type AlgorithmicScoreEvaluator struct {
	md *RecommendationMetadata
}

// NewAlgorithmicScoreEvaluator builds an evaluator backed by the supplied metadata snapshot.
// Pass a freshly-loaded RecommendationMetadata; the constructor does no I/O.
func NewAlgorithmicScoreEvaluator(md *RecommendationMetadata) AlgorithmicScoreEvaluator {
	return AlgorithmicScoreEvaluator{md: md}
}

func (AlgorithmicScoreEvaluator) Source() string { return "algorithm" }

func (e AlgorithmicScoreEvaluator) Evaluate(_ context.Context, row *PrecomputedScoreRow) (*ScoreEvaluation, error) {
	// 构造一个 RecommendationCandidate-shaped 视图，复用 service 端的 fallback*Base 公式，
	// 保证"种子行"和"未种子时的实时回退"两条路径结果一致。
	// PrecomputedScoreRow 缺少的字段（SoftRank / PostgraduateRecommendationRate /
	// IsNationalFeature / SoftMajorGrade / MajorEvaluationScore）这里保持零值，
	// fallback 公式对零值的处理就是退化到 1.0 base —— 与 LLM 评估器输出冲突时被覆盖即可。
	c := &RecommendationCandidate{
		City:               row.City,
		ProvinceCode:       row.ProvinceCode,
		UniversityTier:     row.UniversityTier,
		Is985:              row.Is985,
		Is211:              row.Is211,
		IsDoubleClass:      row.IsDoubleClass,
		DisciplineCategory: row.DisciplineCategory,
	}

	md := e.md
	if md == nil {
		md = &RecommendationMetadata{CityToGroupCode: map[string]string{}}
	}

	city := fallbackCityBase(c, md)
	school := fallbackSchoolBase(c)
	major := fallbackMajorBase(c)
	future := fallbackFutureBase(c)
	ability := 1.0 // 算法侧无法判断"对学生综合能力提升"，留给 LLM/人工填

	cityReason := "算法回退：所在城市未在重点城市群"
	if _, ok := md.CityToGroupCode[row.City]; ok {
		cityReason = fmt.Sprintf("算法回退：%s 属于重点城市群", row.City)
	}
	schoolReason := fmt.Sprintf("算法回退：基于学校档次 %s", row.UniversityTier)
	if row.UniversityTier == "" {
		schoolReason = "算法回退：基于 985/211/双一流 标记"
	}

	return &ScoreEvaluation{
		CityScore:                   city,
		SchoolScore:                 school,
		MajorScore:                  major,
		AbilityImprovementScore:     ability,
		FutureCompetitivenessScore:  future,
		CityReason:                  cityReason,
		SchoolReason:                schoolReason,
		MajorReason:                 "算法回退：缺少学科评估数据，待 LLM/人工评估",
		AbilityImprovementReason:    "算法回退：能力提升维度需 LLM/人工评估",
		FutureCompetitivenessReason: "算法回退：基于学科门类的粗略估计",
		ModelID:                     "",
	}, nil
}
