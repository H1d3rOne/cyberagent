package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestCloneAgentLoopSessionMutationPayload(t *testing.T) {
	valuePayload := runtime.AgentLoopRecordUserMessagePayload{
		Operator: "operator-1",
		Language: "zh",
	}
	clonedValue, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopRecordUserMessagePayload](valuePayload)
	require.True(t, ok)
	valuePayload.Operator = "mutated"
	require.Equal(t, "operator-1", clonedValue.Operator)
	require.Equal(t, "zh", clonedValue.Language)

	pointerPayload := &runtime.AgentLoopAppendMessagesPayload{
		Messages: []runtime.AgentLoopMessage{{ID: "msg-1", Content: "original"}},
	}
	clonedPointer, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopAppendMessagesPayload](pointerPayload)
	require.True(t, ok)
	pointerPayload.Messages[0].Content = "mutated"
	require.Equal(t, "original", clonedPointer.Messages[0].Content)

	var nilPayload *runtime.AgentLoopControlSessionPayload
	_, ok = cloneAgentLoopSessionMutationPayload[runtime.AgentLoopControlSessionPayload](nilPayload)
	require.False(t, ok)

	_, ok = cloneAgentLoopSessionMutationPayload[runtime.AgentLoopControlSessionPayload]("invalid")
	require.False(t, ok)
}

func TestDaemonSessionMutationDispatchTableCoversKinds(t *testing.T) {
	wantKinds := map[runtime.SessionMutationKind]bool{
		runtime.SessionMutationAppendMessages:       true,
		runtime.SessionMutationRecordUserMessage:    true,
		runtime.SessionMutationRequestAuthorization: true,
		runtime.SessionMutationCapabilityBlocked:    true,
		runtime.SessionMutationControlSession:       true,
		runtime.SessionMutationResolveAuthorization: true,
	}
	seenKinds := map[runtime.SessionMutationKind]bool{}

	for _, entry := range daemonSessionMutationDispatchTable {
		require.NotEmpty(t, entry.kind)
		require.False(t, seenKinds[entry.kind], "duplicate dispatch entry for %q", entry.kind)
		require.NotNil(t, entry.handler)
		seenKinds[entry.kind] = true

		handler, ok := daemonSessionMutationDispatchForKind(entry.kind)
		require.True(t, ok)
		require.NotNil(t, handler)
	}

	require.Equal(t, wantKinds, seenKinds)
	_, ok := daemonSessionMutationDispatchForKind(runtime.SessionMutationKind("unsupported"))
	require.False(t, ok)
}

func TestDaemonSessionMutationCallerInvalidPayloadTableCoversKinds(t *testing.T) {
	wantMessages := map[runtime.SessionMutationKind]string{
		runtime.SessionMutationAppendMessages:       "daemon: append messages mutator returned invalid payload",
		runtime.SessionMutationRecordUserMessage:    "daemon: record user message mutator returned invalid payload",
		runtime.SessionMutationRequestAuthorization: "daemon: request authorization mutator returned invalid payload",
		runtime.SessionMutationCapabilityBlocked:    "daemon: capability blocked mutator returned invalid payload",
		runtime.SessionMutationControlSession:       "daemon: control session mutator returned invalid payload",
		runtime.SessionMutationResolveAuthorization: "daemon: resolve authorization mutator returned invalid payload",
	}
	seenMessages := map[runtime.SessionMutationKind]string{}

	for _, entry := range daemonSessionMutationCallerInvalidPayloadTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.message)
		require.Empty(t, seenMessages[entry.kind], "duplicate invalid payload entry for %q", entry.kind)
		seenMessages[entry.kind] = entry.message
		require.Equal(t, entry.message, daemonSessionMutationCallerInvalidPayloadError(entry.kind))

		_, ok := daemonSessionMutationResultAdapterForKind(entry.kind)
		require.True(t, ok, "missing result adapter for invalid payload audited kind %q", entry.kind)
	}

	require.Equal(t, wantMessages, seenMessages)
	require.Equal(t, daemonSessionMutationCallerDefaultInvalidPayloadError, daemonSessionMutationCallerInvalidPayloadError(runtime.SessionMutationKind("unsupported")))
}

func TestDaemonSessionMutationCallerPolicyTableCoversKinds(t *testing.T) {
	wantPolicies := map[runtime.SessionMutationKind]daemonSessionMutationCallerPolicy{
		runtime.SessionMutationAppendMessages:       daemonSessionMutationCallerPolicyBoolOnly,
		runtime.SessionMutationRecordUserMessage:    daemonSessionMutationCallerPolicyStrictResult,
		runtime.SessionMutationRequestAuthorization: daemonSessionMutationCallerPolicyOptionalResult,
		runtime.SessionMutationCapabilityBlocked:    daemonSessionMutationCallerPolicyOptionalResult,
		runtime.SessionMutationControlSession:       daemonSessionMutationCallerPolicyStrictResult,
		runtime.SessionMutationResolveAuthorization: daemonSessionMutationCallerPolicyStrictResult,
	}
	seenPolicies := map[runtime.SessionMutationKind]daemonSessionMutationCallerPolicy{}

	for _, entry := range daemonSessionMutationCallerPolicyTable {
		require.NotEmpty(t, entry.kind)
		require.NotEmpty(t, entry.policy)
		require.Empty(t, seenPolicies[entry.kind], "duplicate caller policy entry for %q", entry.kind)
		seenPolicies[entry.kind] = entry.policy

		policy, ok := daemonSessionMutationCallerPolicyForKind(entry.kind)
		require.True(t, ok)
		require.Equal(t, entry.policy, policy)
		require.NotEqual(t, daemonSessionMutationCallerDefaultInvalidPayloadError, daemonSessionMutationCallerInvalidPayloadError(entry.kind))

		_, ok = daemonSessionMutationResultAdapterForKind(entry.kind)
		require.True(t, ok, "missing result adapter for policy audited kind %q", entry.kind)
	}

	require.Equal(t, wantPolicies, seenPolicies)
	_, ok := daemonSessionMutationCallerPolicyForKind(runtime.SessionMutationKind("unsupported"))
	require.False(t, ok)

	require.True(t, daemonSessionMutationCallerHandledForKind(
		daemonSessionMutationCallerEgress{handled: true},
		runtime.SessionMutationAppendMessages,
	))
	require.True(t, daemonSessionMutationCallerHandledForKind(
		daemonSessionMutationCallerEgress{handled: true, resultErr: errors.New("mutator failed")},
		runtime.SessionMutationAppendMessages,
	))
	require.False(t, daemonSessionMutationCallerHandledForKind(
		daemonSessionMutationCallerEgress{handled: true},
		runtime.SessionMutationRecordUserMessage,
	))
	require.False(t, daemonSessionMutationCallerHandledForKind(
		daemonSessionMutationCallerEgress{handled: true, resultErr: errors.New("mutator failed")},
		runtime.SessionMutationRecordUserMessage,
	))

	_, handled, err := daemonSessionMutationCallerResultPayloadForKind[daemonRecordUserMessageMutationResultPayload](
		daemonSessionMutationCallerEgress{handled: true},
		runtime.SessionMutationRequestAuthorization,
		daemonRecordUserMessageMutationResultPayloadFromSessionMutationResult,
	)
	require.True(t, handled)
	require.ErrorContains(t, err, "session mutation caller policy mismatch")

	optionalPayload, ok := daemonSessionMutationCallerOptionalResultPayloadForKind(
		daemonSessionMutationCallerEgress{handled: true},
		runtime.SessionMutationRecordUserMessage,
		daemonRequestAuthorizationMutationResultPayloadFromSessionMutationResult,
	)
	require.False(t, ok)
	require.Empty(t, optionalPayload.Session.SessionID)
}

func TestDaemonSessionMutationCallerFallbackOwnershipTableCoversReasons(t *testing.T) {
	wantReasons := map[daemonSessionMutationCallerFallbackReason]string{
		daemonSessionMutationCallerFallbackEngineNotAttached: daemonSessionMutationCallerFallbackOwnerLegacyCaller,
		daemonSessionMutationCallerFallbackBlocked:           daemonSessionMutationCallerFallbackOwnerLegacyCaller,
	}
	seenReasons := map[daemonSessionMutationCallerFallbackReason]string{}

	for _, entry := range daemonSessionMutationCallerFallbackOwnershipTable {
		require.NotEmpty(t, entry.reason)
		require.NotEmpty(t, entry.owner)
		require.Empty(t, seenReasons[entry.reason], "duplicate fallback ownership entry for %q", entry.reason)
		seenReasons[entry.reason] = entry.owner

		owner, ok := daemonSessionMutationCallerFallbackOwnerForReason(entry.reason)
		require.True(t, ok)
		require.Equal(t, entry.owner, owner)
		require.True(t, daemonSessionMutationCallerFallbackOwnedByLegacyCaller(entry.reason))
	}

	require.Equal(t, wantReasons, seenReasons)
	_, ok := daemonSessionMutationCallerFallbackOwnerForReason(daemonSessionMutationCallerFallbackReason("unsupported"))
	require.False(t, ok)
	require.False(t, daemonSessionMutationCallerFallbackOwnedByLegacyCaller(daemonSessionMutationCallerFallbackReason("unsupported")))
	require.ErrorContains(t, daemonSessionMutationCallerFallbackOwnershipError(daemonSessionMutationCallerFallbackReason("unsupported")), "fallback ownership missing")
	require.False(t, daemonSessionMutationCallerFallbackOwnedByLegacyCaller(daemonSessionMutationCallerFallbackMutatorNotAttached))
	require.ErrorContains(t, daemonSessionMutationCallerFallbackOwnershipError(daemonSessionMutationCallerFallbackMutatorNotAttached), "fallback ownership missing")

	reason, fallback := daemonSessionMutationCallerPreDispatchFallbackReason(nil)
	require.True(t, fallback)
	require.Equal(t, daemonSessionMutationCallerFallbackEngineNotAttached, reason)

	eng := runtime.NewEngine(runtime.NewMemStore())
	reason, fallback = daemonSessionMutationCallerPreDispatchFallbackReason(eng)
	require.True(t, fallback)
	require.Equal(t, daemonSessionMutationCallerFallbackMutatorNotAttached, reason)
	require.False(t, daemonSessionMutatorNotAttachedRetirementAllowsLegacyFallback(reason))

	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(context.Context, runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		return runtime.SessionMutationResult{}, nil
	}))
	_, fallback = daemonSessionMutationCallerPreDispatchFallbackReason(eng)
	require.False(t, fallback)
}

func TestDaemonSharedCycleSessionMutationFallbackAuditCoversAppendBoundary(t *testing.T) {
	wantStages := map[string]daemonSharedCycleSessionMutationFallbackStage{
		"runtime_replan_cycle_append_decision":        daemonSharedCycleSessionMutationFallbackStageRuntimeAppendDecisionOwned,
		"shared_cycle_append_mutation_dispatch":       daemonSharedCycleSessionMutationFallbackStageDaemonMutationDispatch,
		"append_messages_legacy_fallback":             daemonSharedCycleSessionMutationFallbackStageLegacyFallbackRetained,
		"shared_cycle_session_mutation_next_boundary": daemonSharedCycleSessionMutationFallbackStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonSharedCycleSessionMutationFallbackStage{}

	for _, entry := range daemonSharedCycleSessionMutationFallbackTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.Equal(t, runtime.SessionMutationAppendMessages, entry.mutationKind)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonSharedCycleSessionMutationFallbackNextBoundary, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate shared-cycle session mutation fallback entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonSharedCycleSessionMutationFallbackEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Empty(t, daemonSharedCycleSessionMutationFallbackEntriesForStage(daemonSharedCycleSessionMutationFallbackStage("unsupported")))
	require.Len(t, daemonSharedCycleSessionMutationFallbackEntriesForStage(daemonSharedCycleSessionMutationFallbackStageRuntimeAppendDecisionOwned), 1)
	require.Len(t, daemonSharedCycleSessionMutationFallbackEntriesForStage(daemonSharedCycleSessionMutationFallbackStageDaemonMutationDispatch), 1)
	require.Len(t, daemonSharedCycleSessionMutationFallbackEntriesForStage(daemonSharedCycleSessionMutationFallbackStageLegacyFallbackRetained), 1)
	require.Len(t, daemonSharedCycleSessionMutationFallbackEntriesForStage(daemonSharedCycleSessionMutationFallbackStageNextBoundaryChosen), 1)

	selectedBoundary, ok := daemonSharedCycleSessionMutationFallbackSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonSharedCycleSessionMutationFallbackNextBoundary, selectedBoundary)

	dispatch, ok := daemonSharedCycleSessionMutationFallbackEntryForComponent("shared_cycle_append_mutation_dispatch")
	require.True(t, ok)
	require.Contains(t, dispatch.evidence, "appendAgentPlannerActionReplanCycleDecisionForTurn")
	require.Contains(t, dispatch.nextStep, "SessionMutator")

	fallback, ok := daemonSharedCycleSessionMutationFallbackEntryForComponent("append_messages_legacy_fallback")
	require.True(t, ok)
	require.Equal(t, daemonSharedCycleSessionMutationFallbackDispositionLegacyFallbackRetained, fallback.disposition)
	require.Contains(t, fallback.evidence, "appendMessagesWithPlannerLegacyForTurn")
}

func TestDaemonSessionMutatorNotAttachedRetirementAuditCoversAppendBoundary(t *testing.T) {
	wantStages := map[string]daemonSessionMutatorNotAttachedRetirementStage{
		"no_engine_session_mutation_legacy_fallback": daemonSessionMutatorNotAttachedStageNoEngineFallbackRetained,
		"attached_engine_missing_session_mutator":    daemonSessionMutatorNotAttachedStageMissingMutatorBlocked,
		"detached_session_mutator_result":            daemonSessionMutatorNotAttachedStageDetachedResultBlocked,
		"session_mutation_next_fallback_boundary":    daemonSessionMutatorNotAttachedStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonSessionMutatorNotAttachedRetirementStage{}

	for _, entry := range daemonSessionMutatorNotAttachedRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonSessionMutatorNextFallbackRetirementBoundaryBlockedOrError, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate session mutator not-attached retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonSessionMutatorNotAttachedRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Empty(t, daemonSessionMutatorNotAttachedRetirementEntriesForStage(daemonSessionMutatorNotAttachedRetirementStage("unsupported")))
	require.Len(t, daemonSessionMutatorNotAttachedRetirementEntriesForStage(daemonSessionMutatorNotAttachedStageNoEngineFallbackRetained), 1)
	require.Len(t, daemonSessionMutatorNotAttachedRetirementEntriesForStage(daemonSessionMutatorNotAttachedStageMissingMutatorBlocked), 1)
	require.Len(t, daemonSessionMutatorNotAttachedRetirementEntriesForStage(daemonSessionMutatorNotAttachedStageDetachedResultBlocked), 1)
	require.Len(t, daemonSessionMutatorNotAttachedRetirementEntriesForStage(daemonSessionMutatorNotAttachedStageNextBoundaryChosen), 1)

	selectedBoundary, ok := daemonSessionMutatorNotAttachedRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonSessionMutatorNextFallbackRetirementBoundaryBlockedOrError, selectedBoundary)

	require.True(t, daemonSessionMutatorNotAttachedRetirementAllowsLegacyFallback(daemonSessionMutationCallerFallbackEngineNotAttached))
	require.False(t, daemonSessionMutatorNotAttachedRetirementAllowsLegacyFallback(daemonSessionMutationCallerFallbackMutatorNotAttached))
	require.False(t, daemonSessionMutatorNotAttachedRetirementAllowsLegacyFallback(daemonSessionMutationCallerFallbackBlocked))

	missing, ok := daemonSessionMutatorNotAttachedRetirementEntryForComponent("attached_engine_missing_session_mutator")
	require.True(t, ok)
	require.Equal(t, runtime.SessionMutationAppendMessages, missing.mutationKind)
	require.Equal(t, daemonSessionMutatorNotAttachedDispositionBlockedError, missing.disposition)
	require.Contains(t, missing.evidence, "AttachEngine attaches daemonSessionMutator")

	next, ok := daemonSessionMutatorNotAttachedRetirementEntryForComponent("session_mutation_next_fallback_boundary")
	require.True(t, ok)
	require.Contains(t, next.nextStep, "P0.5-N235")
}

