package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"admission-api/internal/conversation"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

// Handler exposes AI chat endpoints (SSE streaming + rollback / regenerate /
// suggestions). It depends on the conversation service for persistence and on
// an Agent for LLM orchestration.
type Handler struct {
	web.BaseHandler
	convs        conversation.Service
	agent        *Agent
	llm          LLMProxy
	rdb          *redis.Client
	defaultModel string
	systemPrompt string
}

// HandlerConfig groups the runtime knobs that aren't dependencies.
type HandlerConfig struct {
	DefaultModel string
	SystemPrompt string
}

// NewHandler constructs an AI handler.
func NewHandler(convs conversation.Service, agent *Agent, llm LLMProxy, rdb *redis.Client, cfg HandlerConfig) *Handler {
	prompt := cfg.SystemPrompt
	if prompt == "" {
		prompt = defaultSystemPrompt
	}
	return &Handler{
		convs:        convs,
		agent:        agent,
		llm:          llm,
		rdb:          rdb,
		defaultModel: cfg.DefaultModel,
		systemPrompt: prompt,
	}
}

const defaultSystemPrompt = `你是高考志愿填报智能助手，专为中国考生服务。
- 用专业、严谨、有同理心的语气回答。
- 涉及录取概率、薪资、就业率等数据时，优先调用工具获取，不要凭空编造数字。
- 只有在数据明显适合可视化（多年份趋势、跨校对比等）时才调用 render_chart。
- 推荐某所具体高校或专业时，可使用 render_card 输出结构化卡片。
- 工具返回错误时，向用户解释失败原因，不要硬编造结果。`

type chatRequest struct {
	Content string `json:"content" binding:"required"`
}

// Chat godoc
// @Summary      与 AI 对话（SSE 流式）
// @Tags         ai-conversation
// @Accept       json
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        id    path  int                true  "会话ID"
// @Param        body  body  chatRequest        true  "用户消息"
// @Router       /api/v1/conversations/{id}/ai-chat [post]
func (h *Handler) Chat(c *gin.Context) {
	userID, ok := conversation.UserIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	convID, ok := conversation.ParseIDParam(c, "id")
	if !ok {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	conv, err := h.convs.GetForUser(c.Request.Context(), userID, convID)
	if err != nil {
		h.respondConvError(c, err)
		return
	}

	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := conversation.ValidateMessageContent(req.Content); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	// Persist the new user message BEFORE opening the stream so editing /
	// rollback can target it. createdAt is server-controlled.
	if _, err := h.convs.Store().CreateMessage(c.Request.Context(), &conversation.CreateMessageInput{
		ConversationID: convID,
		Role:           conversation.RoleUser,
		Content:        req.Content,
	}); err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to save user message")
		return
	}
	_ = h.convs.Store().TouchConversation(c.Request.Context(), convID)

	h.runAgentOnHistory(c, conv)
}

// Regenerate replaces (or repeats) the last turn and re-streams the assistant
// response. Body is empty; behaviour depends on the trailing message role.
func (h *Handler) Regenerate(c *gin.Context) {
	userID, ok := conversation.UserIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	convID, ok := conversation.ParseIDParam(c, "id")
	if !ok {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	conv, err := h.convs.GetForUser(c.Request.Context(), userID, convID)
	if err != nil {
		h.respondConvError(c, err)
		return
	}

	last, err := h.convs.Store().GetLastMessage(c.Request.Context(), convID)
	if err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "no message to regenerate")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "load last message")
		return
	}
	if last.Role == conversation.RoleAssistant {
		if _, _, err := h.convs.Store().DeleteMessagesFrom(c.Request.Context(), convID, last.ID, true); err != nil {
			h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "rollback failed")
			return
		}
	}
	h.runAgentOnHistory(c, conv)
}

type rollbackRequest struct {
	MessageID int64 `json:"message_id" binding:"required"`
	Inclusive *bool `json:"inclusive"`
}

type rollbackResponse struct {
	DeletedCount    int64 `json:"deleted_count"`
	LatestMessageID int64 `json:"latest_message_id"`
}

