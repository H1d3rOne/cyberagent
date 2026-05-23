package builtin

import (
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestExtractHTML(t *testing.T) {
	html := []byte(`<!doctype html>
<html><head><script src="/app.js"></script></head>
<body>
<a href="/admin">Admin</a>
<a href="javascript:alert(1)">skip</a>
<form action="/login" method="post"></form>
<!-- build: dev -->
</body></html>`)
	result, err := ExtractHTML(HTMLExtractInput{JobID: "job_001", StepID: "step_html_extract_001", ArtifactID: "artifact_html_001", BaseURL: "http://example.test/base", HTML: html})
	if err != nil {
		t.Fatalf("ExtractHTML() error = %v", err)
	}
	if len(result.Links) != 1 || result.Links[0].Resolved != "http://example.test/admin" {
		t.Fatalf("unexpected links: %#v", result.Links)
	}
	if len(result.Scripts) != 1 || result.Scripts[0].Resolved != "http://example.test/app.js" {
		t.Fatalf("unexpected scripts: %#v", result.Scripts)
	}
	if len(result.Forms) != 1 || result.Forms[0].Action.Resolved != "http://example.test/login" || result.Forms[0].Method != "POST" {
		t.Fatalf("unexpected forms: %#v", result.Forms)
	}
	if len(result.Comments) != 1 || result.Comments[0] != "build: dev" {
		t.Fatalf("unexpected comments: %#v", result.Comments)
	}
	if len(result.Evidence) != 3 {
		t.Fatalf("evidence count = %d, want 3", len(result.Evidence))
	}
	var formEvidence bool
	for _, evidence := range result.Evidence {
		if evidence.JobID != "job_001" || evidence.ArtifactID != "artifact_html_001" {
			t.Fatalf("unexpected evidence ownership: %#v", evidence)
		}
		if evidence.Type == contracts.EvidenceHTMLForm {
			formEvidence = true
		}
	}
	if !formEvidence {
		t.Fatalf("expected HTML form evidence: %#v", result.Evidence)
	}
}

func TestExtractHTMLRejectsInvalidBaseURL(t *testing.T) {
	_, err := ExtractHTML(HTMLExtractInput{BaseURL: "://bad", HTML: []byte(`<a href="/x">x</a>`)})
	if err == nil {
		t.Fatalf("expected invalid base URL error")
	}
}
