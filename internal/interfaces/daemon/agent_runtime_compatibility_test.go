package daemon

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints(t *testing.T) {
	wantEntries := map[daemonRuntimeCompatibilityTrampolineKind]daemonRuntimeCompatibilityTrampolineEntry{
		daemonRuntimeCompatibilityTrampolineLLMTurnRunner: {
			kind:       daemonRuntimeCompatibilityTrampolineLLMTurnRunner,
			mode:       runtime.AgentLoopModeLLMToolLoop,
			entrypoint: daemonRuntimeCompatibilityEntrypointLLMTurnRunner,
			target:     "agentManager.runRuntimeAgentLoopResult",
			scope:      daemonRuntimeCompatibilityTrampolineScopeEngineSeam,
		},
		daemonRuntimeCompatibilityTrampolineDeterministicRouteTurnExecutor: {
			kind:       daemonRuntimeCompatibilityTrampolineDeterministicRouteTurnExecutor,
			mode:       runtime.AgentLoopModeDeterministicRoute,
			entrypoint: daemonRuntimeCompatibilityEntrypointDeterministicRouteTurnExecutor,
			target:     "agentManager.runAgentDeterministicRouteAgentLoopResult",
			scope:      daemonRuntimeCompatibilityTrampolineScopeEngineSeam,
		},
		daemonRuntimeCompatibilityTrampolineReplanActionContinuationTurnExecutor: {
			kind:       daemonRuntimeCompatibilityTrampolineReplanActionContinuationTurnExecutor,
			mode:       runtime.AgentLoopModeReplanContinuation,
			entrypoint: daemonRuntimeCompatibilityEntrypointReplanActionContinuationTurnExecutor,
			target:     "agentManager.runRuntimeAgentLoopResult",
			scope:      daemonRuntimeCompatibilityTrampolineScopeEngineSeam,
		},
		daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor: {
			kind:       daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor,
			mode:       runtime.AgentLoopModePlannerActionExecution,
			entrypoint: daemonRuntimeCompatibilityEntrypointPlannerActionExecutionTurnExecutor,
			target:     "daemonAgentLoopActionExecutor.ExecuteAgentLoopAction",
			scope:      daemonRuntimeCompatibilityTrampolineScopeEngineSeam,
		},
	}
	seenKinds := map[daemonRuntimeCompatibilityTrampolineKind]daemonRuntimeCompatibilityTrampolineEntry{}
	seenEntrypoints := map[string]daemonRuntimeCompatibilityTrampolineKind{}

	for _, entry := range daemonRuntimeCompatibilityTrampolineTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.mode)
		require.NotEmpty(t, entry.entrypoint)
		require.NotEmpty(t, entry.target)
		require.NotEmpty(t, entry.scope)
		require.Empty(t, seenKinds[entry.kind].entrypoint, "duplicate compatibility trampoline kind %q", entry.kind)
		require.Empty(t, seenEntrypoints[entry.entrypoint], "duplicate compatibility trampoline entrypoint %q", entry.entrypoint)
		seenKinds[entry.kind] = entry
		seenEntrypoints[entry.entrypoint] = entry.kind

		byKind, ok := daemonRuntimeCompatibilityTrampolineEntryForKind(entry.kind)
		require.True(t, ok)
		require.Equal(t, entry, byKind)

		byEntrypoint, ok := daemonRuntimeCompatibilityTrampolineEntryForEntrypoint(entry.entrypoint)
		require.True(t, ok)
		require.Equal(t, entry, byEntrypoint)
	}

	require.Equal(t, wantEntries, seenKinds)
	_, ok := daemonRuntimeCompatibilityTrampolineEntryForKind(daemonRuntimeCompatibilityTrampolineKind("unsupported"))
	require.False(t, ok)
	_, ok = daemonRuntimeCompatibilityTrampolineEntryForEntrypoint("unsupported")
	require.False(t, ok)
}

func TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive(t *testing.T) {
	for _, entrypoint := range []string{
		daemonRuntimeDirectEntrypointDeterministicRouteAgentLoop,
		daemonRuntimeDirectEntrypointLLMToolLoopAgentLoop,
		daemonRuntimeDirectEntrypointReplanContinuationAgentLoop,
		"agentManager.runAgentDeterministicRouteAgentLoopResult",
		"agentManager.runRuntimeLLMToolLoop",
		"agentManager.runRuntimeReplanActionContinuation",
	} {
		_, ok := daemonRuntimeCompatibilityTrampolineEntryForEntrypoint(entrypoint)
		require.False(t, ok, "%s must not be audited as an active compatibility trampoline", entrypoint)
	}
}

func TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines(t *testing.T) {
	wantOwnership := map[daemonRuntimeCompatibilityTrampolineKind]daemonRuntimeCompatibilityTrampolineOwnershipEntry{
		daemonRuntimeCompatibilityTrampolineLLMTurnRunner: {
			kind:          daemonRuntimeCompatibilityTrampolineLLMTurnRunner,
			owner:         daemonRuntimeCompatibilityTrampolineOwnerExternalEngineSeam,
			replacement:   "runtime.Engine.RunAgentLoop",
			stage:         daemonRuntimeCompatibilityTrampolineRemovalStageActiveCompatibility,
			removalGate:   "migrate runtime.Engine.RunLLMTurn compatibility callers to Engine.RunAgentLoop",
			apiMarker:     daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
			apiNote:       "runtime.Engine.RunLLMTurn and LLMTurnRunner are marked Deprecated; prefer runtime.Engine.RunAgentLoop",
			callerSurface: daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly,
			callerAudit:   "no non-test RunLLMTurn callers outside runtime API definitions, daemon trampoline implementation, AttachEngine attachment, and compatibility audit metadata",
		},
		daemonRuntimeCompatibilityTrampolineDeterministicRouteTurnExecutor: {
			kind:          daemonRuntimeCompatibilityTrampolineDeterministicRouteTurnExecutor,
			owner:         daemonRuntimeCompatibilityTrampolineOwnerExternalEngineSeam,
			replacement:   "runtime.Engine.RunAgentLoop",
			stage:         daemonRuntimeCompatibilityTrampolineRemovalStageActiveCompatibility,
			removalGate:   "migrate external runtime.Engine.ExecuteTurn deterministic_route callers to Engine.RunAgentLoop",
			apiMarker:     daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
			apiNote:       "runtime.Engine.ExecuteTurn and TurnExecutor are marked Deprecated; internal deterministic_route callers now use Engine.RunAgentLoop",
			callerSurface: daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly,
			callerAudit:   "runRuntimeDeterministicRoute uses RunAgentLoop; deterministic route TurnExecutor branch remains reachable only through attached runtime.Engine.ExecuteTurn compatibility input",
		},
		daemonRuntimeCompatibilityTrampolineReplanActionContinuationTurnExecutor: {
			kind:          daemonRuntimeCompatibilityTrampolineReplanActionContinuationTurnExecutor,
			owner:         daemonRuntimeCompatibilityTrampolineOwnerExternalEngineSeam,
			replacement:   "runtime.Engine.RunAgentLoop",
			stage:         daemonRuntimeCompatibilityTrampolineRemovalStageActiveCompatibility,
			removalGate:   "migrate runtime.Engine.ExecuteTurn replan_action_continuation callers to Engine.RunAgentLoop",
			apiMarker:     daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
			apiNote:       "runtime.Engine.ExecuteTurn and TurnExecutor are marked Deprecated; replan_action_continuation migration remains gated on Engine.RunAgentLoop",
			callerSurface: daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly,
			callerAudit:   "no non-test agentReplanActionContinuationTurnContextFromRequest references; replan branch remains reachable only through attached runtime.Engine.ExecuteTurn compatibility input",
		},
		daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor: {
			kind:          daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor,
			owner:         daemonRuntimeCompatibilityTrampolineOwnerExternalEngineSeam,
			replacement:   "runtime.Engine.RunAgentLoop / runtime.Engine.ExecuteAgentLoopAction",
			stage:         daemonRuntimeCompatibilityTrampolineRemovalStageActiveCompatibility,
			removalGate:   "migrate runtime.Engine.ExecuteTurn planner_action_execution callers to Engine.RunAgentLoop or Engine.ExecuteAgentLoopAction",
			apiMarker:     daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
			apiNote:       "runtime.Engine.ExecuteTurn and TurnExecutor are marked Deprecated; internal planner_action_execution callers now use Engine.RunAgentLoop and Engine.ExecuteAgentLoopAction",
			callerSurface: daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly,
			callerAudit:   "runRuntimePlannerActionExecution uses RunAgentLoop; planner action TurnExecutor branch remains reachable only through attached runtime.Engine.ExecuteTurn compatibility input",
		},
	}
	seenOwnership := map[daemonRuntimeCompatibilityTrampolineKind]daemonRuntimeCompatibilityTrampolineOwnershipEntry{}
	activeTrampolines := map[daemonRuntimeCompatibilityTrampolineKind]bool{}

	for _, entry := range daemonRuntimeCompatibilityTrampolineTable {
		activeTrampolines[entry.kind] = true
	}
	for _, entry := range daemonRuntimeCompatibilityTrampolineOwnershipTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.owner)
		require.NotEmpty(t, entry.replacement)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.removalGate)
		require.NotEmpty(t, entry.apiMarker)
		require.NotEmpty(t, entry.apiNote)
		require.NotEmpty(t, entry.callerSurface)
		require.NotEmpty(t, entry.callerAudit)
		require.Empty(t, seenOwnership[entry.kind].owner, "duplicate compatibility trampoline ownership for %q", entry.kind)
		seenOwnership[entry.kind] = entry
		require.True(t, activeTrampolines[entry.kind], "ownership entry %q must map to an active compatibility trampoline", entry.kind)

		byKind, ok := daemonRuntimeCompatibilityTrampolineOwnershipForKind(entry.kind)
		require.True(t, ok)
		require.Equal(t, entry, byKind)
		require.True(t, daemonRuntimeCompatibilityTrampolineOwnedByExternalEngineSeam(entry.kind))
		require.False(t, daemonRuntimeCompatibilityTrampolineReadyForRemoval(entry.kind))
	}

	require.Equal(t, wantOwnership, seenOwnership)
	require.Len(t, seenOwnership, len(activeTrampolines))
	_, ok := daemonRuntimeCompatibilityTrampolineOwnershipForKind(daemonRuntimeCompatibilityTrampolineKind("unsupported"))
	require.False(t, ok)
	require.False(t, daemonRuntimeCompatibilityTrampolineOwnedByExternalEngineSeam(daemonRuntimeCompatibilityTrampolineKind("unsupported")))
	require.False(t, daemonRuntimeCompatibilityTrampolineReadyForRemoval(daemonRuntimeCompatibilityTrampolineKind("unsupported")))
}

func TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus(t *testing.T) {
	for _, tc := range []struct {
		kind daemonRuntimeCompatibilityTrampolineKind
		want daemonRuntimeCompatibilityRuntimeAPIMarker
	}{
		{
			kind: daemonRuntimeCompatibilityTrampolineLLMTurnRunner,
			want: daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
		},
		{
			kind: daemonRuntimeCompatibilityTrampolineDeterministicRouteTurnExecutor,
			want: daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
		},
		{
			kind: daemonRuntimeCompatibilityTrampolineReplanActionContinuationTurnExecutor,
			want: daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
		},
		{
			kind: daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor,
			want: daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
		},
	} {
		got, ok := daemonRuntimeCompatibilityTrampolineRuntimeAPIMarkerForKind(tc.kind)
		require.True(t, ok)
		require.Equal(t, tc.want, got)
		require.False(t, daemonRuntimeCompatibilityTrampolineReadyForRemoval(tc.kind))
	}

	_, ok := daemonRuntimeCompatibilityTrampolineRuntimeAPIMarkerForKind(daemonRuntimeCompatibilityTrampolineKind("unsupported"))
	require.False(t, ok)
}

func TestDaemonRuntimeCompatibilityTrampolineCallerSurfacesDocumentPostCleanupState(t *testing.T) {
	for _, kind := range []daemonRuntimeCompatibilityTrampolineKind{
		daemonRuntimeCompatibilityTrampolineLLMTurnRunner,
		daemonRuntimeCompatibilityTrampolineDeterministicRouteTurnExecutor,
		daemonRuntimeCompatibilityTrampolineReplanActionContinuationTurnExecutor,
		daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor,
	} {
		got, ok := daemonRuntimeCompatibilityTrampolineCallerSurfaceForKind(kind)
		require.True(t, ok)
		require.Equal(t, daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly, got)
		require.True(t, daemonRuntimeCompatibilityTrampolineOwnedByExternalEngineSeam(kind))
		require.False(t, daemonRuntimeCompatibilityTrampolineReadyForRemoval(kind))
	}

	_, ok := daemonRuntimeCompatibilityTrampolineCallerSurfaceForKind(daemonRuntimeCompatibilityTrampolineKind("unsupported"))
	require.False(t, ok)
}

func TestDaemonLLMTurnRunnerCompatibilityRetirementAuditNarrowsToExternalSeam(t *testing.T) {
	wantStages := map[string]daemonLLMTurnRunnerCompatibilityRetirementStage{
		"runtime_engine_run_llm_turn_api":      daemonLLMTurnRunnerCompatibilityStageRuntimeAPIDeprecatedRetained,
		"daemon_llm_turn_runner_trampoline":    daemonLLMTurnRunnerCompatibilityStageInternalCallersRetired,
		"send_message_llm_tool_loop_main_path": daemonLLMTurnRunnerCompatibilityStageAgentLoopMainPathOwned,
		"attach_engine_llm_turn_runner":        daemonLLMTurnRunnerCompatibilityStageDefaultAttachmentRetained,
		"llm_turn_runner_next_boundary":        daemonLLMTurnRunnerCompatibilityStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonLLMTurnRunnerCompatibilityRetirementStage{}

	for _, entry := range daemonLLMTurnRunnerCompatibilityRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonLLMTurnRunnerCompatibilityNextBoundaryTurnExecutorCompatibility, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate LLMTurnRunner compatibility retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonLLMTurnRunnerCompatibilityRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonLLMTurnRunnerCompatibilityRetirementEntriesForStage(daemonLLMTurnRunnerCompatibilityStageRuntimeAPIDeprecatedRetained), 1)
	require.Len(t, daemonLLMTurnRunnerCompatibilityRetirementEntriesForStage(daemonLLMTurnRunnerCompatibilityStageInternalCallersRetired), 1)
	require.Len(t, daemonLLMTurnRunnerCompatibilityRetirementEntriesForStage(daemonLLMTurnRunnerCompatibilityStageDefaultAttachmentRetained), 1)
	require.Len(t, daemonLLMTurnRunnerCompatibilityRetirementEntriesForStage(daemonLLMTurnRunnerCompatibilityStageAgentLoopMainPathOwned), 1)
	require.Len(t, daemonLLMTurnRunnerCompatibilityRetirementEntriesForStage(daemonLLMTurnRunnerCompatibilityStageNextBoundaryChosen), 1)
	require.Empty(t, daemonLLMTurnRunnerCompatibilityRetirementEntriesForStage("unsupported"))

	selectedBoundary, ok := daemonLLMTurnRunnerCompatibilityRetirementSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonLLMTurnRunnerCompatibilityNextBoundaryTurnExecutorCompatibility, selectedBoundary)

	ownership, ok := daemonRuntimeCompatibilityTrampolineOwnershipForKind(daemonRuntimeCompatibilityTrampolineLLMTurnRunner)
	require.True(t, ok)
	require.Equal(t, daemonRuntimeCompatibilityTrampolineRemovalStageActiveCompatibility, ownership.stage)
	require.Equal(t, daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly, ownership.callerSurface)
	require.False(t, daemonRuntimeCompatibilityTrampolineReadyForRemoval(daemonRuntimeCompatibilityTrampolineLLMTurnRunner))
}

func TestDaemonLLMTurnRunnerCompatibilitySourceAuditKeepsMainPathOnAgentLoopRunner(t *testing.T) {
	runLLMTurnCalls := productionCallSitesNamed(t, "RunLLMTurn", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"../../agent/runtime/llm_turn_runner.go:RunLLMTurn"}, runLLMTurnCalls)

	attachLLMTurnRunnerCalls := productionCallSitesNamed(t, "AttachLLMTurnRunner", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"agent_runtime.go:AttachLLMTurnRunner"}, attachLLMTurnRunnerCalls)

	sessionFlowData, err := os.ReadFile("agent_session_flow.go")
	require.NoError(t, err)
	sessionFlow := string(sessionFlowData)
	require.NotContains(t, sessionFlow, "RunLLMTurn")
	require.Contains(t, sessionFlow, "runRuntimeLLMToolLoopAgentLoop")

	llmTurnRunnerData, err := os.ReadFile("agent_llm_turn_runner.go")
	require.NoError(t, err)
	llmTurnRunner := string(llmTurnRunnerData)
	require.Contains(t, llmTurnRunner, "runRuntimeAgentLoopResult(ctx, loop)")
	require.NotContains(t, llmTurnRunner, "runAgentLLMToolLoop(ctx")
}

func TestDaemonTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam(t *testing.T) {
	wantStages := map[string]daemonTurnExecutorCompatibilityRetirementStage{
		"runtime_engine_execute_turn_api":                     daemonTurnExecutorCompatibilityStageRuntimeAPICompatibilityRetained,
		"deterministic_route_turn_executor_trampoline":        daemonTurnExecutorCompatibilityStageDeterministicTrampolineExternal,
		"replan_action_continuation_turn_executor_trampoline": daemonTurnExecutorCompatibilityStageReplanTrampolineExternal,
		"attach_engine_turn_executor":                         daemonTurnExecutorCompatibilityStageDefaultAttachmentRetained,
		"deterministic_route_agent_loop_main_path":            daemonTurnExecutorCompatibilityStageAgentLoopMainPathOwned,
		"replan_action_continuation_agent_loop_main_path":     daemonTurnExecutorCompatibilityStageAgentLoopMainPathOwned,
		"turn_executor_next_boundary":                         daemonTurnExecutorCompatibilityStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonTurnExecutorCompatibilityRetirementStage{}

	for _, entry := range daemonTurnExecutorCompatibilityRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonTurnExecutorCompatibilityNextBoundaryPlannerActionExecutionCompatibility, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate TurnExecutor compatibility retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonTurnExecutorCompatibilityRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonTurnExecutorCompatibilityRetirementEntriesForStage(daemonTurnExecutorCompatibilityStageRuntimeAPICompatibilityRetained), 1)
	require.Len(t, daemonTurnExecutorCompatibilityRetirementEntriesForStage(daemonTurnExecutorCompatibilityStageDeterministicTrampolineExternal), 1)
	require.Len(t, daemonTurnExecutorCompatibilityRetirementEntriesForStage(daemonTurnExecutorCompatibilityStageReplanTrampolineExternal), 1)
	require.Len(t, daemonTurnExecutorCompatibilityRetirementEntriesForStage(daemonTurnExecutorCompatibilityStageDefaultAttachmentRetained), 1)
	require.Len(t, daemonTurnExecutorCompatibilityRetirementEntriesForStage(daemonTurnExecutorCompatibilityStageAgentLoopMainPathOwned), 2)
	require.Len(t, daemonTurnExecutorCompatibilityRetirementEntriesForStage(daemonTurnExecutorCompatibilityStageNextBoundaryChosen), 1)
	require.Empty(t, daemonTurnExecutorCompatibilityRetirementEntriesForStage("unsupported"))

	selectedBoundary, ok := daemonTurnExecutorCompatibilityRetirementSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonTurnExecutorCompatibilityNextBoundaryPlannerActionExecutionCompatibility, selectedBoundary)

	for _, kind := range []daemonRuntimeCompatibilityTrampolineKind{
		daemonRuntimeCompatibilityTrampolineDeterministicRouteTurnExecutor,
		daemonRuntimeCompatibilityTrampolineReplanActionContinuationTurnExecutor,
	} {
		ownership, ok := daemonRuntimeCompatibilityTrampolineOwnershipForKind(kind)
		require.True(t, ok)
		require.Equal(t, daemonRuntimeCompatibilityTrampolineRemovalStageActiveCompatibility, ownership.stage)
		require.Equal(t, daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly, ownership.callerSurface)
		require.False(t, daemonRuntimeCompatibilityTrampolineReadyForRemoval(kind))
	}
}