func TestDaemonAppendSessionMutationBlockedOrErrorRetirementAuditCoversAppendBoundary(t *testing.T) {
	wantStages := map[string]daemonAppendSessionMutationBlockedOrErrorRetirementStage{
		"no_engine_append_legacy_fallback":        daemonAppendSessionMutationBlockedOrErrorStageNoEngineFallbackRetained,
		"attached_session_mutator_blocked_result": daemonAppendSessionMutationBlockedOrErrorStageBlockedResultSuppressed,
		"attached_session_mutator_hard_error":     daemonAppendSessionMutationBlockedOrErrorStageHardErrorSuppressed,
		"session_mutation_next_fallback_boundary": daemonAppendSessionMutationBlockedOrErrorStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonAppendSessionMutationBlockedOrErrorRetirementStage{}

	for _, entry := range daemonAppendSessionMutationBlockedOrErrorRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonAppendSessionMutationNextFallbackRetirementBoundaryOptionalResult, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate append blocked/error retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonAppendSessionMutationBlockedOrErrorRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Empty(t, daemonAppendSessionMutationBlockedOrErrorRetirementEntriesForStage(daemonAppendSessionMutationBlockedOrErrorRetirementStage("unsupported")))
	require.Len(t, daemonAppendSessionMutationBlockedOrErrorRetirementEntriesForStage(daemonAppendSessionMutationBlockedOrErrorStageNoEngineFallbackRetained), 1)
	require.Len(t, daemonAppendSessionMutationBlockedOrErrorRetirementEntriesForStage(daemonAppendSessionMutationBlockedOrErrorStageBlockedResultSuppressed), 1)
	require.Len(t, daemonAppendSessionMutationBlockedOrErrorRetirementEntriesForStage(daemonAppendSessionMutationBlockedOrErrorStageHardErrorSuppressed), 1)
	require.Len(t, daemonAppendSessionMutationBlockedOrErrorRetirementEntriesForStage(daemonAppendSessionMutationBlockedOrErrorStageNextBoundaryChosen), 1)

	selectedBoundary, ok := daemonAppendSessionMutationBlockedOrErrorRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonAppendSessionMutationNextFallbackRetirementBoundaryOptionalResult, selectedBoundary)

	require.True(t, daemonAppendSessionMutationBlockedOrErrorRetirementAllowsLegacyFallback(daemonSessionMutationCallerFallbackEngineNotAttached))
	require.False(t, daemonAppendSessionMutationBlockedOrErrorRetirementAllowsLegacyFallback(daemonSessionMutationCallerFallbackMutatorNotAttached))
	require.False(t, daemonAppendSessionMutationBlockedOrErrorRetirementAllowsLegacyFallback(daemonSessionMutationCallerFallbackBlocked))

	blocked, ok := daemonAppendSessionMutationBlockedOrErrorRetirementEntryForComponent("attached_session_mutator_blocked_result")
	require.True(t, ok)
	require.Equal(t, runtime.SessionMutationAppendMessages, blocked.mutationKind)
	require.Equal(t, daemonSessionMutationCallerFallbackBlocked, blocked.reason)
	require.Equal(t, daemonAppendSessionMutationBlockedOrErrorDispositionBlockedError, blocked.disposition)

	hardError, ok := daemonAppendSessionMutationBlockedOrErrorRetirementEntryForComponent("attached_session_mutator_hard_error")
	require.True(t, ok)
	require.Equal(t, runtime.SessionMutationAppendMessages, hardError.mutationKind)
	require.Equal(t, daemonAppendSessionMutationBlockedOrErrorDispositionHardError, hardError.disposition)

	next, ok := daemonAppendSessionMutationBlockedOrErrorRetirementEntryForComponent("session_mutation_next_fallback_boundary")
	require.True(t, ok)
	require.Contains(t, next.nextStep, "P0.5-N236")
}

func TestDaemonOptionalSessionMutationFallbackRetirementAuditCoversOptionalCallers(t *testing.T) {
	wantStages := map[string]daemonOptionalSessionMutationFallbackRetirementStage{
		"request_authorization_no_engine_legacy_fallback": daemonOptionalSessionMutationFallbackStageNoEngineFallbackRetained,
		"capability_blocked_no_engine_legacy_fallback":    daemonOptionalSessionMutationFallbackStageNoEngineFallbackRetained,
		"request_authorization_attached_failure":          daemonOptionalSessionMutationFallbackStageRequestAuthorizationSuppressed,
		"capability_blocked_attached_failure":             daemonOptionalSessionMutationFallbackStageCapabilityBlockedSuppressed,
		"session_mutation_next_fallback_boundary":         daemonOptionalSessionMutationFallbackStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonOptionalSessionMutationFallbackRetirementStage{}

	for _, entry := range daemonOptionalSessionMutationFallbackRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonOptionalSessionMutationNextFallbackRetirementBoundaryNoEngineFallback, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate optional fallback retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonOptionalSessionMutationFallbackRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Empty(t, daemonOptionalSessionMutationFallbackRetirementEntriesForStage(daemonOptionalSessionMutationFallbackRetirementStage("unsupported")))
	require.Len(t, daemonOptionalSessionMutationFallbackRetirementEntriesForStage(daemonOptionalSessionMutationFallbackStageNoEngineFallbackRetained), 2)
	require.Len(t, daemonOptionalSessionMutationFallbackRetirementEntriesForStage(daemonOptionalSessionMutationFallbackStageRequestAuthorizationSuppressed), 1)
	require.Len(t, daemonOptionalSessionMutationFallbackRetirementEntriesForStage(daemonOptionalSessionMutationFallbackStageCapabilityBlockedSuppressed), 1)
	require.Len(t, daemonOptionalSessionMutationFallbackRetirementEntriesForStage(daemonOptionalSessionMutationFallbackStageNextBoundaryChosen), 1)

	selectedBoundary, ok := daemonOptionalSessionMutationFallbackRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonOptionalSessionMutationNextFallbackRetirementBoundaryNoEngineFallback, selectedBoundary)

	require.True(t, daemonOptionalSessionMutationFallbackRetirementAllowsLegacyFallback(runtime.SessionMutationRequestAuthorization, daemonSessionMutationCallerFallbackEngineNotAttached))
	require.True(t, daemonOptionalSessionMutationFallbackRetirementAllowsLegacyFallback(runtime.SessionMutationCapabilityBlocked, daemonSessionMutationCallerFallbackEngineNotAttached))
	require.False(t, daemonOptionalSessionMutationFallbackRetirementAllowsLegacyFallback(runtime.SessionMutationRequestAuthorization, daemonSessionMutationCallerFallbackBlocked))
	require.False(t, daemonOptionalSessionMutationFallbackRetirementAllowsLegacyFallback(runtime.SessionMutationCapabilityBlocked, daemonSessionMutationCallerFallbackBlocked))
	require.True(t, daemonOptionalSessionMutationFallbackRetirementBlocksLegacyFallback(runtime.SessionMutationRequestAuthorization))
	require.True(t, daemonOptionalSessionMutationFallbackRetirementBlocksLegacyFallback(runtime.SessionMutationCapabilityBlocked))
	require.False(t, daemonOptionalSessionMutationFallbackRetirementBlocksLegacyFallback(runtime.SessionMutationRecordUserMessage))

	requestAuth, ok := daemonOptionalSessionMutationFallbackRetirementEntryForComponent("request_authorization_attached_failure")
	require.True(t, ok)
	require.Equal(t, runtime.SessionMutationRequestAuthorization, requestAuth.mutationKind)
	require.Equal(t, daemonOptionalSessionMutationFallbackDispositionBlockedError, requestAuth.disposition)
	require.Contains(t, requestAuth.evidence, "blocked, hard-error or invalid-payload")

	next, ok := daemonOptionalSessionMutationFallbackRetirementEntryForComponent("session_mutation_next_fallback_boundary")
	require.True(t, ok)
	require.Contains(t, next.nextStep, "P0.5-N237")
}

