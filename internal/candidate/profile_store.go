package candidate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const profileColumns = `
	id, user_id, real_name, candidate_id_card_enc, candidate_id_card_hash,
	candidate_phone, province_id, city_id, county_id, graduation_school_name,
	grade, candidate_type, gender, ethnicity, color_vision,
	status, is_deleted, deleted_at, created_at, updated_at
`

// ProfileStore defines candidate profile data access operations.
type ProfileStore interface {
	Create(ctx context.Context, input *CreateProfileInput) (*Profile, error)
	GetByID(ctx context.Context, id int64) (*Profile, error)
	ListByOwner(ctx context.Context, ownerUserID int64) ([]*Profile, error)
	ListByOwnerOrBoundUser(ctx context.Context, userID int64) ([]*Profile, error)
	Update(ctx context.Context, id int64, input *UpdateProfileInput) (*Profile, error)
	SoftDelete(ctx context.Context, id int64) error

	GetByIDCardHash(ctx context.Context, hash string) (*Profile, error)
	GetByPhone(ctx context.Context, phone string) (*Profile, error)

	GetOwnerUserID(ctx context.Context, profileID int64) (int64, error)
}

type profileStore struct {
	pool *pgxpool.Pool
}

// NewProfileStore constructs a profile store backed by pgx.
func NewProfileStore(pool *pgxpool.Pool) ProfileStore {
	return &profileStore{pool: pool}
}

func scanProfileRow(row pgx.Row) (*Profile, error) {
	var p Profile
	if err := row.Scan(
		&p.ID, &p.UserID, &p.RealName, &p.CandidateIDCardEnc, &p.CandidateIDCardHash,
		&p.CandidatePhone, &p.ProvinceID, &p.CityID, &p.CountyID, &p.GraduationSchoolName,
		&p.Grade, &p.CandidateType, &p.Gender, &p.Ethnicity, &p.ColorVision,
		&p.Status, &p.IsDeleted, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *profileStore) Create(ctx context.Context, input *CreateProfileInput) (*Profile, error) {
	query := fmt.Sprintf(`
		INSERT INTO candidate_profiles (
			user_id, real_name, candidate_id_card_enc, candidate_id_card_hash,
			candidate_phone, province_id, city_id, county_id, graduation_school_name,
			grade, candidate_type, gender, ethnicity, color_vision, status
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
		RETURNING %s
	`, profileColumns)

	row := s.pool.QueryRow(ctx, query,
		input.UserID, input.RealName, input.CandidateIDCardEnc, input.CandidateIDCardHash,
		input.CandidatePhone, input.ProvinceID, input.CityID, input.CountyID, input.GraduationSchoolName,
		input.Grade, input.CandidateType, input.Gender, input.Ethnicity, input.ColorVision, input.Status,
	)
	p, err := scanProfileRow(row)
	if err != nil {
		return nil, fmt.Errorf("create candidate profile: %w", err)
	}
	return p, nil
}

func (s *profileStore) GetByID(ctx context.Context, id int64) (*Profile, error) {
	query := fmt.Sprintf(`SELECT %s FROM candidate_profiles WHERE id = $1 AND is_deleted = false`, profileColumns)
	p, err := scanProfileRow(s.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get candidate profile: %w", err)
	}
	return p, nil
}

func (s *profileStore) ListByOwner(ctx context.Context, ownerUserID int64) ([]*Profile, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM candidate_profiles
		WHERE user_id = $1 AND is_deleted = false
		ORDER BY created_at DESC
	`, profileColumns)
	return s.queryProfiles(ctx, query, ownerUserID)
}

// ListByOwnerOrBoundUser returns profiles owned directly by the user OR by their
// bound counterpart through user_bindings (parent↔student account binding).
// userType is informational; the OR clause covers both directions.
func (s *profileStore) ListByOwnerOrBoundUser(ctx context.Context, userID int64) ([]*Profile, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM candidate_profiles
		WHERE is_deleted = false
		  AND (
			user_id = $1
			OR user_id IN (
				SELECT student_id FROM user_bindings WHERE parent_id = $1
				UNION
				SELECT parent_id  FROM user_bindings WHERE student_id = $1
			)
		  )
		ORDER BY created_at DESC
	`, profileColumns)
	return s.queryProfiles(ctx, query, userID)
}