func TestDaemonTurnExecutorCompatibilitySourceAuditKeepsMainPathsOnAgentLoopRunner(t *testing.T) {
	executeTurnCalls := productionCallSitesNamed(t, "ExecuteTurn", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"../../agent/runtime/turn_executor.go:ExecuteTurn"}, executeTurnCalls)

	attachTurnExecutorCalls := productionCallSitesNamed(t, "AttachTurnExecutor", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"agent_runtime.go:AttachTurnExecutor"}, attachTurnExecutorCalls)

	sessionFlowData, err := os.ReadFile("agent_session_flow.go")
	require.NoError(t, err)
	sessionFlow := string(sessionFlowData)
	require.NotContains(t, sessionFlow, "ExecuteTurn")
	require.Contains(t, sessionFlow, "runRuntimeReplanActionContinuationAgentLoop")

	turnExecutorData, err := os.ReadFile("agent_turn_executor.go")
	require.NoError(t, err)
	turnExecutor := string(turnExecutorData)
	require.Contains(t, turnExecutor, "func (m *agentManager) runRuntimeDeterministicRouteWithToolServices")
	require.Contains(t, turnExecutor, "loopResult, err := m.runRuntimeAgentLoopResult(ctx, loopReq)")
	require.Contains(t, turnExecutor, "func (m *agentManager) runRuntimeReplanActionContinuationAgentLoop")
	require.Contains(t, turnExecutor, "loopResult, err := e.manager.runRuntimeAgentLoopResult(ctx, loopReq)")
	require.Contains(t, turnExecutor, "loopResult, err := e.manager.runAgentDeterministicRouteAgentLoopResult(ctx, loopReq)")
}

func TestDaemonPlannerActionExecutionTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam(t *testing.T) {
	wantStages := map[string]daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementStage{
		"runtime_engine_execute_turn_api_after_planner_audit": daemonPlannerActionExecutionTurnExecutorCompatibilityStageRuntimeAPIExternalOnly,
		"planner_action_execution_turn_executor_trampoline":   daemonPlannerActionExecutionTurnExecutorCompatibilityStagePlannerTrampolineExternal,
		"planner_action_execution_internal_callers":           daemonPlannerActionExecutionTurnExecutorCompatibilityStageInternalCallersRetired,
		"planner_action_execution_agent_loop_main_path":       daemonPlannerActionExecutionTurnExecutorCompatibilityStageAgentLoopMainPathOwned,
		"planner_action_execution_turn_executor_target":       daemonPlannerActionExecutionTurnExecutorCompatibilityStageActionExecutorTargetOwned,
		"attach_engine_turn_executor_after_planner_audit":     daemonPlannerActionExecutionTurnExecutorCompatibilityStageDefaultAttachmentRetained,
		"execute_turn_api_next_boundary":                      daemonPlannerActionExecutionTurnExecutorCompatibilityStageExecuteTurnBoundaryChosen,
	}
	seenStages := map[string]daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementStage{}

	for _, entry := range daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonPlannerActionExecutionTurnExecutorCompatibilityNextBoundaryExecuteTurnAPI, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate planner action TurnExecutor compatibility retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnExecutorCompatibilityStageRuntimeAPIExternalOnly), 1)
	require.Len(t, daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnExecutorCompatibilityStagePlannerTrampolineExternal), 1)
	require.Len(t, daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnExecutorCompatibilityStageInternalCallersRetired), 1)
	require.Len(t, daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnExecutorCompatibilityStageAgentLoopMainPathOwned), 1)
	require.Len(t, daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnExecutorCompatibilityStageActionExecutorTargetOwned), 1)
	require.Len(t, daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnExecutorCompatibilityStageDefaultAttachmentRetained), 1)
	require.Len(t, daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnExecutorCompatibilityStageExecuteTurnBoundaryChosen), 1)
	require.Empty(t, daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementEntriesForStage("unsupported"))

	selectedBoundary, ok := daemonPlannerActionExecutionTurnExecutorCompatibilityRetirementSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionTurnExecutorCompatibilityNextBoundaryExecuteTurnAPI, selectedBoundary)

	ownership, ok := daemonRuntimeCompatibilityTrampolineOwnershipForKind(daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor)
	require.True(t, ok)
	require.Equal(t, daemonRuntimeCompatibilityTrampolineRemovalStageActiveCompatibility, ownership.stage)
	require.Equal(t, daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly, ownership.callerSurface)
	require.False(t, daemonRuntimeCompatibilityTrampolineReadyForRemoval(daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor))
}

func TestDaemonPlannerActionExecutionTurnExecutorCompatibilitySourceAuditKeepsMainPathOnAgentLoopRunner(t *testing.T) {
	executeTurnCalls := productionCallSitesNamed(t, "ExecuteTurn", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"../../agent/runtime/turn_executor.go:ExecuteTurn"}, executeTurnCalls)

	attachTurnExecutorCalls := productionCallSitesNamed(t, "AttachTurnExecutor", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"agent_runtime.go:AttachTurnExecutor"}, attachTurnExecutorCalls)

	turnExecutorData, err := os.ReadFile("agent_turn_executor.go")
	require.NoError(t, err)
	turnExecutor := string(turnExecutorData)
	require.Contains(t, turnExecutor, "func (m *agentManager) runRuntimePlannerActionExecution")
	require.Contains(t, turnExecutor, "loopReq.Mode = runtime.AgentLoopModePlannerActionExecution")
	require.Contains(t, turnExecutor, "loopResult, err := m.runRuntimeAgentLoopResult(ctx, loopReq)")

	plannerTurnStart := strings.Index(turnExecutor, "func (e daemonAgentTurnExecutor) executePlannerActionExecutionTurn")
	require.NotEqual(t, -1, plannerTurnStart)
	plannerTurnEnd := strings.Index(turnExecutor[plannerTurnStart:], "\nfunc daemonTurnExecutionErrorResult")
	require.NotEqual(t, -1, plannerTurnEnd)
	plannerTurnBranch := turnExecutor[plannerTurnStart : plannerTurnStart+plannerTurnEnd]
	require.Contains(t, plannerTurnBranch, "daemonAgentLoopActionExecutor{manager: e.manager}.ExecuteAgentLoopAction")
	require.NotContains(t, plannerTurnBranch, "daemonPlannerActionExecutionExecutorForManager")

	loopRunnerData, err := os.ReadFile("agent_loop_runner.go")
	require.NoError(t, err)
	loopRunner := string(loopRunnerData)
	require.Contains(t, loopRunner, "case runtime.AgentLoopModePlannerActionExecution:")
	require.Contains(t, loopRunner, "return r.manager.runAgentPlannerActionExecutionAgentLoopResult(ctx, req)")
}

