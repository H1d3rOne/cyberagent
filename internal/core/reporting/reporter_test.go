package reporting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestGenerateWritesMarkdownAndJSONReport(t *testing.T) {
	workspace := t.TempDir()
	target, err := contracts.NewWebTarget("target_001", "http://localhost:8080")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{Hosts: []string{"localhost"}})
	job := contracts.NewWebScanJob("job_001", target.ID, scope.ID, "safe", workspace)
	job.Status = contracts.StatusSucceeded
	plan := contracts.NewPlan("plan_001", job.ID, "builtin.web.safe.v1", []string{"step_001"})
	artifact := contracts.Artifact{ID: "artifact_001", JobID: job.ID, StepID: "step_001", Type: contracts.ArtifactJSON, Path: "artifacts/http/probe.json", SizeBytes: 12}
	evidence := contracts.Evidence{ID: "evidence_001", JobID: job.ID, StepID: "step_001", ArtifactID: artifact.ID, Type: contracts.EvidenceHTTPResponse, Title: "HTTP status", Summary: "200 OK", Confidence: contracts.ConfidenceHigh, CreatedAt: contracts.Now()}
	finding := contracts.Finding{ID: "finding_001", JobID: job.ID, Title: "Missing CSP", Severity: contracts.SeverityLow, Confidence: contracts.ConfidenceHigh, Category: "headers", AffectedTargets: []string{target.Value}, EvidenceIDs: []string{evidence.ID}, Description: "CSP is missing.", Impact: "Browser hardening is weaker.", Reproduction: []string{"curl -I http://localhost:8080"}, Remediation: "Add a CSP header.", Source: contracts.FindingSource{Tool: "builtin.header_checker", StepID: "step_001"}, CreatedAt: contracts.Now()}
	generated := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	output, err := Generate(workspace, Input{Job: job, Target: target, Scope: scope, Plan: plan, Artifacts: []contracts.Artifact{artifact}, Evidence: []contracts.Evidence{evidence}, Findings: []contracts.Finding{finding}, SkippedSteps: []string{"step_skipped_001"}, Limitations: []string{"custom limitation"}, GeneratedTime: generated})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if output.Report.JobID != job.ID || output.Report.GeneratedAt != generated || len(output.Report.FindingIDs) != 1 || output.Report.FindingIDs[0] != finding.ID {
		t.Fatalf("unexpected report contract: %#v", output.Report)
	}
	if output.MDPath != filepath.Join(workspace, "reports", "report.md") || output.JSONPath != filepath.Join(workspace, "reports", "report.json") {
		t.Fatalf("unexpected paths: %#v", output)
	}

	md, err := os.ReadFile(output.MDPath)
	if err != nil {
		t.Fatalf("read markdown: %v", err)
	}
	markdown := string(md)
	for _, want := range []string{"# CyberAgent Report", "Missing CSP", "evidence_001", "step_skipped_001", "custom limitation"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}

	var reportJSON struct {
		Job       contracts.Job        `json:"job"`
		Target    contracts.Target     `json:"target"`
		Scope     contracts.Scope      `json:"scope"`
		Plan      contracts.Plan       `json:"plan"`
		Evidence  []contracts.Evidence `json:"evidence"`
		Findings  []contracts.Finding  `json:"findings"`
		Artifacts []contracts.Artifact `json:"artifacts"`
	}
	readJSONReport(t, output.JSONPath, &reportJSON)
	if reportJSON.Job.ID != job.ID || reportJSON.Target.ID != target.ID || len(reportJSON.Findings) != 1 || reportJSON.Findings[0].ID != finding.ID {
		t.Fatalf("unexpected report json: %#v", reportJSON)
	}
}

func TestBuildEvidenceIndex(t *testing.T) {
	artifact := contracts.Artifact{ID: "artifact_001"}
	evidence := contracts.Evidence{ID: "evidence_001"}
	index := BuildEvidenceIndex([]contracts.Artifact{artifact}, []contracts.Evidence{evidence})
	if len(index.Artifacts) != 1 || index.Artifacts[0].ID != artifact.ID || len(index.Evidence) != 1 || index.Evidence[0].ID != evidence.ID {
		t.Fatalf("unexpected index: %#v", index)
	}
}

func readJSONReport(t *testing.T, path string, out any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