func (s *profileStore) queryProfiles(ctx context.Context, query string, args ...any) ([]*Profile, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list candidate profiles: %w", err)
	}
	defer rows.Close()

	out := []*Profile{}
	for rows.Next() {
		p, err := scanProfileRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan candidate profile: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidate profiles: %w", err)
	}
	return out, nil
}

func (s *profileStore) Update(ctx context.Context, id int64, input *UpdateProfileInput) (*Profile, error) {
	sets, args, err := buildUpdateSets(input)
	if err != nil {
		return nil, err
	}
	if len(sets) == 0 {
		return s.GetByID(ctx, id)
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)
	idIdx := len(args)

	query := fmt.Sprintf(`
		UPDATE candidate_profiles
		SET %s
		WHERE id = $%d AND is_deleted = false
		RETURNING %s
	`, strings.Join(sets, ", "), idIdx, profileColumns)

	p, err := scanProfileRow(s.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("update candidate profile: %w", err)
	}
	return p, nil
}

func buildUpdateSets(in *UpdateProfileInput) ([]string, []any, error) {
	var sets []string
	var args []any
	idx := 1

	add := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, val)
		idx++
	}

	if in.RealName != nil {
		add("real_name", *in.RealName)
	}
	if in.UpdateIDCardFields {
		add("candidate_id_card_enc", in.CandidateIDCardEnc)
		add("candidate_id_card_hash", in.CandidateIDCardHash)
	}
	if in.CandidatePhone != nil {
		add("candidate_phone", *in.CandidatePhone)
	}
	if in.ProvinceID != nil {
		add("province_id", *in.ProvinceID)
	}
	if in.CityID != nil {
		add("city_id", *in.CityID)
	}
	if in.CountyID != nil {
		add("county_id", *in.CountyID)
	}
	if in.GraduationSchoolName != nil {
		add("graduation_school_name", *in.GraduationSchoolName)
	}
	if in.Grade != nil {
		add("grade", *in.Grade)
	}
	if in.CandidateType != nil {
		add("candidate_type", *in.CandidateType)
	}
	if in.Gender != nil {
		add("gender", *in.Gender)
	}
	if in.Ethnicity != nil {
		add("ethnicity", *in.Ethnicity)
	}
	if in.ColorVision != nil {
		add("color_vision", *in.ColorVision)
	}
	if in.Status != nil {
		add("status", *in.Status)
	}

	return sets, args, nil
}

func (s *profileStore) SoftDelete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE candidate_profiles
		SET is_deleted = true, deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND is_deleted = false
	`, id)
	if err != nil {
		return fmt.Errorf("soft delete candidate profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *profileStore) GetByIDCardHash(ctx context.Context, hash string) (*Profile, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM candidate_profiles
		WHERE candidate_id_card_hash = $1 AND is_deleted = false
		ORDER BY created_at DESC
		LIMIT 1
	`, profileColumns)
	p, err := scanProfileRow(s.pool.QueryRow(ctx, query, hash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get candidate profile by id_card hash: %w", err)
	}
	return p, nil
}

func (s *profileStore) GetByPhone(ctx context.Context, phone string) (*Profile, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM candidate_profiles
		WHERE candidate_phone = $1 AND is_deleted = false
		ORDER BY created_at DESC
		LIMIT 1
	`, profileColumns)
	p, err := scanProfileRow(s.pool.QueryRow(ctx, query, phone))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get candidate profile by phone: %w", err)
	}
	return p, nil
}

func (s *profileStore) GetOwnerUserID(ctx context.Context, profileID int64) (int64, error) {
	var userID int64
	err := s.pool.QueryRow(ctx, `
		SELECT user_id FROM candidate_profiles
		WHERE id = $1 AND is_deleted = false
	`, profileID).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, pgx.ErrNoRows
		}
		return 0, fmt.Errorf("get owner user_id: %w", err)
	}
	return userID, nil
}
