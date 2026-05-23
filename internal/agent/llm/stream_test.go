package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicStreamChat(t *testing.T) {
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","usage":{"input_tokens":25,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}

event: message_stop
data: {"type":"message_stop"}

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	provider := NewAnthropic(ProviderConfig{APIKey: "test-key", BaseURL: server.URL, Model: "claude-sonnet-4-20250514"})
	ch, err := provider.StreamChat(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var textParts []string
	var gotDone bool
	for event := range ch {
		switch event.Type {
		case "text_delta":
			textParts = append(textParts, event.TextDelta)
		case "done":
			gotDone = true
			if event.StopReason != StopReasonEndTurn {
				t.Errorf("expected end_turn, got %s", event.StopReason)
			}
			if event.Usage.OutputTokens != 10 {
				t.Errorf("expected 10 output tokens, got %d", event.Usage.OutputTokens)
			}
		}
	}

	if !gotDone {
		t.Error("expected done event")
	}
	fullText := ""
	for _, p := range textParts {
		fullText += p
	}
	if fullText != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", fullText)
	}
}

func TestAnthropicStreamToolUse(t *testing.T) {
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_02","usage":{"input_tokens":30,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tu_01","name":"http_probe"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"url\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"http://example.com\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":15}}

event: message_stop
data: {"type":"message_stop"}

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	provider := NewAnthropic(ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	ch, err := provider.StreamChat(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "probe"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var gotToolUse bool
	var gotDone bool
	for event := range ch {
		switch event.Type {
		case "tool_use":
			gotToolUse = true
			tu, ok := event.Block.(ToolUseBlock)
			if !ok {
				t.Fatal("expected ToolUseBlock")
			}
			if tu.Name != "http_probe" {
				t.Errorf("expected http_probe, got %s", tu.Name)
			}
			if tu.ID != "tu_01" {
				t.Errorf("expected tu_01, got %s", tu.ID)
			}
		case "done":
			gotDone = true
			if event.StopReason != StopReasonToolUse {
				t.Errorf("expected tool_use, got %s", event.StopReason)
			}
		}
	}

	if !gotToolUse {
		t.Error("expected tool_use event")
	}
	if !gotDone {
		t.Error("expected done event")
	}
}
