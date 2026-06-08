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

func TestPrepareAgentLoopActionBatchTruncatesClonesAndSeedsMetadata(t *testing.T) {
	actions := []AgentLoopAction{
		{Name: "daemon.health", Arguments: map[string]any{"probe": "first"}},
		{Name: "plugins.list", Arguments: map[string]any{"probe": "second"}},
		{Name: "runs.list", Arguments: map[string]any{"probe": "third"}},
	}

	prepared := PrepareAgentLoopActionBatch(actions, "queue-test", 2)
	actions[0].Arguments["probe"] = "mutated"

	if prepared.QueueID != "queue-test" {
		t.Fatalf("QueueID = %q, want queue-test", prepared.QueueID)
	}
	if len(prepared.Actions) != 2 {
		t.Fatalf("prepared action count = %d, want 2", len(prepared.Actions))
	}
	if prepared.Actions[0].Arguments["probe"] != "first" {
		t.Fatalf("prepared action shares argument map: %#v", prepared.Actions[0].Arguments)
	}
	if prepared.Result.ActionCount != 2 || !prepared.Result.Truncated {
		t.Fatalf("result count/truncated = %d/%v, want 2/true", prepared.Result.ActionCount, prepared.Result.Truncated)
	}
	if prepared.Result.Metadata[AgentLoopActionQueueIDMetadataKey] != "queue-test" ||
		prepared.Result.Metadata[AgentLoopActionCountMetadataKey] != 2 ||
		prepared.Result.Metadata[AgentLoopActionBatchTruncatedMetadataKey] != true {
		t.Fatalf("result metadata = %#v", prepared.Result.Metadata)
	}

	cloned := prepared.Clone()
	prepared.Actions[0].Arguments["probe"] = "mutated-prepared"
	prepared.Result.Metadata[AgentLoopActionQueueIDMetadataKey] = "mutated-queue"
	if cloned.Actions[0].Arguments["probe"] != "first" {
		t.Fatalf("prepared clone shares action arguments: %#v", cloned.Actions[0].Arguments)
	}
	if cloned.Result.Metadata[AgentLoopActionQueueIDMetadataKey] != "queue-test" {
		t.Fatalf("prepared clone shares result metadata: %#v", cloned.Result.Metadata)
	}
}

func TestAgentLoopActionQueueAndToolMetadataHelpersClone(t *testing.T) {
	queueMetadata := AgentLoopActionQueueMetadata("queue-test", 2, 3, "daemon.health")
	if queueMetadata[AgentLoopActionQueueIDMetadataKey] != "queue-test" ||
		queueMetadata[AgentLoopActionIndexMetadataKey] != 2 ||
		queueMetadata[AgentLoopActionCountMetadataKey] != 3 ||
		queueMetadata[AgentLoopActionNameMetadataKey] != "daemon.health" {
		t.Fatalf("queue metadata = %#v", queueMetadata)
	}

	call := AgentLoopToolCall{ID: "tool-top", Name: "daemon.health", Metadata: map[string]any{"phase": "existing"}}
	calls := []AgentLoopToolCall{call}
	messages := []AgentLoopMessage{{ID: "msg", ToolCalls: []AgentLoopToolCall{{ID: "tool-message", Name: "daemon.health", Metadata: map[string]any{"phase": "message"}}}}}

	withTopLevelMetadata := AgentLoopToolCallsWithMetadata(calls, queueMetadata)
	withMessageMetadata := AgentLoopMessagesWithToolMetadata(messages, queueMetadata)
	queueMetadata[AgentLoopActionQueueIDMetadataKey] = "mutated-queue"
	calls[0].Metadata["phase"] = "mutated-top"
	messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message"

	if withTopLevelMetadata[0].Metadata["phase"] != "existing" || withTopLevelMetadata[0].Metadata[AgentLoopActionQueueIDMetadataKey] != "queue-test" {
		t.Fatalf("top-level metadata not cloned/merged: %#v", withTopLevelMetadata[0].Metadata)
	}
	if withMessageMetadata[0].ToolCalls[0].Metadata["phase"] != "message" || withMessageMetadata[0].ToolCalls[0].Metadata[AgentLoopActionQueueIDMetadataKey] != "queue-test" {
		t.Fatalf("message tool metadata not cloned/merged: %#v", withMessageMetadata[0].ToolCalls[0].Metadata)
	}
	if _, ok := calls[0].Metadata[AgentLoopActionQueueIDMetadataKey]; ok {
		t.Fatalf("original top-level call was mutated: %#v", calls[0].Metadata)
	}
	if _, ok := messages[0].ToolCalls[0].Metadata[AgentLoopActionQueueIDMetadataKey]; ok {
		t.Fatalf("original message tool call was mutated: %#v", messages[0].ToolCalls[0].Metadata)
	}
}