func TestDaemonNoEngineSessionMutationFallbackRetirementAuditKeepsRollbackSwitch(t *testing.T) {
	wantStages := map[string]daemonNoEngineSessionMutationFallbackRetirementStage{
		"production_default_runtime_engine":               daemonNoEngineSessionMutationFallbackStageProductionDefaultEngineAttached,
		"legacy_runtime_opt_out_switch":                   daemonNoEngineSessionMutationFallbackStageLegacyRuntimeOptOutRetained,
		"append_messages_no_engine_legacy_fallback":       daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		"record_user_message_no_engine_legacy_fallback":   daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		"request_authorization_no_engine_legacy_fallback": daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		"capability_blocked_no_engine_legacy_fallback":    daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		"control_session_no_engine_legacy_fallback":       daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		"resolve_authorization_no_engine_legacy_fallback": daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained,
		"session_mutation_no_engine_next_boundary":        daemonNoEngineSessionMutationFallbackStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonNoEngineSessionMutationFallbackRetirementStage{}

	for _, entry := range daemonNoEngineSessionMutationFallbackRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization, entry.nextBoundary)
		require.NotEmpty(t, entry.nextStep)
		require.Empty(t, seenStages[entry.component], "duplicate no-engine session mutation fallback entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonNoEngineSessionMutationFallbackRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, wantStages, seenStages)
	require.Empty(t, daemonNoEngineSessionMutationFallbackRetirementEntriesForStage(daemonNoEngineSessionMutationFallbackRetirementStage("unsupported")))
	require.Len(t, daemonNoEngineSessionMutationFallbackRetirementEntriesForStage(daemonNoEngineSessionMutationFallbackStageProductionDefaultEngineAttached), 1)
	require.Len(t, daemonNoEngineSessionMutationFallbackRetirementEntriesForStage(daemonNoEngineSessionMutationFallbackStageLegacyRuntimeOptOutRetained), 1)
	require.Len(t, daemonNoEngineSessionMutationFallbackRetirementEntriesForStage(daemonNoEngineSessionMutationFallbackStageNoEngineFallbackRetained), len(daemonSessionMutationCallerPolicyTable))
	require.Len(t, daemonNoEngineSessionMutationFallbackRetirementEntriesForStage(daemonNoEngineSessionMutationFallbackStageNextBoundaryChosen), 1)

	selectedBoundary, ok := daemonNoEngineSessionMutationFallbackRetirementSelectedBoundary()
	require.True(t, ok)
	require.Equal(t, daemonNoEngineSessionMutationFallbackNextBoundaryManualAuthorization, selectedBoundary)

	for _, policy := range daemonSessionMutationCallerPolicyTable {
		require.True(t, daemonNoEngineSessionMutationFallbackRetirementRetainsKind(policy.kind), "missing no-engine fallback audit for %s", policy.kind)
	}
	require.False(t, daemonNoEngineSessionMutationFallbackRetirementRetainsKind(runtime.SessionMutationKind("unsupported")))

	defaultEngine, ok := daemonNoEngineSessionMutationFallbackRetirementEntryForComponent("production_default_runtime_engine")
	require.True(t, ok)
	require.Contains(t, defaultEngine.evidence, "AttachEngine")
	require.Equal(t, daemonNoEngineSessionMutationFallbackDispositionRuntimeDefault, defaultEngine.disposition)

	optOut, ok := daemonNoEngineSessionMutationFallbackRetirementEntryForComponent("legacy_runtime_opt_out_switch")
	require.True(t, ok)
	require.Equal(t, daemonSessionMutationCallerFallbackEngineNotAttached, optOut.reason)
	require.Equal(t, daemonNoEngineSessionMutationFallbackDispositionRollbackSwitch, optOut.disposition)
	require.Contains(t, optOut.evidence, "CYBERAGENT_RUNTIME=legacy")

	next, ok := daemonNoEngineSessionMutationFallbackRetirementEntryForComponent("session_mutation_no_engine_next_boundary")
	require.True(t, ok)
	require.Equal(t, daemonNoEngineSessionMutationFallbackDispositionManualGateNext, next.disposition)
	require.Contains(t, next.nextStep, "manual stage/commit/tag")
}

func TestDaemonNoEngineSessionMutationFallbackSourceAuditKeepsWholeEngineRollback(t *testing.T) {
	serverData, err := os.ReadFile("server.go")
	require.NoError(t, err)
	serverSource := string(serverData)
	require.Contains(t, serverSource, `agentRuntimeEngineEnabledFromEnv(os.Getenv("CYBERAGENT_RUNTIME"))`)
	require.Contains(t, serverSource, `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`)
	require.NotContains(t, serverSource, "CYBERAGENT_RUNTIME_COMPAT")
	require.NotContains(t, serverSource, "CYBERAGENT_DEPRECATED_RUNTIME_API")

	runtimeData, err := os.ReadFile("agent_runtime.go")
	require.NoError(t, err)
	runtimeSource := string(runtimeData)
	require.Contains(t, runtimeSource, "func (m *agentManager) AttachEngine(e *runtime.Engine)")
	require.Contains(t, runtimeSource, "e.AttachSessionMutator(daemonSessionMutator{manager: m})")
	require.NotContains(t, runtimeSource, "CYBERAGENT_RUNTIME_COMPAT")
	require.NotContains(t, runtimeSource, "CYBERAGENT_DEPRECATED_RUNTIME_API")

	sessionMutationData, err := os.ReadFile("agent_session_mutator.go")
	require.NoError(t, err)
	sessionMutationSource := string(sessionMutationData)
	start := strings.Index(sessionMutationSource, "func (m *agentManager) runRuntimeSessionMutation")
	require.NotEqual(t, -1, start)
	end := strings.Index(sessionMutationSource[start:], "\nfunc (m *agentManager) runRuntimeSessionMutationCallerEgress")
	require.NotEqual(t, -1, end)
	runRuntimeMutation := sessionMutationSource[start : start+end]
	require.Contains(t, runRuntimeMutation, "daemonSessionMutationCallerPreDispatchFallbackReason(eng)")
	require.Contains(t, runRuntimeMutation, "return runtime.SessionMutationResult{}, false, nil")
	require.Contains(t, runRuntimeMutation, "runtime.ErrSessionMutatorNotAttached")
}

func TestDaemonSharedCycleSessionMutationFallbackSourceAuditScopesAppendFallback(t *testing.T) {
	plannerData, err := os.ReadFile("agent_planner_loop.go")
	require.NoError(t, err)
	plannerSource := string(plannerData)
	start := strings.Index(plannerSource, "func (m *agentManager) appendAgentPlannerActionReplanCycleDecisionForTurn")
	require.NotEqual(t, -1, start)
	end := strings.Index(plannerSource[start:], "\nfunc (m *agentManager) runAgentPlannerActionReplanCycleActionExecution")
	require.NotEqual(t, -1, end)
	appendDecision := plannerSource[start : start+end]
	require.Contains(t, appendDecision, "decision.Clone()")
	require.Contains(t, appendDecision, "m.appendMessagesWithPlannerForTurn")
	require.Contains(t, appendDecision, "m.appendMessagesForTurn")
	require.NotContains(t, appendDecision, "appendMessagesWithPlannerLegacyForTurn")
	require.NotContains(t, appendDecision, "runRuntimeSessionMutation")

	helpersData, err := os.ReadFile("agent_session_helpers.go")
	require.NoError(t, err)
	helpersSource := string(helpersData)
	start = strings.Index(helpersSource, "func (m *agentManager) appendMessagesWithPlannerForTurn")
	require.NotEqual(t, -1, start)
	end = strings.Index(helpersSource[start:], "\nfunc (m *agentManager) appendMessagesWithPlannerLegacy")
	require.NotEqual(t, -1, end)
	appendForTurn := helpersSource[start : start+end]
	require.Contains(t, appendForTurn, "m.runRuntimeAppendMessagesMutationForTurn")
	require.Contains(t, appendForTurn, "m.appendMessagesWithPlannerLegacyForTurn")

	mutatorData, err := os.ReadFile("agent_session_mutator.go")
	require.NoError(t, err)
	mutatorSource := string(mutatorData)
	require.Contains(t, mutatorSource, "func (m *agentManager) runRuntimeSessionMutation")
	require.Contains(t, mutatorSource, "daemonSessionMutationCallerPreDispatchFallbackReason")
	require.Contains(t, mutatorSource, "daemonSessionMutationCallerResultFallbackReason")
	require.Contains(t, mutatorSource, "daemonSessionMutationCallerFallbackMutatorNotAttached")
	require.Contains(t, mutatorSource, "daemonSessionMutationCallerMutatorNotAttachedBlockedResult")
	require.Contains(t, mutatorSource, "daemonSessionMutatorNotAttachedRetirementAllowsLegacyFallback")
}

func TestDaemonSessionMutationAdapterAuditTablesStayAligned(t *testing.T) {
	wantKinds := map[runtime.SessionMutationKind]bool{
		runtime.SessionMutationAppendMessages:       true,
		runtime.SessionMutationRecordUserMessage:    true,
		runtime.SessionMutationRequestAuthorization: true,
		runtime.SessionMutationCapabilityBlocked:    true,
		runtime.SessionMutationControlSession:       true,
		runtime.SessionMutationResolveAuthorization: true,
	}
	dispatchKinds := map[runtime.SessionMutationKind]bool{}
	resultAdapterKinds := map[runtime.SessionMutationKind]bool{}
	invalidPayloadKinds := map[runtime.SessionMutationKind]bool{}
	policyKinds := map[runtime.SessionMutationKind]bool{}

	for _, entry := range daemonSessionMutationDispatchTable {
		dispatchKinds[entry.kind] = true
	}
	for _, entry := range daemonSessionMutationResultAdapterTable {
		resultAdapterKinds[entry.kind] = true
	}
	for _, entry := range daemonSessionMutationCallerInvalidPayloadTable {
		invalidPayloadKinds[entry.kind] = true
	}
	for _, entry := range daemonSessionMutationCallerPolicyTable {
		policyKinds[entry.kind] = true
	}

	require.Equal(t, wantKinds, dispatchKinds)
	require.Equal(t, wantKinds, resultAdapterKinds)
	require.Equal(t, wantKinds, invalidPayloadKinds)
	require.Equal(t, wantKinds, policyKinds)
}

func TestDaemonSessionMutationPayloadHelperClonesAndReportsInvalid(t *testing.T) {
	source := &runtime.AgentLoopAppendMessagesPayload{
		Messages: []runtime.AgentLoopMessage{{
			ID:      "msg-session-mutation-payload-helper",
			Role:    "agent",
			Content: "original",
			ToolCalls: []runtime.AgentLoopToolCall{{
				ID:       "tool-session-mutation-payload-helper",
				Name:     "daemon.health",
				Metadata: map[string]any{"phase": "original"},
			}},
		}},
	}
	payload, err := daemonSessionMutationPayload[runtime.AgentLoopAppendMessagesPayload](runtime.SessionMutationRequest{Payload: source}, "daemon: invalid append messages mutation payload")
	require.NoError(t, err)
	source.Messages[0].Content = "mutated"
	source.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated"
	require.Equal(t, "original", payload.Messages[0].Content)
	require.Equal(t, "original", payload.Messages[0].ToolCalls[0].Metadata["phase"])

	_, err = daemonSessionMutationPayload[runtime.AgentLoopAppendMessagesPayload](
		runtime.SessionMutationRequest{Payload: "invalid"},
		"daemon: invalid append messages mutation payload",
	)
	require.EqualError(t, err, "daemon: invalid append messages mutation payload")

	var nilPayload *runtime.AgentLoopAppendMessagesPayload
	_, err = daemonSessionMutationPayload[runtime.AgentLoopAppendMessagesPayload](
		runtime.SessionMutationRequest{Payload: nilPayload},
		"daemon: invalid append messages mutation payload",
	)
	require.EqualError(t, err, "daemon: invalid append messages mutation payload")
}

func TestDaemonSessionMutationFinishedResultClonesData(t *testing.T) {
	data := map[string]any{
		"mutator":    "daemon_legacy_session",
		"kind":       string(runtime.SessionMutationRecordUserMessage),
		"session_id": "session-finished-result",
	}
	result := daemonSessionMutationFinishedResult(
		"legacy daemon user message mutation executed",
		runtime.AgentLoopRecordUserMessageResultPayload{
			Session: runtime.AgentLoopSessionSnapshot{SessionID: "session-finished-result"},
			Mutated: true,
		},
		data,
	)
	data["kind"] = "mutated"

	require.Equal(t, runtime.TurnResultFinished, result.Status)
	require.Equal(t, "legacy daemon user message mutation executed", result.Reason)
	require.Equal(t, string(runtime.SessionMutationRecordUserMessage), result.Data["kind"])
	payload, ok := result.Payload.(runtime.AgentLoopRecordUserMessageResultPayload)
	require.True(t, ok, "payload type = %T", result.Payload)
	require.Equal(t, "session-finished-result", payload.Session.SessionID)
	require.True(t, payload.Mutated)
}

func TestDaemonLegacySessionMutationResultPayload(t *testing.T) {
	resultPayload := daemonControlSessionMutationResultPayload{
		Session: agentSession{SessionID: "agent-result"},
		Message: agentMessage{ID: "msg-result"},
		Mutated: true,
	}
	result := runtime.SessionMutationResult{Payload: resultPayload}
	payload, ok := daemonLegacySessionMutationResultPayload[daemonControlSessionMutationResultPayload](result)
	require.True(t, ok)
	require.Equal(t, "agent-result", payload.Session.SessionID)
	require.Equal(t, "msg-result", payload.Message.ID)
	require.True(t, payload.Mutated)

	_, ok = daemonLegacySessionMutationResultPayload[daemonControlSessionMutationResultPayload](runtime.SessionMutationResult{Payload: "invalid"})
	require.False(t, ok)
}

func TestDaemonSessionMutationResultPayloadFromRuntimeOrLegacy(t *testing.T) {
	convert := func(result runtime.SessionMutationResult) (daemonControlSessionMutationResultPayload, bool) {
		return daemonSessionMutationResultPayloadFromRuntimeOrLegacy[daemonControlSessionMutationResultPayload, runtime.AgentLoopControlSessionResultPayload](
			result,
			daemonControlSessionMutationResultPayloadFromAgentLoop,
		)
	}

	legacyPayload := daemonControlSessionMutationResultPayload{
		Session: agentSession{SessionID: "legacy-result"},
		Message: agentMessage{ID: "msg-legacy"},
		Mutated: true,
	}
	gotLegacy, ok := convert(runtime.SessionMutationResult{Payload: legacyPayload})
	require.True(t, ok)
	require.Equal(t, "legacy-result", gotLegacy.Session.SessionID)
	require.Equal(t, "msg-legacy", gotLegacy.Message.ID)
	require.True(t, gotLegacy.Mutated)

	runtimePayload := runtime.AgentLoopControlSessionResultPayload{
		Session: runtime.AgentLoopSessionSnapshot{SessionID: "runtime-result"},
		Message: runtime.AgentLoopMessage{ID: "msg-runtime"},
		Mutated: true,
	}
	gotRuntime, ok := convert(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, "runtime-result", gotRuntime.Session.SessionID)
	require.Equal(t, "msg-runtime", gotRuntime.Message.ID)

	gotPointer, ok := convert(runtime.SessionMutationResult{Payload: &runtimePayload})
	require.True(t, ok)
	require.Equal(t, gotRuntime, gotPointer)

	var nilRuntimePayload *runtime.AgentLoopControlSessionResultPayload
	_, ok = convert(runtime.SessionMutationResult{Payload: nilRuntimePayload})
	require.False(t, ok)

	_, ok = convert(runtime.SessionMutationResult{Payload: "invalid"})
	require.False(t, ok)
}

func TestDaemonSessionMutationResultAdapterTableCoversKinds(t *testing.T) {
	wantKinds := map[runtime.SessionMutationKind]bool{
		runtime.SessionMutationAppendMessages:       true,
		runtime.SessionMutationRecordUserMessage:    true,
		runtime.SessionMutationRequestAuthorization: true,
		runtime.SessionMutationCapabilityBlocked:    true,
		runtime.SessionMutationControlSession:       true,
		runtime.SessionMutationResolveAuthorization: true,
	}
	seenKinds := map[runtime.SessionMutationKind]bool{}

	for _, entry := range daemonSessionMutationResultAdapterTable {
		require.NotEmpty(t, entry.kind)
		require.False(t, seenKinds[entry.kind], "duplicate result adapter entry for %q", entry.kind)
		require.NotNil(t, entry.fromSessionMutationResult)
		seenKinds[entry.kind] = true

		recovered, ok := daemonSessionMutationResultAdapterForKind(entry.kind)
		require.True(t, ok)
		require.Equal(t, entry.kind, recovered.kind)
		require.NotNil(t, recovered.fromSessionMutationResult)
	}

	require.Equal(t, wantKinds, seenKinds)
	_, ok := daemonSessionMutationResultAdapterForKind(runtime.SessionMutationKind("unsupported"))
	require.False(t, ok)
}

func TestDaemonSessionMutationResultPayloadForKindUsesAdapterTable(t *testing.T) {
	controlPayload := runtime.AgentLoopControlSessionResultPayload{
		Session: runtime.AgentLoopSessionSnapshot{
			SessionID: "session-result-adapter-table",
			Messages: []runtime.AgentLoopMessage{{
				ID: "msg-session-result-adapter-table-history",
				ToolCalls: []runtime.AgentLoopToolCall{{
					ID:       "tool-session-result-adapter-table-history",
					Metadata: map[string]any{"phase": "history"},
				}},
			}},
		},
		Message: runtime.AgentLoopMessage{
			ID: "msg-session-result-adapter-table",
			ToolCalls: []runtime.AgentLoopToolCall{{
				ID:       "tool-session-result-adapter-table",
				Metadata: map[string]any{"phase": "result"},
			}},
		},
		Mutated: true,
	}

	converted, ok := daemonSessionMutationResultPayloadForKind[daemonControlSessionMutationResultPayload](
		runtime.SessionMutationControlSession,
		runtime.SessionMutationResult{Payload: controlPayload},
	)
	require.True(t, ok)
	require.Equal(t, "session-result-adapter-table", converted.Session.SessionID)
	require.Equal(t, "msg-session-result-adapter-table", converted.Message.ID)
	require.True(t, converted.Mutated)

	controlPayload.Session.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-history"
	controlPayload.Message.ToolCalls[0].Metadata["phase"] = "mutated-result"
	require.Equal(t, "history", converted.Session.Messages[0].ToolCalls[0].Metadata["phase"])
	require.Equal(t, "result", converted.Message.ToolCalls[0].Metadata["phase"])

	legacyPayload := daemonCapabilityBlockedMutationResultPayload{
		Session:         agentSession{SessionID: "legacy-session-result-adapter-table"},
		ActionName:      "workspace.files.write",
		IntentID:        "intent-result-adapter-table",
		Reason:          "requires approval",
		SuggestedPrompt: "try read-only",
		Mutated:         true,
	}
	convertedLegacy, ok := daemonSessionMutationResultPayloadForKind[daemonCapabilityBlockedMutationResultPayload](
		runtime.SessionMutationCapabilityBlocked,
		runtime.SessionMutationResult{Payload: legacyPayload},
	)
	require.True(t, ok)
	require.Equal(t, legacyPayload, convertedLegacy)

	_, ok = daemonSessionMutationResultPayloadForKind[daemonControlSessionMutationResultPayload](
		runtime.SessionMutationCapabilityBlocked,
		runtime.SessionMutationResult{Payload: legacyPayload},
	)
	require.False(t, ok)

	_, ok = daemonSessionMutationResultPayloadForKind[daemonControlSessionMutationResultPayload](
		runtime.SessionMutationKind("unsupported"),
		runtime.SessionMutationResult{Payload: controlPayload},
	)
	require.False(t, ok)
}

func TestDaemonSessionMutationCallerResultPayloadHandlesEgressStates(t *testing.T) {
	expected := runtime.AgentLoopControlSessionResultPayload{
		Session: runtime.AgentLoopSessionSnapshot{
			SessionID: "session-caller-egress",
			Messages: []runtime.AgentLoopMessage{{
				ID: "msg-caller-egress",
				ToolCalls: []runtime.AgentLoopToolCall{{
					ID:       "tool-message-caller-egress",
					Metadata: map[string]any{"phase": "message"},
				}},
			}},
			ToolCalls: []runtime.AgentLoopToolCall{{
				ID:       "tool-session-caller-egress",
				Result:   map[string]any{"ok": true},
				Metadata: map[string]any{"phase": "session"},
			}},
		},
		Message: runtime.AgentLoopMessage{
			ID: "msg-result-caller-egress",
			ToolCalls: []runtime.AgentLoopToolCall{{
				ID:       "tool-result-caller-egress",
				Metadata: map[string]any{"phase": "result"},
			}},
		},
		Mutated: true,
	}

	payload, handled, err := daemonSessionMutationCallerResultPayload(
		daemonSessionMutationCallerEgress{result: runtime.SessionMutationResult{Payload: expected}, handled: true},
		daemonControlSessionMutationResultPayloadFromSessionMutationResult,
		"daemon: custom mutator returned invalid payload",
	)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "session-caller-egress", payload.Session.SessionID)
	require.Equal(t, "msg-result-caller-egress", payload.Message.ID)
	require.True(t, payload.Mutated)

	expected.Session.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message"
	expected.Session.ToolCalls[0].Result["ok"] = false
	expected.Session.ToolCalls[0].Metadata["phase"] = "mutated-session"
	expected.Message.ToolCalls[0].Metadata["phase"] = "mutated-result"
	require.Equal(t, "message", payload.Session.Messages[0].ToolCalls[0].Metadata["phase"])
	require.Equal(t, true, payload.Session.ToolCalls[0].Result["ok"])
	require.Equal(t, "session", payload.Session.ToolCalls[0].Metadata["phase"])
	require.Equal(t, "result", payload.Message.ToolCalls[0].Metadata["phase"])

	pointerPayload := expected.Clone()
	payload, handled, err = daemonSessionMutationCallerResultPayload(
		daemonSessionMutationCallerEgress{result: runtime.SessionMutationResult{Payload: &pointerPayload}, handled: true},
		daemonControlSessionMutationResultPayloadFromSessionMutationResult,
		"daemon: custom mutator returned invalid payload",
	)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "session-caller-egress", payload.Session.SessionID)

	payload, handled, err = daemonSessionMutationCallerResultPayload(
		daemonSessionMutationCallerEgress{},
		daemonControlSessionMutationResultPayloadFromSessionMutationResult,
		"daemon: custom mutator returned invalid payload",
	)
	require.NoError(t, err)
	require.False(t, handled)
	require.Empty(t, payload.Session.SessionID)

	boom := errors.New("mutator failed")
	payload, handled, err = daemonSessionMutationCallerResultPayload(
		daemonSessionMutationCallerEgress{handled: true, resultErr: boom},
		daemonControlSessionMutationResultPayloadFromSessionMutationResult,
		"daemon: custom mutator returned invalid payload",
	)
	require.ErrorIs(t, err, boom)
	require.True(t, handled)
	require.Empty(t, payload.Session.SessionID)

	payload, handled, err = daemonSessionMutationCallerResultPayload(
		daemonSessionMutationCallerEgress{result: runtime.SessionMutationResult{Payload: "not a payload"}, handled: true},
		daemonControlSessionMutationResultPayloadFromSessionMutationResult,
		"daemon: custom mutator returned invalid payload",
	)
	require.ErrorContains(t, err, "custom mutator returned invalid payload")
	require.True(t, handled)
	require.Empty(t, payload.Session.SessionID)
}

