package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"admission-api/internal/admission"
	"admission-api/internal/volunteerplan"
)

// ToolResult is the result of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// ToolExecContext is the per-run capability bundle handed to tool
// executions. Keeping it as a value (not a field on ToolExecutor) means
// the executor itself stays stateless and can safely be shared across
// concurrent agent runs.
//
//   - EmitWidget pushes a widget into the streaming/persistence pipeline.
//     Tools call it when they produce a structured render unit.
//   - ResolveResult looks up the textual content of a prior tool result
//     from the same run by call_id. render_chart uses it to back a chart
//     with data produced by an earlier search_universities call without
//     duplicating that payload through the LLM.
//   - CardLinkWhitelist restricts hrefs in render_card. An empty slice
//     means no external links are allowed; tools should still permit
//     relative ("/...") hrefs to the same site.
type ToolExecContext struct {
	UserID            int64
	ConversationID    int64
	EmitWidget        func(Widget)
	ResolveResult     func(callID string) (string, bool)
	CardLinkWhitelist []string
}

// ToolExecutor executes tool calls from the LLM.
type ToolExecutor struct {
	admissionLineStore admission.AdmissionLineStore
	aggregateStore     admission.AggregateStore
	recommendations    admission.RecommendationService
	draftStore         volunteerplan.DraftStore
	cardLinkWhitelist  []string
}

// NewToolExecutor creates a new tool executor.
func NewToolExecutor(
	admissionLineStore admission.AdmissionLineStore,
	aggregateStore admission.AggregateStore,
	recommendations admission.RecommendationService,
	draftStore volunteerplan.DraftStore,
) *ToolExecutor {
	return &ToolExecutor{
		admissionLineStore: admissionLineStore,
		aggregateStore:     aggregateStore,
		recommendations:    recommendations,
		draftStore:         draftStore,
	}
}

// SetCardLinkWhitelist configures the host whitelist used by render_card.
// Entries are matched case-insensitively against the URL host.
func (e *ToolExecutor) SetCardLinkWhitelist(hosts []string) {
	cleaned := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			cleaned = append(cleaned, h)
		}
	}
	e.cardLinkWhitelist = cleaned
}

// Execute runs a tool call and returns the result. The ToolExecContext
// carries per-run capabilities (widget emission, prior result lookup);
// pass an empty struct in tests that don't exercise those features.
func (e *ToolExecutor) Execute(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	switch call.Function.Name {
	case "apply_filter":
		return e.executeApplyFilter(ctx, call)
	case "search_universities":
		return e.executeSearchUniversities(ctx, call)
	case "aggregate_data":
		return e.executeAggregateData(ctx, call)
	case "render_chart":
		return e.executeRenderChart(ctx, call, execCtx)
	case "render_card":
		return e.executeRenderCard(ctx, call, execCtx)
	case "generate_volunteer_plan_draft":
		return e.executeGenerateVolunteerPlanDraft(ctx, call, execCtx)
	default:
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Unknown tool: %s", call.Function.Name)}, nil
	}
}

