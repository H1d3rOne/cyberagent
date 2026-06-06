package daemon

import (
	"context"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerSendMessageRunsInsideRuntimeTurnLifecycle(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	expectedMessage := agentMessage{
		ID:      "msg-send-message-lifecycle-final",
		Role:    "agent",
		Phase:   "final_answer",
		Content: "turn lifecycle completed",
	}
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		require.Equal(t, sessionID, turn.SessionID)
		require.NotEmpty(t, turn.TaskID)
		require.NotEmpty(t, turn.RunID)
		require.NotEmpty(t, turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionDeterministicRoute, turn.Origin)
		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonDeterministicRouteTurnResultPayload{
				Messages: []agentMessage{expectedMessage},
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
	require.Len(t, resp.Messages, 2)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, expectedMessage, resp.Messages[1])
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.NotEmpty(t, appendReq.Turn.TaskID)
	require.NotEmpty(t, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)

	events := store.Events()
	startedIdx := -1
	tokenIdx := -1
	finishedIdx := -1
	taskFinishedIdx := -1
	for i, ev := range events {
		if ev.RunID != appendReq.Turn.RunID && ev.TaskID != appendReq.Turn.TaskID {
			continue
		}
		switch ev.Type {
		case runtime.EventTurnStarted:
			startedIdx = i
			require.Equal(t, appendReq.Turn.TurnID, ev.TurnID)
			require.Equal(t, sessionID, ev.Data["session_id"])
			require.Equal(t, "daemon health", ev.Data["prompt"])
		case runtime.EventModelToken:
			if ev.Data["message_id"] == expectedMessage.ID {
				tokenIdx = i
			}
		case runtime.EventTurnFinished:
			finishedIdx = i
			require.Equal(t, appendReq.Turn.TurnID, ev.TurnID)
			require.Equal(t, string(runtime.TurnResultFinished), ev.Data["status"])
		case runtime.EventTaskFinished:
			taskFinishedIdx = i
			require.Equal(t, sessionID, ev.Data["session_id"])
		}
	}
	require.NotEqual(t, -1, startedIdx, "turn_started missing: %#v", events)
	require.NotEqual(t, -1, tokenIdx, "model_token missing: %#v", events)
	require.NotEqual(t, -1, finishedIdx, "turn_finished missing: %#v", events)
	require.NotEqual(t, -1, taskFinishedIdx, "task_finished missing: %#v", events)
	require.Less(t, startedIdx, tokenIdx)
	require.Less(t, tokenIdx, finishedIdx)
	require.Less(t, finishedIdx, taskFinishedIdx)
}
