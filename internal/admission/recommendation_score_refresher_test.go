package admission

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubScoreStore struct {
	pending []PrecomputedScoreRow
	upserts []PrecomputedScoreRow
	upErr   error
}

func (s *stubScoreStore) PendingForRefresh(_ context.Context, _ time.Duration, _ int) ([]PrecomputedScoreRow, error) {
	return s.pending, nil
}

func (s *stubScoreStore) Upsert(_ context.Context, row *PrecomputedScoreRow) error {
	if s.upErr != nil {
		return s.upErr
	}
	s.upserts = append(s.upserts, *row)
	return nil
}

type stubScoreEvaluator struct {
	source string
	eval   *ScoreEvaluation
	err    error
}

func (s stubScoreEvaluator) Source() string { return s.source }
func (s stubScoreEvaluator) Evaluate(_ context.Context, _ *PrecomputedScoreRow) (*ScoreEvaluation, error) {
	return s.eval, s.err
}

func TestRefresherIteratesAndUpserts(t *testing.T) {
	store := &stubScoreStore{
		pending: []PrecomputedScoreRow{
			{UniversityID: 1, LocalMajorCode: "01", UniversityName: "A大学", LocalMajorName: "计算机"},
			{UniversityID: 2, LocalMajorCode: "02", UniversityName: "B大学", LocalMajorName: "金融学"},
		},
	}
	eval := stubScoreEvaluator{
		source: "test",
		eval: &ScoreEvaluation{
			CityScore: 1.1, SchoolScore: 1.5, MajorScore: 1.3,
			AbilityImprovementScore: 1.2, FutureCompetitivenessScore: 1.4,
			ModelID: "test-model-1",
		},
	}
	r := NewRecommendationScoreRefresher(store, eval)
	res, err := r.Refresh(context.Background(), RefreshOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, res.Evaluated)
	require.Equal(t, 0, res.Failed)
	require.Equal(t, "test", res.Source)
	require.Len(t, store.upserts, 2)
	require.Equal(t, 1.5, store.upserts[0].SchoolScore)
	require.Equal(t, "test", store.upserts[0].EvaluatedBy)
	require.Equal(t, "test-model-1", store.upserts[0].EvaluatorModel)
}

func TestRefresherCountsFailures(t *testing.T) {
	store := &stubScoreStore{
		pending: []PrecomputedScoreRow{
			{UniversityID: 1, LocalMajorCode: "01"},
			{UniversityID: 2, LocalMajorCode: "02"},
			{UniversityID: 3, LocalMajorCode: "03"},
		},
	}
	calls := 0
	eval := stubFlakyEvaluator{
		ok: &ScoreEvaluation{CityScore: 1, SchoolScore: 1, MajorScore: 1, AbilityImprovementScore: 1, FutureCompetitivenessScore: 1},
		each: func() error {
			calls++
			if calls == 2 {
				return errors.New("LLM rate limit")
			}
			return nil
		},
	}
	r := NewRecommendationScoreRefresher(store, eval)
	res, err := r.Refresh(context.Background(), RefreshOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, res.Evaluated)
	require.Equal(t, 1, res.Failed)
	require.Len(t, res.Errors, 1)
}

type stubFlakyEvaluator struct {
	ok   *ScoreEvaluation
	each func() error
}

func (s stubFlakyEvaluator) Source() string { return "flaky" }
func (s stubFlakyEvaluator) Evaluate(_ context.Context, _ *PrecomputedScoreRow) (*ScoreEvaluation, error) {
	if err := s.each(); err != nil {
		return nil, err
	}
	return s.ok, nil
}

func TestAlgorithmicEvaluatorScoresKnownTier(t *testing.T) {
	// 使用与 service.fallback*Base 相同的公式：清华 (top_2 tier) → school=2.0，
	// 北京在 metadata 的城市群里 → city=1.1。
	md := fixtureMetadata()
	e := NewAlgorithmicScoreEvaluator(md)
	res, err := e.Evaluate(context.Background(), &PrecomputedScoreRow{
		UniversityName: "清华大学",
		UniversityTier: "top_2",
		City:           "北京",
		Is985:          true,
	})
	require.NoError(t, err)
	require.Equal(t, 2.0, res.SchoolScore)
	require.Equal(t, 1.1, res.CityScore)
	require.Equal(t, "algorithm", e.Source())
}

func TestAlgorithmicEvaluatorMatchesServiceFallback(t *testing.T) {
	// I4 回归：AlgorithmicScoreEvaluator 写入的 base 必须与 service 端 fallback*Base 公式
	// 计算结果一致，否则 "种子行" 和 "未种子直接 fallback" 两条路径会给出不同的 composite。
	md := fixtureMetadata()
	row := &PrecomputedScoreRow{
		UniversityTier:     "211_double",
		City:               "上海",
		Is211:              true,
		DisciplineCategory: "工学",
	}
	c := &RecommendationCandidate{
		City:               row.City,
		UniversityTier:     row.UniversityTier,
		Is211:              row.Is211,
		DisciplineCategory: row.DisciplineCategory,
	}

	res, err := NewAlgorithmicScoreEvaluator(md).Evaluate(context.Background(), row)
	require.NoError(t, err)
	require.Equal(t, fallbackCityBase(c, md), res.CityScore)
	require.Equal(t, fallbackSchoolBase(c), res.SchoolScore)
	require.Equal(t, fallbackFutureBase(c), res.FutureCompetitivenessScore)
}
