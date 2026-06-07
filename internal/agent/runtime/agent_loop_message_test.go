package runtime

import (
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestAgentLoopMessageCloneIsolatesMutableFields(t *testing.T) {
	timestamp := time.Date(2026, 6, 7, 11, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 6, 7, 11, 1, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 7, 11, 2, 0, 0, time.UTC)
	originalFinishedAt := finishedAt
	message := AgentLoopMessage{
		ID:        "msg-runtime-cycle",
		Role:      "tool",
		Content:   "tool result",
		Timestamp: timestamp,
		Phase:     "step",
		ToolCalls: []AgentLoopToolCall{{
			ID:         "tool-runtime-message",
			Name:       "daemon.health",
			Status:     contracts.StatusSucceeded,
			StartedAt:  startedAt,
			FinishedAt: &finishedAt,
			Result:     map[string]any{"source": "message"},
			Error:      &AgentLoopToolCallError{Code: services.CodeBadRequest, Message: "warning"},
			Metadata:   map[string]any{"phase": "tool"},
		}},
		Links:            []AgentLoopLink{{To: "/runs/run-1", Label: "Run", Kind: "run"}},
		Reasoning:        "reasoning",
		SuggestedPrompts: []string{"continue"},
		RecommendationOptions: []AgentLoopRecommendationOption{{
			ID:      "rec-1",
			Title:   "Continue",
			Detail:  "detail",
			Prompt:  "continue",
			Command: "/continue",
		}},
	}

	cloned := message.Clone()
	*message.ToolCalls[0].FinishedAt = time.Date(2026, 6, 7, 11, 3, 0, 0, time.UTC)
	message.ToolCalls[0].Result["source"] = "mutated"
	message.ToolCalls[0].Error.Message = "mutated warning"
	message.ToolCalls[0].Metadata["phase"] = "mutated"
	message.Links[0].Label = "Mutated"
	message.SuggestedPrompts[0] = "mutated"
	message.RecommendationOptions[0].Title = "Mutated"

	if cloned.ID != "msg-runtime-cycle" || cloned.Role != "tool" || cloned.Content != "tool result" || !cloned.Timestamp.Equal(timestamp) || cloned.Phase != "step" || cloned.Reasoning != "reasoning" {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
	if cloned.ToolCalls[0].FinishedAt == nil || !cloned.ToolCalls[0].FinishedAt.Equal(originalFinishedAt) || cloned.ToolCalls[0].FinishedAt == message.ToolCalls[0].FinishedAt {
		t.Fatalf("clone shares or lost tool FinishedAt: %#v", cloned.ToolCalls[0].FinishedAt)
	}
	if cloned.ToolCalls[0].Result["source"] != "message" || cloned.ToolCalls[0].Metadata["phase"] != "tool" {
		t.Fatalf("clone shares tool maps: %#v", cloned.ToolCalls[0])
	}
	if cloned.ToolCalls[0].Error == nil || cloned.ToolCalls[0].Error.Message != "warning" || cloned.ToolCalls[0].Error == message.ToolCalls[0].Error {
		t.Fatalf("clone shares or lost tool error: %#v", cloned.ToolCalls[0].Error)
	}
	if cloned.Links[0].Label != "Run" || cloned.SuggestedPrompts[0] != "continue" || cloned.RecommendationOptions[0].Title != "Continue" {
		t.Fatalf("clone shares message slices: %#v", cloned)
	}
}
