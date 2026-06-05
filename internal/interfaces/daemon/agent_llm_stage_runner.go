package daemon

import (
	"context"
	"errors"
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonLLMPlannerChatPayload struct {
	SessionID      string
	TurnRef        agentRuntimeTurnRef
	Prompt         string
	Language       string
	OperationLevel string
	Req            sendAgentMessageRequest
	History        []agentMessage
	MemorySummary  *agentMemorySummary
}

type daemonLLMPlannerChatResultPayload struct {
	Messages  []agentMessage
	ToolCalls []agentToolCall
	Plan      *agentPlannerResponse
}

type daemonLLMToolSynthesisPayload struct {
	SessionID      string
	TurnRef        agentRuntimeTurnRef
	Prompt         string
	Language       string
	OperationLevel string
	Req            sendAgentMessageRequest
	History        []agentMessage
	MemorySummary  *agentMemorySummary
	ToolMessages   []agentMessage
}

type daemonLLMToolSynthesisResultPayload struct {
	Messages  []agentMessage
	ToolCalls []agentToolCall
	Plan      *agentPlannerResponse
}

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
		payload, ok := req.Payload.(daemonLLMPlannerChatPayload)
		if !ok {
			err := errors.New("daemon: invalid llm planner chat payload")
			return daemonLLMStageErrorResult(req, err), err
		}
		messages, toolCalls, plan, err := r.manager.handleAgentLLMChat(
			ctx,
			payload.SessionID,
			payload.TurnRef,
			payload.Language,
			payload.Req,
			payload.History,
			payload.MemorySummary,
		)
		status := runtime.TurnResultFinished
		reason := "legacy daemon LLM planner chat executed"
		if err != nil {
			status = runtime.TurnResultFailed
			reason = err.Error()
		}
		return runtime.LLMStageResult{
			Status: status,
			Reason: reason,
			Error:  err,
			Payload: daemonLLMPlannerChatResultPayload{
				Messages:  messages,
				ToolCalls: toolCalls,
				Plan:      plan,
			},
			Data: map[string]any{
				"runner":     "daemon_legacy_llm_stage",
				"stage":      string(req.Stage),
				"session_id": payload.SessionID,
				"task_id":    payload.TurnRef.TaskID,
				"run_id":     payload.TurnRef.RunID,
				"turn_id":    payload.TurnRef.TurnID,
			},
		}, err
	case runtime.LLMStageToolSynthesis:
		payload, ok := req.Payload.(daemonLLMToolSynthesisPayload)
		if !ok {
			err := errors.New("daemon: invalid llm tool synthesis payload")
			return daemonLLMStageErrorResult(req, err), err
		}
		messages, toolCalls, plan, err := r.manager.synthesizeAgentToolResults(
			ctx,
			payload.SessionID,
			payload.TurnRef,
			payload.Language,
			payload.Req,
			payload.History,
			payload.MemorySummary,
			payload.ToolMessages,
		)
		status := runtime.TurnResultFinished
		reason := "legacy daemon LLM tool synthesis executed"
		if err != nil {
			status = runtime.TurnResultFailed
			reason = err.Error()
		}
		return runtime.LLMStageResult{
			Status: status,
			Reason: reason,
			Error:  err,
			Payload: daemonLLMToolSynthesisResultPayload{
				Messages:  messages,
				ToolCalls: toolCalls,
				Plan:      plan,
			},
			Data: map[string]any{
				"runner":     "daemon_legacy_llm_stage",
				"stage":      string(req.Stage),
				"session_id": payload.SessionID,
				"task_id":    payload.TurnRef.TaskID,
				"run_id":     payload.TurnRef.RunID,
				"turn_id":    payload.TurnRef.TurnID,
			},
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
		Data: map[string]any{
			"runner":     "daemon_legacy_llm_stage",
			"stage":      string(req.Stage),
			"session_id": req.Turn.SessionID,
			"task_id":    req.Turn.TaskID,
			"run_id":     req.Turn.RunID,
			"turn_id":    req.Turn.TurnID,
		},
	}
}

func (m *agentManager) runRuntimeAgentLLMPlannerChat(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary) ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
	eng := m.runtimeEngine()
	if eng == nil || eng.LLMStageRunner() == nil {
		return m.handleAgentLLMChat(ctx, sessionID, turnRef, language, req, history, memorySummary)
	}

	result, err := eng.RunLLMStage(ctx, runtime.LLMStageRequest{
		Turn: runtime.TurnContext{
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
		},
		Stage: runtime.LLMStagePlannerChat,
		Payload: daemonLLMPlannerChatPayload{
			SessionID:      sessionID,
			TurnRef:        turnRef,
			Prompt:         prompt,
			Language:       language,
			OperationLevel: operationLevel,
			Req:            req,
			History:        history,
			MemorySummary:  memorySummary,
		},
		Metadata: map[string]any{
			"runner": "llm_stage",
			"stage":  string(runtime.LLMStagePlannerChat),
		},
	})
	if errors.Is(err, runtime.ErrLLMStageRunnerNotAttached) {
		return m.handleAgentLLMChat(ctx, sessionID, turnRef, language, req, history, memorySummary)
	}

	payload, ok := result.Payload.(daemonLLMPlannerChatResultPayload)
	if ok {
		return payload.Messages, payload.ToolCalls, payload.Plan, err
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

	result, err := eng.RunLLMStage(ctx, runtime.LLMStageRequest{
		Turn: runtime.TurnContext{
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
		},
		Stage: runtime.LLMStageToolSynthesis,
		Payload: daemonLLMToolSynthesisPayload{
			SessionID:      sessionID,
			TurnRef:        turnRef,
			Prompt:         prompt,
			Language:       language,
			OperationLevel: operationLevel,
			Req:            req,
			History:        history,
			MemorySummary:  memorySummary,
			ToolMessages:   toolMessages,
		},
		Metadata: map[string]any{
			"runner": "llm_stage",
			"stage":  string(runtime.LLMStageToolSynthesis),
		},
	})
	if errors.Is(err, runtime.ErrLLMStageRunnerNotAttached) {
		return m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, history, memorySummary, toolMessages)
	}

	payload, ok := result.Payload.(daemonLLMToolSynthesisResultPayload)
	if ok {
		return payload.Messages, payload.ToolCalls, payload.Plan, err
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return nil, nil, nil, errors.New("daemon: llm tool synthesis runner returned invalid payload")
}
