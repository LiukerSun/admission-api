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

// StreamChunkType enumerates the kinds of incremental events a streaming
// LLM completion can produce. Concrete LLM clients translate provider
// SDK frames into these values so callers (Agent.RunStream) never see
// provider-specific shapes.
const (
	StreamChunkText         = "text_delta"
	StreamChunkToolCallDone = "tool_call_done"
	StreamChunkDone         = "done"
	StreamChunkError        = "error"
)

// StreamChunk is a single incremental event from ChatCompletionStream.
//
// Tool-call streaming semantics: providers (notably OpenAI) deliver tool
// calls as a sequence of partial frames keyed by an index, where the id
// and function.name typically arrive only in the first frame and the
// arguments field is concatenated across many frames. Clients of this
// interface are responsible for accumulating those partial frames inside
// the LLM client implementation, and only emitting a StreamChunkToolCallDone
// chunk once the full ID / Name / Arguments JSON for a given index has
// been assembled. This keeps the Agent layer simple — it never has to
// concatenate arguments fragments itself.
type StreamChunk struct {
	Type string
	// TextDelta is set for StreamChunkText chunks.
	TextDelta string
	// ToolCall is set for StreamChunkToolCallDone chunks and contains
	// the fully-accumulated ID, function name, and arguments JSON.
	ToolCall ToolCall
	// ContentBlocks is set on StreamChunkDone chunks for providers that
	// surface structured assistant content (Anthropic 风格的 thinking +
	// text + tool_use)。Agent 必须把这些原样塞回下一轮请求的 history，
	// 否则像 DeepSeek-v4 这种 thinking 强制回填的模型会 400。
	ContentBlocks []ContentBlock
	// Err is set for StreamChunkError chunks. The channel will be closed
	// after an error chunk; receivers must not expect further data.
	Err error
}

// LLMProxy abstracts multi-provider LLM access.
//
// ChatCompletion is the blocking single-shot variant kept for callers
// that do not need token-level streaming (e.g. the suggestions endpoint
// which makes a one-off classifier-style call).
//
// ChatCompletionStream returns a channel of StreamChunk values. The
// channel is closed by the implementation when the stream terminates
// (normally with a StreamChunkDone, or after a StreamChunkError). The
// caller must drain the channel until it is closed to avoid leaking the
// producer goroutine and its underlying HTTP body. Cancelling ctx is
// the standard way to abort an in-flight stream.
type LLMProxy interface {
	ChatCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (*LLMResponse, error)
	ChatCompletionStream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error)
}
