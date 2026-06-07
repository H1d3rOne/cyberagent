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
