package volunteerplan

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlanStore interface {
	ListByUser(ctx context.Context, userID int64) ([]*UserVolunteerPlan, error)
	GetByID(ctx context.Context, userID, planID int64) (*UserVolunteerPlan, error)
	CreateFromDraft(ctx context.Context, userID, draftID int64, title string, planJSON []byte) (*UserVolunteerPlan, error)
	GetByDraftID(ctx context.Context, userID, draftID int64) (*UserVolunteerPlan, error)
}

type planStore struct {
	pool *pgxpool.Pool
}

func NewPlanStore(pool *pgxpool.Pool) PlanStore {
	return &planStore{pool: pool}
}

func scanPlan(row pgx.Row) (*UserVolunteerPlan, error) {
	var p UserVolunteerPlan
	if err := row.Scan(&p.ID, &p.UserID, &p.Title, &p.SourceDraftID, &p.PlanJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *planStore) ListByUser(ctx context.Context, userID int64) ([]*UserVolunteerPlan, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, title, source_draft_id, plan_json, created_at, updated_at
		FROM user_volunteer_plans
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var plans []*UserVolunteerPlan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plans: %w", err)
	}
	return plans, nil
}

func (s *planStore) GetByID(ctx context.Context, userID, planID int64) (*UserVolunteerPlan, error) {
	p, err := scanPlan(s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, source_draft_id, plan_json, created_at, updated_at
		FROM user_volunteer_plans
		WHERE id = $1 AND user_id = $2
	`, planID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("get plan: %w", err)
	}
	return p, nil
}

func (s *planStore) GetByDraftID(ctx context.Context, userID, draftID int64) (*UserVolunteerPlan, error) {
	p, err := scanPlan(s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, source_draft_id, plan_json, created_at, updated_at
		FROM user_volunteer_plans
		WHERE user_id = $1 AND source_draft_id = $2
	`, userID, draftID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("get plan by draft: %w", err)
	}
	return p, nil
}

func (s *planStore) CreateFromDraft(ctx context.Context, userID, draftID int64, title string, planJSON []byte) (*UserVolunteerPlan, error) {
	p, err := scanPlan(s.pool.QueryRow(ctx, `
		INSERT INTO user_volunteer_plans (user_id, title, source_draft_id, plan_json)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, source_draft_id) DO NOTHING
		RETURNING id, user_id, title, source_draft_id, plan_json, created_at, updated_at
	`, userID, title, draftID, planJSON))
	if err == nil {
		return p, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return s.GetByDraftID(ctx, userID, draftID)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return nil, fmt.Errorf("insert plan: %w", err)
	}
	return nil, fmt.Errorf("insert plan: %w", err)
}
