package runtime

import "testing"

func TestAgentLoopActionGateAndBatchResultCloneIsolatesMutableFields(t *testing.T) {
	gate := AgentLoopActionGateResult{
		Decision: AgentLoopActionGateDecision{
			Kind:            AgentLoopActionGateCapabilityBlocked,
			ActionName:      "scan.web.safe",
			RequiredLevel:   "safe_scan",
			Reason:          "capability mismatch",
			IntentID:        "content_discovery",
			SuggestedPrompt: "Use content discovery",
		},
		Messages: []AgentLoopMessage{{
			ID:      "msg-gate",
			Content: "blocked",
			ToolCalls: []AgentLoopToolCall{{
				ID:       "tool-in-message",
				Name:     "agent.capability_match",
				Metadata: map[string]any{"phase": "step_replan"},
			}},
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-gate",
			Name:     "agent.capability_match",
			Result:   map[string]any{"action": "scan.web.safe"},
			Metadata: map[string]any{"phase": "step_replan"},
		}},
		AuthorizationRequired: true,
		Handled:               true,
	}
	batch := AgentLoopActionBatchResult{
		Messages:              gate.Messages,
		ToolCalls:             gate.ToolCalls,
		AuthorizationRequired: true,
		StopGate:              &gate,
		ActionCount:           2,
		ProcessedActionCount:  1,
		Truncated:             true,
		Metadata:              map[string]any{"action_queue_id": "queue-1"},
	}

	clonedGate := gate.Clone()
	clonedBatch := batch.Clone()
	gate.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message-tool"
	gate.ToolCalls[0].Result["action"] = "mutated-action"
	gate.ToolCalls[0].Metadata["phase"] = "mutated-tool"
	batch.StopGate.Decision.Reason = "mutated-reason"
	batch.Metadata["action_queue_id"] = "mutated-queue"

	if clonedGate.Messages[0].ToolCalls[0].Metadata["phase"] != "step_replan" {
		t.Fatalf("gate clone shares message tool metadata: %#v", clonedGate.Messages[0].ToolCalls[0].Metadata)
	}
	if clonedGate.ToolCalls[0].Result["action"] != "scan.web.safe" {
		t.Fatalf("gate clone shares tool result: %#v", clonedGate.ToolCalls[0].Result)
	}
	if clonedGate.ToolCalls[0].Metadata["phase"] != "step_replan" {
		t.Fatalf("gate clone shares tool metadata: %#v", clonedGate.ToolCalls[0].Metadata)
	}
	if clonedBatch.StopGate == nil || clonedBatch.StopGate.Decision.Reason != "capability mismatch" {
		t.Fatalf("batch clone shares stop gate: %#v", clonedBatch.StopGate)
	}
	if clonedBatch.Metadata["action_queue_id"] != "queue-1" {
		t.Fatalf("batch clone shares metadata: %#v", clonedBatch.Metadata)
	}
	if !clonedBatch.AuthorizationRequired || clonedBatch.ActionCount != 2 || clonedBatch.ProcessedActionCount != 1 || !clonedBatch.Truncated {
		t.Fatalf("batch clone changed scalar fields: %#v", clonedBatch)
	}
}
