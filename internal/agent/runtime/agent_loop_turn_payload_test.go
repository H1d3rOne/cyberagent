package runtime

import (
	"testing"
	"time"
)

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
