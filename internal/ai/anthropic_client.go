package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// AnthropicClient implements LLMProxy for Anthropic's Messages API.
type AnthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewAnthropicClient creates a new Anthropic client.
func NewAnthropicClient(baseURL, apiKey, model string) *AnthropicClient {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *AnthropicClient) ChatCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (*LLMResponse, error) {
	// Convert messages to Anthropic format (system is a separate field)
	var system string
	anthropicMessages := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		role := m.Role
		if role == "tool" {
			// Tool results in Anthropic are user messages with tool_result content blocks
			role = "user"
			toolResultBlock := map[string]any{
				"type":    "tool_result",
				"content": m.Content,
			}
			if m.ToolCallID != "" {
				toolResultBlock["tool_use_id"] = m.ToolCallID
			}
			anthropicMessages = append(anthropicMessages, map[string]any{
				"role":    role,
				"content": []map[string]any{toolResultBlock},
			})
			continue
		}
		if role == "assistant" && len(m.ContentBlocks) > 0 {
			// Use preserved content blocks (includes thinking, text, tool_use)
			contentBlocks := make([]map[string]any, 0, len(m.ContentBlocks))
			for _, block := range m.ContentBlocks {
				b := map[string]any{"type": block.Type}
				switch block.Type {
				case "text":
					b["text"] = block.Text
				case "thinking":
					b["thinking"] = block.Thinking
					if block.Signature != "" {
						b["signature"] = block.Signature
					}
				case "tool_use":
					b["id"] = block.ID
					b["name"] = block.Name
					b["input"] = block.Input
				}
				contentBlocks = append(contentBlocks, b)
			}
			anthropicMessages = append(anthropicMessages, map[string]any{
				"role":    role,
				"content": contentBlocks,
			})
			continue
		}
		if role == "assistant" && len(m.ToolCalls) > 0 {
			// Fallback: assistant message with tool calls needs content blocks
			contentBlocks := make([]map[string]any, 0)
			if m.Content != "" {
				contentBlocks = append(contentBlocks, map[string]any{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var input map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				contentBlocks = append(contentBlocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			anthropicMessages = append(anthropicMessages, map[string]any{
				"role":    role,
				"content": contentBlocks,
			})
			continue
		}
		anthropicMessages = append(anthropicMessages, map[string]any{
			"role":    role,
			"content": m.Content,
		})
	}

	reqBody := map[string]any{
		"model":      c.model,
		"max_tokens": 4096,
		"messages":   anthropicMessages,
	}
	if system != "" {
		reqBody["system"] = system
	}
	if len(tools) > 0 {
		// Anthropic tools format differs slightly; map from OpenAI-compatible format
		anthropicTools := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			anthropicTools = append(anthropicTools, map[string]any{
				"name":         t.Function.Name,
				"description":  t.Function.Description,
				"input_schema": t.Function.Parameters,
			})
		}
		reqBody["tools"] = anthropicTools
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("API error %d: %s (%s)", resp.StatusCode, errResp.Error.Message, errResp.Error.Type)
	}

	var result struct {
		Content []struct {
			Type      string         `json:"type"`
			Text      string         `json:"text"`
			Name      string         `json:"name"`
			ID        string         `json:"id"`
			Input     map[string]any `json:"input"`
			Thinking  string         `json:"thinking"`
			Data      string         `json:"data"`
			Signature string         `json:"signature"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	llmResp := &LLMResponse{}
	for _, block := range result.Content {
		cb := ContentBlock{
			Type:      block.Type,
			Text:      block.Text,
			Name:      block.Name,
			ID:        block.ID,
			Input:     block.Input,
			Thinking:  block.Thinking,
			Data:      block.Data,
			Signature: block.Signature,
		}
		llmResp.ContentBlocks = append(llmResp.ContentBlocks, cb)
		switch block.Type {
		case "text":
			llmResp.Content += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			llmResp.ToolCalls = append(llmResp.ToolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}
	return llmResp, nil
}
