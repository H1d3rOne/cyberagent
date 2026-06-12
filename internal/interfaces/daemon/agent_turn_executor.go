package daemon

import (
	"context"
	"errors"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

const (
	daemonTurnExecutionDeterministicRoute       = "deterministic_route"
	daemonTurnExecutionReplanActionContinuation = "replan_action_continuation"
	daemonTurnExecutionPlannerActionExecution   = "planner_action_execution"
)

type daemonAgentTurnExecutor struct {
	manager *agentManager
}

type daemonTurnExecutionCallerInvalidPayloadEntry struct {
	kind    string
	message string
}

type daemonTurnExecutionCallerPolicy string

const daemonTurnExecutionCallerPolicyStrictResult daemonTurnExecutionCallerPolicy = "strict_result"

type daemonTurnExecutionCallerPolicyEntry struct {
	kind   string
	policy daemonTurnExecutionCallerPolicy
}

type daemonTurnExecutionCallerFallbackReason string

const (
	daemonTurnExecutionCallerFallbackExecutorNotAttached   daemonTurnExecutionCallerFallbackReason = "executor_not_attached"
	daemonTurnExecutionCallerFallbackBlocked               daemonTurnExecutionCallerFallbackReason = "blocked"
	daemonTurnExecutionCallerFallbackRunningWithoutPayload daemonTurnExecutionCallerFallbackReason = "running_without_payload"
)

const daemonTurnExecutionCallerFallbackOwnerLegacyCaller = "legacy_caller"

type daemonTurnExecutionCallerFallbackOwnershipEntry struct {
	reason daemonTurnExecutionCallerFallbackReason
	owner  string
}

type daemonTurnExecutionCallerMigrationStage string

const (
	daemonTurnExecutionCallerMigrationStageActiveTurnExecutor daemonTurnExecutionCallerMigrationStage = "active_turn_executor"
	daemonTurnExecutionCallerMigrationStageAgentLoopInternal  daemonTurnExecutionCallerMigrationStage = "agent_loop_internal_caller"
)

type daemonTurnExecutionCallerMigrationEntry struct {
	kind          string
	stage         daemonTurnExecutionCallerMigrationStage
	currentSeam   string
	target        string
	migrationGate string
}

type daemonTurnExecutionHelperOwner string

const (
	daemonTurnExecutionHelperOwnerRuntime                  daemonTurnExecutionHelperOwner = "runtime"
	daemonTurnExecutionHelperOwnerDaemonTurnRefAdapter     daemonTurnExecutionHelperOwner = "daemon_turn_ref_adapter"
	daemonTurnExecutionHelperOwnerDaemonCompatibilityLayer daemonTurnExecutionHelperOwner = "daemon_compatibility_layer"
)

type daemonTurnExecutionHelperOwnershipEntry struct {
	kind                 string
	callerSurface        string
	requestHelperOwner   daemonTurnExecutionHelperOwner
	turnHelperOwner      daemonTurnExecutionHelperOwner
	payloadHelperOwner   daemonTurnExecutionHelperOwner
	remainingDaemonScope string
	nextMigration        string
}

type daemonTurnExecutionAgentLoopMigrationStage string

const (
	daemonTurnExecutionAgentLoopMigrationStagePreconditionReady   daemonTurnExecutionAgentLoopMigrationStage = "precondition_ready"
	daemonTurnExecutionAgentLoopMigrationStageAgentLoopCallerLive daemonTurnExecutionAgentLoopMigrationStage = "agent_loop_caller_live"
	daemonTurnExecutionAgentLoopMigrationStageBlockedBySideEffect daemonTurnExecutionAgentLoopMigrationStage = "blocked_by_daemon_side_effect"
)

type daemonTurnExecutionAgentLoopMigrationPreconditionEntry struct {
	kind                   string
	stage                  daemonTurnExecutionAgentLoopMigrationStage
	currentCaller          string
	proposedMode           string
	requiredRuntimeHelpers string
	requiredAdapter        string
	remainingDaemonScope   string
	nextStep               string
}

type daemonTurnExecutionAgentLoopMigrationBlockerOwner string

const (
	daemonTurnExecutionAgentLoopMigrationBlockerOwnerDaemonLegacy daemonTurnExecutionAgentLoopMigrationBlockerOwner = "daemon_legacy_side_effect"
	daemonTurnExecutionAgentLoopMigrationBlockerOwnerRuntimeDTO   daemonTurnExecutionAgentLoopMigrationBlockerOwner = "runtime_dto_helper"
)

type daemonTurnExecutionAgentLoopMigrationBlockerEntry struct {
	kind       string
	blocker    string
	owner      daemonTurnExecutionAgentLoopMigrationBlockerOwner
	resolution string
}

type daemonPlannerActionExecutionSideEffectOwner string

const (
	daemonPlannerActionExecutionSideEffectOwnerRuntimeDTO           daemonPlannerActionExecutionSideEffectOwner = "runtime_dto_helper"
	daemonPlannerActionExecutionSideEffectOwnerDaemonLegacy         daemonPlannerActionExecutionSideEffectOwner = "daemon_legacy_side_effect"
	daemonPlannerActionExecutionSideEffectOwnerCompatibilityFixture daemonPlannerActionExecutionSideEffectOwner = "test_compatibility_fixture"
	daemonPlannerActionExecutionSideEffectOwnerRuntimeContract      daemonPlannerActionExecutionSideEffectOwner = "runtime_action_executor_contract"
)

type daemonPlannerActionExecutionSideEffectBoundaryPlanEntry struct {
	boundary     string
	currentOwner daemonPlannerActionExecutionSideEffectOwner
	targetOwner  daemonPlannerActionExecutionSideEffectOwner
	extractedBy  string
	nextStep     string
}

type daemonPlannerActionExecutorContractStage string

const (
	daemonPlannerActionExecutorContractStageReady   daemonPlannerActionExecutorContractStage = "ready"
	daemonPlannerActionExecutorContractStageBlocked daemonPlannerActionExecutorContractStage = "blocked"
)

type daemonPlannerActionExecutorContractPreconditionEntry struct {
	component        string
	stage            daemonPlannerActionExecutorContractStage
	currentSurface   string
	requiredContract string
	blocker          string
	nextStep         string
}

type daemonPlannerActionExecutionLegacySurfaceStage string

const (
	daemonPlannerActionExecutionLegacySurfaceStageExecutorAdapterImplementationOnly daemonPlannerActionExecutionLegacySurfaceStage = "executor_adapter_implementation_only"
	daemonPlannerActionExecutionLegacySurfaceStageRetired                           daemonPlannerActionExecutionLegacySurfaceStage = "retired"
)

type daemonPlannerActionExecutionLegacySurfaceEntry struct {
	symbol                  string
	stage                   daemonPlannerActionExecutionLegacySurfaceStage
	allowedNonTestSurface   string
	currentAdapter          string
	newCallerContract       string
	runtimeMigrationBlocker string
	removalGate             string
}

type daemonPlannerActionExecutionFallbackReason string

const (
	daemonPlannerActionExecutionFallbackEngineNotAttached         daemonPlannerActionExecutionFallbackReason = "runtime_engine_not_attached"
	daemonPlannerActionExecutionFallbackActionExecutorNotAttached daemonPlannerActionExecutionFallbackReason = "agent_loop_action_executor_not_attached"
	daemonPlannerActionExecutionFallbackDirectLegacyExecutor      daemonPlannerActionExecutionFallbackReason = "direct_legacy_executor"
)

const daemonPlannerActionExecutionFallbackOwnerLegacyCaller = "legacy_caller"

const (
	daemonPlannerActionExecutionFallbackStageActionExecutor = "action_executor"
	daemonPlannerActionExecutionFallbackStageDirectLegacy   = "direct_legacy_executor"
)

type daemonPlannerActionExecutionFallbackOwnershipEntry struct {
	reason daemonPlannerActionExecutionFallbackReason
	owner  string
}

type daemonPlannerActionExecutionFallbackBoundaryEntry struct {
	stage          string
	callerSurface  string
	runtimeSurface string
	legacyFallback string
	owner          string
	scope          string
}

type daemonPlannerActionExecutionRuntimeActionServiceOwnershipStage string

const (
	daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityOwned           daemonPlannerActionExecutionRuntimeActionServiceOwnershipStage = "runtime_capability_owned"
	daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached daemonPlannerActionExecutionRuntimeActionServiceOwnershipStage = "runtime_capability_adapter_attached"
	daemonPlannerActionExecutionRuntimeActionServiceStageDaemonManagerOwned               daemonPlannerActionExecutionRuntimeActionServiceOwnershipStage = "daemon_manager_owned"
	daemonPlannerActionExecutionRuntimeActionServiceStageSelectedNextCapability           daemonPlannerActionExecutionRuntimeActionServiceOwnershipStage = "selected_next_runtime_capability"
)

const (
	daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability = "runtime_action_service_capability"
	daemonPlannerActionExecutionRuntimeActionServiceOwnerDaemonManager     = "daemon_registry_manager"
)

type daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntry struct {
	component          string
	stage              daemonPlannerActionExecutionRuntimeActionServiceOwnershipStage
	actions            []string
	managerMethods     []string
	currentOwner       string
	currentSurface     string
	targetCapabilities []runtime.AgentLoopActionServiceCapability
	evidence           string
	nextStep           string
}

type daemonPlannerActionExecutionRegistryAdapterCompletionStage string

const (
	daemonPlannerActionExecutionRegistryAdapterCompletionStageAdaptersComplete       daemonPlannerActionExecutionRegistryAdapterCompletionStage = "registry_action_service_adapters_complete"
	daemonPlannerActionExecutionRegistryAdapterCompletionStageSourceAuditClear       daemonPlannerActionExecutionRegistryAdapterCompletionStage = "registry_handler_source_direct_manager_calls_clear"
	daemonPlannerActionExecutionRegistryAdapterCompletionStageFallbackBoundaryChosen daemonPlannerActionExecutionRegistryAdapterCompletionStage = "planner_action_executor_fallback_boundary_chosen"
)

const daemonPlannerActionExecutionNextFallbackRetirementBoundaryTurnExecutorCompatibility = "turn_executor_compatibility_fallback_retirement_readiness"
const daemonPlannerActionExecutionNextFallbackRetirementBoundaryDirectLegacyExecutor = "direct_legacy_executor_fallback_retirement_readiness"

type daemonPlannerActionExecutionRegistryAdapterCompletionEntry struct {
	component    string
	stage        daemonPlannerActionExecutionRegistryAdapterCompletionStage
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonPlannerActionExecutionTurnCompatibilityRetirementStage string

const (
	daemonPlannerActionExecutionTurnCompatibilityRetirementStageInternalFallbackRetired daemonPlannerActionExecutionTurnCompatibilityRetirementStage = "planner_action_execution_internal_turn_executor_fallback_retired"
	daemonPlannerActionExecutionTurnCompatibilityRetirementStageExternalCompatibility   daemonPlannerActionExecutionTurnCompatibilityRetirementStage = "external_turn_executor_compatibility_retained"
	daemonPlannerActionExecutionTurnCompatibilityRetirementStageNextBoundaryChosen      daemonPlannerActionExecutionTurnCompatibilityRetirementStage = "direct_legacy_executor_fallback_boundary_chosen"
)

type daemonPlannerActionExecutionTurnCompatibilityRetirementEntry struct {
	component    string
	stage        daemonPlannerActionExecutionTurnCompatibilityRetirementStage
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonPlannerActionExecutionDirectLegacyRetirementStage string

const (
	daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionExecutorPrimary  daemonPlannerActionExecutionDirectLegacyRetirementStage = "runtime_action_executor_primary"
	daemonPlannerActionExecutionDirectLegacyRetirementStageDirectExecutorSurfaceScoped   daemonPlannerActionExecutionDirectLegacyRetirementStage = "direct_legacy_executor_surface_scoped"
	daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionServicesCovered  daemonPlannerActionExecutionDirectLegacyRetirementStage = "runtime_action_service_adapters_covered"
	daemonPlannerActionExecutionDirectLegacyRetirementStageCompatibilityFallbackRetained daemonPlannerActionExecutionDirectLegacyRetirementStage = "compatibility_fallback_retained"
	daemonPlannerActionExecutionDirectLegacyRetirementStageNextBoundaryChosen            daemonPlannerActionExecutionDirectLegacyRetirementStage = "no_engine_direct_fallback_boundary_chosen"
)

const daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy = "no_engine_agent_loop_direct_fallback_retirement_readiness"

type daemonPlannerActionExecutionDirectLegacyRetirementEntry struct {
	component    string
	stage        daemonPlannerActionExecutionDirectLegacyRetirementStage
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonPlannerActionExecutionActionExecutorMissingRetirementStage string

const (
	daemonPlannerActionExecutionActionExecutorMissingRetirementStageNoEngineFallbackRetained daemonPlannerActionExecutionActionExecutorMissingRetirementStage = "no_engine_agent_loop_direct_fallback_retained"
	daemonPlannerActionExecutionActionExecutorMissingRetirementStageMissingExecutorBlocked   daemonPlannerActionExecutionActionExecutorMissingRetirementStage = "attached_engine_missing_action_executor_blocked"
	daemonPlannerActionExecutionActionExecutorMissingRetirementStageDetachedResultBlocked    daemonPlannerActionExecutionActionExecutorMissingRetirementStage = "detached_action_executor_result_blocked"
	daemonPlannerActionExecutionActionExecutorMissingRetirementStageNextBoundaryChosen       daemonPlannerActionExecutionActionExecutorMissingRetirementStage = "no_engine_direct_fallback_boundary_chosen"
)

const (
	daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionDirectLegacyFallback = "direct_legacy_fallback"
	daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionBlockedError         = "blocked_error"
)

type daemonPlannerActionExecutionActionExecutorMissingRetirementEntry struct {
	component    string
	stage        daemonPlannerActionExecutionActionExecutorMissingRetirementStage
	reason       daemonPlannerActionExecutionFallbackReason
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonPlannerActionExecutorServiceContractMigrationStage string

const (
	daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded  daemonPlannerActionExecutorServiceContractMigrationStage = "runtime_contract_landed"
	daemonPlannerActionExecutorServiceContractStageDaemonAdapterPending   daemonPlannerActionExecutorServiceContractMigrationStage = "daemon_adapter_pending"
	daemonPlannerActionExecutorServiceContractStageDaemonAdapterAttached  daemonPlannerActionExecutorServiceContractMigrationStage = "daemon_adapter_attached"
	daemonPlannerActionExecutorServiceContractStageBlockedByLegacyBinding daemonPlannerActionExecutorServiceContractMigrationStage = "blocked_by_legacy_binding"
)

type daemonPlannerActionExecutorServiceContractMigrationEntry struct {
	component      string
	stage          daemonPlannerActionExecutorServiceContractMigrationStage
	currentSurface string
	targetSurface  string
	evidence       string
	nextStep       string
}

type daemonAgentLoopRunnerCompletionOwner string

const (
	daemonAgentLoopRunnerCompletionOwnerRuntimeSeam                daemonAgentLoopRunnerCompletionOwner = "runtime_seam"
	daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback    daemonAgentLoopRunnerCompletionOwner = "runtime_seam_with_daemon_fallback"
	daemonAgentLoopRunnerCompletionOwnerDaemonAdapter              daemonAgentLoopRunnerCompletionOwner = "daemon_adapter"
	daemonAgentLoopRunnerCompletionOwnerDaemonAdapterViaRuntimeDTO daemonAgentLoopRunnerCompletionOwner = "daemon_adapter_via_runtime_dto"
	daemonAgentLoopRunnerCompletionOwnerNotApplicable              daemonAgentLoopRunnerCompletionOwner = "not_applicable"
)

type daemonAgentLoopRunnerCompletionAuditEntry struct {
	mode                 runtime.AgentLoopMode
	runnerBranch         string
	appendPersistOwner   daemonAgentLoopRunnerCompletionOwner
	toolSynthesisOwner   daemonAgentLoopRunnerCompletionOwner
	actionExecutionOwner daemonAgentLoopRunnerCompletionOwner
	fallbackOwner        daemonAgentLoopRunnerCompletionOwner
	currentSurface       string
	nextRuntimeSeam      string
}

const (
	daemonAgentLoopRunnerCompletionNextRuntimeSeamSharedCycleAppendPersist        = "shared_cycle_append_persist"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamToolSynthesisFallback           = "tool_synthesis_fallback_ownership"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamAgentLoopDirectFallback         = "agent_loop_direct_fallback_ownership"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamActionExecutionFallback         = "planner_action_execution_fallback_chain_ownership"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamActionServiceOwnership          = "planner_action_execution_runtime_action_service_ownership"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamNestedReplanCycleDispatch       = "planner_action_replan_cycle_nested_dispatch_fallback"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamSessionMutationFallback         = "shared_cycle_session_mutation_dispatch_fallback"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamAttachedSessionMutator          = "shared_cycle_attached_session_mutator_fallback_retirement_readiness"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamSessionMutationBlocked          = "session_mutation_blocked_or_error_fallback_retirement_readiness"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamSessionMutationOptionalResult   = "session_mutation_optional_result_fallback_retirement_readiness"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamSessionMutationNoEngineFallback = "session_mutation_no_engine_legacy_fallback_retirement_readiness"
	daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate        = "await_explicit_user_stage_commit_tag_authorization"
)

type daemonPlannerActionExecutionSideEffectBoundary struct {
	sessionID      string
	language       string
	operator       string
	operationLevel string
	toolServices   runtime.AgentLoopToolServiceBinding
	action         agentPlannerAction
	authorization  runtime.AgentLoopAuthorization
}

type daemonPlannerActionExecutionExecutor interface {
	ExecutePlannerAction(ctx context.Context, boundary daemonPlannerActionExecutionSideEffectBoundary) (runtime.AgentLoopActionExecutorResult, error)
}

type daemonLegacyPlannerActionExecutionExecutor struct {
	manager *agentManager
}

const daemonTurnExecutionCallerDefaultInvalidPayloadError = "daemon: turn executor returned invalid payload"

var daemonTurnExecutionCallerInvalidPayloadTable = [...]daemonTurnExecutionCallerInvalidPayloadEntry{
	{kind: daemonTurnExecutionDeterministicRoute, message: "daemon: deterministic route turn executor returned invalid payload"},
	{kind: daemonTurnExecutionPlannerActionExecution, message: "daemon: planner action execution turn executor returned invalid payload"},
}

var daemonTurnExecutionCallerPolicyTable = [...]daemonTurnExecutionCallerPolicyEntry{
	{kind: daemonTurnExecutionDeterministicRoute, policy: daemonTurnExecutionCallerPolicyStrictResult},
	{kind: daemonTurnExecutionPlannerActionExecution, policy: daemonTurnExecutionCallerPolicyStrictResult},
}

var daemonTurnExecutionCallerFallbackOwnershipTable = [...]daemonTurnExecutionCallerFallbackOwnershipEntry{
	{reason: daemonTurnExecutionCallerFallbackExecutorNotAttached, owner: daemonTurnExecutionCallerFallbackOwnerLegacyCaller},
	{reason: daemonTurnExecutionCallerFallbackBlocked, owner: daemonTurnExecutionCallerFallbackOwnerLegacyCaller},
	{reason: daemonTurnExecutionCallerFallbackRunningWithoutPayload, owner: daemonTurnExecutionCallerFallbackOwnerLegacyCaller},
}

var daemonTurnExecutionCallerMigrationTable = [...]daemonTurnExecutionCallerMigrationEntry{
	{
		kind:          daemonTurnExecutionDeterministicRoute,
		stage:         daemonTurnExecutionCallerMigrationStageAgentLoopInternal,
		currentSeam:   "runtime.Engine.RunAgentLoop",
		target:        "runtime-native deterministic route",
		migrationGate: "deterministic route registry dispatch now uses runtime AgentLoopToolServiceBinding; retire TurnExecutor compatibility after response egress and legacy route adapter cleanup",
	},
	{
		kind:          daemonTurnExecutionPlannerActionExecution,
		stage:         daemonTurnExecutionCallerMigrationStageAgentLoopInternal,
		currentSeam:   "runtime.Engine.RunAgentLoop",
		target:        "runtime-native action execution",
		migrationGate: "move executeAgentPlannerAction side effects and result egress runtime-side before retiring planner action TurnExecutor compatibility",
	},
}

var daemonTurnExecutionHelperOwnershipTable = [...]daemonTurnExecutionHelperOwnershipEntry{
	{
		kind:                 daemonTurnExecutionDeterministicRoute,
		callerSurface:        "internal_agent_loop_caller",
		requestHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
		turnHelperOwner:      daemonTurnExecutionHelperOwnerRuntime,
		payloadHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
		remainingDaemonScope: "turn_ref_adapter_and_legacy_route_agent_loop_adapter",
		nextMigration:        "keep deterministic route registry dispatch on runtime tool services while retiring response egress and external TurnExecutor compatibility",
	},
	{
		kind:                 daemonTurnExecutionPlannerActionExecution,
		callerSurface:        "internal_agent_loop_caller",
		requestHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
		turnHelperOwner:      daemonTurnExecutionHelperOwnerRuntime,
		payloadHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
		remainingDaemonScope: "turn_ref_adapter_legacy_action_side_effects_and_turn_executor_compatibility",
		nextMigration:        "move executeAgentPlannerAction side effects and result egress behind runtime-native action execution; keep TurnExecutor branch as compatibility until seam retirement",
	},
	{
		kind:                 daemonTurnExecutionReplanActionContinuation,
		callerSurface:        "external_compatibility_trampoline",
		requestHelperOwner:   daemonTurnExecutionHelperOwnerDaemonCompatibilityLayer,
		turnHelperOwner:      daemonTurnExecutionHelperOwnerRuntime,
		payloadHelperOwner:   daemonTurnExecutionHelperOwnerRuntime,
		remainingDaemonScope: "legacy AgentLoop request adapter and external TurnExecutor compatibility",
		nextMigration:        "not an internal ExecuteTurn migration candidate; remove only after compatibility seam is retired",
	},
}

var daemonTurnExecutionAgentLoopMigrationPreconditionTable = [...]daemonTurnExecutionAgentLoopMigrationPreconditionEntry{
	{
		kind:                   daemonTurnExecutionDeterministicRoute,
		stage:                  daemonTurnExecutionAgentLoopMigrationStageAgentLoopCallerLive,
		currentCaller:          "agentManager.runRuntimeDeterministicRouteWithToolServices -> runtime.Engine.RunAgentLoop",
		proposedMode:           "AgentLoopModeDeterministicRoute",
		requiredRuntimeHelpers: "AgentLoopTurnExecutionRequest, AgentLoopTurnExecutionTurnFromRequest, AgentLoopTurnResultPayload",
		requiredAdapter:        "daemonAgentLoopRunner deterministic_route branch preserving routePromptForTurnWithToolServices parity",
		remainingDaemonScope:   "route response egress conversion and upstream routerServices-owned non-route services; registry dispatch uses runtime AgentLoopToolServiceBinding",
		nextStep:               "retire deterministic route TurnExecutor compatibility after route response egress and remaining legacy route adapter cleanup",
	},
	{
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

var daemonTurnExecutionAgentLoopMigrationBlockerTable = [...]daemonTurnExecutionAgentLoopMigrationBlockerEntry{
	{
		kind:       daemonTurnExecutionPlannerActionExecution,
		blocker:    "executeAgentPlannerAction side effects",
		owner:      daemonTurnExecutionAgentLoopMigrationBlockerOwnerDaemonLegacy,
		resolution: "move tool execution/service dispatch behind a runtime action executor boundary",
	},
	{
		kind:       daemonTurnExecutionPlannerActionExecution,
		blocker:    "legacy routerServices handle unwrap",
		owner:      daemonTurnExecutionAgentLoopMigrationBlockerOwnerDaemonLegacy,
		resolution: "replace daemon routerServices unwrapping with a runtime AgentLoopToolServiceBinding executor contract",
	},
	{
		kind:       daemonTurnExecutionPlannerActionExecution,
		blocker:    "action request/result DTO",
		owner:      daemonTurnExecutionAgentLoopMigrationBlockerOwnerRuntimeDTO,
		resolution: "runtime DTO helpers already cover request, turn payload, result payload and clone boundaries",
	},
}

var daemonPlannerActionExecutionSideEffectBoundaryPlanTable = [...]daemonPlannerActionExecutionSideEffectBoundaryPlanEntry{
	{
		boundary:     "request_turn_action_authorization_dto",
		currentOwner: daemonPlannerActionExecutionSideEffectOwnerRuntimeDTO,
		targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		extractedBy:  "daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop clones AgentLoopRequest, AgentLoopAction and AgentLoopAuthorization",
		nextStep:     "keep runtime DTOs as the AgentLoop mode input when introducing the planner action executor contract",
	},
	{
		boundary:     "tool_service_dispatch",
		currentOwner: daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		extractedBy:  "daemon planner action executor and deterministic route now pass cloned runtime AgentLoopToolServiceBinding directly into registry dispatch without routerServices reconstruction",
		nextStep:     "keep production dispatch on runtime tool service bindings and test coverage on direct ActionServices capabilities",
	},
	{
		boundary:     "tool_service_provider_compatibility_fixture",
		currentOwner: daemonPlannerActionExecutionSideEffectOwnerCompatibilityFixture,
		targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		extractedBy:  "historical provider/routerServices roundtrip contract is retired; production binding carries runtime AgentLoopActionServiceSet and tests assert capabilities directly",
		nextStep:     "keep the test suite free of explicit Handle/provider roundtrip fixtures",
	},
	{
		boundary:     "http_auth_profile_adapter",
		currentOwner: daemonPlannerActionExecutionSideEffectOwnerDaemonLegacy,
		targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		extractedBy:  "daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop carries AgentLoopAuthorization directly through the executor and registry dispatch signatures",
		nextStep:     "keep legacy registry runtime auth-profile compatibility fields read-only while replacing remaining daemon-owned result egress",
	},
	{
		boundary:     "runtime_result_composition",
		currentOwner: daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		extractedBy:  "runtime.AgentLoopPlannerActionExecutionResultFromActionExecutorResult and FromTurnPayload build the AgentLoopResult envelope",
		nextStep:     "keep planner_action_execution AgentLoop result composition runtime-owned while legacy adapters shrink",
	},
	{
		boundary:     "legacy_action_result_adapter",
		currentOwner: daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		targetOwner:  daemonPlannerActionExecutionSideEffectOwnerRuntimeContract,
		extractedBy:  "executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices return runtime AgentLoopActionExecutorResult directly while preserving runtime tool services; legacy routerServices-ingress wrappers are retired",
		nextStep:     "continue shrinking tool-service dispatch adapters behind runtime-native action service interfaces",
	},
}

var daemonPlannerActionExecutorContractPreconditionTable = [...]daemonPlannerActionExecutorContractPreconditionEntry{
	{
		component:        "action_request_dto",
		stage:            daemonPlannerActionExecutorContractStageReady,
		currentSurface:   "AgentLoopActionExecutionRequest + AgentLoopActionExecutionTurn + AgentLoopTurnResultPayload",
		requiredContract: "runtime action executor input can reuse existing request/action/authorization DTOs",
		nextStep:         "keep DTO contract stable while migrating the executor dispatch surface",
	},
	{
		component:        "typed_tool_service_binding",
		stage:            daemonPlannerActionExecutorContractStageReady,
		currentSurface:   "planner action side-effect boundary carries cloned AgentLoopToolServiceBinding; ActionServices carries runtime capability names and Handle is no longer populated from routerServices by default",
		requiredContract: "runtime action service capability set available after retiring test-only provider compatibility fixtures",
		nextStep:         "keep registry handlers on runtime action service capability consumers",
	},
	{
		component:        "registry_handler_service_consumers",
		stage:            daemonPlannerActionExecutorContractStageReady,
		currentSurface:   "builtin registry handlers consume agentRegistryHandlerRuntime.actionServiceResolver(); production registry runtime no longer exposes toolServiceProvider()/legacyRouterServices() helper methods",
		requiredContract: "registry handlers consume runtime AgentLoopActionServiceSet capabilities without opaque Handle fallback",
		nextStep:         "keep provider/routerServices reconstruction retired from production and test runtime paths",
	},
	{
		component:        "authorization_context",
		stage:            daemonPlannerActionExecutorContractStageReady,
		currentSurface:   "planner action execution carries runtime.AgentLoopAuthorization through executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices; http.request resolves credentials from agentRegistryHandlerRuntime.Authorization",
		requiredContract: "action handlers consume AgentLoopAuthorization or a runtime auth resolver directly",
		nextStep:         "remove legacy registry runtime auth-profile compatibility fields once all handler/test callers consume Authorization only",
	},
	{
		component:        "executor_result_egress",
		stage:            daemonPlannerActionExecutorContractStageReady,
		currentSurface:   "executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices return runtime.AgentLoopActionExecutorResult; registryHandlerOutputToActionExecutorResult builds runtime messages/tool calls directly from handler outputs",
		requiredContract: "action executor implementation returns AgentLoopActionExecutorResult-compatible messages/tool calls without daemon-private result types",
		nextStep:         "replace remaining daemon-private tool-service dispatch with runtime-native action service interfaces",
	},
}

var daemonPlannerActionExecutionLegacySurfaceTable = [...]daemonPlannerActionExecutionLegacySurfaceEntry{
	{
		symbol:                  "executeAgentPlannerAction",
		stage:                   daemonPlannerActionExecutionLegacySurfaceStageRetired,
		allowedNonTestSurface:   "none; direct legacy message/tool-call compatibility return has been removed",
		currentAdapter:          "daemonPlannerActionExecutionExecutor",
		newCallerContract:       "new internal callers must use daemonPlannerActionExecutionExecutorForManager(...).ExecutePlannerAction and consume runtime.AgentLoopActionExecutorResult",
		runtimeMigrationBlocker: "retired; direct callers/tests consume executeAgentPlannerActionResultWithToolServices or the runtime action executor contract",
		removalGate:             "already removed after direct callers/tests moved to runtime result wrapper",
	},
}

var daemonPlannerActionExecutionFallbackOwnershipTable = [...]daemonPlannerActionExecutionFallbackOwnershipEntry{
	{reason: daemonPlannerActionExecutionFallbackEngineNotAttached, owner: daemonPlannerActionExecutionFallbackOwnerLegacyCaller},
	{reason: daemonPlannerActionExecutionFallbackDirectLegacyExecutor, owner: daemonPlannerActionExecutionFallbackOwnerLegacyCaller},
}

var daemonPlannerActionExecutionFallbackBoundaryTable = [...]daemonPlannerActionExecutionFallbackBoundaryEntry{
	{
		stage:          daemonPlannerActionExecutionFallbackStageActionExecutor,
		callerSurface:  "runAgentPlannerActionExecutionAgentLoopResult -> runRuntimePlannerActionExecutionActionExecutor",
		runtimeSurface: "runtime.Engine.ExecuteAgentLoopAction",
		legacyFallback: "daemonAgentLoopActionExecutor.ExecuteAgentLoopAction",
		owner:          daemonPlannerActionExecutionFallbackOwnerLegacyCaller,
		scope:          "planner_action_execution falls back directly to the daemon action executor only when the runtime engine is missing; attached-engine missing/detached AgentLoopActionExecutor returns blocked/error and the internal TurnExecutor compatibility hop is retired",
	},
	{
		stage:          daemonPlannerActionExecutionFallbackStageDirectLegacy,
		callerSurface:  "runAgentPlannerActionExecutionAgentLoopResult -> daemonAgentLoopActionExecutor.ExecuteAgentLoopAction",
		runtimeSurface: "daemonAgentLoopActionExecutor",
		legacyFallback: "daemonPlannerActionExecutionExecutor.ExecutePlannerAction",
		owner:          daemonPlannerActionExecutionFallbackOwnerLegacyCaller,
		scope:          "final compatibility executor preserves daemon registry dispatch side effects after the runtime action executor declines handling",
	},
}

var daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable = [...]daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntry{
	{
		component: "scan_registry_handler",
		stage:     daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached,
		actions: []string{
			"scan.web.safe",
		},
		managerMethods: []string{
			"handleAgentScan",
		},
		currentOwner:       daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability,
		currentSurface:     "scan.web.safe resolves runtime AgentLoopActionServiceWebScan through rt.actionServiceResolver().WebScan(); daemonWebScanManagerActionService is the adapter that forwards safe web scan side effects to handleAgentScan while preserving JobManager resolution through the runtime resolver",
		targetCapabilities: []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceWebScan},
		evidence:           "scan.web.safe now calls webScanActionServiceFromRegistryRuntime; executeAgentRegistryHandlerResultWithToolServices attaches the daemon web scan adapter as a runtime capability before handler invocation",
		nextStep:           "keep scan.web.safe capability-adapter-backed while registry action-service adapter completion is audited",
	},
	{
		component: "health_plugins_runs_registry_handlers",
		stage:     daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached,
		actions: []string{
			"daemon.health",
			"plugins.list",
			"runs.list",
		},
		managerMethods: []string{
			"handleAgentHealth",
			"handleAgentPlugins",
			"handleAgentRuns",
		},
		currentOwner:       daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability,
		currentSurface:     "daemon.health, plugins.list and runs.list resolve runtime AgentLoopActionServiceDaemonStatusAction through rt.actionServiceResolver().DaemonStatus(); daemonStatusManagerActionService is the adapter that forwards status orchestration to handleAgentHealth/Plugins/Runs while preserving VersionService/PluginService/RunService resolution through the runtime resolver",
		targetCapabilities: []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceDaemonStatusAction},
		evidence:           "daemon.health/plugins.list/runs.list now call daemonStatusActionServiceFromRegistryRuntime; executeAgentRegistryHandlerResultWithToolServices attaches the daemon status action adapter as a runtime capability before handler invocation",
		nextStep:           "keep daemon status capability-adapter-backed while registry action-service adapter completion is audited",
	},
	{
		component: "schedule_registry_handlers",
		stage:     daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached,
		actions: []string{
			"schedule.create",
			"schedule.list",
			"schedule.delete",
		},
		managerMethods: []string{
			"handleAgentScheduleCreate",
			"handleAgentScheduleDelete",
			"handleAgentScheduleList",
		},
		currentOwner:       daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability,
		currentSurface:     "schedule.create/list/delete resolve runtime AgentLoopActionServiceScheduleAction through rt.actionServiceResolver().ScheduleAction(); daemonScheduleManagerActionService is the adapter that forwards schedule orchestration to handleAgentScheduleCreate/List/Delete while preserving ScheduleManager resolution through the runtime resolver",
		targetCapabilities: []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceScheduleAction},
		evidence:           "schedule.* now calls scheduleActionServiceFromRegistryRuntime; executeAgentRegistryHandlerResultWithToolServices attaches the daemon schedule action adapter as a runtime capability before handler invocation",
		nextStep:           "keep schedule.* capability-adapter-backed while registry action-service adapter completion is audited",
	},
	{
		component: "web_search_registry_handler",
		stage:     daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached,
		actions: []string{
			"web.search",
		},
		managerMethods: []string{
			"handleAgentWebSearch",
		},
		currentOwner:       daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability,
		currentSurface:     "web.search resolves runtime AgentLoopActionServiceWebSearch through rt.actionServiceResolver().WebSearch(); daemonWebSearchManagerActionService is the adapter that forwards search side effects to handleAgentWebSearch while preserving ProviderStore resolution through the runtime resolver",
		targetCapabilities: []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceWebSearch},
		evidence:           "web.search now calls webSearchActionServiceFromRegistryRuntime; executeAgentRegistryHandlerResultWithToolServices attaches the daemon web search adapter as a runtime capability before handler invocation",
		nextStep:           "keep web.search capability-adapter-backed while registry action-service adapter completion is audited",
	},
	{
		component: "run_context_approval_registry_handlers",
		stage:     daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached,
		actions: []string{
			"runs.context",
			"reports.draft",
			"runs.approvals.list",
			"runs.approvals.approve",
			"runs.approvals.deny",
			"runs.resume_approved",
		},
		managerMethods: []string{
			"handleAgentApprovalDecision",
			"handleAgentApprovals",
			"handleAgentReportDraft",
			"handleAgentResumeApproved",
			"handleAgentRunContextAnalysis",
			"handleAgentRunContextTool",
		},
		currentOwner:       daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability,
		currentSurface:     "runs.context, reports.draft, runs.approvals.* and runs.resume_approved resolve runtime AgentLoopActionServiceRunContextApproval through rt.actionServiceResolver().RunContextApproval(); daemonRunContextApprovalManagerActionService is the adapter that forwards run-context and approval orchestration while preserving RunService/ApprovalService resolution through the runtime resolver",
		targetCapabilities: []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceRunContextApproval},
		evidence:           "run-context/approval handlers now call runContextApprovalActionServiceFromRegistryRuntime; executeAgentRegistryHandlerResultWithToolServices attaches the daemon run-context approval adapter as a runtime capability before handler invocation",
		nextStep:           "registry action-service adapter completion is audited; next boundary is TurnExecutor compatibility fallback retirement readiness",
	},
	{
		component: "workspace_registry_handlers",
		stage:     daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached,
		actions: []string{
			"workspace.command",
			"workspace.files.list",
			"workspace.files.tree",
			"workspace.files.read",
			"workspace.search",
			"workspace.files.write",
			"workspace.files.patch",
			"workspace.files.diff",
			"workspace.files.diff_many",
			"workspace.files.rebase",
			"workspace.files.apply",
			"workspace.files.discard",
			"workspace.files.delete",
			"workspace.files.move",
			"workspace.files.copy",
			"workspace.files.mkdir",
		},
		managerMethods: []string{
			"handleAgentCommand",
			"handleAgentWorkspaceApply",
			"handleAgentWorkspaceApplyAll",
			"handleAgentWorkspaceApplyGroup",
			"handleAgentWorkspaceCopy",
			"handleAgentWorkspaceDelete",
			"handleAgentWorkspaceDiff",
			"handleAgentWorkspaceDiffMany",
			"handleAgentWorkspaceDiscard",
			"handleAgentWorkspaceDiscardAll",
			"handleAgentWorkspaceDiscardGroup",
			"handleAgentWorkspaceList",
			"handleAgentWorkspaceMkdir",
			"handleAgentWorkspaceMove",
			"handleAgentWorkspacePatch",
			"handleAgentWorkspaceRead",
			"handleAgentWorkspaceRebase",
			"handleAgentWorkspaceSearch",
			"handleAgentWorkspaceTree",
			"handleAgentWorkspaceWrite",
		},
		currentOwner:       daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability,
		currentSurface:     "workspace registry handlers resolve runtime AgentLoopActionServiceWorkspace through rt.actionServiceResolver().Workspace(); daemonWorkspaceManagerActionService is the adapter that forwards workspace side effects to manager methods",
		targetCapabilities: []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceWorkspace},
		evidence:           "workspace.command, workspace.files.* and workspace.search now call workspaceActionServiceFromRegistryRuntime; executeAgentRegistryHandlerResultWithToolServices attaches the daemon workspace adapter as a runtime capability before handler invocation",
		nextStep:           "keep workspace registry handlers capability-owned while the next smallest remaining side-effect group moves to a network request capability",
	},
	{
		component: "network_request_registry_handlers",
		stage:     daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached,
		actions: []string{
			"http.request",
			"web.fetch",
		},
		managerMethods: []string{
			"handleAgentHTTPRequest",
			"handleAgentWebFetch",
		},
		currentOwner:       daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability,
		currentSurface:     "http.request and web.fetch resolve runtime AgentLoopActionServiceNetworkRequest through rt.actionServiceResolver().NetworkRequest(); daemonNetworkRequestManagerActionService is the adapter that forwards request/fetch side effects to manager methods",
		targetCapabilities: []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServiceNetworkRequest},
		evidence:           "http.request and web.fetch now call networkRequestActionServiceFromRegistryRuntime; executeAgentRegistryHandlerResultWithToolServices attaches the daemon network request adapter as a runtime capability before handler invocation",
		nextStep:           "keep network request registry handlers capability-owned while the remaining port scan side effect moves behind a smaller runtime capability",
	},
	{
		component: "port_scan_registry_handler",
		stage:     daemonPlannerActionExecutionRuntimeActionServiceStageRuntimeCapabilityAdapterAttached,
		actions: []string{
			"ports.scan.safe",
		},
		managerMethods: []string{
			"handleAgentPortScan",
		},
		currentOwner:       daemonPlannerActionExecutionRuntimeActionServiceOwnerRuntimeCapability,
		currentSurface:     "ports.scan.safe resolves runtime AgentLoopActionServicePortScanner through rt.actionServiceResolver().PortScanner(); daemonPortScannerManagerActionService is the adapter that forwards safe port scan side effects to the manager method",
		targetCapabilities: []runtime.AgentLoopActionServiceCapability{runtime.AgentLoopActionServicePortScanner},
		evidence:           "ports.scan.safe now calls portScannerActionServiceFromRegistryRuntime; executeAgentRegistryHandlerResultWithToolServices attaches the daemon port scanner adapter as a runtime capability before handler invocation",
		nextStep:           "keep port scanner capability adapter covered while health/plugins/runs become the next selected orchestration adapter boundary",
	},
}

