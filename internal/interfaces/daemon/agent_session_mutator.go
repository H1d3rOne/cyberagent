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
