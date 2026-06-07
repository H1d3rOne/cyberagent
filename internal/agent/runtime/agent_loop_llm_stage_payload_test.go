package runtime

import (
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestAgentLoopLLMStagePayloadCloneIsolatesMutableFields(t *testing.T) {
	lastMessageAt := time.Date(2026, 6, 7, 14, 0, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	payload := AgentLoopLLMStagePayload{
		Turn: TurnContext{
			SessionID:      "session-stage-payload",
			TaskID:         "task-stage-payload",
			RunID:          "run-stage-payload",
			TurnID:         "turn-stage-payload",
			Prompt:         "summarize tools",
			Origin:         "user",
			OperationLevel: "safe_scan",
			Metadata:       map[string]any{"stage": "tool_synthesis"},
		},
		Language:   "zh-CN",
		LLMRequest: AgentLoopLLMRequest{Model: "gpt-stage", AuthProfiles: []AgentLoopAuthProfile{{ID: "auth-stage", Token: "secret"}}},
		History: []AgentLoopMessage{{
			ID:      "msg-stage-history",
			Role:    "user",
			Content: "history",
		}},
		MemorySummary: &AgentLoopMemorySummary{
			Summary:       "memory",
			MessageCount:  2,
			LastMessageID: "msg-stage-history",
			LastMessageAt: &lastMessageAt,
		},
		ToolMessages: []AgentLoopMessage{{
			ID:      "msg-stage-tool",
			Role:    "tool",
			Content: "observation",
			ToolCalls: []AgentLoopToolCall{{
				ID:       "tool-stage-tool",
				Name:     "daemon.health",
				Status:   contracts.StatusSucceeded,
				Result:   map[string]any{"ok": true},
				Metadata: map[string]any{"phase": "step"},
			}},
		}},
	}

	cloned := payload.Clone()
	payload.Turn.Metadata["stage"] = "mutated"
	payload.LLMRequest.AuthProfiles[0].Token = "mutated"
	payload.History[0].Content = "mutated history"
	payload.MemorySummary.Summary = "mutated memory"
	*payload.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)
	payload.ToolMessages[0].Content = "mutated tool"
	payload.ToolMessages[0].ToolCalls[0].Result["ok"] = false
	payload.ToolMessages[0].ToolCalls[0].Metadata["phase"] = "mutated"

	if cloned.Turn.Metadata["stage"] != "tool_synthesis" || cloned.LLMRequest.AuthProfiles[0].Token != "secret" {
		t.Fatalf("clone shares turn/request fields: %#v", cloned)
	}
	if cloned.History[0].Content != "history" {
		t.Fatalf("clone shares history fields: %#v", cloned.History[0])
	}
	if cloned.MemorySummary == nil || cloned.MemorySummary.Summary != "memory" || cloned.MemorySummary.LastMessageAt == nil || !cloned.MemorySummary.LastMessageAt.Equal(originalLastMessageAt) || cloned.MemorySummary.LastMessageAt == payload.MemorySummary.LastMessageAt {
		t.Fatalf("clone shares memory summary: %#v", cloned.MemorySummary)
	}
	if cloned.ToolMessages[0].Content != "observation" || cloned.ToolMessages[0].ToolCalls[0].Result["ok"] != true || cloned.ToolMessages[0].ToolCalls[0].Metadata["phase"] != "step" {
		t.Fatalf("clone shares tool messages: %#v", cloned.ToolMessages[0])
	}
}

func TestAgentLoopLLMStageResultPayloadCloneIsolatesMutableFields(t *testing.T) {
	payload := AgentLoopLLMStageResultPayload{
		Messages: []AgentLoopMessage{{
			ID:      "msg-stage-result",
			Role:    "agent",
			Content: "summarized",
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-stage-result",
			Name:     "LLM synthesis",
			Result:   map[string]any{"planner": true},
			Metadata: map[string]any{"phase": "tool_synthesis"},
		}},
		Plan: &AgentLoopPlan{
			Phase:            "step_replan",
			Answer:           "summarized",
			SuggestedPrompts: []string{"continue"},
			Actions:          []AgentLoopAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "clone"}}},
		},
	}

	cloned := payload.Clone()
	payload.Messages[0].Content = "mutated"
	payload.ToolCalls[0].Result["planner"] = false
	payload.ToolCalls[0].Metadata["phase"] = "mutated"
	payload.Plan.Answer = "mutated"
	payload.Plan.SuggestedPrompts[0] = "mutated"
	payload.Plan.Actions[0].Arguments["probe"] = "mutated"

	if cloned.Messages[0].Content != "summarized" || cloned.ToolCalls[0].Result["planner"] != true || cloned.ToolCalls[0].Metadata["phase"] != "tool_synthesis" {
		t.Fatalf("clone shares message/tool fields: %#v", cloned)
	}
	if cloned.Plan == nil || cloned.Plan.Answer != "summarized" || cloned.Plan.SuggestedPrompts[0] != "continue" || cloned.Plan.Actions[0].Arguments["probe"] != "clone" || cloned.Plan == payload.Plan {
		t.Fatalf("clone shares plan fields: %#v", cloned.Plan)
	}
}