var daemonPlannerActionExecutionRegistryAdapterCompletionTable = [...]daemonPlannerActionExecutionRegistryAdapterCompletionEntry{
	{
		component:    "runtime_action_service_ownership_completion",
		stage:        daemonPlannerActionExecutionRegistryAdapterCompletionStageAdaptersComplete,
		evidence:     "daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable contains only runtime_capability_adapter_attached entries for all registry side-effect groups; selected_next_runtime_capability and daemon_manager_owned groups are empty",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryTurnExecutorCompatibility,
		nextStep:     "keep registry handlers capability-adapter-backed while planner_action_execution fallback retirement starts at the TurnExecutor compatibility seam",
	},
	{
		component:    "registry_handler_direct_manager_source_audit",
		stage:        daemonPlannerActionExecutionRegistryAdapterCompletionStageSourceAuditClear,
		evidence:     "agent_registry_handlers.go has no direct rt.Manager.handleAgent* orchestration calls; remaining manager forwarding is isolated inside daemon action-service adapter files",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryTurnExecutorCompatibility,
		nextStep:     "continue source-auditing registry handlers while fallback retirement proceeds",
	},
	{
		component:    "planner_action_executor_next_fallback_boundary",
		stage:        daemonPlannerActionExecutionRegistryAdapterCompletionStageFallbackBoundaryChosen,
		evidence:     "daemonPlannerActionExecutionFallbackBoundaryTable recorded ActionExecutor -> TurnExecutor compatibility -> daemonAgentLoopActionExecutor compatibility ordering at the N211 audit point",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryTurnExecutorCompatibility,
		nextStep:     "P0.5-N212 should audit TurnExecutor compatibility callers and tests to decide whether ErrAgentLoopActionExecutorNotAttached can bypass or narrow that fallback",
	},
}

