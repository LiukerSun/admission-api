package volunteerplan

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DraftStore interface {
	GetByID(ctx context.Context, userID, draftID int64) (*Draft, error)
	MarkAdopted(ctx context.Context, userID, draftID int64) error
}

type draftStore struct {
	pool *pgxpool.Pool
}

func NewDraftStore(pool *pgxpool.Pool) DraftStore {
	return &draftStore{pool: pool}
}

func (s *draftStore) GetByID(ctx context.Context, userID, draftID int64) (*Draft, error) {
	var d Draft
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, conversation_id, status, input_json, plan_json, algorithm_version, COALESCE(error, ''), created_at, updated_at
		FROM conversation_plan_drafts
		WHERE id = $1 AND user_id = $2
	`, draftID, userID).Scan(
		&d.ID,
		&d.UserID,
		&d.ConversationID,
		&d.Status,
		&d.InputJSON,
		&d.PlanJSON,
		&d.AlgorithmVersion,
		&d.Error,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDraftNotFound
		}
		return nil, fmt.Errorf("get draft: %w", err)
	}
	return &d, nil
}

func (s *draftStore) MarkAdopted(ctx context.Context, userID, draftID int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversation_plan_drafts
		SET status = 'adopted', updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, draftID, userID)
	if err != nil {
		return fmt.Errorf("mark draft adopted: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDraftNotFound
	}
	return nil
}