func (e *ToolExecutor) executeGenerateVolunteerPlanDraft(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	if execCtx.UserID <= 0 || execCtx.ConversationID <= 0 {
		return &ToolResult{ToolCallID: call.ID, Content: `{"status":"failed","error":"generate_volunteer_plan_draft requires conversation context"}`}, nil
	}

	var req admission.RecommendationRequest
	if err := json.Unmarshal([]byte(call.Function.Arguments), &req); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf(`{"status":"failed","error":"invalid arguments: %v"}`, err)}, nil
	}

	if req.RegionCode == "" || req.SubjectCategoryCode == "" || req.TotalScore == 0 || req.ProvincialRank == 0 {
		return &ToolResult{ToolCallID: call.ID, Content: `{"status":"failed","error":"missing required fields: region_code, subject_category_code, total_score, provincial_rank"}`}, nil
	}
	if req.RegionCode != "230000" {
		return &ToolResult{ToolCallID: call.ID, Content: `{"status":"failed","error":"only supports region_code=230000"}`}, nil
	}

	inputJSON, _ := json.Marshal(req)
	draftID, err := e.draftStore.Create(ctx, execCtx.UserID, execCtx.ConversationID, inputJSON, "recommendations_v1")
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf(`{"status":"failed","error":"create draft failed: %v"}`, err)}, nil
	}

	resp, err := e.recommendations.Recommend(ctx, &req)
	if err != nil {
		_ = e.draftStore.MarkFailed(ctx, execCtx.UserID, draftID, err.Error())
		payload := map[string]any{"draft_id": draftID, "status": "failed", "error": err.Error()}
		content, _ := json.Marshal(payload)
		return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
	}
	if resp == nil || resp.VolunteerPlan == nil {
		_ = e.draftStore.MarkFailed(ctx, execCtx.UserID, draftID, "recommendation response missing volunteer_plan")
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf(`{"draft_id":%d,"status":"failed","error":"recommendation response missing volunteer_plan"}`, draftID)}, nil
	}

	planJSON, _ := json.Marshal(resp.VolunteerPlan)
	if err := e.draftStore.MarkReady(ctx, execCtx.UserID, draftID, planJSON); err != nil {
		_ = e.draftStore.MarkFailed(ctx, execCtx.UserID, draftID, err.Error())
		payload := map[string]any{"draft_id": draftID, "status": "failed", "error": fmt.Sprintf("persist draft failed: %v", err)}
		content, _ := json.Marshal(payload)
		return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
	}

	out := map[string]any{
		"draft_id":    draftID,
		"status":      "ready",
		"strategy":    resp.Strategy,
		"rush_count":  resp.RushCount,
		"match_count": resp.MatchCount,
		"safe_count":  resp.SafeCount,
		"rank_window": resp.RankWindow,
	}
	content, _ := json.Marshal(out)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
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

// Allowed chart types — we control the rendered echarts options, but
// still limit which kinds the model can pick so unsupported shapes
// fail fast instead of producing empty visuals client-side.
var allowedChartTypes = map[string]bool{
	"bar":  true,
	"line": true,
	"pie":  true,
}

// chartParams is what the LLM is allowed to send. Notice there is NO
// way for the model to pass raw echarts option JSON — the server-side
// builder takes these high-level intent fields and constructs a
// whitelisted echarts option from them.
type chartParams struct {
	ChartType  string           `json:"chart_type"`
	Title      string           `json:"title"`
	DataSource string           `json:"data_source"` // "tool_result:<call_id>" or "inline"
	InlineData []map[string]any `json:"inline_data"`
	XField     string           `json:"x_field"`
	YFields    []string         `json:"y_fields"`
}

func (e *ToolExecutor) executeRenderChart(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	var p chartParams
	if err := json.Unmarshal([]byte(call.Function.Arguments), &p); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid render_chart arguments: %v", err)}, nil
	}
	if !allowedChartTypes[p.ChartType] {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Unsupported chart_type: %s", p.ChartType)}, nil
	}

	rows, err := resolveChartData(&p, execCtx)
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("render_chart: %v", err)}, nil
	}
	if len(rows) == 0 {
		return &ToolResult{ToolCallID: call.ID, Content: "render_chart: no data points"}, nil
	}
	if p.XField == "" {
		return &ToolResult{ToolCallID: call.ID, Content: "render_chart: x_field is required"}, nil
	}
	if p.ChartType != "pie" && len(p.YFields) == 0 {
		return &ToolResult{ToolCallID: call.ID, Content: "render_chart: y_fields is required for bar/line"}, nil
	}

	option := buildEchartsOption(&p, rows)
	widget := Widget{
		ID:   NewWidgetID(),
		Kind: "chart",
		Payload: map[string]any{
			"title":  p.Title,
			"type":   p.ChartType,
			"option": option,
		},
	}
	if execCtx.EmitWidget != nil {
		execCtx.EmitWidget(widget)
	}
	return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Chart rendered (%d points)", len(rows))}, nil
}

// resolveChartData picks data rows either from a prior tool result or
// from the inline_data field. When pulling from a prior tool result, we
// expect that result's content to be JSON with either {"top": [...]}
// (the search_universities shape) or a top-level array.
func resolveChartData(p *chartParams, execCtx ToolExecContext) ([]map[string]any, error) {
	if p.DataSource == "" || p.DataSource == "inline" {
		return p.InlineData, nil
	}
	if !strings.HasPrefix(p.DataSource, "tool_result:") {
		return nil, fmt.Errorf("data_source must be \"inline\" or \"tool_result:<id>\"")
	}
	if execCtx.ResolveResult == nil {
		return nil, fmt.Errorf("no resolver for tool_result references")
	}
	id := strings.TrimPrefix(p.DataSource, "tool_result:")
	content, ok := execCtx.ResolveResult(id)
	if !ok {
		return nil, fmt.Errorf("tool_result %s not found in this run", id)
	}
	// Try the search_universities shape first; fall back to aggregate_data shape, then bare array.
	var wrapper struct {
		Top []map[string]any `json:"top"`
	}
	if err := json.Unmarshal([]byte(content), &wrapper); err == nil && len(wrapper.Top) > 0 {
		return wrapper.Top, nil
	}
	var aggWrapper struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(content), &aggWrapper); err == nil && len(aggWrapper.Items) > 0 {
		return aggWrapper.Items, nil
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(content), &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("tool_result %s is not chartable", id)
}

