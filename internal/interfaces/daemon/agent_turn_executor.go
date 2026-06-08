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
	messages, toolCalls, err := e.manager.routePromptForTurn(ctx, loopReq.Turn.SessionID, agentRuntimeTurnRefFromTurn(loopReq.Turn), loopReq.Turn.Prompt, loopReq.Language, loopReq.Operator, loopReq.Turn.OperationLevel, legacyRouterServicesFromAgentLoopToolServiceBinding(loopReq.ToolServices))
	if err != nil {
		return daemonTurnExecutionErrorResult(turn, err), err
	}
	resultPayload := runtime.AgentLoopTurnResultPayload{
		Messages:  agentLoopMessagesFromAgentMessages(messages),
		ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(toolCalls),
	}
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
			"message_count":    len(messages),
			"tool_call_count":  len(toolCalls),
			"operation_level":  loopReq.Turn.OperationLevel,
			"manager_attached": true,
		},
	}, nil
}

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
	resultPayload := runtime.AgentLoopTurnResultPayload{
		Messages:   runtime.CloneAgentLoopMessages(loopResult.Messages),
		ToolCalls:  runtime.CloneAgentLoopToolCalls(loopResult.ToolCalls),
		FinalPlan:  runtime.CloneAgentLoopPlan(loopResult.FinalPlan),
		StopReason: loopResult.StopReason,
		LoopCount:  loopResult.LoopCount,
		MaxLoops:   intFromAny(loopResult.Metadata[runtime.AgentLoopEgressMetadataMaxLoops]),
		Metadata:   runtime.CloneAgentLoopMapStringAny(loopResult.Metadata),
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
	action := agentPlannerActionFromAgentLoopAction(payload.Action)
	authorization := loopReq.Authorization.Clone()
	messages, toolCalls, err := e.manager.executeAgentPlannerAction(
		ctx,
		loopReq.Turn.SessionID,
		loopReq.Language,
		loopReq.Operator,
		loopReq.Turn.OperationLevel,
		legacyRouterServicesFromAgentLoopToolServiceBinding(loopReq.ToolServices),
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
		Payload: runtime.AgentLoopTurnResultPayload{
			Messages:  agentLoopMessagesFromAgentMessages(messages),
			ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(toolCalls),
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

func (m *agentManager) runRuntimeDeterministicRoute(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, svc routerServices) (runtime.AgentLoopTurnResultPayload, bool, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.TurnExecutor() == nil {
		return runtime.AgentLoopTurnResultPayload{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loopReq := agentDeterministicRouteRequestFromRuntime(sessionID, turnRef, prompt, language, operator, operationLevel, agentLoopToolServiceBindingFromRouterServices(svc))
	result, err := eng.ExecuteTurn(ctx, agentDeterministicRouteTurnContextFromRequest(loopReq))
	if errors.Is(err, runtime.ErrTurnExecutorNotAttached) ||
		errors.Is(result.Error, runtime.ErrTurnExecutorNotAttached) {
		return runtime.AgentLoopTurnResultPayload{}, false, nil
	}
	if err != nil {
		return runtime.AgentLoopTurnResultPayload{}, true, err
	}
	if result.Status == runtime.TurnResultBlocked ||
		(result.Status == runtime.TurnResultRunning && result.Payload == nil) {
		return runtime.AgentLoopTurnResultPayload{}, false, nil
	}
	payload, ok := result.Payload.(runtime.AgentLoopTurnResultPayload)
	if !ok {
		return runtime.AgentLoopTurnResultPayload{}, true, errors.New("daemon: deterministic route turn executor returned invalid payload")
	}
	return payload.Clone(), true, nil
}

func agentDeterministicRouteRequestFromRuntime(sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding) runtime.AgentLoopRequest {
	return runtime.AgentLoopRequest{
		Turn:         agentPlannerActionReplanCycleTurnContext(sessionID, turnRef, prompt, daemonTurnExecutionDeterministicRoute, operationLevel),
		Language:     language,
		Operator:     operator,
		ToolServices: toolServices.Clone(),
	}
}

func agentDeterministicRouteTurnContextFromRequest(loopReq runtime.AgentLoopRequest) runtime.TurnContext {
	turn := runtime.CloneAgentLoopTurnContext(loopReq.Turn)
	turn.Origin = daemonTurnExecutionDeterministicRoute
	if turn.TurnIndex == 0 {
		turn.TurnIndex = 1
	}
	turn.Payload = loopReq.Clone()
	metadata := runtime.CloneAgentLoopMapStringAny(turn.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["executor"] = "turn_execution"
	metadata["kind"] = daemonTurnExecutionDeterministicRoute
	metadata["operation_level"] = turn.OperationLevel
	turn.Metadata = metadata
	return turn
}

func (m *agentManager) routePromptWithRuntimeDeterministicRoute(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	routed, handled, err := m.runRuntimeDeterministicRoute(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc)
	if err != nil {
		return nil, nil, err
	}
	if handled {
		return agentMessagesFromAgentLoopMessages(routed.Messages), agentLoopToolCallItemsToAgentToolCalls(routed.ToolCalls), nil
	}
	return m.routePromptForTurn(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc)
}

func (m *agentManager) runRuntimeReplanActionContinuation(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices, plan *agentPlannerResponse, maxLoops int) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall) {
	return m.runRuntimeReplanActionContinuationAgentLoop(ctx, agentReplanActionContinuationRequestFromLegacy(sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc, plan, maxLoops))
}

func (m *agentManager) runRuntimeReplanActionContinuationAgentLoop(ctx context.Context, loopReq runtime.AgentLoopRequest) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall) {
	loopReq = normalizeAgentReplanActionContinuationRequest(loopReq)
	eng := m.runtimeEngine()
	if eng == nil || eng.TurnExecutor() == nil {
		return m.runAgentReplanActionContinuationAgentLoop(ctx, loopReq)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := eng.ExecuteTurn(ctx, agentReplanActionContinuationTurnContextFromRequest(loopReq))
	if errors.Is(err, runtime.ErrTurnExecutorNotAttached) ||
		errors.Is(result.Error, runtime.ErrTurnExecutorNotAttached) ||
		result.Status == runtime.TurnResultBlocked ||
		(result.Status == runtime.TurnResultRunning && result.Payload == nil) {
		return m.runAgentReplanActionContinuationAgentLoop(ctx, loopReq)
	}
	if err != nil {
		return m.runAgentReplanActionContinuationAgentLoop(ctx, loopReq)
	}
	payload, ok := result.Payload.(runtime.AgentLoopTurnResultPayload)
	if !ok {
		return m.runAgentReplanActionContinuationAgentLoop(ctx, loopReq)
	}
	payload = payload.Clone()
	return payload.Messages, payload.ToolCalls
}

func agentReplanActionContinuationTurnContextFromRequest(loopReq runtime.AgentLoopRequest) runtime.TurnContext {
	turn := runtime.CloneAgentLoopTurnContext(loopReq.Turn)
	turn.Origin = daemonTurnExecutionReplanActionContinuation
	if turn.TurnIndex == 0 {
		turn.TurnIndex = 1
	}
	turn.Payload = loopReq.Clone()
	metadata := runtime.CloneAgentLoopMapStringAny(turn.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["executor"] = "turn_execution"
	metadata["kind"] = daemonTurnExecutionReplanActionContinuation
	metadata["operation_level"] = turn.OperationLevel
	metadata["max_loops"] = loopReq.MaxLoops
	turn.Metadata = metadata
	return turn
}

func (m *agentManager) runRuntimePlannerActionExecution(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding, action runtime.AgentLoopAction, authorization runtime.AgentLoopAuthorization) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, error) {
	toolServices = toolServices.Clone()
	action = cloneAgentLoopAction(action)
	authorization = authorization.Clone()
	loopReq := agentPlannerActionExecutionRequestFromRuntime(sessionID, turnRef, prompt, language, operator, operationLevel, toolServices, authorization)
	legacyAction := agentPlannerActionFromAgentLoopAction(action)
	legacyAuthProfiles := agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(loopReq.Authorization.AuthProfiles)
	legacyServices := legacyRouterServicesFromAgentLoopToolServiceBinding(loopReq.ToolServices)
	eng := m.runtimeEngine()
	if eng == nil || eng.TurnExecutor() == nil {
		messages, toolCalls, execErr := m.executeAgentPlannerAction(ctx, loopReq.Turn.SessionID, loopReq.Language, loopReq.Operator, loopReq.Turn.OperationLevel, legacyServices, legacyAction, legacyAuthProfiles, loopReq.Authorization.DefaultAuthProfileID)
		return agentLoopMessagesFromAgentMessages(messages), agentLoopToolCallItemsFromAgentToolCalls(toolCalls), execErr
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := eng.ExecuteTurn(ctx, agentPlannerActionExecutionTurnContextFromRequest(loopReq, action))
	if errors.Is(err, runtime.ErrTurnExecutorNotAttached) ||
		errors.Is(result.Error, runtime.ErrTurnExecutorNotAttached) ||
		result.Status == runtime.TurnResultBlocked ||
		(result.Status == runtime.TurnResultRunning && result.Payload == nil) {
		messages, toolCalls, execErr := m.executeAgentPlannerAction(ctx, loopReq.Turn.SessionID, loopReq.Language, loopReq.Operator, loopReq.Turn.OperationLevel, legacyServices, legacyAction, legacyAuthProfiles, loopReq.Authorization.DefaultAuthProfileID)
		return agentLoopMessagesFromAgentMessages(messages), agentLoopToolCallItemsFromAgentToolCalls(toolCalls), execErr
	}
	payload, ok := result.Payload.(runtime.AgentLoopTurnResultPayload)
	if ok {
		return runtime.CloneAgentLoopMessages(payload.Messages), runtime.CloneAgentLoopToolCalls(payload.ToolCalls), err
	}
	if err != nil {
		return nil, nil, err
	}
	return nil, nil, errors.New("daemon: planner action execution turn executor returned invalid payload")
}

func agentPlannerActionExecutionRequestFromRuntime(sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding, authorization runtime.AgentLoopAuthorization) runtime.AgentLoopRequest {
	return runtime.AgentLoopRequest{
		Turn:          agentPlannerActionReplanCycleTurnContext(sessionID, turnRef, prompt, daemonTurnExecutionPlannerActionExecution, operationLevel),
		Language:      language,
		Operator:      operator,
		Authorization: authorization.Clone(),
		ToolServices:  toolServices.Clone(),
	}
}

func agentPlannerActionExecutionTurnContextFromRequest(loopReq runtime.AgentLoopRequest, action runtime.AgentLoopAction) runtime.TurnContext {
	turn := runtime.CloneAgentLoopTurnContext(loopReq.Turn)
	turn.Origin = daemonTurnExecutionPlannerActionExecution
	if turn.TurnIndex == 0 {
		turn.TurnIndex = 1
	}
	turn.Payload = runtime.AgentLoopActionExecutionPayload{
		Request: loopReq.Clone(),
		Action:  runtime.CloneAgentLoopAction(action),
	}
	metadata := runtime.CloneAgentLoopMapStringAny(turn.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["executor"] = "turn_execution"
	metadata["kind"] = daemonTurnExecutionPlannerActionExecution
	metadata["operation_level"] = turn.OperationLevel
	metadata["action_name"] = normalizeAgentPlannerActionName(action.Name)
	turn.Metadata = metadata
	return turn
}
