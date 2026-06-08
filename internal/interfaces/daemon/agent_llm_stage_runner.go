package daemon

import (
	"context"
	"errors"
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonAgentLLMStageRunner struct {
	manager *agentManager
}

func (r daemonAgentLLMStageRunner) RunLLMStage(ctx context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
	if r.manager == nil {
		err := errors.New("daemon: llm stage runner has no manager")
		return daemonLLMStageErrorResult(req, err), err
	}

	switch req.Stage {
	case runtime.LLMStagePlannerChat:
		payload, ok := agentLoopLLMStagePayloadFromLLMStageRequest(req.Payload)
		if !ok {
			err := errors.New("daemon: invalid llm planner chat payload")
			return daemonLLMStageErrorResult(req, err), err
		}
		messages, toolCalls, plan, err := r.manager.handleAgentLLMChat(
			ctx,
			payload.Turn.SessionID,
			agentRuntimeTurnRefFromTurn(payload.Turn),
			payload.Language,
			sendMessageRequestFromAgentLoopLLMRequest(payload.LLMRequest),
			agentMessagesFromAgentLoopMessages(payload.History),
			agentMemorySummaryPtrFromAgentLoopMemorySummary(payload.MemorySummary),
		)
		status := runtime.TurnResultFinished
		reason := "legacy daemon LLM planner chat executed"
		if err != nil {
			status = runtime.TurnResultFailed
			reason = err.Error()
		}
		return runtime.LLMStageResult{
			Status:  status,
			Reason:  reason,
			Error:   err,
			Payload: agentLoopLLMStageResultPayloadFromLegacy(messages, toolCalls, plan),
			Data:    daemonLLMStageResultData(req.Stage, payload.Turn),
		}, err
	case runtime.LLMStageToolSynthesis:
		payload, ok := agentLoopLLMStagePayloadFromLLMStageRequest(req.Payload)
		if !ok {
			err := errors.New("daemon: invalid llm tool synthesis payload")
			return daemonLLMStageErrorResult(req, err), err
		}
		messages, toolCalls, plan, err := r.manager.synthesizeAgentToolResults(
			ctx,
			payload.Turn.SessionID,
			agentRuntimeTurnRefFromTurn(payload.Turn),
			payload.Language,
			sendMessageRequestFromAgentLoopLLMRequest(payload.LLMRequest),
			agentMessagesFromAgentLoopMessages(payload.History),
			agentMemorySummaryPtrFromAgentLoopMemorySummary(payload.MemorySummary),
			agentMessagesFromAgentLoopMessages(payload.ToolMessages),
		)
		status := runtime.TurnResultFinished
		reason := "legacy daemon LLM tool synthesis executed"
		if err != nil {
			status = runtime.TurnResultFailed
			reason = err.Error()
		}
		return runtime.LLMStageResult{
			Status:  status,
			Reason:  reason,
			Error:   err,
			Payload: agentLoopLLMStageResultPayloadFromLegacy(messages, toolCalls, plan),
			Data:    daemonLLMStageResultData(req.Stage, payload.Turn),
		}, err
	default:
		err := fmt.Errorf("daemon: unsupported llm stage %q", req.Stage)
		return daemonLLMStageErrorResult(req, err), err
	}
}

func daemonLLMStageErrorResult(req runtime.LLMStageRequest, err error) runtime.LLMStageResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return runtime.LLMStageResult{
		Status: runtime.TurnResultFailed,
		Reason: reason,
		Error:  err,
		Data:   daemonLLMStageResultData(req.Stage, req.Turn),
	}
}

func daemonLLMStageResultData(stage runtime.LLMStage, turn runtime.TurnContext) map[string]any {
	return map[string]any{
		"runner":     "daemon_legacy_llm_stage",
		"stage":      string(stage),
		"session_id": turn.SessionID,
		"task_id":    turn.TaskID,
		"run_id":     turn.RunID,
		"turn_id":    turn.TurnID,
	}
}

func (m *agentManager) runRuntimeAgentLLMPlannerChat(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, *runtime.AgentLoopPlan, error) {
	payload, err := m.runRuntimeAgentLLMStage(ctx, runtime.LLMStagePlannerChat, agentLLMStageDispatchInput{
		SessionID:      sessionID,
		TurnRef:        turnRef,
		Prompt:         prompt,
		Language:       language,
		OperationLevel: operationLevel,
		Request:        req,
		History:        history,
		MemorySummary:  memorySummary,
	}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
		messages, toolCalls, plan, err := m.handleAgentLLMChat(ctx, sessionID, turnRef, language, req, history, memorySummary)
		return messages, toolCalls, plan, err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return runtime.CloneAgentLoopMessages(payload.Messages), runtime.CloneAgentLoopToolCalls(payload.ToolCalls), runtime.CloneAgentLoopPlan(payload.Plan), nil
}

func (m *agentManager) runRuntimeAgentLLMToolSynthesis(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, toolMessages []runtime.AgentLoopMessage) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, *runtime.AgentLoopPlan, error) {
	toolMessages = runtime.CloneAgentLoopMessages(toolMessages)
	payload, err := m.runRuntimeAgentLLMStage(ctx, runtime.LLMStageToolSynthesis, agentLLMStageDispatchInput{
		SessionID:      sessionID,
		TurnRef:        turnRef,
		Prompt:         prompt,
		Language:       language,
		OperationLevel: operationLevel,
		Request:        req,
		History:        history,
		MemorySummary:  memorySummary,
		ToolMessages:   toolMessages,
	}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
		messages, toolCalls, plan, err := m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, history, memorySummary, agentMessagesFromAgentLoopMessages(toolMessages))
		return messages, toolCalls, plan, err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return runtime.CloneAgentLoopMessages(payload.Messages), runtime.CloneAgentLoopToolCalls(payload.ToolCalls), runtime.CloneAgentLoopPlan(payload.Plan), nil
}

