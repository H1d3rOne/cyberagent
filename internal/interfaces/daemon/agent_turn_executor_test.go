package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerAttachEngineWiresDaemonTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	require.NotNil(t, eng.LLMTurnRunner())
	require.NotNil(t, eng.LLMStageRunner())
	require.NotNil(t, eng.AgentLoopRunner())
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

func attachRecordingSessionMutator(t *testing.T, eng *runtime.Engine, mgr *agentManager, records *[]runtime.SessionMutationRequest) {
	t.Helper()
	legacy := daemonSessionMutator{manager: mgr}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		if req.Kind == runtime.SessionMutationAppendMessages {
			*records = append(*records, req)
		}
		return legacy.MutateSession(ctx, req)
	}))
}

func agentMessageIDs(messages []agentMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func agentLoopMessageIDs(messages []runtime.AgentLoopMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func TestAgentManagerDeterministicRoutePlanOnlyAppendUsesTurnRef(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	turnRef := agentRuntimeTurnRef{
		TaskID: "task-deterministic-plan-only",
		RunID:  "run-deterministic-plan-only",
		TurnID: "turn-deterministic-plan-only",
	}
	messages, toolCalls, err := mgr.routePromptWithRuntimeDeterministicRoute(
		context.Background(),
		sessionID,
		turnRef,
		"run ffuf directory enumeration on https://example.com",
		"en",
		"operator-1",
		"safe_scan",
		routerServices{},
	)

	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Contains(t, messages[0].Content, "content discovery")
	require.Empty(t, toolCalls)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, turnRef.TaskID, appendReq.Turn.TaskID)
	require.Equal(t, turnRef.RunID, appendReq.Turn.RunID)
	require.Equal(t, turnRef.TurnID, appendReq.Turn.TurnID)
	require.Equal(t, "run ffuf directory enumeration on https://example.com", appendReq.Turn.Prompt)
	require.Equal(t, daemonTurnExecutionDeterministicRoute, appendReq.Turn.Origin)
	require.Equal(t, "safe_scan", appendReq.Turn.OperationLevel)
	require.Equal(t, 0, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, true, appendReq.Turn.Metadata["planner_apply"])

	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.NotNil(t, payload.Plan)
	require.Empty(t, payload.Messages)
}

func TestAgentManagerControlSessionJobCancelAppendUsesExternalTurnRef(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 13, 0, 0, 0, time.UTC)
	runID := "run-external-control"
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:          "task-external-control",
		Goal:        "cancel a running daemon job",
		Kind:        "scan",
		Status:      "running",
		ActiveRunID: runID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	mgr.mu.Unlock()

	jobID := "job-external-control"
	cancelled := false
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	jobs.mu.Lock()
	jobs.jobs[jobID] = &daemonJob{
		JobID:     jobID,
		Kind:      scanWebJobKind,
		Status:    contracts.StatusRunning,
		RunID:     runID,
		CreatedAt: now,
		StartedAt: timePtr(now),
		cancel: func() {
			cancelled = true
		},
		inFlight: true,
	}
	jobs.running = 1
	jobs.mu.Unlock()

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	_, err = mgr.ControlSession(context.Background(), sessionID, agentControlRequest{
		Action:         "cancel",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "safe_scan",
		Prompt:         "cancel the running daemon job",
		JobID:          jobID,
	}, routerServices{jobs: jobs})

	require.NoError(t, err)
	require.True(t, cancelled)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, "task-external-control", appendReq.Turn.TaskID)
	require.Equal(t, runID, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "cancel the running daemon job", appendReq.Turn.Prompt)
	require.Equal(t, string(runtime.SessionMutationControlSession), appendReq.Turn.Origin)
	require.Equal(t, "safe_scan", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])

	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, "tool", payload.Messages[0].Role)
	require.Len(t, payload.Messages[0].ToolCalls, 1)
	require.Equal(t, "POST /api/v1/jobs/{job_id}/cancel", payload.Messages[0].ToolCalls[0].Name)
}

func TestAgentManagerDecideAgentAuthorizationApprovedActionAppendUsesExternalTurnRef(t *testing.T) {
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
	now := time.Date(2026, 6, 6, 13, 30, 0, 0, time.UTC)
	runID := "run-external-authorization"
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.ActiveTask = &agentTask{
		ID:          "task-external-authorization",
		Goal:        "approve a pending daemon action",
		Kind:        "tool",
		Status:      "blocked",
		ActiveRunID: runID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-external-authorization",
		ActionName:     "daemon.health",
		Arguments:      map[string]any{"probe": "external-authorization"},
		RequiredLevel:  "read_only",
		Policy:         "manual",
		Reason:         "operator approval required",
		OriginalPrompt: "run daemon health after approval",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mgr.mu.Unlock()

	spec, ok := toolRegistry().Lookup("daemon.health")
	require.True(t, ok, "daemon.health missing from registry")
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()
	spec.Handler = func(_ context.Context, in toolregistry.HandlerInput) (any, error) {
		require.Equal(t, sessionID, in.SessionID)
		require.Equal(t, "operator-1", in.Operator)
		require.Equal(t, "en", in.Language)
		require.Equal(t, "read_only", in.OperationLevel)
		require.Equal(t, "external-authorization", stringFromAny(in.RawArgs["probe"]))
		tool := runningAgentToolCall("daemon.health", "authorization")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"approved_action": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "authorized action executed", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	resp, err := mgr.DecideAgentAuthorization(context.Background(), sessionID, agentAuthorizationDecisionRequest{
		Action:          "approve",
		AuthorizationID: "auth-external-authorization",
		Operator:        "operator-1",
		Language:        "en",
		OperationLevel:  "read_only",
		SkipSynthesis:   true,
	}, routerServices{})

	require.NoError(t, err)
	require.NotNil(t, resp.ApprovalState)
	require.Equal(t, approvalCanonicalStateApproved, resp.ApprovalState.State)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, "task-external-authorization", appendReq.Turn.TaskID)
	require.Equal(t, runID, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "run daemon health after approval", appendReq.Turn.Prompt)
	require.Equal(t, daemonTurnExecutionPlannerActionExecution, appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])

	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, "authorized action executed", payload.Messages[0].Content)
	require.Equal(t, "step", payload.Messages[0].Phase)
	require.Len(t, payload.Messages[0].ToolCalls, 1)
	require.Equal(t, "daemon.health", payload.Messages[0].ToolCalls[0].Name)
}

func TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners(t *testing.T) {
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
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	runID := "run-external-authorization-flow"
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.ActiveTask = &agentTask{
		ID:          "task-external-authorization-flow",
		Goal:        "approve, synthesize, and continue a pending daemon action",
		Kind:        "tool",
		Status:      "blocked",
		ActiveRunID: runID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-external-authorization-flow",
		ActionName:     "daemon.health",
		Arguments:      map[string]any{"probe": "external-authorization-flow"},
		RequiredLevel:  "read_only",
		Policy:         "manual",
		Reason:         "operator approval required",
		OriginalPrompt: "run daemon health and summarize after approval",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mgr.mu.Unlock()

	actionTool := runningAgentToolCall("daemon.health", "authorization-flow")
	actionTool = finishAgentToolCall(actionTool, nil)
	actionTool.Result = map[string]any{"approved_action": true}
	actionMessage := mgr.message("en", "tool", "authorized action executed through runtime runner", []agentToolCall{actionTool}, nil)
	synthesisTool := runningAgentToolCall("LLM synthesis", "authorization-flow")
	synthesisTool = finishAgentToolCall(synthesisTool, nil)
	synthesisTool.Result = map[string]any{"synthesized": true}
	synthesisMessage := agentMessageWithPhase(mgr.message("en", "agent", "authorized action synthesized through runtime runner", []agentToolCall{synthesisTool}, nil), "step_replan")
	synthesisPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
	}

	turnExecutions := []runtime.TurnContext{}
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		kind, _ := turn.Metadata["kind"].(string)
		switch kind {
		case daemonTurnExecutionPlannerActionExecution:
			turnExecutions = append(turnExecutions, turn)
			payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
			require.True(t, ok, "payload type = %T", turn.Payload)
			require.Equal(t, "daemon.health", payload.Action.Name)
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "approved action handled by test executor",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{actionTool}),
				},
			}, nil
		case daemonTurnExecutionReplanActionContinuation:
			turnExecutions = append(turnExecutions, turn)
			payload, ok := turn.Payload.(runtime.AgentLoopRequest)
			require.True(t, ok, "payload type = %T", turn.Payload)
			require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(synthesisPlan), payload.InitialPlan)
			return runtime.TurnResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "follow-up handled by test executor",
				Payload: runtime.AgentLoopTurnResultPayload{},
			}, nil
		default:
			return runtime.TurnResult{Status: runtime.TurnResultRunning}, nil
		}
	}))

	llmStages := []runtime.LLMStageRequest{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		llmStages = append(llmStages, req)
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Len(t, payload.ToolMessages, 1)
		require.Equal(t, actionMessage.ID, payload.ToolMessages[0].ID)
		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "synthesis handled by test runner",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, []agentToolCall{synthesisTool}, synthesisPlan),
		}, nil
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	resp, err := mgr.DecideAgentAuthorization(context.Background(), sessionID, agentAuthorizationDecisionRequest{
		Action:          "approve",
		AuthorizationID: "auth-external-authorization-flow",
		Operator:        "operator-1",
		Language:        "en",
		OperationLevel:  "read_only",
		Provider:        "ollama",
	}, routerServices{})

	require.NoError(t, err)
	require.NotNil(t, resp.ApprovalState)
	require.Equal(t, approvalCanonicalStateApproved, resp.ApprovalState.State)

	expectedTurnID := "turn_auth-external-authorization-flow"
	require.Len(t, turnExecutions, 2)
	require.Len(t, llmStages, 1)
	require.Len(t, appendMutations, 2)
	for _, turn := range turnExecutions {
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, "task-external-authorization-flow", turn.TaskID)
		require.Equal(t, runID, turn.RunID)
		require.Equal(t, expectedTurnID, turn.TurnID)
		require.Equal(t, "run daemon health and summarize after approval", turn.Prompt)
		require.Equal(t, "read_only", turn.OperationLevel)
	}
	require.Equal(t, daemonTurnExecutionPlannerActionExecution, turnExecutions[0].Metadata["kind"])
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, turnExecutions[1].Metadata["kind"])

	require.Equal(t, sessionID, llmStages[0].Turn.SessionID)
	require.Equal(t, "task-external-authorization-flow", llmStages[0].Turn.TaskID)
	require.Equal(t, runID, llmStages[0].Turn.RunID)
	require.Equal(t, expectedTurnID, llmStages[0].Turn.TurnID)
	require.Equal(t, "run daemon health and summarize after approval", llmStages[0].Turn.Prompt)
	require.Equal(t, "read_only", llmStages[0].Turn.OperationLevel)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), llmStages[0].Metadata["stage"])

	require.Equal(t, daemonTurnExecutionPlannerActionExecution, appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)
	for _, appendReq := range appendMutations {
		require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
		require.Equal(t, sessionID, appendReq.Turn.SessionID)
		require.Equal(t, "task-external-authorization-flow", appendReq.Turn.TaskID)
		require.Equal(t, runID, appendReq.Turn.RunID)
		require.Equal(t, expectedTurnID, appendReq.Turn.TurnID)
		require.Equal(t, "run daemon health and summarize after approval", appendReq.Turn.Prompt)
		require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	}
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

		payload, ok := turn.Payload.(runtime.AgentLoopRequest)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "session-route-executor", payload.Turn.SessionID)
		require.Equal(t, "daemon health", payload.Turn.Prompt)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
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
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), result.Messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), result.ToolCalls)
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
	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)
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

		payload, ok := turn.Payload.(runtime.AgentLoopRequest)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.Equal(t, "daemon health", payload.Turn.Prompt)
		require.Equal(t, "operator-1", payload.Operator)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
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
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.NotEmpty(t, appendReq.Turn.TaskID)
	require.NotEmpty(t, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "daemon health", appendReq.Turn.Prompt)
	require.Equal(t, daemonTurnExecutionDeterministicRoute, appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])
	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, expectedMessage.ID, payload.Messages[0].ID)
}

