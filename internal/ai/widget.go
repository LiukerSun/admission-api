package ai

import (
	"crypto/rand"
	"encoding/hex"
)

// Widget represents a structured display unit emitted by a tool call.
// It is shipped both inline over SSE (as a "widget" event) and persisted
// alongside the assistant message so historical replay reproduces the
// same UI. The payload is intentionally a free-form map because each
// kind (chart, card, …) has its own schema; the server-side tool
// constructors are the ones that enforce per-kind invariants — never
// the LLM.
type Widget struct {
	ID      string         `json:"id"`
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

// Per-run upper bound on widgets to prevent a runaway LLM from flooding
// the SSE stream and assistant row with chart spam. Enforced inside the
// agent loop, not the tool, so it applies across both render_chart and
// render_card calls in a single turn.
const MaxWidgetsPerRun = 5

// NewWidgetID returns an opaque random ID used by the frontend to
// de-duplicate widgets across SSE replays and history reads. Crypto rand
// is used so IDs are not predictable, not for security — collisions
// would just cause a single widget to render twice.
func NewWidgetID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand never fails on supported platforms; fall back to
		// a fixed prefix to keep the producer chain alive rather than
		// panic mid-stream.
		return "wgt_fallback"
	}
	return "wgt_" + hex.EncodeToString(b[:])
}
