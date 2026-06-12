package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

// Session mutation result payloads below are daemon adapter DTOs consumed by
// legacy callers. Runtime-neutral mutation input payloads live under the
// runtime package; session mutation results roundtrip through runtime AgentLoop
// session snapshot/result DTOs before converting back at this boundary.
type daemonAppendMessagesMutationResultPayload struct {
	Session    agentSession
	Compaction *agentMemoryCompaction
	Mutated    bool
}

type daemonRecordUserMessageMutationResultPayload struct {
	Session       agentSession
	UserMessage   agentMessage
	History       []agentMessage
	MemorySummary *agentMemorySummary
	Compaction    *agentMemoryCompaction
	Mutated       bool
}

type daemonRequestAuthorizationMutationResultPayload struct {
	Session       agentSession
	Authorization agentAuthorizationRequest
	Message       agentMessage
	Mutated       bool
}

type daemonCapabilityBlockedMutationResultPayload struct {
	Session         agentSession
	ActionName      string
	IntentID        string
	Reason          string
	SuggestedPrompt string
	Mutated         bool
}

type daemonResolveAuthorizationMutationResultPayload struct {
	Session        agentSession
	Pending        agentAuthorizationRequest
	Message        agentMessage
	OperationLevel string
	Compaction     *agentMemoryCompaction
	Mutated        bool
}

type daemonControlSessionMutationResultPayload struct {
	Session     agentSession
	Message     agentMessage
	CancelJobID string
	Compaction  *agentMemoryCompaction
	Mutated     bool
}

type daemonSessionMutator struct {
	manager *agentManager
}

type agentLoopSessionMutationPayloadCloner[T any] interface {
	Clone() T
}

func cloneAgentLoopSessionMutationPayload[T agentLoopSessionMutationPayloadCloner[T]](payload any) (T, bool) {
	var zero T
	if value, ok := payload.(T); ok {
		return value.Clone(), true
	}
	if value, ok := payload.(*T); ok {
		if value == nil {
			return zero, false
		}
		return (*value).Clone(), true
	}
	return zero, false
}

// daemonLegacySessionMutationResultPayload is the adapter-only compatibility
// fallback for callers that still return daemon legacy result payload structs.
// daemonSessionMutator.MutateSession must return runtime AgentLoop result DTOs;
// legacy structs are only consumed by daemon egress adapters/caller shims.
func daemonLegacySessionMutationResultPayload[T any](result runtime.SessionMutationResult) (T, bool) {
	var zero T
	if payload, ok := result.Payload.(T); ok {
		return payload, true
	}
	return zero, false
}

type daemonSessionMutationHandler func(daemonSessionMutator, context.Context, runtime.SessionMutationRequest) (runtime.SessionMutationResult, error)

type daemonSessionMutationDispatchEntry struct {
	kind    runtime.SessionMutationKind
	handler daemonSessionMutationHandler
}

var daemonSessionMutationDispatchTable = [...]daemonSessionMutationDispatchEntry{
	{kind: runtime.SessionMutationAppendMessages, handler: daemonSessionMutator.mutateAppendMessages},
	{kind: runtime.SessionMutationRecordUserMessage, handler: daemonSessionMutator.mutateRecordUserMessage},
	{kind: runtime.SessionMutationRequestAuthorization, handler: daemonSessionMutator.mutateRequestAuthorization},
	{kind: runtime.SessionMutationCapabilityBlocked, handler: daemonSessionMutator.mutateCapabilityBlocked},
	{kind: runtime.SessionMutationControlSession, handler: daemonSessionMutator.mutateControlSession},
	{kind: runtime.SessionMutationResolveAuthorization, handler: daemonSessionMutator.mutateResolveAuthorization},
}

func daemonSessionMutationDispatchForKind(kind runtime.SessionMutationKind) (daemonSessionMutationHandler, bool) {
	for _, entry := range daemonSessionMutationDispatchTable {
		if entry.kind == kind && entry.handler != nil {
			return entry.handler, true
		}
	}
	return nil, false
}

func daemonSessionMutationPayload[T agentLoopSessionMutationPayloadCloner[T]](req runtime.SessionMutationRequest, invalidPayloadError string) (T, error) {
	payload, ok := cloneAgentLoopSessionMutationPayload[T](req.Payload)
	if ok {
		return payload, nil
	}
	var zero T
	return zero, errors.New(invalidPayloadError)
}

func (r daemonSessionMutator) MutateSession(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	if r.manager == nil {
		err := errors.New("daemon: session mutator has no manager")
		return daemonSessionMutationErrorResult(req, err), err
	}

	if handler, ok := daemonSessionMutationDispatchForKind(req.Kind); ok {
		return handler(r, ctx, req)
	}

	err := errors.New("daemon: unsupported session mutation kind")
	return daemonSessionMutationErrorResult(req, err), err
}

func (r daemonSessionMutator) mutateAppendMessages(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	payload, err := daemonSessionMutationPayload[runtime.AgentLoopAppendMessagesPayload](req, "daemon: invalid append messages mutation payload")
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	sessionID := req.Turn.SessionID
	plan := agentPlannerResponsePtrFromAgentLoopPlan(payload.Plan)
	messages := agentMessagesFromAgentLoopMessages(payload.Messages)
	resultPayload, mutated := r.manager.appendMessagesWithPlannerLegacyForTurn(ctx, sessionID, agentRuntimeTurnRefFromTurn(req.Turn), plan, messages...)
	resultPayload.Mutated = mutated
	return daemonSessionMutationFinishedResult(
		"legacy daemon session append mutation executed",
		agentLoopAppendMessagesResultPayloadFromDaemon(resultPayload),
		map[string]any{
			"mutator":       "daemon_legacy_session",
			"kind":          string(req.Kind),
			"session_id":    sessionID,
			"message_count": len(payload.Messages),
			"planner_apply": payload.Plan != nil,
			"mutated":       mutated,
		},
	), nil
}

func (r daemonSessionMutator) mutateRecordUserMessage(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	payload, err := daemonSessionMutationPayload[runtime.AgentLoopRecordUserMessagePayload](req, "daemon: invalid record user message mutation payload")
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	sessionID := req.Turn.SessionID
	prompt := req.Turn.Prompt
	operationLevel := req.Turn.OperationLevel
	resultPayload, err := r.manager.recordUserMessageLegacy(sessionID, prompt, payload.Operator, payload.Language, operationLevel)
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	resultPayload.Mutated = true
	return daemonSessionMutationFinishedResult(
		"legacy daemon user message mutation executed",
		agentLoopRecordUserMessageResultPayloadFromDaemon(resultPayload),
		map[string]any{
			"mutator":         "daemon_legacy_session",
			"kind":            string(req.Kind),
			"session_id":      sessionID,
			"message_id":      resultPayload.UserMessage.ID,
			"operation_level": operationLevel,
			"mutated":         true,
		},
	), nil
}

func (r daemonSessionMutator) mutateRequestAuthorization(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	payload, err := daemonSessionMutationPayload[runtime.AgentLoopAuthorizationRequestPayload](req, "daemon: invalid request authorization mutation payload")
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	sessionID := req.Turn.SessionID
	operatorPrompt := req.Turn.Prompt
	operationLevel := req.Turn.OperationLevel
	action := agentPlannerActionFromAgentLoopAction(payload.Action)
	resultPayload := r.manager.requestAgentAuthorizationLegacy(sessionID, payload.Language, operatorPrompt, action, payload.ActionName, payload.RequiredLevel, payload.Policy, payload.Reason)
	return daemonSessionMutationFinishedResult(
		"legacy daemon authorization request mutation executed",
		agentLoopRequestAuthorizationResultPayloadFromDaemon(resultPayload),
		map[string]any{
			"mutator":          "daemon_legacy_session",
			"kind":             string(req.Kind),
			"session_id":       sessionID,
			"authorization_id": resultPayload.Authorization.ID,
			"action_name":      resultPayload.Authorization.ActionName,
			"required_level":   resultPayload.Authorization.RequiredLevel,
			"policy":           resultPayload.Authorization.Policy,
			"message_id":       resultPayload.Message.ID,
			"operation_level":  operationLevel,
			"mutated":          resultPayload.Mutated,
		},
	), nil
}

func (r daemonSessionMutator) mutateCapabilityBlocked(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	payload, err := daemonSessionMutationPayload[runtime.AgentLoopCapabilityBlockedPayload](req, "daemon: invalid capability blocked mutation payload")
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	sessionID := req.Turn.SessionID
	operationLevel := req.Turn.OperationLevel
	resultPayload := r.manager.markAgentCapabilityBlockedLegacy(sessionID, payload.Language, payload.ActionName, payload.IntentID, payload.Reason, payload.SuggestedPrompt)
	return daemonSessionMutationFinishedResult(
		"legacy daemon capability blocked mutation executed",
		agentLoopCapabilityBlockedResultPayloadFromDaemon(resultPayload),
		map[string]any{
			"mutator":          "daemon_legacy_session",
			"kind":             string(req.Kind),
			"session_id":       sessionID,
			"action_name":      resultPayload.ActionName,
			"intent_id":        resultPayload.IntentID,
			"suggested_prompt": resultPayload.SuggestedPrompt,
			"operation_level":  operationLevel,
			"mutated":          resultPayload.Mutated,
		},
	), nil
}

func (r daemonSessionMutator) mutateControlSession(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	payload, err := daemonSessionMutationPayload[runtime.AgentLoopControlSessionPayload](req, "daemon: invalid control session mutation payload")
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	sessionID := req.Turn.SessionID
	prompt := req.Turn.Prompt
	operationLevel := req.Turn.OperationLevel
	resultPayload, err := r.manager.controlSessionLegacyForTurn(ctx, agentRuntimeTurnRefFromTurn(req.Turn), sessionID, payload.Action, prompt, payload.Reason, payload.RunID, payload.JobID, payload.ClearInput, payload.Operator, payload.Language, operationLevel)
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	resultPayload.Mutated = true
	return daemonSessionMutationFinishedResult(
		"legacy daemon control session mutation executed",
		agentLoopControlSessionResultPayloadFromDaemon(resultPayload),
		map[string]any{
			"mutator":         "daemon_legacy_session",
			"kind":            string(req.Kind),
			"session_id":      sessionID,
			"action":          payload.Action,
			"message_id":      resultPayload.Message.ID,
			"cancel_job_id":   resultPayload.CancelJobID,
			"operation_level": operationLevel,
			"mutated":         true,
		},
	), nil
}

