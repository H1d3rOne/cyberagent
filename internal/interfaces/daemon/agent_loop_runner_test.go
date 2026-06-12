package daemon

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestDaemonAgentLoopCallerFallbackOwnershipTableCoversReasons(t *testing.T) {
	wantReasons := map[daemonAgentLoopCallerFallbackReason]string{
		daemonAgentLoopCallerFallbackEngineNotAttached: daemonAgentLoopCallerFallbackOwnerLegacyCaller,
	}
	seenReasons := map[daemonAgentLoopCallerFallbackReason]string{}

	for _, entry := range daemonAgentLoopCallerFallbackOwnershipTable {
		require.NotEmpty(t, entry.reason)
		require.NotEmpty(t, entry.owner)
		require.Empty(t, seenReasons[entry.reason], "duplicate agent loop fallback ownership entry for %q", entry.reason)
		seenReasons[entry.reason] = entry.owner

		owner, ok := daemonAgentLoopCallerFallbackOwnerForReason(entry.reason)
		require.True(t, ok)
		require.Equal(t, entry.owner, owner)
		require.True(t, daemonAgentLoopCallerFallbackOwnedByLegacyCaller(entry.reason))
	}

	require.Equal(t, wantReasons, seenReasons)
	_, ok := daemonAgentLoopCallerFallbackOwnerForReason(daemonAgentLoopCallerFallbackReason("unsupported"))
	require.False(t, ok)
	require.False(t, daemonAgentLoopCallerFallbackOwnedByLegacyCaller(daemonAgentLoopCallerFallbackReason("unsupported")))
	require.False(t, daemonAgentLoopCallerFallbackOwnedByLegacyCaller(daemonAgentLoopCallerFallbackRunnerNotAttached))
	require.ErrorContains(t, daemonAgentLoopCallerFallbackOwnershipError(daemonAgentLoopCallerFallbackReason("unsupported")), "fallback ownership missing")

	reason, fallback := daemonAgentLoopCallerPreDispatchFallbackReason(nil)
	require.True(t, fallback)
	require.Equal(t, daemonAgentLoopCallerFallbackEngineNotAttached, reason)

	eng := runtime.NewEngine(runtime.NewMemStore())
	reason, fallback = daemonAgentLoopCallerPreDispatchFallbackReason(eng)
	require.True(t, fallback)
	require.Equal(t, daemonAgentLoopCallerFallbackRunnerNotAttached, reason)

	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(context.Context, runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		return runtime.AgentLoopResult{}, nil
	}))
	_, fallback = daemonAgentLoopCallerPreDispatchFallbackReason(eng)
	require.False(t, fallback)

	reason, fallback = daemonAgentLoopCallerResultFallbackReason(runtime.ErrAgentLoopRunnerNotAttached)
	require.True(t, fallback)
	require.Equal(t, daemonAgentLoopCallerFallbackRunnerNotAttached, reason)

	blocked, ok := daemonAgentLoopCallerRunnerNotAttachedBlockedResult(runtime.AgentLoopRequest{Mode: runtime.AgentLoopModeLLMToolLoop}, runtime.AgentLoopResult{}, runtime.ErrAgentLoopRunnerNotAttached)
	require.True(t, ok)
	require.True(t, blocked.CapabilityBlocked)
	require.Equal(t, runtime.ErrAgentLoopRunnerNotAttached.Error(), blocked.StopReason)
	require.Equal(t, string(runtime.AgentLoopModeLLMToolLoop), blocked.Metadata["mode"])
	require.Equal(t, string(daemonAgentLoopCallerFallbackRunnerNotAttached), blocked.Metadata["blocked_reason"])

	_, fallback = daemonAgentLoopCallerResultFallbackReason(errors.New("runner hard error"))
	require.False(t, fallback)
	_, ok = daemonAgentLoopCallerRunnerNotAttachedBlockedResult(runtime.AgentLoopRequest{}, runtime.AgentLoopResult{}, errors.New("runner hard error"))
	require.False(t, ok)
}

