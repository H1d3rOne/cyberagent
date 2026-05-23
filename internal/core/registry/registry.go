package registry

import "sync"

type ToolType string

const (
	ToolTypeBuiltin ToolType = "builtin"
	ToolTypePlugin  ToolType = "plugin"
)

type RiskLevel string

const (
	RiskInfo     RiskLevel = "info"
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type ToolEntry struct {
	Name         string
	Type         ToolType
	Risk         RiskLevel
	Description  string
	Capabilities []string
	Enabled      bool
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]ToolEntry
}

func New() *Registry {
	r := &Registry{tools: make(map[string]ToolEntry)}
	r.registerDefaults()
	return r
}

func (r *Registry) Register(entry ToolEntry) {
	r.mu.Lock()
	r.tools[entry.Name] = entry
	r.mu.Unlock()
}

func (r *Registry) Get(name string) (ToolEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.tools[name]
	return entry, ok
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

func (r *Registry) List() []ToolEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolEntry, 0, len(r.tools))
	for _, entry := range r.tools {
		out = append(out, entry)
	}
	return out
}

func (r *Registry) ListByType(t ToolType) []ToolEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []ToolEntry
	for _, entry := range r.tools {
		if entry.Type == t {
			out = append(out, entry)
		}
	}
	return out
}

func (r *Registry) ListEnabled() []ToolEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []ToolEntry
	for _, entry := range r.tools {
		if entry.Enabled {
			out = append(out, entry)
		}
	}
	return out
}

func (r *Registry) Enable(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.tools[name]
	if !ok {
		return false
	}
	entry.Enabled = true
	r.tools[name] = entry
	return true
}

func (r *Registry) Disable(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.tools[name]
	if !ok {
		return false
	}
	entry.Enabled = false
	r.tools[name] = entry
	return true
}

func (r *Registry) registerDefaults() {
	builtins := []ToolEntry{
		{Name: "builtin.scope_guard", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Verify target is within authorized scope", Capabilities: []string{"scope_check"}, Enabled: true},
		{Name: "builtin.target_normalizer", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Normalize target URL", Capabilities: []string{"normalize"}, Enabled: true},
		{Name: "builtin.doctor", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Check runtime dependencies", Capabilities: []string{"health_check"}, Enabled: true},
		{Name: "builtin.http_probe", Type: ToolTypeBuiltin, Risk: RiskLow, Description: "HTTP HEAD/GET probe for status and redirects", Capabilities: []string{"http", "probe"}, Enabled: true},
		{Name: "builtin.http_fetcher", Type: ToolTypeBuiltin, Risk: RiskLow, Description: "Fetch page content with size limits", Capabilities: []string{"http", "fetch"}, Enabled: true},
		{Name: "builtin.header_checker", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Check security response headers", Capabilities: []string{"analysis", "headers"}, Enabled: true},
		{Name: "builtin.cookie_checker", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Check cookie security attributes", Capabilities: []string{"analysis", "cookies"}, Enabled: true},
		{Name: "builtin.robots_fetcher", Type: ToolTypeBuiltin, Risk: RiskLow, Description: "Fetch and parse robots.txt", Capabilities: []string{"http", "discovery"}, Enabled: true},
		{Name: "builtin.sitemap_fetcher", Type: ToolTypeBuiltin, Risk: RiskLow, Description: "Fetch and parse sitemap.xml", Capabilities: []string{"http", "discovery"}, Enabled: true},
		{Name: "builtin.html_extractor", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Extract links, forms, scripts from HTML", Capabilities: []string{"analysis", "html"}, Enabled: true},
		{Name: "builtin.js_asset_collector", Type: ToolTypeBuiltin, Risk: RiskLow, Description: "Fetch external JS assets", Capabilities: []string{"http", "js"}, Enabled: true},
		{Name: "builtin.js_route_extractor_light", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Extract routes from JS sources", Capabilities: []string{"analysis", "js", "routes"}, Enabled: true},
		{Name: "builtin.api_hint_detector", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Detect API endpoint hints", Capabilities: []string{"analysis", "api"}, Enabled: true},
		{Name: "builtin.content_discovery_small", Type: ToolTypeBuiltin, Risk: RiskLow, Description: "Small wordlist directory discovery", Capabilities: []string{"http", "discovery", "brute"}, Enabled: true},
		{Name: "builtin.tls_inspector", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Inspect TLS certificates and protocol", Capabilities: []string{"tls", "analysis"}, Enabled: true},
		{Name: "builtin.tcp_port_check", Type: ToolTypeBuiltin, Risk: RiskLow, Description: "Check TCP port connectivity", Capabilities: []string{"network", "probe"}, Enabled: true},
		{Name: "builtin.evidence_indexer", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Index and hash evidence", Capabilities: []string{"evidence"}, Enabled: true},
		{Name: "builtin.finding_normalizer", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Normalize and deduplicate findings", Capabilities: []string{"findings"}, Enabled: true},
		{Name: "builtin.report_generator", Type: ToolTypeBuiltin, Risk: RiskInfo, Description: "Generate Markdown/JSON reports", Capabilities: []string{"report"}, Enabled: true},
	}
	for _, entry := range builtins {
		r.tools[entry.Name] = entry
	}
}