func TestAgentManagerSendMessageActiveRunContinueUsesTurnExecutor(t *testing.T) {
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
	runID := "run_active_continue"
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:          "task-active-run-continue",
		Goal:        "track active run context",
		Kind:        "scan",
		Status:      "running",
		ActiveRunID: runID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.NextAction = &agentNextAction{
		Type:            "continue",
		Reason:          "tracking run",
		SuggestedPrompt: "continue tracking run context",
	}
	mgr.mu.Unlock()

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)
	expectedToolCall := agentToolCall{
		ID:     "tool-active-run-context",
		Name:   "runs.context",
		Status: contracts.StatusSucceeded,
		Result: map[string]any{
			"run_id":  runID,
			"context": "events",
		},
	}
	expectedMessage := agentMessage{
		ID:        "msg-active-run-context",
		Role:      "tool",
		Content:   "active run context routed through executor",
		ToolCalls: []agentToolCall{expectedToolCall},
	}

	routeCalled := false
	runtimeRunID := ""
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		routeCalled = true
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, "task-active-run-continue", turn.TaskID)
		require.NotEmpty(t, turn.RunID)
		require.NotEmpty(t, turn.TurnID)
		runtimeRunID = turn.RunID
		require.Contains(t, turn.Prompt, "读取运行 "+runID)
		require.Contains(t, turn.Prompt, "events")
		require.Equal(t, daemonTurnExecutionDeterministicRoute, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, "turn_execution", turn.Metadata["executor"])
		require.Equal(t, daemonTurnExecutionDeterministicRoute, turn.Metadata["kind"])

		payload, ok := turn.Payload.(runtime.AgentLoopRequest)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.Equal(t, turn.Prompt, payload.Turn.Prompt)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
			},
		}, nil
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        "继续",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "read_only",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, routeCalled)
	require.Len(t, resp.Messages, 3)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, "agent", resp.Messages[1].Role)
	require.Equal(t, expectedMessage, resp.Messages[2])
	require.Equal(t, []agentToolCall{expectedToolCall}, resp.ToolCalls)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedMessage.ID, got.Messages[len(got.Messages)-1].ID)
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, "task-active-run-continue", appendReq.Turn.TaskID)
	require.Equal(t, runtimeRunID, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "继续", appendReq.Turn.Prompt)
	require.Equal(t, "active_run_continue", appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])
	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, expectedMessage.ID, payload.Messages[0].ID)
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
	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	llmErr := errors.New("provider down")
	llmCalled := false
	eng.AttachLLMTurnRunner(runtime.LLMTurnRunnerFunc(func(_ context.Context, req runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
		llmCalled = true
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "hello fallback route", req.Turn.Prompt)
		return runtime.LLMTurnResult{
			Status:  runtime.TurnResultFailed,
			Reason:  llmErr.Error(),
			Error:   llmErr,
			Payload: runtime.AgentLoopTurnResultPayload{},
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

		payload, ok := turn.Payload.(runtime.AgentLoopRequest)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.Equal(t, "hello fallback route", payload.Turn.Prompt)
		require.Equal(t, "operator-1", payload.Operator)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
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
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.NotEmpty(t, appendReq.Turn.TaskID)
	require.NotEmpty(t, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, "hello fallback route", appendReq.Turn.Prompt)
	require.Equal(t, "llm_fallback_route", appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 2, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])
	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 2)
	require.Contains(t, payload.Messages[0].Content, "provider down")
	require.Equal(t, expectedMessage.ID, payload.Messages[1].ID)
}

func TestAgentManagerSendMessageLLMFallbackAuthorizationUsesSessionMutations(t *testing.T) {
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
	prompt := "write file notes.md content: hello from fallback authorization"

	var authMutations []runtime.SessionMutationRequest
	var appendMutations []runtime.SessionMutationRequest
	legacy := daemonSessionMutator{manager: mgr}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		switch req.Kind {
		case runtime.SessionMutationRequestAuthorization:
			authMutations = append(authMutations, req)
		case runtime.SessionMutationAppendMessages:
			appendMutations = append(appendMutations, req)
		}
		return legacy.MutateSession(ctx, req)
	}))

	llmErr := errors.New("provider down")
	llmCalled := false
	eng.AttachLLMTurnRunner(runtime.LLMTurnRunnerFunc(func(_ context.Context, req runtime.LLMTurnRequest) (runtime.LLMTurnResult, error) {
		llmCalled = true
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, prompt, req.Turn.Prompt)
		require.NotEmpty(t, req.Turn.TaskID)
		require.NotEmpty(t, req.Turn.RunID)
		require.NotEmpty(t, req.Turn.TurnID)
		require.Equal(t, "workspace_write", req.Turn.OperationLevel)
		return runtime.LLMTurnResult{
			Status:  runtime.TurnResultFailed,
			Reason:  llmErr.Error(),
			Error:   llmErr,
			Payload: runtime.AgentLoopTurnResultPayload{},
		}, llmErr
	}))

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Message:        prompt,
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "workspace_write",
		Provider:       "openai",
		APIKey:         "test-key",
	}, routerServices{})

	require.NoError(t, err)
	require.True(t, llmCalled)
	require.Len(t, resp.Messages, 3)
	require.Equal(t, "user", resp.Messages[0].Role)
	require.Equal(t, prompt, resp.Messages[0].Content)
	require.Equal(t, "tool", resp.Messages[1].Role)
	require.Contains(t, resp.Messages[1].Content, "provider down")
	require.Equal(t, "agent", resp.Messages[2].Role)
	require.Equal(t, "step", resp.Messages[2].Phase)
	require.Contains(t, resp.Messages[2].Content, "Authorization required")
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "LLM chat", resp.ToolCalls[0].Name)

	require.Len(t, authMutations, 1)
	authReq := authMutations[0]
	require.Equal(t, runtime.SessionMutationRequestAuthorization, authReq.Kind)
	require.Equal(t, sessionID, authReq.Turn.SessionID)
	require.NotEmpty(t, authReq.Turn.TaskID)
	require.NotEmpty(t, authReq.Turn.RunID)
	require.NotEmpty(t, authReq.Turn.TurnID)
	require.Equal(t, prompt, authReq.Turn.Prompt)
	require.Equal(t, string(runtime.SessionMutationRequestAuthorization), authReq.Turn.Origin)
	require.Equal(t, "workspace_write", authReq.Turn.OperationLevel)
	require.Equal(t, string(runtime.SessionMutationRequestAuthorization), authReq.Metadata["kind"])
	require.Equal(t, "workspace.files.write", authReq.Metadata["action_name"])
	require.Equal(t, "workspace_write", authReq.Metadata["required_level"])
	require.Equal(t, "manual_tool_authorization_required", authReq.Metadata["policy"])

	authPayload, ok := authReq.Payload.(daemonRequestAuthorizationMutationPayload)
	require.True(t, ok, "payload type = %T", authReq.Payload)
	require.Equal(t, sessionID, authPayload.SessionID)
	require.Equal(t, "en", authPayload.Language)
	require.Equal(t, prompt, authPayload.OperatorPrompt)
	require.Equal(t, "workspace_write", authPayload.OperationLevel)
	require.Equal(t, "workspace.files.write", authPayload.ActionName)
	require.Equal(t, "workspace_write", authPayload.RequiredLevel)
	require.Equal(t, "manual_tool_authorization_required", authPayload.Policy)
	require.Equal(t, "workspace.files.write", authPayload.Action.Name)
	require.Equal(t, "notes.md", authPayload.Action.Arguments["path"])
	require.Equal(t, "hello from fallback authorization", authPayload.Action.Arguments["content"])

	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.Equal(t, authReq.Turn.TaskID, appendReq.Turn.TaskID)
	require.Equal(t, authReq.Turn.RunID, appendReq.Turn.RunID)
	require.Equal(t, authReq.Turn.TurnID, appendReq.Turn.TurnID)
	require.Equal(t, prompt, appendReq.Turn.Prompt)
	require.Equal(t, "llm_fallback_authorization", appendReq.Turn.Origin)
	require.Equal(t, "workspace_write", appendReq.Turn.OperationLevel)
	require.Equal(t, 2, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])

	appendPayload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, appendPayload.Messages, 2)
	require.Contains(t, appendPayload.Messages[0].Content, "provider down")
	require.Contains(t, appendPayload.Messages[1].Content, "Authorization required")
	require.Equal(t, resp.Messages[1].ID, appendPayload.Messages[0].ID)
	require.Equal(t, resp.Messages[2].ID, appendPayload.Messages[1].ID)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "waiting_authorization", got.Status)
	require.NotNil(t, got.PendingAuthorization)
	require.Equal(t, "workspace.files.write", got.PendingAuthorization.ActionName)
	require.Equal(t, "workspace_write", got.PendingAuthorization.RequiredLevel)
	require.Equal(t, "manual_tool_authorization_required", got.PendingAuthorization.Policy)
	require.Equal(t, prompt, got.PendingAuthorization.OriginalPrompt)
	require.Equal(t, "pending", got.PendingAuthorization.Status)
	require.NotNil(t, got.NextAction)
	require.Equal(t, "authorization_required", got.NextAction.Type)
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
	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)
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

		payload, ok := turn.Payload.(runtime.AgentLoopRequest)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.Equal(t, prompt, payload.Turn.Prompt)
		require.Equal(t, "operator-1", payload.Operator)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages: agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
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
	require.Len(t, appendMutations, 1)
	appendReq := appendMutations[0]
	require.Equal(t, runtime.SessionMutationAppendMessages, appendReq.Kind)
	require.Equal(t, sessionID, appendReq.Turn.SessionID)
	require.NotEmpty(t, appendReq.Turn.TaskID)
	require.NotEmpty(t, appendReq.Turn.RunID)
	require.NotEmpty(t, appendReq.Turn.TurnID)
	require.Equal(t, prompt, appendReq.Turn.Prompt)
	require.Equal(t, daemonTurnExecutionDeterministicRoute, appendReq.Turn.Origin)
	require.Equal(t, "read_only", appendReq.Turn.OperationLevel)
	require.Equal(t, 1, appendReq.Turn.Metadata["message_count"])
	require.Equal(t, false, appendReq.Turn.Metadata["planner_apply"])
	payload, ok := appendReq.Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendReq.Payload)
	require.Len(t, payload.Messages, 1)
	require.Equal(t, expectedMessage.ID, payload.Messages[0].ID)
	require.Equal(t, "step", payload.Messages[0].Phase)
}

func TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-llm", Role: "agent", Content: "ok"}
	expectedToolCall := agentToolCall{ID: "tool-llm", Name: "LLM chat"}
	lastMessageAt := time.Date(2026, 6, 7, 12, 20, 0, 0, time.UTC)
	enabled := true
	authProfiles := []agentHTTPAuthProfile{{
		ID:      "auth-llm-runner",
		Token:   "secret-token",
		Enabled: &enabled,
	}}
	history := []agentMessage{{ID: "history-1", Role: "user", Content: "hello"}}
	memorySummary := &agentMemorySummary{Summary: "memory", LastMessageAt: &lastMessageAt}
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
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

		payload, ok := req.Payload.(runtime.AgentLoopRequest)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-llm-runner", payload.Turn.SessionID)
		require.Equal(t, "task-llm-runner", payload.Turn.TaskID)
		require.Equal(t, "run-llm-runner", payload.Turn.RunID)
		require.Equal(t, "turn-llm-runner", payload.Turn.TurnID)
		require.Equal(t, "plan safely", payload.Turn.Prompt)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Equal(t, runtime.AgentLoopModeLLMToolLoop, payload.Mode)
		require.Equal(t, 3, payload.MaxLoops)
		require.Equal(t, "planner_loop", payload.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
		require.Equal(t, true, payload.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])
		require.Equal(t, "gpt-test", payload.LLMRequest.Model)
		require.Equal(t, "secret-token", payload.LLMRequest.AuthProfiles[0].Token)
		require.NotNil(t, payload.LLMRequest.AuthProfiles[0].Enabled)
		require.True(t, *payload.LLMRequest.AuthProfiles[0].Enabled)
		require.Len(t, payload.History, 1)
		require.Equal(t, "hello", payload.History[0].Content)
		require.Len(t, payload.State.LoopHistory, 1)
		require.Equal(t, "hello", payload.State.LoopHistory[0].Content)
		require.Equal(t, "memory", payload.MemorySummary.Summary)
		require.NotNil(t, payload.MemorySummary.LastMessageAt)
		require.Equal(t, lastMessageAt, *payload.MemorySummary.LastMessageAt)
		payloadServices, ok := routerServicesFromAgentLoopToolServiceBinding(payload.ToolServices)
		require.True(t, ok)
		require.Same(t, jobs, payloadServices.jobs)

		payload.History[0].Content = "mutated payload history"
		payload.Turn.Metadata["runner"] = "mutated payload runner"
		payload.MemorySummary.Summary = "mutated payload memory"
		*payload.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)
		payload.LLMRequest.AuthProfiles[0].Token = "mutated-token"
		*payload.LLMRequest.AuthProfiles[0].Enabled = false
		require.Equal(t, "llm_tool_loop", req.Turn.Metadata["runner"])

		return runtime.LLMTurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
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
		sendAgentMessageRequest{Model: "gpt-test", AuthProfiles: authProfiles},
		history,
		memorySummary,
		routerServices{jobs: jobs},
	)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
	require.Equal(t, "hello", history[0].Content)
	require.Equal(t, "memory", memorySummary.Summary)
	require.Equal(t, lastMessageAt, *memorySummary.LastMessageAt)
	require.Equal(t, "secret-token", authProfiles[0].Token)
	require.True(t, *authProfiles[0].Enabled)
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

		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-planner-stage", payload.Turn.SessionID)
		require.Equal(t, "task-planner-stage", payload.Turn.TaskID)
		require.Equal(t, "run-planner-stage", payload.Turn.RunID)
		require.Equal(t, "turn-planner-stage", payload.Turn.TurnID)
		require.Equal(t, "make a plan", payload.Turn.Prompt)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "safe_scan", payload.Turn.OperationLevel)
		require.Equal(t, "gpt-planner", payload.LLMRequest.Model)
		require.Len(t, payload.History, 1)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "done",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan),
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
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
	require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), plan)
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

		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-synthesis-stage", payload.Turn.SessionID)
		require.Equal(t, "task-synthesis-stage", payload.Turn.TaskID)
		require.Equal(t, "run-synthesis-stage", payload.Turn.RunID)
		require.Equal(t, "turn-synthesis-stage", payload.Turn.TurnID)
		require.Equal(t, "summarize tools", payload.Turn.Prompt)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "safe_scan", payload.Turn.OperationLevel)
		require.Equal(t, "gpt-synthesis", payload.LLMRequest.Model)
		require.Len(t, payload.History, 1)
		require.Len(t, payload.ToolMessages, 1)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "done",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan),
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
		[]runtime.AgentLoopMessage{{ID: "tool-message-1", Role: "tool", Content: "observation"}},
	)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
	require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), plan)
}

func TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	lastMessageAt := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)
	req := agentPlannerActionReplanCycleRequest{
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-cycle-synthesis",
			agentRuntimeTurnRef{
				TaskID: "task-cycle-synthesis",
				RunID:  "run-cycle-synthesis",
				TurnID: "turn-cycle-synthesis",
			},
			"summarize cycle tools",
			"user",
			"safe_scan",
		),
		Language: "zh-CN",
		LLMRequest: runtime.AgentLoopLLMRequest{
			Origin:         "user",
			OperationLevel: "safe_scan",
			Model:          "gpt-cycle-synthesis",
		},
		MemorySummary: agentPlannerActionReplanCycleMemorySummaryFromAgentMemorySummary(&agentMemorySummary{
			Summary:       "cycle memory",
			MessageCount:  3,
			LastMessageID: "msg-cycle-synthesis-action",
			LastMessageAt: &lastMessageAt,
		}),
	}
	state := agentPlannerActionReplanCycleStateFromLegacy(
		nil,
		nil,
		[]agentMessage{
			{ID: "history-cycle-synthesis-user", Role: "user", Content: "operator asked"},
			{ID: "msg-cycle-synthesis-action", Role: "tool", Content: "action observation"},
		},
	)
	actionMessages := agentPlannerActionReplanCycleMessagesFromAgentMessages([]agentMessage{{
		ID:      "msg-cycle-synthesis-action",
		Role:    "tool",
		Content: "action observation",
		ToolCalls: []agentToolCall{{
			ID:       "tool-cycle-synthesis-action",
			Name:     "daemon.health",
			Metadata: map[string]interface{}{"phase": "step"},
		}},
	}})

	synthesisMessage := agentMessage{ID: "msg-cycle-synthesis-summary", Role: "agent", Content: "cycle summarized"}
	synthesisTool := agentToolCall{ID: "tool-cycle-synthesis-summary", Name: "LLM synthesis", Result: map[string]any{"ok": true}}
	synthesisPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "next"},
		}},
	}

	called := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, stageReq runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		called = true
		require.Equal(t, runtime.LLMStageToolSynthesis, stageReq.Stage)
		require.Equal(t, "session-cycle-synthesis", stageReq.Turn.SessionID)
		require.Equal(t, "task-cycle-synthesis", stageReq.Turn.TaskID)
		require.Equal(t, "run-cycle-synthesis", stageReq.Turn.RunID)
		require.Equal(t, "turn-cycle-synthesis", stageReq.Turn.TurnID)
		require.Equal(t, "summarize cycle tools", stageReq.Turn.Prompt)
		require.Equal(t, "safe_scan", stageReq.Turn.OperationLevel)

		payload, ok := stageReq.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", stageReq.Payload)
		require.Equal(t, "zh-CN", payload.Language)
		require.Equal(t, "safe_scan", payload.Turn.OperationLevel)
		require.Equal(t, "gpt-cycle-synthesis", payload.LLMRequest.Model)
		require.Equal(t, "cycle memory", payload.MemorySummary.Summary)
		require.NotNil(t, payload.MemorySummary.LastMessageAt)
		require.Equal(t, lastMessageAt, *payload.MemorySummary.LastMessageAt)
		require.Equal(t, []string{"history-cycle-synthesis-user", "msg-cycle-synthesis-action"}, agentLoopMessageIDs(payload.History))
		require.Equal(t, []string{"msg-cycle-synthesis-action"}, agentLoopMessageIDs(payload.ToolMessages))

		payload.History[0].Content = "mutated payload history"
		payload.ToolMessages[0].Content = "mutated payload tool message"
		payload.MemorySummary.Summary = "mutated payload memory"
		*payload.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "cycle synthesis handled",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, []agentToolCall{synthesisTool}, synthesisPlan),
		}, nil
	}))

	loopReq := req.agentLoopRequest(runtime.AgentLoopModeReplanContinuation)
	result, err := mgr.runAgentPlannerActionReplanCycleSynthesisAgentLoop(context.Background(), loopReq, state.state, actionMessages.agentLoopMessages())

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "operator asked", state.legacyLoopHistory()[0].Content)
	require.Equal(t, "action observation", actionMessages.legacyAgentMessages()[0].Content)
	require.Equal(t, "cycle memory", req.MemorySummary.legacyAgentMemorySummary().Summary)
	require.Equal(t, lastMessageAt, *req.MemorySummary.legacyAgentMemorySummary().LastMessageAt)

	gotMessages := agentMessagesFromAgentLoopMessages(result.Messages)
	gotToolCalls := agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls)
	require.Equal(t, []agentMessage{synthesisMessage}, gotMessages)
	require.Equal(t, []agentToolCall{synthesisTool}, gotToolCalls)
	resultPlan := agentPlannerActionReplanCyclePlanFromAgentLoopPlan(result.FinalPlan)
	require.True(t, resultPlan.hasActions())
	require.True(t, resultPlan.shouldAutoContinue())
	require.Equal(t, "next", resultPlan.actions()[0].Arguments["probe"])

	synthesisMessage.Content = "mutated returned message source"
	synthesisTool.Result["ok"] = false
	synthesisPlan.Actions[0].Arguments["probe"] = "mutated-next"
	synthesisPlan.NextAction.AutoContinue = false
	gotMessages[0].Content = "mutated got message"
	gotToolCalls[0].Result["ok"] = "mutated"

	require.Equal(t, "cycle summarized", agentMessagesFromAgentLoopMessages(result.Messages)[0].Content)
	require.Equal(t, true, agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls)[0].Result["ok"])
	resultPlan = agentPlannerActionReplanCyclePlanFromAgentLoopPlan(result.FinalPlan)
	require.True(t, resultPlan.shouldAutoContinue())
	require.Equal(t, "next", resultPlan.actions()[0].Arguments["probe"])
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

		payload, ok := turn.Payload.(runtime.AgentLoopRequest)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, runtime.AgentLoopModeReplanContinuation, payload.Mode)
		require.Equal(t, "session-replan-continuation", payload.Turn.SessionID)
		require.Equal(t, "task-replan-continuation", payload.Turn.TaskID)
		require.Equal(t, "run-replan-continuation", payload.Turn.RunID)
		require.Equal(t, "turn-replan-continuation", payload.Turn.TurnID)
		require.Equal(t, "continue daemon health", payload.Turn.Prompt)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "tester", payload.Operator)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Equal(t, "gpt-continuation", payload.LLMRequest.Model)
		require.Len(t, payload.History, 1)
		require.Equal(t, "memory", payload.MemorySummary.Summary)
		require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), payload.InitialPlan)
		require.Equal(t, 2, payload.MaxLoops)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
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
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
}

func TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := agentMessage{ID: "msg-planner-action", Role: "tool", Content: "executed through executor"}
	expectedToolCall := agentToolCall{ID: "tool-planner-action", Name: "daemon.health"}
	authProfiles := []agentHTTPAuthProfile{{
		ID:      "auth-default",
		Type:    "bearer",
		BaseURL: "https://sandbox.example",
		Token:   "token",
	}}
	action := agentPlannerAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "approved-action"},
	}
	runtimeAction := agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action)
	called := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		called = true
		require.Equal(t, "session-planner-action", turn.SessionID)
		require.Equal(t, "task-planner-action", turn.TaskID)
		require.Equal(t, "run-planner-action", turn.RunID)
		require.Equal(t, "turn-planner-action", turn.TurnID)
		require.Equal(t, "approved daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "safe_scan", turn.OperationLevel)
		require.Equal(t, "turn_execution", turn.Metadata["executor"])
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Metadata["kind"])
		require.Equal(t, "safe_scan", turn.Metadata["operation_level"])
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "session-planner-action", payload.Request.Turn.SessionID)
		require.Equal(t, "en", payload.Request.Language)
		require.Equal(t, "tester", payload.Request.Operator)
		require.Equal(t, "safe_scan", payload.Request.Turn.OperationLevel)
		require.Equal(t, runtimeAction, payload.Action)
		require.Equal(t, agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles), payload.Request.Authorization.AuthProfiles)
		require.Equal(t, "auth-default", payload.Request.Authorization.DefaultAuthProfileID)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
			},
		}, nil
	}))

	messages, toolCalls, err := mgr.runRuntimePlannerActionExecution(
		context.Background(),
		"session-planner-action",
		agentRuntimeTurnRef{TaskID: "task-planner-action", RunID: "run-planner-action", TurnID: "turn-planner-action"},
		"approved daemon health",
		"en",
		"tester",
		"safe_scan",
		agentLoopToolServiceBindingFromRouterServices(routerServices{}),
		runtimeAction,
		runtime.AgentLoopAuthorization{
			AuthProfiles:         agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles),
			DefaultAuthProfileID: "auth-default",
		},
	)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), toolCalls)
}

func TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	enabled := true
	authProfiles := []agentHTTPAuthProfile{{
		ID:      "auth-cycle-action-exec",
		Type:    "bearer",
		BaseURL: "https://sandbox.example",
		Token:   "token-cycle-action-exec",
		Enabled: &enabled,
	}}
	cycleAuthProfiles := agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles)
	plan := &agentPlannerResponse{Actions: []agentPlannerAction{{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "cycle-action-exec"},
	}}}
	cyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(plan)
	plan.Actions[0].Arguments["probe"] = "mutated-source-plan"
	cyclePlan.item.Actions = nil
	cyclePlan.item.ToolCalls = nil
	authProfiles[0].Token = "mutated-source-token"
	*authProfiles[0].Enabled = false

	req := agentPlannerActionReplanCycleRequest{
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-cycle-action-exec",
			agentRuntimeTurnRef{TaskID: "task-cycle-action-exec", RunID: "run-cycle-action-exec", TurnID: "turn-cycle-action-exec"},
			"daemon health",
			"user",
			"read_only",
		),
		Language: "en",
		Operator: "tester",
		Authorization: runtime.AgentLoopAuthorization{
			AuthProfiles:         cycleAuthProfiles,
			DefaultAuthProfileID: "auth-cycle-action-exec",
		},
		ToolContext: agentPlannerActionReplanCycleToolContextFromRouterServices(routerServices{jobs: jobs}),
		Plan:        cyclePlan,
	}

	actionMessage := agentMessage{
		ID:      "msg-cycle-action-exec",
		Role:    "tool",
		Content: "cycle action executed",
		ToolCalls: []agentToolCall{{
			ID:     "tool-cycle-action-exec-message",
			Name:   "daemon.health",
			Result: map[string]any{"ok": true},
		}},
	}
	actionTool := agentToolCall{
		ID:     "tool-cycle-action-exec",
		Name:   "daemon.health",
		Result: map[string]any{"ok": true},
	}

	called := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		called = true
		require.Equal(t, "session-cycle-action-exec", turn.SessionID)
		require.Equal(t, "task-cycle-action-exec", turn.TaskID)
		require.Equal(t, "run-cycle-action-exec", turn.RunID)
		require.Equal(t, "turn-cycle-action-exec", turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Metadata["kind"])
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "session-cycle-action-exec", payload.Request.Turn.SessionID)
		require.Equal(t, "en", payload.Request.Language)
		require.Equal(t, "tester", payload.Request.Operator)
		require.Equal(t, "read_only", payload.Request.Turn.OperationLevel)
		require.Equal(t, "cycle-action-exec", stringFromAny(payload.Action.Arguments["probe"]))
		require.Len(t, payload.Request.Authorization.AuthProfiles, 1)
		require.Equal(t, "auth-cycle-action-exec", payload.Request.Authorization.DefaultAuthProfileID)
		require.Equal(t, "token-cycle-action-exec", payload.Request.Authorization.AuthProfiles[0].Token)
		require.NotNil(t, payload.Request.Authorization.AuthProfiles[0].Enabled)
		require.True(t, *payload.Request.Authorization.AuthProfiles[0].Enabled)
		payloadServices, ok := routerServicesFromAgentLoopToolServiceBinding(payload.Request.ToolServices)
		require.True(t, ok)
		require.Same(t, jobs, payloadServices.jobs)

		payload.Action.Arguments["probe"] = "mutated-payload-action"
		payload.Request.Authorization.AuthProfiles[0].Token = "mutated-payload-token"
		*payload.Request.Authorization.AuthProfiles[0].Enabled = false

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "cycle action execution handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{actionTool}),
			},
		}, nil
	}))

	loopReq := req.agentLoopRequest(runtime.AgentLoopModeReplanContinuation)
	result := mgr.runAgentPlannerActionReplanCycleActionExecutionAgentLoop(context.Background(), loopReq)

	require.True(t, called)
	require.False(t, result.AuthorizationRequired)
	require.Equal(t, "cycle-action-exec", stringFromAny(req.Plan.actions()[0].Arguments["probe"]))
	require.Equal(t, "cycle-action-exec", stringFromAny(loopReq.InitialPlan.Actions[0].Arguments["probe"]))
	require.Equal(t, "token-cycle-action-exec", req.Authorization.AuthProfiles[0].Token)
	require.True(t, *req.Authorization.AuthProfiles[0].Enabled)

	gotMessages := agentMessagesFromAgentLoopMessages(result.Messages)
	gotToolCalls := agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls)
	require.Len(t, gotMessages, 1)
	require.Len(t, gotToolCalls, 1)
	require.Equal(t, "cycle action executed", gotMessages[0].Content)
	require.Equal(t, "daemon.health", gotMessages[0].ToolCalls[0].Metadata["action_name"])
	require.Equal(t, 1, gotMessages[0].ToolCalls[0].Metadata["action_index"])
	require.Equal(t, 1, gotMessages[0].ToolCalls[0].Metadata["action_count"])
	require.NotEmpty(t, gotMessages[0].ToolCalls[0].Metadata["action_queue_id"])
	require.Equal(t, "daemon.health", gotToolCalls[0].Metadata["action_name"])
	require.Equal(t, gotMessages[0].ToolCalls[0].Metadata["action_queue_id"], gotToolCalls[0].Metadata["action_queue_id"])

	actionMessage.Content = "mutated source message"
	actionMessage.ToolCalls[0].Result["ok"] = false
	actionTool.Result["ok"] = false
	gotMessages[0].Content = "mutated got message"
	gotMessages[0].ToolCalls[0].Result["ok"] = "mutated"
	gotToolCalls[0].Result["ok"] = "mutated"

	require.Equal(t, "cycle action executed", agentMessagesFromAgentLoopMessages(result.Messages)[0].Content)
	require.Equal(t, true, agentMessagesFromAgentLoopMessages(result.Messages)[0].ToolCalls[0].Result["ok"])
	require.Equal(t, true, agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls)[0].Result["ok"])
}

func TestAgentManagerExecuteAgentPlannerActionsDispatchesPlannerActionsThroughTurnExecutor(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedToolCall := agentToolCall{ID: "tool-planner-loop-action", Name: "daemon.health"}
	expectedMessage := agentMessage{
		ID:        "msg-planner-loop-action",
		Role:      "tool",
		Content:   "planner loop action executed through executor",
		ToolCalls: []agentToolCall{expectedToolCall},
	}
	action := agentPlannerAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "planner-loop"},
	}
	called := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		called = true
		require.Equal(t, "session-planner-loop-action", turn.SessionID)
		require.Equal(t, "task-planner-loop-action", turn.TaskID)
		require.Equal(t, "run-planner-loop-action", turn.RunID)
		require.Equal(t, "turn-planner-loop-action", turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Metadata["kind"])
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "session-planner-loop-action", payload.Request.Turn.SessionID)
		require.Equal(t, "en", payload.Request.Language)
		require.Equal(t, "tester", payload.Request.Operator)
		require.Equal(t, "read_only", payload.Request.Turn.OperationLevel)
		require.Equal(t, agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action), payload.Action)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "done",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}),
			},
		}, nil
	}))

	messages, toolCalls, authorizationRequired := mgr.executeAgentPlannerActions(
		context.Background(),
		"session-planner-loop-action",
		agentRuntimeTurnRef{TaskID: "task-planner-loop-action", RunID: "run-planner-loop-action", TurnID: "turn-planner-loop-action"},
		"daemon health",
		"en",
		"tester",
		"read_only",
		routerServices{},
		&agentPlannerResponse{Actions: []agentPlannerAction{action}},
		nil,
		"",
	)

	require.True(t, called)
	require.False(t, authorizationRequired)
	require.Len(t, messages, 1)
	require.Equal(t, expectedMessage.ID, messages[0].ID)
	require.Len(t, messages[0].ToolCalls, 1)
	require.Equal(t, expectedToolCall.ID, messages[0].ToolCalls[0].ID)
	require.Equal(t, "daemon.health", messages[0].ToolCalls[0].Metadata["action_name"])
	require.Equal(t, 1, messages[0].ToolCalls[0].Metadata["action_index"])
	require.Equal(t, 1, messages[0].ToolCalls[0].Metadata["action_count"])
	require.NotEmpty(t, messages[0].ToolCalls[0].Metadata["action_queue_id"])
	require.Len(t, toolCalls, 1)
	require.Equal(t, expectedToolCall.ID, toolCalls[0].ID)
	require.Equal(t, "daemon.health", toolCalls[0].Metadata["action_name"])
	require.Equal(t, messages[0].ToolCalls[0].Metadata["action_queue_id"], toolCalls[0].Metadata["action_queue_id"])
}

func TestAgentPlannerActionReplanCycleLLMRequestFromMessageRequestKeepsOnlyLLMFields(t *testing.T) {
	enabled := true
	authProfiles := []agentHTTPAuthProfile{{
		ID:          "auth-cycle-llm",
		Name:        "Cycle Auth",
		Type:        "bearer",
		BaseURL:     "https://sandbox.example",
		Token:       "secret-token",
		Username:    "agent-user",
		Password:    "agent-pass",
		HeaderName:  "X-Agent-Auth",
		HeaderValue: "header-secret",
		Cookie:      "session=secret",
		Enabled:     &enabled,
	}}
	req := sendAgentMessageRequest{
		Message:              "operator prompt should not enter cycle llm dto",
		Operator:             "operator should not enter cycle llm dto",
		Language:             "zh-CN",
		OperationLevel:       "safe_scan",
		Origin:               "user",
		Provider:             "openai",
		Model:                "gpt-cycle",
		APIKey:               "api-key",
		BaseURL:              "https://llm.example",
		Instructions:         "planner instructions",
		AuthProfiles:         authProfiles,
		DefaultAuthProfileID: "auth-cycle-llm",
	}

	llmReq := agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req)
	authProfiles[0].Token = "mutated-token"
	*authProfiles[0].Enabled = false

	require.Equal(t, "user", llmReq.Origin)
	require.Equal(t, "safe_scan", llmReq.OperationLevel)
	require.Equal(t, "openai", llmReq.Provider)
	require.Equal(t, "gpt-cycle", llmReq.Model)
	require.Equal(t, "api-key", llmReq.APIKey)
	require.Equal(t, "https://llm.example", llmReq.BaseURL)
	require.Equal(t, "planner instructions", llmReq.Instructions)
	require.Equal(t, "auth-cycle-llm", llmReq.DefaultAuthProfileID)
	require.Len(t, llmReq.AuthProfiles, 1)
	require.Equal(t, "secret-token", llmReq.AuthProfiles[0].Token)
	require.NotNil(t, llmReq.AuthProfiles[0].Enabled)
	require.True(t, *llmReq.AuthProfiles[0].Enabled)

	converted := sendMessageRequestFromAgentLoopLLMRequest(llmReq)
	require.Empty(t, converted.Message)
	require.Empty(t, converted.Operator)
	require.Empty(t, converted.Language)
	require.Equal(t, "safe_scan", converted.OperationLevel)
	require.Equal(t, "user", converted.Origin)
	require.Equal(t, "openai", converted.Provider)
	require.Equal(t, "gpt-cycle", converted.Model)
	require.Equal(t, "api-key", converted.APIKey)
	require.Equal(t, "https://llm.example", converted.BaseURL)
	require.Equal(t, "planner instructions", converted.Instructions)
	require.Equal(t, "auth-cycle-llm", converted.DefaultAuthProfileID)
	require.Len(t, converted.AuthProfiles, 1)
	require.Equal(t, agentHTTPAuthProfile{
		ID:          "auth-cycle-llm",
		Name:        "Cycle Auth",
		Type:        "bearer",
		BaseURL:     "https://sandbox.example",
		Token:       "secret-token",
		Username:    "agent-user",
		Password:    "agent-pass",
		HeaderName:  "X-Agent-Auth",
		HeaderValue: "header-secret",
		Cookie:      "session=secret",
		Enabled:     converted.AuthProfiles[0].Enabled,
	}, converted.AuthProfiles[0])
	require.NotNil(t, converted.AuthProfiles[0].Enabled)
	require.True(t, *converted.AuthProfiles[0].Enabled)

	converted.AuthProfiles[0].Token = "converted-mutated-token"
	*converted.AuthProfiles[0].Enabled = false
	require.Equal(t, "secret-token", llmReq.AuthProfiles[0].Token)
	require.True(t, *llmReq.AuthProfiles[0].Enabled)
}

func TestAgentPlannerActionReplanCycleAuthorizationFromMessageRequestUsesPrivateAuthProfileDTO(t *testing.T) {
	enabled := true
	req := sendAgentMessageRequest{
		AuthProfiles: []agentHTTPAuthProfile{{
			ID:          "auth-cycle-action",
			Name:        "Action Auth",
			Type:        "header",
			BaseURL:     "https://sandbox-action.example",
			Token:       "action-token",
			Username:    "action-user",
			Password:    "action-pass",
			HeaderName:  "X-Action-Auth",
			HeaderValue: "action-header-secret",
			Cookie:      "action=session",
			Enabled:     &enabled,
		}},
		DefaultAuthProfileID: "auth-cycle-action",
	}

	auth := agentPlannerActionReplanCycleAuthorizationFromMessageRequest(req)
	req.AuthProfiles[0].Token = "mutated-token"
	*req.AuthProfiles[0].Enabled = false

	require.Equal(t, "auth-cycle-action", auth.DefaultAuthProfileID)
	require.Len(t, auth.AuthProfiles, 1)
	require.Equal(t, "auth-cycle-action", auth.AuthProfiles[0].ID)
	require.Equal(t, "Action Auth", auth.AuthProfiles[0].Name)
	require.Equal(t, "header", auth.AuthProfiles[0].Type)
	require.Equal(t, "https://sandbox-action.example", auth.AuthProfiles[0].BaseURL)
	require.Equal(t, "action-token", auth.AuthProfiles[0].Token)
	require.Equal(t, "action-user", auth.AuthProfiles[0].Username)
	require.Equal(t, "action-pass", auth.AuthProfiles[0].Password)
	require.Equal(t, "X-Action-Auth", auth.AuthProfiles[0].HeaderName)
	require.Equal(t, "action-header-secret", auth.AuthProfiles[0].HeaderValue)
	require.Equal(t, "action=session", auth.AuthProfiles[0].Cookie)
	require.NotNil(t, auth.AuthProfiles[0].Enabled)
	require.True(t, *auth.AuthProfiles[0].Enabled)

	converted := agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(auth.AuthProfiles)
	require.Len(t, converted, 1)
	require.Equal(t, "action-token", converted[0].Token)
	require.NotNil(t, converted[0].Enabled)
	require.True(t, *converted[0].Enabled)

	converted[0].Token = "converted-mutated-token"
	*converted[0].Enabled = false
	require.Equal(t, "action-token", auth.AuthProfiles[0].Token)
	require.True(t, *auth.AuthProfiles[0].Enabled)
}

