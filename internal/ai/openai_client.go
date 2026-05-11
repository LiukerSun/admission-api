package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIClient implements LLMProxy for OpenAI-compatible APIs (DeepSeek, etc.).
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIClient creates a new OpenAI-compatible client.
func NewOpenAIClient(baseURL, apiKey, model string) *OpenAIClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			// Streaming responses are long-lived; rely on caller ctx for
			// timeout instead of a default client-level deadline.
			Timeout: 0,
		},
	}
}

func (c *OpenAIClient) buildRequestBody(messages []Message, tools []ToolDefinition, stream bool) map[string]any {
	reqBody := map[string]any{
		"model":       c.model,
		"messages":    messages,
		"temperature": 0.7,
		"max_tokens":  4096,
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
		reqBody["tool_choice"] = "auto"
	}
	if stream {
		reqBody["stream"] = true
		// We do not consume usage stats; opt out to avoid an extra final
		// chunk that some compatible servers emit unexpectedly.
		reqBody["stream_options"] = map[string]any{"include_usage": false}
	}
	return reqBody
}

func (c *OpenAIClient) ChatCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (*LLMResponse, error) {
	// Use a per-call deadline only for the non-streaming variant. The
	// streaming variant must rely on caller ctx since responses can run
	// minutes for long multi-turn tool loops.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	jsonBody, err := json.Marshal(c.buildRequestBody(messages, tools, false))
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeOpenAIError(resp)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Role      string     `json:"role"`
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &LLMResponse{
		Content:   result.Choices[0].Message.Content,
		ToolCalls: result.Choices[0].Message.ToolCalls,
	}, nil
}

// ChatCompletionStream issues a streaming chat completion. The returned
// channel emits StreamChunk values as SSE frames arrive. Tool calls are
// accumulated by index here and only emitted as StreamChunkToolCallDone
// once the upstream stream signals finish_reason=="tool_calls" (or the
// stream ends), so callers never see partial arguments fragments.
func (c *OpenAIClient) ChatCompletionStream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	jsonBody, err := json.Marshal(c.buildRequestBody(messages, tools, true))
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Drain & close before returning the decoded error so the
		// underlying TCP connection can be reused.
		errVal := decodeOpenAIError(resp)
		_ = resp.Body.Close()
		return nil, errVal
	}

	out := make(chan StreamChunk, 32)
	go streamOpenAIBody(ctx, resp.Body, out)
	return out, nil
}

// streamOpenAIBody reads SSE frames from the OpenAI response body and
// translates them into StreamChunk values on out. It closes out before
// returning and closes the HTTP body so the connection can be reused.
func streamOpenAIBody(ctx context.Context, body io.ReadCloser, out chan<- StreamChunk) {
	defer close(out)
	defer body.Close()

	// toolCallAccumulator stores the partial ID / name / arguments for
	// each tool call slot OpenAI streams back. The index keys this map.
	type toolCallAcc struct {
		ID        string
		Name      string
		Arguments strings.Builder
	}
	pending := make(map[int]*toolCallAcc)
	// indexOrder preserves the order indices first appeared so the
	// flushed tool calls match the model's emission order.
	var indexOrder []int

	flushPending := func() {
		for _, idx := range indexOrder {
			acc := pending[idx]
			if acc == nil {
				continue
			}
			tc := ToolCall{ID: acc.ID, Type: "function"}
			tc.Function.Name = acc.Name
			tc.Function.Arguments = acc.Arguments.String()
			select {
			case out <- StreamChunk{Type: StreamChunkToolCallDone, ToolCall: tc}:
			case <-ctx.Done():
				return
			}
		}
		pending = make(map[int]*toolCallAcc)
		indexOrder = nil
	}

	send := func(chunk StreamChunk) bool {
		select {
		case out <- chunk:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// SSE frames are delimited by blank lines; bufio.Scanner with the
	// default ScanLines splitter works because we look for the "data: "
	// prefix per non-empty line.
	scanner := bufio.NewScanner(body)
	// Default 64KB buffer is too small for long tool_call argument
	// payloads; bump to 1MB.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			flushPending()
			send(StreamChunk{Type: StreamChunkDone})
			return
		}

		var frame struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			// Skip malformed frames rather than killing the stream; a
			// transient bad frame should not poison the whole response.
			continue
		}
		if len(frame.Choices) == 0 {
			continue
		}
		choice := frame.Choices[0]
		if choice.Delta.Content != "" {
			if !send(StreamChunk{Type: StreamChunkText, TextDelta: choice.Delta.Content}) {
				return
			}
		}
		for _, tc := range choice.Delta.ToolCalls {
			acc, exists := pending[tc.Index]
			if !exists {
				acc = &toolCallAcc{}
				pending[tc.Index] = acc
				indexOrder = append(indexOrder, tc.Index)
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Function.Name != "" {
				acc.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.Arguments.WriteString(tc.Function.Arguments)
			}
		}
		if choice.FinishReason == "tool_calls" || choice.FinishReason == "stop" {
			// Some servers stop here without ever emitting [DONE]; flush
			// any pending tool calls now so the agent loop can react.
			if choice.FinishReason == "tool_calls" {
				flushPending()
			}
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		send(StreamChunk{Type: StreamChunkError, Err: fmt.Errorf("scan stream: %w", err)})
		return
	}

	flushPending()
	send(StreamChunk{Type: StreamChunkDone})
}

// decodeOpenAIError reads the JSON error body from a non-200 response.
// It is shared by the streaming and non-streaming paths.
func decodeOpenAIError(resp *http.Response) error {
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	return fmt.Errorf("API error %d: %s (%s)", resp.StatusCode, errResp.Error.Message, errResp.Error.Type)
}
