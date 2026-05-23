package llm

import (
	"context"
	"encoding/json"
)

// ToolDef describes a tool the LLM can call.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ChatRequest is the input to a Provider.Chat call.
type ChatRequest struct {
	Model       string    `json:"model"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature,omitempty"`
}

// ChatResponse is the output of a Provider.Chat call.
type ChatResponse struct {
	ID         string         `json:"id"`
	Content    []ContentBlock `json:"content"`
	StopReason StopReason     `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent is one chunk from a streaming response.
type StreamEvent struct {
	Type       string       // "text_delta", "tool_use", "done", "error"
	TextDelta  string       // partial text for streaming display
	Block      ContentBlock // complete block (on finish)
	Done       bool
	StopReason StopReason
	Usage      Usage
	Error      error
}

// Provider is the interface for LLM backends.
type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Name() string
}

// StreamProvider extends Provider with streaming support.
type StreamProvider interface {
	Provider
	StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
}

// ProviderConfig holds common provider configuration.
type ProviderConfig struct {
	APIKey  string `json:"api_key" yaml:"api_key"`
	BaseURL string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Model   string `json:"model" yaml:"model"`
}
