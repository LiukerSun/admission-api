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
//
// Field usage by event type:
//   - "text_delta": Content holds the token slice
//   - "tool_call_start" / "tool_call_end": Data holds a structured payload
//   - "widget": Data holds the Widget value
//   - "done": Data holds the final AgentResult
//   - "error" / "warning": Content holds a human-readable message
//
// The legacy "step_start" / "step_finish" / Step fields are retained
// only to avoid a breaking shape change on the wire; new code paths do
// not emit them.
type SSEEvent struct {
	Type          string `json:"type"`
	Step          string `json:"step,omitempty"`
	Content       string `json:"content,omitempty"`
	Data          any    `json:"data,omitempty"`
	ID            string `json:"id,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Payload       any    `json:"payload,omitempty"`
	CallID        string `json:"call_id,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	Success       *bool  `json:"success,omitempty"`
	Error         string `json:"error,omitempty"`
	ResultContent string `json:"result_content,omitempty"`
}

// Handler handles AI chat endpoints.
type Handler struct {
	web.BaseHandler
	agent               *Agent
	conversationService conversation.Service
	turns               *TurnManager
}

// NewHandler creates a new AI handler.
//
// turns 可为 nil（兼容旧测试）；为 nil 时 ChatWithConversation 退化到
// 同步模式（客户端断 → agent 死），相当于禁用"切对话后台续跑"特性。
func NewHandler(agent *Agent, conversationService conversation.Service, turns *TurnManager) *Handler {
	if turns == nil {
		turns = NewTurnManager()
	}
	return &Handler{agent: agent, conversationService: conversationService, turns: turns}
}

// streamWriter encapsulates the SSE write loop for a single request so
// Chat, ChatWithConversation, and Regenerate share one implementation.
type streamWriter struct {
	c     *gin.Context
	flush func()
}

func newStreamWriter(c *gin.Context) *streamWriter {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)
	extendWriteDeadline(c, aiStreamTimeout)
	flush := func() {}
	if f, ok := c.Writer.(http.Flusher); ok {
		flush = f.Flush
	}
	return &streamWriter{c: c, flush: flush}
}

func (w *streamWriter) write(event *SSEEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		// Marshal failure on an SSE event is exotic — most commonly an
		// unsupported type in event.Data. Log it but keep the stream
		// alive; the next event may succeed.
		slog.Error("marshal sse event", "type", event.Type, "error", err)
		return
	}
	if _, err := fmt.Fprintf(w.c.Writer, "data: %s\n\n", data); err != nil {
		slog.Warn("sse write failed", "type", event.Type, "error", err)
		return
	}
	w.flush()
	// Refresh the per-write deadline every time we ship a chunk; long
	// LLM calls can otherwise hit the connection's hard write timeout.
	extendWriteDeadline(w.c, aiStreamTimeout)
}

// runAgentOnHistory is the single streaming pipeline used by all three
// AI entrypoints (Chat, ChatWithConversation, Regenerate). It owns the
// translation from AgentCallbacks to SSE events. The agent itself never
// knows about HTTP; this keeps tests easy and prevents three drifting
// implementations of the streaming protocol.
//
// On success it returns the final AgentResult so the caller can persist
// the assistant message (when in conversation mode). On failure it has
// already written an "error" event to sw — the caller need not re-emit.
func (h *Handler) runAgentOnHistory(ctx context.Context, sw *streamWriter, history []Message, opts RunOptions) (*AgentResult, error) {
	cb := AgentCallbacks{
		OnTextDelta: func(content string) {
			sw.write(&SSEEvent{Type: "text_delta", Content: content})
		},
		OnToolCallStart: func(callID, toolName string) {
			sw.write(&SSEEvent{
				Type:     "tool_call_start",
				CallID:   callID,
				ToolName: toolName,
				Data:     map[string]any{"call_id": callID, "tool_name": toolName},
			})
		},
		OnToolCallEnd: func(callID string, success bool, errMsg string, resultContent string) {
			payload := map[string]any{"call_id": callID, "success": success}
			if errMsg != "" {
				payload["error"] = errMsg
			}
			if resultContent != "" {
				payload["result_content"] = resultContent
			}
			ok := success
			sw.write(&SSEEvent{
				Type:          "tool_call_end",
				CallID:        callID,
				Success:       &ok,
				Error:         errMsg,
				ResultContent: resultContent,
				Data:          payload,
			})
		},
		OnWidget: func(widget Widget) {
			sw.write(&SSEEvent{Type: "widget", ID: widget.ID, Kind: widget.Kind, Payload: widget.Payload, Data: widget})
		},
	}

	result, err := h.agent.RunStreamWithOptions(ctx, history, cb, opts)
	if err != nil {
		slog.Error("agent run failed", "error", err)
		sw.write(&SSEEvent{Type: "error", Content: err.Error()})
		return nil, err
	}
	return result, nil
}

