package builtin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// CookieCheckInput contains Set-Cookie headers and ownership metadata.
type CookieCheckInput struct {
	JobID      string
	StepID     string
	ArtifactID string
	TargetRef  string
	Headers    http.Header
	HTTPS      bool
}

// CookieCheckResult is the output of Cookie Checker.
type CookieCheckResult struct {
	Evidence []contracts.Evidence
	Findings []contracts.Finding
}

// CheckCookies checks common cookie security attributes.
func CheckCookies(input CookieCheckInput) CookieCheckResult {
	result := CookieCheckResult{}
	cookies := input.Headers["Set-Cookie"]
	for i, raw := range cookies {
		cookie := parseSetCookie(raw)
		name := cookie["name"]
		if name == "" {
			name = fmt.Sprintf("cookie_%d", i+1)
		}
		base := sanitizeID(name)
		attrs := map[string]any{"cookie": name, "secure": cookie["secure"] == "true", "httponly": cookie["httponly"] == "true", "samesite": cookie["samesite"]}
		evidenceID := fmt.Sprintf("evidence_cookie_%s_%03d", base, i+1)
		result.Evidence = append(result.Evidence, contracts.Evidence{ID: evidenceID, JobID: input.JobID, StepID: input.StepID, ArtifactID: input.ArtifactID, Type: contracts.EvidenceCookieAttribute, Title: "Cookie attributes observed", Summary: "Cookie attributes observed for " + name, TargetRef: input.TargetRef, Data: attrs, Confidence: contracts.ConfidenceHigh, CreatedAt: contracts.Now()})
		if input.HTTPS && cookie["secure"] != "true" {
			result.Findings = append(result.Findings, cookieFinding(input, evidenceID, "missing_secure_cookie_"+base, "Cookie Missing Secure Attribute", name+" does not set the Secure attribute on an HTTPS response.", contracts.SeverityLow, "Cookies without Secure may be sent over non-HTTPS connections if reachable.", "Set the Secure attribute for cookies issued over HTTPS."))
		}
		if cookie["httponly"] != "true" {
			result.Findings = append(result.Findings, cookieFinding(input, evidenceID, "missing_httponly_cookie_"+base, "Cookie Missing HttpOnly Attribute", name+" does not set the HttpOnly attribute.", contracts.SeverityLow, "Client-side scripts may access the cookie if an XSS issue exists.", "Set the HttpOnly attribute where JavaScript access is not required."))
		}
		if cookie["samesite"] == "" {
			result.Findings = append(result.Findings, cookieFinding(input, evidenceID, "missing_samesite_cookie_"+base, "Cookie Missing SameSite Attribute", name+" does not set the SameSite attribute.", contracts.SeverityInfo, "The browser's default SameSite behavior may not match the application's intended CSRF posture.", "Set SameSite=Lax or SameSite=Strict where appropriate."))
		}
	}
	return result
}

func parseSetCookie(raw string) map[string]string {
	out := map[string]string{}
	parts := strings.Split(raw, ";")
	if len(parts) == 0 {
		return out
	}
	nameValue := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)
	out["name"] = strings.TrimSpace(nameValue[0])
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		if key == "" {
			continue
		}
		if len(kv) == 1 {
			out[key] = "true"
		} else {
			out[key] = strings.TrimSpace(kv[1])
		}
	}
	return out
}

func cookieFinding(input CookieCheckInput, evidenceID, slug, title, description string, severity contracts.Severity, impact, remediation string) contracts.Finding {
	return contracts.Finding{ID: "finding_" + slug + "_001", JobID: input.JobID, Title: title, Severity: severity, Confidence: contracts.ConfidenceHigh, Category: "cookie_security", AffectedTargets: []string{input.TargetRef}, EvidenceIDs: []string{evidenceID}, Description: description, Impact: impact, Reproduction: []string{fmt.Sprintf("curl -I %s", input.TargetRef)}, Remediation: remediation, Source: contracts.FindingSource{Tool: "builtin.cookie_checker", StepID: input.StepID}, CreatedAt: contracts.Now()}
}

func sanitizeID(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unknown"
	}
	return out
}
