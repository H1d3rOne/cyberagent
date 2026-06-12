package daemon

import (
	"context"
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/stretchr/testify/require"
)

func attachPlannerActionExecutionActionExecutorFromTurnHandler(t *testing.T, eng *runtime.Engine, handler runtime.TurnExecutorFunc) {
	t.Helper()
	require.NotNil(t, eng)
	require.NotNil(t, handler)
	eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(ctx context.Context, req runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
		if ctx == nil {
			ctx = context.Background()
		}
		action := req.Action.Clone()
		turn := runtime.AgentLoopActionExecutionTurn(req.Request, daemonTurnExecutionPlannerActionExecution, action, normalizeAgentPlannerActionName(action.Name))
		result, err := handler(ctx, turn)
		payload, ok := agentLoopTurnResultPayloadFromTurnResult(result.Payload)
		actionResult := runtime.AgentLoopActionExecutorResult{
			Status: result.Status,
			Reason: result.Reason,
			Error:  result.Error,
		}
		if ok {
			actionResult.Messages = payload.Messages
			actionResult.ToolCalls = payload.ToolCalls
			actionResult.Metadata = payload.Metadata
		}
		if err != nil && actionResult.Error == nil {
			actionResult.Error = err
		}
		return actionResult, err
	}))
}

func TestAgentManagerAttachEngineWiresDaemonTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	require.NotNil(t, eng.LLMTurnRunner())
	require.NotNil(t, eng.LLMStageRunner())
	require.NotNil(t, eng.AgentLoopRunner())
	require.NotNil(t, eng.AgentLoopActionExecutor())
	require.NotNil(t, eng.SessionMutator())

	result, err := eng.ExecuteTurn(context.Background(), runtime.TurnContext{
		SessionID:      "session-legacy-executor",
		TaskID:         "task-legacy-executor",
		RunID:          "run-legacy-executor",
		TurnID:         "turn-legacy-executor",
		Prompt:         "keep daemon behavior",
		Origin:         "user",
		OperationLevel: "read_only",
	})

	require.NoError(t, err)
	require.Equal(t, runtime.TurnResultRunning, result.Status)
	require.Equal(t, "legacy daemon SendMessage owns turn execution", result.Reason)
	require.Equal(t, "daemon_legacy_adapter", result.Data["executor"])
	require.Equal(t, true, result.Data["manager_attached"])
	require.Equal(t, "session-legacy-executor", result.Data["session_id"])
	require.Equal(t, "turn-legacy-executor", result.Data["turn_id"])
}

func TestDaemonTurnExecutionCallerInvalidPayloadTableCoversKinds(t *testing.T) {
	wantMessages := map[string]string{
		daemonTurnExecutionDeterministicRoute:     "daemon: deterministic route turn executor returned invalid payload",
		daemonTurnExecutionPlannerActionExecution: "daemon: planner action execution turn executor returned invalid payload",
	}
	seenMessages := map[string]string{}

	for _, entry := range daemonTurnExecutionCallerInvalidPayloadTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.message)
		require.Empty(t, seenMessages[entry.kind], "duplicate invalid payload entry for %q", entry.kind)
		seenMessages[entry.kind] = entry.message
		require.Equal(t, entry.message, daemonTurnExecutionCallerInvalidPayloadError(entry.kind))

		payload, handled, err := agentTurnExecutionResultPayloadForKind(runtime.TurnResult{
			Status:  runtime.TurnResultFinished,
			Payload: "not a payload",
		}, nil, entry.kind)
		require.ErrorContains(t, err, entry.message)
		require.True(t, handled)
		require.Empty(t, payload.Messages)
		require.Empty(t, payload.ToolCalls)
	}

	require.Equal(t, wantMessages, seenMessages)
	require.Equal(t, daemonTurnExecutionCallerDefaultInvalidPayloadError, daemonTurnExecutionCallerInvalidPayloadError("unsupported"))

	payload, handled, err := agentTurnExecutionResultPayloadForKind(runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Payload: "not a payload",
	}, nil, "unsupported")
	require.ErrorContains(t, err, daemonTurnExecutionCallerDefaultInvalidPayloadError)
	require.True(t, handled)
	require.Empty(t, payload.Messages)
	require.Empty(t, payload.ToolCalls)
}

func TestDaemonTurnExecutionCallerPolicyTableCoversKinds(t *testing.T) {
	wantPolicies := map[string]daemonTurnExecutionCallerPolicy{
		daemonTurnExecutionDeterministicRoute:     daemonTurnExecutionCallerPolicyStrictResult,
		daemonTurnExecutionPlannerActionExecution: daemonTurnExecutionCallerPolicyStrictResult,
	}
	seenPolicies := map[string]daemonTurnExecutionCallerPolicy{}

	for _, entry := range daemonTurnExecutionCallerPolicyTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.policy)
		require.Empty(t, seenPolicies[entry.kind], "duplicate caller policy entry for %q", entry.kind)
		seenPolicies[entry.kind] = entry.policy

		policy, ok := daemonTurnExecutionCallerPolicyForKind(entry.kind)
		require.True(t, ok)
		require.Equal(t, entry.policy, policy)
		require.NotEqual(t, daemonTurnExecutionCallerDefaultInvalidPayloadError, daemonTurnExecutionCallerInvalidPayloadError(entry.kind))
	}

	require.Equal(t, wantPolicies, seenPolicies)
	_, ok := daemonTurnExecutionCallerPolicyForKind("unsupported")
	require.False(t, ok)
	require.True(t, daemonTurnExecutionCallerPolicyMatches("unsupported", daemonTurnExecutionCallerPolicyStrictResult))

	expected := runtime.AgentLoopTurnResultPayload{
		Messages:  []runtime.AgentLoopMessage{{ID: "msg-turn-policy"}},
		ToolCalls: []runtime.AgentLoopToolCall{{ID: "tool-turn-policy"}},
	}
	payload, handled, err := agentTurnExecutionResultPayloadForKind(runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Payload: expected,
	}, nil, daemonTurnExecutionDeterministicRoute)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, expected, payload)
}

func TestDaemonTurnExecutionCallerFallbackOwnershipTableCoversReasons(t *testing.T) {
	wantReasons := map[daemonTurnExecutionCallerFallbackReason]string{
		daemonTurnExecutionCallerFallbackExecutorNotAttached:   daemonTurnExecutionCallerFallbackOwnerLegacyCaller,
		daemonTurnExecutionCallerFallbackBlocked:               daemonTurnExecutionCallerFallbackOwnerLegacyCaller,
		daemonTurnExecutionCallerFallbackRunningWithoutPayload: daemonTurnExecutionCallerFallbackOwnerLegacyCaller,
	}
	seenReasons := map[daemonTurnExecutionCallerFallbackReason]string{}

	for _, entry := range daemonTurnExecutionCallerFallbackOwnershipTable {
		require.NotEmpty(t, entry.reason)
		require.NotEmpty(t, entry.owner)
		require.Empty(t, seenReasons[entry.reason], "duplicate fallback ownership entry for %q", entry.reason)
		seenReasons[entry.reason] = entry.owner

		owner, ok := daemonTurnExecutionCallerFallbackOwnerForReason(entry.reason)
		require.True(t, ok)
		require.Equal(t, entry.owner, owner)
		require.True(t, daemonTurnExecutionCallerFallbackOwnedByLegacyCaller(entry.reason))
	}

	require.Equal(t, wantReasons, seenReasons)
	_, ok := daemonTurnExecutionCallerFallbackOwnerForReason(daemonTurnExecutionCallerFallbackReason("unsupported"))
	require.False(t, ok)
	require.False(t, daemonTurnExecutionCallerFallbackOwnedByLegacyCaller(daemonTurnExecutionCallerFallbackReason("unsupported")))
	require.ErrorContains(t, daemonTurnExecutionCallerFallbackOwnershipError(daemonTurnExecutionCallerFallbackReason("unsupported")), "fallback ownership missing")

	reason, fallback := daemonTurnExecutionCallerPreDispatchFallbackReason(nil)
	require.True(t, fallback)
	require.Equal(t, daemonTurnExecutionCallerFallbackExecutorNotAttached, reason)

	eng := runtime.NewEngine(runtime.NewMemStore())
	reason, fallback = daemonTurnExecutionCallerPreDispatchFallbackReason(eng)
	require.True(t, fallback)
	require.Equal(t, daemonTurnExecutionCallerFallbackExecutorNotAttached, reason)

	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(context.Context, runtime.TurnContext) (runtime.TurnResult, error) {
		return runtime.TurnResult{}, nil
	}))
	_, fallback = daemonTurnExecutionCallerPreDispatchFallbackReason(eng)
	require.False(t, fallback)
}

func TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers(t *testing.T) {
	wantMigrations := map[string]daemonTurnExecutionCallerMigrationEntry{
		daemonTurnExecutionDeterministicRoute: {
			kind:          daemonTurnExecutionDeterministicRoute,
			stage:         daemonTurnExecutionCallerMigrationStageAgentLoopInternal,
			currentSeam:   "runtime.Engine.RunAgentLoop",
			target:        "runtime-native deterministic route",
			migrationGate: "deterministic route registry dispatch now uses runtime AgentLoopToolServiceBinding; retire TurnExecutor compatibility after response egress and legacy route adapter cleanup",
		},
		daemonTurnExecutionPlannerActionExecution: {
			kind:          daemonTurnExecutionPlannerActionExecution,
			stage:         daemonTurnExecutionCallerMigrationStageAgentLoopInternal,
			currentSeam:   "runtime.Engine.RunAgentLoop",
			target:        "runtime-native action execution",
			migrationGate: "move executeAgentPlannerAction side effects and result egress runtime-side before retiring planner action TurnExecutor compatibility",
		},
	}
	seenMigrations := map[string]daemonTurnExecutionCallerMigrationEntry{}

	for _, entry := range daemonTurnExecutionCallerMigrationTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.currentSeam)
		require.NotEmpty(t, entry.target)
		require.NotEmpty(t, entry.migrationGate)
		require.Empty(t, seenMigrations[entry.kind].kind, "duplicate migration entry for %q", entry.kind)
		seenMigrations[entry.kind] = entry

		got, ok := daemonTurnExecutionCallerMigrationForKind(entry.kind)
		require.True(t, ok)
		require.Equal(t, entry, got)

		_, hasPolicy := daemonTurnExecutionCallerPolicyForKind(entry.kind)
		require.True(t, hasPolicy)
	}

	require.Equal(t, wantMigrations, seenMigrations)
	require.Empty(t, seenMigrations[daemonTurnExecutionReplanActionContinuation].kind)
	_, ok := daemonTurnExecutionCallerMigrationForKind("unsupported")
	require.False(t, ok)
	require.False(t, daemonTurnExecutionCallerMigrationActiveTurnExecutor("unsupported"))
	require.False(t, daemonTurnExecutionCallerMigrationActiveTurnExecutor(daemonTurnExecutionDeterministicRoute))
	require.False(t, daemonTurnExecutionCallerMigrationActiveTurnExecutor(daemonTurnExecutionPlannerActionExecution))
	require.False(t, daemonTurnExecutionCallerMigrationActiveTurnExecutor(daemonTurnExecutionReplanActionContinuation))
}

func TestDaemonTurnExecutionHelperOwnershipTableDocumentsRuntimeOwnedHelpers(t *testing.T) {
	wantOwnership := map[string]daemonTurnExecutionHelperOwnershipEntry{
		daemonTurnExecutionDeterministicRoute: {
			kind:                 daemonTurnExecutionDeterministicRoute,
			callerSurface:        "internal_agent_loop_caller",
			requestHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
			turnHelperOwner:      daemonTurnExecutionHelperOwnerRuntime,
			payloadHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
			remainingDaemonScope: "turn_ref_adapter_and_legacy_route_agent_loop_adapter",
			nextMigration:        "keep deterministic route registry dispatch on runtime tool services while retiring response egress and external TurnExecutor compatibility",
		},
		daemonTurnExecutionPlannerActionExecution: {
			kind:                 daemonTurnExecutionPlannerActionExecution,
			callerSurface:        "internal_agent_loop_caller",
			requestHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
			turnHelperOwner:      daemonTurnExecutionHelperOwnerRuntime,
			payloadHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
			remainingDaemonScope: "turn_ref_adapter_legacy_action_side_effects_and_turn_executor_compatibility",
			nextMigration:        "move executeAgentPlannerAction side effects and result egress behind runtime-native action execution; keep TurnExecutor branch as compatibility until seam retirement",
		},
		daemonTurnExecutionReplanActionContinuation: {
			kind:                 daemonTurnExecutionReplanActionContinuation,
			callerSurface:        "external_compatibility_trampoline",
			requestHelperOwner:   daemonTurnExecutionHelperOwnerDaemonCompatibilityLayer,
			turnHelperOwner:      daemonTurnExecutionHelperOwnerRuntime,
			payloadHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
			remainingDaemonScope: "legacy AgentLoop request adapter and external TurnExecutor compatibility",
			nextMigration:        "not an internal ExecuteTurn migration candidate; remove only after compatibility seam is retired",
		},
	}
	seenOwnership := map[string]daemonTurnExecutionHelperOwnershipEntry{}

	for _, entry := range daemonTurnExecutionHelperOwnershipTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.callerSurface)
		require.NotEmpty(t, entry.requestHelperOwner)
		require.NotEmpty(t, entry.turnHelperOwner)
		require.NotEmpty(t, entry.payloadHelperOwner)
		require.NotEmpty(t, entry.remainingDaemonScope)
		require.NotEmpty(t, entry.nextMigration)
		require.Empty(t, seenOwnership[entry.kind].kind, "duplicate helper ownership entry for %q", entry.kind)
		seenOwnership[entry.kind] = entry

		got, ok := daemonTurnExecutionHelperOwnershipForKind(entry.kind)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantOwnership, seenOwnership)
	require.True(t, daemonTurnExecutionInternalHelperRuntimeOwned(daemonTurnExecutionDeterministicRoute))
	require.True(t, daemonTurnExecutionInternalHelperRuntimeOwned(daemonTurnExecutionPlannerActionExecution))
	require.False(t, daemonTurnExecutionInternalHelperRuntimeOwned(daemonTurnExecutionReplanActionContinuation))
	_, ok := daemonTurnExecutionHelperOwnershipForKind("unsupported")
	require.False(t, ok)
	require.False(t, daemonTurnExecutionInternalHelperRuntimeOwned("unsupported"))
}

func TestDaemonTurnExecutionAgentLoopMigrationPreconditions(t *testing.T) {
	want := map[string]daemonTurnExecutionAgentLoopMigrationPreconditionEntry{
		daemonTurnExecutionDeterministicRoute: {
			kind:                   daemonTurnExecutionDeterministicRoute,
			stage:                  daemonTurnExecutionAgentLoopMigrationStageAgentLoopCallerLive,
			currentCaller:          "agentManager.runRuntimeDeterministicRouteWithToolServices -> runtime.Engine.RunAgentLoop",
			proposedMode:           "AgentLoopModeDeterministicRoute",
			requiredRuntimeHelpers: "AgentLoopTurnExecutionRequest, AgentLoopTurnExecutionTurnFromRequest, AgentLoopTurnResultPayload",
			requiredAdapter:        "daemonAgentLoopRunner deterministic_route branch preserving routePromptForTurnWithToolServices parity",
			remainingDaemonScope:   "route response egress conversion and upstream routerServices-owned non-route services; registry dispatch uses runtime AgentLoopToolServiceBinding",
			nextStep:               "retire deterministic route TurnExecutor compatibility after route response egress and remaining legacy route adapter cleanup",
		},
		daemonTurnExecutionPlannerActionExecution: {
			kind:                   daemonTurnExecutionPlannerActionExecution,
			stage:                  daemonTurnExecutionAgentLoopMigrationStageBlockedBySideEffect,
			currentCaller:          "agentManager.runRuntimePlannerActionExecution -> runtime.Engine.RunAgentLoop with planner_action_execution mode",
			proposedMode:           "AgentLoopModePlannerActionExecution",
			requiredRuntimeHelpers: "AgentLoopPlannerActionExecutionRequest, AgentLoopActionExecutorRequest, AgentLoopActionExecutorResult, AgentLoopActionExecutionTurn, AgentLoopTurnResultPayload",
			requiredAdapter:        "daemonAgentLoopRunner planner_action_execution branch preserving Engine.ExecuteAgentLoopAction, TurnExecutor compatibility fallback and legacy executor parity",
			remainingDaemonScope:   "executeAgentPlannerAction side effects, legacy routerServices handle unwrap, action error/result egress",
			nextStep:               "continue runtime-native service dispatch and result egress cleanup before retiring TurnExecutor/direct legacy fallbacks",
		},
	}
	seen := map[string]daemonTurnExecutionAgentLoopMigrationPreconditionEntry{}

	for _, entry := range daemonTurnExecutionAgentLoopMigrationPreconditionTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.currentCaller)
		require.NotEmpty(t, entry.proposedMode)
		require.NotEmpty(t, entry.requiredRuntimeHelpers)
		require.NotEmpty(t, entry.requiredAdapter)
		require.NotEmpty(t, entry.remainingDaemonScope)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seen[entry.kind].kind, "duplicate AgentLoop migration precondition entry for %q", entry.kind)
		seen[entry.kind] = entry

		got, ok := daemonTurnExecutionAgentLoopMigrationPreconditionForKind(entry.kind)
		require.True(t, ok)
		require.Equal(t, entry, got)
		require.True(t, daemonTurnExecutionInternalHelperRuntimeOwned(entry.kind))
		switch entry.stage {
		case daemonTurnExecutionAgentLoopMigrationStagePreconditionReady, daemonTurnExecutionAgentLoopMigrationStageAgentLoopCallerLive:
			require.True(t, daemonTurnExecutionAgentLoopMigrationPreconditionReady(entry.kind))
		default:
			require.False(t, daemonTurnExecutionAgentLoopMigrationPreconditionReady(entry.kind))
		}
	}

	require.Equal(t, want, seen)
	require.True(t, daemonTurnExecutionAgentLoopMigrationPreconditionReady(daemonTurnExecutionDeterministicRoute))
	require.False(t, daemonTurnExecutionAgentLoopMigrationPreconditionReady(daemonTurnExecutionPlannerActionExecution))
	require.False(t, daemonTurnExecutionAgentLoopMigrationPreconditionReady(daemonTurnExecutionReplanActionContinuation))
	_, ok := daemonTurnExecutionAgentLoopMigrationPreconditionForKind("unsupported")
	require.False(t, ok)
	require.False(t, daemonTurnExecutionAgentLoopMigrationPreconditionReady("unsupported"))
}

func TestDaemonTurnExecutionPlannerActionExecutionAgentLoopMigrationBlockers(t *testing.T) {
	want := map[string]daemonTurnExecutionAgentLoopMigrationBlockerEntry{
		"executeAgentPlannerAction side effects": {
			kind:       daemonTurnExecutionPlannerActionExecution,
			blocker:    "executeAgentPlannerAction side effects",
			owner:      daemonTurnExecutionAgentLoopMigrationBlockerOwnerDaemonLegacy,
			resolution: "move tool execution/service dispatch behind a runtime action executor boundary",
		},
		"legacy routerServices handle unwrap": {
			kind:       daemonTurnExecutionPlannerActionExecution,
			blocker:    "legacy routerServices handle unwrap",
			owner:      daemonTurnExecutionAgentLoopMigrationBlockerOwnerDaemonLegacy,
			resolution: "replace daemon routerServices unwrapping with a runtime AgentLoopToolServiceBinding executor contract",
		},
		"action request/result DTO": {
			kind:       daemonTurnExecutionPlannerActionExecution,
			blocker:    "action request/result DTO",
			owner:      daemonTurnExecutionAgentLoopMigrationBlockerOwnerRuntimeDTO,
			resolution: "runtime DTO helpers already cover request, turn payload, result payload and clone boundaries",
		},
	}
	seen := map[string]daemonTurnExecutionAgentLoopMigrationBlockerEntry{}

	for _, entry := range daemonTurnExecutionAgentLoopMigrationBlockerTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.blocker)
		require.NotEmpty(t, entry.owner)
		require.NotEmpty(t, entry.resolution)
		require.Empty(t, seen[entry.blocker].kind, "duplicate AgentLoop migration blocker entry for %q", entry.blocker)
		seen[entry.blocker] = entry
	}

	require.Equal(t, want, seen)
	require.Equal(t, len(want), len(daemonTurnExecutionAgentLoopMigrationBlockersForKind(daemonTurnExecutionPlannerActionExecution)))
	require.True(t, daemonTurnExecutionAgentLoopMigrationBlockedByDaemonSideEffects(daemonTurnExecutionPlannerActionExecution))
	require.False(t, daemonTurnExecutionAgentLoopMigrationBlockedByDaemonSideEffects(daemonTurnExecutionDeterministicRoute))
	require.False(t, daemonTurnExecutionAgentLoopMigrationBlockedByDaemonSideEffects(daemonTurnExecutionReplanActionContinuation))
	require.Empty(t, daemonTurnExecutionAgentLoopMigrationBlockersForKind("unsupported"))
	require.False(t, daemonTurnExecutionAgentLoopMigrationBlockedByDaemonSideEffects("unsupported"))
}

func TestDaemonPlannerActionExecutionSideEffectBoundaryFromAgentLoopClonesRuntimeInputs(t *testing.T) {
	enabled := true
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	loopReq := agentPlannerActionExecutionRequestFromRuntime(
		"session-side-effect-boundary",
		agentRuntimeTurnRef{TaskID: "task-side-effect-boundary", RunID: "run-side-effect-boundary", TurnID: "turn-side-effect-boundary"},
		"request https://example.com",
		"en",
		"tester",
		"safe_scan",
		agentLoopToolServiceBindingFromRouterServices(routerServices{jobs: jobs}),
		runtime.AgentLoopAuthorization{
			AuthProfiles: []runtime.AgentLoopAuthProfile{{
				ID:      "auth-1",
				Token:   "secret-token",
				Enabled: &enabled,
			}},
			DefaultAuthProfileID: "auth-1",
		},
	)
	action := runtime.AgentLoopAction{
		Name:      "http.request",
		Arguments: map[string]any{"url": "https://example.com"},
	}

	boundary := daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop(loopReq, action)

	loopReq.Authorization.AuthProfiles[0].Token = "mutated-token"
	*loopReq.Authorization.AuthProfiles[0].Enabled = false
	loopReq.ToolServices.Metadata["adapter"] = "mutated-adapter"
	action.Arguments["url"] = "https://mutated.example"

	require.Equal(t, "session-side-effect-boundary", boundary.sessionID)
	require.Equal(t, "en", boundary.language)
	require.Equal(t, "tester", boundary.operator)
	require.Equal(t, "safe_scan", boundary.operationLevel)
	require.Equal(t, daemonToolServiceBindingKind, boundary.toolServices.Kind)
	require.Equal(t, "daemon_legacy_router_services", boundary.toolServices.Metadata["adapter"])
	boundaryJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](boundary.toolServices.ActionServices, runtime.AgentLoopActionServiceJobs)
	require.True(t, ok)
	require.Same(t, jobs, boundaryJobs)
	require.Equal(t, "http.request", boundary.action.Name)
	require.Equal(t, "https://example.com", boundary.action.Arguments["url"])
	require.Equal(t, "auth-1", boundary.authorization.DefaultAuthProfileID)
	require.Len(t, boundary.authorization.AuthProfiles, 1)
	require.Equal(t, "secret-token", boundary.authorization.AuthProfiles[0].Token)
	require.NotNil(t, boundary.authorization.AuthProfiles[0].Enabled)
	require.True(t, *boundary.authorization.AuthProfiles[0].Enabled)
}

func TestDaemonPlannerActionExecutionSideEffectBoundaryDoesNotReconstructLegacyServices(t *testing.T) {
	loopReq := agentPlannerActionExecutionRequestFromRuntime(
		"session-side-effect-runtime-services",
		agentRuntimeTurnRef{TaskID: "task-runtime-services", RunID: "run-runtime-services", TurnID: "turn-runtime-services"},
		"run runtime action",
		"en",
		"tester",
		"safe_scan",
		runtime.AgentLoopToolServiceBinding{
			Kind:   daemonToolServiceBindingKind,
			Handle: registryPanicActionServiceFallback{},
			Metadata: map[string]any{
				"adapter": "opaque-handle-panic-on-legacy-reconstruction",
			},
		},
		runtime.AgentLoopAuthorization{},
	)

	boundary := daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop(loopReq, runtime.AgentLoopAction{Name: "daemon.health"})

	loopReq.ToolServices.Metadata["adapter"] = "mutated"
	require.Equal(t, daemonToolServiceBindingKind, boundary.toolServices.Kind)
	require.Equal(t, "opaque-handle-panic-on-legacy-reconstruction", boundary.toolServices.Metadata["adapter"])
	require.IsType(t, registryPanicActionServiceFallback{}, boundary.toolServices.Handle)
}

func TestDaemonPlannerActionExecutionExecutorSeamDispatchesLegacyPlannerAction(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok)
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		require.NotNil(t, ctx)
		require.Equal(t, "session-executor-seam", in.SessionID)
		require.Equal(t, "tester", in.Operator)
		require.Equal(t, "en", in.Language)
		require.Equal(t, "safe_scan", in.OperationLevel)
		require.Equal(t, "ok", in.RawArgs["probe"])
		tool := runningAgentToolCall("daemon.health", "registry.handler")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"seam": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "executor seam dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	result, err := daemonPlannerActionExecutionExecutorForManager(mgr).ExecutePlannerAction(context.Background(), daemonPlannerActionExecutionSideEffectBoundary{
		sessionID:      "session-executor-seam",
		language:       "en",
		operator:       "tester",
		operationLevel: "safe_scan",
		action: agentPlannerAction{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "ok"},
		},
	})

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, runtime.TurnResultFinished, result.Status)
	require.Equal(t, "daemon_legacy_planner_action_executor", result.Metadata["executor"])
	require.Len(t, result.Messages, 1)
	require.Equal(t, "executor seam dispatched", result.Messages[0].Content)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["seam"])
}

func TestDaemonPlannerActionExecutionExecutorSeamRequiresManager(t *testing.T) {
	result, err := daemonPlannerActionExecutionExecutorForManager(nil).ExecutePlannerAction(context.Background(), daemonPlannerActionExecutionSideEffectBoundary{})
	require.ErrorContains(t, err, "daemon: planner action execution executor has no manager")
	require.Equal(t, runtime.TurnResultBlocked, result.Status)
	require.ErrorIs(t, result.Error, err)
	require.Empty(t, result.Messages)
	require.Empty(t, result.ToolCalls)
	require.Equal(t, "daemon_legacy_planner_action_executor", result.Metadata["executor"])
}

func TestDaemonAgentLoopActionExecutorCoversHeuristicRoute(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok)
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		require.Equal(t, "session-heuristic-seam", in.SessionID)
		require.Equal(t, "tester", in.Operator)
		require.Equal(t, "en", in.Language)
		require.Equal(t, "read_only", in.OperationLevel)
		tool := runningAgentToolCall("daemon.health", "registry.handler")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"heuristic_seam": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "heuristic seam dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, calls, err := mgr.routeReadOnlySystemHeuristicActionsWithToolServices(context.Background(), "session-heuristic-seam", "daemon health", "en", "tester", "read_only", testAgentLoopToolServices())

	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, messages, 1)
	require.Equal(t, "heuristic seam dispatched", messages[0].Content)
	require.Len(t, calls, 1)
	require.Equal(t, true, calls[0].Result["heuristic_seam"])
}

