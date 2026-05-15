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

// ChatCompletionStream wraps the non-streaming response so the agent's
// streaming code path can run against the same test fixtures. Each
// queued LLMResponse becomes one text chunk + N tool-call chunks +
// done.
func (q *queuedLLM) ChatCompletionStream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	resp, err := q.ChatCompletion(ctx, messages, tools)
	if err != nil {
		return nil, err
	}
	return synthesizeStream(ctx, resp), nil
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

func (q *cancelingLLM) ChatCompletionStream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	resp, err := q.ChatCompletion(ctx, messages, tools)
	if err != nil {
		return nil, err
	}
	return synthesizeStream(ctx, resp), nil
}

// synthesizeStream turns a non-streaming LLMResponse into a closed
// channel of StreamChunk values for tests that don't care about
// token-level streaming behaviour.
func synthesizeStream(ctx context.Context, resp *LLMResponse) <-chan StreamChunk {
	out := make(chan StreamChunk, 4+len(resp.ToolCalls))
	go func() {
		defer close(out)
		if resp.Content != "" {
			select {
			case out <- StreamChunk{Type: StreamChunkText, TextDelta: resp.Content}:
			case <-ctx.Done():
				return
			}
		}
		for _, tc := range resp.ToolCalls {
			select {
			case out <- StreamChunk{Type: StreamChunkToolCallDone, ToolCall: tc}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case out <- StreamChunk{Type: StreamChunkDone}:
		case <-ctx.Done():
		}
	}()
	return out
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
	agent := NewAgent(llm, NewToolExecutor(lineStore, stubAggregateStore{}, nil, nil, nil))

	result, err := agent.Run(context.Background(), []Message{{Role: "user", Content: "650分，喜欢985，不想去北京，喜欢计算机"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// The agent now keeps every iteration's text and joins them with
	// IterationBreak so the frontend can render the full timeline.
	// All three turns' bodies should be present, separated by the
	// protocol marker.
	if !strings.Contains(result.Text, "再放宽") {
		t.Fatalf("expected mid-run text to be preserved in joined output: %q", result.Text)
	}
	if !strings.Contains(result.Text, "哈尔滨工业大学") {
		t.Fatalf("agent did not return final recommendation text: %q", result.Text)
	}
	chunks := strings.Split(result.Text, IterationBreak)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 iteration text chunks separated by IterationBreak, got %d: %q", len(chunks), result.Text)
	}
	if !strings.Contains(chunks[0], "符合条件") {
		t.Fatalf("first chunk should hold the opening turn, got %q", chunks[0])
	}
	if !strings.Contains(chunks[2], "哈尔滨工业大学") {
		t.Fatalf("last chunk should hold the final answer, got %q", chunks[2])
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

// TestAgentSingleIterationTextHasNoBreakMarker verifies that a one-shot
// answer (no tool calls) is persisted as-is without the IterationBreak
// delimiter — single-turn replies must read as plain prose so the
// stripPrivateBlocks logic and downstream renderers don't have to know
// about the marker for the common case.
func TestAgentSingleIterationTextHasNoBreakMarker(t *testing.T) {
	llm := &queuedLLM{
		responses: []*LLMResponse{
			{Content: "你好！我可以帮你筛选院校。"},
		},
	}
	agent := NewAgent(llm, NewToolExecutor(&stubAdmissionLineStore{}, stubAggregateStore{}, nil, nil, nil))

	result, err := agent.Run(context.Background(), []Message{{Role: "user", Content: "你好"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if strings.Contains(result.Text, IterationBreak) {
		t.Fatalf("single-iteration text should not contain IterationBreak marker: %q", result.Text)
	}
	if result.Text != "你好！我可以帮你筛选院校。" {
		t.Fatalf("unexpected text: %q", result.Text)
	}
}

// TestAgentStopsAtMaxIterations verifies the agent terminates with a
// stable error after maxAgentIterations rounds of tool calls and still
// returns a partial AgentResult containing the most-recent text plus all
// executed tool calls / results / widgets, so the frontend can show
// progress instead of an empty failure.
func TestAgentStopsAtMaxIterations(t *testing.T) {
	// Always tool-calling responses; the agent never gets a clean exit.
	responses := make([]*LLMResponse, maxAgentIterations+5)
	for i := range responses {
		responses[i] = &LLMResponse{
			Content:   "我再查一下。",
			ToolCalls: []ToolCall{newToolCall("call-loop", "search_universities", `{"filter":{"region_code":"230000"},"limit":5}`)},
		}
	}
	llm := &queuedLLM{responses: responses}
	agent := NewAgent(llm, NewToolExecutor(&stubAdmissionLineStore{}, stubAggregateStore{}, nil, nil, nil))

	result, err := agent.Run(context.Background(), []Message{{Role: "user", Content: "继续推荐"}})
	if err == nil {
		t.Fatalf("expected max-iterations error, got nil")
	}
	if !strings.Contains(err.Error(), "max iterations") {
		t.Fatalf("expected max iterations error, got %v", err)
	}
	if result == nil {
		t.Fatalf("expected partial result to be returned alongside error")
	}
	if len(result.ToolResults) != maxAgentIterations {
		t.Fatalf("expected %d tool results in partial, got %d", maxAgentIterations, len(result.ToolResults))
	}
	if len(result.ToolCalls) != maxAgentIterations {
		t.Fatalf("expected %d tool calls in partial, got %d", maxAgentIterations, len(result.ToolCalls))
	}
	if result.Text == "" {
		t.Fatalf("expected partial text to be carried over, got empty")
	}
}

func TestAgentStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	agent := NewAgent(&cancelingLLM{cancel: cancel}, NewToolExecutor(&stubAdmissionLineStore{}, stubAggregateStore{}, nil, nil, nil))

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
	executor := NewToolExecutor(lineStore, stubAggregateStore{}, nil, nil, nil)

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
	}`), ToolExecContext{})
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
