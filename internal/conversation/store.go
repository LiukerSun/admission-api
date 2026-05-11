package conversation

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConversationNotFound = errors.New("conversation not found")

// MaxMessagesReturned bounds how many rows ListMessages will pull from
// conversation_messages in a single call. Without this cap, a long
// conversation forces every AI request to load the full history into
// memory and replay it to the LLM, which scales linearly with chat age
// and explodes context-window cost.
//
// 500 is well above any realistic chat (the AI handler caps a single
// /ai/chat request at 50 input messages anyway) but bounded enough that
// a single bad actor can't make the server load tens of thousands of
// rows per request.
const MaxMessagesReturned = 500

type Store interface {
	CreateConversation(ctx context.Context, title string, userID *int64) (*Conversation, error)
	GetConversation(ctx context.Context, id int64) (*Conversation, error)
	ListConversations(ctx context.Context, userID *int64, status string) ([]*Conversation, error)
	UpdateConversationTitle(ctx context.Context, id int64, title string) error
	UpdateConversationStatus(ctx context.Context, id int64, status string) error
	DeleteConversation(ctx context.Context, id int64) error

	AddMessage(ctx context.Context, conversationID int64, role, content string, toolCalls, toolResults, widgets []byte) (*Message, error)
	ListMessages(ctx context.Context, conversationID int64) ([]*Message, error)
	// Rollback removes every message in conversationID whose
	// (created_at, id) sorts at or after the row identified by
	// messageID. When inclusive is false, the messageID row itself is
	// kept and only later rows are deleted. The (created_at, id) tuple
	// comparison guarantees stable ordering even when multiple rows
	// share the same created_at value (same-second inserts under load).
	//
	// Returns the number of rows deleted and the latest remaining
	// message id (nil if the conversation is now empty). If messageID
	// does not belong to conversationID, returns ErrConversationNotFound
	// — callers should already have authorized the conversation, so
	// this only fires on programming bugs or torn deletes.
	Rollback(ctx context.Context, conversationID, messageID int64, inclusive bool) (deletedCount int, latestMessageID *int64, err error)
}

type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &store{pool: pool}
}

func (s *store) CreateConversation(ctx context.Context, title string, userID *int64) (*Conversation, error) {
	query := `
		INSERT INTO conversations (title, user_id, status)
		VALUES ($1, $2, 'active')
		RETURNING id, user_id, title, status, created_at, updated_at
	`
	var conv Conversation
	err := s.pool.QueryRow(ctx, query, title, userID).Scan(
		&conv.ID, &conv.UserID, &conv.Title, &conv.Status, &conv.CreatedAt, &conv.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return &conv, nil
}

func (s *store) GetConversation(ctx context.Context, id int64) (*Conversation, error) {
	query := `
		SELECT id, user_id, title, status, created_at, updated_at
		FROM conversations
		WHERE id = $1
	`
	var conv Conversation
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&conv.ID, &conv.UserID, &conv.Title, &conv.Status, &conv.CreatedAt, &conv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	return &conv, nil
}

func (s *store) ListConversations(ctx context.Context, userID *int64, status string) ([]*Conversation, error) {
	where := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if userID != nil {
		where = append(where, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, *userID)
		argIdx++
	}
	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
	}

	whereClause := ""
	for i, w := range where {
		if i > 0 {
			whereClause += " AND "
		}
		whereClause += w
	}
	query := fmt.Sprintf(`
		SELECT id, user_id, title, status, created_at, updated_at
		FROM conversations
		WHERE %s
		ORDER BY updated_at DESC
	`, whereClause)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var conversations []*Conversation
	for rows.Next() {
		var conv Conversation
		if err := rows.Scan(
			&conv.ID, &conv.UserID, &conv.Title, &conv.Status, &conv.CreatedAt, &conv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, &conv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}
	return conversations, nil
}

func (s *store) UpdateConversationTitle(ctx context.Context, id int64, title string) error {
	query := `UPDATE conversations SET title = $1, updated_at = NOW() WHERE id = $2`
	result, err := s.pool.Exec(ctx, query, title, id)
	if err != nil {
		return fmt.Errorf("update conversation title: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrConversationNotFound
	}
	return nil
}

func (s *store) UpdateConversationStatus(ctx context.Context, id int64, status string) error {
	query := `UPDATE conversations SET status = $1, updated_at = NOW() WHERE id = $2`
	result, err := s.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update conversation status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrConversationNotFound
	}
	return nil
}

func (s *store) DeleteConversation(ctx context.Context, id int64) error {
	query := `DELETE FROM conversations WHERE id = $1`
	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrConversationNotFound
	}
	return nil
}

