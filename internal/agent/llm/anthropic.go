package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const anthropicDefaultURL = "https://api.anthropic.com/v1/messages"
const anthropicAPIVersion = "2023-06-01"

type AnthropicProvider struct {
	config ProviderConfig
	client *http.Client
}

func NewAnthropic(config ProviderConfig) *AnthropicProvider {
	if config.BaseURL == "" {
		config.BaseURL = anthropicDefaultURL
	}
	if config.Model == "" {
		config.Model = "claude-sonnet-4-20250514"
	}
	return &AnthropicProvider{config: config, client: &http.Client{}}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = p.config.Model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	body := p.buildRequestBody(req)
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
	}

	return p.parseResponse(respBody)
}

type anthropicRequest struct {
	Model       string          `json:"model"`
	System      string          `json:"system,omitempty"`
	Messages    []anthropicMsg  `json:"messages"`
	Tools       []anthropicTool `json:"tools,omitempty"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature,omitempty"`
}

type anthropicMsg struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func (p *AnthropicProvider) buildRequestBody(req ChatRequest) anthropicRequest {
	msgs := make([]anthropicMsg, 0, len(req.Messages))
	for _, msg := range req.Messages {
		aMsg := anthropicMsg{Role: string(msg.Role)}
		for _, block := range msg.Content {
			switch b := block.(type) {
			case TextBlock:
				aMsg.Content = append(aMsg.Content, map[string]string{"type": "text", "text": b.Text})
			case ToolUseBlock:
				aMsg.Content = append(aMsg.Content, map[string]interface{}{"type": "tool_use", "id": b.ID, "name": b.Name, "input": json.RawMessage(b.Input)})
			case ToolResultBlock:
				aMsg.Content = append(aMsg.Content, map[string]interface{}{"type": "tool_result", "tool_use_id": b.ToolUseID, "content": b.Content, "is_error": b.IsError})
			}
		}
		msgs = append(msgs, aMsg)
	}

	tools := make([]anthropicTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}

	return anthropicRequest{
		Model:       req.Model,
		System:      req.System,
		Messages:    msgs,
		Tools:       tools,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
}

type anthropicResponse struct {
	ID         string                   `json:"id"`
	Content    []map[string]interface{} `json:"content"`
	StopReason string                   `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *AnthropicProvider) parseResponse(body []byte) (*ChatResponse, error) {
	var raw anthropicResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	blocks := make([]ContentBlock, 0, len(raw.Content))
	for _, item := range raw.Content {
		typ, _ := item["type"].(string)
		switch typ {
		case "text":
			text, _ := item["text"].(string)
			blocks = append(blocks, TextBlock{Text: text})
		case "tool_use":
			id, _ := item["id"].(string)
			name, _ := item["name"].(string)
			inputRaw, _ := json.Marshal(item["input"])
			blocks = append(blocks, ToolUseBlock{ID: id, Name: name, Input: inputRaw})
		}
	}

	return &ChatResponse{
		ID:         raw.ID,
		Content:    blocks,
		StopReason: StopReason(raw.StopReason),
		Usage:      Usage{InputTokens: raw.Usage.InputTokens, OutputTokens: raw.Usage.OutputTokens},
	}, nil
}
