package builtin

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// DefaultContentDiscoveryPaths is the MVP small low-rate wordlist.
var DefaultContentDiscoveryPaths = []string{"/admin", "/login", "/api", "/graphql", "/health", "/debug", "/status"}

// ContentDiscoveryInput controls small content discovery.
type ContentDiscoveryInput struct {
	JobID   string
	StepID  string
	BaseURL string
	Paths   []string
	Timeout time.Duration
	Allowed func(string) bool
}

// ContentDiscoveryResult is the small discovery output.
type ContentDiscoveryResult struct {
	Evidence []contracts.Evidence
	Results  []HTTPProbeResult
	Skipped  []string
}

// DiscoverContentSmall probes a small list of common paths with low request volume.
func DiscoverContentSmall(ctx context.Context, input ContentDiscoveryInput) ContentDiscoveryResult {
	paths := input.Paths
	if len(paths) == 0 {
		paths = DefaultContentDiscoveryPaths
	}
	base, err := url.Parse(input.BaseURL)
	if err != nil {
		return ContentDiscoveryResult{Skipped: paths}
	}
	result := ContentDiscoveryResult{Results: []HTTPProbeResult{}, Skipped: []string{}}
	idx := 1
	for _, p := range paths {
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		u := *base
		u.Path = p
		u.RawQuery = ""
		candidate := u.String()
		if input.Allowed != nil && !input.Allowed(candidate) {
			result.Skipped = append(result.Skipped, candidate)
			continue
		}
		probe, err := HTTPProbe(ctx, HTTPProbeOptions{URL: candidate, Methods: []string{http.MethodGet}, Timeout: input.Timeout, MaxRedirects: 1})
		if err != nil {
			result.Skipped = append(result.Skipped, candidate)
			continue
		}
		if probe.StatusCode > 0 && probe.StatusCode < 400 {
			result.Results = append(result.Results, probe)
			result.Evidence = append(result.Evidence, contracts.Evidence{ID: fmtEvidenceID("content_discovery", idx), JobID: input.JobID, StepID: input.StepID, Type: contracts.EvidenceHTTPResponse, Title: "Interesting path discovered", Summary: probe.Status + " at " + candidate, TargetRef: candidate, Data: map[string]any{"url": candidate, "status_code": probe.StatusCode}, Confidence: contracts.ConfidenceMedium, CreatedAt: contracts.Now()})
			idx++
		}
	}
	return result
}
