package web

import (
	"encoding/json"
	"net/http"
)

// BaseHandler provides common HTTP helper methods.
type BaseHandler struct{}

// RespondJSON writes a JSON response with the given status code.
func (h *BaseHandler) RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// RespondError writes a standardized error response.
func (h *BaseHandler) RespondError(w http.ResponseWriter, status, code int, message string) {
	h.RespondJSON(w, status, ErrorResponse(code, message))
}