func TestDaemonAgentLoopActionExecutorDispatchesLegacyPlannerAction(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)
	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok)
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		require.NotNil(t, ctx)
		require.Equal(t, "session-runtime-action-executor", in.SessionID)
		require.Equal(t, "tester", in.Operator)
		require.Equal(t, "en", in.Language)
		require.Equal(t, "safe_scan", in.OperationLevel)
		require.Equal(t, "runtime-action-executor", in.RawArgs["probe"])
		tool := runningAgentToolCall("daemon.health", "runtime.action.executor")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"runtime_action_executor": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "runtime action executor dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	result, err := eng.ExecuteAgentLoopAction(context.Background(), runtime.AgentLoopActionExecutorRequest{
		Request: agentPlannerActionExecutionRequestFromRuntime(
			"session-runtime-action-executor",
			agentRuntimeTurnRef{TaskID: "task-runtime-action-executor", RunID: "run-runtime-action-executor", TurnID: "turn-runtime-action-executor"},
			"run daemon health",
			"en",
			"tester",
			"safe_scan",
			agentLoopToolServiceBindingFromRouterServices(routerServices{}),
			runtime.AgentLoopAuthorization{},
		),
		Action: runtime.AgentLoopAction{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "runtime-action-executor"},
		},
	})

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, runtime.TurnResultFinished, result.Status)
	require.Equal(t, "daemon_legacy_planner_action_executor", result.Metadata["executor"])
	require.Len(t, result.Messages, 1)
	require.Equal(t, "runtime action executor dispatched", result.Messages[0].Content)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["runtime_action_executor"])
}

func TestDaemonAgentLoopActionExecutorRequiresManager(t *testing.T) {
	result, err := daemonAgentLoopActionExecutor{}.ExecuteAgentLoopAction(context.Background(), runtime.AgentLoopActionExecutorRequest{})
	require.ErrorContains(t, err, "daemon: agent loop action executor manager not attached")
	require.Equal(t, runtime.TurnResultBlocked, result.Status)
	require.ErrorIs(t, result.Error, err)
	require.Equal(t, "daemon_legacy_planner_action_executor", result.Metadata["executor"])
}

func TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers(t *testing.T) {
	want := map[string]daemonPlannerActionExecutionSideEffectBoundaryPlanEntry{
		"request_turn_action_authorization_dto": {
			boundary:     "request_turn_action_authorization_dto",
			currentOwner: daemonPlannerActionExecutionSideEffectOwnerRuntimeDTO,
			targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			extractedBy:  "daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop clones AgentLoopRequest, AgentLoopAction and AgentLoopAuthorization",
			nextStep:     "keep runtime DTOs as the AgentLoop mode input when introducing the planner action executor contract",
		},
		"tool_service_dispatch": {
			boundary:     "tool_service_dispatch",
			currentOwner: daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			extractedBy:  "daemon planner action executor and deterministic route now pass cloned runtime AgentLoopToolServiceBinding directly into registry dispatch without routerServices reconstruction",
			nextStep:     "keep production dispatch on runtime tool service bindings and test coverage on direct ActionServices capabilities",
		},
		"tool_service_provider_compatibility_fixture": {
			boundary:     "tool_service_provider_compatibility_fixture",
			currentOwner: daemonPlannerActionExecutionSideEffectOwnerCompatibilityFixture,
			targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			extractedBy:  "historical provider/routerServices roundtrip contract is retired; production binding carries runtime AgentLoopActionServiceSet and tests assert capabilities directly",
			nextStep:     "keep the test suite free of explicit Handle/provider roundtrip fixtures",
		},
		"http_auth_profile_adapter": {
			boundary:     "http_auth_profile_adapter",
			currentOwner: daemonPlannerActionExecutionSideEffectOwnerDaemonLegacy,
			targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			extractedBy:  "daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop carries AgentLoopAuthorization directly through the executor and registry dispatch signatures",
			nextStep:     "keep legacy registry runtime auth-profile compatibility fields read-only while replacing remaining daemon-owned result egress",
		},
		"runtime_result_composition": {
			boundary:     "runtime_result_composition",
			currentOwner: daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			extractedBy:  "runtime.AgentLoopPlannerActionExecutionResultFromActionExecutorResult and FromTurnPayload build the AgentLoopResult envelope",
			nextStep:     "keep planner_action_execution AgentLoop result composition runtime-owned while legacy adapters shrink",
		},
		"legacy_action_result_adapter": {
			boundary:     "legacy_action_result_adapter",
			currentOwner: daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
			extractedBy:  "executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices return runtime AgentLoopActionExecutorResult directly while preserving runtime tool services; legacy routerServices-ingress wrappers are retired",
			nextStep:     "continue shrinking tool-service dispatch adapters behind runtime-native action service interfaces",
		},
	}
	seen := map[string]daemonPlannerActionExecutionSideEffectBoundaryPlanEntry{}

	for _, entry := range daemonPlannerActionExecutionSideEffectBoundaryPlanTable {
		require.NotEmpty(t, entry.boundary)
		require.NotEmpty(t, entry.currentOwner)
		require.NotEmpty(t, entry.targetOwner)
		require.NotEmpty(t, entry.extractedBy)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seen[entry.boundary].boundary, "duplicate planner action side-effect boundary plan entry for %q", entry.boundary)
		seen[entry.boundary] = entry
	}

	require.Equal(t, want, seen)
	require.True(t, daemonPlannerActionExecutionSideEffectBoundaryPlanBlockedByDaemonLegacy())
}

func TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers(t *testing.T) {
	want := map[string]daemonPlannerActionExecutorContractPreconditionEntry{
		"action_request_dto": {
			component:        "action_request_dto",
			stage:            daemonPlannerActionExecutorContractStageReady,
			currentSurface:   "AgentLoopActionExecutionRequest + AgentLoopActionExecutionTurn + AgentLoopTurnResultPayload",
			requiredContract: "runtime action executor input can reuse existing request/action/authorization DTOs",
			nextStep:         "keep DTO contract stable while migrating the executor dispatch surface",
		},
		"typed_tool_service_binding": {
			component:        "typed_tool_service_binding",
			stage:            daemonPlannerActionExecutorContractStageReady,
			currentSurface:   "planner action side-effect boundary carries cloned AgentLoopToolServiceBinding; ActionServices carries runtime capability names and Handle is no longer populated from routerServices by default",
			requiredContract: "runtime action service capability set available after retiring test-only provider compatibility fixtures",
			nextStep:         "keep registry handlers on runtime action service capability consumers",
		},
		"registry_handler_service_consumers": {
			component:        "registry_handler_service_consumers",
			stage:            daemonPlannerActionExecutorContractStageReady,
			currentSurface:   "builtin registry handlers consume agentRegistryHandlerRuntime.actionServiceResolver(); production registry runtime no longer exposes toolServiceProvider()/legacyRouterServices() helper methods",
			requiredContract: "registry handlers consume runtime AgentLoopActionServiceSet capabilities without opaque Handle fallback",
			nextStep:         "keep provider/routerServices reconstruction retired from production and test runtime paths",
		},
		"authorization_context": {
			component:        "authorization_context",
			stage:            daemonPlannerActionExecutorContractStageReady,
			currentSurface:   "planner action execution carries runtime.AgentLoopAuthorization through executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices; http.request resolves credentials from agentRegistryHandlerRuntime.Authorization",
			requiredContract: "action handlers consume AgentLoopAuthorization or a runtime auth resolver directly",
			nextStep:         "remove legacy registry runtime auth-profile compatibility fields once all handler/test callers consume Authorization only",
		},
		"executor_result_egress": {
			component:        "executor_result_egress",
			stage:            daemonPlannerActionExecutorContractStageReady,
			currentSurface:   "executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices return runtime.AgentLoopActionExecutorResult; registryHandlerOutputToActionExecutorResult builds runtime messages/tool calls directly from handler outputs",
			requiredContract: "action executor implementation returns AgentLoopActionExecutorResult-compatible messages/tool calls without daemon-private result types",
			nextStep:         "replace remaining daemon-private tool-service dispatch with runtime-native action service interfaces",
		},
	}
	seen := map[string]daemonPlannerActionExecutorContractPreconditionEntry{}

	for _, entry := range daemonPlannerActionExecutorContractPreconditionTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.currentSurface)
		require.NotEmpty(t, entry.requiredContract)
		require.NotEmpty(t, entry.nextStep)
		if entry.stage == daemonPlannerActionExecutorContractStageBlocked {
			require.NotEmpty(t, entry.blocker)
		}
		require.Empty(t, seen[entry.component].component, "duplicate planner action executor contract precondition for %q", entry.component)
		seen[entry.component] = entry
	}

	require.Equal(t, want, seen)
	require.True(t, daemonPlannerActionExecutorContractReadyForAgentLoop())
	blockers := daemonPlannerActionExecutorContractBlockers()
	require.Empty(t, blockers)
}

func TestDaemonPlannerActionExecutionLegacyCompatibilitySurfaceRetiredAudit(t *testing.T) {
	want := map[string]daemonPlannerActionExecutionLegacySurfaceEntry{
		"executeAgentPlannerAction": {
			symbol:                  "executeAgentPlannerAction",
			stage:                   daemonPlannerActionExecutionLegacySurfaceStageRetired,
			allowedNonTestSurface:   "none; direct legacy message/tool-call compatibility return has been removed",
			currentAdapter:          "daemonPlannerActionExecutionExecutor",
			newCallerContract:       "new internal callers must use daemonPlannerActionExecutionExecutorForManager(...).ExecutePlannerAction and consume runtime.AgentLoopActionExecutorResult",
			runtimeMigrationBlocker: "retired; direct callers/tests consume executeAgentPlannerActionResultWithToolServices or the runtime action executor contract",
			removalGate:             "already removed after direct callers/tests moved to runtime result wrapper",
		},
	}
	seen := map[string]daemonPlannerActionExecutionLegacySurfaceEntry{}

	for _, entry := range daemonPlannerActionExecutionLegacySurfaceTable {
		require.NotEmpty(t, entry.symbol)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.allowedNonTestSurface)
		require.NotEmpty(t, entry.currentAdapter)
		require.NotEmpty(t, entry.newCallerContract)
		require.NotEmpty(t, entry.runtimeMigrationBlocker)
		require.NotEmpty(t, entry.removalGate)
		require.Empty(t, seen[entry.symbol].symbol, "duplicate planner action legacy surface entry for %q", entry.symbol)
		seen[entry.symbol] = entry

		got, ok := daemonPlannerActionExecutionLegacySurfaceForSymbol(entry.symbol)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, want, seen)
	require.False(t, daemonPlannerActionExecutionLegacySurfaceImplementationOnly("executeAgentPlannerAction"))
	_, ok := daemonPlannerActionExecutionLegacySurfaceForSymbol("unsupported")
	require.False(t, ok)
	require.False(t, daemonPlannerActionExecutionLegacySurfaceImplementationOnly("unsupported"))
	require.True(t, daemonPlannerActionExecutorContractReadyForAgentLoop())
}

func TestDaemonPlannerActionExecutionFallbackOwnershipTableCoversChain(t *testing.T) {
	wantReasons := map[daemonPlannerActionExecutionFallbackReason]string{
		daemonPlannerActionExecutionFallbackEngineNotAttached:    daemonPlannerActionExecutionFallbackOwnerLegacyCaller,
		daemonPlannerActionExecutionFallbackDirectLegacyExecutor: daemonPlannerActionExecutionFallbackOwnerLegacyCaller,
	}
	seenReasons := map[daemonPlannerActionExecutionFallbackReason]string{}

	for _, entry := range daemonPlannerActionExecutionFallbackOwnershipTable {
		require.NotEmpty(t, entry.reason)
		require.NotEmpty(t, entry.owner)
		require.Empty(t, seenReasons[entry.reason], "duplicate planner action execution fallback ownership entry for %q", entry.reason)
		seenReasons[entry.reason] = entry.owner

		owner, ok := daemonPlannerActionExecutionFallbackOwnerForReason(entry.reason)
		require.True(t, ok)
		require.Equal(t, entry.owner, owner)
		require.True(t, daemonPlannerActionExecutionFallbackOwnedByLegacyCaller(entry.reason))
	}

	require.Equal(t, wantReasons, seenReasons)
	_, ok := daemonPlannerActionExecutionFallbackOwnerForReason(daemonPlannerActionExecutionFallbackReason("unsupported"))
	require.False(t, ok)
	require.False(t, daemonPlannerActionExecutionFallbackOwnedByLegacyCaller(daemonPlannerActionExecutionFallbackReason("unsupported")))
	require.ErrorContains(t, daemonPlannerActionExecutionFallbackOwnershipError(daemonPlannerActionExecutionFallbackReason("unsupported")), "fallback ownership missing")

	reason, fallback := daemonPlannerActionExecutionActionExecutorPreDispatchFallbackReason(nil)
	require.True(t, fallback)
	require.Equal(t, daemonPlannerActionExecutionFallbackEngineNotAttached, reason)
	require.True(t, daemonPlannerActionExecutionActionExecutorMissingRetirementAllowsDirectFallback(reason))

	eng := runtime.NewEngine(runtime.NewMemStore())
	_, fallback = daemonPlannerActionExecutionActionExecutorPreDispatchFallbackReason(eng)
	require.False(t, fallback)
	require.False(t, daemonPlannerActionExecutionActionExecutorMissingRetirementAllowsDirectFallback(daemonPlannerActionExecutionFallbackActionExecutorNotAttached))

	eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(context.Context, runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
		return runtime.AgentLoopActionExecutorResult{}, nil
	}))
	_, fallback = daemonPlannerActionExecutionActionExecutorPreDispatchFallbackReason(eng)
	require.False(t, fallback)

	_, fallback = daemonPlannerActionExecutionActionExecutorResultFallbackReason(runtime.AgentLoopActionExecutorResult{}, runtime.ErrAgentLoopActionExecutorNotAttached)
	require.False(t, fallback)

	_, fallback = daemonPlannerActionExecutionActionExecutorResultFallbackReason(runtime.AgentLoopActionExecutorResult{Error: runtime.ErrAgentLoopActionExecutorNotAttached}, nil)
	require.False(t, fallback)

	blocked, ok := daemonPlannerActionExecutionActionExecutorNotAttachedBlockedResult(runtime.AgentLoopActionExecutorResult{}, runtime.ErrAgentLoopActionExecutorNotAttached)
	require.True(t, ok)
	require.Equal(t, runtime.TurnResultBlocked, blocked.Status)
	require.ErrorIs(t, blocked.Error, runtime.ErrAgentLoopActionExecutorNotAttached)
	require.Equal(t, "retired", blocked.Metadata["planner_action_execution_action_executor_fallback"])

	blocked, ok = daemonPlannerActionExecutionActionExecutorNotAttachedBlockedResult(runtime.AgentLoopActionExecutorResult{Error: runtime.ErrAgentLoopActionExecutorNotAttached}, nil)
	require.True(t, ok)
	require.Equal(t, runtime.TurnResultBlocked, blocked.Status)
	require.ErrorIs(t, blocked.Error, runtime.ErrAgentLoopActionExecutorNotAttached)

	_, fallback = daemonPlannerActionExecutionActionExecutorResultFallbackReason(runtime.AgentLoopActionExecutorResult{Status: runtime.TurnResultFailed}, errors.New("action executor hard error"))
	require.False(t, fallback)
}

func TestDaemonPlannerActionExecutionFallbackBoundaryCoversChain(t *testing.T) {
	want := map[string]daemonPlannerActionExecutionFallbackBoundaryEntry{
		daemonPlannerActionExecutionFallbackStageActionExecutor: {
			stage:          daemonPlannerActionExecutionFallbackStageActionExecutor,
			callerSurface:  "runAgentPlannerActionExecutionAgentLoopResult -> runRuntimePlannerActionExecutionActionExecutor",
			runtimeSurface: "runtime.Engine.ExecuteAgentLoopAction",
			legacyFallback: "daemonAgentLoopActionExecutor.ExecuteAgentLoopAction",
			owner:          daemonPlannerActionExecutionFallbackOwnerLegacyCaller,
			scope:          "planner_action_execution falls back directly to the daemon action executor only when the runtime engine is missing; attached-engine missing/detached AgentLoopActionExecutor returns blocked/error and the internal TurnExecutor compatibility hop is retired",
		},
		daemonPlannerActionExecutionFallbackStageDirectLegacy: {
			stage:          daemonPlannerActionExecutionFallbackStageDirectLegacy,
			callerSurface:  "runAgentPlannerActionExecutionAgentLoopResult -> daemonAgentLoopActionExecutor.ExecuteAgentLoopAction",
			runtimeSurface: "daemonAgentLoopActionExecutor",
			legacyFallback: "daemonPlannerActionExecutionExecutor.ExecutePlannerAction",
			owner:          daemonPlannerActionExecutionFallbackOwnerLegacyCaller,
			scope:          "final compatibility executor preserves daemon registry dispatch side effects after the runtime action executor declines handling",
		},
	}
	seen := map[string]daemonPlannerActionExecutionFallbackBoundaryEntry{}

	for _, entry := range daemonPlannerActionExecutionFallbackBoundaryTable {
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.callerSurface)
		require.NotEmpty(t, entry.runtimeSurface)
		require.NotEmpty(t, entry.legacyFallback)
		require.Equal(t, daemonPlannerActionExecutionFallbackOwnerLegacyCaller, entry.owner)
		require.NotEmpty(t, entry.scope)
		require.Empty(t, seen[entry.stage].stage, "duplicate planner action execution fallback boundary entry for %q", entry.stage)
		seen[entry.stage] = entry

		got, ok := daemonPlannerActionExecutionFallbackBoundaryForStage(entry.stage)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, want, seen)
	actionExecutor, ok := daemonPlannerActionExecutionFallbackBoundaryForStage(daemonPlannerActionExecutionFallbackStageActionExecutor)
	require.True(t, ok)
	require.Equal(t, "daemonAgentLoopActionExecutor.ExecuteAgentLoopAction", actionExecutor.legacyFallback)
	require.Contains(t, actionExecutor.scope, "only when the runtime engine is missing")
	require.Contains(t, actionExecutor.scope, "attached-engine missing/detached AgentLoopActionExecutor returns blocked/error")
	require.Contains(t, actionExecutor.scope, "TurnExecutor compatibility hop is retired")

	_, ok = daemonPlannerActionExecutionFallbackBoundaryForStage("unsupported")
	require.False(t, ok)
}

func TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	managerCallPattern := regexp.MustCompile(`rt\.Manager\.(handleAgent[A-Za-z0-9]+)\(`)
	actualMethods := map[string]bool{}
	for _, match := range managerCallPattern.FindAllStringSubmatch(string(data), -1) {
		actualMethods[match[1]] = true
	}

	seenComponents := map[string]bool{}
	seenActions := map[string]string{}
	coveredMethods := map[string]bool{}
	for _, entry := range daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.actions)
		require.NotEmpty(t, entry.managerMethods)
		require.NotEmpty(t, entry.currentOwner)
		require.NotEmpty(t, entry.currentSurface)
		require.NotEmpty(t, entry.targetCapabilities)
		require.NotEmpty(t, entry.evidence)
		require.NotEmpty(t, entry.nextStep)
		require.False(t, seenComponents[entry.component], "duplicate runtime action service ownership component %q", entry.component)
		seenComponents[entry.component] = true

		got, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)

		for _, action := range entry.actions {
			require.NotEmpty(t, action)
			require.Empty(t, seenActions[action], "action %q appears in both %q and %q", action, seenActions[action], entry.component)
			seenActions[action] = entry.component
		}
		for _, method := range entry.managerMethods {
			require.NotEmpty(t, method)
			if entry.stage == daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached {
				continue
			}
			require.False(t, coveredMethods[method], "duplicate manager method ownership for %q", method)
			coveredMethods[method] = true
		}
	}

	require.Equal(t, sortedStringKeys(actualMethods), sortedStringKeys(coveredMethods))
	require.Equal(t, map[string]bool{
		"scan_registry_handler":                  true,
		"health_plugins_runs_registry_handlers":  true,
		"schedule_registry_handlers":             true,
		"web_search_registry_handler":            true,
		"run_context_approval_registry_handlers": true,
		"workspace_registry_handlers":            true,
		"network_request_registry_handlers":      true,
		"port_scan_registry_handler":             true,
	}, seenComponents)
	require.Empty(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityOwned))
	require.Len(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached), 8)
	require.Empty(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageSelectedNextCapability))
	require.Empty(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageDaemonManagerOwned))
	require.Empty(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage("unsupported"))

	_, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipSelectedNextCapability()
	require.False(t, ok)

	health, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("health_plugins_runs_registry_handlers")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, health.stage)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, health.currentOwner)
	require.Equal(t, []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceDaemonStatusAction}, health.targetCapabilities)
	require.Contains(t, health.actions, "daemon.health")
	require.Contains(t, health.actions, "plugins.list")
	require.Contains(t, health.actions, "runs.list")
	require.Contains(t, health.managerMethods, "handleAgentHealth")
	require.Contains(t, health.currentSurface, "DaemonStatus()")
	require.NotContains(t, string(data), "rt.Manager.handleAgentHealth(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentPlugins(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentRuns(")

	scan, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("scan_registry_handler")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, scan.stage)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, scan.currentOwner)
	require.Equal(t, []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceWebScan}, scan.targetCapabilities)
	require.Contains(t, scan.actions, "scan.web.safe")
	require.Contains(t, scan.managerMethods, "handleAgentScan")
	require.Contains(t, scan.currentSurface, "WebScan()")
	require.NotContains(t, string(data), "rt.Manager.handleAgentScan(")

	schedule, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("schedule_registry_handlers")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, schedule.stage)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, schedule.currentOwner)
	require.Equal(t, []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceScheduleAction}, schedule.targetCapabilities)
	require.Contains(t, schedule.actions, "schedule.create")
	require.Contains(t, schedule.actions, "schedule.delete")
	require.Contains(t, schedule.managerMethods, "handleAgentScheduleCreate")
	require.Contains(t, schedule.currentSurface, "ScheduleAction()")
	require.NotContains(t, string(data), "rt.Manager.handleAgentScheduleCreate(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentScheduleList(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentScheduleDelete(")

	runContext, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("run_context_approval_registry_handlers")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, runContext.stage)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, runContext.currentOwner)
	require.Equal(t, []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceRunContextApproval}, runContext.targetCapabilities)
	require.Contains(t, runContext.actions, "runs.context")
	require.Contains(t, runContext.actions, "runs.approvals.approve")
	require.Contains(t, runContext.managerMethods, "handleAgentRunContextTool")
	require.Contains(t, runContext.currentSurface, "RunContextApproval()")
	require.Contains(t, runContext.nextStep, "TurnExecutor compatibility fallback retirement")
	require.NotContains(t, string(data), "rt.Manager.handleAgentRunContextTool(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentReportDraft(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentApprovals(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentRunContextAnalysis(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentResumeApproved(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentApprovalDecision(")

	workspace, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("workspace_registry_handlers")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, workspace.stage)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, workspace.currentOwner)
	require.Equal(t, []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceWorkspace}, workspace.targetCapabilities)
	require.Contains(t, workspace.actions, "workspace.files.apply")
	require.Contains(t, workspace.actions, "workspace.search")
	require.Contains(t, workspace.managerMethods, "handleAgentCommand")
	require.Contains(t, workspace.managerMethods, "handleAgentWorkspaceWrite")
	require.Contains(t, workspace.currentSurface, "Workspace()")
	require.NotContains(t, string(data), "rt.Manager.handleAgentCommand(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentWorkspace")

	network, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("network_request_registry_handlers")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, network.stage)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, network.currentOwner)
	require.Equal(t, []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceNetworkRequest}, network.targetCapabilities)
	require.Contains(t, network.actions, "http.request")
	require.Contains(t, network.actions, "web.fetch")
	require.Contains(t, network.managerMethods, "handleAgentHTTPRequest")
	require.Contains(t, network.managerMethods, "handleAgentWebFetch")
	require.Contains(t, network.currentSurface, "NetworkRequest()")
	require.NotContains(t, string(data), "rt.Manager.handleAgentHTTPRequest(")
	require.NotContains(t, string(data), "rt.Manager.handleAgentWebFetch(")

	webSearch, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("web_search_registry_handler")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, webSearch.stage)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, webSearch.currentOwner)
	require.Equal(t, []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceWebSearch}, webSearch.targetCapabilities)
	require.Contains(t, webSearch.actions, "web.search")
	require.Contains(t, webSearch.managerMethods, "handleAgentWebSearch")
	require.Contains(t, webSearch.currentSurface, "WebSearch()")
	require.NotContains(t, string(data), "rt.Manager.handleAgentWebSearch(")

	portScan, ok := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("port_scan_registry_handler")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, portScan.stage)
	require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, portScan.currentOwner)
	require.Equal(t, []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServicePortScanner}, portScan.targetCapabilities)
	require.Contains(t, portScan.actions, "ports.scan.safe")
	require.Contains(t, portScan.managerMethods, "handleAgentPortScan")
	require.Contains(t, portScan.currentSurface, "PortScanner()")
	require.NotContains(t, string(data), "rt.Manager.handleAgentPortScan(")

	_, ok = daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent("unsupported")
	require.False(t, ok)
}

func TestDaemonPlannerActionExecutionRegistryAdapterCompletionAuditSelectsNextFallbackBoundary(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	require.Empty(t, regexp.MustCompile(`rt\.Manager\.handleAgent[A-Za-z0-9]+\(`).FindAllString(string(data), -1))

	wantStages := map[string]daemonPlannerActionExecutionRegistryAdapterCompletionStage{
		"runtime_action_service_ownership_completion":    daemonPlannerActionExecutionRegistryAdapterCompletionStageAdaptersComplete,
		"registry_handler_direct_manager_source_audit":   daemonPlannerActionExecutionRegistryAdapterCompletionStageSourceAuditClear,
		"planner_action_executor_next_fallback_boundary": daemonPlannerActionExecutionRegistryAdapterCompletionStageFallbackBoundaryChosen,
	}
	seenStages := map[string]daemonPlannerActionExecutionRegistryAdapterCompletionStage{}
	for _, entry := range daemonPlannerActionExecutionRegistryAdapterCompletionTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonPlannerActionExecutionNextFallbackRetirementBoundaryTurnExecutorCompatibility, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate registry adapter completion entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonPlannerActionExecutionRegistryAdapterCompletionEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}
	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonPlannerActionExecutionRegistryAdapterCompletionEntriesForStage(daemonPlannerActionExecutionRegistryAdapterCompletionStageAdaptersComplete), 1)
	require.Len(t, daemonPlannerActionExecutionRegistryAdapterCompletionEntriesForStage(daemonPlannerActionExecutionRegistryAdapterCompletionStageSourceAuditClear), 1)
	require.Len(t, daemonPlannerActionExecutionRegistryAdapterCompletionEntriesForStage(daemonPlannerActionExecutionRegistryAdapterCompletionStageFallbackBoundaryChosen), 1)
	require.Empty(t, daemonPlannerActionExecutionRegistryAdapterCompletionEntriesForStage("unsupported"))

	selectedBoundary, ok := daemonPlannerActionExecutionRegistryAdapterCompletionSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionNextFallbackRetirementBoundaryTurnExecutorCompatibility, selectedBoundary)

	for _, entry := range daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable {
		require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, entry.stage)
		require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, entry.currentOwner)
	}
	require.Len(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached), 8)
	require.Empty(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageSelectedNextCapability))
	require.Empty(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageDaemonManagerOwned))
	_, ok = daemonPlannerActionExecutionRuntimeActionServiceOwnershipSelectedNextCapability()
	require.False(t, ok)

	actionExecutor, ok := daemonPlannerActionExecutionFallbackBoundaryForStage(daemonPlannerActionExecutionFallbackStageActionExecutor)
	require.True(t, ok)
	require.Equal(t, "daemonAgentLoopActionExecutor.ExecuteAgentLoopAction", actionExecutor.legacyFallback)
	require.Contains(t, actionExecutor.scope, "TurnExecutor compatibility hop is retired")
	nextBoundary, ok := daemonPlannerActionExecutionRegistryAdapterCompletionEntryForComponent("planner_action_executor_next_fallback_boundary")
	require.True(t, ok)
	require.Contains(t, nextBoundary.nextStep, "P0.5-N212")
}

