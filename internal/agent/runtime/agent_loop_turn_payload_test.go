package runtime

import (
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestAgentLoopTurnPayloadCloneIsolatesMutableFields(t *testing.T) {
	enabled := true
	lastMessageAt := time.Date(2026, 6, 7, 11, 10, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	finishedAt := time.Date(2026, 6, 7, 11, 11, 0, 0, time.UTC)
	originalFinishedAt := finishedAt
	payload := AgentLoopTurnPayload{
		Turn: TurnContext{
			SessionID:      "session-turn-payload",
			TaskID:         "task-turn-payload",
			RunID:          "run-turn-payload",
			TurnID:         "turn-turn-payload",
			Prompt:         "inspect safely",
			Origin:         "user",
			OperationLevel: "safe_scan",
			Metadata:       map[string]any{"runner": "llm_tool_loop"},
		},
		Language: "zh-CN",
		Operator: "operator-1",
		LLMRequest: AgentLoopLLMRequest{
			Origin:         "user",
			OperationLevel: "safe_scan",
			Provider:       "openai",
			Model:          "gpt-turn",
			AuthProfiles: []AgentLoopAuthProfile{{
				ID:      "auth-turn",
				Token:   "secret-token",
				Enabled: &enabled,
			}},
		},
		History: []AgentLoopMessage{{
			ID:      "msg-turn-history",
			Role:    "user",
			Content: "history",
			ToolCalls: []AgentLoopToolCall{{
				ID:         "tool-turn-history",
				Name:       "history.tool",
				Status:     contracts.StatusSucceeded,
				FinishedAt: &finishedAt,
				Result:     map[string]any{"source": "history"},
				Error:      &AgentLoopToolCallError{Code: services.CodeBadRequest, Message: "warning"},
				Metadata:   map[string]any{"phase": "plan"},
			}},
			SuggestedPrompts: []string{"continue"},
		}},
		MemorySummary: &AgentLoopMemorySummary{
			Summary:       "memory",
			MessageCount:  3,
			LastMessageID: "msg-turn-history",
			LastMessageAt: &lastMessageAt,
		},
	}

	cloned := payload.Clone()
	payload.Turn.Metadata["runner"] = "mutated"
	payload.LLMRequest.AuthProfiles[0].Token = "mutated-token"
	*payload.LLMRequest.AuthProfiles[0].Enabled = false
	payload.History[0].Content = "mutated history"
	payload.History[0].ToolCalls[0].Result["source"] = "mutated"
	payload.History[0].ToolCalls[0].Error.Message = "mutated warning"
	*payload.History[0].ToolCalls[0].FinishedAt = finishedAt.Add(time.Hour)
	payload.History[0].SuggestedPrompts[0] = "mutated"
	payload.MemorySummary.Summary = "mutated memory"
	*payload.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)

	if cloned.Turn.SessionID != "session-turn-payload" || cloned.Turn.Prompt != "inspect safely" || cloned.Turn.Metadata["runner"] != "llm_tool_loop" {
		t.Fatalf("clone changed or shared turn context: %#v", cloned.Turn)
	}
	if cloned.LLMRequest.AuthProfiles[0].Token != "secret-token" || cloned.LLMRequest.AuthProfiles[0].Enabled == nil || !*cloned.LLMRequest.AuthProfiles[0].Enabled || cloned.LLMRequest.AuthProfiles[0].Enabled == payload.LLMRequest.AuthProfiles[0].Enabled {
		t.Fatalf("clone shares llm auth profile: %#v", cloned.LLMRequest.AuthProfiles[0])
	}
	if cloned.History[0].Content != "history" || cloned.History[0].ToolCalls[0].Result["source"] != "history" || cloned.History[0].SuggestedPrompts[0] != "continue" {
		t.Fatalf("clone shares history fields: %#v", cloned.History[0])
	}
	if cloned.History[0].ToolCalls[0].Error == nil || cloned.History[0].ToolCalls[0].Error.Message != "warning" || cloned.History[0].ToolCalls[0].Error == payload.History[0].ToolCalls[0].Error {
		t.Fatalf("clone shares history tool error: %#v", cloned.History[0].ToolCalls[0].Error)
	}
	if cloned.History[0].ToolCalls[0].FinishedAt == nil || !cloned.History[0].ToolCalls[0].FinishedAt.Equal(originalFinishedAt) || cloned.History[0].ToolCalls[0].FinishedAt == payload.History[0].ToolCalls[0].FinishedAt {
		t.Fatalf("clone shares history FinishedAt: %#v", cloned.History[0].ToolCalls[0].FinishedAt)
	}
	if cloned.MemorySummary == nil || cloned.MemorySummary.Summary != "memory" || cloned.MemorySummary.LastMessageAt == nil || !cloned.MemorySummary.LastMessageAt.Equal(originalLastMessageAt) || cloned.MemorySummary.LastMessageAt == payload.MemorySummary.LastMessageAt {
		t.Fatalf("clone shares memory summary: %#v", cloned.MemorySummary)
	}
}

func TestAgentLoopTurnResultPayloadCloneIsolatesMutableFields(t *testing.T) {
	finishedAt := time.Date(2026, 6, 7, 11, 30, 0, 0, time.UTC)
	originalFinishedAt := finishedAt
	payload := AgentLoopTurnResultPayload{
		Messages: []AgentLoopMessage{{
			ID:      "msg-turn-result",
			Role:    "agent",
			Content: "done",
			ToolCalls: []AgentLoopToolCall{{
				ID:         "tool-turn-result-message",
				Name:       "message.tool",
				FinishedAt: &finishedAt,
				Result:     map[string]any{"ok": true},
			}},
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-turn-result-top-level",
			Name:     "top.level",
			Result:   map[string]any{"top": true},
			Metadata: map[string]any{"phase": "step"},
		}},
	}

	cloned := payload.Clone()
	payload.Messages[0].Content = "mutated"
	payload.Messages[0].ToolCalls[0].Result["ok"] = false
	*payload.Messages[0].ToolCalls[0].FinishedAt = finishedAt.Add(time.Hour)
	payload.ToolCalls[0].Result["top"] = false
	payload.ToolCalls[0].Metadata["phase"] = "mutated"

	if cloned.Messages[0].Content != "done" || cloned.Messages[0].ToolCalls[0].Result["ok"] != true {
		t.Fatalf("clone shares message result fields: %#v", cloned.Messages[0])
	}
	if cloned.Messages[0].ToolCalls[0].FinishedAt == nil || !cloned.Messages[0].ToolCalls[0].FinishedAt.Equal(originalFinishedAt) || cloned.Messages[0].ToolCalls[0].FinishedAt == payload.Messages[0].ToolCalls[0].FinishedAt {
		t.Fatalf("clone shares message FinishedAt: %#v", cloned.Messages[0].ToolCalls[0].FinishedAt)
	}
	if cloned.ToolCalls[0].Result["top"] != true || cloned.ToolCalls[0].Metadata["phase"] != "step" {
		t.Fatalf("clone shares top-level tool fields: %#v", cloned.ToolCalls[0])
	}
}