// Rollback truncates the conversation at the given message. inclusive defaults
// to true so callers can express "edit the user message at this ID" with a
// single call.
func (h *Handler) Rollback(c *gin.Context) {
	userID, ok := conversation.UserIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	convID, ok := conversation.ParseIDParam(c, "id")
	if !ok {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	if _, err := h.convs.GetForUser(c.Request.Context(), userID, convID); err != nil {
		h.respondConvError(c, err)
		return
	}

	var req rollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	inclusive := true
	if req.Inclusive != nil {
		inclusive = *req.Inclusive
	}

	// Verify the pivot belongs to this conversation; collapses cross-conv
	// access into 404 like the rest of the API.
	msg, err := h.convs.Store().GetMessage(c.Request.Context(), req.MessageID)
	if err != nil || msg.ConversationID != convID {
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "message not found")
		return
	}

	deleted, latest, err := h.convs.Store().DeleteMessagesFrom(c.Request.Context(), convID, req.MessageID, inclusive)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "rollback failed")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(rollbackResponse{
		DeletedCount:    deleted,
		LatestMessageID: latest,
	}))
}

type suggestionsResponse struct {
	Suggestions []string `json:"suggestions"`
}

// Suggestions returns 2-4 follow-up question suggestions based on recent
// conversation history. Always responds 200 with at least an empty array even
// if the LLM call fails — the frontend treats suggestions as best-effort.
func (h *Handler) Suggestions(c *gin.Context) {
	userID, ok := conversation.UserIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	convID, ok := conversation.ParseIDParam(c, "id")
	if !ok {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}
	if _, err := h.convs.GetForUser(c.Request.Context(), userID, convID); err != nil {
		h.respondConvError(c, err)
		return
	}

	ctx := c.Request.Context()
	recent, err := h.convs.Store().ListRecentMessages(ctx, convID, 10)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "load history")
		return
	}
	if len(recent) == 0 {
		h.RespondJSON(c, http.StatusOK, web.SuccessResponse(suggestionsResponse{Suggestions: []string{}}))
		return
	}
	lastID := recent[len(recent)-1].ID

	cacheKey := fmt.Sprintf("conv_suggest:%d:%d", convID, lastID)
	if h.rdb != nil {
		if cached, err := h.rdb.Get(ctx, cacheKey); err == nil && cached != "" {
			var arr []string
			if json.Unmarshal([]byte(cached), &arr) == nil {
				h.RespondJSON(c, http.StatusOK, web.SuccessResponse(suggestionsResponse{Suggestions: arr}))
				return
			}
		}
	}

	suggestions := h.generateSuggestions(ctx, recent)

	if h.rdb != nil {
		if payload, err := json.Marshal(suggestions); err == nil {
			_ = h.rdb.Set(ctx, cacheKey, string(payload), time.Hour)
		}
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(suggestionsResponse{Suggestions: suggestions}))
}

func (h *Handler) generateSuggestions(ctx context.Context, recent []*conversation.Message) []string {
	prompt := []ChatMessage{
		{Role: "system", Content: `请根据下面的对话生成 2-4 条简短的中文追问问题，帮助考生深入了解志愿填报。仅输出 JSON 数组，例如 ["问题一","问题二"]。不要任何解释、Markdown 或额外文字。`},
	}
	for _, m := range recent {
		role := m.Role
		if role != conversation.RoleUser && role != conversation.RoleAssistant {
			continue
		}
		content := m.Content
		if len(content) > 800 {
			content = content[:800]
		}
		prompt = append(prompt, ChatMessage{Role: role, Content: content})
	}

	output, err := h.llm.ChatCompletion(ctx, ChatRequest{
		Model:       h.defaultModel,
		Messages:    prompt,
		Temperature: 0.7,
	})
	if err != nil {
		slog.Warn("suggestions llm failed", "error", err)
		return []string{}
	}
	parsed := parseSuggestionsJSON(output)
	return parsed
}

func parseSuggestionsJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	// Try direct unmarshal.
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return clampSuggestions(arr)
	}
	// Try to find the first JSON array in the text.
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start >= 0 && end > start {
		candidate := raw[start : end+1]
		if err := json.Unmarshal([]byte(candidate), &arr); err == nil {
			return clampSuggestions(arr)
		}
	}
	return []string{}
}

