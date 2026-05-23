package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const openaiDefaultURL = "https://api.openai.com/v1/chat/completions"

type OpenAIProvider struct {
	config ProviderConfig
	client *http.Client
}

func NewOpenAI(config ProviderConfig) *OpenAIProvider {
	if config.BaseURL == "" {
		config.BaseURL = openaiDefaultURL
	}
	if config.Model == "" {
		config.Model = "gpt-4o"
	}
	return &OpenAIProvider{config: config, client: &http.Client{}}
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
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
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

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
		return nil, fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(respBody))
	}

	return p.parseResponse(respBody)
}

type openaiRequest struct {
	Model       string       `json:"model"`
	Messages    []openaiMsg  `json:"messages"`
	Tools       []openaiTool `json:"tools,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
}

type openaiMsg struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (p *OpenAIProvider) buildRequestBody(req ChatRequest) openaiRequest {
	msgs := make([]openaiMsg, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, openaiMsg{Role: "system", Content: req.System})
	}
	for _, msg := range req.Messages {
		oMsg := openaiMsg{Role: string(msg.Role)}
		for _, block := range msg.Content {
			switch b := block.(type) {
			case TextBlock:
				oMsg.Content = b.Text
			case ToolUseBlock:
				oMsg.ToolCalls = append(oMsg.ToolCalls, openaiToolCall{
					ID:   b.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: b.Name, Arguments: string(b.Input)},
				})
			case ToolResultBlock:
				msgs = append(msgs, openaiMsg{Role: "tool", Content: b.Content, ToolCallID: b.ToolUseID})
				continue
			}
		}
		if oMsg.Content != nil || oMsg.ToolCalls != nil {
			msgs = append(msgs, oMsg)
		}
	}

	tools := make([]openaiTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, openaiTool{
			Type:     "function",
			Function: openaiFunction{Name: t.Name, Description: t.Description, Parameters: t.InputSchema},
		})
	}

	return openaiRequest{Model: req.Model, Messages: msgs, Tools: tools, MaxTokens: req.MaxTokens, Temperature: req.Temperature}
}

type openaiResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string           `json:"role"`
			Content   *string          `json:"content"`
			ToolCalls []openaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *OpenAIProvider) parseResponse(body []byte) (*ChatResponse, error) {
	var raw openaiResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := raw.Choices[0]
	blocks := make([]ContentBlock, 0)
	if choice.Message.Content != nil && *choice.Message.Content != "" {
		blocks = append(blocks, TextBlock{Text: *choice.Message.Content})
	}
	for _, tc := range choice.Message.ToolCalls {
		blocks = append(blocks, ToolUseBlock{ID: tc.ID, Name: tc.Function.Name, Input: json.RawMessage(tc.Function.Arguments)})
	}

	stopReason := StopReasonEndTurn
	switch choice.FinishReason {
	case "tool_calls":
		stopReason = StopReasonToolUse
	case "length":
		stopReason = StopReasonMaxTokens
	}

	return &ChatResponse{
		ID:         raw.ID,
		Content:    blocks,
		StopReason: stopReason,
		Usage:      Usage{InputTokens: raw.Usage.PromptTokens, OutputTokens: raw.Usage.CompletionTokens},
	}, nil
}
