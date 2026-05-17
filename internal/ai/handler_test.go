package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admission-api/internal/conversation"
	"admission-api/internal/platform/middleware"

	"github.com/gin-gonic/gin"
)

func TestConversationMessagesToAIMessagesSkipsLegacyMissingToolResults(t *testing.T) {
	toolCalls, _ := json.Marshal([]ToolCall{
		newToolCall("call-1", "search_universities", `{"filter":{"region_code":"230000"}}`),
	})

	messages := conversationMessagesToAIMessages([]*conversation.Message{
		{
			Role:      "assistant",
			Content:   "我先看看符合条件的院校。",
			ToolCalls: toolCalls,
		},
	})

	if len(messages) != 1 {
		t.Fatalf("expected one assistant message, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 0 {
		t.Fatalf("expected legacy dangling tool calls to be skipped, got %d", len(messages[0].ToolCalls))
	}
}

func TestConversationMessagesToAIMessagesReplaysPersistedToolResults(t *testing.T) {
	toolCalls, _ := json.Marshal([]ToolCall{
		newToolCall("call-1", "search_universities", `{"filter":{"region_code":"230000"}}`),
	})
	toolResults, _ := json.Marshal([]ToolResult{
		{ToolCallID: "call-1", Content: `{"count":1}`},
	})

	messages := conversationMessagesToAIMessages([]*conversation.Message{
		{
			Role:        "assistant",
			Content:     "我先看看符合条件的院校。",
			ToolCalls:   toolCalls,
			ToolResults: toolResults,
		},
	})

	if len(messages) != 2 {
		t.Fatalf("expected assistant plus tool result, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool call to be preserved, got %d", len(messages[0].ToolCalls))
	}
	if messages[1].Role != "tool" || messages[1].ToolCallID != "call-1" {
		t.Fatalf("expected tool result replay, got %#v", messages[1])
	}
}

// --- Handler validation tests ----------------------------------------

// noopLLM is a stub LLMProxy that always returns a trivial reply, used
// only to satisfy the agent constructor in tests where the agent should
// never actually run.
type noopLLM struct{}

func (noopLLM) ChatCompletion(_ context.Context, _ []Message, _ []ToolDefinition) (*LLMResponse, error) {
	return &LLMResponse{Content: "ok"}, nil
}

func (noopLLM) ChatCompletionStream(ctx context.Context, _ []Message, _ []ToolDefinition) (<-chan StreamChunk, error) {
	out := make(chan StreamChunk, 2)
	out <- StreamChunk{Type: StreamChunkText, TextDelta: "ok"}
	out <- StreamChunk{Type: StreamChunkDone}
	close(out)
	return out, nil
}

// stubConvServiceForAI implements just enough of conversation.Service
// for AI handler tests. Methods that aren't expected to be called by
// the test panic via the embedded nil interface.
type stubConvServiceForAI struct {
	conversation.Service
	getFunc          func(ctx context.Context, id int64) (*conversation.Conversation, error)
	addMessageFunc   func(ctx context.Context, conversationID int64, role, content string, toolCalls, toolResults, widgets []byte) (*conversation.Message, error)
	listMessagesFunc func(ctx context.Context, conversationID int64) ([]*conversation.Message, error)
}

func (s stubConvServiceForAI) GetConversation(ctx context.Context, id int64) (*conversation.Conversation, error) {
	return s.getFunc(ctx, id)
}

func (s stubConvServiceForAI) AddMessage(ctx context.Context, conversationID int64, role, content string, toolCalls, toolResults, widgets []byte) (*conversation.Message, error) {
	return s.addMessageFunc(ctx, conversationID, role, content, toolCalls, toolResults, widgets)
}

func (s stubConvServiceForAI) ListMessages(ctx context.Context, conversationID int64) ([]*conversation.Message, error) {
	return s.listMessagesFunc(ctx, conversationID)
}

func withUserAI(userID int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.ContextUserIDKey, userID)
		c.Next()
	}
}

// TestChatRejectsTooManyMessages enforces the per-request message-count
// cap. Without this, a caller could submit hundreds of historical
// messages and amplify cost / latency on every request.
func TestChatRejectsTooManyMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agent := NewAgent(noopLLM{}, NewToolExecutor(nil, nil, nil, nil, nil, nil))
	handler := NewHandler(agent, nil, nil)

	router := gin.New()
	router.Use(withUserAI(7))
	router.POST("/api/v1/ai/chat", handler.Chat)

	msgs := make([]Message, MaxAIChatMessages+1)
	for i := range msgs {
		msgs[i] = Message{Role: "user", Content: "hi"}
	}
	body, _ := json.Marshal(ChatRequest{Messages: msgs})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for excessive message count, got %d", rec.Code)
	}
}

// TestChatRejectsOversizedSingleMessage caps any single message in the
// /ai/chat array to MaxAIChatMessageBytes.
func TestChatRejectsOversizedSingleMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agent := NewAgent(noopLLM{}, NewToolExecutor(nil, nil, nil, nil, nil, nil))
	handler := NewHandler(agent, nil, nil)

	router := gin.New()
	router.Use(withUserAI(7))
	router.POST("/api/v1/ai/chat", handler.Chat)

	huge := strings.Repeat("a", MaxAIChatMessageBytes+1)
	body, _ := json.Marshal(ChatRequest{Messages: []Message{{Role: "user", Content: huge}}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized message, got %d", rec.Code)
	}
}

