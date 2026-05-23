package agent

import (
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

func TestCompressHistoryNoOp(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "hi"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
	}
	result := compressHistory(history)
	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
	}
}

func TestCompressHistoryTruncatesOldToolResults(t *testing.T) {
	history := make([]llm.Message, 0, 80)
	for i := 0; i < 80; i++ {
		if i%2 == 0 {
			history = append(history, llm.Message{
				Role:    llm.RoleAssistant,
				Content: []llm.ContentBlock{llm.ToolUseBlock{ID: "tu", Name: "test", Input: []byte(`{}`)}},
			})
		} else {
			bigContent := strings.Repeat("x", 10000)
			history = append(history, llm.Message{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{llm.ToolResultBlock{ToolUseID: "tu", Content: bigContent}},
			})
		}
	}

	result := compressHistory(history)
	for i, msg := range result {
		if i >= len(result)-10 {
			break
		}
		for _, b := range msg.Content {
			if tr, ok := b.(llm.ToolResultBlock); ok {
				if len(tr.Content) > maxToolResultLength+100 {
					t.Errorf("message %d: tool result not truncated, len=%d", i, len(tr.Content))
				}
			}
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("a", 400)}}},
	}
	tokens := estimateTokens(history)
	if tokens != 100 {
		t.Errorf("expected ~100 tokens for 400 chars, got %d", tokens)
	}
}
