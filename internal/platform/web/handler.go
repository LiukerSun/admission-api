package web

import "github.com/gin-gonic/gin"

// BaseHandler provides common HTTP helper methods.
type BaseHandler struct{}

// RespondJSON writes a JSON response with the given status code.
func (h *BaseHandler) RespondJSON(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

// RespondError writes a standardized error response.
func (h *BaseHandler) RespondError(c *gin.Context, status, code int, message string) {
	h.RespondJSON(c, status, ErrorResponse(code, message))
}