func TestDaemonPlannerActionExecutionTurnCompatibilityRetirementAuditSelectsDirectLegacyBoundary(t *testing.T) {
	data, err := os.ReadFile("agent_turn_executor.go")
	require.NoError(t, err)
	source := string(data)
	require.NotContains(t, source, "func (m *agentManager) runRuntimePlannerActionExecutionTurn")
	require.NotContains(t, source, "agentPlannerActionExecutionTurnResultPayload")
	require.NotContains(t, source, "agentPlannerActionExecutionTurnContextFromRequest")

	agentLoopBranch := regexp.MustCompile(`(?s)func \(m \*agentManager\) runAgentPlannerActionExecutionAgentLoopResult\(.*?\n}\n`)
	branch := agentLoopBranch.FindString(source)
	require.NotEmpty(t, branch)
	require.NotContains(t, branch, "runRuntimePlannerActionExecutionTurn")
	require.Contains(t, branch, "daemonAgentLoopActionExecutor{manager: m}.ExecuteAgentLoopAction")
	require.Contains(t, source, "func (e daemonAgentTurnExecutor) executePlannerActionExecutionTurn")

	wantStages := map[string]daemonPlannerActionExecutionTurnCompatibilityRetirementStage{
		"planner_action_execution_agent_loop_fallback":                  daemonPlannerActionExecutionTurnCompatibilityRetirementStageInternalFallbackRetired,
		"planner_action_execution_external_turn_executor_compatibility": daemonPlannerActionExecutionTurnCompatibilityRetirementStageExternalCompatibility,
		"planner_action_execution_next_fallback_boundary":               daemonPlannerActionExecutionTurnCompatibilityRetirementStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonPlannerActionExecutionTurnCompatibilityRetirementStage{}
	for _, entry := range daemonPlannerActionExecutionTurnCompatibilityRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonPlannerActionExecutionNextFallbackRetirementBoundaryDirectLegacyExecutor, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate turn compatibility retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonPlannerActionExecutionTurnCompatibilityRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}
	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonPlannerActionExecutionTurnCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnCompatibilityRetirementStageInternalFallbackRetired), 1)
	require.Len(t, daemonPlannerActionExecutionTurnCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnCompatibilityRetirementStageExternalCompatibility), 1)
	require.Len(t, daemonPlannerActionExecutionTurnCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnCompatibilityRetirementStageNextBoundaryChosen), 1)
	require.Empty(t, daemonPlannerActionExecutionTurnCompatibilityRetirementEntriesForStage("unsupported"))

	selectedBoundary, ok := daemonPlannerActionExecutionTurnCompatibilityRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionNextFallbackRetirementBoundaryDirectLegacyExecutor, selectedBoundary)

	actionExecutor, ok := daemonPlannerActionExecutionFallbackBoundaryForStage(daemonPlannerActionExecutionFallbackStageActionExecutor)
	require.True(t, ok)
	require.Equal(t, "daemonAgentLoopActionExecutor.ExecuteAgentLoopAction", actionExecutor.legacyFallback)
	require.Contains(t, actionExecutor.scope, "internal TurnExecutor compatibility hop is retired")
	require.Len(t, daemonPlannerActionExecutionFallbackBoundaryTable, 2)
	_, ok = daemonPlannerActionExecutionFallbackBoundaryForStage("turn_executor")
	require.False(t, ok)
}

func TestDaemonPlannerActionExecutionDirectLegacyRetirementAuditScopesExecutorEntrypoint(t *testing.T) {
	promptData, err := os.ReadFile("agent_prompt_dispatch.go")
	require.NoError(t, err)
	promptSource := string(promptData)
	require.NotContains(t, promptSource, "daemonPlannerActionExecutionExecutorForManager(")
	require.Contains(t, promptSource, "daemonAgentLoopActionExecutor{manager: m}.ExecuteAgentLoopAction")

	turnData, err := os.ReadFile("agent_turn_executor.go")
	require.NoError(t, err)
	turnSource := string(turnData)
	start := strings.Index(turnSource, "func (e daemonAgentTurnExecutor) executePlannerActionExecutionTurn")
	require.NotEqual(t, -1, start)
	end := strings.Index(turnSource[start:], "\nfunc (m *agentManager) runRuntimePlannerActionExecution")
	require.NotEqual(t, -1, end)
	plannerTurnBranch := turnSource[start : start+end]
	require.NotContains(t, plannerTurnBranch, "daemonPlannerActionExecutionExecutorForManager(")
	require.Contains(t, plannerTurnBranch, "daemonAgentLoopActionExecutor{manager: e.manager}.ExecuteAgentLoopAction")

	actionExecutorData, err := os.ReadFile("agent_action_executor.go")
	require.NoError(t, err)
	actionExecutorSource := string(actionExecutorData)
	require.Equal(t, 1, strings.Count(actionExecutorSource, "daemonPlannerActionExecutionExecutorForManager(executor.manager).ExecutePlannerAction"))
	require.Contains(t, actionExecutorSource, "func (executor daemonAgentLoopActionExecutor) ExecuteAgentLoopAction")

	wantStages := map[string]daemonPlannerActionExecutionDirectLegacyRetirementStage{
		"primary_agent_loop_runtime_action_executor":      daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionExecutorPrimary,
		"direct_legacy_executor_call_surface":             daemonPlannerActionExecutionDirectLegacyRetirementStageDirectExecutorSurfaceScoped,
		"registry_action_side_effect_ownership":           daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionServicesCovered,
		"no_engine_agent_loop_direct_fallback":            daemonPlannerActionExecutionDirectLegacyRetirementStageCompatibilityFallbackRetained,
		"planner_action_execution_next_fallback_boundary": daemonPlannerActionExecutionDirectLegacyRetirementStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonPlannerActionExecutionDirectLegacyRetirementStage{}
	for _, entry := range daemonPlannerActionExecutionDirectLegacyRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate direct legacy retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonPlannerActionExecutionDirectLegacyRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}
	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionExecutorPrimary), 1)
	require.Len(t, daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(daemonPlannerActionExecutionDirectLegacyRetirementStageDirectExecutorSurfaceScoped), 1)
	require.Len(t, daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionServicesCovered), 1)
	require.Len(t, daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(daemonPlannerActionExecutionDirectLegacyRetirementStageCompatibilityFallbackRetained), 1)
	require.Len(t, daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(daemonPlannerActionExecutionDirectLegacyRetirementStageNextBoundaryChosen), 1)
	require.Empty(t, daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage("unsupported"))
	require.True(t, daemonPlannerActionExecutionDirectLegacyRetirementReadyForSingleExecutorEntrypoint())

	selectedBoundary, ok := daemonPlannerActionExecutionDirectLegacyRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy, selectedBoundary)

	for _, entry := range daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable {
		require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached, entry.stage)
		require.Equal(t, daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability, entry.currentOwner)
	}
	require.Len(t, daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached), 8)

	fallback, ok := daemonPlannerActionExecutionDirectLegacyRetirementEntryForComponent("no_engine_agent_loop_direct_fallback")
	require.True(t, ok)
	require.Contains(t, fallback.evidence, "only when the runtime engine is missing")
	require.Contains(t, fallback.nextStep, "P0.5-N215")
}

func TestDaemonPlannerActionExecutionActionExecutorMissingRetirementAuditNarrowsFallback(t *testing.T) {
	wantStages := map[string]daemonPlannerActionExecutionActionExecutorMissingRetirementStage{
		"no_engine_agent_loop_direct_fallback":            daemonPlannerActionExecutionActionExecutorMissingRetirementStageNoEngineFallbackRetained,
		"attached_engine_missing_action_executor":         daemonPlannerActionExecutionActionExecutorMissingRetirementStageMissingExecutorBlocked,
		"detached_action_executor_result":                 daemonPlannerActionExecutionActionExecutorMissingRetirementStageDetachedResultBlocked,
		"planner_action_execution_next_fallback_boundary": daemonPlannerActionExecutionActionExecutorMissingRetirementStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonPlannerActionExecutionActionExecutorMissingRetirementStage{}
	for _, entry := range daemonPlannerActionExecutionActionExecutorMissingRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate missing action executor retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonPlannerActionExecutionActionExecutorMissingRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}
	require.Equal(t, wantStages, seenStages)
	require.Len(t, daemonPlannerActionExecutionActionExecutorMissingRetirementEntriesForStage(daemonPlannerActionExecutionActionExecutorMissingRetirementStageNoEngineFallbackRetained), 1)
	require.Len(t, daemonPlannerActionExecutionActionExecutorMissingRetirementEntriesForStage(daemonPlannerActionExecutionActionExecutorMissingRetirementStageMissingExecutorBlocked), 1)
	require.Len(t, daemonPlannerActionExecutionActionExecutorMissingRetirementEntriesForStage(daemonPlannerActionExecutionActionExecutorMissingRetirementStageDetachedResultBlocked), 1)
	require.Len(t, daemonPlannerActionExecutionActionExecutorMissingRetirementEntriesForStage(daemonPlannerActionExecutionActionExecutorMissingRetirementStageNextBoundaryChosen), 1)
	require.Empty(t, daemonPlannerActionExecutionActionExecutorMissingRetirementEntriesForStage("unsupported"))

	selectedBoundary, ok := daemonPlannerActionExecutionActionExecutorMissingRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy, selectedBoundary)

	require.True(t, daemonPlannerActionExecutionActionExecutorMissingRetirementAllowsDirectFallback(daemonPlannerActionExecutionFallbackEngineNotAttached))
	require.False(t, daemonPlannerActionExecutionActionExecutorMissingRetirementAllowsDirectFallback(daemonPlannerActionExecutionFallbackActionExecutorNotAttached))

	missing, ok := daemonPlannerActionExecutionActionExecutorMissingRetirementEntryForComponent("attached_engine_missing_action_executor")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionBlockedError, missing.disposition)
	require.Contains(t, missing.evidence, "runtime.ErrAgentLoopActionExecutorNotAttached")

	detached, ok := daemonPlannerActionExecutionActionExecutorMissingRetirementEntryForComponent("detached_action_executor_result")
	require.True(t, ok)
	require.Equal(t, daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionBlockedError, detached.disposition)
	require.Contains(t, detached.evidence, "no longer falls through")
}

func sortedStringKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint(t *testing.T) {
	want := map[string]daemonPlannerActionExecutorServiceContractMigrationEntry{
		"runtime_action_executor_seam": {
			component:      "runtime_action_executor_seam",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "runtime.AgentLoopActionExecutor / Engine.ExecuteAgentLoopAction",
			targetSurface:  "planner_action_execution dispatches one action through the runtime action executor contract",
			evidence:       "runtime executor request/result use AgentLoopRequest, AgentLoopAction, AgentLoopMessage and AgentLoopToolCall DTOs with clone isolation",
			nextStep:       "keep the seam stable while remaining typed service, authorization and result egress blockers move behind runtime contracts",
		},
		"runtime_action_service_capability_set": {
			component:      "runtime_action_service_capability_set",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "runtime.AgentLoopActionServiceSet names version/plugins/runs/approvals/jobs/agent/scan_manager/schedules/providers/workspace/network_request/port_scanner/web_search/web_scan/schedule_action/daemon_status_action/run_context_approval_action capabilities, and AgentLoopToolServiceBinding carries ActionServices as the default service surface",
			targetSurface:  "planner_action_execution resolves action services through runtime capability names instead of opaque Handle/provider fixtures",
			evidence:       "agentLoopToolServiceBindingFromRouterServices populates shared ActionServices without Handle, registry dispatch attaches workspace/network request/port scanner/web search/web scan/schedule/daemon status/run-context approval action capability adapters, and agentRegistryHandlerRuntime.actionServiceResolver reads the runtime capability set directly",
			nextStep:       "keep action-service capability consumers as the compatibility fixtures shrink",
		},
		"daemon_legacy_executor_adapter": {
			component:      "daemon_legacy_executor_adapter",
			stage:          daemonPlannerActionExecutorServiceContractStageDaemonAdapterAttached,
			currentSurface: "runRuntimePlannerActionExecution calls Engine.RunAgentLoop planner_action_execution mode; side-effect boundary carries runtime AgentLoopToolServiceBinding and daemon legacy executor forwards that binding directly to registry dispatch",
			targetSurface:  "planner_action_execution runner can execute through runtime-native action services without TurnExecutor/direct legacy fallbacks",
			evidence:       "primary caller dispatches an AgentLoopRequest carrying mode/action/toolServices; boundary clones the runtime binding and executeAgentPlannerActionResultWithToolServices preserves custom binding metadata through handler runtime",
			nextStep:       "retire TurnExecutor/direct legacy fallbacks after remaining compatibility callers move to AgentLoopRunner",
		},
		"runtime_action_result_composition": {
			component:      "runtime_action_result_composition",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "runtime.AgentLoopPlannerActionExecutionResultFromActionExecutorResult / FromTurnPayload",
			targetSurface:  "planner_action_execution runner result egress envelope is runtime-owned for action executor and TurnExecutor fallback paths",
			evidence:       "runner branch now delegates ActionExecutorResult/TurnPayload to runtime helpers that stamp mode/action/count/status/error metadata and clone DTOs",
			nextStep:       "continue shrinking daemon legacy result conversion before retiring direct legacy executor fallback",
		},
		"legacy_result_adapter_centralized": {
			component:      "legacy_result_adapter_centralized",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "daemonAgentLoopActionExecutor is the scoped compatibility entrypoint and daemonPlannerActionExecutionExecutor returns runtime.AgentLoopActionExecutorResult behind that adapter",
			targetSurface:  "planner_action_execution callers no longer receive daemon-private agentMessage/agentToolCall result types",
			evidence:       "daemon AgentLoop action executor is the only production caller of daemonPlannerActionExecutionExecutor; TurnExecutor compatibility and heuristic route enter through daemonAgentLoopActionExecutor.ExecuteAgentLoopAction",
			nextStep:       "keep direct executor access implementation-only while auditing missing ActionExecutor compatibility fallback",
		},
		"registry_handler_runtime_authorization_context": {
			component:      "registry_handler_runtime_authorization_context",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "agentRegistryHandlerRuntime carries runtime.AgentLoopAuthorization and http.request resolves credentials through agentResolveAgentLoopAuthProfileHeaders",
			targetSurface:  "registry handlers consume runtime authorization context instead of daemon HTTP auth-profile slices",
			evidence:       "focused http.request registry handler test passes runtime-only auth profile while legacy AuthProfiles contains a conflicting profile",
			nextStep:       "remove legacy registry runtime auth-profile compatibility fields once all handler/test callers consume Authorization only",
		},
		"tool_service_provider_contract_test_fixture": {
			component:      "tool_service_provider_contract_test_fixture",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "historical typed-provider contract table, explicit Handle fallback helpers and ActionServices-to-routerServices test roundtrip helpers are retired",
			targetSurface:  "runtime action executor receives runtime action services instead of daemon routerServices/provider bundles",
			evidence:       "daemonActionServiceSetFromRouterServices maps routerServices fields to runtime capability names; source-audit test rejects provider contract names and provider metadata in production daemon files",
			nextStep:       "keep tests asserting runtime ActionServices capabilities directly as the legacy compatibility fixture stays retired",
		},
		"registry_handler_action_service_first_batch": {
			component:      "registry_handler_action_service_first_batch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "daemon.health, plugins.list and runs.list call daemonStatusActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceDaemonStatusAction without Handle fallback; the daemon adapter receives the runtime resolver so VersionService/PluginService/RunService still load through ActionServices",
			targetSurface:  "daemon.health/plugins.list/runs.list consume runtime daemon status action service capability without direct rt.Manager.handleAgentHealth/Plugins/Runs calls",
			evidence:       "focused daemon status registry handler test uses ActionServices with a fake daemon status capability and no Manager, while source audit rejects direct status manager calls in agent_registry_handlers.go",
			nextStep:       "keep daemon status capability adapter covered while registry action-service adapter completion is audited",
		},
		"registry_handler_action_service_schedule_batch": {
			component:      "registry_handler_action_service_schedule_batch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "schedule.create, schedule.list and schedule.delete call scheduleActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceScheduleAction without Handle fallback; the daemon adapter receives the runtime resolver so ScheduleManager reads/writes still load through ActionServices",
			targetSurface:  "schedule.* consumes runtime schedule action service capability without direct rt.Manager.handleAgentSchedule* calls",
			evidence:       "focused schedule registry handler test uses ActionServices with a fake schedule action capability and no Manager, while source audit rejects direct schedule manager calls in agent_registry_handlers.go",
			nextStep:       "keep schedule action capability adapter covered while registry action-service adapter completion is audited",
		},
		"registry_handler_action_service_web_search_batch": {
			component:      "registry_handler_action_service_web_search_batch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "web.search calls webSearchActionServiceFromRegistryRuntime() and resolves runtime AgentLoopActionServiceWebSearch without Handle fallback; the daemon adapter receives the runtime resolver so ProviderStore config still loads through ActionServices",
			targetSurface:  "web.search consumes runtime web search action service capability without direct rt.Manager.handleAgentWebSearch calls",
			evidence:       "focused web search registry handler test uses ActionServices with a fake web search capability and no Manager, while source audit rejects direct web search manager calls in agent_registry_handlers.go",
			nextStep:       "keep web search capability adapter covered while registry action-service adapter completion is audited",
		},
		"registry_handler_action_service_scan_batch": {
			component:      "registry_handler_action_service_scan_batch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "scan.web.safe calls webScanActionServiceFromRegistryRuntime() and resolves runtime AgentLoopActionServiceWebScan without Handle fallback; the daemon adapter receives the runtime resolver so JobManager scan creation still loads through ActionServices",
			targetSurface:  "scan.web.safe consumes runtime web scan action service capability without direct rt.Manager.handleAgentScan calls",
			evidence:       "focused web scan registry handler test uses ActionServices with a fake web scan capability and no Manager, while source audit rejects direct scan manager calls in agent_registry_handlers.go",
			nextStep:       "keep web scan capability adapter covered while registry action-service adapter completion is audited",
		},
		"registry_handler_action_service_run_context_approval_batch": {
			component:      "registry_handler_action_service_run_context_approval_batch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "runs.context, reports.draft, runs.approvals.list, runs.resume_approved and runs.approvals.approve/deny call runContextApprovalActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceRunContextApproval without Handle fallback; the daemon adapter receives the runtime resolver so RunService/ApprovalService reads/writes still load through ActionServices",
			targetSurface:  "run-context/approval registry handlers consume runtime run-context approval action service capability without direct rt.Manager.handleAgentRunContext*/handleAgentReportDraft/handleAgentApprovals/handleAgentResumeApproved/handleAgentApprovalDecision calls",
			evidence:       "focused run-context/approval registry handler test uses ActionServices with a fake run-context approval capability and no Manager, while source audit rejects direct run-context/approval manager calls in agent_registry_handlers.go",
			nextStep:       "audit registry action-service adapter completion before selecting the next planner action executor fallback retirement boundary",
		},
		"registry_handler_action_service_workspace_batch": {
			component:      "registry_handler_action_service_workspace_batch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "workspace.command, workspace.files.* and workspace.search call workspaceActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceWorkspace without Handle fallback",
			targetSurface:  "workspace registry handlers consume runtime workspace action service capability without direct rt.Manager.handleAgentCommand/handleAgentWorkspace* calls",
			evidence:       "focused workspace registry handler test uses ActionServices with a fake workspace capability and no Manager, while source audit rejects direct workspace manager calls in agent_registry_handlers.go",
			nextStep:       "keep workspace capability adapter covered while network request and port-scan side-effect groups are migrated",
		},
		"registry_handler_action_service_network_request_batch": {
			component:      "registry_handler_action_service_network_request_batch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "http.request and web.fetch call networkRequestActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceNetworkRequest without Handle fallback",
			targetSurface:  "network request registry handlers consume runtime network request action service capability without direct rt.Manager.handleAgentHTTPRequest/handleAgentWebFetch calls",
			evidence:       "focused network request registry handler test uses ActionServices with a fake network request capability and no Manager, while source audit rejects direct network manager calls in agent_registry_handlers.go",
			nextStep:       "keep network request capability adapter covered while the port-scan side-effect group is migrated",
		},
		"registry_handler_action_service_port_scan_batch": {
			component:      "registry_handler_action_service_port_scan_batch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "ports.scan.safe calls portScannerActionServiceFromRegistryRuntime() and resolves runtime AgentLoopActionServicePortScanner without Handle fallback",
			targetSurface:  "port scan registry handler consumes runtime port scanner action service capability without direct rt.Manager.handleAgentPortScan calls",
			evidence:       "focused port scanner registry handler test uses ActionServices with a fake port scanner capability and no Manager, while source audit rejects direct port scan manager calls in agent_registry_handlers.go",
			nextStep:       "keep port scanner capability adapter covered while remaining registry runtime capability consumers are evaluated for narrower adapters",
		},
		"authorization_context": {
			component:      "authorization_context",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "daemonPlannerActionExecutionSideEffectBoundary, executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices pass runtime.AgentLoopAuthorization end-to-end",
			targetSurface:  "runtime action executor consumes AgentLoopAuthorization or a typed runtime auth resolver directly",
			evidence:       "agentResolveAgentLoopAuthProfileHeaders consumes runtime profiles directly; executor and registry dispatch signatures accept AgentLoopAuthorization instead of []agentHTTPAuthProfile/default profile fields",
			nextStep:       "remove legacy registry runtime auth-profile compatibility fields after handler compatibility coverage is retired",
		},
		"registry_handler_service_consumers": {
			component:      "registry_handler_service_consumers",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "all builtin registry handlers call agentRegistryHandlerRuntime.actionServiceResolver(); production resolver consumes runtime ActionServices only",
			targetSurface:  "registry handlers consume runtime action service capabilities without opaque Handle fallback",
			evidence:       "focused tests cover first/schedule/web.search/scan/run-context/approval/workspace/network request/port scanner runtime capability batches with Handle fallbacks that panic on provider accessors, plus web.search adapter dispatch with no Manager",
			nextStep:       "keep provider/routerServices reconstruction retired from production and test runtime paths",
		},
		"registry_handler_runtime_fallback_surface": {
			component:      "registry_handler_runtime_fallback_surface",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "agentRegistryHandlerRuntime production surface exposes actionServiceResolver()/authorizationContext() only; toolServiceProvider()/legacyRouterServices() registry runtime helpers have been removed",
			targetSurface:  "builtin registry handlers cannot call provider/routerServices reconstruction helpers directly",
			evidence:       "source-audit test scans production registry dispatch/handler files and rejects agentRegistryHandlerRuntime toolServiceProvider()/legacyRouterServices() helper definitions or calls",
			nextStep:       "keep production registry runtime and tests free of provider/routerServices reconstruction helpers",
		},
		"tool_service_binding_handle_default_surface": {
			component:      "tool_service_binding_handle_default_surface",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "agentLoopToolServiceBindingFromRouterServices emits ActionServices-only bindings with nil Handle; explicit Handle reconstruction support has been retired from binding roundtrip helpers",
			targetSurface:  "primary daemon callers do not rely on AgentLoopToolServiceBinding.Handle to carry service providers",
			evidence:       "binding tests assert nil Handle and direct ActionServices capabilities while Handle-only bindings are rejected by registry runtime and do not expose runtime capabilities",
			nextStep:       "keep direct ActionServices capability assertions on loop runner and turn executor boundaries",
		},
		"runtime_tool_service_direct_dispatch": {
			component:      "runtime_tool_service_direct_dispatch",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "executeAgentPlannerActionResultWithToolServices, executeAgentRegistryHandlerResultWithToolServices, executeAgentPlannerActionsWithToolServices, routePromptWithToolServices, routePromptForTurnWithToolServices, routePromptWithRuntimeDeterministicRouteWithToolServices, ControlSessionWithToolServices, DecideAgentAuthorizationWithToolServices, SendMessageWithToolServices, runAgentSendMessageTurnWithToolServices, handleAgentActiveRunContinueForTurnWithToolServices, agentLLMToolLoopRequestFromLegacyWithToolServices, agentReplanActionContinuationRequestFromLegacyWithToolServices and agentPlannerActionReplanCycleToolContextFromToolServices preserve runtime AgentLoopToolServiceBinding across planner action, deterministic/heuristic route, control/auth/send turn, active-run continue and LLM/replan request dispatch",
			targetSurface:  "production planner action, deterministic route, control/auth continuation, send-message lifecycle and send-turn paths dispatch downstream runtime helpers without reconstructing daemon routerServices from runtime bindings",
			evidence:       "focused tests assert custom runtime binding metadata survives through daemon legacy executor and deterministic route AgentLoop paths; source audit rejects retired legacy routerServices ingress wrappers and legacyRouterServicesFromAgentLoopToolServiceBinding calls in production dispatch files",
			nextStep:       "keep direct runtime ActionServices capability checks covering loop runner and turn executor boundaries",
		},
		"registry_runtime_action_services_only": {
			component:      "registry_runtime_action_services_only",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "agentRegistryHandlerRuntime.actionServiceResolver delegates to an ActionServices-only resolver and no longer unwraps AgentLoopToolServiceBinding.Handle",
			targetSurface:  "production registry runtime cannot recover daemon provider/routerServices through opaque Handle fallback",
			evidence:       "focused registry runtime test asserts Handle-only binding is rejected by actionServiceResolver and direct capability checks show no runtime ActionServices capability is present",
			nextStep:       "keep Handle/provider/routerServices fallback retired",
		},
		"tool_service_binding_provider_reconstruction_test_only": {
			component:      "tool_service_binding_provider_reconstruction_test_only",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "provider/routerServices reconstruction helpers for AgentLoopToolServiceBinding are retired from both production and test runtime paths",
			targetSurface:  "production runtime files cannot reconstruct daemon provider/routerServices from runtime tool service bindings",
			evidence:       "source-audit test scans all non-test daemon files and rejects routerServicesFromAgentLoopToolServiceBinding, agentLoopActionServiceResolverFromAgentLoopToolServiceBinding, provider-from-Handle and provider/routerServices reconstruction helper definitions; focused tests assert ActionServices capabilities directly",
			nextStep:       "keep helper-free test coverage while future callers migrate away from daemon-private routerServices entirely",
		},
		"legacy_result_egress": {
			component:      "legacy_result_egress",
			stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
			currentSurface: "executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices produce runtime.AgentLoopActionExecutorResult directly for all current callers; legacy routerServices-ingress wrappers are retired",
			targetSurface:  "action executor implementation returns runtime result DTOs without daemon-private egress types",
			evidence:       "registryHandlerOutputToActionExecutorResult builds runtime AgentLoopMessage/AgentLoopToolCall DTOs directly and no longer calls registryHandlerOutputToMessages; legacy handler output types are adapted as compatibility inputs",
			nextStep:       "replace remaining daemon-private tool-service dispatch with runtime-native action service interfaces",
		},
	}
	seen := map[string]daemonPlannerActionExecutorServiceContractMigrationEntry{}

	for _, entry := range daemonPlannerActionExecutorServiceContractMigrationTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.currentSurface)
		require.NotEmpty(t, entry.targetSurface)
		require.NotEmpty(t, entry.evidence)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seen[entry.component].component, "duplicate planner action executor service contract entry for %q", entry.component)
		seen[entry.component] = entry
	}

	require.Equal(t, want, seen)
	require.True(t, daemonPlannerActionExecutorServiceContractHasRuntimeSeam())
	require.False(t, daemonPlannerActionExecutorServiceContractReadyForAgentLoop())
	require.Len(t, daemonPlannerActionExecutorServiceContractMigrationEntriesForStage(daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded), 22)
	require.Empty(t, daemonPlannerActionExecutorServiceContractMigrationEntriesForStage(daemonPlannerActionExecutorServiceContractStageDaemonAdapterPending))
	require.Len(t, daemonPlannerActionExecutorServiceContractMigrationEntriesForStage(daemonPlannerActionExecutorServiceContractStageDaemonAdapterAttached), 1)
	require.Empty(t, daemonPlannerActionExecutorServiceContractMigrationEntriesForStage(daemonPlannerActionExecutorServiceContractStageBlockedByLegacyBinding))
	require.Empty(t, daemonPlannerActionExecutorServiceContractMigrationEntriesForStage("unsupported"))
}

func TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes(t *testing.T) {
	wantModes := map[runtime.AgentLoopMode]bool{
		runtime.AgentLoopModeDeterministicRoute:       true,
		runtime.AgentLoopModeLLMToolLoop:              true,
		runtime.AgentLoopModeReplanContinuation:       true,
		runtime.AgentLoopModePlannerActionReplanCycle: true,
		runtime.AgentLoopModePlannerActionExecution:   true,
	}
	seenModes := map[runtime.AgentLoopMode]bool{}

	for _, entry := range daemonAgentLoopRunnerCompletionAuditTable {
		require.NotEmpty(t, entry.mode)
		require.NotEmpty(t, entry.runnerBranch)
		require.NotEmpty(t, entry.appendPersistOwner)
		require.NotEmpty(t, entry.toolSynthesisOwner)
		require.NotEmpty(t, entry.actionExecutionOwner)
		require.NotEmpty(t, entry.fallbackOwner)
		require.NotEmpty(t, entry.currentSurface)
		require.NotEmpty(t, entry.nextRuntimeSeam)
		require.False(t, seenModes[entry.mode], "duplicate loop runner completion audit entry for %q", entry.mode)
		seenModes[entry.mode] = true
	}

	require.Equal(t, wantModes, seenModes)
	require.Equal(t, daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate, daemonAgentLoopRunnerCompletionAuditNextRuntimeSeam())
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamSharedCycleAppendPersist))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamToolSynthesisFallback))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamAgentLoopDirectFallback))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamActionExecutionFallback))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamActionServiceOwnership))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamNestedReplanCycleDispatch))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamSessionMutationFallback))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamAttachedSessionMutator))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamSessionMutationBlocked))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamSessionMutationOptionalResult))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamSessionMutationNoEngineFallback))
	require.Len(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate), len(wantModes))
	require.Empty(t, daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam("unsupported"))

	cycle, ok := daemonAgentLoopRunnerCompletionAuditEntryForMode(runtime.AgentLoopModePlannerActionReplanCycle)
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback, cycle.appendPersistOwner)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback, cycle.toolSynthesisOwner)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback, cycle.actionExecutionOwner)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerDaemonAdapter, cycle.fallbackOwner)
	require.Equal(t, daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate, cycle.nextRuntimeSeam)
	require.Contains(t, cycle.currentSurface, "runRuntimeAgentLoopResult")
	require.Contains(t, cycle.currentSurface, "SessionMutation")

	action, ok := daemonAgentLoopRunnerCompletionAuditEntryForMode(runtime.AgentLoopModePlannerActionExecution)
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerNotApplicable, action.appendPersistOwner)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback, action.actionExecutionOwner)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerDaemonAdapter, action.fallbackOwner)

	deterministic, ok := daemonAgentLoopRunnerCompletionAuditEntryForMode(runtime.AgentLoopModeDeterministicRoute)
	require.True(t, ok)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerDaemonAdapter, deterministic.actionExecutionOwner)
	require.Equal(t, daemonAgentLoopRunnerCompletionOwnerDaemonAdapterViaRuntimeDTO, deterministic.appendPersistOwner)

	_, ok = daemonAgentLoopRunnerCompletionAuditEntryForMode("unsupported")
	require.False(t, ok)
}

func TestDaemonTurnExecutionCallerAuditTablesStayAligned(t *testing.T) {
	wantKinds := map[string]bool{
		daemonTurnExecutionDeterministicRoute:     true,
		daemonTurnExecutionPlannerActionExecution: true,
	}
	invalidPayloadKinds := map[string]bool{}
	policyKinds := map[string]bool{}
	migrationKinds := map[string]bool{}
	helperInternalKinds := map[string]bool{}
	helperCompatibilityKinds := map[string]bool{}

	for _, entry := range daemonTurnExecutionCallerInvalidPayloadTable {
		invalidPayloadKinds[entry.kind] = true
	}
	for _, entry := range daemonTurnExecutionCallerPolicyTable {
		policyKinds[entry.kind] = true
	}
	for _, entry := range daemonTurnExecutionCallerMigrationTable {
		migrationKinds[entry.kind] = true
	}
	for _, entry := range daemonTurnExecutionHelperOwnershipTable {
		if entry.callerSurface == "internal_execute_turn_caller" {
			helperInternalKinds[entry.kind] = true
		}
		if entry.callerSurface == "internal_agent_loop_caller" {
			helperInternalKinds[entry.kind] = true
		}
		if entry.callerSurface == "external_compatibility_trampoline" {
			helperCompatibilityKinds[entry.kind] = true
		}
	}

	require.Equal(t, wantKinds, invalidPayloadKinds)
	require.Equal(t, wantKinds, policyKinds)
	require.Equal(t, wantKinds, migrationKinds)
	require.Equal(t, wantKinds, helperInternalKinds)
	require.Equal(t, map[string]bool{daemonTurnExecutionReplanActionContinuation: true}, helperCompatibilityKinds)
	require.False(t, invalidPayloadKinds[daemonTurnExecutionReplanActionContinuation])
	require.False(t, policyKinds[daemonTurnExecutionReplanActionContinuation])
	require.False(t, migrationKinds[daemonTurnExecutionReplanActionContinuation])
}

func attachRecordingSessionMutator(t *testing.T, eng *runtime.Engine, mgr *agentManager, records *[]runtime.SessionMutationRequest) {
	t.Helper()
	legacy := daemonSessionMutator{manager: mgr}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		if req.Kind == runtime.SessionMutationAppendMessages {
			*records = append(*records, req)
		}
		return legacy.MutateSession(ctx, req)
	}))
}

func agentMessageIDs(messages []agentMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func agentLoopMessageIDs(messages []runtime.AgentLoopMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func TestAgentManagerDeterministicRoutePlanOnlyAppendUsesTurnRef(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	turnRef := agentRuntimeTurnRef{
		TaskID: "task-deterministic-plan-only",
		RunID:  "run-deterministic-plan-only",
		TurnID: "turn-deterministic-plan-only",
	}
	messages, toolCalls, err := mgr.routePromptWithRuntimeDeterministicRouteWithToolServices(
		context.Background(),
		sessionID,
		turnRef,
		"run ffuf directory enumeration on https://example.com",
		"en",
		"operator-1",
		"safe_scan",
		testAgentLoopToolServices(),
	)

	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Contains(t, messages[0].Content, "content discovery")
	require.Empty(t, toolCalls)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, turnRef.TaskID, appendReq.Turn.TaskID)
	require.Equal(t, turnRef.RunID, appendReq.Turn.RunID)
	require.Equal(t, turnRef.TurnID, appendReq.Turn.TurnID)
	require.Equal(t, "run ffuf directory enumeration on https://example.com", appendReq.Turn.Prompt)
	require.Equal(t, daemonTurnExecutionDeterministicRoute, appendReq.Turn.Origin)
	require.Equal(t, "safe_scan", appendReq.Turn.OperationLevel)
	require.Equal(t, 0, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, true, appendReq.Turn.Metadata["planner_apply"])

	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.NotNil(t, payload.Plan)
	require.Empty(t, payload.Messages)
}

func TestAgentManagerControlSessionJobCancelAppendUsesExternalTurnRef(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 13, 0, 0, 0, time.UTC)
	runID := "run-external-control"
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:          "task-external-control",
		Goal:        "cancel a running daemon job",
		Kind:        "scan",
		Status:      "running",
		ActiveRunID: runID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	mgr.mu.Unlock()

	jobID := "job-external-control"
	cancelled := false
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	jobs.mu.Lock()
	jobs.jobs[jobID] = &daemonJob{
		JobID:     jobID,
		Kind:      scanWebJobKind,
		Status:    contracts.StatusRunning,
		RunID:     runID,
		CreatedAt: now,
		StartedAt: timePtr(now),
		cancel: func() {
			cancelled = true
		},
		inFlight: true,
	}
	jobs.running = 1
	jobs.mu.Unlock()

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	_, err = mgr.ControlSessionWithToolServices(context.Background(), sessionID, agentControlRequest{
		Action:         "cancel",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "safe_scan",
		Prompt:         "cancel the running daemon job",
		JobID:          jobID,
	}, agentLoopToolServiceBindingFromRouterServices(routerServices{jobs: jobs}))

	require.NoError(t, err)
	require.True(t, cancelled)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, "task-external-control", appendReq.Turn.TaskID)
	require.Equal(t, runID, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "cancel the running daemon job", appendReq.Turn.Prompt)
	require.Equal(t, string(runtime.SessionMutationControlSession), appendReq.Turn.Origin)
	require.Equal(t, "safe_scan", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])

	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, "tool", payload.Messages[0].Role)
	require.Len(t, payload.Messages[0].ToolCalls, 1)
	require.Equal(t, "POST /api/v1/jobs/{job_id}/cancel", payload.Messages[0].ToolCalls[0].Name)
}

func TestAgentManagerDecideAgentAuthorizationApprovedActionAppendUsesExternalTurnRef(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 13, 30, 0, 0, time.UTC)
	runID := "run-external-authorization"
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.ActiveTask = &agentTask{
		ID:          "task-external-authorization",
		Goal:        "approve a pending daemon action",
		Kind:        "tool",
		Status:      "blocked",
		ActiveRunID: runID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-external-authorization",
		ActionName:     "daemon.health",
		Arguments:      map[string]any{"probe": "external-authorization"},
		RequiredLevel:  "read_only",
		Policy:         "manual",
		Reason:         "operator approval required",
		OriginalPrompt: "run daemon health after approval",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mgr.mu.Unlock()

	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok, "daemon.health missing from registry")
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()
	spec.Handler = func(_ context.Context, in toolregistry.HandlerInput) (any, error) {
		require.Equal(t, sessionID, in.SessionID)
		require.Equal(t, "operator-1", in.Operator)
		require.Equal(t, "en", in.Language)
		require.Equal(t, "read_only", in.OperationLevel)
		require.Equal(t, "external-authorization", stringFromAny(in.RawArgs["probe"]))
		tool := runningAgentToolCall("daemon.health", "authorization")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"approved_action": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "authorized action executed", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	resp, err := mgr.DecideAgentAuthorizationWithToolServices(context.Background(), sessionID, agentAuthorizationDecisionRequest{
		Action:          "approve",
		AuthorizationID: "auth-external-authorization",
		Operator:        "operator-1",
		Language:        "en",
		OperationLevel:  "read_only",
		SkipSynthesis:   true,
	}, agentLoopToolServiceBindingFromRouterServices(routerServices{}))

	require.NoError(t, err)
	require.NotNil(t, resp.ApprovalState)
	require.Equal(t, approvalCanonicalStateApproved, resp.ApprovalState.State)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, "task-external-authorization", appendReq.Turn.TaskID)
	require.Equal(t, runID, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "run daemon health after approval", appendReq.Turn.Prompt)
	require.Equal(t, daemonTurnExecutionPlannerActionExecution, appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])

	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, "authorized action executed", payload.Messages[0].Content)
	require.Equal(t, "step", payload.Messages[0].Phase)
	require.Len(t, payload.Messages[0].ToolCalls, 1)
	require.Equal(t, "daemon.health", payload.Messages[0].ToolCalls[0].Name)
	metadata := payload.Messages[0].ToolCalls[0].Metadata
	require.Equal(t, "auth-external-authorization", metadata[runtime.AgentLoopAuthorizationIDMetadataKey])
	require.Equal(t, "approved", metadata[runtime.AgentLoopAuthorizationStatusMetadataKey])
	require.Equal(t, "daemon.health", metadata[runtime.AgentLoopActionNameMetadataKey])
}

func TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	runID := "run-external-authorization-flow"
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.ActiveTask = &agentTask{
		ID:          "task-external-authorization-flow",
		Goal:        "approve, synthesize, and continue a pending daemon action",
		Kind:        "tool",
		Status:      "blocked",
		ActiveRunID: runID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-external-authorization-flow",
		ActionName:     "daemon.health",
		Arguments:      map[string]any{"probe": "external-authorization-flow"},
		RequiredLevel:  "read_only",
		Policy:         "manual",
		Reason:         "operator approval required",
		OriginalPrompt: "run daemon health and summarize after approval",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mgr.mu.Unlock()

	actionTool := runningAgentToolCall("daemon.health", "authorization-flow")
	actionTool = finishAgentToolCall(actionTool, nil)
	actionTool.Result = map[string]any{"approved_action": true}
	actionMessage := mgr.message("en", "tool", "authorized action executed through runtime runner", []agentToolCall{actionTool}, nil)
	synthesisTool := runningAgentToolCall("LLM synthesis", "authorization-flow")
	synthesisTool = finishAgentToolCall(synthesisTool, nil)
	synthesisTool.Result = map[string]any{"synthesized": true}
	synthesisMessage := agentMessageWithPhase(mgr.message("en", "agent", "authorized action synthesized through runtime runner", []agentToolCall{synthesisTool}, nil), "step_replan")
	synthesisPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
	}

	turnExecutions := []runtime.TurnContext{}
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		kind, _ := turn.Metadata["kind"].(string)
		switch kind {
		case daemonTurnExecutionPlannerActionExecution:
			turnExecutions = append(turnExecutions, turn)
			payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
			require.True(t, ok, "payload type = %T", turn.Payload)
			require.Equal(t, "daemon.health", payload.Action.Name)
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "approved action handled by test executor",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{actionTool}),
				},
			}, nil
		default:
			return runtime.TurnResult{Status: runtime.TurnResultRunning}, nil
		}
	}))

	llmStages := []runtime.LLMStageRequest{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		llmStages = append(llmStages, req)
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Len(t, payload.ToolMessages, 1)
		require.Equal(t, actionMessage.ID, payload.ToolMessages[0].ID)
		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "synthesis handled by test runner",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, []agentToolCall{synthesisTool}, synthesisPlan),
		}, nil
	}))

	loopExecutions := []runtime.AgentLoopRequest{}
	baseRunner := daemonAgentLoopRunner{manager: mgr}
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(ctx context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		loopExecutions = append(loopExecutions, req)
		switch req.Mode {
		case runtime.AgentLoopModePlannerActionExecution:
			require.Equal(t, daemonTurnExecutionPlannerActionExecution, req.Turn.Origin)
			require.Equal(t, "daemon.health", req.Action.Name)
			return baseRunner.RunAgentLoop(ctx, req)
		case runtime.AgentLoopModeReplanContinuation:
			require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(synthesisPlan), req.InitialPlan)
			return runtime.AgentLoopResult{}, nil
		default:
			t.Fatalf("unexpected loop mode %s", req.Mode)
			return runtime.AgentLoopResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	resp, err := mgr.DecideAgentAuthorizationWithToolServices(context.Background(), sessionID, agentAuthorizationDecisionRequest{
		Action:          "approve",
		AuthorizationID: "auth-external-authorization-flow",
		Operator:        "operator-1",
		Language:        "en",
		OperationLevel:  "read_only",
		Provider:        "ollama",
	}, agentLoopToolServiceBindingFromRouterServices(routerServices{}))

	require.NoError(t, err)
	require.NotNil(t, resp.ApprovalState)
	require.Equal(t, approvalCanonicalStateApproved, resp.ApprovalState.State)

	expectedTurnID := "turn_auth-external-authorization-flow"
	require.Len(t, turnExecutions, 1)
	require.Len(t, loopExecutions, 2)
	require.Len(t, llmStages, 1)
	require.Len(t, appendMutations, 2)
	for _, turn := range turnExecutions {
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, "task-external-authorization-flow", turn.TaskID)
		require.Equal(t, runID, turn.RunID)
		require.Equal(t, expectedTurnID, turn.TurnID)
		require.Equal(t, "run daemon health and summarize after approval", turn.Prompt)
		require.Equal(t, "read_only", turn.OperationLevel)
	}
	require.Equal(t, runtime.AgentLoopModePlannerActionExecution, loopExecutions[0].Mode)
	require.Equal(t, runtime.AgentLoopModeReplanContinuation, loopExecutions[1].Mode)
	for _, loopReq := range loopExecutions {
		require.Equal(t, sessionID, loopReq.Turn.SessionID)
		require.Equal(t, "task-external-authorization-flow", loopReq.Turn.TaskID)
		require.Equal(t, runID, loopReq.Turn.RunID)
		require.Equal(t, expectedTurnID, loopReq.Turn.TurnID)
		require.Equal(t, "run daemon health and summarize after approval", loopReq.Turn.Prompt)
		require.Equal(t, "read_only", loopReq.Turn.OperationLevel)
		switch loopReq.Mode {
		case runtime.AgentLoopModePlannerActionExecution:
			require.Equal(t, daemonTurnExecutionPlannerActionExecution, loopReq.Turn.Origin)
			require.Equal(t, "daemon.health", loopReq.Action.Name)
		case runtime.AgentLoopModeReplanContinuation:
			require.Equal(t, daemonTurnExecutionReplanActionContinuation, loopReq.Turn.Origin)
			require.Equal(t, daemonTurnExecutionReplanActionContinuation, loopReq.Turn.Metadata["kind"])
		default:
			t.Fatalf("unexpected recorded loop mode %s", loopReq.Mode)
		}
	}
	require.Equal(t, daemonTurnExecutionPlannerActionExecution, turnExecutions[0].Metadata["kind"])

	require.Equal(t, sessionID, llmStages[0].Turn.SessionID)
	require.Equal(t, "task-external-authorization-flow", llmStages[0].Turn.TaskID)
	require.Equal(t, runID, llmStages[0].Turn.RunID)
	require.Equal(t, expectedTurnID, llmStages[0].Turn.TurnID)
	require.Equal(t, "run daemon health and summarize after approval", llmStages[0].Turn.Prompt)
	require.Equal(t, "read_only", llmStages[0].Turn.OperationLevel)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), llmStages[0].Metadata["stage"])

	require.Equal(t, daemonTurnExecutionPlannerActionExecution, appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)
	for _, appendReq := range appendMutations {
		require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
		require.Equal(t, sessionID, appendReq.Turn.SessionID)
		require.Equal(t, "task-external-authorization-flow", appendReq.Turn.TaskID)
		require.Equal(t, runID, appendReq.Turn.RunID)
		require.Equal(t, expectedTurnID, appendReq.Turn.TurnID)
		require.Equal(t, "run daemon health and summarize after approval", appendReq.Turn.Prompt)
		require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	}
}

func TestAgentManagerRunRuntimeDeterministicRouteDispatchesAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-route", Role: "tool", Content: "routed"}
	expectedToolCall := agentToolCall{ID: "tool-route", Name: "daemon.health"}
	called := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, payload runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		called = true
		require.Equal(t, runtime.AgentLoopModeDeterministicRoute, payload.Mode)
		require.Equal(t, "session-route-executor", payload.Turn.SessionID)
		require.Equal(t, "task-route-executor", payload.Turn.TaskID)
		require.Equal(t, "run-route-executor", payload.Turn.RunID)
		require.Equal(t, "turn-route-executor", payload.Turn.TurnID)
		require.Equal(t, "daemon health", payload.Turn.Prompt)
		require.Equal(t, daemonTurnExecutionDeterministicRoute, payload.Turn.Origin)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Equal(t, "turn_execution", payload.Turn.Metadata["executor"])
		require.Equal(t, daemonTurnExecutionDeterministicRoute, payload.Turn.Metadata["kind"])

		return runtime.AgentLoopResult{
			Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
			ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
		}, nil
	}))

	result, handled, err := mgr.runRuntimeDeterministicRouteWithToolServices(
		context.Background(),
		"session-route-executor",
		agentRuntimeTurnRef{TaskID: "task-route-executor", RunID: "run-route-executor", TurnID: "turn-route-executor"},
		"daemon health",
		"en",
		"operator-1",
		"read_only",
		testAgentLoopToolServices(),
	)

	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, called)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), result.Messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), result.ToolCalls)
}

func TestAgentManagerTurnExecutionCallerInvalidPayloadAudit(t *testing.T) {
	t.Run("deterministic route compatibility request payload", func(t *testing.T) {
		mgr := newAgentManager(t.TempDir())
		result, err := daemonAgentTurnExecutor{manager: mgr}.ExecuteTurn(nil, runtime.TurnContext{
			SessionID:      "session-route-invalid-payload",
			TaskID:         "task-route-invalid-payload",
			RunID:          "run-route-invalid-payload",
			TurnID:         "turn-route-invalid-payload",
			Prompt:         "daemon health",
			Origin:         daemonTurnExecutionDeterministicRoute,
			OperationLevel: "read_only",
			Payload:        "not a request",
			Metadata: map[string]any{
				"kind": daemonTurnExecutionDeterministicRoute,
			},
		})

		require.ErrorContains(t, err, "daemon: invalid deterministic route turn payload")
		require.Equal(t, runtime.TurnResultFailed, result.Status)
		require.Equal(t, daemonTurnExecutionDeterministicRoute, result.Data["kind"])
		require.Equal(t, "session-route-invalid-payload", result.Data["session_id"])
	})

	t.Run("planner action execution compatibility request payload", func(t *testing.T) {
		mgr := newAgentManager(t.TempDir())
		result, err := daemonAgentTurnExecutor{manager: mgr}.ExecuteTurn(nil, runtime.TurnContext{
			SessionID:      "session-action-invalid-payload",
			TaskID:         "task-action-invalid-payload",
			RunID:          "run-action-invalid-payload",
			TurnID:         "turn-action-invalid-payload",
			Prompt:         "daemon health",
			Origin:         daemonTurnExecutionPlannerActionExecution,
			OperationLevel: "read_only",
			Payload:        "not an action execution payload",
			Metadata: map[string]any{
				"kind": daemonTurnExecutionPlannerActionExecution,
			},
		})

		require.ErrorContains(t, err, "daemon: invalid planner action execution turn payload")
		require.Equal(t, runtime.TurnResultFailed, result.Status)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, result.Data["kind"])
		require.Equal(t, "session-action-invalid-payload", result.Data["session_id"])
	})
}

func TestAgentManagerSendMessageNoLLMRouteUsesAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)
	expectedMessage := agentMessage{ID: "msg-send-route", Role: "tool", Content: "routed through executor"}
	expectedToolCall := agentToolCall{ID: "tool-send-route", Name: "daemon.health"}

	called := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, payload runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		called = true
		require.Equal(t, runtime.AgentLoopModeDeterministicRoute, payload.Mode)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.NotEmpty(t, payload.Turn.TaskID)
		require.NotEmpty(t, payload.Turn.RunID)
		require.NotEmpty(t, payload.Turn.TurnID)
		require.Equal(t, "daemon health", payload.Turn.Prompt)
		require.Equal(t, daemonTurnExecutionDeterministicRoute, payload.Turn.Origin)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Equal(t, "operator-1", payload.Operator)

		return runtime.AgentLoopResult{
			Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
			ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
		}, nil
	}))

	resp, err := mgr.SendMessageWithToolServices(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        "daemon health",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "read_only",
	}, agentLoopToolServiceBindingFromRouterServices(routerServices{}))

	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, resp.Messages, 2)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, expectedMessage, resp.Messages[1])
	require.Equal(t, []agentToolCall{expectedToolCall}, resp.ToolCalls)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedMessage.ID, got.Messages[len(got.Messages)-1].ID)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.NotEmpty(t, appendReq.Turn.TaskID)
	require.NotEmpty(t, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "daemon health", appendReq.Turn.Prompt)
	require.Equal(t, daemonTurnExecutionDeterministicRoute, appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])
	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, expectedMessage.ID, payload.Messages[0].ID)
}

func TestAgentManagerSendMessageActiveRunContinueUsesAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	runID := "run_active_continue"
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:          "task-active-run-continue",
		Goal:        "track active run context",
		Kind:        "scan",
		Status:      "running",
		ActiveRunID: runID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.NextAction = &agentNextAction{
		Type:            "continue",
		Reason:          "tracking run",
		SuggestedPrompt: "continue tracking run context",
	}
	mgr.mu.Unlock()

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)
	expectedToolCall := agentToolCall{
		ID:     "tool-active-run-context",
		Name:   "runs.context",
		Status: contracts.StatusSucceeded,
		Result: map[string]any{
			"run_id":  runID,
			"context": "events",
		},
	}
	expectedMessage := agentMessage{
		ID:        "msg-active-run-context",
		Role:      "tool",
		Content:   "active run context routed through executor",
		ToolCalls: []agentToolCall{expectedToolCall},
	}

	routeCalled := false
	runtimeRunID := ""
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, payload runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		routeCalled = true
		require.Equal(t, runtime.AgentLoopModeDeterministicRoute, payload.Mode)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.Equal(t, "task-active-run-continue", payload.Turn.TaskID)
		require.NotEmpty(t, payload.Turn.RunID)
		require.NotEmpty(t, payload.Turn.TurnID)
		runtimeRunID = payload.Turn.RunID
		require.Contains(t, payload.Turn.Prompt, "读取运行 "+runID)
		require.Contains(t, payload.Turn.Prompt, "events")
		require.Equal(t, daemonTurnExecutionDeterministicRoute, payload.Turn.Origin)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Equal(t, "turn_execution", payload.Turn.Metadata["executor"])
		require.Equal(t, daemonTurnExecutionDeterministicRoute, payload.Turn.Metadata["kind"])
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)

		return runtime.AgentLoopResult{
			Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
			ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
		}, nil
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        "继续",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "read_only",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, routeCalled)
	require.Len(t, resp.Messages, 3)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, "agent", resp.Messages[1].Role)
	require.Equal(t, expectedMessage, resp.Messages[2])
	require.Equal(t, []agentToolCall{expectedToolCall}, resp.ToolCalls)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedMessage.ID, got.Messages[len(got.Messages)-1].ID)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, "task-active-run-continue", appendReq.Turn.TaskID)
	require.Equal(t, runtimeRunID, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "继续", appendReq.Turn.Prompt)
	require.Equal(t, "active_run_continue", appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])
	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, expectedMessage.ID, payload.Messages[0].ID)
}

