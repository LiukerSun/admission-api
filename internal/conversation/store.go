package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store defines conversation persistence operations.
type Store interface {
	CreateConversation(ctx context.Context, userID int64, title, modelName string) (*Conversation, error)
	GetConversation(ctx context.Context, id int64) (*Conversation, error)
	ListConversationsByUser(ctx context.Context, userID int64, page, pageSize int) ([]*Conversation, int64, error)
	DeleteConversation(ctx context.Context, id int64) error
	UpdateConversationTitle(ctx context.Context, id int64, title string) error
	TouchConversation(ctx context.Context, id int64) error

	CreateMessage(ctx context.Context, input *CreateMessageInput) (*Message, error)
	GetMessage(ctx context.Context, id int64) (*Message, error)
	ListMessages(ctx context.Context, conversationID int64) ([]*Message, error)
	ListRecentMessages(ctx context.Context, conversationID int64, limit int) ([]*Message, error)
	GetLastMessage(ctx context.Context, conversationID int64) (*Message, error)
	UpdateMessageWidgets(ctx context.Context, messageID int64, widgets json.RawMessage) error

	// DeleteMessagesFrom removes messages whose (created_at, id) is >= pivot's
	// when inclusive == true, or > pivot's when inclusive == false. Returns
	// the number of rows removed and the new latest message ID (0 if none).
	DeleteMessagesFrom(ctx context.Context, conversationID, pivotMessageID int64, inclusive bool) (int64, int64, error)
}

type pgStore struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Postgres-backed conversation store.
func NewStore(pool *pgxpool.Pool) Store {
	return &pgStore{pool: pool}
}

func scanConversation(row pgx.Row) (*Conversation, error) {
	var c Conversation
	if err := row.Scan(&c.ID, &c.UserID, &c.Title, &c.ModelName, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func scanMessage(row pgx.Row) (*Message, error) {
	var m Message
	var toolCalls, toolResults, widgets []byte
	if err := row.Scan(
		&m.ID, &m.ConversationID, &m.Role, &m.Content,
		&toolCalls, &toolResults, &widgets, &m.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(toolCalls) > 0 {
		m.ToolCalls = json.RawMessage(toolCalls)
	}
	if len(toolResults) > 0 {
		m.ToolResults = json.RawMessage(toolResults)
	}
	if len(widgets) > 0 {
		m.Widgets = json.RawMessage(widgets)
	}
	return &m, nil
}

func (s *pgStore) CreateConversation(ctx context.Context, userID int64, title, modelName string) (*Conversation, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO ai_conversations (user_id, title, model_name)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, title, model_name, created_at, updated_at
	`, userID, title, modelName)
	c, err := scanConversation(row)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return c, nil
}

func (s *pgStore) GetConversation(ctx context.Context, id int64) (*Conversation, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, model_name, created_at, updated_at
		FROM ai_conversations
		WHERE id = $1
	`, id)
	c, err := scanConversation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	return c, nil
}

func (s *pgStore) ListConversationsByUser(ctx context.Context, userID int64, page, pageSize int) ([]*Conversation, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_conversations WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count conversations: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, title, model_name, created_at, updated_at
		FROM ai_conversations
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	out := []*Conversation{}
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan conversation: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate conversations: %w", err)
	}
	return out, total, nil
}

func (s *pgStore) DeleteConversation(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM ai_conversations WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

func (s *pgStore) UpdateConversationTitle(ctx context.Context, id int64, title string) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE ai_conversations SET title = $2, updated_at = NOW() WHERE id = $1
	`, id, title); err != nil {
		return fmt.Errorf("update conversation title: %w", err)
	}
	return nil
}

func (s *pgStore) TouchConversation(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE ai_conversations SET updated_at = NOW() WHERE id = $1
	`, id); err != nil {
		return fmt.Errorf("touch conversation: %w", err)
	}
	return nil
}

func (s *pgStore) CreateMessage(ctx context.Context, input *CreateMessageInput) (*Message, error) {
	toolCalls := jsonOrEmpty(input.ToolCalls, "[]")
	toolResults := jsonOrEmpty(input.ToolResults, "[]")
	widgets := jsonOrEmpty(input.Widgets, "[]")

	row := s.pool.QueryRow(ctx, `
		INSERT INTO ai_conversation_messages
			(conversation_id, role, content, tool_calls, tool_results, widgets)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6::jsonb)
		RETURNING id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
	`, input.ConversationID, input.Role, input.Content, toolCalls, toolResults, widgets)
	m, err := scanMessage(row)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	return m, nil
}

func (s *pgStore) GetMessage(ctx context.Context, id int64) (*Message, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
		FROM ai_conversation_messages
		WHERE id = $1
	`, id)
	m, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	return m, nil
}

func (s *pgStore) ListMessages(ctx context.Context, conversationID int64) ([]*Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
		FROM ai_conversation_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC, id ASC
	`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	out := []*Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return out, nil
}

func (s *pgStore) ListRecentMessages(ctx context.Context, conversationID int64, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
		FROM (
			SELECT id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
			FROM ai_conversation_messages
			WHERE conversation_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		) sub
		ORDER BY created_at ASC, id ASC
	`, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent messages: %w", err)
	}
	defer rows.Close()

	out := []*Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent messages: %w", err)
	}
	return out, nil
}

func (s *pgStore) GetLastMessage(ctx context.Context, conversationID int64) (*Message, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, conversation_id, role, content, tool_calls, tool_results, widgets, created_at
		FROM ai_conversation_messages
		WHERE conversation_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, conversationID)
	m, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get last message: %w", err)
	}
	return m, nil
}

func (s *pgStore) UpdateMessageWidgets(ctx context.Context, messageID int64, widgets json.RawMessage) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE ai_conversation_messages SET widgets = $2::jsonb WHERE id = $1
	`, messageID, jsonOrEmpty(widgets, "[]")); err != nil {
		return fmt.Errorf("update message widgets: %w", err)
	}
	return nil
}

func (s *pgStore) DeleteMessagesFrom(ctx context.Context, conversationID, pivotMessageID int64, inclusive bool) (int64, int64, error) {
	// Use a single Exec with a CTE so the comparison stays consistent.
	op := ">="
	if !inclusive {
		op = ">"
	}
	query := fmt.Sprintf(`
		WITH pivot AS (
			SELECT created_at, id
			FROM ai_conversation_messages
			WHERE id = $2 AND conversation_id = $1
		),
		deleted AS (
			DELETE FROM ai_conversation_messages m
			USING pivot p
			WHERE m.conversation_id = $1
			  AND (m.created_at, m.id) %s (p.created_at, p.id)
			RETURNING m.id
		)
		SELECT (SELECT COUNT(*) FROM deleted) AS deleted_count,
		       COALESCE((SELECT id FROM ai_conversation_messages WHERE conversation_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1), 0) AS latest_id
	`, op)
	var deleted, latest int64
	if err := s.pool.QueryRow(ctx, query, conversationID, pivotMessageID).Scan(&deleted, &latest); err != nil {
		return 0, 0, fmt.Errorf("delete messages from pivot: %w", err)
	}
	return deleted, latest, nil
}

func jsonOrEmpty(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 {
		return fallback
	}
	return string(raw)
}
