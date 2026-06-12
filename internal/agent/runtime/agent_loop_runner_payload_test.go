package runtime

import (
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestAgentLoopRequestCloneIsolatesMutableFields(t *testing.T) {
	llmEnabled := true
	authEnabled := true
	lastMessageAt := time.Date(2026, 6, 7, 15, 0, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	finishedAt := time.Date(2026, 6, 7, 15, 1, 0, 0, time.UTC)
	originalFinishedAt := finishedAt
	handle := &struct{ Name string }{Name: "daemon-services"}
	turnPayload := &struct{ Kind string }{Kind: "opaque"}

	req := AgentLoopRequest{
		Turn: TurnContext{
			SessionID:      "session-loop-request",
			TaskID:         "task-loop-request",
			RunID:          "run-loop-request",
			TurnID:         "turn-loop-request",
			Prompt:         "inspect target",
			Origin:         "user",
			OperationLevel: "safe_scan",
			Payload:        turnPayload,
			Metadata:       map[string]any{"runner": "agent_loop"},
		},
		Mode:     AgentLoopModeLLMToolLoop,
		Language: "zh-CN",
		Operator: "operator-1",
		LLMRequest: AgentLoopLLMRequest{
			Origin:               "user",
			OperationLevel:       "safe_scan",
			Provider:             "openai",
			Model:                "gpt-loop",
			APIKey:               "secret-api-key",
			BaseURL:              "https://sandbox.example",
			Instructions:         "stay bounded",
			DefaultAuthProfileID: "llm-auth",
			AuthProfiles: []AgentLoopAuthProfile{{
				ID:      "llm-auth",
				Token:   "llm-token",
				Enabled: &llmEnabled,
			}},
		},
		History: []AgentLoopMessage{{
			ID:      "msg-loop-history",
			Role:    "user",
			Content: "history",
			ToolCalls: []AgentLoopToolCall{{
				ID:         "tool-loop-history",
				Name:       "history.tool",
				Status:     contracts.StatusSucceeded,
				FinishedAt: &finishedAt,
				Result:     map[string]any{"source": "history"},
				Error:      &AgentLoopToolCallError{Code: services.CodeBadRequest, Message: "warning"},
				Metadata:   map[string]any{"phase": "history"},
			}},
			SuggestedPrompts: []string{"continue"},
		}},
		MemorySummary: &AgentLoopMemorySummary{
			Summary:       "memory",
			MessageCount:  7,
			LastMessageID: "msg-loop-history",
			LastMessageAt: &lastMessageAt,
		},
		InitialPlan: &AgentLoopPlan{
			Phase:            "step_replan",
			Answer:           "initial answer",
			SuggestedPrompts: []string{"next"},
			Actions:          []AgentLoopAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "initial"}}},
		},
		State: AgentLoopState{
			ResponseMessages: []AgentLoopMessage{{ID: "msg-state-response", Content: "state response"}},
			ToolCalls:        []AgentLoopToolCall{{ID: "tool-state", Result: map[string]any{"state": "tool"}}},
			LoopHistory:      []AgentLoopMessage{{ID: "msg-state-history", Content: "state history"}},
		},
		Action: AgentLoopAction{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "single"},
		},
		Authorization: AgentLoopAuthorization{
			DefaultAuthProfileID: "auth-1",
			AuthProfiles: []AgentLoopAuthProfile{{
				ID:      "auth-1",
				Token:   "auth-token",
				Enabled: &authEnabled,
			}},
		},
		ToolServices: AgentLoopToolServiceBinding{
			Kind:     "daemon_router_services",
			Handle:   handle,
			Metadata: map[string]any{"adapter": "daemon"},
		},
		MaxLoops: 3,
		Metadata: map[string]any{"request": "loop"},
	}

	cloned := req.Clone()
	req.Turn.Metadata["runner"] = "mutated"
	req.LLMRequest.AuthProfiles[0].Token = "mutated-llm-token"
	*req.LLMRequest.AuthProfiles[0].Enabled = false
	req.History[0].Content = "mutated history"
	req.History[0].ToolCalls[0].Result["source"] = "mutated"
	req.History[0].ToolCalls[0].Error.Message = "mutated warning"
	req.History[0].ToolCalls[0].Metadata["phase"] = "mutated"
	*req.History[0].ToolCalls[0].FinishedAt = finishedAt.Add(time.Hour)
	req.History[0].SuggestedPrompts[0] = "mutated"
	req.MemorySummary.Summary = "mutated memory"
	*req.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)
	req.InitialPlan.Answer = "mutated answer"
	req.InitialPlan.SuggestedPrompts[0] = "mutated"
	req.InitialPlan.Actions[0].Arguments["probe"] = "mutated"
	req.State.ResponseMessages[0].Content = "mutated state response"
	req.State.ToolCalls[0].Result["state"] = "mutated"
	req.State.LoopHistory[0].Content = "mutated state history"
	req.Action.Arguments["probe"] = "mutated"
	req.Authorization.AuthProfiles[0].Token = "mutated-auth-token"
	*req.Authorization.AuthProfiles[0].Enabled = false
	req.ToolServices.Metadata["adapter"] = "mutated"
	handle.Name = "mutated-services"
	req.Metadata["request"] = "mutated"

	if cloned.Mode != AgentLoopModeLLMToolLoop || cloned.Language != "zh-CN" || cloned.Operator != "operator-1" || cloned.MaxLoops != 3 {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
	if cloned.Turn.SessionID != "session-loop-request" || cloned.Turn.Prompt != "inspect target" || cloned.Turn.Payload != turnPayload || cloned.Turn.Metadata["runner"] != "agent_loop" {
		t.Fatalf("clone changed or shared turn context: %#v", cloned.Turn)
	}
	if cloned.LLMRequest.AuthProfiles[0].Token != "llm-token" || cloned.LLMRequest.AuthProfiles[0].Enabled == nil || !*cloned.LLMRequest.AuthProfiles[0].Enabled || cloned.LLMRequest.AuthProfiles[0].Enabled == req.LLMRequest.AuthProfiles[0].Enabled {
		t.Fatalf("clone shares llm auth profile: %#v", cloned.LLMRequest.AuthProfiles[0])
	}
	if cloned.History[0].Content != "history" || cloned.History[0].ToolCalls[0].Result["source"] != "history" || cloned.History[0].ToolCalls[0].Metadata["phase"] != "history" || cloned.History[0].SuggestedPrompts[0] != "continue" {
		t.Fatalf("clone shares history fields: %#v", cloned.History[0])
	}
	if cloned.History[0].ToolCalls[0].Error == nil || cloned.History[0].ToolCalls[0].Error.Message != "warning" || cloned.History[0].ToolCalls[0].Error == req.History[0].ToolCalls[0].Error {
		t.Fatalf("clone shares history tool error: %#v", cloned.History[0].ToolCalls[0].Error)
	}
	if cloned.History[0].ToolCalls[0].FinishedAt == nil || !cloned.History[0].ToolCalls[0].FinishedAt.Equal(originalFinishedAt) || cloned.History[0].ToolCalls[0].FinishedAt == req.History[0].ToolCalls[0].FinishedAt {
		t.Fatalf("clone shares history FinishedAt: %#v", cloned.History[0].ToolCalls[0].FinishedAt)
	}
	if cloned.MemorySummary == nil || cloned.MemorySummary.Summary != "memory" || cloned.MemorySummary.LastMessageAt == nil || !cloned.MemorySummary.LastMessageAt.Equal(originalLastMessageAt) || cloned.MemorySummary.LastMessageAt == req.MemorySummary.LastMessageAt {
		t.Fatalf("clone shares memory summary: %#v", cloned.MemorySummary)
	}
	if cloned.InitialPlan == nil || cloned.InitialPlan.Answer != "initial answer" || cloned.InitialPlan.SuggestedPrompts[0] != "next" || cloned.InitialPlan.Actions[0].Arguments["probe"] != "initial" || cloned.InitialPlan == req.InitialPlan {
		t.Fatalf("clone shares initial plan: %#v", cloned.InitialPlan)
	}
	if cloned.State.ResponseMessages[0].Content != "state response" || cloned.State.ToolCalls[0].Result["state"] != "tool" || cloned.State.LoopHistory[0].Content != "state history" {
		t.Fatalf("clone shares state: %#v", cloned.State)
	}
	if cloned.Action.Name != "daemon.health" || cloned.Action.Arguments["probe"] != "single" {
		t.Fatalf("clone shares action: %#v", cloned.Action)
	}
	if cloned.Authorization.AuthProfiles[0].Token != "auth-token" || cloned.Authorization.AuthProfiles[0].Enabled == nil || !*cloned.Authorization.AuthProfiles[0].Enabled || cloned.Authorization.AuthProfiles[0].Enabled == req.Authorization.AuthProfiles[0].Enabled {
		t.Fatalf("clone shares authorization profile: %#v", cloned.Authorization.AuthProfiles[0])
	}
	if cloned.ToolServices.Kind != "daemon_router_services" || cloned.ToolServices.Handle != handle || cloned.ToolServices.Metadata["adapter"] != "daemon" {
		t.Fatalf("clone changed service binding or shared metadata: %#v", cloned.ToolServices)
	}
	if cloned.ToolServices.Handle.(*struct{ Name string }).Name != "mutated-services" {
		t.Fatalf("clone should preserve service handle reference: %#v", cloned.ToolServices.Handle)
	}
	if cloned.Metadata["request"] != "loop" {
		t.Fatalf("clone shares metadata: %#v", cloned.Metadata)
	}
}

