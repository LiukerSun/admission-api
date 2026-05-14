package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"admission-api/internal/admission"
)

const defaultSystemPrompt = `你是一个智能高考志愿填报助手。当前平台数据范围：仅支持黑龙江省（region_code=230000）。

【整体工作流程：四个阶段】
核心理念：漏斗式收敛。第一次信息很少时，候选池可能上千所院校专业组（远超目标值）；随着用户每补充一项偏好，候选池真实变小，逐步逼近 plan_size 这个"目标志愿数"。当池子规模收敛到 plan_size 与 plan_size×1.5 之间且三档非空时，就引导用户落盘成草稿。

阶段 1：基本盘（必备四要素 + 目标志愿数）
- 第一轮收集这四项必填：region_code（仅 230000）、subject_category_code（physics 或 history）、total_score、provincial_rank。
- 同步收集 plan_size（目标志愿数）。不要假定默认值——必须问用户"这次你想生成几个院校专业组的志愿表？"。可补充一句参考："黑龙江新高考政策一次填 40 个院校专业组；如果你只想先看一个小范围的精选可以填少一点（如 20）。"
- 缺哪几项就追问哪几项；每轮 1~2 个最关键的问题，不堆问题。
- 四要素 + plan_size 都齐了立刻进入阶段 2。

阶段 2：漏斗式试探（核心循环）
- 必备字段齐了，立即调用一次 generate_volunteer_plan_draft，dry_run=true。这是首轮基础筛选，仅有硬性必填项，池子通常较大（几百到上千）。
- 此后每收到一项新偏好（专业方向 / 城市偏好 / 学费上限 / 排除关键词 / 单科分数硬门槛 / 家庭资源 / 职业规划 等），都立刻再调用一次 dry_run=true，参数传"累计的完整集合"，不传增量。
- dry_run=true 工具结果字段说明：
  * pool_size：过滤后**真实候选池总数**（漏斗的核心数字，会随用户偏好真实变化）。
  * pool_rush_count / pool_match_count / pool_safe_count：池子按位次窗口的**真实三档分布**。
  * plan_size：用户选定的目标志愿数（落盘时会截到这个数）。
  * sample_items：按 composite_score 排序后取的样例条目（仅供叙述用）。
  * active_hard_filters：本次请求里被算法当作硬过滤剔除候选的字段。
  * active_soft_scoring：本次请求里只参与排序加权、不剔除候选的字段。
  * unused_fields：用户给了但当前算法版本没读取的字段。
- 怎样把数字讲给用户（关键，要诚实）：
  * pool_size 是漏斗主轴。"目前候选池有 X 个院校专业组（目标 plan_size 个）"是每轮回复的核心句。
  * 池子缩小幅度要客观对比上一轮，例如"上轮 850 → 这轮 420，主要因为加了 budget_tuition_max=20000 这条硬过滤"。
  * active_soft_scoring 里的字段不会减少 pool_size，但会让 sample_items 里目标城市/专业的院校排到前面，告诉用户"你给的 preferred_cities 让 [样例院校] 排到前列，但池子里仍保留其他匹配候选"，避免误解为"非偏好项被砍掉"。
  * unused_fields 非空时主动告知"该字段已记录但算法当前版本未读取"，避免画饼。
  * 不要编数字。pool_size、三档分布、sample_items 必须直接来自本次 tool 结果。

阶段 3：判断是否进入落盘
满足下列任一条件时停止追问，进入阶段 4：
- (a) plan_size ≤ pool_size ≤ plan_size × 1.5（例：plan_size=40 时 pool_size ∈ [40, 60]），且 pool_rush/match/safe_count 都 ≥ 1。
- (b) 用户主动说"生成志愿表 / 出方案 / 先看看草稿 / 这就行了"等明确指令。
- (c) 已追问 ≥ 5 轮但 pool_size 仍 < plan_size，告知用户"条件偏严格，先按现状生成一版草稿"。

阶段 4：落盘并引导保存
- 调用一次 generate_volunteer_plan_draft，dry_run=false（或省略该字段），参数与最后一次 dry_run 完全一致。
- 一次会话只能落盘一次；除非用户明确要求"重新生成"，否则不重复落盘。
- 落盘返回 draft_id 后，回复包含两段内容：
  1) 自然语言摘要："志愿表草稿已生成，共 X 个院校专业组（plan_size=X），冲/稳/保为 A/B/C。点击下方『保存为志愿方案』即可入库。"
  2) 一个名为 volunteer_plan_draft 的 Markdown 代码块（用三个反引号包裹），内容为 {"draft_id": <数字>}。
- 不要在自然语言里罗列所有院校；用户保存后可看完整明细。

【硬性规则】
1. region_code 仅支持 230000。
2. 工具参数永远使用 snake_case；列表字段每次传完整累计集合。
3. 单次回复最多 2 个 tool_calls；大多数轮次仅 1 个 dry_run。
4. 不编造院校 / 专业 / 分数；具体院校论断必须来自 tool 结果。
5. 用户消息里 ` + "```recommendation_request```" + ` 等私有 JSON 代码块仅用于提取参数，不要原样复述。
6. 每次回复末尾输出一个 ` + "```recommendation_snapshot```" + ` Markdown 代码块（三个反引号包裹），内容为当前累计入参 JSON。必含 region_code、subject_category_code、total_score、provincial_rank。已收集的偏好字段照填，未收集的字段一律省略（包括 plan_size——用户没明确告知前不要写默认值）。该 JSON 是私有信息，不要在自然语言里复述。

【支持的可选偏好字段】
plan_size（目标志愿数，用户给出）、priority_strategy(auto|school|major)、preferred_majors、excluded_majors、excluded_keywords、preferred_cities、excluded_cities、preferred_provinces、excluded_provinces、family_resources、family_economy、holland_code、math_score / physics_score / chinese_score / english_score、budget_tuition_max、career_plans。

【辅助工具（按需调用）】
- search_universities / aggregate_data：用户问"XX 大学今年位次"这类点查问题时使用；志愿表主流程不依赖。
- render_card / render_chart：用户明确要求可视化时使用。
- apply_filter：旧版搜索筛选工具，新流程默认不需要。`