// agentCallbacksForTurn 构造把 agent 事件写进 Turn backlog 的回调。
// Turn.Append 内部会同步广播给所有当前订阅者。
func agentCallbacksForTurn(t *Turn) AgentCallbacks {
	return AgentCallbacks{
		OnTextDelta: func(content string) {
			t.Append(SSEEvent{Type: "text_delta", Content: content})
		},
		OnToolCallStart: func(callID, toolName string) {
			t.Append(SSEEvent{
				Type:     "tool_call_start",
				CallID:   callID,
				ToolName: toolName,
				Data:     map[string]any{"call_id": callID, "tool_name": toolName},
			})
		},
		OnToolCallEnd: func(callID string, success bool, errMsg string, resultContent string) {
			payload := map[string]any{"call_id": callID, "success": success}
			if errMsg != "" {
				payload["error"] = errMsg
			}
			if resultContent != "" {
				payload["result_content"] = resultContent
			}
			ok := success
			t.Append(SSEEvent{
				Type:          "tool_call_end",
				CallID:        callID,
				Success:       &ok,
				Error:         errMsg,
				ResultContent: resultContent,
				Data:          payload,
			})
		},
		OnWidget: func(widget Widget) {
			t.Append(SSEEvent{Type: "widget", ID: widget.ID, Kind: widget.Kind, Payload: widget.Payload, Data: widget})
		},
	}
}

// runTurnBody 是 detached agent 主体——在 TurnManager.Start 的 goroutine
// 里执行。它接管 history 加载、agent 执行、assistant 落盘、错误处理与
// 可选的 user-message 回滚。pendingUserMsgID 仅 ChatWithConversation 传，
// Regenerate 传 nil（没有"刚插入的 user 行"需要兜底回滚）。
//
// 所有 DB 写入都走 context.Background() 而不是 turn.Context()——agent
// run 超时（DeadlineExceeded）也得保证 assistant 文本能落盘，否则用户
// 切回来看不到任何输出。
func (h *Handler) runTurnBody(t *Turn, convID, userID int64, pendingUserMsgID *int64) {
	msgs, err := h.conversationService.ListMessages(t.Context(), convID)
	if err != nil {
		slog.Error("turn: load history failed", "error", err, "conversationID", convID)
		t.Append(SSEEvent{Type: "error", Content: "failed to load conversation history"})
		rollbackPendingUserMessage(h.conversationService, convID, pendingUserMsgID)
		return
	}
	aiMessages := conversationMessagesToAIMessages(msgs)

	cb := agentCallbacksForTurn(t)
	result, runErr := h.agent.RunStreamWithOptions(t.Context(), aiMessages, cb, RunOptions{
		ToolContext: ToolExecContext{UserID: userID, ConversationID: convID},
	})
	if runErr != nil {
		slog.Error("turn: agent run failed", "error", runErr, "conversationID", convID)
		t.Append(SSEEvent{Type: "error", Content: runErr.Error()})
		// Deadline / cancel 时保留 user msg：用户问题真实存在，切回来后
		// 可点"继续生成"重试。其它 agent 失败（LLM 5xx / 解析错）走 rollback：
		// 重新发问题成本低，让历史保持干净。
		if !errors.Is(runErr, context.DeadlineExceeded) && !errors.Is(runErr, context.Canceled) {
			rollbackPendingUserMessage(h.conversationService, convID, pendingUserMsgID)
		}
		return
	}

	var toolCallsJSON, toolResultsJSON, widgetsJSON []byte
	if len(result.ToolCalls) > 0 {
		toolCallsJSON, _ = json.Marshal(result.ToolCalls)
	}
	if len(result.ToolResults) > 0 {
		toolResultsJSON, _ = json.Marshal(result.ToolResults)
	}
	if len(result.Widgets) > 0 {
		widgetsJSON, _ = json.Marshal(result.Widgets)
	}
	if _, err := h.conversationService.AddMessage(context.Background(), convID, "assistant", result.Text, toolCallsJSON, toolResultsJSON, widgetsJSON); err != nil {
		slog.Error("turn: persist assistant failed", "error", err, "conversationID", convID)
		t.Append(SSEEvent{Type: "warning", Content: "assistant message could not be saved; future replies in this conversation may not see it"})
	}
	t.Append(SSEEvent{Type: "done", Data: result})
}

