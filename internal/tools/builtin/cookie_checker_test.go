package builtin

import (
	"net/http"
	"testing"
)

func TestCheckCookiesFindsMissingAttributes(t *testing.T) {
	headers := http.Header{}
	headers.Add("Set-Cookie", "session=abc; Path=/")
	result := CheckCookies(CookieCheckInput{JobID: "job_001", StepID: "step_cookie_check_001", ArtifactID: "artifact_headers_001", TargetRef: "https://example.test/", Headers: headers, HTTPS: true})
	if len(result.Evidence) != 1 {
		t.Fatalf("evidence count = %d, want 1", len(result.Evidence))
	}
	ids := map[string]bool{}
	for _, finding := range result.Findings {
		ids[finding.ID] = true
		if len(finding.EvidenceIDs) == 0 {
			t.Fatalf("finding without evidence: %#v", finding)
		}
	}
	for _, want := range []string{"finding_missing_secure_cookie_session_001", "finding_missing_httponly_cookie_session_001", "finding_missing_samesite_cookie_session_001"} {
		if !ids[want] {
			t.Fatalf("missing finding %s in %#v", want, ids)
		}
	}
}

func TestCheckCookiesNoFindingsForSecureCookie(t *testing.T) {
	headers := http.Header{}
	headers.Add("Set-Cookie", "session=abc; Path=/; Secure; HttpOnly; SameSite=Lax")
	result := CheckCookies(CookieCheckInput{JobID: "job_001", StepID: "step_cookie_check_001", TargetRef: "https://example.test/", Headers: headers, HTTPS: true})
	if len(result.Evidence) != 1 {
		t.Fatalf("evidence count = %d, want 1", len(result.Evidence))
	}
	if len(result.Findings) != 0 {
		t.Fatalf("unexpected findings: %#v", result.Findings)
	}
}
