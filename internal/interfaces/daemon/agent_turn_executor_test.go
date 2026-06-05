package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerAttachEngineWiresDaemonTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	require.NotNil(t, eng.LLMTurnRunner())
	require.NotNil(t, eng.LLMStageRunner())
	require.NotNil(t, eng.SessionMutator())

	result, err := eng.ExecuteTurn(context.Background(), runtime.TurnContext{
		SessionID:      "session-legacy-executor",
		TaskID:         "task-legacy-executor",
		RunID:          "run-legacy-executor",
		TurnID:         "turn-legacy-executor",
		Prompt:         "keep daemon behavior",
		Origin:         "user",
		OperationLevel: "read_only",
	})

	require.NoError(t, err)
	require.Equal(t, runtime.TurnResultRunning, result.Status)
	require.Equal(t, "legacy daemon SendMessage owns turn execution", result.Reason)
	require.Equal(t, "daemon_legacy_adapter", result.Data["executor"])
	require.Equal(t, true, result.Data["manager_attached"])
	require.Equal(t, "session-legacy-executor", result.Data["session_id"])
	require.Equal(t, "turn-legacy-executor", result.Data["turn_id"])
}

func TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-route", Role: "tool", Content: "routed"}
	expectedToolCall := agentToolCall{ID: "tool-route", Name: "daemon.health"}
	called := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		called = true
		require.Equal(t, "session-route-executor", turn.SessionID)
		require.Equal(t, "task-route-executor", turn.TaskID)
		require.Equal(t, "run-route-executor", turn.RunID)
		require.Equal(t, "turn-route-executor", turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionDeterministicRoute, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, "turn_execution", turn.Metadata["executor"])
		require.Equal(t, daemonTurnExecutionDeterministicRoute, turn.Metadata["kind"])

		payload, ok := turn.Payload.(daemonDeterministicRouteTurnPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "session-route-executor", payload.SessionID)
		require.Equal(t, "daemon health", payload.Prompt)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "read_only", payload.OperationLevel)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonDeterministicRouteTurnResultPayload{
				Messages:  []agentMessage{expectedMessage},
				ToolCalls: []agentToolCall{expectedToolCall},
			},
		}, nil
	}))

	result, handled, err := mgr.runRuntimeDeterministicRoute(
		context.Background(),
		"session-route-executor",
		agentRuntimeTurnRef{TaskID: "task-route-executor", RunID: "run-route-executor", TurnID: "turn-route-executor"},
		"daemon health",
		"en",
		"operator-1",
		"read_only",
		routerServices{},
	)

	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, called)
	require.Equal(t, []agentMessage{expectedMessage}, result.Messages)
	require.Equal(t, []agentToolCall{expectedToolCall}, result.ToolCalls)
}

func TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	expectedMessage := agentMessage{ID: "msg-send-route", Role: "tool", Content: "routed through executor"}
	expectedToolCall := agentToolCall{ID: "tool-send-route", Name: "daemon.health"}

	called := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		called = true
		require.Equal(t, sessionID, turn.SessionID)
		require.NotEmpty(t, turn.TaskID)
		require.NotEmpty(t, turn.RunID)
		require.NotEmpty(t, turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionDeterministicRoute, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)

		payload, ok := turn.Payload.(daemonDeterministicRouteTurnPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.SessionID)
		require.Equal(t, "daemon health", payload.Prompt)
		require.Equal(t, "operator-1", payload.Operator)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonDeterministicRouteTurnResultPayload{
				Messages:  []agentMessage{expectedMessage},
				ToolCalls: []agentToolCall{expectedToolCall},
			},
		}, nil
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        "daemon health",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "read_only",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, resp.Messages, 2)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, expectedMessage, resp.Messages[1])
	require.Equal(t, []agentToolCall{expectedToolCall}, resp.ToolCalls)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedMessage.ID, got.Messages[len(got.Messages)-1].ID)
}

func TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	llmErr := errors.New("provider down")
	llmCalled := false
	eng.AttachLLMTurnRunner(runtime.LLMTurnRunnerFunc(func(_ context.Context, req runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
		llmCalled = true
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "hello fallback route", req.Turn.Prompt)
		return runtime.LLMTurnResult{
			Status: runtime.TurnResultFailed,
			Reason: llmErr.Error(),
			Error:  llmErr,
			Payload: daemonLLMTurnResultPayload{
				Messages:  nil,
				ToolCalls: nil,
			},
		}, llmErr
	}))

	expectedMessage := agentMessage{ID: "msg-llm-fallback-route", Role: "agent", Content: "fallback routed through executor"}
	expectedToolCall := agentToolCall{ID: "tool-llm-fallback-route", Name: "daemon.health"}
	routeCalled := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		routeCalled = true
		require.Equal(t, sessionID, turn.SessionID)
		require.NotEmpty(t, turn.TaskID)
		require.NotEmpty(t, turn.RunID)
		require.NotEmpty(t, turn.TurnID)
		require.Equal(t, "hello fallback route", turn.Prompt)
		require.Equal(t, daemonTurnExecutionDeterministicRoute, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)

		payload, ok := turn.Payload.(daemonDeterministicRouteTurnPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.SessionID)
		require.Equal(t, "hello fallback route", payload.Prompt)
		require.Equal(t, "operator-1", payload.Operator)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonDeterministicRouteTurnResultPayload{
				Messages:  []agentMessage{expectedMessage},
				ToolCalls: []agentToolCall{expectedToolCall},
			},
		}, nil
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        "hello fallback route",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "read_only",
		Provider:       "openai",
		APIKey:         "test-key",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, llmCalled)
	require.True(t, routeCalled)
	require.Len(t, resp.Messages, 3)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, "tool", resp.Messages[1].Role)
	require.Contains(t, resp.Messages[1].Content, "provider down")
	require.Equal(t, expectedMessage, resp.Messages[2])
	require.Len(t, resp.ToolCalls, 2)
	require.Equal(t, "LLM chat", resp.ToolCalls[0].Name)
	require.Equal(t, expectedToolCall, resp.ToolCalls[1])

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedMessage.ID, got.Messages[len(got.Messages)-1].ID)
}

func TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	prompt := "列出所有定时任务"
	require.True(t, agentShouldRouteBeforeLLMSynthesis(prompt))

	llmCalled := false
	eng.AttachLLMTurnRunner(runtime.LLMTurnRunnerFunc(func(_ context.Context, _ runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
		llmCalled = true
		return runtime.LLMTurnResult{Status: runtime.TurnResultFailed, Reason: "llm should not run"}, errors.New("llm should not run")
	}))

	expectedMessage := agentMessage{ID: "msg-route-before-llm", Role: "tool", Content: "route-before-llm through executor"}
	routeCalled := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		routeCalled = true
		require.Equal(t, sessionID, turn.SessionID)
		require.NotEmpty(t, turn.TaskID)
		require.NotEmpty(t, turn.RunID)
		require.NotEmpty(t, turn.TurnID)
		require.Equal(t, prompt, turn.Prompt)
		require.Equal(t, daemonTurnExecutionDeterministicRoute, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)

		payload, ok := turn.Payload.(daemonDeterministicRouteTurnPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.SessionID)
		require.Equal(t, prompt, payload.Prompt)
		require.Equal(t, "operator-1", payload.Operator)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonDeterministicRouteTurnResultPayload{
				Messages: []agentMessage{expectedMessage},
			},
		}, nil
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        prompt,
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "read_only",
		Provider:       "openai",
		APIKey:         "test-key",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, routeCalled)
	require.False(t, llmCalled)
	require.Len(t, resp.Messages, 2)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, "step", resp.Messages[1].Phase)
	require.Equal(t, expectedMessage.ID, resp.Messages[1].ID)
	require.Equal(t, expectedMessage.Role, resp.Messages[1].Role)
	require.Equal(t, expectedMessage.Content, resp.Messages[1].Content)
	require.Empty(t, resp.ToolCalls)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedMessage.ID, got.Messages[len(got.Messages)-1].ID)
	require.Equal(t, "step", got.Messages[len(got.Messages)-1].Phase)
}

func TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-llm", Role: "agent", Content: "ok"}
	expectedToolCall := agentToolCall{ID: "tool-llm", Name: "LLM chat"}
	called := false
	eng.AttachLLMTurnRunner(runtime.LLMTurnRunnerFunc(func(_ context.Context, req runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
		called = true
		require.Equal(t, "session-llm-runner", req.Turn.SessionID)
		require.Equal(t, "task-llm-runner", req.Turn.TaskID)
		require.Equal(t, "run-llm-runner", req.Turn.RunID)
		require.Equal(t, "turn-llm-runner", req.Turn.TurnID)
		require.Equal(t, "plan safely", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)
		require.Equal(t, "llm_tool_loop", req.Metadata["runner"])

		payload, ok := req.Payload.(daemonLLMTurnPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-llm-runner", payload.SessionID)
		require.Equal(t, "task-llm-runner", payload.TurnRef.TaskID)
		require.Equal(t, "run-llm-runner", payload.TurnRef.RunID)
		require.Equal(t, "turn-llm-runner", payload.TurnRef.TurnID)
		require.Equal(t, "plan safely", payload.Prompt)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "read_only", payload.OperationLevel)
		require.Equal(t, "gpt-test", payload.Req.Model)
		require.Len(t, payload.History, 1)

		return runtime.LLMTurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonLLMTurnResultPayload{
				Messages:  []agentMessage{expectedMessage},
				ToolCalls: []agentToolCall{expectedToolCall},
			},
		}, nil
	}))

	messages, toolCalls, err := mgr.runRuntimeLLMToolLoop(
		context.Background(),
		"session-llm-runner",
		agentRuntimeTurnRef{TaskID: "task-llm-runner", RunID: "run-llm-runner", TurnID: "turn-llm-runner"},
		"plan safely",
		"zh-CN",
		"operator-1",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-test"},
		[]agentMessage{{ID: "history-1", Role: "user", Content: "hello"}},
		&agentMemorySummary{Summary: "memory"},
		routerServices{},
	)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []agentMessage{expectedMessage}, messages)
	require.Equal(t, []agentToolCall{expectedToolCall}, toolCalls)
}

func TestAgentManagerRunRuntimeAgentLLMPlannerChatDispatchesEngineStageRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-planner", Role: "agent", Content: "planned"}
	expectedToolCall := agentToolCall{ID: "tool-planner", Name: "LLM chat"}
	expectedPlan := &agentPlannerResponse{Phase: "plan", Answer: "planned"}
	called := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		called = true
		require.Equal(t, runtime.LLMStagePlannerChat, req.Stage)
		require.Equal(t, "session-planner-stage", req.Turn.SessionID)
		require.Equal(t, "task-planner-stage", req.Turn.TaskID)
		require.Equal(t, "run-planner-stage", req.Turn.RunID)
		require.Equal(t, "turn-planner-stage", req.Turn.TurnID)
		require.Equal(t, "make a plan", req.Turn.Prompt)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "llm_stage", req.Metadata["runner"])
		require.Equal(t, string(runtime.LLMStagePlannerChat), req.Metadata["stage"])

		payload, ok := req.Payload.(daemonLLMPlannerChatPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-planner-stage", payload.SessionID)
		require.Equal(t, "task-planner-stage", payload.TurnRef.TaskID)
		require.Equal(t, "run-planner-stage", payload.TurnRef.RunID)
		require.Equal(t, "turn-planner-stage", payload.TurnRef.TurnID)
		require.Equal(t, "make a plan", payload.Prompt)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "safe_scan", payload.OperationLevel)
		require.Equal(t, "gpt-planner", payload.Req.Model)
		require.Len(t, payload.History, 1)

		return runtime.LLMStageResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonLLMPlannerChatResultPayload{
				Messages:  []agentMessage{expectedMessage},
				ToolCalls: []agentToolCall{expectedToolCall},
				Plan:      expectedPlan,
			},
		}, nil
	}))

	messages, toolCalls, plan, err := mgr.runRuntimeAgentLLMPlannerChat(
		context.Background(),
		"session-planner-stage",
		agentRuntimeTurnRef{TaskID: "task-planner-stage", RunID: "run-planner-stage", TurnID: "turn-planner-stage"},
		"make a plan",
		"zh-CN",
		"safe_scan",
		sendAgentMessageRequest{Model: "gpt-planner"},
		[]agentMessage{{ID: "history-1", Role: "user", Content: "hello"}},
		&agentMemorySummary{Summary: "memory"},
	)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []agentMessage{expectedMessage}, messages)
	require.Equal(t, []agentToolCall{expectedToolCall}, toolCalls)
	require.Same(t, expectedPlan, plan)
}

