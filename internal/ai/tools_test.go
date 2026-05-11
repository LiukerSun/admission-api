package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderChart_InlineBar(t *testing.T) {
	tool := NewRenderChartTool()
	args := json.RawMessage(`{
		"chart_type":"bar","title":"测试","data_source":"inline",
		"x_field":"year","y_fields":["plan"],
		"inline_data":[{"year":"2022","plan":100},{"year":"2023","plan":120}]
	}`)
	cc := &CallContext{Ctx: context.Background(), ToolResults: map[string]ToolResult{}}
	res := tool.Execute(cc, args)
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Error)
	}
	if res.Widget == nil || res.Widget.Kind != WidgetKindChart {
		t.Fatalf("expected chart widget, got %+v", res.Widget)
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Widget.Payload, &payload); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if _, ok := payload["title"]; !ok {
		t.Errorf("title missing")
	}
	if _, ok := payload["series"]; !ok {
		t.Errorf("series missing")
	}
	// Must NOT contain a 'formatter' key anywhere — backend never accepts it.
	raw, _ := json.Marshal(payload)
	if strings.Contains(string(raw), "formatter") {
		t.Errorf("formatter leaked into payload: %s", raw)
	}
}

func TestRenderChart_ResolveToolResultData(t *testing.T) {
	tool := NewRenderChartTool()
	cc := &CallContext{
		Ctx: context.Background(),
		ToolResults: map[string]ToolResult{
			"call_42": {Content: `{"items":[{"year":"2022","plan":80},{"year":"2023","plan":90}]}`},
		},
	}
	args := json.RawMessage(`{
		"chart_type":"line","title":"trend","data_source":"tool_result:call_42",
		"x_field":"year","y_fields":["plan"]
	}`)
	res := tool.Execute(cc, args)
	if res.IsError {
		t.Fatalf("error: %s", res.Error)
	}
	if res.Widget == nil {
		t.Fatalf("expected widget")
	}
}

func TestRenderChart_RejectsBadDataSource(t *testing.T) {
	tool := NewRenderChartTool()
	cc := &CallContext{Ctx: context.Background(), ToolResults: map[string]ToolResult{}}
	args := json.RawMessage(`{"chart_type":"bar","title":"x","data_source":"tool_result:missing"}`)
	res := tool.Execute(cc, args)
	if !res.IsError {
		t.Fatalf("expected error for missing tool_result reference")
	}
}

func TestRenderCard_HrefAllowlist(t *testing.T) {
	tool := NewRenderCardTool([]string{"eol.cn"})
	cc := &CallContext{Ctx: context.Background(), ToolResults: map[string]ToolResult{}}

	cases := []struct {
		name    string
		href    string
		wantErr bool
	}{
		{"allowed https", "https://eol.cn/school/42", false},
		{"disallowed host", "https://evil.example/x", true},
		{"http scheme rejected", "http://eol.cn/x", true},
		{"javascript rejected", "javascript:alert(1)", true},
		{"internal path allowed", "/schools/42", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"title": "清华",
				"link":  map[string]any{"text": "详情", "href": tc.href},
			})
			res := tool.Execute(cc, body)
			if tc.wantErr && !res.IsError {
				t.Errorf("expected error for %q", tc.href)
			}
			if !tc.wantErr && res.IsError {
				t.Errorf("unexpected error for %q: %s", tc.href, res.Error)
			}
		})
	}
}

func TestRenderCard_EscapesTitle(t *testing.T) {
	tool := NewRenderCardTool(nil)
	cc := &CallContext{Ctx: context.Background(), ToolResults: map[string]ToolResult{}}
	body, _ := json.Marshal(map[string]any{"title": `<script>alert(1)</script>`})
	res := tool.Execute(cc, body)
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Error)
	}
	var payload map[string]any
	_ = json.Unmarshal(res.Widget.Payload, &payload)
	title, _ := payload["title"].(string)
	if strings.Contains(title, "<script>") {
		t.Errorf("title not escaped: %s", title)
	}
}

func TestSuggestionsParse_Tolerant(t *testing.T) {
	cases := []struct {
		in    string
		want  int
		first string
	}{
		{`["问题一","问题二"]`, 2, "问题一"},
		{"```\n[\"a\",\"b\"]\n```", 2, "a"},
		{"不是 JSON", 0, ""},
		{`["a","b","c","d","e"]`, 4, "a"}, // clamped to 4
	}
	for _, tc := range cases {
		got := parseSuggestionsJSON(tc.in)
		if len(got) != tc.want {
			t.Errorf("input %q -> %d suggestions, want %d", tc.in, len(got), tc.want)
		}
		if tc.want > 0 && got[0] != tc.first {
			t.Errorf("first suggestion = %q, want %q", got[0], tc.first)
		}
	}
}
