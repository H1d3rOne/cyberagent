package daemon

import (
	"context"
	"errors"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonLLMTurnPayload struct {
	SessionID      string
	TurnRef        agentRuntimeTurnRef
	Prompt         string
	Language       string
	Operator       string
	OperationLevel string
	Req            sendAgentMessageRequest
	History        []agentMessage
	MemorySummary  *agentMemorySummary
	Services       routerServices
}

type daemonLLMTurnResultPayload struct {
	Messages  []agentMessage
	ToolCalls []agentToolCall
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

	messages, toolCalls, err := r.manager.runAgentLLMToolLoop(
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
		payload.Services,
	)

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
		Payload: daemonLLMTurnResultPayload{
			Messages:  messages,
			ToolCalls: toolCalls,
		},
		Data: map[string]any{
			"runner":     "daemon_legacy_llm_tool_loop",
			"session_id": payload.SessionID,
			"task_id":    payload.TurnRef.TaskID,
			"run_id":     payload.TurnRef.RunID,
			"turn_id":    payload.TurnRef.TurnID,
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

func (m *agentManager) runRuntimeLLMToolLoop(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.LLMTurnRunner() == nil {
		return m.runAgentLLMToolLoop(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc)
	}

	result, err := eng.RunLLMTurn(ctx, runtime.LLMTurnRequest{
		Turn: runtime.TurnContext{
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
		},
		Payload: daemonLLMTurnPayload{
			SessionID:      sessionID,
			TurnRef:        turnRef,
			Prompt:         prompt,
			Language:       language,
			Operator:       operator,
			OperationLevel: operationLevel,
			Req:            req,
			History:        history,
			MemorySummary:  memorySummary,
			Services:       svc,
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

	payload, ok := result.Payload.(daemonLLMTurnResultPayload)
	if ok {
		return payload.Messages, payload.ToolCalls, err
	}
	if err != nil {
		return nil, nil, err
	}
	return nil, nil, errors.New("daemon: llm turn runner returned invalid payload")
}
