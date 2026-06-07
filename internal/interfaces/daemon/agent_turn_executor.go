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

type daemonDeterministicRouteTurnPayload struct {
	SessionID      string
	Prompt         string
	Language       string
	Operator       string
	OperationLevel string
	Services       runtime.AgentLoopToolServiceBinding
}

type daemonDeterministicRouteTurnResultPayload struct {
	Messages  []agentMessage
	ToolCalls []agentToolCall
}

type daemonReplanActionContinuationTurnPayload struct {
	SessionID      string
	TurnRef        agentRuntimeTurnRef
	Prompt         string
	Language       string
	Operator       string
	OperationLevel string
	Req            sendAgentMessageRequest
	History        []agentMessage
	MemorySummary  *agentMemorySummary
	Services       runtime.AgentLoopToolServiceBinding
	Plan           *agentPlannerResponse
	MaxLoops       int
}

type daemonReplanActionContinuationTurnResultPayload struct {
	Messages  []agentMessage
	ToolCalls []agentToolCall
}

type daemonPlannerActionExecutionTurnPayload struct {
	SessionID            string
	Language             string
	Operator             string
	OperationLevel       string
	Services             runtime.AgentLoopToolServiceBinding
	Action               runtime.AgentLoopAction
	AuthProfiles         []agentHTTPAuthProfile
	DefaultAuthProfileID string
}