func TestDaemonSessionMutationCallerOptionalResultPayloadSuppressesFailures(t *testing.T) {
	expected := runtime.AgentLoopCapabilityBlockedResultPayload{
		Session:         runtime.AgentLoopSessionSnapshot{SessionID: "session-optional-egress"},
		ActionName:      "daemon.health",
		IntentID:        "intent-optional-egress",
		Reason:          "requires approval",
		SuggestedPrompt: "try read-only",
		Mutated:         true,
	}

	payload, ok := daemonSessionMutationCallerOptionalResultPayload(
		daemonSessionMutationCallerEgress{result: runtime.SessionMutationResult{Payload: expected}, handled: true},
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
		"daemon: capability blocked mutator returned invalid payload",
	)
	require.True(t, ok)
	require.Equal(t, "session-optional-egress", payload.Session.SessionID)
	require.Equal(t, "daemon.health", payload.ActionName)
	require.True(t, payload.Mutated)

	payload, ok = daemonSessionMutationCallerOptionalResultPayload(
		daemonSessionMutationCallerEgress{},
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
		"daemon: capability blocked mutator returned invalid payload",
	)
	require.False(t, ok)
	require.Empty(t, payload.Session.SessionID)

	payload, ok = daemonSessionMutationCallerOptionalResultPayload(
		daemonSessionMutationCallerEgress{handled: true, resultErr: errors.New("mutator failed")},
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
		"daemon: capability blocked mutator returned invalid payload",
	)
	require.False(t, ok)
	require.Empty(t, payload.Session.SessionID)

	payload, ok = daemonSessionMutationCallerOptionalResultPayload(
		daemonSessionMutationCallerEgress{result: runtime.SessionMutationResult{Payload: "not a payload"}, handled: true},
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
		"daemon: capability blocked mutator returned invalid payload",
	)
	require.False(t, ok)
	require.Empty(t, payload.Session.SessionID)

	payload, ok = daemonSessionMutationCallerOptionalResultPayloadForKind(
		daemonSessionMutationCallerEgress{result: runtime.SessionMutationResult{Payload: expected}, handled: true},
		runtime.SessionMutationCapabilityBlocked,
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
	)
	require.True(t, ok)
	require.Equal(t, "session-optional-egress", payload.Session.SessionID)

	payload, ok = daemonSessionMutationCallerOptionalResultPayloadForKind(
		daemonSessionMutationCallerEgress{},
		runtime.SessionMutationCapabilityBlocked,
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
	)
	require.False(t, ok)
	require.Empty(t, payload.Session.SessionID)

	payload, ok = daemonSessionMutationCallerOptionalResultPayloadForKind(
		daemonSessionMutationCallerEgress{handled: true, resultErr: errors.New("mutator failed")},
		runtime.SessionMutationCapabilityBlocked,
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
	)
	require.True(t, ok)
	require.Empty(t, payload.Session.SessionID)

	payload, ok = daemonSessionMutationCallerOptionalResultPayloadForKind(
		daemonSessionMutationCallerEgress{
			result: runtime.SessionMutationResult{
				Status: runtime.TurnResultBlocked,
				Reason: "blocked by attached mutator",
			},
		},
		runtime.SessionMutationCapabilityBlocked,
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
	)
	require.True(t, ok)
	require.Empty(t, payload.Session.SessionID)

	payload, ok = daemonSessionMutationCallerOptionalResultPayloadForKind(
		daemonSessionMutationCallerEgress{result: runtime.SessionMutationResult{Payload: "not a payload"}, handled: true},
		runtime.SessionMutationCapabilityBlocked,
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
	)
	require.True(t, ok)
	require.Empty(t, payload.Session.SessionID)

	payload, ok = daemonSessionMutationCallerOptionalResultPayloadForKind(
		daemonSessionMutationCallerEgress{handled: true, resultErr: errors.New("mutator failed")},
		runtime.SessionMutationRecordUserMessage,
		daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult,
	)
	require.False(t, ok)
	require.Empty(t, payload.Session.SessionID)
}

func TestDaemonSessionMutationCallerHandledSuppressesFailures(t *testing.T) {
	require.True(t, daemonSessionMutationCallerHandled(daemonSessionMutationCallerEgress{handled: true}))
	require.False(t, daemonSessionMutationCallerHandled(daemonSessionMutationCallerEgress{}))
	require.False(t, daemonSessionMutationCallerHandled(daemonSessionMutationCallerEgress{handled: true, resultErr: errors.New("mutator failed")}))
	require.True(t, daemonSessionMutationCallerHandledForKind(
		daemonSessionMutationCallerEgress{handled: true, resultErr: errors.New("mutator failed")},
		runtime.SessionMutationAppendMessages,
	))
	require.True(t, daemonSessionMutationCallerHandledForKind(
		daemonSessionMutationCallerEgress{
			result: runtime.SessionMutationResult{
				Status: runtime.TurnResultBlocked,
				Reason: "blocked by attached mutator",
			},
		},
		runtime.SessionMutationAppendMessages,
	))
	require.False(t, daemonSessionMutationCallerHandledForKind(
		daemonSessionMutationCallerEgress{handled: true, resultErr: errors.New("mutator failed")},
		runtime.SessionMutationRecordUserMessage,
	))
}

func TestAgentManagerSessionMutationCallerInvalidPayloadFallbackAudit(t *testing.T) {
	newInvalidPayloadManager := func(t *testing.T) *agentManager {
		t.Helper()
		mgr := newAgentManager(t.TempDir())
		eng := runtime.NewEngine(runtime.NewMemStore())
		mgr.AttachEngine(eng)
		eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
			require.NotNil(t, ctx)
			require.NotEmpty(t, req.Kind)
			return runtime.SessionMutationResult{
				Status:  runtime.TurnResultFinished,
				Payload: "not a session mutation payload",
			}, nil
		}))
		return mgr
	}

	t.Run("append messages ignores result payload and only checks handled error state", func(t *testing.T) {
		mgr := newInvalidPayloadManager(t)
		handled := mgr.runRuntimeAppendMessagesMutationWithTurn(
			nil,
			runtime.AgentLoopAppendMessagesMutationTurn("session-invalid-append", "task-invalid-append", "run-invalid-append", "turn-invalid-append", "append", "planner", "safe_scan", 1, false),
			nil,
			agentMessage{ID: "msg-invalid-append", Role: "agent", Content: "append"},
		)
		require.True(t, handled)
	})

	t.Run("record user message treats invalid payload as handled error", func(t *testing.T) {
		mgr := newInvalidPayloadManager(t)
		payload, handled, err := mgr.runRuntimeRecordUserMessageMutation(nil, "session-invalid-record", "scan safely", "operator-1", "en", "safe_scan")
		require.True(t, handled)
		require.ErrorContains(t, err, daemonSessionMutationCallerInvalidPayloadError(runtime.SessionMutationRecordUserMessage))
		require.Empty(t, payload.Session.SessionID)
	})

	t.Run("control session treats invalid payload as handled error", func(t *testing.T) {
		mgr := newInvalidPayloadManager(t)
		payload, handled, err := mgr.runRuntimeControlSessionMutation(nil, "session-invalid-control", "pause", agentControlRequest{Action: "pause", Prompt: "pause"}, "operator-1", "en", "safe_scan")
		require.True(t, handled)
		require.ErrorContains(t, err, daemonSessionMutationCallerInvalidPayloadError(runtime.SessionMutationControlSession))
		require.Empty(t, payload.Session.SessionID)
	})

	t.Run("resolve authorization treats invalid payload as handled error", func(t *testing.T) {
		mgr := newInvalidPayloadManager(t)
		payload, handled, err := mgr.runRuntimeResolveAuthorizationMutation(nil, "session-invalid-resolve", "deny", agentAuthorizationDecisionRequest{Action: "deny", AuthorizationID: "auth-invalid-resolve"}, "operator-1", "en", "safe_scan")
		require.True(t, handled)
		require.ErrorContains(t, err, daemonSessionMutationCallerInvalidPayloadError(runtime.SessionMutationResolveAuthorization))
		require.Empty(t, payload.Session.SessionID)
	})

	t.Run("request authorization invalid payload blocks legacy caller", func(t *testing.T) {
		mgr := newInvalidPayloadManager(t)
		payload, handled := mgr.runRuntimeRequestAuthorizationMutationForTurn(
			nil,
			"session-invalid-request-auth",
			agentRuntimeTurnRef{TaskID: "task-invalid-request-auth", RunID: "run-invalid-request-auth", TurnID: "turn-invalid-request-auth"},
			"write a file",
			"en",
			"safe_scan",
			runtime.AgentLoopAction{Name: "workspace.files.write"},
			"workspace.files.write",
			"workspace_write",
			"manual",
			"requires write approval",
		)
		require.True(t, handled)
		require.Empty(t, payload.Message.ID)
	})

	t.Run("capability blocked invalid payload blocks legacy caller", func(t *testing.T) {
		mgr := newInvalidPayloadManager(t)
		payload, handled := mgr.runRuntimeCapabilityBlockedMutationForTurn(
			nil,
			"session-invalid-capability",
			agentRuntimeTurnRef{TaskID: "task-invalid-capability", RunID: "run-invalid-capability", TurnID: "turn-invalid-capability"},
			"scan aggressively",
			"en",
			"safe_scan",
			"ports.scan.full",
			"full_port_scan",
			"requires advanced capability",
			"use ports.scan.safe instead",
		)
		require.True(t, handled)
		require.Empty(t, payload.Session.SessionID)
	})
}

func TestAgentLoopRecordUserMessageResultPayloadAdapterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	lastMessageAt := now.Add(-time.Hour)
	lastCompactedAt := now.Add(-30 * time.Minute)
	appliedAt := now.Add(-15 * time.Minute)
	session := agentSession{
		SessionID:      "session-record-result",
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now,
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "safe_scan",
		Status:         "running",
		Messages: []agentMessage{{
			ID:      "msg-record-result-user",
			Role:    "user",
			Content: "scan safely",
			ToolCalls: []agentToolCall{{
				ID:       "tool-record-result-message",
				Name:     "daemon.health",
				Status:   "succeeded",
				Result:   map[string]any{"source": "message"},
				Metadata: map[string]interface{}{"phase": "history"},
			}},
			SuggestedPrompts: []string{"continue"},
		}},
		ActiveTask: &agentTask{
			ID:        "task-record-result",
			Goal:      "inspect target",
			Kind:      "chat",
			Status:    "running",
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		},
		Todos: []agentTodo{{
			ID:            "todo-record-result",
			Title:         "collect evidence",
			Status:        "running",
			Priority:      1,
			ContextRefIDs: []string{"ctx-record-result"},
			UpdatedAt:     now,
		}},
		ToolCalls: []agentToolCall{{
			ID:     "tool-record-result-top",
			Name:   "workspace.files.list",
			Status: "succeeded",
			Result: map[string]any{"path": "."},
		}},
		ContextRefs: []agentContextRef{{
			ID:      "ctx-record-result",
			Kind:    "file",
			RefID:   "notes.md",
			Label:   "notes",
			Summary: "operator notes",
		}},
		PendingChanges: []agentWorkspaceChange{{
			ID:          "change-record-result",
			Path:        "notes.md",
			DiffPreview: "+hello",
			Status:      "pending",
			CreatedAt:   now.Add(-20 * time.Minute),
			UpdatedAt:   now.Add(-10 * time.Minute),
			AppliedAt:   &appliedAt,
		}},
		PendingAuthorization: &agentAuthorizationRequest{
			ID:             "auth-record-result",
			ActionName:     "workspace.files.write",
			Arguments:      map[string]any{"path": "notes.md"},
			RequiredLevel:  "workspace_write",
			Reason:         "write requires approval",
			OriginalPrompt: "scan safely",
			Status:         "pending",
			CreatedAt:      now.Add(-5 * time.Minute),
			UpdatedAt:      now,
		},
		ApprovalState: &approvalCanonicalState{
			ApprovalKind:  approvalKindAgentAuthorization,
			State:         approvalCanonicalStatePending,
			ApprovalID:    "auth-record-result",
			ActionName:    "workspace.files.write",
			RequiredLevel: "workspace_write",
			Reason:        "write requires approval",
		},
		MemorySummary: &agentMemorySummary{
			Summary:       "session memory",
			MessageCount:  4,
			LastMessageID: "msg-old",
			LastMessageAt: &lastMessageAt,
			UpdatedAt:     now,
		},
		NextAction: &agentNextAction{
			Type:            "continue",
			Reason:          "continue safely",
			SuggestedPrompt: "continue",
			AutoContinue:    true,
		},
		RunSummary: &agentSessionRunSummary{
			RunID:    "run-record-result",
			Status:   "succeeded",
			Findings: 1,
			Summary:  "run summary",
		},
		UIState: &agentUIState{
			State:  "approval",
			Phase:  "approval",
			Tone:   "warning",
			Source: "pending_authorization",
		},
	}
	resultPayload := daemonRecordUserMessageMutationResultPayload{
		Session:       session,
		UserMessage:   session.Messages[0],
		History:       cloneAgentMessages(session.Messages),
		MemorySummary: cloneAgentMemorySummary(session.MemorySummary),
		Compaction: &agentMemoryCompaction{
			Summary: &agentMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
			CompactedCount:   2,
			RetainedCount:    80,
			PreviousCount:    1,
			LastCompactedID:  "msg-old",
			LastCompactedAt:  &lastCompactedAt,
			Trigger:          "turn_count",
			TriggerThreshold: 40,
			OccurredAt:       now,
		},
		Mutated: true,
	}

	runtimePayload := agentLoopRecordUserMessageResultPayloadFromDaemon(resultPayload)
	expectedPayload := daemonRecordUserMessageMutationResultPayloadFromAgentLoop(runtimePayload)
	resultPayload.Session.Messages[0].Content = "mutated"
	resultPayload.Session.PendingAuthorization.Arguments["path"] = "mutated"
	roundTrip := daemonRecordUserMessageMutationResultPayloadFromAgentLoop(runtimePayload)

	require.Equal(t, expectedPayload, roundTrip)
	require.Equal(t, "scan safely", roundTrip.Session.Messages[0].Content)
	require.Equal(t, "notes.md", roundTrip.Session.PendingAuthorization.Arguments["path"])
	require.Equal(t, "compacted memory", roundTrip.Compaction.Summary.Summary)
	require.Equal(t, lastCompactedAt, *roundTrip.Compaction.LastCompactedAt)
	require.True(t, roundTrip.Mutated)

	converted, ok := daemonRecordUserMessageMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, converted)
}

func TestAgentLoopControlSessionResultPayloadAdapterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 8, 10, 30, 0, 0, time.UTC)
	lastMessageAt := now.Add(-time.Hour)
	lastCompactedAt := now.Add(-30 * time.Minute)
	resultPayload := daemonControlSessionMutationResultPayload{
		Session: agentSession{
			SessionID:      "session-control-result",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now,
			Operator:       "operator-1",
			Language:       "en",
			OperationLevel: "safe_scan",
			Status:         "paused",
			Messages: []agentMessage{{
				ID:      "msg-control-result-history",
				Role:    "agent",
				Content: "paused",
				ToolCalls: []agentToolCall{{
					ID:       "tool-control-result-history",
					Name:     "daemon.control",
					Status:   "succeeded",
					Result:   map[string]any{"status": "paused"},
					Metadata: map[string]any{"phase": "control"},
				}},
				SuggestedPrompts: []string{"continue"},
			}},
			ActiveTask: &agentTask{
				ID:        "task-control-result",
				Goal:      "pause safely",
				Kind:      "analysis",
				Status:    "blocked",
				CreatedAt: now.Add(-time.Hour),
				UpdatedAt: now,
			},
			MemorySummary: &agentMemorySummary{
				Summary:       "session memory",
				MessageCount:  3,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
			NextAction: &agentNextAction{
				Type:            "continue",
				Reason:          "operator pause",
				SuggestedPrompt: "resume later",
			},
		},
		Message: agentMessage{
			ID:      "msg-control-result",
			Role:    "agent",
			Content: "control message",
			ToolCalls: []agentToolCall{{
				ID:     "tool-control-result",
				Name:   "daemon.pause",
				Status: "succeeded",
				Result: map[string]any{"action": "pause"},
			}},
			SuggestedPrompts: []string{"resume"},
		},
		CancelJobID: "job-control-result",
		Compaction: &agentMemoryCompaction{
			Summary: &agentMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
			CompactedCount:  2,
			RetainedCount:   80,
			PreviousCount:   82,
			LastCompactedID: "msg-old",
			LastCompactedAt: &lastCompactedAt,
			Trigger:         "turn_count",
			OccurredAt:      now,
		},
		Mutated: true,
	}

	runtimePayload := agentLoopControlSessionResultPayloadFromDaemon(resultPayload)
	expectedPayload := daemonControlSessionMutationResultPayloadFromAgentLoop(runtimePayload)
	resultPayload.Session.Messages[0].Content = "mutated"
	resultPayload.Session.Messages[0].ToolCalls[0].Result["status"] = "mutated"
	resultPayload.Message.ToolCalls[0].Result["action"] = "mutated"
	roundTrip := daemonControlSessionMutationResultPayloadFromAgentLoop(runtimePayload)

	require.Equal(t, expectedPayload, roundTrip)
	require.Equal(t, "paused", roundTrip.Session.Messages[0].Content)
	require.Equal(t, "paused", roundTrip.Session.Messages[0].ToolCalls[0].Result["status"])
	require.Equal(t, "pause", roundTrip.Message.ToolCalls[0].Result["action"])
	require.Equal(t, "job-control-result", roundTrip.CancelJobID)
	require.Equal(t, "compacted memory", roundTrip.Compaction.Summary.Summary)
	require.Equal(t, lastCompactedAt, *roundTrip.Compaction.LastCompactedAt)
	require.True(t, roundTrip.Mutated)

	converted, ok := daemonControlSessionMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, converted)

	convertedPointer, ok := daemonControlSessionMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: &runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, convertedPointer)
}

