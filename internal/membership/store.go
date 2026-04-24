package membership

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPlanNotFound       = errors.New("membership plan not found")
	ErrPlanNotPurchasable = errors.New("membership plan is not purchasable")
	ErrGrantAlreadyExists = errors.New("membership grant already exists")
)

// Store defines membership data access.
type Store interface {
	ListActivePlans(ctx context.Context) ([]*Plan, error)
	GetActivePlanByCode(ctx context.Context, planCode string) (*Plan, error)
	GetCurrentMembership(ctx context.Context, userID int64) (*UserMembership, error)
	HasActiveMembership(ctx context.Context, userID int64, now time.Time) (bool, error)
	GrantMembership(ctx context.Context, req GrantRequest) (*UserMembership, *Grant, bool, error)
}

type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &store{pool: pool}
}

func scanPlan(row pgx.Row) (*Plan, error) {
	var p Plan
	if err := row.Scan(
		&p.ID,
		&p.PlanCode,
		&p.PlanName,
		&p.MembershipLevel,
		&p.DurationDays,
		&p.PriceAmount,
		&p.Currency,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

func scanMembership(row pgx.Row) (*UserMembership, error) {
	var m UserMembership
	if err := row.Scan(
		&m.ID,
		&m.UserID,
		&m.MembershipLevel,
		&m.Status,
		&m.StartedAt,
		&m.EndsAt,
		&m.LastOrderID,
		&m.CreatedAt,
		&m.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *store) ListActivePlans(ctx context.Context) ([]*Plan, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, plan_code, plan_name, membership_level, duration_days, price_amount, currency, status, created_at, updated_at
		FROM membership_plans
		WHERE status = 'active'
		ORDER BY duration_days ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list active membership plans: %w", err)
	}
	defer rows.Close()

	plans := []*Plan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan membership plan: %w", err)
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate membership plans: %w", err)
	}
	return plans, nil
}

func (s *store) GetActivePlanByCode(ctx context.Context, planCode string) (*Plan, error) {
	p, err := scanPlan(s.pool.QueryRow(ctx, `
		SELECT id, plan_code, plan_name, membership_level, duration_days, price_amount, currency, status, created_at, updated_at
		FROM membership_plans
		WHERE plan_code = $1
	`, planCode))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("get membership plan: %w", err)
	}
	if p.Status != PlanStatusActive {
		return nil, ErrPlanNotPurchasable
	}
	return p, nil
}

func (s *store) GetCurrentMembership(ctx context.Context, userID int64) (*UserMembership, error) {
	m, err := scanMembership(s.pool.QueryRow(ctx, `
		SELECT id, user_id, membership_level, status, started_at, ends_at, last_order_id, created_at, updated_at
		FROM user_memberships
		WHERE user_id = $1
	`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &UserMembership{
				UserID:          userID,
				MembershipLevel: LevelPremium,
				Status:          MembershipStatusInactive,
			}, nil
		}
		return nil, fmt.Errorf("get current membership: %w", err)
	}
	return normalizeMembershipStatus(m, time.Now()), nil
}