type agentLLMStageDispatchInput struct {
	SessionID      string
	TurnRef        agentRuntimeTurnRef
	Prompt         string
	Language       string
	OperationLevel string
	Request        sendAgentMessageRequest
	History        []agentMessage
	MemorySummary  *agentMemorySummary
	ToolMessages   []runtime.AgentLoopMessage
}

func (m *agentManager) runRuntimeAgentLLMStage(ctx context.Context, stage runtime.LLMStage, input agentLLMStageDispatchInput, fallback func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error)) (runtime.AgentLoopLLMStageResultPayload, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.LLMStageRunner() == nil {
		return agentLoopLLMStageResultPayloadFromLegacyFallback(fallback)
	}

	turn := agentLLMStageTurnContext(stage, input)
	result, err := eng.RunLLMStage(ctx, runtime.LLMStageRequest{
		Turn:     turn,
		Stage:    stage,
		Payload:  agentLLMStagePayloadFromDispatchInput(turn, input),
		Metadata: agentLLMStageMetadata(stage),
	})
	if errors.Is(err, runtime.ErrLLMStageRunnerNotAttached) {
		return agentLoopLLMStageResultPayloadFromLegacyFallback(fallback)
	}

	payload, ok := agentLoopLLMStageResultPayloadFromLLMStageResult(result.Payload)
	if ok {
		return payload, err
	}
	if err != nil {
		return runtime.AgentLoopLLMStageResultPayload{}, err
	}
	return runtime.AgentLoopLLMStageResultPayload{}, fmt.Errorf("daemon: llm %s runner returned invalid payload", stage)
}

func agentLLMStageTurnContext(stage runtime.LLMStage, input agentLLMStageDispatchInput) runtime.TurnContext {
	return runtime.TurnContext{
		SessionID:      input.SessionID,
		TaskID:         input.TurnRef.TaskID,
		RunID:          input.TurnRef.RunID,
		TurnID:         input.TurnRef.TurnID,
		Prompt:         input.Prompt,
		Origin:         input.Request.Origin,
		OperationLevel: input.OperationLevel,
		Metadata:       agentLLMStageMetadata(stage),
	}
}

func agentLLMStagePayloadFromDispatchInput(turn runtime.TurnContext, input agentLLMStageDispatchInput) runtime.AgentLoopLLMStagePayload {
	return runtime.AgentLoopLLMStagePayload{
		Turn:          runtime.CloneAgentLoopTurnContext(turn),
		Language:      input.Language,
		LLMRequest:    agentPlannerActionReplanCycleLLMRequestFromMessageRequest(input.Request),
		History:       agentLoopMessagesFromAgentMessages(input.History),
		MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(input.MemorySummary),
		ToolMessages:  runtime.CloneAgentLoopMessages(input.ToolMessages),
	}
}

func agentLLMStageMetadata(stage runtime.LLMStage) map[string]any {
	return map[string]any{
		"runner": "llm_stage",
		"stage":  string(stage),
	}
}

func agentLoopLLMStageResultPayloadFromLegacyFallback(fallback func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error)) (runtime.AgentLoopLLMStageResultPayload, error) {
	if fallback == nil {
		return runtime.AgentLoopLLMStageResultPayload{}, errors.New("daemon: llm stage fallback is not configured")
	}
	messages, toolCalls, plan, err := fallback()
	return agentLoopLLMStageResultPayloadFromLegacy(messages, toolCalls, plan), err
}

func agentLoopLLMStagePayloadFromLLMStageRequest(payload any) (runtime.AgentLoopLLMStagePayload, bool) {
	switch value := payload.(type) {
	case runtime.AgentLoopLLMStagePayload:
		return value.Clone(), true
	case *runtime.AgentLoopLLMStagePayload:
		if value == nil {
			return runtime.AgentLoopLLMStagePayload{}, false
		}
		return value.Clone(), true
	default:
		return runtime.AgentLoopLLMStagePayload{}, false
	}
}

func agentLoopLLMStageResultPayloadFromLLMStageResult(payload any) (runtime.AgentLoopLLMStageResultPayload, bool) {
	switch value := payload.(type) {
	case runtime.AgentLoopLLMStageResultPayload:
		return value.Clone(), true
	case *runtime.AgentLoopLLMStageResultPayload:
		if value == nil {
			return runtime.AgentLoopLLMStageResultPayload{}, false
		}
		return value.Clone(), true
	default:
		return runtime.AgentLoopLLMStageResultPayload{}, false
	}
}

func agentLoopLLMStageResultPayloadFromLegacy(messages []agentMessage, toolCalls []agentToolCall, plan *agentPlannerResponse) runtime.AgentLoopLLMStageResultPayload {
	return runtime.AgentLoopLLMStageResultPayload{
		Messages:  agentLoopMessagesFromAgentMessages(messages),
		ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(toolCalls),
		Plan:      agentLoopPlanPtrFromAgentPlannerResponse(plan),
	}
}

func agentLoopPlanPtrFromAgentPlannerResponse(plan *agentPlannerResponse) *runtime.AgentLoopPlan {
	if plan == nil {
		return nil
	}
	converted := agentPlannerActionReplanCyclePlanItemFromAgentPlannerResponse(*plan)
	return &converted
}

func agentPlannerResponsePtrFromAgentLoopPlan(plan *runtime.AgentLoopPlan) *agentPlannerResponse {
	if plan == nil {
		return nil
	}
	return agentPlannerResponseFromAgentLoopPlan(*plan)
}
