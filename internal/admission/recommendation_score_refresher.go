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
	MaxAge time.Duration // rows older than this are eligible
	Limit  int           // batch size
}

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
		opts.Limit = 50
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
		eval, err := r.evaluator.Evaluate(ctx, row)
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

// AlgorithmicScoreEvaluator is the default fallback evaluator: no LLM, just
// derives reasonable scores from existing fields. Useful for bulk seeding
// when the LLM key is not available.
type AlgorithmicScoreEvaluator struct{}

func (AlgorithmicScoreEvaluator) Source() string { return "algorithm" }

func (AlgorithmicScoreEvaluator) Evaluate(_ context.Context, row *PrecomputedScoreRow) (*ScoreEvaluation, error) {
	city := 1.0
	if isHotCityName(row.City) {
		city = 1.1
	}

	school := 1.0
	switch row.UniversityTier {
	case "top_2":
		school = 2.0
	case "hua_5":
		school = 1.8
	case "c9":
		school = 1.7
	case "985_other":
		school = 1.5
	case "211_double":
		school = 1.3
	case "key":
		school = 1.15
	default:
		switch {
		case row.Is985:
			school = 1.5
		case row.Is211, row.IsDoubleClass:
			school = 1.3
		}
	}

	major := 1.0
	ability := 1.0
	future := 1.0

	return &ScoreEvaluation{
		CityScore:                   city,
		SchoolScore:                 school,
		MajorScore:                  major,
		AbilityImprovementScore:     ability,
		FutureCompetitivenessScore:  future,
		CityReason:                  "算法默认值（需 LLM/人工评估替换）",
		SchoolReason:                fmt.Sprintf("基于学校档次 %s", row.UniversityTier),
		MajorReason:                 "算法默认值（需 LLM/人工评估替换）",
		AbilityImprovementReason:    "算法默认值（需 LLM/人工评估替换）",
		FutureCompetitivenessReason: "算法默认值（需 LLM/人工评估替换）",
		ModelID:                     "",
	}, nil
}

func isHotCityName(city string) bool {
	hot := []string{"北京", "上海", "广州", "深圳", "杭州", "南京", "成都", "重庆", "西安", "武汉"}
	for _, h := range hot {
		if city == h {
			return true
		}
	}
	return false
}
