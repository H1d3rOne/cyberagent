package daemon

import (
	"context"
	"errors"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonLLMTurnPayload struct {
	Loop runtime.AgentLoopRequest
}

type daemonAgentLLMTurnRunner struct {
	manager *agentManager
}

func (r daemonAgentLLMTurnRunner) RunLLMTurn(ctx context.Context, req runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
	if r.manager == nil {
		err := errors.New("daemon: llm turn runner has no manager")
		return daemonLLMTurnErrorResult(req, err), err
	}
	payload, ok := req.Payload.(daemonLLMTurnPayload)
	if !ok {
		err := errors.New("daemon: invalid llm turn payload")
		return daemonLLMTurnErrorResult(req, err), err
	}

	loop := payload.Loop.Clone()
	messages, toolCalls, err := r.manager.runAgentLLMToolLoopAgentLoop(ctx, loop)

	status := runtime.TurnResultFinished
	reason := "legacy daemon LLM tool loop executed"
	if err != nil {
		status = runtime.TurnResultFailed
		reason = err.Error()
	}
	return runtime.LLMTurnResult{
		Status: status,
		Reason: reason,
		Error:  err,
		Payload: runtime.AgentLoopTurnResultPayload{
			Messages:  runtime.CloneAgentLoopMessages(messages),
			ToolCalls: runtime.CloneAgentLoopToolCalls(toolCalls),
		},
		Data: map[string]any{
			"runner":     "daemon_legacy_llm_tool_loop",
			"session_id": loop.Turn.SessionID,
			"task_id":    loop.Turn.TaskID,
			"run_id":     loop.Turn.RunID,
			"turn_id":    loop.Turn.TurnID,
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
	eng := m.runtimeEngine()
	if eng == nil || eng.LLMTurnRunner() == nil {
		return m.runAgentLLMToolLoop(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc)
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
			"runner":   "llm_tool_loop",
			"language": language,
			"operator": operator,
		},
	}
	loopHistory := agentLoopMessagesFromAgentMessages(history)
	result, err := eng.RunLLMTurn(ctx, runtime.LLMTurnRequest{
		Turn: turn,
		Payload: daemonLLMTurnPayload{
			Loop: runtime.AgentLoopRequest{
				Mode:          runtime.AgentLoopModeLLMToolLoop,
				Turn:          runtime.CloneAgentLoopTurnContext(turn),
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
			},
		},
		Metadata: map[string]any{
			"runner":   "llm_tool_loop",
			"language": language,
			"operator": operator,
		},
	})
	if errors.Is(err, runtime.ErrLLMTurnRunnerNotAttached) {
		return m.runAgentLLMToolLoop(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc)
	}

	payload, ok := agentLoopTurnResultPayloadFromLLMTurnResult(result.Payload)
	if ok {
		return runtime.CloneAgentLoopMessages(payload.Messages), runtime.CloneAgentLoopToolCalls(payload.ToolCalls), err
	}
	if err != nil {
		return nil, nil, err
	}
	return nil, nil, errors.New("daemon: llm turn runner returned invalid payload")
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
