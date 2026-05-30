package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

type mockProvider struct {
	responses []*llm.ChatResponse
	callIdx   int
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if isPlanRequest(req) {
		return planResponse(`{"goal":"assess","todos":[{"title":"assess","stage":"analysis"}]}`), nil
	}
	if m.callIdx >= len(m.responses) {
		return &llm.ChatResponse{
			Content:    []llm.ContentBlock{llm.TextBlock{Text: "Done."}},
			StopReason: llm.StopReasonEndTurn,
		}, nil
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

func isPlanRequest(req llm.ChatRequest) bool {
	for _, msg := range req.Messages {
		for _, b := range msg.Content {
			if tb, ok := b.(llm.TextBlock); ok && strings.Contains(tb.Text, "Output ONLY a JSON object") {
				return true
			}
		}
	}
	return false
}

func planResponse(jsonText string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Content:    []llm.ContentBlock{llm.TextBlock{Text: jsonText}},
		StopReason: llm.StopReasonEndTurn,
	}
}

func TestAgentRunTextOnly(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.ChatResponse{
			{Content: []llm.ContentBlock{llm.TextBlock{Text: "No issues found."}}, StopReason: llm.StopReasonEndTurn},
		},
	}

	agent := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		Scope:    []string{"example.com"},
	})

	result, err := agent.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", result.Turns)
	}
	if result.Summary != "No issues found." {
		t.Errorf("unexpected summary: %s", result.Summary)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestAgentRunWithToolUse(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.ChatResponse{
			{
				Content: []llm.ContentBlock{llm.ToolUseBlock{
					ID:    "tu_01",
					Name:  "add_finding",
					Input: json.RawMessage(`{"title":"Missing HSTS","severity":"medium","description":"No HSTS header","evidence":"Header not present"}`),
				}},
				StopReason: llm.StopReasonToolUse,
			},
			{
				Content:    []llm.ContentBlock{llm.TextBlock{Text: "Assessment complete."}},
				StopReason: llm.StopReasonEndTurn,
			},
		},
	}

	var events []Event
	agent := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		Scope:    []string{"example.com"},
		OnEvent:  func(e Event) { events = append(events, e) },
	})

	result, err := agent.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Turns != 2 {
		t.Errorf("expected 2 turns, got %d", result.Turns)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Title != "Missing HSTS" {
		t.Errorf("unexpected finding title: %s", result.Findings[0].Title)
	}
	if result.Findings[0].Severity != "medium" {
		t.Errorf("unexpected severity: %s", result.Findings[0].Severity)
	}

	hasToolCall := false
	hasFinding := false
	for _, e := range events {
		if e.Type == EventToolCall {
			hasToolCall = true
		}
		if e.Type == EventFinding {
			hasFinding = true
		}
	}
	if !hasToolCall {
		t.Error("expected tool_call event")
	}
	if !hasFinding {
		t.Error("expected finding event")
	}
}

func TestAgentMaxTurns(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.ChatResponse{
			{Content: []llm.ContentBlock{llm.ToolUseBlock{ID: "tu_01", Name: "unknown_tool", Input: json.RawMessage(`{}`)}}, StopReason: llm.StopReasonToolUse},
			{Content: []llm.ContentBlock{llm.ToolUseBlock{ID: "tu_02", Name: "unknown_tool", Input: json.RawMessage(`{}`)}}, StopReason: llm.StopReasonToolUse},
			{Content: []llm.ContentBlock{llm.ToolUseBlock{ID: "tu_03", Name: "unknown_tool", Input: json.RawMessage(`{}`)}}, StopReason: llm.StopReasonToolUse},
		},
	}

	agent := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		MaxTurns: 3,
	})

	result, err := agent.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Turns != 3 {
		t.Errorf("expected 3 turns, got %d", result.Turns)
	}
	if result.Summary != "Assessment stopped: maximum turns reached." {
		t.Errorf("unexpected summary: %s", result.Summary)
	}
}

