package admission

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RecommendationCandidate is one (university × admission group × major) row enriched with all
// the columns the recommendation algorithm needs to filter and score.
type RecommendationCandidate struct {
	UniversityMajorAdmissionID int64
	AdmissionGroupID           int64

	UniversityID                   int64
	UniversityCode                 string
	UniversityName                 string
	City                           string
	ProvinceCode                   string
	UniversityTier                 string // top_2 / hua_5 / c9 / 985_other / 211_double / key / regular / private / vocational. May be empty.
	Is985                          bool
	Is211                          bool
	IsDoubleClass                  bool
	IsNationalKey                  bool
	SoftRank                       *string
	PostgraduateRecommendationRate *float64

	BatchCode              string
	GroupCode              string
	SubjectRequirementCode string

	LocalMajorCode     string
	LocalMajorName     string
	PlanCount          *int
	MinScore           *int
	MinRank            *int
	MaxScore           *int
	MaxRank            *int
	EquivalentMinScore *int
	Tuition            *int
	Duration           string

	MajorIntro            string
	EmploymentDirection   string
	PostgraduateDirection string

	DisciplineCategory   string
	FirstLevelDiscipline string
	SoftMajorGrade       string
	MajorRank            string
	MajorEvaluationScore *float64
	IsNationalFeature    *bool

	// CHSI mapped tags (joined as comma-separated for cheap matching downstream)
	TagCategoryCodes string
	TagClassCodes    string
	TagMajorCodes    string
	TagNames         string

	// Precomputed five-dimension base scores (from recommendation_precomputed_scores).
	// Nil pointer → algorithm falls back to the on-the-fly formula.
	PrecomputedCityScore                  *float64
	PrecomputedSchoolScore                *float64
	PrecomputedMajorScore                 *float64
	PrecomputedAbilityImprovementScore    *float64
	PrecomputedFutureCompetitivenessScore *float64

	PrecomputedCityReason                  string
	PrecomputedSchoolReason                string
	PrecomputedMajorReason                 string
	PrecomputedAbilityImprovementReason    string
	PrecomputedFutureCompetitivenessReason string

	PrecomputedEvaluatedBy    string // 'algorithm' | 'llm' | 'manual' | '' (never evaluated)
	PrecomputedEvaluatorModel string
}

// RecommendationStore fetches the candidate pool from the database. The algorithm
// (in recommendation_service.go) does all in-memory filtering and scoring on top.
type RecommendationStore interface {
	// FetchCandidates returns admission rows whose historical min_rank lies within [rankMin, rankMax],
	// optionally filtered by region/subject/batch and excluded provinces (the cheap "一刀切" pre-filter).
	FetchCandidates(ctx context.Context, q *CandidateQuery) ([]RecommendationCandidate, error)
	// LatestAdmissionYear returns the most recent admission_year present in the DB for the given filter.
	LatestAdmissionYear(ctx context.Context, regionCode, subjectCategoryCode string) (int, error)
}

// CandidateQuery is the inputs the store needs to assemble the SQL.
type CandidateQuery struct {
	AdmissionYear          int
	RegionCode             string
	SubjectCategoryCode    string
	SubjectRequirementCode string
	BatchCodes             []string
	RankMin                int
	RankMax                int
	BudgetTuitionMax       *int
	ExcludedProvinces      []string
	ExcludedCities         []string
}

type recommendationStore struct {
	pool *pgxpool.Pool
}

func NewRecommendationStore(pool *pgxpool.Pool) RecommendationStore {
	return &recommendationStore{pool: pool}
}

func (s *recommendationStore) LatestAdmissionYear(ctx context.Context, regionCode, subjectCategoryCode string) (int, error) {
	// 必须取"最新且实际带分数线的年份"——单纯 MAX(admission_year) 会撞到
	// 当年招生计划行（min_rank/min_score 全空），导致后续算法 0 命中。
	const q = `
		SELECT COALESCE(MAX(ag.admission_year), 0)
		FROM admission_groups ag
		WHERE ($1::text IS NULL OR ag.region_code = $1)
		  AND ($2::text IS NULL OR ag.subject_category_code = $2)
		  AND EXISTS (
		      SELECT 1 FROM university_major_admissions uma
		      WHERE uma.admission_group_id = ag.id AND uma.min_rank IS NOT NULL
		  )
	`
	var year int
	if err := s.pool.QueryRow(ctx, q, nullableString(regionCode), nullableString(subjectCategoryCode)).Scan(&year); err != nil {
		return 0, fmt.Errorf("latest admission year: %w", err)
	}
	return year, nil
}

