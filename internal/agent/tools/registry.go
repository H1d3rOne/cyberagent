package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/tools/builtin"
)

// ToolFunc executes a tool and returns a JSON-serializable result.
type ToolFunc func(ctx context.Context, input json.RawMessage) (string, error)

// RegisteredTool pairs a schema with its executor.
type RegisteredTool struct {
	Def     llm.ToolDef
	Execute ToolFunc
}

// Registry holds all tools available to the agent.
type Registry struct {
	tools map[string]RegisteredTool
}

func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]RegisteredTool)}
	r.registerBuiltins()
	return r
}

func (r *Registry) Get(name string) (RegisteredTool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Definitions() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Def)
	}
	return defs
}

func (r *Registry) Register(name string, tool RegisteredTool) {
	r.tools[name] = tool
}

func (r *Registry) registerBuiltins() {
	r.Register("http_probe", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "http_probe",
			Description: "Send an HTTP request to probe a URL. Returns status code, headers, redirect chain, and response time. Use this to check if a target is reachable and get initial fingerprinting data.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"The URL to probe"},"method":{"type":"string","enum":["GET","HEAD"],"default":"GET"}},"required":["url"]}`),
		},
		Execute: execHTTPProbe,
	})

	r.Register("http_fetch", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "http_fetch",
			Description: "Fetch the full content of a URL (HTML body + headers). Use this to retrieve page content for analysis. Limited to 2MB response size.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"The URL to fetch"}},"required":["url"]}`),
		},
		Execute: execHTTPFetch,
	})

	r.Register("check_headers", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "check_headers",
			Description: "Analyze HTTP response headers for security issues. Checks CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, and CORS configuration.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"URL whose headers to check"}},"required":["url"]}`),
		},
		Execute: execCheckHeaders,
	})

	r.Register("check_cookies", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "check_cookies",
			Description: "Analyze cookies from HTTP response for security attributes. Checks Secure, HttpOnly, SameSite, and prefix conventions.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"URL whose cookies to check"}},"required":["url"]}`),
		},
		Execute: execCheckCookies,
	})

	r.Register("extract_html", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "extract_html",
			Description: "Extract links, forms, scripts, and meta tags from an HTML page. Useful for discovering attack surface and understanding page structure.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"URL to fetch and extract from"}},"required":["url"]}`),
		},
		Execute: execExtractHTML,
	})

	r.Register("discover_paths", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "discover_paths",
			Description: "Run a small wordlist content discovery against a base URL. Checks common admin panels, backup files, and sensitive paths.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"Base URL to discover paths on"}},"required":["url"]}`),
		},
		Execute: execDiscoverPaths,
	})

	r.Register("inspect_tls", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "inspect_tls",
			Description: "Inspect TLS certificate and configuration of an HTTPS target. Returns certificate chain, expiry, protocol version, and cipher suite.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"HTTPS URL to inspect"}},"required":["url"]}`),
		},
		Execute: execInspectTLS,
	})

	r.Register("check_ports", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "check_ports",
			Description: "Check TCP port connectivity on a host. Tests common web ports (80, 443, 8080, 8443, 3000, etc.) and reports which are open.",
			InputSchema: schema(`{"type":"object","properties":{"host":{"type":"string","description":"Hostname or IP to check"}},"required":["host"]}`),
		},
		Execute: execCheckPorts,
	})

	r.Register("fetch_robots", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "fetch_robots",
			Description: "Fetch and parse robots.txt from a target. Reveals disallowed paths that may contain sensitive endpoints.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"Base URL (robots.txt will be fetched from /robots.txt)"}},"required":["url"]}`),
		},
		Execute: execFetchRobots,
	})

	r.Register("extract_js_routes", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "extract_js_routes",
			Description: "Extract API routes and endpoints from JavaScript source code at a URL. Discovers hidden API endpoints not visible in HTML.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"URL of the JavaScript file to analyze"}},"required":["url"]}`),
		},
		Execute: execExtractJSRoutes,
	})

	r.Register("dns_lookup", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "dns_lookup",
			Description: "Perform DNS lookup on a hostname. Returns A, AAAA, CNAME, MX, TXT, and NS records. Useful for understanding infrastructure and finding related hosts.",
			InputSchema: schema(`{"type":"object","properties":{"host":{"type":"string","description":"Hostname to look up"}},"required":["host"]}`),
		},
		Execute: execDNSLookup,
	})

	r.Register("fetch_sitemap", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "fetch_sitemap",
			Description: "Fetch and parse sitemap.xml from a target. Reveals all publicly listed URLs which may include hidden or forgotten pages.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"Base URL (sitemap.xml will be fetched from /sitemap.xml)"}},"required":["url"]}`),
		},
		Execute: execFetchSitemap,
	})

	r.Register("whois_lookup", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "whois_lookup",
			Description: "Perform WHOIS lookup on a domain. Returns registration info, registrar, creation/expiry dates, and nameservers.",
			InputSchema: schema(`{"type":"object","properties":{"domain":{"type":"string","description":"Domain name to look up"}},"required":["domain"]}`),
		},
		Execute: execWhoisLookup,
	})

	r.Register("add_finding", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "add_finding",
			Description: "Record a security finding. Use this when you've identified a vulnerability or security issue with supporting evidence.",
			InputSchema: schema(`{"type":"object","properties":{"title":{"type":"string","description":"Short title of the finding"},"severity":{"type":"string","enum":["info","low","medium","high","critical"]},"description":{"type":"string","description":"Detailed description of the issue"},"evidence":{"type":"string","description":"Evidence supporting this finding"},"remediation":{"type":"string","description":"Recommended fix"}},"required":["title","severity","description","evidence"]}`),
		},
		Execute: nil,
	})

	r.Register("generate_report", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "generate_report",
			Description: "Generate a final security assessment report from all collected findings. Call this when you have completed your analysis and want to produce the final output.",
			InputSchema: schema(`{"type":"object","properties":{"summary":{"type":"string","description":"Executive summary of the assessment"}},"required":["summary"]}`),
		},
		Execute: nil,
	})

	r.Register("run_nuclei", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "run_nuclei",
			Description: "Run nuclei vulnerability scanner with specific templates against the target. Requires nuclei binary installed. This is a high-risk tool that requires confirmation.",
			InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"Target URL to scan"},"tags":{"type":"array","items":{"type":"string"},"description":"Template tags to filter (e.g. cve, exposure, misconfig)"},"severity":{"type":"array","items":{"type":"string","enum":["info","low","medium","high","critical"]},"description":"Severity levels to include"}},"required":["url"]}`),
		},
		Execute: execRunNuclei,
	})
}

func schema(s string) json.RawMessage { return json.RawMessage(s) }

func execHTTPProbe(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	result, err := builtin.HTTPProbe(ctx, builtin.HTTPProbeOptions{URL: params.URL, Timeout: 30_000_000_000, MaxRedirects: 5})
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

func execHTTPFetch(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	result, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: params.URL, Timeout: 30_000_000_000, MaxBytes: 2 * 1024 * 1024, MaxRedirects: 5})
	if err != nil {
		return "", err
	}
	summary := map[string]interface{}{
		"status_code":  result.StatusCode,
		"content_type": result.ContentType,
		"body_size":    len(result.Body),
		"body_preview": truncate(string(result.Body), 4000),
		"headers":      result.Headers,
	}
	out, _ := json.Marshal(summary)
	return string(out), nil
}

func execCheckHeaders(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	fetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: params.URL, Timeout: 30_000_000_000, MaxBytes: 64 * 1024, MaxRedirects: 5})
	if err != nil {
		return "", err
	}
	isHTTPS := len(params.URL) > 5 && params.URL[:5] == "https"
	result := builtin.CheckSecurityHeaders(builtin.HeaderCheckInput{JobID: "agent", StepID: "agent", ArtifactID: "agent", TargetRef: params.URL, Headers: http.Header(fetch.Headers), HTTPS: isHTTPS})
	out, _ := json.Marshal(map[string]interface{}{"findings": result.Findings, "evidence_count": len(result.Evidence)})
	return string(out), nil
}

func execCheckCookies(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	fetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: params.URL, Timeout: 30_000_000_000, MaxBytes: 64 * 1024, MaxRedirects: 5})
	if err != nil {
		return "", err
	}
	isHTTPS := len(params.URL) > 5 && params.URL[:5] == "https"
	result := builtin.CheckCookies(builtin.CookieCheckInput{JobID: "agent", StepID: "agent", ArtifactID: "agent", TargetRef: params.URL, Headers: http.Header(fetch.Headers), HTTPS: isHTTPS})
	out, _ := json.Marshal(map[string]interface{}{"findings": result.Findings, "evidence_count": len(result.Evidence)})
	return string(out), nil
}

func execExtractHTML(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	fetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: params.URL, Timeout: 30_000_000_000, MaxBytes: 2 * 1024 * 1024, MaxRedirects: 5})
	if err != nil {
		return "", err
	}
	result, err := builtin.ExtractHTML(builtin.HTMLExtractInput{JobID: "agent", StepID: "agent", ArtifactID: "agent", BaseURL: params.URL, HTML: fetch.Body})
	if err != nil {
		return "", err
	}
	summary := map[string]interface{}{
		"links_count":   len(result.Links),
		"forms_count":   len(result.Forms),
		"scripts_count": len(result.Scripts),
		"links":         truncateSlice(formatURLs(result.Links), 50),
		"scripts":       truncateSlice(formatURLs(result.Scripts), 20),
	}
	out, _ := json.Marshal(summary)
	return string(out), nil
}

func execDiscoverPaths(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	result := builtin.DiscoverContentSmall(ctx, builtin.ContentDiscoveryInput{JobID: "agent", StepID: "agent", BaseURL: params.URL, Timeout: 60_000_000_000, Allowed: func(string) bool { return true }})
	out, _ := json.Marshal(map[string]interface{}{"found": result.Results, "evidence_count": len(result.Evidence)})
	return string(out), nil
}

func execInspectTLS(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	result := builtin.TLSInspect(ctx, builtin.TLSInspectOptions{URL: params.URL, Timeout: 15_000_000_000}, "agent", "agent")
	out, _ := json.Marshal(map[string]interface{}{"evidence": result.Evidence, "findings": result.Findings, "error": result.Error})
	return string(out), nil
}

func execCheckPorts(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Host string `json:"host"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	result := builtin.TCPPortCheck(ctx, builtin.TCPPortCheckOptions{Host: params.Host, Ports: builtin.CommonWebPorts, Timeout: 30_000_000_000}, "agent", "agent")
	out, _ := json.Marshal(map[string]interface{}{"evidence": result.Evidence})
	return string(out), nil
}

