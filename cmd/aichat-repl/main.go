// aichat-repl 是 prompt 调优用的轻量 REPL：装配真实 LLM + 真实推荐算法 + 真实 PG，
// 但用内存版 draftStore 隔离草稿副作用，便于反复跑端到端对话。
//
// 用法：
//
//	go run ./cmd/aichat-repl                     # 交互模式：stdin 逐行输入
//	go run ./cmd/aichat-repl -script demo.txt    # 脚本模式：文件里每行一句用户消息
//	go run ./cmd/aichat-repl -msg "我是黑龙江物理类580分"
//
// stdin/script 模式都把同一个 Conversation 的历史攒在内存里，模拟真实多轮对话。
// 工具调用、文本增量、最终结果都会带前缀打印到 stdout。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"admission-api/internal/admission"
	"admission-api/internal/ai"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/volunteerplan"
)

const (
	demoUserID         int64 = 9001
	demoConversationID int64 = 9001
)

func main() {
	if err := run(); err != nil {
		slog.Error("aichat-repl failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	scriptPath := flag.String("script", "", "脚本模式：从该文件按行读取用户消息（# 开头视为注释）")
	oneShot := flag.String("msg", "", "单条消息模式：只发送这一条用户消息然后退出")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.LLMAPIKey == "" {
		return errors.New("LLM_API_KEY 未配置；调优需要真实 LLM 调用")
	}

	ctx := context.Background()
	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer database.Close()

	var llmProxy ai.LLMProxy
	if cfg.LLMProvider == "anthropic" {
		llmProxy = ai.NewAnthropicClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	} else {
		llmProxy = ai.NewOpenAIClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	}

	recStore := admission.NewRecommendationStore(database.Pool())
	mdStore := admission.NewRecommendationMetadataStore(database.Pool())
	// 不挂 LLM tuner：调优时只关心算法本身和 prompt 行为，避免双层 LLM 调用拖慢节奏。
	recService := admission.NewRecommendationService(recStore, mdStore, nil)
	admissionLines := admission.NewAdmissionLineStore(database.Pool())
	aggregates := admission.NewAggregateStore(database.Pool())

	draftStore := newMemDraftStore()
	exec := ai.NewToolExecutor(admissionLines, aggregates, recService, draftStore)
	agent := ai.NewAgent(llmProxy, exec)

	r := &repl{
		agent:      agent,
		draftStore: draftStore,
	}

	switch {
	case *oneShot != "":
		return r.send(ctx, *oneShot)
	case *scriptPath != "":
		return r.runScript(ctx, *scriptPath)
	default:
		return r.runInteractive(ctx)
	}
}

// repl 持有一段 Conversation 的累计历史，模拟前端多轮请求里 ListMessages 返回的状态。
type repl struct {
	agent      *ai.Agent
	draftStore *memDraftStore
	history    []ai.Message
}

func (r *repl) runInteractive(ctx context.Context) error {
	fmt.Println("== aichat-repl == 输入消息回车发送；空行退出；/reset 清空历史。")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for {
		fmt.Print("\n你> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			return nil
		}
		if line == "/reset" {
			r.history = nil
			r.draftStore.reset()
			fmt.Println("[history cleared]")
			continue
		}
		if err := r.send(ctx, line); err != nil {
			fmt.Printf("[error] %v\n", err)
		}
	}
}

func (r *repl) runScript(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open script: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fmt.Printf("\n你> %s\n", line)
		if err := r.send(ctx, line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (r *repl) send(ctx context.Context, userMsg string) error {
	r.history = append(r.history, ai.Message{Role: "user", Content: userMsg})

	cb := ai.AgentCallbacks{
		OnTextDelta: func(s string) {
			fmt.Print(s)
		},
		OnToolCallStart: func(callID, toolName string) {
			fmt.Printf("\n[tool→ %s id=%s]\n", toolName, shortID(callID))
		},
		OnToolCallEnd: func(callID string, ok bool, errMsg string) {
			if ok {
				fmt.Printf("\n[tool✓ id=%s]\n", shortID(callID))
			} else {
				fmt.Printf("\n[tool✗ id=%s err=%s]\n", shortID(callID), errMsg)
			}
		},
	}

	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	fmt.Println("\nAI> ")
	result, err := r.agent.RunStreamWithOptions(runCtx, r.history, cb, ai.RunOptions{
		ToolContext: ai.ToolExecContext{
			UserID:         demoUserID,
			ConversationID: demoConversationID,
		},
	})
	if err != nil {
		return err
	}

	// 把这一轮的 assistant 回复 + 所有 tool_call / tool_result 攒进 history，
	// 下一轮才能让模型看到上一次的工具结果。和 handler.streamConversationTurn 行为一致。
	r.history = append(r.history, ai.Message{
		Role:          "assistant",
		Content:       result.Text,
		ToolCalls:     result.ToolCalls,
		ContentBlocks: contentBlocksOf(result),
	})
	for _, tr := range result.ToolResults {
		r.history = append(r.history, ai.Message{
			Role:       "tool",
			Content:    tr.Content,
			ToolCallID: tr.ToolCallID,
		})
	}

	// 打印工具调用的入参 + 摘要返回，方便对照 prompt 行为。
	for i, tc := range result.ToolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		fmt.Printf("\n--- tool_call #%d: %s ---\n  args: %s\n", i+1, tc.Function.Name, compactJSON(tc.Function.Arguments))
		if i < len(result.ToolResults) {
			fmt.Printf("  result: %s\n", trimLong(result.ToolResults[i].Content, 600))
		}
	}
	fmt.Println()
	return nil
}

func contentBlocksOf(r *ai.AgentResult) []ai.ContentBlock {
	// AgentResult 本身不暴露 content blocks；Anthropic 路径上需要把 thinking + tool_use 原样回填，
	// 但 RunStream 收尾时只输出 ToolCalls。为了在 repl 里维持多轮对话，这里只把 tool_calls 重建回来。
	if len(r.ToolCalls) == 0 {
		return nil
	}
	blocks := make([]ai.ContentBlock, 0, len(r.ToolCalls)+1)
	if r.Text != "" {
		blocks = append(blocks, ai.ContentBlock{Type: "text", Text: r.Text})
	}
	for _, tc := range r.ToolCalls {
		var input map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		blocks = append(blocks, ai.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return blocks
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func compactJSON(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func trimLong(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...<truncated>"
}

// memDraftStore 是只为 repl 准备的内存版 DraftStore，保证 dry_run=false 落盘时不污染真实 DB。
type memDraftStore struct {
	mu     sync.Mutex
	nextID int64
	drafts map[int64]*volunteerplan.Draft
}

func newMemDraftStore() *memDraftStore {
	return &memDraftStore{nextID: 1, drafts: map[int64]*volunteerplan.Draft{}}
}

func (m *memDraftStore) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drafts = map[int64]*volunteerplan.Draft{}
}

func (m *memDraftStore) GetByID(_ context.Context, userID, draftID int64) (*volunteerplan.Draft, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drafts[draftID]
	if !ok || d.UserID != userID {
		return nil, volunteerplan.ErrDraftNotFound
	}
	return d, nil
}

func (m *memDraftStore) ListByConversation(_ context.Context, userID, conversationID int64) ([]*volunteerplan.Draft, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*volunteerplan.Draft, 0)
	for _, d := range m.drafts {
		if d.UserID == userID && d.ConversationID == conversationID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *memDraftStore) Create(_ context.Context, userID, conversationID int64, inputJSON []byte, algorithmVersion string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	m.drafts[id] = &volunteerplan.Draft{
		ID:               id,
		UserID:           userID,
		ConversationID:   conversationID,
		Status:           volunteerplan.DraftStatusGenerating,
		InputJSON:        inputJSON,
		AlgorithmVersion: algorithmVersion,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	return id, nil
}

func (m *memDraftStore) MarkReady(_ context.Context, userID, draftID int64, planJSON []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drafts[draftID]
	if !ok || d.UserID != userID {
		return volunteerplan.ErrDraftNotFound
	}
	d.Status = volunteerplan.DraftStatusReady
	d.PlanJSON = planJSON
	d.UpdatedAt = time.Now()
	return nil
}

func (m *memDraftStore) MarkFailed(_ context.Context, userID, draftID int64, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drafts[draftID]
	if !ok || d.UserID != userID {
		return volunteerplan.ErrDraftNotFound
	}
	d.Status = volunteerplan.DraftStatusFailed
	d.Error = errMsg
	d.UpdatedAt = time.Now()
	return nil
}

func (m *memDraftStore) MarkAdopted(_ context.Context, userID, draftID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drafts[draftID]
	if !ok || d.UserID != userID {
		return volunteerplan.ErrDraftNotFound
	}
	d.Status = volunteerplan.DraftStatusAdopted
	d.UpdatedAt = time.Now()
	return nil
}
