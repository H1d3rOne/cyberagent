package daemon

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
)

// TestToolRegistryParityWithAgentLLMToolDefinitions guards Phase 1 of the
// production-runtime refactor: the new toolregistry package must produce the
// exact same LLM tool list (name + description + schema) as the legacy
// agentLLMToolDefinitions() literal until the daemon swaps over to it.
func TestToolRegistryParityWithAgentLLMToolDefinitions(t *testing.T) {
	legacy := agentLLMToolDefinitions()
	got := toolregistry.NewWithBuiltins().LLMDefinitions()

	sort.Slice(legacy, func(i, j int) bool { return legacy[i].Name < legacy[j].Name })
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	if len(legacy) != len(got) {
		t.Fatalf("tool count mismatch: legacy=%d registry=%d", len(legacy), len(got))
	}
	for i := range legacy {
		if legacy[i].Name != got[i].Name {
			t.Fatalf("tool[%d] name mismatch: legacy=%q registry=%q", i, legacy[i].Name, got[i].Name)
		}
		if legacy[i].Description != got[i].Description {
			t.Fatalf("tool %q description mismatch:\nlegacy=%s\nregistry=%s", legacy[i].Name, legacy[i].Description, got[i].Description)
		}
		if !jsonEqual(t, legacy[i].InputSchema, got[i].InputSchema) {
			t.Fatalf("tool %q schema mismatch:\nlegacy=%s\nregistry=%s", legacy[i].Name, legacy[i].InputSchema, got[i].InputSchema)
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
// literals would let the model see a stale tool list and was the original
// source of the schedule.create regression. The test reads agent.go directly
// so the literals do not need to be exported.
func TestToolRegistryParityWithPromptEnumString(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(wd, "agent.go"))
	if err != nil {
		t.Fatalf("read agent.go: %v", err)
	}
	want := toolregistry.NewWithBuiltins().PromptEnumString()
	occurrences := bytes.Count(source, []byte(want))
	if occurrences < 4 {
		t.Fatalf("prompt enum string appears %d times in agent.go, expected ≥4 (planner zh/en + synthesis zh/en); registry value=%q", occurrences, want)
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

// TestToolRegistryParityWithPromptEnumStrings ensures the four hand-written
// tool enum literals in agent.go's prompt templates stay in lock-step with
// toolregistry.PromptEnumString. When the daemon switches to generated
// templates these literals disappear; until then this test prevents drift.
func TestToolRegistryParityWithPromptEnumStrings(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	path := filepath.Join(wd, "agent.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read agent.go: %v", err)
	}
	want := toolregistry.NewWithBuiltins().PromptEnumString()
	count := bytes.Count(data, []byte(want))
	if count < 4 {
		t.Fatalf("expected prompt enum literal %q to appear at least 4 times in agent.go, got %d", want, count)
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