func TestDaemonAgentLoopCallerFallbackBoundaryCoversModes(t *testing.T) {
	want := map[runtime.AgentLoopMode]daemonAgentLoopCallerFallbackBoundaryEntry{
		runtime.AgentLoopModeDeterministicRoute: {
			mode:           runtime.AgentLoopModeDeterministicRoute,
			callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
			legacyFallback: "runAgentDeterministicRouteAgentLoopResult",
			owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
			scope:          "deterministic_route compatibility fallback only when runtime engine is not attached",
		},
		runtime.AgentLoopModeLLMToolLoop: {
			mode:           runtime.AgentLoopModeLLMToolLoop,
			callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
			legacyFallback: "runAgentLLMToolLoopAgentLoopResult",
			owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
			scope:          "llm_tool_loop compatibility fallback only when runtime engine is not attached",
		},
		runtime.AgentLoopModeReplanContinuation: {
			mode:           runtime.AgentLoopModeReplanContinuation,
			callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
			legacyFallback: "runAgentReplanActionContinuationAgentLoopResult",
			owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
			scope:          "replan_action_continuation compatibility fallback only when runtime engine is not attached",
		},
		runtime.AgentLoopModePlannerActionReplanCycle: {
			mode:           runtime.AgentLoopModePlannerActionReplanCycle,
			callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
			legacyFallback: "runAgentPlannerActionReplanCycleAgentLoop",
			owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
			scope:          "planner_action_replan_cycle compatibility fallback only when runtime engine is not attached",
		},
		runtime.AgentLoopModePlannerActionExecution: {
			mode:           runtime.AgentLoopModePlannerActionExecution,
			callerSurface:  "runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult",
			legacyFallback: "runAgentPlannerActionExecutionAgentLoopResult",
			owner:          daemonAgentLoopCallerFallbackOwnerLegacyCaller,
			scope:          "planner_action_execution compatibility fallback only when runtime engine is not attached",
		},
	}
	seen := map[runtime.AgentLoopMode]daemonAgentLoopCallerFallbackBoundaryEntry{}

	for _, entry := range daemonAgentLoopCallerFallbackBoundaryTable {
		require.NotEmpty(t, entry.mode)
		require.NotEmpty(t, entry.callerSurface)
		require.NotEmpty(t, entry.legacyFallback)
		require.Equal(t, daemonAgentLoopCallerFallbackOwnerLegacyCaller, entry.owner)
		require.NotEmpty(t, entry.scope)
		require.Empty(t, seen[entry.mode].mode, "duplicate agent loop fallback boundary entry for %q", entry.mode)
		seen[entry.mode] = entry

		got, ok := daemonAgentLoopCallerFallbackBoundaryForMode(entry.mode)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, want, seen)
	cycle, ok := daemonAgentLoopCallerFallbackBoundaryForMode(runtime.AgentLoopModePlannerActionReplanCycle)
	require.True(t, ok)
	require.Equal(t, "runAgentPlannerActionReplanCycleAgentLoop", cycle.legacyFallback)
	require.Contains(t, cycle.scope, "planner_action_replan_cycle")

	_, ok = daemonAgentLoopCallerFallbackBoundaryForMode(runtime.AgentLoopMode("unsupported"))
	require.False(t, ok)
}