func TestAgentLoopResultCloneIsolatesMutableFields(t *testing.T) {
	finishedAt := time.Date(2026, 6, 7, 16, 0, 0, 0, time.UTC)
	originalFinishedAt := finishedAt
	result := AgentLoopResult{
		State: AgentLoopState{
			ResponseMessages: []AgentLoopMessage{{ID: "msg-result-state-response", Content: "state response"}},
			ToolCalls:        []AgentLoopToolCall{{ID: "tool-result-state", Result: map[string]any{"state": "tool"}}},
			LoopHistory:      []AgentLoopMessage{{ID: "msg-result-state-history", Content: "state history"}},
		},
		Messages: []AgentLoopMessage{{
			ID:      "msg-result",
			Role:    "agent",
			Content: "done",
			ToolCalls: []AgentLoopToolCall{{
				ID:         "tool-result-message",
				Name:       "message.tool",
				FinishedAt: &finishedAt,
				Result:     map[string]any{"message": true},
			}},
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-result-top-level",
			Name:     "top.level",
			Result:   map[string]any{"top": true},
			Metadata: map[string]any{"phase": "planner_loop"},
		}},
		FinalPlan: &AgentLoopPlan{
			Phase:   "step_replan",
			Answer:  "final answer",
			Actions: []AgentLoopAction{{Name: "reports.draft", Arguments: map[string]any{"format": "markdown"}}},
		},
		StopReason:            "completed",
		AuthorizationRequired: true,
		CapabilityBlocked:     true,
		LoopCount:             2,
		ActionQueueIDs:        []string{"queue-1", "queue-2"},
		Metadata:              map[string]any{"result": "loop"},
	}

	cloned := result.Clone()
	result.State.ResponseMessages[0].Content = "mutated state response"
	result.State.ToolCalls[0].Result["state"] = "mutated"
	result.State.LoopHistory[0].Content = "mutated state history"
	result.Messages[0].Content = "mutated message"
	result.Messages[0].ToolCalls[0].Result["message"] = false
	*result.Messages[0].ToolCalls[0].FinishedAt = finishedAt.Add(time.Hour)
	result.ToolCalls[0].Result["top"] = false
	result.ToolCalls[0].Metadata["phase"] = "mutated"
	result.FinalPlan.Answer = "mutated answer"
	result.FinalPlan.Actions[0].Arguments["format"] = "json"
	result.ActionQueueIDs[0] = "mutated-queue"
	result.Metadata["result"] = "mutated"

	if cloned.StopReason != "completed" || !cloned.AuthorizationRequired || !cloned.CapabilityBlocked || cloned.LoopCount != 2 {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
	if cloned.State.ResponseMessages[0].Content != "state response" || cloned.State.ToolCalls[0].Result["state"] != "tool" || cloned.State.LoopHistory[0].Content != "state history" {
		t.Fatalf("clone shares state fields: %#v", cloned.State)
	}
	if cloned.Messages[0].Content != "done" || cloned.Messages[0].ToolCalls[0].Result["message"] != true {
		t.Fatalf("clone shares message fields: %#v", cloned.Messages[0])
	}
	if cloned.Messages[0].ToolCalls[0].FinishedAt == nil || !cloned.Messages[0].ToolCalls[0].FinishedAt.Equal(originalFinishedAt) || cloned.Messages[0].ToolCalls[0].FinishedAt == result.Messages[0].ToolCalls[0].FinishedAt {
		t.Fatalf("clone shares message FinishedAt: %#v", cloned.Messages[0].ToolCalls[0].FinishedAt)
	}
	if cloned.ToolCalls[0].Result["top"] != true || cloned.ToolCalls[0].Metadata["phase"] != "planner_loop" {
		t.Fatalf("clone shares top-level tool fields: %#v", cloned.ToolCalls[0])
	}
	if cloned.FinalPlan == nil || cloned.FinalPlan.Answer != "final answer" || cloned.FinalPlan.Actions[0].Arguments["format"] != "markdown" || cloned.FinalPlan == result.FinalPlan {
		t.Fatalf("clone shares final plan: %#v", cloned.FinalPlan)
	}
	if cloned.ActionQueueIDs[0] != "queue-1" || cloned.ActionQueueIDs[1] != "queue-2" {
		t.Fatalf("clone shares action queue ids: %#v", cloned.ActionQueueIDs)
	}
	if cloned.Metadata["result"] != "loop" {
		t.Fatalf("clone shares metadata: %#v", cloned.Metadata)
	}
}
