package daemon

import (
	"context"
	"errors"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

const daemonTurnExecutionDeterministicRoute = "deterministic_route"

type daemonDeterministicRouteTurnPayload struct {
	SessionID      string
	Prompt         string
	Language       string
	Operator       string
	OperationLevel string
	Services       routerServices
}

type daemonDeterministicRouteTurnResultPayload struct {
	Messages  []agentMessage
	ToolCalls []agentToolCall
}

type daemonAgentTurnExecutor struct {
	manager *agentManager
}

func (e daemonAgentTurnExecutor) ExecuteTurn(ctx context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
	if turn.Metadata != nil && turn.Metadata["kind"] == daemonTurnExecutionDeterministicRoute {
		return e.executeDeterministicRouteTurn(ctx, turn)
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
	payload, ok := turn.Payload.(daemonDeterministicRouteTurnPayload)
	if !ok {
		err := errors.New("daemon: invalid deterministic route turn payload")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	messages, toolCalls, err := e.manager.routePrompt(ctx, payload.SessionID, payload.Prompt, payload.Language, payload.Operator, payload.OperationLevel, payload.Services)
	if err != nil {
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	resultPayload := daemonDeterministicRouteTurnResultPayload{
		Messages:  messages,
		ToolCalls: toolCalls,
	}
	return runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Reason:  "legacy daemon deterministic route executed",
		Payload: resultPayload,
		Data: map[string]any{
			"executor":         "daemon_legacy_adapter",
			"kind":             daemonTurnExecutionDeterministicRoute,
			"session_id":       payload.SessionID,
			"task_id":          turn.TaskID,
			"run_id":           turn.RunID,
			"turn_id":          turn.TurnID,
			"message_count":    len(messages),
			"tool_call_count":  len(toolCalls),
			"operation_level":  payload.OperationLevel,
			"manager_attached": true,
		},
	}, nil
}

func daemonTurnExecutionErrorResult(turn runtime.TurnContext, err error) runtime.TurnResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return runtime.TurnResult{
		Status: runtime.TurnResultFailed,
		Reason: reason,
		Error:  err,
		Data: map[string]any{
			"executor":   "daemon_legacy_adapter",
			"kind":       daemonTurnExecutionDeterministicRoute,
			"session_id": turn.SessionID,
			"task_id":    turn.TaskID,
			"run_id":     turn.RunID,
			"turn_id":    turn.TurnID,
		},
	}
}

func (m *agentManager) runRuntimeDeterministicRoute(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, svc routerServices) (daemonDeterministicRouteTurnResultPayload, bool, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.TurnExecutor() == nil {
		return daemonDeterministicRouteTurnResultPayload{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := eng.ExecuteTurn(ctx, runtime.TurnContext{
		SessionID:      sessionID,
		TaskID:         turnRef.TaskID,
		RunID:          turnRef.RunID,
		TurnID:         turnRef.TurnID,
		Prompt:         prompt,
		Origin:         daemonTurnExecutionDeterministicRoute,
		OperationLevel: operationLevel,
		TurnIndex:      1,
		Payload: daemonDeterministicRouteTurnPayload{
			SessionID:      sessionID,
			Prompt:         prompt,
			Language:       language,
			Operator:       operator,
			OperationLevel: operationLevel,
			Services:       svc,
		},
		Metadata: map[string]any{
			"executor":        "turn_execution",
			"kind":            daemonTurnExecutionDeterministicRoute,
			"operation_level": operationLevel,
		},
	})
	if errors.Is(err, runtime.ErrTurnExecutorNotAttached) ||
		errors.Is(result.Error, runtime.ErrTurnExecutorNotAttached) {
		return daemonDeterministicRouteTurnResultPayload{}, false, nil
	}
	if err != nil {
		return daemonDeterministicRouteTurnResultPayload{}, true, err
	}
	if result.Status == runtime.TurnResultBlocked ||
		(result.Status == runtime.TurnResultRunning && result.Payload == nil) {
		return daemonDeterministicRouteTurnResultPayload{}, false, nil
	}
	payload, ok := result.Payload.(daemonDeterministicRouteTurnResultPayload)
	if !ok {
		return daemonDeterministicRouteTurnResultPayload{}, true, errors.New("daemon: deterministic route turn executor returned invalid payload")
	}
	return payload, true, nil
}

func (m *agentManager) routePromptWithRuntimeDeterministicRoute(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	routed, handled, err := m.runRuntimeDeterministicRoute(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc)
	if err != nil {
		return nil, nil, err
	}
	if handled {
		return routed.Messages, routed.ToolCalls, nil
	}
	return m.routePrompt(ctx, sessionID, prompt, language, operator, operationLevel, svc)
}
