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

// TestBuiltinsAliasesAreSelfConsistent walks every builtin spec and confirms
// each declared alias collapses back to its canonical name through the same
// registry. Catches new alias rows that accidentally shadow a different tool's
// canonical name (the original schedule.create regression class).
func TestBuiltinsAliasesAreSelfConsistent(t *testing.T) {
	r := NewWithBuiltins()
	for _, spec := range Builtins() {
		if got := r.Normalize(spec.Name); got != spec.Name {
			t.Fatalf("canonical %q normalized to %q", spec.Name, got)
		}
		for _, alias := range spec.Aliases {
			if got := r.Normalize(alias); got != spec.Name {
				t.Fatalf("alias %q expected to resolve to %q, got %q", alias, spec.Name, got)
			}
		}
	}
}

// TestBuiltinsRequiredLevels asserts every builtin spec has a non-empty
// RequiredLevel and the registry-derived RequiredLevel matches the spec.
// Without this check a typo in builtins.go could silently downgrade a tool to
// the "advanced" fallback path and let a high-risk action slip through.
func TestBuiltinsRequiredLevels(t *testing.T) {
	r := NewWithBuiltins()
	for _, spec := range Builtins() {
		if spec.RequiredLevel == "" {
			t.Fatalf("tool %q has empty RequiredLevel", spec.Name)
		}
		if got := r.RequiredLevel(spec.Name); got != spec.RequiredLevel {
			t.Fatalf("tool %q registry RequiredLevel=%q want %q", spec.Name, got, spec.RequiredLevel)
		}
	}
}

// TestBuiltinsDerivedViewsAreStable pins the basic shape of the four
// downstream artifacts (LLM tool list, prompt enum string, capability
// metadata, skill references) so accidentally dropping a tool fails loud.
func TestBuiltinsDerivedViewsAreStable(t *testing.T) {
	r := NewWithBuiltins()
	want := len(Builtins())
	if got := len(r.LLMDefinitions()); got != want {
		t.Fatalf("LLMDefinitions count = %d, want %d", got, want)
	}
	enum := r.PromptEnumString()
	if enum == "" {
		t.Fatal("PromptEnumString empty")
	}
	if want, got := want, len(strings.Split(enum, "|")); want != got {
		t.Fatalf("PromptEnumString contains %d names, want %d", got, want)
	}
	caps := r.CapabilityMetadataByName("zh")
	for _, spec := range Builtins() {
		if _, ok := caps[spec.Name]; !ok {
			t.Fatalf("CapabilityMetadataByName missing %q", spec.Name)
		}
	}
	// Skills must be a subset of registered tools.
	for _, ref := range r.Skills() {
		if _, ok := r.Lookup(ref.ToolName); !ok {
			t.Fatalf("Skills() reference %q is not a registered tool", ref.ToolName)
		}
	}
}