var daemonPlannerActionExecutionTurnCompatibilityRetirementTable = [...]daemonPlannerActionExecutionTurnCompatibilityRetirementEntry{
	{
		component:    "planner_action_execution_agent_loop_fallback",
		stage:        daemonPlannerActionExecutionTurnCompatibilityRetirementStageInternalFallbackRetired,
		evidence:     "runAgentPlannerActionExecutionAgentLoopResult no longer uses the internal TurnExecutor compatibility hop for ActionExecutor missing/ErrAgentLoopActionExecutorNotAttached handling",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryDirectLegacyExecutor,
		nextStep:     "keep focused tests proving TurnExecutor is not called on planner_action_execution action-executor fallback",
	},
	{
		component:    "planner_action_execution_external_turn_executor_compatibility",
		stage:        daemonPlannerActionExecutionTurnCompatibilityRetirementStageExternalCompatibility,
		evidence:     "daemonAgentTurnExecutor.ExecuteTurn still accepts kind=planner_action_execution as an explicit external compatibility trampoline",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryDirectLegacyExecutor,
		nextStep:     "retire the external TurnExecutor trampoline only after direct ExecuteTurn compatibility callers are audited",
	},
	{
		component:    "planner_action_execution_next_fallback_boundary",
		stage:        daemonPlannerActionExecutionTurnCompatibilityRetirementStageNextBoundaryChosen,
		evidence:     "planner_action_execution fallback boundary table now contains only action_executor and direct_legacy_executor stages",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryDirectLegacyExecutor,
		nextStep:     "P0.5-N213 should audit direct legacy executor fallback preconditions before retiring daemonAgentLoopActionExecutor compatibility",
	},
}