func TestAgentLoopResolveAuthorizationResultPayloadAdapterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 8, 11, 30, 0, 0, time.UTC)
	lastMessageAt := now.Add(-time.Hour)
	lastCompactedAt := now.Add(-30 * time.Minute)
	resultPayload := daemonResolveAuthorizationMutationResultPayload{
		Session: agentSession{
			SessionID:      "session-auth-result",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now,
			Operator:       "operator-1",
			Language:       "en",
			OperationLevel: "safe_scan",
			Status:         "blocked",
			Messages: []agentMessage{{
				ID:      "msg-auth-result-history",
				Role:    "agent",
				Content: "Authorization denied",
				ToolCalls: []agentToolCall{{
					ID:       "tool-auth-result-history",
					Name:     "approval.resolved",
					Status:   "succeeded",
					Result:   map[string]any{"decision": "deny"},
					Metadata: map[string]any{"phase": "approval"},
				}},
			}},
			ActiveTask: &agentTask{
				ID:        "task-auth-result",
				Goal:      "authorize write",
				Kind:      "tool",
				Status:    "blocked",
				CreatedAt: now.Add(-time.Hour),
				UpdatedAt: now,
			},
			NextAction: &agentNextAction{
				Type:   "blocked",
				Reason: "not within scope",
			},
		},
		Pending: agentAuthorizationRequest{
			ID:             "auth-result",
			ActionName:     "workspace.files.write",
			Arguments:      map[string]any{"path": "README.md"},
			RequiredLevel:  "workspace_write",
			Policy:         "manual",
			Reason:         "workspace write requires approval",
			OriginalPrompt: "write README.md",
			Status:         "denied",
			CreatedAt:      now.Add(-time.Hour),
			UpdatedAt:      now,
		},
		Message: agentMessage{
			ID:      "msg-auth-result",
			Role:    "agent",
			Phase:   "step",
			Content: "Authorization denied",
			ToolCalls: []agentToolCall{{
				ID:     "tool-auth-result",
				Name:   "approval.resolved",
				Status: "succeeded",
				Result: map[string]any{"authorization_id": "auth-result"},
			}},
			SuggestedPrompts: []string{"revise scope"},
		},
		OperationLevel: "safe_scan",
		Compaction: &agentMemoryCompaction{
			Summary: &agentMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
			CompactedCount:  2,
			RetainedCount:   80,
			PreviousCount:   82,
			LastCompactedID: "msg-old",
			LastCompactedAt: &lastCompactedAt,
			Trigger:         "turn_count",
			OccurredAt:      now,
		},
		Mutated: true,
	}

	runtimePayload := agentLoopResolveAuthorizationResultPayloadFromDaemon(resultPayload)
	expectedPayload := daemonResolveAuthorizationMutationResultPayloadFromAgentLoop(runtimePayload)
	resultPayload.Session.Messages[0].ToolCalls[0].Result["decision"] = "mutated"
	resultPayload.Pending.Arguments["path"] = "mutated"
	resultPayload.Message.ToolCalls[0].Result["authorization_id"] = "mutated"
	roundTrip := daemonResolveAuthorizationMutationResultPayloadFromAgentLoop(runtimePayload)

	require.Equal(t, expectedPayload, roundTrip)
	require.Equal(t, "deny", roundTrip.Session.Messages[0].ToolCalls[0].Result["decision"])
	require.Equal(t, "README.md", roundTrip.Pending.Arguments["path"])
	require.Equal(t, "auth-result", roundTrip.Message.ToolCalls[0].Result["authorization_id"])
	require.Equal(t, "safe_scan", roundTrip.OperationLevel)
	require.Equal(t, "compacted memory", roundTrip.Compaction.Summary.Summary)
	require.Equal(t, lastCompactedAt, *roundTrip.Compaction.LastCompactedAt)
	require.True(t, roundTrip.Mutated)

	converted, ok := daemonResolveAuthorizationMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, converted)

	convertedPointer, ok := daemonResolveAuthorizationMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: &runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, convertedPointer)
}

func TestAgentLoopRequestAuthorizationResultPayloadAdapterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	resultPayload := daemonRequestAuthorizationMutationResultPayload{
		Session: agentSession{
			SessionID:      "session-request-auth-result",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now,
			Operator:       "operator-1",
			Language:       "en",
			OperationLevel: "read_only",
			Status:         "waiting_authorization",
			PendingAuthorization: &agentAuthorizationRequest{
				ID:            "auth-request-result",
				ActionName:    "workspace.files.write",
				Arguments:     map[string]any{"path": "README.md"},
				RequiredLevel: "workspace_write",
				Policy:        "manual",
				Status:        "pending",
				CreatedAt:     now.Add(-time.Minute),
				UpdatedAt:     now,
			},
			NextAction: &agentNextAction{
				Type:            "authorization_required",
				Reason:          "workspace write requires approval",
				SuggestedPrompt: "approve or deny",
			},
		},
		Authorization: agentAuthorizationRequest{
			ID:             "auth-request-result",
			ActionName:     "workspace.files.write",
			Arguments:      map[string]any{"path": "README.md"},
			RequiredLevel:  "workspace_write",
			Policy:         "manual",
			Reason:         "workspace write requires approval",
			OriginalPrompt: "write README.md",
			Status:         "pending",
			CreatedAt:      now.Add(-time.Minute),
			UpdatedAt:      now,
		},
		Message: agentMessage{
			ID:      "msg-request-auth-result",
			Role:    "agent",
			Phase:   "step",
			Content: "Authorization required",
			ToolCalls: []agentToolCall{{
				ID:     "tool-request-auth-result",
				Name:   "approval.requested",
				Status: "pending",
				Result: map[string]any{"authorization_id": "auth-request-result"},
			}},
			SuggestedPrompts: []string{"approve"},
		},
		Mutated: true,
	}

	runtimePayload := agentLoopRequestAuthorizationResultPayloadFromDaemon(resultPayload)
	expectedPayload := daemonRequestAuthorizationMutationResultPayloadFromAgentLoop(runtimePayload)
	resultPayload.Session.PendingAuthorization.Arguments["path"] = "mutated"
	resultPayload.Authorization.Arguments["path"] = "mutated"
	resultPayload.Message.ToolCalls[0].Result["authorization_id"] = "mutated"
	roundTrip := daemonRequestAuthorizationMutationResultPayloadFromAgentLoop(runtimePayload)

	require.Equal(t, expectedPayload, roundTrip)
	require.Equal(t, "README.md", roundTrip.Session.PendingAuthorization.Arguments["path"])
	require.Equal(t, "README.md", roundTrip.Authorization.Arguments["path"])
	require.Equal(t, "auth-request-result", roundTrip.Message.ToolCalls[0].Result["authorization_id"])
	require.Equal(t, "authorization_required", roundTrip.Session.NextAction.Type)
	require.True(t, roundTrip.Mutated)

	converted, ok := daemonRequestAuthorizationMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, converted)

	convertedPointer, ok := daemonRequestAuthorizationMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: &runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, convertedPointer)
}

func TestAgentLoopCapabilityBlockedResultPayloadAdapterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 30, 0, 0, time.UTC)
	resultPayload := daemonCapabilityBlockedMutationResultPayload{
		Session: agentSession{
			SessionID:      "session-capability-result",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now,
			Operator:       "operator-1",
			Language:       "en",
			OperationLevel: "safe_scan",
			Status:         "blocked",
			Todos: []agentTodo{{
				ID:            "todo-capability-result",
				Title:         "select safe capability",
				Status:        "blocked",
				ContextRefIDs: []string{"ctx-capability-result"},
				UpdatedAt:     now,
			}},
			NextAction: &agentNextAction{
				Type:            "blocked",
				Reason:          "capability mismatch",
				SuggestedPrompt: "use scan.web.safe",
			},
		},
		ActionName:      "scan.web.safe",
		IntentID:        "content_discovery",
		Reason:          "capability mismatch",
		SuggestedPrompt: "use scan.web.safe",
		Mutated:         true,
	}

	runtimePayload := agentLoopCapabilityBlockedResultPayloadFromDaemon(resultPayload)
	expectedPayload := daemonCapabilityBlockedMutationResultPayloadFromAgentLoop(runtimePayload)
	resultPayload.Session.Todos[0].ContextRefIDs[0] = "mutated"
	resultPayload.Session.NextAction.SuggestedPrompt = "mutated"
	roundTrip := daemonCapabilityBlockedMutationResultPayloadFromAgentLoop(runtimePayload)

	require.Equal(t, expectedPayload, roundTrip)
	require.Equal(t, "ctx-capability-result", roundTrip.Session.Todos[0].ContextRefIDs[0])
	require.Equal(t, "use scan.web.safe", roundTrip.Session.NextAction.SuggestedPrompt)
	require.Equal(t, "scan.web.safe", roundTrip.ActionName)
	require.Equal(t, "content_discovery", roundTrip.IntentID)
	require.Equal(t, "capability mismatch", roundTrip.Reason)
	require.Equal(t, "use scan.web.safe", roundTrip.SuggestedPrompt)
	require.True(t, roundTrip.Mutated)

	converted, ok := daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, converted)

	convertedPointer, ok := daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: &runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, convertedPointer)
}

func TestAgentLoopAppendMessagesResultPayloadAdapterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 8, 13, 30, 0, 0, time.UTC)
	lastMessageAt := now.Add(-time.Hour)
	lastCompactedAt := now.Add(-30 * time.Minute)
	resultPayload := daemonAppendMessagesMutationResultPayload{
		Session: agentSession{
			SessionID:      "session-append-result",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now,
			Operator:       "operator-1",
			Language:       "en",
			OperationLevel: "safe_scan",
			Status:         "running",
			Messages: []agentMessage{{
				ID:      "msg-append-result",
				Role:    "agent",
				Content: "appended",
				ToolCalls: []agentToolCall{{
					ID:     "tool-append-result",
					Name:   "workspace.files.read",
					Status: "succeeded",
					Result: map[string]any{"path": "README.md"},
				}},
			}},
			MemorySummary: &agentMemorySummary{
				Summary:       "session memory",
				MessageCount:  3,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
		},
		Compaction: &agentMemoryCompaction{
			Summary: &agentMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
			CompactedCount:  2,
			RetainedCount:   80,
			PreviousCount:   82,
			LastCompactedID: "msg-old",
			LastCompactedAt: &lastCompactedAt,
			Trigger:         "turn_count",
			OccurredAt:      now,
		},
		Mutated: true,
	}

	runtimePayload := agentLoopAppendMessagesResultPayloadFromDaemon(resultPayload)
	expectedPayload := daemonAppendMessagesMutationResultPayloadFromAgentLoop(runtimePayload)
	resultPayload.Session.Messages[0].ToolCalls[0].Result["path"] = "mutated"
	resultPayload.Compaction.Summary.Summary = "mutated"
	roundTrip := daemonAppendMessagesMutationResultPayloadFromAgentLoop(runtimePayload)

	require.Equal(t, expectedPayload, roundTrip)
	require.Equal(t, "README.md", roundTrip.Session.Messages[0].ToolCalls[0].Result["path"])
	require.Equal(t, "compacted memory", roundTrip.Compaction.Summary.Summary)
	require.Equal(t, lastCompactedAt, *roundTrip.Compaction.LastCompactedAt)
	require.True(t, roundTrip.Mutated)

	converted, ok := daemonAppendMessagesMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, converted)

	convertedPointer, ok := daemonAppendMessagesMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: &runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, convertedPointer)
}

