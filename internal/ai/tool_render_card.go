package ai

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
)

// RenderCardTool produces an info card widget. Free-text fields are
// HTML-escaped; link hrefs are validated against a host allowlist supplied at
// construction time so the LLM cannot funnel users to arbitrary domains.
type RenderCardTool struct {
	allowedHosts map[string]struct{}
}

// NewRenderCardTool constructs the render_card tool with the given host
// allowlist. Hosts are case-insensitive. Pass an empty slice to reject all
// external links (internal relative paths are always allowed).
func NewRenderCardTool(allowedHosts []string) Tool {
	set := make(map[string]struct{}, len(allowedHosts))
	for _, h := range allowedHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			set[h] = struct{}{}
		}
	}
	return &RenderCardTool{allowedHosts: set}
}

func (t *RenderCardTool) Name() string { return "render_card" }

func (t *RenderCardTool) Schema() FunctionDef {
	return FunctionDef{
		Name:        "render_card",
		Description: "Render a structured info card to highlight a school/major/program. Use when recommending one specific item with up to 6 key metrics.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":       map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
				"metrics": map[string]any{
					"type": "array",
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
						"href": map[string]any{"type": "string"},
					},
				},
			},
			"required": []string{"title"},
		},
	}
}

type cardMetric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Trend string `json:"trend,omitempty"`
}

type cardLink struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

type renderCardArgs struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Metrics     []cardMetric `json:"metrics"`
	Link        *cardLink    `json:"link"`
}

const (
	maxCardTitle       = 80
	maxCardDescription = 400
	maxCardMetrics     = 6
	maxCardLabelLen    = 40
	maxCardValueLen    = 60
)

func (t *RenderCardTool) Execute(cc *CallContext, raw json.RawMessage) ToolResult {
	var args renderCardArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return failCard("invalid arguments: " + err.Error())
	}
	title := truncateEscape(args.Title, maxCardTitle)
	if title == "" {
		return failCard("title is required")
	}
	desc := truncateEscape(args.Description, maxCardDescription)

	if len(args.Metrics) > maxCardMetrics {
		args.Metrics = args.Metrics[:maxCardMetrics]
	}
	cleanMetrics := make([]cardMetric, 0, len(args.Metrics))
	for _, m := range args.Metrics {
		label := truncateEscape(m.Label, maxCardLabelLen)
		value := truncateEscape(m.Value, maxCardValueLen)
		if label == "" || value == "" {
			continue
		}
		trend := strings.ToLower(strings.TrimSpace(m.Trend))
		if trend != "up" && trend != "down" && trend != "flat" {
			trend = ""
		}
		cleanMetrics = append(cleanMetrics, cardMetric{Label: label, Value: value, Trend: trend})
	}

	var link *cardLink
	if args.Link != nil && args.Link.Href != "" {
		validated, ok := t.validateLink(args.Link.Href)
		if !ok {
			return failCard("link href not allowed")
		}
		link = &cardLink{
			Text: truncateEscape(args.Link.Text, maxCardValueLen),
			Href: validated,
		}
	}

	payload := map[string]any{
		"title":       title,
		"description": desc,
		"metrics":     cleanMetrics,
	}
	if link != nil {
		payload["link"] = link
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return failCard("encode payload: " + err.Error())
	}
	widget := Widget{
		ID:      newWidgetID(),
		Kind:    WidgetKindCard,
		Payload: encoded,
	}
	if cc.OnWidget != nil {
		cc.OnWidget(widget)
	}
	return ToolResult{
		Content: fmt.Sprintf(`{"ok":true,"metrics":%d}`, len(cleanMetrics)),
		Widget:  &widget,
	}
}

func (t *RenderCardTool) validateLink(href string) (string, bool) {
	href = strings.TrimSpace(href)
	if href == "" {
		return "", false
	}
	// Relative path → internal link.
	if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
		return href, true
	}
	u, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	if u.Scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	if _, ok := t.allowedHosts[host]; !ok {
		return "", false
	}
	return u.String(), true
}

func failCard(msg string) ToolResult {
	return ToolResult{
		Content: fmt.Sprintf(`{"ok":false,"error":%q}`, msg),
		Error:   msg,
		IsError: true,
	}
}

func truncateEscape(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len([]rune(s)) > max {
		r := []rune(s)
		s = string(r[:max])
	}
	return html.EscapeString(s)
}
