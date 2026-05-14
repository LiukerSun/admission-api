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
	ListByConversation(ctx context.Context, userID, conversationID int64) ([]*Draft, error)
	Create(ctx context.Context, userID, conversationID int64, inputJSON []byte, algorithmVersion string) (int64, error)
	MarkReady(ctx context.Context, userID, draftID int64, planJSON []byte) error
	MarkFailed(ctx context.Context, userID, draftID int64, errMsg string) error
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

func (s *draftStore) ListByConversation(ctx context.Context, userID, conversationID int64) ([]*Draft, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, conversation_id, status, input_json, plan_json, algorithm_version, COALESCE(error, ''), created_at, updated_at
		FROM conversation_plan_drafts
		WHERE user_id = $1 AND conversation_id = $2
		ORDER BY created_at DESC, id DESC
	`, userID, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list drafts by conversation: %w", err)
	}
	defer rows.Close()

	var out []*Draft
	for rows.Next() {
		var d Draft
		if err := rows.Scan(
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
		); err != nil {
			return nil, fmt.Errorf("scan draft: %w", err)
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate drafts: %w", err)
	}
	return out, nil
}

func (s *draftStore) Create(ctx context.Context, userID, conversationID int64, inputJSON []byte, algorithmVersion string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO conversation_plan_drafts (user_id, conversation_id, status, input_json, algorithm_version)
		VALUES ($1, $2, 'generating', $3, $4)
		RETURNING id
	`, userID, conversationID, inputJSON, algorithmVersion).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create draft: %w", err)
	}
	return id, nil
}

func (s *draftStore) MarkReady(ctx context.Context, userID, draftID int64, planJSON []byte) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversation_plan_drafts
		SET status = 'ready', plan_json = $3, error = NULL, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, draftID, userID, planJSON)
	if err != nil {
		return fmt.Errorf("mark draft ready: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDraftNotFound
	}
	return nil
}

func (s *draftStore) MarkFailed(ctx context.Context, userID, draftID int64, errMsg string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversation_plan_drafts
		SET status = 'failed', error = $3, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, draftID, userID, errMsg)
	if err != nil {
		return fmt.Errorf("mark draft failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDraftNotFound
	}
	return nil
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
