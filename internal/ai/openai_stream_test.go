package ai

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// streamOpenAIBody is the most error-prone path in the whole codebase:
// OpenAI streams tool calls as a sequence of partial frames keyed by an
// index, where the id and function.name typically arrive in the first
// frame and the arguments JSON is concatenated across many. The agent
// loop downstream EXPECTS each tool_call_done chunk to carry the full
// arguments JSON. If the accumulator drops a chunk, mis-keys an index,
// or flushes early, the tool will execute with malformed JSON.
//
// These tests use synthetic OpenAI-shaped SSE bodies to pin the
// accumulation behaviour without touching the real API.

// fakeBody wraps a string as an io.ReadCloser. Close is a no-op.
type fakeBody struct{ *strings.Reader }

func (fakeBody) Close() error { return nil }

func bodyFromFrames(frames ...string) io.ReadCloser {
	var sb strings.Builder
	for _, f := range frames {
		sb.WriteString(f)
		// Each SSE frame is terminated by a blank line.
		sb.WriteString("\n\n")
	}
	return fakeBody{strings.NewReader(sb.String())}
}

// drainChunks consumes all chunks from out until it closes, with a
// safety timeout so a stuck producer doesn't hang the suite.
func drainChunks(t *testing.T, out <-chan StreamChunk) []StreamChunk {
	t.Helper()
	var got []StreamChunk
	timeout := time.After(2 * time.Second)
	for {
		select {
		case c, ok := <-out:
			if !ok {
				return got
			}
			got = append(got, c)
		case <-timeout:
			t.Fatalf("stream did not close within 2s; got %d chunks so far", len(got))
			return got
		}
	}
}

