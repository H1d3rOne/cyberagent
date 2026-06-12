package runtime

import (
	"errors"
	"testing"
)

func TestAgentLoopPlannerActionExecutionResultFromActionExecutorResultBuildsLoopResult(t *testing.T) {
	result := AgentLoopActionExecutorResult{
		Status: TurnResultFinished,
		Reason: "action handled",
		Messages: []AgentLoopMessage{{
			ID:      "msg-action",
			Role:    "tool",
			Content: "action ok",
			ToolCalls: []AgentLoopToolCall{{
				ID:     "tool-nested",
				Name:   "daemon.health",
				Result: map[string]any{"ok": true},
			}},
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-action",
			Name:     "daemon.health",
			Result:   map[string]any{"ok": true},
			Metadata: map[string]any{"source": "executor"},
		}},
		Metadata: map[string]any{"executor": "test_executor"},
	}

	loopResult := AgentLoopPlannerActionExecutionResultFromActionExecutorResult("daemon.health", result, nil)

	if loopResult.StopReason != "action handled" || loopResult.CapabilityBlocked {
		t.Fatalf("loop result status = stop %q blocked %v", loopResult.StopReason, loopResult.CapabilityBlocked)
	}
	if loopResult.Metadata["mode"] != string(AgentLoopModePlannerActionExecution) ||
		loopResult.Metadata[AgentLoopActionNameMetadataKey] != "daemon.health" ||
		loopResult.Metadata["message_count"] != 1 ||
		loopResult.Metadata["tool_call_count"] != 1 ||
		loopResult.Metadata["action_executor_status"] != string(TurnResultFinished) ||
		loopResult.Metadata["executor"] != "test_executor" {
		t.Fatalf("loop metadata = %#v", loopResult.Metadata)
	}
	if loopResult.Messages[0].Content != "action ok" ||
		loopResult.State.ResponseMessages[0].Content != "action ok" ||
		loopResult.ToolCalls[0].Result["ok"] != true ||
		loopResult.State.ToolCalls[0].Metadata["source"] != "executor" {
		t.Fatalf("loop result did not clone messages/tool calls: %#v", loopResult)
	}

	result.Messages[0].Content = "mutated"
	result.Messages[0].ToolCalls[0].Result["ok"] = false
	result.ToolCalls[0].Metadata["source"] = "mutated"
	result.Metadata["executor"] = "mutated"
	if loopResult.Messages[0].Content != "action ok" ||
		loopResult.State.ResponseMessages[0].ToolCalls[0].Result["ok"] != true ||
		loopResult.ToolCalls[0].Metadata["source"] != "executor" ||
		loopResult.Metadata["executor"] != "test_executor" {
		t.Fatalf("loop result shares mutable action result fields: %#v", loopResult)
	}
}

func TestAgentLoopPlannerActionExecutionResultFromActionExecutorResultPreservesErrorAndBlocked(t *testing.T) {
	err := errors.New("action denied")
	loopResult := AgentLoopPlannerActionExecutionResultFromActionExecutorResult("workspace.files.write", AgentLoopActionExecutorResult{
		Status: TurnResultBlocked,
		Reason: "blocked by policy",
	}, err)

	if loopResult.StopReason != "action denied" || !loopResult.CapabilityBlocked {
		t.Fatalf("loop result = stop %q blocked %v", loopResult.StopReason, loopResult.CapabilityBlocked)
	}
	if loopResult.Metadata["error"] != "action denied" ||
		loopResult.Metadata["action_executor_status"] != string(TurnResultBlocked) ||
		loopResult.Metadata[AgentLoopActionNameMetadataKey] != "workspace.files.write" {
		t.Fatalf("loop metadata = %#v", loopResult.Metadata)
	}
}

func TestAgentLoopPlannerActionExecutionResultFromTurnPayloadBuildsLoopResult(t *testing.T) {
	payload := AgentLoopTurnResultPayload{
		Messages: []AgentLoopMessage{{
			ID:      "msg-turn-payload",
			Role:    "tool",
			Content: "turn payload ok",
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:     "tool-turn-payload",
			Name:   "daemon.health",
			Result: map[string]any{"turn_payload": true},
		}},
		StopReason: "turn payload handled",
		Metadata:   map[string]any{"compat": "turn_executor"},
	}

	loopResult := AgentLoopPlannerActionExecutionResultFromTurnPayload("daemon.health", payload, nil)

	if loopResult.StopReason != "turn payload handled" {
		t.Fatalf("stop reason = %q", loopResult.StopReason)
	}
	if loopResult.Metadata["mode"] != string(AgentLoopModePlannerActionExecution) ||
		loopResult.Metadata[AgentLoopActionNameMetadataKey] != "daemon.health" ||
		loopResult.Metadata["message_count"] != 1 ||
		loopResult.Metadata["tool_call_count"] != 1 ||
		loopResult.Metadata["compat"] != "turn_executor" {
		t.Fatalf("loop metadata = %#v", loopResult.Metadata)
	}
	if loopResult.Messages[0].Content != "turn payload ok" ||
		loopResult.State.ResponseMessages[0].Content != "turn payload ok" ||
		loopResult.ToolCalls[0].Result["turn_payload"] != true {
		t.Fatalf("loop result did not clone payload fields: %#v", loopResult)
	}

	payload.Messages[0].Content = "mutated"
	payload.ToolCalls[0].Result["turn_payload"] = false
	payload.Metadata["compat"] = "mutated"
	if loopResult.Messages[0].Content != "turn payload ok" ||
		loopResult.ToolCalls[0].Result["turn_payload"] != true ||
		loopResult.Metadata["compat"] != "turn_executor" {
		t.Fatalf("loop result shares mutable payload fields: %#v", loopResult)
	}
}
