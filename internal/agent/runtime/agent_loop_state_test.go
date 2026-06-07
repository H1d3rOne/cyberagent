package runtime

import (
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestAgentLoopStateCloneIsolatesMutableFields(t *testing.T) {
	startedAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 7, 12, 1, 0, 0, time.UTC)
	originalFinishedAt := finishedAt
	state := AgentLoopState{
		ResponseMessages: []AgentLoopMessage{{
			ID:      "msg-runtime-state-response",
			Role:    "tool",
			Content: "response message",
			ToolCalls: []AgentLoopToolCall{{
				ID:         "tool-runtime-state-response",
				Name:       "response.tool",
				Status:     contracts.StatusSucceeded,
				StartedAt:  startedAt,
				FinishedAt: &finishedAt,
				Result:     map[string]any{"source": "response"},
				Error:      &AgentLoopToolCallError{Code: services.CodeBadRequest, Message: "response warning"},
				Metadata:   map[string]any{"phase": "response"},
			}},
			Links:                 []AgentLoopLink{{To: "/runs/run-1", Label: "Run", Kind: "run"}},
			SuggestedPrompts:      []string{"continue"},
			RecommendationOptions: []AgentLoopRecommendationOption{{ID: "rec-1", Title: "Continue"}},
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-runtime-state-top-level",
			Name:     "top.level",
			Result:   map[string]any{"source": "tool"},
			Error:    &AgentLoopToolCallError{Code: services.CodeInternal, Message: "top-level warning"},
			Metadata: map[string]any{"phase": "tool"},
		}},
		LoopHistory: []AgentLoopMessage{{
			ID:      "msg-runtime-state-history",
			Role:    "agent",
			Content: "history message",
			ToolCalls: []AgentLoopToolCall{{
				ID:       "tool-runtime-state-history",
				Name:     "history.tool",
				Result:   map[string]any{"source": "history"},
				Metadata: map[string]any{"phase": "history"},
			}},
		}},
	}

	cloned := state.Clone()
	state.ResponseMessages[0].Content = "mutated response"
	*state.ResponseMessages[0].ToolCalls[0].FinishedAt = time.Date(2026, 6, 7, 12, 2, 0, 0, time.UTC)
	state.ResponseMessages[0].ToolCalls[0].Result["source"] = "mutated response"
	state.ResponseMessages[0].ToolCalls[0].Error.Message = "mutated response warning"
	state.ResponseMessages[0].Links[0].Label = "Mutated"
	state.ResponseMessages[0].SuggestedPrompts[0] = "mutated"
	state.ResponseMessages[0].RecommendationOptions[0].Title = "Mutated"
	state.ToolCalls[0].Result["source"] = "mutated tool"
	state.ToolCalls[0].Error.Message = "mutated top-level warning"
	state.LoopHistory[0].Content = "mutated history"
	state.LoopHistory[0].ToolCalls[0].Metadata["phase"] = "mutated history"

	if cloned.ResponseMessages[0].Content != "response message" || cloned.ResponseMessages[0].ToolCalls[0].Result["source"] != "response" {
		t.Fatalf("clone shares response message fields: %#v", cloned.ResponseMessages[0])
	}
	if cloned.ResponseMessages[0].ToolCalls[0].FinishedAt == nil || !cloned.ResponseMessages[0].ToolCalls[0].FinishedAt.Equal(originalFinishedAt) || cloned.ResponseMessages[0].ToolCalls[0].FinishedAt == state.ResponseMessages[0].ToolCalls[0].FinishedAt {
		t.Fatalf("clone shares or lost nested FinishedAt: %#v", cloned.ResponseMessages[0].ToolCalls[0].FinishedAt)
	}
	if cloned.ResponseMessages[0].ToolCalls[0].Error == nil || cloned.ResponseMessages[0].ToolCalls[0].Error.Message != "response warning" || cloned.ResponseMessages[0].ToolCalls[0].Error == state.ResponseMessages[0].ToolCalls[0].Error {
		t.Fatalf("clone shares or lost nested error: %#v", cloned.ResponseMessages[0].ToolCalls[0].Error)
	}
	if cloned.ResponseMessages[0].Links[0].Label != "Run" || cloned.ResponseMessages[0].SuggestedPrompts[0] != "continue" || cloned.ResponseMessages[0].RecommendationOptions[0].Title != "Continue" {
		t.Fatalf("clone shares response message slices: %#v", cloned.ResponseMessages[0])
	}
	if cloned.ToolCalls[0].Result["source"] != "tool" || cloned.ToolCalls[0].Error.Message != "top-level warning" {
		t.Fatalf("clone shares top-level tool fields: %#v", cloned.ToolCalls[0])
	}
	if cloned.LoopHistory[0].Content != "history message" || cloned.LoopHistory[0].ToolCalls[0].Metadata["phase"] != "history" {
		t.Fatalf("clone shares loop history fields: %#v", cloned.LoopHistory[0])
	}
}