type daemonPlannerActionExecutionTurnResultPayload struct {
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
	payload, ok := turn.Payload.(daemonDeterministicRouteTurnPayload)
	if !ok {
		err := errors.New("daemon: invalid deterministic route turn payload")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	messages, toolCalls, err := e.manager.routePromptForTurn(ctx, payload.SessionID, agentRuntimeTurnRefFromTurn(turn), payload.Prompt, payload.Language, payload.Operator, payload.OperationLevel, legacyRouterServicesFromAgentLoopToolServiceBinding(payload.Services))
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

func (e daemonAgentTurnExecutor) executeReplanActionContinuationTurn(ctx context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
	if e.manager == nil {
		err := errors.New("daemon: turn executor has no manager")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	payload, ok := turn.Payload.(daemonReplanActionContinuationTurnPayload)
	if !ok {
		err := errors.New("daemon: invalid replan action continuation turn payload")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	messages, toolCalls := e.manager.runAgentReplanActionContinuation(
		ctx,
		payload.SessionID,
		payload.TurnRef,
		payload.Prompt,
		payload.Language,
		payload.Operator,
		payload.OperationLevel,
		payload.Req,
		payload.History,
		payload.MemorySummary,
		legacyRouterServicesFromAgentLoopToolServiceBinding(payload.Services),
		payload.Plan,
		payload.MaxLoops,
	)
	resultPayload := daemonReplanActionContinuationTurnResultPayload{
		Messages:  messages,
		ToolCalls: toolCalls,
	}
	return runtime.TurnResult{
		Status:  runtime.TurnResultFinished,
		Reason:  "legacy daemon replan action continuation executed",
		Payload: resultPayload,
		Data: map[string]any{
			"executor":         "daemon_legacy_adapter",
			"kind":             daemonTurnExecutionReplanActionContinuation,
			"session_id":       payload.SessionID,
			"task_id":          turn.TaskID,
			"run_id":           turn.RunID,
			"turn_id":          turn.TurnID,
			"message_count":    len(messages),
			"tool_call_count":  len(toolCalls),
			"operation_level":  payload.OperationLevel,
			"max_loops":        payload.MaxLoops,
			"manager_attached": true,
		},
	}, nil
}

func (e daemonAgentTurnExecutor) executePlannerActionExecutionTurn(ctx context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
	if e.manager == nil {
		err := errors.New("daemon: turn executor has no manager")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	payload, ok := turn.Payload.(daemonPlannerActionExecutionTurnPayload)
	if !ok {
		err := errors.New("daemon: invalid planner action execution turn payload")
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	action := agentPlannerActionFromAgentLoopAction(payload.Action)
	messages, toolCalls, err := e.manager.executeAgentPlannerAction(
		ctx,
		payload.SessionID,
		payload.Language,
		payload.Operator,
		payload.OperationLevel,
		legacyRouterServicesFromAgentLoopToolServiceBinding(payload.Services),
		action,
		payload.AuthProfiles,
		payload.DefaultAuthProfileID,
	)
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
		Payload: daemonPlannerActionExecutionTurnResultPayload{
			Messages:  messages,
			ToolCalls: toolCalls,
		},
		Data: map[string]any{
			"executor":         "daemon_legacy_adapter",
			"kind":             daemonTurnExecutionPlannerActionExecution,
			"session_id":       payload.SessionID,
			"task_id":          turn.TaskID,
			"run_id":           turn.RunID,
			"turn_id":          turn.TurnID,
			"action_name":      normalizeAgentPlannerActionName(payload.Action.Name),
			"message_count":    len(messages),
			"tool_call_count":  len(toolCalls),
			"operation_level":  payload.OperationLevel,
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
			Services:       agentLoopToolServiceBindingFromRouterServices(svc),
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
	return m.routePromptForTurn(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc)
}

func (m *agentManager) runRuntimeReplanActionContinuation(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices, plan *agentPlannerResponse, maxLoops int) ([]agentMessage, []agentToolCall) {
	eng := m.runtimeEngine()
	if eng == nil || eng.TurnExecutor() == nil {
		return m.runAgentReplanActionContinuation(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc, plan, maxLoops)
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
		Origin:         daemonTurnExecutionReplanActionContinuation,
		OperationLevel: operationLevel,
		TurnIndex:      1,
		Payload: daemonReplanActionContinuationTurnPayload{
			SessionID:      sessionID,
			TurnRef:        turnRef,
			Prompt:         prompt,
			Language:       language,
			Operator:       operator,
			OperationLevel: operationLevel,
			Req:            req,
			History:        history,
			MemorySummary:  memorySummary,
			Services:       agentLoopToolServiceBindingFromRouterServices(svc),
			Plan:           plan,
			MaxLoops:       maxLoops,
		},
		Metadata: map[string]any{
			"executor":        "turn_execution",
			"kind":            daemonTurnExecutionReplanActionContinuation,
			"operation_level": operationLevel,
			"max_loops":       maxLoops,
		},
	})
	if errors.Is(err, runtime.ErrTurnExecutorNotAttached) ||
		errors.Is(result.Error, runtime.ErrTurnExecutorNotAttached) ||
		result.Status == runtime.TurnResultBlocked ||
		(result.Status == runtime.TurnResultRunning && result.Payload == nil) {
		return m.runAgentReplanActionContinuation(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc, plan, maxLoops)
	}
	if err != nil {
		return m.runAgentReplanActionContinuation(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc, plan, maxLoops)
	}
	payload, ok := result.Payload.(daemonReplanActionContinuationTurnResultPayload)
	if !ok {
		return m.runAgentReplanActionContinuation(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc, plan, maxLoops)
	}
	return payload.Messages, payload.ToolCalls
}

func (m *agentManager) runRuntimePlannerActionExecution(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, svc routerServices, action runtime.AgentLoopAction, authProfiles []agentHTTPAuthProfile, defaultAuthProfileID string) ([]agentMessage, []agentToolCall, error) {
	action = cloneAgentLoopAction(action)
	legacyAction := agentPlannerActionFromAgentLoopAction(action)
	eng := m.runtimeEngine()
	if eng == nil || eng.TurnExecutor() == nil {
		return m.executeAgentPlannerAction(ctx, sessionID, language, operator, operationLevel, svc, legacyAction, authProfiles, defaultAuthProfileID)
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
		Origin:         daemonTurnExecutionPlannerActionExecution,
		OperationLevel: operationLevel,
		TurnIndex:      1,
		Payload: daemonPlannerActionExecutionTurnPayload{
			SessionID:            sessionID,
			Language:             language,
			Operator:             operator,
			OperationLevel:       operationLevel,
			Services:             agentLoopToolServiceBindingFromRouterServices(svc),
			Action:               action,
			AuthProfiles:         authProfiles,
			DefaultAuthProfileID: defaultAuthProfileID,
		},
		Metadata: map[string]any{
			"executor":        "turn_execution",
			"kind":            daemonTurnExecutionPlannerActionExecution,
			"operation_level": operationLevel,
			"action_name":     normalizeAgentPlannerActionName(action.Name),
		},
	})
	if errors.Is(err, runtime.ErrTurnExecutorNotAttached) ||
		errors.Is(result.Error, runtime.ErrTurnExecutorNotAttached) ||
		result.Status == runtime.TurnResultBlocked ||
		(result.Status == runtime.TurnResultRunning && result.Payload == nil) {
		return m.executeAgentPlannerAction(ctx, sessionID, language, operator, operationLevel, svc, legacyAction, authProfiles, defaultAuthProfileID)
	}
	payload, ok := result.Payload.(daemonPlannerActionExecutionTurnResultPayload)
	if ok {
		return payload.Messages, payload.ToolCalls, err
	}
	if err != nil {
		return nil, nil, err
	}
	return nil, nil, errors.New("daemon: planner action execution turn executor returned invalid payload")
}
