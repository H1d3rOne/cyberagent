package daemon

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
)

// TestToolRegistryDaemonLLMDefinitions guards Phase 1 of the
// production-runtime refactor: the daemon's tool registry must continue to
// surface every builtin spec through Registry.LLMDefinitions() so the LLM
// request body never drifts from the canonical source.
func TestToolRegistryDaemonLLMDefinitions(t *testing.T) {
	got := toolRegistry().LLMDefinitions()
	if len(got) == 0 {
		t.Fatal("toolRegistry().LLMDefinitions() returned empty")
	}
	specs := toolregistry.Builtins()
	if len(got) != len(specs) {
		t.Fatalf("tool count mismatch: registry=%d builtins=%d", len(got), len(specs))
	}
	byName := make(map[string]toolregistry.ToolSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	defs := append([]llm.ToolDef{}, got...)
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	for _, def := range defs {
		spec, ok := byName[def.Name]
		if !ok {
			t.Fatalf("registry surfaced unknown tool %q", def.Name)
		}
		if def.Description != spec.Description.Pick("zh") {
			t.Fatalf("tool %q description drift:\nregistry=%s\nbuiltin=%s", def.Name, def.Description, spec.Description.Pick("zh"))
		}
		if !jsonEqual(t, def.InputSchema, spec.InputSchema) {
			t.Fatalf("tool %q schema drift:\nregistry=%s\nbuiltin=%s", def.Name, def.InputSchema, spec.InputSchema)
		}
	}
}

// TestToolRegistryParityWithNormalizeAgentPlannerActionName verifies every
// alias the daemon collapses today still resolves to the same canonical name
// through the registry.
func TestToolRegistryParityWithNormalizeAgentPlannerActionName(t *testing.T) {
	registry := toolregistry.NewWithBuiltins()
	for _, spec := range toolregistry.Builtins() {
		// Canonical name itself.
		if got := registry.Normalize(spec.Name); got != spec.Name {
			t.Fatalf("canonical %q normalized to %q", spec.Name, got)
		}
		if got := normalizeAgentPlannerActionName(spec.Name); got != spec.Name {
			t.Fatalf("legacy normalized canonical %q to %q", spec.Name, got)
		}
		// Aliases listed in the registry must collapse identically through
		// both code paths.
		for _, alias := range spec.Aliases {
			legacy := normalizeAgentPlannerActionName(alias)
			modern := registry.Normalize(alias)
			if legacy != modern {
				t.Fatalf("alias %q: legacy=%q registry=%q", alias, legacy, modern)
			}
			if modern != spec.Name {
				t.Fatalf("alias %q expected to resolve to %q, got %q", alias, spec.Name, modern)
			}
		}
	}
}

// TestToolRegistryParityWithPromptEnumString pins the four hand-edited tool
// enum strings inside the planner / synthesis prompt templates against the
// registry's PromptEnumString(). Adding a new tool without updating these
// literals used to silently leave the model with a stale list (the original
// source of the schedule.create regression). The daemon now substitutes the
// legacy literal with the live registry value at render time, so this test
// asserts both rendered prompts (planner + synthesis) for both languages
// already contain the registry-derived string.
func TestToolRegistryParityWithPromptEnumString(t *testing.T) {
	want := toolregistry.NewWithBuiltins().PromptEnumString()
	if want == "" {
		t.Fatal("registry PromptEnumString returned empty")
	}
	cases := []struct {
		name   string
		render func() string
	}{
		{"planner_zh", func() string { return agentPlannerSystemPrompt("zh", "", "safe_scan") }},
		{"planner_en", func() string { return agentPlannerSystemPrompt("en", "", "safe_scan") }},
		{"synthesis_zh", func() string { return agentToolSynthesisSystemPrompt("zh", "", "safe_scan") }},
		{"synthesis_en", func() string { return agentToolSynthesisSystemPrompt("en", "", "safe_scan") }},
	}
	for _, tc := range cases {
		rendered := tc.render()
		if !strings.Contains(rendered, want) {
			t.Fatalf("%s prompt missing registry tool enum %q", tc.name, want)
		}
		// Belt-and-braces: the legacy hand-written enum literal that used to
		// drift on its own must NOT be present after substitution unless it
		// happens to match the registry's current output.
		if strings.Contains(rendered, legacyPromptToolEnum) && legacyPromptToolEnum != want {
			t.Fatalf("%s prompt still contains the legacy literal %q after substitution", tc.name, legacyPromptToolEnum)
		}
	}
}

// TestToolRegistryParityWithRequiredOperationLevel pins the per-tool
// operation_level mapping so the registry can later replace
// agentOperationLevelRequiredForAction without behaviour change.
func TestToolRegistryParityWithRequiredOperationLevel(t *testing.T) {
	for _, spec := range toolregistry.Builtins() {
		legacy := agentOperationLevelRequiredForAction(spec.Name)
		if spec.RequiredLevel != legacy {
			t.Fatalf("tool %q required_level mismatch: legacy=%q registry=%q", spec.Name, legacy, spec.RequiredLevel)
		}
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("decode legacy schema: %v", err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("decode registry schema: %v", err)
	}
	xj, err := json.Marshal(x)
	if err != nil {
		t.Fatalf("marshal legacy schema: %v", err)
	}
	yj, err := json.Marshal(y)
	if err != nil {
		t.Fatalf("marshal registry schema: %v", err)
	}
	return bytes.Equal(xj, yj)
}
