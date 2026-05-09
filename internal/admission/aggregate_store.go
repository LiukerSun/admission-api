package admission

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AggregateStore interface {
	Aggregate(ctx context.Context, filter *AggregateFilter) (*AggregateResponse, error)
}

type aggregateStore struct {
	pool *pgxpool.Pool
}

func NewAggregateStore(pool *pgxpool.Pool) AggregateStore {
	return &aggregateStore{pool: pool}
}

func (s *aggregateStore) Aggregate(ctx context.Context, filter *AggregateFilter) (*AggregateResponse, error) {
	args, conditions := buildAggregateWhere(filter)

	groupCol, nameCol := resolveGroupBy(filter.GroupBy)

	query := fmt.Sprintf(`
		SELECT
			COALESCE(%s, '') AS code,
			COALESCE(%s, '') AS name,
			COUNT(*) AS count,
			AVG(uma.min_score) AS avg_min_score,
			AVG(uma.min_rank) AS avg_min_rank,
			AVG(uma.tuition) AS avg_tuition,
			COUNT(CASE WHEN up.is_985 THEN 1 END) AS is_985_count,
			COUNT(CASE WHEN up.is_211 THEN 1 END) AS is_211_count,
			COUNT(CASE WHEN up.is_double_first_class THEN 1 END) AS is_double_first_class_count
		FROM university_major_admissions uma
		JOIN admission_groups ag ON ag.id = uma.admission_group_id
		JOIN universities u ON u.id = ag.university_id
		LEFT JOIN LATERAL (
			SELECT latest_up.*
			FROM university_profiles latest_up
			WHERE latest_up.university_id = u.id
			ORDER BY latest_up.profile_year DESC
			LIMIT 1
		) up ON true
		LEFT JOIN regions r ON r.code = up.region_code
		WHERE %s
		GROUP BY %s, %s
	`, groupCol, nameCol, strings.Join(conditions, " AND "), groupCol, nameCol)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("aggregate query: %w", err)
	}
	defer rows.Close()

	resp := &AggregateResponse{
		GroupBy: filter.GroupBy,
		Items:   []AggregateItem{},
	}
	for rows.Next() {
		var item AggregateItem
		var avgMinScore, avgMinRank, avgTuition *float64
		var is985Count, is211Count, isDoubleCount *int64

		if err := rows.Scan(
			&item.Code,
			&item.Name,
			&item.Count,
			&avgMinScore,
			&avgMinRank,
			&avgTuition,
			&is985Count,
			&is211Count,
			&isDoubleCount,
		); err != nil {
			return nil, fmt.Errorf("scan aggregate item: %w", err)
		}

		if containsMetric(filter.Metrics, "avg_min_score") {
			item.AvgMinScore = avgMinScore
		}
		if containsMetric(filter.Metrics, "avg_min_rank") {
			item.AvgMinRank = avgMinRank
		}
		if containsMetric(filter.Metrics, "avg_tuition") {
			item.AvgTuition = avgTuition
		}
		if containsMetric(filter.Metrics, "is_985_count") {
			item.Is985Count = is985Count
		}
		if containsMetric(filter.Metrics, "is_211_count") {
			item.Is211Count = is211Count
		}
		if containsMetric(filter.Metrics, "is_double_first_class_count") {
			item.IsDoubleCount = isDoubleCount
		}

		resp.Items = append(resp.Items, item)
		resp.Total += item.Count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aggregate rows: %w", err)
	}
	return resp, nil
}

