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

func (r daemonSessionMutator) MutateSession(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	if r.manager == nil {
		err := errors.New("daemon: session mutator has no manager")
		return daemonSessionMutationErrorResult(req, err), err
	}

	switch req.Kind {
	case runtime.SessionMutationAppendMessages:
		payload, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopAppendMessagesPayload](req.Payload)
		if !ok {
			err := errors.New("daemon: invalid append messages mutation payload")
			return daemonSessionMutationErrorResult(req, err), err
		}
		sessionID := req.Turn.SessionID
		plan := agentPlannerResponsePtrFromAgentLoopPlan(payload.Plan)
		messages := agentMessagesFromAgentLoopMessages(payload.Messages)
		resultPayload, mutated := r.manager.appendMessagesWithPlannerLegacyForTurn(ctx, sessionID, agentRuntimeTurnRefFromTurn(req.Turn), plan, messages...)
		resultPayload.Mutated = mutated
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon session append mutation executed",
			Payload: agentLoopAppendMessagesResultPayloadFromDaemon(resultPayload),
			Data: map[string]any{
				"mutator":       "daemon_legacy_session",
				"kind":          string(req.Kind),
				"session_id":    sessionID,
				"message_count": len(payload.Messages),
				"planner_apply": payload.Plan != nil,
				"mutated":       mutated,
			},
		}, nil
	case runtime.SessionMutationRecordUserMessage:
		payload, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopRecordUserMessagePayload](req.Payload)
		if !ok {
			err := errors.New("daemon: invalid record user message mutation payload")
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
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon user message mutation executed",
			Payload: agentLoopRecordUserMessageResultPayloadFromDaemon(resultPayload),
			Data: map[string]any{
				"mutator":         "daemon_legacy_session",
				"kind":            string(req.Kind),
				"session_id":      sessionID,
				"message_id":      resultPayload.UserMessage.ID,
				"operation_level": operationLevel,
				"mutated":         true,
			},
		}, nil
	case runtime.SessionMutationRequestAuthorization:
		payload, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopAuthorizationRequestPayload](req.Payload)
		if !ok {
			err := errors.New("daemon: invalid request authorization mutation payload")
			return daemonSessionMutationErrorResult(req, err), err
		}
		sessionID := req.Turn.SessionID
		operatorPrompt := req.Turn.Prompt
		operationLevel := req.Turn.OperationLevel
		action := agentPlannerActionFromAgentLoopAction(payload.Action)
		resultPayload := r.manager.requestAgentAuthorizationLegacy(sessionID, payload.Language, operatorPrompt, action, payload.ActionName, payload.RequiredLevel, payload.Policy, payload.Reason)
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon authorization request mutation executed",
			Payload: agentLoopRequestAuthorizationResultPayloadFromDaemon(resultPayload),
			Data: map[string]any{
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
		}, nil
	case runtime.SessionMutationCapabilityBlocked:
		payload, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopCapabilityBlockedPayload](req.Payload)
		if !ok {
			err := errors.New("daemon: invalid capability blocked mutation payload")
			return daemonSessionMutationErrorResult(req, err), err
		}
		sessionID := req.Turn.SessionID
		operationLevel := req.Turn.OperationLevel
		resultPayload := r.manager.markAgentCapabilityBlockedLegacy(sessionID, payload.Language, payload.ActionName, payload.IntentID, payload.Reason, payload.SuggestedPrompt)
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon capability blocked mutation executed",
			Payload: agentLoopCapabilityBlockedResultPayloadFromDaemon(resultPayload),
			Data: map[string]any{
				"mutator":          "daemon_legacy_session",
				"kind":             string(req.Kind),
				"session_id":       sessionID,
				"action_name":      resultPayload.ActionName,
				"intent_id":        resultPayload.IntentID,
				"suggested_prompt": resultPayload.SuggestedPrompt,
				"operation_level":  operationLevel,
				"mutated":          resultPayload.Mutated,
			},
		}, nil
	case runtime.SessionMutationControlSession:
		payload, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopControlSessionPayload](req.Payload)
		if !ok {
			err := errors.New("daemon: invalid control session mutation payload")
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
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon control session mutation executed",
			Payload: agentLoopControlSessionResultPayloadFromDaemon(resultPayload),
			Data: map[string]any{
				"mutator":         "daemon_legacy_session",
				"kind":            string(req.Kind),
				"session_id":      sessionID,
				"action":          payload.Action,
				"message_id":      resultPayload.Message.ID,
				"cancel_job_id":   resultPayload.CancelJobID,
				"operation_level": operationLevel,
				"mutated":         true,
			},
		}, nil
	case runtime.SessionMutationResolveAuthorization:
		payload, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopResolveAuthorizationPayload](req.Payload)
		if !ok {
			err := errors.New("daemon: invalid resolve authorization mutation payload")
			return daemonSessionMutationErrorResult(req, err), err
		}
		sessionID := req.Turn.SessionID
		operationLevel := req.Turn.OperationLevel
		resultPayload, err := r.manager.resolveAuthorizationLegacyForTurn(ctx, agentRuntimeTurnRefFromTurn(req.Turn), sessionID, payload.Decision, payload.AuthorizationID, payload.Reason, payload.Operator, payload.Language, operationLevel, payload.OperationLevelProvided)
		if err != nil {
			return daemonSessionMutationErrorResult(req, err), err
		}
		resultPayload.Mutated = true
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon authorization mutation executed",
			Payload: agentLoopResolveAuthorizationResultPayloadFromDaemon(resultPayload),
			Data: map[string]any{
				"mutator":          "daemon_legacy_session",
				"kind":             string(req.Kind),
				"session_id":       sessionID,
				"authorization_id": resultPayload.Pending.ID,
				"decision":         payload.Decision,
				"operation_level":  resultPayload.OperationLevel,
				"message_id":       resultPayload.Message.ID,
				"mutated":          true,
			},
		}, nil
	default:
		err := errors.New("daemon: unsupported session mutation kind")
		return daemonSessionMutationErrorResult(req, err), err
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

