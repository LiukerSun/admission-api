package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeStream replays a pre-built script of chunks.
type fakeStream struct {
	chunks []StreamChunk
	idx    int
}

func (s *fakeStream) Recv() (StreamChunk, error) {
	if s.idx >= len(s.chunks) {
		return StreamChunk{}, io.EOF
	}
	c := s.chunks[s.idx]
	s.idx++
	return c, nil
}

func (s *fakeStream) Close() error { return nil }

// fakeLLM returns a queue of streams, one per ChatCompletionStream call.
type fakeLLM struct {
	streams [][]StreamChunk
	calls   int
}

func (f *fakeLLM) ChatCompletionStream(ctx context.Context, req ChatRequest) (StreamReader, error) {
	if f.calls >= len(f.streams) {
		return nil, errors.New("no more streams")
	}
	s := &fakeStream{chunks: f.streams[f.calls]}
	f.calls++
	return s, nil
}

func (f *fakeLLM) ChatCompletion(ctx context.Context, req ChatRequest) (string, error) {
	return "", errors.New("not used")
}

// echoTool emits a fixed content payload for whatever it's called with.
type echoTool struct {
	name    string
	content string
}

func (t *echoTool) Name() string { return t.name }
func (t *echoTool) Schema() FunctionDef {
	return FunctionDef{Name: t.name, Parameters: map[string]any{"type": "object"}}
}
func (t *echoTool) Execute(cc *CallContext, args json.RawMessage) ToolResult {
	return ToolResult{Content: t.content}
}

func TestAgentRunStream_PureText(t *testing.T) {
	llm := &fakeLLM{
		streams: [][]StreamChunk{
			{
				{Kind: ChunkText, TextDelta: "你好"},
				{Kind: ChunkText, TextDelta: "，世界"},
				{Kind: ChunkFinish, FinishReason: FinishStop},
			},
		},
	}
	agent := NewAgent(llm, NewToolRegistry())

	var seen strings.Builder
	cb := AgentCallbacks{OnTextDelta: func(d string) { seen.WriteString(d) }}

	result, err := agent.RunStream(context.Background(), nil, cb)
	if err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	if got := seen.String(); got != "你好，世界" {
		t.Fatalf("seen delta = %q, want %q", got, "你好，世界")
	}
	if result.FinalContent != "你好，世界" {
		t.Fatalf("final content = %q", result.FinalContent)
	}
	if llm.calls != 1 {
		t.Fatalf("expected 1 stream call, got %d", llm.calls)
	}
}

func TestAgentRunStream_ToolLoop(t *testing.T) {
	llm := &fakeLLM{
		streams: [][]StreamChunk{
			// Turn 1: emit tool call in fragments, then finish_reason=tool_calls.
			{
				{Kind: ChunkToolCall, ToolCallIdx: 0, ToolCallID: "call_1", ToolName: "echo", ArgsDelta: `{"q":`},
				{Kind: ChunkToolCall, ToolCallIdx: 0, ArgsDelta: `"hi"}`},
				{Kind: ChunkFinish, FinishReason: FinishToolCalls},
			},
			// Turn 2: emit text then stop.
			{
				{Kind: ChunkText, TextDelta: "tool returned"},
				{Kind: ChunkFinish, FinishReason: FinishStop},
			},
		},
	}
	reg := NewToolRegistry()
	reg.Register(&echoTool{name: "echo", content: `{"answer":"hello"}`})
	agent := NewAgent(llm, reg)

	var (
		toolStarted, toolEnded int
		startedArgs            string
	)
	cb := AgentCallbacks{
		OnToolCallStart: func(callID, name string, args json.RawMessage) {
			toolStarted++
			startedArgs = string(args)
		},
		OnToolCallEnd: func(callID string, ok bool, errMsg string) {
			toolEnded++
			if !ok {
				t.Errorf("tool reported failure: %s", errMsg)
			}
		},
	}

	result, err := agent.RunStream(context.Background(), nil, cb)
	if err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	if toolStarted != 1 || toolEnded != 1 {
		t.Errorf("tool callbacks fired %d start / %d end, want 1/1", toolStarted, toolEnded)
	}
	if startedArgs != `{"q":"hi"}` {
		t.Errorf("accumulated args = %q", startedArgs)
	}
	if result.FinalContent != "tool returned" {
		t.Errorf("final content = %q", result.FinalContent)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "echo" {
		t.Errorf("expected 1 echo tool call, got %+v", result.ToolCalls)
	}
	if len(result.ToolResults) != 1 || result.ToolResults[0].Content != `{"answer":"hello"}` {
		t.Errorf("tool result = %+v", result.ToolResults)
	}
	if llm.calls != 2 {
		t.Errorf("expected 2 stream calls, got %d", llm.calls)
	}
}

func TestAgentRunStream_MaxTurnsCap(t *testing.T) {
	// Loop that never finishes: every turn emits the same tool call.
	makeToolLoopChunks := func() []StreamChunk {
		return []StreamChunk{
			{Kind: ChunkToolCall, ToolCallIdx: 0, ToolCallID: "c", ToolName: "echo", ArgsDelta: `{}`},
			{Kind: ChunkFinish, FinishReason: FinishToolCalls},
		}
	}
	llm := &fakeLLM{streams: [][]StreamChunk{
		makeToolLoopChunks(), makeToolLoopChunks(), makeToolLoopChunks(),
	}}
	reg := NewToolRegistry()
	reg.Register(&echoTool{name: "echo", content: "{}"})
	agent := NewAgent(llm, reg, WithMaxTurns(3))

	var warned int
	_, err := agent.RunStream(context.Background(), nil, AgentCallbacks{
		OnWarning: func(string) { warned++ },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if warned == 0 {
		t.Errorf("expected at least one warning at max turns")
	}
}