func (r daemonSessionMutator) mutateResolveAuthorization(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	payload, err := daemonSessionMutationPayload[runtime.AgentLoopResolveAuthorizationPayload](req, "daemon: invalid resolve authorization mutation payload")
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	sessionID := req.Turn.SessionID
	operationLevel := req.Turn.OperationLevel
	resultPayload, err := r.manager.resolveAuthorizationLegacyForTurn(ctx, agentRuntimeTurnRefFromTurn(req.Turn), sessionID, payload.Decision, payload.AuthorizationID, payload.Reason, payload.Operator, payload.Language, operationLevel, payload.OperationLevelProvided)
	if err != nil {
		return daemonSessionMutationErrorResult(req, err), err
	}
	resultPayload.Mutated = true
	return daemonSessionMutationFinishedResult(
		"legacy daemon authorization mutation executed",
		agentLoopResolveAuthorizationResultPayloadFromDaemon(resultPayload),
		map[string]any{
			"mutator":          "daemon_legacy_session",
			"kind":             string(req.Kind),
			"session_id":       sessionID,
			"authorization_id": resultPayload.Pending.ID,
			"decision":         payload.Decision,
			"operation_level":  resultPayload.OperationLevel,
			"message_id":       resultPayload.Message.ID,
			"mutated":          true,
		},
	), nil
}

func daemonSessionMutationFinishedResult(reason string, payload any, data map[string]any) runtime.SessionMutationResult {
	return runtime.SessionMutationResult{
		Status:  runtime.TurnResultFinished,
		Reason:  reason,
		Payload: payload,
		Data:    runtime.CloneAgentLoopMapStringAny(data),
	}
}

func daemonSessionMutationErrorResult(req runtime.SessionMutationRequest, err error) runtime.SessionMutationResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return runtime.SessionMutationResult{
		Status: runtime.TurnResultFailed,
		Reason: reason,
		Error:  err,
		Data: map[string]any{
			"mutator":    "daemon_legacy_session",
			"kind":       string(req.Kind),
			"session_id": req.Turn.SessionID,
			"task_id":    req.Turn.TaskID,
			"run_id":     req.Turn.RunID,
			"turn_id":    req.Turn.TurnID,
		},
	}
}

type daemonSessionMutationCallerPayloadExtractor[T any] func(runtime.SessionMutationResult) (T, bool)

type daemonSessionMutationCallerEgress struct {
	result    runtime.SessionMutationResult
	handled   bool
	resultErr error
}

type daemonSessionMutationCallerInvalidPayloadEntry struct {
	kind    runtime.SessionMutationKind
	message string
}

type daemonSessionMutationCallerPolicy string

const (
	daemonSessionMutationCallerPolicyBoolOnly       daemonSessionMutationCallerPolicy = "bool_only"
	daemonSessionMutationCallerPolicyStrictResult   daemonSessionMutationCallerPolicy = "strict_result"
	daemonSessionMutationCallerPolicyOptionalResult daemonSessionMutationCallerPolicy = "optional_result"
)

type daemonSessionMutationCallerPolicyEntry struct {
	kind   runtime.SessionMutationKind
	policy daemonSessionMutationCallerPolicy
}

type daemonSessionMutationCallerFallbackReason string

const (
	daemonSessionMutationCallerFallbackEngineNotAttached  daemonSessionMutationCallerFallbackReason = "runtime_engine_not_attached"
	daemonSessionMutationCallerFallbackMutatorNotAttached daemonSessionMutationCallerFallbackReason = "session_mutator_not_attached"
	daemonSessionMutationCallerFallbackBlocked            daemonSessionMutationCallerFallbackReason = "blocked"
)

const daemonSessionMutationCallerFallbackOwnerLegacyCaller = "legacy_caller"

type daemonSessionMutationCallerFallbackOwnershipEntry struct {
	reason daemonSessionMutationCallerFallbackReason
	owner  string
}

type daemonSharedCycleSessionMutationFallbackStage string

const (
	daemonSharedCycleSessionMutationFallbackStageRuntimeAppendDecisionOwned daemonSharedCycleSessionMutationFallbackStage = "runtime_append_decision_owned"
	daemonSharedCycleSessionMutationFallbackStageDaemonMutationDispatch     daemonSharedCycleSessionMutationFallbackStage = "daemon_session_mutation_dispatch"
	daemonSharedCycleSessionMutationFallbackStageLegacyFallbackRetained     daemonSharedCycleSessionMutationFallbackStage = "legacy_append_fallback_retained"
	daemonSharedCycleSessionMutationFallbackStageNextBoundaryChosen         daemonSharedCycleSessionMutationFallbackStage = "attached_session_mutator_fallback_boundary_chosen"
)

const (
	daemonSharedCycleSessionMutationFallbackDispositionRuntimeDecision        = "runtime_decision"
	daemonSharedCycleSessionMutationFallbackDispositionDaemonDispatch         = "daemon_dispatch"
	daemonSharedCycleSessionMutationFallbackDispositionLegacyFallbackRetained = "legacy_fallback_retained"
	daemonSharedCycleSessionMutationFallbackDispositionAuditNextBoundary      = "audit_next_boundary"
)

const daemonSharedCycleSessionMutationFallbackNextBoundary = "shared_cycle_attached_session_mutator_fallback_retirement_readiness"