var daemonPlannerActionExecutionDirectLegacyRetirementTable = [...]daemonPlannerActionExecutionDirectLegacyRetirementEntry{
	{
		component:    "primary_agent_loop_runtime_action_executor",
		stage:        daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionExecutorPrimary,
		evidence:     "agentManager.AttachEngine attaches daemonAgentLoopActionExecutor and runAgentPlannerActionExecutionAgentLoopResult first dispatches through runtime.Engine.ExecuteAgentLoopAction",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "keep the primary AgentLoop planner_action_execution path on Engine.ExecuteAgentLoopAction while auditing the remaining no-engine direct fallback",
	},
	{
		component:    "direct_legacy_executor_call_surface",
		stage:        daemonPlannerActionExecutionDirectLegacyRetirementStageDirectExecutorSurfaceScoped,
		evidence:     "production daemonPlannerActionExecutionExecutorForManager(...).ExecutePlannerAction is now implementation-only behind daemonAgentLoopActionExecutor; heuristic route and external TurnExecutor compatibility enter through daemonAgentLoopActionExecutor.ExecuteAgentLoopAction",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "keep new planner action execution callers entering through runtime.AgentLoopActionExecutor instead of daemonPlannerActionExecutionExecutor",
	},
	{
		component:    "registry_action_side_effect_ownership",
		stage:        daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionServicesCovered,
		evidence:     "all eight registry action side-effect groups remain runtime capability adapter attached and agent_registry_handlers.go has no direct rt.Manager.handleAgent* orchestration calls",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "keep registry action-service adapter coverage while fallback retirement proceeds",
	},
	{
		component:    "no_engine_agent_loop_direct_fallback",
		stage:        daemonPlannerActionExecutionDirectLegacyRetirementStageCompatibilityFallbackRetained,
		evidence:     "runAgentPlannerActionExecutionAgentLoopResult now invokes daemonAgentLoopActionExecutor directly only when the runtime engine is missing; attached-engine missing/detached AgentLoopActionExecutor returns blocked/error through the runtime action executor result",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "P0.5-N215 should audit whether the remaining no-engine AgentLoop direct fallback can be retired after all callers attach runtime.Engine",
	},
	{
		component:    "planner_action_execution_next_fallback_boundary",
		stage:        daemonPlannerActionExecutionDirectLegacyRetirementStageNextBoundaryChosen,
		evidence:     "missing/detached action-executor fallback is narrowed to blocked/error, leaving only no-engine AgentLoop direct fallback as the next retirement boundary",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "audit no-engine AgentLoop direct fallback callers before removing daemonAgentLoopActionExecutor direct compatibility from runAgentPlannerActionExecutionAgentLoopResult",
	},
}

var daemonPlannerActionExecutionActionExecutorMissingRetirementTable = [...]daemonPlannerActionExecutionActionExecutorMissingRetirementEntry{
	{
		component:    "no_engine_agent_loop_direct_fallback",
		stage:        daemonPlannerActionExecutionActionExecutorMissingRetirementStageNoEngineFallbackRetained,
		reason:       daemonPlannerActionExecutionFallbackEngineNotAttached,
		disposition:  daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionDirectLegacyFallback,
		evidence:     "when runtime.Engine is not attached, runRuntimeAgentLoopResult already enters the daemon AgentLoop direct fallback and planner_action_execution preserves daemonAgentLoopActionExecutor compatibility",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "keep no-engine fallback until all production planner_action_execution callers have a runtime.Engine attached",
	},
	{
		component:    "attached_engine_missing_action_executor",
		stage:        daemonPlannerActionExecutionActionExecutorMissingRetirementStageMissingExecutorBlocked,
		reason:       daemonPlannerActionExecutionFallbackActionExecutorNotAttached,
		disposition:  daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionBlockedError,
		evidence:     "agentManager.AttachEngine attaches daemonAgentLoopActionExecutor, so a nil AgentLoopActionExecutor on an attached engine represents an explicit detach/miswire and now returns runtime.ErrAgentLoopActionExecutorNotAttached instead of direct legacy fallback",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "treat attached-engine missing ActionExecutor as a blocked runtime configuration error",
	},
	{
		component:    "detached_action_executor_result",
		stage:        daemonPlannerActionExecutionActionExecutorMissingRetirementStageDetachedResultBlocked,
		reason:       daemonPlannerActionExecutionFallbackActionExecutorNotAttached,
		disposition:  daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionBlockedError,
		evidence:     "ErrAgentLoopActionExecutorNotAttached returned by Engine.ExecuteAgentLoopAction or by the executor result is normalized to blocked/error and no longer falls through to daemonAgentLoopActionExecutor",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "keep hard action-executor errors and detached executor results visible to callers instead of invoking legacy side effects",
	},
	{
		component:    "planner_action_execution_next_fallback_boundary",
		stage:        daemonPlannerActionExecutionActionExecutorMissingRetirementStageNextBoundaryChosen,
		disposition:  daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionDirectLegacyFallback,
		evidence:     "after narrowing missing/detached ActionExecutor to blocked/error, the only remaining direct legacy fallback in runAgentPlannerActionExecutionAgentLoopResult is the no-engine AgentLoop compatibility path",
		nextBoundary: daemonPlannerActionExecutionNextFallbackRetirementBoundaryNoEngineDirectLegacy,
		nextStep:     "P0.5-N215 should decide whether no-engine AgentLoop direct fallback can become blocked/error once engine attachment is mandatory",
	},
}

