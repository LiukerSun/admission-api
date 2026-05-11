package ai

import (
	"crypto/rand"
	"encoding/hex"
)

// newWidgetID returns a short random identifier suitable for correlating an
// SSE widget event with its persisted record. Not cryptographically meaningful;
// just collision-resistant enough within a single conversation.
func newWidgetID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "widget_fallback"
	}
	return "w_" + hex.EncodeToString(b[:])
}
