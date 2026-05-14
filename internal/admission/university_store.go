package admission

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UniversityStore interface {
	ListUniversities(ctx context.Context, filter UniversityFilter) ([]UniversityResponse, error)
	GetUniversityProfile(ctx context.Context, universityID int64, profileYear *int) (*UniversityProfileResponse, error)
}

type universityStore struct {
	pool *pgxpool.Pool
}

func NewUniversityStore(pool *pgxpool.Pool) UniversityStore {
	return &universityStore{pool: pool}
}

func (s *universityStore) ListUniversities(ctx context.Context, filter UniversityFilter) ([]UniversityResponse, error) {
	args := []any{}
	conditions := []string{"1 = 1"}

	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.Query != "" {
		placeholder := addArg("%" + filter.Query + "%")
		conditions = append(conditions,
			fmt.Sprintf("(u.university_code ILIKE %s OR u.name ILIKE %s OR u.normalized_name ILIKE %s)", placeholder, placeholder, placeholder),
		)
	}
	if len(filter.RegionCodes) > 0 {
		placeholders := make([]string, 0, len(filter.RegionCodes))
		for _, code := range filter.RegionCodes {
			placeholders = append(placeholders, addArg(code))
		}
		conditions = append(conditions, fmt.Sprintf("up.region_code IN (%s)", strings.Join(placeholders, ",")))
	}
	if len(filter.SchoolCategoryCodes) > 0 {
		placeholders := make([]string, 0, len(filter.SchoolCategoryCodes))
		for _, code := range filter.SchoolCategoryCodes {
			placeholders = append(placeholders, addArg(code))
		}
		conditions = append(conditions, fmt.Sprintf("up.school_category_code IN (%s)", strings.Join(placeholders, ",")))
	}
	if len(filter.OwnershipTypeCodes) > 0 {
		placeholders := make([]string, 0, len(filter.OwnershipTypeCodes))
		for _, code := range filter.OwnershipTypeCodes {
			placeholders = append(placeholders, addArg(code))
		}
		conditions = append(conditions, fmt.Sprintf("up.ownership_type_code IN (%s)", strings.Join(placeholders, ",")))
	}
	if filter.EducationLevelCode != "" {
		conditions = append(conditions, fmt.Sprintf("up.education_level_code = %s", addArg(filter.EducationLevelCode)))
	}
	if filter.Is985 != nil {
		conditions = append(conditions, fmt.Sprintf("up.is_985 = %s", addArg(*filter.Is985)))
	}
	if filter.Is211 != nil {
		conditions = append(conditions, fmt.Sprintf("up.is_211 = %s", addArg(*filter.Is211)))
	}
	if filter.IsDoubleFirstClass != nil {
		conditions = append(conditions, fmt.Sprintf("up.is_double_first_class = %s", addArg(*filter.IsDoubleFirstClass)))
	}
	if filter.IsNationalKey != nil {
		conditions = append(conditions, fmt.Sprintf("up.is_national_key = %s", addArg(*filter.IsNationalKey)))
	}
	if filter.IsProvincialKey != nil {
		conditions = append(conditions, fmt.Sprintf("up.is_provincial_key = %s", addArg(*filter.IsProvincialKey)))
	}
	if filter.HasPostgraduateRecommendation != nil {
		conditions = append(conditions, fmt.Sprintf("up.has_postgraduate_recommendation = %s", addArg(*filter.HasPostgraduateRecommendation)))
	}

	hasProfileFilter := len(filter.RegionCodes) > 0 ||
		len(filter.SchoolCategoryCodes) > 0 ||
		len(filter.OwnershipTypeCodes) > 0 ||
		filter.EducationLevelCode != "" ||
		filter.Is985 != nil ||
		filter.Is211 != nil ||
		filter.IsDoubleFirstClass != nil ||
		filter.IsNationalKey != nil ||
		filter.IsProvincialKey != nil ||
		filter.HasPostgraduateRecommendation != nil

	joinKind := "LEFT JOIN"
	if hasProfileFilter {
		joinKind = "JOIN"
	}

	query := fmt.Sprintf(`
		SELECT
			u.id, u.university_code, u.name, COALESCE(u.normalized_name, ''),
			up.profile_year,
			COALESCE(up.region_code, ''), COALESCE(r.name, ''),
			COALESCE(up.city, ''),
			COALESCE(up.ownership_type_code, ''), COALESCE(ot.name, ''),
			COALESCE(up.school_category_code, ''), COALESCE(sc.name, ''),
			COALESCE(up.education_level_code, ''), COALESCE(el.name, ''),
			up.is_985, up.is_211, up.is_double_first_class, up.is_national_key, up.is_provincial_key,
			up.has_postgraduate_recommendation,
			COALESCE(up.soft_rank, ''),
			up.master_program_count, up.doctoral_program_count,
			COALESCE(up.affiliation, '')
		FROM universities u
		%s LATERAL (
			SELECT *
			FROM university_profiles
			WHERE university_id = u.id
			ORDER BY profile_year DESC
			LIMIT 1
		) up ON TRUE
		LEFT JOIN regions r ON r.code = up.region_code
		LEFT JOIN school_ownership_types ot ON ot.code = up.ownership_type_code
		LEFT JOIN school_categories sc ON sc.code = up.school_category_code
		LEFT JOIN education_levels el ON el.code = up.education_level_code
		WHERE %s
		ORDER BY u.university_code, u.name
	`, joinKind, strings.Join(conditions, " AND "))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list universities: %w", err)
	}
	defer rows.Close()

	universities := []UniversityResponse{}
	for rows.Next() {
		var u UniversityResponse
		if err := rows.Scan(
			&u.ID, &u.UniversityCode, &u.Name, &u.NormalizedName,
			&u.ProfileYear,
			&u.RegionCode, &u.RegionName,
			&u.City,
			&u.OwnershipTypeCode, &u.OwnershipTypeName,
			&u.SchoolCategoryCode, &u.SchoolCategoryName,
			&u.EducationLevelCode, &u.EducationLevelName,
			&u.Is985, &u.Is211, &u.IsDoubleFirstClass, &u.IsNationalKey, &u.IsProvincialKey,
			&u.HasPostgraduateRecommendation,
			&u.SoftRank,
			&u.MasterProgramCount, &u.DoctoralProgramCount,
			&u.Affiliation,
		); err != nil {
			return nil, fmt.Errorf("scan university: %w", err)
		}
		universities = append(universities, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate universities: %w", err)
	}
	return universities, nil
}

