package runtime

import "testing"

func TestAgentLoopResolveAuthorizationPayloadClone(t *testing.T) {
	payload := AgentLoopResolveAuthorizationPayload{
		Decision:               "deny",
		AuthorizationID:        "auth-1",
		Reason:                 "not within scope",
		Operator:               "operator",
		Language:               "en",
		OperationLevelProvided: true,
	}
	cloned := payload.Clone()
	payload.Decision = "approve"
	payload.OperationLevelProvided = false

	if cloned.Decision != "deny" || !cloned.OperationLevelProvided || cloned.AuthorizationID != "auth-1" {
		t.Fatalf("cloned payload = %#v, want original scalar fields", cloned)
	}
}
