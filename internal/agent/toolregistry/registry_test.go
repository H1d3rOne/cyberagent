package toolregistry

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleSpec(name string, aliases ...string) ToolSpec {
	return ToolSpec{
		Name:          name,
		Aliases:       aliases,
		Description:   LocalizedText{ZH: "中文 " + name, EN: "english " + name},
		InputSchema:   json.RawMessage(`{"type":"object"}`),
		Risk:          RiskSafeScan,
		RequiredLevel: "safe_scan",
		IntentTags:    []string{"tag_" + name},
		Examples:      LocalizedExamples{{ZH: "示例 " + name, EN: "example " + name}},
		SkillID:       "skill_" + name,
	}
}

func TestRegisterAndLookupCanonicalAndAlias(t *testing.T) {
	r := New()
	if err := r.Register(sampleSpec("scan.web.safe", "scan.web", "web.scan")); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got, ok := r.Lookup("Scan.Web.Safe"); !ok || got.Name != "scan.web.safe" {
		t.Fatalf("canonical case-insensitive lookup failed: %+v ok=%v", got, ok)
	}
	if got, ok := r.Lookup("web.scan"); !ok || got.Name != "scan.web.safe" {
		t.Fatalf("alias lookup failed: %+v ok=%v", got, ok)
	}
	if got := r.Normalize("WEB.SCAN"); got != "scan.web.safe" {
		t.Fatalf("normalize alias failed: %q", got)
	}
	if got := r.Normalize("unknown.tool"); got != "unknown.tool" {
		t.Fatalf("normalize should pass through unknown tools, got %q", got)
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	r := New()
	if err := r.Register(sampleSpec("scan.web.safe")); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(sampleSpec("scan.web.safe")); err == nil {
		t.Fatal("expected duplicate canonical name to fail")
	}
}

func TestRegisterRejectsAliasCollision(t *testing.T) {
	r := New()
	if err := r.Register(sampleSpec("scan.web.safe", "shared")); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(sampleSpec("ports.scan.safe", "shared")); err == nil {
		t.Fatal("expected alias collision to fail")
	}
}

func TestValidateRequiresName(t *testing.T) {
	r := New()
	if err := r.Register(ToolSpec{Risk: RiskSafeScan, RequiredLevel: "safe_scan"}); err == nil {
		t.Fatal("expected missing name to fail")
	}
}

func TestDerivedViews(t *testing.T) {
	r := New()
	r.MustRegister(sampleSpec("a.tool"))
	r.MustRegister(sampleSpec("b.tool"))

	defs := r.LLMDefinitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}
	if defs[0].Name != "a.tool" || defs[1].Name != "b.tool" {
		t.Fatalf("definitions not sorted: %+v", defs)
	}

	enum := r.PromptEnumString()
	if enum != "a.tool|b.tool" {
		t.Fatalf("prompt enum = %q", enum)
	}

	meta := r.CapabilityMetadataByName("en")
	if got := meta["a.tool"].Examples; len(got) != 1 || !strings.Contains(got[0], "example a.tool") {
		t.Fatalf("english example missing: %+v", got)
	}

	skills := r.Skills()
	if len(skills) != 2 {
		t.Fatalf("expected 2 skill refs, got %d", len(skills))
	}
}

func TestRequiredLevelFallback(t *testing.T) {
	r := New()
	r.MustRegister(sampleSpec("scan.web.safe"))
	if got := r.RequiredLevel("scan.web.safe"); got != "safe_scan" {
		t.Fatalf("required level: %q", got)
	}
	if got := r.RequiredLevel("plugin.unknown"); got != "advanced" {
		t.Fatalf("unknown tool should default to advanced, got %q", got)
	}
}
