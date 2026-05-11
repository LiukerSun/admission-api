package ai

import "encoding/json"

// Widget kinds.
const (
	WidgetKindChart = "chart"
	WidgetKindCard  = "card"
)

// Widget is a structured UI element produced by a tool and broadcast over SSE.
// The Payload shape depends on Kind and is validated by the tool that emits
// the widget; persistence is opaque (stored as JSONB in the messages table).
type Widget struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}