func TestDaemonSessionMutatorAppendMessagesReturnsAgentLoopResultPayload(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	mutator := daemonSessionMutator{manager: mgr}

	result, err := mutator.MutateSession(context.Background(), runtime.SessionMutationRequest{
		Turn: runtime.TurnContext{
			SessionID: sessionID,
			Metadata: map[string]any{
				"kind": string(runtime.SessionMutationAppendMessages),
			},
		},
		Kind: runtime.SessionMutationAppendMessages,
		Payload: runtime.AgentLoopAppendMessagesPayload{
			Messages: []runtime.AgentLoopMessage{{
				ID:      "msg-append-runtime-result",
				Role:    "agent",
				Content: "append through daemon mutator",
			}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, runtime.TurnResultFinished, result.Status)
	payload, ok := result.Payload.(runtime.AgentLoopAppendMessagesResultPayload)
	require.True(t, ok, "payload type = %T", result.Payload)
	require.True(t, payload.Mutated)
	require.Equal(t, sessionID, payload.Session.SessionID)
	require.Len(t, payload.Session.Messages, len(created.Session.Messages)+1)
	require.Equal(t, "msg-append-runtime-result", payload.Session.Messages[len(payload.Session.Messages)-1].ID)
}

func TestDaemonSessionMutatorReturnsAgentLoopResultPayloads(t *testing.T) {
	testCases := []struct {
		name   string
		build  func(t *testing.T, mgr *agentManager, sessionID string) runtime.SessionMutationRequest
		assert func(t *testing.T, sessionID string, result runtime.SessionMutationResult)
	}{
		{
			name: "append_messages",
			build: func(t *testing.T, _ *agentManager, sessionID string) runtime.SessionMutationRequest {
				return runtime.SessionMutationRequest{
					Turn: runtime.TurnContext{
						SessionID:      sessionID,
						OperationLevel: "safe_scan",
					},
					Kind: runtime.SessionMutationAppendMessages,
					Payload: runtime.AgentLoopAppendMessagesPayload{
						Messages: []runtime.AgentLoopMessage{{
							ID:      "msg-mutator-runtime-append",
							Role:    "agent",
							Content: "append through runtime result dto",
						}},
					},
				}
			},
			assert: func(t *testing.T, sessionID string, result runtime.SessionMutationResult) {
				payload, ok := result.Payload.(runtime.AgentLoopAppendMessagesResultPayload)
				require.True(t, ok, "payload type = %T", result.Payload)
				require.Equal(t, sessionID, payload.Session.SessionID)
				require.True(t, payload.Mutated)
			},
		},
		{
			name: "record_user_message",
			build: func(t *testing.T, _ *agentManager, sessionID string) runtime.SessionMutationRequest {
				return runtime.SessionMutationRequest{
					Turn: runtime.TurnContext{
						SessionID:      sessionID,
						Prompt:         "record through runtime result dto",
						OperationLevel: "safe_scan",
					},
					Kind: runtime.SessionMutationRecordUserMessage,
					Payload: runtime.AgentLoopRecordUserMessagePayload{
						Operator: "operator-1",
						Language: "en",
					},
				}
			},
			assert: func(t *testing.T, sessionID string, result runtime.SessionMutationResult) {
				payload, ok := result.Payload.(runtime.AgentLoopRecordUserMessageResultPayload)
				require.True(t, ok, "payload type = %T", result.Payload)
				require.Equal(t, sessionID, payload.Session.SessionID)
				require.Equal(t, "user", payload.UserMessage.Role)
				require.Equal(t, "record through runtime result dto", payload.UserMessage.Content)
				require.True(t, payload.Mutated)
			},
		},
		{
			name: "request_authorization",
			build: func(t *testing.T, _ *agentManager, sessionID string) runtime.SessionMutationRequest {
				return runtime.SessionMutationRequest{
					Turn: runtime.TurnContext{
						SessionID:      sessionID,
						Prompt:         "request authorization through runtime result dto",
						OperationLevel: "read_only",
					},
					Kind: runtime.SessionMutationRequestAuthorization,
					Payload: runtime.AgentLoopAuthorizationRequestPayload{
						Language:      "en",
						Action:        runtime.AgentLoopAction{Name: "scan.web.safe", Arguments: map[string]any{"target": "https://example.com"}},
						ActionName:    "scan.web.safe",
						RequiredLevel: "safe_scan",
						Policy:        "operation_level_required",
						Reason:        "safe scan requires authorization",
					},
				}
			},
			assert: func(t *testing.T, sessionID string, result runtime.SessionMutationResult) {
				payload, ok := result.Payload.(runtime.AgentLoopRequestAuthorizationResultPayload)
				require.True(t, ok, "payload type = %T", result.Payload)
				require.Equal(t, sessionID, payload.Session.SessionID)
				require.Equal(t, "scan.web.safe", payload.Authorization.ActionName)
				require.Equal(t, "safe_scan", payload.Authorization.RequiredLevel)
				require.True(t, payload.Mutated)
			},
		},
		{
			name: "capability_blocked",
			build: func(t *testing.T, _ *agentManager, sessionID string) runtime.SessionMutationRequest {
				return runtime.SessionMutationRequest{
					Turn: runtime.TurnContext{
						SessionID:      sessionID,
						Prompt:         "block mismatched capability through runtime result dto",
						OperationLevel: "safe_scan",
					},
					Kind: runtime.SessionMutationCapabilityBlocked,
					Payload: runtime.AgentLoopCapabilityBlockedPayload{
						Language:        "en",
						ActionName:      "scan.web.safe",
						IntentID:        agentIntentContentDiscovery,
						Reason:          "requested capability is outside the available safe action",
						SuggestedPrompt: "use the safe scan action",
					},
				}
			},
			assert: func(t *testing.T, sessionID string, result runtime.SessionMutationResult) {
				payload, ok := result.Payload.(runtime.AgentLoopCapabilityBlockedResultPayload)
				require.True(t, ok, "payload type = %T", result.Payload)
				require.Equal(t, sessionID, payload.Session.SessionID)
				require.Equal(t, "scan.web.safe", payload.ActionName)
				require.Equal(t, agentIntentContentDiscovery, payload.IntentID)
				require.True(t, payload.Mutated)
			},
		},
		{
			name: "control_session",
			build: func(t *testing.T, _ *agentManager, sessionID string) runtime.SessionMutationRequest {
				return runtime.SessionMutationRequest{
					Turn: runtime.TurnContext{
						SessionID:      sessionID,
						Prompt:         "resume later",
						OperationLevel: "safe_scan",
					},
					Kind: runtime.SessionMutationControlSession,
					Payload: runtime.AgentLoopControlSessionPayload{
						Action:   "pause",
						Reason:   "operator pause",
						Operator: "operator-1",
						Language: "en",
					},
				}
			},
			assert: func(t *testing.T, sessionID string, result runtime.SessionMutationResult) {
				payload, ok := result.Payload.(runtime.AgentLoopControlSessionResultPayload)
				require.True(t, ok, "payload type = %T", result.Payload)
				require.Equal(t, sessionID, payload.Session.SessionID)
				require.Equal(t, "paused", payload.Session.Status)
				require.Equal(t, "agent", payload.Message.Role)
				require.True(t, payload.Mutated)
			},
		},
		{
			name: "resolve_authorization",
			build: func(t *testing.T, mgr *agentManager, sessionID string) runtime.SessionMutationRequest {
				t.Helper()
				pending := mgr.requestAgentAuthorizationLegacy(
					sessionID,
					"en",
					"resolve authorization through runtime result dto",
					agentPlannerAction{Name: "scan.web.safe", Arguments: map[string]any{"target": "https://example.com"}},
					"scan.web.safe",
					"safe_scan",
					"manual",
					"needs operator authorization",
				)
				require.NotEmpty(t, pending.Authorization.ID)
				return runtime.SessionMutationRequest{
					Turn: runtime.TurnContext{
						SessionID:      sessionID,
						OperationLevel: "safe_scan",
					},
					Kind: runtime.SessionMutationResolveAuthorization,
					Payload: runtime.AgentLoopResolveAuthorizationPayload{
						Decision:               "deny",
						AuthorizationID:        pending.Authorization.ID,
						Reason:                 "not in scope",
						Operator:               "operator-1",
						Language:               "en",
						OperationLevelProvided: true,
					},
				}
			},
			assert: func(t *testing.T, sessionID string, result runtime.SessionMutationResult) {
				payload, ok := result.Payload.(runtime.AgentLoopResolveAuthorizationResultPayload)
				require.True(t, ok, "payload type = %T", result.Payload)
				require.Equal(t, sessionID, payload.Session.SessionID)
				require.Equal(t, "denied", payload.Pending.Status)
				require.Equal(t, "safe_scan", payload.OperationLevel)
				require.True(t, payload.Mutated)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newAgentManager(t.TempDir())
			created, err := mgr.CreateSession(createAgentSessionRequest{
				Operator:       "tester",
				Language:       "en",
				OperationLevel: "safe_scan",
			})
			require.NoError(t, err)
			sessionID := created.Session.SessionID
			mutator := daemonSessionMutator{manager: mgr}

			result, err := mutator.MutateSession(context.Background(), tc.build(t, mgr, sessionID))

			require.NoError(t, err)
			require.Equal(t, runtime.TurnResultFinished, result.Status)
			tc.assert(t, sessionID, result)
		})
	}
}

func TestAgentManagerRunRuntimeSessionMutationDispatchHelper(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	request := runtime.AgentLoopRecordUserMessageMutationRequest(
		"session-dispatch-helper",
		"scan safely",
		"safe_scan",
		"operator-1",
		"en",
		runtime.AgentLoopRecordUserMessageMutationMetadata("safe_scan"),
	)

	result, handled, err := mgr.runRuntimeSessionMutation(nil, request)
	require.NoError(t, err)
	require.False(t, handled)
	require.Equal(t, runtime.SessionMutationResult{}, result)

	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr.AttachEngine(eng)
	eng.AttachSessionMutator(nil)
	result, handled, err = mgr.runRuntimeSessionMutation(context.Background(), request)
	require.ErrorIs(t, err, runtime.ErrSessionMutatorNotAttached)
	require.True(t, handled)
	require.Equal(t, runtime.TurnResultBlocked, result.Status)
	require.Equal(t, runtime.ErrSessionMutatorNotAttached, result.Error)
	require.Equal(t, runtime.ErrSessionMutatorNotAttached.Error(), result.Data["error"])
	require.Equal(t, string(daemonSessionMutationCallerFallbackMutatorNotAttached), result.Data["blocked_reason"])

	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		require.NotNil(t, ctx)
		require.Equal(t, runtime.SessionMutationRecordUserMessage, req.Kind)
		require.Equal(t, "session-dispatch-helper", req.Turn.SessionID)
		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled",
			Payload: runtime.AgentLoopRecordUserMessageResultPayload{
				Session: runtime.AgentLoopSessionSnapshot{SessionID: req.Turn.SessionID},
				Mutated: true,
			},
		}, nil
	}))
	result, handled, err = mgr.runRuntimeSessionMutation(nil, request)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, runtime.TurnResultFinished, result.Status)
	require.Equal(t, "handled", result.Reason)

	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(context.Context, runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		return runtime.SessionMutationResult{Status: runtime.TurnResultBlocked, Reason: "blocked"}, nil
	}))
	result, handled, err = mgr.runRuntimeSessionMutation(context.Background(), request)
	require.NoError(t, err)
	require.False(t, handled)
	require.Equal(t, runtime.TurnResultBlocked, result.Status)
	reason, fallback := daemonSessionMutationCallerResultFallbackReason(result, err)
	require.True(t, fallback)
	require.Equal(t, daemonSessionMutationCallerFallbackBlocked, reason)
	require.True(t, daemonSessionMutationCallerFallbackOwnedByLegacyCaller(reason))

	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(context.Context, runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		return runtime.SessionMutationResult{}, runtime.ErrSessionMutatorNotAttached
	}))
	result, handled, err = mgr.runRuntimeSessionMutation(context.Background(), request)
	require.ErrorIs(t, err, runtime.ErrSessionMutatorNotAttached)
	require.True(t, handled)
	require.Equal(t, runtime.TurnResultBlocked, result.Status)
	require.Equal(t, runtime.ErrSessionMutatorNotAttached, result.Error)
	reason, fallback = daemonSessionMutationCallerResultFallbackReason(result, runtime.ErrSessionMutatorNotAttached)
	require.True(t, fallback)
	require.Equal(t, daemonSessionMutationCallerFallbackMutatorNotAttached, reason)

	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(context.Context, runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		return runtime.SessionMutationResult{Status: runtime.TurnResultBlocked, Error: runtime.ErrSessionMutatorNotAttached}, nil
	}))
	result, handled, err = mgr.runRuntimeSessionMutation(context.Background(), request)
	require.ErrorIs(t, err, runtime.ErrSessionMutatorNotAttached)
	require.True(t, handled)
	require.Equal(t, runtime.TurnResultBlocked, result.Status)
	require.Equal(t, runtime.ErrSessionMutatorNotAttached, result.Error)
	reason, fallback = daemonSessionMutationCallerResultFallbackReason(result, err)
	require.True(t, fallback)
	require.Equal(t, daemonSessionMutationCallerFallbackMutatorNotAttached, reason)

	sentinel := errors.New("synthetic dispatch failure")
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(context.Context, runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		return runtime.SessionMutationResult{Status: runtime.TurnResultFailed, Error: sentinel}, sentinel
	}))
	result, handled, err = mgr.runRuntimeSessionMutation(context.Background(), request)
	require.ErrorIs(t, err, sentinel)
	require.True(t, handled)
	require.Equal(t, sentinel, result.Error)
	_, fallback = daemonSessionMutationCallerResultFallbackReason(result, err)
	require.False(t, fallback)
}

func TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "zh",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	initialMessages := len(created.Session.Messages)
	message := agentMessage{
		ID:        "msg-session-mutation",
		Role:      "agent",
		Content:   "mutation handled by runtime seam",
		Timestamp: time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
	}
	plan := &agentPlannerResponse{
		Answer: "planner state",
		Task: &agentPlannerTask{
			Goal:   "keep session mutation behind engine",
			Status: "running",
		},
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Metadata["kind"])
		require.Equal(t, 1, req.Metadata["message_count"])
		require.Equal(t, true, req.Metadata["planner_apply"])

		payload, ok := req.Payload.(runtime.AgentLoopAppendMessagesPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(plan), payload.Plan)
		require.Len(t, payload.Messages, 1)
		require.Equal(t, message.ID, payload.Messages[0].ID)
		require.Equal(t, message.Content, payload.Messages[0].Content)

		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled by test mutator",
			Data:   map[string]any{"mutator": "test"},
		}, nil
	}))

	mgr.appendMessagesWithPlanner(sessionID, plan, message)

	require.True(t, called)
	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, got.Messages, initialMessages, "handled runtime mutator should prevent legacy fallback")
}

func TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "zh",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	initialMessages := len(created.Session.Messages)
	turnRef := agentRuntimeTurnRef{
		TaskID: "task-append-turn",
		RunID:  "run-append-turn",
		TurnID: "turn-append-turn",
	}
	message := agentMessage{
		ID:        "msg-session-mutation-turn",
		Role:      "tool",
		Content:   "turn-aware append mutation",
		Timestamp: time.Date(2026, 6, 6, 12, 30, 0, 0, time.UTC),
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, turnRef.TaskID, req.Turn.TaskID)
		require.Equal(t, turnRef.RunID, req.Turn.RunID)
		require.Equal(t, turnRef.TurnID, req.Turn.TurnID)
		require.Equal(t, "continue approved run", req.Turn.Prompt)
		require.Equal(t, "planner_loop", req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Turn.Metadata["kind"])
		require.Equal(t, 1, req.Turn.Metadata["message_count"])
		require.Equal(t, false, req.Turn.Metadata["planner_apply"])
		require.Equal(t, "safe_scan", req.Turn.Metadata["operation_level"])
		require.Equal(t, "session_mutation", req.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Metadata["kind"])
		require.Equal(t, 1, req.Metadata["message_count"])
		require.Equal(t, false, req.Metadata["planner_apply"])

		payload, ok := req.Payload.(runtime.AgentLoopAppendMessagesPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Nil(t, payload.Plan)
		require.Len(t, payload.Messages, 1)
		require.Equal(t, message.ID, payload.Messages[0].ID)
		require.Equal(t, message.Content, payload.Messages[0].Content)

		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled by test mutator",
			Data:   map[string]any{"mutator": "test"},
		}, nil
	}))

	mgr.appendMessagesForTurn(context.Background(), sessionID, turnRef, "continue approved run", "planner_loop", "safe_scan", message)

	require.True(t, called)
	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, got.Messages, initialMessages, "handled runtime mutator should prevent legacy fallback")
}

func TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan(t *testing.T) {
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
	initialMessages := len(created.Session.Messages)
	turnRef := agentRuntimeTurnRef{
		TaskID: "task-planner-append",
		RunID:  "run-planner-append",
		TurnID: "turn-planner-append",
	}
	message := agentMessage{
		ID:        "msg-planner-append-turn",
		Role:      "agent",
		Content:   "planner append mutation",
		Timestamp: time.Date(2026, 6, 6, 12, 45, 0, 0, time.UTC),
	}
	plan := &agentPlannerResponse{
		Phase:  "step_replan",
		Answer: "planner state updated",
		Task: &agentPlannerTask{
			Goal:   "persist planner state through runtime seam",
			Status: "running",
		},
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, turnRef.TaskID, req.Turn.TaskID)
		require.Equal(t, turnRef.RunID, req.Turn.RunID)
		require.Equal(t, turnRef.TurnID, req.Turn.TurnID)
		require.Equal(t, "summarize executed tools", req.Turn.Prompt)
		require.Equal(t, string(runtime.LLMStageToolSynthesis), req.Turn.Origin)
		require.Equal(t, "read_only", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Turn.Metadata["kind"])
		require.Equal(t, 1, req.Turn.Metadata["message_count"])
		require.Equal(t, true, req.Turn.Metadata["planner_apply"])
		require.Equal(t, "read_only", req.Turn.Metadata["operation_level"])
		require.Equal(t, "session_mutation", req.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Metadata["kind"])
		require.Equal(t, 1, req.Metadata["message_count"])
		require.Equal(t, true, req.Metadata["planner_apply"])

		payload, ok := req.Payload.(runtime.AgentLoopAppendMessagesPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(plan), payload.Plan)
		require.Len(t, payload.Messages, 1)
		require.Equal(t, message.ID, payload.Messages[0].ID)
		require.Equal(t, message.Content, payload.Messages[0].Content)

		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled by test mutator",
			Data:   map[string]any{"mutator": "test"},
		}, nil
	}))

	mgr.appendMessagesWithPlannerForTurn(context.Background(), sessionID, turnRef, "summarize executed tools", string(runtime.LLMStageToolSynthesis), "read_only", plan, message)

	require.True(t, called)
	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, got.Messages, initialMessages, "handled runtime mutator should prevent legacy fallback")
}

func TestAgentManagerAppendMessagesWithPlannerForTurnSessionMutatorNotAttachedBlocksLegacyFallback(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*agentManager, *runtime.Engine)
		wantAppended  bool
		wantEngineNil bool
	}{
		{
			name:          "no_engine_legacy_fallback_retained",
			wantAppended:  true,
			wantEngineNil: true,
		},
		{
			name: "attached_engine_missing_mutator_blocks_legacy_append",
			setup: func(_ *agentManager, eng *runtime.Engine) {
				eng.AttachSessionMutator(nil)
			},
		},
		{
			name: "detached_mutator_error_blocks_legacy_append",
			setup: func(_ *agentManager, eng *runtime.Engine) {
				eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(context.Context, runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
					return runtime.SessionMutationResult{}, runtime.ErrSessionMutatorNotAttached
				}))
			},
		},
		{
			name: "detached_mutator_result_error_blocks_legacy_append",
			setup: func(_ *agentManager, eng *runtime.Engine) {
				eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(context.Context, runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
					return runtime.SessionMutationResult{
						Status: runtime.TurnResultBlocked,
						Error:  runtime.ErrSessionMutatorNotAttached,
					}, nil
				}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newAgentManager(t.TempDir())
			var eng *runtime.Engine
			if !tt.wantEngineNil {
				eng = runtime.NewEngine(runtime.NewMemStore())
				mgr.AttachEngine(eng)
			}

			created, err := mgr.CreateSession(createAgentSessionRequest{
				Operator:       "tester",
				Language:       "en",
				OperationLevel: "read_only",
			})
			require.NoError(t, err)
			sessionID := created.Session.SessionID
			initialMessages := len(created.Session.Messages)
			if tt.setup != nil {
				tt.setup(mgr, eng)
			}

			mgr.appendMessagesWithPlannerForTurn(
				context.Background(),
				sessionID,
				agentRuntimeTurnRef{TaskID: "task-mutator-not-attached", RunID: "run-mutator-not-attached", TurnID: "turn-mutator-not-attached"},
				"continue shared cycle",
				string(runtime.AgentLoopModePlannerActionReplanCycle),
				"read_only",
				nil,
				agentMessage{
					ID:        "msg-mutator-not-attached-" + tt.name,
					Role:      "agent",
					Content:   "should only append through allowed fallback",
					Timestamp: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
				},
			)

			got, err := mgr.GetSession(context.Background(), sessionID)
			require.NoError(t, err)
			if tt.wantAppended {
				require.Len(t, got.Messages, initialMessages+1)
				return
			}
			require.Len(t, got.Messages, initialMessages, "attached-engine missing/detached SessionMutator must not legacy-append")
		})
	}
}