// AgentResult contains the final output of an agent run.
type AgentResult struct {
	Text        string       `json:"text"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
	Widgets     []Widget     `json:"widgets,omitempty"`
	Filter      any          `json:"filter,omitempty"`
	Data        any          `json:"data,omitempty"`
}

// AgentCallbacks lets the caller observe streaming events without
// coupling the agent to any specific transport (HTTP/SSE). The handler
// translates each callback into a SSE event; tests can substitute
// noop callbacks. All callbacks are optional — a nil function is
// treated as no-op.
//
// Ordering guarantees within a single agent run:
//   - OnTextDelta fires in stream order as the LLM emits content
//   - OnToolCallStart fires once per tool call, immediately before the
//     tool executes; OnToolCallEnd fires once the tool result is in
//   - OnWidget fires inside OnToolCallStart..OnToolCallEnd if the tool
//     produced one (so the frontend can attach widgets to the call)
type AgentCallbacks struct {
	OnTextDelta     func(content string)
	OnToolCallStart func(callID, toolName string)
	OnToolCallEnd   func(callID string, success bool, errMsg string)
	OnWidget        func(widget Widget)
}

func (cb AgentCallbacks) textDelta(s string) {
	if cb.OnTextDelta != nil {
		cb.OnTextDelta(s)
	}
}

func (cb AgentCallbacks) toolCallStart(id, name string) {
	if cb.OnToolCallStart != nil {
		cb.OnToolCallStart(id, name)
	}
}

func (cb AgentCallbacks) toolCallEnd(id string, success bool, errMsg string) {
	if cb.OnToolCallEnd != nil {
		cb.OnToolCallEnd(id, success, errMsg)
	}
}

func (cb AgentCallbacks) widget(w Widget) {
	if cb.OnWidget != nil {
		cb.OnWidget(w)
	}
}

// Agent orchestrates LLM calls with tool execution.
type Agent struct {
	llm      LLMProxy
	executor *ToolExecutor
}

type RunOptions struct {
	ToolContext ToolExecContext
}

// NewAgent creates a new agent.
func NewAgent(llm LLMProxy, executor *ToolExecutor) *Agent {
	return &Agent{
		llm:      llm,
		executor: executor,
	}
}

// Run executes the agent without streaming callbacks. It is a thin
// wrapper around RunStream with no-op callbacks so the two code paths
// never drift apart.
func (a *Agent) Run(ctx context.Context, messages []Message) (*AgentResult, error) {
	return a.RunStreamWithOptions(ctx, messages, AgentCallbacks{}, RunOptions{})
}

// RunStream executes the agent over a streaming LLM connection, invoking
// cb as text, tool calls, and widgets arrive. The returned AgentResult
// is the same shape as Run's, populated cumulatively across iterations.
func (a *Agent) RunStream(ctx context.Context, messages []Message, cb AgentCallbacks) (*AgentResult, error) {
	return a.RunStreamWithOptions(ctx, messages, cb, RunOptions{})
}

func (a *Agent) RunStreamWithOptions(ctx context.Context, messages []Message, cb AgentCallbacks, opts RunOptions) (*AgentResult, error) {
	fullMessages := append([]Message{{Role: "system", Content: defaultSystemPrompt}}, messages...)

	tools := DefaultTools()
	var executedCalls []ToolCall
	var toolResults []ToolResult
	var widgets []Widget

	// widgetEmitter wires the tool executor's widget output into both
	// the cumulative result and the streaming callback. We enforce
	// MaxWidgetsPerRun here so a runaway model cannot flood the channel.
	widgetEmitter := func(w Widget) {
		if len(widgets) >= MaxWidgetsPerRun {
			slog.Warn("widget cap reached, dropping",
				"kind", w.Kind,
				"limit", MaxWidgetsPerRun,
			)
			return
		}
		widgets = append(widgets, w)
		cb.widget(w)
	}

	// toolCallResolver lets render_chart resolve data_source="tool_result:<id>"
	// references against the current run's prior tool results. Passed by
	// closure so the executor itself can stay stateless.
	toolCallResolver := func(callID string) (string, bool) {
		for _, r := range toolResults {
			if r.ToolCallID == callID {
				return r.Content, true
			}
		}
		return "", false
	}

	for iteration := 1; ; iteration++ {
		if err := ctx.Err(); err != nil {
			slog.Warn("agent context finished",
				"iteration", iteration,
				"executedCalls", len(executedCalls),
				"error", err,
			)
			return nil, fmt.Errorf("agent context finished: %w", err)
		}

		slog.Info("agent iteration", "iteration", iteration, "messageCount", len(fullMessages))

		iterText, iterToolCalls, iterBlocks, err := a.runOneIteration(ctx, fullMessages, tools, cb)
		if err != nil {
			// A cancelled context shows up as either an LLM error or
			// just as an early-closed stream; in both cases the cause
			// is "user / upstream gave up", not "model failed". Report
			// it with the same error envelope as the explicit ctx
			// check at the top of the loop so callers get one stable
			// shape.
			if ctx.Err() != nil {
				slog.Warn("agent context finished during llm call",
					"iteration", iteration,
					"executedCalls", len(executedCalls),
					"error", ctx.Err(),
				)
				return nil, fmt.Errorf("agent context finished: %w", ctx.Err())
			}
			slog.Error("agent llm call failed", "error", err)
			return nil, fmt.Errorf("llm call: %w", err)
		}
		// runOneIteration may return cleanly when its producer
		// goroutine bails out on ctx.Done — without this check, a
		// stream that was cut short by cancellation would surface as
		// an empty AgentResult and the caller would think the run
		// succeeded with no text.
		if ctx.Err() != nil {
			slog.Warn("agent context finished after llm stream",
				"iteration", iteration,
				"executedCalls", len(executedCalls),
				"error", ctx.Err(),
			)
			return nil, fmt.Errorf("agent context finished: %w", ctx.Err())
		}

		slog.Info("agent llm iteration result", "contentLen", len(iterText), "toolCalls", len(iterToolCalls))
		for _, tc := range iterToolCalls {
			slog.Info("agent tool call", "name", tc.Function.Name, "id", tc.ID)
		}

		if len(iterToolCalls) == 0 {
			slog.Info("agent returning text", "textLen", len(iterText))
			return &AgentResult{
				Text:        strings.TrimSpace(iterText),
				ToolCalls:   executedCalls,
				ToolResults: toolResults,
				Widgets:     widgets,
			}, nil
		}

		fullMessages = append(fullMessages, Message{
			Role:          "assistant",
			Content:       iterText,
			ToolCalls:     iterToolCalls,
			ContentBlocks: iterBlocks,
		})

		for _, call := range iterToolCalls {
			if err := ctx.Err(); err != nil {
				slog.Warn("agent context finished before tool execution",
					"iteration", iteration,
					"executedCalls", len(executedCalls),
					"error", err,
				)
				return nil, fmt.Errorf("agent context finished: %w", err)
			}

			executedCalls = append(executedCalls, call)
			cb.toolCallStart(call.ID, call.Function.Name)

			execCtx := opts.ToolContext
			execCtx.EmitWidget = widgetEmitter
			execCtx.ResolveResult = toolCallResolver
			result, execErr := a.executor.Execute(ctx, call, execCtx)
			success := true
			errMsg := ""
			if execErr != nil {
				success = false
				errMsg = execErr.Error()
				result = &ToolResult{
					ToolCallID: call.ID,
					Content:    fmt.Sprintf("Error: %v", execErr),
				}
			}
			cb.toolCallEnd(call.ID, success, errMsg)

			toolResults = append(toolResults, *result)

			fullMessages = append(fullMessages, Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: call.ID,
			})
		}
	}
}

// runOneIteration consumes a single ChatCompletionStream call, fanning
// text deltas to cb and accumulating any tool calls to return. It does
// not execute tools — that happens in RunStream so we can interleave
// tool callbacks (start/end/widget) cleanly.
func (a *Agent) runOneIteration(ctx context.Context, msgs []Message, tools []ToolDefinition, cb AgentCallbacks) (string, []ToolCall, []ContentBlock, error) {
	stream, err := a.llm.ChatCompletionStream(ctx, msgs, tools)
	if err != nil {
		return "", nil, nil, err
	}

	var textBuilder strings.Builder
	var toolCalls []ToolCall
	var contentBlocks []ContentBlock

	for chunk := range stream {
		switch chunk.Type {
		case StreamChunkText:
			textBuilder.WriteString(chunk.TextDelta)
			cb.textDelta(chunk.TextDelta)
		case StreamChunkToolCallDone:
			toolCalls = append(toolCalls, chunk.ToolCall)
		case StreamChunkError:
			// Drain the rest of the channel before returning so the
			// producer goroutine exits cleanly.
			for range stream {
			}
			if chunk.Err != nil {
				return "", nil, nil, chunk.Err
			}
			return "", nil, nil, fmt.Errorf("stream error")
		case StreamChunkDone:
			// providers that surface structured content (Anthropic 系，
			// 含 thinking blocks) 在 done chunk 上携带完整 ContentBlocks。
			contentBlocks = chunk.ContentBlocks
		}
	}

	return textBuilder.String(), toolCalls, contentBlocks, nil
}

// ExtractFilter tries to extract the current AdmissionLineFilter from tool call history.
func ExtractFilter(calls []ToolCall) (*FilterState, error) {
	state := &FilterState{}
	for _, call := range calls {
		if call.Function.Name != "apply_filter" {
			continue
		}
		var params struct {
			FilterType string          `json:"filter_type"`
			FilterData json.RawMessage `json:"filter_data"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
			continue
		}
		var filter admission.AdmissionLineFilter
		if err := decodeAdmissionLineFilter(params.FilterData, &filter); err != nil {
			continue
		}
		switch params.FilterType {
		case "replace", "add":
			state.Apply(&filter)
		case "remove":
			state.Remove(&filter)
		case "reset":
			state.Reset()
		}
	}
	return state, nil
}