func TestAgentManagerRunRuntimeAgentLLMToolSynthesisDispatchesEngineStageRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-synthesis", Role: "agent", Content: "summarized"}
	expectedToolCall := agentToolCall{ID: "tool-synthesis", Name: "LLM synthesis"}
	expectedPlan := &agentPlannerResponse{Phase: "step_replan", Answer: "summarized"}
	called := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		called = true
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		require.Equal(t, "session-synthesis-stage", req.Turn.SessionID)
		require.Equal(t, "task-synthesis-stage", req.Turn.TaskID)
		require.Equal(t, "run-synthesis-stage", req.Turn.RunID)
		require.Equal(t, "turn-synthesis-stage", req.Turn.TurnID)
		require.Equal(t, "summarize tools", req.Turn.Prompt)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "llm_stage", req.Metadata["runner"])
		require.Equal(t, string(runtime.LLMStageToolSynthesis), req.Metadata["stage"])

		payload, ok := req.Payload.(daemonLLMToolSynthesisPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-synthesis-stage", payload.SessionID)
		require.Equal(t, "task-synthesis-stage", payload.TurnRef.TaskID)
		require.Equal(t, "run-synthesis-stage", payload.TurnRef.RunID)
		require.Equal(t, "turn-synthesis-stage", payload.TurnRef.TurnID)
		require.Equal(t, "summarize tools", payload.Prompt)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "safe_scan", payload.OperationLevel)
		require.Equal(t, "gpt-synthesis", payload.Req.Model)
		require.Len(t, payload.History, 1)
		require.Len(t, payload.ToolMessages, 1)

		return runtime.LLMStageResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonLLMToolSynthesisResultPayload{
				Messages:  []agentMessage{expectedMessage},
				ToolCalls: []agentToolCall{expectedToolCall},
				Plan:      expectedPlan,
			},
		}, nil
	}))

	messages, toolCalls, plan, err := mgr.runRuntimeAgentLLMToolSynthesis(
		context.Background(),
		"session-synthesis-stage",
		agentRuntimeTurnRef{TaskID: "task-synthesis-stage", RunID: "run-synthesis-stage", TurnID: "turn-synthesis-stage"},
		"summarize tools",
		"zh-CN",
		"safe_scan",
		sendAgentMessageRequest{Model: "gpt-synthesis"},
		[]agentMessage{{ID: "history-1", Role: "user", Content: "hello"}},
		&agentMemorySummary{Summary: "memory"},
		[]agentMessage{{ID: "tool-message-1", Role: "tool", Content: "observation"}},
	)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []agentMessage{expectedMessage}, messages)
	require.Equal(t, []agentToolCall{expectedToolCall}, toolCalls)
	require.Same(t, expectedPlan, plan)
}

func TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-replan-continuation", Role: "agent", Content: "continued through executor"}
	expectedToolCall := agentToolCall{ID: "tool-replan-continuation", Name: "daemon.health"}
	expectedPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		ToolCalls: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "continuation"},
		}},
	}
	called := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		called = true
		require.Equal(t, "session-replan-continuation", turn.SessionID)
		require.Equal(t, "task-replan-continuation", turn.TaskID)
		require.Equal(t, "run-replan-continuation", turn.RunID)
		require.Equal(t, "turn-replan-continuation", turn.TurnID)
		require.Equal(t, "continue daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionReplanActionContinuation, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, "turn_execution", turn.Metadata["executor"])
		require.Equal(t, daemonTurnExecutionReplanActionContinuation, turn.Metadata["kind"])
		require.Equal(t, 2, turn.Metadata["max_loops"])

		payload, ok := turn.Payload.(daemonReplanActionContinuationTurnPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "session-replan-continuation", payload.SessionID)
		require.Equal(t, "task-replan-continuation", payload.TurnRef.TaskID)
		require.Equal(t, "run-replan-continuation", payload.TurnRef.RunID)
		require.Equal(t, "turn-replan-continuation", payload.TurnRef.TurnID)
		require.Equal(t, "continue daemon health", payload.Prompt)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "tester", payload.Operator)
		require.Equal(t, "read_only", payload.OperationLevel)
		require.Equal(t, "gpt-continuation", payload.Req.Model)
		require.Len(t, payload.History, 1)
		require.Equal(t, "memory", payload.MemorySummary.Summary)
		require.Same(t, expectedPlan, payload.Plan)
		require.Equal(t, 2, payload.MaxLoops)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonReplanActionContinuationTurnResultPayload{
				Messages:  []agentMessage{expectedMessage},
				ToolCalls: []agentToolCall{expectedToolCall},
			},
		}, nil
	}))

	messages, toolCalls := mgr.runRuntimeReplanActionContinuation(
		context.Background(),
		"session-replan-continuation",
		agentRuntimeTurnRef{TaskID: "task-replan-continuation", RunID: "run-replan-continuation", TurnID: "turn-replan-continuation"},
		"continue daemon health",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-continuation"},
		[]agentMessage{{ID: "history-1", Role: "agent", Content: "previous step"}},
		&agentMemorySummary{Summary: "memory"},
		routerServices{},
		expectedPlan,
		2,
	)

	require.True(t, called)
	require.Equal(t, []agentMessage{expectedMessage}, messages)
	require.Equal(t, []agentToolCall{expectedToolCall}, toolCalls)
}

func TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok, "daemon.health missing from registry")
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	actionCalled := false
	spec.Handler = func(_ context.Context, in toolregistry.HandlerInput) (any, error) {
		actionCalled = true
		require.Equal(t, sessionID, in.SessionID)
		require.Equal(t, "tester", in.Operator)
		require.Equal(t, "en", in.Language)
		require.Equal(t, "read_only", in.OperationLevel)
		require.Equal(t, "continuation", stringFromAny(in.RawArgs["probe"]))

		tool := runningAgentToolCall("daemon.health", "continuation")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"ok": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "health ok", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	expectedMessage := agentMessage{ID: "msg-continuation-synthesis", Role: "agent", Content: "continuation summarized"}
	expectedToolCall := agentToolCall{ID: "tool-continuation-synthesis", Name: "LLM synthesis"}
	expectedPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "continuation summarized"}
	synthesisCalled := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		synthesisCalled = true
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-continuation", req.Turn.TaskID)
		require.Equal(t, "run-continuation", req.Turn.RunID)
		require.Equal(t, "turn-continuation", req.Turn.TurnID)
		require.Equal(t, "continue daemon health", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		payload, ok := req.Payload.(daemonLLMToolSynthesisPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, sessionID, payload.SessionID)
		require.Equal(t, "continue daemon health", payload.Prompt)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "read_only", payload.OperationLevel)
		require.Len(t, payload.History, 2)
		require.Len(t, payload.ToolMessages, 1)
		require.Contains(t, payload.ToolMessages[0].Content, "health ok")

		return runtime.LLMStageResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonLLMToolSynthesisResultPayload{
				Messages:  []agentMessage{expectedMessage},
				ToolCalls: []agentToolCall{expectedToolCall},
				Plan:      expectedPlan,
			},
		}, nil
	}))

	messages, toolCalls := mgr.runAgentReplanActionContinuation(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-continuation", RunID: "run-continuation", TurnID: "turn-continuation"},
		"continue daemon health",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-continuation"},
		[]agentMessage{{ID: "history-1", Role: "agent", Content: "previous step"}},
		&agentMemorySummary{Summary: "memory"},
		routerServices{},
		&agentPlannerResponse{
			Phase: "step_replan",
			NextAction: &agentNextAction{
				Type:         "continue",
				AutoContinue: true,
			},
			ToolCalls: []agentPlannerAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"probe": "continuation"},
			}},
		},
		1,
	)

	require.True(t, actionCalled)
	require.True(t, synthesisCalled)
	require.Len(t, messages, 2)
	require.Contains(t, messages[0].Content, "health ok")
	require.Equal(t, expectedMessage, messages[1])
	require.Len(t, toolCalls, 2)
	require.Equal(t, "daemon.health", toolCalls[0].Name)
	require.Equal(t, expectedToolCall, toolCalls[1])
}
