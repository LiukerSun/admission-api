package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"admission-api/internal/conversation"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

const aiStreamTimeout = 10 * time.Minute

// Input-size limits on the AI chat endpoints. These exist to bound LLM
// token cost, prevent oversized payloads from blowing the context
// window, and stop a single caller from amplifying load by sending huge
// prior-message histories.
//
// The numbers are intentionally generous for normal use:
//   - MaxAIChatMessages: at most this many messages per /ai/chat request
//   - MaxAIChatMessageBytes: per-message cap (matches the conversation
//     endpoint's MaxMessageContentBytes so a message that fits in one
//     place fits in the other)
//   - MaxAIChatTotalBytes: cumulative content-byte cap across the whole
//     /ai/chat array, so an attacker can't get around the per-message
//     cap by sending many medium-sized messages.
const (
	MaxAIChatMessages     = 50
	MaxAIChatMessageBytes = 8 * 1024
	MaxAIChatTotalBytes   = 32 * 1024
)

// errEmptyChatMessages, errTooManyChatMessages, etc. are sentinel errors
// returned by validateChatMessages. They let the caller respond with a
// stable, user-facing message without leaking internal limits beyond the
// response body.
var (
	errEmptyChatMessages         = errors.New("messages must not be empty")
	errTooManyChatMessages       = errors.New("too many messages in a single request")
	errChatMessageTooLarge       = errors.New("a single message exceeds maximum length")
	errChatMessagesTotalTooLarge = errors.New("messages total length exceeds maximum")
)

// validateChatMessages enforces the per-request input caps on the
// /ai/chat endpoint. It returns a sentinel error describing which limit
// was hit so the caller can map it to a 400 response.
func validateChatMessages(msgs []Message) error {
	if len(msgs) == 0 {
		return errEmptyChatMessages
	}
	if len(msgs) > MaxAIChatMessages {
		return errTooManyChatMessages
	}
	total := 0
	for _, m := range msgs {
		size := len(m.Content)
		if size > MaxAIChatMessageBytes {
			return errChatMessageTooLarge
		}
		total += size
		if total > MaxAIChatTotalBytes {
			return errChatMessagesTotalTooLarge
		}
	}
	return nil
}

// extendWriteDeadline sets a longer write deadline for SSE streaming.
func extendWriteDeadline(c *gin.Context, d time.Duration) {
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(d))
}

// ChatRequest is the request body for AI chat.
type ChatRequest struct {
	Messages []Message `json:"messages"`
}

// ConversationChatRequest is the request body for conversation-scoped AI chat.
type ConversationChatRequest struct {
	Message string `json:"message"`
}