type daemonSharedCycleSessionMutationFallbackEntry struct {
	component    string
	stage        daemonSharedCycleSessionMutationFallbackStage
	mutationKind runtime.SessionMutationKind
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonSessionMutatorNotAttachedRetirementStage string

const (
	daemonSessionMutatorNotAttachedStageNoEngineFallbackRetained daemonSessionMutatorNotAttachedRetirementStage = "no_engine_session_mutation_legacy_fallback_retained"
	daemonSessionMutatorNotAttachedStageMissingMutatorBlocked    daemonSessionMutatorNotAttachedRetirementStage = "attached_engine_missing_session_mutator_blocked"
	daemonSessionMutatorNotAttachedStageDetachedResultBlocked    daemonSessionMutatorNotAttachedRetirementStage = "detached_session_mutator_result_blocked"
	daemonSessionMutatorNotAttachedStageNextBoundaryChosen       daemonSessionMutatorNotAttachedRetirementStage = "session_mutation_blocked_or_error_boundary_chosen"
)

const (
	daemonSessionMutatorNotAttachedDispositionLegacyFallback    = "legacy_fallback"
	daemonSessionMutatorNotAttachedDispositionBlockedError      = "blocked_error"
	daemonSessionMutatorNotAttachedDispositionAuditNextBoundary = "audit_next_boundary"
)

const daemonSessionMutatorNextFallbackRetirementBoundaryBlockedOrError = "session_mutation_blocked_or_error_fallback_retirement_readiness"

type daemonSessionMutatorNotAttachedRetirementEntry struct {
	component    string
	stage        daemonSessionMutatorNotAttachedRetirementStage
	reason       daemonSessionMutationCallerFallbackReason
	mutationKind runtime.SessionMutationKind
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonAppendSessionMutationBlockedOrErrorRetirementStage string

const (
	daemonAppendSessionMutationBlockedOrErrorStageNoEngineFallbackRetained daemonAppendSessionMutationBlockedOrErrorRetirementStage = "no_engine_append_legacy_fallback_retained"
	daemonAppendSessionMutationBlockedOrErrorStageBlockedResultSuppressed  daemonAppendSessionMutationBlockedOrErrorRetirementStage = "attached_session_mutator_blocked_result_blocks_legacy_append"
	daemonAppendSessionMutationBlockedOrErrorStageHardErrorSuppressed      daemonAppendSessionMutationBlockedOrErrorRetirementStage = "attached_session_mutator_hard_error_blocks_legacy_append"
	daemonAppendSessionMutationBlockedOrErrorStageNextBoundaryChosen       daemonAppendSessionMutationBlockedOrErrorRetirementStage = "session_mutation_optional_result_fallback_boundary_chosen"
)

const (
	daemonAppendSessionMutationBlockedOrErrorDispositionLegacyFallback    = "legacy_fallback"
	daemonAppendSessionMutationBlockedOrErrorDispositionBlockedError      = "blocked_error"
	daemonAppendSessionMutationBlockedOrErrorDispositionHardError         = "hard_error"
	daemonAppendSessionMutationBlockedOrErrorDispositionAuditNextBoundary = "audit_next_boundary"
)

const daemonAppendSessionMutationNextFallbackRetirementBoundaryOptionalResult = "session_mutation_optional_result_fallback_retirement_readiness"

type daemonAppendSessionMutationBlockedOrErrorRetirementEntry struct {
	component    string
	stage        daemonAppendSessionMutationBlockedOrErrorRetirementStage
	reason       daemonSessionMutationCallerFallbackReason
	mutationKind runtime.SessionMutationKind
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonOptionalSessionMutationFallbackRetirementStage string

const (
	daemonOptionalSessionMutationFallbackStageNoEngineFallbackRetained       daemonOptionalSessionMutationFallbackRetirementStage = "no_engine_optional_result_legacy_fallback_retained"
	daemonOptionalSessionMutationFallbackStageRequestAuthorizationSuppressed daemonOptionalSessionMutationFallbackRetirementStage = "request_authorization_attached_failure_blocks_legacy_caller"
	daemonOptionalSessionMutationFallbackStageCapabilityBlockedSuppressed    daemonOptionalSessionMutationFallbackRetirementStage = "capability_blocked_attached_failure_blocks_legacy_caller"
	daemonOptionalSessionMutationFallbackStageNextBoundaryChosen             daemonOptionalSessionMutationFallbackRetirementStage = "session_mutation_no_engine_fallback_boundary_chosen"
)

const (
	daemonOptionalSessionMutationFallbackDispositionLegacyFallback    = "legacy_fallback"
	daemonOptionalSessionMutationFallbackDispositionBlockedError      = "blocked_error"
	daemonOptionalSessionMutationFallbackDispositionAuditNextBoundary = "audit_next_boundary"
)

const daemonOptionalSessionMutationNextFallbackRetirementBoundaryNoEngineFallback = "session_mutation_no_engine_legacy_fallback_retirement_readiness"

type daemonOptionalSessionMutationFallbackRetirementEntry struct {
	component    string
	stage        daemonOptionalSessionMutationFallbackRetirementStage
	reason       daemonSessionMutationCallerFallbackReason
	mutationKind runtime.SessionMutationKind
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

type daemonNoEngineSessionMutationFallbackRetirementStage string

const (
	daemonNoEngineSessionMutationFallbackStageProductionDefaultEngineAttached daemonNoEngineSessionMutationFallbackRetirementStage = "production_default_engine_attached"
	daemonNoEngineSessionMutationFallbackStageLegacyRuntimeOptOutRetained     daemonNoEngineSessionMutationFallbackRetirementStage = "legacy_runtime_opt_out_switch_retained"
	daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained        daemonNoEngineSessionMutationFallbackRetirementStage = "no_engine_session_mutation_legacy_fallback_retained"
	daemonNoEngineSessionMutationFallbackStageNextBoundaryChosen              daemonNoEngineSessionMutationFallbackRetirementStage = "manual_stage_commit_tag_authorization_boundary_chosen"
)

const (
	daemonNoEngineSessionMutationFallbackDispositionRuntimeDefault       = "runtime_default"
	daemonNoEngineSessionMutationFallbackDispositionRollbackSwitch       = "rollback_switch"
	daemonNoEngineSessionMutationFallbackDispositionLegacyFallback       = "legacy_fallback"
	daemonNoEngineSessionMutationFallbackDispositionManualGateNext       = "manual_gate_next"
	daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization = "await_explicit_user_stage_commit_tag_authorization"
)

type daemonNoEngineSessionMutationFallbackRetirementEntry struct {
	component    string
	stage        daemonNoEngineSessionMutationFallbackRetirementStage
	reason       daemonSessionMutationCallerFallbackReason
	mutationKind runtime.SessionMutationKind
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

const daemonSessionMutationCallerDefaultInvalidPayloadError = "daemon: session mutator returned invalid payload"

var daemonSessionMutationCallerInvalidPayloadTable = [...]daemonSessionMutationCallerInvalidPayloadEntry{
	{kind: runtime.SessionMutationAppendMessages, message: "daemon: append messages mutator returned invalid payload"},
	{kind: runtime.SessionMutationRecordUserMessage, message: "daemon: record user message mutator returned invalid payload"},
	{kind: runtime.SessionMutationRequestAuthorization, message: "daemon: request authorization mutator returned invalid payload"},
	{kind: runtime.SessionMutationCapabilityBlocked, message: "daemon: capability blocked mutator returned invalid payload"},
	{kind: runtime.SessionMutationControlSession, message: "daemon: control session mutator returned invalid payload"},
	{kind: runtime.SessionMutationResolveAuthorization, message: "daemon: resolve authorization mutator returned invalid payload"},
}

var daemonSessionMutationCallerPolicyTable = [...]daemonSessionMutationCallerPolicyEntry{
	{kind: runtime.SessionMutationAppendMessages, policy: daemonSessionMutationCallerPolicyBoolOnly},
	{kind: runtime.SessionMutationRecordUserMessage, policy: daemonSessionMutationCallerPolicyStrictResult},
	{kind: runtime.SessionMutationRequestAuthorization, policy: daemonSessionMutationCallerPolicyOptionalResult},
	{kind: runtime.SessionMutationCapabilityBlocked, policy: daemonSessionMutationCallerPolicyOptionalResult},
	{kind: runtime.SessionMutationControlSession, policy: daemonSessionMutationCallerPolicyStrictResult},
	{kind: runtime.SessionMutationResolveAuthorization, policy: daemonSessionMutationCallerPolicyStrictResult},
}

var daemonSessionMutationCallerFallbackOwnershipTable = [...]daemonSessionMutationCallerFallbackOwnershipEntry{
	{reason: daemonSessionMutationCallerFallbackEngineNotAttached, owner: daemonSessionMutationCallerFallbackOwnerLegacyCaller},
	{reason: daemonSessionMutationCallerFallbackBlocked, owner: daemonSessionMutationCallerFallbackOwnerLegacyCaller},
}

var daemonSharedCycleSessionMutationFallbackTable = [...]daemonSharedCycleSessionMutationFallbackEntry{
	{
		component:    "runtime_replan_cycle_append_decision",
		stage:        daemonSharedCycleSessionMutationFallbackStageRuntimeAppendDecisionOwned,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonSharedCycleSessionMutationFallbackDispositionRuntimeDecision,
		evidence:     "planner_action_replan_cycle action/warning/synthesis placement and state egress are runtime-owned by AgentLoopReplanCycleApply*Append helpers before daemon dispatches append mutations",
		nextBoundary: daemonSharedCycleSessionMutationFallbackNextBoundary,
		nextStep:     "keep append placement in runtime DTO helpers while auditing the remaining daemon SessionMutation dispatch/fallback boundary",
	},
	{
		component:    "shared_cycle_append_mutation_dispatch",
		stage:        daemonSharedCycleSessionMutationFallbackStageDaemonMutationDispatch,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonSharedCycleSessionMutationFallbackDispositionDaemonDispatch,
		evidence:     "appendAgentPlannerActionReplanCycleDecisionForTurn converts runtime append decisions to appendMessagesForTurn/appendMessagesWithPlannerForTurn, which build runtime SessionMutationAppendMessages requests",
		nextBoundary: daemonSharedCycleSessionMutationFallbackNextBoundary,
		nextStep:     "audit whether attached-engine SessionMutator nil/ErrSessionMutatorNotAttached should stay legacy-fallback or become blocked/error",
	},
	{
		component:    "append_messages_legacy_fallback",
		stage:        daemonSharedCycleSessionMutationFallbackStageLegacyFallbackRetained,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonSharedCycleSessionMutationFallbackDispositionLegacyFallbackRetained,
		evidence:     "appendMessagesWithPlannerForTurn falls back to appendMessagesWithPlannerLegacyForTurn whenever runRuntimeAppendMessagesMutationForTurn is not handled; runRuntimeSessionMutation still treats no-engine, mutator-not-attached and blocked as legacy caller fallback reasons",
		nextBoundary: daemonSharedCycleSessionMutationFallbackNextBoundary,
		nextStep:     "split no-engine rollback fallback from attached-engine mutator missing/detached fallback before changing append behavior",
	},
	{
		component:    "shared_cycle_session_mutation_next_boundary",
		stage:        daemonSharedCycleSessionMutationFallbackStageNextBoundaryChosen,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonSharedCycleSessionMutationFallbackDispositionAuditNextBoundary,
		evidence:     "nested dispatch fallback is source-audited and shared-cycle append placement is runtime-owned; the remaining daemon-owned edge is append SessionMutation dispatch/fallback",
		nextBoundary: daemonSharedCycleSessionMutationFallbackNextBoundary,
		nextStep:     "P0.5-N234 should decide whether attached-engine SessionMutator missing/detached fallback can be narrowed without breaking the explicit no-engine legacy runtime opt-out path",
	},
}

var daemonSessionMutatorNotAttachedRetirementTable = [...]daemonSessionMutatorNotAttachedRetirementEntry{
	{
		component:    "no_engine_session_mutation_legacy_fallback",
		stage:        daemonSessionMutatorNotAttachedStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		disposition:  daemonSessionMutatorNotAttachedDispositionLegacyFallback,
		evidence:     "no-engine SessionMutation fallback remains scoped to the documented CYBERAGENT_RUNTIME legacy opt-out path, matching the AgentLoop/LLMStage no-engine fallback audits",
		nextBoundary: daemonSessionMutatorNextFallbackRetirementBoundaryBlockedOrError,
		nextStep:     "do not retire the no-engine legacy append/session mutation fallback until the explicit legacy runtime opt-out switch is removed",
	},
	{
		component:    "attached_engine_missing_session_mutator",
		stage:        daemonSessionMutatorNotAttachedStageMissingMutatorBlocked,
		reason:       daemonSessionMutationCallerFallbackMutatorNotAttached,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonSessionMutatorNotAttachedDispositionBlockedError,
		evidence:     "agentManager.AttachEngine attaches daemonSessionMutator, so a nil SessionMutator on an attached engine represents an explicit detach/miswire and now returns runtime.ErrSessionMutatorNotAttached instead of falling through to legacy append",
		nextBoundary: daemonSessionMutatorNextFallbackRetirementBoundaryBlockedOrError,
		nextStep:     "treat attached-engine missing SessionMutator as a blocked runtime configuration error for shared-cycle append dispatch",
	},
	{
		component:    "detached_session_mutator_result",
		stage:        daemonSessionMutatorNotAttachedStageDetachedResultBlocked,
		reason:       daemonSessionMutationCallerFallbackMutatorNotAttached,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonSessionMutatorNotAttachedDispositionBlockedError,
		evidence:     "ErrSessionMutatorNotAttached returned by Engine.MutateSession is normalized to blocked/error and no longer allows appendMessagesWithPlannerForTurn to execute the legacy append fallback",
		nextBoundary: daemonSessionMutatorNextFallbackRetirementBoundaryBlockedOrError,
		nextStep:     "continue by auditing remaining SessionMutation blocked/hard-error fallback behavior after the not-attached edge is narrowed",
	},
	{
		component:    "session_mutation_next_fallback_boundary",
		stage:        daemonSessionMutatorNotAttachedStageNextBoundaryChosen,
		disposition:  daemonSessionMutatorNotAttachedDispositionAuditNextBoundary,
		evidence:     "no-engine SessionMutation fallback is retained for explicit runtime opt-out; attached-engine SessionMutator nil/ErrSessionMutatorNotAttached is blocked/error and no longer writes via legacy append",
		nextBoundary: daemonSessionMutatorNextFallbackRetirementBoundaryBlockedOrError,
		nextStep:     "P0.5-N235 should decide whether generic blocked results or hard errors from an attached SessionMutator may still fall back to legacy callers",
	},
}

var daemonAppendSessionMutationBlockedOrErrorRetirementTable = [...]daemonAppendSessionMutationBlockedOrErrorRetirementEntry{
	{
		component:    "no_engine_append_legacy_fallback",
		stage:        daemonAppendSessionMutationBlockedOrErrorStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonAppendSessionMutationBlockedOrErrorDispositionLegacyFallback,
		evidence:     "no-engine append fallback remains scoped to the documented CYBERAGENT_RUNTIME legacy opt-out path",
		nextBoundary: daemonAppendSessionMutationNextFallbackRetirementBoundaryOptionalResult,
		nextStep:     "do not retire the no-engine append fallback until the explicit legacy runtime opt-out switch is removed",
	},
	{
		component:    "attached_session_mutator_blocked_result",
		stage:        daemonAppendSessionMutationBlockedOrErrorStageBlockedResultSuppressed,
		reason:       daemonSessionMutationCallerFallbackBlocked,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonAppendSessionMutationBlockedOrErrorDispositionBlockedError,
		evidence:     "a blocked SessionMutationAppendMessages result from an attached SessionMutator is now treated as handled for append callers, preventing appendMessagesWithPlannerForTurn from executing legacy append fallback",
		nextBoundary: daemonAppendSessionMutationNextFallbackRetirementBoundaryOptionalResult,
		nextStep:     "propagate or surface blocked append diagnostics separately if a UI-facing error channel is added",
	},
	{
		component:    "attached_session_mutator_hard_error",
		stage:        daemonAppendSessionMutationBlockedOrErrorStageHardErrorSuppressed,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonAppendSessionMutationBlockedOrErrorDispositionHardError,
		evidence:     "a non-fallback hard error from an attached SessionMutator is now treated as handled for append callers, preventing silent legacy writes after runtime mutation failure",
		nextBoundary: daemonAppendSessionMutationNextFallbackRetirementBoundaryOptionalResult,
		nextStep:     "continue by auditing optional-result session mutation callers whose signatures can still hide attached mutator errors by falling back to legacy callers",
	},
	{
		component:    "session_mutation_next_fallback_boundary",
		stage:        daemonAppendSessionMutationBlockedOrErrorStageNextBoundaryChosen,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonAppendSessionMutationBlockedOrErrorDispositionAuditNextBoundary,
		evidence:     "append callers now only execute legacy append fallback when no runtime engine is attached; attached SessionMutator not-attached, blocked and hard-error egress no longer writes through legacy append",
		nextBoundary: daemonAppendSessionMutationNextFallbackRetirementBoundaryOptionalResult,
		nextStep:     "P0.5-N236 should decide whether request_authorization/capability_blocked optional-result callers need error-capable egress to avoid fallback after attached mutator errors",
	},
}

var daemonOptionalSessionMutationFallbackRetirementTable = [...]daemonOptionalSessionMutationFallbackRetirementEntry{
	{
		component:    "request_authorization_no_engine_legacy_fallback",
		stage:        daemonOptionalSessionMutationFallbackStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationRequestAuthorization,
		disposition:  daemonOptionalSessionMutationFallbackDispositionLegacyFallback,
		evidence:     "no-engine request_authorization fallback remains scoped to the documented CYBERAGENT_RUNTIME legacy opt-out path",
		nextBoundary: daemonOptionalSessionMutationNextFallbackRetirementBoundaryNoEngineFallback,
		nextStep:     "do not retire no-engine request_authorization fallback until the explicit legacy runtime opt-out switch is removed",
	},
	{
		component:    "capability_blocked_no_engine_legacy_fallback",
		stage:        daemonOptionalSessionMutationFallbackStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationCapabilityBlocked,
		disposition:  daemonOptionalSessionMutationFallbackDispositionLegacyFallback,
		evidence:     "no-engine capability_blocked fallback remains scoped to the documented CYBERAGENT_RUNTIME legacy opt-out path",
		nextBoundary: daemonOptionalSessionMutationNextFallbackRetirementBoundaryNoEngineFallback,
		nextStep:     "do not retire no-engine capability_blocked fallback until the explicit legacy runtime opt-out switch is removed",
	},
	{
		component:    "request_authorization_attached_failure",
		stage:        daemonOptionalSessionMutationFallbackStageRequestAuthorizationSuppressed,
		mutationKind: runtime.SessionMutationRequestAuthorization,
		disposition:  daemonOptionalSessionMutationFallbackDispositionBlockedError,
		evidence:     "request_authorization optional-result callers now treat attached SessionMutator blocked, hard-error or invalid-payload egress as handled, preventing legacy authorization state writes after runtime mutation failure",
		nextBoundary: daemonOptionalSessionMutationNextFallbackRetirementBoundaryNoEngineFallback,
		nextStep:     "add an explicit error-capable request_authorization egress if UI-facing diagnostics are required",
	},
	{
		component:    "capability_blocked_attached_failure",
		stage:        daemonOptionalSessionMutationFallbackStageCapabilityBlockedSuppressed,
		mutationKind: runtime.SessionMutationCapabilityBlocked,
		disposition:  daemonOptionalSessionMutationFallbackDispositionBlockedError,
		evidence:     "capability_blocked optional-result callers now treat attached SessionMutator blocked, hard-error or invalid-payload egress as handled, preventing legacy blocked-state writes after runtime mutation failure",
		nextBoundary: daemonOptionalSessionMutationNextFallbackRetirementBoundaryNoEngineFallback,
		nextStep:     "add an explicit error-capable capability_blocked egress if UI-facing diagnostics are required",
	},
	{
		component:    "session_mutation_next_fallback_boundary",
		stage:        daemonOptionalSessionMutationFallbackStageNextBoundaryChosen,
		disposition:  daemonOptionalSessionMutationFallbackDispositionAuditNextBoundary,
		evidence:     "append and optional-result callers no longer legacy-write after attached SessionMutator not-attached, blocked, hard-error or invalid-payload egress; remaining fallback is the no-engine legacy runtime opt-out path",
		nextBoundary: daemonOptionalSessionMutationNextFallbackRetirementBoundaryNoEngineFallback,
		nextStep:     "P0.5-N237 should audit whether the remaining no-engine SessionMutation legacy fallback can be retired or must stay bound to CYBERAGENT_RUNTIME",
	},
}

var daemonNoEngineSessionMutationFallbackRetirementTable = [...]daemonNoEngineSessionMutationFallbackRetirementEntry{
	{
		component:    "production_default_runtime_engine",
		stage:        daemonNoEngineSessionMutationFallbackStageProductionDefaultEngineAttached,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionRuntimeDefault,
		evidence:     "NewServer defaults CYBERAGENT_RUNTIME to engine mode and attaches runtime.Engine before calling agentManager.AttachEngine, which wires daemonSessionMutator",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "keep production defaults on runtime engine; do not treat no-engine fallback as a production default path",
	},
	{
		component:    "legacy_runtime_opt_out_switch",
		stage:        daemonNoEngineSessionMutationFallbackStageLegacyRuntimeOptOutRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionRollbackSwitch,
		evidence:     "CYBERAGENT_RUNTIME=legacy/off/disabled/false/0 remains the documented whole-engine rollback switch and intentionally leaves agentManager without a runtime.Engine",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "do not retire no-engine SessionMutation fallback while CYBERAGENT_RUNTIME remains the whole-engine rollback switch",
	},
	{
		component:    "append_messages_no_engine_legacy_fallback",
		stage:        daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationAppendMessages,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionLegacyFallback,
		evidence:     "append_messages legacy fallback is retained only when no runtime engine is attached; attached mutator missing, blocked and hard-error egress no longer legacy-append",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "keep append_messages fallback bound to the explicit legacy runtime opt-out path",
	},
	{
		component:    "record_user_message_no_engine_legacy_fallback",
		stage:        daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationRecordUserMessage,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionLegacyFallback,
		evidence:     "record_user_message strict-result caller falls back to recordUserMessageLegacy only when no runtime engine is attached",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "keep record_user_message fallback bound to the explicit legacy runtime opt-out path",
	},
	{
		component:    "request_authorization_no_engine_legacy_fallback",
		stage:        daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationRequestAuthorization,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionLegacyFallback,
		evidence:     "request_authorization optional-result legacy fallback is retained only for no-engine rollback; attached failure egress is handled and blocks legacy authorization writes",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "keep request_authorization fallback bound to the explicit legacy runtime opt-out path",
	},
	{
		component:    "capability_blocked_no_engine_legacy_fallback",
		stage:        daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationCapabilityBlocked,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionLegacyFallback,
		evidence:     "capability_blocked optional-result legacy fallback is retained only for no-engine rollback; attached failure egress is handled and blocks legacy blocked-state writes",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "keep capability_blocked fallback bound to the explicit legacy runtime opt-out path",
	},
	{
		component:    "control_session_no_engine_legacy_fallback",
		stage:        daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationControlSession,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionLegacyFallback,
		evidence:     "control_session strict-result caller falls back to controlSessionLegacy only when no runtime engine is attached",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "keep control_session fallback bound to the explicit legacy runtime opt-out path",
	},
	{
		component:    "resolve_authorization_no_engine_legacy_fallback",
		stage:        daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		reason:       daemonSessionMutationCallerFallbackEngineNotAttached,
		mutationKind: runtime.SessionMutationResolveAuthorization,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionLegacyFallback,
		evidence:     "resolve_authorization strict-result caller falls back to resolveAuthorizationLegacy only when no runtime engine is attached",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "keep resolve_authorization fallback bound to the explicit legacy runtime opt-out path",
	},
	{
		component:    "session_mutation_no_engine_next_boundary",
		stage:        daemonNoEngineSessionMutationFallbackStageNextBoundaryChosen,
		disposition:  daemonNoEngineSessionMutationFallbackDispositionManualGateNext,
		evidence:     "all SessionMutation caller families now retain legacy fallback only for no-engine rollback; retiring that fallback would remove the documented CYBERAGENT_RUNTIME whole-engine rollback path",
		nextBoundary: daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization,
		nextStep:     "return to the existing manual stage/commit/tag authorization gate before changing release state or deprecated runtime API defaults",
	},
}

func daemonSessionMutationCallerInvalidPayloadError(kind runtime.SessionMutationKind) string {
	for _, entry := range daemonSessionMutationCallerInvalidPayloadTable {
		if entry.kind == kind {
			return entry.message
		}
	}
	return daemonSessionMutationCallerDefaultInvalidPayloadError
}

func daemonSessionMutationCallerPolicyForKind(kind runtime.SessionMutationKind) (daemonSessionMutationCallerPolicy, bool) {
	for _, entry := range daemonSessionMutationCallerPolicyTable {
		if entry.kind == kind {
			return entry.policy, true
		}
	}
	return "", false
}

func daemonSessionMutationCallerPolicyMatches(kind runtime.SessionMutationKind, want daemonSessionMutationCallerPolicy) bool {
	policy, ok := daemonSessionMutationCallerPolicyForKind(kind)
	return !ok || policy == want
}

func daemonSessionMutationCallerPolicyMismatchError(kind runtime.SessionMutationKind, want daemonSessionMutationCallerPolicy) error {
	return fmt.Errorf("daemon: session mutation caller policy mismatch for %s: expected %s", kind, want)
}

func daemonSessionMutationCallerFallbackOwnerForReason(reason daemonSessionMutationCallerFallbackReason) (string, bool) {
	for _, entry := range daemonSessionMutationCallerFallbackOwnershipTable {
		if entry.reason == reason {
			return entry.owner, true
		}
	}
	return "", false
}

func daemonSessionMutationCallerFallbackOwnedByLegacyCaller(reason daemonSessionMutationCallerFallbackReason) bool {
	owner, ok := daemonSessionMutationCallerFallbackOwnerForReason(reason)
	return ok && owner == daemonSessionMutationCallerFallbackOwnerLegacyCaller
}

func daemonSessionMutationCallerFallbackOwnershipError(reason daemonSessionMutationCallerFallbackReason) error {
	return fmt.Errorf("daemon: session mutation fallback ownership missing for %s", reason)
}

func daemonSharedCycleSessionMutationFallbackEntryForComponent(component string) (daemonSharedCycleSessionMutationFallbackEntry, bool) {
	for _, entry := range daemonSharedCycleSessionMutationFallbackTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonSharedCycleSessionMutationFallbackEntry{}, false
}

func daemonSharedCycleSessionMutationFallbackEntriesForStage(stage daemonSharedCycleSessionMutationFallbackStage) []daemonSharedCycleSessionMutationFallbackEntry {
	var entries []daemonSharedCycleSessionMutationFallbackEntry
	for _, entry := range daemonSharedCycleSessionMutationFallbackTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonSharedCycleSessionMutationFallbackSelectedBoundary() (string, bool) {
	entries := daemonSharedCycleSessionMutationFallbackEntriesForStage(daemonSharedCycleSessionMutationFallbackStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonSessionMutatorNotAttachedRetirementEntryForComponent(component string) (daemonSessionMutatorNotAttachedRetirementEntry, bool) {
	for _, entry := range daemonSessionMutatorNotAttachedRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonSessionMutatorNotAttachedRetirementEntry{}, false
}

func daemonSessionMutatorNotAttachedRetirementEntriesForStage(stage daemonSessionMutatorNotAttachedRetirementStage) []daemonSessionMutatorNotAttachedRetirementEntry {
	var entries []daemonSessionMutatorNotAttachedRetirementEntry
	for _, entry := range daemonSessionMutatorNotAttachedRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonSessionMutatorNotAttachedRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonSessionMutatorNotAttachedRetirementEntriesForStage(daemonSessionMutatorNotAttachedStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonSessionMutatorNotAttachedRetirementAllowsLegacyFallback(reason daemonSessionMutationCallerFallbackReason) bool {
	for _, entry := range daemonSessionMutatorNotAttachedRetirementTable {
		if entry.reason == reason &&
			entry.disposition == daemonSessionMutatorNotAttachedDispositionLegacyFallback {
			return true
		}
	}
	return false
}

func daemonAppendSessionMutationBlockedOrErrorRetirementEntryForComponent(component string) (daemonAppendSessionMutationBlockedOrErrorRetirementEntry, bool) {
	for _, entry := range daemonAppendSessionMutationBlockedOrErrorRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonAppendSessionMutationBlockedOrErrorRetirementEntry{}, false
}

func daemonAppendSessionMutationBlockedOrErrorRetirementEntriesForStage(stage daemonAppendSessionMutationBlockedOrErrorRetirementStage) []daemonAppendSessionMutationBlockedOrErrorRetirementEntry {
	var entries []daemonAppendSessionMutationBlockedOrErrorRetirementEntry
	for _, entry := range daemonAppendSessionMutationBlockedOrErrorRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonAppendSessionMutationBlockedOrErrorRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonAppendSessionMutationBlockedOrErrorRetirementEntriesForStage(daemonAppendSessionMutationBlockedOrErrorStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonAppendSessionMutationBlockedOrErrorRetirementAllowsLegacyFallback(reason daemonSessionMutationCallerFallbackReason) bool {
	for _, entry := range daemonAppendSessionMutationBlockedOrErrorRetirementTable {
		if entry.reason == reason &&
			entry.disposition == daemonAppendSessionMutationBlockedOrErrorDispositionLegacyFallback {
			return true
		}
	}
	return false
}

func daemonOptionalSessionMutationFallbackRetirementEntryForComponent(component string) (daemonOptionalSessionMutationFallbackRetirementEntry, bool) {
	for _, entry := range daemonOptionalSessionMutationFallbackRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonOptionalSessionMutationFallbackRetirementEntry{}, false
}

func daemonOptionalSessionMutationFallbackRetirementEntriesForStage(stage daemonOptionalSessionMutationFallbackRetirementStage) []daemonOptionalSessionMutationFallbackRetirementEntry {
	var entries []daemonOptionalSessionMutationFallbackRetirementEntry
	for _, entry := range daemonOptionalSessionMutationFallbackRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonOptionalSessionMutationFallbackRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonOptionalSessionMutationFallbackRetirementEntriesForStage(daemonOptionalSessionMutationFallbackStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonOptionalSessionMutationFallbackRetirementAllowsLegacyFallback(kind runtime.SessionMutationKind, reason daemonSessionMutationCallerFallbackReason) bool {
	for _, entry := range daemonOptionalSessionMutationFallbackRetirementTable {
		if entry.mutationKind == kind &&
			entry.reason == reason &&
			entry.disposition == daemonOptionalSessionMutationFallbackDispositionLegacyFallback {
			return true
		}
	}
	return false
}

func daemonOptionalSessionMutationFallbackRetirementBlocksLegacyFallback(kind runtime.SessionMutationKind) bool {
	for _, entry := range daemonOptionalSessionMutationFallbackRetirementTable {
		if entry.mutationKind == kind &&
			entry.disposition == daemonOptionalSessionMutationFallbackDispositionBlockedError {
			return true
		}
	}
	return false
}

func daemonNoEngineSessionMutationFallbackRetirementEntryForComponent(component string) (daemonNoEngineSessionMutationFallbackRetirementEntry, bool) {
	for _, entry := range daemonNoEngineSessionMutationFallbackRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonNoEngineSessionMutationFallbackRetirementEntry{}, false
}

func daemonNoEngineSessionMutationFallbackRetirementEntriesForStage(stage daemonNoEngineSessionMutationFallbackRetirementStage) []daemonNoEngineSessionMutationFallbackRetirementEntry {
	var entries []daemonNoEngineSessionMutationFallbackRetirementEntry
	for _, entry := range daemonNoEngineSessionMutationFallbackRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonNoEngineSessionMutationFallbackRetirementSelectedBoundary() (string, bool) {
	entries := daemonNoEngineSessionMutationFallbackRetirementEntriesForStage(daemonNoEngineSessionMutationFallbackStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonNoEngineSessionMutationFallbackRetirementRetainsKind(kind runtime.SessionMutationKind) bool {
	for _, entry := range daemonNoEngineSessionMutationFallbackRetirementTable {
		if entry.mutationKind == kind &&
			entry.reason == daemonSessionMutationCallerFallbackEngineNotAttached &&
			entry.disposition == daemonNoEngineSessionMutationFallbackDispositionLegacyFallback {
			return true
		}
	}
	return false
}

func daemonSessionMutationCallerPreDispatchFallbackReason(eng *runtime.Engine) (daemonSessionMutationCallerFallbackReason, bool) {
	if eng == nil {
		return daemonSessionMutationCallerFallbackEngineNotAttached, true
	}
	if eng.SessionMutator() == nil {
		return daemonSessionMutationCallerFallbackMutatorNotAttached, true
	}
	return "", false
}

func daemonSessionMutationCallerResultFallbackReason(result runtime.SessionMutationResult, resultErr error) (daemonSessionMutationCallerFallbackReason, bool) {
	if errors.Is(resultErr, runtime.ErrSessionMutatorNotAttached) ||
		errors.Is(result.Error, runtime.ErrSessionMutatorNotAttached) {
		return daemonSessionMutationCallerFallbackMutatorNotAttached, true
	}
	if result.Status == runtime.TurnResultBlocked {
		return daemonSessionMutationCallerFallbackBlocked, true
	}
	return "", false
}

func daemonSessionMutationCallerMutatorNotAttachedBlockedResult(request runtime.SessionMutationRequest, result runtime.SessionMutationResult, resultErr error) (runtime.SessionMutationResult, bool) {
	blockedErr := error(nil)
	if errors.Is(resultErr, runtime.ErrSessionMutatorNotAttached) {
		blockedErr = resultErr
	} else if errors.Is(result.Error, runtime.ErrSessionMutatorNotAttached) {
		blockedErr = result.Error
	}
	if blockedErr == nil {
		return runtime.SessionMutationResult{}, false
	}
	blocked := result
	blocked.Status = runtime.TurnResultBlocked
	if blocked.Reason == "" {
		blocked.Reason = blockedErr.Error()
	}
	blocked.Error = blockedErr
	data := runtime.CloneAgentLoopMapStringAny(blocked.Data)
	if data == nil {
		data = map[string]any{}
	}
	if request.Kind != "" {
		data["kind"] = string(request.Kind)
	}
	if request.Turn.SessionID != "" {
		data["session_id"] = request.Turn.SessionID
	}
	if request.Turn.TaskID != "" {
		data["task_id"] = request.Turn.TaskID
	}
	if request.Turn.RunID != "" {
		data["run_id"] = request.Turn.RunID
	}
	if request.Turn.TurnID != "" {
		data["turn_id"] = request.Turn.TurnID
	}
	data["error"] = blockedErr.Error()
	data["blocked_reason"] = string(daemonSessionMutationCallerFallbackMutatorNotAttached)
	blocked.Data = data
	return blocked, true
}

func daemonSessionMutationCallerResultPayload[T any](
	egress daemonSessionMutationCallerEgress,
	extract daemonSessionMutationCallerPayloadExtractor[T],
	invalidPayloadError string,
) (T, bool, error) {
	var zero T
	if !egress.handled {
		return zero, false, nil
	}
	if egress.resultErr != nil {
		return zero, true, egress.resultErr
	}
	if extract == nil {
		return zero, true, errors.New(invalidPayloadError)
	}
	payload, ok := extract(egress.result)
	if !ok {
		return zero, true, errors.New(invalidPayloadError)
	}
	return payload, true, nil
}

func daemonSessionMutationCallerResultPayloadForKind[T any](
	egress daemonSessionMutationCallerEgress,
	kind runtime.SessionMutationKind,
	extract daemonSessionMutationCallerPayloadExtractor[T],
) (T, bool, error) {
	if !daemonSessionMutationCallerPolicyMatches(kind, daemonSessionMutationCallerPolicyStrictResult) {
		var zero T
		return zero, true, daemonSessionMutationCallerPolicyMismatchError(kind, daemonSessionMutationCallerPolicyStrictResult)
	}
	return daemonSessionMutationCallerResultPayload(egress, extract, daemonSessionMutationCallerInvalidPayloadError(kind))
}

func daemonSessionMutationCallerOptionalResultPayload[T any](
	egress daemonSessionMutationCallerEgress,
	extract daemonSessionMutationCallerPayloadExtractor[T],
	invalidPayloadError string,
) (T, bool) {
	payload, handled, err := daemonSessionMutationCallerResultPayload(egress, extract, invalidPayloadError)
	if !handled || err != nil {
		var zero T
		return zero, false
	}
	return payload, true
}

func daemonSessionMutationCallerOptionalResultPayloadForKind[T any](
	egress daemonSessionMutationCallerEgress,
	kind runtime.SessionMutationKind,
	extract daemonSessionMutationCallerPayloadExtractor[T],
) (T, bool) {
	var zero T
	if !daemonSessionMutationCallerPolicyMatches(kind, daemonSessionMutationCallerPolicyOptionalResult) {
		return zero, false
	}
	payload, handled, err := daemonSessionMutationCallerResultPayload(egress, extract, daemonSessionMutationCallerInvalidPayloadError(kind))
	if handled {
		if err != nil {
			return zero, daemonOptionalSessionMutationFallbackRetirementBlocksLegacyFallback(kind)
		}
		return payload, true
	}
	if reason, fallback := daemonSessionMutationCallerResultFallbackReason(egress.result, egress.resultErr); fallback &&
		!daemonOptionalSessionMutationFallbackRetirementAllowsLegacyFallback(kind, reason) {
		return zero, daemonOptionalSessionMutationFallbackRetirementBlocksLegacyFallback(kind)
	}
	return zero, false
}

func daemonSessionMutationCallerHandled(egress daemonSessionMutationCallerEgress) bool {
	return egress.handled && egress.resultErr == nil
}

func daemonSessionMutationCallerHandledForKind(egress daemonSessionMutationCallerEgress, kind runtime.SessionMutationKind) bool {
	if !daemonSessionMutationCallerPolicyMatches(kind, daemonSessionMutationCallerPolicyBoolOnly) {
		return false
	}
	if egress.handled {
		return true
	}
	reason, fallback := daemonSessionMutationCallerResultFallbackReason(egress.result, egress.resultErr)
	return fallback && !daemonAppendSessionMutationBlockedOrErrorRetirementAllowsLegacyFallback(reason)
}

func (m *agentManager) runRuntimeSessionMutation(ctx context.Context, request runtime.SessionMutationRequest) (runtime.SessionMutationResult, bool, error) {
	eng := m.runtimeEngine()
	if reason, ok := daemonSessionMutationCallerPreDispatchFallbackReason(eng); ok {
		if reason == daemonSessionMutationCallerFallbackMutatorNotAttached &&
			!daemonSessionMutatorNotAttachedRetirementAllowsLegacyFallback(reason) {
			blocked, _ := daemonSessionMutationCallerMutatorNotAttachedBlockedResult(request, runtime.SessionMutationResult{}, runtime.ErrSessionMutatorNotAttached)
			return blocked, true, runtime.ErrSessionMutatorNotAttached
		}
		if !daemonSessionMutationCallerFallbackOwnedByLegacyCaller(reason) {
			return runtime.SessionMutationResult{}, true, daemonSessionMutationCallerFallbackOwnershipError(reason)
		}
		return runtime.SessionMutationResult{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := eng.MutateSession(ctx, request)
	if blocked, ok := daemonSessionMutationCallerMutatorNotAttachedBlockedResult(request, result, err); ok {
		return blocked, true, blocked.Error
	}
	if reason, ok := daemonSessionMutationCallerResultFallbackReason(result, err); ok {
		if reason == daemonSessionMutationCallerFallbackMutatorNotAttached &&
			!daemonSessionMutatorNotAttachedRetirementAllowsLegacyFallback(reason) {
			if blocked, blockedOK := daemonSessionMutationCallerMutatorNotAttachedBlockedResult(request, result, err); blockedOK {
				return blocked, true, blocked.Error
			}
			return result, true, daemonSessionMutationCallerFallbackOwnershipError(reason)
		}
		if !daemonSessionMutationCallerFallbackOwnedByLegacyCaller(reason) {
			return result, true, daemonSessionMutationCallerFallbackOwnershipError(reason)
		}
		return result, false, nil
	}
	if err != nil {
		return result, true, err
	}
	return result, true, nil
}

func (m *agentManager) runRuntimeSessionMutationCallerEgress(ctx context.Context, request runtime.SessionMutationRequest) daemonSessionMutationCallerEgress {
	result, handled, err := m.runRuntimeSessionMutation(ctx, request)
	return daemonSessionMutationCallerEgress{
		result:    result,
		handled:   handled,
		resultErr: err,
	}
}

func (m *agentManager) runRuntimeAppendMessagesMutation(ctx context.Context, sessionID string, plan *agentPlannerResponse, messages ...agentMessage) bool {
	return m.runRuntimeAppendMessagesMutationWithTurn(ctx, runtime.AgentLoopAppendMessagesMutationTurn(
		sessionID,
		"",
		"",
		"",
		"",
		"",
		"",
		len(messages),
		plan != nil,
	), plan, messages...)
}

func (m *agentManager) runRuntimeAppendMessagesMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, origin, operationLevel string, plan *agentPlannerResponse, messages ...agentMessage) bool {
	return m.runRuntimeAppendMessagesMutationWithTurn(ctx, runtime.AgentLoopAppendMessagesMutationTurn(
		sessionID,
		turnRef.TaskID,
		turnRef.RunID,
		turnRef.TurnID,
		prompt,
		origin,
		operationLevel,
		len(messages),
		plan != nil,
	), plan, messages...)
}

func (m *agentManager) runRuntimeAppendMessagesMutationWithTurn(ctx context.Context, turn runtime.TurnContext, plan *agentPlannerResponse, messages ...agentMessage) bool {
	operationLevel := turn.OperationLevel
	turn.Metadata = runtime.MergeAgentLoopMetadata(
		turn.Metadata,
		runtime.AgentLoopAppendMessagesMutationMetadata(len(messages), plan != nil, operationLevel),
	)
	requestMetadata := runtime.AgentLoopAppendMessagesMutationMetadata(len(messages), plan != nil, "")
	request := runtime.AgentLoopAppendMessagesMutationRequest(
		turn,
		agentLoopPlanPtrFromAgentPlannerResponse(plan),
		agentLoopMessagesFromAgentMessages(messages),
		requestMetadata,
	)
	return daemonSessionMutationCallerHandledForKind(m.runRuntimeSessionMutationCallerEgress(ctx, request), request.Kind)
}

func (m *agentManager) runRuntimeRecordUserMessageMutation(ctx context.Context, sessionID, prompt, operator, language, operationLevel string) (daemonRecordUserMessageMutationResultPayload, bool, error) {
	request := runtime.AgentLoopRecordUserMessageMutationRequest(
		sessionID,
		prompt,
		operationLevel,
		operator,
		language,
		runtime.AgentLoopRecordUserMessageMutationMetadata(operationLevel),
	)
	return daemonSessionMutationCallerResultPayloadForKind(
		m.runRuntimeSessionMutationCallerEgress(ctx, request),
		request.Kind,
		daemonRecordUserMessageMutationResultPayloadFromSessionMutationResult,
	)
}

func (m *agentManager) runRuntimeRequestAuthorizationMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, operatorPrompt, language, operationLevel string, action runtime.AgentLoopAction, actionName, requiredLevel, policy, reason string) (daemonRequestAuthorizationMutationResultPayload, bool) {
	action = cloneAgentLoopAction(action)
	actionMetadataName := firstNonEmpty(normalizeAgentPlannerActionName(actionName), strings.TrimSpace(action.Name))
	requestMetadata := runtime.AgentLoopAuthorizationRequestMutationMetadata(
		actionMetadataName,
		normalizeAgentOperationLevel(firstNonEmpty(requiredLevel, "read_only")),
		strings.TrimSpace(policy),
		operationLevel,
	)
	request := runtime.AgentLoopAuthorizationRequestMutationRequest(
		sessionID,
		turnRef.TaskID,
		turnRef.RunID,
		turnRef.TurnID,
		operatorPrompt,
		language,
		operationLevel,
		action,
		actionName,
		requiredLevel,
		policy,
		reason,
		requestMetadata,
	)
	return daemonSessionMutationCallerOptionalResultPayloadForKind(
		m.runRuntimeSessionMutationCallerEgress(ctx, request),
		request.Kind,
		daemonRequestAuthorizationMutationResultPayloadFromSessionMutationResult,
	)
}

func (m *agentManager) runRuntimeCapabilityBlockedMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, operatorPrompt, language, operationLevel, actionName, intentID, reason, suggestedPrompt string) (daemonCapabilityBlockedMutationResultPayload, bool) {
	actionName = firstNonEmpty(normalizeAgentPlannerActionName(actionName), strings.TrimSpace(actionName))
	intentID = strings.TrimSpace(intentID)
	requestMetadata := runtime.AgentLoopCapabilityBlockedMutationMetadata(actionName, intentID, strings.TrimSpace(suggestedPrompt), operationLevel)
	request := runtime.AgentLoopCapabilityBlockedMutationRequest(
		sessionID,
		turnRef.TaskID,
		turnRef.RunID,
		turnRef.TurnID,
		operatorPrompt,
		language,
		operationLevel,
		actionName,
		intentID,
		reason,
		suggestedPrompt,
		requestMetadata,
	)
	return daemonSessionMutationCallerOptionalResultPayloadForKind(
		m.runRuntimeSessionMutationCallerEgress(ctx, request),
		request.Kind,
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
	)
}

func (m *agentManager) runRuntimeControlSessionMutation(ctx context.Context, sessionID, action string, req agentControlRequest, operator, language, operationLevel string) (daemonControlSessionMutationResultPayload, bool, error) {
	turnRef := m.controlSessionMutationTurnRef(ctx, sessionID, action, req)
	return m.runRuntimeControlSessionMutationForTurn(ctx, sessionID, turnRef, action, req, operator, language, operationLevel)
}

func (m *agentManager) controlSessionMutationTurnRef(ctx context.Context, sessionID, action string, req agentControlRequest) agentRuntimeTurnRef {
	return m.externalSessionMutationTurnRef(ctx, sessionID, firstNonEmpty(strings.TrimSpace(req.JobID), strings.TrimSpace(req.RunID), strings.TrimSpace(action)))
}

func (m *agentManager) runRuntimeControlSessionMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, action string, req agentControlRequest, operator, language, operationLevel string) (daemonControlSessionMutationResultPayload, bool, error) {
	requestMetadata := runtime.AgentLoopControlSessionMutationMetadata(action, operationLevel, req.ClearInput, strings.TrimSpace(req.RunID), strings.TrimSpace(req.JobID))
	request := runtime.AgentLoopControlSessionMutationRequest(
		sessionID,
		turnRef.TaskID,
		turnRef.RunID,
		turnRef.TurnID,
		req.Prompt,
		operationLevel,
		action,
		req.Reason,
		req.RunID,
		req.JobID,
		req.ClearInput,
		operator,
		language,
		requestMetadata,
	)
	return daemonSessionMutationCallerResultPayloadForKind(
		m.runRuntimeSessionMutationCallerEgress(ctx, request),
		request.Kind,
		daemonControlSessionMutationResultPayloadFromSessionMutationResult,
	)
}

func (m *agentManager) runRuntimeResolveAuthorizationMutation(ctx context.Context, sessionID, decision string, req agentAuthorizationDecisionRequest, operator, language, operationLevel string) (daemonResolveAuthorizationMutationResultPayload, bool, error) {
	turnRef := m.resolveAuthorizationMutationTurnRef(ctx, sessionID, decision, req)
	return m.runRuntimeResolveAuthorizationMutationForTurn(ctx, sessionID, turnRef, decision, req, operator, language, operationLevel)
}

func (m *agentManager) resolveAuthorizationMutationTurnRef(ctx context.Context, sessionID, decision string, req agentAuthorizationDecisionRequest) agentRuntimeTurnRef {
	return m.externalSessionMutationTurnRef(ctx, sessionID, firstNonEmpty(strings.TrimSpace(req.AuthorizationID), strings.TrimSpace(decision)))
}

func (m *agentManager) runRuntimeResolveAuthorizationMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, decision string, req agentAuthorizationDecisionRequest, operator, language, operationLevel string) (daemonResolveAuthorizationMutationResultPayload, bool, error) {
	operationLevelProvided := strings.TrimSpace(req.OperationLevel) != ""
	authorizationID := strings.TrimSpace(req.AuthorizationID)
	reason := strings.TrimSpace(req.Reason)
	requestMetadata := runtime.AgentLoopResolveAuthorizationMutationMetadata(authorizationID, decision, operationLevel, operationLevelProvided)
	request := runtime.AgentLoopResolveAuthorizationMutationRequest(
		sessionID,
		turnRef.TaskID,
		turnRef.RunID,
		turnRef.TurnID,
		operationLevel,
		decision,
		authorizationID,
		reason,
		operator,
		language,
		operationLevelProvided,
		requestMetadata,
	)
	return daemonSessionMutationCallerResultPayloadForKind(
		m.runRuntimeSessionMutationCallerEgress(ctx, request),
		request.Kind,
		daemonResolveAuthorizationMutationResultPayloadFromSessionMutationResult,
	)
}

func (m *agentManager) recordUserMessageLegacy(sessionID, prompt, operator, language, operationLevel string) (daemonRecordUserMessageMutationResultPayload, error) {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return daemonRecordUserMessageMutationResultPayload{}, loadErr
		}
		session = loaded
	}
	session.Operator = operator
	session.Language = language
	session.OperationLevel = operationLevel
	userMessage := m.newMessageLocked(language, "user", prompt, nil, nil)
	session.Messages = append(session.Messages, userMessage)
	session.UpdatedAt = userMessage.Timestamp
	if m.shouldStartNewAgentTaskLocked(session, prompt, language) {
		m.startAgentTaskLocked(session, prompt, language)
	} else {
		m.touchAgentTaskForMessageLocked(session, prompt, language)
	}
	compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	history := cloneAgentMessages(session.Messages)
	memorySummary := cloneAgentMemorySummary(session.MemorySummary)
	_ = m.persistSessionLocked(session)
	sessionSnapshot := cloneAgentSession(session)
	m.mu.Unlock()
	return daemonRecordUserMessageMutationResultPayload{
		Session:       sessionSnapshot,
		UserMessage:   userMessage,
		History:       history,
		MemorySummary: memorySummary,
		Compaction:    compaction,
		Mutated:       true,
	}, nil
}

func (m *agentManager) controlSessionLegacy(ctx context.Context, sessionID, action, prompt, reason, runID, jobID string, clearInput bool, operator, language, operationLevel string) (daemonControlSessionMutationResultPayload, error) {
	return m.controlSessionLegacyForTurn(ctx, agentRuntimeTurnRef{}, sessionID, action, prompt, reason, runID, jobID, clearInput, operator, language, operationLevel)
}

func (m *agentManager) controlSessionLegacyForTurn(ctx context.Context, turnRef agentRuntimeTurnRef, sessionID, action, prompt, reason, runID, jobID string, clearInput bool, operator, language, operationLevel string) (daemonControlSessionMutationResultPayload, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	action = normalizeAgentControlAction(action)
	if action == "" {
		return daemonControlSessionMutationResultPayload{}, services.NewError(services.CodeBadRequest, "unsupported agent control action", map[string]any{"action": action}, nil)
	}
	now := time.Now().UTC()

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return daemonControlSessionMutationResultPayload{}, loadErr
		}
		session = loaded
	}
	session.Operator = operator
	session.Language = language
	session.OperationLevel = operationLevel
	previousNextAction := cloneAgentNextAction(session.NextAction)

	if session.PendingAuthorization != nil &&
		strings.EqualFold(session.PendingAuthorization.Status, "pending") &&
		(action == "continue" || action == "revise") {
		session.Status = "waiting_authorization"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "authorization_required",
			Reason:          session.PendingAuthorization.Reason,
			SuggestedPrompt: agentAuthorizationSuggestedPrompt(language, session.PendingAuthorization),
			AutoContinue:    false,
		}
		message := agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, "当前有挂起的工具授权，请先点击允许或拒绝；在此之前不会继续执行任务。", "A tool authorization is pending. Approve or deny it first; the task will not continue before that."), nil, nil), "step")
		session.Messages = append(session.Messages, message)
		session.UpdatedAt = now
		compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
		if err := m.persistSessionLocked(session); err != nil {
			m.mu.Unlock()
			return daemonControlSessionMutationResultPayload{}, err
		}
		sessionState := cloneAgentSession(session)
		m.mu.Unlock()
		turnRef = m.ensureExternalSessionMutationTurnRef(ctx, sessionID, turnRef, message.ID)
		m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, turnRef, compaction)
		return daemonControlSessionMutationResultPayload{
			Session:    sessionState,
			Message:    message,
			Compaction: compaction,
			Mutated:    true,
		}, nil
	}

	var message agentMessage
	var cancelJobID string
	switch action {
	case "continue":
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		m.ensureRunningTodoLocked(session, now)
		previousAutoContinue := previousNextAction != nil &&
			previousNextAction.Type == "continue" &&
			previousNextAction.AutoContinue &&
			strings.TrimSpace(prompt) == "" &&
			strings.TrimSpace(previousNextAction.SuggestedPrompt) != ""
		previousSuggestedPrompt := ""
		if previousAutoContinue {
			previousSuggestedPrompt = previousNextAction.SuggestedPrompt
		}
		suggestedPrompt := firstNonEmpty(strings.TrimSpace(prompt), strings.TrimSpace(previousSuggestedPrompt), agentSuggestedContinuePrompt(language, session))
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "用户请求继续当前任务。", "The operator requested continuing the current task."),
			SuggestedPrompt: suggestedPrompt,
			AutoContinue:    previousAutoContinue,
		}
		if agentEvidenceActionHasApprovalGate(suggestedPrompt) {
			session.NextAction = &agentNextAction{
				Type:            "approval_required",
				Reason:          agentText(language, "当前继续动作仍包含待审批 gate，需先处理审批后再继续。", "The continue action still contains a pending approval gate; resolve approvals before continuing."),
				SuggestedPrompt: suggestedPrompt,
			}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "收到：继续当前任务。我会基于当前计划、工具调用和工作区上下文推进下一步。", "Received: continue the current task. I will proceed based on the current plan, tool calls and workspace context."), nil, nil)
	case "revise":
		revision := strings.TrimSpace(prompt)
		if revision == "" {
			revision = strings.TrimSpace(reason)
		}
		if revision == "" {
			revision = agentText(language, "修订当前任务计划。", "Revise the current task plan.")
		}
		session.Status = "running"
		if session.ActiveTask == nil {
			session.ActiveTask = &agentTask{
				ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
				Goal:      agentText(language, "修订安全分析任务", "Revise security analysis task"),
				Kind:      "planning",
				Status:    "running",
				Summary:   revision,
				CreatedAt: now,
				UpdatedAt: now,
			}
		} else {
			session.ActiveTask.Status = "running"
			session.ActiveTask.Summary = trimSummary(revision, 360)
			session.ActiveTask.UpdatedAt = now
		}
		isEvidenceAction := agentIsEvidenceActionPrompt(revision)
		evidenceActionRequiresApproval := false
		if isEvidenceAction {
			evidenceActionRequiresApproval = agentEvidenceActionHasApprovalGate(revision)
			session.Todos = agentEvidenceActionTodos(language, revision, now)
		} else {
			m.ensureRunningTodoLocked(session, now)
			if len(session.Todos) == 0 {
				session.Todos = agentInitialTodos(language, "chat", now)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "用户要求修订当前任务计划。", "The operator requested revising the current task plan."),
			SuggestedPrompt: revision,
		}
		if evidenceActionRequiresApproval {
			session.NextAction = &agentNextAction{
				Type:            "approval_required",
				Reason:          agentText(language, "当前证据行动计划存在待审批 gate，需先处理审批后再继续。", "The evidence action plan has a pending approval gate; resolve approvals before continuing."),
				SuggestedPrompt: revision,
			}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "收到：我会按你的修订要求更新当前任务计划，并在下一轮继续推进。", "Received: I will update the current task plan according to your revision and continue in the next turn."), nil, nil)
	case "pause":
		session.Status = "paused"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = agentText(language, "用户请求暂停当前 Agent 会话。", "The operator requested pausing the current Agent session.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(prompt), agentSuggestedContinuePrompt(language, session)),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已暂停当前 Agent 会话；不会清空上下文。发送继续/Resume 可从当前计划恢复。", "Paused the current Agent session without clearing context. Send continue/resume to proceed from the current plan."), nil, nil)
	case "cancel":
		session.PendingAuthorization = nil
		session.Status = "cancelled"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		cancelJobID = strings.TrimSpace(jobID)
		if cancelJobID == "" {
			cancelJobID = m.findActiveJobIDLocked(session, strings.TrimSpace(runID))
		}
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = agentText(language, "用户请求取消当前 Agent 任务。", "The operator requested cancelling the current Agent task.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "skipped", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(prompt), agentText(language, "开启新任务并说明授权目标与 scope", "start a new task and provide the authorized target and scope")),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已取消当前 Agent 任务；如存在运行中的 daemon job，会尝试取消。", "Cancelled the current Agent task; if a daemon job is running, I will try to cancel it."), nil, nil)
	case "stop":
		session.PendingAuthorization = nil
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		cancelJobID = strings.TrimSpace(jobID)
		if cancelJobID == "" {
			cancelJobID = m.findActiveJobIDLocked(session, strings.TrimSpace(runID))
		}
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = agentText(language, "用户请求停止当前任务。", "The operator requested stopping the current task.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(prompt), agentSuggestedStopPrompt(language, session)),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已暂停当前 Agent 任务状态；如存在运行中的 daemon job，会尝试取消。", "The current Agent task state is paused; if a daemon job is running, I will try to cancel it."), nil, nil)
	case "new_task":
		session.Status = "idle"
		session.ActiveTask = nil
		session.Todos = []agentTodo{}
		session.ToolCalls = []agentToolCall{}
		session.ContextRefs = []agentContextRef{}
		session.PendingChanges = []agentWorkspaceChange{}
		session.PendingAuthorization = nil
		session.MemorySummary = nil
		session.NextAction = &agentNextAction{
			Type:   "wait_user",
			Reason: agentText(language, "已开启新任务，等待用户输入目标、scope 或下一步指令。", "New task started; waiting for target, scope, or next instruction."),
		}
		if clearInput {
			session.Messages = []agentMessage{}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已开启新的 Agent 任务上下文。", "Started a new Agent task context."), nil, nil)
	default:
		m.mu.Unlock()
		return daemonControlSessionMutationResultPayload{}, services.NewError(services.CodeBadRequest, "unsupported agent control action", map[string]any{"action": action}, nil)
	}
	session.Messages = append(session.Messages, message)
	session.UpdatedAt = now
	compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return daemonControlSessionMutationResultPayload{}, err
	}
	sessionState := cloneAgentSession(session)
	m.mu.Unlock()
	turnRef = m.ensureExternalSessionMutationTurnRef(ctx, sessionID, turnRef, message.ID)
	m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, turnRef, compaction)
	m.emitAgentRuntimeEvents(ctx, m.runtimeEngine(), sessionID, sessionState, message)
	return daemonControlSessionMutationResultPayload{
		Session:     sessionState,
		Message:     message,
		CancelJobID: cancelJobID,
		Compaction:  compaction,
		Mutated:     true,
	}, nil
}

