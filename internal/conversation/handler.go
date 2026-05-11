package conversation

import (
	"errors"
	"net/http"
	"strconv"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

// Handler exposes CRUD endpoints for AI conversations.
type Handler struct {
	web.BaseHandler
	service Service
}

// NewHandler constructs a conversation HTTP handler.
func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

type createConversationRequest struct {
	Title     string `json:"title"`
	ModelName string `json:"model_name"`
}

type renameConversationRequest struct {
	Title string `json:"title" binding:"required"`
}

type conversationDetailResponse struct {
	Conversation *Conversation `json:"conversation"`
	Messages     []*Message    `json:"messages"`
}

type conversationListResponse struct {
	Items   []*Conversation `json:"items"`
	Total   int64           `json:"total"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
}

// Create godoc
// @Summary      创建 AI 对话会话
// @Tags         ai-conversation
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      createConversationRequest  true  "会话标题与模型名（可选）"
// @Success      200   {object}  web.Response{data=Conversation}
// @Router       /api/v1/conversations [post]
func (h *Handler) Create(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	var req createConversationRequest
	_ = c.ShouldBindJSON(&req)

	conv, err := h.service.Create(c.Request.Context(), userID, req.Title, req.ModelName)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(conv))
}

// List godoc
// @Summary      列出当前用户的会话
// @Tags         ai-conversation
// @Produce      json
// @Security     BearerAuth
// @Param        page      query     int  false  "页码"     default(1)
// @Param        page_size query     int  false  "每页数量" default(20)
// @Success      200       {object}  web.Response{data=conversationListResponse}
// @Router       /api/v1/conversations [get]
func (h *Handler) List(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	items, total, err := h.service.ListForUser(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(conversationListResponse{
		Items:   items,
		Total:   total,
		Page:    page,
		PerPage: pageSize,
	}))
}

// Get godoc
// @Summary      获取会话详情（含历史消息）
// @Tags         ai-conversation
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "会话ID"
// @Success      200  {object}  web.Response{data=conversationDetailResponse}
// @Router       /api/v1/conversations/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	convID, ok := parseIDParam(c, "id")
	if !ok {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}

	conv, err := h.service.GetForUser(c.Request.Context(), userID, convID)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	messages, err := h.service.Store().ListMessages(c.Request.Context(), convID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to load messages")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(conversationDetailResponse{
		Conversation: conv,
		Messages:     messages,
	}))
}

// Delete godoc
// @Summary      删除会话
// @Tags         ai-conversation
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "会话ID"
// @Success      200  {object}  web.Response
// @Router       /api/v1/conversations/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	convID, ok := parseIDParam(c, "id")
	if !ok {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}

	if err := h.service.DeleteForUser(c.Request.Context(), userID, convID); err != nil {
		h.respondServiceError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(map[string]bool{"deleted": true}))
}

// Rename godoc
// @Summary      重命名会话
// @Tags         ai-conversation
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                        true  "会话ID"
// @Param        body  body      renameConversationRequest  true  "新标题"
// @Success      200   {object}  web.Response
// @Router       /api/v1/conversations/{id}/title [put]
func (h *Handler) Rename(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	convID, ok := parseIDParam(c, "id")
	if !ok {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	var req renameConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.service.RenameForUser(c.Request.Context(), userID, convID, req.Title); err != nil {
		h.respondServiceError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(map[string]bool{"updated": true}))
}

func (h *Handler) respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
	case errors.Is(err, ErrInvalidArgument):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
	default:
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
	}
}

func userIDFromContext(c *gin.Context) (int64, bool) {
	raw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	id, ok := raw.(int64)
	return id, ok
}

func parseIDParam(c *gin.Context, name string) (int64, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// UserIDFromContext is the public helper for sibling packages (ai) so they can
// reuse the same extraction logic.
func UserIDFromContext(c *gin.Context) (int64, bool) {
	return userIDFromContext(c)
}

// ParseIDParam is the public helper for sibling packages.
func ParseIDParam(c *gin.Context, name string) (int64, bool) {
	return parseIDParam(c, name)
}