func clampSuggestions(arr []string) []string {
	out := make([]string, 0, len(arr))
	for _, s := range arr {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len([]rune(s)) > 120 {
			r := []rune(s)
			s = string(r[:120])
		}
		out = append(out, s)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

// runAgentOnHistory is the shared streaming pipeline used by both Chat and
// Regenerate. It expects all relevant user messages to already be persisted.
func (h *Handler) runAgentOnHistory(c *gin.Context, conv *conversation.Conversation) {
	writer, ok := newSSEWriter(c)
	if !ok {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "streaming not supported")
		return
	}
	ctx := c.Request.Context()
	convID := conv.ID

	stored, err := h.convs.Store().ListMessages(ctx, convID)
	if err != nil {
		writer.Event(EventError, gin.H{"code": web.ErrCodeInternal, "message": "load history"})
		return
	}

	history := []ChatMessage{{Role: "system", Content: h.systemPrompt}}
	history = append(history, toChatMessages(stored)...)

	callbacks := AgentCallbacks{
		OnTextDelta: func(delta string) {
			writer.Event(EventTextDelta, gin.H{"delta": delta})
		},
		OnToolCallStart: func(callID, name string, args json.RawMessage) {
			writer.Event(EventToolCallStart, gin.H{
				"call_id":   callID,
				"tool_name": name,
				"arguments": json.RawMessage(args),
			})
		},
		OnToolCallEnd: func(callID string, success bool, errMsg string) {
			writer.Event(EventToolCallEnd, gin.H{
				"call_id": callID,
				"success": success,
				"error":   errMsg,
			})
		},
		OnWidget: func(w Widget) {
			writer.Event(EventWidget, gin.H{
				"id":      w.ID,
				"kind":    w.Kind,
				"payload": w.Payload,
			})
		},
		OnWarning: func(message string) {
			writer.Event(EventWarning, gin.H{"message": message})
		},
	}

	result, runErr := h.agent.RunStream(ctx, history, callbacks)
	if runErr != nil {
		writer.Event(EventError, gin.H{
			"code":    web.ErrCodeInternal,
			"message": runErr.Error(),
		})
		// Persist what we got so the user doesn't lose partial output.
	}

	msg, persistErr := h.persistAssistantTurn(ctx, convID, result)
	if persistErr != nil {
		writer.Event(EventError, gin.H{
			"code":    web.ErrCodeInternal,
			"message": "persist assistant message",
		})
		return
	}
	_ = h.convs.Store().TouchConversation(ctx, convID)

	writer.Event(EventDone, gin.H{
		"message_id": msg.ID,
	})
}

func (h *Handler) persistAssistantTurn(ctx context.Context, convID int64, result *AgentResult) (*conversation.Message, error) {
	toolCallsJSON, _ := json.Marshal(result.ToolCalls)
	toolResultsJSON, _ := json.Marshal(result.ToolResults)
	widgetsJSON, _ := json.Marshal(result.Widgets)
	if len(result.ToolCalls) == 0 {
		toolCallsJSON = []byte("[]")
	}
	if len(result.ToolResults) == 0 {
		toolResultsJSON = []byte("[]")
	}
	if len(result.Widgets) == 0 {
		widgetsJSON = []byte("[]")
	}
	return h.convs.Store().CreateMessage(ctx, &conversation.CreateMessageInput{
		ConversationID: convID,
		Role:           conversation.RoleAssistant,
		Content:        result.FinalContent,
		ToolCalls:      toolCallsJSON,
		ToolResults:    toolResultsJSON,
		Widgets:        widgetsJSON,
	})
}

func toChatMessages(msgs []*conversation.Message) []ChatMessage {
	out := make([]ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case conversation.RoleUser, conversation.RoleSystem:
			out = append(out, ChatMessage{Role: m.Role, Content: m.Content})
		case conversation.RoleAssistant:
			cm := ChatMessage{Role: m.Role, Content: m.Content}
			if len(m.ToolCalls) > 0 {
				var calls []ToolCallSpec
				if err := json.Unmarshal(m.ToolCalls, &calls); err == nil {
					cm.ToolCalls = calls
				}
			}
			out = append(out, cm)
			// Re-attach tool results so the LLM sees the prior turn correctly.
			if len(m.ToolResults) > 0 {
				var results []PersistedToolResult
				if err := json.Unmarshal(m.ToolResults, &results); err == nil {
					for _, r := range results {
						out = append(out, ChatMessage{
							Role:       "tool",
							ToolCallID: r.CallID,
							Content:    r.Content,
						})
					}
				}
			}
		}
	}
	return out
}

func (h *Handler) respondConvError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, conversation.ErrNotFound):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
	case errors.Is(err, conversation.ErrInvalidArgument):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
	default:
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
	}
}

// Ensure imports referenced indirectly stay used during incremental edits.
var _ = middleware.ContextUserIDKey
