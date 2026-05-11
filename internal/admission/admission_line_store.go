package admission

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AdmissionLineStore interface { //nolint:revive // Matches route constructor naming.
	ListAdmissionLines(ctx context.Context, filter *AdmissionLineFilter) ([]AdmissionLineResponse, error)
}

type admissionLineStore struct {
	pool *pgxpool.Pool
}

func NewAdmissionLineStore(pool *pgxpool.Pool) AdmissionLineStore {
	return &admissionLineStore{pool: pool}
}

func (s *admissionLineStore) ListAdmissionLines(ctx context.Context, filter *AdmissionLineFilter) ([]AdmissionLineResponse, error) {
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

	query := fmt.Sprintf(`
		SELECT
			uma.id, ag.id, uma.id,
			u.id, u.university_code, u.name,
			ag.admission_year, ag.region_code,
			ag.subject_category_code, COALESCE(sc.name, ''),
			ag.batch_code, COALESCE(b.name, ''),
			ag.group_code, COALESCE(ag.group_major_names, ''),
			COALESCE(ag.subject_requirement_code, ''), COALESCE(sr.name, ''),
			COALESCE(age.batch_remark, ''), age.group_min_score, age.group_min_rank,
			age.equivalent_min_score_2024, age.equivalent_min_score_2023, age.equivalent_min_score_2022,
			COALESCE(age.subject_change_2024, ''),
			uma.local_major_code, uma.local_major_name,
			uma.plan_count, uma.admitted_count, uma.min_score, uma.min_rank,
			uma.max_score, uma.max_rank, uma.equivalent_min_score, uma.tuition,
			COALESCE(uma.duration, ''), COALESCE(uma.admission_remark, ''),
			COALESCE(uma.major_intro, ''), COALESCE(uma.training_goal, ''),
			COALESCE(uma.subject_study_requirement, ''), COALESCE(uma.main_courses, ''),
			COALESCE(uma.postgraduate_direction, ''), COALESCE(uma.employment_direction, ''),
			COALESCE(ump.discipline_category, ''), COALESCE(ump.first_level_discipline, ''),
			COALESCE(ump.fourth_round_subject_eval, ''), COALESCE(ump.double_first_class_subject, ''),
			COALESCE(ump.soft_major_grade, ''), ump.major_evaluation_score, COALESCE(ump.major_rank, ''),
			ump.is_national_feature, COALESCE(ump.corresponding_master_majors, ''),
			COALESCE(ump.corresponding_doctoral_majors, ''),
			upp.master_major_count, COALESCE(upp.master_major_names, ''),
			upp.doctoral_major_count, COALESCE(upp.doctoral_major_names, '')
		FROM university_major_admissions uma
		JOIN admission_groups ag ON ag.id = uma.admission_group_id
		JOIN universities u ON u.id = ag.university_id
		LEFT JOIN subject_categories sc ON sc.code = ag.subject_category_code
		LEFT JOIN batches b ON b.code = ag.batch_code
		LEFT JOIN subject_requirements sr ON sr.code = ag.subject_requirement_code
		LEFT JOIN admission_group_extensions age ON age.admission_group_id = ag.id
		LEFT JOIN university_major_profiles ump ON ump.university_major_admission_id = uma.id
		LEFT JOIN LATERAL (
			SELECT latest_upp.*
			FROM university_postgraduate_profiles latest_upp
			WHERE latest_upp.university_id = u.id
			ORDER BY latest_upp.profile_year DESC
			LIMIT 1
		) upp ON true
		LEFT JOIN LATERAL (
			SELECT latest_up.*
			FROM university_profiles latest_up
			WHERE latest_up.university_id = u.id
			ORDER BY latest_up.profile_year DESC
			LIMIT 1
		) up ON true
		WHERE %s
		ORDER BY u.name, ag.group_code, uma.local_major_code
	`, strings.Join(conditions, " AND "))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list admission lines: %w", err)
	}
	defer rows.Close()

	lines := []AdmissionLineResponse{}
	for rows.Next() {
		var line AdmissionLineResponse
		if err := rows.Scan(
			&line.ID,
			&line.AdmissionGroupID,
			&line.UniversityMajorLineID,
			&line.UniversityID,
			&line.UniversityCode,
			&line.UniversityName,
			&line.AdmissionYear,
			&line.RegionCode,
			&line.SubjectCategory,
			&line.SubjectCategoryName,
			&line.BatchCode,
			&line.BatchName,
			&line.GroupCode,
			&line.GroupMajorNames,
			&line.SubjectRequirementCode,
			&line.SubjectRequirementName,
			&line.BatchRemark,
			&line.GroupMinScore,
			&line.GroupMinRank,
			&line.EquivalentMinScore2024,
			&line.EquivalentMinScore2023,
			&line.EquivalentMinScore2022,
			&line.SubjectChange2024,
			&line.LocalMajorCode,
			&line.LocalMajorName,
			&line.PlanCount,
			&line.AdmittedCount,
			&line.MinScore,
			&line.MinRank,
			&line.MaxScore,
			&line.MaxRank,
			&line.EquivalentMinScore,
			&line.Tuition,
			&line.Duration,
			&line.AdmissionRemark,
			&line.MajorIntro,
			&line.TrainingGoal,
			&line.SubjectStudyRequirement,
			&line.MainCourses,
			&line.PostgraduateDirection,
			&line.EmploymentDirection,
			&line.DisciplineCategory,
			&line.FirstLevelDiscipline,
			&line.FourthRoundSubjectEval,
			&line.DoubleFirstClassSubject,
			&line.SoftMajorGrade,
			&line.MajorEvaluationScore,
			&line.MajorRank,
			&line.IsNationalFeature,
			&line.CorrespondingMasterMajors,
			&line.CorrespondingDoctoralMajors,
			&line.MasterMajorCount,
			&line.MasterMajorNames,
			&line.DoctoralMajorCount,
			&line.DoctoralMajorNames,
		); err != nil {
			return nil, fmt.Errorf("scan admission line: %w", err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admission lines: %w", err)
	}
	return lines, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
