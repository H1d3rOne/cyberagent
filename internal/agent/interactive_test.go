package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

func TestRunInteractiveExitCommand(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.ChatResponse{
			{Content: []llm.ContentBlock{llm.TextBlock{Text: "Ready to assist."}}, StopReason: llm.StopReasonEndTurn},
		},
	}

	a := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		Scope:    []string{"example.com"},
		MaxTurns: 10,
	})

	input := strings.NewReader("exit\n")
	var output bytes.Buffer

	result, err := a.RunInteractive(context.Background(), input, &output)
	if err != nil {
		t.Fatal(err)
	}
	if result.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", result.Turns)
	}
	if !strings.Contains(output.String(), "Ready to assist.") {
		t.Errorf("expected agent response in output, got: %s", output.String())
	}
}

func TestRunInteractiveMultiTurn(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.ChatResponse{
			{Content: []llm.ContentBlock{llm.TextBlock{Text: "Starting analysis."}}, StopReason: llm.StopReasonEndTurn},
			{Content: []llm.ContentBlock{llm.TextBlock{Text: "Headers look fine."}}, StopReason: llm.StopReasonEndTurn},
		},
	}

	a := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		Scope:    []string{"example.com"},
		MaxTurns: 10,
	})

	input := strings.NewReader("check headers\nquit\n")
	var output bytes.Buffer

	result, err := a.RunInteractive(context.Background(), input, &output)
	if err != nil {
		t.Fatal(err)
	}
	if result.Turns != 2 {
		t.Errorf("expected 2 turns, got %d", result.Turns)
	}
	if !strings.Contains(output.String(), "Starting analysis.") {
		t.Errorf("expected first response in output")
	}
	if !strings.Contains(output.String(), "Headers look fine.") {
		t.Errorf("expected second response in output")
	}
}

func TestRunInteractiveEmptyInput(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.ChatResponse{
			{Content: []llm.ContentBlock{llm.TextBlock{Text: "Hello."}}, StopReason: llm.StopReasonEndTurn},
		},
	}

	a := New(Config{
		Provider: provider,
		Target:   "http://example.com",
		Scope:    []string{"example.com"},
		MaxTurns: 10,
	})

	input := strings.NewReader("\n")
	var output bytes.Buffer

	result, err := a.RunInteractive(context.Background(), input, &output)
	if err != nil {
		t.Fatal(err)
	}
	if result.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", result.Turns)
	}
}
