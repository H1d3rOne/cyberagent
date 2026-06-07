package runtime

import "testing"

func TestAgentLoopControlSessionPayloadClone(t *testing.T) {
	payload := AgentLoopControlSessionPayload{
		Action:     "pause",
		Reason:     "operator pause",
		RunID:      "run-1",
		JobID:      "job-1",
		ClearInput: true,
		Operator:   "operator",
		Language:   "en",
	}
	cloned := payload.Clone()
	payload.Action = "changed"
	payload.ClearInput = false

	if cloned.Action != "pause" || !cloned.ClearInput || cloned.JobID != "job-1" {
		t.Fatalf("cloned payload = %#v, want original scalar fields", cloned)
	}
}
