package daemon

import (
	"context"
	"errors"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonAgentLLMTurnRunner struct {
	manager *agentManager
}

func (r daemonAgentLLMTurnRunner) RunLLMTurn(ctx context.Context, req runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
	if r.manager == nil {
		err := errors.New("daemon: llm turn runner has no manager")
		return daemonLLMTurnErrorResult(req, err), err
	}
	loop, ok := req.Payload.(runtime.AgentLoopRequest)
	if !ok {
		err := errors.New("daemon: invalid llm turn payload")
		return daemonLLMTurnErrorResult(req, err), err
	}
	loop = loop.Clone()
	loopResult, err := r.manager.runRuntimeAgentLoopResult(ctx, loop)

	status := runtime.TurnResultFinished
	reason := "legacy daemon LLM tool loop executed"
	if err != nil {
		status = runtime.TurnResultFailed
		reason = err.Error()
	}
	return runtime.LLMTurnResult{
		Status:  status,
		Reason:  reason,
		Error:   err,
		Payload: agentLoopTurnResultPayloadFromAgentLoopResult(loopResult),
		Data: map[string]any{
			"runner":      "daemon_legacy_llm_tool_loop",
			"session_id":  loop.Turn.SessionID,
			"task_id":     loop.Turn.TaskID,
			"run_id":      loop.Turn.RunID,
			"turn_id":     loop.Turn.TurnID,
			"stop_reason": loopResult.StopReason,
			"loop_count":  loopResult.LoopCount,
			"max_loops":   agentLoopResultMaxLoops(loopResult),
		},
	}, err
}

func daemonLLMTurnErrorResult(req runtime.LLMTurnRequest, err error) runtime.LLMTurnResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return runtime.LLMTurnResult{
		Status: runtime.TurnResultFailed,
		Reason: reason,
		Error:  err,
		Data: map[string]any{
			"runner":     "daemon_legacy_llm_tool_loop",
			"session_id": req.Turn.SessionID,
			"task_id":    req.Turn.TaskID,
			"run_id":     req.Turn.RunID,
			"turn_id":    req.Turn.TurnID,
		},
	}
}

func (m *agentManager) runRuntimeLLMToolLoop(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, error) {
	return m.runRuntimeLLMToolLoopAgentLoop(ctx, agentLLMToolLoopRequestFromLegacy(sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc))
}

func (m *agentManager) runRuntimeLLMToolLoopAgentLoop(ctx context.Context, loopReq runtime.AgentLoopRequest) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, error) {
	loopReq = normalizeAgentLLMToolLoopRequest(loopReq)
	turn := agentLLMToolLoopTurnContextFromRequest(loopReq)
	loopReq.Turn = runtime.CloneAgentLoopTurnContext(turn)
	loopResult, err := m.runRuntimeAgentLoopResult(ctx, loopReq)
	return runtime.CloneAgentLoopMessages(loopResult.Messages), runtime.CloneAgentLoopToolCalls(loopResult.ToolCalls), err
}

func agentLLMToolLoopRequestFromLegacy(sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices) runtime.AgentLoopRequest {
	loopHistory := agentLoopMessagesFromAgentMessages(history)
	turn := agentPlannerActionReplanCycleTurnContext(sessionID, turnRef, prompt, req.Origin, operationLevel)
	turn.Metadata = map[string]any{
		"runner":   "llm_tool_loop",
		"language": language,
		"operator": operator,
	}
	return runtime.AgentLoopRequest{
		Mode:          runtime.AgentLoopModeLLMToolLoop,
		Turn:          turn,
		Language:      language,
		Operator:      operator,
		LLMRequest:    agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req),
		History:       runtime.CloneAgentLoopMessages(loopHistory),
		MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(memorySummary),
		State: runtime.AgentLoopState{
			LoopHistory: runtime.CloneAgentLoopMessages(loopHistory),
		},
		Authorization: agentPlannerActionReplanCycleAuthorizationFromMessageRequest(req),
		ToolServices:  agentLoopToolServiceBindingFromRouterServices(svc),
		MaxLoops:      reqMaxAgentReplans(req),
		Metadata: agentPlannerActionReplanCycleMetadataFromOptions(agentPlannerActionReplanCycleOptions{
			ActionAppendOrigin:                      "planner_loop",
			AppendActionToolCallsBeforeEmptyMessage: true,
		}),
	}
}

func agentLLMToolLoopTurnContextFromRequest(loopReq runtime.AgentLoopRequest) runtime.TurnContext {
	turn := runtime.CloneAgentLoopTurnContext(loopReq.Turn)
	metadata := runtime.CloneAgentLoopMapStringAny(turn.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["runner"] = "llm_tool_loop"
	metadata["language"] = loopReq.Language
	metadata["operator"] = loopReq.Operator
	turn.Metadata = metadata
	return turn
}

func agentLoopMemorySummaryPtrFromAgentMemorySummary(summary *agentMemorySummary) *runtime.AgentLoopMemorySummary {
	if summary == nil {
		return nil
	}
	converted := agentLoopMemorySummaryFromAgentMemorySummary(*summary)
	return &converted
}

func agentMemorySummaryPtrFromAgentLoopMemorySummary(summary *runtime.AgentLoopMemorySummary) *agentMemorySummary {
	if summary == nil {
		return nil
	}
	return agentMemorySummaryFromAgentLoopMemorySummary(*summary)
}

func agentLoopTurnResultPayloadFromLLMTurnResult(payload any) (runtime.AgentLoopTurnResultPayload, bool) {
	switch value := payload.(type) {
	case runtime.AgentLoopTurnResultPayload:
		return value.Clone(), true
	case *runtime.AgentLoopTurnResultPayload:
		if value == nil {
			return runtime.AgentLoopTurnResultPayload{}, false
		}
		return value.Clone(), true
	default:
		return runtime.AgentLoopTurnResultPayload{}, false
	}
}
