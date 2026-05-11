package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolResult is the structured outcome of a tool invocation. Content is the
// string fed back to the LLM (must be machine-readable, typically JSON). If
// Widget is non-nil, it is also surfaced to the UI via the SSE widget event.
// A non-empty Error marks the result as a failure: Content is still sent to
// the LLM so it can react, but IsError is true on persistence.
type ToolResult struct {
	Content string
	Widget  *Widget
	Error   string
	IsError bool
}

// CallContext carries per-invocation state into a tool.
type CallContext struct {
	Ctx            context.Context
	ConversationID int64
	UserID         int64

	// ToolResults exposes previous results of the SAME turn keyed by call_id.
	// Tools like render_chart use this to look up data produced by earlier
	// tool calls and chart them.
	ToolResults map[string]ToolResult

	// OnWidget is invoked whenever a tool emits a widget. Wired by the agent
	// to broadcast over SSE.
	OnWidget func(w Widget)
}

// Tool is the interface every callable function exposes.
type Tool interface {
	Name() string
	Schema() FunctionDef
	Execute(cc *CallContext, args json.RawMessage) ToolResult
}

// ToolRegistry holds the tools available for a given Agent.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	order []string
}

// NewToolRegistry constructs an empty registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: map[string]Tool{}}
}

// Register adds a tool. Re-registering a name replaces the previous tool.
func (r *ToolRegistry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

// Schemas returns the function-definition list for inclusion in an LLM request.
func (r *ToolRegistry) Schemas() []FunctionDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]FunctionDef, 0, len(r.order))
	for _, name := range r.order {
		if t, ok := r.tools[name]; ok {
			out = append(out, t.Schema())
		}
	}
	return out
}

// Names lists registered tool names (mostly for tests and logging).
func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Invoke dispatches a tool by name and records the result into cc.ToolResults.
// callID is the LLM-provided invocation ID used both for downstream chart
// referencing (data_source: "tool_result:<call_id>") and for SSE correlation.
func (r *ToolRegistry) Invoke(cc *CallContext, callID, name string, args json.RawMessage) (tr ToolResult, err error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		tr = ToolResult{
			Content: fmt.Sprintf(`{"error":"unknown tool %q"}`, name),
			Error:   fmt.Sprintf("unknown tool %q", name),
			IsError: true,
		}
		cc.ToolResults[callID] = tr
		return tr, nil
	}

	defer func() {
		if rec := recover(); rec != nil {
			tr = ToolResult{
				Content: fmt.Sprintf(`{"error":"tool panic: %v"}`, rec),
				Error:   fmt.Sprintf("tool panic: %v", rec),
				IsError: true,
			}
			cc.ToolResults[callID] = tr
		}
	}()

	tr = tool.Execute(cc, args)
	cc.ToolResults[callID] = tr
	return tr, nil
}
