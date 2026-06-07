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
	Request runtime.AgentLoopRequest
}

type daemonReplanActionContinuationTurnResultPayload struct {
	Messages  []runtime.AgentLoopMessage
	ToolCalls []runtime.AgentLoopToolCall
}

type daemonPlannerActionExecutionTurnPayload struct {
	SessionID      string
	Language       string
	Operator       string
	OperationLevel string
	Services       runtime.AgentLoopToolServiceBinding
	Action         runtime.AgentLoopAction
	Authorization  runtime.AgentLoopAuthorization
}

type daemonPlannerActionExecutionTurnResultPayload struct {
	Messages  []runtime.AgentLoopMessage
	ToolCalls []runtime.AgentLoopToolCall
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
	loopReq := payload.Request.Clone()
	history := runtime.CloneAgentLoopMessages(loopReq.History)
	if len(history) == 0 && len(loopReq.State.LoopHistory) > 0 {
		history = runtime.CloneAgentLoopMessages(loopReq.State.LoopHistory)
	}
	messages, toolCalls := e.manager.runAgentReplanActionContinuation(
		ctx,
		loopReq.Turn.SessionID,
		agentRuntimeTurnRefFromTurn(loopReq.Turn),
		loopReq.Turn.Prompt,
		loopReq.Language,
		loopReq.Operator,
		loopReq.Turn.OperationLevel,
		sendMessageRequestFromAgentLoopLLMRequest(loopReq.LLMRequest),
		agentMessagesFromAgentLoopMessages(history),
		agentMemorySummaryPtrFromAgentLoopMemorySummary(loopReq.MemorySummary),
		legacyRouterServicesFromAgentLoopToolServiceBinding(loopReq.ToolServices),
		agentPlannerResponsePtrFromAgentLoopPlan(loopReq.InitialPlan),
		loopReq.MaxLoops,
	)
	resultPayload := daemonReplanActionContinuationTurnResultPayload{
		Messages:  runtime.CloneAgentLoopMessages(messages),
		ToolCalls: runtime.CloneAgentLoopToolCalls(toolCalls),
	}
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
			"message_count":    len(messages),
			"tool_call_count":  len(toolCalls),
			"operation_level":  loopReq.Turn.OperationLevel,
			"max_loops":        loopReq.MaxLoops,
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
	authorization := payload.Authorization.Clone()
	messages, toolCalls, err := e.manager.executeAgentPlannerAction(
		ctx,
		payload.SessionID,
		payload.Language,
		payload.Operator,
		payload.OperationLevel,
		legacyRouterServicesFromAgentLoopToolServiceBinding(payload.Services),
		action,
		agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(authorization.AuthProfiles),
		authorization.DefaultAuthProfileID,
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
			Messages:  agentLoopMessagesFromAgentMessages(messages),
			ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(toolCalls),
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

func (m *agentManager) runRuntimeReplanActionContinuation(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices, plan *agentPlannerResponse, maxLoops int) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall) {
	eng := m.runtimeEngine()
	if eng == nil || eng.TurnExecutor() == nil {
		return m.runAgentReplanActionContinuation(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc, plan, maxLoops)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loopReq := runtime.AgentLoopRequest{
		Mode:          runtime.AgentLoopModeReplanContinuation,
		Turn:          agentPlannerActionReplanCycleTurnContext(sessionID, turnRef, prompt, req.Origin, operationLevel),
		Language:      language,
		Operator:      operator,
		LLMRequest:    agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req),
		History:       agentLoopMessagesFromAgentMessages(history),
		MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(memorySummary),
		InitialPlan:   agentLoopPlanFromPlannerResponse(plan),
		State: runtime.AgentLoopState{
			LoopHistory: agentLoopMessagesFromAgentMessages(history),
		},
		Authorization: agentPlannerActionReplanCycleAuthorizationFromMessageRequest(req),
		ToolServices:  agentLoopToolServiceBindingFromRouterServices(svc),
		MaxLoops:      maxLoops,
		Metadata: agentPlannerActionReplanCycleMetadataFromOptions(agentPlannerActionReplanCycleOptions{
			ActionAppendOrigin: daemonTurnExecutionReplanActionContinuation,
		}),
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
			Request: loopReq.Clone(),
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
	return runtime.CloneAgentLoopMessages(payload.Messages), runtime.CloneAgentLoopToolCalls(payload.ToolCalls)
}

func (m *agentManager) runRuntimePlannerActionExecution(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding, action runtime.AgentLoopAction, authorization runtime.AgentLoopAuthorization) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, error) {
	toolServices = toolServices.Clone()
	action = cloneAgentLoopAction(action)
	authorization = authorization.Clone()
	legacyAction := agentPlannerActionFromAgentLoopAction(action)
	legacyAuthProfiles := agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(authorization.AuthProfiles)
	legacyServices := legacyRouterServicesFromAgentLoopToolServiceBinding(toolServices)
	eng := m.runtimeEngine()
	if eng == nil || eng.TurnExecutor() == nil {
		messages, toolCalls, execErr := m.executeAgentPlannerAction(ctx, sessionID, language, operator, operationLevel, legacyServices, legacyAction, legacyAuthProfiles, authorization.DefaultAuthProfileID)
		return agentLoopMessagesFromAgentMessages(messages), agentLoopToolCallItemsFromAgentToolCalls(toolCalls), execErr
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
			SessionID:      sessionID,
			Language:       language,
			Operator:       operator,
			OperationLevel: operationLevel,
			Services:       toolServices,
			Action:         action,
			Authorization:  authorization,
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
		messages, toolCalls, execErr := m.executeAgentPlannerAction(ctx, sessionID, language, operator, operationLevel, legacyServices, legacyAction, legacyAuthProfiles, authorization.DefaultAuthProfileID)
		return agentLoopMessagesFromAgentMessages(messages), agentLoopToolCallItemsFromAgentToolCalls(toolCalls), execErr
	}
	payload, ok := result.Payload.(daemonPlannerActionExecutionTurnResultPayload)
	if ok {
		return runtime.CloneAgentLoopMessages(payload.Messages), runtime.CloneAgentLoopToolCalls(payload.ToolCalls), err
	}
	if err != nil {
		return nil, nil, err
	}
	return nil, nil, errors.New("daemon: planner action execution turn executor returned invalid payload")
}
