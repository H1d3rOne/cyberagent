package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent"
	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

type scenarioProvider struct {
	target    string
	callCount int
}

func (p *scenarioProvider) Name() string { return "scenario" }

func (p *scenarioProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	p.callCount++

	switch p.callCount {
	case 1:
		return &llm.ChatResponse{
			Content: []llm.ContentBlock{
				llm.TextBlock{Text: "I'll start by probing the target."},
				llm.ToolUseBlock{
					ID:    "tu_1",
					Name:  "http_probe",
					Input: json.RawMessage(fmt.Sprintf(`{"url":"%s"}`, p.target)),
				},
			},
			StopReason: llm.StopReasonToolUse,
		}, nil
	case 2:
		return &llm.ChatResponse{
			Content: []llm.ContentBlock{
				llm.TextBlock{Text: "Now checking security headers."},
				llm.ToolUseBlock{
					ID:    "tu_2",
					Name:  "check_headers",
					Input: json.RawMessage(fmt.Sprintf(`{"url":"%s"}`, p.target)),
				},
			},
			StopReason: llm.StopReasonToolUse,
		}, nil
	case 3:
		return &llm.ChatResponse{
			Content: []llm.ContentBlock{
				llm.ToolUseBlock{
					ID:   "tu_3",
					Name: "add_finding",
					Input: json.RawMessage(`{
						"title": "Missing HSTS Header",
						"severity": "medium",
						"description": "The target does not set Strict-Transport-Security header",
						"evidence": "Response headers lack HSTS"
					}`),
				},
			},
			StopReason: llm.StopReasonToolUse,
		}, nil
	default:
		return &llm.ChatResponse{
			Content:    []llm.ContentBlock{llm.TextBlock{Text: "Assessment complete. Found 1 medium severity issue."}},
			StopReason: llm.StopReasonEndTurn,
		}, nil
	}
}

func TestIntegrationFullScan(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(200)
		w.Write([]byte(`<html><head><title>Test</title></head><body><h1>Hello</h1></body></html>`))
	}))
	defer ts.Close()

	provider := &scenarioProvider{target: ts.URL}

	a := agent.New(agent.Config{
		Provider:  provider,
		Target:    ts.URL,
		Scope:     []string{ts.URL},
		MaxTurns:  20,
		NoConfirm: true,
	})

	result, err := a.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if result.Turns != 4 {
		t.Errorf("expected 4 turns, got %d", result.Turns)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Title != "Missing HSTS Header" {
		t.Errorf("unexpected finding title: %s", result.Findings[0].Title)
	}
	if result.Summary != "Assessment complete. Found 1 medium severity issue." {
		t.Errorf("unexpected summary: %s", result.Summary)
	}
}

func TestIntegrationScopeBlocks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	outOfScope := "http://evil.com/hack"
	provider := &scenarioProvider{target: outOfScope}

	a := agent.New(agent.Config{
		Provider:  provider,
		Target:    ts.URL,
		Scope:     []string{ts.URL},
		MaxTurns:  10,
		NoConfirm: true,
	})

	result, err := a.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if provider.callCount < 2 {
		t.Error("expected at least 2 LLM calls")
	}
	_ = result
}
