package daemon

import (
	"context"
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