func rollbackPendingUserMessage(svc conversation.Service, convID int64, id *int64) {
	if id == nil {
		return
	}
	if _, _, err := svc.Rollback(context.Background(), convID, *id, true); err != nil {
		slog.Error("turn: rollback user message failed",
			"conversationID", convID,
			"messageID", *id,
			"error", err,
		)
	}
}

// streamTurnToClient 把一个 Turn 的事件流（backlog + 增量）灌到当前
// HTTP 连接。客户端在生成中断开（切对话）→ unsubscribe，agent 继续；
// 客户端重连调 StreamActiveTurn → 同样走这里，先收 backlog 回放再追平
// 实时。
func (h *Handler) streamTurnToClient(c *gin.Context, t *Turn) {
	sw := newStreamWriter(c)
	backlog, ch, unsub := t.Subscribe()
	defer unsub()

	for i := range backlog {
		sw.write(&backlog[i])
		if c.Request.Context().Err() != nil {
			return
		}
	}
	if ch == nil {
		// turn 已完成；backlog 全部回放完即可退出。
		return
	}
	for {
		select {
		case <-c.Request.Context().Done():
			// 客户端断；agent 继续在后台跑。
			return
		case ev, ok := <-ch:
			if !ok {
				// turn 结束（markFinished 关了 channel）。
				return
			}
			sw.write(&ev)
		}
	}
}

// Chat godoc
// @Summary      AI chat with SSE streaming
// @Description  Streams AI responses via SSE. Send messages array; receive text_delta / tool_call_start / tool_call_end / widget / done events.
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

	sw := newStreamWriter(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), aiStreamTimeout)
	defer cancel()

	result, err := h.runAgentOnHistory(ctx, sw, req.Messages, RunOptions{})
	if err != nil {
		return
	}
	sw.write(&SSEEvent{Type: "done", Data: result})
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

	if !h.verifyConversationOwnership(c, convID, userID) {
		return
	}

	// 拒绝"同一对话已有 active turn 时再开新一轮"——这种情况通常意味
	// 着用户上一轮还没跑完就再发一条。让前端走 StreamActiveTurn 续看
	// 旧 turn，而不是把新问题也插进去（容易混淆历史 + 双倍 LLM 成本）。
	// 注意是软检查：在 AddMessage + Start 之间可能有微小窗口让别的请求
	// 抢先，Start 内部会再做一次原子检查兜底。
	if existing := h.turns.Get(convID); existing != nil && !existing.isFinished() {
		h.RespondError(c, http.StatusConflict, web.ErrCodeBadRequest, "previous turn still running; reconnect via stream endpoint")
		return
	}

	// Save user message. We must NOT swallow this error: if the user's
	// turn never lands in the database, the LLM will run on a history
	// that's missing the latest question, fabricate a reply, and that
	// reply will then be persisted as if it answered nothing — corrupting
	// every future replay of this conversation.
	userMsg, err := h.conversationService.AddMessage(c.Request.Context(), convID, "user", req.Message, nil, nil, nil)
	if err != nil {
		slog.Error("failed to persist user message before AI run", "error", err, "conversationID", convID)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to save message")
		return
	}
	pendingUserMsgID := userMsg.ID

	// 启动后台 turn。run goroutine 拿到 detached ctx，客户端断开不再
	// 影响它；只有 turnRunDeadline 才能终止 agent。
	turn, startErr := h.turns.Start(convID, func(t *Turn) {
		h.runTurnBody(t, convID, userID, &pendingUserMsgID)
	})
	if startErr != nil {
		// 与上面的软检查同义；并发抢跑时这里兜底——回滚刚插入的 user
		// 消息保持历史干净，告诉客户端去 stream endpoint 续看现有 turn。
		_, _, _ = h.conversationService.Rollback(context.Background(), convID, pendingUserMsgID, true)
		h.RespondError(c, http.StatusConflict, web.ErrCodeBadRequest, "previous turn still running; reconnect via stream endpoint")
		return
	}

	h.streamTurnToClient(c, turn)
}

