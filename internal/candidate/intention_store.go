package candidate

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IntentionStore defines intention data access operations.
type IntentionStore interface {
	ListByProfile(ctx context.Context, profileID int64, intentionType string) ([]*Intention, error)
	GetByID(ctx context.Context, id int64) (*Intention, error)
	ReplaceByType(ctx context.Context, profileID int64, intentionType string, items []*CreateIntentionInput) error
	DeleteByID(ctx context.Context, profileID int64, id int64) error
	DeleteByType(ctx context.Context, profileID int64, intentionType string) error
}

type intentionStore struct {
	pool *pgxpool.Pool
}

// NewIntentionStore creates a new intention store.
func NewIntentionStore(pool *pgxpool.Pool) IntentionStore {
	return &intentionStore{pool: pool}
}

func scanIntention(row pgx.Row) (*Intention, error) {
	var i Intention
	if err := row.Scan(
		&i.ID, &i.ProfileID, &i.IntentionType, &i.TargetID, &i.TargetName,
		&i.Priority, &i.Notes, &i.CreatedAt, &i.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &i, nil
}

func (s *intentionStore) ListByProfile(ctx context.Context, profileID int64, intentionType string) ([]*Intention, error) {
	var query string
	var args []any
	if intentionType != "" {
		query = `
			SELECT id, profile_id, intention_type, target_id, target_name, priority, notes, created_at, updated_at
			FROM candidate_intentions
			WHERE profile_id = $1 AND intention_type = $2
			ORDER BY priority ASC, created_at ASC
		`
		args = []any{profileID, intentionType}
	} else {
		query = `
			SELECT id, profile_id, intention_type, target_id, target_name, priority, notes, created_at, updated_at
			FROM candidate_intentions
			WHERE profile_id = $1
			ORDER BY intention_type ASC, priority ASC, created_at ASC
		`
		args = []any{profileID}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list intentions: %w", err)
	}
	defer rows.Close()

	out := []*Intention{}
	for rows.Next() {
		i, err := scanIntention(rows)
		if err != nil {
			return nil, fmt.Errorf("scan intention: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate intentions: %w", err)
	}
	return out, nil
}

func (s *intentionStore) GetByID(ctx context.Context, id int64) (*Intention, error) {
	i, err := scanIntention(s.pool.QueryRow(ctx, `
		SELECT id, profile_id, intention_type, target_id, target_name, priority, notes, created_at, updated_at
		FROM candidate_intentions
		WHERE id = $1
	`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get intention: %w", err)
	}
	return i, nil
}

func (s *intentionStore) ReplaceByType(ctx context.Context, profileID int64, intentionType string, items []*CreateIntentionInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		DELETE FROM candidate_intentions
		WHERE profile_id = $1 AND intention_type = $2
	`, profileID, intentionType); err != nil {
		return fmt.Errorf("delete old intentions: %w", err)
	}

	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO candidate_intentions (profile_id, intention_type, target_id, target_name, priority, notes)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, item.ProfileID, item.IntentionType, item.TargetID, item.TargetName, item.Priority, item.Notes); err != nil {
			return fmt.Errorf("insert intention: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *intentionStore) DeleteByID(ctx context.Context, profileID int64, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM candidate_intentions
		WHERE id = $1 AND profile_id = $2
	`, id, profileID)
	if err != nil {
		return fmt.Errorf("delete intention: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *intentionStore) DeleteByType(ctx context.Context, profileID int64, intentionType string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM candidate_intentions
		WHERE profile_id = $1 AND intention_type = $2
	`, profileID, intentionType)
	if err != nil {
		return fmt.Errorf("delete intentions by type: %w", err)
	}
	return nil
}
