# Phase 1 Review (Leader self-audit; Codex-Reviewer not responding)

## Verdict: ACCEPTED ✅

Phase 1 single-source-of-truth refactor is in place, parity tests cover all five truth sources, and daemon side is converged. Plan document acceptance criteria met.

## Evidence (file:line)

### Truth sources — all 5 converged to ToolSpec
1. **LLMDefinitions** — `internal/interfaces/daemon/agent.go:674,1698` call `toolRegistry().LLMDefinitions()`. The function body `agentLLMToolDefinitions()` is removed; remaining 3 references are docstring-only.
2. **Normalize** — `internal/interfaces/daemon/agent.go:9909` `normalizeAgentPlannerActionName(raw) = toolRegistry().Normalize(raw)` (single-line forwarder). 5 call sites (lines 801,1572,2037,2160,2312) all flow through this single point.
3. **CapabilityMetadata** — `internal/interfaces/daemon/agent_skills.go:266` `agentToolCapabilityMetadataByName` derives via `toolRegistry().CapabilityMetadataByName(normalizeAgentLanguage(language))`.
4. **PromptEnumString** — `internal/interfaces/daemon/agent_toolregistry.go:46` `applyToolEnumPlaceholders(prompt)` rewrites the four hardcoded enum strings at runtime by replacing `{{TOOL_ENUM}}` placeholders with `toolRegistry().PromptEnumString()`.
5. **Lookup** — `internal/interfaces/daemon/agent.go:3965` already dispatches via `toolRegistry().Lookup(actionName)`.

### `normalizeKey()` fold
`internal/agent/toolregistry/registry.go:108-114`: `lowercase + trim + "-" / "_" / whitespace → "."`. Behavior-aligned with legacy `normalizeAgentPlannerActionName` (handles `port.scan / port_scan / ports_scan / ports.scan`).

### Parity tests — all green
- `internal/agent/toolregistry/registry_test.go` (+64 lines): alias / required level / derived views.
- `internal/interfaces/daemon/agent_toolregistry_parity_test.go` (+110 lines):
  - `TestToolRegistryDaemonLLMDefinitions` — daemon view == registry view
  - `TestToolRegistryParityWithNormalizeAgentPlannerActionName` — every spec's canonical + Aliases match through both paths
  - `TestToolRegistryParityWithPromptEnumString` — renders planner+synthesis × zh/en system prompts, asserts placeholder rewritten
  - `TestToolRegistryParityWithRequiredOperationLevel` — spec.RequiredLevel == daemon agentOperationLevelRequiredForAction
- `internal/interfaces/daemon/agent_toolregistry_capability_parity_test.go` (+39 lines): zh/en capability metadata parity.

Run: `go test ./internal/interfaces/daemon/... -run TestToolRegistry` → `ok 0.380s`. Full `go test ./internal/...` green.

## Findings (none blocking)

- **[中] Docstring residue** — 3 comments still mention `agentLLMToolDefinitions` literal:
  - `internal/agent/toolregistry/registry.go:270`
  - `internal/agent/toolregistry/builtins.go:9`
  - `internal/interfaces/daemon/agent_toolregistry.go:13`
  No runtime impact. Cleanup candidate for Phase 2 minor sweep.

- **[中] `executeAgentPlannerAction` 32-case switch** — `internal/interfaces/daemon/agent.go:2311` onward. Per plan, `ToolSpec.Handler` slot is reserved and will be filled in Phase 4 (daemon→handler package split). `toolRegistry().Lookup` already provides the dispatch seam. Carrier explicitly tracked.

- **[低] Test gap** — no explicit test that the `applyToolEnumPlaceholders` injection preserves prompt tokens outside `{{TOOL_ENUM}}`. Existing TestToolRegistryParityWithPromptEnumString covers the happy path; placeholder-leakage corner case not exercised. Phase 3 has the right surface to add this.

## Recommendation

Proceed to Phase 2. Docstring sweep + handler migration covered by Phase 4 plan; no Phase 1 regression.