func TestAgentManagerSendMessageLLMFallbackRouteUsesAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	llmErr := errors.New("provider down")
	llmCalled := false
	expectedMessage := agentMessage{ID: "msg-llm-fallback-route", Role: "agent", Content: "fallback routed through executor"}
	expectedToolCall := agentToolCall{ID: "tool-llm-fallback-route", Name: "daemon.health"}
	routeCalled := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "hello fallback route", req.Turn.Prompt)
		switch req.Mode {
		case runtime.AgentLoopModeLLMToolLoop:
			llmCalled = true
			return runtime.AgentLoopResult{StopReason: llmErr.Error()}, llmErr
		case runtime.AgentLoopModeDeterministicRoute:
			routeCalled = true
			require.NotEmpty(t, req.Turn.TaskID)
			require.NotEmpty(t, req.Turn.RunID)
			require.NotEmpty(t, req.Turn.TurnID)
			require.Equal(t, daemonTurnExecutionDeterministicRoute, req.Turn.Origin)
			require.Equal(t, "read_only", req.Turn.OperationLevel)
			require.Equal(t, "operator-1", req.Operator)
			return runtime.AgentLoopResult{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
			}, nil
		default:
			t.Fatalf("unexpected loop mode %s", req.Mode)
		}
		return runtime.AgentLoopResult{}, nil
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        "hello fallback route",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "read_only",
		Provider:       "openai",
		APIKey:         "test-key",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, llmCalled)
	require.True(t, routeCalled)
	require.Len(t, resp.Messages, 3)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, "tool", resp.Messages[1].Role)
	require.Contains(t, resp.Messages[1].Content, "provider down")
	require.Equal(t, expectedMessage, resp.Messages[2])
	require.Len(t, resp.ToolCalls, 2)
	require.Equal(t, "LLM chat", resp.ToolCalls[0].Name)
	require.Equal(t, expectedToolCall, resp.ToolCalls[1])

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedMessage.ID, got.Messages[len(got.Messages)-1].ID)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.NotEmpty(t, appendReq.Turn.TaskID)
	require.NotEmpty(t, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "hello fallback route", appendReq.Turn.Prompt)
	require.Equal(t, "llm_fallback_route", appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 2, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])
	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 2)
	require.Contains(t, payload.Messages[0].Content, "provider down")
	require.Equal(t, expectedMessage.ID, payload.Messages[1].ID)
}

func TestAgentManagerSendMessageLLMFallbackAuthorizationUsesSessionMutations(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "workspace_write",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	prompt := "write file notes.md content: hello from fallback authorization"

	var authMutations []runtime.SessionMutationRequest
	var appendMutations []runtime.SessionMutationRequest
	legacy := daemonSessionMutator{manager: mgr}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		switch req.Kind {
		case runtime.SessionMutationRequestAuthorization:
			authMutations = append(authMutations, req)
		case runtime.SessionMutationAppendMessages:
			appendMutations = append(appendMutations, req)
		}
		return legacy.MutateSession(ctx, req)
	}))

	llmErr := errors.New("provider down")
	llmCalled := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		llmCalled = true
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, prompt, req.Turn.Prompt)
		require.NotEmpty(t, req.Turn.TaskID)
		require.NotEmpty(t, req.Turn.RunID)
		require.NotEmpty(t, req.Turn.TurnID)
		require.Equal(t, "workspace_write", req.Turn.OperationLevel)
		return runtime.AgentLoopResult{StopReason: llmErr.Error()}, llmErr
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        prompt,
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "workspace_write",
		Provider:       "openai",
		APIKey:         "test-key",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, llmCalled)
	require.Len(t, resp.Messages, 3)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, prompt, resp.Messages[0].Content)
	require.Equal(t, "tool", resp.Messages[1].Role)
	require.Contains(t, resp.Messages[1].Content, "provider down")
	require.Equal(t, "agent", resp.Messages[2].Role)
	require.Equal(t, "step", resp.Messages[2].Phase)
	require.Contains(t, resp.Messages[2].Content, "Authorization required")
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "LLM chat", resp.ToolCalls[0].Name)

	require.Len(t, authMutations, 1)
	authReq := authMutations[0]
	require.Equal(t, runtime.SessionMutationRequestAuthorization, authReq.Kind)
	require.Equal(t, sessionID, authReq.Turn.SessionID)
	require.NotEmpty(t, authReq.Turn.TaskID)
	require.NotEmpty(t, authReq.Turn.RunID)
	require.NotEmpty(t, authReq.Turn.TurnID)
	require.Equal(t, prompt, authReq.Turn.Prompt)
	require.Equal(t, string(runtime.SessionMutationRequestAuthorization), authReq.Turn.Origin)
	require.Equal(t, "workspace_write", authReq.Turn.OperationLevel)
	require.Equal(t, string(runtime.SessionMutationRequestAuthorization), authReq.Metadata["kind"])
	require.Equal(t, "workspace.files.write", authReq.Metadata["action_name"])
	require.Equal(t, "workspace_write", authReq.Metadata["required_level"])
	require.Equal(t, "manual_tool_authorization_required", authReq.Metadata["policy"])

	authPayload, ok := authReq.Payload.(runtime.AgentLoopAuthorizationRequestPayload)
	require.True(t, ok, "payload type = %T", authReq.Payload)
	require.Equal(t, "en", authPayload.Language)
	require.Equal(t, "workspace.files.write", authPayload.ActionName)
	require.Equal(t, "workspace_write", authPayload.RequiredLevel)
	require.Equal(t, "manual_tool_authorization_required", authPayload.Policy)
	require.Equal(t, "workspace.files.write", authPayload.Action.Name)
	require.Equal(t, "notes.md", authPayload.Action.Arguments["path"])
	require.Equal(t, "hello from fallback authorization", authPayload.Action.Arguments["content"])

	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, authReq.Turn.TaskID, appendReq.Turn.TaskID)
	require.Equal(t, authReq.Turn.RunID, appendReq.Turn.RunID)
	require.Equal(t, authReq.Turn.TurnID, appendReq.Turn.TurnID)
	require.Equal(t, prompt, appendReq.Turn.Prompt)
	require.Equal(t, "llm_fallback_authorization", appendReq.Turn.Origin)
	require.Equal(t, "workspace_write", appendReq.Turn.OperationLevel)
	require.Equal(t, 2, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])

	appendPayload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, appendPayload.Messages, 2)
	require.Contains(t, appendPayload.Messages[0].Content, "provider down")
	require.Contains(t, appendPayload.Messages[1].Content, "Authorization required")
	require.Equal(t, resp.Messages[1].ID, appendPayload.Messages[0].ID)
	require.Equal(t, resp.Messages[2].ID, appendPayload.Messages[1].ID)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "waiting_authorization", got.Status)
	require.NotNil(t, got.PendingAuthorization)
	require.Equal(t, "workspace.files.write", got.PendingAuthorization.ActionName)
	require.Equal(t, "workspace_write", got.PendingAuthorization.RequiredLevel)
	require.Equal(t, "manual_tool_authorization_required", got.PendingAuthorization.Policy)
	require.Equal(t, prompt, got.PendingAuthorization.OriginalPrompt)
	require.Equal(t, "pending", got.PendingAuthorization.Status)
	require.NotNil(t, got.NextAction)
	require.Equal(t, "authorization_required", got.NextAction.Type)
}

func TestAgentManagerSendMessageRouteBeforeLLMUsesAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)
	prompt := "列出所有定时任务"
	require.True(t, agentShouldRouteBeforeLLMSynthesis(prompt))

	llmCalled := false
	expectedMessage := agentMessage{ID: "msg-route-before-llm", Role: "tool", Content: "route-before-llm through runner"}
	routeCalled := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, payload runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		switch payload.Mode {
		case runtime.AgentLoopModeDeterministicRoute:
			routeCalled = true
			require.Equal(t, sessionID, payload.Turn.SessionID)
			require.NotEmpty(t, payload.Turn.TaskID)
			require.NotEmpty(t, payload.Turn.RunID)
			require.NotEmpty(t, payload.Turn.TurnID)
			require.Equal(t, prompt, payload.Turn.Prompt)
			require.Equal(t, daemonTurnExecutionDeterministicRoute, payload.Turn.Origin)
			require.Equal(t, "read_only", payload.Turn.OperationLevel)
			require.Equal(t, "operator-1", payload.Operator)
			return runtime.AgentLoopResult{
				Messages: agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
			}, nil
		case runtime.AgentLoopModeLLMToolLoop:
			llmCalled = true
			return runtime.AgentLoopResult{StopReason: "llm should not run"}, errors.New("llm should not run")
		default:
			t.Fatalf("unexpected loop mode %s", payload.Mode)
		}
		return runtime.AgentLoopResult{}, nil
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        prompt,
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "read_only",
		Provider:       "openai",
		APIKey:         "test-key",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, routeCalled)
	require.False(t, llmCalled)
	require.Len(t, resp.Messages, 2)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, "step", resp.Messages[1].Phase)
	require.Equal(t, expectedMessage.ID, resp.Messages[1].ID)
	require.Equal(t, expectedMessage.Role, resp.Messages[1].Role)
	require.Equal(t, expectedMessage.Content, resp.Messages[1].Content)
	require.Empty(t, resp.ToolCalls)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedMessage.ID, got.Messages[len(got.Messages)-1].ID)
	require.Equal(t, "step", got.Messages[len(got.Messages)-1].Phase)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.NotEmpty(t, appendReq.Turn.TaskID)
	require.NotEmpty(t, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, prompt, appendReq.Turn.Prompt)
	require.Equal(t, daemonTurnExecutionDeterministicRoute, appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])
	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, expectedMessage.ID, payload.Messages[0].ID)
	require.Equal(t, "step", payload.Messages[0].Phase)
}

func TestAgentManagerRunRuntimeLLMToolLoopAgentLoopDispatchesAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-llm", Role: "agent", Content: "ok"}
	expectedToolCall := agentToolCall{ID: "tool-llm", Name: "LLM chat"}
	lastMessageAt := time.Date(2026, 6, 7, 12, 20, 0, 0, time.UTC)
	enabled := true
	authProfiles := []agentHTTPAuthProfile{{
		ID:      "auth-llm-runner",
		Token:   "secret-token",
		Enabled: &enabled,
	}}
	history := []agentMessage{{ID: "history-1", Role: "user", Content: "hello"}}
	memorySummary := &agentMemorySummary{Summary: "memory", LastMessageAt: &lastMessageAt}
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	called := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, payload runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		called = true
		require.Equal(t, "session-llm-runner", payload.Turn.SessionID)
		require.Equal(t, "task-llm-runner", payload.Turn.TaskID)
		require.Equal(t, "run-llm-runner", payload.Turn.RunID)
		require.Equal(t, "turn-llm-runner", payload.Turn.TurnID)
		require.Equal(t, "plan safely", payload.Turn.Prompt)
		require.Equal(t, "llm_tool_loop", payload.Turn.Metadata["runner"])
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Equal(t, runtime.AgentLoopModeLLMToolLoop, payload.Mode)
		require.Equal(t, 3, payload.MaxLoops)
		require.Equal(t, "planner_loop", payload.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
		require.Equal(t, true, payload.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])
		require.Equal(t, "gpt-test", payload.LLMRequest.Model)
		require.Equal(t, "secret-token", payload.LLMRequest.AuthProfiles[0].Token)
		require.NotNil(t, payload.LLMRequest.AuthProfiles[0].Enabled)
		require.True(t, *payload.LLMRequest.AuthProfiles[0].Enabled)
		require.Len(t, payload.History, 1)
		require.Equal(t, "hello", payload.History[0].Content)
		require.Len(t, payload.State.LoopHistory, 1)
		require.Equal(t, "hello", payload.State.LoopHistory[0].Content)
		require.Equal(t, "memory", payload.MemorySummary.Summary)
		require.NotNil(t, payload.MemorySummary.LastMessageAt)
		require.Equal(t, lastMessageAt, *payload.MemorySummary.LastMessageAt)
		payloadJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](payload.ToolServices.ActionServices, runtime.AgentLoopActionServiceJobs)
		require.True(t, ok)
		require.Same(t, jobs, payloadJobs)

		payload.History[0].Content = "mutated payload history"
		payload.Turn.Metadata["runner"] = "mutated payload runner"
		payload.MemorySummary.Summary = "mutated payload memory"
		*payload.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)
		payload.LLMRequest.AuthProfiles[0].Token = "mutated-token"
		*payload.LLMRequest.AuthProfiles[0].Enabled = false

		return runtime.AgentLoopResult{
			Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
			ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
		}, nil
	}))

	loopReq := agentLLMToolLoopRequestFromLegacyWithToolServices(
		"session-llm-runner",
		agentRuntimeTurnRef{TaskID: "task-llm-runner", RunID: "run-llm-runner", TurnID: "turn-llm-runner"},
		"plan safely",
		"zh-CN",
		"operator-1",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-test", AuthProfiles: authProfiles},
		history,
		memorySummary,
		agentLoopToolServiceBindingFromRouterServices(routerServices{jobs: jobs}),
	)
	messages, toolCalls, err := mgr.runRuntimeLLMToolLoopAgentLoop(context.Background(), loopReq)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
	require.Equal(t, "hello", history[0].Content)
	require.Equal(t, "memory", memorySummary.Summary)
	require.Equal(t, lastMessageAt, *memorySummary.LastMessageAt)
	require.Equal(t, "secret-token", authProfiles[0].Token)
	require.True(t, *authProfiles[0].Enabled)
}

func TestAgentManagerRunRuntimeAgentLLMPlannerChatDispatchesEngineStageRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-planner", Role: "agent", Content: "planned"}
	expectedToolCall := agentToolCall{ID: "tool-planner", Name: "LLM chat"}
	expectedPlan := &agentPlannerResponse{Phase: "plan", Answer: "planned"}
	called := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		called = true
		require.Equal(t, runtime.LLMStagePlannerChat, req.Stage)
		require.Equal(t, "session-planner-stage", req.Turn.SessionID)
		require.Equal(t, "task-planner-stage", req.Turn.TaskID)
		require.Equal(t, "run-planner-stage", req.Turn.RunID)
		require.Equal(t, "turn-planner-stage", req.Turn.TurnID)
		require.Equal(t, "make a plan", req.Turn.Prompt)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "llm_stage", req.Metadata["runner"])
		require.Equal(t, string(runtime.LLMStagePlannerChat), req.Metadata["stage"])

		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-planner-stage", payload.Turn.SessionID)
		require.Equal(t, "task-planner-stage", payload.Turn.TaskID)
		require.Equal(t, "run-planner-stage", payload.Turn.RunID)
		require.Equal(t, "turn-planner-stage", payload.Turn.TurnID)
		require.Equal(t, "make a plan", payload.Turn.Prompt)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "safe_scan", payload.Turn.OperationLevel)
		require.Equal(t, "gpt-planner", payload.LLMRequest.Model)
		require.Len(t, payload.History, 1)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "done",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan),
		}, nil
	}))

	messages, toolCalls, plan, err := mgr.runRuntimeAgentLLMPlannerChat(
		context.Background(),
		"session-planner-stage",
		agentRuntimeTurnRef{TaskID: "task-planner-stage", RunID: "run-planner-stage", TurnID: "turn-planner-stage"},
		"make a plan",
		"zh-CN",
		"safe_scan",
		sendAgentMessageRequest{Model: "gpt-planner"},
		[]agentMessage{{ID: "history-1", Role: "user", Content: "hello"}},
		&agentMemorySummary{Summary: "memory"},
	)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
	require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), plan)
}

func TestAgentManagerRunRuntimeAgentLLMToolSynthesisDispatchesEngineStageRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-synthesis", Role: "agent", Content: "summarized"}
	expectedToolCall := agentToolCall{ID: "tool-synthesis", Name: "LLM synthesis"}
	expectedPlan := &agentPlannerResponse{Phase: "step_replan", Answer: "summarized"}
	called := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		called = true
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		require.Equal(t, "session-synthesis-stage", req.Turn.SessionID)
		require.Equal(t, "task-synthesis-stage", req.Turn.TaskID)
		require.Equal(t, "run-synthesis-stage", req.Turn.RunID)
		require.Equal(t, "turn-synthesis-stage", req.Turn.TurnID)
		require.Equal(t, "summarize tools", req.Turn.Prompt)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "llm_stage", req.Metadata["runner"])
		require.Equal(t, string(runtime.LLMStageToolSynthesis), req.Metadata["stage"])

		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-synthesis-stage", payload.Turn.SessionID)
		require.Equal(t, "task-synthesis-stage", payload.Turn.TaskID)
		require.Equal(t, "run-synthesis-stage", payload.Turn.RunID)
		require.Equal(t, "turn-synthesis-stage", payload.Turn.TurnID)
		require.Equal(t, "summarize tools", payload.Turn.Prompt)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "safe_scan", payload.Turn.OperationLevel)
		require.Equal(t, "gpt-synthesis", payload.LLMRequest.Model)
		require.Len(t, payload.History, 1)
		require.Len(t, payload.ToolMessages, 1)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "done",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan),
		}, nil
	}))

	messages, toolCalls, plan, err := mgr.runRuntimeAgentLLMToolSynthesis(
		context.Background(),
		"session-synthesis-stage",
		agentRuntimeTurnRef{TaskID: "task-synthesis-stage", RunID: "run-synthesis-stage", TurnID: "turn-synthesis-stage"},
		"summarize tools",
		"zh-CN",
		"safe_scan",
		sendAgentMessageRequest{Model: "gpt-synthesis"},
		[]agentMessage{{ID: "history-1", Role: "user", Content: "hello"}},
		&agentMemorySummary{Summary: "memory"},
		[]runtime.AgentLoopMessage{{ID: "tool-message-1", Role: "tool", Content: "observation"}},
	)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
	require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), plan)
}

func TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	lastMessageAt := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)
	req := agentPlannerActionReplanCycleRequest{
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-cycle-synthesis",
			agentRuntimeTurnRef{
				TaskID: "task-cycle-synthesis",
				RunID:  "run-cycle-synthesis",
				TurnID: "turn-cycle-synthesis",
			},
			"summarize cycle tools",
			"user",
			"safe_scan",
		),
		Language: "zh-CN",
		LLMRequest: runtime.AgentLoopLLMRequest{
			Origin:         "user",
			OperationLevel: "safe_scan",
			Model:          "gpt-cycle-synthesis",
		},
		MemorySummary: agentPlannerActionReplanCycleMemorySummaryFromAgentMemorySummary(&agentMemorySummary{
			Summary:       "cycle memory",
			MessageCount:  3,
			LastMessageID: "msg-cycle-synthesis-action",
			LastMessageAt: &lastMessageAt,
		}),
	}
	state := agentPlannerActionReplanCycleStateFromLegacy(
		nil,
		nil,
		[]agentMessage{
			{ID: "history-cycle-synthesis-user", Role: "user", Content: "operator asked"},
			{ID: "msg-cycle-synthesis-action", Role: "tool", Content: "action observation"},
		},
	)
	actionMessages := agentPlannerActionReplanCycleMessagesFromAgentMessages([]agentMessage{{
		ID:      "msg-cycle-synthesis-action",
		Role:    "tool",
		Content: "action observation",
		ToolCalls: []agentToolCall{{
			ID:       "tool-cycle-synthesis-action",
			Name:     "daemon.health",
			Metadata: map[string]interface{}{"phase": "step"},
		}},
	}})

	synthesisMessage := agentMessage{ID: "msg-cycle-synthesis-summary", Role: "agent", Content: "cycle summarized"}
	synthesisTool := agentToolCall{ID: "tool-cycle-synthesis-summary", Name: "LLM synthesis", Result: map[string]any{"ok": true}}
	synthesisPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "next"},
		}},
	}

	called := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, stageReq runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		called = true
		require.Equal(t, runtime.LLMStageToolSynthesis, stageReq.Stage)
		require.Equal(t, "session-cycle-synthesis", stageReq.Turn.SessionID)
		require.Equal(t, "task-cycle-synthesis", stageReq.Turn.TaskID)
		require.Equal(t, "run-cycle-synthesis", stageReq.Turn.RunID)
		require.Equal(t, "turn-cycle-synthesis", stageReq.Turn.TurnID)
		require.Equal(t, "summarize cycle tools", stageReq.Turn.Prompt)
		require.Equal(t, "safe_scan", stageReq.Turn.OperationLevel)

		payload, ok := stageReq.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", stageReq.Payload)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "safe_scan", payload.Turn.OperationLevel)
		require.Equal(t, "gpt-cycle-synthesis", payload.LLMRequest.Model)
		require.Equal(t, "cycle memory", payload.MemorySummary.Summary)
		require.NotNil(t, payload.MemorySummary.LastMessageAt)
		require.Equal(t, lastMessageAt, *payload.MemorySummary.LastMessageAt)
		require.Equal(t, []string{"history-cycle-synthesis-user", "msg-cycle-synthesis-action"}, agentLoopMessageIDs(payload.History))
		require.Equal(t, []string{"msg-cycle-synthesis-action"}, agentLoopMessageIDs(payload.ToolMessages))

		payload.History[0].Content = "mutated payload history"
		payload.ToolMessages[0].Content = "mutated payload tool message"
		payload.MemorySummary.Summary = "mutated payload memory"
		*payload.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "cycle synthesis handled",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, []agentToolCall{synthesisTool}, synthesisPlan),
		}, nil
	}))

	loopReq := req.agentLoopRequest(runtime.AgentLoopModeReplanContinuation)
	result, err := mgr.runAgentPlannerActionReplanCycleSynthesisAgentLoop(context.Background(), loopReq, state.state, actionMessages.agentLoopMessages())

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "operator asked", state.legacyLoopHistory()[0].Content)
	require.Equal(t, "action observation", actionMessages.legacyAgentMessages()[0].Content)
	require.Equal(t, "cycle memory", req.MemorySummary.legacyAgentMemorySummary().Summary)
	require.Equal(t, lastMessageAt, *req.MemorySummary.legacyAgentMemorySummary().LastMessageAt)

	gotMessages := agentMessagesFromAgentLoopMessages(result.Messages)
	gotToolCalls := agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls)
	require.Equal(t, []agentMessage{synthesisMessage}, gotMessages)
	require.Equal(t, []agentToolCall{synthesisTool}, gotToolCalls)
	resultPlan := agentPlannerActionReplanCyclePlanFromAgentLoopPlan(result.FinalPlan)
	require.True(t, resultPlan.hasActions())
	require.True(t, resultPlan.shouldAutoContinue())
	require.Equal(t, "next", resultPlan.actions()[0].Arguments["probe"])

	synthesisMessage.Content = "mutated returned message source"
	synthesisTool.Result["ok"] = false
	synthesisPlan.Actions[0].Arguments["probe"] = "mutated-next"
	synthesisPlan.NextAction.AutoContinue = false
	gotMessages[0].Content = "mutated got message"
	gotToolCalls[0].Result["ok"] = "mutated"

	require.Equal(t, "cycle summarized", agentMessagesFromAgentLoopMessages(result.Messages)[0].Content)
	require.Equal(t, true, agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls)[0].Result["ok"])
	resultPlan = agentPlannerActionReplanCyclePlanFromAgentLoopPlan(result.FinalPlan)
	require.True(t, resultPlan.shouldAutoContinue())
	require.Equal(t, "next", resultPlan.actions()[0].Arguments["probe"])
}

func TestAgentManagerRunRuntimeReplanActionContinuationAgentLoopDispatchesAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-replan-continuation", Role: "agent", Content: "continued through executor"}
	expectedToolCall := agentToolCall{ID: "tool-replan-continuation", Name: "daemon.health"}
	expectedPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		ToolCalls: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "continuation"},
		}},
	}
	called := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, payload runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		called = true
		require.Equal(t, runtime.AgentLoopModeReplanContinuation, payload.Mode)
		require.Equal(t, "session-replan-continuation", payload.Turn.SessionID)
		require.Equal(t, "task-replan-continuation", payload.Turn.TaskID)
		require.Equal(t, "run-replan-continuation", payload.Turn.RunID)
		require.Equal(t, "turn-replan-continuation", payload.Turn.TurnID)
		require.Equal(t, "continue daemon health", payload.Turn.Prompt)
		require.Equal(t, daemonTurnExecutionReplanActionContinuation, payload.Turn.Origin)
		require.Equal(t, "turn_execution", payload.Turn.Metadata["executor"])
		require.Equal(t, daemonTurnExecutionReplanActionContinuation, payload.Turn.Metadata["kind"])
		require.Equal(t, 2, payload.Turn.Metadata["max_loops"])
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "tester", payload.Operator)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Equal(t, "gpt-continuation", payload.LLMRequest.Model)
		require.Len(t, payload.History, 1)
		require.Equal(t, "memory", payload.MemorySummary.Summary)
		require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), payload.InitialPlan)
		require.Equal(t, 2, payload.MaxLoops)

		return runtime.AgentLoopResult{
			Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
			ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
		}, nil
	}))

	loopReq := agentReplanActionContinuationRequestFromLegacyWithToolServices(
		"session-replan-continuation",
		agentRuntimeTurnRef{TaskID: "task-replan-continuation", RunID: "run-replan-continuation", TurnID: "turn-replan-continuation"},
		"continue daemon health",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-continuation"},
		[]agentMessage{{ID: "history-1", Role: "agent", Content: "previous step"}},
		&agentMemorySummary{Summary: "memory"},
		testAgentLoopToolServices(),
		expectedPlan,
		2,
	)
	messages, toolCalls := mgr.runRuntimeReplanActionContinuationAgentLoop(context.Background(), loopReq)

	require.True(t, called)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
}

func TestDaemonAgentTurnExecutorReplanContinuationReturnsEgressPayload(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	loopReq := runtime.AgentLoopRequest{
		Mode: runtime.AgentLoopModeReplanContinuation,
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-continuation-egress",
			agentRuntimeTurnRef{TaskID: "task-continuation-egress", RunID: "run-continuation-egress", TurnID: "turn-continuation-egress"},
			"continue",
			"user",
			"read_only",
		),
		Language: "en",
		Operator: "tester",
		InitialPlan: agentLoopPlanFromPlannerResponse(&agentPlannerResponse{
			Phase: "step_replan",
			NextAction: &agentNextAction{
				Type:         "continue",
				AutoContinue: true,
			},
			Actions: []agentPlannerAction{{Name: "daemon.health"}},
		}),
		MaxLoops: 0,
	}

	result, err := daemonAgentTurnExecutor{manager: mgr}.ExecuteTurn(context.Background(), agentReplanActionContinuationTurnContextFromRequest(loopReq))

	require.NoError(t, err)
	require.Equal(t, runtime.TurnResultFinished, result.Status)
	payload, ok := result.Payload.(runtime.AgentLoopTurnResultPayload)
	require.True(t, ok, "payload type = %T", result.Payload)
	require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, payload.StopReason)
	require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, payload.Metadata[runtime.AgentLoopEgressMetadataStopReason])
	require.Equal(t, 0, payload.LoopCount)
	require.Equal(t, 0, payload.MaxLoops)
	require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, result.Data["stop_reason"])
	require.Equal(t, 0, result.Data["loop_count"])
	require.Equal(t, 0, result.Data["max_loops"])
	require.Equal(t, 0, result.Data["message_count"])
	require.Equal(t, 0, result.Data["tool_call_count"])
}

func TestAgentManagerRunRuntimePlannerActionExecutionDispatchesActionExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := runtime.AgentLoopMessage{ID: "msg-planner-action-executor", Role: "tool", Content: "executed through action executor"}
	expectedToolCall := runtime.AgentLoopToolCall{ID: "tool-planner-action-executor", Name: "daemon.health"}
	authProfiles := []agentHTTPAuthProfile{{
		ID:      "auth-action-executor",
		Type:    "bearer",
		BaseURL: "https://sandbox.example",
		Token:   "token",
	}}
	action := agentPlannerAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "runtime-action-executor"},
	}
	runtimeAction := agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action)
	actionCalled := false
	turnCalled := false
	eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(ctx context.Context, req runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
		actionCalled = true
		require.NotNil(t, ctx)
		require.Equal(t, "session-planner-action-executor", req.Request.Turn.SessionID)
		require.Equal(t, "task-planner-action-executor", req.Request.Turn.TaskID)
		require.Equal(t, "run-planner-action-executor", req.Request.Turn.RunID)
		require.Equal(t, "turn-planner-action-executor", req.Request.Turn.TurnID)
		require.Equal(t, "approved daemon health", req.Request.Turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, req.Request.Turn.Origin)
		require.Equal(t, "safe_scan", req.Request.Turn.OperationLevel)
		require.Equal(t, "en", req.Request.Language)
		require.Equal(t, "tester", req.Request.Operator)
		require.Equal(t, runtimeAction, req.Action)
		require.Equal(t, agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles), req.Request.Authorization.AuthProfiles)
		require.Equal(t, "auth-action-executor", req.Request.Authorization.DefaultAuthProfileID)

		return runtime.AgentLoopActionExecutorResult{
			Status:    runtime.TurnResultFinished,
			Reason:    "done",
			Messages:  []runtime.AgentLoopMessage{expectedMessage},
			ToolCalls: []runtime.AgentLoopToolCall{expectedToolCall},
		}, nil
	}))
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, _ runtime.TurnContext) (runtime.TurnResult, error) {
		turnCalled = true
		return runtime.TurnResult{Status: runtime.TurnResultFailed}, errors.New("turn executor should not be called when action executor is attached")
	}))

	messages, toolCalls, err := mgr.runRuntimePlannerActionExecution(
		context.Background(),
		"session-planner-action-executor",
		agentRuntimeTurnRef{TaskID: "task-planner-action-executor", RunID: "run-planner-action-executor", TurnID: "turn-planner-action-executor"},
		"approved daemon health",
		"en",
		"tester",
		"safe_scan",
		agentLoopToolServiceBindingFromRouterServices(routerServices{}),
		runtimeAction,
		runtime.AgentLoopAuthorization{
			AuthProfiles:         agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles),
			DefaultAuthProfileID: "auth-action-executor",
		},
	)

	require.NoError(t, err)
	require.True(t, actionCalled)
	require.False(t, turnCalled)
	require.Equal(t, []runtime.AgentLoopMessage{expectedMessage}, messages)
	require.Equal(t, []runtime.AgentLoopToolCall{expectedToolCall}, toolCalls)
}