func (s *recommendationStore) FetchCandidates(ctx context.Context, q *CandidateQuery) ([]RecommendationCandidate, error) {
	args := []any{}
	conditions := []string{"1 = 1"}

	args = append(args, q.AdmissionYear)
	conditions = append(conditions, fmt.Sprintf("ag.admission_year = $%d", len(args)))

	if q.RegionCode != "" {
		args = append(args, q.RegionCode)
		conditions = append(conditions, fmt.Sprintf("ag.region_code = $%d", len(args)))
	}
	if q.SubjectCategoryCode != "" {
		args = append(args, q.SubjectCategoryCode)
		conditions = append(conditions, fmt.Sprintf("ag.subject_category_code = $%d", len(args)))
	}
	if q.SubjectRequirementCode != "" {
		args = append(args, q.SubjectRequirementCode)
		conditions = append(conditions, fmt.Sprintf("(ag.subject_requirement_code IS NULL OR ag.subject_requirement_code = '' OR ag.subject_requirement_code = $%d)", len(args)))
	}
	if len(q.BatchCodes) > 0 {
		args = append(args, q.BatchCodes)
		conditions = append(conditions, fmt.Sprintf("ag.batch_code = ANY($%d)", len(args)))
	}
	if q.RankMin > 0 {
		args = append(args, q.RankMin)
		conditions = append(conditions, fmt.Sprintf("uma.min_rank >= $%d", len(args)))
	}
	if q.RankMax > 0 {
		args = append(args, q.RankMax)
		conditions = append(conditions, fmt.Sprintf("uma.min_rank <= $%d", len(args)))
	}
	conditions = append(conditions, "uma.min_rank IS NOT NULL")

	if q.BudgetTuitionMax != nil {
		args = append(args, *q.BudgetTuitionMax)
		conditions = append(conditions, fmt.Sprintf("(uma.tuition IS NULL OR uma.tuition <= $%d)", len(args)))
	}
	if len(q.ExcludedProvinces) > 0 {
		args = append(args, q.ExcludedProvinces)
		conditions = append(conditions, fmt.Sprintf("(up.region_code IS NULL OR up.region_code <> ALL($%d))", len(args)))
	}
	if len(q.ExcludedCities) > 0 {
		args = append(args, q.ExcludedCities)
		conditions = append(conditions, fmt.Sprintf("(up.city IS NULL OR up.city <> ALL($%d))", len(args)))
	}

	query := fmt.Sprintf(`
		SELECT
			uma.id, ag.id,
			u.id, u.university_code, u.name,
			COALESCE(up.city, ''), COALESCE(up.region_code, ''),
			COALESCE(up.university_tier, ''),
			COALESCE(up.is_985, false), COALESCE(up.is_211, false),
			COALESCE(up.is_double_first_class, false), COALESCE(up.is_national_key, false),
			up.soft_rank, up.postgraduate_recommendation_rate,
			ag.batch_code, ag.group_code, COALESCE(ag.subject_requirement_code, ''),
			uma.local_major_code, uma.local_major_name,
			uma.plan_count, uma.min_score, uma.min_rank, uma.max_score, uma.max_rank,
			uma.equivalent_min_score, uma.tuition, COALESCE(uma.duration, ''),
			COALESCE(uma.major_intro, ''), COALESCE(uma.employment_direction, ''),
			COALESCE(uma.postgraduate_direction, ''),
			COALESCE(ump.discipline_category, ''), COALESCE(ump.first_level_discipline, ''),
			COALESCE(ump.soft_major_grade, ''), COALESCE(ump.major_rank, ''),
			ump.major_evaluation_score, ump.is_national_feature,
			COALESCE(STRING_AGG(DISTINCT tag.category_code, ',') FILTER (WHERE tag.category_code IS NOT NULL), ''),
			COALESCE(STRING_AGG(DISTINCT tag.class_code, ',') FILTER (WHERE tag.class_code IS NOT NULL), ''),
			COALESCE(STRING_AGG(DISTINCT tag.major_code, ',') FILTER (WHERE tag.major_code IS NOT NULL), ''),
			COALESCE(STRING_AGG(DISTINCT
				CONCAT_WS('|',
					NULLIF(tag.category_name, ''),
					NULLIF(tag.class_name, ''),
					NULLIF(tag.major_name, '')
				), ',') FILTER (WHERE tag.id IS NOT NULL), ''),
			ps.city_score, ps.school_score, ps.major_score,
			ps.ability_improvement_score, ps.future_competitiveness_score,
			COALESCE(ps.city_reason, ''), COALESCE(ps.school_reason, ''),
			COALESCE(ps.major_reason, ''), COALESCE(ps.ability_improvement_reason, ''),
			COALESCE(ps.future_competitiveness_reason, ''),
			COALESCE(ps.evaluated_by, ''), COALESCE(ps.evaluator_model, '')
		FROM university_major_admissions uma
		JOIN admission_groups ag ON ag.id = uma.admission_group_id
		JOIN universities u ON u.id = ag.university_id
		LEFT JOIN university_major_profiles ump ON ump.university_major_admission_id = uma.id
		LEFT JOIN admission_major_tags tag ON tag.university_major_admission_id = uma.id
		LEFT JOIN recommendation_precomputed_scores ps
			ON ps.university_id = u.id AND ps.local_major_code = uma.local_major_code
		LEFT JOIN LATERAL (
			SELECT latest_up.*
			FROM university_profiles latest_up
			WHERE latest_up.university_id = u.id
			ORDER BY latest_up.profile_year DESC
			LIMIT 1
		) up ON true
		WHERE %s
		GROUP BY uma.id, ag.id, u.id, up.city, up.region_code,
			up.university_tier,
			up.is_985, up.is_211, up.is_double_first_class, up.is_national_key,
			up.soft_rank, up.postgraduate_recommendation_rate,
			ps.city_score, ps.school_score, ps.major_score,
			ps.ability_improvement_score, ps.future_competitiveness_score,
			ps.city_reason, ps.school_reason, ps.major_reason,
			ps.ability_improvement_reason, ps.future_competitiveness_reason,
			ps.evaluated_by, ps.evaluator_model,
			ump.discipline_category, ump.first_level_discipline,
			ump.soft_major_grade, ump.major_rank, ump.major_evaluation_score, ump.is_national_feature
		ORDER BY uma.min_rank ASC, u.name
	`, strings.Join(conditions, " AND "))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch recommendation candidates: %w", err)
	}
	defer rows.Close()

	var out []RecommendationCandidate
	for rows.Next() {
		var c RecommendationCandidate
		if err := rows.Scan(
			&c.UniversityMajorAdmissionID, &c.AdmissionGroupID,
			&c.UniversityID, &c.UniversityCode, &c.UniversityName,
			&c.City, &c.ProvinceCode,
			&c.UniversityTier,
			&c.Is985, &c.Is211, &c.IsDoubleClass, &c.IsNationalKey,
			&c.SoftRank, &c.PostgraduateRecommendationRate,
			&c.BatchCode, &c.GroupCode, &c.SubjectRequirementCode,
			&c.LocalMajorCode, &c.LocalMajorName,
			&c.PlanCount, &c.MinScore, &c.MinRank, &c.MaxScore, &c.MaxRank,
			&c.EquivalentMinScore, &c.Tuition, &c.Duration,
			&c.MajorIntro, &c.EmploymentDirection, &c.PostgraduateDirection,
			&c.DisciplineCategory, &c.FirstLevelDiscipline,
			&c.SoftMajorGrade, &c.MajorRank,
			&c.MajorEvaluationScore, &c.IsNationalFeature,
			&c.TagCategoryCodes, &c.TagClassCodes, &c.TagMajorCodes, &c.TagNames,
			&c.PrecomputedCityScore, &c.PrecomputedSchoolScore, &c.PrecomputedMajorScore,
			&c.PrecomputedAbilityImprovementScore, &c.PrecomputedFutureCompetitivenessScore,
			&c.PrecomputedCityReason, &c.PrecomputedSchoolReason,
			&c.PrecomputedMajorReason, &c.PrecomputedAbilityImprovementReason,
			&c.PrecomputedFutureCompetitivenessReason,
			&c.PrecomputedEvaluatedBy, &c.PrecomputedEvaluatorModel,
		); err != nil {
			return nil, fmt.Errorf("scan recommendation candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recommendation candidates: %w", err)
	}
	return out, nil
}
