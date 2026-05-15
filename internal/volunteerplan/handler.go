package volunteerplan

import (
	"errors"
	"net/http"
	"strconv"

	"admission-api/internal/conversation"
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
		// GetPlan is the public plan-by-id lookup: a missing plan IS a
		// legitimate 404 here, unlike the adopt path where the same
		// sentinel signals an internal inconsistency. Map it inline so
		// writeError can keep its adopt-path interpretation.
		if errors.Is(err, ErrPlanNotFound) {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "plan not found")
			return
		}
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

func (h *Handler) ListDraftsByConversation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}

	drafts, err := h.service.ListDraftsByConversation(c.Request.Context(), userID, conversationID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(drafts))
}

// writeError maps service-layer sentinels to HTTP responses.
//
// Status-code rationale:
//   - ErrDraftNotFound / conversation-not-found → 404. The caller can't
//     tell whether the row truly doesn't exist or just doesn't belong to
//     them; collapsing both into 404 avoids leaking ownership.
//   - ErrDraftNotReady → 409. The resource exists but the requested
//     transition (adopt) requires it to be in 'ready' state.
//   - ErrDraftAlreadyAdopted → 409. Idempotent double-click feedback.
//   - ErrDraftCorrupted → 422. The data we have is unprocessable —
//     plan_json is empty or not valid JSON; the user must regenerate.
//   - ErrPlanNotFound → 500. Surfaces only inside the adopt path
//     (CreateFromDraft.GetByDraftID after ON CONFLICT), where it means
//     the plan we just inserted vanished or the index was wrong — a
//     real internal inconsistency, not a missing resource the user
//     requested. Plan-by-id GET still hits 404 explicitly.
func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDraftNotFound):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "draft not found")
	case errors.Is(err, conversation.ErrConversationNotFound):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
	case errors.Is(err, ErrDraftNotReady):
		h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "draft is not in ready state")
	case errors.Is(err, ErrDraftAlreadyAdopted):
		h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "draft has already been adopted")
	case errors.Is(err, ErrDraftCorrupted):
		h.RespondError(c, http.StatusUnprocessableEntity, web.ErrCodeBadRequest, "draft plan data is corrupted, please regenerate")
	case errors.Is(err, ErrPlanNotFound):
		// Only reachable from the adopt path's idempotency fallback — a
		// missing plan after a successful INSERT ON CONFLICT means the
		// DB is in an inconsistent state, not a user-visible 404.
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "plan lookup failed after adopt")
	default:
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal error")
	}
}