func TestDaemonAgentLoopRunnerNotAttachedRetirementAuditNarrowsFallback(t *testing.T) {
	wantStages := map[string]daemonAgentLoopRunnerNotAttachedRetirementStage{
		"no_engine_agent_loop_direct_fallback":      daemonAgentLoopRunnerNotAttachedStageNoEngineFallbackRetained,
		"attached_engine_missing_agent_loop_runner": daemonAgentLoopRunnerNotAttachedStageMissingRunnerBlocked,
		"detached_agent_loop_runner_result":         daemonAgentLoopRunnerNotAttachedStageDetachedResultBlocked,
		"agent_loop_next_fallback_boundary":         daemonAgentLoopRunnerNotAttachedStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonAgentLoopRunnerNotAttachedRetirementStage{}
	for _, entry := range daemonAgentLoopRunnerNotAttachedRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonAgentLoopNextFallbackRetirementBoundaryLLMStageRunnerNotAttached, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate agent loop runner-not-attached retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonAgentLoopRunnerNotAttachedRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}
	require.Equal(t, wantStages, seenStages)
	require.Empty(t, daemonAgentLoopRunnerNotAttachedRetirementEntriesForStage(daemonAgentLoopRunnerNotAttachedRetirementStage("unsupported")))
	require.Len(t, daemonAgentLoopRunnerNotAttachedRetirementEntriesForStage(daemonAgentLoopRunnerNotAttachedStageNoEngineFallbackRetained), 1)
	require.Len(t, daemonAgentLoopRunnerNotAttachedRetirementEntriesForStage(daemonAgentLoopRunnerNotAttachedStageMissingRunnerBlocked), 1)
	require.Len(t, daemonAgentLoopRunnerNotAttachedRetirementEntriesForStage(daemonAgentLoopRunnerNotAttachedStageDetachedResultBlocked), 1)
	require.Len(t, daemonAgentLoopRunnerNotAttachedRetirementEntriesForStage(daemonAgentLoopRunnerNotAttachedStageNextBoundaryChosen), 1)

	selectedBoundary, ok := daemonAgentLoopRunnerNotAttachedRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopNextFallbackRetirementBoundaryLLMStageRunnerNotAttached, selectedBoundary)
	require.True(t, daemonAgentLoopRunnerNotAttachedRetirementAllowsDirectFallback(daemonAgentLoopCallerFallbackEngineNotAttached))
	require.False(t, daemonAgentLoopRunnerNotAttachedRetirementAllowsDirectFallback(daemonAgentLoopCallerFallbackRunnerNotAttached))

	missing, ok := daemonAgentLoopRunnerNotAttachedRetirementEntryForComponent("attached_engine_missing_agent_loop_runner")
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopRunnerNotAttachedDispositionBlockedError, missing.disposition)
	require.Contains(t, missing.evidence, "runtime.ErrAgentLoopRunnerNotAttached")

	detached, ok := daemonAgentLoopRunnerNotAttachedRetirementEntryForComponent("detached_agent_loop_runner_result")
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopRunnerNotAttachedDispositionBlockedError, detached.disposition)
	require.Contains(t, detached.evidence, "Engine.RunAgentLoop")

	next, ok := daemonAgentLoopRunnerNotAttachedRetirementEntryForComponent("agent_loop_next_fallback_boundary")
	require.True(t, ok)
	require.Contains(t, next.nextStep, "P0.5-N217")
}

