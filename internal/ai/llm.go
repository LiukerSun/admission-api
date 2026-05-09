package ai

import "context"

// Message represents a chat message.
type Message struct {
	Role          string         `json:"role"`
	Content       string         `json:"content"`
	ToolCalls     []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID    string         `json:"tool_call_id,omitempty"`
	ContentBlocks []ContentBlock `json:"-"`
}

// ToolParameter defines a JSON Schema parameter for a tool.
type ToolParameter struct {
	Type        string         `json:"type"`
	Properties  map[string]any `json:"properties,omitempty"`
	Required    []string       `json:"required,omitempty"`
	Description string         `json:"description,omitempty"`
}

// ToolDefinition defines a callable tool.
type ToolDefinition struct {
	Type     string `json:"type"`
	Function struct {
		Name        string        `json:"name"`
		Description string        `json:"description"`
		Parameters  ToolParameter `json:"parameters"`
	} `json:"function"`
}

// ToolCall represents a tool invocation from the LLM.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ContentBlock represents a single block in the LLM response content.
type ContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Name      string         `json:"name,omitempty"`
	ID        string         `json:"id,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	Data      string         `json:"data,omitempty"`
	Signature string         `json:"signature,omitempty"`
}

// LLMResponse is the normalized response from any LLM provider.
type LLMResponse struct {
	Content       string         `json:"content"`
	ToolCalls     []ToolCall     `json:"tool_calls"`
	ContentBlocks []ContentBlock `json:"-"`
}

// LLMProxy abstracts multi-provider LLM access.
type LLMProxy interface {
	ChatCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (*LLMResponse, error)
}