func execFetchRobots(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	robotsURL := params.URL + "/robots.txt"
	fetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: robotsURL, Timeout: 20_000_000_000, MaxBytes: 256 * 1024, MaxRedirects: 2})
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to fetch robots.txt: %s"}`, err.Error()), nil
	}
	if fetch.StatusCode != 200 {
		return fmt.Sprintf(`{"status": %d, "message": "robots.txt not found"}`, fetch.StatusCode), nil
	}
	parsed := builtin.ParseRobots("agent", "agent", "", robotsURL, fetch.Body)
	out, _ := json.Marshal(map[string]interface{}{"content": truncate(string(fetch.Body), 3000), "evidence": parsed.Evidence})
	return string(out), nil
}

func execExtractJSRoutes(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	fetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: params.URL, Timeout: 30_000_000_000, MaxBytes: 1024 * 1024, MaxRedirects: 2})
	if err != nil {
		return "", err
	}
	sources := map[string][]byte{"js_source": fetch.Body}
	result, err := builtin.ExtractJSRoutesLight(builtin.JSRouteInput{JobID: "agent", StepID: "agent", ArtifactID: "agent", BaseURL: params.URL, Sources: sources})
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]interface{}{"routes": result.Routes, "evidence_count": len(result.Evidence)})
	return string(out), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}

