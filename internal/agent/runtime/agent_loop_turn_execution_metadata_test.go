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
