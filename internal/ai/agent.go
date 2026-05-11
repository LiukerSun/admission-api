package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// AgentCallbacks lets the caller observe streaming events as they happen. All
// callbacks are optional and called from the agent's own goroutine; they
// must not block for long.
type AgentCallbacks struct {
	OnTextDelta     func(delta string)
	OnToolCallStart func(callID, name string, args json.RawMessage)
	OnToolCallEnd   func(callID string, ok bool, errMsg string)
	OnWidget        func(w Widget)
	OnWarning       func(message string)
}

// AgentResult collects the final state of a streamed run for persistence.
type AgentResult struct {
	FinalContent string
	ToolCalls    []ToolCallSpec
	ToolResults  []PersistedToolResult
	Widgets      []Widget
}

// PersistedToolResult is the shape we write to conversation_messages.tool_results.
type PersistedToolResult struct {
	CallID  string `json:"call_id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// Agent runs a streamed LLM-with-tools loop.
type Agent struct {
	llm      LLMProxy
	tools    *ToolRegistry
	model    string
	maxTurns int
}

// AgentOption mutates an Agent in NewAgent.
type AgentOption func(*Agent)

// WithMaxTurns caps the number of LLM round-trips (default 6).
func WithMaxTurns(n int) AgentOption {
	return func(a *Agent) {
		if n > 0 {
			a.maxTurns = n
		}
	}
}

// WithModel overrides the default model name.
func WithModel(name string) AgentOption {
	return func(a *Agent) {
		if name != "" {
			a.model = name
		}
	}
}

// NewAgent constructs an Agent.
func NewAgent(llm LLMProxy, tools *ToolRegistry, opts ...AgentOption) *Agent {
	a := &Agent{
		llm:      llm,
		tools:    tools,
		model:    "",
		maxTurns: 6,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// RunStream drives the agent loop. It mutates `history` in place by appending
// assistant and tool messages so the caller can persist exactly what was sent
// to the model.
func (a *Agent) RunStream(ctx context.Context, history []ChatMessage, cb AgentCallbacks) (*AgentResult, error) {
	result := &AgentResult{}
	toolResults := map[string]ToolResult{}

	working := make([]ChatMessage, len(history))
	copy(working, history)

	for turn := 0; turn < a.maxTurns; turn++ {
		req := ChatRequest{
			Model:    a.model,
			Messages: working,
		}
		if a.tools != nil {
			req.Tools = a.tools.Schemas()
		}
		stream, err := a.llm.ChatCompletionStream(ctx, req)
		if err != nil {
			return result, fmt.Errorf("start stream: %w", err)
		}

		turnText, pending, finishReason, streamErr := drainStream(ctx, stream, cb)
		_ = stream.Close()
		if streamErr != nil {
			return result, streamErr
		}

		if turnText != "" {
			result.FinalContent += turnText
		}

		if finishReason != FinishToolCalls {
			// stop / length / unknown — end loop.
			if finishReason == FinishLength && cb.OnWarning != nil {
				cb.OnWarning("response truncated by length limit")
			}
			return result, nil
		}

		if len(pending) == 0 {
			// finish_reason claims tool_calls but no calls accumulated — bail
			// to avoid an infinite loop.
			if cb.OnWarning != nil {
				cb.OnWarning("model signalled tool_calls without payload")
			}
			return result, nil
		}

		assistantCalls := make([]ToolCallSpec, 0, len(pending))
		newToolMsgs := make([]ChatMessage, 0, len(pending))

		for _, pc := range pending {
			args := normalizeArgs(pc.argsBuf.String())
			spec := ToolCallSpec{
				ID:        pc.id,
				Type:      "function",
				Name:      pc.name,
				Arguments: args,
			}
			assistantCalls = append(assistantCalls, spec)
			result.ToolCalls = append(result.ToolCalls, spec)

			if cb.OnToolCallStart != nil {
				cb.OnToolCallStart(pc.id, pc.name, args)
			}

			var (
				tr  ToolResult
				err error
			)
			if a.tools == nil {
				err = fmt.Errorf("no tool registry configured")
			} else {
				cc := &CallContext{
					Ctx:            ctx,
					ConversationID: 0,
					UserID:         0,
					ToolResults:    toolResults,
					OnWidget: func(w Widget) {
						if cb.OnWidget != nil {
							cb.OnWidget(w)
						}
					},
				}
				tr, err = a.tools.Invoke(cc, pc.id, pc.name, args)
			}

			ok := err == nil && tr.Error == ""
			errMsg := tr.Error
			if err != nil {
				errMsg = err.Error()
			}
			if cb.OnToolCallEnd != nil {
				cb.OnToolCallEnd(pc.id, ok, errMsg)
			}
			if !ok {
				if tr.Content == "" {
					if errMsg != "" {
						tr.Content = fmt.Sprintf(`{"error":%q}`, errMsg)
					} else {
						tr.Content = `{"error":"tool failed"}`
					}
				}
				tr.IsError = true
			}
			if tr.Widget != nil {
				result.Widgets = append(result.Widgets, *tr.Widget)
			}
			toolResults[pc.id] = tr
			result.ToolResults = append(result.ToolResults, PersistedToolResult{
				CallID:  pc.id,
				Content: tr.Content,
				IsError: tr.IsError,
			})

			newToolMsgs = append(newToolMsgs, ChatMessage{
				Role:       "tool",
				ToolCallID: pc.id,
				Name:       pc.name,
				Content:    tr.Content,
			})
		}

		working = append(working, ChatMessage{
			Role:      "assistant",
			ToolCalls: assistantCalls,
		})
		working = append(working, newToolMsgs...)
	}

	if cb.OnWarning != nil {
		cb.OnWarning("max tool-call turns reached")
	}
	return result, nil
}

type pendingCall struct {
	id      string
	name    string
	argsBuf strings.Builder
}

func drainStream(ctx context.Context, stream StreamReader, cb AgentCallbacks) (text string, calls []*pendingCall, finishReason string, err error) {
	var (
		textBuf  strings.Builder
		callsMap = map[int]*pendingCall{}
		order    []int
	)

	for {
		// Respect cancellation between chunks.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return textBuf.String(), gather(callsMap, order), "", ctxErr
		}

		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			return textBuf.String(), gather(callsMap, order), finishReason, nil
		}
		if recvErr != nil {
			return textBuf.String(), gather(callsMap, order), finishReason, recvErr
		}

		switch chunk.Kind {
		case ChunkText:
			if chunk.TextDelta != "" {
				textBuf.WriteString(chunk.TextDelta)
				if cb.OnTextDelta != nil {
					cb.OnTextDelta(chunk.TextDelta)
				}
			}
		case ChunkToolCall:
			pc, exists := callsMap[chunk.ToolCallIdx]
			if !exists {
				pc = &pendingCall{}
				callsMap[chunk.ToolCallIdx] = pc
				order = append(order, chunk.ToolCallIdx)
			}
			if chunk.ToolCallID != "" {
				pc.id = chunk.ToolCallID
			}
			if chunk.ToolName != "" {
				pc.name = chunk.ToolName
			}
			if chunk.ArgsDelta != "" {
				pc.argsBuf.WriteString(chunk.ArgsDelta)
			}
		case ChunkFinish:
			finishReason = chunk.FinishReason
			return textBuf.String(), gather(callsMap, order), finishReason, nil
		}
	}
}

func gather(m map[int]*pendingCall, order []int) []*pendingCall {
	out := make([]*pendingCall, 0, len(m))
	for _, idx := range order {
		if pc, ok := m[idx]; ok {
			if pc.id == "" {
				// Synthesize a deterministic ID so the LLM round-trip works.
				pc.id = fmt.Sprintf("call_%d_%d", idx, time.Now().UnixNano())
			}
			out = append(out, pc)
		}
	}
	return out
}

func normalizeArgs(raw string) json.RawMessage {
	s := strings.TrimSpace(raw)
	if s == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s)
}
