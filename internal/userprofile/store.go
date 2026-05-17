package userprofile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store defines data access for user_profiles. Two methods, both keyed by
// user_id (the table's PK + FK to users).
type Store interface {
	GetByUserID(ctx context.Context, userID int64) (*Profile, error)
	Upsert(ctx context.Context, userID int64, req *UpsertRequest, markCompleted bool) (*Profile, error)
}

type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &store{pool: pool}
}

// migration 008 之后，user_profiles 只剩 4 项核心信息 + 系统/状态字段。
const profileColumns = `user_id, region_code, subject_category_code, elective_subjects,
	total_score, completed_at, created_at, updated_at`

func scanProfile(row pgx.Row) (*Profile, error) {
	var p Profile
	if err := row.Scan(
		&p.UserID,
		&p.RegionCode,
		&p.SubjectCategoryCode,
		&p.ElectiveSubjects,
		&p.TotalScore,
		&p.CompletedAt,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *store) GetByUserID(ctx context.Context, userID int64) (*Profile, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+profileColumns+` FROM user_profiles WHERE user_id = $1`, userID)
	p, err := scanProfile(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProfileNotFound
		}
		return nil, fmt.Errorf("get user profile: %w", err)
	}
	return p, nil
}

// Upsert inserts a new profile row or replaces every column on the existing
// one. The service layer has already validated values; we just write them.
// markCompleted=true sets completed_at to NOW() on this call (when all 4
// required fields are present); markCompleted=false preserves the existing
// completed_at value to avoid silently un-completing a profile.
func (s *store) Upsert(ctx context.Context, userID int64, req *UpsertRequest, markCompleted bool) (*Profile, error) {
	// elective_subjects: 空切片转 nil，让 pgx 写 NULL 而不是 {}（CHECK 拒绝空数组）。
	electives := req.ElectiveSubjects
	if len(electives) == 0 {
		electives = nil
	}

	var completedExpr string
	args := []any{
		userID,
		req.RegionCode,
		req.SubjectCategoryCode,
		electives,
		req.TotalScore,
	}
	if markCompleted {
		completedExpr = "$6"
		args = append(args, time.Now())
	} else {
		// Preserve existing completed_at on UPDATE; default to NULL on first INSERT.
		completedExpr = "user_profiles.completed_at"
	}

	// In an INSERT path we cannot reference user_profiles.completed_at in the
	// VALUES list, so we split: for INSERT, set NULL unless markCompleted; for
	// UPDATE, COALESCE keeps the existing value unless markCompleted overrides.
	var insertCompleted string
	if markCompleted {
		insertCompleted = "$6"
	} else {
		insertCompleted = "NULL"
	}

	query := `
		INSERT INTO user_profiles (
			user_id, region_code, subject_category_code, elective_subjects, total_score, completed_at
		) VALUES ($1, $2, $3, $4, $5, ` + insertCompleted + `)
		ON CONFLICT (user_id) DO UPDATE SET
			region_code           = EXCLUDED.region_code,
			subject_category_code = EXCLUDED.subject_category_code,
			elective_subjects     = EXCLUDED.elective_subjects,
			total_score           = EXCLUDED.total_score,
			completed_at          = ` + completedExpr + `,
			updated_at            = NOW()
		RETURNING ` + profileColumns

	row := s.pool.QueryRow(ctx, query, args...)
	p, err := scanProfile(row)
	if err != nil {
		return nil, fmt.Errorf("upsert user profile: %w", err)
	}
	return p, nil
}