// StreamActiveTurn godoc
// @Summary      Resume the active AI turn (if any) over SSE
// @Description  Subscribes to the in-flight or recently finished turn for the conversation. Returns 204 when no turn is available. Used by the frontend to seamlessly resume streaming after the user switches conversations mid-generation.
// @Tags         ai
// @Produce      text/event-stream
// @Param        id path int true "Conversation ID"
// @Success      200 {string} string "SSE stream"
// @Success      204 {string} string "No active turn"
// @Failure      401 {object} web.Response
// @Failure      404 {object} web.Response
// @Router       /api/v1/conversations/{id}/active-turn-stream [get]
func (h *Handler) StreamActiveTurn(c *gin.Context) {
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
	if !h.verifyConversationOwnership(c, convID, userID) {
		return
	}

	turn := h.turns.Get(convID)
	if turn == nil {
		// 没有 active 或保留期内的 turn——告诉客户端可以直接 loadMessages
		// 显示最终状态，无需挂 SSE。
		c.Status(http.StatusNoContent)
		return
	}
	h.streamTurnToClient(c, turn)
}

// Regenerate godoc
// @Summary      Regenerate the last assistant reply
// @Description  Discards the most recent assistant turn (if any) and re-runs the agent on the resulting history. SSE stream identical to ChatWithConversation.
// @Tags         ai
// @Produce      text/event-stream
// @Param        id path int true "Conversation ID"
// @Success      200 {string} string "SSE stream"
// @Failure      400 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/conversations/{id}/regenerate [post]
func (h *Handler) Regenerate(c *gin.Context) {
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
	if !h.verifyConversationOwnership(c, convID, userID) {
		return
	}

	// Inspect the last message: if assistant, roll it back inclusive so
	// the agent re-runs against the same history that produced it. If
	// user, leave it alone — the model gets a second chance at the
	// same question. Empty history is a 400, because there is nothing
	// to regenerate from.
	msgs, err := h.conversationService.ListMessages(c.Request.Context(), convID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to load messages")
		return
	}
	if len(msgs) == 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "conversation has no messages to regenerate")
		return
	}
	last := msgs[len(msgs)-1]
	if last.Role == "assistant" {
		if _, _, err := h.conversationService.Rollback(c.Request.Context(), convID, last.ID, true); err != nil {
			slog.Error("regenerate rollback failed", "error", err, "conversationID", convID, "messageID", last.ID)
			h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to rollback last assistant message")
			return
		}
	}

	// Regenerate 与 ChatWithConversation 共享 turn 模型——区别只在不传
	// pendingUserMsgID，因为这一轮重跑针对的是历史尾巴上的 user 消息，
	// 不存在"刚插入需要兜底回滚"的概念。
	if existing := h.turns.Get(convID); existing != nil && !existing.isFinished() {
		h.RespondError(c, http.StatusConflict, web.ErrCodeBadRequest, "previous turn still running; reconnect via stream endpoint")
		return
	}
	turn, startErr := h.turns.Start(convID, func(t *Turn) {
		h.runTurnBody(t, convID, userID, nil)
	})
	if startErr != nil {
		h.RespondError(c, http.StatusConflict, web.ErrCodeBadRequest, "previous turn still running; reconnect via stream endpoint")
		return
	}
	h.streamTurnToClient(c, turn)
}

// verifyConversationOwnership writes the appropriate error response and
// returns false if the conversation is missing or owned by someone
// else. Mirrors conversation.Handler.canAccessConversation but uses the
// AI handler's BaseHandler instance.
//
// Returns 404 (not 403) when the user is not the owner so the API does
// not leak the existence of conversations belonging to other users.
func (h *Handler) verifyConversationOwnership(c *gin.Context, convID, userID int64) bool {
	conv, err := h.conversationService.GetConversation(c.Request.Context(), convID)
	if err != nil {
		if errors.Is(err, conversation.ErrConversationNotFound) {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
			return false
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get conversation")
		return false
	}
	if conv.UserID == nil || *conv.UserID != userID {
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
		return false
	}
	if conv.Status != "active" {
		h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "conversation is archived")
		return false
	}
	return true
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
