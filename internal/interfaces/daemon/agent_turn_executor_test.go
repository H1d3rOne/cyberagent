package daemon

import (
	"context"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerAttachEngineWiresDaemonTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	require.NotNil(t, eng.LLMTurnRunner())
	require.NotNil(t, eng.LLMStageRunner())

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
