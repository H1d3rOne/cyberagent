package runtime

import (
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

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

func TestAgentLoopActionBatchAssemblyHelpersClone(t *testing.T) {
	result := AgentLoopActionBatchResult{ActionCount: 2}
	messages := []AgentLoopMessage{{
		ID:      "msg-action",
		Content: "action done",
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-in-message",
			Metadata: map[string]any{"phase": "message"},
		}},
	}}
	toolCalls := []AgentLoopToolCall{{
		ID:       "tool-top",
		Result:   map[string]any{"action": "daemon.health"},
		Metadata: map[string]any{"phase": "top"},
	}}

	AppendAgentLoopActionBatchActionResult(&result, 1, messages, toolCalls)
	messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message"
	toolCalls[0].Result["action"] = "mutated-action"
	toolCalls[0].Metadata["phase"] = "mutated-top"

	if result.ProcessedActionCount != 1 {
		t.Fatalf("ProcessedActionCount = %d, want 1", result.ProcessedActionCount)
	}
	if result.Messages[0].ToolCalls[0].Metadata["phase"] != "message" {
		t.Fatalf("batch shares message metadata: %#v", result.Messages[0].ToolCalls[0].Metadata)
	}
	if result.ToolCalls[0].Result["action"] != "daemon.health" || result.ToolCalls[0].Metadata["phase"] != "top" {
		t.Fatalf("batch shares tool data: result=%#v metadata=%#v", result.ToolCalls[0].Result, result.ToolCalls[0].Metadata)
	}

	gate := AgentLoopActionGateResult{
		Decision: AgentLoopActionGateDecision{Kind: AgentLoopActionGateManualAuthorizationRequired, ActionName: "workspace.files.write"},
		Messages: []AgentLoopMessage{{
			ID:      "msg-gate",
			Content: "authorization required",
			ToolCalls: []AgentLoopToolCall{{
				ID:       "tool-gate-message",
				Metadata: map[string]any{"phase": "gate-message"},
			}},
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-gate",
			Metadata: map[string]any{"phase": "gate"},
		}},
		AuthorizationRequired: true,
		Handled:               true,
	}

	StopAgentLoopActionBatchAtGate(&result, 2, gate)
	gate.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-gate-message"
	gate.ToolCalls[0].Metadata["phase"] = "mutated-gate"
	result.Messages[1].ToolCalls[0].Metadata["phase"] = "mutated-batch-gate-message"

	if result.ProcessedActionCount != 2 || !result.AuthorizationRequired || result.StopGate == nil {
		t.Fatalf("gate stop result = %#v", result)
	}
	if result.StopGate.Decision.Kind != AgentLoopActionGateManualAuthorizationRequired {
		t.Fatalf("StopGate decision = %#v", result.StopGate.Decision)
	}
	if result.StopGate.Messages[0].ToolCalls[0].Metadata["phase"] != "gate-message" {
		t.Fatalf("stop gate shares message metadata: %#v", result.StopGate.Messages[0].ToolCalls[0].Metadata)
	}
	if result.StopGate.ToolCalls[0].Metadata["phase"] != "gate" {
		t.Fatalf("stop gate shares tool metadata: %#v", result.StopGate.ToolCalls[0].Metadata)
	}
}

