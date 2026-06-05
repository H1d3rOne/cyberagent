package daemon

import (
	"context"
	"errors"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
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

type daemonSessionMutator struct {
	manager *agentManager
}

func (r daemonSessionMutator) MutateSession(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
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