var daemonPlannerActionExecutorServiceContractMigrationTable = [...]daemonPlannerActionExecutorServiceContractMigrationEntry{
	{
		component:      "runtime_action_executor_seam",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "runtime.AgentLoopActionExecutor / Engine.ExecuteAgentLoopAction",
		targetSurface:  "planner_action_execution dispatches one action through the runtime action executor contract",
		evidence:       "runtime executor request/result use AgentLoopRequest, AgentLoopAction, AgentLoopMessage and AgentLoopToolCall DTOs with clone isolation",
		nextStep:       "keep the seam stable while remaining typed service, authorization and result egress blockers move behind runtime contracts",
	},
	{
		component:      "runtime_action_service_capability_set",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "runtime.AgentLoopActionServiceSet names version/plugins/runs/approvals/jobs/agent/scan_manager/schedules/providers/workspace/network_request/port_scanner/web_search/web_scan/schedule_action/daemon_status_action/run_context_approval_action capabilities, and AgentLoopToolServiceBinding carries ActionServices as the default service surface",
		targetSurface:  "planner_action_execution resolves action services through runtime capability names instead of opaque Handle/provider fixtures",
		evidence:       "agentLoopToolServiceBindingFromRouterServices populates shared ActionServices without Handle, registry dispatch attaches workspace/network request/port scanner/web search/web scan/schedule/daemon status/run-context approval action capability adapters, and agentRegistryHandlerRuntime.actionServiceResolver reads the runtime capability set directly",
		nextStep:       "keep action-service capability consumers as the compatibility fixtures shrink",
	},
	{
		component:      "daemon_legacy_executor_adapter",
		stage:          daemonPlannerActionExecutorServiceContractStageDaemonAdapterAttached,
		currentSurface: "runRuntimePlannerActionExecution calls Engine.RunAgentLoop planner_action_execution mode; side-effect boundary carries runtime AgentLoopToolServiceBinding and daemon legacy executor forwards that binding directly to registry dispatch",
		targetSurface:  "planner_action_execution runner can execute through runtime-native action services without TurnExecutor/direct legacy fallbacks",
		evidence:       "primary caller dispatches an AgentLoopRequest carrying mode/action/toolServices; boundary clones the runtime binding and executeAgentPlannerActionResultWithToolServices preserves custom binding metadata through handler runtime",
		nextStep:       "retire TurnExecutor/direct legacy fallbacks after remaining compatibility callers move to AgentLoopRunner",
	},
	{
		component:      "tool_service_provider_contract_test_fixture",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "historical typed-provider contract table, explicit Handle fallback helpers and ActionServices-to-routerServices test roundtrip helpers are retired",
		targetSurface:  "runtime action executor receives runtime action services instead of daemon routerServices/provider bundles",
		evidence:       "daemonActionServiceSetFromRouterServices maps routerServices fields to runtime capability names; source-audit test rejects provider contract names and provider metadata in production daemon files",
		nextStep:       "keep tests asserting runtime ActionServices capabilities directly as the legacy compatibility fixture stays retired",
	},
	{
		component:      "registry_handler_action_service_first_batch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "daemon.health, plugins.list and runs.list call daemonStatusActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceDaemonStatusAction without Handle fallback; the daemon adapter receives the runtime resolver so VersionService/PluginService/RunService still load through ActionServices",
		targetSurface:  "daemon.health/plugins.list/runs.list consume runtime daemon status action service capability without direct rt.Manager.handleAgentHealth/Plugins/Runs calls",
		evidence:       "focused daemon status registry handler test uses ActionServices with a fake daemon status capability and no Manager, while source audit rejects direct status manager calls in agent_registry_handlers.go",
		nextStep:       "keep daemon status capability adapter covered while registry action-service adapter completion is audited",
	},
	{
		component:      "registry_handler_action_service_schedule_batch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "schedule.create, schedule.list and schedule.delete call scheduleActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceScheduleAction without Handle fallback; the daemon adapter receives the runtime resolver so ScheduleManager reads/writes still load through ActionServices",
		targetSurface:  "schedule.* consumes runtime schedule action service capability without direct rt.Manager.handleAgentSchedule* calls",
		evidence:       "focused schedule registry handler test uses ActionServices with a fake schedule action capability and no Manager, while source audit rejects direct schedule manager calls in agent_registry_handlers.go",
		nextStep:       "keep schedule action capability adapter covered while registry action-service adapter completion is audited",
	},
	{
		component:      "registry_handler_action_service_web_search_batch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "web.search calls webSearchActionServiceFromRegistryRuntime() and resolves runtime AgentLoopActionServiceWebSearch without Handle fallback; the daemon adapter receives the runtime resolver so ProviderStore config still loads through ActionServices",
		targetSurface:  "web.search consumes runtime web search action service capability without direct rt.Manager.handleAgentWebSearch calls",
		evidence:       "focused web search registry handler test uses ActionServices with a fake web search capability and no Manager, while source audit rejects direct web search manager calls in agent_registry_handlers.go",
		nextStep:       "keep web search capability adapter covered while registry action-service adapter completion is audited",
	},
	{
		component:      "registry_handler_action_service_scan_batch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "scan.web.safe calls webScanActionServiceFromRegistryRuntime() and resolves runtime AgentLoopActionServiceWebScan without Handle fallback; the daemon adapter receives the runtime resolver so JobManager scan creation still loads through ActionServices",
		targetSurface:  "scan.web.safe consumes runtime web scan action service capability without direct rt.Manager.handleAgentScan calls",
		evidence:       "focused web scan registry handler test uses ActionServices with a fake web scan capability and no Manager, while source audit rejects direct scan manager calls in agent_registry_handlers.go",
		nextStep:       "keep web scan capability adapter covered while registry action-service adapter completion is audited",
	},
	{
		component:      "registry_handler_action_service_run_context_approval_batch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "runs.context, reports.draft, runs.approvals.list, runs.resume_approved and runs.approvals.approve/deny call runContextApprovalActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceRunContextApproval without Handle fallback; the daemon adapter receives the runtime resolver so RunService/ApprovalService reads/writes still load through ActionServices",
		targetSurface:  "run-context/approval registry handlers consume runtime run-context approval action service capability without direct rt.Manager.handleAgentRunContext*/handleAgentReportDraft/handleAgentApprovals/handleAgentResumeApproved/handleAgentApprovalDecision calls",
		evidence:       "focused run-context/approval registry handler test uses ActionServices with a fake run-context approval capability and no Manager, while source audit rejects direct run-context/approval manager calls in agent_registry_handlers.go",
		nextStep:       "audit registry action-service adapter completion before selecting the next planner action executor fallback retirement boundary",
	},
	{
		component:      "registry_handler_action_service_workspace_batch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "workspace.command, workspace.files.* and workspace.search call workspaceActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceWorkspace without Handle fallback",
		targetSurface:  "workspace registry handlers consume runtime workspace action service capability without direct rt.Manager.handleAgentCommand/handleAgentWorkspace* calls",
		evidence:       "focused workspace registry handler test uses ActionServices with a fake workspace capability and no Manager, while source audit rejects direct workspace manager calls in agent_registry_handlers.go",
		nextStep:       "keep workspace capability adapter covered while network request and port-scan side-effect groups are migrated",
	},
	{
		component:      "registry_handler_action_service_network_request_batch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "http.request and web.fetch call networkRequestActionServiceFromRegistryRuntime() and resolve runtime AgentLoopActionServiceNetworkRequest without Handle fallback",
		targetSurface:  "network request registry handlers consume runtime network request action service capability without direct rt.Manager.handleAgentHTTPRequest/handleAgentWebFetch calls",
		evidence:       "focused network request registry handler test uses ActionServices with a fake network request capability and no Manager, while source audit rejects direct network manager calls in agent_registry_handlers.go",
		nextStep:       "keep network request capability adapter covered while the port-scan side-effect group is migrated",
	},
	{
		component:      "registry_handler_action_service_port_scan_batch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "ports.scan.safe calls portScannerActionServiceFromRegistryRuntime() and resolves runtime AgentLoopActionServicePortScanner without Handle fallback",
		targetSurface:  "port scan registry handler consumes runtime port scanner action service capability without direct rt.Manager.handleAgentPortScan calls",
		evidence:       "focused port scanner registry handler test uses ActionServices with a fake port scanner capability and no Manager, while source audit rejects direct port scan manager calls in agent_registry_handlers.go",
		nextStep:       "keep port scanner capability adapter covered while remaining registry runtime capability consumers are evaluated for narrower adapters",
	},
	{
		component:      "runtime_action_result_composition",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "runtime.AgentLoopPlannerActionExecutionResultFromActionExecutorResult / FromTurnPayload",
		targetSurface:  "planner_action_execution runner result egress envelope is runtime-owned for action executor and TurnExecutor fallback paths",
		evidence:       "runner branch now delegates ActionExecutorResult/TurnPayload to runtime helpers that stamp mode/action/count/status/error metadata and clone DTOs",
		nextStep:       "continue shrinking daemon legacy result conversion before retiring direct legacy executor fallback",
	},
	{
		component:      "legacy_result_adapter_centralized",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "daemonAgentLoopActionExecutor is the scoped compatibility entrypoint and daemonPlannerActionExecutionExecutor returns runtime.AgentLoopActionExecutorResult behind that adapter",
		targetSurface:  "planner_action_execution callers no longer receive daemon-private agentMessage/agentToolCall result types",
		evidence:       "daemon AgentLoop action executor is the only production caller of daemonPlannerActionExecutionExecutor; TurnExecutor compatibility and heuristic route enter through daemonAgentLoopActionExecutor.ExecuteAgentLoopAction",
		nextStep:       "keep direct executor access implementation-only while auditing missing ActionExecutor compatibility fallback",
	},
	{
		component:      "registry_handler_runtime_authorization_context",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "agentRegistryHandlerRuntime carries runtime.AgentLoopAuthorization and http.request resolves credentials through agentResolveAgentLoopAuthProfileHeaders",
		targetSurface:  "registry handlers consume runtime authorization context instead of daemon HTTP auth-profile slices",
		evidence:       "focused http.request registry handler test passes runtime-only auth profile while legacy AuthProfiles contains a conflicting profile",
		nextStep:       "remove legacy registry runtime auth-profile compatibility fields once all handler/test callers consume Authorization only",
	},
	{
		component:      "authorization_context",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "daemonPlannerActionExecutionSideEffectBoundary, executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices pass runtime.AgentLoopAuthorization end-to-end",
		targetSurface:  "runtime action executor consumes AgentLoopAuthorization or a typed runtime auth resolver directly",
		evidence:       "agentResolveAgentLoopAuthProfileHeaders consumes runtime profiles directly; executor and registry dispatch signatures accept AgentLoopAuthorization instead of []agentHTTPAuthProfile/default profile fields",
		nextStep:       "remove legacy registry runtime auth-profile compatibility fields after handler compatibility coverage is retired",
	},
	{
		component:      "registry_handler_service_consumers",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "all builtin registry handlers call agentRegistryHandlerRuntime.actionServiceResolver(); production resolver consumes runtime ActionServices only",
		targetSurface:  "registry handlers consume runtime action service capabilities without opaque Handle fallback",
		evidence:       "focused tests cover first/schedule/web.search/scan/run-context/approval/workspace/network request/port scanner runtime capability batches with Handle fallbacks that panic on provider accessors, plus web.search adapter dispatch with no Manager",
		nextStep:       "keep provider/routerServices reconstruction retired from production and test runtime paths",
	},
	{
		component:      "registry_handler_runtime_fallback_surface",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "agentRegistryHandlerRuntime production surface exposes actionServiceResolver()/authorizationContext() only; toolServiceProvider()/legacyRouterServices() registry runtime helpers have been removed",
		targetSurface:  "builtin registry handlers cannot call provider/routerServices reconstruction helpers directly",
		evidence:       "source-audit test scans production registry dispatch/handler files and rejects agentRegistryHandlerRuntime toolServiceProvider()/legacyRouterServices() helper definitions or calls",
		nextStep:       "keep production registry runtime and tests free of provider/routerServices reconstruction helpers",
	},
	{
		component:      "tool_service_binding_handle_default_surface",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "agentLoopToolServiceBindingFromRouterServices emits ActionServices-only bindings with nil Handle; explicit Handle reconstruction support has been retired from binding roundtrip helpers",
		targetSurface:  "primary daemon callers do not rely on AgentLoopToolServiceBinding.Handle to carry service providers",
		evidence:       "binding tests assert nil Handle and direct ActionServices capabilities while Handle-only bindings are rejected by registry runtime and do not expose runtime capabilities",
		nextStep:       "keep direct ActionServices capability assertions on loop runner and turn executor boundaries",
	},
	{
		component:      "runtime_tool_service_direct_dispatch",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "executeAgentPlannerActionResultWithToolServices, executeAgentRegistryHandlerResultWithToolServices, executeAgentPlannerActionsWithToolServices, routePromptWithToolServices, routePromptForTurnWithToolServices, routePromptWithRuntimeDeterministicRouteWithToolServices, ControlSessionWithToolServices, DecideAgentAuthorizationWithToolServices, SendMessageWithToolServices, runAgentSendMessageTurnWithToolServices, handleAgentActiveRunContinueForTurnWithToolServices, agentLLMToolLoopRequestFromLegacyWithToolServices, agentReplanActionContinuationRequestFromLegacyWithToolServices and agentPlannerActionReplanCycleToolContextFromToolServices preserve runtime AgentLoopToolServiceBinding across planner action, deterministic/heuristic route, control/auth/send turn, active-run continue and LLM/replan request dispatch",
		targetSurface:  "production planner action, deterministic route, control/auth continuation, send-message lifecycle and send-turn paths dispatch downstream runtime helpers without reconstructing daemon routerServices from runtime bindings",
		evidence:       "focused tests assert custom runtime binding metadata survives through daemon legacy executor and deterministic route AgentLoop paths; source audit rejects retired legacy routerServices ingress wrappers and legacyRouterServicesFromAgentLoopToolServiceBinding calls in production dispatch files",
		nextStep:       "keep direct runtime ActionServices capability checks covering loop runner and turn executor boundaries",
	},
	{
		component:      "registry_runtime_action_services_only",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "agentRegistryHandlerRuntime.actionServiceResolver delegates to an ActionServices-only resolver and no longer unwraps AgentLoopToolServiceBinding.Handle",
		targetSurface:  "production registry runtime cannot recover daemon provider/routerServices through opaque Handle fallback",
		evidence:       "focused registry runtime test asserts Handle-only binding is rejected by actionServiceResolver and direct capability checks show no runtime ActionServices capability is present",
		nextStep:       "keep Handle/provider/routerServices fallback retired",
	},
	{
		component:      "tool_service_binding_provider_reconstruction_test_only",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "provider/routerServices reconstruction helpers for AgentLoopToolServiceBinding are retired from both production and test runtime paths",
		targetSurface:  "production runtime files cannot reconstruct daemon provider/routerServices from runtime tool service bindings",
		evidence:       "source-audit test scans all non-test daemon files and rejects routerServicesFromAgentLoopToolServiceBinding, agentLoopActionServiceResolverFromAgentLoopToolServiceBinding, provider-from-Handle and provider/routerServices reconstruction helper definitions; focused tests assert ActionServices capabilities directly",
		nextStep:       "keep helper-free test coverage while future callers migrate away from daemon-private routerServices entirely",
	},
	{
		component:      "legacy_result_egress",
		stage:          daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded,
		currentSurface: "executeAgentPlannerActionResultWithToolServices and executeAgentRegistryHandlerResultWithToolServices produce runtime.AgentLoopActionExecutorResult directly for all current callers; legacy routerServices-ingress wrappers are retired",
		targetSurface:  "action executor implementation returns runtime result DTOs without daemon-private egress types",
		evidence:       "registryHandlerOutputToActionExecutorResult builds runtime AgentLoopMessage/AgentLoopToolCall DTOs directly and no longer calls registryHandlerOutputToMessages; legacy handler output types are adapted as compatibility inputs",
		nextStep:       "replace remaining daemon-private tool-service dispatch with runtime-native action service interfaces",
	},
}

var daemonAgentLoopRunnerCompletionAuditTable = [...]daemonAgentLoopRunnerCompletionAuditEntry{
	{
		mode:                 runtime.AgentLoopModeDeterministicRoute,
		runnerBranch:         "daemonAgentLoopRunner.RunAgentLoop -> runAgentDeterministicRouteAgentLoopResult",
		appendPersistOwner:   daemonAgentLoopRunnerCompletionOwnerDaemonAdapterViaRuntimeDTO,
		toolSynthesisOwner:   daemonAgentLoopRunnerCompletionOwnerNotApplicable,
		actionExecutionOwner: daemonAgentLoopRunnerCompletionOwnerDaemonAdapter,
		fallbackOwner:        daemonAgentLoopRunnerCompletionOwnerDaemonAdapter,
		currentSurface:       "deterministic route runs through AgentLoopRunner; direct daemon fallback is covered by agent loop fallback ownership/boundary tables, capability-gap plan append is still triggered by routePromptForTurnWithToolServices, heuristic registry actions call the daemon planner action executor adapter directly, planner_action_execution action fallback chain and runtime action-service ownership are table-covered, nested planner_action_replan_cycle dispatch is source-audited through runRuntimeAgentLoopResult, optional-result SessionMutation fallback is narrowed, and remaining no-engine SessionMutation fallback is audited as CYBERAGENT_RUNTIME rollback-only",
		nextRuntimeSeam:      daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate,
	},
	{
		mode:                 runtime.AgentLoopModeLLMToolLoop,
		runnerBranch:         "daemonAgentLoopRunner.RunAgentLoop -> runAgentLLMToolLoopAgentLoopResult",
		appendPersistOwner:   daemonAgentLoopRunnerCompletionOwnerDaemonAdapterViaRuntimeDTO,
		toolSynthesisOwner:   daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback,
		actionExecutionOwner: daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback,
		fallbackOwner:        daemonAgentLoopRunnerCompletionOwnerDaemonAdapter,
		currentSurface:       "outer LLM loop is engine-dispatched; planner chat/tool synthesis use runtime LLMStageRunner with legacy fallback covered by daemon LLM stage fallback tables, direct daemon fallback is covered by agent loop fallback tables, planner_action_execution action fallback chain and runtime action-service ownership are table-covered, nested cycle dispatch now reuses runRuntimeAgentLoopResult, nested cycle append placement goes through runtime AgentLoopReplanCycleAppendDecision, attached SessionMutator not-attached fallback is narrowed; append SessionMutation blocked/error fallback is narrowed; optional-result SessionMutation fallback is narrowed; remaining no-engine SessionMutation fallback is audited and retained only for CYBERAGENT_RUNTIME rollback",
		nextRuntimeSeam:      daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate,
	},
	{
		mode:                 runtime.AgentLoopModeReplanContinuation,
		runnerBranch:         "daemonAgentLoopRunner.RunAgentLoop -> runAgentReplanActionContinuationAgentLoopResult",
		appendPersistOwner:   daemonAgentLoopRunnerCompletionOwnerDaemonAdapterViaRuntimeDTO,
		toolSynthesisOwner:   daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback,
		actionExecutionOwner: daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback,
		fallbackOwner:        daemonAgentLoopRunnerCompletionOwnerDaemonAdapter,
		currentSurface:       "continuation loop is engine-dispatched and delegates each step to planner_action_replan_cycle through runRuntimeAgentLoopResult; direct daemon fallback is covered by agent loop fallback ownership/boundary tables, LLM stage fallback ownership and planner_action_execution action fallback chain are table-covered, nested cycle append placement is runtime-decision-owned, attached SessionMutator not-attached fallback is narrowed; append SessionMutation blocked/error fallback is narrowed; optional-result SessionMutation fallback is narrowed; remaining no-engine SessionMutation fallback is audited and retained only for CYBERAGENT_RUNTIME rollback",
		nextRuntimeSeam:      daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate,
	},
	{
		mode:                 runtime.AgentLoopModePlannerActionReplanCycle,
		runnerBranch:         "daemonAgentLoopRunner.RunAgentLoop -> runAgentPlannerActionReplanCycleAgentLoop",
		appendPersistOwner:   daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback,
		toolSynthesisOwner:   daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback,
		actionExecutionOwner: daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback,
		fallbackOwner:        daemonAgentLoopRunnerCompletionOwnerDaemonAdapter,
		currentSurface:       "shared cycle executes action batch through planner_action_execution and synthesis through LLMStageRunner; tool_synthesis, direct loop and planner_action_execution action fallback compatibility are table-covered, action/warning/synthesis append placement and state egress use runtime AgentLoopReplanCycleAppendDecision, nested dispatch source audit confirms runRuntimeAgentLoopResult owns fallback policy, attached SessionMutator not-attached fallback is narrowed, append SessionMutation blocked/error fallback is narrowed, optional-result SessionMutation fallback is narrowed, and no-engine SessionMutation fallback is retained only as CYBERAGENT_RUNTIME rollback",
		nextRuntimeSeam:      daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate,
	},
	{
		mode:                 runtime.AgentLoopModePlannerActionExecution,
		runnerBranch:         "daemonAgentLoopRunner.RunAgentLoop -> runAgentPlannerActionExecutionAgentLoopResult",
		appendPersistOwner:   daemonAgentLoopRunnerCompletionOwnerNotApplicable,
		toolSynthesisOwner:   daemonAgentLoopRunnerCompletionOwnerNotApplicable,
		actionExecutionOwner: daemonAgentLoopRunnerCompletionOwnerRuntimeSeamWithFallback,
		fallbackOwner:        daemonAgentLoopRunnerCompletionOwnerDaemonAdapter,
		currentSurface:       "planner action execution is engine-dispatched, direct loop and action fallback chain ownership are table-covered, result composition is runtime-owned, daemon branch still preserves ActionExecutor, TurnExecutor and direct legacy executor compatibility ordering, nested planner_action_replan_cycle dispatch is aligned with runRuntimeAgentLoopResult, and no-engine SessionMutation fallback is audited as rollback-only",
		nextRuntimeSeam:      daemonAgentLoopRunnerCompletionNextRuntimeSeamManualStageCommitTagGate,
	},
}

