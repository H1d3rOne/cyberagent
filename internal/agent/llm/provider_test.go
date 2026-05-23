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

func TestOllamaName(t *testing.T) {
	p := NewOllama(ProviderConfig{})
	if p.Name() != "ollama" {
		t.Errorf("expected ollama, got %s", p.Name())
	}
}
