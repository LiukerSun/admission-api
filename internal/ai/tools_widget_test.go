package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// ---------- isAllowedCardLink ----------------------------------------
//
// This function is the only thing standing between an LLM hallucinating
// "click here" links and the user landing on a phishing site or
// triggering a javascript: payload. The whitelist behaviour is
// security-sensitive, so every path is pinned.
func TestIsAllowedCardLink(t *testing.T) {
	whitelist := []string{"trusted.example.com", "docs.example.com"}

	cases := []struct {
		name string
		href string
		want bool
	}{
		{"relative same-site path", "/universities/123", true},
		{"relative with query", "/search?q=tsinghua", true},
		{"protocol-relative is rejected", "//evil.example.com/x", false},
		{"https on whitelisted host", "https://trusted.example.com/x", true},
		{"https on second whitelisted host", "https://docs.example.com/x", true},
		{"https on unknown host", "https://evil.example.com/x", false},
		{"http even on whitelisted host", "http://trusted.example.com/x", false},
		{"javascript: scheme", "javascript:alert(1)", false},
		{"data: scheme", "data:text/html,<script>alert(1)</script>", false},
		{"file: scheme", "file:///etc/passwd", false},
		{"malformed URL", "https://[::1", false},
		{"empty string", "", false},
		// Hostname case must not matter — most browsers lowercase hosts.
		{"uppercase host matches lowercase whitelist", "https://TRUSTED.EXAMPLE.COM/x", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isAllowedCardLink(tc.href, whitelist)
			if got != tc.want {
				t.Fatalf("isAllowedCardLink(%q) = %v, want %v", tc.href, got, tc.want)
			}
		})
	}
}

// ---------- executeRenderChart --------------------------------------
//
// render_chart is the most LLM-untrusted tool: the model picks a chart
// type and points us at some rows, and we build a fully whitelisted
// echarts option from those rows. The tests below pin three invariants:
//
//  1. valid arguments produce a widget with the expected option shape
//  2. invalid arguments produce a tool result the LLM can read, NOT a
//     widget — the SSE stream must not show a half-baked chart
//  3. the produced option NEVER contains keys outside the whitelist
//     (no formatter strings, no JS expressions, no arbitrary fields)
func TestExecuteRenderChartBuildsBarOption(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"chart_type": "bar",
		"title": "热门院校录取分对比",
		"data_source": "inline",
		"inline_data": [
			{"name": "清华", "score": 695},
			{"name": "北大", "score": 690}
		],
		"x_field": "name",
		"y_fields": ["score"]
	}`
	res, err := executor.Execute(context.Background(), newToolCall("c1", "render_chart", args), ToolExecContext{EmitWidget: emitted.fn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Content, "Chart rendered") {
		t.Fatalf("expected success content, got %q", res.Content)
	}
	w := emitted.must(t)
	if w.Kind != "chart" {
		t.Fatalf("kind = %q, want chart", w.Kind)
	}
	if w.ID == "" || !strings.HasPrefix(w.ID, "wgt_") {
		t.Fatalf("widget id should look like wgt_*, got %q", w.ID)
	}
	option := mustOption(t, w.Payload)
	assertWhitelistedKeysOnly(t, option, allowedEchartsTopKeys())
	if _, ok := option["xAxis"]; !ok {
		t.Fatalf("bar chart should set xAxis, got option=%#v", option)
	}
	series := option["series"].([]map[string]any)
	if len(series) != 1 {
		t.Fatalf("expected one series for one y_field, got %d", len(series))
	}
	if series[0]["type"] != "bar" {
		t.Fatalf("series type = %v, want bar", series[0]["type"])
	}
	// Series data must be a plain float64 slice — no objects, no
	// formatters smuggled in via a numeric position.
	data, ok := series[0]["data"].([]float64)
	if !ok {
		t.Fatalf("series data should be []float64, got %T", series[0]["data"])
	}
	if len(data) != 2 || data[0] != 695 || data[1] != 690 {
		t.Fatalf("series data = %v, want [695 690]", data)
	}
}

func TestExecuteRenderChartBuildsPieOption(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"chart_type": "pie",
		"title": "分布",
		"data_source": "inline",
		"inline_data": [
			{"name": "A", "value": 30},
			{"name": "B", "value": 70}
		],
		"x_field": "name",
		"y_fields": ["value"]
	}`
	res, err := executor.Execute(context.Background(), newToolCall("c2", "render_chart", args), ToolExecContext{EmitWidget: emitted.fn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Content, "Chart rendered") {
		t.Fatalf("expected success content, got %q", res.Content)
	}
	w := emitted.must(t)
	option := mustOption(t, w.Payload)
	// Pie has no xAxis/yAxis.
	if _, ok := option["xAxis"]; ok {
		t.Fatalf("pie should not have xAxis")
	}
	if _, ok := option["yAxis"]; ok {
		t.Fatalf("pie should not have yAxis")
	}
	series := option["series"].([]map[string]any)
	if series[0]["type"] != "pie" {
		t.Fatalf("series type = %v, want pie", series[0]["type"])
	}
}