func (m *agentManager) runRuntimeAppendMessagesMutation(ctx context.Context, sessionID string, plan *agentPlannerResponse, messages ...agentMessage) bool {
	return m.runRuntimeAppendMessagesMutationWithTurn(ctx, runtime.TurnContext{
		SessionID: sessionID,
		Metadata: map[string]any{
			"mutator": "session_mutation",
			"kind":    string(runtime.SessionMutationAppendMessages),
		},
	}, plan, messages...)
}

func (m *agentManager) runRuntimeAppendMessagesMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, origin, operationLevel string, plan *agentPlannerResponse, messages ...agentMessage) bool {
	return m.runRuntimeAppendMessagesMutationWithTurn(ctx, runtime.TurnContext{
		SessionID:      sessionID,
		TaskID:         turnRef.TaskID,
		RunID:          turnRef.RunID,
		TurnID:         turnRef.TurnID,
		Prompt:         prompt,
		Origin:         origin,
		OperationLevel: operationLevel,
		Metadata: map[string]any{
			"mutator":         "session_mutation",
			"kind":            string(runtime.SessionMutationAppendMessages),
			"message_count":   len(messages),
			"planner_apply":   plan != nil,
			"operation_level": operationLevel,
		},
	}, plan, messages...)
}

func (m *agentManager) runRuntimeAppendMessagesMutationWithTurn(ctx context.Context, turn runtime.TurnContext, plan *agentPlannerResponse, messages ...agentMessage) bool {
	eng := m.runtimeEngine()
	if eng == nil || eng.SessionMutator() == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if turn.Metadata == nil {
		turn.Metadata = map[string]any{}
	}
	turn.Metadata["mutator"] = "session_mutation"
	turn.Metadata["kind"] = string(runtime.SessionMutationAppendMessages)
	turn.Metadata["message_count"] = len(messages)
	turn.Metadata["planner_apply"] = plan != nil
	if turn.OperationLevel != "" {
		turn.Metadata["operation_level"] = turn.OperationLevel
	}
	result, err := eng.MutateSession(ctx, runtime.SessionMutationRequest{
		Turn: turn,
		Kind: runtime.SessionMutationAppendMessages,
		Payload: runtime.AgentLoopAppendMessagesPayload{
			Plan:     agentLoopPlanPtrFromAgentPlannerResponse(plan),
			Messages: agentLoopMessagesFromAgentMessages(messages),
		},
		Metadata: map[string]any{
			"mutator":       "session_mutation",
			"kind":          string(runtime.SessionMutationAppendMessages),
			"message_count": len(messages),
			"planner_apply": plan != nil,
		},
	})
	if errors.Is(err, runtime.ErrSessionMutatorNotAttached) ||
		errors.Is(result.Error, runtime.ErrSessionMutatorNotAttached) {
		return false
	}
	if err != nil {
		return false
	}
	return result.Status != runtime.TurnResultBlocked
}

