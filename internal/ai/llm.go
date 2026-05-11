package ai

import (
	"context"
	"encoding/json"
)

// ChunkKind enumerates the categories of stream chunks the LLM client emits.
type ChunkKind string

const (
	ChunkText     ChunkKind = "text"
	ChunkToolCall ChunkKind = "tool_call"
	ChunkFinish   ChunkKind = "finish"
)

// FinishReason mirrors OpenAI's finish_reason values that matter to the agent.
const (
	FinishStop      = "stop"
	FinishToolCalls = "tool_calls"
	FinishLength    = "length"
)

// StreamChunk is a normalized fragment from the LLM stream. Tool-call argument
// fragments arrive piecewise indexed by ToolCallIdx; the agent loop is
// responsible for accumulating them.
type StreamChunk struct {
	Kind ChunkKind

	// Populated when Kind == ChunkText.
	TextDelta string

	// Populated when Kind == ChunkToolCall.
	ToolCallIdx int
	ToolCallID  string
	ToolName    string
	ArgsDelta   string

	// Populated when Kind == ChunkFinish.
	FinishReason string
}

// StreamReader is a chunk-oriented stream cursor. Recv returns ChunkFinish
// once before reporting io.EOF or an error.
type StreamReader interface {
	Recv() (StreamChunk, error)
	Close() error
}

// ChatMessage is a provider-neutral chat message used both as LLM input and
// for shaping conversation history.
type ChatMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCalls  []ToolCallSpec  `json:"tool_calls,omitempty"`
	RawArgs    json.RawMessage `json:"-"`
}

// ToolCallSpec captures the tool call records we need to send back to the LLM
// in subsequent turns.
type ToolCallSpec struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"` // always "function"
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// FunctionDef is the local mirror of OpenAI's function tool schema so that
// neither the agent nor the tools package needs to import the vendor SDK.
type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatRequest is the input to a streamed chat completion call.
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	Tools       []FunctionDef
	Temperature float32
}

// LLMProxy is the small surface the agent depends on. Production wires this
// to an OpenAI-compatible client; tests inject fakes.
type LLMProxy interface {
	ChatCompletionStream(ctx context.Context, req ChatRequest) (StreamReader, error)
	ChatCompletion(ctx context.Context, req ChatRequest) (string, error)
}
