package planner

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestActionUnmarshalAcceptsModelArgumentVariants(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		wantName string
		wantArgs map[string]any
	}{
		{
			name:     "arguments object",
			payload:  `{"name":"http.request","arguments":{"url":"https://example.com","method":"GET"}}`,
			wantName: "http.request",
			wantArgs: map[string]any{"url": "https://example.com", "method": "GET"},
		},
		{
			name:     "args alias",
			payload:  `{"name":"workspace.search","args":{"query":"TODO"}}`,
			wantName: "workspace.search",
			wantArgs: map[string]any{"query": "TODO"},
		},
		{
			name:     "input json string",
			payload:  `{"name":"workspace.files.read","input":"{\"path\":\"README.md\"}"}`,
			wantName: "workspace.files.read",
			wantArgs: map[string]any{"path": "README.md"},
		},
		{
			name:     "function envelope",
			payload:  `{"function":{"name":"ports.scan.safe","arguments":{"host":"example.com"}}}`,
			wantName: "ports.scan.safe",
			wantArgs: map[string]any{"host": "example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var action Action
			if err := json.Unmarshal([]byte(tt.payload), &action); err != nil {
				t.Fatalf("unmarshal action: %v", err)
			}
			if action.Name != tt.wantName {
				t.Fatalf("name = %q, want %q", action.Name, tt.wantName)
			}
			if !reflect.DeepEqual(action.Arguments, tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", action.Arguments, tt.wantArgs)
			}
		})
	}
}

func TestNormalizeActionArguments(t *testing.T) {
	if got := NormalizeActionArguments(nil); len(got) != 0 {
		t.Fatalf("nil args = %#v, want empty map", got)
	}
	if got := NormalizeActionArguments(map[string]any{" path ": "a.md"}); got["path"] != "a.md" {
		t.Fatalf("map any args = %#v", got)
	}
	if got := NormalizeActionArguments(map[string]string{" query ": "x"}); got["query"] != "x" {
		t.Fatalf("map string args = %#v", got)
	}
	if got := NormalizeActionArguments(`{"path":"a.md"}`); got["path"] != "a.md" {
		t.Fatalf("json string args = %#v", got)
	}
	if got := NormalizeActionArguments("plain input"); got["input"] != "plain input" {
		t.Fatalf("plain string args = %#v", got)
	}
	if got := NormalizeActionArguments(42); got["input"] != 42 {
		t.Fatalf("default args = %#v", got)
	}
}

func TestNextActionUnmarshalPromptAliases(t *testing.T) {
	var next NextAction
	if err := json.Unmarshal([]byte(`{"type":"continue","reason":"r","prompt":"fallback","auto_continue":true}`), &next); err != nil {
		t.Fatalf("unmarshal next action: %v", err)
	}
	if next.SuggestedPrompt != "fallback" || !next.AutoContinue {
		t.Fatalf("unexpected next action: %#v", next)
	}

	if err := json.Unmarshal([]byte(`{"type":"continue","suggest_prompt":"alias","suggested_prompt":"primary"}`), &next); err != nil {
		t.Fatalf("unmarshal next action alias: %v", err)
	}
	if next.SuggestedPrompt != "primary" {
		t.Fatalf("suggested prompt = %q", next.SuggestedPrompt)
	}
}

func TestParseResponsePlainJSONNormalizesSuggestedPrompt(t *testing.T) {
	raw := `{
		"phase":"plan",
		"answer":"ok",
		"suggest_prompt":"continue with safe scan",
		"actions":[{"name":"scan.web.safe","arguments":{"target":"https://example.com"}}]
	}`
	plan, err := ParseResponse(raw, func(plan *Response) bool { return len(plan.Actions) > 0 })
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if plan.Answer != "ok" || len(plan.Actions) != 1 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if plan.NextAction == nil || plan.NextAction.SuggestedPrompt != "continue with safe scan" {
		t.Fatalf("suggested prompt was not normalized: %#v", plan.NextAction)
	}
}

func TestParseResponseFindsLaterBalancedJSONObject(t *testing.T) {
	raw := `planner draft {"answer": ""} final {"answer":"use this","task":{"kind":"chat"}}`
	plan, err := ParseResponse(raw, nil)
	if err != nil {
		t.Fatalf("parse balanced objects: %v", err)
	}
	if plan.Answer != "use this" || plan.Task == nil || plan.Task.Kind != "chat" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestParseResponseUnwrapsOpenAIChatCompletion(t *testing.T) {
	envelope := `{
		"suggested_prompt":"root suggestion",
		"choices":[{"message":{"content":"{\"answer\":\"wrapped\",\"actions\":[{\"name\":\"web.fetch\",\"arguments\":{\"url\":\"https://example.com\"}}]}"}}]
	}`
	plan, err := ParseResponse(envelope, func(plan *Response) bool { return len(plan.Actions) > 0 })
	if err != nil {
		t.Fatalf("parse OpenAI envelope: %v", err)
	}
	if plan.Answer != "wrapped" || len(plan.Actions) != 1 || plan.NextAction == nil || plan.NextAction.SuggestedPrompt != "root suggestion" {
		t.Fatalf("unexpected envelope plan: %#v", plan)
	}
}

func TestParseResponseEmptyAndMissingJSONErrors(t *testing.T) {
	if _, err := ParseResponse(`{}`, nil); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty response error, got %v", err)
	}
	if _, err := ParseResponse(`no json here`, nil); err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("expected missing json error, got %v", err)
	}
}

func TestPrimarySuggestedPromptAndNormalizeExistingNextAction(t *testing.T) {
	plan := &Response{SuggestedPrompts: []string{"", "from list"}, NextAction: &NextAction{Type: "wait_user"}}
	NormalizeSuggestedPrompt(plan)
	if got := PrimarySuggestedPrompt(plan); got != "from list" {
		t.Fatalf("primary prompt = %q", got)
	}
	if plan.NextAction.SuggestedPrompt != "from list" || plan.NextAction.Type != "wait_user" {
		t.Fatalf("existing next action not updated: %#v", plan.NextAction)
	}
}

func TestExtractBalancedJSONObjectsAndOpenAIContentArrays(t *testing.T) {
	objects := extractBalancedJSONObjects(`ignore {"answer":"first { kept }"} then {"answer":"second"}`)
	if len(objects) != 2 || !strings.Contains(objects[0], "second") || !strings.Contains(objects[1], "first") {
		t.Fatalf("unexpected balanced objects: %#v", objects)
	}
	text := openAIEnvelopeContentText([]any{
		map[string]any{"text": "hello"},
		"world",
		map[string]any{"content": "again"},
	})
	if text != "hello\nworld\nagain" {
		t.Fatalf("content array text = %q", text)
	}
	merged := mergeSuggestedPromptContent(`{"answer":"ok"}`, "next")
	if !strings.Contains(merged, `"suggest_prompt":"next"`) {
		t.Fatalf("merged content = %s", merged)
	}
}
