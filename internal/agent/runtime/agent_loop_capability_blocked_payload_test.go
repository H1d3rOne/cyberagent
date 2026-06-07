package runtime

import "testing"

func TestAgentLoopCapabilityBlockedPayloadClone(t *testing.T) {
	payload := AgentLoopCapabilityBlockedPayload{
		Language:        "en",
		ActionName:      "scan.web.safe",
		IntentID:        "intent-1",
		Reason:          "requires safe scan",
		SuggestedPrompt: "Run a safe scan",
	}
	cloned := payload.Clone()
	payload.ActionName = "changed"
	payload.SuggestedPrompt = "changed"

	if cloned.ActionName != "scan.web.safe" || cloned.SuggestedPrompt != "Run a safe scan" {
		t.Fatalf("cloned payload = %#v, want original scalar fields", cloned)
	}
}