func TestDaemonAgentLoopNoEngineDirectFallbackRetirementAuditScopesFallback(t *testing.T) {
	wantStages := map[string]daemonAgentLoopNoEngineDirectFallbackRetirementStage{
		"production_new_server_runtime_engine":               daemonAgentLoopNoEngineDirectFallbackStageProductionDefaultAttached,
		"legacy_runtime_opt_out":                             daemonAgentLoopNoEngineDirectFallbackStageLegacyOptOutRetained,
		"planner_action_execution_no_engine_direct_fallback": daemonAgentLoopNoEngineDirectFallbackStagePlannerActionScoped,
		"agent_loop_next_fallback_boundary":                  daemonAgentLoopNoEngineDirectFallbackStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonAgentLoopNoEngineDirectFallbackRetirementStage{}
	for _, entry := range daemonAgentLoopNoEngineDirectFallbackRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonAgentLoopNextFallbackRetirementBoundaryRunnerNotAttached, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate no-engine direct fallback retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonAgentLoopNoEngineDirectFallbackRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}
	require.Equal(t, wantStages, seenStages)
	require.Empty(t, daemonAgentLoopNoEngineDirectFallbackRetirementEntriesForStage(daemonAgentLoopNoEngineDirectFallbackRetirementStage("unsupported")))
	require.Len(t, daemonAgentLoopNoEngineDirectFallbackRetirementEntriesForStage(daemonAgentLoopNoEngineDirectFallbackStageProductionDefaultAttached), 1)
	require.Len(t, daemonAgentLoopNoEngineDirectFallbackRetirementEntriesForStage(daemonAgentLoopNoEngineDirectFallbackStageLegacyOptOutRetained), 1)
	require.Len(t, daemonAgentLoopNoEngineDirectFallbackRetirementEntriesForStage(daemonAgentLoopNoEngineDirectFallbackStagePlannerActionScoped), 1)
	require.Len(t, daemonAgentLoopNoEngineDirectFallbackRetirementEntriesForStage(daemonAgentLoopNoEngineDirectFallbackStageNextBoundaryChosen), 1)

	selectedBoundary, ok := daemonAgentLoopNoEngineDirectFallbackRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopNextFallbackRetirementBoundaryRunnerNotAttached, selectedBoundary)

	require.True(t, agentRuntimeEngineEnabledFromEnv(""))
	require.True(t, agentRuntimeEngineEnabledFromEnv("engine"))
	require.False(t, agentRuntimeEngineEnabledFromEnv("legacy"))
	require.False(t, agentRuntimeEngineEnabledFromEnv("off"))
	require.True(t, daemonAgentLoopNoEngineDirectFallbackRetirementAllowsDirectFallback(daemonAgentLoopCallerFallbackEngineNotAttached))
	require.False(t, daemonAgentLoopNoEngineDirectFallbackRetirementAllowsDirectFallback(daemonAgentLoopCallerFallbackRunnerNotAttached))

	production, ok := daemonAgentLoopNoEngineDirectFallbackRetirementEntryForComponent("production_new_server_runtime_engine")
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopNoEngineDirectFallbackDispositionEngineAttached, production.disposition)
	require.Contains(t, production.evidence, "AttachEngine")

	legacy, ok := daemonAgentLoopNoEngineDirectFallbackRetirementEntryForComponent("legacy_runtime_opt_out")
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopNoEngineDirectFallbackDispositionDirectLegacyFallback, legacy.disposition)
	require.Contains(t, legacy.evidence, "CYBERAGENT_RUNTIME=legacy")

	plannerAction, ok := daemonAgentLoopNoEngineDirectFallbackRetirementEntryForComponent("planner_action_execution_no_engine_direct_fallback")
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopCallerFallbackEngineNotAttached, plannerAction.reason)
	require.Contains(t, plannerAction.evidence, "daemonAgentLoopActionExecutor")
	require.Contains(t, plannerAction.evidence, "N214")

	next, ok := daemonAgentLoopNoEngineDirectFallbackRetirementEntryForComponent("agent_loop_next_fallback_boundary")
	require.True(t, ok)
	require.Contains(t, next.nextStep, "P0.5-N216")
}

func TestAgentManagerRunRuntimeAgentLoopResultDirectFallbackBoundary(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*agentManager)
	}{
		{
			name: "missing_engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newAgentManager(t.TempDir())
			if tt.setup != nil {
				tt.setup(mgr)
			}

			result, err := mgr.runRuntimeAgentLoopResult(context.Background(), runtime.AgentLoopRequest{
				Mode:     runtime.AgentLoopModeReplanContinuation,
				Turn:     runtime.TurnContext{SessionID: "session-loop-fallback", TaskID: "task-loop-fallback", RunID: "run-loop-fallback", TurnID: "turn-loop-fallback", Prompt: "continue safely", OperationLevel: "read_only"},
				Language: "en",
				Operator: "operator-1",
				MaxLoops: 0,
			})

			require.NoError(t, err)
			require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, result.StopReason)
			require.Equal(t, 0, result.LoopCount)
			require.Equal(t, string(runtime.AgentLoopModeReplanContinuation), result.Metadata["mode"])
			require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, result.Metadata[runtime.AgentLoopEgressMetadataStopReason])
		})
	}
}

