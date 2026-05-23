package findings

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestStoreWriteEvidence(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	index := EvidenceIndex{
		Artifacts: []contracts.Artifact{{ID: "artifact_001", JobID: "job_001", StepID: "step_001", Type: contracts.ArtifactJSON, Path: "artifacts/http/probe.json"}},
		Evidence:  []contracts.Evidence{{ID: "evidence_001", JobID: "job_001", StepID: "step_001", ArtifactID: "artifact_001", Type: contracts.EvidenceHTTPResponse, Title: "HTTP status", Summary: "200 OK", Confidence: contracts.ConfidenceHigh, CreatedAt: contracts.Now()}},
	}

	if err := store.WriteEvidence(index); err != nil {
		t.Fatalf("WriteEvidence() error = %v", err)
	}

	var loaded EvidenceIndex
	readJSON(t, filepath.Join(workspace, "evidence", "index.json"), &loaded)
	if len(loaded.Artifacts) != 1 || loaded.Artifacts[0].ID != "artifact_001" || len(loaded.Evidence) != 1 || loaded.Evidence[0].ID != "evidence_001" {
		t.Fatalf("unexpected evidence index: %#v", loaded)
	}

	file, err := os.Open(filepath.Join(workspace, "evidence", "evidence.jsonl"))
	if err != nil {
		t.Fatalf("open evidence jsonl: %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatalf("expected evidence jsonl line")
	}
	var evidence contracts.Evidence
	if err := json.Unmarshal(scanner.Bytes(), &evidence); err != nil {
		t.Fatalf("decode evidence jsonl: %v", err)
	}
	if evidence.ID != "evidence_001" {
		t.Fatalf("unexpected evidence jsonl item: %#v", evidence)
	}
	if scanner.Scan() {
		t.Fatalf("expected one evidence jsonl line")
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan evidence jsonl: %v", err)
	}
}

func TestStoreWriteFindings(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	items := []contracts.Finding{{ID: "finding_001", JobID: "job_001", Title: "Missing CSP", Severity: contracts.SeverityLow, Confidence: contracts.ConfidenceHigh, Category: "headers", EvidenceIDs: []string{"evidence_001"}, Source: contracts.FindingSource{Tool: "builtin.header_checker", StepID: "step_001"}, CreatedAt: contracts.Now()}}

	if err := store.WriteFindings(items); err != nil {
		t.Fatalf("WriteFindings() error = %v", err)
	}
	var loaded []contracts.Finding
	readJSON(t, filepath.Join(workspace, "findings.json"), &loaded)
	if len(loaded) != 1 || loaded[0].ID != "finding_001" || loaded[0].EvidenceIDs[0] != "evidence_001" {
		t.Fatalf("unexpected findings: %#v", loaded)
	}
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
