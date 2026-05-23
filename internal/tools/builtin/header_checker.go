package builtin

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// HeaderCheckInput contains headers and ownership metadata for evidence/finding generation.
type HeaderCheckInput struct {
	JobID      string
	StepID     string
	ArtifactID string
	TargetRef  string
	Headers    http.Header
	HTTPS      bool
}

// HeaderCheckResult is the output of the built-in security header checker.
type HeaderCheckResult struct {
	Evidence []contracts.Evidence
	Findings []contracts.Finding
}

// CheckSecurityHeaders creates evidence and low-risk findings for common security header issues.
func CheckSecurityHeaders(input HeaderCheckInput) HeaderCheckResult {
	result := HeaderCheckResult{}
	add := func(slug, title, summary string, data map[string]any, severity contracts.Severity, confidence contracts.Confidence, impact, remediation string) {
		evidenceID := "evidence_" + slug + "_001"
		findingID := "finding_" + slug + "_001"
		result.Evidence = append(result.Evidence, contracts.Evidence{
			ID:         evidenceID,
			JobID:      input.JobID,
			StepID:     input.StepID,
			ArtifactID: input.ArtifactID,
			Type:       contracts.EvidenceHTTPHeader,
			Title:      title,
			Summary:    summary,
			TargetRef:  input.TargetRef,
			Data:       data,
			Confidence: confidence,
			CreatedAt:  contracts.Now(),
		})
		result.Findings = append(result.Findings, contracts.Finding{
			ID:              findingID,
			JobID:           input.JobID,
			Title:           title,
			Severity:        severity,
			Confidence:      confidence,
			Category:        "security_headers",
			AffectedTargets: []string{input.TargetRef},
			EvidenceIDs:     []string{evidenceID},
			Description:     summary,
			Impact:          impact,
			Reproduction:    []string{fmt.Sprintf("curl -I %s", input.TargetRef)},
			Remediation:     remediation,
			Source:          contracts.FindingSource{Tool: "builtin.header_checker", StepID: input.StepID},
			CreatedAt:       contracts.Now(),
		})
	}

	if input.HTTPS && input.Headers.Get("Strict-Transport-Security") == "" {
		add("missing_hsts", "Missing HSTS Header", "Strict-Transport-Security header is not present on an HTTPS response.", map[string]any{"header": "Strict-Transport-Security", "present": false}, contracts.SeverityLow, contracts.ConfidenceHigh, "Browsers may not enforce HTTPS-only access for this host.", "Enable HSTS on HTTPS responses with an appropriate max-age value.")
	}
	if input.Headers.Get("X-Content-Type-Options") == "" {
		add("missing_x_content_type_options", "Missing X-Content-Type-Options Header", "X-Content-Type-Options header is not present.", map[string]any{"header": "X-Content-Type-Options", "present": false}, contracts.SeverityLow, contracts.ConfidenceHigh, "Browsers may MIME-sniff responses in some contexts.", "Set X-Content-Type-Options: nosniff.")
	}
	if input.Headers.Get("Content-Security-Policy") == "" && input.Headers.Get("X-Frame-Options") == "" {
		add("missing_frame_protection", "Missing Frame Protection Headers", "Neither Content-Security-Policy frame-ancestors nor X-Frame-Options is present.", map[string]any{"headers": []string{"Content-Security-Policy", "X-Frame-Options"}, "present": false}, contracts.SeverityLow, contracts.ConfidenceMedium, "Pages may be more exposed to clickjacking depending on application context.", "Set CSP frame-ancestors and/or X-Frame-Options as appropriate.")
	}
	if input.Headers.Get("Referrer-Policy") == "" {
		add("missing_referrer_policy", "Missing Referrer-Policy Header", "Referrer-Policy header is not present.", map[string]any{"header": "Referrer-Policy", "present": false}, contracts.SeverityInfo, contracts.ConfidenceHigh, "Browsers may send more referrer information than intended.", "Set a Referrer-Policy such as strict-origin-when-cross-origin.")
	}
	if isPermissiveCORS(input.Headers) {
		add("permissive_cors", "Permissive CORS Policy", "Access-Control-Allow-Origin is wildcard or reflected with credential-like settings.", map[string]any{"access_control_allow_origin": input.Headers.Get("Access-Control-Allow-Origin"), "access_control_allow_credentials": input.Headers.Get("Access-Control-Allow-Credentials")}, contracts.SeverityLow, contracts.ConfidenceMedium, "Overly permissive CORS can expose API responses to unintended origins depending on credentials and browser behavior.", "Restrict CORS origins to trusted origins and avoid wildcard policies for sensitive APIs.")
	}
	return result
}

func isPermissiveCORS(headers http.Header) bool {
	origin := strings.TrimSpace(headers.Get("Access-Control-Allow-Origin"))
	if origin == "*" {
		return true
	}
	if origin == "" {
		return false
	}
	credentials := strings.EqualFold(strings.TrimSpace(headers.Get("Access-Control-Allow-Credentials")), "true")
	if credentials && regexp.MustCompile(`(?i)^https?://`).MatchString(origin) {
		return true
	}
	return false
}
