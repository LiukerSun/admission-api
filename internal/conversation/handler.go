package conversation

import (
	"net/http"
	"strconv"
	"strings"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

// MaxMessageContentBytes caps the size of a single user message that can
// be inserted via the public /messages endpoint. 8 KiB is well above any
// reasonable chat input but small enough to keep DB rows and LLM context
// bounded.
const MaxMessageContentBytes = 8 * 1024

type Handler struct {
	web.BaseHandler
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// CreateConversation godoc
// @Summary      Create a conversation
// @Description  Creates a new AI conversation. If user_id is omitted, the conversation is anonymous.
// @Tags         conversation
// @Accept       json
// @Produce      json
// @Param        body body CreateConversationRequest true "Conversation creation request"
// @Success      200 {object} web.Response
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations [post]
func (h *Handler) CreateConversation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	var req CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	conv, err := h.service.CreateConversation(c.Request.Context(), req.Title, &userID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to create conversation")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(conv))
}

// GetConversation godoc
// @Summary      Get conversation with messages
// @Description  Returns a conversation and its messages.
// @Tags         conversation
// @Produce      json
// @Param        id path int true "Conversation ID"
// @Success      200 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations/{id} [get]
func (h *Handler) GetConversation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	conv, err := h.service.GetConversation(c.Request.Context(), id)
	if err != nil {
		if err == ErrConversationNotFound {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get conversation")
		return
	}
	if !ownsConversation(conv, userID) {
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
		return
	}
	msgs, err := h.service.ListMessages(c.Request.Context(), id)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get messages")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(WithMessages{
		Conversation: conv,
		Messages:     msgs,
	}))
}

// ListConversations godoc
// @Summary      List conversations
// @Description  Lists active conversations, optionally filtered by user_id.
// @Tags         conversation
// @Produce      json
// @Param        user_id query int false "User ID"
// @Success      200 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations [get]
func (h *Handler) ListConversations(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	convs, err := h.service.ListConversations(c.Request.Context(), &userID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list conversations")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(convs))
}

// AddMessage godoc
// @Summary      Add a user message
// @Description  Appends a user message to an existing conversation.
//
//	The role is always forced to "user" on the server side; clients
//	cannot insert assistant, tool, or system messages through this
//	endpoint. Use POST /conversations/{id}/ai-chat to obtain assistant
//	replies.
//
// @Tags         conversation
// @Accept       json
// @Produce      json
// @Param        id path int true "Conversation ID"
// @Param        body body AddMessageRequest true "Message"
// @Success      200 {object} web.Response
// @Failure      400 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations/{id}/messages [post]
func (h *Handler) AddMessage(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	var req AddMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "content must not be empty")
		return
	}
	if len(content) > MaxMessageContentBytes {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "content exceeds maximum length")
		return
	}
	conv, ok := h.canAccessConversation(c, id, userID)
	if !ok {
		return
	}
	if conv.Status == "archived" {
		h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "conversation is archived")
		return
	}
	// Role is hardcoded to "user" and tool_calls / tool_results / widgets
	// are nil so this public endpoint cannot be abused to fabricate
	// assistant or tool history.
	msg, err := h.service.AddMessage(c.Request.Context(), id, "user", content, nil, nil, nil)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to add message")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(msg))
}

// RollbackRequest is the body for POST /conversations/:id/rollback.
//
// Inclusive defaults to true (delete the anchor row itself). Use false
// to keep the anchor — useful when "editing" the latest user message:
// the client first POSTs the edited content as a new user message, then
// rolls back inclusive from the OLD user message, leaving only the new
// edit and any messages that follow it.
type RollbackRequest struct {
	MessageID int64 `json:"message_id"`
	Inclusive *bool `json:"inclusive,omitempty"`
}

// RollbackResponse describes the result of a rollback.
type RollbackResponse struct {
	DeletedCount    int    `json:"deleted_count"`
	LatestMessageID *int64 `json:"latest_message_id"`
}

// Rollback godoc
// @Summary      Rollback conversation history
// @Description  Deletes messages at or after the specified message_id (inclusive by default). Used by the frontend "edit / regenerate" flows. Non-owners receive 404 to avoid leaking conversation existence.
// @Tags         conversation
// @Accept       json
// @Produce      json
// @Param        id path int true "Conversation ID"
// @Param        body body RollbackRequest true "Rollback target"
// @Success      200 {object} web.Response
// @Failure      400 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations/{id}/rollback [post]
func (h *Handler) Rollback(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	if req.MessageID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "message_id is required")
		return
	}
	inclusive := true
	if req.Inclusive != nil {
		inclusive = *req.Inclusive
	}
	conv, ok := h.canAccessConversation(c, id, userID)
	if !ok {
		return
	}
	if conv.Status == "archived" {
		h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "conversation is archived")
		return
	}
	deleted, latest, err := h.service.Rollback(c.Request.Context(), id, req.MessageID, inclusive)
	if err != nil {
		if err == ErrConversationNotFound {
			// The anchor row didn't belong to this conversation. Use 404
			// rather than 400 so the response is indistinguishable from
			// a non-owned conversation — same reason ownsConversation
			// returns 404.
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "message not found in conversation")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to rollback messages")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(RollbackResponse{
		DeletedCount:    deleted,
		LatestMessageID: latest,
	}))
}

// DeleteConversation godoc
// @Summary      Delete conversation
// @Description  Soft-deletes a conversation by marking it as deleted.
// @Tags         conversation
// @Produce      json
// @Param        id path int true "Conversation ID"
// @Success      200 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations/{id} [delete]
func (h *Handler) DeleteConversation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	if _, ok := h.canAccessConversation(c, id, userID); !ok {
		return
	}
	if err := h.service.DeleteConversation(c.Request.Context(), id); err != nil {
		if err == ErrConversationNotFound {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to delete conversation")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(nil))
}

// ArchiveConversation godoc
// @Summary      Archive conversation
// @Description  Archives a conversation.
// @Tags         conversation
// @Produce      json
// @Param        id path int true "Conversation ID"
// @Success      200 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations/{id}/archive [post]
func (h *Handler) ArchiveConversation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	if _, ok := h.canAccessConversation(c, id, userID); !ok {
		return
	}
	if err := h.service.ArchiveConversation(c.Request.Context(), id); err != nil {
		if err == ErrConversationNotFound {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to archive conversation")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(nil))
}

type WithMessages struct {
	Conversation *Conversation `json:"conversation"`
	Messages     []*Message    `json:"messages"`
}

func (h *Handler) canAccessConversation(c *gin.Context, id, userID int64) (*Conversation, bool) {
	conv, err := h.service.GetConversation(c.Request.Context(), id)
	if err != nil {
		if err == ErrConversationNotFound {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
			return nil, false
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get conversation")
		return nil, false
	}
	if !ownsConversation(conv, userID) {
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
		return nil, false
	}
	return conv, true
}

func ownsConversation(conv *Conversation, userID int64) bool {
	return conv != nil && conv.UserID != nil && *conv.UserID == userID
}
