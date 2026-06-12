package runtime

import "testing"

func TestAgentLoopActionExecutionPayloadCloneIsolatesMutableFields(t *testing.T) {
	enabled := true
	payload := AgentLoopActionExecutionPayload{
		Request: AgentLoopRequest{
			Turn: TurnContext{
				SessionID:      "session-action",
				Prompt:         "daemon health",
				OperationLevel: "safe_scan",
				Metadata:       map[string]any{"kind": "planner_action_execution"},
			},
			Language: "en",
			Operator: "tester",
			Authorization: AgentLoopAuthorization{
				DefaultAuthProfileID: "auth-1",
				AuthProfiles: []AgentLoopAuthProfile{{
					ID:      "auth-1",
					Token:   "secret-token",
					Enabled: &enabled,
				}},
			},
		},
		Action: AgentLoopAction{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "runtime-payload"},
		},
	}

	cloned := payload.Clone()
	payload.Request.Turn.Metadata["kind"] = "mutated-kind"
	payload.Request.Authorization.AuthProfiles[0].Token = "mutated-token"
	*payload.Request.Authorization.AuthProfiles[0].Enabled = false
	payload.Action.Arguments["probe"] = "mutated-probe"

	if cloned.Request.Turn.Metadata["kind"] != "planner_action_execution" {
		t.Fatalf("clone shares request turn metadata: %#v", cloned.Request.Turn.Metadata)
	}
	if cloned.Request.Authorization.AuthProfiles[0].Token != "secret-token" {
		t.Fatalf("clone shares authorization profiles: %#v", cloned.Request.Authorization.AuthProfiles)
	}
	if cloned.Request.Authorization.AuthProfiles[0].Enabled == nil || !*cloned.Request.Authorization.AuthProfiles[0].Enabled {
		t.Fatalf("clone shares or lost Enabled value: %#v", cloned.Request.Authorization.AuthProfiles[0].Enabled)
	}
	if cloned.Action.Arguments["probe"] != "runtime-payload" {
		t.Fatalf("clone shares action arguments: %#v", cloned.Action.Arguments)
	}
}

func TestAgentLoopActionExecutionTurnBuildsPayloadAndMetadata(t *testing.T) {
	req := AgentLoopRequest{
		Turn: TurnContext{
			SessionID:      "session-action",
			TaskID:         "task-action",
			RunID:          "run-action",
			TurnID:         "turn-action",
			Prompt:         "run action",
			Origin:         "legacy-origin",
			OperationLevel: "safe_scan",
			Metadata:       map[string]any{"phase": "action"},
		},
		Language: "en",
		Operator: "tester",
	}
	action := AgentLoopAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "runtime-turn"},
	}

	turn := AgentLoopActionExecutionTurn(req, "planner_action_execution", action, "daemon.health")
	req.Turn.Metadata["phase"] = "mutated"
	action.Arguments["probe"] = "mutated"

	if turn.Origin != "planner_action_execution" || turn.TurnIndex != 1 {
		t.Fatalf("turn origin/index = %q/%d, want planner_action_execution/1", turn.Origin, turn.TurnIndex)
	}
	if turn.Metadata["phase"] != "action" ||
		turn.Metadata[AgentLoopActionExecutionExecutorMetadataKey] != AgentLoopActionExecutionExecutorMetadataValue ||
		turn.Metadata[AgentLoopActionExecutionKindMetadataKey] != "planner_action_execution" ||
		turn.Metadata[AgentLoopActionExecutionOperationLevelMetadataKey] != "safe_scan" ||
		turn.Metadata[AgentLoopActionNameMetadataKey] != "daemon.health" {
		t.Fatalf("action execution turn metadata = %#v", turn.Metadata)
	}

	payload, ok := turn.Payload.(AgentLoopActionExecutionPayload)
	if !ok {
		t.Fatalf("turn payload = %T, want AgentLoopActionExecutionPayload", turn.Payload)
	}
	if payload.Request.Turn.Metadata["phase"] != "action" {
		t.Fatalf("payload request shares original request metadata: %#v", payload.Request.Turn.Metadata)
	}
	if payload.Action.Arguments["probe"] != "runtime-turn" {
		t.Fatalf("payload action shares original action arguments: %#v", payload.Action.Arguments)
	}
}

func TestAgentLoopActionExecutionTurnMetadataMergesAndClones(t *testing.T) {
	existing := map[string]any{
		"phase": "step_replan",
		AgentLoopActionExecutionExecutorMetadataKey: "legacy_executor",
		AgentLoopActionExecutionKindMetadataKey:     "legacy_kind",
		AgentLoopActionNameMetadataKey:              "legacy.action",
	}

	metadata := AgentLoopActionExecutionTurnMetadata(existing, "planner_action_execution", "safe_scan", "daemon.health")
	existing["phase"] = "mutated_phase"
	metadata["new_key"] = "new_value"

	if metadata["phase"] != "step_replan" {
		t.Fatalf("metadata did not preserve cloned existing fields: %#v", metadata)
	}
	if metadata[AgentLoopActionExecutionExecutorMetadataKey] != AgentLoopActionExecutionExecutorMetadataValue ||
		metadata[AgentLoopActionExecutionKindMetadataKey] != "planner_action_execution" ||
		metadata[AgentLoopActionExecutionOperationLevelMetadataKey] != "safe_scan" ||
		metadata[AgentLoopActionNameMetadataKey] != "daemon.health" {
		t.Fatalf("action execution metadata = %#v", metadata)
	}
	if _, ok := existing["new_key"]; ok {
		t.Fatalf("metadata result shares original map: existing=%#v metadata=%#v", existing, metadata)
	}
}
