package ai

import (
	"context"
	"strings"
	"testing"

	"admission-api/internal/admission"
)

type queuedLLM struct {
	responses []*LLMResponse
	requests  [][]Message
}

func (q *queuedLLM) ChatCompletion(_ context.Context, messages []Message, _ []ToolDefinition) (*LLMResponse, error) {
	q.requests = append(q.requests, append([]Message(nil), messages...))
	if len(q.responses) == 0 {
		return &LLMResponse{Content: "fallback final answer"}, nil
	}
	resp := q.responses[0]
	q.responses = q.responses[1:]
	return resp, nil
}

type cancelingLLM struct {
	cancel context.CancelFunc
}

func (q *cancelingLLM) ChatCompletion(_ context.Context, messages []Message, _ []ToolDefinition) (*LLMResponse, error) {
	q.cancel()
	return &LLMResponse{
		Content:   "我再查一下。",
		ToolCalls: []ToolCall{newToolCall("call-cancel", "search_universities", `{"filter":{"region_code":"230000"},"limit":5}`)},
	}, nil
}

type stubAdmissionLineStore struct {
	filters []admission.AdmissionLineFilter
}

func (s *stubAdmissionLineStore) ListAdmissionLines(_ context.Context, filter *admission.AdmissionLineFilter) ([]admission.AdmissionLineResponse, error) {
	if filter != nil {
		s.filters = append(s.filters, *filter)
	}
	score := 645
	return []admission.AdmissionLineResponse{
		{UniversityName: "哈尔滨工业大学", LocalMajorName: "计算机类", MinScore: &score},
	}, nil
}

type stubAggregateStore struct{}

func (s stubAggregateStore) Aggregate(context.Context, *admission.AggregateFilter) (*admission.AggregateResponse, error) {
	return &admission.AggregateResponse{}, nil
}

func TestAgentContinuesAfterThreeToolCallsUntilFinalAnswer(t *testing.T) {
	llm := &queuedLLM{
		responses: []*LLMResponse{
			{
				Content: "我先看一下符合条件的院校。",
				ToolCalls: []ToolCall{
					newToolCall("call-1", "apply_filter", `{"filter_type":"replace","filter_data":{"region_code":"230000","is_985":true}}`),
					newToolCall("call-2", "apply_filter", `{"filter_type":"add","filter_data":{"exclude_provinces":["110000"]}}`),
					newToolCall("call-3", "search_universities", `{"filter":{"region_code":"230000","is_985":true},"limit":5}`),
				},
			},
			{
				Content: "条件有点严格，我再放宽看看。",
				ToolCalls: []ToolCall{
					newToolCall("call-4", "search_universities", `{"filter":{"region_code":"230000","is_985":true,"exclude_provinces":["110000"]},"limit":5}`),
				},
			},
			{
				Content: "可以重点看哈尔滨工业大学的计算机类，同时保留几所非北京985作为备选。",
			},
		},
	}
	lineStore := &stubAdmissionLineStore{}
	agent := NewAgent(llm, NewToolExecutor(lineStore, stubAggregateStore{}))

	result, err := agent.Run(context.Background(), []Message{{Role: "user", Content: "650分，喜欢985，不想去北京，喜欢计算机"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if strings.Contains(result.Text, "再放宽") {
		t.Fatalf("agent returned intermediate tool-planning text: %q", result.Text)
	}
	if !strings.Contains(result.Text, "哈尔滨工业大学") {
		t.Fatalf("agent did not return final recommendation text: %q", result.Text)
	}
	if len(result.ToolCalls) != 4 {
		t.Fatalf("expected 4 executed tool calls, got %d", len(result.ToolCalls))
	}
	if len(result.ToolResults) != 4 {
		t.Fatalf("expected 4 tool results, got %d", len(result.ToolResults))
	}
	if len(llm.requests) != 3 {
		t.Fatalf("expected 3 LLM requests, got %d", len(llm.requests))
	}
}

func TestAgentDoesNotStopAfterTenToolIterations(t *testing.T) {
	responses := make([]*LLMResponse, 35, 36)
	for i := range responses {
		responses[i] = &LLMResponse{
			Content:   "我再查一下。",
			ToolCalls: []ToolCall{newToolCall("call-loop", "search_universities", `{"filter":{"region_code":"230000"},"limit":5}`)},
		}
	}
	responses = append(responses, &LLMResponse{
		Content: "根据查询结果，可以优先看哈尔滨工业大学计算机类。",
	})
	llm := &queuedLLM{responses: responses}
	agent := NewAgent(llm, NewToolExecutor(&stubAdmissionLineStore{}, stubAggregateStore{}))

	result, err := agent.Run(context.Background(), []Message{{Role: "user", Content: "继续推荐"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(result.Text, "哈尔滨工业大学") {
		t.Fatalf("expected final answer after extended tool use, got %q", result.Text)
	}
	if len(result.ToolResults) != 35 {
		t.Fatalf("expected 35 tool results, got %d", len(result.ToolResults))
	}
}

func TestAgentStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	agent := NewAgent(&cancelingLLM{cancel: cancel}, NewToolExecutor(&stubAdmissionLineStore{}, stubAggregateStore{}))

	result, err := agent.Run(ctx, []Message{{Role: "user", Content: "继续查询"}})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if result != nil {
		t.Fatalf("expected nil result on cancellation, got %#v", result)
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestToolExecutorParsesSnakeCaseFilterArguments(t *testing.T) {
	lineStore := &stubAdmissionLineStore{}
	executor := NewToolExecutor(lineStore, stubAggregateStore{})

	_, err := executor.Execute(context.Background(), newToolCall("call-1", "search_universities", `{
		"filter": {
			"region_code": "230000",
			"exclude_provinces": ["110000"],
			"is_985": true,
			"min_score_from": 630,
			"min_score_to": 660,
			"tag_query": "计算机"
		},
		"limit": 5
	}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(lineStore.filters) != 1 {
		t.Fatalf("expected one captured filter, got %d", len(lineStore.filters))
	}
	filter := lineStore.filters[0]
	if filter.RegionCode != "230000" {
		t.Fatalf("expected region_code to map to RegionCode, got %q", filter.RegionCode)
	}
	if len(filter.ExcludeProvinces) != 1 || filter.ExcludeProvinces[0] != "110000" {
		t.Fatalf("expected exclude_provinces to map, got %#v", filter.ExcludeProvinces)
	}
	if filter.Is985 == nil || !*filter.Is985 {
		t.Fatalf("expected is_985 to map true, got %#v", filter.Is985)
	}
	if filter.MinScoreFrom == nil || *filter.MinScoreFrom != 630 {
		t.Fatalf("expected min_score_from to map 630, got %#v", filter.MinScoreFrom)
	}
	if filter.MinScoreTo == nil || *filter.MinScoreTo != 660 {
		t.Fatalf("expected min_score_to to map 660, got %#v", filter.MinScoreTo)
	}
	if filter.TagQuery != "计算机" {
		t.Fatalf("expected tag_query to map, got %q", filter.TagQuery)
	}
}

func newToolCall(id, name, args string) ToolCall {
	call := ToolCall{ID: id, Type: "function"}
	call.Function.Name = name
	call.Function.Arguments = args
	return call
}