func TestAgentPlannerActionReplanCyclePlanWrapsLegacyPlannerResponse(t *testing.T) {
	emptyCyclePlan := agentPlannerActionReplanCyclePlan{}
	require.Nil(t, emptyCyclePlan.legacyPlannerResponse())
	require.Zero(t, emptyCyclePlan.actionCount())
	require.False(t, emptyCyclePlan.hasActions())
	require.False(t, emptyCyclePlan.shouldAutoContinue())

	noActionCyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
		NextAction: &agentNextAction{AutoContinue: true},
	})
	require.Zero(t, noActionCyclePlan.actionCount())
	require.False(t, noActionCyclePlan.hasActions())
	require.False(t, noActionCyclePlan.shouldAutoContinue())

	manualStopCyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
		NextAction: &agentNextAction{AutoContinue: false},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "manual-stop"},
		}},
	})
	require.Equal(t, 1, manualStopCyclePlan.actionCount())
	require.True(t, manualStopCyclePlan.hasActions())
	require.False(t, manualStopCyclePlan.shouldAutoContinue())

	plan := &agentPlannerResponse{
		Phase:            "step_replan",
		Answer:           "cycle plan answer",
		SuggestedPrompt:  "primary cycle suggestion",
		SuggestPrompt:    "legacy cycle suggestion",
		SuggestedPrompts: []string{"continue cycle"},
		RecommendationOptions: []agentRecommendationOpt{{
			ID:      "rec-cycle-plan",
			Title:   "Continue",
			Detail:  "continue detail",
			Prompt:  "continue cycle",
			Command: "/continue-cycle",
		}},
		Task: &agentPlannerTask{
			Goal:        "inspect cycle plan",
			Kind:        "tool",
			Status:      "running",
			ActiveRunID: "run-cycle-plan",
			Summary:     "cycle plan task summary",
		},
		Todos: []agentPlannerTodo{{
			Title:         "call daemon health",
			Detail:        "collect health status",
			Status:        "in_progress",
			Priority:      2,
			ContextRefIDs: []string{"ctx-cycle-plan"},
			BlockedReason: "waiting for action",
		}},
		ContextRefs: []agentContextRef{{
			ID:      "ctx-cycle-plan",
			Kind:    "message",
			RefID:   "msg-cycle-plan",
			Label:   "Cycle plan",
			To:      "message:msg-cycle-plan",
			Summary: "cycle context summary",
		}},
		NextAction: &agentNextAction{
			Type:            "continue",
			Reason:          "more tool work",
			SuggestedPrompt: "continue with daemon health",
			AutoContinue:    true,
		},
		Reasoning: "cycle private plan reasoning",
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "cycle-plan"},
		}},
		ToolCalls: []agentPlannerAction{{
			Name:      "runs.list",
			Arguments: map[string]any{"limit": 1},
		}},
	}

	cyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(plan)
	require.Equal(t, 2, cyclePlan.actionCount())
	require.True(t, cyclePlan.hasActions())
	require.True(t, cyclePlan.shouldAutoContinue())
	require.Len(t, cyclePlan.actions(), 2)
	require.Equal(t, "cycle-plan", cyclePlan.actions()[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.actions()[1].Arguments["limit"])
	require.True(t, cyclePlan.present)
	require.Equal(t, "cycle plan answer", cyclePlan.item.Answer)
	require.Equal(t, "primary cycle suggestion", cyclePlan.item.SuggestedPrompt)
	require.Equal(t, "legacy cycle suggestion", cyclePlan.item.SuggestPrompt)
	require.Equal(t, "continue cycle", cyclePlan.item.SuggestedPrompts[0])
	require.Equal(t, "Continue", cyclePlan.item.RecommendationOptions[0].Title)
	require.Equal(t, "continue detail", cyclePlan.item.RecommendationOptions[0].Detail)
	require.Equal(t, "/continue-cycle", cyclePlan.item.RecommendationOptions[0].Command)
	require.Equal(t, "inspect cycle plan", cyclePlan.item.Task.Goal)
	require.Equal(t, "run-cycle-plan", cyclePlan.item.Task.ActiveRunID)
	require.Equal(t, "cycle plan task summary", cyclePlan.item.Task.Summary)
	require.Equal(t, "collect health status", cyclePlan.item.Todos[0].Detail)
	require.Equal(t, 2, cyclePlan.item.Todos[0].Priority)
	require.Equal(t, "waiting for action", cyclePlan.item.Todos[0].BlockedReason)
	require.Equal(t, "message:msg-cycle-plan", cyclePlan.item.ContextRefs[0].To)
	require.Equal(t, "cycle context summary", cyclePlan.item.ContextRefs[0].Summary)
	require.Equal(t, "more tool work", cyclePlan.item.NextAction.Reason)
	require.Equal(t, "continue with daemon health", cyclePlan.item.NextAction.SuggestedPrompt)
	require.Equal(t, "cycle private plan reasoning", cyclePlan.item.Reasoning)
	require.Equal(t, "cycle-plan", cyclePlan.item.Actions[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.item.ToolCalls[0].Arguments["limit"])

	plan.Phase = "mutated"
	plan.SuggestedPrompt = "mutated primary suggestion"
	plan.SuggestPrompt = "mutated legacy suggestion"
	plan.SuggestedPrompts[0] = "mutated prompt"
	plan.RecommendationOptions[0].Title = "Mutated"
	plan.RecommendationOptions[0].Detail = "mutated detail"
	plan.RecommendationOptions[0].Command = "/mutated"
	plan.Task.Goal = "mutated goal"
	plan.Task.ActiveRunID = "run-mutated"
	plan.Task.Summary = "mutated summary"
	plan.Todos[0].Title = "mutated todo"
	plan.Todos[0].Detail = "mutated todo detail"
	plan.Todos[0].Priority = 99
	plan.Todos[0].BlockedReason = "mutated blocked"
	plan.Todos[0].ContextRefIDs[0] = "ctx-mutated"
	plan.ContextRefs[0].Label = "Mutated context"
	plan.ContextRefs[0].To = "message:mutated"
	plan.ContextRefs[0].Summary = "mutated context summary"
	plan.NextAction.Type = "stop"
	plan.NextAction.Reason = "mutated reason"
	plan.NextAction.SuggestedPrompt = "mutated next prompt"
	plan.NextAction.AutoContinue = false
	plan.Reasoning = "mutated reasoning"
	plan.Actions[0].Arguments["probe"] = "mutated-probe"
	plan.ToolCalls[0].Arguments["limit"] = 99
	plan.Actions = nil
	plan.ToolCalls = nil

	require.Equal(t, 2, cyclePlan.actionCount())
	require.True(t, cyclePlan.hasActions())
	require.True(t, cyclePlan.shouldAutoContinue())
	require.Len(t, cyclePlan.actions(), 2)
	require.Equal(t, "cycle-plan", cyclePlan.actions()[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.actions()[1].Arguments["limit"])

	got := cyclePlan.legacyPlannerResponse()
	require.NotNil(t, got)
	require.NotSame(t, plan, got)
	require.Equal(t, "step_replan", got.Phase)
	require.Equal(t, "cycle plan answer", got.Answer)
	require.Equal(t, "primary cycle suggestion", got.SuggestedPrompt)
	require.Equal(t, "legacy cycle suggestion", got.SuggestPrompt)
	require.Equal(t, "continue cycle", got.SuggestedPrompts[0])
	require.Equal(t, "Continue", got.RecommendationOptions[0].Title)
	require.Equal(t, "continue detail", got.RecommendationOptions[0].Detail)
	require.Equal(t, "/continue-cycle", got.RecommendationOptions[0].Command)
	require.NotSame(t, plan.Task, got.Task)
	require.Equal(t, "inspect cycle plan", got.Task.Goal)
	require.Equal(t, "run-cycle-plan", got.Task.ActiveRunID)
	require.Equal(t, "cycle plan task summary", got.Task.Summary)
	require.Equal(t, "call daemon health", got.Todos[0].Title)
	require.Equal(t, "collect health status", got.Todos[0].Detail)
	require.Equal(t, 2, got.Todos[0].Priority)
	require.Equal(t, "waiting for action", got.Todos[0].BlockedReason)
	require.Equal(t, "ctx-cycle-plan", got.Todos[0].ContextRefIDs[0])
	require.Equal(t, "Cycle plan", got.ContextRefs[0].Label)
	require.Equal(t, "message:msg-cycle-plan", got.ContextRefs[0].To)
	require.Equal(t, "cycle context summary", got.ContextRefs[0].Summary)
	require.NotSame(t, plan.NextAction, got.NextAction)
	require.Equal(t, "continue", got.NextAction.Type)
	require.Equal(t, "more tool work", got.NextAction.Reason)
	require.Equal(t, "continue with daemon health", got.NextAction.SuggestedPrompt)
	require.True(t, got.NextAction.AutoContinue)
	require.Equal(t, "cycle private plan reasoning", got.Reasoning)
	require.Equal(t, "cycle-plan", got.Actions[0].Arguments["probe"])
	require.Equal(t, 1, got.ToolCalls[0].Arguments["limit"])
	require.Len(t, agentPlannerActions(got), 2)

	got.Phase = "mutated returned"
	got.SuggestedPrompt = "mutated returned primary"
	got.SuggestPrompt = "mutated returned legacy"
	got.SuggestedPrompts[0] = "mutated returned prompt"
	got.RecommendationOptions[0].Title = "Mutated returned"
	got.RecommendationOptions[0].Detail = "mutated returned detail"
	got.RecommendationOptions[0].Command = "/mutated-returned"
	got.Task.Goal = "mutated returned goal"
	got.Task.ActiveRunID = "run-mutated-returned"
	got.Task.Summary = "mutated returned summary"
	got.Todos[0].Title = "mutated returned todo"
	got.Todos[0].Detail = "mutated returned todo detail"
	got.Todos[0].Priority = 100
	got.Todos[0].BlockedReason = "mutated returned blocked"
	got.Todos[0].ContextRefIDs[0] = "ctx-mutated-returned"
	got.ContextRefs[0].Label = "Mutated returned context"
	got.ContextRefs[0].To = "message:mutated-returned"
	got.ContextRefs[0].Summary = "mutated returned context summary"
	got.NextAction.Type = "stop returned"
	got.NextAction.Reason = "mutated returned reason"
	got.NextAction.SuggestedPrompt = "mutated returned next prompt"
	got.NextAction.AutoContinue = false
	got.Reasoning = "mutated returned reasoning"
	got.Actions[0].Arguments["probe"] = "mutated-returned-probe"
	got.ToolCalls[0].Arguments["limit"] = 100
	got.Actions = nil
	got.ToolCalls = nil

	require.Equal(t, 2, cyclePlan.actionCount())
	require.True(t, cyclePlan.hasActions())
	require.True(t, cyclePlan.shouldAutoContinue())

	returnedActions := cyclePlan.actions()
	require.Len(t, returnedActions, 2)
	returnedActions[0].Arguments["probe"] = "mutated-returned-actions"
	returnedActions[1].Arguments["limit"] = 101
	require.Equal(t, "cycle-plan", cyclePlan.actions()[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.actions()[1].Arguments["limit"])

	cyclePlan.item.Actions = nil
	cyclePlan.item.ToolCalls = nil
	cyclePlan.item.NextAction.AutoContinue = false

	require.Equal(t, 2, cyclePlan.actionCount())
	require.True(t, cyclePlan.hasActions())
	require.True(t, cyclePlan.shouldAutoContinue())
	require.Len(t, cyclePlan.actions(), 2)
	require.Equal(t, "cycle-plan", cyclePlan.actions()[0].Arguments["probe"])
	require.Equal(t, 1, cyclePlan.actions()[1].Arguments["limit"])

	gotAgain := cyclePlan.legacyPlannerResponse()
	require.NotNil(t, gotAgain)
	require.Equal(t, "step_replan", gotAgain.Phase)
	require.Equal(t, "cycle plan answer", gotAgain.Answer)
	require.Equal(t, "primary cycle suggestion", gotAgain.SuggestedPrompt)
	require.Equal(t, "legacy cycle suggestion", gotAgain.SuggestPrompt)
	require.Equal(t, "continue cycle", gotAgain.SuggestedPrompts[0])
	require.Equal(t, "Continue", gotAgain.RecommendationOptions[0].Title)
	require.Equal(t, "continue detail", gotAgain.RecommendationOptions[0].Detail)
	require.Equal(t, "/continue-cycle", gotAgain.RecommendationOptions[0].Command)
	require.Equal(t, "inspect cycle plan", gotAgain.Task.Goal)
	require.Equal(t, "run-cycle-plan", gotAgain.Task.ActiveRunID)
	require.Equal(t, "cycle plan task summary", gotAgain.Task.Summary)
	require.Equal(t, "call daemon health", gotAgain.Todos[0].Title)
	require.Equal(t, "collect health status", gotAgain.Todos[0].Detail)
	require.Equal(t, 2, gotAgain.Todos[0].Priority)
	require.Equal(t, "waiting for action", gotAgain.Todos[0].BlockedReason)
	require.Equal(t, "ctx-cycle-plan", gotAgain.Todos[0].ContextRefIDs[0])
	require.Equal(t, "Cycle plan", gotAgain.ContextRefs[0].Label)
	require.Equal(t, "message:msg-cycle-plan", gotAgain.ContextRefs[0].To)
	require.Equal(t, "cycle context summary", gotAgain.ContextRefs[0].Summary)
	require.Equal(t, "continue", gotAgain.NextAction.Type)
	require.Equal(t, "more tool work", gotAgain.NextAction.Reason)
	require.Equal(t, "continue with daemon health", gotAgain.NextAction.SuggestedPrompt)
	require.False(t, gotAgain.NextAction.AutoContinue)
	require.Equal(t, "cycle private plan reasoning", gotAgain.Reasoning)
	require.Empty(t, agentPlannerActions(gotAgain))
}

func TestAgentPlannerActionReplanCycleMemorySummaryWrapsLegacyMemory(t *testing.T) {
	require.Nil(t, agentPlannerActionReplanCycleMemorySummary{}.legacyAgentMemorySummary())
	require.Nil(t, agentPlannerActionReplanCycleMemorySummaryFromAgentMemorySummary(nil).legacyAgentMemorySummary())

	lastMessageAt := time.Date(2026, 6, 6, 15, 45, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	updatedAt := time.Date(2026, 6, 6, 15, 50, 0, 0, time.UTC)
	memory := &agentMemorySummary{
		Summary:       "cycle memory",
		MessageCount:  7,
		LastMessageID: "msg-cycle-memory",
		LastMessageAt: &lastMessageAt,
		UpdatedAt:     updatedAt,
	}

	cycleMemory := agentPlannerActionReplanCycleMemorySummaryFromAgentMemorySummary(memory)
	require.True(t, cycleMemory.present)
	require.Equal(t, "cycle memory", cycleMemory.item.Summary)
	require.Equal(t, 7, cycleMemory.item.MessageCount)
	require.Equal(t, "msg-cycle-memory", cycleMemory.item.LastMessageID)
	require.NotNil(t, cycleMemory.item.LastMessageAt)
	require.Equal(t, originalLastMessageAt, *cycleMemory.item.LastMessageAt)
	require.Equal(t, updatedAt, cycleMemory.item.UpdatedAt)
	require.NotSame(t, memory.LastMessageAt, cycleMemory.item.LastMessageAt)

	mutatedLastMessageAt := time.Date(2026, 6, 6, 16, 0, 0, 0, time.UTC)
	memory.Summary = "mutated memory"
	memory.MessageCount = 99
	memory.LastMessageID = "msg-mutated-memory"
	*memory.LastMessageAt = mutatedLastMessageAt
	memory.UpdatedAt = mutatedLastMessageAt

	got := cycleMemory.legacyAgentMemorySummary()
	require.NotNil(t, got)
	require.Equal(t, "cycle memory", got.Summary)
	require.Equal(t, 7, got.MessageCount)
	require.Equal(t, "msg-cycle-memory", got.LastMessageID)
	require.NotNil(t, got.LastMessageAt)
	require.Equal(t, originalLastMessageAt, *got.LastMessageAt)
	require.Equal(t, updatedAt, got.UpdatedAt)
	require.NotSame(t, memory.LastMessageAt, got.LastMessageAt)
	require.NotSame(t, cycleMemory.item.LastMessageAt, got.LastMessageAt)

	returnedLastMessageAt := time.Date(2026, 6, 6, 16, 15, 0, 0, time.UTC)
	got.Summary = "mutated returned memory"
	got.MessageCount = 100
	got.LastMessageID = "msg-mutated-returned-memory"
	*got.LastMessageAt = returnedLastMessageAt
	got.UpdatedAt = returnedLastMessageAt

	gotAgain := cycleMemory.legacyAgentMemorySummary()
	require.NotNil(t, gotAgain)
	require.Equal(t, "cycle memory", gotAgain.Summary)
	require.Equal(t, 7, gotAgain.MessageCount)
	require.Equal(t, "msg-cycle-memory", gotAgain.LastMessageID)
	require.NotNil(t, gotAgain.LastMessageAt)
	require.Equal(t, originalLastMessageAt, *gotAgain.LastMessageAt)
	require.Equal(t, updatedAt, gotAgain.UpdatedAt)
	require.NotSame(t, got.LastMessageAt, gotAgain.LastMessageAt)
}

func TestAgentPlannerActionReplanCycleStateWrapsLegacyMessagesAndToolCalls(t *testing.T) {
	messageAt := time.Date(2026, 6, 6, 15, 28, 0, 0, time.UTC)
	startedAt := time.Date(2026, 6, 6, 15, 29, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 6, 15, 30, 0, 0, time.UTC)
	responseMessages := []agentMessage{{
		ID:        "msg-cycle-state-response",
		Role:      "tool",
		Content:   "response message",
		Timestamp: messageAt,
		Phase:     "tool",
		ToolCalls: []agentToolCall{{
			ID:         "tool-cycle-state-response",
			Name:       "daemon.health",
			Status:     contracts.StatusSucceeded,
			Detail:     "response detail",
			StartedAt:  startedAt,
			FinishedAt: &finishedAt,
			Result:     map[string]any{"source": "response"},
			Error:      &daemonJobError{Code: services.CodeBadRequest, Message: "response warning"},
			Metadata:   map[string]interface{}{"phase": "response"},
		}},
		Links: []agentLink{{
			To:    "/runs/run-cycle-state",
			Label: "Run",
			Kind:  "run",
		}},
		Reasoning:        "cycle state reasoning",
		SuggestedPrompts: []string{"continue cycle state"},
		RecommendationOptions: []agentRecommendationOpt{{
			ID:     "rec-cycle-state",
			Title:  "Continue",
			Detail: "Continue cycle state",
			Prompt: "continue cycle state",
		}},
	}}
	toolCalls := []agentToolCall{{
		ID:        "tool-cycle-state-top-level",
		Name:      "LLM synthesis",
		Status:    contracts.StatusSucceeded,
		Detail:    "top level detail",
		StartedAt: startedAt,
		Result:    map[string]any{"source": "tool"},
		Error:     &daemonJobError{Code: services.CodeInternal, Message: "top-level warning"},
		Metadata:  map[string]interface{}{"phase": "tool"},
	}}
	loopHistory := []agentMessage{{
		ID:      "msg-cycle-state-history",
		Role:    "agent",
		Content: "history message",
		ToolCalls: []agentToolCall{{
			ID:       "tool-cycle-state-history",
			Name:     "history.tool",
			Status:   contracts.StatusSucceeded,
			Result:   map[string]any{"source": "history"},
			Metadata: map[string]interface{}{"phase": "history"},
		}},
	}}

	state := agentPlannerActionReplanCycleStateFromLegacy(responseMessages, toolCalls, loopHistory)

	responseMessages[0].Content = "mutated response"
	responseMessages[0].ToolCalls[0].Result["source"] = "mutated response result"
	responseMessages[0].ToolCalls[0].Error.Message = "mutated response error"
	responseMessages[0].Links[0].Label = "Mutated run"
	responseMessages[0].SuggestedPrompts[0] = "mutated prompt"
	responseMessages[0].RecommendationOptions[0].Title = "Mutated"
	toolCalls[0].Result["source"] = "mutated tool result"
	toolCalls[0].Error.Message = "mutated tool error"
	loopHistory[0].Content = "mutated history"
	loopHistory[0].ToolCalls[0].Metadata["phase"] = "mutated history metadata"

	gotResponses := state.legacyResponseMessages()
	gotTools := state.legacyToolCalls()
	gotHistory := state.legacyLoopHistory()
	require.Equal(t, 1, state.responseMessages().len())
	require.Equal(t, 1, state.toolCalls().len())
	require.Len(t, gotResponses, 1)
	require.Len(t, gotResponses[0].ToolCalls, 1)
	require.Equal(t, "response message", gotResponses[0].Content)
	require.Equal(t, messageAt, gotResponses[0].Timestamp)
	require.Equal(t, "tool", gotResponses[0].Phase)
	require.Equal(t, "cycle state reasoning", gotResponses[0].Reasoning)
	require.Equal(t, "Run", gotResponses[0].Links[0].Label)
	require.Equal(t, "continue cycle state", gotResponses[0].SuggestedPrompts[0])
	require.Equal(t, "Continue", gotResponses[0].RecommendationOptions[0].Title)
	require.Equal(t, "response", gotResponses[0].ToolCalls[0].Result["source"])
	require.Equal(t, "response detail", gotResponses[0].ToolCalls[0].Detail)
	require.Equal(t, startedAt, gotResponses[0].ToolCalls[0].StartedAt)
	require.Equal(t, "response", gotResponses[0].ToolCalls[0].Metadata["phase"])
	require.NotNil(t, gotResponses[0].ToolCalls[0].FinishedAt)
	require.Equal(t, finishedAt, *gotResponses[0].ToolCalls[0].FinishedAt)
	require.NotSame(t, responseMessages[0].ToolCalls[0].FinishedAt, gotResponses[0].ToolCalls[0].FinishedAt)
	require.NotNil(t, gotResponses[0].ToolCalls[0].Error)
	require.Equal(t, services.CodeBadRequest, gotResponses[0].ToolCalls[0].Error.Code)
	require.Equal(t, "response warning", gotResponses[0].ToolCalls[0].Error.Message)
	require.NotSame(t, responseMessages[0].ToolCalls[0].Error, gotResponses[0].ToolCalls[0].Error)
	require.Len(t, gotTools, 1)
	require.Equal(t, "tool", gotTools[0].Result["source"])
	require.Equal(t, "top level detail", gotTools[0].Detail)
	require.Equal(t, startedAt, gotTools[0].StartedAt)
	require.Equal(t, "tool", gotTools[0].Metadata["phase"])
	require.NotNil(t, gotTools[0].Error)
	require.Equal(t, services.CodeInternal, gotTools[0].Error.Code)
	require.Equal(t, "top-level warning", gotTools[0].Error.Message)
	require.Len(t, gotHistory, 1)
	require.Equal(t, "history message", gotHistory[0].Content)
	require.Equal(t, "history", gotHistory[0].ToolCalls[0].Result["source"])
	require.Equal(t, "history", gotHistory[0].ToolCalls[0].Metadata["phase"])

	gotResponses[0].Content = "mutated returned response"
	gotResponses[0].ToolCalls[0].Result["source"] = "mutated returned response result"
	gotResponses[0].ToolCalls[0].Error.Message = "mutated returned response error"
	gotResponses[0].Links[0].Label = "Mutated returned run"
	gotResponses[0].SuggestedPrompts[0] = "mutated returned prompt"
	gotResponses[0].RecommendationOptions[0].Title = "Mutated returned"
	gotTools[0].Result["source"] = "mutated returned tool result"
	gotTools[0].Error.Message = "mutated returned tool error"
	gotHistory[0].Content = "mutated returned history"
	gotHistory[0].ToolCalls[0].Metadata["phase"] = "mutated returned history metadata"

	require.Equal(t, "response message", state.legacyResponseMessages()[0].Content)
	require.Equal(t, "response", state.legacyResponseMessages()[0].ToolCalls[0].Result["source"])
	require.Equal(t, "response warning", state.legacyResponseMessages()[0].ToolCalls[0].Error.Message)
	require.Equal(t, "Run", state.legacyResponseMessages()[0].Links[0].Label)
	require.Equal(t, "continue cycle state", state.legacyResponseMessages()[0].SuggestedPrompts[0])
	require.Equal(t, "Continue", state.legacyResponseMessages()[0].RecommendationOptions[0].Title)
	require.Equal(t, "tool", state.legacyToolCalls()[0].Result["source"])
	require.Equal(t, "top-level warning", state.legacyToolCalls()[0].Error.Message)
	require.Equal(t, "history message", state.legacyLoopHistory()[0].Content)
	require.Equal(t, "history", state.legacyLoopHistory()[0].ToolCalls[0].Metadata["phase"])

	phasedResponses := state.responseMessages().withPhase("step")
	phasedTools := state.toolCalls().withPhase("step")
	require.Equal(t, "response", state.legacyResponseMessages()[0].ToolCalls[0].Metadata["phase"])
	require.Nil(t, state.legacyResponseMessages()[0].ToolCalls[0].Metadata["agent_phase"])
	require.Equal(t, "tool", state.legacyToolCalls()[0].Metadata["phase"])
	require.Nil(t, state.legacyToolCalls()[0].Metadata["agent_phase"])
	require.Equal(t, "step", phasedResponses.legacyAgentMessages()[0].Phase)
	require.Equal(t, "response", phasedResponses.legacyAgentMessages()[0].ToolCalls[0].Metadata["phase"])
	require.Equal(t, "step", phasedResponses.legacyAgentMessages()[0].ToolCalls[0].Metadata["agent_phase"])
	require.Equal(t, "tool", phasedTools.legacyAgentToolCalls()[0].Metadata["phase"])
	require.Equal(t, "step", phasedTools.legacyAgentToolCalls()[0].Metadata["agent_phase"])
	phasedResponses.legacyAgentMessages()[0].Content = "mutated phased response"
	phasedTools.legacyAgentToolCalls()[0].Result["source"] = "mutated phased tool"
	require.Equal(t, "response message", state.legacyResponseMessages()[0].Content)
	require.Equal(t, "tool", state.legacyToolCalls()[0].Result["source"])

	appendedMessage := agentMessage{
		ID:      "msg-cycle-state-appended",
		Role:    "agent",
		Content: "appended message",
		ToolCalls: []agentToolCall{{
			ID:       "tool-cycle-state-appended-message",
			Name:     "appended.message.tool",
			Result:   map[string]any{"source": "appended-message"},
			Metadata: map[string]interface{}{"phase": "appended-message"},
		}},
	}
	appendedTool := agentToolCall{
		ID:       "tool-cycle-state-appended",
		Name:     "appended.tool",
		Result:   map[string]any{"source": "appended-tool"},
		Metadata: map[string]interface{}{"phase": "appended-tool"},
	}
	state.appendResponseMessages(agentPlannerActionReplanCycleMessagesFromAgentMessages([]agentMessage{appendedMessage}))
	state.appendLoopHistory(agentPlannerActionReplanCycleMessagesFromAgentMessages([]agentMessage{appendedMessage}))
	state.appendToolCalls(agentPlannerActionReplanCycleToolCallsFromAgentToolCalls([]agentToolCall{appendedTool}))
	appendedMessage.Content = "mutated appended message"
	appendedMessage.ToolCalls[0].Result["source"] = "mutated appended message result"
	appendedTool.Result["source"] = "mutated appended tool result"

	finalResponses := state.legacyResponseMessages()
	finalTools := state.legacyToolCalls()
	finalHistory := state.legacyLoopHistory()
	require.Len(t, finalResponses, 2)
	require.Equal(t, "appended message", finalResponses[1].Content)
	require.Equal(t, "appended-message", finalResponses[1].ToolCalls[0].Result["source"])
	require.Len(t, finalTools, 2)
	require.Equal(t, "appended-tool", finalTools[1].Result["source"])
	require.Len(t, finalHistory, 2)
	require.Equal(t, "appended message", finalHistory[1].Content)
}

func TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	actionMessage := agentMessage{
		ID:      "msg-cycle-action-snapshot-action",
		Role:    "tool",
		Content: "snapshot action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-cycle-action-snapshot-action",
			Name: "daemon.health",
		}},
	}
	synthesisMessage := agentMessage{ID: "msg-cycle-action-snapshot-synthesis", Role: "agent", Content: "snapshot summarized"}
	synthesisPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "snapshot summarized"}

	cyclePlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "snapshot-action"},
		}},
	})
	require.Len(t, cyclePlan.actions(), 1)
	cyclePlan.item.Actions = nil
	cyclePlan.item.ToolCalls = nil

	actionCalled := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		actionCalled = true
		require.Equal(t, "session-cycle-action-snapshot", turn.SessionID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "snapshot-action", stringFromAny(payload.Action.Arguments["probe"]))

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "snapshot action handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(actionMessage.ToolCalls),
			},
		}, nil
	}))

	synthesisCalled := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		synthesisCalled = true
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Len(t, payload.ToolMessages, 1)
		require.Equal(t, actionMessage.ID, payload.ToolMessages[0].ID)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "snapshot synthesis handled",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, nil, synthesisPlan),
		}, nil
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	result := mgr.runAgentPlannerActionReplanCycle(context.Background(), agentPlannerActionReplanCycleRequest{
		Turn:          agentPlannerActionReplanCycleTurnContext("session-cycle-action-snapshot", agentRuntimeTurnRef{TaskID: "task-cycle-action-snapshot", RunID: "run-cycle-action-snapshot", TurnID: "turn-cycle-action-snapshot"}, "daemon health", "user", "read_only"),
		Language:      "en",
		Operator:      "tester",
		LLMRequest:    runtime.AgentLoopLLMRequest{Model: "gpt-cycle-action-snapshot"},
		Authorization: runtime.AgentLoopAuthorization{},
		ToolContext:   agentPlannerActionReplanCycleToolContextFromRouterServices(routerServices{}),
		Plan:          cyclePlan,
		State: agentPlannerActionReplanCycleStateFromLegacy(
			[]agentMessage{{ID: "msg-cycle-action-snapshot-plan", Role: "agent", Content: "plan"}},
			nil,
			[]agentMessage{{ID: "history-cycle-action-snapshot", Role: "user", Content: "daemon health"}},
		),
		Options: agentPlannerActionReplanCycleOptions{ActionAppendOrigin: "planner_loop"},
	})

	require.True(t, actionCalled)
	require.True(t, synthesisCalled)
	require.True(t, result.Continue)
	require.False(t, result.Plan.hasActions())
	responses := result.State.legacyResponseMessages()
	require.Len(t, responses, 3)
	require.Equal(t, actionMessage.ID, responses[1].ID)
	require.Equal(t, "step", responses[1].Phase)
	require.Equal(t, synthesisMessage.ID, responses[2].ID)
	require.Len(t, appendMutations, 2)
	require.Equal(t, "planner_loop", appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)
}

func TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle(t *testing.T) {
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

	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	toolServices := routerServices{jobs: jobs}
	planMessage := agentMessage{ID: "msg-planner-cycle-plan", Role: "agent", Content: "planning daemon health", Phase: "plan"}
	planTool := agentToolCall{ID: "tool-planner-cycle-plan", Name: "LLM chat"}
	actionTool := agentToolCall{ID: "tool-planner-cycle-action", Name: "daemon.health"}
	actionMessage := agentMessage{
		ID:        "msg-planner-cycle-action",
		Role:      "tool",
		Content:   "health ok through shared cycle",
		ToolCalls: []agentToolCall{actionTool},
	}
	authProfiles := []agentHTTPAuthProfile{{
		ID:      "auth-planner-cycle",
		Type:    "bearer",
		BaseURL: "https://sandbox.example",
		Token:   "token",
	}}
	synthesisTool := agentToolCall{ID: "tool-planner-cycle-synthesis", Name: "LLM synthesis"}
	synthesisMessage := agentMessage{ID: "msg-planner-cycle-synthesis", Role: "agent", Content: "health summarized", Phase: "step_replan", ToolCalls: []agentToolCall{synthesisTool}}
	actionPlan := &agentPlannerResponse{
		Phase: "plan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "planner-cycle"},
		}},
	}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "health summarized"}

	turnExecutions := []runtime.TurnContext{}
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		turnExecutions = append(turnExecutions, turn)
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, "task-planner-cycle", turn.TaskID)
		require.Equal(t, "run-planner-cycle", turn.RunID)
		require.Equal(t, "turn-planner-cycle", turn.TurnID)
		require.Equal(t, "daemon health", turn.Prompt)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Metadata["kind"])
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])

		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, sessionID, payload.Request.Turn.SessionID)
		require.Equal(t, "tester", payload.Request.Operator)
		require.Equal(t, "en", payload.Request.Language)
		require.Equal(t, "read_only", payload.Request.Turn.OperationLevel)
		require.Equal(t, "planner-cycle", stringFromAny(payload.Action.Arguments["probe"]))
		require.Equal(t, agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles), payload.Request.Authorization.AuthProfiles)
		require.Equal(t, "auth-planner-cycle", payload.Request.Authorization.DefaultAuthProfileID)
		payloadServices, ok := routerServicesFromAgentLoopToolServiceBinding(payload.Request.ToolServices)
		require.True(t, ok)
		require.Same(t, jobs, payloadServices.jobs)

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "action handled by test executor",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{actionTool}),
			},
		}, nil
	}))

	llmStages := []runtime.LLMStageRequest{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		llmStages = append(llmStages, req)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-planner-cycle", req.Turn.TaskID)
		require.Equal(t, "run-planner-cycle", req.Turn.RunID)
		require.Equal(t, "turn-planner-cycle", req.Turn.TurnID)
		require.Equal(t, "daemon health", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		switch req.Stage {
		case runtime.LLMStagePlannerChat:
			payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
			require.True(t, ok, "payload type = %T", req.Payload)
			require.Equal(t, "gpt-planner-cycle", payload.LLMRequest.Model)
			require.Len(t, payload.History, 1)
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "planner handled by test runner",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{planMessage}, []agentToolCall{planTool}, actionPlan),
			}, nil
		case runtime.LLMStageToolSynthesis:
			payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
			require.True(t, ok, "payload type = %T", req.Payload)
			require.Equal(t, "gpt-planner-cycle", payload.LLMRequest.Model)
			require.Equal(t, agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(authProfiles), payload.LLMRequest.AuthProfiles)
			require.Equal(t, "auth-planner-cycle", payload.LLMRequest.DefaultAuthProfileID)
			require.Len(t, payload.History, 3)
			require.Len(t, payload.ToolMessages, 1)
			require.Equal(t, actionMessage.ID, payload.ToolMessages[0].ID)
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "synthesis handled by test runner",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, []agentToolCall{synthesisTool}, finalPlan),
			}, nil
		default:
			t.Fatalf("unexpected llm stage %s", req.Stage)
			return runtime.LLMStageResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	messages, toolCalls, err := mgr.runAgentLLMToolLoop(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-planner-cycle", RunID: "run-planner-cycle", TurnID: "turn-planner-cycle"},
		"daemon health",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{
			Model:                "gpt-planner-cycle",
			AuthProfiles:         authProfiles,
			DefaultAuthProfileID: "auth-planner-cycle",
		},
		[]agentMessage{{ID: "history-planner-cycle", Role: "user", Content: "daemon health"}},
		&agentMemorySummary{Summary: "memory"},
		toolServices,
	)

	require.NoError(t, err)
	require.Len(t, llmStages, 2)
	require.Equal(t, runtime.LLMStagePlannerChat, llmStages[0].Stage)
	require.Equal(t, runtime.LLMStageToolSynthesis, llmStages[1].Stage)
	require.Len(t, turnExecutions, 1)
	require.Len(t, appendMutations, 3)
	require.Equal(t, string(runtime.LLMStagePlannerChat), appendMutations[0].Turn.Origin)
	require.Equal(t, "planner_loop", appendMutations[1].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[2].Turn.Origin)
	require.Equal(t, true, appendMutations[0].Turn.Metadata["planner_apply"])
	require.Equal(t, false, appendMutations[1].Turn.Metadata["planner_apply"])
	require.Equal(t, true, appendMutations[2].Turn.Metadata["planner_apply"])

	actionPayload, ok := appendMutations[1].Payload.(runtime.AgentLoopAppendMessagesPayload)
	require.True(t, ok, "payload type = %T", appendMutations[1].Payload)
	require.Len(t, actionPayload.Messages, 1)
	require.Equal(t, "step", actionPayload.Messages[0].Phase)
	require.Equal(t, "daemon.health", actionPayload.Messages[0].ToolCalls[0].Metadata["action_name"])
	require.NotEmpty(t, actionPayload.Messages[0].ToolCalls[0].Metadata["action_queue_id"])

	require.Len(t, messages, 3)
	require.Equal(t, planMessage.ID, messages[0].ID)
	require.Equal(t, actionMessage.ID, messages[1].ID)
	require.Equal(t, "step", messages[1].Phase)
	require.Equal(t, synthesisMessage.ID, messages[2].ID)
	require.Len(t, toolCalls, 3)
	require.Equal(t, planTool.ID, toolCalls[0].ID)
	require.Equal(t, actionTool.ID, toolCalls[1].ID)
	require.Equal(t, "daemon.health", toolCalls[1].Metadata["action_name"])
	require.Equal(t, synthesisTool.ID, toolCalls[2].ID)
}

func TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps(t *testing.T) {
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

	planMessage := agentMessage{ID: "msg-cycle-handle-plan", Role: "agent", Content: "plan first", Phase: "plan"}
	firstActionMessage := agentMessage{
		ID:      "msg-cycle-handle-action-1",
		Role:    "tool",
		Content: "first action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-cycle-handle-action-1",
			Name: "daemon.health",
		}},
	}
	firstSynthesisMessage := agentMessage{ID: "msg-cycle-handle-synthesis-1", Role: "agent", Content: "continue with second", Phase: "step_replan"}
	secondActionMessage := agentMessage{
		ID:      "msg-cycle-handle-action-2",
		Role:    "tool",
		Content: "second action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-cycle-handle-action-2",
			Name: "daemon.health",
		}},
	}
	finalSynthesisMessage := agentMessage{ID: "msg-cycle-handle-synthesis-final", Role: "agent", Content: "done", Phase: "final_answer"}

	firstPlan := &agentPlannerResponse{
		Phase: "plan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "first"},
		}},
	}
	secondPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "second"},
		}},
	}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "done"}

	actionProbes := []string{}
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		probe := stringFromAny(payload.Action.Arguments["probe"])
		actionProbes = append(actionProbes, probe)
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "daemon.health", turn.Metadata["action_name"])
		require.Equal(t, "read_only", turn.OperationLevel)

		switch probe {
		case "first":
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "first action handled",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{firstActionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(firstActionMessage.ToolCalls),
				},
			}, nil
		case "second":
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "second action handled",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{secondActionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(secondActionMessage.ToolCalls),
				},
			}, nil
		default:
			t.Fatalf("unexpected action probe %q", probe)
			return runtime.TurnResult{}, nil
		}
	}))

	synthesisCount := 0
	synthesisHistorySnapshots := [][]string{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-cycle-handle", req.Turn.TaskID)
		require.Equal(t, "run-cycle-handle", req.Turn.RunID)
		require.Equal(t, "turn-cycle-handle", req.Turn.TurnID)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		switch req.Stage {
		case runtime.LLMStagePlannerChat:
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "planner handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{planMessage}, nil, firstPlan),
			}, nil
		case runtime.LLMStageToolSynthesis:
			synthesisCount++
			payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
			require.True(t, ok, "payload type = %T", req.Payload)
			require.Len(t, payload.ToolMessages, 1)
			synthesisHistorySnapshots = append(synthesisHistorySnapshots, agentLoopMessageIDs(payload.History))
			switch synthesisCount {
			case 1:
				require.Equal(t, firstActionMessage.ID, payload.ToolMessages[0].ID)
				return runtime.LLMStageResult{
					Status:  runtime.TurnResultFinished,
					Reason:  "first synthesis handled",
					Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{firstSynthesisMessage}, nil, secondPlan),
				}, nil
			case 2:
				require.Equal(t, secondActionMessage.ID, payload.ToolMessages[0].ID)
				return runtime.LLMStageResult{
					Status:  runtime.TurnResultFinished,
					Reason:  "final synthesis handled",
					Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{finalSynthesisMessage}, nil, finalPlan),
				}, nil
			default:
				t.Fatalf("unexpected synthesis count %d", synthesisCount)
				return runtime.LLMStageResult{}, nil
			}
		default:
			t.Fatalf("unexpected llm stage %s", req.Stage)
			return runtime.LLMStageResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	messages, toolCalls, err := mgr.runAgentLLMToolLoop(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-cycle-handle", RunID: "run-cycle-handle", TurnID: "turn-cycle-handle"},
		"daemon health twice",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-cycle-handle"},
		[]agentMessage{{ID: "history-cycle-handle", Role: "user", Content: "daemon health twice"}},
		&agentMemorySummary{Summary: "memory"},
		routerServices{},
	)

	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, actionProbes)
	require.Equal(t, 2, synthesisCount)
	require.Equal(t, [][]string{
		{"history-cycle-handle", planMessage.ID, firstActionMessage.ID},
		{"history-cycle-handle", planMessage.ID, firstActionMessage.ID, firstSynthesisMessage.ID, secondActionMessage.ID},
	}, synthesisHistorySnapshots)
	require.Len(t, appendMutations, 5)
	require.Equal(t, string(runtime.LLMStagePlannerChat), appendMutations[0].Turn.Origin)
	require.Equal(t, "planner_loop", appendMutations[1].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[2].Turn.Origin)
	require.Equal(t, "planner_loop", appendMutations[3].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[4].Turn.Origin)

	require.Len(t, messages, 5)
	require.Equal(t, planMessage.ID, messages[0].ID)
	require.Equal(t, firstActionMessage.ID, messages[1].ID)
	require.Equal(t, firstSynthesisMessage.ID, messages[2].ID)
	require.Equal(t, secondActionMessage.ID, messages[3].ID)
	require.Equal(t, finalSynthesisMessage.ID, messages[4].ID)
	require.Len(t, toolCalls, 2)
	require.Equal(t, firstActionMessage.ToolCalls[0].ID, toolCalls[0].ID)
	require.Equal(t, secondActionMessage.ToolCalls[0].ID, toolCalls[1].ID)
}

func TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps(t *testing.T) {
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

	firstActionMessage := agentMessage{
		ID:      "msg-continuation-state-action-1",
		Role:    "tool",
		Content: "first continuation action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-continuation-state-action-1",
			Name: "daemon.health",
		}},
	}
	firstSynthesisMessage := agentMessage{ID: "msg-continuation-state-synthesis-1", Role: "agent", Content: "continue with second", Phase: "step_replan"}
	secondActionMessage := agentMessage{
		ID:      "msg-continuation-state-action-2",
		Role:    "tool",
		Content: "second continuation action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-continuation-state-action-2",
			Name: "daemon.health",
		}},
	}
	finalSynthesisMessage := agentMessage{ID: "msg-continuation-state-synthesis-final", Role: "agent", Content: "done", Phase: "final_answer"}

	initialPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "first"},
		}},
	}
	secondPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "second"},
		}},
	}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "done"}

	actionProbes := []string{}
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		probe := stringFromAny(payload.Action.Arguments["probe"])
		actionProbes = append(actionProbes, probe)
		require.Equal(t, sessionID, turn.SessionID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		require.Equal(t, "read_only", turn.OperationLevel)

		switch probe {
		case "first":
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "first continuation action handled",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{firstActionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(firstActionMessage.ToolCalls),
				},
			}, nil
		case "second":
			return runtime.TurnResult{
				Status: runtime.TurnResultFinished,
				Reason: "second continuation action handled",
				Payload: runtime.AgentLoopTurnResultPayload{
					Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{secondActionMessage}),
					ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(secondActionMessage.ToolCalls),
				},
			}, nil
		default:
			t.Fatalf("unexpected action probe %q", probe)
			return runtime.TurnResult{}, nil
		}
	}))

	synthesisCount := 0
	synthesisHistorySnapshots := [][]string{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-continuation-state", req.Turn.TaskID)
		require.Equal(t, "run-continuation-state", req.Turn.RunID)
		require.Equal(t, "turn-continuation-state", req.Turn.TurnID)
		require.Equal(t, "continue daemon health twice", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)

		synthesisCount++
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Len(t, payload.ToolMessages, 1)
		synthesisHistorySnapshots = append(synthesisHistorySnapshots, agentLoopMessageIDs(payload.History))
		switch synthesisCount {
		case 1:
			require.Equal(t, firstActionMessage.ID, payload.ToolMessages[0].ID)
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "first continuation synthesis handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{firstSynthesisMessage}, nil, secondPlan),
			}, nil
		case 2:
			require.Equal(t, secondActionMessage.ID, payload.ToolMessages[0].ID)
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "final continuation synthesis handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{finalSynthesisMessage}, nil, finalPlan),
			}, nil
		default:
			t.Fatalf("unexpected synthesis count %d", synthesisCount)
			return runtime.LLMStageResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	messages, toolCalls := mgr.runAgentReplanActionContinuation(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-continuation-state", RunID: "run-continuation-state", TurnID: "turn-continuation-state"},
		"continue daemon health twice",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-continuation-state"},
		[]agentMessage{{ID: "history-continuation-state", Role: "agent", Content: "previous step"}},
		&agentMemorySummary{Summary: "memory"},
		routerServices{},
		initialPlan,
		3,
	)

	require.Equal(t, []string{"first", "second"}, actionProbes)
	require.Equal(t, 2, synthesisCount)
	require.Equal(t, [][]string{
		{"history-continuation-state", firstActionMessage.ID},
		{"history-continuation-state", firstActionMessage.ID, firstSynthesisMessage.ID, secondActionMessage.ID},
	}, synthesisHistorySnapshots)
	require.Len(t, appendMutations, 4)
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, appendMutations[2].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[3].Turn.Origin)

	require.Len(t, messages, 4)
	require.Equal(t, firstActionMessage.ID, messages[0].ID)
	require.Equal(t, firstSynthesisMessage.ID, messages[1].ID)
	require.Equal(t, secondActionMessage.ID, messages[2].ID)
	require.Equal(t, finalSynthesisMessage.ID, messages[3].ID)
	require.Len(t, toolCalls, 2)
	require.Equal(t, firstActionMessage.ToolCalls[0].ID, toolCalls[0].ID)
	require.Equal(t, secondActionMessage.ToolCalls[0].ID, toolCalls[1].ID)
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
	appendOrigins := []string{}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-continuation", req.Turn.TaskID)
		require.Equal(t, "run-continuation", req.Turn.RunID)
		require.Equal(t, "turn-continuation", req.Turn.TurnID)
		require.Equal(t, "continue daemon health", req.Turn.Prompt)
		require.Equal(t, "read_only", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Turn.Metadata["kind"])
		require.Equal(t, 1, req.Turn.Metadata["message_count"])

		payload, ok := req.Payload.(runtime.AgentLoopAppendMessagesPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Len(t, payload.Messages, 1)

		appendOrigins = append(appendOrigins, req.Turn.Origin)
		switch len(appendOrigins) {
		case 1:
			require.Equal(t, daemonTurnExecutionReplanActionContinuation, req.Turn.Origin)
			require.Equal(t, false, req.Turn.Metadata["planner_apply"])
			require.Nil(t, payload.Plan)
			require.Contains(t, payload.Messages[0].Content, "health ok")
		case 2:
			require.Equal(t, string(runtime.LLMStageToolSynthesis), req.Turn.Origin)
			require.Equal(t, true, req.Turn.Metadata["planner_apply"])
			require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), payload.Plan)
			require.Equal(t, expectedMessage.ID, payload.Messages[0].ID)
		default:
			t.Fatalf("unexpected append mutation call %d", len(appendOrigins))
		}

		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled by test mutator",
		}, nil
	}))
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

		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, sessionID, payload.Turn.SessionID)
		require.Equal(t, "continue daemon health", payload.Turn.Prompt)
		require.Equal(t, "en", payload.Language)
		require.Equal(t, "read_only", payload.Turn.OperationLevel)
		require.Len(t, payload.History, 2)
		require.Len(t, payload.ToolMessages, 1)
		require.Contains(t, payload.ToolMessages[0].Content, "health ok")

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "done",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan),
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
	require.Equal(t, []string{daemonTurnExecutionReplanActionContinuation, string(runtime.LLMStageToolSynthesis)}, appendOrigins)
	require.Len(t, messages, 2)
	require.Contains(t, messages[0].Content, "health ok")
	require.Equal(t, agentLoopMessageFromAgentMessage(expectedMessage), messages[1])
	require.Len(t, toolCalls, 2)
	require.Equal(t, "daemon.health", toolCalls[0].Name)
	require.Equal(t, agentLoopToolCallFromAgentToolCall(expectedToolCall), toolCalls[1])
}
