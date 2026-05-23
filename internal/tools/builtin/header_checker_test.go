package builtin

import (
	"net/http"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestCheckSecurityHeadersFindsMissingHeaders(t *testing.T) {
	result := CheckSecurityHeaders(HeaderCheckInput{
		JobID:      "job_001",
		StepID:     "step_header_check_001",
		ArtifactID: "artifact_headers_001",
		TargetRef:  "https://example.test/",
		Headers:    http.Header{},
		HTTPS:      true,
	})
	if len(result.Evidence) == 0 || len(result.Findings) == 0 {
		t.Fatalf("expected evidence and findings, got evidence=%d findings=%d", len(result.Evidence), len(result.Findings))
	}
	assertFinding(t, result.Findings, "finding_missing_hsts_001")
	for _, finding := range result.Findings {
		if len(finding.EvidenceIDs) == 0 {
			t.Fatalf("finding without evidence: %#v", finding)
		}
		if finding.Source.Tool != "builtin.header_checker" {
			t.Fatalf("unexpected finding source: %#v", finding.Source)
		}
	}
}

func TestCheckSecurityHeadersPermissiveCORS(t *testing.T) {
	headers := http.Header{}
	headers.Set("Access-Control-Allow-Origin", "*")
	result := CheckSecurityHeaders(HeaderCheckInput{JobID: "job_001", StepID: "step_001", TargetRef: "http://example.test/", Headers: headers})
	assertFinding(t, result.Findings, "finding_permissive_cors_001")
}

func TestCheckSecurityHeadersNoHSTSForHTTP(t *testing.T) {
	result := CheckSecurityHeaders(HeaderCheckInput{JobID: "job_001", StepID: "step_001", TargetRef: "http://example.test/", Headers: http.Header{}, HTTPS: false})
	for _, finding := range result.Findings {
		if finding.ID == "finding_missing_hsts_001" {
			t.Fatalf("did not expect HSTS finding for non-HTTPS target")
		}
	}
}

func assertFinding(t *testing.T, findings []contracts.Finding, id string) {
	t.Helper()
	for _, finding := range findings {
		if finding.ID == id {
			return
		}
	}
	t.Fatalf("finding %s not found in %#v", id, findings)
}
