package admission

import (
	"context"
	"fmt"

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
	query := `
		SELECT id, university_code, name, COALESCE(normalized_name, '')
		FROM universities
		WHERE 1 = 1
	`
	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		query += fmt.Sprintf(" AND (university_code ILIKE $%d OR name ILIKE $%d OR normalized_name ILIKE $%d)", len(args), len(args), len(args))
	}
	query += " ORDER BY university_code, name"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list universities: %w", err)
	}
	defer rows.Close()

	universities := []UniversityResponse{}
	for rows.Next() {
		var u UniversityResponse
		if err := rows.Scan(&u.ID, &u.UniversityCode, &u.Name, &u.NormalizedName); err != nil {
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
		SELECT id, university_id, profile_year,
		       COALESCE(region_code, ''), COALESCE(city, ''),
		       COALESCE(ownership_type_code, ''), COALESCE(school_category_code, ''), COALESCE(education_level_code, ''),
		       is_985, is_211, is_double_first_class, is_national_key, is_provincial_key,
		       has_postgraduate_recommendation, postgraduate_recommendation_rate,
		       COALESCE(soft_rank, ''), COALESCE(alumni_rank, ''), COALESCE(difficulty_rank, ''),
		       doctoral_program_count, master_program_count, national_key_subject_count,
		       COALESCE(affiliation, ''), COALESCE(school_level_tags, ''), COALESCE(excellence_tags, '')
		FROM university_profiles
		WHERE university_id = $1 AND %s
	`, yearPredicate), args...).Scan(
		&p.ID,
		&p.UniversityID,
		&p.ProfileYear,
		&p.RegionCode,
		&p.City,
		&p.OwnershipTypeCode,
		&p.SchoolCategoryCode,
		&p.EducationLevelCode,
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