// SSEEvent is a server-sent event.
type SSEEvent struct {
	Type    string `json:"type"`
	Step    string `json:"step,omitempty"`
	Content string `json:"content,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// Handler handles AI chat endpoints.
type Handler struct {
	web.BaseHandler
	agent               *Agent
	conversationService conversation.Service
}

// NewHandler creates a new AI handler.
func NewHandler(agent *Agent, conversationService conversation.Service) *Handler {
	return &Handler{agent: agent, conversationService: conversationService}
}

// Chat godoc
// @Summary      AI chat with SSE streaming
// @Description  Streams AI responses via SSE. Send messages array; receive step_start/step_finish/text_delta/done events.
// @Tags         ai
// @Accept       json
// @Produce      text/event-stream
// @Param        body body ChatRequest true "Chat messages"
// @Success      200 {string} string "SSE stream"
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/ai/chat [post]
func (h *Handler) Chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := validateChatMessages(req.Messages); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)
	extendWriteDeadline(c, aiStreamTimeout)

	flush := func() {
		if f, ok := c.Writer.(http.Flusher); ok {
			f.Flush()
		}
	}

	writeEvent := func(event SSEEvent) {
		data, _ := json.Marshal(event)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flush()
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), aiStreamTimeout)
	defer cancel()

	writeEvent(SSEEvent{Type: "step_start", Step: "thinking"})

	result, err := h.agent.Run(ctx, req.Messages)
	if err != nil {
		writeEvent(SSEEvent{Type: "error", Content: err.Error()})
		return
	}

	writeEvent(SSEEvent{Type: "step_finish", Step: "thinking"})

	// Stream text in chunks to simulate typing
	runes := []rune(result.Text)
	for i := 0; i < len(runes); i += 10 {
		end := i + 10
		if end > len(runes) {
			end = len(runes)
		}
		writeEvent(SSEEvent{Type: "text_delta", Content: string(runes[i:end])})
	}

	writeEvent(SSEEvent{Type: "done", Data: result})
}

// ChatWithConversation godoc
// @Summary      AI chat within a conversation
// @Description  Sends a message in a conversation context. Persists messages and streams AI response via SSE.
// @Tags         ai
// @Accept       json
// @Produce      text/event-stream
// @Param        id path int true "Conversation ID"
// @Param        body body ConversationChatRequest true "User message"
// @Success      200 {string} string "SSE stream"
// @Failure      400 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations/{id}/ai-chat [post]
func (h *Handler) ChatWithConversation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	convID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid conversation id")
		return
	}

	var req ConversationChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	// Reject empty / whitespace-only messages so they don't pollute the
	// conversation history with no-op rows the LLM has to read past, and
	// cap a single message at MaxAIChatMessageBytes to bound DB storage
	// and per-call LLM cost.
	trimmed := strings.TrimSpace(req.Message)
	if trimmed == "" {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "message must not be empty")
		return
	}
	if len(req.Message) > MaxAIChatMessageBytes {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "message exceeds maximum length")
		return
	}
	req.Message = trimmed

	// Verify conversation exists
	conv, err := h.conversationService.GetConversation(c.Request.Context(), convID)
	if err != nil {
		if err == conversation.ErrConversationNotFound {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get conversation")
		return
	}
	if conv.UserID == nil || *conv.UserID != userID {
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
		return
	}

	// Save user message. We must NOT swallow this error: if the user's
	// turn never lands in the database, the LLM will run on a history
	// that's missing the latest question, fabricate a reply, and that
	// reply will then be persisted as if it answered nothing — corrupting
	// every future replay of this conversation.
	if _, err := h.conversationService.AddMessage(c.Request.Context(), convID, "user", req.Message, nil, nil); err != nil {
		slog.Error("failed to persist user message before AI run", "error", err, "conversationID", convID)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to save message")
		return
	}

	// Load conversation history
	msgs, err := h.conversationService.ListMessages(c.Request.Context(), convID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to load messages")
		return
	}

	aiMessages := conversationMessagesToAIMessages(msgs)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)
	extendWriteDeadline(c, aiStreamTimeout)

	flush := func() {
		if f, ok := c.Writer.(http.Flusher); ok {
			f.Flush()
		}
	}

	writeEvent := func(event SSEEvent) {
		data, _ := json.Marshal(event)
		n, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		slog.Info("sse write", "type", event.Type, "bytes", n, "err", err)
		flush()
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), aiStreamTimeout)
	defer cancel()

	writeEvent(SSEEvent{Type: "step_start", Step: "thinking"})

	result, err := h.agent.Run(ctx, aiMessages)
	if err != nil {
		slog.Error("agent run failed", "error", err)
		writeEvent(SSEEvent{Type: "error", Content: err.Error()})
		return
	}

	writeEvent(SSEEvent{Type: "step_finish", Step: "thinking"})

	// Stream text
	runes := []rune(result.Text)
	slog.Info("streaming text", "charCount", len(runes))
	for i := 0; i < len(runes); i += 10 {
		end := i + 10
		if end > len(runes) {
			end = len(runes)
		}
		writeEvent(SSEEvent{Type: "text_delta", Content: string(runes[i:end])})
	}

	writeEvent(SSEEvent{Type: "done", Data: result})
	slog.Info("sse stream complete")

	// Save assistant message with tool calls. The SSE stream has already
	// been flushed to the client by this point, so a save failure can't
	// be turned into a non-200 status — but we MUST surface it in logs
	// (and on the wire as a non-fatal warn event) instead of silently
	// dropping it. Otherwise the next request to this conversation will
	// replay history without the assistant's reply, the LLM will repeat
	// itself, and ops has no signal that anything went wrong.
	var toolCallsJSON []byte
	if len(result.ToolCalls) > 0 {
		toolCallsJSON, _ = json.Marshal(result.ToolCalls)
	}
	var toolResultsJSON []byte
	if len(result.ToolResults) > 0 {
		toolResultsJSON, _ = json.Marshal(result.ToolResults)
	}
	if _, err := h.conversationService.AddMessage(c.Request.Context(), convID, "assistant", result.Text, toolCallsJSON, toolResultsJSON); err != nil {
		slog.Error("failed to persist assistant message after AI run", "error", err, "conversationID", convID)
		writeEvent(SSEEvent{Type: "warning", Content: "assistant message could not be saved; future replies in this conversation may not see it"})
	}
}

func conversationMessagesToAIMessages(messages []*conversation.Message) []Message {
	aiMessages := make([]Message, 0, len(messages))
	for _, m := range messages {
		msg := Message{Role: m.Role, Content: m.Content}
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			aiMessages = append(aiMessages, msg)
			continue
		}

		var toolCalls []ToolCall
		if err := json.Unmarshal(m.ToolCalls, &toolCalls); err != nil || len(toolCalls) == 0 {
			aiMessages = append(aiMessages, msg)
			continue
		}

		var toolResults []ToolResult
		if err := json.Unmarshal(m.ToolResults, &toolResults); err != nil || len(toolResults) == 0 {
			aiMessages = append(aiMessages, msg)
			continue
		}

		msg.ToolCalls = toolCalls
		aiMessages = append(aiMessages, msg)
		for _, result := range toolResults {
			aiMessages = append(aiMessages, Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: result.ToolCallID,
			})
		}
	}
	return aiMessages
}
