// Package toolregistry is the daemon-side single source of truth for the
// LLM-facing tool catalog. It deliberately lives outside internal/agent/tools
// because that package is wired into the legacy CLI/web agent stack and
// changing it would risk breaking unrelated integrations.
//
// A ToolSpec carries every facet that used to live in five different places
// (LLM definitions, prompt enum strings, capability metadata, builtin skills,
// and alias normalization). Adding a new tool means writing one ToolSpec; all
// downstream views are derived.
package toolregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

// Risk classifies the worst-case impact a tool can have. Operators map this to
// the `operation_level` UI control via RequiredLevel.
type Risk string

const (
	RiskReadOnly       Risk = "read_only"
	RiskWorkspaceWrite Risk = "workspace_write"
	RiskSafeScan       Risk = "safe_scan"
	RiskApprovalOps    Risk = "approval_ops"
	RiskAdvanced       Risk = "advanced"
)

// LocalizedText carries zh/en pairs. We keep both in the spec so callers can
// pick a locale without hopping into an i18n table.
type LocalizedText struct {
	ZH string
	EN string
}

// Pick returns the text for the requested locale, defaulting to zh.
func (l LocalizedText) Pick(language string) string {
	if strings.EqualFold(strings.TrimSpace(language), "en") {
		if l.EN != "" {
			return l.EN
		}
		return l.ZH
	}
	if l.ZH != "" {
		return l.ZH
	}
	return l.EN
}

// LocalizedExamples is a list of bilingual example strings.
type LocalizedExamples []LocalizedText

// Pick collapses the list to the requested locale.
func (e LocalizedExamples) Pick(language string) []string {
	out := make([]string, 0, len(e))
	for _, ex := range e {
		out = append(out, ex.Pick(language))
	}
	return out
}

// HandlerInput is what runtime delivers to a tool handler.
type HandlerInput struct {
	SessionID      string
	Operator       string
	Language       string
	OperationLevel string
	RawArgs        map[string]any
}

// Handler is the (eventually) streaming executor for a tool. The Phase 1 cut
// returns a single result; Phase 3 will swap this for a channel-based stream.
type Handler func(ctx context.Context, in HandlerInput) (any, error)

// ToolSpec is the canonical description of one tool.
type ToolSpec struct {
	Name        string
	Aliases     []string
	Description LocalizedText
	InputSchema json.RawMessage

	Risk          Risk
	RequiredLevel string // mirrors agentOperationLevelRequiredForAction
	AutoRun       bool

	IntentTags []string
	CanDo      []string
	CannotDo   []string
	Examples   LocalizedExamples

	SkillID string

	Handler Handler
}

// Validate makes sure a spec is internally consistent. Called once at
// registration so misconfigurations fail loud at startup.
func (s ToolSpec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("toolregistry: tool name is required")
	}
	if s.Risk == "" {
		return fmt.Errorf("toolregistry: risk is required for %s", s.Name)
	}
	if strings.TrimSpace(s.RequiredLevel) == "" {
		return fmt.Errorf("toolregistry: required_level is required for %s", s.Name)
	}
	for _, alias := range s.Aliases {
		if strings.EqualFold(strings.TrimSpace(alias), s.Name) {
			return fmt.Errorf("toolregistry: alias %q duplicates the canonical name for %s", alias, s.Name)
		}
	}
	return nil
}

// Registry is the in-memory single source of truth.
//
// Iteration order is the order in which tools were registered. Several
// downstream artifacts (LLM tool list, prompt enum string used in system
// prompts) are sensitive to ordering — sorting alphabetically would silently
// drift the user-facing prompt every time a new tool is added.
type Registry struct {
	mu      sync.RWMutex
	tools   map[string]*ToolSpec // canonical name → spec
	aliases map[string]string    // alias (lower) → canonical name
	order   []string             // canonical names in registration order
}

// New returns an empty registry. Builtins are registered separately so tests
// can construct minimal registries.
func New() *Registry {
	return &Registry{
		tools:   make(map[string]*ToolSpec),
		aliases: make(map[string]string),
		order:   make([]string, 0, 32),
	}
}

// Register adds a tool. Duplicate canonical names or alias collisions are
// errors — the daemon should refuse to start rather than silently shadow a
// tool.
func (r *Registry) Register(spec ToolSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	canonical := strings.ToLower(strings.TrimSpace(spec.Name))
	if _, exists := r.tools[canonical]; exists {
		return fmt.Errorf("toolregistry: duplicate tool %q", spec.Name)
	}
	if existing, ok := r.aliases[canonical]; ok && existing != canonical {
		return fmt.Errorf("toolregistry: tool name %q collides with alias of %q", spec.Name, existing)
	}
	stored := spec
	stored.Name = canonical
	r.tools[canonical] = &stored
	r.order = append(r.order, canonical)
	r.aliases[canonical] = canonical
	for _, alias := range spec.Aliases {
		key := strings.ToLower(strings.TrimSpace(alias))
		if key == "" {
			continue
		}
		if existing, ok := r.aliases[key]; ok && existing != canonical {
			return fmt.Errorf("toolregistry: alias %q already maps to %q (cannot also map to %q)", alias, existing, canonical)
		}
		r.aliases[key] = canonical
	}
	return nil
}

