package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const openaiDefaultURL = "https://api.openai.com/v1/chat/completions"

type OpenAIProvider struct {
	config ProviderConfig
	client *http.Client
}

func NewOpenAI(config ProviderConfig) *OpenAIProvider {
	config.BaseURL = ResolveOpenAIEndpoint(config.BaseURL)
	if config.Model == "" {
		config.Model = "gpt-4o"
	}
	return &OpenAIProvider{config: config, client: &http.Client{}}
}

// ResolveOpenAIEndpoint resolves a user-provided OpenAI-compatible base URL to
// the concrete chat-completions endpoint used by this provider.
func ResolveOpenAIEndpoint(base string) string {
	if strings.TrimSpace(base) == "" {
		base = openaiDefaultURL
	}
	return normalizeOpenAIBaseURL(base)
}

// NormalizeOpenAIBaseURL normalizes non-empty OpenAI-compatible URLs without
// applying the default OpenAI endpoint.
func NormalizeOpenAIBaseURL(base string) string {
	return normalizeOpenAIBaseURL(base)
}

// normalizeOpenAIBaseURL appends the chat completions path when the user
// provides only a base endpoint (e.g. ".../v1", ".../api/v1",
// ".../compatible-mode/v1"), matching standard OpenAI-compatible client
// behavior. Custom non-versioned paths are treated as already-concrete
// endpoints and are left unchanged.
func normalizeOpenAIBaseURL(base string) string {
	base = strings.TrimSpace(base)
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return base
	}
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	lowerPath := strings.ToLower(trimmedPath)
	if strings.HasSuffix(lowerPath, "/chat/completions") {
		parsed.Path = trimmedPath
		return parsed.String()
	}
	switch {
	case trimmedPath == "":
		parsed.Path = "/v1/chat/completions"
		return parsed.String()
	case strings.HasSuffix(lowerPath, "/v1"):
		parsed.Path = trimmedPath + "/chat/completions"
		return parsed.String()
	default:
		return base
	}
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
		return nil, fmt.Errorf("openai API error %d at %s: %s", resp.StatusCode, diagnosticURL(p.config.BaseURL), responseSnippet(respBody, 1200))
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
	ID               string   `json:"id"`
	SuggestedPrompt  string   `json:"suggested_prompt,omitempty"`
	SuggestPrompt    string   `json:"suggest_prompt,omitempty"`
	SuggestedPrompts []string `json:"suggested_prompts,omitempty"`
	Choices          []struct {
		SuggestedPrompt  string   `json:"suggested_prompt,omitempty"`
		SuggestPrompt    string   `json:"suggest_prompt,omitempty"`
		SuggestedPrompts []string `json:"suggested_prompts,omitempty"`
		Message          struct {
			Role             string           `json:"role"`
			Content          openaiContent    `json:"content"`
			SuggestedPrompt  string           `json:"suggested_prompt,omitempty"`
			SuggestPrompt    string           `json:"suggest_prompt,omitempty"`
			SuggestedPrompts []string         `json:"suggested_prompts,omitempty"`
			ToolCalls        []openaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type openaiContent struct {
	raw json.RawMessage
}

func (c *openaiContent) UnmarshalJSON(data []byte) error {
	c.raw = append(c.raw[:0], data...)
	return nil
}

func (c openaiContent) Text() string {
	if len(c.raw) == 0 || bytes.Equal(bytes.TrimSpace(c.raw), []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(c.raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var parts []any
	if err := json.Unmarshal(c.raw, &parts); err != nil {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch item := part.(type) {
		case string:
			if value := strings.TrimSpace(item); value != "" {
				out = append(out, value)
			}
		case map[string]any:
			for _, key := range []string{"text", "content"} {
				if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
					out = append(out, strings.TrimSpace(value))
					break
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
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
	if content := mergeOpenAISuggestPrompt(choice.Message.Content.Text(), firstNonEmpty(
		choice.Message.SuggestedPrompt,
		choice.Message.SuggestPrompt,
		firstString(choice.Message.SuggestedPrompts),
		choice.SuggestedPrompt,
		choice.SuggestPrompt,
		firstString(choice.SuggestedPrompts),
		raw.SuggestedPrompt,
		raw.SuggestPrompt,
		firstString(raw.SuggestedPrompts),
	)); content != "" {
		blocks = append(blocks, TextBlock{Text: content})
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

func mergeOpenAISuggestPrompt(content, suggest string) string {
	content = strings.TrimSpace(content)
	suggest = strings.TrimSpace(suggest)
	if suggest == "" {
		return content
	}
	if content == "" {
		encoded, _ := json.Marshal(map[string]any{"suggest_prompt": suggest})
		return string(encoded)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil || payload == nil {
		return content
	}
	if strings.TrimSpace(stringFromAny(payload["suggest_prompt"])) == "" &&
		strings.TrimSpace(stringFromAny(payload["suggested_prompt"])) == "" {
		payload["suggest_prompt"] = suggest
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return content
	}
	return string(encoded)
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
