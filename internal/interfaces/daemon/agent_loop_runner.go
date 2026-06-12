package daemon

import (
	"context"
	"errors"
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonAgentLoopRunner struct {
	manager *agentManager
}

type daemonAgentLoopCallerFallbackReason string

const (
	daemonAgentLoopCallerFallbackEngineNotAttached daemonAgentLoopCallerFallbackReason = "runtime_engine_not_attached"
	daemonAgentLoopCallerFallbackRunnerNotAttached daemonAgentLoopCallerFallbackReason = "agent_loop_runner_not_attached"
)

const daemonAgentLoopCallerFallbackOwnerLegacyCaller = "legacy_caller"

type daemonAgentLoopCallerFallbackOwnershipEntry struct {
	reason daemonAgentLoopCallerFallbackReason
	owner  string
}

type daemonAgentLoopCallerFallbackBoundaryEntry struct {
	mode           runtime.AgentLoopMode
	callerSurface  string
	legacyFallback string
	owner          string
	scope          string
}

type daemonAgentLoopNoEngineDirectFallbackRetirementStage string

const (
	daemonAgentLoopNoEngineDirectFallbackStageProductionDefaultAttached daemonAgentLoopNoEngineDirectFallbackRetirementStage = "production_default_runtime_engine_attached"
	daemonAgentLoopNoEngineDirectFallbackStageLegacyOptOutRetained      daemonAgentLoopNoEngineDirectFallbackRetirementStage = "legacy_runtime_opt_out_direct_fallback_retained"
	daemonAgentLoopNoEngineDirectFallbackStagePlannerActionScoped       daemonAgentLoopNoEngineDirectFallbackRetirementStage = "planner_action_execution_no_engine_direct_fallback_scoped"
	daemonAgentLoopNoEngineDirectFallbackStageNextBoundaryChosen        daemonAgentLoopNoEngineDirectFallbackRetirementStage = "agent_loop_runner_not_attached_boundary_chosen"
)

const (
	daemonAgentLoopNoEngineDirectFallbackDispositionEngineAttached       = "runtime_engine_attached"
	daemonAgentLoopNoEngineDirectFallbackDispositionDirectLegacyFallback = "direct_legacy_fallback"
	daemonAgentLoopNoEngineDirectFallbackDispositionAuditNextBoundary    = "audit_next_boundary"
)

const daemonAgentLoopNextFallbackRetirementBoundaryRunnerNotAttached = "agent_loop_runner_not_attached_fallback_retirement_readiness"

type daemonAgentLoopNoEngineDirectFallbackRetirementEntry struct {
	component    string
	stage        daemonAgentLoopNoEngineDirectFallbackRetirementStage
	reason       daemonAgentLoopCallerFallbackReason
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonAgentLoopRunnerNotAttachedRetirementStage string

const (
	daemonAgentLoopRunnerNotAttachedStageNoEngineFallbackRetained daemonAgentLoopRunnerNotAttachedRetirementStage = "no_engine_agent_loop_direct_fallback_retained"
	daemonAgentLoopRunnerNotAttachedStageMissingRunnerBlocked     daemonAgentLoopRunnerNotAttachedRetirementStage = "attached_engine_missing_agent_loop_runner_blocked"
	daemonAgentLoopRunnerNotAttachedStageDetachedResultBlocked    daemonAgentLoopRunnerNotAttachedRetirementStage = "detached_agent_loop_runner_result_blocked"
	daemonAgentLoopRunnerNotAttachedStageNextBoundaryChosen       daemonAgentLoopRunnerNotAttachedRetirementStage = "llm_stage_runner_not_attached_boundary_chosen"
)

const (
	daemonAgentLoopRunnerNotAttachedDispositionDirectLegacyFallback = "direct_legacy_fallback"
	daemonAgentLoopRunnerNotAttachedDispositionBlockedError         = "blocked_error"
	daemonAgentLoopRunnerNotAttachedDispositionAuditNextBoundary    = "audit_next_boundary"
)

const daemonAgentLoopNextFallbackRetirementBoundaryLLMStageRunnerNotAttached = "llm_stage_runner_not_attached_fallback_retirement_readiness"

type daemonAgentLoopRunnerNotAttachedRetirementEntry struct {
	component    string
	stage        daemonAgentLoopRunnerNotAttachedRetirementStage
	reason       daemonAgentLoopCallerFallbackReason
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

var daemonAgentLoopCallerFallbackOwnershipTable = [...]daemonAgentLoopCallerFallbackOwnershipEntry{
	{reason: daemonAgentLoopCallerFallbackEngineNotAttached, owner: daemonAgentLoopCallerFallbackOwnerLegacyCaller},
}

var daemonAgentLoopCallerFallbackBoundaryTable = [...]daemonAgentLoopCallerFallbackBoundaryEntry{
	{
		mode:           runtime.AgentLoopModeDeterministicRoute,
		callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
		legacyFallback: "runAgentDeterministicRouteAgentLoopResult",
		owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
		scope:          "deterministic_route compatibility fallback only when runtime engine is not attached",
	},
	{
		mode:           runtime.AgentLoopModeLLMToolLoop,
		callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
		legacyFallback: "runAgentLLMToolLoopAgentLoopResult",
		owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
		scope:          "llm_tool_loop compatibility fallback only when runtime engine is not attached",
	},
	{
		mode:           runtime.AgentLoopModeReplanContinuation,
		callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
		legacyFallback: "runAgentReplanActionContinuationAgentLoopResult",
		owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
		scope:          "replan_action_continuation compatibility fallback only when runtime engine is not attached",
	},
	{
		mode:           runtime.AgentLoopModePlannerActionReplanCycle,
		callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
		legacyFallback: "runAgentPlannerActionReplanCycleAgentLoop",
		owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
		scope:          "planner_action_replan_cycle compatibility fallback only when runtime engine is not attached",
	},
	{
		mode:           runtime.AgentLoopModePlannerActionExecution,
		callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
		legacyFallback: "runAgentPlannerActionExecutionAgentLoopResult",
		owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
		scope:          "planner_action_execution compatibility fallback only when runtime engine is not attached",
	},
}

var daemonAgentLoopNoEngineDirectFallbackRetirementTable = [...]daemonAgentLoopNoEngineDirectFallbackRetirementEntry{
	{
		component:    "production_new_server_runtime_engine",
		stage:        daemonAgentLoopNoEngineDirectFallbackStageProductionDefaultAttached,
		disposition:  daemonAgentLoopNoEngineDirectFallbackDispositionEngineAttached,
		evidence:     "agentRuntimeEngineEnabledFromEnv(\"\") defaults true and NewServer creates a runtime.Engine before calling agentManager.AttachEngine, which attaches daemonAgentLoopRunner and daemonAgentLoopActionExecutor",
		nextBoundary: daemonAgentLoopNextFallbackRetirementBoundaryRunnerNotAttached,
		nextStep:     "keep production callers on NewServer/AttachEngine; do not use the no-engine fallback as a production default path",
	},
	{
		component:    "legacy_runtime_opt_out",
		stage:        daemonAgentLoopNoEngineDirectFallbackStageLegacyOptOutRetained,
		reason:       daemonAgentLoopCallerFallbackEngineNotAttached,
		disposition:  daemonAgentLoopNoEngineDirectFallbackDispositionDirectLegacyFallback,
		evidence:     "CYBERAGENT_RUNTIME=legacy/off/disabled/false/0 remains the documented rollback switch and intentionally leaves the manager without a runtime.Engine",
		nextBoundary: daemonAgentLoopNextFallbackRetirementBoundaryRunnerNotAttached,
		nextStep:     "only retire this no-engine direct fallback together with the documented legacy runtime opt-out switch",
	},
	{
		component:    "planner_action_execution_no_engine_direct_fallback",
		stage:        daemonAgentLoopNoEngineDirectFallbackStagePlannerActionScoped,
		reason:       daemonAgentLoopCallerFallbackEngineNotAttached,
		disposition:  daemonAgentLoopNoEngineDirectFallbackDispositionDirectLegacyFallback,
		evidence:     "planner_action_execution reaches daemonAgentLoopActionExecutor only through runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult -> runAgentPlannerActionExecutionAgentLoopResult when runtime.Engine is absent; attached-engine missing/detached AgentLoopActionExecutor is blocked by the N214 audit",
		nextBoundary: daemonAgentLoopNextFallbackRetirementBoundaryRunnerNotAttached,
		nextStep:     "continue by narrowing attached-engine AgentLoopRunner missing/detached fallback before revisiting the explicit no-engine rollback path",
	},
	{
		component:    "agent_loop_next_fallback_boundary",
		stage:        daemonAgentLoopNoEngineDirectFallbackStageNextBoundaryChosen,
		disposition:  daemonAgentLoopNoEngineDirectFallbackDispositionAuditNextBoundary,
		evidence:     "no-engine direct fallback is retained for the explicit legacy runtime opt-out; attached-engine AgentLoopRunner nil/ErrAgentLoopRunnerNotAttached is the next narrower fallback boundary",
		nextBoundary: daemonAgentLoopNextFallbackRetirementBoundaryRunnerNotAttached,
		nextStep:     "P0.5-N216 should decide whether attached-engine AgentLoopRunner missing/detached fallback can become blocked/error while keeping no-engine legacy opt-out fallback",
	},
}

var daemonAgentLoopRunnerNotAttachedRetirementTable = [...]daemonAgentLoopRunnerNotAttachedRetirementEntry{
	{
		component:    "no_engine_agent_loop_direct_fallback",
		stage:        daemonAgentLoopRunnerNotAttachedStageNoEngineFallbackRetained,
		reason:       daemonAgentLoopCallerFallbackEngineNotAttached,
		disposition:  daemonAgentLoopRunnerNotAttachedDispositionDirectLegacyFallback,
		evidence:     "no-engine direct fallback remains scoped to the documented CYBERAGENT_RUNTIME legacy opt-out path from the N215 audit",
		nextBoundary: daemonAgentLoopNextFallbackRetirementBoundaryLLMStageRunnerNotAttached,
		nextStep:     "do not retire the no-engine direct fallback until the explicit legacy runtime opt-out switch is removed",
	},
	{
		component:    "attached_engine_missing_agent_loop_runner",
		stage:        daemonAgentLoopRunnerNotAttachedStageMissingRunnerBlocked,
		reason:       daemonAgentLoopCallerFallbackRunnerNotAttached,
		disposition:  daemonAgentLoopRunnerNotAttachedDispositionBlockedError,
		evidence:     "agentManager.AttachEngine attaches daemonAgentLoopRunner, so a nil AgentLoopRunner on an attached engine represents an explicit detach/miswire and now returns runtime.ErrAgentLoopRunnerNotAttached instead of direct daemon fallback",
		nextBoundary: daemonAgentLoopNextFallbackRetirementBoundaryLLMStageRunnerNotAttached,
		nextStep:     "continue with the analogous attached-engine LLMStageRunner missing/detached fallback boundary",
	},
	{
		component:    "detached_agent_loop_runner_result",
		stage:        daemonAgentLoopRunnerNotAttachedStageDetachedResultBlocked,
		reason:       daemonAgentLoopCallerFallbackRunnerNotAttached,
		disposition:  daemonAgentLoopRunnerNotAttachedDispositionBlockedError,
		evidence:     "ErrAgentLoopRunnerNotAttached returned by Engine.RunAgentLoop is normalized to blocked/error and no longer falls through to runDaemonAgentLoopDirectResult",
		nextBoundary: daemonAgentLoopNextFallbackRetirementBoundaryLLMStageRunnerNotAttached,
		nextStep:     "continue with the analogous attached-engine LLMStageRunner missing/detached fallback boundary",
	},
	{
		component:    "agent_loop_next_fallback_boundary",
		stage:        daemonAgentLoopRunnerNotAttachedStageNextBoundaryChosen,
		disposition:  daemonAgentLoopRunnerNotAttachedDispositionAuditNextBoundary,
		evidence:     "AgentLoop direct fallback is now limited to no-engine legacy opt-out; attached-engine runner nil/ErrAgentLoopRunnerNotAttached is blocked/error",
		nextBoundary: daemonAgentLoopNextFallbackRetirementBoundaryLLMStageRunnerNotAttached,
		nextStep:     "P0.5-N217 should decide whether attached-engine LLMStageRunner missing/detached fallback can become blocked/error while keeping no-engine legacy opt-out fallback",
	},
}

func daemonAgentLoopCallerFallbackOwnerForReason(reason daemonAgentLoopCallerFallbackReason) (string, bool) {
	for _, entry := range daemonAgentLoopCallerFallbackOwnershipTable {
		if entry.reason == reason {
			return entry.owner, true
		}
	}
	return "", false
}

func daemonAgentLoopCallerFallbackOwnedByLegacyCaller(reason daemonAgentLoopCallerFallbackReason) bool {
	owner, ok := daemonAgentLoopCallerFallbackOwnerForReason(reason)
	return ok && owner == daemonAgentLoopCallerFallbackOwnerLegacyCaller
}

func daemonAgentLoopCallerFallbackOwnershipError(reason daemonAgentLoopCallerFallbackReason) error {
	return fmt.Errorf("daemon: agent loop fallback ownership missing for %s", reason)
}

func daemonAgentLoopCallerPreDispatchFallbackReason(eng *runtime.Engine) (daemonAgentLoopCallerFallbackReason, bool) {
	if eng == nil {
		return daemonAgentLoopCallerFallbackEngineNotAttached, true
	}
	if eng.AgentLoopRunner() == nil {
		return daemonAgentLoopCallerFallbackRunnerNotAttached, true
	}
	return "", false
}

func daemonAgentLoopCallerResultFallbackReason(resultErr error) (daemonAgentLoopCallerFallbackReason, bool) {
	if errors.Is(resultErr, runtime.ErrAgentLoopRunnerNotAttached) {
		return daemonAgentLoopCallerFallbackRunnerNotAttached, true
	}
	return "", false
}

func daemonAgentLoopCallerFallbackBoundaryForMode(mode runtime.AgentLoopMode) (daemonAgentLoopCallerFallbackBoundaryEntry, bool) {
	for _, entry := range daemonAgentLoopCallerFallbackBoundaryTable {
		if entry.mode == mode {
			return entry, true
		}
	}
	return daemonAgentLoopCallerFallbackBoundaryEntry{}, false
}

func daemonAgentLoopCallerRunnerNotAttachedBlockedResult(req runtime.AgentLoopRequest, result runtime.AgentLoopResult, resultErr error) (runtime.AgentLoopResult, bool) {
	if !errors.Is(resultErr, runtime.ErrAgentLoopRunnerNotAttached) {
		return runtime.AgentLoopResult{}, false
	}
	blocked := result.Clone()
	blocked.CapabilityBlocked = true
	if blocked.StopReason == "" {
		blocked.StopReason = resultErr.Error()
	}
	metadata := runtime.CloneAgentLoopMapStringAny(blocked.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	if req.Mode != "" {
		metadata["mode"] = string(req.Mode)
	}
	metadata["error"] = resultErr.Error()
	metadata["blocked_reason"] = string(daemonAgentLoopCallerFallbackRunnerNotAttached)
	blocked.Metadata = metadata
	return blocked, true
}

func daemonAgentLoopNestedDispatchErrorResult(req runtime.AgentLoopRequest, result runtime.AgentLoopResult, resultErr error) runtime.AgentLoopResult {
	if resultErr == nil {
		return result.Clone()
	}
	failed := result.Clone()
	if failed.StopReason == "" {
		failed.StopReason = resultErr.Error()
	}
	metadata := runtime.CloneAgentLoopMapStringAny(failed.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	if req.Mode != "" {
		metadata["mode"] = string(req.Mode)
	}
	if _, ok := metadata["error"]; !ok {
		metadata["error"] = resultErr.Error()
	}
	metadata["nested_dispatch_error"] = true
	failed.Metadata = metadata
	return failed
}

func daemonAgentLoopNoEngineDirectFallbackRetirementEntryForComponent(component string) (daemonAgentLoopNoEngineDirectFallbackRetirementEntry, bool) {
	for _, entry := range daemonAgentLoopNoEngineDirectFallbackRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonAgentLoopNoEngineDirectFallbackRetirementEntry{}, false
}

func daemonAgentLoopNoEngineDirectFallbackRetirementEntriesForStage(stage daemonAgentLoopNoEngineDirectFallbackRetirementStage) []daemonAgentLoopNoEngineDirectFallbackRetirementEntry {
	var entries []daemonAgentLoopNoEngineDirectFallbackRetirementEntry
	for _, entry := range daemonAgentLoopNoEngineDirectFallbackRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonAgentLoopNoEngineDirectFallbackRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonAgentLoopNoEngineDirectFallbackRetirementEntriesForStage(daemonAgentLoopNoEngineDirectFallbackStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonAgentLoopNoEngineDirectFallbackRetirementAllowsDirectFallback(reason daemonAgentLoopCallerFallbackReason) bool {
	for _, entry := range daemonAgentLoopNoEngineDirectFallbackRetirementTable {
		if entry.reason == reason &&
			entry.disposition == daemonAgentLoopNoEngineDirectFallbackDispositionDirectLegacyFallback {
			return true
		}
	}
	return false
}

func daemonAgentLoopRunnerNotAttachedRetirementEntryForComponent(component string) (daemonAgentLoopRunnerNotAttachedRetirementEntry, bool) {
	for _, entry := range daemonAgentLoopRunnerNotAttachedRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonAgentLoopRunnerNotAttachedRetirementEntry{}, false
}

func daemonAgentLoopRunnerNotAttachedRetirementEntriesForStage(stage daemonAgentLoopRunnerNotAttachedRetirementStage) []daemonAgentLoopRunnerNotAttachedRetirementEntry {
	var entries []daemonAgentLoopRunnerNotAttachedRetirementEntry
	for _, entry := range daemonAgentLoopRunnerNotAttachedRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonAgentLoopRunnerNotAttachedRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonAgentLoopRunnerNotAttachedRetirementEntriesForStage(daemonAgentLoopRunnerNotAttachedStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonAgentLoopRunnerNotAttachedRetirementAllowsDirectFallback(reason daemonAgentLoopCallerFallbackReason) bool {
	for _, entry := range daemonAgentLoopRunnerNotAttachedRetirementTable {
		if entry.reason == reason &&
			entry.disposition == daemonAgentLoopRunnerNotAttachedDispositionDirectLegacyFallback {
			return true
		}
	}
	return false
}

func (r daemonAgentLoopRunner) RunAgentLoop(ctx context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
	if r.manager == nil {
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        "daemon agent loop runner manager not attached",
		}, fmt.Errorf("daemon agent loop runner manager not attached")
	}

	switch req.Mode {
	case runtime.AgentLoopModeDeterministicRoute:
		return r.manager.runAgentDeterministicRouteAgentLoopResult(ctx, req)
	case runtime.AgentLoopModeLLMToolLoop:
		return r.manager.runAgentLLMToolLoopAgentLoopResult(ctx, req)
	case runtime.AgentLoopModeReplanContinuation:
		return r.manager.runAgentReplanActionContinuationAgentLoopResult(ctx, req), nil
	case runtime.AgentLoopModePlannerActionReplanCycle:
		return r.manager.runAgentPlannerActionReplanCycleAgentLoop(ctx, req), nil
	case runtime.AgentLoopModePlannerActionExecution:
		return r.manager.runAgentPlannerActionExecutionAgentLoopResult(ctx, req)
	default:
		err := fmt.Errorf("unsupported agent loop mode %q", req.Mode)
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
		}, err
	}
}

func (m *agentManager) runRuntimeAgentLoopResult(ctx context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req = req.Clone()
	eng := m.runtimeEngine()
	if reason, ok := daemonAgentLoopCallerPreDispatchFallbackReason(eng); ok {
		if !daemonAgentLoopRunnerNotAttachedRetirementAllowsDirectFallback(reason) {
			blocked, _ := daemonAgentLoopCallerRunnerNotAttachedBlockedResult(req, runtime.AgentLoopResult{}, runtime.ErrAgentLoopRunnerNotAttached)
			return blocked, runtime.ErrAgentLoopRunnerNotAttached
		}
		if !daemonAgentLoopCallerFallbackOwnedByLegacyCaller(reason) {
			return runtime.AgentLoopResult{}, daemonAgentLoopCallerFallbackOwnershipError(reason)
		}
		if !daemonAgentLoopNoEngineDirectFallbackRetirementAllowsDirectFallback(reason) {
			return runtime.AgentLoopResult{}, daemonAgentLoopCallerFallbackOwnershipError(reason)
		}
		return m.runDaemonAgentLoopDirectResult(ctx, req)
	}
	result, err := eng.RunAgentLoop(ctx, req)
	if blocked, ok := daemonAgentLoopCallerRunnerNotAttachedBlockedResult(req, result, err); ok {
		return blocked, err
	}
	if reason, ok := daemonAgentLoopCallerResultFallbackReason(err); ok {
		if !daemonAgentLoopCallerFallbackOwnedByLegacyCaller(reason) {
			return runtime.AgentLoopResult{}, daemonAgentLoopCallerFallbackOwnershipError(reason)
		}
		return m.runDaemonAgentLoopDirectResult(ctx, req)
	}
	return result, err
}

func (m *agentManager) runDaemonAgentLoopDirectResult(ctx context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
	switch req.Mode {
	case runtime.AgentLoopModeDeterministicRoute:
		return m.runAgentDeterministicRouteAgentLoopResult(ctx, req)
	case runtime.AgentLoopModeLLMToolLoop:
		return m.runAgentLLMToolLoopAgentLoopResult(ctx, req)
	case runtime.AgentLoopModeReplanContinuation:
		return m.runAgentReplanActionContinuationAgentLoopResult(ctx, req), nil
	case runtime.AgentLoopModePlannerActionReplanCycle:
		return m.runAgentPlannerActionReplanCycleAgentLoop(ctx, req), nil
	case runtime.AgentLoopModePlannerActionExecution:
		return m.runAgentPlannerActionExecutionAgentLoopResult(ctx, req)
	default:
		err := fmt.Errorf("unsupported agent loop mode %q", req.Mode)
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
		}, err
	}
}
