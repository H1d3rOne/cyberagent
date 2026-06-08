package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerRunRuntimeAgentLLMStageFallsBackWithoutRunner(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	expectedMessage := agentMessage{ID: "msg-fallback", Role: "agent", Content: "fallback response"}
	expectedToolCall := agentToolCall{ID: "tool-fallback", Name: "LLM chat"}
	expectedPlan := &agentPlannerResponse{Phase: "plan", Answer: "fallback response"}

	payload, err := mgr.runRuntimeAgentLLMStage(context.Background(), runtime.LLMStagePlannerChat, agentLLMStageDispatchInput{
		SessionID:      "session-stage-fallback",
		TurnRef:        agentRuntimeTurnRef{TaskID: "task-stage-fallback", RunID: "run-stage-fallback", TurnID: "turn-stage-fallback"},
		Prompt:         "make a fallback plan",
		Language:       "en",
		OperationLevel: "read_only",
		Request:        sendAgentMessageRequest{Model: "gpt-fallback"},
	}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
		return []agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan, nil
	})

	require.NoError(t, err)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), payload.Messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), payload.ToolCalls)
	require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), payload.Plan)
}

func TestAgentManagerRunRuntimeAgentLLMStageInvalidPayloadDoesNotFallback(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		return runtime.LLMStageResult{Status: runtime.TurnResultFinished, Reason: "missing payload"}, nil
	}))

	payload, err := mgr.runRuntimeAgentLLMStage(context.Background(), runtime.LLMStageToolSynthesis, agentLLMStageDispatchInput{
		SessionID:      "session-stage-invalid",
		TurnRef:        agentRuntimeTurnRef{TaskID: "task-stage-invalid", RunID: "run-stage-invalid", TurnID: "turn-stage-invalid"},
		Prompt:         "summarize",
		Language:       "en",
		OperationLevel: "read_only",
		Request:        sendAgentMessageRequest{Model: "gpt-invalid"},
	}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
		t.Fatalf("fallback should not run for non-error invalid payload")
		return nil, nil, nil, nil
	})

	require.ErrorContains(t, err, "llm tool_synthesis runner returned invalid payload")
	require.Empty(t, payload.Messages)
	require.Empty(t, payload.ToolCalls)
	require.Nil(t, payload.Plan)
}

func TestAgentLLMStagePayloadFromDispatchInputClonesMutableFields(t *testing.T) {
	lastMessageAt := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	input := agentLLMStageDispatchInput{
		SessionID:      "session-stage-clone",
		TurnRef:        agentRuntimeTurnRef{TaskID: "task-stage-clone", RunID: "run-stage-clone", TurnID: "turn-stage-clone"},
		Prompt:         "clone stage payload",
		Language:       "zh-CN",
		OperationLevel: "safe_scan",
		Request: sendAgentMessageRequest{
			Model: "gpt-clone",
			AuthProfiles: []agentHTTPAuthProfile{{
				ID:    "auth-1",
				Token: "secret-token",
			}},
		},
		History:       []agentMessage{{ID: "history-1", Role: "user", Content: "hello"}},
		MemorySummary: &agentMemorySummary{Summary: "memory", LastMessageAt: &lastMessageAt},
		ToolMessages:  []runtime.AgentLoopMessage{{ID: "tool-message-1", Role: "tool", Content: "observation"}},
	}
	turn := agentLLMStageTurnContext(runtime.LLMStageToolSynthesis, input)

	payload := agentLLMStagePayloadFromDispatchInput(turn, input)

	require.Equal(t, "session-stage-clone", payload.Turn.SessionID)
	require.Equal(t, "zh-CN", payload.Language)
	require.Equal(t, "gpt-clone", payload.LLMRequest.Model)
	require.Len(t, payload.History, 1)
	require.Len(t, payload.ToolMessages, 1)
	require.NotNil(t, payload.MemorySummary)
	require.Equal(t, "memory", payload.MemorySummary.Summary)

	payload.History[0].Content = "mutated history"
	payload.ToolMessages[0].Content = "mutated tool message"
	payload.MemorySummary.Summary = "mutated memory"
	payload.LLMRequest.AuthProfiles[0].Token = "mutated-token"

	require.Equal(t, "hello", input.History[0].Content)
	require.Equal(t, "observation", input.ToolMessages[0].Content)
	require.Equal(t, "memory", input.MemorySummary.Summary)
	require.Equal(t, "secret-token", input.Request.AuthProfiles[0].Token)
}