func TestExecuteRenderChartRejectsUnsupportedChartType(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"chart_type": "scatter3d",
		"title": "x",
		"data_source": "inline",
		"inline_data": [{"a": 1}],
		"x_field": "a",
		"y_fields": ["a"]
	}`
	res, _ := executor.Execute(context.Background(), newToolCall("c3", "render_chart", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "Unsupported chart_type") {
		t.Fatalf("expected rejection content, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted for unsupported chart_type")
	}
}

func TestExecuteRenderChartRejectsMissingXField(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"chart_type": "bar",
		"data_source": "inline",
		"inline_data": [{"a": 1, "b": 2}],
		"y_fields": ["b"]
	}`
	res, _ := executor.Execute(context.Background(), newToolCall("c4", "render_chart", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "x_field is required") {
		t.Fatalf("expected x_field required error, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted when x_field is missing")
	}
}

func TestExecuteRenderChartRejectsEmptyData(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"chart_type": "bar",
		"title": "x",
		"data_source": "inline",
		"inline_data": [],
		"x_field": "a",
		"y_fields": ["b"]
	}`
	res, _ := executor.Execute(context.Background(), newToolCall("c5", "render_chart", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "no data points") {
		t.Fatalf("expected no-data-points error, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted when data is empty")
	}
}

func TestExecuteRenderChartResolvesPriorToolResult(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	prior := `{"top": [
		{"city": "哈尔滨", "rank": 100},
		{"city": "深圳", "rank": 50}
	]}`
	resolver := func(id string) (string, bool) {
		if id == "tr1" {
			return prior, true
		}
		return "", false
	}
	args := `{
		"chart_type": "line",
		"title": "排名",
		"data_source": "tool_result:tr1",
		"x_field": "city",
		"y_fields": ["rank"]
	}`
	res, _ := executor.Execute(context.Background(), newToolCall("c6", "render_chart", args), ToolExecContext{EmitWidget: emitted.fn, ResolveResult: resolver})
	if !strings.Contains(res.Content, "Chart rendered") {
		t.Fatalf("expected success content, got %q", res.Content)
	}
	w := emitted.must(t)
	option := mustOption(t, w.Payload)
	xAxis := option["xAxis"].(map[string]any)
	cats := xAxis["data"].([]string)
	if len(cats) != 2 || cats[0] != "哈尔滨" || cats[1] != "深圳" {
		t.Fatalf("x categories = %v, want [哈尔滨 深圳]", cats)
	}
}

func TestExecuteRenderChartRejectsUnknownToolResult(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"chart_type": "bar",
		"title": "x",
		"data_source": "tool_result:missing",
		"x_field": "a",
		"y_fields": ["b"]
	}`
	res, _ := executor.Execute(context.Background(), newToolCall("c7", "render_chart", args), ToolExecContext{
		EmitWidget:    emitted.fn,
		ResolveResult: func(id string) (string, bool) { return "", false },
	})
	if !strings.Contains(res.Content, "missing") {
		t.Fatalf("expected not-found error, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted for unknown tool_result")
	}
}

// ---------- executeRenderCard ---------------------------------------

