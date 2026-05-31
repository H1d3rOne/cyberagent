package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing api key")
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Error("missing version header")
		}
		resp := `{"id":"msg_01","content":[{"type":"text","text":"Hello"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
	defer server.Close()

	provider := NewAnthropic(ProviderConfig{APIKey: "test-key", BaseURL: server.URL, Model: "claude-sonnet-4-20250514"})
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Errorf("expected end_turn, got %s", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(resp.Content))
	}
	text, ok := resp.Content[0].(TextBlock)
	if !ok {
		t.Fatal("expected TextBlock")
	}
	if text.Text != "Hello" {
		t.Errorf("expected Hello, got %s", text.Text)
	}
}

func TestAnthropicToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"id":"msg_02","content":[{"type":"tool_use","id":"tu_01","name":"http_probe","input":{"url":"http://example.com"}}],"stop_reason":"tool_use","usage":{"input_tokens":20,"output_tokens":15}}`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
	defer server.Close()

	provider := NewAnthropic(ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "probe example.com"}}}},
		Tools:    []ToolDef{{Name: "http_probe", Description: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != StopReasonToolUse {
		t.Errorf("expected tool_use, got %s", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(resp.Content))
	}
	tu, ok := resp.Content[0].(ToolUseBlock)
	if !ok {
		t.Fatal("expected ToolUseBlock")
	}
	if tu.Name != "http_probe" {
		t.Errorf("expected http_probe, got %s", tu.Name)
	}
}

func TestOpenAIChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		resp := `{"id":"chatcmpl-01","choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
	defer server.Close()

	provider := NewOpenAI(ProviderConfig{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4o"})
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Errorf("expected end_turn, got %s", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(resp.Content))
	}
}

func TestOpenAIChatContentArrayAndSuggestPrompt(t *testing.T) {
	provider := NewOpenAI(ProviderConfig{})
	resp, err := provider.parseResponse([]byte(`{
		"id":"chatcmpl-02",
		"suggest_prompt":"继续分析 HTTP 响应头",
		"choices":[{"message":{"role":"assistant","content":[{"type":"text","text":"{\"answer\":\"HEAD 请求完成\"}"}]},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":10,"completion_tokens":5}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(resp.Content))
	}
	text, ok := resp.Content[0].(TextBlock)
	if !ok {
		t.Fatal("expected TextBlock")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("expected merged JSON content: %v, text=%s", err, text.Text)
	}
	if payload["answer"] != "HEAD 请求完成" {
		t.Fatalf("unexpected answer: %#v", payload["answer"])
	}
	if payload["suggest_prompt"] != "继续分析 HTTP 响应头" {
		t.Fatalf("suggest_prompt not merged: %#v", payload["suggest_prompt"])
	}
}

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "host root", in: "https://api.example.test", want: "https://api.example.test/v1/chat/completions"},
		{name: "v1 base", in: "https://api.example.test/v1", want: "https://api.example.test/v1/chat/completions"},
		{name: "prefixed v1 base", in: "https://api.example.test/api/v1", want: "https://api.example.test/api/v1/chat/completions"},
		{name: "compatible mode v1 base", in: "https://api.example.test/compatible-mode/v1/", want: "https://api.example.test/compatible-mode/v1/chat/completions"},
		{name: "full endpoint", in: "https://api.example.test/v1/chat/completions", want: "https://api.example.test/v1/chat/completions"},
		{name: "prefixed full endpoint", in: "https://api.example.test/openai/v1/chat/completions", want: "https://api.example.test/openai/v1/chat/completions"},
		{name: "custom path preserved", in: "https://api.example.test/custom/openai", want: "https://api.example.test/custom/openai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOpenAIBaseURL(tt.in); got != tt.want {
				t.Fatalf("normalizeOpenAIBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeAnthropicBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "host root", in: "https://api.example.test", want: "https://api.example.test/v1/messages"},
		{name: "v1 base", in: "https://api.example.test/v1", want: "https://api.example.test/v1/messages"},
		{name: "prefixed v1 base", in: "https://api.example.test/anthropic/v1", want: "https://api.example.test/anthropic/v1/messages"},
		{name: "anthropic sdk base", in: "https://api.example.test/anthropic", want: "https://api.example.test/anthropic/v1/messages"},
		{name: "full endpoint", in: "https://api.example.test/v1/messages", want: "https://api.example.test/v1/messages"},
		{name: "custom path preserved", in: "https://api.example.test/custom/messages-endpoint", want: "https://api.example.test/custom/messages-endpoint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAnthropicBaseURL(tt.in); got != tt.want {
				t.Fatalf("normalizeAnthropicBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestOllamaName(t *testing.T) {
	p := NewOllama(ProviderConfig{})
	if p.Name() != "ollama" {
		t.Errorf("expected ollama, got %s", p.Name())
	}
}