func (m *agentManager) resolveAuthorizationLegacy(ctx context.Context, sessionID, decision, authorizationID, reason, operator, language, operationLevel string, operationLevelProvided bool) (daemonResolveAuthorizationMutationResultPayload, error) {
	return m.resolveAuthorizationLegacyForTurn(ctx, agentRuntimeTurnRef{}, sessionID, decision, authorizationID, reason, operator, language, operationLevel, operationLevelProvided)
}

func (m *agentManager) resolveAuthorizationLegacyForTurn(ctx context.Context, turnRef agentRuntimeTurnRef, sessionID, decision, authorizationID, reason, operator, language, operationLevel string, operationLevelProvided bool) (daemonResolveAuthorizationMutationResultPayload, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	decision = normalizeAgentAuthorizationDecision(decision)
	if decision == "" {
		return daemonResolveAuthorizationMutationResultPayload{}, services.NewError(services.CodeBadRequest, "unsupported authorization decision", map[string]any{"action": decision}, nil)
	}
	now := time.Now().UTC()

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return daemonResolveAuthorizationMutationResultPayload{}, loadErr
		}
		session = loaded
	}
	pending := cloneAgentAuthorizationRequest(session.PendingAuthorization)
	if pending == nil || !strings.EqualFold(pending.Status, "pending") {
		m.mu.Unlock()
		return daemonResolveAuthorizationMutationResultPayload{}, services.NewError(services.CodeConflict, "no pending agent authorization", map[string]any{"session_id": sessionID}, nil)
	}
	if requestedID := strings.TrimSpace(authorizationID); requestedID != "" && requestedID != pending.ID {
		m.mu.Unlock()
		return daemonResolveAuthorizationMutationResultPayload{}, services.NewError(services.CodeConflict, "authorization id does not match pending request", map[string]any{"authorization_id": requestedID, "pending_authorization_id": pending.ID}, nil)
	}
	session.Operator = operator
	session.Language = language
	if !operationLevelProvided {
		operationLevel = normalizeAgentOperationLevel(session.OperationLevel)
	}
	if decision == "approve" && !agentOperationLevelAtLeast(operationLevel, pending.RequiredLevel) {
		operationLevel = normalizeAgentOperationLevel(pending.RequiredLevel)
	}
	session.OperationLevel = operationLevel

	var message agentMessage
	var sessionState agentSession
	var compaction *agentMemoryCompaction
	switch decision {
	case "deny":
		pending.Status = "denied"
		pending.UpdatedAt = now
		session.PendingAuthorization = nil
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", agentText(language, "操作者拒绝授权该工具动作。", "Operator denied authorization for this tool action."))
			}
		}
		reason = firstNonEmpty(strings.TrimSpace(reason), agentText(language, "操作者拒绝授权，挂起动作未执行。", "Operator denied authorization; the pending action was not executed."))
		session.NextAction = &agentNextAction{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "换一个低风险方案或重新说明授权边界。", "Choose a lower-risk alternative or restate the authorization boundary."),
		}
		message = agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, fmt.Sprintf("已拒绝授权：`%s` 未执行。", pending.ActionName), fmt.Sprintf("Authorization denied: `%s` was not executed.", pending.ActionName)), nil, nil), "step")
	case "approve":
		pending.Status = "approved"
		pending.UpdatedAt = now
		session.PendingAuthorization = nil
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "授权已通过，继续执行挂起工具动作。", "Authorization approved; continuing the pending tool action."),
			SuggestedPrompt: pending.OriginalPrompt,
			AutoContinue:    true,
		}
		message = agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, fmt.Sprintf("已授权，继续执行挂起动作：`%s`。", pending.ActionName), fmt.Sprintf("Authorized. Continuing pending action: `%s`.", pending.ActionName)), nil, nil), "step")
	default:
		m.mu.Unlock()
		return daemonResolveAuthorizationMutationResultPayload{}, services.NewError(services.CodeBadRequest, "unsupported authorization decision", map[string]any{"action": decision}, nil)
	}
	session.Messages = append(session.Messages, message)
	session.UpdatedAt = now
	compaction = compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return daemonResolveAuthorizationMutationResultPayload{}, err
	}
	sessionState = cloneAgentSession(session)
	m.mu.Unlock()

	m.emitRuntimeApprovalResolved(ctx, sessionID, sessionState, *pending, pending.Status)
	turnRef = m.ensureExternalSessionMutationTurnRef(ctx, sessionID, turnRef, pending.ID)
	m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, turnRef, compaction)
	m.emitAgentRuntimeEvents(ctx, m.runtimeEngine(), sessionID, sessionState, message)
	return daemonResolveAuthorizationMutationResultPayload{
		Session:        sessionState,
		Pending:        *pending,
		Message:        message,
		OperationLevel: operationLevel,
		Compaction:     compaction,
		Mutated:        true,
	}, nil
}
