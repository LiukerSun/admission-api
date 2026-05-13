package volunteerplan

import (
	"errors"
	"net/http"
	"strconv"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	web.BaseHandler
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

type AdoptRequest struct {
	DraftID int64  `json:"draft_id"`
	Title   string `json:"title,omitempty"`
}

func (h *Handler) Adopt(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	var req AdoptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	if req.DraftID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "draft_id is required")
		return
	}
	plan, err := h.service.AdoptDraft(c.Request.Context(), userID, req.DraftID, req.Title)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(plan))
}

func (h *Handler) ListPlans(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	plans, err := h.service.ListPlans(c.Request.Context(), userID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list volunteer plans")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(plans))
}

func (h *Handler) GetPlan(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid plan id")
		return
	}
	plan, err := h.service.GetPlan(c.Request.Context(), userID, id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(plan))
}

func (h *Handler) GetDraft(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(c.Param("draft_id"), 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid draft id")
		return
	}
	draft, err := h.service.GetDraft(c.Request.Context(), userID, id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(draft))
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDraftNotFound), errors.Is(err, ErrPlanNotFound):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "not found")
	case errors.Is(err, ErrDraftNotReady):
		h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "draft is not ready")
	default:
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal error")
	}
}

