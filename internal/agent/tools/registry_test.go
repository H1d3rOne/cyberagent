package tools

import (
	"encoding/json"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

func TestRegistryHasAllTools(t *testing.T) {
	r := NewRegistry()
	expected := []string{
		"http_probe", "http_fetch", "check_headers", "check_cookies",
		"extract_html", "discover_paths", "inspect_tls", "check_ports",
		"fetch_robots", "extract_js_routes", "dns_lookup", "fetch_sitemap",
		"whois_lookup", "add_finding", "generate_report", "run_nuclei",
	}
	for _, name := range expected {
		if _, ok := r.Get(name); !ok {
			t.Errorf("registry missing tool: %s", name)
		}
	}
}

func TestRegistryDefinitions(t *testing.T) {
	r := NewRegistry()
	defs := r.Definitions()
	if len(defs) != 16 {
		t.Errorf("expected 16 tool definitions, got %d", len(defs))
	}
	for _, d := range defs {
		if d.Name == "" {
			t.Error("tool definition has empty name")
		}
		if d.Description == "" {
			t.Errorf("tool %s has empty description", d.Name)
		}
		if len(d.InputSchema) == 0 {
			t.Errorf("tool %s has empty input schema", d.Name)
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(d.InputSchema, &schema); err != nil {
			t.Errorf("tool %s has invalid JSON schema: %v", d.Name, err)
		}
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent_tool")
	if ok {
		t.Error("expected Get to return false for unknown tool")
	}
}

func TestParseFindingInput(t *testing.T) {
	input := json.RawMessage(`{"title":"XSS in search","severity":"high","description":"Reflected XSS","evidence":"<script>alert(1)</script>","remediation":"Encode output"}`)
	f, err := ParseFindingInput(input)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "XSS in search" {
		t.Errorf("unexpected title: %s", f.Title)
	}
	if f.Severity != "high" {
		t.Errorf("unexpected severity: %s", f.Severity)
	}
	if f.Remediation != "Encode output" {
		t.Errorf("unexpected remediation: %s", f.Remediation)
	}
}

func TestParseFindingInputMissingRequired(t *testing.T) {
	input := json.RawMessage(`{"description":"something"}`)
	_, err := ParseFindingInput(input)
	if err == nil {
		t.Error("expected error for missing title and severity")
	}
}

func TestFindingInputToFinding(t *testing.T) {
	f := FindingInput{
		Title:       "Missing HSTS",
		Severity:    "medium",
		Description: "No HSTS header",
		Evidence:    "Response headers lack Strict-Transport-Security",
		Remediation: "Add HSTS header",
	}
	finding := f.ToFinding("f-001", "https://example.com")
	if finding.ID != "f-001" {
		t.Errorf("unexpected ID: %s", finding.ID)
	}
	if string(finding.Severity) != "medium" {
		t.Errorf("unexpected severity: %s", finding.Severity)
	}
	if len(finding.AffectedTargets) != 1 || finding.AffectedTargets[0] != "https://example.com" {
		t.Errorf("unexpected affected targets: %v", finding.AffectedTargets)
	}
}

func TestRegistryRegisterCustom(t *testing.T) {
	r := NewRegistry()
	r.Register("custom_tool", RegisteredTool{
		Def: llm.ToolDef{
			Name:        "custom_tool",
			Description: "A custom tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	})
	tool, ok := r.Get("custom_tool")
	if !ok {
		t.Fatal("custom tool not found")
	}
	if tool.Def.Name != "custom_tool" {
		t.Errorf("unexpected name: %s", tool.Def.Name)
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if truncate(short, 10) != "hello" {
		t.Error("short string should not be truncated")
	}
	long := "hello world this is a long string"
	result := truncate(long, 5)
	if result != "hello...[truncated]" {
		t.Errorf("unexpected truncation: %s", result)
	}
}