// TestStreamOpenAIBodyAccumulatesToolCallArguments is the headline
// case: OpenAI emits tool_call.id and .function.name in the first frame
// of an index, then dribbles the JSON arguments out across several
// frames, then issues a finish_reason="tool_calls". We must produce one
// tool_call_done chunk per index with the fully concatenated arguments.
func TestStreamOpenAIBodyAccumulatesToolCallArguments(t *testing.T) {
	body := bodyFromFrames(
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"search_universities","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"filter\":{\"region_code\":\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"230000\",\"is_985\":true}}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	)

	out := make(chan StreamChunk, 16)
	go streamOpenAIBody(context.Background(), body, out)
	chunks := drainChunks(t, out)

	var doneCalls []ToolCall
	sawDone := false
	for _, c := range chunks {
		switch c.Type {
		case StreamChunkToolCallDone:
			doneCalls = append(doneCalls, c.ToolCall)
		case StreamChunkDone:
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatalf("expected a StreamChunkDone, got chunks=%#v", chunks)
	}
	if len(doneCalls) != 1 {
		t.Fatalf("expected exactly one tool_call_done, got %d (chunks=%#v)", len(doneCalls), chunks)
	}
	tc := doneCalls[0]
	if tc.ID != "call_abc" {
		t.Fatalf("tool call id = %q, want call_abc", tc.ID)
	}
	if tc.Function.Name != "search_universities" {
		t.Fatalf("tool call name = %q, want search_universities", tc.Function.Name)
	}
	want := `{"filter":{"region_code":"230000","is_985":true}}`
	if tc.Function.Arguments != want {
		t.Fatalf("tool call arguments = %q, want %q", tc.Function.Arguments, want)
	}
}

// TestStreamOpenAIBodyEmitsTextDeltasInOrder pins that text content
// arrives as a stream of separate text_delta chunks in the order the
// upstream model emits them.
func TestStreamOpenAIBodyEmitsTextDeltasInOrder(t *testing.T) {
	body := bodyFromFrames(
		`data: {"choices":[{"delta":{"content":"你好"}}]}`,
		`data: {"choices":[{"delta":{"content":"，我"}}]}`,
		`data: {"choices":[{"delta":{"content":"来帮你查"}}]}`,
		`data: [DONE]`,
	)

	out := make(chan StreamChunk, 16)
	go streamOpenAIBody(context.Background(), body, out)
	chunks := drainChunks(t, out)

	var texts []string
	for _, c := range chunks {
		if c.Type == StreamChunkText {
			texts = append(texts, c.TextDelta)
		}
	}
	want := []string{"你好", "，我", "来帮你查"}
	if !equalStringSlice(texts, want) {
		t.Fatalf("text deltas = %#v, want %#v", texts, want)
	}
}

// TestStreamOpenAIBodyHandlesMultipleToolCallIndexes verifies that when
// the model emits parallel tool calls in a single turn (indexes 0 and 1
// interleaved across frames), both are flushed with their own arguments
// in the index order they first appeared.
func TestStreamOpenAIBodyHandlesMultipleToolCallIndexes(t *testing.T) {
	body := bodyFromFrames(
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c0","type":"function","function":{"name":"apply_filter","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"c1","type":"function","function":{"name":"search_universities","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"y\":2}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	)

	out := make(chan StreamChunk, 16)
	go streamOpenAIBody(context.Background(), body, out)
	chunks := drainChunks(t, out)

	var calls []ToolCall
	for _, c := range chunks {
		if c.Type == StreamChunkToolCallDone {
			calls = append(calls, c.ToolCall)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls flushed, got %d", len(calls))
	}
	// Index 0 appeared first → must be emitted first.
	if calls[0].ID != "c0" || calls[0].Function.Arguments != `{"x":1}` {
		t.Fatalf("first call wrong: %+v", calls[0])
	}
	if calls[1].ID != "c1" || calls[1].Function.Arguments != `{"y":2}` {
		t.Fatalf("second call wrong: %+v", calls[1])
	}
}

// TestStreamOpenAIBodyTerminatesOnDoneWithoutFinishReason proves we
// flush any pending tool calls when the upstream sends [DONE] without
// ever emitting a finish_reason. Some compatible servers (DeepSeek,
// older OpenAI proxies) don't include finish_reason; we must still
// produce the tool_call_done chunk.
func TestStreamOpenAIBodyTerminatesOnDoneWithoutFinishReason(t *testing.T) {
	body := bodyFromFrames(
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c","type":"function","function":{"name":"t","arguments":"{\"q\":1}"}}]}}]}`,
		`data: [DONE]`,
	)
	out := make(chan StreamChunk, 16)
	go streamOpenAIBody(context.Background(), body, out)
	chunks := drainChunks(t, out)

	var calls []ToolCall
	for _, c := range chunks {
		if c.Type == StreamChunkToolCallDone {
			calls = append(calls, c.ToolCall)
		}
	}
	if len(calls) != 1 {
		t.Fatalf("expected one tool_call_done, got %d", len(calls))
	}
	if calls[0].Function.Arguments != `{"q":1}` {
		t.Fatalf("arguments wrong: %q", calls[0].Function.Arguments)
	}
}

// TestStreamOpenAIBodyIgnoresMalformedFrames documents that a single
// junk SSE frame in the middle of a stream does not abort the whole
// response — we keep going so a transient parse error does not kill an
// otherwise good answer.
func TestStreamOpenAIBodyIgnoresMalformedFrames(t *testing.T) {
	body := bodyFromFrames(
		`data: {"choices":[{"delta":{"content":"A"}}]}`,
		`data: not-json-at-all`,
		`data: {"choices":[{"delta":{"content":"B"}}]}`,
		`data: [DONE]`,
	)
	out := make(chan StreamChunk, 16)
	go streamOpenAIBody(context.Background(), body, out)
	chunks := drainChunks(t, out)

	var texts []string
	for _, c := range chunks {
		if c.Type == StreamChunkText {
			texts = append(texts, c.TextDelta)
		}
	}
	want := []string{"A", "B"}
	if !equalStringSlice(texts, want) {
		t.Fatalf("text deltas = %#v, want %#v", texts, want)
	}
}

// TestStreamOpenAIBodyClosesBodyOnContextCancel proves we don't leak
// the HTTP body when the caller cancels mid-stream.
func TestStreamOpenAIBodyClosesBodyOnContextCancel(t *testing.T) {
	closed := &trackingBody{Reader: bytes.NewReader([]byte(""))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := make(chan StreamChunk, 4)
	streamOpenAIBody(ctx, closed, out) // synchronous so we know when it returns
	if !closed.closeCalled {
		t.Fatalf("body.Close should be called on context cancel")
	}
}

type trackingBody struct {
	*bytes.Reader
	closeCalled bool
}

func (t *trackingBody) Close() error {
	t.closeCalled = true
	return nil
}