func TestAgentManagerRunRuntimePlannerActionExecutionActionExecutorFallbackBoundary(t *testing.T) {
	loopReq := agentPlannerActionExecutionRequestFromRuntime(
		"session-action-executor-fallback",
		agentRuntimeTurnRef{TaskID: "task-action-executor-fallback", RunID: "run-action-executor-fallback", TurnID: "turn-action-executor-fallback"},
		"daemon health",
		"en",
		"tester",
		"read_only",
		runtime.AgentLoopToolServiceBinding{},
		runtime.AgentLoopAuthorization{},
	)
	action := runtime.AgentLoopAction{Name: "daemon.health"}

	t.Run("nil_engine", func(t *testing.T) {
		mgr := newAgentManager(t.TempDir())
		result, handled, err := mgr.runRuntimePlannerActionExecutionActionExecutor(context.Background(), loopReq, action)
		require.NoError(t, err)
		require.False(t, handled)
		require.Empty(t, result.Messages)
	})

	t.Run("missing_executor", func(t *testing.T) {
		eng := runtime.NewEngine(runtime.NewMemStore())
		mgr := newAgentManager(t.TempDir())
		mgr.AttachEngine(eng)
		eng.AttachAgentLoopActionExecutor(nil)

		result, handled, err := mgr.runRuntimePlannerActionExecutionActionExecutor(context.Background(), loopReq, action)
		require.ErrorIs(t, err, runtime.ErrAgentLoopActionExecutorNotAttached)
		require.True(t, handled)
		require.Equal(t, runtime.TurnResultBlocked, result.Status)
		require.ErrorIs(t, result.Error, runtime.ErrAgentLoopActionExecutorNotAttached)
		require.Equal(t, "retired", result.Metadata["planner_action_execution_action_executor_fallback"])
		require.Empty(t, result.Messages)
	})

	t.Run("not_attached_error", func(t *testing.T) {
		eng := runtime.NewEngine(runtime.NewMemStore())
		mgr := newAgentManager(t.TempDir())
		mgr.AttachEngine(eng)
		eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(context.Context, runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
			return runtime.AgentLoopActionExecutorResult{Status: runtime.TurnResultBlocked}, runtime.ErrAgentLoopActionExecutorNotAttached
		}))

		result, handled, err := mgr.runRuntimePlannerActionExecutionActionExecutor(context.Background(), loopReq, action)
		require.ErrorIs(t, err, runtime.ErrAgentLoopActionExecutorNotAttached)
		require.True(t, handled)
		require.Equal(t, runtime.TurnResultBlocked, result.Status)
		require.ErrorIs(t, result.Error, runtime.ErrAgentLoopActionExecutorNotAttached)
		require.Equal(t, "retired", result.Metadata["planner_action_execution_action_executor_fallback"])
		require.Empty(t, result.Messages)
	})

	t.Run("not_attached_result_error", func(t *testing.T) {
		eng := runtime.NewEngine(runtime.NewMemStore())
		mgr := newAgentManager(t.TempDir())
		mgr.AttachEngine(eng)
		eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(context.Context, runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
			return runtime.AgentLoopActionExecutorResult{Status: runtime.TurnResultBlocked, Error: runtime.ErrAgentLoopActionExecutorNotAttached}, nil
		}))

		result, handled, err := mgr.runRuntimePlannerActionExecutionActionExecutor(context.Background(), loopReq, action)
		require.NoError(t, err)
		require.True(t, handled)
		require.Equal(t, runtime.TurnResultBlocked, result.Status)
		require.ErrorIs(t, result.Error, runtime.ErrAgentLoopActionExecutorNotAttached)
		require.Equal(t, "retired", result.Metadata["planner_action_execution_action_executor_fallback"])
		require.Empty(t, result.Messages)
	})

	t.Run("hard_error_does_not_fallback", func(t *testing.T) {
		eng := runtime.NewEngine(runtime.NewMemStore())
		mgr := newAgentManager(t.TempDir())
		mgr.AttachEngine(eng)
		wantErr := errors.New("action executor hard error")
		eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(context.Context, runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
			return runtime.AgentLoopActionExecutorResult{Status: runtime.TurnResultFailed, Reason: "failed"}, wantErr
		}))

		result, handled, err := mgr.runRuntimePlannerActionExecutionActionExecutor(context.Background(), loopReq, action)
		require.ErrorIs(t, err, wantErr)
		require.True(t, handled)
		require.Equal(t, runtime.TurnResultFailed, result.Status)
	})
}

func TestAgentManagerRunRuntimePlannerActionExecutionDispatchesAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := runtime.AgentLoopMessage{ID: "msg-planner-action-loop-runner", Role: "tool", Content: "executed through AgentLoop runner"}
	expectedToolCall := runtime.AgentLoopToolCall{ID: "tool-planner-action-loop-runner", Name: "daemon.health"}
	authProfiles := []agentHTTPAuthProfile{{
		ID:    "auth-action-loop-runner",
		Type:  "bearer",
		Token: "token",
	}}
	action := runtime.AgentLoopAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "planner-action-loop-runner"},
	}

	loopCalled := false
	actionExecutorCalled := false
	turnCalled := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(ctx context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		loopCalled = true
		require.NotNil(t, ctx)
		require.Equal(t, runtime.AgentLoopModePlannerActionExecution, req.Mode)
		require.Equal(t, "session-planner-action-loop-runner", req.Turn.SessionID)
		require.Equal(t, "task-planner-action-loop-runner", req.Turn.TaskID)
		require.Equal(t, "run-planner-action-loop-runner", req.Turn.RunID)
		require.Equal(t, "turn-planner-action-loop-runner", req.Turn.TurnID)
		require.Equal(t, "approved daemon health through loop runner", req.Turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "en", req.Language)
		require.Equal(t, "tester", req.Operator)
		require.Equal(t, action, req.Action)
		require.Equal(t, agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles), req.Authorization.AuthProfiles)
		require.Equal(t, "auth-action-loop-runner", req.Authorization.DefaultAuthProfileID)
		return runtime.AgentLoopResult{
			State: runtime.AgentLoopState{
				ResponseMessages: []runtime.AgentLoopMessage{expectedMessage},
				ToolCalls:        []runtime.AgentLoopToolCall{expectedToolCall},
			},
			Messages:   []runtime.AgentLoopMessage{expectedMessage},
			ToolCalls:  []runtime.AgentLoopToolCall{expectedToolCall},
			StopReason: "planner action loop runner handled",
		}, nil
	}))
	eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(_ context.Context, _ runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
		actionExecutorCalled = true
		return runtime.AgentLoopActionExecutorResult{Status: runtime.TurnResultFailed}, errors.New("action executor should not be called when AgentLoop runner handles planner action")
	}))
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, _ runtime.TurnContext) (runtime.TurnResult, error) {
		turnCalled = true
		return runtime.TurnResult{Status: runtime.TurnResultFailed}, errors.New("turn executor should not be called when AgentLoop runner handles planner action")
	}))

	messages, toolCalls, err := mgr.runRuntimePlannerActionExecution(
		context.Background(),
		"session-planner-action-loop-runner",
		agentRuntimeTurnRef{TaskID: "task-planner-action-loop-runner", RunID: "run-planner-action-loop-runner", TurnID: "turn-planner-action-loop-runner"},
		"approved daemon health through loop runner",
		"en",
		"tester",
		"safe_scan",
		agentLoopToolServiceBindingFromRouterServices(routerServices{}),
		action,
		runtime.AgentLoopAuthorization{
			AuthProfiles:         agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles),
			DefaultAuthProfileID: "auth-action-loop-runner",
		},
	)

	require.NoError(t, err)
	require.True(t, loopCalled)
	require.False(t, actionExecutorCalled)
	require.False(t, turnCalled)
	require.Equal(t, []runtime.AgentLoopMessage{expectedMessage}, messages)
	require.Equal(t, []runtime.AgentLoopToolCall{expectedToolCall}, toolCalls)
}

func TestAgentManagerRunRuntimePlannerActionExecutionNoEngineKeepsDirectLegacyFallback(t *testing.T) {
	mgr := newAgentManager(t.TempDir())

	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok)
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	legacyCalled := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		legacyCalled = true
		require.NotNil(t, ctx)
		require.Equal(t, "session-no-engine-direct-legacy", in.SessionID)
		require.Equal(t, "tester", in.Operator)
		tool := runningAgentToolCall("daemon.health", "no-engine-direct-legacy")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"no_engine_direct_legacy_fallback": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "no-engine direct legacy fallback executed", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, toolCalls, err := mgr.runRuntimePlannerActionExecution(
		context.Background(),
		"session-no-engine-direct-legacy",
		agentRuntimeTurnRef{TaskID: "task-no-engine-direct-legacy", RunID: "run-no-engine-direct-legacy", TurnID: "turn-no-engine-direct-legacy"},
		"approved daemon health",
		"en",
		"tester",
		"read_only",
		agentLoopToolServiceBindingFromRouterServices(routerServices{}),
		runtime.AgentLoopAction{Name: "daemon.health", Arguments: map[string]any{"probe": "no-engine-direct-legacy"}},
		runtime.AgentLoopAuthorization{},
	)

	require.NoError(t, err)
	require.True(t, legacyCalled)
	require.Len(t, messages, 1)
	require.Equal(t, "no-engine direct legacy fallback executed", messages[0].Content)
	require.Len(t, toolCalls, 1)
	require.Equal(t, true, toolCalls[0].Result["no_engine_direct_legacy_fallback"])
}

func TestAgentManagerRunRuntimePlannerActionExecutionMissingActionExecutorBlocksWithoutTurnExecutorFallback(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)
	eng.AttachAgentLoopActionExecutor(nil)

	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok)
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	turnCalled := false
	legacyCalled := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(context.Context, runtime.TurnContext) (runtime.TurnResult, error) {
		turnCalled = true
		return runtime.TurnResult{Status: runtime.TurnResultFailed}, errors.New("planner action execution should not invoke TurnExecutor when action executor is missing")
	}))
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		legacyCalled = true
		require.NotNil(t, ctx)
		require.Equal(t, "session-missing-action-direct-legacy", in.SessionID)
		require.Equal(t, "tester", in.Operator)
		tool := runningAgentToolCall("daemon.health", "missing-action-direct-legacy")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"direct_legacy_fallback": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "direct legacy fallback executed", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, toolCalls, err := mgr.runRuntimePlannerActionExecution(
		context.Background(),
		"session-missing-action-direct-legacy",
		agentRuntimeTurnRef{TaskID: "task-missing-action-direct-legacy", RunID: "run-missing-action-direct-legacy", TurnID: "turn-missing-action-direct-legacy"},
		"approved daemon health",
		"en",
		"tester",
		"read_only",
		agentLoopToolServiceBindingFromRouterServices(routerServices{}),
		runtime.AgentLoopAction{Name: "daemon.health", Arguments: map[string]any{"probe": "missing-action-direct-legacy"}},
		runtime.AgentLoopAuthorization{},
	)

	require.ErrorIs(t, err, runtime.ErrAgentLoopActionExecutorNotAttached)
	require.False(t, turnCalled)
	require.False(t, legacyCalled)
	require.Empty(t, messages)
	require.Empty(t, toolCalls)
}

func TestAgentManagerRunRuntimePlannerActionExecutionActionExecutorNotAttachedBlocksWithoutDirectLegacyFallback(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok)
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	action := runtime.AgentLoopAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "action-not-attached-direct-legacy"},
	}
	actionCalled := false
	turnCalled := false
	legacyCalled := false
	eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(context.Context, runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
		actionCalled = true
		return runtime.AgentLoopActionExecutorResult{Status: runtime.TurnResultBlocked}, runtime.ErrAgentLoopActionExecutorNotAttached
	}))
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(context.Context, runtime.TurnContext) (runtime.TurnResult, error) {
		turnCalled = true
		return runtime.TurnResult{Status: runtime.TurnResultFailed}, errors.New("planner action execution should not invoke TurnExecutor when action executor returns ErrAgentLoopActionExecutorNotAttached")
	}))
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		legacyCalled = true
		require.NotNil(t, ctx)
		require.Equal(t, "session-action-not-attached-direct-legacy", in.SessionID)
		require.Equal(t, "tester", in.Operator)
		tool := runningAgentToolCall("daemon.health", "action-not-attached-direct-legacy")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"direct_legacy_fallback": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "direct legacy fallback executed after action executor not attached", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, toolCalls, err := mgr.runRuntimePlannerActionExecution(
		context.Background(),
		"session-action-not-attached-direct-legacy",
		agentRuntimeTurnRef{TaskID: "task-action-not-attached-direct-legacy", RunID: "run-action-not-attached-direct-legacy", TurnID: "turn-action-not-attached-direct-legacy"},
		"approved daemon health",
		"en",
		"tester",
		"read_only",
		agentLoopToolServiceBindingFromRouterServices(routerServices{}),
		action,
		runtime.AgentLoopAuthorization{},
	)

	require.ErrorIs(t, err, runtime.ErrAgentLoopActionExecutorNotAttached)
	require.True(t, actionCalled)
	require.False(t, turnCalled)
	require.False(t, legacyCalled)
	require.Empty(t, messages)
	require.Empty(t, toolCalls)
}

func TestAgentTurnExecutionResultPayloadHandlesDispatchStates(t *testing.T) {
	expected := runtime.AgentLoopTurnResultPayload{
		Messages: []runtime.AgentLoopMessage{{
			ID:      "msg-turn-result-shared",
			Role:    "tool",
			Content: "handled",
			ToolCalls: []runtime.AgentLoopToolCall{{
				ID:       "tool-message-turn-result-shared",
				Name:     "daemon.health",
				Metadata: map[string]any{"phase": "message"},
			}},
		}},
		ToolCalls: []runtime.AgentLoopToolCall{{
			ID:       "tool-turn-result-shared",
			Name:     "daemon.health",
			Result:   map[string]any{"ok": true},
			Metadata: map[string]any{"phase": "top"},
		}},
		Metadata: map[string]any{"source": "shared"},
	}

	payload, handled, err := agentTurnExecutionResultPayload(runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Payload: expected,
	}, nil, "daemon: shared invalid payload")
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, expected, payload)
	expected.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message"
	expected.ToolCalls[0].Result["ok"] = false
	expected.ToolCalls[0].Metadata["phase"] = "mutated-top"
	expected.Metadata["source"] = "mutated"
	require.Equal(t, "message", payload.Messages[0].ToolCalls[0].Metadata["phase"])
	require.Equal(t, true, payload.ToolCalls[0].Result["ok"])
	require.Equal(t, "top", payload.ToolCalls[0].Metadata["phase"])
	require.Equal(t, "shared", payload.Metadata["source"])

	pointerPayload := payload
	payload, handled, err = agentTurnExecutionResultPayload(runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Payload: &pointerPayload,
	}, nil, "daemon: shared invalid payload")
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "shared", payload.Metadata["source"])

	missingErr := runtime.ErrTurnExecutorNotAttached
	for _, tc := range []struct {
		name      string
		result    runtime.TurnResult
		resultErr error
		reason    daemonTurnExecutionCallerFallbackReason
	}{
		{name: "returned_missing_executor", resultErr: missingErr, reason: daemonTurnExecutionCallerFallbackExecutorNotAttached},
		{name: "embedded_missing_executor", result: runtime.TurnResult{Error: missingErr}, reason: daemonTurnExecutionCallerFallbackExecutorNotAttached},
		{name: "blocked", result: runtime.TurnResult{Status: runtime.TurnResultBlocked, Payload: pointerPayload}, reason: daemonTurnExecutionCallerFallbackBlocked},
		{name: "running_without_payload", result: runtime.TurnResult{Status: runtime.TurnResultRunning}, reason: daemonTurnExecutionCallerFallbackRunningWithoutPayload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, ok := daemonTurnExecutionCallerFallbackReasonForResult(tc.result, tc.resultErr)
			require.True(t, ok)
			require.Equal(t, tc.reason, reason)
			require.True(t, daemonTurnExecutionCallerFallbackOwnedByLegacyCaller(reason))

			payload, handled, err := agentTurnExecutionResultPayload(tc.result, tc.resultErr, "daemon: shared invalid payload")
			require.NoError(t, err)
			require.False(t, handled)
			require.Empty(t, payload.Messages)
			require.Empty(t, payload.ToolCalls)
		})
	}

	_, fallback := daemonTurnExecutionCallerFallbackReasonForResult(runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Payload: pointerPayload,
	}, nil)
	require.False(t, fallback)

	boom := errors.New("executor failed")
	payload, handled, err = agentTurnExecutionResultPayload(runtime.TurnResult{
		Status: runtime.TurnResultFailed,
	}, boom, "daemon: shared invalid payload")
	require.ErrorIs(t, err, boom)
	require.True(t, handled)
	require.Empty(t, payload.Messages)

	payload, handled, err = agentTurnExecutionResultPayload(runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Payload: "not a payload",
	}, nil, "daemon: custom turn executor returned invalid payload")
	require.ErrorContains(t, err, "custom turn executor returned invalid payload")
	require.True(t, handled)
	require.Empty(t, payload.Messages)
}

func TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	enabled := true
	authProfiles := []agentHTTPAuthProfile{{
		ID:      "auth-cycle-action-exec",
		Type:    "bearer",
		BaseURL: "https://sandbox.example",
		Token:   "token-cycle-action-exec",
		Enabled: &enabled,
	}}
	cycleAuthProfiles := agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles)
	plan := &agentPlannerResponse{Actions: []agentPlannerAction{{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "cycle-action-exec"},
	}}}
	cyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(plan)
	plan.Actions[0].Arguments["probe"] = "mutated-source-plan"
	cyclePlan.item.Actions = nil
	cyclePlan.item.ToolCalls = nil
	authProfiles[0].Token = "mutated-source-token"
	*authProfiles[0].Enabled = false

	req := agentPlannerActionReplanCycleRequest{
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-cycle-action-exec",
			agentRuntimeTurnRef{TaskID: "task-cycle-action-exec", RunID: "run-cycle-action-exec", TurnID: "turn-cycle-action-exec"},
			"daemon health",
			"user",
			"read_only",
		),
		Language: "en",
		Operator: "tester",
		Authorization: runtime.AgentLoopAuthorization{
			AuthProfiles:         cycleAuthProfiles,
			DefaultAuthProfileID: "auth-cycle-action-exec",
		},
		ToolContext: agentPlannerActionReplanCycleToolContextFromToolServices(agentLoopToolServiceBindingFromRouterServices(routerServices{jobs: jobs})),
		Plan:        cyclePlan,
	}

	actionMessage := agentMessage{
		ID:      "msg-cycle-action-exec",
		Role:    "tool",
		Content: "cycle action executed",
		ToolCalls: []agentToolCall{{
			ID:     "tool-cycle-action-exec-message",
			Name:   "daemon.health",
			Result: map[string]any{"ok": true},
		}},
	}
	actionTool := agentToolCall{
		ID:     "tool-cycle-action-exec",
		Name:   "daemon.health",
		Result: map[string]any{"ok": true},
	}

	called := false
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		called = true
		require.Equal(t, "session-cycle-action-exec", turn.SessionID)
		require.Equal(t, "task-cycle-action-exec", turn.TaskID)
		require.Equal(t, "run-cycle-action-exec", turn.RunID)
		require.Equal(t, "turn-cycle-action-exec", turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Metadata["kind"])
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "session-cycle-action-exec", payload.Request.Turn.SessionID)
		require.Equal(t, "en", payload.Request.Language)
		require.Equal(t, "tester", payload.Request.Operator)
		require.Equal(t, "read_only", payload.Request.Turn.OperationLevel)
		require.Equal(t, "cycle-action-exec", stringFromAny(payload.Action.Arguments["probe"]))
		require.Len(t, payload.Request.Authorization.AuthProfiles, 1)
		require.Equal(t, "auth-cycle-action-exec", payload.Request.Authorization.DefaultAuthProfileID)
		require.Equal(t, "token-cycle-action-exec", payload.Request.Authorization.AuthProfiles[0].Token)
		require.NotNil(t, payload.Request.Authorization.AuthProfiles[0].Enabled)
		require.True(t, *payload.Request.Authorization.AuthProfiles[0].Enabled)
		payloadJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](payload.Request.ToolServices.ActionServices, runtime.AgentLoopActionServiceJobs)
		require.True(t, ok)
		require.Same(t, jobs, payloadJobs)

		payload.Action.Arguments["probe"] = "mutated-payload-action"
		payload.Request.Authorization.AuthProfiles[0].Token = "mutated-payload-token"
		*payload.Request.Authorization.AuthProfiles[0].Enabled = false

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "cycle action execution handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{actionTool}),
			},
		}, nil
	}))

	loopReq := req.agentLoopRequest(runtime.AgentLoopModeReplanContinuation)
	result := mgr.runAgentPlannerActionReplanCycleActionExecutionAgentLoop(context.Background(), loopReq)

	require.True(t, called)
	require.False(t, result.AuthorizationRequired)
	require.Equal(t, "cycle-action-exec", stringFromAny(req.Plan.actions()[0].Arguments["probe"]))
	require.Equal(t, "cycle-action-exec", stringFromAny(loopReq.InitialPlan.Actions[0].Arguments["probe"]))
	require.Equal(t, "token-cycle-action-exec", req.Authorization.AuthProfiles[0].Token)
	require.True(t, *req.Authorization.AuthProfiles[0].Enabled)

	gotMessages := agentMessagesFromAgentLoopMessages(result.Messages)
	gotToolCalls := agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls)
	require.Len(t, gotMessages, 1)
	require.Len(t, gotToolCalls, 1)
	require.Equal(t, "cycle action executed", gotMessages[0].Content)
	require.Equal(t, "daemon.health", gotMessages[0].ToolCalls[0].Metadata["action_name"])
	require.Equal(t, 1, gotMessages[0].ToolCalls[0].Metadata["action_index"])
	require.Equal(t, 1, gotMessages[0].ToolCalls[0].Metadata["action_count"])
	require.NotEmpty(t, gotMessages[0].ToolCalls[0].Metadata["action_queue_id"])
	require.Equal(t, "daemon.health", gotToolCalls[0].Metadata["action_name"])
	require.Equal(t, gotMessages[0].ToolCalls[0].Metadata["action_queue_id"], gotToolCalls[0].Metadata["action_queue_id"])

	actionMessage.Content = "mutated source message"
	actionMessage.ToolCalls[0].Result["ok"] = false
	actionTool.Result["ok"] = false
	gotMessages[0].Content = "mutated got message"
	gotMessages[0].ToolCalls[0].Result["ok"] = "mutated"
	gotToolCalls[0].Result["ok"] = "mutated"

	require.Equal(t, "cycle action executed", agentMessagesFromAgentLoopMessages(result.Messages)[0].Content)
	require.Equal(t, true, agentMessagesFromAgentLoopMessages(result.Messages)[0].ToolCalls[0].Result["ok"])
	require.Equal(t, true, agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls)[0].Result["ok"])
}

func TestAgentManagerExecuteAgentPlannerActionsDispatchesPlannerActionsThroughTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedToolCall := agentToolCall{ID: "tool-planner-loop-action", Name: "daemon.health"}
	expectedMessage := agentMessage{
		ID:        "msg-planner-loop-action",
		Role:      "tool",
		Content:   "planner loop action executed through executor",
		ToolCalls: []agentToolCall{expectedToolCall},
	}
	action := agentPlannerAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "planner-loop"},
	}
	called := false
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		called = true
		require.Equal(t, "session-planner-loop-action", turn.SessionID)
		require.Equal(t, "task-planner-loop-action", turn.TaskID)
		require.Equal(t, "run-planner-loop-action", turn.RunID)
		require.Equal(t, "turn-planner-loop-action", turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Metadata["kind"])
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "session-planner-loop-action", payload.Request.Turn.SessionID)
		require.Equal(t, "en", payload.Request.Language)
		require.Equal(t, "tester", payload.Request.Operator)
		require.Equal(t, "read_only", payload.Request.Turn.OperationLevel)
		require.Equal(t, agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action), payload.Action)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
			},
		}, nil
	}))

	messages, toolCalls, authorizationRequired := mgr.executeAgentPlannerActionsWithToolServices(
		context.Background(),
		"session-planner-loop-action",
		agentRuntimeTurnRef{TaskID: "task-planner-loop-action", RunID: "run-planner-loop-action", TurnID: "turn-planner-loop-action"},
		"daemon health",
		"en",
		"tester",
		"read_only",
		testAgentLoopToolServices(),
		&agentPlannerResponse{Actions: []agentPlannerAction{action}},
		nil,
		"",
	)

	require.True(t, called)
	require.False(t, authorizationRequired)
	require.Len(t, messages, 1)
	require.Equal(t, expectedMessage.ID, messages[0].ID)
	require.Len(t, messages[0].ToolCalls, 1)
	require.Equal(t, expectedToolCall.ID, messages[0].ToolCalls[0].ID)
	require.Equal(t, "daemon.health", messages[0].ToolCalls[0].Metadata["action_name"])
	require.Equal(t, 1, messages[0].ToolCalls[0].Metadata["action_index"])
	require.Equal(t, 1, messages[0].ToolCalls[0].Metadata["action_count"])
	require.NotEmpty(t, messages[0].ToolCalls[0].Metadata["action_queue_id"])
	require.Len(t, toolCalls, 1)
	require.Equal(t, expectedToolCall.ID, toolCalls[0].ID)
	require.Equal(t, "daemon.health", toolCalls[0].Metadata["action_name"])
	require.Equal(t, messages[0].ToolCalls[0].Metadata["action_queue_id"], toolCalls[0].Metadata["action_queue_id"])
}

