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
	"admission-api/internal/platform/redis"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

// suggestionsSystemPrompt instructs the LLM to emit raw JSON only. The
// prompt is intentionally narrow — the suggestions endpoint is a
// classifier-style use of the same backend, not a free-form chat.
const suggestionsSystemPrompt = `你是一个对话建议生成器。基于给出的最近对话历史，输出 2-4 条用户最可能想接下来问的追问，用于在对话底部展示为推荐胶囊。

严格要求：
1. 只输出一个 JSON 数组，数组元素是字符串，例如 ["推荐问题1","推荐问题2","推荐问题3"]。
2. 不要输出任何额外文本、解释、markdown、代码块标记或前后缀。
3. 每条建议不超过 30 个字符，使用问句形式，尽量贴近用户的下一步意图。
4. 如果对话历史不足以判断意图，仍然返回 2 条通用追问，不要返回空数组。`

// suggestionsMaxHistory caps how many trailing messages we feed to the
// suggestions LLM. Older context rarely changes the recommendation and
// adds token cost; keep it tight.
const suggestionsMaxHistory = 10

// suggestionsCacheTTL determines how long a generated set survives in
// Redis. One hour balances cost (no repeat LLM call on quick refresh)
// against staleness if the user comes back to an old conversation.
const suggestionsCacheTTL = time.Hour

// suggestionsLLMTimeout caps the LLM call so a slow upstream cannot
// pin the suggestions endpoint past the rate-limit refill window.
const suggestionsLLMTimeout = 8 * time.Second

// SuggestionsResponse is the public response shape.
type SuggestionsResponse struct {
	Suggestions []string `json:"suggestions"`
}

// SuggestionsHandler is intentionally a separate handler from the main
// AI Handler because it depends on the LLM proxy directly (not the
// Agent + tool loop) and on Redis. Keeping them split lets the main
// streaming handler stay free of Redis and the suggestions endpoint
// stay free of agent plumbing.
type SuggestionsHandler struct {
	web.BaseHandler
	llm                 LLMProxy
	conversationService conversation.Service
	cache               *redis.Client
}

// NewSuggestionsHandler builds a suggestions handler.
func NewSuggestionsHandler(llm LLMProxy, conversationService conversation.Service, cache *redis.Client) *SuggestionsHandler {
	return &SuggestionsHandler{llm: llm, conversationService: conversationService, cache: cache}
}

// Suggestions godoc
// @Summary      Suggested follow-up questions
// @Description  Generates 2-4 follow-up question chips for a conversation. Cached in Redis keyed by (conversation_id, last_message_id).
// @Tags         ai
// @Produce      json
// @Param        id path int true "Conversation ID"
// @Success      200 {object} web.Response
// @Failure      400 {object} web.Response
// @Failure      404 {object} web.Response
// @Router       /api/v1/conversations/{id}/suggestions [get]
func (h *SuggestionsHandler) Suggestions(c *gin.Context) {
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
	conv, err := h.conversationService.GetConversation(c.Request.Context(), convID)
	if err != nil {
		if errors.Is(err, conversation.ErrConversationNotFound) {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to load conversation")
		return
	}
	if conv.UserID == nil || *conv.UserID != userID {
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "conversation not found")
		return
	}

	msgs, err := h.conversationService.ListMessages(c.Request.Context(), convID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to load messages")
		return
	}
	if len(msgs) == 0 {
		// No history yet — return an empty list rather than spending an
		// LLM call. The frontend can hide the rail when empty.
		h.RespondJSON(c, http.StatusOK, web.SuccessResponse(SuggestionsResponse{Suggestions: []string{}}))
		return
	}

	cacheKey := fmt.Sprintf("conv_suggest:%d:%d", convID, msgs[len(msgs)-1].ID)
	if h.cache != nil {
		if cached, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil && cached != "" {
			var s []string
			if json.Unmarshal([]byte(cached), &s) == nil {
				h.RespondJSON(c, http.StatusOK, web.SuccessResponse(SuggestionsResponse{Suggestions: s}))
				return
			}
		}
	}

	suggestions := h.generate(c.Request.Context(), msgs)

	// Cache even on empty / parse-failure results — we still want to
	// suppress repeated LLM calls for the same conversation tail when
	// the upstream is misbehaving. The frontend treats empty as "hide".
	if h.cache != nil {
		if payload, err := json.Marshal(suggestions); err == nil {
			if cacheErr := h.cache.Set(c.Request.Context(), cacheKey, payload, suggestionsCacheTTL); cacheErr != nil {
				slog.Warn("suggestions cache set failed", "error", cacheErr, "conversationID", convID)
			}
		}
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(SuggestionsResponse{Suggestions: suggestions}))
}

// generate calls the LLM once with the suggestions system prompt and
// parses the response as a JSON array of strings. Any failure mode
// (timeout, non-JSON output, oversized strings) collapses to an empty
// slice so the endpoint can return 200 — the suggestions rail is a
// nice-to-have, not load-bearing for conversation correctness.
func (h *SuggestionsHandler) generate(ctx context.Context, msgs []*conversation.Message) []string {
	llmCtx, cancel := context.WithTimeout(ctx, suggestionsLLMTimeout)
	defer cancel()

	tail := msgs
	if len(tail) > suggestionsMaxHistory {
		tail = tail[len(tail)-suggestionsMaxHistory:]
	}
	// Skip widgets / tool calls — the suggestions prompt only needs the
	// natural-language back-and-forth to infer the user's next move.
	llmMessages := make([]Message, 0, len(tail)+1)
	llmMessages = append(llmMessages, Message{Role: "system", Content: suggestionsSystemPrompt})
	for _, m := range tail {
		role := m.Role
		// Collapse tool / system roles into the textual feed; the
		// suggestions model doesn't need raw tool envelopes.
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		llmMessages = append(llmMessages, Message{Role: role, Content: content})
	}

	resp, err := h.llm.ChatCompletion(llmCtx, llmMessages, nil)
	if err != nil {
		slog.Warn("suggestions llm call failed", "error", err)
		return []string{}
	}

	return parseSuggestions(resp.Content)
}

// parseSuggestions extracts a string array from the LLM output. Models
// occasionally wrap the JSON in code fences or prose despite the system
// prompt, so we try the easy parse first, then fall back to slicing
// out the first [...] block before giving up.
func parseSuggestions(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	// Strip common markdown wrappers (```json … ```).
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		// Fall back to the substring between the first '[' and the
		// last ']'.
		start := strings.Index(raw, "[")
		end := strings.LastIndex(raw, "]")
		if start < 0 || end <= start {
			return []string{}
		}
		if err := json.Unmarshal([]byte(raw[start:end+1]), &arr); err != nil {
			return []string{}
		}
	}
	return sanitizeSuggestions(arr)
}

// sanitizeSuggestions trims, drops empties, caps length per item, and
// enforces a 2-4 count. We accept the LLM's order but never propagate
// strings the frontend would render badly.
func sanitizeSuggestions(items []string) []string {
	const maxLen = 60
	const minCount = 2
	const maxCount = 4

	cleaned := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len(s) > maxLen {
			s = s[:maxLen]
		}
		cleaned = append(cleaned, s)
		if len(cleaned) == maxCount {
			break
		}
	}
	if len(cleaned) < minCount {
		// Below the minimum count means the model didn't comply; treat
		// it as a parse failure so the frontend hides the rail.
		return []string{}
	}
	return cleaned
}