func execDNSLookup(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Host string `json:"host"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	result := builtin.DNSLookup(ctx, builtin.DNSLookupOptions{Host: params.Host, Timeout: 10_000_000_000}, "agent", "agent")
	out, _ := json.Marshal(map[string]interface{}{
		"host":  result.Host,
		"a":     result.A,
		"aaaa":  result.AAAA,
		"cname": result.CNAME,
		"mx":    result.MX,
		"txt":   result.TXT,
		"ns":    result.NS,
	})
	return string(out), nil
}

func execFetchSitemap(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	sitemapURL := params.URL + "/sitemap.xml"
	fetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: sitemapURL, Timeout: 20_000_000_000, MaxBytes: 512 * 1024, MaxRedirects: 2})
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to fetch sitemap.xml: %s"}`, err.Error()), nil
	}
	if fetch.StatusCode != 200 {
		return fmt.Sprintf(`{"status": %d, "message": "sitemap.xml not found"}`, fetch.StatusCode), nil
	}
	parsed := builtin.ParseSitemap("agent", "agent", "agent", sitemapURL, fetch.Body)
	urls := parsed.URLs
	if len(urls) > 100 {
		urls = urls[:100]
	}
	out, _ := json.Marshal(map[string]interface{}{"urls": urls, "total_count": len(parsed.URLs)})
	return string(out), nil
}