func TestAgentLoopActionBatchStepResultHelpersCloneAndApply(t *testing.T) {
	prepared := PrepareAgentLoopActionBatch([]AgentLoopAction{
		{Name: "daemon.health", Arguments: map[string]any{"probe": "first"}},
		{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md"}},
	}, "queue-step", 0)
	result := prepared.Result
	messages := []AgentLoopMessage{{
		ID:      "msg-step",
		Content: "executed",
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-message-step",
			Name:     "daemon.health",
			Metadata: map[string]any{"phase": "message"},
		}},
	}}
	toolCalls := []AgentLoopToolCall{{
		ID:       "tool-top-step",
		Name:     "daemon.health",
		Result:   map[string]any{"ok": true},
		Metadata: map[string]any{"phase": "top"},
	}}

	step := AgentLoopActionBatchExecutionStepResult(1, "daemon.health", messages, toolCalls)
	messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message-input"
	toolCalls[0].Result["ok"] = false
	toolCalls[0].Metadata["phase"] = "mutated-top-input"

	stopped := ApplyAgentLoopActionBatchStepResult(&result, prepared, step)
	step.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-step-message"
	step.ToolCalls[0].Result["ok"] = "mutated-step"
	step.ToolCalls[0].Metadata["phase"] = "mutated-step-top"

	if stopped {
		t.Fatalf("ApplyAgentLoopActionBatchStepResult stopped on execution step")
	}
	if result.ProcessedActionCount != 1 || result.AuthorizationRequired {
		t.Fatalf("execution step result = %#v, want processed without authorization", result)
	}
	if result.ToolCalls[0].Result["ok"] != true || result.ToolCalls[0].Metadata["phase"] != "top" {
		t.Fatalf("execution result shares top-level data: result=%#v metadata=%#v", result.ToolCalls[0].Result, result.ToolCalls[0].Metadata)
	}
	if result.Messages[0].ToolCalls[0].Metadata["phase"] != "message" {
		t.Fatalf("execution result shares message metadata: %#v", result.Messages[0].ToolCalls[0].Metadata)
	}
	for _, metadata := range []map[string]any{result.ToolCalls[0].Metadata, result.Messages[0].ToolCalls[0].Metadata} {
		if metadata[AgentLoopActionQueueIDMetadataKey] != "queue-step" ||
			metadata[AgentLoopActionIndexMetadataKey] != 1 ||
			metadata[AgentLoopActionCountMetadataKey] != 2 ||
			metadata[AgentLoopActionNameMetadataKey] != "daemon.health" {
			t.Fatalf("execution metadata = %#v, want queue metadata", metadata)
		}
	}

	gate := AgentLoopActionGateResult{
		Decision: AgentLoopActionGateDecision{
			Kind:       AgentLoopActionGateManualAuthorizationRequired,
			ActionName: "workspace.files.write",
			Policy:     "manual_tool_authorization_required",
		},
		Messages: []AgentLoopMessage{{
			ID:      "msg-gate-step",
			Content: "authorization required",
			ToolCalls: []AgentLoopToolCall{{
				ID:       "tool-gate-message-step",
				Metadata: map[string]any{"phase": "gate-message"},
			}},
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-gate-step",
			Metadata: map[string]any{"phase": "gate-top"},
		}},
		AuthorizationRequired: true,
		Handled:               true,
	}
	gateStep := AgentLoopActionBatchGateStepResult(2, gate)
	gate.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-gate-input"
	gate.ToolCalls[0].Metadata["phase"] = "mutated-gate-top-input"

	stopped = ApplyAgentLoopActionBatchStepResult(&result, prepared, gateStep)
	gateStep.StopGate.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-gate-step"
	gateStep.StopGate.ToolCalls[0].Metadata["phase"] = "mutated-gate-step-top"

	if !stopped || !result.AuthorizationRequired || result.ProcessedActionCount != 2 || result.StopGate == nil {
		t.Fatalf("gate step result = %#v, want stopped authorization gate", result)
	}
	if result.StopGate.Decision.ActionName != "workspace.files.write" {
		t.Fatalf("stop gate action = %#v, want workspace.files.write", result.StopGate.Decision)
	}
	if result.StopGate.Messages[0].ToolCalls[0].Metadata["phase"] != "gate-message" ||
		result.StopGate.ToolCalls[0].Metadata["phase"] != "gate-top" {
		t.Fatalf("stop gate shares gate step data: messages=%#v toolCalls=%#v", result.StopGate.Messages, result.StopGate.ToolCalls)
	}
}

