package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

type orchestratorMockProvider struct {
	callCount int
}

func (p *orchestratorMockProvider) Name() string { return "mock" }

func (p *orchestratorMockProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	p.callCount++

	if p.callCount == 1 {
		return &llm.ChatResponse{
			Content: []llm.ContentBlock{
				llm.ToolUseBlock{
					ID:    "tu_1",
					Name:  "add_finding",
					Input: json.RawMessage(`{"title":"Open port 8080","severity":"info","description":"Port 8080 is open","evidence":"TCP connect succeeded"}`),
				},
			},
			StopReason: llm.StopReasonToolUse,
			Usage:      llm.Usage{InputTokens: 100, OutputTokens: 50},
		}, nil
	}

	return &llm.ChatResponse{
		Content:    []llm.ContentBlock{llm.TextBlock{Text: "Analysis complete."}},
		StopReason: llm.StopReasonEndTurn,
		Usage:      llm.Usage{InputTokens: 80, OutputTokens: 30},
	}, nil
}

func TestOrchestratorRun(t *testing.T) {
	provider := &orchestratorMockProvider{}

	orch := NewOrchestrator(OrchestratorConfig{
		Provider:  provider,
		Target:    "http://example.com",
		Scope:     []string{"example.com"},
		MaxTurns:  20,
		NoConfirm: true,
	})

	result, err := orch.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if result.Combined == nil {
		t.Fatal("expected combined result")
	}
	if result.Combined.Turns < 2 {
		t.Errorf("expected at least 2 total turns, got %d", result.Combined.Turns)
	}
	if result.ReconResult == nil || result.DeepResult == nil {
		t.Error("expected both recon and deep results")
	}
	if result.Combined.TokensIn == 0 {
		t.Error("expected non-zero token count")
	}
}

func TestOrchestratorDeduplicatesFindings(t *testing.T) {
	provider := &orchestratorMockProvider{}

	orch := NewOrchestrator(OrchestratorConfig{
		Provider:  provider,
		Target:    "http://example.com",
		Scope:     []string{"example.com"},
		MaxTurns:  20,
		NoConfirm: true,
	})

	result, err := orch.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	titles := map[string]int{}
	for _, f := range result.Combined.Findings {
		titles[f.Title]++
	}
	for title, count := range titles {
		if count > 1 {
			t.Errorf("duplicate finding: %s (count=%d)", title, count)
		}
	}
}
