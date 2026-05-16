package userprofile

import (
	"context"
	"encoding/json"
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

const profileColumns = `user_id, region_code, subject_category_code, total_score, provincial_rank,
	plan_size, priority_strategy, math_score, physics_score, chinese_score, english_score,
	preferences, completed_at, created_at, updated_at`

func scanProfile(row pgx.Row) (*Profile, error) {
	var p Profile
	var prefsRaw []byte
	if err := row.Scan(
		&p.UserID,
		&p.RegionCode,
		&p.SubjectCategoryCode,
		&p.TotalScore,
		&p.ProvincialRank,
		&p.PlanSize,
		&p.PriorityStrategy,
		&p.MathScore,
		&p.PhysicsScore,
		&p.ChineseScore,
		&p.EnglishScore,
		&prefsRaw,
		&p.CompletedAt,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(prefsRaw) > 0 {
		if err := json.Unmarshal(prefsRaw, &p.Preferences); err != nil {
			return nil, fmt.Errorf("decode preferences: %w", err)
		}
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
// required scalars are present); markCompleted=false preserves the existing
// completed_at value to avoid silently un-completing a profile when the user
// edits an optional field.
func (s *store) Upsert(ctx context.Context, userID int64, req *UpsertRequest, markCompleted bool) (*Profile, error) {
	prefs := Preferences{}
	if req.Preferences != nil {
		prefs = *req.Preferences
	}
	prefsRaw, err := json.Marshal(prefs)
	if err != nil {
		return nil, fmt.Errorf("encode preferences: %w", err)
	}

	var completedExpr string
	args := []any{
		userID,
		req.RegionCode,
		req.SubjectCategoryCode,
		req.TotalScore,
		req.ProvincialRank,
		req.PlanSize,
		req.PriorityStrategy,
		req.MathScore,
		req.PhysicsScore,
		req.ChineseScore,
		req.EnglishScore,
		prefsRaw,
	}
	if markCompleted {
		completedExpr = "$13"
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
		insertCompleted = "$13"
	} else {
		insertCompleted = "NULL"
	}

	query := `
		INSERT INTO user_profiles (
			user_id, region_code, subject_category_code, total_score, provincial_rank,
			plan_size, priority_strategy, math_score, physics_score, chinese_score, english_score,
			preferences, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, ` + insertCompleted + `)
		ON CONFLICT (user_id) DO UPDATE SET
			region_code           = EXCLUDED.region_code,
			subject_category_code = EXCLUDED.subject_category_code,
			total_score           = EXCLUDED.total_score,
			provincial_rank       = EXCLUDED.provincial_rank,
			plan_size             = EXCLUDED.plan_size,
			priority_strategy     = EXCLUDED.priority_strategy,
			math_score            = EXCLUDED.math_score,
			physics_score         = EXCLUDED.physics_score,
			chinese_score         = EXCLUDED.chinese_score,
			english_score         = EXCLUDED.english_score,
			preferences           = EXCLUDED.preferences,
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
