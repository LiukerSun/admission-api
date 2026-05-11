package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// SSE event names broadcast to the frontend.
const (
	EventTextDelta     = "text_delta"
	EventToolCallStart = "tool_call_start"
	EventToolCallEnd   = "tool_call_end"
	EventWidget        = "widget"
	EventDone          = "done"
	EventError         = "error"
	EventWarning       = "warning"
)

// sseWriter wraps a gin.Context to emit SSE-formatted events. It is NOT safe
// for concurrent use; the agent loop calls it from a single goroutine.
type sseWriter struct {
	mu      sync.Mutex
	c       *gin.Context
	flusher http.Flusher
	closed  bool
}

// newSSEWriter installs SSE-friendly headers and returns a writer. Headers
// MUST be set before any body is sent. Returns false if the underlying
// ResponseWriter does not support flushing (which shouldn't happen with Gin's
// default writer but is checked defensively).
func newSSEWriter(c *gin.Context) (*sseWriter, bool) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, false
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher.Flush()
	return &sseWriter{c: c, flusher: flusher}, true
}

// Event writes a single SSE event. Payload is JSON-marshalled. Errors here
// almost always mean the client disconnected; subsequent Event calls become
// no-ops once an error is seen.
func (w *sseWriter) Event(name string, payload any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"error":"marshal failed"}`)
	}
	if _, err := fmt.Fprintf(w.c.Writer, "event: %s\ndata: %s\n\n", name, data); err != nil {
		w.closed = true
		return
	}
	w.flusher.Flush()
}