func (s *universityStore) GetUniversityProfile(ctx context.Context, universityID int64, profileYear *int) (*UniversityProfileResponse, error) {
	args := []any{universityID}
	yearPredicate := "profile_year = (SELECT MAX(profile_year) FROM university_profiles WHERE university_id = $1)"
	if profileYear != nil {
		args = append(args, *profileYear)
		yearPredicate = "$2 = profile_year"
	}

	var p UniversityProfileResponse
	err := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT up.id, up.university_id, up.profile_year,
		       COALESCE(up.region_code, ''), COALESCE(up.city, ''),
		       COALESCE(up.ownership_type_code, ''), COALESCE(ot.name, ''),
		       COALESCE(up.school_category_code, ''), COALESCE(sc.name, ''),
		       COALESCE(up.education_level_code, ''), COALESCE(el.name, ''),
		       up.is_985, up.is_211, up.is_double_first_class, up.is_national_key, up.is_provincial_key,
		       up.has_postgraduate_recommendation, up.postgraduate_recommendation_rate,
		       COALESCE(up.soft_rank, ''), COALESCE(up.alumni_rank, ''), COALESCE(up.difficulty_rank, ''),
		       up.doctoral_program_count, up.master_program_count, up.national_key_subject_count,
		       COALESCE(up.affiliation, ''), COALESCE(up.school_level_tags, ''), COALESCE(up.excellence_tags, '')
		FROM university_profiles up
		LEFT JOIN school_ownership_types ot ON ot.code = up.ownership_type_code
		LEFT JOIN school_categories sc ON sc.code = up.school_category_code
		LEFT JOIN education_levels el ON el.code = up.education_level_code
		WHERE up.university_id = $1 AND %s
	`, yearPredicate), args...).Scan(
		&p.ID,
		&p.UniversityID,
		&p.ProfileYear,
		&p.RegionCode,
		&p.City,
		&p.OwnershipTypeCode,
		&p.OwnershipTypeName,
		&p.SchoolCategoryCode,
		&p.SchoolCategoryName,
		&p.EducationLevelCode,
		&p.EducationLevelName,
		&p.Is985,
		&p.Is211,
		&p.IsDoubleFirstClass,
		&p.IsNationalKey,
		&p.IsProvincialKey,
		&p.HasPostgraduateRecommendation,
		&p.PostgraduateRecommendationRate,
		&p.SoftRank,
		&p.AlumniRank,
		&p.DifficultyRank,
		&p.DoctoralProgramCount,
		&p.MasterProgramCount,
		&p.NationalKeySubjectCount,
		&p.Affiliation,
		&p.SchoolLevelTags,
		&p.ExcellenceTags,
	)
	if err != nil {
		return nil, fmt.Errorf("get university profile: %w", err)
	}
	return &p, nil
}