func TestAgentPlannerActionReplanCycleLLMRequestFromMessageRequestKeepsOnlyLLMFields(t *testing.T) {
	enabled := true
	authProfiles := []agentHTTPAuthProfile{{
		ID:          "auth-cycle-llm",
		Name:        "Cycle Auth",
		Type:        "bearer",
		BaseURL:     "https://sandbox.example",
		Token:       "secret-token",
		Username:    "agent-user",
		Password:    "agent-pass",
		HeaderName:  "X-Agent-Auth",
		HeaderValue: "header-secret",
		Cookie:      "session=secret",
		Enabled:     &enabled,
	}}
	req := sendAgentMessageRequest{
		Message:              "operator prompt should not enter cycle llm dto",
		Operator:             "operator should not enter cycle llm dto",
		Language:             "zh-CN",
		OperationLevel:       "safe_scan",
		Origin:               "user",
		Provider:             "openai",
		Model:                "gpt-cycle",
		APIKey:               "api-key",
		BaseURL:              "https://llm.example",
		Instructions:         "planner instructions",
		AuthProfiles:         authProfiles,
		DefaultAuthProfileID: "auth-cycle-llm",
	}

	llmReq := agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req)
	authProfiles[0].Token = "mutated-token"
	*authProfiles[0].Enabled = false

	require.Equal(t, "user", llmReq.Origin)
	require.Equal(t, "safe_scan", llmReq.OperationLevel)
	require.Equal(t, "openai", llmReq.Provider)
	require.Equal(t, "gpt-cycle", llmReq.Model)
	require.Equal(t, "api-key", llmReq.APIKey)
	require.Equal(t, "https://llm.example", llmReq.BaseURL)
	require.Equal(t, "planner instructions", llmReq.Instructions)
	require.Equal(t, "auth-cycle-llm", llmReq.DefaultAuthProfileID)
	require.Len(t, llmReq.AuthProfiles, 1)
	require.Equal(t, "secret-token", llmReq.AuthProfiles[0].Token)
	require.NotNil(t, llmReq.AuthProfiles[0].Enabled)
	require.True(t, *llmReq.AuthProfiles[0].Enabled)

	converted := sendMessageRequestFromAgentLoopLLMRequest(llmReq)
	require.Empty(t, converted.Message)
	require.Empty(t, converted.Operator)
	require.Empty(t, converted.Language)
	require.Equal(t, "safe_scan", converted.OperationLevel)
	require.Equal(t, "user", converted.Origin)
	require.Equal(t, "openai", converted.Provider)
	require.Equal(t, "gpt-cycle", converted.Model)
	require.Equal(t, "api-key", converted.APIKey)
	require.Equal(t, "https://llm.example", converted.BaseURL)
	require.Equal(t, "planner instructions", converted.Instructions)
	require.Equal(t, "auth-cycle-llm", converted.DefaultAuthProfileID)
	require.Len(t, converted.AuthProfiles, 1)
	require.Equal(t, agentHTTPAuthProfile{
		ID:          "auth-cycle-llm",
		Name:        "Cycle Auth",
		Type:        "bearer",
		BaseURL:     "https://sandbox.example",
		Token:       "secret-token",
		Username:    "agent-user",
		Password:    "agent-pass",
		HeaderName:  "X-Agent-Auth",
		HeaderValue: "header-secret",
		Cookie:      "session=secret",
		Enabled:     converted.AuthProfiles[0].Enabled,
	}, converted.AuthProfiles[0])
	require.NotNil(t, converted.AuthProfiles[0].Enabled)
	require.True(t, *converted.AuthProfiles[0].Enabled)

	converted.AuthProfiles[0].Token = "converted-mutated-token"
	*converted.AuthProfiles[0].Enabled = false
	require.Equal(t, "secret-token", llmReq.AuthProfiles[0].Token)
	require.True(t, *llmReq.AuthProfiles[0].Enabled)
}

func TestAgentPlannerActionReplanCycleAuthorizationFromMessageRequestUsesPrivateAuthProfileDTO(t *testing.T) {
	enabled := true
	req := sendAgentMessageRequest{
		AuthProfiles: []agentHTTPAuthProfile{{
			ID:          "auth-cycle-action",
			Name:        "Action Auth",
			Type:        "header",
			BaseURL:     "https://sandbox-action.example",
			Token:       "action-token",
			Username:    "action-user",
			Password:    "action-pass",
			HeaderName:  "X-Action-Auth",
			HeaderValue: "action-header-secret",
			Cookie:      "action=session",
			Enabled:     &enabled,
		}},
		DefaultAuthProfileID: "auth-cycle-action",
	}

	auth := agentPlannerActionReplanCycleAuthorizationFromMessageRequest(req)
	req.AuthProfiles[0].Token = "mutated-token"
	*req.AuthProfiles[0].Enabled = false

	require.Equal(t, "auth-cycle-action", auth.DefaultAuthProfileID)
	require.Len(t, auth.AuthProfiles, 1)
	require.Equal(t, "auth-cycle-action", auth.AuthProfiles[0].ID)
	require.Equal(t, "Action Auth", auth.AuthProfiles[0].Name)
	require.Equal(t, "header", auth.AuthProfiles[0].Type)
	require.Equal(t, "https://sandbox-action.example", auth.AuthProfiles[0].BaseURL)
	require.Equal(t, "action-token", auth.AuthProfiles[0].Token)
	require.Equal(t, "action-user", auth.AuthProfiles[0].Username)
	require.Equal(t, "action-pass", auth.AuthProfiles[0].Password)
	require.Equal(t, "X-Action-Auth", auth.AuthProfiles[0].HeaderName)
	require.Equal(t, "action-header-secret", auth.AuthProfiles[0].HeaderValue)
	require.Equal(t, "action=session", auth.AuthProfiles[0].Cookie)
	require.NotNil(t, auth.AuthProfiles[0].Enabled)
	require.True(t, *auth.AuthProfiles[0].Enabled)

	converted := agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(auth.AuthProfiles)
	require.Len(t, converted, 1)
	require.Equal(t, "action-token", converted[0].Token)
	require.NotNil(t, converted[0].Enabled)
	require.True(t, *converted[0].Enabled)

	converted[0].Token = "converted-mutated-token"
	*converted[0].Enabled = false
	require.Equal(t, "action-token", auth.AuthProfiles[0].Token)
	require.True(t, *auth.AuthProfiles[0].Enabled)
}

func TestAgentPlannerActionReplanCyclePlanWrapsLegacyPlannerResponse(t *testing.T) {
	emptyCyclePlan := agentPlannerActionReplanCyclePlan{}
	require.Nil(t, emptyCyclePlan.legacyPlannerResponse())
	require.Zero(t, emptyCyclePlan.actionCount())
	require.False(t, emptyCyclePlan.hasActions())
	require.False(t, emptyCyclePlan.shouldAutoContinue())

	noActionCyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
		NextAction: &agentNextAction{AutoContinue: true},
	})
	require.Zero(t, noActionCyclePlan.actionCount())
	require.False(t, noActionCyclePlan.hasActions())
	require.False(t, noActionCyclePlan.shouldAutoContinue())

	manualStopCyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
		NextAction: &agentNextAction{AutoContinue: false},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "manual-stop"},
		}},
	})
	require.Equal(t, 1, manualStopCyclePlan.actionCount())
	require.True(t, manualStopCyclePlan.hasActions())
	require.False(t, manualStopCyclePlan.shouldAutoContinue())

	plan := &agentPlannerResponse{
		Phase:            "step_replan",
		Answer:           "cycle plan answer",
		SuggestedPrompt:  "primary cycle suggestion",
		SuggestPrompt:    "legacy cycle suggestion",
		SuggestedPrompts: []string{"continue cycle"},
		RecommendationOptions: []agentRecommendationOpt{{
			ID:      "rec-cycle-plan",
			Title:   "Continue",
			Detail:  "continue detail",
			Prompt:  "continue cycle",
			Command: "/continue-cycle",
		}},
		Task: &agentPlannerTask{
			Goal:        "inspect cycle plan",
			Kind:        "tool",
			Status:      "running",
			ActiveRunID: "run-cycle-plan",
			Summary:     "cycle plan task summary",
		},
		Todos: []agentPlannerTodo{{
			Title:         "call daemon health",
			Detail:        "collect health status",
			Status:        "in_progress",
			Priority:      2,
			ContextRefIDs: []string{"ctx-cycle-plan"},
			BlockedReason: "waiting for action",
		}},
		ContextRefs: []agentContextRef{{
			ID:      "ctx-cycle-plan",
			Kind:    "message",
			RefID:   "msg-cycle-plan",
			Label:   "Cycle plan",
			To:      "message:msg-cycle-plan",
			Summary: "cycle context summary",
		}},
		NextAction: &agentNextAction{
			Type:            "continue",
			Reason:          "more tool work",
			SuggestedPrompt: "continue with daemon health",
			AutoContinue:    true,
		},
		Reasoning: "cycle private plan reasoning",
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "cycle-plan"},
		}},
		ToolCalls: []agentPlannerAction{{
			Name:      "runs.list",
			Arguments: map[string]any{"limit": 1},
		}},
	}

	cyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(plan)
	require.Equal(t, 2, cyclePlan.actionCount())
	require.True(t, cyclePlan.hasActions())
	require.True(t, cyclePlan.shouldAutoContinue())
	require.Len(t, cyclePlan.actions(), 2)
	require.Equal(t, "cycle-plan", cyclePlan.actions()[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.actions()[1].Arguments["limit"])
	require.True(t, cyclePlan.present)
	require.Equal(t, "cycle plan answer", cyclePlan.item.Answer)
	require.Equal(t, "primary cycle suggestion", cyclePlan.item.SuggestedPrompt)
	require.Equal(t, "legacy cycle suggestion", cyclePlan.item.SuggestPrompt)
	require.Equal(t, "continue cycle", cyclePlan.item.SuggestedPrompts[0])
	require.Equal(t, "Continue", cyclePlan.item.RecommendationOptions[0].Title)
	require.Equal(t, "continue detail", cyclePlan.item.RecommendationOptions[0].Detail)
	require.Equal(t, "/continue-cycle", cyclePlan.item.RecommendationOptions[0].Command)
	require.Equal(t, "inspect cycle plan", cyclePlan.item.Task.Goal)
	require.Equal(t, "run-cycle-plan", cyclePlan.item.Task.ActiveRunID)
	require.Equal(t, "cycle plan task summary", cyclePlan.item.Task.Summary)
	require.Equal(t, "collect health status", cyclePlan.item.Todos[0].Detail)
	require.Equal(t, 2, cyclePlan.item.Todos[0].Priority)
	require.Equal(t, "waiting for action", cyclePlan.item.Todos[0].BlockedReason)
	require.Equal(t, "message:msg-cycle-plan", cyclePlan.item.ContextRefs[0].To)
	require.Equal(t, "cycle context summary", cyclePlan.item.ContextRefs[0].Summary)
	require.Equal(t, "more tool work", cyclePlan.item.NextAction.Reason)
	require.Equal(t, "continue with daemon health", cyclePlan.item.NextAction.SuggestedPrompt)
	require.Equal(t, "cycle private plan reasoning", cyclePlan.item.Reasoning)
	require.Equal(t, "cycle-plan", cyclePlan.item.Actions[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.item.ToolCalls[0].Arguments["limit"])

	plan.Phase = "mutated"
	plan.SuggestedPrompt = "mutated primary suggestion"
	plan.SuggestPrompt = "mutated legacy suggestion"
	plan.SuggestedPrompts[0] = "mutated prompt"
	plan.RecommendationOptions[0].Title = "Mutated"
	plan.RecommendationOptions[0].Detail = "mutated detail"
	plan.RecommendationOptions[0].Command = "/mutated"
	plan.Task.Goal = "mutated goal"
	plan.Task.ActiveRunID = "run-mutated"
	plan.Task.Summary = "mutated summary"
	plan.Todos[0].Title = "mutated todo"
	plan.Todos[0].Detail = "mutated todo detail"
	plan.Todos[0].Priority = 99
	plan.Todos[0].BlockedReason = "mutated blocked"
	plan.Todos[0].ContextRefIDs[0] = "ctx-mutated"
	plan.ContextRefs[0].Label = "Mutated context"
	plan.ContextRefs[0].To = "message:mutated"
	plan.ContextRefs[0].Summary = "mutated context summary"
	plan.NextAction.Type = "stop"
	plan.NextAction.Reason = "mutated reason"
	plan.NextAction.SuggestedPrompt = "mutated next prompt"
	plan.NextAction.AutoContinue = false
	plan.Reasoning = "mutated reasoning"
	plan.Actions[0].Arguments["probe"] = "mutated-probe"
	plan.ToolCalls[0].Arguments["limit"] = 99
	plan.Actions = nil
	plan.ToolCalls = nil

	require.Equal(t, 2, cyclePlan.actionCount())
	require.True(t, cyclePlan.hasActions())
	require.True(t, cyclePlan.shouldAutoContinue())
	require.Len(t, cyclePlan.actions(), 2)
	require.Equal(t, "cycle-plan", cyclePlan.actions()[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.actions()[1].Arguments["limit"])

	got := cyclePlan.legacyPlannerResponse()
	require.NotNil(t, got)
	require.NotSame(t, plan, got)
	require.Equal(t, "step_replan", got.Phase)
	require.Equal(t, "cycle plan answer", got.Answer)
	require.Equal(t, "primary cycle suggestion", got.SuggestedPrompt)
	require.Equal(t, "legacy cycle suggestion", got.SuggestPrompt)
	require.Equal(t, "continue cycle", got.SuggestedPrompts[0])
	require.Equal(t, "Continue", got.RecommendationOptions[0].Title)
	require.Equal(t, "continue detail", got.RecommendationOptions[0].Detail)
	require.Equal(t, "/continue-cycle", got.RecommendationOptions[0].Command)
	require.NotSame(t, plan.Task, got.Task)
	require.Equal(t, "inspect cycle plan", got.Task.Goal)
	require.Equal(t, "run-cycle-plan", got.Task.ActiveRunID)
	require.Equal(t, "cycle plan task summary", got.Task.Summary)
	require.Equal(t, "call daemon health", got.Todos[0].Title)
	require.Equal(t, "collect health status", got.Todos[0].Detail)
	require.Equal(t, 2, got.Todos[0].Priority)
	require.Equal(t, "waiting for action", got.Todos[0].BlockedReason)
	require.Equal(t, "ctx-cycle-plan", got.Todos[0].ContextRefIDs[0])
	require.Equal(t, "Cycle plan", got.ContextRefs[0].Label)
	require.Equal(t, "message:msg-cycle-plan", got.ContextRefs[0].To)
	require.Equal(t, "cycle context summary", got.ContextRefs[0].Summary)
	require.NotSame(t, plan.NextAction, got.NextAction)
	require.Equal(t, "continue", got.NextAction.Type)
	require.Equal(t, "more tool work", got.NextAction.Reason)
	require.Equal(t, "continue with daemon health", got.NextAction.SuggestedPrompt)
	require.True(t, got.NextAction.AutoContinue)
	require.Equal(t, "cycle private plan reasoning", got.Reasoning)
	require.Equal(t, "cycle-plan", got.Actions[0].Arguments["probe"])
	require.Equal(t, 1, got.ToolCalls[0].Arguments["limit"])
	require.Len(t, agentPlannerActions(got), 2)

	got.Phase = "mutated returned"
	got.SuggestedPrompt = "mutated returned primary"
	got.SuggestPrompt = "mutated returned legacy"
	got.SuggestedPrompts[0] = "mutated returned prompt"
	got.RecommendationOptions[0].Title = "Mutated returned"
	got.RecommendationOptions[0].Detail = "mutated returned detail"
	got.RecommendationOptions[0].Command = "/mutated-returned"
	got.Task.Goal = "mutated returned goal"
	got.Task.ActiveRunID = "run-mutated-returned"
	got.Task.Summary = "mutated returned summary"
	got.Todos[0].Title = "mutated returned todo"
	got.Todos[0].Detail = "mutated returned todo detail"
	got.Todos[0].Priority = 100
	got.Todos[0].BlockedReason = "mutated returned blocked"
	got.Todos[0].ContextRefIDs[0] = "ctx-mutated-returned"
	got.ContextRefs[0].Label = "Mutated returned context"
	got.ContextRefs[0].To = "message:mutated-returned"
	got.ContextRefs[0].Summary = "mutated returned context summary"
	got.NextAction.Type = "stop returned"
	got.NextAction.Reason = "mutated returned reason"
	got.NextAction.SuggestedPrompt = "mutated returned next prompt"
	got.NextAction.AutoContinue = false
	got.Reasoning = "mutated returned reasoning"
	got.Actions[0].Arguments["probe"] = "mutated-returned-probe"
	got.ToolCalls[0].Arguments["limit"] = 100
	got.Actions = nil
	got.ToolCalls = nil

	require.Equal(t, 2, cyclePlan.actionCount())
	require.True(t, cyclePlan.hasActions())
	require.True(t, cyclePlan.shouldAutoContinue())

	returnedActions := cyclePlan.actions()
	require.Len(t, returnedActions, 2)
	returnedActions[0].Arguments["probe"] = "mutated-returned-actions"
	returnedActions[1].Arguments["limit"] = 101
	require.Equal(t, "cycle-plan", cyclePlan.actions()[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.actions()[1].Arguments["limit"])

	cyclePlan.item.Actions = nil
	cyclePlan.item.ToolCalls = nil
	cyclePlan.item.NextAction.AutoContinue = false

	require.Equal(t, 2, cyclePlan.actionCount())
	require.True(t, cyclePlan.hasActions())
	require.True(t, cyclePlan.shouldAutoContinue())
	require.Len(t, cyclePlan.actions(), 2)
	require.Equal(t, "cycle-plan", cyclePlan.actions()[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.actions()[1].Arguments["limit"])

	gotAgain := cyclePlan.legacyPlannerResponse()
	require.NotNil(t, gotAgain)
	require.Equal(t, "step_replan", gotAgain.Phase)
	require.Equal(t, "cycle plan answer", gotAgain.Answer)
	require.Equal(t, "primary cycle suggestion", gotAgain.SuggestedPrompt)
	require.Equal(t, "legacy cycle suggestion", gotAgain.SuggestPrompt)
	require.Equal(t, "continue cycle", gotAgain.SuggestedPrompts[0])
	require.Equal(t, "Continue", gotAgain.RecommendationOptions[0].Title)
	require.Equal(t, "continue detail", gotAgain.RecommendationOptions[0].Detail)
	require.Equal(t, "/continue-cycle", gotAgain.RecommendationOptions[0].Command)
	require.Equal(t, "inspect cycle plan", gotAgain.Task.Goal)
	require.Equal(t, "run-cycle-plan", gotAgain.Task.ActiveRunID)
	require.Equal(t, "cycle plan task summary", gotAgain.Task.Summary)
	require.Equal(t, "call daemon health", gotAgain.Todos[0].Title)
	require.Equal(t, "collect health status", gotAgain.Todos[0].Detail)
	require.Equal(t, 2, gotAgain.Todos[0].Priority)
	require.Equal(t, "waiting for action", gotAgain.Todos[0].BlockedReason)
	require.Equal(t, "ctx-cycle-plan", gotAgain.Todos[0].ContextRefIDs[0])
	require.Equal(t, "Cycle plan", gotAgain.ContextRefs[0].Label)
	require.Equal(t, "message:msg-cycle-plan", gotAgain.ContextRefs[0].To)
	require.Equal(t, "cycle context summary", gotAgain.ContextRefs[0].Summary)
	require.Equal(t, "continue", gotAgain.NextAction.Type)
	require.Equal(t, "more tool work", gotAgain.NextAction.Reason)
	require.Equal(t, "continue with daemon health", gotAgain.NextAction.SuggestedPrompt)
	require.False(t, gotAgain.NextAction.AutoContinue)
	require.Equal(t, "cycle private plan reasoning", gotAgain.Reasoning)
	require.Empty(t, agentPlannerActions(gotAgain))
}

func TestAgentPlannerActionReplanCycleMemorySummaryWrapsLegacyMemory(t *testing.T) {
	require.Nil(t, agentPlannerActionReplanCycleMemorySummary{}.legacyAgentMemorySummary())
	require.Nil(t, agentPlannerActionReplanCycleMemorySummaryFromAgentMemorySummary(nil).legacyAgentMemorySummary())

	lastMessageAt := time.Date(2026, 6, 6, 15, 45, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	updatedAt := time.Date(2026, 6, 6, 15, 50, 0, 0, time.UTC)
	memory := &agentMemorySummary{
		Summary:       "cycle memory",
		MessageCount:  7,
		LastMessageID: "msg-cycle-memory",
		LastMessageAt: &lastMessageAt,
		UpdatedAt:     updatedAt,
	}

	cycleMemory := agentPlannerActionReplanCycleMemorySummaryFromAgentMemorySummary(memory)
	require.True(t, cycleMemory.present)
	require.Equal(t, "cycle memory", cycleMemory.item.Summary)
	require.Equal(t, 7, cycleMemory.item.MessageCount)
	require.Equal(t, "msg-cycle-memory", cycleMemory.item.LastMessageID)
	require.NotNil(t, cycleMemory.item.LastMessageAt)
	require.Equal(t, originalLastMessageAt, *cycleMemory.item.LastMessageAt)
	require.Equal(t, updatedAt, cycleMemory.item.UpdatedAt)
	require.NotSame(t, memory.LastMessageAt, cycleMemory.item.LastMessageAt)

	mutatedLastMessageAt := time.Date(2026, 6, 6, 16, 0, 0, 0, time.UTC)
	memory.Summary = "mutated memory"
	memory.MessageCount = 99
	memory.LastMessageID = "msg-mutated-memory"
	*memory.LastMessageAt = mutatedLastMessageAt
	memory.UpdatedAt = mutatedLastMessageAt

	got := cycleMemory.legacyAgentMemorySummary()
	require.NotNil(t, got)
	require.Equal(t, "cycle memory", got.Summary)
	require.Equal(t, 7, got.MessageCount)
	require.Equal(t, "msg-cycle-memory", got.LastMessageID)
	require.NotNil(t, got.LastMessageAt)
	require.Equal(t, originalLastMessageAt, *got.LastMessageAt)
	require.Equal(t, updatedAt, got.UpdatedAt)
	require.NotSame(t, memory.LastMessageAt, got.LastMessageAt)
	require.NotSame(t, cycleMemory.item.LastMessageAt, got.LastMessageAt)

	returnedLastMessageAt := time.Date(2026, 6, 6, 16, 15, 0, 0, time.UTC)
	got.Summary = "mutated returned memory"
	got.MessageCount = 100
	got.LastMessageID = "msg-mutated-returned-memory"
	*got.LastMessageAt = returnedLastMessageAt
	got.UpdatedAt = returnedLastMessageAt

	gotAgain := cycleMemory.legacyAgentMemorySummary()
	require.NotNil(t, gotAgain)
	require.Equal(t, "cycle memory", gotAgain.Summary)
	require.Equal(t, 7, gotAgain.MessageCount)
	require.Equal(t, "msg-cycle-memory", gotAgain.LastMessageID)
	require.NotNil(t, gotAgain.LastMessageAt)
	require.Equal(t, originalLastMessageAt, *gotAgain.LastMessageAt)
	require.Equal(t, updatedAt, gotAgain.UpdatedAt)
	require.NotSame(t, got.LastMessageAt, gotAgain.LastMessageAt)
}

func TestAgentPlannerActionReplanCycleStateWrapsLegacyMessagesAndToolCalls(t *testing.T) {
	messageAt := time.Date(2026, 6, 6, 15, 28, 0, 0, time.UTC)
	startedAt := time.Date(2026, 6, 6, 15, 29, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 6, 15, 30, 0, 0, time.UTC)
	responseMessages := []agentMessage{{
		ID:        "msg-cycle-state-response",
		Role:      "tool",
		Content:   "response message",
		Timestamp: messageAt,
		Phase:     "tool",
		ToolCalls: []agentToolCall{{
			ID:         "tool-cycle-state-response",
			Name:       "daemon.health",
			Status:     contracts.StatusSucceeded,
			Detail:     "response detail",
			StartedAt:  startedAt,
			FinishedAt: &finishedAt,
			Result:     map[string]any{"source": "response"},
			Error:      &daemonJobError{Code: services.CodeBadRequest, Message: "response warning"},
			Metadata:   map[string]interface{}{"phase": "response"},
		}},
		Links: []agentLink{{
			To:    "/runs/run-cycle-state",
			Label: "Run",
			Kind:  "run",
		}},
		Reasoning:        "cycle state reasoning",
		SuggestedPrompts: []string{"continue cycle state"},
		RecommendationOptions: []agentRecommendationOpt{{
			ID:     "rec-cycle-state",
			Title:  "Continue",
			Detail: "Continue cycle state",
			Prompt: "continue cycle state",
		}},
	}}
	toolCalls := []agentToolCall{{
		ID:        "tool-cycle-state-top-level",
		Name:      "LLM synthesis",
		Status:    contracts.StatusSucceeded,
		Detail:    "top level detail",
		StartedAt: startedAt,
		Result:    map[string]any{"source": "tool"},
		Error:     &daemonJobError{Code: services.CodeInternal, Message: "top-level warning"},
		Metadata:  map[string]interface{}{"phase": "tool"},
	}}
	loopHistory := []agentMessage{{
		ID:      "msg-cycle-state-history",
		Role:    "agent",
		Content: "history message",
		ToolCalls: []agentToolCall{{
			ID:       "tool-cycle-state-history",
			Name:     "history.tool",
			Status:   contracts.StatusSucceeded,
			Result:   map[string]any{"source": "history"},
			Metadata: map[string]interface{}{"phase": "history"},
		}},
	}}

	state := agentPlannerActionReplanCycleStateFromLegacy(responseMessages, toolCalls, loopHistory)

	responseMessages[0].Content = "mutated response"
	responseMessages[0].ToolCalls[0].Result["source"] = "mutated response result"
	responseMessages[0].ToolCalls[0].Error.Message = "mutated response error"
	responseMessages[0].Links[0].Label = "Mutated run"
	responseMessages[0].SuggestedPrompts[0] = "mutated prompt"
	responseMessages[0].RecommendationOptions[0].Title = "Mutated"
	toolCalls[0].Result["source"] = "mutated tool result"
	toolCalls[0].Error.Message = "mutated tool error"
	loopHistory[0].Content = "mutated history"
	loopHistory[0].ToolCalls[0].Metadata["phase"] = "mutated history metadata"

	gotResponses := state.legacyResponseMessages()
	gotTools := state.legacyToolCalls()
	gotHistory := state.legacyLoopHistory()
	require.Equal(t, 1, state.responseMessages().len())
	require.Equal(t, 1, state.toolCalls().len())
	require.Len(t, gotResponses, 1)
	require.Len(t, gotResponses[0].ToolCalls, 1)
	require.Equal(t, "response message", gotResponses[0].Content)
	require.Equal(t, messageAt, gotResponses[0].Timestamp)
	require.Equal(t, "tool", gotResponses[0].Phase)
	require.Equal(t, "cycle state reasoning", gotResponses[0].Reasoning)
	require.Equal(t, "Run", gotResponses[0].Links[0].Label)
	require.Equal(t, "continue cycle state", gotResponses[0].SuggestedPrompts[0])
	require.Equal(t, "Continue", gotResponses[0].RecommendationOptions[0].Title)
	require.Equal(t, "response", gotResponses[0].ToolCalls[0].Result["source"])
	require.Equal(t, "response detail", gotResponses[0].ToolCalls[0].Detail)
	require.Equal(t, startedAt, gotResponses[0].ToolCalls[0].StartedAt)
	require.Equal(t, "response", gotResponses[0].ToolCalls[0].Metadata["phase"])
	require.NotNil(t, gotResponses[0].ToolCalls[0].FinishedAt)
	require.Equal(t, finishedAt, *gotResponses[0].ToolCalls[0].FinishedAt)
	require.NotSame(t, responseMessages[0].ToolCalls[0].FinishedAt, gotResponses[0].ToolCalls[0].FinishedAt)
	require.NotNil(t, gotResponses[0].ToolCalls[0].Error)
	require.Equal(t, services.CodeBadRequest, gotResponses[0].ToolCalls[0].Error.Code)
	require.Equal(t, "response warning", gotResponses[0].ToolCalls[0].Error.Message)
	require.NotSame(t, responseMessages[0].ToolCalls[0].Error, gotResponses[0].ToolCalls[0].Error)
	require.Len(t, gotTools, 1)
	require.Equal(t, "tool", gotTools[0].Result["source"])
	require.Equal(t, "top level detail", gotTools[0].Detail)
	require.Equal(t, startedAt, gotTools[0].StartedAt)
	require.Equal(t, "tool", gotTools[0].Metadata["phase"])
	require.NotNil(t, gotTools[0].Error)
	require.Equal(t, services.CodeInternal, gotTools[0].Error.Code)
	require.Equal(t, "top-level warning", gotTools[0].Error.Message)
	require.Len(t, gotHistory, 1)
	require.Equal(t, "history message", gotHistory[0].Content)
	require.Equal(t, "history", gotHistory[0].ToolCalls[0].Result["source"])
	require.Equal(t, "history", gotHistory[0].ToolCalls[0].Metadata["phase"])

	gotResponses[0].Content = "mutated returned response"
	gotResponses[0].ToolCalls[0].Result["source"] = "mutated returned response result"
	gotResponses[0].ToolCalls[0].Error.Message = "mutated returned response error"
	gotResponses[0].Links[0].Label = "Mutated returned run"
	gotResponses[0].SuggestedPrompts[0] = "mutated returned prompt"
	gotResponses[0].RecommendationOptions[0].Title = "Mutated returned"
	gotTools[0].Result["source"] = "mutated returned tool result"
	gotTools[0].Error.Message = "mutated returned tool error"
	gotHistory[0].Content = "mutated returned history"
	gotHistory[0].ToolCalls[0].Metadata["phase"] = "mutated returned history metadata"

	require.Equal(t, "response message", state.legacyResponseMessages()[0].Content)
	require.Equal(t, "response", state.legacyResponseMessages()[0].ToolCalls[0].Result["source"])
	require.Equal(t, "response warning", state.legacyResponseMessages()[0].ToolCalls[0].Error.Message)
	require.Equal(t, "Run", state.legacyResponseMessages()[0].Links[0].Label)
	require.Equal(t, "continue cycle state", state.legacyResponseMessages()[0].SuggestedPrompts[0])
	require.Equal(t, "Continue", state.legacyResponseMessages()[0].RecommendationOptions[0].Title)
	require.Equal(t, "tool", state.legacyToolCalls()[0].Result["source"])
	require.Equal(t, "top-level warning", state.legacyToolCalls()[0].Error.Message)
	require.Equal(t, "history message", state.legacyLoopHistory()[0].Content)
	require.Equal(t, "history", state.legacyLoopHistory()[0].ToolCalls[0].Metadata["phase"])

	phasedResponses := state.responseMessages().withPhase("step")
	phasedTools := state.toolCalls().withPhase("step")
	require.Equal(t, "response", state.legacyResponseMessages()[0].ToolCalls[0].Metadata["phase"])
	require.Nil(t, state.legacyResponseMessages()[0].ToolCalls[0].Metadata["agent_phase"])
	require.Equal(t, "tool", state.legacyToolCalls()[0].Metadata["phase"])
	require.Nil(t, state.legacyToolCalls()[0].Metadata["agent_phase"])
	require.Equal(t, "step", phasedResponses.legacyAgentMessages()[0].Phase)
	require.Equal(t, "response", phasedResponses.legacyAgentMessages()[0].ToolCalls[0].Metadata["phase"])
	require.Equal(t, "step", phasedResponses.legacyAgentMessages()[0].ToolCalls[0].Metadata["agent_phase"])
	require.Equal(t, "tool", phasedTools.legacyAgentToolCalls()[0].Metadata["phase"])
	require.Equal(t, "step", phasedTools.legacyAgentToolCalls()[0].Metadata["agent_phase"])
	phasedResponses.legacyAgentMessages()[0].Content = "mutated phased response"
	phasedTools.legacyAgentToolCalls()[0].Result["source"] = "mutated phased tool"
	require.Equal(t, "response message", state.legacyResponseMessages()[0].Content)
	require.Equal(t, "tool", state.legacyToolCalls()[0].Result["source"])

	appendedMessage := agentMessage{
		ID:      "msg-cycle-state-appended",
		Role:    "agent",
		Content: "appended message",
		ToolCalls: []agentToolCall{{
			ID:       "tool-cycle-state-appended-message",
			Name:     "appended.message.tool",
			Result:   map[string]any{"source": "appended-message"},
			Metadata: map[string]interface{}{"phase": "appended-message"},
		}},
	}
	appendedTool := agentToolCall{
		ID:       "tool-cycle-state-appended",
		Name:     "appended.tool",
		Result:   map[string]any{"source": "appended-tool"},
		Metadata: map[string]interface{}{"phase": "appended-tool"},
	}
	state.appendResponseMessages(agentPlannerActionReplanCycleMessagesFromAgentMessages([]agentMessage{appendedMessage}))
	state.appendLoopHistory(agentPlannerActionReplanCycleMessagesFromAgentMessages([]agentMessage{appendedMessage}))
	state.appendToolCalls(agentPlannerActionReplanCycleToolCallsFromAgentToolCalls([]agentToolCall{appendedTool}))
	appendedMessage.Content = "mutated appended message"
	appendedMessage.ToolCalls[0].Result["source"] = "mutated appended message result"
	appendedTool.Result["source"] = "mutated appended tool result"

	finalResponses := state.legacyResponseMessages()
	finalTools := state.legacyToolCalls()
	finalHistory := state.legacyLoopHistory()
	require.Len(t, finalResponses, 2)
	require.Equal(t, "appended message", finalResponses[1].Content)
	require.Equal(t, "appended-message", finalResponses[1].ToolCalls[0].Result["source"])
	require.Len(t, finalTools, 2)
	require.Equal(t, "appended-tool", finalTools[1].Result["source"])
	require.Len(t, finalHistory, 2)
	require.Equal(t, "appended message", finalHistory[1].Content)
}

func TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	actionMessage := agentMessage{
		ID:      "msg-cycle-action-snapshot-action",
		Role:    "tool",
		Content: "snapshot action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-cycle-action-snapshot-action",
			Name: "daemon.health",
		}},
	}
	synthesisMessage := agentMessage{ID: "msg-cycle-action-snapshot-synthesis", Role: "agent", Content: "snapshot summarized"}
	synthesisPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "snapshot summarized"}

	cyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "snapshot-action"},
		}},
	})
	require.Len(t, cyclePlan.actions(), 1)
	cyclePlan.item.Actions = nil
	cyclePlan.item.ToolCalls = nil

	actionCalled := false
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		actionCalled = true
		require.Equal(t, "session-cycle-action-snapshot", turn.SessionID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "snapshot-action", stringFromAny(payload.Action.Arguments["probe"]))

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "snapshot action handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(actionMessage.ToolCalls),
			},
		}, nil
	}))

	synthesisCalled := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		synthesisCalled = true
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Len(t, payload.ToolMessages, 1)
		require.Equal(t, actionMessage.ID, payload.ToolMessages[0].ID)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "snapshot synthesis handled",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, nil, synthesisPlan),
		}, nil
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	result := mgr.runAgentPlannerActionReplanCycle(context.Background(), agentPlannerActionReplanCycleRequest{
		Turn:          agentPlannerActionReplanCycleTurnContext("session-cycle-action-snapshot", agentRuntimeTurnRef{TaskID: "task-cycle-action-snapshot", RunID: "run-cycle-action-snapshot", TurnID: "turn-cycle-action-snapshot"}, "daemon health", "user", "read_only"),
		Language:      "en",
		Operator:      "tester",
		LLMRequest:    runtime.AgentLoopLLMRequest{Model: "gpt-cycle-action-snapshot"},
		Authorization: runtime.AgentLoopAuthorization{},
		ToolContext:   agentPlannerActionReplanCycleToolContextFromToolServices(agentLoopToolServiceBindingFromRouterServices(routerServices{})),
		Plan:          cyclePlan,
		State: agentPlannerActionReplanCycleStateFromLegacy(
			[]agentMessage{{ID: "msg-cycle-action-snapshot-plan", Role: "agent", Content: "plan"}},
			nil,
			[]agentMessage{{ID: "history-cycle-action-snapshot", Role: "user", Content: "daemon health"}},
		),
		Options: agentPlannerActionReplanCycleOptions{ActionAppendOrigin: "planner_loop"},
	})

	require.True(t, actionCalled)
	require.True(t, synthesisCalled)
	require.True(t, result.Continue)
	require.False(t, result.Plan.hasActions())
	responses := result.State.legacyResponseMessages()
	require.Len(t, responses, 3)
	require.Equal(t, actionMessage.ID, responses[1].ID)
	require.Equal(t, "step", responses[1].Phase)
	require.Equal(t, synthesisMessage.ID, responses[2].ID)
	require.Len(t, appendMutations, 2)
	require.Equal(t, "planner_loop", appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)
}

func TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	toolServices := agentLoopToolServiceBindingFromRouterServices(routerServices{jobs: jobs})
	planMessage := agentMessage{ID: "msg-planner-cycle-plan", Role: "agent", Content: "planning daemon health", Phase: "plan"}
	planTool := agentToolCall{ID: "tool-planner-cycle-plan", Name: "LLM chat"}
	actionTool := agentToolCall{ID: "tool-planner-cycle-action", Name: "daemon.health"}
	actionMessage := agentMessage{
		ID:        "msg-planner-cycle-action",
		Role:      "tool",
		Content:   "health ok through shared cycle",
		ToolCalls: []agentToolCall{actionTool},
	}
	authProfiles := []agentHTTPAuthProfile{{
		ID:      "auth-planner-cycle",
		Type:    "bearer",
		BaseURL: "https://sandbox.example",
		Token:   "token",
	}}
	synthesisTool := agentToolCall{ID: "tool-planner-cycle-synthesis", Name: "LLM synthesis"}
	synthesisMessage := agentMessage{ID: "msg-planner-cycle-synthesis", Role: "agent", Content: "health summarized", Phase: "step_replan", ToolCalls: []agentToolCall{synthesisTool}}
	actionPlan := &agentPlannerResponse{
		Phase: "plan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "planner-cycle"},
		}},
	}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "health summarized"}

	turnExecutions := []runtime.TurnContext{}
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		turnExecutions = append(turnExecutions, turn)
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, "task-planner-cycle", turn.TaskID)
		require.Equal(t, "run-planner-cycle", turn.RunID)
		require.Equal(t, "turn-planner-cycle", turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Metadata["kind"])
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.Request.Turn.SessionID)
		require.Equal(t, "tester", payload.Request.Operator)
		require.Equal(t, "en", payload.Request.Language)
		require.Equal(t, "read_only", payload.Request.Turn.OperationLevel)
		require.Equal(t, "planner-cycle", stringFromAny(payload.Action.Arguments["probe"]))
		require.Equal(t, agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles), payload.Request.Authorization.AuthProfiles)
		require.Equal(t, "auth-planner-cycle", payload.Request.Authorization.DefaultAuthProfileID)
		payloadJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](payload.Request.ToolServices.ActionServices, runtime.AgentLoopActionServiceJobs)
		require.True(t, ok)
		require.Same(t, jobs, payloadJobs)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "action handled by test executor",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{actionTool}),
			},
		}, nil
	}))

	llmStages := []runtime.LLMStageRequest{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		llmStages = append(llmStages, req)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-planner-cycle", req.Turn.TaskID)
		require.Equal(t, "run-planner-cycle", req.Turn.RunID)
		require.Equal(t, "turn-planner-cycle", req.Turn.TurnID)
		require.Equal(t, "daemon health", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		switch req.Stage {
		case runtime.LLMStagePlannerChat:
			payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
			require.True(t, ok, "payload type = %T", req.Payload)
			require.Equal(t, "gpt-planner-cycle", payload.LLMRequest.Model)
			require.Len(t, payload.History, 1)
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "planner handled by test runner",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{planMessage}, []agentToolCall{planTool}, actionPlan),
			}, nil
		case runtime.LLMStageToolSynthesis:
			payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
			require.True(t, ok, "payload type = %T", req.Payload)
			require.Equal(t, "gpt-planner-cycle", payload.LLMRequest.Model)
			require.Equal(t, agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles), payload.LLMRequest.AuthProfiles)
			require.Equal(t, "auth-planner-cycle", payload.LLMRequest.DefaultAuthProfileID)
			require.Len(t, payload.History, 3)
			require.Len(t, payload.ToolMessages, 1)
			require.Equal(t, actionMessage.ID, payload.ToolMessages[0].ID)
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "synthesis handled by test runner",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, []agentToolCall{synthesisTool}, finalPlan),
			}, nil
		default:
			t.Fatalf("unexpected llm stage %s", req.Stage)
			return runtime.LLMStageResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	messages, toolCalls, err := mgr.runAgentLLMToolLoopWithToolServices(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-planner-cycle", RunID: "run-planner-cycle", TurnID: "turn-planner-cycle"},
		"daemon health",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{
			Model:                "gpt-planner-cycle",
			AuthProfiles:         authProfiles,
			DefaultAuthProfileID: "auth-planner-cycle",
		},
		[]agentMessage{{ID: "history-planner-cycle", Role: "user", Content: "daemon health"}},
		&agentMemorySummary{Summary: "memory"},
		toolServices,
	)

	require.NoError(t, err)
	require.Len(t, llmStages, 2)
	require.Equal(t, runtime.LLMStagePlannerChat, llmStages[0].Stage)
	require.Equal(t, runtime.LLMStageToolSynthesis, llmStages[1].Stage)
	require.Len(t, turnExecutions, 1)
	require.Len(t, appendMutations, 3)
	require.Equal(t, string(runtime.LLMStagePlannerChat), appendMutations[0].Turn.Origin)
	require.Equal(t, "planner_loop", appendMutations[1].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[2].Turn.Origin)
	require.Equal(t, true, appendMutations[0].Turn.Metadata["planner_apply"])
	require.Equal(t, false, appendMutations[1].Turn.Metadata["planner_apply"])
	require.Equal(t, true, appendMutations[2].Turn.Metadata["planner_apply"])

	actionPayload, ok := appendMutations[1].Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendMutations[1].Payload)
	require.Len(t, actionPayload.Messages, 1)
	require.Equal(t, "step", actionPayload.Messages[0].Phase)
	require.Equal(t, "daemon.health", actionPayload.Messages[0].ToolCalls[0].Metadata["action_name"])
	require.NotEmpty(t, actionPayload.Messages[0].ToolCalls[0].Metadata["action_queue_id"])

	require.Len(t, messages, 3)
	require.Equal(t, planMessage.ID, messages[0].ID)
	require.Equal(t, actionMessage.ID, messages[1].ID)
	require.Equal(t, "step", messages[1].Phase)
	require.Equal(t, synthesisMessage.ID, messages[2].ID)
	require.Len(t, toolCalls, 3)
	require.Equal(t, planTool.ID, toolCalls[0].ID)
	require.Equal(t, actionTool.ID, toolCalls[1].ID)
	require.Equal(t, "daemon.health", toolCalls[1].Metadata["action_name"])
	require.Equal(t, synthesisTool.ID, toolCalls[2].ID)
}

func TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	planMessage := agentMessage{ID: "msg-cycle-handle-plan", Role: "agent", Content: "plan first", Phase: "plan"}
	firstActionMessage := agentMessage{
		ID:      "msg-cycle-handle-action-1",
		Role:    "tool",
		Content: "first action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-cycle-handle-action-1",
			Name: "daemon.health",
		}},
	}
	firstSynthesisMessage := agentMessage{ID: "msg-cycle-handle-synthesis-1", Role: "agent", Content: "continue with second", Phase: "step_replan"}
	secondActionMessage := agentMessage{
		ID:      "msg-cycle-handle-action-2",
		Role:    "tool",
		Content: "second action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-cycle-handle-action-2",
			Name: "daemon.health",
		}},
	}
	finalSynthesisMessage := agentMessage{ID: "msg-cycle-handle-synthesis-final", Role: "agent", Content: "done", Phase: "final_answer"}

	firstPlan := &agentPlannerResponse{
		Phase: "plan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "first"},
		}},
	}
	secondPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "second"},
		}},
	}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "done"}

	actionProbes := []string{}
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		probe := stringFromAny(payload.Action.Arguments["probe"])
		actionProbes = append(actionProbes, probe)
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])
		require.Equal(t, "read_only", turn.OperationLevel)

		switch probe {
		case "first":
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "first action handled",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{firstActionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(firstActionMessage.ToolCalls),
				},
			}, nil
		case "second":
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "second action handled",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{secondActionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(secondActionMessage.ToolCalls),
				},
			}, nil
		default:
			t.Fatalf("unexpected action probe %q", probe)
			return runtime.TurnResult{}, nil
		}
	}))

	synthesisCount := 0
	synthesisHistorySnapshots := [][]string{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-cycle-handle", req.Turn.TaskID)
		require.Equal(t, "run-cycle-handle", req.Turn.RunID)
		require.Equal(t, "turn-cycle-handle", req.Turn.TurnID)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		switch req.Stage {
		case runtime.LLMStagePlannerChat:
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "planner handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{planMessage}, nil, firstPlan),
			}, nil
		case runtime.LLMStageToolSynthesis:
			synthesisCount++
			payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
			require.True(t, ok, "payload type = %T", req.Payload)
			require.Len(t, payload.ToolMessages, 1)
			synthesisHistorySnapshots = append(synthesisHistorySnapshots, agentLoopMessageIDs(payload.History))
			switch synthesisCount {
			case 1:
				require.Equal(t, firstActionMessage.ID, payload.ToolMessages[0].ID)
				return runtime.LLMStageResult{
					Status:  runtime.TurnResultFinished,
					Reason:  "first synthesis handled",
					Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{firstSynthesisMessage}, nil, secondPlan),
				}, nil
			case 2:
				require.Equal(t, secondActionMessage.ID, payload.ToolMessages[0].ID)
				return runtime.LLMStageResult{
					Status:  runtime.TurnResultFinished,
					Reason:  "final synthesis handled",
					Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{finalSynthesisMessage}, nil, finalPlan),
				}, nil
			default:
				t.Fatalf("unexpected synthesis count %d", synthesisCount)
				return runtime.LLMStageResult{}, nil
			}
		default:
			t.Fatalf("unexpected llm stage %s", req.Stage)
			return runtime.LLMStageResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	messages, toolCalls, err := mgr.runAgentLLMToolLoopWithToolServices(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-cycle-handle", RunID: "run-cycle-handle", TurnID: "turn-cycle-handle"},
		"daemon health twice",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-cycle-handle"},
		[]agentMessage{{ID: "history-cycle-handle", Role: "user", Content: "daemon health twice"}},
		&agentMemorySummary{Summary: "memory"},
		testAgentLoopToolServices(),
	)

	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, actionProbes)
	require.Equal(t, 2, synthesisCount)
	require.Equal(t, [][]string{
		{"history-cycle-handle", planMessage.ID, firstActionMessage.ID},
		{"history-cycle-handle", planMessage.ID, firstActionMessage.ID, firstSynthesisMessage.ID, secondActionMessage.ID},
	}, synthesisHistorySnapshots)
	require.Len(t, appendMutations, 5)
	require.Equal(t, string(runtime.LLMStagePlannerChat), appendMutations[0].Turn.Origin)
	require.Equal(t, "planner_loop", appendMutations[1].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[2].Turn.Origin)
	require.Equal(t, "planner_loop", appendMutations[3].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[4].Turn.Origin)

	require.Len(t, messages, 5)
	require.Equal(t, planMessage.ID, messages[0].ID)
	require.Equal(t, firstActionMessage.ID, messages[1].ID)
	require.Equal(t, firstSynthesisMessage.ID, messages[2].ID)
	require.Equal(t, secondActionMessage.ID, messages[3].ID)
	require.Equal(t, finalSynthesisMessage.ID, messages[4].ID)
	require.Len(t, toolCalls, 2)
	require.Equal(t, firstActionMessage.ToolCalls[0].ID, toolCalls[0].ID)
	require.Equal(t, secondActionMessage.ToolCalls[0].ID, toolCalls[1].ID)
}

func TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	firstActionMessage := agentMessage{
		ID:      "msg-continuation-state-action-1",
		Role:    "tool",
		Content: "first continuation action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-continuation-state-action-1",
			Name: "daemon.health",
		}},
	}
	firstSynthesisMessage := agentMessage{ID: "msg-continuation-state-synthesis-1", Role: "agent", Content: "continue with second", Phase: "step_replan"}
	secondActionMessage := agentMessage{
		ID:      "msg-continuation-state-action-2",
		Role:    "tool",
		Content: "second continuation action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-continuation-state-action-2",
			Name: "daemon.health",
		}},
	}
	finalSynthesisMessage := agentMessage{ID: "msg-continuation-state-synthesis-final", Role: "agent", Content: "done", Phase: "final_answer"}

	initialPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "first"},
		}},
	}
	secondPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "second"},
		}},
	}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "done"}

	actionProbes := []string{}
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		probe := stringFromAny(payload.Action.Arguments["probe"])
		actionProbes = append(actionProbes, probe)
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)

		switch probe {
		case "first":
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "first continuation action handled",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{firstActionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(firstActionMessage.ToolCalls),
				},
			}, nil
		case "second":
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "second continuation action handled",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{secondActionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(secondActionMessage.ToolCalls),
				},
			}, nil
		default:
			t.Fatalf("unexpected action probe %q", probe)
			return runtime.TurnResult{}, nil
		}
	}))

	synthesisCount := 0
	synthesisHistorySnapshots := [][]string{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-continuation-state", req.Turn.TaskID)
		require.Equal(t, "run-continuation-state", req.Turn.RunID)
		require.Equal(t, "turn-continuation-state", req.Turn.TurnID)
		require.Equal(t, "continue daemon health twice", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		synthesisCount++
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Len(t, payload.ToolMessages, 1)
		synthesisHistorySnapshots = append(synthesisHistorySnapshots, agentLoopMessageIDs(payload.History))
		switch synthesisCount {
		case 1:
			require.Equal(t, firstActionMessage.ID, payload.ToolMessages[0].ID)
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "first continuation synthesis handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{firstSynthesisMessage}, nil, secondPlan),
			}, nil
		case 2:
			require.Equal(t, secondActionMessage.ID, payload.ToolMessages[0].ID)
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "final continuation synthesis handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{finalSynthesisMessage}, nil, finalPlan),
			}, nil
		default:
			t.Fatalf("unexpected synthesis count %d", synthesisCount)
			return runtime.LLMStageResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	messages, toolCalls := mgr.runAgentReplanActionContinuationWithToolServices(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-continuation-state", RunID: "run-continuation-state", TurnID: "turn-continuation-state"},
		"continue daemon health twice",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-continuation-state"},
		[]agentMessage{{ID: "history-continuation-state", Role: "agent", Content: "previous step"}},
		&agentMemorySummary{Summary: "memory"},
		testAgentLoopToolServices(),
		initialPlan,
		3,
	)

	require.Equal(t, []string{"first", "second"}, actionProbes)
	require.Equal(t, 2, synthesisCount)
	require.Equal(t, [][]string{
		{"history-continuation-state", firstActionMessage.ID},
		{"history-continuation-state", firstActionMessage.ID, firstSynthesisMessage.ID, secondActionMessage.ID},
	}, synthesisHistorySnapshots)
	require.Len(t, appendMutations, 4)
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, appendMutations[2].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[3].Turn.Origin)

	require.Len(t, messages, 4)
	require.Equal(t, firstActionMessage.ID, messages[0].ID)
	require.Equal(t, firstSynthesisMessage.ID, messages[1].ID)
	require.Equal(t, secondActionMessage.ID, messages[2].ID)
	require.Equal(t, finalSynthesisMessage.ID, messages[3].ID)
	require.Len(t, toolCalls, 2)
	require.Equal(t, firstActionMessage.ToolCalls[0].ID, toolCalls[0].ID)
	require.Equal(t, secondActionMessage.ToolCalls[0].ID, toolCalls[1].ID)
}

func TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok, "daemon.health missing from registry")
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	actionCalled := false
	spec.Handler = func(_ context.Context, in toolregistry.HandlerInput) (any, error) {
		actionCalled = true
		require.Equal(t, sessionID, in.SessionID)
		require.Equal(t, "tester", in.Operator)
		require.Equal(t, "en", in.Language)
		require.Equal(t, "read_only", in.OperationLevel)
		require.Equal(t, "continuation", stringFromAny(in.RawArgs["probe"]))

		tool := runningAgentToolCall("daemon.health", "continuation")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"ok": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "health ok", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	expectedMessage := agentMessage{ID: "msg-continuation-synthesis", Role: "agent", Content: "continuation summarized"}
	expectedToolCall := agentToolCall{ID: "tool-continuation-synthesis", Name: "LLM synthesis"}
	expectedPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "continuation summarized"}
	appendOrigins := []string{}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-continuation", req.Turn.TaskID)
		require.Equal(t, "run-continuation", req.Turn.RunID)
		require.Equal(t, "turn-continuation", req.Turn.TurnID)
		require.Equal(t, "continue daemon health", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Turn.Metadata["kind"])
		require.Equal(t, 1, req.Turn.Metadata["message_count"])

		payload, ok := req.Payload.(runtime.AgentLoopAppendMessagesPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Len(t, payload.Messages, 1)

		appendOrigins = append(appendOrigins, req.Turn.Origin)
		switch len(appendOrigins) {
		case 1:
			require.Equal(t, daemonTurnExecutionReplanActionContinuation, req.Turn.Origin)
			require.Equal(t, false, req.Turn.Metadata["planner_apply"])
			require.Nil(t, payload.Plan)
			require.Contains(t, payload.Messages[0].Content, "health ok")
		case 2:
			require.Equal(t, string(runtime.LLMStageToolSynthesis), req.Turn.Origin)
			require.Equal(t, true, req.Turn.Metadata["planner_apply"])
			require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), payload.Plan)
			require.Equal(t, expectedMessage.ID, payload.Messages[0].ID)
		default:
			t.Fatalf("unexpected append mutation call %d", len(appendOrigins))
		}

		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled by test mutator",
		}, nil
	}))
	synthesisCalled := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		synthesisCalled = true
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-continuation", req.Turn.TaskID)
		require.Equal(t, "run-continuation", req.Turn.RunID)
		require.Equal(t, "turn-continuation", req.Turn.TurnID)
		require.Equal(t, "continue daemon health", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.Equal(t, "continue daemon health", payload.Turn.Prompt)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Len(t, payload.History, 2)
		require.Len(t, payload.ToolMessages, 1)
		require.Contains(t, payload.ToolMessages[0].Content, "health ok")

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "done",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan),
		}, nil
	}))

	messages, toolCalls := mgr.runAgentReplanActionContinuationWithToolServices(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-continuation", RunID: "run-continuation", TurnID: "turn-continuation"},
		"continue daemon health",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-continuation"},
		[]agentMessage{{ID: "history-1", Role: "agent", Content: "previous step"}},
		&agentMemorySummary{Summary: "memory"},
		testAgentLoopToolServices(),
		&agentPlannerResponse{
			Phase: "step_replan",
			NextAction: &agentNextAction{
				Type:         "continue",
				AutoContinue: true,
			},
			ToolCalls: []agentPlannerAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"probe": "continuation"},
			}},
		},
		1,
	)

	require.True(t, actionCalled)
	require.True(t, synthesisCalled)
	require.Equal(t, []string{daemonTurnExecutionReplanActionContinuation, string(runtime.LLMStageToolSynthesis)}, appendOrigins)
	require.Len(t, messages, 2)
	require.Contains(t, messages[0].Content, "health ok")
	require.Equal(t, agentLoopMessageFromAgentMessage(expectedMessage), messages[1])
	require.Len(t, toolCalls, 2)
	require.Equal(t, "daemon.health", toolCalls[0].Name)
	require.Equal(t, agentLoopToolCallFromAgentToolCall(expectedToolCall), toolCalls[1])
}