func TestAgentManagerAppendMessagesWithPlannerForTurnSessionMutatorBlockedOrHardErrorBlocksLegacyFallback(t *testing.T) {
	hardErr := errors.New("append mutator hard error")
	tests := []struct {
		name          string
		setup         func(*testing.T, *runtime.Engine) *bool
		wantAppended  bool
		wantEngineNil bool
	}{
		{
			name:          "no_engine_legacy_fallback_retained",
			wantAppended:  true,
			wantEngineNil: true,
		},
		{
			name: "attached_mutator_blocked_result_blocks_legacy_append",
			setup: func(t *testing.T, eng *runtime.Engine) *bool {
				t.Helper()
				called := false
				eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
					called = true
					require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
					return runtime.SessionMutationResult{
						Status: runtime.TurnResultBlocked,
						Reason: "append blocked by test mutator",
						Data:   map[string]any{"blocked": true},
					}, nil
				}))
				return &called
			},
		},
		{
			name: "attached_mutator_hard_error_blocks_legacy_append",
			setup: func(t *testing.T, eng *runtime.Engine) *bool {
				t.Helper()
				called := false
				eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
					called = true
					require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
					return runtime.SessionMutationResult{
						Status: runtime.TurnResultFailed,
						Error:  hardErr,
					}, hardErr
				}))
				return &called
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newAgentManager(t.TempDir())
			var eng *runtime.Engine
			if !tt.wantEngineNil {
				eng = runtime.NewEngine(runtime.NewMemStore())
				mgr.AttachEngine(eng)
			}

			created, err := mgr.CreateSession(createAgentSessionRequest{
				Operator:       "tester",
				Language:       "en",
				OperationLevel: "read_only",
			})
			require.NoError(t, err)
			sessionID := created.Session.SessionID
			initialMessages := len(created.Session.Messages)

			var called *bool
			if tt.setup != nil {
				called = tt.setup(t, eng)
			}

			mgr.appendMessagesWithPlannerForTurn(
				context.Background(),
				sessionID,
				agentRuntimeTurnRef{TaskID: "task-blocked-error", RunID: "run-blocked-error", TurnID: "turn-blocked-error"},
				"continue shared cycle",
				string(runtime.AgentLoopModePlannerActionReplanCycle),
				"read_only",
				nil,
				agentMessage{
					ID:        "msg-blocked-error-" + tt.name,
					Role:      "agent",
					Content:   "should only append through no-engine fallback",
					Timestamp: time.Date(2026, 6, 9, 10, 15, 0, 0, time.UTC),
				},
			)

			if called != nil {
				require.True(t, *called)
			}
			got, err := mgr.GetSession(context.Background(), sessionID)
			require.NoError(t, err)
			if tt.wantAppended {
				require.Len(t, got.Messages, initialMessages+1)
				return
			}
			require.Len(t, got.Messages, initialMessages, "attached mutator blocked/hard-error egress must not legacy-append")
		})
	}
}

func TestAgentManagerRequestAuthorizationOptionalMutationFailureBlocksLegacyFallback(t *testing.T) {
	hardErr := errors.New("request authorization mutator hard error")
	tests := []struct {
		name          string
		setup         func(*testing.T, *runtime.Engine) *bool
		wantMutated   bool
		wantEngineNil bool
	}{
		{
			name:          "no_engine_legacy_fallback_retained",
			wantMutated:   true,
			wantEngineNil: true,
		},
		{
			name: "attached_mutator_blocked_result_blocks_legacy_authorization",
			setup: func(t *testing.T, eng *runtime.Engine) *bool {
				t.Helper()
				called := false
				eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
					called = true
					require.Equal(t, runtime.SessionMutationRequestAuthorization, req.Kind)
					return runtime.SessionMutationResult{
						Status: runtime.TurnResultBlocked,
						Reason: "request authorization blocked by test mutator",
						Data:   map[string]any{"blocked": true},
					}, nil
				}))
				return &called
			},
		},
		{
			name: "attached_mutator_hard_error_blocks_legacy_authorization",
			setup: func(t *testing.T, eng *runtime.Engine) *bool {
				t.Helper()
				called := false
				eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
					called = true
					require.Equal(t, runtime.SessionMutationRequestAuthorization, req.Kind)
					return runtime.SessionMutationResult{
						Status: runtime.TurnResultFailed,
						Error:  hardErr,
					}, hardErr
				}))
				return &called
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newAgentManager(t.TempDir())
			var eng *runtime.Engine
			if !tt.wantEngineNil {
				eng = runtime.NewEngine(runtime.NewMemStore())
				mgr.AttachEngine(eng)
			}

			created, err := mgr.CreateSession(createAgentSessionRequest{
				Operator:       "tester",
				Language:       "en",
				OperationLevel: "read_only",
			})
			require.NoError(t, err)
			sessionID := created.Session.SessionID
			now := time.Date(2026, 6, 9, 10, 30, 0, 0, time.UTC)
			mgr.mu.Lock()
			session := mgr.sessions[sessionID]
			session.Status = "running"
			session.ActiveTask = &agentTask{
				ID:        "task-request-authorization-fallback",
				Goal:      "request authorization through optional mutation",
				Kind:      "analysis",
				Status:    "running",
				CreatedAt: now,
				UpdatedAt: now,
			}
			session.NextAction = nil
			session.PendingAuthorization = nil
			mgr.mu.Unlock()

			var called *bool
			if tt.setup != nil {
				called = tt.setup(t, eng)
			}

			message := mgr.requestAgentAuthorizationForTurn(
				context.Background(),
				sessionID,
				agentRuntimeTurnRef{TaskID: "task-request-authorization-fallback", RunID: "run-request-authorization-fallback", TurnID: "turn-request-authorization-fallback"},
				"en",
				"write a file",
				"read_only",
				runtime.AgentLoopAction{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.txt"}},
				"workspace.files.write",
				"workspace_write",
				"manual",
				"requires write approval",
			)

			if called != nil {
				require.True(t, *called)
			}
			got, err := mgr.GetSession(context.Background(), sessionID)
			require.NoError(t, err)
			if tt.wantMutated {
				require.NotEmpty(t, message.ID)
				require.Equal(t, "waiting_authorization", got.Status)
				require.NotNil(t, got.PendingAuthorization)
				require.Equal(t, "workspace.files.write", got.PendingAuthorization.ActionName)
				require.Equal(t, "workspace_write", got.PendingAuthorization.RequiredLevel)
				require.NotNil(t, got.NextAction)
				require.Equal(t, "authorization_required", got.NextAction.Type)
				require.NotNil(t, got.ActiveTask)
				require.Equal(t, "blocked", got.ActiveTask.Status)
				return
			}
			require.Empty(t, message.ID)
			require.Equal(t, "running", got.Status, "attached mutator failure must not legacy-write waiting_authorization")
			require.Nil(t, got.PendingAuthorization)
			require.Nil(t, got.NextAction)
			require.NotNil(t, got.ActiveTask)
			require.Equal(t, "running", got.ActiveTask.Status)
		})
	}
}

func TestAgentManagerCapabilityBlockedOptionalMutationFailureBlocksLegacyFallback(t *testing.T) {
	hardErr := errors.New("capability blocked mutator hard error")
	tests := []struct {
		name          string
		setup         func(*testing.T, *runtime.Engine) *bool
		wantMutated   bool
		wantEngineNil bool
	}{
		{
			name:          "no_engine_legacy_fallback_retained",
			wantMutated:   true,
			wantEngineNil: true,
		},
		{
			name: "attached_mutator_blocked_result_blocks_legacy_capability_blocked",
			setup: func(t *testing.T, eng *runtime.Engine) *bool {
				t.Helper()
				called := false
				eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
					called = true
					require.Equal(t, runtime.SessionMutationCapabilityBlocked, req.Kind)
					return runtime.SessionMutationResult{
						Status: runtime.TurnResultBlocked,
						Reason: "capability blocked mutation blocked by test mutator",
						Data:   map[string]any{"blocked": true},
					}, nil
				}))
				return &called
			},
		},
		{
			name: "attached_mutator_hard_error_blocks_legacy_capability_blocked",
			setup: func(t *testing.T, eng *runtime.Engine) *bool {
				t.Helper()
				called := false
				eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
					called = true
					require.Equal(t, runtime.SessionMutationCapabilityBlocked, req.Kind)
					return runtime.SessionMutationResult{
						Status: runtime.TurnResultFailed,
						Error:  hardErr,
					}, hardErr
				}))
				return &called
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newAgentManager(t.TempDir())
			var eng *runtime.Engine
			if !tt.wantEngineNil {
				eng = runtime.NewEngine(runtime.NewMemStore())
				mgr.AttachEngine(eng)
			}

			created, err := mgr.CreateSession(createAgentSessionRequest{
				Operator:       "tester",
				Language:       "en",
				OperationLevel: "safe_scan",
			})
			require.NoError(t, err)
			sessionID := created.Session.SessionID
			now := time.Date(2026, 6, 9, 10, 45, 0, 0, time.UTC)
			mgr.mu.Lock()
			session := mgr.sessions[sessionID]
			session.Status = "running"
			session.ActiveTask = &agentTask{
				ID:        "task-capability-blocked-fallback",
				Goal:      "validate capability blocked optional mutation",
				Kind:      "analysis",
				Status:    "running",
				CreatedAt: now,
				UpdatedAt: now,
			}
			session.Todos = []agentTodo{
				{ID: "todo-capability-blocked-fallback", Title: "validate capability gate", Status: "running", Priority: 1, UpdatedAt: now},
			}
			session.NextAction = nil
			mgr.mu.Unlock()

			var called *bool
			if tt.setup != nil {
				called = tt.setup(t, eng)
			}

			mgr.markAgentCapabilityBlockedForTurn(
				context.Background(),
				sessionID,
				agentRuntimeTurnRef{TaskID: "task-capability-blocked-fallback", RunID: "run-capability-blocked-fallback", TurnID: "turn-capability-blocked-fallback"},
				"scan aggressively",
				"en",
				"safe_scan",
				"ports.scan.full",
				"full_port_scan",
				"requires advanced capability",
				"use ports.scan.safe instead",
			)

			if called != nil {
				require.True(t, *called)
			}
			got, err := mgr.GetSession(context.Background(), sessionID)
			require.NoError(t, err)
			if tt.wantMutated {
				require.Equal(t, "blocked", got.Status)
				require.NotNil(t, got.NextAction)
				require.Equal(t, "blocked", got.NextAction.Type)
				require.Equal(t, "requires advanced capability", got.NextAction.Reason)
				require.Equal(t, "use ports.scan.safe instead", got.NextAction.SuggestedPrompt)
				require.NotNil(t, got.ActiveTask)
				require.Equal(t, "blocked", got.ActiveTask.Status)
				require.Len(t, got.Todos, 1)
				require.Equal(t, "blocked", got.Todos[0].Status)
				return
			}
			require.Equal(t, "running", got.Status, "attached mutator failure must not legacy-write blocked state")
			require.Nil(t, got.NextAction)
			require.NotNil(t, got.ActiveTask)
			require.Equal(t, "running", got.ActiveTask.Status)
			require.Len(t, got.Todos, 1)
			require.Equal(t, "running", got.Todos[0].Status)
		})
	}
}

func TestAgentManagerAppendMessagesForTurnCompactionUsesTurnRef(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
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
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Messages = nil
	for i := 0; i < agentSessionMaxMessages+4; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		session.Messages = append(session.Messages, agentMessage{
			ID:        fmt.Sprintf("msg-append-compaction-%03d", i),
			Role:      role,
			Content:   fmt.Sprintf("historical message %03d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	mgr.mu.Unlock()

	turnRef := agentRuntimeTurnRef{
		TaskID: "task-append-compaction",
		RunID:  "run-append-compaction",
		TurnID: "turn-append-compaction",
	}
	message := agentMessage{
		ID:        "msg-append-compaction-new",
		Role:      "agent",
		Content:   "new message triggers compaction",
		Timestamp: now.Add(2 * time.Hour),
	}
	mgr.appendMessagesForTurn(context.Background(), sessionID, turnRef, "compact with turn", "planner_loop", "safe_scan", message)

	var compacted *runtime.Event
	for _, ev := range store.Events() {
		if ev.Type == "memory_compacted" {
			event := ev
			compacted = &event
			break
		}
	}
	require.NotNil(t, compacted, "expected memory_compacted event")
	require.Equal(t, turnRef.TaskID, compacted.TaskID)
	require.Equal(t, turnRef.RunID, compacted.RunID)
	require.Equal(t, turnRef.TurnID, compacted.TurnID)
	require.Equal(t, sessionID, compacted.Data["session_id"])
	require.Greater(t, compacted.Data["compacted_count"], 0)
}

func TestAgentManagerControlSessionCompactionUsesMutationTurnRef(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 13, 15, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.Messages = nil
	for i := 0; i < agentSessionMaxMessages+4; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		session.Messages = append(session.Messages, agentMessage{
			ID:        fmt.Sprintf("msg-control-compaction-%03d", i),
			Role:      role,
			Content:   fmt.Sprintf("control history %03d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	session.ActiveTask = &agentTask{
		ID:          "task-control-compaction",
		Goal:        "pause with memory compaction",
		Kind:        "analysis",
		Status:      "running",
		ActiveRunID: "run-control-compaction",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.Todos = []agentTodo{
		{ID: "todo-control-compaction", Title: "pause session", Status: "running", Priority: 1, UpdatedAt: now},
	}
	mgr.mu.Unlock()

	_, err = mgr.ControlSession(context.Background(), sessionID, agentControlRequest{
		Action:         "pause",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "safe_scan",
		Reason:         "operator pause",
	}, routerServices{})
	require.NoError(t, err)

	var compacted *runtime.Event
	for _, ev := range store.Events() {
		if ev.Type == "memory_compacted" {
			event := ev
			compacted = &event
			break
		}
	}
	require.NotNil(t, compacted, "expected memory_compacted event")
	require.Equal(t, "task-control-compaction", compacted.TaskID)
	require.Equal(t, "run-control-compaction", compacted.RunID)
	require.Equal(t, "turn_pause", compacted.TurnID)
	require.Equal(t, sessionID, compacted.Data["session_id"])
	require.Greater(t, compacted.Data["compacted_count"], 0)
}

func TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator(t *testing.T) {
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
	initialMessages := len(created.Session.Messages)

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationRecordUserMessage, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "scan safely", req.Turn.Prompt)
		require.Equal(t, "user", req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, string(runtime.SessionMutationRecordUserMessage), req.Metadata["kind"])

		payload, ok := req.Payload.(runtime.AgentLoopRecordUserMessagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "en", payload.Language)

		resultPayload, err := mgr.recordUserMessageLegacy(req.Turn.SessionID, req.Turn.Prompt, payload.Operator, payload.Language, req.Turn.OperationLevel)
		require.NoError(t, err)
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "handled by test mutator",
			Payload: agentLoopRecordUserMessageResultPayloadFromDaemon(resultPayload),
			Data:    map[string]any{"mutator": "test"},
		}, nil
	}))

	result, handled, err := mgr.runRuntimeRecordUserMessageMutation(context.Background(), sessionID, "scan safely", "operator-1", "en", "safe_scan")

	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, called)
	require.Equal(t, sessionID, result.Session.SessionID)
	require.Equal(t, "operator-1", result.Session.Operator)
	require.Equal(t, "safe_scan", result.Session.OperationLevel)
	require.Equal(t, "user", result.UserMessage.Role)
	require.Equal(t, "scan safely", result.UserMessage.Content)
	require.Len(t, result.History, initialMessages+1)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, got.Messages, initialMessages+1)
	require.Equal(t, "scan safely", got.Messages[len(got.Messages)-1].Content)
	require.NotNil(t, got.ActiveTask)
}

func TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator(t *testing.T) {
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
	turnRef := agentRuntimeTurnRef{
		TaskID: "task-request-auth",
		RunID:  "run-request-auth",
		TurnID: "turn-request-auth",
	}
	action := agentPlannerAction{
		Name:      "scan.web.safe",
		Arguments: map[string]any{"target": "https://example.com"},
	}

	records := []runtime.SessionMutationRequest{}
	legacy := daemonSessionMutator{manager: mgr}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		if req.Kind == runtime.SessionMutationRequestAuthorization {
			records = append(records, req)
		}
		return legacy.MutateSession(ctx, req)
	}))

	messages, toolCalls, authorizationRequired := mgr.executeAgentPlannerActionsWithToolServices(
		context.Background(),
		sessionID,
		turnRef,
		"scan https://example.com",
		"en",
		"tester",
		"read_only",
		testAgentLoopToolServices(),
		&agentPlannerResponse{Actions: []agentPlannerAction{action}},
		nil,
		"",
	)

	require.True(t, authorizationRequired)
	require.Empty(t, toolCalls)
	require.Len(t, messages, 1)
	require.Equal(t, "step", messages[0].Phase)
	require.Contains(t, messages[0].Content, "Authorization required")
	require.Len(t, records, 1)

	req := records[0]
	require.Equal(t, runtime.SessionMutationRequestAuthorization, req.Kind)
	require.Equal(t, sessionID, req.Turn.SessionID)
	require.Equal(t, turnRef.TaskID, req.Turn.TaskID)
	require.Equal(t, turnRef.RunID, req.Turn.RunID)
	require.Equal(t, turnRef.TurnID, req.Turn.TurnID)
	require.Equal(t, "scan https://example.com", req.Turn.Prompt)
	require.Equal(t, string(runtime.SessionMutationRequestAuthorization), req.Turn.Origin)
	require.Equal(t, "read_only", req.Turn.OperationLevel)
	require.Equal(t, string(runtime.SessionMutationRequestAuthorization), req.Metadata["kind"])
	require.Equal(t, "scan.web.safe", req.Metadata["action_name"])
	require.Equal(t, "safe_scan", req.Metadata["required_level"])
	require.Equal(t, "operation_level_required", req.Metadata["policy"])

	payload, ok := req.Payload.(runtime.AgentLoopAuthorizationRequestPayload)
	require.True(t, ok, "payload type = %T", req.Payload)
	require.Equal(t, "en", payload.Language)
	require.Equal(t, "scan.web.safe", payload.ActionName)
	require.Equal(t, "safe_scan", payload.RequiredLevel)
	require.Equal(t, "operation_level_required", payload.Policy)
	require.Equal(t, agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action), payload.Action)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "waiting_authorization", got.Status)
	require.NotNil(t, got.PendingAuthorization)
	require.Equal(t, "scan.web.safe", got.PendingAuthorization.ActionName)
	require.Equal(t, "safe_scan", got.PendingAuthorization.RequiredLevel)
	require.Equal(t, "operation_level_required", got.PendingAuthorization.Policy)
	require.Equal(t, "scan https://example.com", got.PendingAuthorization.OriginalPrompt)
	require.NotNil(t, got.NextAction)
	require.Equal(t, "authorization_required", got.NextAction.Type)
}

func TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator(t *testing.T) {
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
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:        "task-capability-blocked",
		Goal:      "run content discovery with an equivalent tool",
		Kind:      "content_discovery",
		Status:    "running",
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.Todos = []agentTodo{
		{ID: "todo-capability-running", Title: "select content discovery tool", Status: "running", Priority: 1, UpdatedAt: now},
		{ID: "todo-capability-pending", Title: "execute bounded discovery", Status: "pending", Priority: 2, UpdatedAt: now},
		{ID: "todo-capability-done", Title: "capture scope", Status: "done", Priority: 3, UpdatedAt: now},
	}
	mgr.mu.Unlock()

	turnRef := agentRuntimeTurnRef{
		TaskID: "task-capability-blocked",
		RunID:  "run-capability-blocked",
		TurnID: "turn-capability-blocked",
	}
	operatorPrompt := "run ffuf directory enumeration on https://example.com"
	action := agentPlannerAction{
		Name:      "scan.web.safe",
		Arguments: map[string]any{"target": "https://example.com"},
	}
	expectedReason, expectedSuggestedPrompt, mismatch := agentPlannerActionCapabilityMismatch(operatorPrompt, action.Name, "en")
	require.True(t, mismatch)
	require.NotEmpty(t, expectedSuggestedPrompt)
	intentID := agentClassifyCapabilityIntent(operatorPrompt).ID
	require.Equal(t, agentIntentContentDiscovery, intentID)

	records := []runtime.SessionMutationRequest{}
	legacy := daemonSessionMutator{manager: mgr}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		if req.Kind == runtime.SessionMutationCapabilityBlocked {
			records = append(records, req)
		}
		return legacy.MutateSession(ctx, req)
	}))

	messages, toolCalls, authorizationRequired := mgr.executeAgentPlannerActionsWithToolServices(
		context.Background(),
		sessionID,
		turnRef,
		operatorPrompt,
		"en",
		"tester",
		"safe_scan",
		testAgentLoopToolServices(),
		&agentPlannerResponse{Actions: []agentPlannerAction{action}},
		nil,
		"",
	)

	require.True(t, authorizationRequired)
	require.Len(t, messages, 1)
	require.Equal(t, "step_replan", messages[0].Phase)
	require.Equal(t, expectedReason, messages[0].Content)
	require.Len(t, toolCalls, 1)
	require.Equal(t, "step_replan", toolCalls[0].Metadata["agent_phase"])
	require.Len(t, records, 1)

	req := records[0]
	require.Equal(t, runtime.SessionMutationCapabilityBlocked, req.Kind)
	require.Equal(t, sessionID, req.Turn.SessionID)
	require.Equal(t, turnRef.TaskID, req.Turn.TaskID)
	require.Equal(t, turnRef.RunID, req.Turn.RunID)
	require.Equal(t, turnRef.TurnID, req.Turn.TurnID)
	require.Equal(t, operatorPrompt, req.Turn.Prompt)
	require.Equal(t, string(runtime.SessionMutationCapabilityBlocked), req.Turn.Origin)
	require.Equal(t, "safe_scan", req.Turn.OperationLevel)
	require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
	require.Equal(t, string(runtime.SessionMutationCapabilityBlocked), req.Turn.Metadata["kind"])
	require.Equal(t, "scan.web.safe", req.Turn.Metadata["action_name"])
	require.Equal(t, intentID, req.Turn.Metadata["intent_id"])
	require.Equal(t, expectedSuggestedPrompt, req.Turn.Metadata["suggested_prompt"])
	require.Equal(t, "safe_scan", req.Turn.Metadata["operation_level"])
	require.Equal(t, string(runtime.SessionMutationCapabilityBlocked), req.Metadata["kind"])
	require.Equal(t, "scan.web.safe", req.Metadata["action_name"])
	require.Equal(t, intentID, req.Metadata["intent_id"])
	require.Equal(t, expectedSuggestedPrompt, req.Metadata["suggested_prompt"])
	require.Equal(t, "safe_scan", req.Metadata["operation_level"])

	payload, ok := req.Payload.(runtime.AgentLoopCapabilityBlockedPayload)
	require.True(t, ok, "payload type = %T", req.Payload)
	require.Equal(t, "en", payload.Language)
	require.Equal(t, "scan.web.safe", payload.ActionName)
	require.Equal(t, intentID, payload.IntentID)
	require.Equal(t, expectedReason, payload.Reason)
	require.Equal(t, expectedSuggestedPrompt, payload.SuggestedPrompt)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "blocked", got.Status)
	require.NotNil(t, got.ActiveTask)
	require.Equal(t, "blocked", got.ActiveTask.Status)
	require.Len(t, got.Todos, 3)
	require.Equal(t, "blocked", got.Todos[0].Status)
	require.Equal(t, expectedReason, got.Todos[0].BlockedReason)
	require.Equal(t, "blocked", got.Todos[1].Status)
	require.Equal(t, expectedReason, got.Todos[1].BlockedReason)
	require.Equal(t, "done", got.Todos[2].Status)
	require.NotNil(t, got.NextAction)
	require.Equal(t, "blocked", got.NextAction.Type)
	require.Equal(t, trimSummary(expectedReason, 360), got.NextAction.Reason)
	require.Equal(t, expectedSuggestedPrompt, got.NextAction.SuggestedPrompt)
	require.False(t, got.NextAction.AutoContinue)
}

func TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator(t *testing.T) {
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
	initialMessages := len(created.Session.Messages)
	now := time.Date(2026, 6, 6, 12, 15, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:        "task-control-session-mutation",
		Goal:      "pause through runtime session mutator",
		Kind:      "analysis",
		Status:    "running",
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.Todos = []agentTodo{
		{ID: "todo-control-running", Title: "inspect runtime seam", Status: "running", Priority: 1, UpdatedAt: now},
		{ID: "todo-control-pending", Title: "verify mutation result", Status: "pending", Priority: 2, UpdatedAt: now},
	}
	mgr.mu.Unlock()

	req := agentControlRequest{
		Action:         "pause",
		Prompt:         "resume later",
		Reason:         "operator pause",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "safe_scan",
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationControlSession, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-control-session-mutation", req.Turn.TaskID)
		require.Equal(t, "run_pause", req.Turn.RunID)
		require.Equal(t, "turn_pause", req.Turn.TurnID)
		require.Equal(t, "resume later", req.Turn.Prompt)
		require.Equal(t, "operator", req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationControlSession), req.Turn.Metadata["kind"])
		require.Equal(t, string(runtime.SessionMutationControlSession), req.Metadata["kind"])
		require.Equal(t, "pause", req.Metadata["action"])
		require.Equal(t, "safe_scan", req.Metadata["operation_level"])
		require.Equal(t, false, req.Metadata["clear_input"])

		payload, ok := req.Payload.(runtime.AgentLoopControlSessionPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "pause", payload.Action)
		require.Equal(t, "operator pause", payload.Reason)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "en", payload.Language)

		resultPayload, err := mgr.controlSessionLegacy(ctx, req.Turn.SessionID, payload.Action, req.Turn.Prompt, payload.Reason, payload.RunID, payload.JobID, payload.ClearInput, payload.Operator, payload.Language, req.Turn.OperationLevel)
		require.NoError(t, err)
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "handled by test mutator",
			Payload: agentLoopControlSessionResultPayloadFromDaemon(resultPayload),
			Data:    map[string]any{"mutator": "test"},
		}, nil
	}))

	result, handled, err := mgr.runRuntimeControlSessionMutation(context.Background(), sessionID, "pause", req, "operator-1", "en", "safe_scan")

	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, called)
	require.True(t, result.Mutated)
	require.Equal(t, sessionID, result.Session.SessionID)
	require.Equal(t, "operator-1", result.Session.Operator)
	require.Equal(t, "safe_scan", result.Session.OperationLevel)
	require.Equal(t, "paused", result.Session.Status)
	require.Equal(t, "agent", result.Message.Role)
	require.NotEmpty(t, result.Message.ID)
	require.Empty(t, result.CancelJobID)
	require.NotNil(t, result.Session.ActiveTask)
	require.Equal(t, "blocked", result.Session.ActiveTask.Status)
	require.NotNil(t, result.Session.NextAction)
	require.Equal(t, "continue", result.Session.NextAction.Type)
	require.Equal(t, "operator pause", result.Session.NextAction.Reason)
	require.Equal(t, "resume later", result.Session.NextAction.SuggestedPrompt)
	require.Len(t, result.Session.Todos, 2)
	require.Equal(t, "blocked", result.Session.Todos[0].Status)
	require.Equal(t, "pending", result.Session.Todos[1].Status)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "paused", got.Status)
	require.Len(t, got.Messages, initialMessages+1)
	require.Equal(t, result.Message.ID, got.Messages[len(got.Messages)-1].ID)
}

func TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator(t *testing.T) {
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
	now := time.Date(2026, 6, 6, 12, 30, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.ActiveTask = &agentTask{
		ID:        "task-auth-session-mutation",
		Goal:      "authorize a workspace action",
		Kind:      "tool",
		Status:    "blocked",
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.Todos = []agentTodo{
		{ID: "todo-auth-running", Title: "run protected action", Status: "running", Priority: 1, UpdatedAt: now},
		{ID: "todo-auth-pending", Title: "verify result", Status: "pending", Priority: 2, UpdatedAt: now},
	}
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-session-mutation",
		ActionName:     "workspace.files.write",
		Arguments:      map[string]any{"path": "README.md"},
		RequiredLevel:  "workspace_write",
		Policy:         "manual",
		Reason:         "workspace write requires operator confirmation",
		OriginalPrompt: "write README.md",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mgr.mu.Unlock()

	req := agentAuthorizationDecisionRequest{
		Action:          "deny",
		AuthorizationID: "auth-session-mutation",
		Operator:        "operator-1",
		Language:        "en",
		OperationLevel:  "safe_scan",
		Reason:          "not within scope",
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationResolveAuthorization, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-auth-session-mutation", req.Turn.TaskID)
		require.Equal(t, "run_auth-session-mutation", req.Turn.RunID)
		require.Equal(t, "turn_auth-session-mutation", req.Turn.TurnID)
		require.Equal(t, "operator", req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationResolveAuthorization), req.Metadata["kind"])
		require.Equal(t, "deny", req.Metadata["decision"])
		require.Equal(t, "auth-session-mutation", req.Metadata["authorization_id"])
		require.Equal(t, "safe_scan", req.Metadata["operation_level"])
		require.Equal(t, true, req.Metadata["operation_level_provided"])

		payload, ok := req.Payload.(runtime.AgentLoopResolveAuthorizationPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "deny", payload.Decision)
		require.Equal(t, "auth-session-mutation", payload.AuthorizationID)
		require.Equal(t, "not within scope", payload.Reason)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "en", payload.Language)
		require.True(t, payload.OperationLevelProvided)

		resultPayload, err := mgr.resolveAuthorizationLegacy(ctx, req.Turn.SessionID, payload.Decision, payload.AuthorizationID, payload.Reason, payload.Operator, payload.Language, req.Turn.OperationLevel, payload.OperationLevelProvided)
		require.NoError(t, err)
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "handled by test mutator",
			Payload: agentLoopResolveAuthorizationResultPayloadFromDaemon(resultPayload),
			Data:    map[string]any{"mutator": "test"},
		}, nil
	}))

	result, handled, err := mgr.runRuntimeResolveAuthorizationMutation(context.Background(), sessionID, "deny", req, "operator-1", "en", "safe_scan")

	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, called)
	require.True(t, result.Mutated)
	require.Equal(t, sessionID, result.Session.SessionID)
	require.Equal(t, "operator-1", result.Session.Operator)
	require.Equal(t, "safe_scan", result.OperationLevel)
	require.Equal(t, "denied", result.Pending.Status)
	require.Equal(t, "auth-session-mutation", result.Pending.ID)
	require.Equal(t, "agent", result.Message.Role)
	require.Equal(t, "step", result.Message.Phase)
	require.Contains(t, result.Message.Content, "Authorization denied")
	require.Nil(t, result.Session.PendingAuthorization)
	require.Equal(t, "blocked", result.Session.Status)
	require.NotNil(t, result.Session.ActiveTask)
	require.Equal(t, "blocked", result.Session.ActiveTask.Status)
	require.NotNil(t, result.Session.NextAction)
	require.Equal(t, "blocked", result.Session.NextAction.Type)
	require.Equal(t, "not within scope", result.Session.NextAction.Reason)
	require.Len(t, result.Session.Todos, 2)
	require.Equal(t, "blocked", result.Session.Todos[0].Status)
	require.Equal(t, "blocked", result.Session.Todos[1].Status)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Nil(t, got.PendingAuthorization)
	require.Equal(t, "blocked", got.Status)
	require.Equal(t, result.Message.ID, got.Messages[len(got.Messages)-1].ID)
}

func TestAgentManagerResolveAuthorizationCompactionUsesMutationTurnRef(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 13, 30, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.Messages = nil
	for i := 0; i < agentSessionMaxMessages+4; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		session.Messages = append(session.Messages, agentMessage{
			ID:        fmt.Sprintf("msg-auth-compaction-%03d", i),
			Role:      role,
			Content:   fmt.Sprintf("authorization history %03d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	session.ActiveTask = &agentTask{
		ID:          "task-auth-compaction",
		Goal:        "deny with memory compaction",
		Kind:        "tool",
		Status:      "blocked",
		ActiveRunID: "run-auth-compaction",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-compaction",
		ActionName:     "workspace.files.write",
		Arguments:      map[string]any{"path": "README.md"},
		RequiredLevel:  "workspace_write",
		Policy:         "manual",
		Reason:         "workspace write requires operator confirmation",
		OriginalPrompt: "write README.md",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mgr.mu.Unlock()

	_, err = mgr.DecideAgentAuthorization(context.Background(), sessionID, agentAuthorizationDecisionRequest{
		Action:          "deny",
		AuthorizationID: "auth-compaction",
		Operator:        "operator-1",
		Language:        "en",
		OperationLevel:  "safe_scan",
		Reason:          "not within scope",
	}, routerServices{})
	require.NoError(t, err)

	var compacted *runtime.Event
	for _, ev := range store.Events() {
		if ev.Type == "memory_compacted" {
			event := ev
			compacted = &event
			break
		}
	}
	require.NotNil(t, compacted, "expected memory_compacted event")
	require.Equal(t, "task-auth-compaction", compacted.TaskID)
	require.Equal(t, "run-auth-compaction", compacted.RunID)
	require.Equal(t, "turn_auth-compaction", compacted.TurnID)
	require.Equal(t, sessionID, compacted.Data["session_id"])
	require.Greater(t, compacted.Data["compacted_count"], 0)
}
