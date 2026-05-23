package builtin

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

var routeRe = regexp.MustCompile(`(?i)(?:fetch|axios\.(?:get|post|put|delete|patch)|\$\.ajax)\s*\(?\s*["']([^"']+)["']|["']((?:/api|/graphql|/v[0-9]+/)[^"'\s<>]*)["']`)

// JSRouteInput contains JavaScript assets for route extraction.
type JSRouteInput struct {
	JobID      string
	StepID     string
	ArtifactID string
	BaseURL    string
	Sources    map[string][]byte
}

// JSRouteResult is the output of light JS route extraction.
type JSRouteResult struct {
	Routes   []ExtractedURL
	Evidence []contracts.Evidence
}

// ExtractJSRoutesLight extracts likely API routes using conservative regex patterns.
func ExtractJSRoutesLight(input JSRouteInput) (JSRouteResult, error) {
	base, err := url.Parse(input.BaseURL)
	if err != nil {
		return JSRouteResult{}, err
	}
	result := JSRouteResult{Routes: []ExtractedURL{}}
	seen := map[string]struct{}{}
	idx := 1
	for source, body := range input.Sources {
		matches := routeRe.FindAllSubmatch(body, -1)
		for _, match := range matches {
			raw := string(firstNonEmptyBytes(match[1], match[2]))
			if raw == "" {
				continue
			}
			extracted, ok := resolveExtractedURL(base, raw)
			if !ok {
				continue
			}
			if _, exists := seen[extracted.Resolved]; exists {
				continue
			}
			seen[extracted.Resolved] = struct{}{}
			result.Routes = append(result.Routes, extracted)
			result.Evidence = append(result.Evidence, contracts.Evidence{ID: evidenceIDJS("js_route", idx), JobID: input.JobID, StepID: input.StepID, ArtifactID: input.ArtifactID, Type: contracts.EvidenceJSRoute, Title: "JavaScript route discovered", Summary: "JavaScript route discovered: " + extracted.Resolved, TargetRef: extracted.Resolved, Data: map[string]any{"raw": extracted.Raw, "resolved": extracted.Resolved, "source": source}, Confidence: contracts.ConfidenceMedium, CreatedAt: contracts.Now()})
			idx++
		}
	}
	return result, nil
}

// DetectAPIHints creates info evidence for likely API endpoints.
func DetectAPIHints(jobID, stepID string, routes []ExtractedURL) []contracts.Evidence {
	evidence := []contracts.Evidence{}
	seen := map[string]struct{}{}
	idx := 1
	for _, route := range routes {
		lower := strings.ToLower(route.Resolved)
		if !strings.Contains(lower, "/api") && !strings.Contains(lower, "/graphql") && !regexp.MustCompile(`/v[0-9]+/`).MatchString(lower) {
			continue
		}
		if _, ok := seen[route.Resolved]; ok {
			continue
		}
		seen[route.Resolved] = struct{}{}
		evidence = append(evidence, contracts.Evidence{ID: evidenceIDJS("api_hint", idx), JobID: jobID, StepID: stepID, Type: contracts.EvidenceAPIHint, Title: "API endpoint hint discovered", Summary: "API endpoint hint discovered: " + route.Resolved, TargetRef: route.Resolved, Data: map[string]any{"raw": route.Raw, "resolved": route.Resolved}, Confidence: contracts.ConfidenceMedium, CreatedAt: contracts.Now()})
		idx++
	}
	return evidence
}

func firstNonEmptyBytes(values ...[]byte) []byte {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func evidenceIDJS(kind string, n int) string {
	return fmtEvidenceID(kind, n)
}