func TestAgentManagerRunRuntimeAgentLoopResultRunnerNotAttachedBlocksWithoutDirectFallback(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*runtime.Engine)
	}{
		{
			name: "missing_runner",
			setup: func(eng *runtime.Engine) {
				eng.AttachAgentLoopRunner(nil)
			},
		},
		{
			name: "runner_not_attached_error",
			setup: func(eng *runtime.Engine) {
				eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(context.Context, runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
					return runtime.AgentLoopResult{StopReason: "runner_not_attached_result"}, runtime.ErrAgentLoopRunnerNotAttached
				}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := runtime.NewEngine(runtime.NewMemStore())
			mgr := newAgentManager(t.TempDir())
			mgr.AttachEngine(eng)
			tt.setup(eng)

			result, err := mgr.runRuntimeAgentLoopResult(context.Background(), runtime.AgentLoopRequest{
				Mode:     runtime.AgentLoopModeReplanContinuation,
				Turn:     runtime.TurnContext{SessionID: "session-loop-runner-blocked", TaskID: "task-loop-runner-blocked", RunID: "run-loop-runner-blocked", TurnID: "turn-loop-runner-blocked", Prompt: "continue safely", OperationLevel: "read_only"},
				Language: "en",
				Operator: "operator-1",
				MaxLoops: 0,
			})

			require.ErrorIs(t, err, runtime.ErrAgentLoopRunnerNotAttached)
			require.True(t, result.CapabilityBlocked)
			require.Equal(t, string(runtime.AgentLoopModeReplanContinuation), result.Metadata["mode"])
			require.Equal(t, runtime.ErrAgentLoopRunnerNotAttached.Error(), result.Metadata["error"])
			require.Equal(t, string(daemonAgentLoopCallerFallbackRunnerNotAttached), result.Metadata["blocked_reason"])
			require.NotEqual(t, runtime.AgentLoopStopReasonMaxLoops, result.StopReason)
		})
	}
}

func TestAgentManagerRunRuntimePlannerActionReplanCycleUsesAgentLoopFallbackPolicy(t *testing.T) {
	t.Run("attached_engine_missing_runner_blocks", func(t *testing.T) {
		mgr := newAgentManager(t.TempDir())
		eng := runtime.NewEngine(runtime.NewMemStore())
		mgr.AttachEngine(eng)
		eng.AttachAgentLoopRunner(nil)

		result := mgr.runRuntimeAgentPlannerActionReplanCycleAgentLoop(context.Background(), runtime.AgentLoopRequest{
			Mode: runtime.AgentLoopModeLLMToolLoop,
			Turn: runtime.TurnContext{
				SessionID:      "session-nested-cycle-missing-runner",
				TaskID:         "task-nested-cycle-missing-runner",
				RunID:          "run-nested-cycle-missing-runner",
				TurnID:         "turn-nested-cycle-missing-runner",
				Prompt:         "continue one nested cycle",
				OperationLevel: "read_only",
			},
			InitialPlan: &runtime.AgentLoopPlan{
				Phase: "step_replan",
				Actions: []runtime.AgentLoopAction{{
					Name: "daemon.health",
				}},
			},
			MaxLoops: 1,
		})

		require.True(t, result.CapabilityBlocked)
		require.Equal(t, runtime.ErrAgentLoopRunnerNotAttached.Error(), result.StopReason)
		require.Equal(t, runtime.ErrAgentLoopRunnerNotAttached.Error(), result.Metadata["error"])
		require.Equal(t, string(runtime.AgentLoopModePlannerActionReplanCycle), result.Metadata["mode"])
		require.Equal(t, string(daemonAgentLoopCallerFallbackRunnerNotAttached), result.Metadata["blocked_reason"])
		require.Equal(t, true, result.Metadata["nested_dispatch_error"])
	})

	t.Run("runner_hard_error_does_not_direct_fallback", func(t *testing.T) {
		mgr := newAgentManager(t.TempDir())
		eng := runtime.NewEngine(runtime.NewMemStore())
		mgr.AttachEngine(eng)
		wantErr := errors.New("nested cycle runner hard error")
		eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
			require.Equal(t, runtime.AgentLoopModePlannerActionReplanCycle, req.Mode)
			return runtime.AgentLoopResult{
				StopReason: "runner stopped before fallback",
				Metadata: map[string]any{
					"runner": "custom",
				},
			}, wantErr
		}))

		result := mgr.runRuntimeAgentPlannerActionReplanCycleAgentLoop(context.Background(), runtime.AgentLoopRequest{
			Mode: runtime.AgentLoopModeReplanContinuation,
			Turn: runtime.TurnContext{
				SessionID:      "session-nested-cycle-hard-error",
				TaskID:         "task-nested-cycle-hard-error",
				RunID:          "run-nested-cycle-hard-error",
				TurnID:         "turn-nested-cycle-hard-error",
				Prompt:         "continue one nested cycle",
				OperationLevel: "read_only",
			},
			InitialPlan: &runtime.AgentLoopPlan{
				Phase: "step_replan",
				Actions: []runtime.AgentLoopAction{{
					Name: "daemon.health",
				}},
			},
			MaxLoops: 1,
		})

		require.Equal(t, "runner stopped before fallback", result.StopReason)
		require.Equal(t, "custom", result.Metadata["runner"])
		require.Equal(t, wantErr.Error(), result.Metadata["error"])
		require.Equal(t, string(runtime.AgentLoopModePlannerActionReplanCycle), result.Metadata["mode"])
		require.Equal(t, true, result.Metadata["nested_dispatch_error"])
		require.False(t, result.CapabilityBlocked)
	})
}