// TestChatRejectsOversizedTotalContent caps the sum of all message
// content so an attacker can't sneak in many medium-sized messages.
func TestChatRejectsOversizedTotalContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agent := NewAgent(noopLLM{}, NewToolExecutor(nil, nil, nil, nil, nil, nil))
	handler := NewHandler(agent, nil, nil)

	router := gin.New()
	router.Use(withUserAI(7))
	router.POST("/api/v1/ai/chat", handler.Chat)

	// Each message just below per-message limit; enough of them to
	// blow the total cap.
	chunk := strings.Repeat("b", MaxAIChatMessageBytes-1)
	count := MaxAIChatTotalBytes/(MaxAIChatMessageBytes-1) + 2
	if count > MaxAIChatMessages {
		count = MaxAIChatMessages // stay under the count limit so we hit total bytes first
	}
	msgs := make([]Message, count)
	for i := range msgs {
		msgs[i] = Message{Role: "user", Content: chunk}
	}
	body, _ := json.Marshal(ChatRequest{Messages: msgs})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized total content, got %d", rec.Code)
	}
}

// TestChatRejectsEmptyMessages prevents a no-op request from triggering
// an LLM call that just bills the user for the system prompt.
func TestChatRejectsEmptyMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agent := NewAgent(noopLLM{}, NewToolExecutor(nil, nil, nil, nil, nil, nil))
	handler := NewHandler(agent, nil, nil)

	router := gin.New()
	router.Use(withUserAI(7))
	router.POST("/api/v1/ai/chat", handler.Chat)

	body, _ := json.Marshal(ChatRequest{Messages: []Message{}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty messages, got %d", rec.Code)
	}
}

// TestChatWithConversationRejectsOversizedMessage mirrors the per-message
// cap on the conversation-scoped endpoint.
func TestChatWithConversationRejectsOversizedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agent := NewAgent(noopLLM{}, NewToolExecutor(nil, nil, nil, nil, nil, nil))
	handler := NewHandler(agent, stubConvServiceForAI{}, nil)

	router := gin.New()
	router.Use(withUserAI(7))
	router.POST("/api/v1/conversations/:id/ai-chat", handler.ChatWithConversation)

	huge := strings.Repeat("c", MaxAIChatMessageBytes+1)
	body, _ := json.Marshal(map[string]string{"message": huge})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/ai-chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized conversation message, got %d", rec.Code)
	}
}

// TestChatWithConversationRejectsEmptyMessage stops a whitespace-only
// payload from creating an empty user row in the conversation history.
func TestChatWithConversationRejectsEmptyMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agent := NewAgent(noopLLM{}, NewToolExecutor(nil, nil, nil, nil, nil, nil))
	handler := NewHandler(agent, stubConvServiceForAI{}, nil)

	router := gin.New()
	router.Use(withUserAI(7))
	router.POST("/api/v1/conversations/:id/ai-chat", handler.ChatWithConversation)

	body, _ := json.Marshal(map[string]string{"message": "   \n\t   "})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/ai-chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty conversation message, got %d", rec.Code)
	}
}

// recordingLLM is a stub LLM that records whether ChatCompletion was
// invoked. Used to prove that when the handler short-circuits on a save
// failure, it never reaches the LLM (which would burn tokens for a
// request that's about to fail anyway, and worse, run on a history that
// is missing the user's latest turn).
type recordingLLM struct{ called bool }

func (r *recordingLLM) ChatCompletion(_ context.Context, _ []Message, _ []ToolDefinition) (*LLMResponse, error) {
	r.called = true
	return &LLMResponse{Content: "should not be reached"}, nil
}

func (r *recordingLLM) ChatCompletionStream(ctx context.Context, _ []Message, _ []ToolDefinition) (<-chan StreamChunk, error) {
	r.called = true
	out := make(chan StreamChunk, 2)
	out <- StreamChunk{Type: StreamChunkText, TextDelta: "should not be reached"}
	out <- StreamChunk{Type: StreamChunkDone}
	close(out)
	return out, nil
}

// TestChatWithConversationFailsFastWhenUserMessageSaveFails proves that
// if persisting the user's message fails, we MUST surface the failure
// to the caller and abort. Previously the handler swallowed the error
// (`_, _ = ...`), then ran the LLM on a history that was missing the
// latest user turn — which produced a real assistant reply tied to a
// non-existent question, polluting future replays of the conversation.
func TestChatWithConversationFailsFastWhenUserMessageSaveFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := int64(7)
	llm := &recordingLLM{}
	agent := NewAgent(llm, NewToolExecutor(nil, nil, nil, nil, nil, nil))

	listCalled := false
	handler := NewHandler(agent, stubConvServiceForAI{
		getFunc: func(ctx context.Context, id int64) (*conversation.Conversation, error) {
			return &conversation.Conversation{ID: id, UserID: &userID, Status: "active"}, nil
		},
		addMessageFunc: func(ctx context.Context, conversationID int64, role, content string, toolCalls, toolResults, widgets []byte) (*conversation.Message, error) {
			return nil, errors.New("db down")
		},
		listMessagesFunc: func(ctx context.Context, conversationID int64) ([]*conversation.Message, error) {
			listCalled = true
			return nil, nil
		},
	}, nil)

	router := gin.New()
	router.Use(withUserAI(userID))
	router.POST("/api/v1/conversations/:id/ai-chat", handler.ChatWithConversation)

	body, _ := json.Marshal(map[string]string{"message": "你好"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/ai-chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 when user message save fails, got %d body=%s", rec.Code, rec.Body.String())
	}
	if listCalled {
		t.Fatal("ListMessages must NOT be called once user message save fails — otherwise we'd hand the LLM a stale history")
	}
	if llm.called {
		t.Fatal("LLM must NOT be invoked once user message save fails — otherwise we burn tokens producing a reply to a question that was never persisted")
	}
}