// FilterState tracks the current filter built up through tool calls.
type FilterState struct {
	Filter *admission.AdmissionLineFilter
}

// Apply merges the given filter into the current state.
func (s *FilterState) Apply(f *admission.AdmissionLineFilter) {
	if s.Filter == nil {
		s.Filter = &admission.AdmissionLineFilter{}
	}
	if f.RegionCode != "" {
		s.Filter.RegionCode = f.RegionCode
	}
	if f.SubjectCategoryCode != "" {
		s.Filter.SubjectCategoryCode = f.SubjectCategoryCode
	}
	if f.AdmissionYear != nil {
		s.Filter.AdmissionYear = f.AdmissionYear
	}
	if len(f.UniversityCodes) > 0 {
		s.Filter.UniversityCodes = append(s.Filter.UniversityCodes, f.UniversityCodes...)
	}
	if len(f.GroupCodes) > 0 {
		s.Filter.GroupCodes = append(s.Filter.GroupCodes, f.GroupCodes...)
	}
	if len(f.Cities) > 0 {
		s.Filter.Cities = append(s.Filter.Cities, f.Cities...)
	}
	if len(f.ExcludeCities) > 0 {
		s.Filter.ExcludeCities = append(s.Filter.ExcludeCities, f.ExcludeCities...)
	}
	if len(f.Provinces) > 0 {
		s.Filter.Provinces = append(s.Filter.Provinces, f.Provinces...)
	}
	if len(f.ExcludeProvinces) > 0 {
		s.Filter.ExcludeProvinces = append(s.Filter.ExcludeProvinces, f.ExcludeProvinces...)
	}
	if f.Is985 != nil {
		s.Filter.Is985 = f.Is985
	}
	if f.Is211 != nil {
		s.Filter.Is211 = f.Is211
	}
	if f.IsDoubleFirstClass != nil {
		s.Filter.IsDoubleFirstClass = f.IsDoubleFirstClass
	}
	if f.MinScoreFrom != nil {
		s.Filter.MinScoreFrom = f.MinScoreFrom
	}
	if f.MinScoreTo != nil {
		s.Filter.MinScoreTo = f.MinScoreTo
	}
	if f.MinRankFrom != nil {
		s.Filter.MinRankFrom = f.MinRankFrom
	}
	if f.MinRankTo != nil {
		s.Filter.MinRankTo = f.MinRankTo
	}
}

// Remove clears fields present in the given filter.
func (s *FilterState) Remove(f *admission.AdmissionLineFilter) {
	if s.Filter == nil {
		return
	}
	if f.Is985 != nil && s.Filter.Is985 != nil && *f.Is985 == *s.Filter.Is985 {
		s.Filter.Is985 = nil
	}
	if f.Is211 != nil && s.Filter.Is211 != nil && *f.Is211 == *s.Filter.Is211 {
		s.Filter.Is211 = nil
	}
}

// Reset clears all filters.
func (s *FilterState) Reset() {
	s.Filter = &admission.AdmissionLineFilter{}
}
