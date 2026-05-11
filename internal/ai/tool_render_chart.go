package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderChartTool produces a sanitized echarts-compatible widget. The LLM is
// never allowed to supply an option blob directly — we accept only declarative
// fields and construct the option from scratch under a strict whitelist.
type RenderChartTool struct{}

// NewRenderChartTool constructs the render_chart tool.
func NewRenderChartTool() Tool { return &RenderChartTool{} }

func (t *RenderChartTool) Name() string { return "render_chart" }

func (t *RenderChartTool) Schema() FunctionDef {
	return FunctionDef{
		Name:        "render_chart",
		Description: "Render a chart (bar/line/pie). Use only when data clearly benefits from visualization. Pull data from a previous tool call via data_source=\"tool_result:<call_id>\" or pass inline_data.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chart_type": map[string]any{
					"type": "string",
					"enum": []string{"bar", "line", "pie"},
				},
				"title": map[string]any{"type": "string"},
				"data_source": map[string]any{
					"type":        "string",
					"description": "Either \"inline\" or \"tool_result:<call_id>\" to reference an earlier tool call from this turn.",
				},
				"inline_data": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": true,
					},
				},
				"x_field":  map[string]any{"type": "string"},
				"y_fields": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"chart_type", "title", "data_source"},
		},
	}
}

type renderChartArgs struct {
	ChartType  string           `json:"chart_type"`
	Title      string           `json:"title"`
	DataSource string           `json:"data_source"`
	InlineData []map[string]any `json:"inline_data"`
	XField     string           `json:"x_field"`
	YFields    []string         `json:"y_fields"`
}

const (
	maxChartPoints  = 200
	maxChartSeries  = 10
	maxChartTitle   = 80
	maxChartYFields = 5
)

func (t *RenderChartTool) Execute(cc *CallContext, raw json.RawMessage) ToolResult {
	var args renderChartArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return failChart("invalid arguments: " + err.Error())
	}
	if args.ChartType != "bar" && args.ChartType != "line" && args.ChartType != "pie" {
		return failChart("chart_type must be bar/line/pie")
	}
	title := stripUnsafe(args.Title)
	if len(title) > maxChartTitle {
		title = title[:maxChartTitle]
	}
	if args.XField == "" {
		args.XField = "x"
	}
	if len(args.YFields) == 0 {
		args.YFields = []string{"y"}
	}
	if len(args.YFields) > maxChartYFields {
		args.YFields = args.YFields[:maxChartYFields]
	}

	rows, err := resolveChartData(args, cc)
	if err != nil {
		return failChart(err.Error())
	}
	if len(rows) > maxChartPoints {
		rows = rows[:maxChartPoints]
	}

	option, err := buildEChartsOption(args.ChartType, title, args.XField, args.YFields, rows)
	if err != nil {
		return failChart(err.Error())
	}

	payload, err := json.Marshal(option)
	if err != nil {
		return failChart("encode option: " + err.Error())
	}

	widget := Widget{
		ID:      newWidgetID(),
		Kind:    WidgetKindChart,
		Payload: payload,
	}
	if cc.OnWidget != nil {
		cc.OnWidget(widget)
	}

	summary := fmt.Sprintf(`{"ok":true,"chart_type":%q,"points":%d}`, args.ChartType, len(rows))
	return ToolResult{Content: summary, Widget: &widget}
}

func failChart(msg string) ToolResult {
	return ToolResult{
		Content: fmt.Sprintf(`{"ok":false,"error":%q}`, msg),
		Error:   msg,
		IsError: true,
	}
}

func resolveChartData(args renderChartArgs, cc *CallContext) ([]map[string]any, error) {
	src := strings.TrimSpace(args.DataSource)
	if src == "" || src == "inline" {
		return args.InlineData, nil
	}
	if !strings.HasPrefix(src, "tool_result:") {
		return nil, fmt.Errorf("unsupported data_source %q", src)
	}
	callID := strings.TrimPrefix(src, "tool_result:")
	prior, ok := cc.ToolResults[callID]
	if !ok {
		return nil, fmt.Errorf("tool_result %q not found in current turn", callID)
	}
	if prior.IsError {
		return nil, fmt.Errorf("referenced tool_result %q is an error", callID)
	}

	// Try the common envelope first: { "items": [...] } or { "data": [...] }.
	var envelope map[string]any
	if err := json.Unmarshal([]byte(prior.Content), &envelope); err == nil {
		if v, ok := envelope["items"]; ok {
			if rows, ok := coerceRows(v); ok {
				return rows, nil
			}
		}
		if v, ok := envelope["data"]; ok {
			if rows, ok := coerceRows(v); ok {
				return rows, nil
			}
		}
	}
	// Fall back to a top-level array.
	var arr []map[string]any
	if err := json.Unmarshal([]byte(prior.Content), &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("tool_result %q does not contain a tabular array", callID)
}

func coerceRows(v any) ([]map[string]any, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		out = append(out, m)
	}
	return out, true
}

func buildEChartsOption(chartType, title, xField string, yFields []string, rows []map[string]any) (map[string]any, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("no data rows")
	}

	if chartType == "pie" {
		// pie: a single series; xField is the name, first yField is the value.
		yField := yFields[0]
		points := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			points = append(points, map[string]any{
				"name":  stringify(row[xField]),
				"value": numeric(row[yField]),
			})
		}
		return map[string]any{
			"title":   map[string]any{"text": title},
			"tooltip": map[string]any{"trigger": "item"},
			"series": []map[string]any{
				{
					"name": yField,
					"type": "pie",
					"data": points,
				},
			},
		}, nil
	}

	// bar / line
	categories := make([]string, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, stringify(row[xField]))
	}
	series := make([]map[string]any, 0, len(yFields))
	for i, yf := range yFields {
		if i >= maxChartSeries {
			break
		}
		values := make([]any, 0, len(rows))
		for _, row := range rows {
			values = append(values, numeric(row[yf]))
		}
		series = append(series, map[string]any{
			"name": yf,
			"type": chartType,
			"data": values,
		})
	}
	return map[string]any{
		"title":   map[string]any{"text": title},
		"tooltip": map[string]any{"trigger": "axis"},
		"legend":  map[string]any{"data": yFields},
		"grid":    map[string]any{"left": "8%", "right": "5%", "top": "15%", "bottom": "10%"},
		"xAxis":   map[string]any{"type": "category", "data": categories},
		"yAxis":   map[string]any{"type": "value"},
		"series":  series,
	}, nil
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func numeric(v any) any {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		var f float64
		if _, err := fmt.Sscanf(t, "%f", &f); err == nil {
			return f
		}
		return 0
	case nil:
		return 0
	default:
		return 0
	}
}

func stripUnsafe(s string) string {
	// Remove control chars and angle brackets to harden against XSS in titles.
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == '<' || r == '>' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
