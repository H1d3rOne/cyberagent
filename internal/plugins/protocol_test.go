package plugins

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestEncodeSubprocessRequest(t *testing.T) {
	encoded, err := EncodeSubprocessRequest(SubprocessRequest{ID: "request_001", JobID: "job_001", StepID: "step_001", Target: "http://localhost:8080", Input: map[string]any{"mode": "test"}})
	if err != nil {
		t.Fatalf("EncodeSubprocessRequest() error = %v", err)
	}
	if !strings.HasSuffix(string(encoded), "\n") {
		t.Fatalf("encoded request should end with newline: %q", encoded)
	}
	var decoded SubprocessRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if decoded.ID != "request_001" || decoded.Target != "http://localhost:8080" || decoded.Input["mode"] != "test" {
		t.Fatalf("unexpected decoded request: %#v", decoded)
	}
}

func TestEncodeSubprocessRequestRequiresID(t *testing.T) {
	_, err := EncodeSubprocessRequest(SubprocessRequest{JobID: "job_001"})
	if err == nil {
		t.Fatalf("expected missing id error")
	}
}

func TestDecodeSubprocessMessages(t *testing.T) {
	stdout := []byte(`{"type":"routes","request":"request_001","routes":[{"raw":"/api/users","resolved":"http://localhost:8080/api/users"}]}
{"type":"finding","findings":[{"template_id":"custom","name":"Custom Finding","severity":"low","matched":"http://localhost:8080"}]}
`)
	messages, err := DecodeSubprocessMessages(stdout)
	if err != nil {
		t.Fatalf("DecodeSubprocessMessages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	if len(messages[0].Routes) != 1 || messages[0].Routes[0].Raw != "/api/users" {
		t.Fatalf("unexpected route message: %#v", messages[0])
	}
	if len(messages[1].Findings) != 1 || messages[1].Findings[0].Name != "Custom Finding" {
		t.Fatalf("unexpected finding message: %#v", messages[1])
	}
}

func TestDecodeSubprocessMessagesKeepsRawData(t *testing.T) {
	messages, err := DecodeSubprocessMessages([]byte("{\"routes\":[],\"request\":\"request_001\"}\n"))
	if err != nil {
		t.Fatalf("DecodeSubprocessMessages() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["request"] != "request_001" {
		t.Fatalf("unexpected raw data: %#v", messages)
	}
}

func TestDecodeSubprocessMessagesRejectsInvalidJSON(t *testing.T) {
	_, err := DecodeSubprocessMessages([]byte("{bad json}\n"))
	if err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestSubprocessRequestIncludesEvidenceAndArtifacts(t *testing.T) {
	encoded, err := EncodeSubprocessRequest(SubprocessRequest{ID: "request_001", Artifacts: []contracts.Artifact{{ID: "artifact_001"}}, Evidence: []contracts.Evidence{{ID: "evidence_001"}}})
	if err != nil {
		t.Fatalf("EncodeSubprocessRequest() error = %v", err)
	}
	if !strings.Contains(string(encoded), "artifact_001") || !strings.Contains(string(encoded), "evidence_001") {
		t.Fatalf("encoded request missing artifacts/evidence: %s", encoded)
	}
}