func TestAgentScopeGuardBlocks(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.ChatResponse{
			{
				Content: []llm.ContentBlock{llm.ToolUseBlock{
					ID:    "tu_01",
					Name:  "http_probe",
					Input: json.RawMessage(`{"url":"http://evil.com/admin"}`),
				}},
				StopReason: llm.StopReasonToolUse,
			},
			{
				Content:    []llm.ContentBlock{llm.TextBlock{Text: "Blocked by scope."}},
				StopReason: llm.StopReasonEndTurn,
			},
		},
	}

	var gotScopeError bool
	agent := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		Scope:    []string{"example.com"},
		OnEvent: func(e Event) {
			if e.Type == EventError && e.ToolName == "http_probe" {
				gotScopeError = true
			}
		},
	})

	result, err := agent.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !gotScopeError {
		t.Error("expected scope denial error event")
	}
	if result.Turns != 2 {
		t.Errorf("expected 2 turns, got %d", result.Turns)
	}
}

func TestAgentParallelToolCalls(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.ChatResponse{
			{
				Content: []llm.ContentBlock{
					llm.ToolUseBlock{ID: "tu_01", Name: "add_finding", Input: json.RawMessage(`{"title":"Issue A","severity":"low","description":"desc A","evidence":"ev A"}`)},
					llm.ToolUseBlock{ID: "tu_02", Name: "add_finding", Input: json.RawMessage(`{"title":"Issue B","severity":"high","description":"desc B","evidence":"ev B"}`)},
				},
				StopReason: llm.StopReasonToolUse,
			},
			{
				Content:    []llm.ContentBlock{llm.TextBlock{Text: "Done."}},
				StopReason: llm.StopReasonEndTurn,
			},
		},
	}

	agent := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		Scope:    []string{"example.com"},
	})

	result, err := agent.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result.Findings))
	}
}

type planProvider struct {
	planJSON string
	callIdx  int
	steps    []*llm.ChatResponse
}

func (p *planProvider) Name() string { return "plan-mock" }

func (p *planProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if isPlanRequest(req) {
		return planResponse(p.planJSON), nil
	}
	if p.callIdx >= len(p.steps) {
		return &llm.ChatResponse{Content: []llm.ContentBlock{llm.TextBlock{Text: "step done"}}, StopReason: llm.StopReasonEndTurn}, nil
	}
	resp := p.steps[p.callIdx]
	p.callIdx++
	return resp, nil
}

func TestAgentPlanCoverage(t *testing.T) {
	provider := &planProvider{
		planJSON: `{"goal":"assess","todos":[{"title":"recon","stage":"recon"},{"title":"analyze","stage":"analysis"}]}`,
		// Each todo's inner loop returns a tool-call then a text completion.
		steps: []*llm.ChatResponse{
			{Content: []llm.ContentBlock{llm.ToolUseBlock{ID: "tu_1", Name: "add_finding", Input: json.RawMessage(`{"title":"A","severity":"low","description":"d","evidence":"e"}`)}}, StopReason: llm.StopReasonToolUse},
			{Content: []llm.ContentBlock{llm.TextBlock{Text: "recon done"}}, StopReason: llm.StopReasonEndTurn},
		},
	}

	var planEvents []Event
	agent := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		Scope:    []string{"example.com"},
		MaxTurns: 20,
		OnEvent: func(e Event) {
			if e.Type == EventPlanUpdate {
				planEvents = append(planEvents, e)
			}
		},
	})

	result, err := agent.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(planEvents) == 0 {
		t.Fatal("expected plan_update events")
	}
	last := planEvents[len(planEvents)-1]
	if last.Plan == nil {
		t.Fatal("plan_update event missing plan")
	}
	done, total := last.Plan.Coverage()
	if total != 2 {
		t.Errorf("expected 2 todos, got %d", total)
	}
	if done != 2 {
		t.Errorf("expected full coverage 2/2, got %d/%d", done, total)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
}
