package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "zh",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	initialMessages := len(created.Session.Messages)
	message := agentMessage{
		ID:        "msg-session-mutation",
		Role:      "agent",
		Content:   "mutation handled by runtime seam",
		Timestamp: time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
	}
	plan := &agentPlannerResponse{
		Answer: "planner state",
		Task: &agentPlannerTask{
			Goal:   "keep session mutation behind engine",
			Status: "running",
		},
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Metadata["kind"])
		require.Equal(t, 1, req.Metadata["message_count"])
		require.Equal(t, true, req.Metadata["planner_apply"])

		payload, ok := req.Payload.(daemonAppendMessagesMutationPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, sessionID, payload.SessionID)
		require.Same(t, plan, payload.Plan)
		require.Len(t, payload.Messages, 1)
		require.Equal(t, message.ID, payload.Messages[0].ID)
		require.Equal(t, message.Content, payload.Messages[0].Content)

		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled by test mutator",
			Data:   map[string]any{"mutator": "test"},
		}, nil
	}))

	mgr.appendMessagesWithPlanner(sessionID, plan, message)

	require.True(t, called)
	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, got.Messages, initialMessages, "handled runtime mutator should prevent legacy fallback")
}
