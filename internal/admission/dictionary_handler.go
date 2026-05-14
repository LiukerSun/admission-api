package admission

import (
	"net/http"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type DictionaryHandler struct {
	web.BaseHandler
	service DictionaryService
}

func NewDictionaryHandler(service DictionaryService) *DictionaryHandler {
	return &DictionaryHandler{service: service}
}

// ListDictionaries godoc
// @Summary      List admission dictionaries
// @Description  Returns code-name values used by admission filters and imports.
// @Tags         admission
// @Produce      json
// @Success      200 {object} web.Response{data=DictionaryResponse}
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/dictionaries [get]
func (h *DictionaryHandler) ListDictionaries(c *gin.Context) {
	resp, err := h.service.ListDictionaries(c.Request.Context())
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list admission dictionaries")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}