func (s *store) HasActiveMembership(ctx context.Context, userID int64, now time.Time) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM user_memberships
			WHERE user_id = $1
			  AND status = 'active'
			  AND ends_at IS NOT NULL
			  AND ends_at > $2
		)
	`, userID, now).Scan(&exists); err != nil {
		return false, fmt.Errorf("check active membership: %w", err)
	}
	if !exists {
		_, _ = s.pool.Exec(ctx, `
			UPDATE user_memberships
			SET status = 'expired', updated_at = NOW()
			WHERE user_id = $1 AND status = 'active' AND ends_at <= $2
		`, userID, now)
	}
	return exists, nil
}

func (s *store) GrantMembership(ctx context.Context, req GrantRequest) (*UserMembership, *Grant, bool, error) {
	if req.Now.IsZero() {
		req.Now = time.Now()
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, false, fmt.Errorf("begin grant membership tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existingGrant Grant
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, payment_order_id, source_type, action, duration_days, starts_at, ends_at, idempotency_key, created_at
		FROM membership_grants
		WHERE idempotency_key = $1
	`, req.IdempotencyKey).Scan(
		&existingGrant.ID,
		&existingGrant.UserID,
		&existingGrant.PaymentOrderID,
		&existingGrant.SourceType,
		&existingGrant.Action,
		&existingGrant.DurationDays,
		&existingGrant.StartsAt,
		&existingGrant.EndsAt,
		&existingGrant.IdempotencyKey,
		&existingGrant.CreatedAt,
	)
	if err == nil {
		m, getErr := membershipByUserTx(ctx, tx, req.UserID)
		if getErr != nil {
			return nil, nil, false, getErr
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, nil, false, fmt.Errorf("commit existing grant tx: %w", err)
		}
		return normalizeMembershipStatus(m, req.Now), &existingGrant, false, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, false, fmt.Errorf("lookup membership grant: %w", err)
	}

	current, err := membershipByUserForUpdateTx(ctx, tx, req.UserID)
	if err != nil {
		return nil, nil, false, err
	}

	startsAt := req.Now
	action := GrantActionActivate
	if current != nil && current.EndsAt != nil && current.EndsAt.After(req.Now) {
		startsAt = *current.EndsAt
		action = GrantActionRenew
	} else if current != nil && current.EndsAt != nil {
		action = GrantActionRestore
	}
	endsAt := startsAt.AddDate(0, 0, req.DurationDays)

	var grant Grant
	err = tx.QueryRow(ctx, `
		INSERT INTO membership_grants (
			user_id, payment_order_id, source_type, action, duration_days, starts_at, ends_at, idempotency_key
		)
		VALUES ($1, $2, 'payment', $3, $4, $5, $6, $7)
		RETURNING id, user_id, payment_order_id, source_type, action, duration_days, starts_at, ends_at, idempotency_key, created_at
	`, req.UserID, req.PaymentOrderID, action, req.DurationDays, startsAt, endsAt, req.IdempotencyKey).Scan(
		&grant.ID,
		&grant.UserID,
		&grant.PaymentOrderID,
		&grant.SourceType,
		&grant.Action,
		&grant.DurationDays,
		&grant.StartsAt,
		&grant.EndsAt,
		&grant.IdempotencyKey,
		&grant.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, nil, false, ErrGrantAlreadyExists
		}
		return nil, nil, false, fmt.Errorf("insert membership grant: %w", err)
	}

	m, err := scanMembership(tx.QueryRow(ctx, `
		INSERT INTO user_memberships (
			user_id, membership_level, status, started_at, ends_at, last_order_id
		)
		VALUES ($1, 'premium', 'active', $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE
		SET membership_level = 'premium',
			status = 'active',
			started_at = COALESCE(user_memberships.started_at, EXCLUDED.started_at),
			ends_at = EXCLUDED.ends_at,
			last_order_id = EXCLUDED.last_order_id,
			updated_at = NOW()
		RETURNING id, user_id, membership_level, status, started_at, ends_at, last_order_id, created_at, updated_at
	`, req.UserID, startsAt, endsAt, req.PaymentOrderID))
	if err != nil {
		return nil, nil, false, fmt.Errorf("upsert user membership: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE users SET role = 'premium', updated_at = NOW() WHERE id = $1`, req.UserID); err != nil {
		return nil, nil, false, fmt.Errorf("sync premium role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, false, fmt.Errorf("commit grant membership tx: %w", err)
	}

	return m, &grant, true, nil
}

func membershipByUserForUpdateTx(ctx context.Context, tx pgx.Tx, userID int64) (*UserMembership, error) {
	m, err := scanMembership(tx.QueryRow(ctx, `
		SELECT id, user_id, membership_level, status, started_at, ends_at, last_order_id, created_at, updated_at
		FROM user_memberships
		WHERE user_id = $1
		FOR UPDATE
	`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lock membership: %w", err)
	}
	return m, nil
}

func membershipByUserTx(ctx context.Context, tx pgx.Tx, userID int64) (*UserMembership, error) {
	m, err := scanMembership(tx.QueryRow(ctx, `
		SELECT id, user_id, membership_level, status, started_at, ends_at, last_order_id, created_at, updated_at
		FROM user_memberships
		WHERE user_id = $1
	`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &UserMembership{
				UserID:          userID,
				MembershipLevel: LevelPremium,
				Status:          MembershipStatusInactive,
			}, nil
		}
		return nil, fmt.Errorf("get membership: %w", err)
	}
	return m, nil
}

func normalizeMembershipStatus(m *UserMembership, now time.Time) *UserMembership {
	if m == nil || m.EndsAt == nil {
		return m
	}
	if m.Status == MembershipStatusActive && !m.EndsAt.After(now) {
		m.Status = MembershipStatusExpired
	}
	return m
}