func TestAgentManagerRunRuntimePlannerActionReplanCycleSourceAuditUsesAgentLoopFallbackPolicy(t *testing.T) {
	plannerData, err := os.ReadFile("agent_planner_loop.go")
	require.NoError(t, err)
	plannerSource := string(plannerData)

	start := strings.Index(plannerSource, "func (m *agentManager) runRuntimeAgentPlannerActionReplanCycleAgentLoop")
	require.NotEqual(t, -1, start)
	end := strings.Index(plannerSource[start:], "\nfunc (m *agentManager) runAgentPlannerActionReplanCycle")
	require.NotEqual(t, -1, end)
	runtimeDispatch := plannerSource[start : start+end]
	require.Contains(t, runtimeDispatch, "m.runRuntimeAgentLoopResult(ctx, req)")
	require.Contains(t, runtimeDispatch, "daemonAgentLoopNestedDispatchErrorResult(req, loopResult, err)")
	require.Contains(t, runtimeDispatch, "normalizeAgentPlannerActionReplanCycleAgentLoopResult(loopResult)")
	require.NotContains(t, runtimeDispatch, "m.runAgentPlannerActionReplanCycleAgentLoop")
	require.NotContains(t, runtimeDispatch, "m.runDaemonAgentLoopDirectResult")
	require.NotContains(t, runtimeDispatch, "m.runtimeEngine()")

	require.Equal(t, 2, strings.Count(plannerSource, "m.runRuntimeAgentPlannerActionReplanCycleAgentLoop(ctx, cycleReq)"))
	require.Equal(t, 1, strings.Count(plannerSource, "m.runAgentPlannerActionReplanCycleAgentLoop(ctx,"))

	loopRunnerData, err := os.ReadFile("agent_loop_runner.go")
	require.NoError(t, err)
	loopRunnerSource := string(loopRunnerData)
	require.Contains(t, loopRunnerSource, "return r.manager.runAgentPlannerActionReplanCycleAgentLoop(ctx, req), nil")
	require.Contains(t, loopRunnerSource, "return m.runAgentPlannerActionReplanCycleAgentLoop(ctx, req), nil")
	require.Contains(t, loopRunnerSource, "case runtime.AgentLoopModePlannerActionReplanCycle:")
	require.Contains(t, loopRunnerSource, "func (m *agentManager) runRuntimeAgentLoopResult")
}

func TestAgentManagerRunRuntimeAgentLoopResultUnsupportedModeStaysDirectFallbackError(t *testing.T) {
	mgr := newAgentManager(t.TempDir())

	result, err := mgr.runRuntimeAgentLoopResult(context.Background(), runtime.AgentLoopRequest{
		Mode: runtime.AgentLoopMode("unsupported"),
	})

	require.ErrorContains(t, err, "unsupported agent loop mode")
	require.True(t, result.CapabilityBlocked)
	require.Contains(t, result.StopReason, "unsupported agent loop mode")
}

func TestAgentManagerRunRuntimeAgentLoopResultDoesNotFallbackForRunnerHardError(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	wantErr := errors.New("runner hard error")
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(context.Context, runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		return runtime.AgentLoopResult{StopReason: "runner_failed"}, wantErr
	}))

	result, err := mgr.runRuntimeAgentLoopResult(context.Background(), runtime.AgentLoopRequest{
		Mode:     runtime.AgentLoopModeReplanContinuation,
		MaxLoops: 0,
	})

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "runner_failed", result.StopReason)
}