// MustRegister panics on error — useful for the top-level builtins list where
// a misconfiguration is a programmer bug.
func (r *Registry) MustRegister(spec ToolSpec) {
	if err := r.Register(spec); err != nil {
		panic(err)
	}
}

// Lookup resolves a raw name (canonical or alias) to its spec.
func (r *Registry) Lookup(rawName string) (*ToolSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	canonical, ok := r.aliases[strings.ToLower(strings.TrimSpace(rawName))]
	if !ok {
		return nil, false
	}
	spec, ok := r.tools[canonical]
	return spec, ok
}

// Normalize maps a raw name (planner output, alias, casing variant) to the
// canonical tool name. Returns the input unchanged when no match is found so
// callers can treat unknown tools as plugin/MCP names without crashing.
func (r *Registry) Normalize(rawName string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if canonical, ok := r.aliases[strings.ToLower(strings.TrimSpace(rawName))]; ok {
		return canonical
	}
	return strings.TrimSpace(rawName)
}

// All returns every spec in registration order. The slice is a copy so
// callers can mutate it safely. Registration order matters: prompt templates
// and the LLM tool list are sensitive to ordering. Use AllSorted when you
// need a deterministic alphabetical view (tests, diffs).
func (r *Registry) All() []*ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		if spec, ok := r.tools[name]; ok {
			out = append(out, spec)
		}
	}
	return out
}

// AllSorted returns every spec sorted by canonical name. Useful for diffs and
// tests; runtime callers should prefer All to keep prompt parity.
func (r *Registry) AllSorted() []*ToolSpec {
	specs := r.All()
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs
}

// LLMDefinitions derives the function-calling tool list. Replaces
// agentLLMToolDefinitions().
func (r *Registry) LLMDefinitions() []llm.ToolDef {
	specs := r.All()
	defs := make([]llm.ToolDef, 0, len(specs))
	for _, spec := range specs {
		defs = append(defs, llm.ToolDef{
			Name:        spec.Name,
			Description: spec.Description.Pick("zh"), // zh fallback; daemon can localise per-call
			InputSchema: spec.InputSchema,
		})
	}
	return defs
}

// PromptEnumString returns the `name1|name2|...` string used in prompt
// templates. Replaces the four hand-edited literals in agent.go.
func (r *Registry) PromptEnumString() string {
	specs := r.All()
	parts := make([]string, 0, len(specs))
	for _, spec := range specs {
		parts = append(parts, spec.Name)
	}
	return strings.Join(parts, "|")
}

// CapabilityMetadata replaces agentToolCapabilityMetadataByName.
type CapabilityMetadata struct {
	IntentTags    []string
	CanDo         []string
	CannotDo      []string
	Examples      []string
	Risk          Risk
	RequiredLevel string
	AutoRun       bool
}

// CapabilityMetadataByName returns one entry per tool, localized.
func (r *Registry) CapabilityMetadataByName(language string) map[string]CapabilityMetadata {
	specs := r.All()
	out := make(map[string]CapabilityMetadata, len(specs))
	for _, spec := range specs {
		out[spec.Name] = CapabilityMetadata{
			IntentTags:    append([]string{}, spec.IntentTags...),
			CanDo:         append([]string{}, spec.CanDo...),
			CannotDo:      append([]string{}, spec.CannotDo...),
			Examples:      spec.Examples.Pick(language),
			Risk:          spec.Risk,
			RequiredLevel: spec.RequiredLevel,
			AutoRun:       spec.AutoRun,
		}
	}
	return out
}

// SkillReference points at an external SKILL.md document. The daemon's skill
// loader merges these with on-disk files at runtime.
type SkillReference struct {
	ToolName string
	SkillID  string
}

// Skills returns one reference per tool that exposes a SkillID.
func (r *Registry) Skills() []SkillReference {
	specs := r.All()
	out := make([]SkillReference, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.SkillID) == "" {
			continue
		}
		out = append(out, SkillReference{ToolName: spec.Name, SkillID: spec.SkillID})
	}
	return out
}

// RequiredLevel returns the operation level required for a tool by raw name.
// Falls back to "advanced" for unknown tools so plugin/MCP names don't get
// silently downgraded.
func (r *Registry) RequiredLevel(rawName string) string {
	if spec, ok := r.Lookup(rawName); ok {
		return spec.RequiredLevel
	}
	return "advanced"
}
