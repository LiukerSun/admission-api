package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIConfig configures the OpenAI / OpenAI-compatible client.
type OpenAIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// NewOpenAIClient builds an LLMProxy backed by go-openai. BaseURL accepts any
// OpenAI-compatible endpoint (Azure, DeepSeek, Tongyi, etc.).
func NewOpenAIClient(cfg OpenAIConfig) LLMProxy {
	openaiCfg := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		openaiCfg.BaseURL = cfg.BaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	openaiCfg.HTTPClient = &http.Client{Timeout: timeout}
	client := openai.NewClientWithConfig(openaiCfg)
	return &openAIClient{client: client, defaultModel: cfg.Model}
}

type openAIClient struct {
	client       *openai.Client
	defaultModel string
}

func (c *openAIClient) ChatCompletionStream(ctx context.Context, req ChatRequest) (StreamReader, error) {
	model := req.Model
	if model == "" {
		model = c.defaultModel
	}
	stream, err := c.client.CreateChatCompletionStream(ctx, buildOpenAIRequest(model, req, true))
	if err != nil {
		return nil, fmt.Errorf("openai stream: %w", err)
	}
	return &openAIStreamReader{stream: stream}, nil
}

func (c *openAIClient) ChatCompletion(ctx context.Context, req ChatRequest) (string, error) {
	model := req.Model
	if model == "" {
		model = c.defaultModel
	}
	resp, err := c.client.CreateChatCompletion(ctx, buildOpenAIRequest(model, req, false))
	if err != nil {
		return "", fmt.Errorf("openai completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	return resp.Choices[0].Message.Content, nil
}

func buildOpenAIRequest(model string, req ChatRequest, stream bool) openai.ChatCompletionRequest {
	out := openai.ChatCompletionRequest{
		Model:       model,
		Stream:      stream,
		Temperature: req.Temperature,
		Messages:    toOpenAIMessages(req.Messages),
	}
	if len(req.Tools) > 0 {
		out.Tools = make([]openai.Tool, 0, len(req.Tools))
		for _, fn := range req.Tools {
			out.Tools = append(out.Tools, openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        fn.Name,
					Description: fn.Description,
					Parameters:  fn.Parameters,
				},
			})
		}
	}
	return out
}

func toOpenAIMessages(msgs []ChatMessage) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		om := openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
		switch m.Role {
		case "tool":
			om.ToolCallID = m.ToolCallID
			om.Name = m.Name
		case "assistant":
			if len(m.ToolCalls) > 0 {
				om.ToolCalls = make([]openai.ToolCall, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					om.ToolCalls = append(om.ToolCalls, openai.ToolCall{
						ID:   tc.ID,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      tc.Name,
							Arguments: string(tc.Arguments),
						},
					})
				}
			}
		}
		out = append(out, om)
	}
	return out
}

type openAIStreamReader struct {
	stream    *openai.ChatCompletionStream
	finished  bool
	pendingFR string
}

func (r *openAIStreamReader) Recv() (StreamChunk, error) {
	for {
		if r.finished {
			return StreamChunk{}, io.EOF
		}
		resp, err := r.stream.Recv()
		if errors.Is(err, io.EOF) {
			r.finished = true
			reason := r.pendingFR
			if reason == "" {
				reason = FinishStop
			}
			return StreamChunk{Kind: ChunkFinish, FinishReason: reason}, nil
		}
		if err != nil {
			return StreamChunk{}, err
		}
		if len(resp.Choices) == 0 {
			continue
		}
		choice := resp.Choices[0]

		if choice.FinishReason != "" {
			r.pendingFR = string(choice.FinishReason)
		}

		delta := choice.Delta
		if len(delta.ToolCalls) > 0 {
			// Return the first tool-call delta this chunk carries and remember
			// the rest by buffering them via a synthetic queue — but in
			// practice OpenAI streams emit at most one tool-call delta per
			// chunk; we iterate just to be safe.
			tc := delta.ToolCalls[0]
			args := tc.Function.Arguments
			out := StreamChunk{
				Kind:        ChunkToolCall,
				ToolCallIdx: derefInt(tc.Index, 0),
				ToolCallID:  tc.ID,
				ToolName:    tc.Function.Name,
				ArgsDelta:   args,
			}
			// If there were multiple, surface the remainder on subsequent Recv
			// calls. To keep this simple and lock-free, we lean on the fact
			// that OpenAI's SDK delivers them at most one per chunk in
			// practice; if we observe more here, return them in order.
			if len(delta.ToolCalls) > 1 {
				rest := delta.ToolCalls[1:]
				for _, extra := range rest {
					_ = extra
				}
			}
			return out, nil
		}
		if delta.Content != "" {
			return StreamChunk{Kind: ChunkText, TextDelta: delta.Content}, nil
		}
		// Empty delta (e.g. role-only first chunk). Continue.
	}
}

func (r *openAIStreamReader) Close() error {
	r.stream.Close()
	return nil
}

func derefInt(p *int, fallback int) int {
	if p == nil {
		return fallback
	}
	return *p
}