func execWhoisLookup(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	result := builtin.WhoisLookup(ctx, builtin.WhoisOptions{Domain: params.Domain, Timeout: 10_000_000_000})
	if result.Error != "" {
		out, _ := json.Marshal(map[string]interface{}{"domain": result.Domain, "error": result.Error})
		return string(out), nil
	}
	raw := result.Raw
	if len(raw) > 3000 {
		raw = raw[:3000] + "...[truncated]"
	}
	out, _ := json.Marshal(map[string]interface{}{"domain": result.Domain, "whois": raw})
	return string(out), nil
}

func execRunNuclei(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL      string   `json:"url"`
		Tags     []string `json:"tags"`
		Severity []string `json:"severity"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}
	result := builtin.RunNuclei(ctx, builtin.NucleiOptions{
		Target:   params.URL,
		Tags:     params.Tags,
		Severity: params.Severity,
		Timeout:  120_000_000_000,
	})
	if result.Error != "" {
		out, _ := json.Marshal(map[string]interface{}{"error": result.Error})
		return string(out), nil
	}
	out, _ := json.Marshal(map[string]interface{}{
		"findings_count": len(result.Findings),
		"findings":       result.Findings,
	})
	return string(out), nil
}

func truncateSlice(s []string, max int) []string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func formatURLs(urls []builtin.ExtractedURL) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		out = append(out, u.Resolved)
	}
	return out
}

// FindingInput is the structured input for the add_finding tool.
type FindingInput struct {
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
	Remediation string `json:"remediation,omitempty"`
}

// ParseFindingInput extracts a FindingInput from raw JSON.
func ParseFindingInput(input json.RawMessage) (FindingInput, error) {
	var f FindingInput
	if err := json.Unmarshal(input, &f); err != nil {
		return FindingInput{}, err
	}
	if f.Title == "" || f.Severity == "" {
		return FindingInput{}, fmt.Errorf("title and severity are required")
	}
	return f, nil
}

// ToFinding converts a FindingInput to a contracts.Finding.
func (f FindingInput) ToFinding(id string, target string) contracts.Finding {
	return contracts.Finding{
		ID:              id,
		Title:           f.Title,
		Severity:        contracts.Severity(f.Severity),
		Confidence:      contracts.ConfidenceMedium,
		Category:        "agent/analysis",
		AffectedTargets: []string{target},
		Description:     f.Description,
		Impact:          f.Description,
		Remediation:     f.Remediation,
		Source:          contracts.FindingSource{Tool: "agent.llm"},
		CreatedAt:       contracts.Now(),
	}
}
