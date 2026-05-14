package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	GetTrend(ctx context.Context, filter *TrendFilter) (*TrendResponse, error)
	GetGroupComparison(ctx context.Context, filter *GroupComparisonFilter) (*GroupComparisonResponse, error)
	GetMajorDistribution(ctx context.Context, filter *MajorDistributionFilter) (*MajorDistributionResponse, error)
	GetMajorComparison(ctx context.Context, filter *MajorComparisonFilter) (*MajorComparisonResponse, error)
}

type analysisStore struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &analysisStore{pool: pool}
}

func (s *analysisStore) GetTrend(ctx context.Context, filter *TrendFilter) (*TrendResponse, error) {
	if filter.Years <= 0 {
		filter.Years = 5
	}

	var universityName string
	err := s.pool.QueryRow(ctx, `SELECT name FROM universities WHERE id = $1`, filter.UniversityID).Scan(&universityName)
	if err != nil {
		return nil, fmt.Errorf("fetch university: %w", err)
	}

	resp := &TrendResponse{
		UniversityID:   filter.UniversityID,
		UniversityName: universityName,
		GroupCode:      filter.GroupCode,
		LocalMajorCode: filter.LocalMajorCode,
	}

	args := []any{filter.UniversityID, filter.Years}
	conditions := []string{"ag.university_id = $1"}

	if filter.GroupCode != "" {
		args = append(args, filter.GroupCode)
		conditions = append(conditions, fmt.Sprintf("ag.group_code = $%d", len(args)))
	}
	if filter.LocalMajorCode != "" {
		args = append(args, filter.LocalMajorCode)
		conditions = append(conditions, fmt.Sprintf("uma.local_major_code = $%d", len(args)))
	}

	var query string
	if filter.LocalMajorCode != "" {
		// Specific major: one row per year (raw data).
		query = fmt.Sprintf(`
			SELECT
				ag.admission_year,
				uma.plan_count,
				uma.admitted_count,
				uma.min_score,
				uma.min_rank,
				uma.equivalent_min_score,
				uma.local_major_name
			FROM admission_groups ag
			JOIN university_major_admissions uma ON uma.admission_group_id = ag.id
			WHERE %s
			  AND ag.admission_year >= (
				  SELECT COALESCE(MAX(admission_year), 0) - $2 + 1
				  FROM admission_groups
				  WHERE university_id = $1
			  )
			ORDER BY ag.admission_year DESC
		`, strings.Join(conditions, " AND "))
	} else {
		// University or group level: aggregate per year.
		query = fmt.Sprintf(`
			SELECT
				ag.admission_year,
				COALESCE(SUM(uma.plan_count), 0)::int,
				SUM(uma.admitted_count),
				AVG(uma.min_score)::int,
				MIN(uma.min_rank)
			FROM admission_groups ag
			LEFT JOIN university_major_admissions uma ON uma.admission_group_id = ag.id
			WHERE %s
			  AND ag.admission_year >= (
				  SELECT COALESCE(MAX(admission_year), 0) - $2 + 1
				  FROM admission_groups
				  WHERE university_id = $1
			  )
			GROUP BY ag.admission_year
			ORDER BY ag.admission_year DESC
		`, strings.Join(conditions, " AND "))
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("trend query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var year int
		var ty TrendYear

		if filter.LocalMajorCode != "" {
			var planCount, admittedCount, minScore, minRank, equivalentMinScore pgtype.Int4
			var localMajorName string
			if err := rows.Scan(
				&year, &planCount, &admittedCount, &minScore, &minRank,
				&equivalentMinScore, &localMajorName,
			); err != nil {
				return nil, fmt.Errorf("scan trend row: %w", err)
			}
			ty.Year = year
			ty.PlanCount = int4Ptr(planCount)
			ty.AdmittedCount = int4Ptr(admittedCount)
			ty.MinScore = int4Ptr(minScore)
			ty.MinRank = int4Ptr(minRank)
			resp.LocalMajorName = localMajorName
			if equivalentMinScore.Valid {
				ty.EquivalentScores = append(ty.EquivalentScores, EquivalentScore{
					ReferenceYear: year,
					Score:         intPtr(int(equivalentMinScore.Int32)),
				})
			}
		} else {
			var planCount, admittedCount, minScore, minRank pgtype.Int4
			if err := rows.Scan(
				&year, &planCount, &admittedCount, &minScore, &minRank,
			); err != nil {
				return nil, fmt.Errorf("scan trend row: %w", err)
			}
			ty.Year = year
			ty.PlanCount = int4Ptr(planCount)
			ty.AdmittedCount = int4Ptr(admittedCount)
			ty.MinScore = int4Ptr(minScore)
			ty.MinRank = int4Ptr(minRank)
		}

		resp.Years = append(resp.Years, ty)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trend rows: %w", err)
	}

	return resp, nil
}

func (s *analysisStore) GetGroupComparison(ctx context.Context, filter *GroupComparisonFilter) (*GroupComparisonResponse, error) {
	var universityName string
	err := s.pool.QueryRow(ctx, `SELECT name FROM universities WHERE id = $1`, filter.UniversityID).Scan(&universityName)
	if err != nil {
		return nil, fmt.Errorf("fetch university: %w", err)
	}

	admissionYear := filter.AdmissionYear
	if admissionYear == nil {
		var latestYear int
		err := s.pool.QueryRow(ctx, `
			SELECT COALESCE(MAX(admission_year), 0)
			FROM admission_groups
			WHERE university_id = $1
			  AND ($2::text IS NULL OR region_code = $2)
			  AND ($3::text IS NULL OR subject_category_code = $3)
		`, filter.UniversityID, nullableString(filter.RegionCode), nullableString(filter.SubjectCategoryCode)).Scan(&latestYear)
		if err != nil {
			return nil, fmt.Errorf("resolve latest year: %w", err)
		}
		admissionYear = &latestYear
	}

	resp := &GroupComparisonResponse{
		UniversityID:   filter.UniversityID,
		UniversityName: universityName,
		AdmissionYear:  *admissionYear,
	}

	args := []any{filter.UniversityID, *admissionYear}
	conditions := []string{"ag.university_id = $1", "ag.admission_year = $2"}

	if filter.RegionCode != "" {
		args = append(args, filter.RegionCode)
		conditions = append(conditions, fmt.Sprintf("ag.region_code = $%d", len(args)))
	}
	if filter.SubjectCategoryCode != "" {
		args = append(args, filter.SubjectCategoryCode)
		conditions = append(conditions, fmt.Sprintf("ag.subject_category_code = $%d", len(args)))
	}

	query := fmt.Sprintf(`
		SELECT
			ag.group_code,
			COALESCE(ag.group_major_names, ''),
			COALESCE(sr.name, ''),
			COALESCE(b.name, ''),
			COALESCE(SUM(uma.plan_count), 0)::int,
			SUM(uma.admitted_count),
			age.group_min_score,
			age.group_min_rank,
			COUNT(uma.id)::int
		FROM admission_groups ag
		LEFT JOIN subject_requirements sr ON sr.code = ag.subject_requirement_code
		LEFT JOIN batches b ON b.code = ag.batch_code
		LEFT JOIN university_major_admissions uma ON uma.admission_group_id = ag.id
		LEFT JOIN admission_group_extensions age ON age.admission_group_id = ag.id
		WHERE %s
		GROUP BY ag.id, ag.group_code, ag.group_major_names, sr.name, b.name,
			age.group_min_score, age.group_min_rank
		ORDER BY COALESCE(age.group_min_rank, 99999999), ag.group_code
	`, strings.Join(conditions, " AND "))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("group comparison query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item GroupComparisonItem
		var admittedCount pgtype.Int4
		if err := rows.Scan(
			&item.GroupCode,
			&item.GroupMajorNames,
			&item.SubjectRequirementName,
			&item.BatchName,
			&item.PlanCount,
			&admittedCount,
			&item.GroupMinScore,
			&item.GroupMinRank,
			&item.MajorCount,
		); err != nil {
			return nil, fmt.Errorf("scan group comparison row: %w", err)
		}
		item.AdmittedCount = int4Ptr(admittedCount)
		resp.Groups = append(resp.Groups, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group comparison rows: %w", err)
	}

	return resp, nil
}

func (s *analysisStore) GetMajorDistribution(ctx context.Context, filter *MajorDistributionFilter) (*MajorDistributionResponse, error) {
	admissionYear := filter.AdmissionYear
	if admissionYear == nil {
		var latestYear int
		args := []any{filter.UniversityID}
		conditions := []string{"university_id = $1"}
		if filter.GroupCode != "" {
			args = append(args, filter.GroupCode)
			conditions = append(conditions, fmt.Sprintf("group_code = $%d", len(args)))
		}
		err := s.pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT COALESCE(MAX(admission_year), 0)
			FROM admission_groups
			WHERE %s
		`, strings.Join(conditions, " AND ")), args...).Scan(&latestYear)
		if err != nil {
			return nil, fmt.Errorf("resolve latest year: %w", err)
		}
		admissionYear = &latestYear
	}

	resp := &MajorDistributionResponse{
		UniversityID:  filter.UniversityID,
		AdmissionYear: *admissionYear,
		GroupCode:     filter.GroupCode,
	}

	args := []any{filter.UniversityID, *admissionYear}
	conditions := []string{"ag.university_id = $1", "ag.admission_year = $2"}

	if filter.GroupCode != "" {
		args = append(args, filter.GroupCode)
		conditions = append(conditions, fmt.Sprintf("ag.group_code = $%d", len(args)))
	}
	if filter.RegionCode != "" {
		args = append(args, filter.RegionCode)
		conditions = append(conditions, fmt.Sprintf("ag.region_code = $%d", len(args)))
	}
	if filter.SubjectCategoryCode != "" {
		args = append(args, filter.SubjectCategoryCode)
		conditions = append(conditions, fmt.Sprintf("ag.subject_category_code = $%d", len(args)))
	}

	query := fmt.Sprintf(`
		SELECT
			uma.local_major_code,
			uma.local_major_name,
			uma.plan_count,
			uma.admitted_count,
			uma.min_score,
			uma.min_rank,
			uma.tuition
		FROM admission_groups ag
		JOIN university_major_admissions uma ON uma.admission_group_id = ag.id
		WHERE %s
		ORDER BY uma.min_rank NULLS LAST, uma.local_major_name
	`, strings.Join(conditions, " AND "))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("major distribution query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item MajorDistributionItem
		var planCount, admittedCount, minScore, minRank, tuition pgtype.Int4
		if err := rows.Scan(
			&item.LocalMajorCode,
			&item.LocalMajorName,
			&planCount,
			&admittedCount,
			&minScore,
			&minRank,
			&tuition,
		); err != nil {
			return nil, fmt.Errorf("scan major distribution row: %w", err)
		}
		item.PlanCount = int4Ptr(planCount)
		item.AdmittedCount = int4Ptr(admittedCount)
		item.MinScore = int4Ptr(minScore)
		item.MinRank = int4Ptr(minRank)
		item.Tuition = int4Ptr(tuition)
		resp.Majors = append(resp.Majors, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate major distribution rows: %w", err)
	}

	return resp, nil
}

func (s *analysisStore) GetMajorComparison(ctx context.Context, filter *MajorComparisonFilter) (*MajorComparisonResponse, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	admissionYear := filter.AdmissionYear
	if admissionYear == nil {
		var latestYear int
		args := []any{nullableString(filter.RegionCode), nullableString(filter.SubjectCategoryCode)}
		err := s.pool.QueryRow(ctx, `
			SELECT COALESCE(MAX(admission_year), 0)
			FROM admission_groups
			WHERE ($1::text IS NULL OR region_code = $1)
			  AND ($2::text IS NULL OR subject_category_code = $2)
		`, args...).Scan(&latestYear)
		if err != nil {
			return nil, fmt.Errorf("resolve latest year: %w", err)
		}
		admissionYear = &latestYear
	}

	resp := &MajorComparisonResponse{
		LocalMajorName: filter.LocalMajorName,
		AdmissionYear:  *admissionYear,
	}

	args := []any{"%" + filter.LocalMajorName + "%", *admissionYear, filter.Limit}
	conditions := []string{"uma.local_major_name ILIKE $1", "ag.admission_year = $2"}

	if filter.RegionCode != "" {
		args = append(args, filter.RegionCode)
		conditions = append(conditions, fmt.Sprintf("ag.region_code = $%d", len(args)))
	}
	if filter.SubjectCategoryCode != "" {
		args = append(args, filter.SubjectCategoryCode)
		conditions = append(conditions, fmt.Sprintf("ag.subject_category_code = $%d", len(args)))
	}

	query := fmt.Sprintf(`
		SELECT
			u.id,
			u.name,
			ag.group_code,
			uma.local_major_code,
			uma.plan_count,
			uma.admitted_count,
			uma.min_score,
			uma.min_rank,
			uma.equivalent_min_score
		FROM university_major_admissions uma
		JOIN admission_groups ag ON ag.id = uma.admission_group_id
		JOIN universities u ON u.id = ag.university_id
		WHERE %s
		ORDER BY uma.min_rank NULLS LAST, u.name
		LIMIT $3
	`, strings.Join(conditions, " AND "))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("major comparison query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item MajorComparisonItem
		var planCount, admittedCount, minScore, minRank, equivalentMinScore pgtype.Int4
		if err := rows.Scan(
			&item.UniversityID,
			&item.UniversityName,
			&item.GroupCode,
			&item.LocalMajorCode,
			&planCount,
			&admittedCount,
			&minScore,
			&minRank,
			&equivalentMinScore,
		); err != nil {
			return nil, fmt.Errorf("scan major comparison row: %w", err)
		}
		item.PlanCount = int4Ptr(planCount)
		item.AdmittedCount = int4Ptr(admittedCount)
		item.MinScore = int4Ptr(minScore)
		item.MinRank = int4Ptr(minRank)
		item.EquivalentMinScore = int4Ptr(equivalentMinScore)
		resp.Items = append(resp.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate major comparison rows: %w", err)
	}

	return resp, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func int4Ptr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int32)
	return &i
}

func intPtr(v int) *int {
	return &v
}
