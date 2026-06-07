package runtime

import "testing"

func TestAgentLoopRecordUserMessagePayloadClone(t *testing.T) {
	payload := AgentLoopRecordUserMessagePayload{Operator: "operator", Language: "en"}
	cloned := payload.Clone()
	payload.Operator = "changed"
	payload.Language = "zh"

	if cloned.Operator != "operator" || cloned.Language != "en" {
		t.Fatalf("cloned payload = %#v, want original operator/language", cloned)
	}
}
