package runtime

import "testing"

func TestAgentLoopTurnExecutionMetadataMergesExtrasAndClones(t *testing.T) {
	existing := map[string]any{
		"phase": "route",
		AgentLoopTurnExecutionExecutorMetadataKey: "legacy_executor",
		AgentLoopTurnExecutionKindMetadataKey:     "legacy_kind",
	}
	extra := map[string]any{
		AgentLoopTurnExecutionMaxLoopsMetadataKey: 3,
		"mode": "replan_action_continuation",
	}

	metadata := AgentLoopTurnExecutionMetadata(existing, "replan_action_continuation", "read_only", extra)
	existing["phase"] = "mutated"
	extra["mode"] = "mutated_mode"
	metadata["new_key"] = "new_value"

	if metadata["phase"] != "route" || metadata["mode"] != "replan_action_continuation" {
		t.Fatalf("metadata did not preserve cloned existing/extra fields: %#v", metadata)
	}
	if metadata[AgentLoopTurnExecutionExecutorMetadataKey] != AgentLoopTurnExecutionExecutorMetadataValue ||
		metadata[AgentLoopTurnExecutionKindMetadataKey] != "replan_action_continuation" ||
		metadata[AgentLoopTurnExecutionOperationLevelMetadataKey] != "read_only" ||
		metadata[AgentLoopTurnExecutionMaxLoopsMetadataKey] != 3 {
		t.Fatalf("turn execution metadata = %#v", metadata)
	}
	if _, ok := existing["new_key"]; ok {
		t.Fatalf("metadata result shares original map: existing=%#v metadata=%#v", existing, metadata)
	}
	if _, ok := extra["new_key"]; ok {
		t.Fatalf("metadata result shares extra map: extra=%#v metadata=%#v", extra, metadata)
	}
}

func TestAgentLoopTurnExecutionTurnFromRequestBuildsClonedBoundary(t *testing.T) {
	req := AgentLoopRequest{
		Turn: TurnContext{
			SessionID:      "session-route",
			TaskID:         "task-route",
			RunID:          "run-route",
			TurnID:         "turn-route",
			Prompt:         "route prompt",
			Origin:         "legacy-origin",
			OperationLevel: "read_only",
			Metadata:       map[string]any{"phase": "route"},
		},
		Language: "zh",
		Operator: "tester",
		Metadata: map[string]any{"request": "metadata"},
	}

	turn := AgentLoopTurnExecutionTurnFromRequest(req, "deterministic_route", map[string]any{
		AgentLoopTurnExecutionMaxLoopsMetadataKey: 2,
	})
	req.Turn.Metadata["phase"] = "mutated"
	req.Metadata["request"] = "mutated"
	turn.Metadata["new_key"] = "new_value"

	if turn.Origin != "deterministic_route" || turn.TurnIndex != 1 {
		t.Fatalf("turn origin/index = %q/%d, want deterministic_route/1", turn.Origin, turn.TurnIndex)
	}
	if turn.Metadata["phase"] != "route" ||
		turn.Metadata[AgentLoopTurnExecutionExecutorMetadataKey] != AgentLoopTurnExecutionExecutorMetadataValue ||
		turn.Metadata[AgentLoopTurnExecutionKindMetadataKey] != "deterministic_route" ||
		turn.Metadata[AgentLoopTurnExecutionOperationLevelMetadataKey] != "read_only" ||
		turn.Metadata[AgentLoopTurnExecutionMaxLoopsMetadataKey] != 2 {
		t.Fatalf("turn metadata = %#v", turn.Metadata)
	}

	payload, ok := turn.Payload.(AgentLoopRequest)
	if !ok {
		t.Fatalf("turn payload = %T, want AgentLoopRequest", turn.Payload)
	}
	if payload.Turn.Metadata["phase"] != "route" || payload.Metadata["request"] != "metadata" {
		t.Fatalf("payload was not cloned before request mutation: %#v", payload)
	}
	if _, ok := req.Turn.Metadata["new_key"]; ok {
		t.Fatalf("turn metadata result shares original turn metadata: req=%#v turn=%#v", req.Turn.Metadata, turn.Metadata)
	}
}

func TestAgentLoopTurnExecutionBoundaryFromRequestPreservesPayload(t *testing.T) {
	payload := &struct{ Kind string }{Kind: "existing-payload"}
	req := AgentLoopRequest{
		Turn: TurnContext{
			SessionID:      "session-replan",
			TaskID:         "task-replan",
			RunID:          "run-replan",
			TurnID:         "turn-replan",
			Prompt:         "continue",
			Origin:         "legacy-origin",
			OperationLevel: "advanced",
			Payload:        payload,
			Metadata:       map[string]any{"phase": "continuation"},
		},
		MaxLoops: 4,
	}

	turn := AgentLoopTurnExecutionBoundaryFromRequest(req, "replan_action_continuation", map[string]any{
		AgentLoopTurnExecutionMaxLoopsMetadataKey: req.MaxLoops,
	})
	req.Turn.Metadata["phase"] = "mutated"
	turn.Metadata["new_key"] = "new_value"

	if turn.Origin != "replan_action_continuation" || turn.TurnIndex != 1 {
		t.Fatalf("turn origin/index = %q/%d, want replan_action_continuation/1", turn.Origin, turn.TurnIndex)
	}
	if turn.Payload != payload {
		t.Fatalf("turn payload was replaced: got %#v want original payload", turn.Payload)
	}
	if turn.Metadata["phase"] != "continuation" ||
		turn.Metadata[AgentLoopTurnExecutionExecutorMetadataKey] != AgentLoopTurnExecutionExecutorMetadataValue ||
		turn.Metadata[AgentLoopTurnExecutionKindMetadataKey] != "replan_action_continuation" ||
		turn.Metadata[AgentLoopTurnExecutionOperationLevelMetadataKey] != "advanced" ||
		turn.Metadata[AgentLoopTurnExecutionMaxLoopsMetadataKey] != 4 {
		t.Fatalf("turn metadata = %#v", turn.Metadata)
	}
	if _, ok := req.Turn.Metadata["new_key"]; ok {
		t.Fatalf("turn metadata result shares original turn metadata: req=%#v turn=%#v", req.Turn.Metadata, turn.Metadata)
	}
}
