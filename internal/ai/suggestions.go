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
	"admission-api/internal/userprofile"

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
//
// 同一个 handler 承载两条相关但语义不同的端点：
//   - Suggestions（对话内）：基于当前对话历史，生成"下一个追问"
//   - WelcomeSuggestions（欢迎页）：基于 user profile + 最近对话标题，
//     生成"开场提问"，让欢迎页 chips 个性化而不是写死。
type SuggestionsHandler struct {
	web.BaseHandler
	llm                 LLMProxy
	conversationService conversation.Service
	userProfileService  userprofile.Service
	cache               *redis.Client
}

// NewSuggestionsHandler builds a suggestions handler.
// userProfileService 可为 nil（兼容只用对话内 suggestions 的场景）；
// 为 nil 时 WelcomeSuggestions 退化到纯历史驱动，画像段省略。
func NewSuggestionsHandler(llm LLMProxy, conversationService conversation.Service, userProfileService userprofile.Service, cache *redis.Client) *SuggestionsHandler {
	return &SuggestionsHandler{
		llm:                 llm,
		conversationService: conversationService,
		userProfileService:  userProfileService,
		cache:               cache,
	}
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

// welcomeSystemPrompt 让 LLM 基于用户画像 + 最近对话标题生成 3-4 条
// 开场提问。和 suggestionsSystemPrompt 的核心区别：
//   - 入参不是对话流，而是"用户已知信息 + 历史意图摘要"
//   - 输出更偏"用户主动想做什么"而非"接下来回什么"
//   - 必须覆盖至少 2 种场景，避免 chips 同质化
const welcomeSystemPrompt = `你是高考志愿填报助手的"欢迎页推荐生成器"。基于用户画像和最近对话标题，输出 3-4 条用户最可能想发起的开场提问，用于在欢迎页展示为可点击的胶囊。

严格要求：
1. 只输出一个 JSON 数组，数组元素是字符串，例如 ["提问1","提问2","提问3"]
2. 不要任何 markdown / 代码块 / 解释文本
3. 每条 ≤ 25 个字符，第一人称口语化
4. 若用户已填写分数 / 选科，禁止再生成"我是 X 分"这种重复画像信息的提问
5. 必须覆盖至少 2 种场景（例如：志愿表生成 / 地域偏好 / 专业方向 / 院校点查）
6. 优先复用最近对话标题里出现过但用户可能想换角度再问的方向

样例输出：["帮我做一份志愿表","我只想看上海北京的院校","推荐适合我的专业方向"]`

const welcomeCacheTTL = 30 * time.Minute
const welcomeMaxConversationTitles = 8

// welcomeUserPrompt 把 profile + 历史标题塞成一段紧凑文本喂给 LLM。
// 故意不用结构化 JSON：LLM 对自然语言更稳，且这段 prompt 不需要
// 让 LLM 再回填，只是输入。
func buildWelcomeUserPrompt(p *userprofile.Profile, titles []string) string {
	var b strings.Builder
	b.WriteString("当前用户画像：\n")
	if p != nil {
		if p.RegionCode != nil && *p.RegionCode != "" {
			b.WriteString("  - 省份：" + *p.RegionCode + "\n")
		}
		if p.SubjectCategoryCode != nil && *p.SubjectCategoryCode != "" {
			b.WriteString("  - 科类：" + *p.SubjectCategoryCode + "\n")
		}
		if len(p.ElectiveSubjects) > 0 {
			b.WriteString("  - 选科：" + strings.Join(p.ElectiveSubjects, "+") + "\n")
		}
		if p.TotalScore != nil {
			fmt.Fprintf(&b, "  - 高考总分：%d\n", *p.TotalScore)
		}
	}
	if b.Len() == len("当前用户画像：\n") {
		b.WriteString("  （画像为空）\n")
	}
	if len(titles) > 0 {
		b.WriteString("\n最近 ")
		b.WriteString(strconv.Itoa(len(titles)))
		b.WriteString(" 个对话标题（按时间倒序）：\n")
		for _, t := range titles {
			b.WriteString("  - ")
			b.WriteString(t)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n请输出 3-4 条开场提问的 JSON 数组。")
	return b.String()
}

// WelcomeSuggestions godoc
// @Summary      Personalised welcome-screen suggestion chips
// @Description  Generates 3-4 opening question chips for the AI welcome screen, tailored to the user's profile and recent conversation titles. Cached in Redis.
// @Tags         ai
// @Produce      json
// @Success      200 {object} web.Response
// @Failure      401 {object} web.Response
// @Router       /api/v1/me/welcome-suggestions [get]
func (h *SuggestionsHandler) WelcomeSuggestions(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	var profile *userprofile.Profile
	if h.userProfileService != nil {
		if resp, err := h.userProfileService.GetMyProfile(c.Request.Context(), userID); err == nil && resp != nil {
			p := resp.Profile
			profile = &p
		}
	}

	convs, err := h.conversationService.ListConversations(c.Request.Context(), &userID)
	if err != nil {
		// 历史拿不到不阻塞——用空标题列表继续，生成的 chips 仍可用。
		slog.Warn("welcome suggestions: list conversations failed", "error", err, "userID", userID)
		convs = nil
	}
	titles := make([]string, 0, welcomeMaxConversationTitles)
	for _, c := range convs {
		if c == nil {
			continue
		}
		t := strings.TrimSpace(c.Title)
		if t == "" || t == "新对话" {
			continue
		}
		titles = append(titles, t)
		if len(titles) == welcomeMaxConversationTitles {
			break
		}
	}

	// 缓存 key 同时考虑画像和最新对话——用户填了分数 / 新建了对话都
	// 应该让 chips 跟着变。profile.UpdatedAt 作为画像指纹，最新对话
	// 用最新 ID。任一缺失就用 0。
	profileFingerprint := int64(0)
	if profile != nil {
		profileFingerprint = profile.UpdatedAt.Unix()
	}
	latestConvID := int64(0)
	if len(convs) > 0 && convs[0] != nil {
		latestConvID = convs[0].ID
	}
	cacheKey := fmt.Sprintf("welcome_suggest:%d:%d:%d", userID, profileFingerprint, latestConvID)

	if h.cache != nil {
		if cached, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil && cached != "" {
			var s []string
			if json.Unmarshal([]byte(cached), &s) == nil && len(s) > 0 {
				h.RespondJSON(c, http.StatusOK, web.SuccessResponse(SuggestionsResponse{Suggestions: s}))
				return
			}
		}
	}

	suggestions := h.generateWelcome(c.Request.Context(), profile, titles)

	if h.cache != nil && len(suggestions) > 0 {
		if payload, err := json.Marshal(suggestions); err == nil {
			if cacheErr := h.cache.Set(c.Request.Context(), cacheKey, payload, welcomeCacheTTL); cacheErr != nil {
				slog.Warn("welcome suggestions cache set failed", "error", cacheErr, "userID", userID)
			}
		}
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(SuggestionsResponse{Suggestions: suggestions}))
}

func (h *SuggestionsHandler) generateWelcome(ctx context.Context, profile *userprofile.Profile, titles []string) []string {
	llmCtx, cancel := context.WithTimeout(ctx, suggestionsLLMTimeout)
	defer cancel()

	user := buildWelcomeUserPrompt(profile, titles)
	llmMessages := []Message{
		{Role: "system", Content: welcomeSystemPrompt},
		{Role: "user", Content: user},
	}
	resp, err := h.llm.ChatCompletion(llmCtx, llmMessages, nil)
	if err != nil {
		slog.Warn("welcome suggestions llm call failed", "error", err)
		return []string{}
	}
	return parseSuggestions(resp.Content)
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
