package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerSendMessageResponseMergeControlLLMBranches(t *testing.T) {
	cases := []struct {
		name         string
		prompt       string
		wantAction   string
		wantMessages []string
		wantTools    []string
	}{
		{
			name:       "continue",
			prompt:     "continue",
			wantAction: "continue",
			wantMessages: []string{
				"user",
				"agent",
				"msg-control-continue-llm",
			},
			wantTools: []string{"tool-control-continue-llm"},
		},
		{
			name:       "revise",
			prompt:     "revise focus on auth checks",
			wantAction: "revise",
			wantMessages: []string{
				"user",
				"agent",
				"msg-control-revise-llm",
			},
			wantTools: []string{"tool-control-revise-llm"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

			expectedMessage := agentMessage{ID: tc.wantMessages[2], Role: "agent", Content: "llm continued after control"}
			expectedTool := agentToolCall{ID: tc.wantTools[0], Name: "LLM chat", Status: contracts.StatusSucceeded}
			llmCalled := false
			eng.AttachLLMTurnRunner(runtime.LLMTurnRunnerFunc(func(_ context.Context, req runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
				llmCalled = true
				require.Equal(t, sessionID, req.Turn.SessionID)
				require.NotEmpty(t, req.Turn.TaskID)
				require.NotEmpty(t, req.Turn.RunID)
				require.NotEmpty(t, req.Turn.TurnID)
				require.Equal(t, tc.prompt, req.Turn.Prompt)
				require.Equal(t, "read_only", req.Turn.OperationLevel)

				payload, ok := req.Payload.(daemonLLMTurnPayload)
				require.True(t, ok, "payload type = %T", req.Payload)
				require.Equal(t, sessionID, payload.Loop.Turn.SessionID)
				require.Equal(t, tc.prompt, payload.Loop.Turn.Prompt)
				require.Equal(t, "operator-1", payload.Loop.Operator)
				require.GreaterOrEqual(t, len(payload.Loop.History), 2)
				require.Equal(t, "user", payload.Loop.History[len(payload.Loop.History)-2].Role)
				require.Equal(t, tc.prompt, payload.Loop.History[len(payload.Loop.History)-2].Content)
				require.Equal(t, "agent", payload.Loop.History[len(payload.Loop.History)-1].Role)

				return runtime.LLMTurnResult{
					Status: runtime.TurnResultFinished,
					Reason: "done",
					Payload: runtime.AgentLoopTurnResultPayload{
						Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
						ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedTool}),
					},
				}, nil
			}))

			resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
				Message:        tc.prompt,
				Operator:       "operator-1",
				Language:       "en",
				OperationLevel: "read_only",
				Provider:       "openai",
				APIKey:         "test-key",
			}, routerServices{})

			require.NoError(t, err)
			require.True(t, llmCalled)
			require.Equal(t, sessionID, resp.SessionID)
			require.Len(t, resp.Messages, 3)
			require.Equal(t, tc.wantMessages[0], resp.Messages[0].Role)
			require.Equal(t, tc.prompt, resp.Messages[0].Content)
			require.Equal(t, tc.wantMessages[1], resp.Messages[1].Role)
			require.Equal(t, tc.wantMessages[2], resp.Messages[2].ID)
			require.Equal(t, []agentToolCall{expectedTool}, resp.ToolCalls)
			require.NotNil(t, resp.NextAction)
			if tc.wantAction == "continue" {
				require.Equal(t, "continue", resp.NextAction.Type)
			}
		})
	}
}

