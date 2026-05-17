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
	"unicode/utf8"
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

	// LLM stream 不保证 chunk 落在 UTF-8 字符边界上，需要本地缓冲不完整
	// 字节避免下游 json.Marshal 把它替换成 U+FFFD。详细背景见 utf8DeltaBuffer。
	var textBuf utf8DeltaBuffer

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
			if tail := textBuf.Flush(); tail != "" {
				send(StreamChunk{Type: StreamChunkText, TextDelta: tail})
			}
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
			safe := textBuf.Push(choice.Delta.Content)
			if safe != "" {
				if !send(StreamChunk{Type: StreamChunkText, TextDelta: safe}) {
					return
				}
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

	// 上游正常结束但没发 [DONE]：同样要 flush UTF-8 残余字节 + tool 调用 + done。
	if tail := textBuf.Flush(); tail != "" {
		send(StreamChunk{Type: StreamChunkText, TextDelta: tail})
	}
	flushPending()
	send(StreamChunk{Type: StreamChunkDone})
}

// utf8DeltaBuffer 缓存 LLM stream chunk 末尾的不完整 UTF-8 字节序列。
//
// 背景：OpenAI / 兼容厂商的 SSE delta 是按 token / 内部 chunk 切的，
// 不保证落在 UTF-8 字符边界上——一个 3 字节中文字符可能被切成"前 1 字
// 节在 chunk N，后 2 字节在 chunk N+1"。Go 的 encoding/json.Marshal 在
// 遇到非法 UTF-8 字节时会替换成 U+FFFD（即 ��），所以如果直接把切碎
// 的 string 一路传到前端，用户会看到尾部乱码（典型现象：句子末尾
// "未来��"）。
//
// Push 收到一段字节后，先和上次的 pending 拼接；找到末尾最后一个 rune
// 起始字节，看它声明的字节数是否被全部覆盖。完整就全部返回，不完整
// 就把"起始字节到末尾"作为 pending 缓存到下次。Flush 在 stream 终态
// 调用，把残余 pending 强制 emit 出去（避免最后一个 rune 真的不完整
// 导致丢字符——尽管这种情况下用户仍可能看到一个 �，但不会"前面文本
// 看不到"）。
type utf8DeltaBuffer struct {
	pending []byte
}

func (b *utf8DeltaBuffer) Push(s string) string {
	if s == "" && len(b.pending) == 0 {
		return ""
	}
	data := make([]byte, 0, len(b.pending)+len(s))
	data = append(data, b.pending...)
	data = append(data, s...)
	b.pending = b.pending[:0]

	// 倒序找最后一个非 continuation 字节（即 rune 起始字节）。
	// utf8.UTFMax=4，所以最多回溯 4 个字节。
	end := len(data)
	for i := end - 1; i >= 0 && end-i <= utf8.UTFMax; i-- {
		c := data[i]
		// continuation byte = 10xxxxxx，跳过。
		if c&0xC0 == 0x80 {
			continue
		}
		need := utf8RuneLenFromLead(c)
		if i+need <= end {
			// 末尾这个 rune 完整——整段 data 安全。
			return string(data)
		}
		// 不完整：把 [i, end) 缓存到下次。
		b.pending = append(b.pending, data[i:]...)
		return string(data[:i])
	}
	// 全是 continuation 字节（罕见，可能上一帧有残留 continuation
	// 但起始字节本身丢了）—— 不缓存，原样发，让 json.Marshal 自行替换。
	return string(data)
}

func (b *utf8DeltaBuffer) Flush() string {
	if len(b.pending) == 0 {
		return ""
	}
	out := string(b.pending)
	b.pending = b.pending[:0]
	return out
}

// utf8RuneLenFromLead 按 lead byte 高位 bit 判定该 rune 总字节数。
// 与 unicode/utf8 内部表保持一致；非法 lead byte 视为 1（不缓存）。
func utf8RuneLenFromLead(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b&0xE0 == 0xC0:
		return 2
	case b&0xF0 == 0xE0:
		return 3
	case b&0xF8 == 0xF0:
		return 4
	default:
		return 1
	}
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