func TestExecuteRenderCardEmitsWidgetWithMetrics(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	executor.SetCardLinkWhitelist([]string{"admission.example.com"})
	emitted := captureWidget()
	args := `{
		"title": "清华大学",
		"description": "顶尖985院校",
		"metrics": [
			{"label": "最低分", "value": "695", "trend": "up"},
			{"label": "录取人数", "value": "200", "trend": "flat"}
		],
		"link": {"text": "查看详情", "href": "/universities/1"}
	}`
	res, err := executor.Execute(context.Background(), newToolCall("c10", "render_card", args), ToolExecContext{EmitWidget: emitted.fn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Content, "Card rendered") {
		t.Fatalf("expected success content, got %q", res.Content)
	}
	w := emitted.must(t)
	if w.Kind != "card" {
		t.Fatalf("kind = %q, want card", w.Kind)
	}
	metrics, ok := w.Payload["metrics"].([]map[string]any)
	if !ok {
		t.Fatalf("metrics should be []map[string]any, got %T", w.Payload["metrics"])
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}
	link, ok := w.Payload["link"].(map[string]any)
	if !ok {
		t.Fatalf("expected link payload, got %T", w.Payload["link"])
	}
	if link["href"] != "/universities/1" {
		t.Fatalf("relative href should pass through, got %v", link["href"])
	}
}

func TestExecuteRenderCardRejectsUnwhitelistedLink(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	executor.SetCardLinkWhitelist([]string{"admission.example.com"})
	emitted := captureWidget()
	args := `{
		"title": "x",
		"link": {"text": "x", "href": "https://evil.com/x"}
	}`
	res, _ := executor.Execute(context.Background(), newToolCall("c11", "render_card", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "not allowed") {
		t.Fatalf("expected link rejection content, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted for unwhitelisted link")
	}
}

func TestExecuteRenderCardSanitizesUnknownTrendValue(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"title": "x",
		"metrics": [
			{"label": "a", "value": "1", "trend": "rocket"}
		]
	}`
	_, _ = executor.Execute(context.Background(), newToolCall("c12", "render_card", args), ToolExecContext{EmitWidget: emitted.fn})
	w := emitted.must(t)
	metrics := w.Payload["metrics"].([]map[string]any)
	if metrics[0]["trend"] != "" {
		t.Fatalf("unknown trend should collapse to empty string, got %v", metrics[0]["trend"])
	}
}

func TestExecuteRenderCardRequiresTitle(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{"title": "   "}`
	res, _ := executor.Execute(context.Background(), newToolCall("c13", "render_card", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "title is required") {
		t.Fatalf("expected title required error, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted without a title")
	}
}

// ---------- executeRenderForm ---------------------------------------
//
// render_form 是把 LLM 偏好收集从"逐条文字追问"压缩成"一次表单"的入口。
// 由于字段选项是后端白名单，LLM 不能凭空生成 options，所以测试只需要
// 确认：(1) 引用白名单内字段成功；(2) 引用未注册字段失败；(3) 不发
// widget 时也返回可读的错误内容；(4) payload 形状对得上前端契约。
func TestExecuteRenderFormEmitsWidgetForWhitelistedFields(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"title": "地域和方向偏好",
		"intro": "勾完一次就能开始筛选",
		"fields": ["preferred_cities", "required_majors", "family_economy"],
		"submit_label": "提交"
	}`
	res, err := executor.Execute(context.Background(), newToolCall("f1", "render_form", args), ToolExecContext{EmitWidget: emitted.fn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Content, "form_rendered") {
		t.Fatalf("expected form_rendered status, got %q", res.Content)
	}
	w := emitted.must(t)
	if w.Kind != "form" {
		t.Fatalf("kind = %q, want form", w.Kind)
	}
	if w.Payload["title"] != "地域和方向偏好" {
		t.Fatalf("title = %v, want 地域和方向偏好", w.Payload["title"])
	}
	if w.Payload["submit_label"] != "提交" {
		t.Fatalf("submit_label = %v, want 提交", w.Payload["submit_label"])
	}
	fields, ok := w.Payload["fields"].([]formFieldDef)
	if !ok {
		t.Fatalf("fields should be []formFieldDef, got %T", w.Payload["fields"])
	}
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	if fields[0].Key != "preferred_cities" || fields[0].Type != formFieldMultiSelect {
		t.Fatalf("first field shape unexpected: %+v", fields[0])
	}
	if fields[2].Key != "family_economy" || fields[2].Type != formFieldSingleSelect {
		t.Fatalf("third field shape unexpected: %+v", fields[2])
	}
}

func TestExecuteRenderFormDefaultsSubmitLabel(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"title": "x",
		"fields": ["plan_size"]
	}`
	_, _ = executor.Execute(context.Background(), newToolCall("f2", "render_form", args), ToolExecContext{EmitWidget: emitted.fn})
	w := emitted.must(t)
	if w.Payload["submit_label"] != "提交并继续" {
		t.Fatalf("expected default submit_label, got %v", w.Payload["submit_label"])
	}
}

func TestExecuteRenderFormRejectsUnknownField(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"title": "x",
		"fields": ["preferred_cities", "nuclear_codes"]
	}`
	res, _ := executor.Execute(context.Background(), newToolCall("f3", "render_form", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "unknown field") || !strings.Contains(res.Content, "nuclear_codes") {
		t.Fatalf("expected unknown field error mentioning nuclear_codes, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted when any field is unknown")
	}
}

func TestExecuteRenderFormRequiresTitle(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{"title": "   ", "fields": ["plan_size"]}`
	res, _ := executor.Execute(context.Background(), newToolCall("f4", "render_form", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "title is required") {
		t.Fatalf("expected title required error, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted without a title")
	}
}

func TestExecuteRenderFormRejectsEmptyFields(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{"title": "x", "fields": []}`
	res, _ := executor.Execute(context.Background(), newToolCall("f5", "render_form", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "at least one field") {
		t.Fatalf("expected at-least-one-field error, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted without fields")
	}
}

func TestExecuteRenderFormRejectsTooManyFields(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"title": "x",
		"fields": [
			"preferred_cities","excluded_cities","preferred_majors",
			"required_majors","excluded_majors","family_economy",
			"priority_strategy"
		]
	}`
	res, _ := executor.Execute(context.Background(), newToolCall("f6", "render_form", args), ToolExecContext{EmitWidget: emitted.fn})
	if !strings.Contains(res.Content, "at most") {
		t.Fatalf("expected too-many-fields error, got %q", res.Content)
	}
	if emitted.count() != 0 {
		t.Fatalf("widget MUST NOT be emitted when fields cap is exceeded")
	}
}

func TestExecuteRenderFormDedupesRepeatedField(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, nil, nil, nil)
	emitted := captureWidget()
	args := `{
		"title": "x",
		"fields": ["plan_size", "plan_size", "family_economy"]
	}`
	_, _ = executor.Execute(context.Background(), newToolCall("f7", "render_form", args), ToolExecContext{EmitWidget: emitted.fn})
	w := emitted.must(t)
	fields := w.Payload["fields"].([]formFieldDef)
	if len(fields) != 2 {
		t.Fatalf("expected dedup to 2 fields, got %d", len(fields))
	}
}

