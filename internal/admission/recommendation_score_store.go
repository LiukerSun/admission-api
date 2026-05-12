package admission

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PrecomputedScoreRow is one row of the precomputed_scores table, plus the
// metadata about the (university × major) it represents — used by the
// refresher to feed the LLM enough context to evaluate.
type PrecomputedScoreRow struct {
	UniversityID   int64
	UniversityName string
	UniversityTier string
	City           string
	ProvinceCode   string
	Is985          bool
	Is211          bool
	IsDoubleClass  bool

	LocalMajorCode       string
	LocalMajorName       string
	DisciplineCategory   string
	FirstLevelDiscipline string
	MajorIntro           string
	EmploymentDirection  string
	TagNames             string

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

	EvaluatedBy    string
	EvaluatorModel string
	EvaluatedAt    *time.Time
}

// RecommendationScoreStore reads + writes recommendation_precomputed_scores.
type RecommendationScoreStore interface {
	// PendingForRefresh returns (university × major) pairs whose precomputed score
	// is missing or older than `maxAge`. Limit caps the batch size.
	PendingForRefresh(ctx context.Context, maxAge time.Duration, limit int) ([]PrecomputedScoreRow, error)
	// Upsert writes one evaluation result. Composite UNIQUE on (university_id, local_major_code).
	Upsert(ctx context.Context, row *PrecomputedScoreRow) error
}

type recommendationScoreStore struct {
	pool *pgxpool.Pool
}

func NewRecommendationScoreStore(pool *pgxpool.Pool) RecommendationScoreStore {
	return &recommendationScoreStore{pool: pool}
}