func (m *agentManager) runRuntimeRecordUserMessageMutation(ctx context.Context, sessionID, prompt, operator, language, operationLevel string) (daemonRecordUserMessageMutationResultPayload, bool, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.SessionMutator() == nil {
		return daemonRecordUserMessageMutationResultPayload{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := eng.MutateSession(ctx, runtime.SessionMutationRequest{
		Turn: runtime.TurnContext{
			SessionID:      sessionID,
			Prompt:         prompt,
			Origin:         "user",
			OperationLevel: operationLevel,
			Metadata: map[string]any{
				"mutator": "session_mutation",
				"kind":    string(runtime.SessionMutationRecordUserMessage),
			},
		},
		Kind: runtime.SessionMutationRecordUserMessage,
		Payload: runtime.AgentLoopRecordUserMessagePayload{
			Operator: operator,
			Language: language,
		},
		Metadata: map[string]any{
			"mutator":         "session_mutation",
			"kind":            string(runtime.SessionMutationRecordUserMessage),
			"operation_level": operationLevel,
		},
	})
	if errors.Is(err, runtime.ErrSessionMutatorNotAttached) ||
		errors.Is(result.Error, runtime.ErrSessionMutatorNotAttached) {
		return daemonRecordUserMessageMutationResultPayload{}, false, nil
	}
	if err != nil {
		return daemonRecordUserMessageMutationResultPayload{}, true, err
	}
	if result.Status == runtime.TurnResultBlocked {
		return daemonRecordUserMessageMutationResultPayload{}, false, nil
	}
	payload, ok := daemonRecordUserMessageMutationResultPayloadFromSessionMutationResult(result)
	if !ok {
		return daemonRecordUserMessageMutationResultPayload{}, true, errors.New("daemon: record user message mutator returned invalid payload")
	}
	return payload, true, nil
}

func (m *agentManager) runRuntimeRequestAuthorizationMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, operatorPrompt, language, operationLevel string, action runtime.AgentLoopAction, actionName, requiredLevel, policy, reason string) (daemonRequestAuthorizationMutationResultPayload, bool) {
	eng := m.runtimeEngine()
	if eng == nil || eng.SessionMutator() == nil {
		return daemonRequestAuthorizationMutationResultPayload{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	action = cloneAgentLoopAction(action)
	result, err := eng.MutateSession(ctx, runtime.SessionMutationRequest{
		Turn: runtime.TurnContext{
			SessionID:      sessionID,
			TaskID:         turnRef.TaskID,
			RunID:          turnRef.RunID,
			TurnID:         turnRef.TurnID,
			Prompt:         operatorPrompt,
			Origin:         string(runtime.SessionMutationRequestAuthorization),
			OperationLevel: operationLevel,
			Metadata: map[string]any{
				"mutator": "session_mutation",
				"kind":    string(runtime.SessionMutationRequestAuthorization),
			},
		},
		Kind: runtime.SessionMutationRequestAuthorization,
		Payload: runtime.AgentLoopAuthorizationRequestPayload{
			Language:      language,
			Action:        action,
			ActionName:    actionName,
			RequiredLevel: requiredLevel,
			Policy:        policy,
			Reason:        reason,
		},
		Metadata: map[string]any{
			"mutator":         "session_mutation",
			"kind":            string(runtime.SessionMutationRequestAuthorization),
			"action_name":     firstNonEmpty(normalizeAgentPlannerActionName(actionName), strings.TrimSpace(action.Name)),
			"required_level":  normalizeAgentOperationLevel(firstNonEmpty(requiredLevel, "read_only")),
			"policy":          strings.TrimSpace(policy),
			"operation_level": operationLevel,
		},
	})
	if errors.Is(err, runtime.ErrSessionMutatorNotAttached) ||
		errors.Is(result.Error, runtime.ErrSessionMutatorNotAttached) {
		return daemonRequestAuthorizationMutationResultPayload{}, false
	}
	if err != nil || result.Status == runtime.TurnResultBlocked {
		return daemonRequestAuthorizationMutationResultPayload{}, false
	}
	payload, ok := daemonRequestAuthorizationMutationResultPayloadFromSessionMutationResult(result)
	if !ok {
		return daemonRequestAuthorizationMutationResultPayload{}, false
	}
	return payload, true
}

func (m *agentManager) runRuntimeCapabilityBlockedMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, operatorPrompt, language, operationLevel, actionName, intentID, reason, suggestedPrompt string) (daemonCapabilityBlockedMutationResultPayload, bool) {
	eng := m.runtimeEngine()
	if eng == nil || eng.SessionMutator() == nil {
		return daemonCapabilityBlockedMutationResultPayload{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	actionName = firstNonEmpty(normalizeAgentPlannerActionName(actionName), strings.TrimSpace(actionName))
	intentID = strings.TrimSpace(intentID)
	result, err := eng.MutateSession(ctx, runtime.SessionMutationRequest{
		Turn: runtime.TurnContext{
			SessionID:      sessionID,
			TaskID:         turnRef.TaskID,
			RunID:          turnRef.RunID,
			TurnID:         turnRef.TurnID,
			Prompt:         operatorPrompt,
			Origin:         string(runtime.SessionMutationCapabilityBlocked),
			OperationLevel: operationLevel,
			Metadata: map[string]any{
				"mutator":          "session_mutation",
				"kind":             string(runtime.SessionMutationCapabilityBlocked),
				"action_name":      actionName,
				"intent_id":        intentID,
				"suggested_prompt": strings.TrimSpace(suggestedPrompt),
				"operation_level":  operationLevel,
			},
		},
		Kind: runtime.SessionMutationCapabilityBlocked,
		Payload: runtime.AgentLoopCapabilityBlockedPayload{
			Language:        language,
			ActionName:      actionName,
			IntentID:        intentID,
			Reason:          reason,
			SuggestedPrompt: suggestedPrompt,
		},
		Metadata: map[string]any{
			"mutator":          "session_mutation",
			"kind":             string(runtime.SessionMutationCapabilityBlocked),
			"action_name":      actionName,
			"intent_id":        intentID,
			"suggested_prompt": strings.TrimSpace(suggestedPrompt),
			"operation_level":  operationLevel,
		},
	})
	if errors.Is(err, runtime.ErrSessionMutatorNotAttached) ||
		errors.Is(result.Error, runtime.ErrSessionMutatorNotAttached) {
		return daemonCapabilityBlockedMutationResultPayload{}, false
	}
	if err != nil || result.Status == runtime.TurnResultBlocked {
		return daemonCapabilityBlockedMutationResultPayload{}, false
	}
	payload, ok := daemonCapabilityBlockedMutationResultPayloadFromSessionMutationResult(result)
	if !ok {
		return daemonCapabilityBlockedMutationResultPayload{}, false
	}
	return payload, true
}

func (m *agentManager) runRuntimeControlSessionMutation(ctx context.Context, sessionID, action string, req agentControlRequest, operator, language, operationLevel string) (daemonControlSessionMutationResultPayload, bool, error) {
	turnRef := m.controlSessionMutationTurnRef(ctx, sessionID, action, req)
	return m.runRuntimeControlSessionMutationForTurn(ctx, sessionID, turnRef, action, req, operator, language, operationLevel)
}

func (m *agentManager) controlSessionMutationTurnRef(ctx context.Context, sessionID, action string, req agentControlRequest) agentRuntimeTurnRef {
	return m.externalSessionMutationTurnRef(ctx, sessionID, firstNonEmpty(strings.TrimSpace(req.JobID), strings.TrimSpace(req.RunID), strings.TrimSpace(action)))
}

func (m *agentManager) runRuntimeControlSessionMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, action string, req agentControlRequest, operator, language, operationLevel string) (daemonControlSessionMutationResultPayload, bool, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.SessionMutator() == nil {
		return daemonControlSessionMutationResultPayload{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := eng.MutateSession(ctx, runtime.SessionMutationRequest{
		Turn: runtime.TurnContext{
			SessionID:      sessionID,
			TaskID:         turnRef.TaskID,
			RunID:          turnRef.RunID,
			TurnID:         turnRef.TurnID,
			Prompt:         req.Prompt,
			Origin:         "operator",
			OperationLevel: operationLevel,
			Metadata: map[string]any{
				"mutator": "session_mutation",
				"kind":    string(runtime.SessionMutationControlSession),
			},
		},
		Kind: runtime.SessionMutationControlSession,
		Payload: runtime.AgentLoopControlSessionPayload{
			Action:     action,
			Reason:     req.Reason,
			RunID:      req.RunID,
			JobID:      req.JobID,
			ClearInput: req.ClearInput,
			Operator:   operator,
			Language:   language,
		},
		Metadata: map[string]any{
			"mutator":         "session_mutation",
			"kind":            string(runtime.SessionMutationControlSession),
			"action":          action,
			"operation_level": operationLevel,
			"clear_input":     req.ClearInput,
			"run_id":          strings.TrimSpace(req.RunID),
			"job_id":          strings.TrimSpace(req.JobID),
		},
	})
	if errors.Is(err, runtime.ErrSessionMutatorNotAttached) ||
		errors.Is(result.Error, runtime.ErrSessionMutatorNotAttached) {
		return daemonControlSessionMutationResultPayload{}, false, nil
	}
	if err != nil {
		return daemonControlSessionMutationResultPayload{}, true, err
	}
	if result.Status == runtime.TurnResultBlocked {
		return daemonControlSessionMutationResultPayload{}, false, nil
	}
	payload, ok := daemonControlSessionMutationResultPayloadFromSessionMutationResult(result)
	if !ok {
		return daemonControlSessionMutationResultPayload{}, true, errors.New("daemon: control session mutator returned invalid payload")
	}
	return payload, true, nil
}

func (m *agentManager) runRuntimeResolveAuthorizationMutation(ctx context.Context, sessionID, decision string, req agentAuthorizationDecisionRequest, operator, language, operationLevel string) (daemonResolveAuthorizationMutationResultPayload, bool, error) {
	turnRef := m.resolveAuthorizationMutationTurnRef(ctx, sessionID, decision, req)
	return m.runRuntimeResolveAuthorizationMutationForTurn(ctx, sessionID, turnRef, decision, req, operator, language, operationLevel)
}

func (m *agentManager) resolveAuthorizationMutationTurnRef(ctx context.Context, sessionID, decision string, req agentAuthorizationDecisionRequest) agentRuntimeTurnRef {
	return m.externalSessionMutationTurnRef(ctx, sessionID, firstNonEmpty(strings.TrimSpace(req.AuthorizationID), strings.TrimSpace(decision)))
}

func (m *agentManager) runRuntimeResolveAuthorizationMutationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, decision string, req agentAuthorizationDecisionRequest, operator, language, operationLevel string) (daemonResolveAuthorizationMutationResultPayload, bool, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.SessionMutator() == nil {
		return daemonResolveAuthorizationMutationResultPayload{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	operationLevelProvided := strings.TrimSpace(req.OperationLevel) != ""
	result, err := eng.MutateSession(ctx, runtime.SessionMutationRequest{
		Turn: runtime.TurnContext{
			SessionID:      sessionID,
			TaskID:         turnRef.TaskID,
			RunID:          turnRef.RunID,
			TurnID:         turnRef.TurnID,
			Origin:         "operator",
			OperationLevel: operationLevel,
			Metadata: map[string]any{
				"mutator": "session_mutation",
				"kind":    string(runtime.SessionMutationResolveAuthorization),
			},
		},
		Kind: runtime.SessionMutationResolveAuthorization,
		Payload: runtime.AgentLoopResolveAuthorizationPayload{
			Decision:               decision,
			AuthorizationID:        strings.TrimSpace(req.AuthorizationID),
			Reason:                 strings.TrimSpace(req.Reason),
			Operator:               operator,
			Language:               language,
			OperationLevelProvided: operationLevelProvided,
		},
		Metadata: map[string]any{
			"mutator":                  "session_mutation",
			"kind":                     string(runtime.SessionMutationResolveAuthorization),
			"authorization_id":         strings.TrimSpace(req.AuthorizationID),
			"decision":                 decision,
			"operation_level":          operationLevel,
			"operation_level_provided": operationLevelProvided,
		},
	})
	if errors.Is(err, runtime.ErrSessionMutatorNotAttached) ||
		errors.Is(result.Error, runtime.ErrSessionMutatorNotAttached) {
		return daemonResolveAuthorizationMutationResultPayload{}, false, nil
	}
	if err != nil {
		return daemonResolveAuthorizationMutationResultPayload{}, true, err
	}
	if result.Status == runtime.TurnResultBlocked {
		return daemonResolveAuthorizationMutationResultPayload{}, false, nil
	}
	payload, ok := daemonResolveAuthorizationMutationResultPayloadFromSessionMutationResult(result)
	if !ok {
		return daemonResolveAuthorizationMutationResultPayload{}, true, errors.New("daemon: resolve authorization mutator returned invalid payload")
	}
	return payload, true, nil
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