func TestDaemonExecuteTurnAPICompatibilityRetirementAuditMarksDeprecatedExternalSeam(t *testing.T) {
	wantStages := map[string]daemonExecuteTurnAPICompatibilityRetirementStage{
		"runtime_engine_execute_turn_api":       daemonExecuteTurnAPICompatibilityStageRuntimeAPIDeprecatedRetained,
		"runtime_turn_executor_interface":       daemonExecuteTurnAPICompatibilityStageTurnExecutorDeprecatedRetained,
		"execute_turn_internal_callers":         daemonExecuteTurnAPICompatibilityStageInternalCallersRetired,
		"execute_turn_trampolines_all_branches": daemonExecuteTurnAPICompatibilityStageAllTrampolinesExternalOnly,
		"attach_engine_turn_executor":           daemonExecuteTurnAPICompatibilityStageDefaultAttachmentRetained,
		"execute_turn_api_removal_gate":         daemonExecuteTurnAPICompatibilityStageRemovalBlockedByExternalPolicy,
		"execute_turn_api_next_boundary":        daemonExecuteTurnAPICompatibilityStageDeprecatedAPIPolicyBoundaryChosen,
	}
	seenStages := map[string]daemonExecuteTurnAPICompatibilityRetirementStage{}

	for _, entry := range daemonExecuteTurnAPICompatibilityRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonExecuteTurnAPICompatibilityNextBoundaryDeprecatedAPIPolicy, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate ExecuteTurn API compatibility retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonExecuteTurnAPICompatibilityRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonExecuteTurnAPICompatibilityRetirementEntriesForStage(daemonExecuteTurnAPICompatibilityStageRuntimeAPIDeprecatedRetained), 1)
	require.Len(t, daemonExecuteTurnAPICompatibilityRetirementEntriesForStage(daemonExecuteTurnAPICompatibilityStageTurnExecutorDeprecatedRetained), 1)
	require.Len(t, daemonExecuteTurnAPICompatibilityRetirementEntriesForStage(daemonExecuteTurnAPICompatibilityStageAllTrampolinesExternalOnly), 1)
	require.Len(t, daemonExecuteTurnAPICompatibilityRetirementEntriesForStage(daemonExecuteTurnAPICompatibilityStageInternalCallersRetired), 1)
	require.Len(t, daemonExecuteTurnAPICompatibilityRetirementEntriesForStage(daemonExecuteTurnAPICompatibilityStageDefaultAttachmentRetained), 1)
	require.Len(t, daemonExecuteTurnAPICompatibilityRetirementEntriesForStage(daemonExecuteTurnAPICompatibilityStageRemovalBlockedByExternalPolicy), 1)
	require.Len(t, daemonExecuteTurnAPICompatibilityRetirementEntriesForStage(daemonExecuteTurnAPICompatibilityStageDeprecatedAPIPolicyBoundaryChosen), 1)
	require.Empty(t, daemonExecuteTurnAPICompatibilityRetirementEntriesForStage("unsupported"))

	selectedBoundary, ok := daemonExecuteTurnAPICompatibilityRetirementSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonExecuteTurnAPICompatibilityNextBoundaryDeprecatedAPIPolicy, selectedBoundary)

	for _, kind := range []daemonRuntimeCompatibilityTrampolineKind{
		daemonRuntimeCompatibilityTrampolineDeterministicRouteTurnExecutor,
		daemonRuntimeCompatibilityTrampolineReplanActionContinuationTurnExecutor,
		daemonRuntimeCompatibilityTrampolinePlannerActionExecutionTurnExecutor,
	} {
		ownership, ok := daemonRuntimeCompatibilityTrampolineOwnershipForKind(kind)
		require.True(t, ok)
		require.Equal(t, daemonRuntimeCompatibilityTrampolineOwnerExternalEngineSeam, ownership.owner)
		require.Equal(t, daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated, ownership.apiMarker)
		require.Equal(t, daemonRuntimeCompatibilityTrampolineRemovalStageActiveCompatibility, ownership.stage)
		require.Equal(t, daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly, ownership.callerSurface)
		require.False(t, daemonRuntimeCompatibilityTrampolineReadyForRemoval(kind))
	}
}

func TestDaemonExecuteTurnAPICompatibilitySourceAuditMarksDeprecatedExternalOnly(t *testing.T) {
	executeTurnCalls := productionCallSitesNamed(t, "ExecuteTurn", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"../../agent/runtime/turn_executor.go:ExecuteTurn"}, executeTurnCalls)

	attachTurnExecutorCalls := productionCallSitesNamed(t, "AttachTurnExecutor", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"agent_runtime.go:AttachTurnExecutor"}, attachTurnExecutorCalls)

	runtimeTurnExecutorData, err := os.ReadFile("../../agent/runtime/turn_executor.go")
	require.NoError(t, err)
	runtimeTurnExecutor := string(runtimeTurnExecutorData)
	require.Contains(t, runtimeTurnExecutor, "type TurnExecutor interface")
	require.Contains(t, runtimeTurnExecutor, "Deprecated: prefer AgentLoopRunner with Engine.RunAgentLoop")
	require.Contains(t, runtimeTurnExecutor, "Deprecated: use AgentLoopRunnerFunc or AgentLoopActionExecutorFunc")
	require.Contains(t, runtimeTurnExecutor, "func (e *Engine) AttachTurnExecutor")
	require.Contains(t, runtimeTurnExecutor, "Deprecated: attach an AgentLoopRunner/AgentLoopActionExecutor")
	require.Contains(t, runtimeTurnExecutor, "func (e *Engine) TurnExecutor() TurnExecutor")
	require.Contains(t, runtimeTurnExecutor, "Deprecated: use AgentLoopRunner or AgentLoopActionExecutor")
	require.Contains(t, runtimeTurnExecutor, "func (e *Engine) ExecuteTurn")
	require.Contains(t, runtimeTurnExecutor, "Deprecated: prefer Engine.RunAgentLoop for AgentLoop-shaped callers")

	turnExecutorData, err := os.ReadFile("agent_turn_executor.go")
	require.NoError(t, err)
	turnExecutor := string(turnExecutorData)
	require.Contains(t, turnExecutor, `turn.Metadata["kind"] == daemonTurnExecutionDeterministicRoute`)
	require.Contains(t, turnExecutor, `turn.Metadata["kind"] == daemonTurnExecutionReplanActionContinuation`)
	require.Contains(t, turnExecutor, `turn.Metadata["kind"] == daemonTurnExecutionPlannerActionExecution`)
	require.Contains(t, turnExecutor, "func (m *agentManager) runRuntimeDeterministicRouteWithToolServices")
	require.Contains(t, turnExecutor, "func (m *agentManager) runRuntimeReplanActionContinuationAgentLoop")
	require.Contains(t, turnExecutor, "func (m *agentManager) runRuntimePlannerActionExecution")

	sessionFlowData, err := os.ReadFile("agent_session_flow.go")
	require.NoError(t, err)
	sessionFlow := string(sessionFlowData)
	require.NotContains(t, sessionFlow, "ExecuteTurn")
	require.Contains(t, sessionFlow, "runRuntimeLLMToolLoopAgentLoop")
	require.Contains(t, sessionFlow, "runRuntimeReplanActionContinuationAgentLoop")
}

func TestDaemonDeprecatedRuntimeAPIPolicyUnifiesRunLLMTurnAndExecuteTurn(t *testing.T) {
	wantEntries := map[daemonDeprecatedRuntimeAPIPolicyFamily]daemonDeprecatedRuntimeAPIPolicyEntry{
		daemonDeprecatedRuntimeAPIPolicyFamilyRunLLMTurn: {
			family:                  daemonDeprecatedRuntimeAPIPolicyFamilyRunLLMTurn,
			runtimeAPI:              "runtime.Engine.RunLLMTurn",
			compatibilityRunner:     "runtime.LLMTurnRunner",
			attachAPI:               "runtime.Engine.AttachLLMTurnRunner",
			replacement:             "runtime.Engine.RunAgentLoop",
			apiMarker:               daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
			callerSurface:           daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly,
			migrationWindow:         daemonDeprecatedRuntimeAPIMigrationWindowCurrentReleaseGate,
			defaultAttachmentPolicy: daemonDeprecatedRuntimeAPIDefaultAttachmentRetainedUntilReleaseGate,
			optInPolicy:             daemonDeprecatedRuntimeAPIOptInDeferredUntilAfterMigrationWindow,
			removalGate:             "external callers must migrate to runtime.Engine.RunAgentLoop and release/API docs must announce the compatibility window before AttachLLMTurnRunner can become opt-in or be removed",
			evidence:                "non-test RunLLMTurn call sites are limited to the runtime API dispatch; SendMessage LLM main path uses runRuntimeLLMToolLoopAgentLoop and Engine.RunAgentLoop",
			nextBoundary:            daemonDeprecatedRuntimeAPIPolicyNextBoundaryReleaseDocsAndOptInAudit,
			nextStep:                "publish deprecated RunLLMTurn migration guidance and only then decide whether the default AttachLLMTurnRunner compatibility attachment becomes opt-in",
		},
		daemonDeprecatedRuntimeAPIPolicyFamilyExecuteTurn: {
			family:                  daemonDeprecatedRuntimeAPIPolicyFamilyExecuteTurn,
			runtimeAPI:              "runtime.Engine.ExecuteTurn",
			compatibilityRunner:     "runtime.TurnExecutor",
			attachAPI:               "runtime.Engine.AttachTurnExecutor",
			replacement:             "runtime.Engine.RunAgentLoop / runtime.Engine.ExecuteAgentLoopAction",
			apiMarker:               daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated,
			callerSurface:           daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly,
			migrationWindow:         daemonDeprecatedRuntimeAPIMigrationWindowCurrentReleaseGate,
			defaultAttachmentPolicy: daemonDeprecatedRuntimeAPIDefaultAttachmentRetainedUntilReleaseGate,
			optInPolicy:             daemonDeprecatedRuntimeAPIOptInDeferredUntilAfterMigrationWindow,
			removalGate:             "external callers must migrate to runtime.Engine.RunAgentLoop or Engine.ExecuteAgentLoopAction and release/API docs must announce the compatibility window before AttachTurnExecutor can become opt-in or be removed",
			evidence:                "non-test ExecuteTurn call sites are limited to the runtime API dispatch; deterministic, replan, and planner action main paths use AgentLoopRunner or ActionExecutor",
			nextBoundary:            daemonDeprecatedRuntimeAPIPolicyNextBoundaryReleaseDocsAndOptInAudit,
			nextStep:                "publish deprecated ExecuteTurn migration guidance and only then decide whether the default AttachTurnExecutor compatibility attachment becomes opt-in",
		},
	}
	seenEntries := map[daemonDeprecatedRuntimeAPIPolicyFamily]daemonDeprecatedRuntimeAPIPolicyEntry{}

	for _, entry := range daemonDeprecatedRuntimeAPIPolicyTable {
		require.NotEmpty(t, entry.family)
		require.NotEmpty(t, entry.runtimeAPI)
		require.NotEmpty(t, entry.compatibilityRunner)
		require.NotEmpty(t, entry.attachAPI)
		require.NotEmpty(t, entry.replacement)
		require.Equal(t, daemonRuntimeCompatibilityRuntimeAPIMarkerDeprecated, entry.apiMarker)
		require.Equal(t, daemonRuntimeCompatibilityTrampolineCallerSurfaceExternalEngineSeamOnly, entry.callerSurface)
		require.Equal(t, daemonDeprecatedRuntimeAPIMigrationWindowCurrentReleaseGate, entry.migrationWindow)
		require.Equal(t, daemonDeprecatedRuntimeAPIDefaultAttachmentRetainedUntilReleaseGate, entry.defaultAttachmentPolicy)
		require.Equal(t, daemonDeprecatedRuntimeAPIOptInDeferredUntilAfterMigrationWindow, entry.optInPolicy)
		require.NotEmpty(t, entry.removalGate)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPIPolicyNextBoundaryReleaseDocsAndOptInAudit, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenEntries[entry.family].runtimeAPI, "duplicate deprecated runtime API policy for %q", entry.family)
		seenEntries[entry.family] = entry

		got, ok := daemonDeprecatedRuntimeAPIPolicyEntryForFamily(entry.family)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantEntries, seenEntries)
	require.Len(t, daemonDeprecatedRuntimeAPIPolicyEntriesForDefaultAttachmentPolicy(daemonDeprecatedRuntimeAPIDefaultAttachmentRetainedUntilReleaseGate), 2)
	require.Empty(t, daemonDeprecatedRuntimeAPIPolicyEntriesForDefaultAttachmentPolicy("unsupported"))

	selectedBoundary, ok := daemonDeprecatedRuntimeAPIPolicySelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIPolicyNextBoundaryReleaseDocsAndOptInAudit, selectedBoundary)

	_, ok = daemonDeprecatedRuntimeAPIPolicyEntryForFamily(daemonDeprecatedRuntimeAPIPolicyFamily("unsupported"))
	require.False(t, ok)
}