func (s *recommendationScoreStore) PendingForRefresh(ctx context.Context, maxAge time.Duration, limit int) ([]PrecomputedScoreRow, error) {
	if limit <= 0 {
		limit = 50
	}
	cutoff := time.Now().Add(-maxAge)
	// 一个 (university, local_major_code) 在不同 admission_group / 年份会出现多次 uma 行，
	// 但 precomputed_scores 表是按 (university_id, local_major_code) 唯一键，
	// 所以这里用 DISTINCT ON 去重，避免对同一对学校×专业重复调用 LLM 评估浪费 token。
	// 取 uma.id 最大的那一行作为代表（即最新一条 admission_group 数据），用它的 major_intro 等元信息。
	const query = `
		WITH latest_profile AS (
			SELECT DISTINCT ON (up.university_id)
				up.university_id,
				up.city,
				up.region_code,
				up.university_tier,
				up.is_985,
				up.is_211,
				up.is_double_first_class
			FROM university_profiles up
			ORDER BY up.university_id, up.profile_year DESC
		),
		distinct_uma AS (
			SELECT DISTINCT ON (u.id, uma.local_major_code)
				u.id           AS university_id,
				u.name         AS university_name,
				uma.id         AS uma_id,
				uma.local_major_code,
				uma.local_major_name,
				uma.major_intro,
				uma.employment_direction
			FROM university_major_admissions uma
			JOIN admission_groups ag ON ag.id = uma.admission_group_id
			JOIN universities u ON u.id = ag.university_id
			ORDER BY u.id, uma.local_major_code, uma.id DESC
		)
		SELECT
			d.university_id, d.university_name,
			COALESCE(up.university_tier, ''),
			COALESCE(up.city, ''), COALESCE(up.region_code, ''),
			COALESCE(up.is_985, false), COALESCE(up.is_211, false), COALESCE(up.is_double_first_class, false),
			d.local_major_code, d.local_major_name,
			COALESCE(ump.discipline_category, ''),
			COALESCE(ump.first_level_discipline, ''),
			COALESCE(d.major_intro, ''),
			COALESCE(d.employment_direction, ''),
			COALESCE(STRING_AGG(DISTINCT
				CONCAT_WS('|',
					NULLIF(tag.category_name, ''),
					NULLIF(tag.class_name, ''),
					NULLIF(tag.major_name, '')
				), ',') FILTER (WHERE tag.id IS NOT NULL), '')
		FROM distinct_uma d
		LEFT JOIN university_major_profiles ump ON ump.university_major_admission_id = d.uma_id
		LEFT JOIN admission_major_tags tag ON tag.university_major_admission_id = d.uma_id
		LEFT JOIN latest_profile up ON up.university_id = d.university_id
		LEFT JOIN recommendation_precomputed_scores ps
			ON ps.university_id = d.university_id AND ps.local_major_code = d.local_major_code
		WHERE ps.id IS NULL OR ps.evaluated_at < $1
		GROUP BY d.university_id, d.university_name, up.university_tier, up.city, up.region_code,
			up.is_985, up.is_211, up.is_double_first_class,
			d.local_major_code, d.local_major_name,
			ump.discipline_category, ump.first_level_discipline,
			d.major_intro, d.employment_direction,
			ps.evaluated_at
		ORDER BY ps.evaluated_at NULLS FIRST, d.university_id, d.local_major_code
		LIMIT $2
	`
	rows, err := s.pool.Query(ctx, query, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("pending for refresh: %w", err)
	}
	defer rows.Close()

	var out []PrecomputedScoreRow
	for rows.Next() {
		var r PrecomputedScoreRow
		if err := rows.Scan(
			&r.UniversityID, &r.UniversityName,
			&r.UniversityTier,
			&r.City, &r.ProvinceCode,
			&r.Is985, &r.Is211, &r.IsDoubleClass,
			&r.LocalMajorCode, &r.LocalMajorName,
			&r.DisciplineCategory, &r.FirstLevelDiscipline,
			&r.MajorIntro, &r.EmploymentDirection,
			&r.TagNames,
		); err != nil {
			return nil, fmt.Errorf("scan pending row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *recommendationScoreStore) Upsert(ctx context.Context, row *PrecomputedScoreRow) error {
	const query = `
		INSERT INTO recommendation_precomputed_scores (
			university_id, local_major_code,
			city_score, school_score, major_score,
			ability_improvement_score, future_competitiveness_score,
			city_reason, school_reason, major_reason,
			ability_improvement_reason, future_competitiveness_reason,
			evaluated_by, evaluator_model, evaluated_at,
			updated_at
		)
		VALUES (
			$1, $2,
			$3, $4, $5,
			$6, $7,
			$8, $9, $10,
			$11, $12,
			$13, $14, NOW(),
			NOW()
		)
		ON CONFLICT (university_id, local_major_code) DO UPDATE SET
			city_score                   = EXCLUDED.city_score,
			school_score                 = EXCLUDED.school_score,
			major_score                  = EXCLUDED.major_score,
			ability_improvement_score    = EXCLUDED.ability_improvement_score,
			future_competitiveness_score = EXCLUDED.future_competitiveness_score,
			city_reason                  = EXCLUDED.city_reason,
			school_reason                = EXCLUDED.school_reason,
			major_reason                 = EXCLUDED.major_reason,
			ability_improvement_reason   = EXCLUDED.ability_improvement_reason,
			future_competitiveness_reason = EXCLUDED.future_competitiveness_reason,
			evaluated_by                 = EXCLUDED.evaluated_by,
			evaluator_model              = EXCLUDED.evaluator_model,
			evaluated_at                 = NOW(),
			updated_at                   = NOW()
	`
	_, err := s.pool.Exec(ctx, query,
		row.UniversityID, row.LocalMajorCode,
		row.CityScore, row.SchoolScore, row.MajorScore,
		row.AbilityImprovementScore, row.FutureCompetitivenessScore,
		row.CityReason, row.SchoolReason, row.MajorReason,
		row.AbilityImprovementReason, row.FutureCompetitivenessReason,
		row.EvaluatedBy, row.EvaluatorModel,
	)
	if err != nil {
		return fmt.Errorf("upsert precomputed score: %w", err)
	}
	return nil
}