func buildAggregateWhere(filter *AggregateFilter) ([]any, []string) {
	args := []any{}
	conditions := []string{"1 = 1"}

	if filter.AdmissionYear != nil {
		args = append(args, *filter.AdmissionYear)
		conditions = append(conditions, fmt.Sprintf("ag.admission_year = $%d", len(args)))
	} else {
		conditions = append(conditions, `ag.admission_year = (
			SELECT COALESCE(MAX(admission_year), 0)
			FROM admission_groups
			WHERE ($1::text IS NULL OR region_code = $1)
			  AND ($2::text IS NULL OR subject_category_code = $2)
		)`)
		args = append(args, nullableString(filter.RegionCode), nullableString(filter.SubjectCategoryCode))
	}

	if filter.RegionCode != "" {
		args = append(args, filter.RegionCode)
		conditions = append(conditions, fmt.Sprintf("ag.region_code = $%d", len(args)))
	}
	if filter.SubjectCategoryCode != "" {
		args = append(args, filter.SubjectCategoryCode)
		conditions = append(conditions, fmt.Sprintf("ag.subject_category_code = $%d", len(args)))
	}
	if len(filter.UniversityIDs) > 0 {
		args = append(args, filter.UniversityIDs)
		conditions = append(conditions, fmt.Sprintf("ag.university_id = ANY($%d)", len(args)))
	}
	if len(filter.UniversityCodes) > 0 {
		args = append(args, filter.UniversityCodes)
		conditions = append(conditions, fmt.Sprintf("u.university_code = ANY($%d)", len(args)))
	}
	if len(filter.GroupCodes) > 0 {
		args = append(args, filter.GroupCodes)
		conditions = append(conditions, fmt.Sprintf("ag.group_code = ANY($%d)", len(args)))
	}
	if filter.MinRankFrom != nil {
		args = append(args, *filter.MinRankFrom)
		conditions = append(conditions, fmt.Sprintf("uma.min_rank >= $%d", len(args)))
	}
	if filter.MinRankTo != nil {
		args = append(args, *filter.MinRankTo)
		conditions = append(conditions, fmt.Sprintf("uma.min_rank <= $%d", len(args)))
	}
	if filter.MinScoreFrom != nil {
		args = append(args, *filter.MinScoreFrom)
		conditions = append(conditions, fmt.Sprintf("uma.min_score >= $%d", len(args)))
	}
	if filter.MinScoreTo != nil {
		args = append(args, *filter.MinScoreTo)
		conditions = append(conditions, fmt.Sprintf("uma.min_score <= $%d", len(args)))
	}

	tagConditions := []string{}
	if filter.TagCatalogYear != nil {
		args = append(args, *filter.TagCatalogYear)
		tagConditions = append(tagConditions, fmt.Sprintf("tag.catalog_year = $%d", len(args)))
	}
	if filter.TagCategoryCode != "" {
		args = append(args, filter.TagCategoryCode)
		tagConditions = append(tagConditions, fmt.Sprintf("tag.category_code = $%d", len(args)))
	}
	if filter.TagClassCode != "" {
		args = append(args, filter.TagClassCode)
		tagConditions = append(tagConditions, fmt.Sprintf("tag.class_code = $%d", len(args)))
	}
	if filter.TagMajorCode != "" {
		args = append(args, filter.TagMajorCode)
		tagConditions = append(tagConditions, fmt.Sprintf("tag.major_code = $%d", len(args)))
	}
	if filter.TagQuery != "" {
		args = append(args, "%"+filter.TagQuery+"%")
		tagConditions = append(tagConditions, fmt.Sprintf(`(
			 tag.category_code ILIKE $%d OR tag.category_name ILIKE $%d OR
			 tag.class_code ILIKE $%d OR tag.class_name ILIKE $%d OR
			 tag.major_code ILIKE $%d OR tag.major_name ILIKE $%d
		 )`, len(args), len(args), len(args), len(args), len(args), len(args)))
	}
	if len(tagConditions) > 0 {
		conditions = append(conditions, fmt.Sprintf(`EXISTS (
			SELECT 1
			FROM admission_major_tags tag
			WHERE tag.university_major_admission_id = uma.id
			  AND %s
		)`, strings.Join(tagConditions, " AND ")))
	}

	if filter.Is985 != nil {
		args = append(args, *filter.Is985)
		conditions = append(conditions, fmt.Sprintf("up.is_985 = $%d", len(args)))
	}
	if filter.Is211 != nil {
		args = append(args, *filter.Is211)
		conditions = append(conditions, fmt.Sprintf("up.is_211 = $%d", len(args)))
	}
	if filter.IsDoubleFirstClass != nil {
		args = append(args, *filter.IsDoubleFirstClass)
		conditions = append(conditions, fmt.Sprintf("up.is_double_first_class = $%d", len(args)))
	}
	if len(filter.Cities) > 0 {
		args = append(args, filter.Cities)
		conditions = append(conditions, fmt.Sprintf("up.city = ANY($%d)", len(args)))
	}
	if len(filter.ExcludeCities) > 0 {
		args = append(args, filter.ExcludeCities)
		conditions = append(conditions, fmt.Sprintf("(up.city IS NULL OR up.city <> ALL($%d))", len(args)))
	}
	if len(filter.Provinces) > 0 {
		args = append(args, filter.Provinces)
		conditions = append(conditions, fmt.Sprintf("up.region_code = ANY($%d)", len(args)))
	}
	if len(filter.ExcludeProvinces) > 0 {
		args = append(args, filter.ExcludeProvinces)
		conditions = append(conditions, fmt.Sprintf("(up.region_code IS NULL OR up.region_code <> ALL($%d))", len(args)))
	}
	if len(filter.SubjectCategories) > 0 {
		args = append(args, filter.SubjectCategories)
		conditions = append(conditions, fmt.Sprintf("ag.subject_category_code = ANY($%d)", len(args)))
	}

	return args, conditions
}

func resolveGroupBy(groupBy string) (groupCol string, nameCol string) {
	switch groupBy {
	case "province":
		return "up.region_code", "r.name"
	case "city":
		return "up.city", "up.city"
	case "subject_category":
		return "ag.subject_category_code", "ag.subject_category_code"
	case "university":
		return "u.university_code", "u.name"
	case "group":
		return "ag.group_code", "ag.group_code"
	default:
		return "up.region_code", "r.name"
	}
}

func containsMetric(metrics []string, target string) bool {
	for _, m := range metrics {
		if m == target {
			return true
		}
	}
	return false
}