// buildEchartsOption assembles a strictly-whitelisted echarts option
// from chart params and rows. ONLY the fields we add here end up in the
// payload, so the LLM cannot smuggle formatter strings, JS expressions,
// or unrelated keys into the rendered chart.
func buildEchartsOption(p *chartParams, rows []map[string]any) map[string]any {
	option := map[string]any{
		"title":   map[string]any{"text": sanitizeString(p.Title)},
		"tooltip": map[string]any{"trigger": "axis"},
		"grid":    map[string]any{"left": "10%", "right": "5%", "bottom": "10%"},
	}

	if p.ChartType == "pie" {
		// Pie uses a single y-field as the value channel; default to the
		// first available y_field, else the second column of the row.
		valueField := ""
		if len(p.YFields) > 0 {
			valueField = p.YFields[0]
		}
		data := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			name, _ := r[p.XField].(string)
			value := numericValue(r, valueField)
			data = append(data, map[string]any{
				"name":  sanitizeString(name),
				"value": value,
			})
		}
		option["series"] = []map[string]any{{
			"type": "pie",
			"data": data,
		}}
		option["legend"] = map[string]any{"orient": "horizontal", "bottom": 0}
		return option
	}

	// bar / line: shared xAxis / yAxis / series structure
	xCategories := make([]string, 0, len(rows))
	for _, r := range rows {
		x, _ := r[p.XField].(string)
		xCategories = append(xCategories, sanitizeString(x))
	}
	option["xAxis"] = map[string]any{
		"type": "category",
		"data": xCategories,
	}
	option["yAxis"] = map[string]any{"type": "value"}

	series := make([]map[string]any, 0, len(p.YFields))
	legend := make([]string, 0, len(p.YFields))
	for _, yf := range p.YFields {
		values := make([]float64, 0, len(rows))
		for _, r := range rows {
			values = append(values, numericValue(r, yf))
		}
		series = append(series, map[string]any{
			"name": sanitizeString(yf),
			"type": p.ChartType,
			"data": values,
		})
		legend = append(legend, sanitizeString(yf))
	}
	option["series"] = series
	option["legend"] = map[string]any{"data": legend}
	return option
}

// numericValue extracts a float from a row's field, coercing
// number-typed JSON values to float64. Anything non-numeric becomes 0
// rather than crashing — bad input becomes a visible zero on the chart,
// not a server error.
func numericValue(row map[string]any, field string) float64 {
	v, ok := row[field]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
}

// sanitizeString trims values used as text labels in chart options.
// We do not strip HTML — the frontend renders echarts options through
// the echarts library which treats labels as text — but we cap length
// so a runaway model cannot ship a megabyte of label text.
func sanitizeString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// cardParams is the LLM-facing shape for render_card.
type cardParams struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Metrics     []struct {
		Label string `json:"label"`
		Value string `json:"value"`
		Trend string `json:"trend"`
	} `json:"metrics"`
	Link struct {
		Text string `json:"text"`
		Href string `json:"href"`
	} `json:"link"`
}

var allowedCardTrends = map[string]bool{
	"":     true, // omitted is fine
	"up":   true,
	"down": true,
	"flat": true,
}

