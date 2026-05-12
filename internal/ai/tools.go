package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"admission-api/internal/admission"
	"admission-api/internal/knowledge"
)

// ToolResult is the result of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// ToolExecutor executes tool calls from the LLM.
type ToolExecutor struct {
	admissionLineStore admission.AdmissionLineStore
	aggregateStore     admission.AggregateStore
	knowledgeStore     knowledge.Store
}

// NewToolExecutor creates a new tool executor.
func NewToolExecutor(admissionLineStore admission.AdmissionLineStore, aggregateStore admission.AggregateStore, knowledgeStore knowledge.Store) *ToolExecutor {
	return &ToolExecutor{
		admissionLineStore: admissionLineStore,
		aggregateStore:     aggregateStore,
		knowledgeStore:     knowledgeStore,
	}
}

// Execute runs a tool call and returns the result.
func (e *ToolExecutor) Execute(ctx context.Context, call ToolCall) (*ToolResult, error) {
	switch call.Function.Name {
	case "apply_filter":
		return e.executeApplyFilter(ctx, call)
	case "search_universities":
		return e.executeSearchUniversities(ctx, call)
	case "aggregate_data":
		return e.executeAggregateData(ctx, call)
	case "retrieve_knowledge":
		return e.executeRetrieveKnowledge(ctx, call)
	default:
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Unknown tool: %s", call.Function.Name)}, nil
	}
}

func (e *ToolExecutor) executeApplyFilter(ctx context.Context, call ToolCall) (*ToolResult, error) {
	var params struct {
		FilterType string          `json:"filter_type"`
		FilterData json.RawMessage `json:"filter_data"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid filter arguments: %v", err)}, nil
	}

	// For MVP, we just validate the filter data can be parsed
	var filter admission.AdmissionLineFilter
	if err := decodeAdmissionLineFilter(params.FilterData, &filter); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid filter data: %v", err)}, nil
	}

	result := map[string]any{
		"filter_type": params.FilterType,
		"valid":       true,
		"filter":      filter,
	}
	content, _ := json.Marshal(result)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

func (e *ToolExecutor) executeSearchUniversities(ctx context.Context, call ToolCall) (*ToolResult, error) {
	var params struct {
		Filter json.RawMessage `json:"filter"`
		Limit  int             `json:"limit"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid search arguments: %v", err)}, nil
	}
	if params.Limit <= 0 || params.Limit > 20 {
		params.Limit = 5
	}

	var filter admission.AdmissionLineFilter
	if err := decodeAdmissionLineFilter(params.Filter, &filter); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid filter: %v", err)}, nil
	}

	lines, err := e.admissionLineStore.ListAdmissionLines(ctx, &filter)
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Search error: %v", err)}, nil
	}

	count := len(lines)
	top := lines
	if len(top) > params.Limit {
		top = top[:params.Limit]
	}

	result := map[string]any{
		"count": count,
		"top":   top,
	}
	content, _ := json.Marshal(result)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

func (e *ToolExecutor) executeAggregateData(ctx context.Context, call ToolCall) (*ToolResult, error) {
	var params struct {
		Filter json.RawMessage `json:"filter"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid aggregate arguments: %v", err)}, nil
	}

	var filter admission.AggregateFilter
	if err := decodeAggregateFilter(params.Filter, &filter); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid filter: %v", err)}, nil
	}

	resp, err := e.aggregateStore.Aggregate(ctx, &filter)
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Aggregate error: %v", err)}, nil
	}

	content, _ := json.Marshal(resp)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

func (e *ToolExecutor) executeRetrieveKnowledge(ctx context.Context, call ToolCall) (*ToolResult, error) {
	var params struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		TopK     int    `json:"top_k"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid retrieve_knowledge arguments: %v", err)}, nil
	}
	if params.TopK <= 0 || params.TopK > 10 {
		params.TopK = 3
	}

	if e.knowledgeStore == nil {
		return &ToolResult{ToolCallID: call.ID, Content: "Knowledge store not available"}, nil
	}

	docs, err := e.knowledgeStore.Search(ctx, params.Query, params.Category, params.TopK)
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Knowledge search error: %v", err)}, nil
	}

	result := map[string]any{
		"query":    params.Query,
		"category": params.Category,
		"count":    len(docs),
		"results":  docs,
	}
	content, _ := json.Marshal(result)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

// DefaultTools returns the default set of tool definitions for the admission agent.
func DefaultTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "apply_filter",
				Description: "Apply a filter to the admission search. filter_type can be: add, remove, replace, reset. Use snake_case filter_data fields such as region_code, subject_category_code, exclude_provinces, is_985, min_score_from, min_score_to, tag_query.",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"filter_type": map[string]any{"type": "string", "enum": []string{"add", "remove", "replace", "reset"}, "description": "How to modify the current filter"},
						"filter_data": map[string]any{"type": "object", "description": "Filter fields as JSON object"},
					},
					Required: []string{"filter_type", "filter_data"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "search_universities",
				Description: "Search universities with the current filter. Returns count and top results. Use snake_case filter fields such as region_code, subject_category_code, exclude_provinces, is_985, min_score_from, min_score_to, tag_query.",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"filter": map[string]any{"type": "object", "description": "AdmissionLineFilter as JSON object"},
						"limit":  map[string]any{"type": "integer", "description": "Max results to return (default 5, max 20)"},
					},
					Required: []string{"filter"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "aggregate_data",
				Description: "Aggregate admission data by a dimension (province, city, etc.). Returns grouped statistics. Use snake_case filter fields.",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"filter": map[string]any{"type": "object", "description": "AggregateFilter as JSON object with group_by and metrics"},
					},
					Required: []string{"filter"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "retrieve_knowledge",
				Description: "Retrieve knowledge documents for policy, major, strategy, or style-related questions. Use this when the user asks about: admission policies (提前批, 强基计划, 赋分规则), major comparisons and career advice, application strategies (冲稳保), or family/economic constraints. Returns relevant document snippets with title and content.",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"query":    map[string]any{"type": "string", "description": "The search query, rephrased to capture the core intent"},
						"category": map[string]any{"type": "string", "enum": []string{"policy", "school", "major", "case", "style", "general", "any"}, "description": "Knowledge category to search (default 'any')"},
						"top_k":    map[string]any{"type": "integer", "description": "Number of documents to retrieve (default 3, max 10)"},
					},
					Required: []string{"query"},
				},
			},
		},
	}
}