func TestDaemonDeprecatedRuntimeAPIPolicySourceAuditKeepsRuntimeAPIsExternalOnly(t *testing.T) {
	runLLMTurnCalls := productionCallSitesNamed(t, "RunLLMTurn", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"../../agent/runtime/llm_turn_runner.go:RunLLMTurn"}, runLLMTurnCalls)

	executeTurnCalls := productionCallSitesNamed(t, "ExecuteTurn", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"../../agent/runtime/turn_executor.go:ExecuteTurn"}, executeTurnCalls)

	attachLLMTurnRunnerCalls := productionCallSitesNamed(t, "AttachLLMTurnRunner", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"agent_runtime.go:AttachLLMTurnRunner"}, attachLLMTurnRunnerCalls)

	attachTurnExecutorCalls := productionCallSitesNamed(t, "AttachTurnExecutor", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"agent_runtime.go:AttachTurnExecutor"}, attachTurnExecutorCalls)

	runtimeLLMTurnRunnerData, err := os.ReadFile("../../agent/runtime/llm_turn_runner.go")
	require.NoError(t, err)
	runtimeLLMTurnRunner := string(runtimeLLMTurnRunnerData)
	require.Contains(t, runtimeLLMTurnRunner, "type LLMTurnRunner interface")
	require.Contains(t, runtimeLLMTurnRunner, "Deprecated: prefer AgentLoopRunner with Engine.RunAgentLoop")
	require.Contains(t, runtimeLLMTurnRunner, "func (e *Engine) AttachLLMTurnRunner")
	require.Contains(t, runtimeLLMTurnRunner, "Deprecated: attach an AgentLoopRunner and call Engine.RunAgentLoop")
	require.Contains(t, runtimeLLMTurnRunner, "func (e *Engine) RunLLMTurn")
	require.Contains(t, runtimeLLMTurnRunner, "Deprecated: prefer Engine.RunAgentLoop for new bounded agent-loop callers")

	runtimeTurnExecutorData, err := os.ReadFile("../../agent/runtime/turn_executor.go")
	require.NoError(t, err)
	runtimeTurnExecutor := string(runtimeTurnExecutorData)
	require.Contains(t, runtimeTurnExecutor, "type TurnExecutor interface")
	require.Contains(t, runtimeTurnExecutor, "Deprecated: prefer AgentLoopRunner with Engine.RunAgentLoop")
	require.Contains(t, runtimeTurnExecutor, "func (e *Engine) AttachTurnExecutor")
	require.Contains(t, runtimeTurnExecutor, "Deprecated: attach an AgentLoopRunner/AgentLoopActionExecutor")
	require.Contains(t, runtimeTurnExecutor, "func (e *Engine) ExecuteTurn")
	require.Contains(t, runtimeTurnExecutor, "Deprecated: prefer Engine.RunAgentLoop for AgentLoop-shaped callers")

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachAgentLoopRunner(daemonAgentLoopRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachAgentLoopActionExecutor(daemonAgentLoopActionExecutor{manager: m})")
}

func TestDaemonDeprecatedRuntimeAPIReleaseDocsAndOptInAuditCoversCurrentGate(t *testing.T) {
	wantStages := map[string]daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStage{
		"release_notes_deprecated_runtime_api_notice":   daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageReleaseNotesCovered,
		"api_contract_deprecated_runtime_api_notice":    daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageAPIContractCovered,
		"release_checklist_deprecated_runtime_api_gate": daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageReleaseChecklistGate,
		"release_plan_migration_window":                 daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageReleasePlanCovered,
		"opt_in_compatibility_flag_policy":              daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageOptInFlagDeferred,
		"post_release_gate_next_boundary":               daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStage{}

	for _, entry := range daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.document)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.decision)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditNextBoundaryPostReleaseGateFlagDecision, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate deprecated runtime API release-docs audit entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntriesForStage(daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageReleaseNotesCovered), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntriesForStage(daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageAPIContractCovered), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntriesForStage(daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageReleaseChecklistGate), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntriesForStage(daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageReleasePlanCovered), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntriesForStage(daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageOptInFlagDeferred), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntriesForStage(daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditStageNextBoundaryChosen), 1)
	require.Empty(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntriesForStage("unsupported"))

	policyBoundary, ok := daemonDeprecatedRuntimeAPIPolicySelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIPolicyNextBoundaryReleaseDocsAndOptInAudit, policyBoundary)

	selectedBoundary, ok := daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditNextBoundaryPostReleaseGateFlagDecision, selectedBoundary)

	optInEntry, ok := daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditEntryForComponent("opt_in_compatibility_flag_policy")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditDecisionOptInDeferred, optInEntry.decision)
	require.Contains(t, optInEntry.evidence, "no new opt-in compatibility flag")
}

func TestDaemonDeprecatedRuntimeAPIReleaseDocsSourceAuditPublishesMigrationGuidance(t *testing.T) {
	requiredDocs := map[string][]string{
		"../../../docs/release-notes/v0.1.2.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.LLMTurnRunner",
			"runtime.Engine.ExecuteTurn",
			"runtime.TurnExecutor",
			"Deprecated",
			"current_release_gate",
			"runtime.Engine.RunAgentLoop",
			"runtime.Engine.ExecuteAgentLoopAction",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/api/v0.1.2-api-contract.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.LLMTurnRunner",
			"runtime.Engine.ExecuteTurn",
			"runtime.TurnExecutor",
			"Deprecated",
			"current_release_gate",
			"runtime.Engine.RunAgentLoop",
			"runtime.Engine.ExecuteAgentLoopAction",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/CyberAgent-Release-Checklist.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.LLMTurnRunner",
			"runtime.Engine.ExecuteTurn",
			"runtime.TurnExecutor",
			"Deprecated",
			"current_release_gate",
			"runtime.Engine.RunAgentLoop",
			"runtime.Engine.ExecuteAgentLoopAction",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/CyberAgent-Release-Plan.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.LLMTurnRunner",
			"runtime.Engine.ExecuteTurn",
			"runtime.TurnExecutor",
			"Deprecated",
			"current_release_gate",
			"runtime.Engine.RunAgentLoop",
			"runtime.Engine.ExecuteAgentLoopAction",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"retained_until_release_gate",
			"opt-in",
		},
	}

	for _, entry := range daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditTable {
		if strings.HasPrefix(entry.document, "docs/") {
			require.Contains(t, requiredDocs, "../../../"+entry.document)
		}
	}
	for path, terms := range requiredDocs {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		doc := string(data)
		for _, term := range terms {
			require.Contains(t, doc, term, "%s must publish deprecated runtime API migration guidance for %q", path, term)
		}
	}

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")
	require.NotContains(t, agentRuntime, "CYBERAGENT_RUNTIME_COMPAT")
	require.NotContains(t, agentRuntime, "CYBERAGENT_DEPRECATED_RUNTIME_API")
}

func TestDaemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionKeepsCurrentDefaults(t *testing.T) {
	wantStages := map[string]daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStage{
		"release_docs_boundary_consumed":            daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageCurrentGateActive,
		"attach_engine_llm_turn_runner_default":     daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageDefaultAttachmentRetained,
		"attach_engine_turn_executor_default":       daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageDefaultAttachmentRetained,
		"runtime_engine_fallback_switch_scope":      daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageExistingRuntimeFallbackOnly,
		"deprecated_runtime_api_opt_in_flag_absent": daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageExplicitCompatibilityFlagNotAdded,
		"next_release_gate_reassessment_boundary":   daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageNextReleaseGateReassessmentChosen,
	}
	seenStages := map[string]daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStage{}

	for _, entry := range daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.decision)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionNextBoundaryNextReleaseGateReassessment, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate deprecated runtime API post-release-gate flag decision entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntriesForStage(daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageCurrentGateActive), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntriesForStage(daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageDefaultAttachmentRetained), 2)
	require.Len(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntriesForStage(daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageExistingRuntimeFallbackOnly), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntriesForStage(daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageExplicitCompatibilityFlagNotAdded), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntriesForStage(daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionStageNextReleaseGateReassessmentChosen), 1)
	require.Empty(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntriesForStage("unsupported"))

	releaseDocsBoundary, ok := daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditNextBoundaryPostReleaseGateFlagDecision, releaseDocsBoundary)

	selectedBoundary, ok := daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionNextBoundaryNextReleaseGateReassessment, selectedBoundary)

	llmAttachment, ok := daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntryForComponent("attach_engine_llm_turn_runner_default")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionKeepDefaultAttachments, llmAttachment.decision)

	turnAttachment, ok := daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntryForComponent("attach_engine_turn_executor_default")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionKeepDefaultAttachments, turnAttachment.decision)

	flagEntry, ok := daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntryForComponent("deprecated_runtime_api_opt_in_flag_absent")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionDoNotIntroduceFlagBeforeGate, flagEntry.decision)

	_, ok = daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionEntryForComponent("unsupported")
	require.False(t, ok)
}

func TestDaemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionSourceAuditKeepsFlagAbsent(t *testing.T) {
	attachLLMTurnRunnerCalls := productionCallSitesNamed(t, "AttachLLMTurnRunner", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"agent_runtime.go:AttachLLMTurnRunner"}, attachLLMTurnRunnerCalls)

	attachTurnExecutorCalls := productionCallSitesNamed(t, "AttachTurnExecutor", "*.go", "../../agent/runtime/*.go")
	require.Equal(t, []string{"agent_runtime.go:AttachTurnExecutor"}, attachTurnExecutorCalls)

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "func (m *agentManager) AttachEngine(e *runtime.Engine)")
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachAgentLoopRunner(daemonAgentLoopRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachAgentLoopActionExecutor(daemonAgentLoopActionExecutor{manager: m})")

	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	server := string(serverData)
	require.Contains(t, server, `os.Getenv("CYBERAGENT_RUNTIME")`)
	require.Contains(t, server, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)

	for _, forbidden := range []string{
		"CYBERAGENT_RUNTIME_COMPAT",
		"CYBERAGENT_DEPRECATED_RUNTIME_API",
	} {
		require.NotContains(t, agentRuntime, forbidden)
		require.NotContains(t, server, forbidden)
	}
}

func TestDaemonDeprecatedRuntimeAPINextReleaseGateReassessmentBlocksFlagUntilGateComplete(t *testing.T) {
	wantStages := map[string]daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStage{
		"n224_post_release_gate_decision_boundary":        daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageN224BoundaryConsumed,
		"current_release_gate_completion_precondition":    daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageCurrentGateCompletionNeeded,
		"llm_turn_runner_default_attachment_preserved":    daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageDefaultAttachmentsPreserved,
		"turn_executor_default_attachment_preserved":      daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageDefaultAttachmentsPreserved,
		"dedicated_deprecated_runtime_api_flag_not_added": daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageNoDedicatedFlagBeforeGate,
		"current_release_gate_completion_next_boundary":   daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStage{}

	for _, entry := range daemonDeprecatedRuntimeAPINextReleaseGateReassessmentTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.decision)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentNextBoundaryCurrentGateCompletionEvidence, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate deprecated runtime API next-release-gate reassessment entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntriesForStage(daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageN224BoundaryConsumed), 1)
	require.Len(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntriesForStage(daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageCurrentGateCompletionNeeded), 1)
	require.Len(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntriesForStage(daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageDefaultAttachmentsPreserved), 2)
	require.Len(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntriesForStage(daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageNoDedicatedFlagBeforeGate), 1)
	require.Len(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntriesForStage(daemonDeprecatedRuntimeAPINextReleaseGateReassessmentStageNextBoundaryChosen), 1)
	require.Empty(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntriesForStage("unsupported"))

	postReleaseGateBoundary, ok := daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionNextBoundaryNextReleaseGateReassessment, postReleaseGateBoundary)

	selectedBoundary, ok := daemonDeprecatedRuntimeAPINextReleaseGateReassessmentSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentNextBoundaryCurrentGateCompletionEvidence, selectedBoundary)

	preconditionEntry, ok := daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntryForComponent("current_release_gate_completion_precondition")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentDecisionGateNotComplete, preconditionEntry.decision)

	flagEntry, ok := daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntryForComponent("dedicated_deprecated_runtime_api_flag_not_added")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentDecisionDoNotAddFlag, flagEntry.decision)

	_, ok = daemonDeprecatedRuntimeAPINextReleaseGateReassessmentEntryForComponent("unsupported")
	require.False(t, ok)
}

func TestDaemonDeprecatedRuntimeAPINextReleaseGateReassessmentSourceAuditKeepsCurrentBehavior(t *testing.T) {
	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")

	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	server := string(serverData)
	require.Contains(t, server, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)

	for _, forbidden := range []string{
		"CYBERAGENT_RUNTIME_COMPAT",
		"CYBERAGENT_DEPRECATED_RUNTIME_API",
	} {
		require.NotContains(t, agentRuntime, forbidden)
		require.NotContains(t, server, forbidden)
	}
}

func TestDaemonLocalRuntimeCompatibilityStrategyKeepsWholeEngineSwitchAndDefaultAttachments(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty defaults to engine", value: "", want: true},
		{name: "unknown defaults to engine", value: "engine", want: true},
		{name: "trimmed true defaults to engine", value: " TRUE ", want: true},
		{name: "legacy opts out", value: "legacy", want: false},
		{name: "off opts out", value: "off", want: false},
		{name: "disabled opts out", value: "disabled", want: false},
		{name: "disable opts out", value: "disable", want: false},
		{name: "false opts out", value: "false", want: false},
		{name: "zero opts out", value: "0", want: false},
		{name: "none opts out", value: "none", want: false},
		{name: "case-insensitive opt out", value: " LEGACY ", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, agentRuntimeEngineEnabledFromEnv(tc.value))
		})
	}

	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	server := string(serverData)
	require.Contains(t, server, `agentRuntimeEngineEnabledFromEnv(os.Getenv("CYBERAGENT_RUNTIME"))`)
	require.Contains(t, server, "runtimesqlite.Open(storagePaths.RuntimeDBPath)")
	require.Contains(t, server, "agentruntime.NewEngine(store)")
	require.Contains(t, server, "svc.agent.AttachEngine(engine)")
	require.Contains(t, server, "RecoverInterruptedRuns")
	require.Contains(t, server, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)

	storageResolverData, err := os.ReadFile("storage_resolver.go")
	require.NoError(t, err)
	storageResolver := string(storageResolverData)
	require.Contains(t, storageResolver, "LocalRuntimeDBPath:     agentRuntimeSQLitePath(cfg.RunsDir)")

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "func (m *agentManager) AttachEngine(e *runtime.Engine)")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachAgentLoopRunner(daemonAgentLoopRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachAgentLoopActionExecutor(daemonAgentLoopActionExecutor{manager: m})")
	require.NotContains(t, agentRuntime, "os.Getenv(")

	for _, forbidden := range []string{
		"CYBERAGENT_RUNTIME_COMPAT",
		"CYBERAGENT_DEPRECATED_RUNTIME_API",
	} {
		require.NotContains(t, server, forbidden)
		require.NotContains(t, agentRuntime, forbidden)
	}
}

func TestDaemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceAuditRecordsReadiness(t *testing.T) {
	wantStages := map[string]daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStage{
		"n225_next_release_gate_reassessment_boundary":   daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageN225BoundaryConsumed,
		"release_notes_and_api_contract_evidence":        daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageReleaseDocsPresent,
		"release_checklist_and_plan_evidence":            daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageReleaseChecklistPresent,
		"source_audit_evidence":                          daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageSourceAuditPresent,
		"regression_command_evidence":                    daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageRegressionEvidenceRecorded,
		"working_tree_release_state":                     daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageWorkingTreeNotTagReady,
		"default_runtime_behavior_preserved":             daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageDefaultBehaviorPreserved,
		"release_candidate_flag_reduction_next_boundary": daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStage{}

	for _, entry := range daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.artifact)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.decision)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceNextBoundaryReleaseCandidateStabilization, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate deprecated runtime API current-release-gate evidence entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage(daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageN225BoundaryConsumed), 1)
	require.Len(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage(daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageReleaseDocsPresent), 1)
	require.Len(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage(daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageReleaseChecklistPresent), 1)
	require.Len(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage(daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageSourceAuditPresent), 1)
	require.Len(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage(daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageRegressionEvidenceRecorded), 1)
	require.Len(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage(daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageWorkingTreeNotTagReady), 1)
	require.Len(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage(daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageDefaultBehaviorPreserved), 1)
	require.Len(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage(daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceStageNextBoundaryChosen), 1)
	require.Empty(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntriesForStage("unsupported"))

	n225Boundary, ok := daemonDeprecatedRuntimeAPINextReleaseGateReassessmentSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPINextReleaseGateReassessmentNextBoundaryCurrentGateCompletionEvidence, n225Boundary)

	selectedBoundary, ok := daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceNextBoundaryReleaseCandidateStabilization, selectedBoundary)

	regressionEntry, ok := daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntryForComponent("regression_command_evidence")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceDecisionRegressionPassed, regressionEntry.decision)

	workingTreeEntry, ok := daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntryForComponent("working_tree_release_state")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceDecisionReleaseNotTagReady, workingTreeEntry.decision)

	defaultEntry, ok := daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntryForComponent("default_runtime_behavior_preserved")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceDecisionPreserveDefaults, defaultEntry.decision)

	_, ok = daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceEntryForComponent("unsupported")
	require.False(t, ok)
}

func TestDaemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceSourceAuditKeepsReleaseEvidenceAndDefaults(t *testing.T) {
	requiredDocs := map[string][]string{
		"../../../docs/release-notes/v0.1.2.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.Engine.ExecuteTurn",
			"Deprecated",
			"current_release_gate",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/api/v0.1.2-api-contract.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.Engine.ExecuteTurn",
			"Deprecated",
			"current_release_gate",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/CyberAgent-Release-Checklist.md": {
			"current_release_gate",
			"runtime.Engine.RunAgentLoop",
			"runtime.Engine.ExecuteAgentLoopAction",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/CyberAgent-Release-Plan.md": {
			"current_release_gate",
			"runtime.Engine.RunAgentLoop",
			"runtime.Engine.ExecuteAgentLoopAction",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/CyberAgent-TODO.md": {
			"P0.5-N226",
			"current release gate completion evidence / release readiness audit",
			"go test ./...",
			"go vet ./...",
			"npm test --prefix web/frontend -- --run",
			"npm run build --prefix web/frontend",
			"go build ./cmd/cyberagent",
			"git diff --check",
			"P0.5-N227",
		},
	}

	for _, entry := range daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceTable {
		for _, artifact := range strings.Split(entry.artifact, ";") {
			if strings.HasPrefix(artifact, "docs/") {
				require.Contains(t, requiredDocs, "../../../"+artifact)
			}
		}
	}
	for path, terms := range requiredDocs {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		doc := string(data)
		for _, term := range terms {
			require.Contains(t, doc, term, "%s must record current release gate evidence for %q", path, term)
		}
	}

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachAgentLoopRunner(daemonAgentLoopRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachAgentLoopActionExecutor(daemonAgentLoopActionExecutor{manager: m})")

	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	server := string(serverData)
	require.Contains(t, server, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)

	for _, forbidden := range []string{
		"CYBERAGENT_RUNTIME_COMPAT",
		"CYBERAGENT_DEPRECATED_RUNTIME_API",
	} {
		require.NotContains(t, agentRuntime, forbidden)
		require.NotContains(t, server, forbidden)
	}
}

func TestDaemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionAuditDefersReduction(t *testing.T) {
	wantStages := map[string]daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStage{
		"n226_release_candidate_stabilization_boundary":        daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageN226BoundaryConsumed,
		"release_readiness_evidence_accepted":                  daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageReleaseEvidenceAccepted,
		"release_candidate_diff_inventory_recorded":            daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageDiffInventoryRecorded,
		"dirty_working_tree_blocks_flag_reduction":             daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageWorkingTreeBlocksReduction,
		"llm_turn_runner_default_attachment_preserved":         daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageDefaultAttachmentsPreserved,
		"turn_executor_default_attachment_preserved":           daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageDefaultAttachmentsPreserved,
		"dedicated_deprecated_runtime_api_flag_not_introduced": daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageDedicatedFlagNotIntroduced,
		"release_candidate_diff_archive_next_boundary":         daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageNextArchiveBoundaryChosen,
	}
	seenStages := map[string]daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStage{}

	for _, entry := range daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.artifact)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.decision)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionNextBoundaryDiffArchive, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate deprecated runtime API release-candidate flag/reduction entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageN226BoundaryConsumed), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageReleaseEvidenceAccepted), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageDiffInventoryRecorded), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageWorkingTreeBlocksReduction), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageDefaultAttachmentsPreserved), 2)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageDedicatedFlagNotIntroduced), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionStageNextArchiveBoundaryChosen), 1)
	require.Empty(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntriesForStage("unsupported"))

	n226Boundary, ok := daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceNextBoundaryReleaseCandidateStabilization, n226Boundary)

	selectedBoundary, ok := daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionNextBoundaryDiffArchive, selectedBoundary)

	workingTreeEntry, ok := daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntryForComponent("dirty_working_tree_blocks_flag_reduction")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionReductionBlocked, workingTreeEntry.decision)

	flagEntry, ok := daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntryForComponent("dedicated_deprecated_runtime_api_flag_not_introduced")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionDoNotIntroduceFlag, flagEntry.decision)

	_, ok = daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionEntryForComponent("unsupported")
	require.False(t, ok)
}

func TestDaemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionSourceAuditKeepsDefaults(t *testing.T) {
	requiredDocs := map[string][]string{
		"../../../docs/release-notes/v0.1.2.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.Engine.ExecuteTurn",
			"Deprecated",
			"current_release_gate",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/api/v0.1.2-api-contract.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.Engine.ExecuteTurn",
			"Deprecated",
			"current_release_gate",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/CyberAgent-Release-Checklist.md": {
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/CyberAgent-Release-Plan.md": {
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"retained_until_release_gate",
			"opt-in",
		},
		"../../../docs/CyberAgent-TODO.md": {
			"P0.5-N227",
			"release candidate stabilization and deprecated runtime API flag/reduction decision",
			"release_candidate_diff_archive_and_tag_readiness_audit",
			"git status --short",
			"git diff --stat",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"P0.5-N228",
		},
	}

	for _, entry := range daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionTable {
		for _, artifact := range strings.Split(entry.artifact, ";") {
			if strings.HasPrefix(artifact, "docs/") {
				require.Contains(t, requiredDocs, "../../../"+artifact)
			}
		}
	}
	for path, terms := range requiredDocs {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		doc := string(data)
		for _, term := range terms {
			require.Contains(t, doc, term, "%s must record N227 release-candidate flag/reduction evidence for %q", path, term)
		}
	}

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")

	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	server := string(serverData)
	require.Contains(t, server, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)

	for _, forbidden := range []string{
		"CYBERAGENT_RUNTIME_COMPAT",
		"CYBERAGENT_DEPRECATED_RUNTIME_API",
	} {
		require.NotContains(t, agentRuntime, forbidden)
		require.NotContains(t, server, forbidden)
	}
}

func TestDaemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessAuditRecordsInventory(t *testing.T) {
	wantStages := map[string]daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStage{
		"n227_diff_archive_boundary":                 daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageN227BoundaryConsumed,
		"release_candidate_diff_archive_document":    daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageArchiveDocumentCreated,
		"tracked_diff_inventory":                     daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageTrackedInventoryRecorded,
		"untracked_diff_inventory":                   daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageUntrackedInventoryRecorded,
		"tag_readiness_blocked_until_stage_commit":   daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageTagReadinessBlocked,
		"default_runtime_behavior_preserved":         daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageDefaultAttachmentsPreserved,
		"stage_commit_full_regression_next_boundary": daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageNextStageCommitGateChosen,
	}
	seenStages := map[string]daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStage{}

	for _, entry := range daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.artifact)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.decision)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessNextBoundaryStageCommit, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate deprecated runtime API release-candidate archive/tag-readiness entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageN227BoundaryConsumed), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageArchiveDocumentCreated), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageTrackedInventoryRecorded), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageUntrackedInventoryRecorded), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageTagReadinessBlocked), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageDefaultAttachmentsPreserved), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessStageNextStageCommitGateChosen), 1)
	require.Empty(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntriesForStage("unsupported"))

	n227Boundary, ok := daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionNextBoundaryDiffArchive, n227Boundary)

	selectedBoundary, ok := daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessNextBoundaryStageCommit, selectedBoundary)

	tagReadinessEntry, ok := daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntryForComponent("tag_readiness_blocked_until_stage_commit")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessDecisionTagReadinessBlocked, tagReadinessEntry.decision)

	defaultEntry, ok := daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntryForComponent("default_runtime_behavior_preserved")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessDecisionPreserveDefaults, defaultEntry.decision)

	_, ok = daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessEntryForComponent("unsupported")
	require.False(t, ok)
}

func TestDaemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessSourceAuditKeepsDefaults(t *testing.T) {
	requiredDocs := map[string][]string{
		"../../../docs/CyberAgent-v0.1.2-Release-Candidate-Diff-Inventory.md": {
			"P0.5-N228",
			"release_candidate_diff_archive_and_tag_readiness_audit",
			"not tag-ready",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"CYBERAGENT_RUNTIME_COMPAT",
			"CYBERAGENT_DEPRECATED_RUNTIME_API",
			"48 tracked modified paths",
			"32 untracked paths",
			"internal/interfaces/daemon/agent_runtime_compatibility.go",
			"release_candidate_stage_commit_and_full_regression_gate",
		},
		"../../../docs/CyberAgent-TODO.md": {
			"P0.5-N228",
			"release_candidate_diff_archive_and_tag_readiness_audit",
			"CyberAgent-v0.1.2-Release-Candidate-Diff-Inventory.md",
			"release_candidate_stage_commit_and_full_regression_gate",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"P0.5-N229",
		},
	}

	for _, entry := range daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessTable {
		for _, artifact := range strings.Split(entry.artifact, ";") {
			if strings.HasPrefix(artifact, "docs/") {
				require.Contains(t, requiredDocs, "../../../"+artifact)
			}
		}
	}
	for path, terms := range requiredDocs {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		doc := string(data)
		for _, term := range terms {
			require.Contains(t, doc, term, "%s must record N228 release-candidate archive/tag-readiness evidence for %q", path, term)
		}
	}

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")

	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	server := string(serverData)
	require.Contains(t, server, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)

	for _, forbidden := range []string{
		"CYBERAGENT_RUNTIME_COMPAT",
		"CYBERAGENT_DEPRECATED_RUNTIME_API",
	} {
		require.NotContains(t, agentRuntime, forbidden)
		require.NotContains(t, server, forbidden)
	}
}

func TestDaemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateAuditBlocksTagWithoutAuthorization(t *testing.T) {
	wantStages := map[string]daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStage{
		"n228_stage_commit_full_regression_boundary":          daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageN228BoundaryConsumed,
		"diff_archive_artifact_accepted":                      daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageArchiveArtifactAccepted,
		"working_tree_stage_commit_state":                     daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageWorkingTreeStillDirty,
		"stage_commit_tag_not_performed":                      daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageStageCommitNotPerformed,
		"full_release_regression_recorded":                    daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageFullRegressionRecorded,
		"default_runtime_behavior_preserved":                  daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageDefaultAttachmentsPreserved,
		"manual_stage_commit_tag_authorization_next_boundary": daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageManualAuthorizationBoundary,
	}
	seenStages := map[string]daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStage{}

	for _, entry := range daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.artifact)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.decision)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateNextBoundaryManualAuthorization, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate deprecated runtime API release-candidate stage/commit gate entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageN228BoundaryConsumed), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageArchiveArtifactAccepted), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageWorkingTreeStillDirty), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageStageCommitNotPerformed), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageFullRegressionRecorded), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageDefaultAttachmentsPreserved), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntriesForStage(daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateStageManualAuthorizationBoundary), 1)
	require.Empty(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntriesForStage("unsupported"))

	n228Boundary, ok := daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessNextBoundaryStageCommit, n228Boundary)

	selectedBoundary, ok := daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateNextBoundaryManualAuthorization, selectedBoundary)

	stageEntry, ok := daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntryForComponent("stage_commit_tag_not_performed")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateDecisionDoNotStageCommit, stageEntry.decision)

	regressionEntry, ok := daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntryForComponent("full_release_regression_recorded")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateDecisionRegressionRecorded, regressionEntry.decision)

	_, ok = daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateEntryForComponent("unsupported")
	require.False(t, ok)
}

func TestDaemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateSourceAuditKeepsManualBoundary(t *testing.T) {
	requiredDocs := map[string][]string{
		"../../../docs/CyberAgent-v0.1.2-Release-Candidate-Diff-Inventory.md": {
			"release_candidate_stage_commit_and_full_regression_gate",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"not tag-ready",
		},
		"../../../docs/CyberAgent-TODO.md": {
			"P0.5-N229",
			"release_candidate_stage_commit_and_full_regression_gate",
			"manual_stage_commit_and_tag_authorization",
			"git status --short",
			"git diff --stat",
			"go test ./...",
			"go vet ./...",
			"npm test --prefix web/frontend -- --run",
			"npm run build --prefix web/frontend",
			"go build ./cmd/cyberagent",
			"git diff --check",
			"P0.5-N230",
		},
	}

	for _, entry := range daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateTable {
		for _, artifact := range strings.Split(entry.artifact, ";") {
			if strings.HasPrefix(artifact, "docs/") {
				require.Contains(t, requiredDocs, "../../../"+artifact)
			}
		}
	}
	for path, terms := range requiredDocs {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		doc := string(data)
		for _, term := range terms {
			require.Contains(t, doc, term, "%s must record N229 stage/commit/full-regression gate evidence for %q", path, term)
		}
	}

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")

	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	server := string(serverData)
	require.Contains(t, server, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)

	for _, forbidden := range []string{
		"CYBERAGENT_RUNTIME_COMPAT",
		"CYBERAGENT_DEPRECATED_RUNTIME_API",
	} {
		require.NotContains(t, agentRuntime, forbidden)
		require.NotContains(t, server, forbidden)
	}
}

func TestDaemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationAuditWaitsForUser(t *testing.T) {
	wantStages := map[string]daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStage{
		"n229_manual_authorization_boundary":       daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageN229BoundaryConsumed,
		"explicit_user_authorization_required":     daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageExplicitAuthorizationNeeded,
		"git_stage_commit_tag_not_run":             daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageGitOpsNotRun,
		"runtime_defaults_preserved":               daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageRuntimeDefaultsPreserved,
		"deprecated_runtime_api_reduction_blocked": daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageCompatibilityReductionBlocked,
		"await_user_authorization_boundary":        daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageAwaitUserBoundarySelected,
	}
	seenStages := map[string]daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStage{}

	for _, entry := range daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.artifact)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.decision)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationNextBoundaryAwaitUser, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate deprecated runtime API manual authorization entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntriesForStage(daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageN229BoundaryConsumed), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntriesForStage(daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageExplicitAuthorizationNeeded), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntriesForStage(daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageGitOpsNotRun), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntriesForStage(daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageRuntimeDefaultsPreserved), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntriesForStage(daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageCompatibilityReductionBlocked), 1)
	require.Len(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntriesForStage(daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationStageAwaitUserBoundarySelected), 1)
	require.Empty(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntriesForStage("unsupported"))

	n229Boundary, ok := daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateNextBoundaryManualAuthorization, n229Boundary)

	selectedBoundary, ok := daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationNextBoundaryAwaitUser, selectedBoundary)

	gitOpsEntry, ok := daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntryForComponent("git_stage_commit_tag_not_run")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationDecisionDoNotRunGitOps, gitOpsEntry.decision)

	defaultEntry, ok := daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntryForComponent("runtime_defaults_preserved")
	require.True(t, ok)
	require.Equal(t, daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationDecisionPreserveDefaults, defaultEntry.decision)

	_, ok = daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationEntryForComponent("unsupported")
	require.False(t, ok)
}

func TestDaemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationSourceAuditKeepsRepoUntouched(t *testing.T) {
	requiredDocs := map[string][]string{
		"../../../docs/release-notes/v0.1.2.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.Engine.ExecuteTurn",
			"Deprecated",
			"retained_until_release_gate",
		},
		"../../../docs/api/v0.1.2-api-contract.md": {
			"runtime.Engine.RunLLMTurn",
			"runtime.Engine.ExecuteTurn",
			"Deprecated",
			"retained_until_release_gate",
		},
		"../../../docs/CyberAgent-TODO.md": {
			"P0.5-N230",
			"manual_stage_commit_and_tag_authorization",
			"await_explicit_user_stage_commit_tag_authorization",
			"git add",
			"git commit",
			"git tag",
			"AttachLLMTurnRunner",
			"AttachTurnExecutor",
			"不新增 dedicated deprecated runtime API opt-in flag",
		},
	}

	for _, entry := range daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationTable {
		for _, artifact := range strings.Split(entry.artifact, ";") {
			if strings.HasPrefix(artifact, "docs/") {
				require.Contains(t, requiredDocs, "../../../"+artifact)
			}
		}
	}
	for path, terms := range requiredDocs {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		doc := string(data)
		for _, term := range terms {
			require.Contains(t, doc, term, "%s must record N230 manual authorization gate evidence for %q", path, term)
		}
	}

	agentRuntimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	agentRuntime := string(agentRuntimeData)
	require.Contains(t, agentRuntime, "e.AttachLLMTurnRunner(daemonAgentLLMTurnRunner{manager: m})")
	require.Contains(t, agentRuntime, "e.AttachTurnExecutor(daemonAgentTurnExecutor{manager: m})")

	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	server := string(serverData)
	require.Contains(t, server, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)

	for _, forbidden := range []string{
		"CYBERAGENT_RUNTIME_COMPAT",
		"CYBERAGENT_DEPRECATED_RUNTIME_API",
	} {
		require.NotContains(t, agentRuntime, forbidden)
		require.NotContains(t, server, forbidden)
	}
}

func productionCallSitesNamed(t *testing.T, name string, patterns ...string) []string {
	t.Helper()
	var got []string
	fset := token.NewFileSet()
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		require.NoError(t, err)
		require.NotEmpty(t, files, "no files matched %q", pattern)
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(fset, file, nil, 0)
			require.NoError(t, err)
			ast.Inspect(parsed, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if productionCallName(call.Fun) == name {
					got = append(got, filepath.ToSlash(file)+":"+name)
				}
				return true
			})
		}
	}
	sort.Strings(got)
	return got
}

func productionCallName(expr ast.Expr) string {
	switch fn := expr.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}