func (e *ToolExecutor) executeRenderCard(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	var p cardParams
	if err := json.Unmarshal([]byte(call.Function.Arguments), &p); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid render_card arguments: %v", err)}, nil
	}
	if strings.TrimSpace(p.Title) == "" {
		return &ToolResult{ToolCallID: call.ID, Content: "render_card: title is required"}, nil
	}

	metrics := make([]map[string]any, 0, len(p.Metrics))
	for _, m := range p.Metrics {
		if !allowedCardTrends[m.Trend] {
			m.Trend = ""
		}
		metrics = append(metrics, map[string]any{
			"label": sanitizeString(m.Label),
			"value": sanitizeString(m.Value),
			"trend": m.Trend,
		})
	}

	payload := map[string]any{
		"title":       sanitizeString(p.Title),
		"description": sanitizeString(p.Description),
		"metrics":     metrics,
	}
	whitelist := execCtx.CardLinkWhitelist
	if whitelist == nil {
		whitelist = e.cardLinkWhitelist
	}
	if href := strings.TrimSpace(p.Link.Href); href != "" {
		if !isAllowedCardLink(href, whitelist) {
			return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("render_card: link %s not allowed", href)}, nil
		}
		payload["link"] = map[string]any{
			"text": sanitizeString(p.Link.Text),
			"href": href,
		}
	}

	widget := Widget{ID: NewWidgetID(), Kind: "card", Payload: payload}
	if execCtx.EmitWidget != nil {
		execCtx.EmitWidget(widget)
	}
	return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Card rendered: %s", payload["title"])}, nil
}

// isAllowedCardLink returns true if href is a relative same-site link
// (starts with "/") or points at a host in the configured whitelist.
// We reject any scheme other than https for absolute URLs so the model
// can't slip data: or javascript: payloads past the frontend.
func isAllowedCardLink(href string, whitelist []string) bool {
	if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
		return true
	}
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range whitelist {
		if host == allowed {
			return true
		}
	}
	return false
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
				Name:        "generate_volunteer_plan_draft",
				Description: "Generate a volunteer plan draft for the current conversation by running the recommendation algorithm. Requires region_code=230000, subject_category_code, total_score, provincial_rank. Returns draft_id and summary.",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"region_code":             map[string]any{"type": "string"},
						"subject_category_code":   map[string]any{"type": "string", "enum": []string{"physics", "history"}},
						"subject_requirement_code": map[string]any{"type": "string"},
						"selected_subjects":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"total_score":              map[string]any{"type": "integer"},
						"provincial_rank":          map[string]any{"type": "integer"},
						"preferred_majors":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"excluded_majors":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"excluded_keywords":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"preferred_cities":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"excluded_cities":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"preferred_provinces":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"excluded_provinces":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"priority_strategy":        map[string]any{"type": "string", "enum": []string{"auto", "school", "major"}},
						"plan_size":                map[string]any{"type": "integer"},
						"enable_llm_tuning":        map[string]any{"type": "boolean"},
					},
					Required: []string{"region_code", "subject_category_code", "total_score", "provincial_rank"},
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
				Name:        "render_chart",
				Description: "Render a chart in the chat UI. Only use when the user explicitly asks for a visualization or when comparing several numeric series side-by-side is clearly more useful than prose. Supply data via inline_data or by referencing a prior tool_result.",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"chart_type":  map[string]any{"type": "string", "enum": []string{"bar", "line", "pie"}, "description": "Chart shape"},
						"title":       map[string]any{"type": "string", "description": "Chart title"},
						"data_source": map[string]any{"type": "string", "description": "\"inline\" to use inline_data, or \"tool_result:<call_id>\" to chart a previous tool result from this run"},
						"inline_data": map[string]any{"type": "array", "description": "Rows used when data_source is \"inline\"; each row is an object with x_field and y_fields keys"},
						"x_field":     map[string]any{"type": "string", "description": "Field name used as the x-axis category"},
						"y_fields":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Numeric fields plotted as series (one series per field)"},
					},
					Required: []string{"chart_type", "x_field"},
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
				Name:        "render_card",
				Description: "Render a structured information card (e.g. for a university or a major). Use when summarizing a single entity with a few key metrics, not for free-form prose.",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"title":       map[string]any{"type": "string", "description": "Card title"},
						"description": map[string]any{"type": "string", "description": "One-line subtitle"},
						"metrics": map[string]any{
							"type":        "array",
							"description": "Up to ~4 metric tiles displayed on the card",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"label": map[string]any{"type": "string"},
									"value": map[string]any{"type": "string"},
									"trend": map[string]any{"type": "string", "enum": []string{"up", "down", "flat"}},
								},
								"required": []string{"label", "value"},
							},
						},
						"link": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
								"href": map[string]any{"type": "string", "description": "Relative path or whitelisted https URL"},
							},
						},
					},
					Required: []string{"title"},
				},
			},
		},
	}
}
