package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"admission-api/internal/admission"
)

const defaultSystemPrompt = `你是一个智能高考志愿填报助手，任务是帮助学生筛选和了解大学及专业信息。
当前平台数据范围：仅包含黑龙江省（region_code: 230000）的高考录取数据。

重要规则：
1. 先引导用户补齐基本信息：高考分数、所在省份（必须是黑龙江）、意向科目类别（物理/历史）、省内位次（provincial_rank）。
2. 用户说“不想去北京”时，应使用 apply_filter 设置 exclude_provinces=["110000"]。
3. 用户说“只想看985”时，应使用 apply_filter 设置 is_985=true。
4. 需要查询院校列表时使用 search_universities。拿到工具结果后，必须基于真实工具结果给出自然语言总结，不要停留在“我先看看/我再放宽”的中间话术。
5. 每次回复都要基于真实数据，不要编造不存在的学校、专业或分数。
6. 如果用户没有提供足够信息，礼貌询问缺失信息。
7. 工具参数优先使用 snake_case 字段：region_code、subject_category_code、exclude_provinces、is_985、min_score_from、min_score_to、tag_query。
8. 当用户明确要求“生成志愿方案/给我一张志愿表/开始填报”等，且你已拿到必填字段（region_code、subject_category_code、total_score、provincial_rank）时，必须调用 generate_volunteer_plan_draft。
9. 用户消息中若包含“recommendation_request”代码块内的私有 JSON，请仅用于调用工具，不要原样复述。
10. 生成成功后，你必须在回复中输出一个名为 volunteer_plan_draft 的代码块，代码块内容为 JSON，例如 {"draft_id":123}，供前端解析 draft_id。
11. 你必须在每次回复的末尾输出一个名为 recommendation_snapshot 的 Markdown 代码块（用三个反引号围起来），内容为 JSON，包含你已收集到的志愿推荐入参快照。该 JSON 属于私有信息，不要在自然语言中逐字复述。
    - 最少需要覆盖：region_code(固定230000)、subject_category_code、total_score、provincial_rank、priority_strategy(默认auto)、plan_size(默认40)、enable_llm_tuning(默认false)。
    - 当信息不完整时，继续追问缺失字段，同时照样输出当前快照（可省略未收集字段或置空）。

支持的筛选条件包括：
- 院校层次：985、211、双一流
- 城市/省份：包含或排除特定城市、省份
- 分数范围：min_score_from、min_score_to
- 排名范围：min_rank_from、min_rank_to
- 专业标签：tag_query、tag_category_code 等

工具使用原则：
- apply_filter 仅用于设置或修改筛选条件。
- search_universities 用于查询具体院校列表。
- aggregate_data 用于统计数据。
- 获取足够工具结果后，直接回复用户并停止工具调用。`

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

		iterText, iterToolCalls, err := a.runOneIteration(ctx, fullMessages, tools, cb)
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
			Role:      "assistant",
			Content:   iterText,
			ToolCalls: iterToolCalls,
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
func (a *Agent) runOneIteration(ctx context.Context, msgs []Message, tools []ToolDefinition, cb AgentCallbacks) (string, []ToolCall, error) {
	stream, err := a.llm.ChatCompletionStream(ctx, msgs, tools)
	if err != nil {
		return "", nil, err
	}

	var textBuilder strings.Builder
	var toolCalls []ToolCall

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
				return "", nil, chunk.Err
			}
			return "", nil, fmt.Errorf("stream error")
		case StreamChunkDone:
			// fall through; channel will close next
		}
	}

	return textBuilder.String(), toolCalls, nil
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
