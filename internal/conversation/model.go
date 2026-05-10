package conversation

import "time"

type Conversation struct {
	ID        int64     `json:"id"`
	UserID    *int64    `json:"user_id,omitempty"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	ToolCalls      []byte    `json:"tool_calls,omitempty"`
	ToolResults    []byte    `json:"tool_results,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type CreateConversationRequest struct {
	Title  string `json:"title"`
	UserID *int64 `json:"user_id,omitempty"`
}

// AddMessageRequest is the public request body for inserting a user
// message into an existing conversation.
//
// Security note: only the Content field is honored. The role is always
// forced to "user" and tool_calls / tool_results are never accepted from
// clients, so this endpoint cannot be used to fabricate assistant or
// tool history that the LLM would later replay as authoritative.
type AddMessageRequest struct {
	Content string `json:"content"`
}
