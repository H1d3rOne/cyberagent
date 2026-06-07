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

func (m *agentManager) runRuntimeAgentLLMPlannerChat(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary) ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.LLMStageRunner() == nil {
		return m.handleAgentLLMChat(ctx, sessionID, turnRef, language, req, history, memorySummary)
	}

	turn := runtime.TurnContext{
		SessionID:      sessionID,
		TaskID:         turnRef.TaskID,
		RunID:          turnRef.RunID,
		TurnID:         turnRef.TurnID,
		Prompt:         prompt,
		Origin:         req.Origin,
		OperationLevel: operationLevel,
		Metadata: map[string]any{
			"runner": "llm_stage",
			"stage":  string(runtime.LLMStagePlannerChat),
		},
	}
	result, err := eng.RunLLMStage(ctx, runtime.LLMStageRequest{
		Turn:  turn,
		Stage: runtime.LLMStagePlannerChat,
		Payload: runtime.AgentLoopLLMStagePayload{
			Turn:          runtime.CloneAgentLoopTurnContext(turn),
			Language:      language,
			LLMRequest:    agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req),
			History:       agentLoopMessagesFromAgentMessages(history),
			MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(memorySummary),
		},
		Metadata: map[string]any{
			"runner": "llm_stage",
			"stage":  string(runtime.LLMStagePlannerChat),
		},
	})
	if errors.Is(err, runtime.ErrLLMStageRunnerNotAttached) {
		return m.handleAgentLLMChat(ctx, sessionID, turnRef, language, req, history, memorySummary)
	}

	payload, ok := agentLoopLLMStageResultPayloadFromLLMStageResult(result.Payload)
	if ok {
		return agentMessagesFromAgentLoopMessages(payload.Messages), agentLoopToolCallItemsToAgentToolCalls(payload.ToolCalls), agentPlannerResponsePtrFromAgentLoopPlan(payload.Plan), err
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return nil, nil, nil, errors.New("daemon: llm planner chat runner returned invalid payload")
}

func (m *agentManager) runRuntimeAgentLLMToolSynthesis(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, toolMessages []agentMessage) ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.LLMStageRunner() == nil {
		return m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, history, memorySummary, toolMessages)
	}

	turn := runtime.TurnContext{
		SessionID:      sessionID,
		TaskID:         turnRef.TaskID,
		RunID:          turnRef.RunID,
		TurnID:         turnRef.TurnID,
		Prompt:         prompt,
		Origin:         req.Origin,
		OperationLevel: operationLevel,
		Metadata: map[string]any{
			"runner": "llm_stage",
			"stage":  string(runtime.LLMStageToolSynthesis),
		},
	}
	result, err := eng.RunLLMStage(ctx, runtime.LLMStageRequest{
		Turn:  turn,
		Stage: runtime.LLMStageToolSynthesis,
		Payload: runtime.AgentLoopLLMStagePayload{
			Turn:          runtime.CloneAgentLoopTurnContext(turn),
			Language:      language,
			LLMRequest:    agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req),
			History:       agentLoopMessagesFromAgentMessages(history),
			MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(memorySummary),
			ToolMessages:  agentLoopMessagesFromAgentMessages(toolMessages),
		},
		Metadata: map[string]any{
			"runner": "llm_stage",
			"stage":  string(runtime.LLMStageToolSynthesis),
		},
	})
	if errors.Is(err, runtime.ErrLLMStageRunnerNotAttached) {
		return m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, history, memorySummary, toolMessages)
	}

	payload, ok := agentLoopLLMStageResultPayloadFromLLMStageResult(result.Payload)
	if ok {
		return agentMessagesFromAgentLoopMessages(payload.Messages), agentLoopToolCallItemsToAgentToolCalls(payload.ToolCalls), agentPlannerResponsePtrFromAgentLoopPlan(payload.Plan), err
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return nil, nil, nil, errors.New("daemon: llm tool synthesis runner returned invalid payload")
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