func TestAppendAgentLoopActionBatchActionResultWithQueueMetadata(t *testing.T) {
	prepared := PrepareAgentLoopActionBatch([]AgentLoopAction{
		{Name: "daemon.health", Arguments: map[string]any{"probe": "first"}},
		{Name: "plugins.list", Arguments: map[string]any{"probe": "second"}},
	}, "queue-test", 0)
	result := prepared.Result
	messages := []AgentLoopMessage{{
		ID: "msg-action",
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-message",
			Name:     "daemon.health",
			Metadata: map[string]any{"phase": "message"},
		}},
	}}
	toolCalls := []AgentLoopToolCall{{
		ID:       "tool-top",
		Name:     "daemon.health",
		Metadata: map[string]any{"phase": "top"},
	}}

	AppendAgentLoopActionBatchActionResultWithQueueMetadata(&result, prepared, 2, "daemon.health", messages, toolCalls)
	messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message"
	toolCalls[0].Metadata["phase"] = "mutated-top"

	if result.ProcessedActionCount != 2 {
		t.Fatalf("ProcessedActionCount = %d, want 2", result.ProcessedActionCount)
	}
	if len(result.Messages) != 1 || len(result.ToolCalls) != 1 || len(result.Messages[0].ToolCalls) != 1 {
		t.Fatalf("unexpected result shape: messages=%#v toolCalls=%#v", result.Messages, result.ToolCalls)
	}
	for _, metadata := range []map[string]any{result.ToolCalls[0].Metadata, result.Messages[0].ToolCalls[0].Metadata} {
		if metadata[AgentLoopActionQueueIDMetadataKey] != "queue-test" ||
			metadata[AgentLoopActionIndexMetadataKey] != 2 ||
			metadata[AgentLoopActionCountMetadataKey] != 2 ||
			metadata[AgentLoopActionNameMetadataKey] != "daemon.health" {
			t.Fatalf("metadata = %#v, want queue metadata", metadata)
		}
	}
	if result.ToolCalls[0].Metadata["phase"] != "top" {
		t.Fatalf("top-level phase = %#v, want top", result.ToolCalls[0].Metadata["phase"])
	}
	if result.Messages[0].ToolCalls[0].Metadata["phase"] != "message" {
		t.Fatalf("message tool phase = %#v, want message", result.Messages[0].ToolCalls[0].Metadata["phase"])
	}
	if _, ok := toolCalls[0].Metadata[AgentLoopActionQueueIDMetadataKey]; ok {
		t.Fatalf("original top-level call mutated: %#v", toolCalls[0].Metadata)
	}
	if _, ok := messages[0].ToolCalls[0].Metadata[AgentLoopActionQueueIDMetadataKey]; ok {
		t.Fatalf("original message tool call mutated: %#v", messages[0].ToolCalls[0].Metadata)
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

func TestPrepareAgentLoopActionBatchWithGeneratedQueueIDUsesDefaults(t *testing.T) {
	actions := make([]AgentLoopAction, AgentLoopActionBatchDefaultMaxActions+1)
	for i := range actions {
		actions[i] = AgentLoopAction{Name: "daemon.health", Arguments: map[string]any{"index": i}}
	}
	generated := 0
	prepared := PrepareAgentLoopActionBatchWithGeneratedQueueID(actions, func() string {
		generated++
		return AgentLoopActionQueueIDFromJobID("job_test")
	})

	if generated != 1 {
		t.Fatalf("generated queue IDs = %d, want 1", generated)
	}
	if prepared.QueueID != "queue_test" {
		t.Fatalf("QueueID = %q, want queue_test", prepared.QueueID)
	}
	if len(prepared.Actions) != AgentLoopActionBatchDefaultMaxActions {
		t.Fatalf("prepared action count = %d, want default %d", len(prepared.Actions), AgentLoopActionBatchDefaultMaxActions)
	}
	if !prepared.Result.Truncated || prepared.Result.ActionCount != AgentLoopActionBatchDefaultMaxActions {
		t.Fatalf("result count/truncated = %d/%v, want default/true", prepared.Result.ActionCount, prepared.Result.Truncated)
	}
	if prepared.Result.Metadata[AgentLoopActionQueueIDMetadataKey] != "queue_test" {
		t.Fatalf("metadata queue = %#v, want queue_test", prepared.Result.Metadata)
	}

	empty := PrepareAgentLoopActionBatchWithGeneratedQueueID(nil, func() string {
		generated++
		return "queue-empty"
	})
	if generated != 1 {
		t.Fatalf("empty batch generated queue ID, generated = %d", generated)
	}
	if empty.QueueID != "" || empty.Result.Metadata != nil || empty.Result.ActionCount != 0 {
		t.Fatalf("empty batch = %#v, want no queue metadata", empty)
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

func TestAgentLoopActionExecutionErrorResultExpandsServiceDetails(t *testing.T) {
	err := services.NewError(services.CodeUnsupported, "unsupported action", map[string]any{
		"action":  "service-detail-action",
		"target":  "https://example.com",
		"missing": "plugin",
	}, nil)

	result := AgentLoopActionExecutionErrorResult("daemon.health", err)
	if result[AgentLoopActionResultActionKey] != "daemon.health" {
		t.Fatalf("action = %#v, want daemon.health", result[AgentLoopActionResultActionKey])
	}
	if result["target"] != "https://example.com" || result["missing"] != "plugin" {
		t.Fatalf("expanded details missing: %#v", result)
	}
	if result["action"] != "daemon.health" {
		t.Fatalf("details overwrote top-level action: %#v", result)
	}
	details, ok := result[AgentLoopActionResultDetailsKey].(map[string]any)
	if !ok {
		t.Fatalf("details type = %T, want map[string]any", result[AgentLoopActionResultDetailsKey])
	}
	if details["action"] != "service-detail-action" || details["target"] != "https://example.com" {
		t.Fatalf("nested details = %#v", details)
	}
	err.Details["target"] = "mutated"
	if result["target"] != "https://example.com" || details["target"] != "https://example.com" {
		t.Fatalf("result shares service error details: result=%#v details=%#v", result, details)
	}
}
