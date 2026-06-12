package runtime

import "testing"

func TestAgentLoopTurnExecutionRequestBuildsClonedBoundary(t *testing.T) {
	turn := TurnContext{
		SessionID:      "session-route",
		TaskID:         "task-route",
		RunID:          "run-route",
		TurnID:         "turn-route",
		Prompt:         "route prompt",
		Origin:         "deterministic_route",
		OperationLevel: "read_only",
		Metadata:       map[string]any{"phase": "route"},
	}
	toolServices := AgentLoopToolServiceBinding{
		Kind:     "daemon",
		Handle:   "opaque-handle",
		Metadata: map[string]any{"service": "router"},
	}
	metadata := map[string]any{"request": "route"}

	req := AgentLoopTurnExecutionRequest(turn, "zh", "tester", toolServices, metadata)
	turn.Metadata["phase"] = "mutated"
	toolServices.Metadata["service"] = "mutated"
	metadata["request"] = "mutated"
	req.Turn.Metadata["new_key"] = "new_value"
	req.ToolServices.Metadata["new_service_key"] = "new_service_value"

	if req.Turn.Metadata["phase"] != "route" || req.Metadata["request"] != "route" {
		t.Fatalf("request did not clone turn/request metadata: %#v", req)
	}
	if req.ToolServices.Metadata["service"] != "router" || req.ToolServices.Handle != "opaque-handle" {
		t.Fatalf("request did not clone tool service metadata or preserve handle: %#v", req.ToolServices)
	}
	if _, ok := turn.Metadata["new_key"]; ok {
		t.Fatalf("request turn metadata shares original map: turn=%#v req=%#v", turn.Metadata, req.Turn.Metadata)
	}
	if _, ok := toolServices.Metadata["new_service_key"]; ok {
		t.Fatalf("request tool service metadata shares original map: services=%#v req=%#v", toolServices.Metadata, req.ToolServices.Metadata)
	}
}

func TestAgentLoopActionExecutionRequestBuildsClonedAuthorizationBoundary(t *testing.T) {
	enabled := true
	turn := TurnContext{
		SessionID:      "session-action",
		Prompt:         "run action",
		Origin:         "planner_action_execution",
		OperationLevel: "safe_scan",
		Metadata:       map[string]any{"phase": "action"},
	}
	toolServices := AgentLoopToolServiceBinding{
		Kind:     "daemon",
		Metadata: map[string]any{"service": "router"},
	}
	authorization := AgentLoopAuthorization{
		DefaultAuthProfileID: "auth-1",
		AuthProfiles: []AgentLoopAuthProfile{{
			ID:      "auth-1",
			Token:   "secret-token",
			Enabled: &enabled,
		}},
	}
	metadata := map[string]any{"request": "action"}

	req := AgentLoopActionExecutionRequest(turn, "en", "tester", toolServices, authorization, metadata)
	turn.Metadata["phase"] = "mutated"
	toolServices.Metadata["service"] = "mutated"
	authorization.AuthProfiles[0].Token = "mutated-token"
	*authorization.AuthProfiles[0].Enabled = false
	metadata["request"] = "mutated"

	if req.Turn.Metadata["phase"] != "action" || req.Metadata["request"] != "action" {
		t.Fatalf("request did not clone turn/request metadata: %#v", req)
	}
	if req.ToolServices.Metadata["service"] != "router" {
		t.Fatalf("request did not clone tool service metadata: %#v", req.ToolServices)
	}
	if req.Authorization.AuthProfiles[0].Token != "secret-token" {
		t.Fatalf("request authorization shares auth profiles: %#v", req.Authorization.AuthProfiles)
	}
	if req.Authorization.AuthProfiles[0].Enabled == nil || !*req.Authorization.AuthProfiles[0].Enabled {
		t.Fatalf("request authorization shares or lost Enabled value: %#v", req.Authorization.AuthProfiles[0].Enabled)
	}
}

func TestAgentLoopPlannerActionExecutionRequestBuildsModeAndClonedAction(t *testing.T) {
	enabled := true
	turn := TurnContext{
		SessionID:      "session-planner-action-mode",
		Prompt:         "run daemon health",
		Origin:         "planner_action_execution",
		OperationLevel: "safe_scan",
		Metadata:       map[string]any{"phase": "action"},
	}
	toolServices := AgentLoopToolServiceBinding{
		Kind:     "daemon",
		Metadata: map[string]any{"service": "router"},
	}
	authorization := AgentLoopAuthorization{
		DefaultAuthProfileID: "auth-planner-action",
		AuthProfiles: []AgentLoopAuthProfile{{
			ID:      "auth-planner-action",
			Token:   "secret-token",
			Enabled: &enabled,
		}},
	}
	action := AgentLoopAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "planner-action-mode"},
	}
	metadata := map[string]any{"request": "planner_action_execution"}

	req := AgentLoopPlannerActionExecutionRequest(turn, "en", "tester", toolServices, authorization, action, metadata)
	turn.Metadata["phase"] = "mutated"
	toolServices.Metadata["service"] = "mutated"
	authorization.AuthProfiles[0].Token = "mutated-token"
	*authorization.AuthProfiles[0].Enabled = false
	action.Arguments["probe"] = "mutated"
	metadata["request"] = "mutated"

	if req.Mode != AgentLoopModePlannerActionExecution {
		t.Fatalf("request mode = %q, want %q", req.Mode, AgentLoopModePlannerActionExecution)
	}
	if req.Turn.Metadata["phase"] != "action" || req.Metadata["request"] != "planner_action_execution" {
		t.Fatalf("request did not clone turn/request metadata: %#v", req)
	}
	if req.ToolServices.Metadata["service"] != "router" {
		t.Fatalf("request did not clone tool service metadata: %#v", req.ToolServices)
	}
	if req.Authorization.AuthProfiles[0].Token != "secret-token" || req.Authorization.AuthProfiles[0].Enabled == nil || !*req.Authorization.AuthProfiles[0].Enabled {
		t.Fatalf("request authorization shares auth profiles: %#v", req.Authorization.AuthProfiles)
	}
	if req.Action.Name != "daemon.health" || req.Action.Arguments["probe"] != "planner-action-mode" {
		t.Fatalf("request action shares mutable fields: %#v", req.Action)
	}
}
