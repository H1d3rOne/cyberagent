package runtime

import "testing"

func TestAgentLoopAuthorizationRequestPayloadCloneIsolatesMutableFields(t *testing.T) {
	payload := AgentLoopAuthorizationRequestPayload{
		Language:      "en",
		Action:        AgentLoopAction{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md"}},
		ActionName:    "workspace.files.write",
		RequiredLevel: "workspace_write",
		Policy:        "manual_tool_authorization_required",
		Reason:        "needs approval",
	}

	cloned := payload.Clone()
	payload.Action.Arguments["path"] = "changed.md"

	if cloned.Action.Arguments["path"] != "notes.md" {
		t.Fatalf("cloned action path = %#v, want isolated notes.md", cloned.Action.Arguments["path"])
	}
	if cloned.Language != "en" || cloned.Policy != "manual_tool_authorization_required" {
		t.Fatalf("cloned scalar fields = %#v, want original fields", cloned)
	}
}
