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
1. 先引导用户补齐基本信息：高考分数、所在省份（必须是黑龙江）、意向科目类别（物理/历史）、性别、出生日期。
2. 用户说“不想去北京”时，应使用 apply_filter 设置 exclude_provinces=["110000"]。
3. 用户说“只想看985”时，应使用 apply_filter 设置 is_985=true。
4. 需要查询院校列表时使用 search_universities。拿到工具结果后，必须基于真实工具结果给出自然语言总结，不要停留在“我先看看/我再放宽”的中间话术。
5. 每次回复都要基于真实数据，不要编造不存在的学校、专业或分数。
6. 如果用户没有提供足够信息，礼貌询问缺失信息。
7. 工具参数优先使用 snake_case 字段：region_code、subject_category_code、exclude_provinces、is_985、min_score_from、min_score_to、tag_query。

支持的筛选条件包括：
- 院校层次：985、211、双一流
- 城市/省份：包含或排除特定城市、省份
- 分数范围：min_score_from、min_score_to
- 排名范围：min_rank_from、min_rank_to
- 专业标签：tag_query、tag_category_code 等

工具使用原则：
- apply_filter 仅用于设置或修改筛选条件。
- search_universities 用于查询具体院校列表和录取分数。
- aggregate_data 用于统计数据。
- retrieve_knowledge 用于检索政策解读、填报策略、专业分析、案例参考等非结构化知识。当用户问以下类型问题时，优先使用 retrieve_knowledge：
  * 政策类：强基计划、提前批、赋分规则、专项计划等
  * 策略类：冲稳保、志愿排序、风险分析等
  * 专业类：专业对比、就业前景、适合什么学生等
  * 家庭/情绪类：经济条件限制、家庭压力、安抚建议等
  * 如果问题同时涉及分数和策略，可同时使用 search_universities 和 retrieve_knowledge
- 获取足够工具结果后，直接回复用户并停止工具调用。`

// AgentResult contains the final output of an agent run.
type AgentResult struct {
	Text        string       `json:"text"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
	Filter      any          `json:"filter,omitempty"`
	Data        any          `json:"data,omitempty"`
}

// Agent orchestrates LLM calls with tool execution.
type Agent struct {
	llm      LLMProxy
	executor *ToolExecutor
}

// NewAgent creates a new agent.
func NewAgent(llm LLMProxy, executor *ToolExecutor) *Agent {
	return &Agent{
		llm:      llm,
		executor: executor,
	}
}

// Run executes the agent with the given conversation messages.
func (a *Agent) Run(ctx context.Context, messages []Message) (*AgentResult, error) {
	fullMessages := append([]Message{{Role: "system", Content: defaultSystemPrompt}}, messages...)

	tools := DefaultTools()
	var executedCalls []ToolCall
	var toolResults []ToolResult

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
		resp, err := a.llm.ChatCompletion(ctx, fullMessages, tools)
		if err != nil {
			slog.Error("agent llm call failed", "error", err)
			return nil, fmt.Errorf("llm call: %w", err)
		}
		slog.Info("agent llm response", "contentLen", len(resp.Content), "toolCalls", len(resp.ToolCalls))
		for _, tc := range resp.ToolCalls {
			slog.Info("agent tool call", "name", tc.Function.Name, "id", tc.ID)
		}

		if len(resp.ToolCalls) == 0 {
			slog.Info("agent returning text", "textLen", len(resp.Content))
			return &AgentResult{
				Text:        strings.TrimSpace(resp.Content),
				ToolCalls:   executedCalls,
				ToolResults: toolResults,
			}, nil
		}

		fullMessages = append(fullMessages, Message{
			Role:          "assistant",
			Content:       resp.Content,
			ToolCalls:     resp.ToolCalls,
			ContentBlocks: resp.ContentBlocks,
		})

		for _, call := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				slog.Warn("agent context finished before tool execution",
					"iteration", iteration,
					"executedCalls", len(executedCalls),
					"error", err,
				)
				return nil, fmt.Errorf("agent context finished: %w", err)
			}

			executedCalls = append(executedCalls, call)

			result, err := a.executor.Execute(ctx, call)
			if err != nil {
				result = &ToolResult{
					ToolCallID: call.ID,
					Content:    fmt.Sprintf("Error: %v", err),
				}
			}
			toolResults = append(toolResults, *result)

			fullMessages = append(fullMessages, Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: call.ID,
			})
		}
	}
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