// ---------- helpers -------------------------------------------------

// widgetCapture records widgets emitted by tool calls under test so
// each test can assert how many were sent and inspect the payload of
// the most recent one.
type widgetCapture struct {
	got []Widget
}

func captureWidget() *widgetCapture {
	return &widgetCapture{}
}

func (w *widgetCapture) fn(widget Widget) {
	w.got = append(w.got, widget)
}

func (w *widgetCapture) count() int {
	return len(w.got)
}

func (w *widgetCapture) must(t *testing.T) Widget {
	t.Helper()
	if len(w.got) == 0 {
		t.Fatalf("expected a widget to be emitted, got none")
	}
	return w.got[len(w.got)-1]
}

func mustOption(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	opt, ok := payload["option"].(map[string]any)
	if !ok {
		// JSON-marshal + unmarshal to expose the shape if it's a
		// non-map type — useful diagnostic when this test breaks.
		raw, _ := json.Marshal(payload)
		t.Fatalf("expected payload.option to be a map[string]any, payload was %s", string(raw))
	}
	return opt
}

func allowedEchartsTopKeys() []string {
	// Mirrors the whitelist enforced by buildEchartsOption — kept in
	// sync manually because the production code names these keys
	// inline in map literals. If you add a new whitelisted top-level
	// key there, add it here too.
	return []string{"title", "tooltip", "grid", "xAxis", "yAxis", "series", "legend"}
}

func assertWhitelistedKeysOnly(t *testing.T, option map[string]any, allowed []string) {
	t.Helper()
	allow := make(map[string]bool, len(allowed))
	for _, k := range allowed {
		allow[k] = true
	}
	for k := range option {
		if !allow[k] {
			t.Fatalf("echarts option contains non-whitelisted key %q; full option=%#v", k, option)
		}
	}
}
