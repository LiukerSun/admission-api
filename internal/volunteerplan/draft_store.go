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
	// MarkSuperseded 把 ready 草稿标记为已被新草稿顶替。reason 写入 error 字段
	// 仅作运维线索（前端不展示）。和 MarkFailed 的语义差别：算法本身没出问题，
	// 只是用户后续又改了偏好；区分开能让历史记录更易解读，也让 stats 不被污染。
	MarkSuperseded(ctx context.Context, userID, draftID int64, reason string) error
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

// markStatusTransition runs a guarded UPDATE that only mutates the row
// if its current status is in fromStatuses. Returns ErrDraftNotFound when
// no row matches (id+user mismatch) and ErrDraftNotInExpectedState when
// the row exists but its current status is not one of fromStatuses — the
// caller is expected to disambiguate (e.g. AdoptDraft maps the latter to
// ErrDraftAlreadyAdopted / ErrDraftNotReady based on the actual status).
func (s *draftStore) markStatusTransition(
	ctx context.Context,
	userID, draftID int64,
	fromStatuses []string,
	setSQL string,
	args ...any,
) error {
	// Build IN clause placeholders ($N, $N+1, ...) starting after the
	// fixed (draftID, userID) + setSQL positional args.
	baseArgs := append([]any{draftID, userID}, args...)
	placeholders := make([]string, 0, len(fromStatuses))
	for i := range fromStatuses {
		baseArgs = append(baseArgs, fromStatuses[i])
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(baseArgs)))
	}
	statusInClause := "(" + joinStrings(placeholders, ",") + ")"

	q := fmt.Sprintf(`
		UPDATE conversation_plan_drafts
		SET %s, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status IN %s
	`, setSQL, statusInClause)

	tag, err := s.pool.Exec(ctx, q, baseArgs...)
	if err != nil {
		return fmt.Errorf("update draft status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Distinguish "row doesn't exist for this user" from "row exists
		// but is in a different status" so callers can produce a precise
		// 404 vs 409 / "already adopted" message.
		var exists bool
		if probeErr := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM conversation_plan_drafts WHERE id = $1 AND user_id = $2)`,
			draftID, userID,
		).Scan(&exists); probeErr != nil {
			return fmt.Errorf("probe draft existence: %w", probeErr)
		}
		if !exists {
			return ErrDraftNotFound
		}
		return ErrDraftNotInExpectedState
	}
	return nil
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += sep + parts[i]
	}
	return out
}

// MarkReady transitions generating -> ready. Anything else (already
// ready, already failed, already adopted) is rejected so a delayed
// tool-callback can't silently overwrite a terminal state.
func (s *draftStore) MarkReady(ctx context.Context, userID, draftID int64, planJSON []byte) error {
	return s.markStatusTransition(
		ctx, userID, draftID,
		[]string{"generating"},
		"status = 'ready', plan_json = $3, error = NULL",
		planJSON,
	)
}

// MarkFailed accepts both generating and ready as starting states: a
// tool can fail after the algorithm produced a draft (e.g. during
// downstream persistence), so we allow the failure to overwrite a
// previously-ready draft. Terminal states (failed, adopted) are still
// rejected so we don't lose adoption history to a late tool error.
func (s *draftStore) MarkFailed(ctx context.Context, userID, draftID int64, errMsg string) error {
	return s.markStatusTransition(
		ctx, userID, draftID,
		[]string{"generating", "ready"},
		"status = 'failed', error = $3",
		errMsg,
	)
}

// MarkAdopted requires the draft to be in 'ready' state. Re-adopting an
// already-adopted draft (double click on the adopt button) returns
// ErrDraftNotInExpectedState so the service layer can surface
// ErrDraftAlreadyAdopted instead of silently re-issuing the plan row.
func (s *draftStore) MarkAdopted(ctx context.Context, userID, draftID int64) error {
	return s.markStatusTransition(
		ctx, userID, draftID,
		[]string{"ready"},
		"status = 'adopted'",
	)
}

// MarkSuperseded only transitions ready -> superseded. A still-generating
// draft can't be superseded (it might MarkReady right after); terminal
// states (failed/adopted/already-superseded) are also rejected so we
// don't rewrite history.
func (s *draftStore) MarkSuperseded(ctx context.Context, userID, draftID int64, reason string) error {
	return s.markStatusTransition(
		ctx, userID, draftID,
		[]string{"ready"},
		"status = 'superseded', error = $3",
		reason,
	)
}
