package conversation

import (
	"encoding/json"
	"time"
)

// Conversation is a chat session owned by a single user.
type Conversation struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Title     string    `json:"title"`
	ModelName string    `json:"model_name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message is one turn in a conversation. JSONB columns are stored as raw bytes
// so the conversation package stays neutral about their interior shape: the
// ai package owns the schemas, this package only persists them.
type Message struct {
	ID             int64           `json:"id"`
	ConversationID int64           `json:"conversation_id"`
	Role           string          `json:"role"`
	Content        string          `json:"content"`
	ToolCalls      json.RawMessage `json:"tool_calls,omitempty"`
	ToolResults    json.RawMessage `json:"tool_results,omitempty"`
	Widgets        json.RawMessage `json:"widgets,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// CreateMessageInput collects optional JSONB attachments for an insert.
type CreateMessageInput struct {
	ConversationID int64
	Role           string
	Content        string
	ToolCalls      json.RawMessage
	ToolResults    json.RawMessage
	Widgets        json.RawMessage
}

// Roles supported by the schema CHECK constraint.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)