func (s *store) AddMessage(ctx context.Context, conversationID int64, role, content string, toolCalls, toolResults, widgets []byte) (*Message, error) {
	query := `
		INSERT INTO conversation_messages (conversation_id, role, content, tool_calls, tool_results, widgets)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6, '[]'::jsonb))
		RETURNING id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
	`
	var msg Message
	err := s.pool.QueryRow(ctx, query, conversationID, role, content, toolCalls, toolResults, widgets).Scan(
		&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ToolCalls, &msg.ToolResults, &msg.Widgets, &msg.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("add message: %w", err)
	}

	// Update conversation updated_at
	_, _ = s.pool.Exec(ctx, `UPDATE conversations SET updated_at = NOW() WHERE id = $1`, conversationID)

	return &msg, nil
}

func (s *store) ListMessages(ctx context.Context, conversationID int64) ([]*Message, error) {
	// Fetch the most-recent MaxMessagesReturned rows (DESC), then re-sort
	// chronologically (ASC) so the caller — and the LLM downstream — sees
	// turn order. The CTE keeps the LIMIT applied to the index-friendly
	// DESC scan instead of forcing the database to materialize the whole
	// history before slicing it.
	query := `
		WITH recent AS (
			SELECT id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
			FROM conversation_messages
			WHERE conversation_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		)
		SELECT id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
		FROM recent
		ORDER BY created_at ASC, id ASC
	`
	rows, err := s.pool.Query(ctx, query, conversationID, MaxMessagesReturned)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(
			&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ToolCalls, &msg.ToolResults, &msg.Widgets, &msg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, &msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return messages, nil
}

func (s *store) Rollback(ctx context.Context, conversationID, messageID int64, inclusive bool) (int, *int64, error) {
	// Compare on (created_at, id) instead of id alone: two messages
	// inserted in the same wall-second can sort by id in either order
	// across servers / replicas, but the tuple comparison is total even
	// when timestamps collide. The anchor row is fetched in the same
	// query as the delete so we don't race against a concurrent rollback
	// on the same conversation.
	cmp := ">="
	if !inclusive {
		cmp = ">"
	}
	query := fmt.Sprintf(`
		WITH anchor AS (
			SELECT created_at, id
			FROM conversation_messages
			WHERE id = $2 AND conversation_id = $1
		)
		DELETE FROM conversation_messages
		WHERE conversation_id = $1
		  AND (created_at, id) %s (SELECT created_at, id FROM anchor)
	`, cmp)
	tag, err := s.pool.Exec(ctx, query, conversationID, messageID)
	if err != nil {
		return 0, nil, fmt.Errorf("rollback messages: %w", err)
	}
	deleted := int(tag.RowsAffected())
	if deleted == 0 {
		// Either the anchor row didn't exist (wrong conversation /
		// message), or inclusive=false with no later rows. Distinguish
		// by probing for the anchor: if it exists, we just had nothing
		// later to delete; if it doesn't, signal not-found so the
		// handler can return 404.
		var exists bool
		_ = s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM conversation_messages WHERE id = $1 AND conversation_id = $2)`,
			messageID, conversationID).Scan(&exists)
		if !exists {
			return 0, nil, ErrConversationNotFound
		}
	}

	var latest *int64
	var id int64
	err = s.pool.QueryRow(ctx,
		`SELECT id FROM conversation_messages WHERE conversation_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1`,
		conversationID).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Conversation is empty after the delete; leave latest nil.
	case err != nil:
		return deleted, nil, fmt.Errorf("rollback fetch latest id: %w", err)
	default:
		latest = &id
	}

	// Touch conversation.updated_at so list endpoints reflect the
	// rollback in chronological order.
	_, _ = s.pool.Exec(ctx, `UPDATE conversations SET updated_at = NOW() WHERE id = $1`, conversationID)
	return deleted, latest, nil
}