func daemonTurnExecutionCallerInvalidPayloadError(kind string) string {
	for _, entry := range daemonTurnExecutionCallerInvalidPayloadTable {
		if entry.kind == kind {
			return entry.message
		}
	}
	return daemonTurnExecutionCallerDefaultInvalidPayloadError
}

func daemonTurnExecutionCallerPolicyForKind(kind string) (daemonTurnExecutionCallerPolicy, bool) {
	for _, entry := range daemonTurnExecutionCallerPolicyTable {
		if entry.kind == kind {
			return entry.policy, true
		}
	}
	return "", false
}

func daemonTurnExecutionCallerPolicyMatches(kind string, want daemonTurnExecutionCallerPolicy) bool {
	policy, ok := daemonTurnExecutionCallerPolicyForKind(kind)
	return !ok || policy == want
}

func daemonTurnExecutionCallerPolicyMismatchError(kind string, want daemonTurnExecutionCallerPolicy) error {
	return errors.New("daemon: turn execution caller policy mismatch for " + kind + ": expected " + string(want))
}

func daemonTurnExecutionCallerFallbackOwnerForReason(reason daemonTurnExecutionCallerFallbackReason) (string, bool) {
	for _, entry := range daemonTurnExecutionCallerFallbackOwnershipTable {
		if entry.reason == reason {
			return entry.owner, true
		}
	}
	return "", false
}

func daemonTurnExecutionCallerFallbackOwnedByLegacyCaller(reason daemonTurnExecutionCallerFallbackReason) bool {
	owner, ok := daemonTurnExecutionCallerFallbackOwnerForReason(reason)
	return ok && owner == daemonTurnExecutionCallerFallbackOwnerLegacyCaller
}

func daemonTurnExecutionCallerFallbackOwnershipError(reason daemonTurnExecutionCallerFallbackReason) error {
	return errors.New("daemon: turn execution fallback ownership missing for " + string(reason))
}

func daemonTurnExecutionCallerPreDispatchFallbackReason(eng *runtime.Engine) (daemonTurnExecutionCallerFallbackReason, bool) {
	if eng == nil || eng.TurnExecutor() == nil {
		return daemonTurnExecutionCallerFallbackExecutorNotAttached, true
	}
	return "", false
}

func daemonTurnExecutionCallerMigrationForKind(kind string) (daemonTurnExecutionCallerMigrationEntry, bool) {
	for _, entry := range daemonTurnExecutionCallerMigrationTable {
		if entry.kind == kind {
			return entry, true
		}
	}
	return daemonTurnExecutionCallerMigrationEntry{}, false
}

func daemonTurnExecutionCallerMigrationActiveTurnExecutor(kind string) bool {
	entry, ok := daemonTurnExecutionCallerMigrationForKind(kind)
	return ok && entry.stage == daemonTurnExecutionCallerMigrationStageActiveTurnExecutor
}

func daemonTurnExecutionHelperOwnershipForKind(kind string) (daemonTurnExecutionHelperOwnershipEntry, bool) {
	for _, entry := range daemonTurnExecutionHelperOwnershipTable {
		if entry.kind == kind {
			return entry, true
		}
	}
	return daemonTurnExecutionHelperOwnershipEntry{}, false
}

func daemonTurnExecutionInternalHelperRuntimeOwned(kind string) bool {
	entry, ok := daemonTurnExecutionHelperOwnershipForKind(kind)
	return ok &&
		(entry.callerSurface == "internal_execute_turn_caller" || entry.callerSurface == "internal_agent_loop_caller") &&
		entry.requestHelperOwner == daemonTurnExecutionHelperOwnerRuntime &&
		entry.turnHelperOwner == daemonTurnExecutionHelperOwnerRuntime &&
		entry.payloadHelperOwner == daemonTurnExecutionHelperOwnerRuntime
}

func daemonTurnExecutionAgentLoopMigrationPreconditionForKind(kind string) (daemonTurnExecutionAgentLoopMigrationPreconditionEntry, bool) {
	for _, entry := range daemonTurnExecutionAgentLoopMigrationPreconditionTable {
		if entry.kind == kind {
			return entry, true
		}
	}
	return daemonTurnExecutionAgentLoopMigrationPreconditionEntry{}, false
}

func daemonTurnExecutionAgentLoopMigrationPreconditionReady(kind string) bool {
	entry, ok := daemonTurnExecutionAgentLoopMigrationPreconditionForKind(kind)
	return ok &&
		(entry.stage == daemonTurnExecutionAgentLoopMigrationStagePreconditionReady ||
			entry.stage == daemonTurnExecutionAgentLoopMigrationStageAgentLoopCallerLive) &&
		daemonTurnExecutionInternalHelperRuntimeOwned(kind)
}

func daemonTurnExecutionAgentLoopMigrationBlockersForKind(kind string) []daemonTurnExecutionAgentLoopMigrationBlockerEntry {
	var blockers []daemonTurnExecutionAgentLoopMigrationBlockerEntry
	for _, entry := range daemonTurnExecutionAgentLoopMigrationBlockerTable {
		if entry.kind == kind {
			blockers = append(blockers, entry)
		}
	}
	return blockers
}

func daemonTurnExecutionAgentLoopMigrationBlockedByDaemonSideEffects(kind string) bool {
	for _, entry := range daemonTurnExecutionAgentLoopMigrationBlockersForKind(kind) {
		if entry.owner == daemonTurnExecutionAgentLoopMigrationBlockerOwnerDaemonLegacy {
			return true
		}
	}
	return false
}

func daemonPlannerActionExecutionSideEffectBoundaryPlanBlockedByDaemonLegacy() bool {
	for _, entry := range daemonPlannerActionExecutionSideEffectBoundaryPlanTable {
		if entry.currentOwner == daemonPlannerActionExecutionSideEffectOwnerDaemonLegacy {
			return true
		}
	}
	return false
}

func daemonPlannerActionExecutorContractBlockers() []daemonPlannerActionExecutorContractPreconditionEntry {
	var blockers []daemonPlannerActionExecutorContractPreconditionEntry
	for _, entry := range daemonPlannerActionExecutorContractPreconditionTable {
		if entry.stage == daemonPlannerActionExecutorContractStageBlocked {
			blockers = append(blockers, entry)
		}
	}
	return blockers
}

func daemonPlannerActionExecutorContractReadyForAgentLoop() bool {
	if len(daemonPlannerActionExecutorContractPreconditionTable) == 0 {
		return false
	}
	for _, entry := range daemonPlannerActionExecutorContractPreconditionTable {
		if entry.stage != daemonPlannerActionExecutorContractStageReady {
			return false
		}
	}
	return true
}

func daemonPlannerActionExecutionLegacySurfaceForSymbol(symbol string) (daemonPlannerActionExecutionLegacySurfaceEntry, bool) {
	for _, entry := range daemonPlannerActionExecutionLegacySurfaceTable {
		if entry.symbol == symbol {
			return entry, true
		}
	}
	return daemonPlannerActionExecutionLegacySurfaceEntry{}, false
}

func daemonPlannerActionExecutionLegacySurfaceImplementationOnly(symbol string) bool {
	entry, ok := daemonPlannerActionExecutionLegacySurfaceForSymbol(symbol)
	return ok && entry.stage == daemonPlannerActionExecutionLegacySurfaceStageExecutorAdapterImplementationOnly
}

func daemonPlannerActionExecutionFallbackOwnerForReason(reason daemonPlannerActionExecutionFallbackReason) (string, bool) {
	for _, entry := range daemonPlannerActionExecutionFallbackOwnershipTable {
		if entry.reason == reason {
			return entry.owner, true
		}
	}
	return "", false
}

func daemonPlannerActionExecutionFallbackOwnedByLegacyCaller(reason daemonPlannerActionExecutionFallbackReason) bool {
	owner, ok := daemonPlannerActionExecutionFallbackOwnerForReason(reason)
	return ok && owner == daemonPlannerActionExecutionFallbackOwnerLegacyCaller
}

func daemonPlannerActionExecutionFallbackOwnershipError(reason daemonPlannerActionExecutionFallbackReason) error {
	return errors.New("daemon: planner action execution fallback ownership missing for " + string(reason))
}

func daemonPlannerActionExecutionActionExecutorPreDispatchFallbackReason(eng *runtime.Engine) (daemonPlannerActionExecutionFallbackReason, bool) {
	if eng == nil {
		return daemonPlannerActionExecutionFallbackEngineNotAttached, true
	}
	return "", false
}

func daemonPlannerActionExecutionActionExecutorResultFallbackReason(result runtime.AgentLoopActionExecutorResult, resultErr error) (daemonPlannerActionExecutionFallbackReason, bool) {
	return "", false
}

func daemonPlannerActionExecutionActionExecutorNotAttachedBlockedResult(result runtime.AgentLoopActionExecutorResult, resultErr error) (runtime.AgentLoopActionExecutorResult, bool) {
	if !errors.Is(resultErr, runtime.ErrAgentLoopActionExecutorNotAttached) &&
		!errors.Is(result.Error, runtime.ErrAgentLoopActionExecutorNotAttached) {
		return result, false
	}
	blocked := result.Clone()
	blocked.Status = runtime.TurnResultBlocked
	if blocked.Error == nil {
		blocked.Error = resultErr
	}
	if blocked.Error == nil {
		blocked.Error = runtime.ErrAgentLoopActionExecutorNotAttached
	}
	if blocked.Reason == "" {
		blocked.Reason = blocked.Error.Error()
	}
	if blocked.Metadata == nil {
		blocked.Metadata = map[string]any{}
	}
	blocked.Metadata["planner_action_execution_action_executor_fallback"] = "retired"
	blocked.Metadata["blocked_reason"] = string(daemonPlannerActionExecutionFallbackActionExecutorNotAttached)
	return blocked, true
}

func daemonPlannerActionExecutionFallbackBoundaryForStage(stage string) (daemonPlannerActionExecutionFallbackBoundaryEntry, bool) {
	for _, entry := range daemonPlannerActionExecutionFallbackBoundaryTable {
		if entry.stage == stage {
			return entry, true
		}
	}
	return daemonPlannerActionExecutionFallbackBoundaryEntry{}, false
}

func daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntryForComponent(component string) (daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntry, bool) {
	for _, entry := range daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntry{}, false
}

func daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(stage daemonPlannerActionExecutionRuntimeActionServiceOwnershipStage) []daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntry {
	var entries []daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntry
	for _, entry := range daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonPlannerActionExecutionRuntimeActionServiceOwnershipSelectedNextCapability() (runtime.AgentLoopActionServiceCapability, bool) {
	entries := daemonPlannerActionExecutionRuntimeActionServiceOwnershipEntriesForStage(daemonPlannerActionExecutionRuntimeActionServiceStageSelectedNextCapability)
	if len(entries) != 1 || len(entries[0].targetCapabilities) != 1 {
		return "", false
	}
	return entries[0].targetCapabilities[0], true
}

func daemonPlannerActionExecutionRegistryAdapterCompletionEntryForComponent(component string) (daemonPlannerActionExecutionRegistryAdapterCompletionEntry, bool) {
	for _, entry := range daemonPlannerActionExecutionRegistryAdapterCompletionTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonPlannerActionExecutionRegistryAdapterCompletionEntry{}, false
}

func daemonPlannerActionExecutionRegistryAdapterCompletionEntriesForStage(stage daemonPlannerActionExecutionRegistryAdapterCompletionStage) []daemonPlannerActionExecutionRegistryAdapterCompletionEntry {
	var entries []daemonPlannerActionExecutionRegistryAdapterCompletionEntry
	for _, entry := range daemonPlannerActionExecutionRegistryAdapterCompletionTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonPlannerActionExecutionRegistryAdapterCompletionSelectedFallbackBoundary() (string, bool) {
	entries := daemonPlannerActionExecutionRegistryAdapterCompletionEntriesForStage(daemonPlannerActionExecutionRegistryAdapterCompletionStageFallbackBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonPlannerActionExecutionTurnCompatibilityRetirementEntryForComponent(component string) (daemonPlannerActionExecutionTurnCompatibilityRetirementEntry, bool) {
	for _, entry := range daemonPlannerActionExecutionTurnCompatibilityRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonPlannerActionExecutionTurnCompatibilityRetirementEntry{}, false
}

func daemonPlannerActionExecutionTurnCompatibilityRetirementEntriesForStage(stage daemonPlannerActionExecutionTurnCompatibilityRetirementStage) []daemonPlannerActionExecutionTurnCompatibilityRetirementEntry {
	var entries []daemonPlannerActionExecutionTurnCompatibilityRetirementEntry
	for _, entry := range daemonPlannerActionExecutionTurnCompatibilityRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonPlannerActionExecutionTurnCompatibilityRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonPlannerActionExecutionTurnCompatibilityRetirementEntriesForStage(daemonPlannerActionExecutionTurnCompatibilityRetirementStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonPlannerActionExecutionDirectLegacyRetirementEntryForComponent(component string) (daemonPlannerActionExecutionDirectLegacyRetirementEntry, bool) {
	for _, entry := range daemonPlannerActionExecutionDirectLegacyRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonPlannerActionExecutionDirectLegacyRetirementEntry{}, false
}

func daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(stage daemonPlannerActionExecutionDirectLegacyRetirementStage) []daemonPlannerActionExecutionDirectLegacyRetirementEntry {
	var entries []daemonPlannerActionExecutionDirectLegacyRetirementEntry
	for _, entry := range daemonPlannerActionExecutionDirectLegacyRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonPlannerActionExecutionDirectLegacyRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(daemonPlannerActionExecutionDirectLegacyRetirementStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonPlannerActionExecutionDirectLegacyRetirementReadyForSingleExecutorEntrypoint() bool {
	for _, entry := range daemonPlannerActionExecutionDirectLegacyRetirementTable {
		if entry.nextBoundary == "" {
			return false
		}
	}
	return len(daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(daemonPlannerActionExecutionDirectLegacyRetirementStageDirectExecutorSurfaceScoped)) == 1 &&
		len(daemonPlannerActionExecutionDirectLegacyRetirementEntriesForStage(daemonPlannerActionExecutionDirectLegacyRetirementStageRuntimeActionServicesCovered)) == 1
}

func daemonPlannerActionExecutionActionExecutorMissingRetirementEntryForComponent(component string) (daemonPlannerActionExecutionActionExecutorMissingRetirementEntry, bool) {
	for _, entry := range daemonPlannerActionExecutionActionExecutorMissingRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonPlannerActionExecutionActionExecutorMissingRetirementEntry{}, false
}

func daemonPlannerActionExecutionActionExecutorMissingRetirementEntriesForStage(stage daemonPlannerActionExecutionActionExecutorMissingRetirementStage) []daemonPlannerActionExecutionActionExecutorMissingRetirementEntry {
	var entries []daemonPlannerActionExecutionActionExecutorMissingRetirementEntry
	for _, entry := range daemonPlannerActionExecutionActionExecutorMissingRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonPlannerActionExecutionActionExecutorMissingRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonPlannerActionExecutionActionExecutorMissingRetirementEntriesForStage(daemonPlannerActionExecutionActionExecutorMissingRetirementStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonPlannerActionExecutionActionExecutorMissingRetirementAllowsDirectFallback(reason daemonPlannerActionExecutionFallbackReason) bool {
	for _, entry := range daemonPlannerActionExecutionActionExecutorMissingRetirementTable {
		if entry.reason == reason &&
			entry.disposition == daemonPlannerActionExecutionActionExecutorMissingRetirementDispositionDirectLegacyFallback {
			return true
		}
	}
	return false
}

func daemonPlannerActionExecutorServiceContractMigrationEntriesForStage(stage daemonPlannerActionExecutorServiceContractMigrationStage) []daemonPlannerActionExecutorServiceContractMigrationEntry {
	var entries []daemonPlannerActionExecutorServiceContractMigrationEntry
	for _, entry := range daemonPlannerActionExecutorServiceContractMigrationTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonPlannerActionExecutorServiceContractHasRuntimeSeam() bool {
	for _, entry := range daemonPlannerActionExecutorServiceContractMigrationTable {
		if entry.component == "runtime_action_executor_seam" &&
			entry.stage == daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded {
			return true
		}
	}
	return false
}

func daemonPlannerActionExecutorServiceContractReadyForAgentLoop() bool {
	if len(daemonPlannerActionExecutorServiceContractMigrationTable) == 0 {
		return false
	}
	for _, entry := range daemonPlannerActionExecutorServiceContractMigrationTable {
		if entry.stage != daemonPlannerActionExecutorServiceContractStageRuntimeContractLanded {
			return false
		}
	}
	return true
}

func daemonAgentLoopRunnerCompletionAuditEntryForMode(mode runtime.AgentLoopMode) (daemonAgentLoopRunnerCompletionAuditEntry, bool) {
	for _, entry := range daemonAgentLoopRunnerCompletionAuditTable {
		if entry.mode == mode {
			return entry, true
		}
	}
	return daemonAgentLoopRunnerCompletionAuditEntry{}, false
}

func daemonAgentLoopRunnerCompletionAuditEntriesForNextRuntimeSeam(nextRuntimeSeam string) []daemonAgentLoopRunnerCompletionAuditEntry {
	var entries []daemonAgentLoopRunnerCompletionAuditEntry
	for _, entry := range daemonAgentLoopRunnerCompletionAuditTable {
		if entry.nextRuntimeSeam == nextRuntimeSeam {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonAgentLoopRunnerCompletionAuditNextRuntimeSeam() string {
	if len(daemonAgentLoopRunnerCompletionAuditTable) == 0 {
		return ""
	}
	next := daemonAgentLoopRunnerCompletionAuditTable[0].nextRuntimeSeam
	for _, entry := range daemonAgentLoopRunnerCompletionAuditTable {
		if entry.nextRuntimeSeam != next {
			return ""
		}
	}
	return next
}

func daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop(loopReq runtime.AgentLoopRequest, action runtime.AgentLoopAction) daemonPlannerActionExecutionSideEffectBoundary {
	loopReq = loopReq.Clone()
	action = cloneAgentLoopAction(action)
	authorization := loopReq.Authorization.Clone()
	return daemonPlannerActionExecutionSideEffectBoundary{
		sessionID:      loopReq.Turn.SessionID,
		language:       loopReq.Language,
		operator:       loopReq.Operator,
		operationLevel: loopReq.Turn.OperationLevel,
		toolServices:   loopReq.ToolServices.Clone(),
		action:         agentPlannerActionFromAgentLoopAction(action),
		authorization:  authorization,
	}
}

func daemonPlannerActionExecutionExecutorForManager(manager *agentManager) daemonPlannerActionExecutionExecutor {
	return daemonLegacyPlannerActionExecutionExecutor{manager: manager}
}

func (executor daemonLegacyPlannerActionExecutionExecutor) ExecutePlannerAction(ctx context.Context, boundary daemonPlannerActionExecutionSideEffectBoundary) (runtime.AgentLoopActionExecutorResult, error) {
	if executor.manager == nil {
		err := errors.New("daemon: planner action execution executor has no manager")
		return runtime.AgentLoopActionExecutorResult{
			Status: runtime.TurnResultBlocked,
			Reason: err.Error(),
			Error:  err,
			Metadata: map[string]any{
				"executor": "daemon_legacy_planner_action_executor",
			},
		}, err
	}
	result, err := executor.manager.executeAgentPlannerActionResultWithToolServices(
		ctx,
		boundary.sessionID,
		boundary.language,
		boundary.operator,
		boundary.operationLevel,
		boundary.toolServices,
		boundary.action,
		boundary.authorization,
	)
	return result, err
}

func daemonPlannerActionExecutionResultFromRuntime(messages []runtime.AgentLoopMessage, toolCalls []runtime.AgentLoopToolCall, err error) runtime.AgentLoopActionExecutorResult {
	result := runtime.AgentLoopActionExecutorResult{
		Status:    runtime.TurnResultFinished,
		Reason:    "daemon legacy planner action executor handled action",
		Messages:  runtime.CloneAgentLoopMessages(messages),
		ToolCalls: runtime.CloneAgentLoopToolCalls(toolCalls),
		Metadata: map[string]any{
			"executor": "daemon_legacy_planner_action_executor",
		},
	}
	if err != nil {
		result.Status = runtime.TurnResultFailed
		result.Reason = err.Error()
		result.Error = err
	}
	return result.Clone()
}

func daemonTurnExecutionCallerFallbackReasonForResult(result runtime.TurnResult, resultErr error) (daemonTurnExecutionCallerFallbackReason, bool) {
	if errors.Is(resultErr, runtime.ErrTurnExecutorNotAttached) ||
		errors.Is(result.Error, runtime.ErrTurnExecutorNotAttached) {
		return daemonTurnExecutionCallerFallbackExecutorNotAttached, true
	}
	if result.Status == runtime.TurnResultBlocked {
		return daemonTurnExecutionCallerFallbackBlocked, true
	}
	if result.Status == runtime.TurnResultRunning && result.Payload == nil {
		return daemonTurnExecutionCallerFallbackRunningWithoutPayload, true
	}
	return "", false
}

func (e daemonAgentTurnExecutor) ExecuteTurn(ctx context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
	if turn.Metadata != nil && turn.Metadata["kind"] == daemonTurnExecutionDeterministicRoute {
		return e.executeDeterministicRouteTurn(ctx, turn)
	}
	if turn.Metadata != nil && turn.Metadata["kind"] == daemonTurnExecutionReplanActionContinuation {
		return e.executeReplanActionContinuationTurn(ctx, turn)
	}
	if turn.Metadata != nil && turn.Metadata["kind"] == daemonTurnExecutionPlannerActionExecution {
		return e.executePlannerActionExecutionTurn(ctx, turn)
	}
	return runtime.TurnResult{
		Status: runtime.TurnResultRunning,
		Reason: "legacy daemon SendMessage owns turn execution",
		Data: map[string]any{
			"executor":         "daemon_legacy_adapter",
			"manager_attached": e.manager != nil,
			"session_id":       turn.SessionID,
			"task_id":          turn.TaskID,
			"run_id":           turn.RunID,
			"turn_id":          turn.TurnID,
		},
	}, nil
}

func (e daemonAgentTurnExecutor) executeDeterministicRouteTurn(ctx context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
	if e.manager == nil {
		err := errors.New("daemon: turn executor has no manager")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	payload, ok := turn.Payload.(runtime.AgentLoopRequest)
	if !ok {
		err := errors.New("daemon: invalid deterministic route turn payload")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loopReq := payload.Clone()
	loopResult, err := e.manager.runAgentDeterministicRouteAgentLoopResult(ctx, loopReq)
	if err != nil {
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	resultPayload := agentLoopTurnResultPayloadFromAgentLoopResult(loopResult)
	return runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Reason:  "legacy daemon deterministic route executed",
		Payload: resultPayload,
		Data: map[string]any{
			"executor":         "daemon_legacy_adapter",
			"kind":             daemonTurnExecutionDeterministicRoute,
			"session_id":       loopReq.Turn.SessionID,
			"task_id":          turn.TaskID,
			"run_id":           turn.RunID,
			"turn_id":          turn.TurnID,
			"message_count":    len(loopResult.Messages),
			"tool_call_count":  len(loopResult.ToolCalls),
			"operation_level":  loopReq.Turn.OperationLevel,
			"manager_attached": true,
		},
	}, nil
}

// executeReplanActionContinuationTurn is the replan-action-continuation branch
// behind runtime.Engine.ExecuteTurn's compatibility seam. Its removal remains
// gated on migrating those AgentLoop callers to runtime.Engine.RunAgentLoop.
func (e daemonAgentTurnExecutor) executeReplanActionContinuationTurn(ctx context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
	if e.manager == nil {
		err := errors.New("daemon: turn executor has no manager")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	payload, ok := turn.Payload.(runtime.AgentLoopRequest)
	if !ok {
		err := errors.New("daemon: invalid replan action continuation turn payload")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loopReq := payload.Clone()
	loopResult, err := e.manager.runRuntimeAgentLoopResult(ctx, loopReq)
	if err != nil {
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	resultPayload := agentLoopTurnResultPayloadFromAgentLoopResult(loopResult)
	return runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Reason:  "legacy daemon replan action continuation executed",
		Payload: resultPayload,
		Data: map[string]any{
			"executor":         "daemon_legacy_adapter",
			"kind":             daemonTurnExecutionReplanActionContinuation,
			"session_id":       loopReq.Turn.SessionID,
			"task_id":          turn.TaskID,
			"run_id":           turn.RunID,
			"turn_id":          turn.TurnID,
			"message_count":    len(loopResult.Messages),
			"tool_call_count":  len(loopResult.ToolCalls),
			"operation_level":  loopReq.Turn.OperationLevel,
			"max_loops":        resultPayload.MaxLoops,
			"loop_count":       resultPayload.LoopCount,
			"stop_reason":      resultPayload.StopReason,
			"manager_attached": true,
		},
	}, nil
}

func (e daemonAgentTurnExecutor) executePlannerActionExecutionTurn(ctx context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
	if e.manager == nil {
		err := errors.New("daemon: turn executor has no manager")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
	if !ok {
		err := errors.New("daemon: invalid planner action execution turn payload")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loopReq := payload.Request.Clone()
	action := payload.Action.Clone()
	actionResult, err := daemonAgentLoopActionExecutor{manager: e.manager}.ExecuteAgentLoopAction(ctx, runtime.AgentLoopActionExecutorRequest{
		Request: loopReq,
		Action:  action,
	})
	messages := runtime.CloneAgentLoopMessages(actionResult.Messages)
	toolCalls := runtime.CloneAgentLoopToolCalls(actionResult.ToolCalls)
	status := runtime.TurnResultFinished
	reason := "legacy daemon planner action executed"
	if err != nil {
		status = runtime.TurnResultFailed
		reason = err.Error()
	}
	return runtime.TurnResult{
		Status: status,
		Reason: reason,
		Error:  err,
		Payload: runtime.AgentLoopTurnResultPayload{
			Messages:  messages,
			ToolCalls: toolCalls,
		},
		Data: map[string]any{
			"executor":         "daemon_legacy_adapter",
			"kind":             daemonTurnExecutionPlannerActionExecution,
			"session_id":       loopReq.Turn.SessionID,
			"task_id":          turn.TaskID,
			"run_id":           turn.RunID,
			"turn_id":          turn.TurnID,
			"action_name":      normalizeAgentPlannerActionName(payload.Action.Name),
			"message_count":    len(messages),
			"tool_call_count":  len(toolCalls),
			"operation_level":  loopReq.Turn.OperationLevel,
			"manager_attached": true,
		},
	}, err
}

func daemonTurnExecutionErrorResult(turn runtime.TurnContext, err error) runtime.TurnResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	kind := daemonTurnExecutionDeterministicRoute
	if turn.Metadata != nil {
		if raw, ok := turn.Metadata["kind"].(string); ok && raw != "" {
			kind = raw
		}
	}
	return runtime.TurnResult{
		Status: runtime.TurnResultFailed,
		Reason: reason,
		Error:  err,
		Data: map[string]any{
			"executor":   "daemon_legacy_adapter",
			"kind":       kind,
			"session_id": turn.SessionID,
			"task_id":    turn.TaskID,
			"run_id":     turn.RunID,
			"turn_id":    turn.TurnID,
		},
	}
}

func (m *agentManager) runRuntimeDeterministicRouteWithToolServices(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding) (runtime.AgentLoopTurnResultPayload, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	loopReq := agentDeterministicRouteRequestFromRuntime(sessionID, turnRef, prompt, language, operator, operationLevel, toolServices)
	loopReq.Turn = agentDeterministicRouteTurnBoundaryFromRequest(loopReq)
	loopResult, err := m.runRuntimeAgentLoopResult(ctx, loopReq)
	return agentLoopTurnResultPayloadFromAgentLoopResult(loopResult), true, err
}

func (m *agentManager) runAgentDeterministicRouteAgentLoopResult(ctx context.Context, loopReq runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
	if m == nil {
		err := errors.New("daemon: agent manager not attached")
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
			Metadata: map[string]any{
				"mode":  string(runtime.AgentLoopModeDeterministicRoute),
				"error": err.Error(),
			},
		}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loopReq = loopReq.Clone()
	turn := loopReq.Turn
	messages, toolCalls, err := m.routePromptForTurnWithToolServices(
		ctx,
		turn.SessionID,
		agentRuntimeTurnRefFromTurn(turn),
		turn.Prompt,
		loopReq.Language,
		loopReq.Operator,
		turn.OperationLevel,
		loopReq.ToolServices,
	)
	metadata := map[string]any{
		"mode":            string(runtime.AgentLoopModeDeterministicRoute),
		"message_count":   len(messages),
		"tool_call_count": len(toolCalls),
	}
	if err != nil {
		metadata["error"] = err.Error()
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
			Metadata:          metadata,
		}, err
	}
	return runtime.AgentLoopResult{
		Messages:   agentLoopMessagesFromAgentMessages(messages),
		ToolCalls:  agentLoopToolCallItemsFromAgentToolCalls(toolCalls),
		StopReason: "deterministic_route_executed",
		Metadata:   metadata,
	}, nil
}

func agentDeterministicRouteRequestFromRuntime(sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding) runtime.AgentLoopRequest {
	loopReq := runtime.AgentLoopTurnExecutionRequest(
		agentPlannerActionReplanCycleTurnContext(sessionID, turnRef, prompt, daemonTurnExecutionDeterministicRoute, operationLevel),
		language,
		operator,
		toolServices,
		nil,
	)
	loopReq.Mode = runtime.AgentLoopModeDeterministicRoute
	return loopReq
}

func agentDeterministicRouteTurnContextFromRequest(loopReq runtime.AgentLoopRequest) runtime.TurnContext {
	return runtime.AgentLoopTurnExecutionTurnFromRequest(loopReq, daemonTurnExecutionDeterministicRoute)
}

func agentDeterministicRouteTurnBoundaryFromRequest(loopReq runtime.AgentLoopRequest) runtime.TurnContext {
	return runtime.AgentLoopTurnExecutionBoundaryFromRequest(loopReq, daemonTurnExecutionDeterministicRoute)
}

func (m *agentManager) routePromptWithRuntimeDeterministicRouteWithToolServices(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding) ([]agentMessage, []agentToolCall, error) {
	routed, handled, err := m.runRuntimeDeterministicRouteWithToolServices(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, toolServices)
	if err != nil {
		return nil, nil, err
	}
	if handled {
		return agentMessagesFromAgentLoopMessages(routed.Messages), agentLoopToolCallItemsToAgentToolCalls(routed.ToolCalls), nil
	}
	return m.routePromptForTurnWithToolServices(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, toolServices)
}

func (m *agentManager) runRuntimeReplanActionContinuationAgentLoop(ctx context.Context, loopReq runtime.AgentLoopRequest) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall) {
	loopReq = normalizeAgentReplanActionContinuationRequest(loopReq)
	fallbackReq := loopReq.Clone()
	if ctx == nil {
		ctx = context.Background()
	}
	turn := agentReplanActionContinuationTurnBoundaryFromRequest(loopReq)
	loopReq.Turn = runtime.CloneAgentLoopTurnContext(turn)
	loopResult, err := m.runRuntimeAgentLoopResult(ctx, loopReq)
	if err != nil {
		return m.runAgentReplanActionContinuationAgentLoop(ctx, fallbackReq)
	}
	return runtime.CloneAgentLoopMessages(loopResult.Messages), runtime.CloneAgentLoopToolCalls(loopResult.ToolCalls)
}

func agentReplanActionContinuationTurnBoundaryFromRequest(loopReq runtime.AgentLoopRequest) runtime.TurnContext {
	return runtime.AgentLoopTurnExecutionBoundaryFromRequest(loopReq, daemonTurnExecutionReplanActionContinuation, map[string]any{
		runtime.AgentLoopTurnExecutionMaxLoopsMetadataKey: loopReq.MaxLoops,
	})
}

func agentLoopTurnResultPayloadFromAgentLoopResult(loopResult runtime.AgentLoopResult) runtime.AgentLoopTurnResultPayload {
	return runtime.AgentLoopTurnResultPayload{
		Messages:   runtime.CloneAgentLoopMessages(loopResult.Messages),
		ToolCalls:  runtime.CloneAgentLoopToolCalls(loopResult.ToolCalls),
		FinalPlan:  runtime.CloneAgentLoopPlan(loopResult.FinalPlan),
		StopReason: loopResult.StopReason,
		LoopCount:  loopResult.LoopCount,
		MaxLoops:   agentLoopResultMaxLoops(loopResult),
		Metadata:   runtime.CloneAgentLoopMapStringAny(loopResult.Metadata),
	}
}

func agentLoopResultMaxLoops(loopResult runtime.AgentLoopResult) int {
	if maxLoops := intFromAny(loopResult.Metadata[runtime.AgentLoopEgressMetadataMaxLoops]); maxLoops != 0 {
		return maxLoops
	}
	return intFromAny(loopResult.Metadata["max_loops"])
}

func (m *agentManager) runRuntimePlannerActionExecution(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding, action runtime.AgentLoopAction, authorization runtime.AgentLoopAuthorization) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, error) {
	toolServices = toolServices.Clone()
	action = cloneAgentLoopAction(action)
	authorization = authorization.Clone()
	loopReq := agentPlannerActionExecutionRequestFromRuntime(sessionID, turnRef, prompt, language, operator, operationLevel, toolServices, authorization)
	loopReq.Mode = runtime.AgentLoopModePlannerActionExecution
	loopReq.Action = action.Clone()

	loopResult, err := m.runRuntimeAgentLoopResult(ctx, loopReq)
	return runtime.CloneAgentLoopMessages(loopResult.Messages), runtime.CloneAgentLoopToolCalls(loopResult.ToolCalls), err
}

func (m *agentManager) runAgentPlannerActionExecutionAgentLoopResult(ctx context.Context, loopReq runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
	if m == nil {
		err := errors.New("daemon: agent manager not attached")
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
			Metadata: map[string]any{
				"mode":  string(runtime.AgentLoopModePlannerActionExecution),
				"error": err.Error(),
			},
		}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loopReq = loopReq.Clone()
	action := cloneAgentLoopAction(loopReq.Action)
	actionName := normalizeAgentPlannerActionName(action.Name)
	if actionName == "" {
		err := errors.New("daemon: planner action execution AgentLoop request missing action")
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
			Metadata: map[string]any{
				"mode":  string(runtime.AgentLoopModePlannerActionExecution),
				"error": err.Error(),
			},
		}, err
	}

	actionResult, handled, err := m.runRuntimePlannerActionExecutionActionExecutor(ctx, loopReq, action)
	if !handled {
		actionResult, err = daemonAgentLoopActionExecutor{manager: m}.ExecuteAgentLoopAction(ctx, runtime.AgentLoopActionExecutorRequest{
			Request: loopReq,
			Action:  action,
		})
	}
	return runtime.AgentLoopPlannerActionExecutionResultFromActionExecutorResult(actionName, actionResult, err), err
}

func (m *agentManager) runRuntimePlannerActionExecutionActionExecutor(ctx context.Context, loopReq runtime.AgentLoopRequest, action runtime.AgentLoopAction) (runtime.AgentLoopActionExecutorResult, bool, error) {
	eng := m.runtimeEngine()
	if reason, ok := daemonPlannerActionExecutionActionExecutorPreDispatchFallbackReason(eng); ok {
		if !daemonPlannerActionExecutionFallbackOwnedByLegacyCaller(reason) {
			return runtime.AgentLoopActionExecutorResult{}, true, daemonPlannerActionExecutionFallbackOwnershipError(reason)
		}
		if !daemonPlannerActionExecutionActionExecutorMissingRetirementAllowsDirectFallback(reason) {
			return runtime.AgentLoopActionExecutorResult{}, true, daemonPlannerActionExecutionFallbackOwnershipError(reason)
		}
		return runtime.AgentLoopActionExecutorResult{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := eng.ExecuteAgentLoopAction(ctx, runtime.AgentLoopActionExecutorRequest{
		Request: loopReq,
		Action:  action,
	})
	if blocked, ok := daemonPlannerActionExecutionActionExecutorNotAttachedBlockedResult(result, err); ok {
		return blocked, true, err
	}
	if reason, ok := daemonPlannerActionExecutionActionExecutorResultFallbackReason(result, err); ok {
		if !daemonPlannerActionExecutionFallbackOwnedByLegacyCaller(reason) {
			return runtime.AgentLoopActionExecutorResult{}, true, daemonPlannerActionExecutionFallbackOwnershipError(reason)
		}
		if !daemonPlannerActionExecutionActionExecutorMissingRetirementAllowsDirectFallback(reason) {
			return runtime.AgentLoopActionExecutorResult{}, true, daemonPlannerActionExecutionFallbackOwnershipError(reason)
		}
		return runtime.AgentLoopActionExecutorResult{}, false, nil
	}
	return result, true, err
}

func agentTurnExecutionResultPayloadForKind(result runtime.TurnResult, resultErr error, kind string) (runtime.AgentLoopTurnResultPayload, bool, error) {
	if !daemonTurnExecutionCallerPolicyMatches(kind, daemonTurnExecutionCallerPolicyStrictResult) {
		return runtime.AgentLoopTurnResultPayload{}, true, daemonTurnExecutionCallerPolicyMismatchError(kind, daemonTurnExecutionCallerPolicyStrictResult)
	}
	return agentTurnExecutionResultPayload(result, resultErr, daemonTurnExecutionCallerInvalidPayloadError(kind))
}

func agentTurnExecutionResultPayload(result runtime.TurnResult, resultErr error, invalidPayloadError string) (runtime.AgentLoopTurnResultPayload, bool, error) {
	if reason, ok := daemonTurnExecutionCallerFallbackReasonForResult(result, resultErr); ok {
		if !daemonTurnExecutionCallerFallbackOwnedByLegacyCaller(reason) {
			return runtime.AgentLoopTurnResultPayload{}, true, daemonTurnExecutionCallerFallbackOwnershipError(reason)
		}
		return runtime.AgentLoopTurnResultPayload{}, false, nil
	}
	payload, ok := agentLoopTurnResultPayloadFromTurnResult(result.Payload)
	if ok {
		return payload, true, resultErr
	}
	if resultErr != nil {
		return runtime.AgentLoopTurnResultPayload{}, true, resultErr
	}
	return runtime.AgentLoopTurnResultPayload{}, true, errors.New(invalidPayloadError)
}

func agentLoopTurnResultPayloadFromTurnResult(payload any) (runtime.AgentLoopTurnResultPayload, bool) {
	switch value := payload.(type) {
	case runtime.AgentLoopTurnResultPayload:
		return value.Clone(), true
	case *runtime.AgentLoopTurnResultPayload:
		if value == nil {
			return runtime.AgentLoopTurnResultPayload{}, false
		}
		return value.Clone(), true
	default:
		return runtime.AgentLoopTurnResultPayload{}, false
	}
}

func agentPlannerActionExecutionRequestFromRuntime(sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding, authorization runtime.AgentLoopAuthorization) runtime.AgentLoopRequest {
	return runtime.AgentLoopActionExecutionRequest(
		agentPlannerActionReplanCycleTurnContext(sessionID, turnRef, prompt, daemonTurnExecutionPlannerActionExecution, operationLevel),
		language,
		operator,
		toolServices,
		authorization,
		nil,
	)
}
