package daemon

import (
	"strings"
	"sync"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
)

// daemonToolRegistry is the daemon's process-wide tool registry. It is the
// canonical source of truth for tool names, aliases, schemas, descriptions,
// risks, required operation levels, and skill metadata. The legacy
// agentLLMToolDefinitions / normalizeAgentPlannerActionName /
// agentOperationLevelRequiredForAction / agentToolCapabilityMetadataByName
// helpers are progressively delegating into this registry; parity tests guard
// the migration window.
var (
	daemonToolRegistryOnce sync.Once
	daemonToolRegistryRef  *toolregistry.Registry
)

// toolRegistry returns the lazily-initialized daemon tool registry. The
// registry is read-mostly after construction; concurrent access goes through
// the package-internal RWMutex.
func toolRegistry() *toolregistry.Registry {
	daemonToolRegistryOnce.Do(func() {
		daemonToolRegistryRef = toolregistry.NewWithBuiltins()
	})
	return daemonToolRegistryRef
}

// legacyPromptToolEnum is the historical pipe-separated tool enum that
// appeared in four hand-edited spots inside agent.go's planner / synthesis
// prompts. It is the source of truth for the legacy template; we rewrite any
// occurrence to the registry's current PromptEnumString() so adding a tool to
// the registry automatically updates every system prompt without further
// edits.
const legacyPromptToolEnum = "scan.web.safe|ports.scan.safe|http.request|web.search|web.fetch|workspace.command|workspace.files.list|workspace.files.tree|workspace.files.read|workspace.search|workspace.files.write|workspace.files.patch|workspace.files.diff|workspace.files.diff_many|workspace.files.rebase|workspace.files.apply|workspace.files.discard|workspace.files.delete|workspace.files.move|workspace.files.copy|workspace.files.mkdir|daemon.health|plugins.list|runs.list|runs.context|reports.draft|runs.approvals.list|runs.approvals.approve|runs.approvals.deny|runs.resume_approved|schedule.create|schedule.list|schedule.delete"

// applyToolEnumPlaceholders rewrites every legacy hand-edited tool enum
// occurrence in the supplied prompt text to the registry-derived value, so
// the four planner/synthesis templates stay in lock-step with the registry
// (and adding a new builtin doesn't silently leave the model with a stale
// list — that drift is what caused the schedule.create regression).
func applyToolEnumPlaceholders(prompt string) string {
	current := toolRegistry().PromptEnumString()
	if current == "" || current == legacyPromptToolEnum {
		return prompt
	}
	return strings.ReplaceAll(prompt, legacyPromptToolEnum, current)
}