func TestAgentManagerSendMessageResponseMergePendingAuthorizationGate(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "workspace_write",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-response-merge-pending",
		ActionName:     "workspace.files.write",
		Arguments:      map[string]any{"path": "notes.md", "content": "hello"},
		RequiredLevel:  "workspace_write",
		Policy:         "manual_tool_authorization_required",
		Reason:         "manual authorization pending",
		OriginalPrompt: "write file notes.md content: hello",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	session.NextAction = &agentNextAction{
		Type:            "authorization_required",
		Reason:          "manual authorization pending",
		SuggestedPrompt: "approve pending write",
		AutoContinue:    false,
	}
	mgr.mu.Unlock()

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)
	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        "daemon health",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "workspace_write",
	}, routerServices{})

	require.NoError(t, err)
	require.Len(t, resp.Messages, 2)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, "daemon health", resp.Messages[0].Content)
	require.Equal(t, "agent", resp.Messages[1].Role)
	require.Equal(t, "step", resp.Messages[1].Phase)
	require.Contains(t, resp.Messages[1].Content, "Approve or deny")
	require.Empty(t, resp.ToolCalls)
	require.NotNil(t, resp.PendingAuthorization)
	require.Equal(t, "auth-response-merge-pending", resp.PendingAuthorization.ID)
	require.Equal(t, "pending", resp.PendingAuthorization.Status)

	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.NotEmpty(t, appendReq.Turn.TaskID)
	require.NotEmpty(t, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "daemon health", appendReq.Turn.Prompt)
	require.Equal(t, "authorization_gate", appendReq.Turn.Origin)
	require.Equal(t, "workspace_write", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	payload, ok := appendReq.Payload.(daemonAppendMessagesMutationPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, resp.Messages[1].ID, payload.Messages[0].ID)
}

func TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis(t *testing.T) {
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

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	routeTool := agentToolCall{ID: "tool-route-before-merge", Name: "schedule.list", Status: contracts.StatusSucceeded}
	routeMessage := agentMessage{
		ID:        "msg-route-before-merge",
		Role:      "tool",
		Content:   "listed schedules before synthesis",
		ToolCalls: []agentToolCall{routeTool},
	}
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
		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: daemonDeterministicRouteTurnResultPayload{
				Messages:  []agentMessage{routeMessage},
				ToolCalls: []agentToolCall{routeTool},
			},
		}, nil
	}))

	llmTurnCalled := false
	eng.AttachLLMTurnRunner(runtime.LLMTurnRunnerFunc(func(_ context.Context, _ runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
		llmTurnCalled = true
		return runtime.LLMTurnResult{Status: runtime.TurnResultFailed, Reason: "llm turn should not run"}, nil
	}))

	synthesisTool := agentToolCall{ID: "tool-route-before-synthesis-merge", Name: "LLM synthesis", Status: contracts.StatusSucceeded}
	synthesisMessage := agentMessage{ID: "msg-route-before-synthesis-merge", Role: "agent", Phase: "final_answer", Content: "synthesized route output"}
	synthesisCalled := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		synthesisCalled = true
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.NotEmpty(t, req.Turn.TaskID)
		require.NotEmpty(t, req.Turn.RunID)
		require.NotEmpty(t, req.Turn.TurnID)
		require.Equal(t, prompt, req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.Equal(t, prompt, payload.Turn.Prompt)
		require.Len(t, payload.ToolMessages, 1)
		require.Equal(t, routeMessage.ID, payload.ToolMessages[0].ID)
		require.Equal(t, "step", payload.ToolMessages[0].Phase)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "done",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, []agentToolCall{synthesisTool}, nil),
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
	require.False(t, llmTurnCalled)
	require.True(t, synthesisCalled)
	require.Len(t, resp.Messages, 3)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, routeMessage.ID, resp.Messages[1].ID)
	require.Equal(t, "step", resp.Messages[1].Phase)
	require.Equal(t, synthesisMessage.ID, resp.Messages[2].ID)
	require.Len(t, resp.ToolCalls, 2)
	require.Equal(t, routeTool.ID, resp.ToolCalls[0].ID)
	require.Equal(t, "step", firstNonEmpty(stringFromAny(resp.ToolCalls[0].Metadata["agent_phase"]), stringFromAny(resp.ToolCalls[0].Metadata["phase"])))
	require.Equal(t, synthesisTool.ID, resp.ToolCalls[1].ID)

	require.Len(t, appendMutations, 2)
	require.Equal(t, daemonTurnExecutionDeterministicRoute, appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)
	require.Equal(t, 1, appendMutations[0].Turn.Metadata["message_count"])
	require.Equal(t, 1, appendMutations[1].Turn.Metadata["message_count"])
}
