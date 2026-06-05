package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

type daemonAppendMessagesMutationPayload struct {
	SessionID string
	Plan      *agentPlannerResponse
	Messages  []agentMessage
}

type daemonAppendMessagesMutationResultPayload struct {
	Session    agentSession
	Compaction *agentMemoryCompaction
	Mutated    bool
}

type daemonRecordUserMessageMutationPayload struct {
	SessionID      string
	Prompt         string
	Operator       string
	Language       string
	OperationLevel string
}

type daemonRecordUserMessageMutationResultPayload struct {
	Session       agentSession
	UserMessage   agentMessage
	History       []agentMessage
	MemorySummary *agentMemorySummary
	Compaction    *agentMemoryCompaction
	Mutated       bool
}

type daemonResolveAuthorizationMutationPayload struct {
	SessionID              string
	Decision               string
	AuthorizationID        string
	Reason                 string
	Operator               string
	Language               string
	OperationLevel         string
	OperationLevelProvided bool
}

type daemonResolveAuthorizationMutationResultPayload struct {
	Session        agentSession
	Pending        agentAuthorizationRequest
	Message        agentMessage
	OperationLevel string
	Compaction     *agentMemoryCompaction
	Mutated        bool
}

type daemonSessionMutator struct {
	manager *agentManager
}

func (r daemonSessionMutator) MutateSession(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
	if r.manager == nil {
		err := errors.New("daemon: session mutator has no manager")
		return daemonSessionMutationErrorResult(req, err), err
	}

	switch req.Kind {
	case runtime.SessionMutationAppendMessages:
		payload, ok := req.Payload.(daemonAppendMessagesMutationPayload)
		if !ok {
			err := errors.New("daemon: invalid append messages mutation payload")
			return daemonSessionMutationErrorResult(req, err), err
		}
		resultPayload, mutated := r.manager.appendMessagesWithPlannerLegacy(payload.SessionID, payload.Plan, payload.Messages...)
		resultPayload.Mutated = mutated
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon session append mutation executed",
			Payload: resultPayload,
			Data: map[string]any{
				"mutator":       "daemon_legacy_session",
				"kind":          string(req.Kind),
				"session_id":    payload.SessionID,
				"message_count": len(payload.Messages),
				"planner_apply": payload.Plan != nil,
				"mutated":       mutated,
			},
		}, nil
	case runtime.SessionMutationRecordUserMessage:
		payload, ok := req.Payload.(daemonRecordUserMessageMutationPayload)
		if !ok {
			err := errors.New("daemon: invalid record user message mutation payload")
			return daemonSessionMutationErrorResult(req, err), err
		}
		resultPayload, err := r.manager.recordUserMessageLegacy(payload.SessionID, payload.Prompt, payload.Operator, payload.Language, payload.OperationLevel)
		if err != nil {
			return daemonSessionMutationErrorResult(req, err), err
		}
		resultPayload.Mutated = true
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon user message mutation executed",
			Payload: resultPayload,
			Data: map[string]any{
				"mutator":         "daemon_legacy_session",
				"kind":            string(req.Kind),
				"session_id":      payload.SessionID,
				"message_id":      resultPayload.UserMessage.ID,
				"operation_level": payload.OperationLevel,
				"mutated":         true,
			},
		}, nil
	case runtime.SessionMutationResolveAuthorization:
		payload, ok := req.Payload.(daemonResolveAuthorizationMutationPayload)
		if !ok {
			err := errors.New("daemon: invalid resolve authorization mutation payload")
			return daemonSessionMutationErrorResult(req, err), err
		}
		resultPayload, err := r.manager.resolveAuthorizationLegacy(ctx, payload.SessionID, payload.Decision, payload.AuthorizationID, payload.Reason, payload.Operator, payload.Language, payload.OperationLevel, payload.OperationLevelProvided)
		if err != nil {
			return daemonSessionMutationErrorResult(req, err), err
		}
		resultPayload.Mutated = true
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "legacy daemon authorization mutation executed",
			Payload: resultPayload,
			Data: map[string]any{
				"mutator":          "daemon_legacy_session",
				"kind":             string(req.Kind),
				"session_id":       payload.SessionID,
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
	eng := m.runtimeEngine()
	if eng == nil || eng.SessionMutator() == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := eng.MutateSession(ctx, runtime.SessionMutationRequest{
		Turn: runtime.TurnContext{
			SessionID: sessionID,
			Metadata: map[string]any{
				"mutator": "session_mutation",
				"kind":    string(runtime.SessionMutationAppendMessages),
			},
		},
		Kind: runtime.SessionMutationAppendMessages,
		Payload: daemonAppendMessagesMutationPayload{
			SessionID: sessionID,
			Plan:      plan,
			Messages:  append([]agentMessage(nil), messages...),
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
		Payload: daemonRecordUserMessageMutationPayload{
			SessionID:      sessionID,
			Prompt:         prompt,
			Operator:       operator,
			Language:       language,
			OperationLevel: operationLevel,
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
	payload, ok := result.Payload.(daemonRecordUserMessageMutationResultPayload)
	if !ok {
		return daemonRecordUserMessageMutationResultPayload{}, true, errors.New("daemon: record user message mutator returned invalid payload")
	}
	return payload, true, nil
}

func (m *agentManager) runRuntimeResolveAuthorizationMutation(ctx context.Context, sessionID, decision string, req agentAuthorizationDecisionRequest, operator, language, operationLevel string) (daemonResolveAuthorizationMutationResultPayload, bool, error) {
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
			Origin:         "operator",
			OperationLevel: operationLevel,
			Metadata: map[string]any{
				"mutator": "session_mutation",
				"kind":    string(runtime.SessionMutationResolveAuthorization),
			},
		},
		Kind: runtime.SessionMutationResolveAuthorization,
		Payload: daemonResolveAuthorizationMutationPayload{
			SessionID:              sessionID,
			Decision:               decision,
			AuthorizationID:        strings.TrimSpace(req.AuthorizationID),
			Reason:                 strings.TrimSpace(req.Reason),
			Operator:               operator,
			Language:               language,
			OperationLevel:         operationLevel,
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
	payload, ok := result.Payload.(daemonResolveAuthorizationMutationResultPayload)
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

func (m *agentManager) resolveAuthorizationLegacy(ctx context.Context, sessionID, decision, authorizationID, reason, operator, language, operationLevel string, operationLevelProvided bool) (daemonResolveAuthorizationMutationResultPayload, error) {
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
	m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
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
