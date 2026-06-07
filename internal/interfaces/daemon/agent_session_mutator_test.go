package daemon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestCloneAgentLoopSessionMutationPayload(t *testing.T) {
	valuePayload := runtime.AgentLoopRecordUserMessagePayload{
		Operator: "operator-1",
		Language: "zh",
	}
	clonedValue, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopRecordUserMessagePayload](valuePayload)
	require.True(t, ok)
	valuePayload.Operator = "mutated"
	require.Equal(t, "operator-1", clonedValue.Operator)
	require.Equal(t, "zh", clonedValue.Language)

	pointerPayload := &runtime.AgentLoopAppendMessagesPayload{
		Messages: []runtime.AgentLoopMessage{{ID: "msg-1", Content: "original"}},
	}
	clonedPointer, ok := cloneAgentLoopSessionMutationPayload[runtime.AgentLoopAppendMessagesPayload](pointerPayload)
	require.True(t, ok)
	pointerPayload.Messages[0].Content = "mutated"
	require.Equal(t, "original", clonedPointer.Messages[0].Content)

	var nilPayload *runtime.AgentLoopControlSessionPayload
	_, ok = cloneAgentLoopSessionMutationPayload[runtime.AgentLoopControlSessionPayload](nilPayload)
	require.False(t, ok)

	_, ok = cloneAgentLoopSessionMutationPayload[runtime.AgentLoopControlSessionPayload]("invalid")
	require.False(t, ok)
}

func TestDaemonSessionMutationResultPayload(t *testing.T) {
	resultPayload := daemonControlSessionMutationResultPayload{
		Session: agentSession{SessionID: "agent-result"},
		Message: agentMessage{ID: "msg-result"},
		Mutated: true,
	}
	result := runtime.SessionMutationResult{Payload: resultPayload}
	cloned, ok := daemonSessionMutationResultPayload[daemonControlSessionMutationResultPayload](result)
	require.True(t, ok)
	require.Equal(t, "agent-result", cloned.Session.SessionID)
	require.Equal(t, "msg-result", cloned.Message.ID)
	require.True(t, cloned.Mutated)

	_, ok = daemonSessionMutationResultPayload[daemonControlSessionMutationResultPayload](runtime.SessionMutationResult{Payload: "invalid"})
	require.False(t, ok)
}

func TestAgentLoopRecordUserMessageResultPayloadAdapterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	lastMessageAt := now.Add(-time.Hour)
	lastCompactedAt := now.Add(-30 * time.Minute)
	appliedAt := now.Add(-15 * time.Minute)
	session := agentSession{
		SessionID:      "session-record-result",
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now,
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "safe_scan",
		Status:         "running",
		Messages: []agentMessage{{
			ID:      "msg-record-result-user",
			Role:    "user",
			Content: "scan safely",
			ToolCalls: []agentToolCall{{
				ID:       "tool-record-result-message",
				Name:     "daemon.health",
				Status:   "succeeded",
				Result:   map[string]any{"source": "message"},
				Metadata: map[string]interface{}{"phase": "history"},
			}},
			SuggestedPrompts: []string{"continue"},
		}},
		ActiveTask: &agentTask{
			ID:        "task-record-result",
			Goal:      "inspect target",
			Kind:      "chat",
			Status:    "running",
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		},
		Todos: []agentTodo{{
			ID:            "todo-record-result",
			Title:         "collect evidence",
			Status:        "running",
			Priority:      1,
			ContextRefIDs: []string{"ctx-record-result"},
			UpdatedAt:     now,
		}},
		ToolCalls: []agentToolCall{{
			ID:     "tool-record-result-top",
			Name:   "workspace.files.list",
			Status: "succeeded",
			Result: map[string]any{"path": "."},
		}},
		ContextRefs: []agentContextRef{{
			ID:      "ctx-record-result",
			Kind:    "file",
			RefID:   "notes.md",
			Label:   "notes",
			Summary: "operator notes",
		}},
		PendingChanges: []agentWorkspaceChange{{
			ID:          "change-record-result",
			Path:        "notes.md",
			DiffPreview: "+hello",
			Status:      "pending",
			CreatedAt:   now.Add(-20 * time.Minute),
			UpdatedAt:   now.Add(-10 * time.Minute),
			AppliedAt:   &appliedAt,
		}},
		PendingAuthorization: &agentAuthorizationRequest{
			ID:             "auth-record-result",
			ActionName:     "workspace.files.write",
			Arguments:      map[string]any{"path": "notes.md"},
			RequiredLevel:  "workspace_write",
			Reason:         "write requires approval",
			OriginalPrompt: "scan safely",
			Status:         "pending",
			CreatedAt:      now.Add(-5 * time.Minute),
			UpdatedAt:      now,
		},
		ApprovalState: &approvalCanonicalState{
			ApprovalKind:  approvalKindAgentAuthorization,
			State:         approvalCanonicalStatePending,
			ApprovalID:    "auth-record-result",
			ActionName:    "workspace.files.write",
			RequiredLevel: "workspace_write",
			Reason:        "write requires approval",
		},
		MemorySummary: &agentMemorySummary{
			Summary:       "session memory",
			MessageCount:  4,
			LastMessageID: "msg-old",
			LastMessageAt: &lastMessageAt,
			UpdatedAt:     now,
		},
		NextAction: &agentNextAction{
			Type:            "continue",
			Reason:          "continue safely",
			SuggestedPrompt: "continue",
			AutoContinue:    true,
		},
		RunSummary: &agentSessionRunSummary{
			RunID:    "run-record-result",
			Status:   "succeeded",
			Findings: 1,
			Summary:  "run summary",
		},
		UIState: &agentUIState{
			State:  "approval",
			Phase:  "approval",
			Tone:   "warning",
			Source: "pending_authorization",
		},
	}
	resultPayload := daemonRecordUserMessageMutationResultPayload{
		Session:       session,
		UserMessage:   session.Messages[0],
		History:       cloneAgentMessages(session.Messages),
		MemorySummary: cloneAgentMemorySummary(session.MemorySummary),
		Compaction: &agentMemoryCompaction{
			Summary: &agentMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
			CompactedCount:   2,
			RetainedCount:    80,
			PreviousCount:    1,
			LastCompactedID:  "msg-old",
			LastCompactedAt:  &lastCompactedAt,
			Trigger:          "turn_count",
			TriggerThreshold: 40,
			OccurredAt:       now,
		},
		Mutated: true,
	}

	runtimePayload := agentLoopRecordUserMessageResultPayloadFromDaemon(resultPayload)
	expectedPayload := daemonRecordUserMessageMutationResultPayloadFromAgentLoop(runtimePayload)
	resultPayload.Session.Messages[0].Content = "mutated"
	resultPayload.Session.PendingAuthorization.Arguments["path"] = "mutated"
	roundTrip := daemonRecordUserMessageMutationResultPayloadFromAgentLoop(runtimePayload)

	require.Equal(t, expectedPayload, roundTrip)
	require.Equal(t, "scan safely", roundTrip.Session.Messages[0].Content)
	require.Equal(t, "notes.md", roundTrip.Session.PendingAuthorization.Arguments["path"])
	require.Equal(t, "compacted memory", roundTrip.Compaction.Summary.Summary)
	require.Equal(t, lastCompactedAt, *roundTrip.Compaction.LastCompactedAt)
	require.True(t, roundTrip.Mutated)

	converted, ok := daemonRecordUserMessageMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, converted)
}

func TestAgentLoopControlSessionResultPayloadAdapterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 8, 10, 30, 0, 0, time.UTC)
	lastMessageAt := now.Add(-time.Hour)
	lastCompactedAt := now.Add(-30 * time.Minute)
	resultPayload := daemonControlSessionMutationResultPayload{
		Session: agentSession{
			SessionID:      "session-control-result",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now,
			Operator:       "operator-1",
			Language:       "en",
			OperationLevel: "safe_scan",
			Status:         "paused",
			Messages: []agentMessage{{
				ID:      "msg-control-result-history",
				Role:    "agent",
				Content: "paused",
				ToolCalls: []agentToolCall{{
					ID:       "tool-control-result-history",
					Name:     "daemon.control",
					Status:   "succeeded",
					Result:   map[string]any{"status": "paused"},
					Metadata: map[string]any{"phase": "control"},
				}},
				SuggestedPrompts: []string{"continue"},
			}},
			ActiveTask: &agentTask{
				ID:        "task-control-result",
				Goal:      "pause safely",
				Kind:      "analysis",
				Status:    "blocked",
				CreatedAt: now.Add(-time.Hour),
				UpdatedAt: now,
			},
			MemorySummary: &agentMemorySummary{
				Summary:       "session memory",
				MessageCount:  3,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
			NextAction: &agentNextAction{
				Type:            "continue",
				Reason:          "operator pause",
				SuggestedPrompt: "resume later",
			},
		},
		Message: agentMessage{
			ID:      "msg-control-result",
			Role:    "agent",
			Content: "control message",
			ToolCalls: []agentToolCall{{
				ID:     "tool-control-result",
				Name:   "daemon.pause",
				Status: "succeeded",
				Result: map[string]any{"action": "pause"},
			}},
			SuggestedPrompts: []string{"resume"},
		},
		CancelJobID: "job-control-result",
		Compaction: &agentMemoryCompaction{
			Summary: &agentMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
				UpdatedAt:     now,
			},
			CompactedCount:  2,
			RetainedCount:   80,
			PreviousCount:   82,
			LastCompactedID: "msg-old",
			LastCompactedAt: &lastCompactedAt,
			Trigger:         "turn_count",
			OccurredAt:      now,
		},
		Mutated: true,
	}

	runtimePayload := agentLoopControlSessionResultPayloadFromDaemon(resultPayload)
	expectedPayload := daemonControlSessionMutationResultPayloadFromAgentLoop(runtimePayload)
	resultPayload.Session.Messages[0].Content = "mutated"
	resultPayload.Session.Messages[0].ToolCalls[0].Result["status"] = "mutated"
	resultPayload.Message.ToolCalls[0].Result["action"] = "mutated"
	roundTrip := daemonControlSessionMutationResultPayloadFromAgentLoop(runtimePayload)

	require.Equal(t, expectedPayload, roundTrip)
	require.Equal(t, "paused", roundTrip.Session.Messages[0].Content)
	require.Equal(t, "paused", roundTrip.Session.Messages[0].ToolCalls[0].Result["status"])
	require.Equal(t, "pause", roundTrip.Message.ToolCalls[0].Result["action"])
	require.Equal(t, "job-control-result", roundTrip.CancelJobID)
	require.Equal(t, "compacted memory", roundTrip.Compaction.Summary.Summary)
	require.Equal(t, lastCompactedAt, *roundTrip.Compaction.LastCompactedAt)
	require.True(t, roundTrip.Mutated)

	converted, ok := daemonControlSessionMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, converted)

	convertedPointer, ok := daemonControlSessionMutationResultPayloadFromSessionMutationResult(runtime.SessionMutationResult{Payload: &runtimePayload})
	require.True(t, ok)
	require.Equal(t, roundTrip, convertedPointer)
}

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

		payload, ok := req.Payload.(runtime.AgentLoopAppendMessagesPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(plan), payload.Plan)
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

func TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef(t *testing.T) {
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
	turnRef := agentRuntimeTurnRef{
		TaskID: "task-append-turn",
		RunID:  "run-append-turn",
		TurnID: "turn-append-turn",
	}
	message := agentMessage{
		ID:        "msg-session-mutation-turn",
		Role:      "tool",
		Content:   "turn-aware append mutation",
		Timestamp: time.Date(2026, 6, 6, 12, 30, 0, 0, time.UTC),
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, turnRef.TaskID, req.Turn.TaskID)
		require.Equal(t, turnRef.RunID, req.Turn.RunID)
		require.Equal(t, turnRef.TurnID, req.Turn.TurnID)
		require.Equal(t, "continue approved run", req.Turn.Prompt)
		require.Equal(t, "planner_loop", req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Turn.Metadata["kind"])
		require.Equal(t, 1, req.Turn.Metadata["message_count"])
		require.Equal(t, false, req.Turn.Metadata["planner_apply"])
		require.Equal(t, "safe_scan", req.Turn.Metadata["operation_level"])
		require.Equal(t, "session_mutation", req.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Metadata["kind"])
		require.Equal(t, 1, req.Metadata["message_count"])
		require.Equal(t, false, req.Metadata["planner_apply"])

		payload, ok := req.Payload.(runtime.AgentLoopAppendMessagesPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Nil(t, payload.Plan)
		require.Len(t, payload.Messages, 1)
		require.Equal(t, message.ID, payload.Messages[0].ID)
		require.Equal(t, message.Content, payload.Messages[0].Content)

		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled by test mutator",
			Data:   map[string]any{"mutator": "test"},
		}, nil
	}))

	mgr.appendMessagesForTurn(context.Background(), sessionID, turnRef, "continue approved run", "planner_loop", "safe_scan", message)

	require.True(t, called)
	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, got.Messages, initialMessages, "handled runtime mutator should prevent legacy fallback")
}

func TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan(t *testing.T) {
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
	initialMessages := len(created.Session.Messages)
	turnRef := agentRuntimeTurnRef{
		TaskID: "task-planner-append",
		RunID:  "run-planner-append",
		TurnID: "turn-planner-append",
	}
	message := agentMessage{
		ID:        "msg-planner-append-turn",
		Role:      "agent",
		Content:   "planner append mutation",
		Timestamp: time.Date(2026, 6, 6, 12, 45, 0, 0, time.UTC),
	}
	plan := &agentPlannerResponse{
		Phase:  "step_replan",
		Answer: "planner state updated",
		Task: &agentPlannerTask{
			Goal:   "persist planner state through runtime seam",
			Status: "running",
		},
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(_ context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationAppendMessages, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, turnRef.TaskID, req.Turn.TaskID)
		require.Equal(t, turnRef.RunID, req.Turn.RunID)
		require.Equal(t, turnRef.TurnID, req.Turn.TurnID)
		require.Equal(t, "summarize executed tools", req.Turn.Prompt)
		require.Equal(t, string(runtime.LLMStageToolSynthesis), req.Turn.Origin)
		require.Equal(t, "read_only", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Turn.Metadata["kind"])
		require.Equal(t, 1, req.Turn.Metadata["message_count"])
		require.Equal(t, true, req.Turn.Metadata["planner_apply"])
		require.Equal(t, "read_only", req.Turn.Metadata["operation_level"])
		require.Equal(t, "session_mutation", req.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationAppendMessages), req.Metadata["kind"])
		require.Equal(t, 1, req.Metadata["message_count"])
		require.Equal(t, true, req.Metadata["planner_apply"])

		payload, ok := req.Payload.(runtime.AgentLoopAppendMessagesPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(plan), payload.Plan)
		require.Len(t, payload.Messages, 1)
		require.Equal(t, message.ID, payload.Messages[0].ID)
		require.Equal(t, message.Content, payload.Messages[0].Content)

		return runtime.SessionMutationResult{
			Status: runtime.TurnResultFinished,
			Reason: "handled by test mutator",
			Data:   map[string]any{"mutator": "test"},
		}, nil
	}))

	mgr.appendMessagesWithPlannerForTurn(context.Background(), sessionID, turnRef, "summarize executed tools", string(runtime.LLMStageToolSynthesis), "read_only", plan, message)

	require.True(t, called)
	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, got.Messages, initialMessages, "handled runtime mutator should prevent legacy fallback")
}

func TestAgentManagerAppendMessagesForTurnCompactionUsesTurnRef(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
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
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Messages = nil
	for i := 0; i < agentSessionMaxMessages+4; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		session.Messages = append(session.Messages, agentMessage{
			ID:        fmt.Sprintf("msg-append-compaction-%03d", i),
			Role:      role,
			Content:   fmt.Sprintf("historical message %03d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	mgr.mu.Unlock()

	turnRef := agentRuntimeTurnRef{
		TaskID: "task-append-compaction",
		RunID:  "run-append-compaction",
		TurnID: "turn-append-compaction",
	}
	message := agentMessage{
		ID:        "msg-append-compaction-new",
		Role:      "agent",
		Content:   "new message triggers compaction",
		Timestamp: now.Add(2 * time.Hour),
	}
	mgr.appendMessagesForTurn(context.Background(), sessionID, turnRef, "compact with turn", "planner_loop", "safe_scan", message)

	var compacted *runtime.Event
	for _, ev := range store.Events() {
		if ev.Type == "memory_compacted" {
			event := ev
			compacted = &event
			break
		}
	}
	require.NotNil(t, compacted, "expected memory_compacted event")
	require.Equal(t, turnRef.TaskID, compacted.TaskID)
	require.Equal(t, turnRef.RunID, compacted.RunID)
	require.Equal(t, turnRef.TurnID, compacted.TurnID)
	require.Equal(t, sessionID, compacted.Data["session_id"])
	require.Greater(t, compacted.Data["compacted_count"], 0)
}

func TestAgentManagerControlSessionCompactionUsesMutationTurnRef(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 13, 15, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.Messages = nil
	for i := 0; i < agentSessionMaxMessages+4; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		session.Messages = append(session.Messages, agentMessage{
			ID:        fmt.Sprintf("msg-control-compaction-%03d", i),
			Role:      role,
			Content:   fmt.Sprintf("control history %03d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	session.ActiveTask = &agentTask{
		ID:          "task-control-compaction",
		Goal:        "pause with memory compaction",
		Kind:        "analysis",
		Status:      "running",
		ActiveRunID: "run-control-compaction",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.Todos = []agentTodo{
		{ID: "todo-control-compaction", Title: "pause session", Status: "running", Priority: 1, UpdatedAt: now},
	}
	mgr.mu.Unlock()

	_, err = mgr.ControlSession(context.Background(), sessionID, agentControlRequest{
		Action:         "pause",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "safe_scan",
		Reason:         "operator pause",
	}, routerServices{})
	require.NoError(t, err)

	var compacted *runtime.Event
	for _, ev := range store.Events() {
		if ev.Type == "memory_compacted" {
			event := ev
			compacted = &event
			break
		}
	}
	require.NotNil(t, compacted, "expected memory_compacted event")
	require.Equal(t, "task-control-compaction", compacted.TaskID)
	require.Equal(t, "run-control-compaction", compacted.RunID)
	require.Equal(t, "turn_pause", compacted.TurnID)
	require.Equal(t, sessionID, compacted.Data["session_id"])
	require.Greater(t, compacted.Data["compacted_count"], 0)
}

func TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator(t *testing.T) {
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
	initialMessages := len(created.Session.Messages)

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationRecordUserMessage, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "scan safely", req.Turn.Prompt)
		require.Equal(t, "user", req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, string(runtime.SessionMutationRecordUserMessage), req.Metadata["kind"])

		payload, ok := req.Payload.(runtime.AgentLoopRecordUserMessagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "en", payload.Language)

		resultPayload, err := mgr.recordUserMessageLegacy(req.Turn.SessionID, req.Turn.Prompt, payload.Operator, payload.Language, req.Turn.OperationLevel)
		require.NoError(t, err)
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "handled by test mutator",
			Payload: agentLoopRecordUserMessageResultPayloadFromDaemon(resultPayload),
			Data:    map[string]any{"mutator": "test"},
		}, nil
	}))

	result, handled, err := mgr.runRuntimeRecordUserMessageMutation(context.Background(), sessionID, "scan safely", "operator-1", "en", "safe_scan")

	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, called)
	require.Equal(t, sessionID, result.Session.SessionID)
	require.Equal(t, "operator-1", result.Session.Operator)
	require.Equal(t, "safe_scan", result.Session.OperationLevel)
	require.Equal(t, "user", result.UserMessage.Role)
	require.Equal(t, "scan safely", result.UserMessage.Content)
	require.Len(t, result.History, initialMessages+1)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, got.Messages, initialMessages+1)
	require.Equal(t, "scan safely", got.Messages[len(got.Messages)-1].Content)
	require.NotNil(t, got.ActiveTask)
}

func TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator(t *testing.T) {
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
	turnRef := agentRuntimeTurnRef{
		TaskID: "task-request-auth",
		RunID:  "run-request-auth",
		TurnID: "turn-request-auth",
	}
	action := agentPlannerAction{
		Name:      "scan.web.safe",
		Arguments: map[string]any{"target": "https://example.com"},
	}

	records := []runtime.SessionMutationRequest{}
	legacy := daemonSessionMutator{manager: mgr}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		if req.Kind == runtime.SessionMutationRequestAuthorization {
			records = append(records, req)
		}
		return legacy.MutateSession(ctx, req)
	}))

	messages, toolCalls, authorizationRequired := mgr.executeAgentPlannerActions(
		context.Background(),
		sessionID,
		turnRef,
		"scan https://example.com",
		"en",
		"tester",
		"read_only",
		routerServices{},
		&agentPlannerResponse{Actions: []agentPlannerAction{action}},
		nil,
		"",
	)

	require.True(t, authorizationRequired)
	require.Empty(t, toolCalls)
	require.Len(t, messages, 1)
	require.Equal(t, "step", messages[0].Phase)
	require.Contains(t, messages[0].Content, "Authorization required")
	require.Len(t, records, 1)

	req := records[0]
	require.Equal(t, runtime.SessionMutationRequestAuthorization, req.Kind)
	require.Equal(t, sessionID, req.Turn.SessionID)
	require.Equal(t, turnRef.TaskID, req.Turn.TaskID)
	require.Equal(t, turnRef.RunID, req.Turn.RunID)
	require.Equal(t, turnRef.TurnID, req.Turn.TurnID)
	require.Equal(t, "scan https://example.com", req.Turn.Prompt)
	require.Equal(t, string(runtime.SessionMutationRequestAuthorization), req.Turn.Origin)
	require.Equal(t, "read_only", req.Turn.OperationLevel)
	require.Equal(t, string(runtime.SessionMutationRequestAuthorization), req.Metadata["kind"])
	require.Equal(t, "scan.web.safe", req.Metadata["action_name"])
	require.Equal(t, "safe_scan", req.Metadata["required_level"])
	require.Equal(t, "operation_level_required", req.Metadata["policy"])

	payload, ok := req.Payload.(runtime.AgentLoopAuthorizationRequestPayload)
	require.True(t, ok, "payload type = %T", req.Payload)
	require.Equal(t, "en", payload.Language)
	require.Equal(t, "scan.web.safe", payload.ActionName)
	require.Equal(t, "safe_scan", payload.RequiredLevel)
	require.Equal(t, "operation_level_required", payload.Policy)
	require.Equal(t, agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action), payload.Action)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "waiting_authorization", got.Status)
	require.NotNil(t, got.PendingAuthorization)
	require.Equal(t, "scan.web.safe", got.PendingAuthorization.ActionName)
	require.Equal(t, "safe_scan", got.PendingAuthorization.RequiredLevel)
	require.Equal(t, "operation_level_required", got.PendingAuthorization.Policy)
	require.Equal(t, "scan https://example.com", got.PendingAuthorization.OriginalPrompt)
	require.NotNil(t, got.NextAction)
	require.Equal(t, "authorization_required", got.NextAction.Type)
}

func TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator(t *testing.T) {
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
	now := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:        "task-capability-blocked",
		Goal:      "run content discovery with an equivalent tool",
		Kind:      "content_discovery",
		Status:    "running",
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.Todos = []agentTodo{
		{ID: "todo-capability-running", Title: "select content discovery tool", Status: "running", Priority: 1, UpdatedAt: now},
		{ID: "todo-capability-pending", Title: "execute bounded discovery", Status: "pending", Priority: 2, UpdatedAt: now},
		{ID: "todo-capability-done", Title: "capture scope", Status: "done", Priority: 3, UpdatedAt: now},
	}
	mgr.mu.Unlock()

	turnRef := agentRuntimeTurnRef{
		TaskID: "task-capability-blocked",
		RunID:  "run-capability-blocked",
		TurnID: "turn-capability-blocked",
	}
	operatorPrompt := "run ffuf directory enumeration on https://example.com"
	action := agentPlannerAction{
		Name:      "scan.web.safe",
		Arguments: map[string]any{"target": "https://example.com"},
	}
	expectedReason, expectedSuggestedPrompt, mismatch := agentPlannerActionCapabilityMismatch(operatorPrompt, action.Name, "en")
	require.True(t, mismatch)
	require.NotEmpty(t, expectedSuggestedPrompt)
	intentID := agentClassifyCapabilityIntent(operatorPrompt).ID
	require.Equal(t, agentIntentContentDiscovery, intentID)

	records := []runtime.SessionMutationRequest{}
	legacy := daemonSessionMutator{manager: mgr}
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		if req.Kind == runtime.SessionMutationCapabilityBlocked {
			records = append(records, req)
		}
		return legacy.MutateSession(ctx, req)
	}))

	messages, toolCalls, authorizationRequired := mgr.executeAgentPlannerActions(
		context.Background(),
		sessionID,
		turnRef,
		operatorPrompt,
		"en",
		"tester",
		"safe_scan",
		routerServices{},
		&agentPlannerResponse{Actions: []agentPlannerAction{action}},
		nil,
		"",
	)

	require.True(t, authorizationRequired)
	require.Len(t, messages, 1)
	require.Equal(t, "step_replan", messages[0].Phase)
	require.Equal(t, expectedReason, messages[0].Content)
	require.Len(t, toolCalls, 1)
	require.Equal(t, "step_replan", toolCalls[0].Metadata["agent_phase"])
	require.Len(t, records, 1)

	req := records[0]
	require.Equal(t, runtime.SessionMutationCapabilityBlocked, req.Kind)
	require.Equal(t, sessionID, req.Turn.SessionID)
	require.Equal(t, turnRef.TaskID, req.Turn.TaskID)
	require.Equal(t, turnRef.RunID, req.Turn.RunID)
	require.Equal(t, turnRef.TurnID, req.Turn.TurnID)
	require.Equal(t, operatorPrompt, req.Turn.Prompt)
	require.Equal(t, string(runtime.SessionMutationCapabilityBlocked), req.Turn.Origin)
	require.Equal(t, "safe_scan", req.Turn.OperationLevel)
	require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
	require.Equal(t, string(runtime.SessionMutationCapabilityBlocked), req.Turn.Metadata["kind"])
	require.Equal(t, "scan.web.safe", req.Turn.Metadata["action_name"])
	require.Equal(t, intentID, req.Turn.Metadata["intent_id"])
	require.Equal(t, expectedSuggestedPrompt, req.Turn.Metadata["suggested_prompt"])
	require.Equal(t, "safe_scan", req.Turn.Metadata["operation_level"])
	require.Equal(t, string(runtime.SessionMutationCapabilityBlocked), req.Metadata["kind"])
	require.Equal(t, "scan.web.safe", req.Metadata["action_name"])
	require.Equal(t, intentID, req.Metadata["intent_id"])
	require.Equal(t, expectedSuggestedPrompt, req.Metadata["suggested_prompt"])
	require.Equal(t, "safe_scan", req.Metadata["operation_level"])

	payload, ok := req.Payload.(runtime.AgentLoopCapabilityBlockedPayload)
	require.True(t, ok, "payload type = %T", req.Payload)
	require.Equal(t, "en", payload.Language)
	require.Equal(t, "scan.web.safe", payload.ActionName)
	require.Equal(t, intentID, payload.IntentID)
	require.Equal(t, expectedReason, payload.Reason)
	require.Equal(t, expectedSuggestedPrompt, payload.SuggestedPrompt)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "blocked", got.Status)
	require.NotNil(t, got.ActiveTask)
	require.Equal(t, "blocked", got.ActiveTask.Status)
	require.Len(t, got.Todos, 3)
	require.Equal(t, "blocked", got.Todos[0].Status)
	require.Equal(t, expectedReason, got.Todos[0].BlockedReason)
	require.Equal(t, "blocked", got.Todos[1].Status)
	require.Equal(t, expectedReason, got.Todos[1].BlockedReason)
	require.Equal(t, "done", got.Todos[2].Status)
	require.NotNil(t, got.NextAction)
	require.Equal(t, "blocked", got.NextAction.Type)
	require.Equal(t, trimSummary(expectedReason, 360), got.NextAction.Reason)
	require.Equal(t, expectedSuggestedPrompt, got.NextAction.SuggestedPrompt)
	require.False(t, got.NextAction.AutoContinue)
}

func TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator(t *testing.T) {
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
	initialMessages := len(created.Session.Messages)
	now := time.Date(2026, 6, 6, 12, 15, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:        "task-control-session-mutation",
		Goal:      "pause through runtime session mutator",
		Kind:      "analysis",
		Status:    "running",
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.Todos = []agentTodo{
		{ID: "todo-control-running", Title: "inspect runtime seam", Status: "running", Priority: 1, UpdatedAt: now},
		{ID: "todo-control-pending", Title: "verify mutation result", Status: "pending", Priority: 2, UpdatedAt: now},
	}
	mgr.mu.Unlock()

	req := agentControlRequest{
		Action:         "pause",
		Prompt:         "resume later",
		Reason:         "operator pause",
		Operator:       "operator-1",
		Language:       "en",
		OperationLevel: "safe_scan",
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationControlSession, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-control-session-mutation", req.Turn.TaskID)
		require.Equal(t, "run_pause", req.Turn.RunID)
		require.Equal(t, "turn_pause", req.Turn.TurnID)
		require.Equal(t, "resume later", req.Turn.Prompt)
		require.Equal(t, "operator", req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationControlSession), req.Turn.Metadata["kind"])
		require.Equal(t, string(runtime.SessionMutationControlSession), req.Metadata["kind"])
		require.Equal(t, "pause", req.Metadata["action"])
		require.Equal(t, "safe_scan", req.Metadata["operation_level"])
		require.Equal(t, false, req.Metadata["clear_input"])

		payload, ok := req.Payload.(runtime.AgentLoopControlSessionPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "pause", payload.Action)
		require.Equal(t, "operator pause", payload.Reason)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "en", payload.Language)

		resultPayload, err := mgr.controlSessionLegacy(ctx, req.Turn.SessionID, payload.Action, req.Turn.Prompt, payload.Reason, payload.RunID, payload.JobID, payload.ClearInput, payload.Operator, payload.Language, req.Turn.OperationLevel)
		require.NoError(t, err)
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "handled by test mutator",
			Payload: agentLoopControlSessionResultPayloadFromDaemon(resultPayload),
			Data:    map[string]any{"mutator": "test"},
		}, nil
	}))

	result, handled, err := mgr.runRuntimeControlSessionMutation(context.Background(), sessionID, "pause", req, "operator-1", "en", "safe_scan")

	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, called)
	require.True(t, result.Mutated)
	require.Equal(t, sessionID, result.Session.SessionID)
	require.Equal(t, "operator-1", result.Session.Operator)
	require.Equal(t, "safe_scan", result.Session.OperationLevel)
	require.Equal(t, "paused", result.Session.Status)
	require.Equal(t, "agent", result.Message.Role)
	require.NotEmpty(t, result.Message.ID)
	require.Empty(t, result.CancelJobID)
	require.NotNil(t, result.Session.ActiveTask)
	require.Equal(t, "blocked", result.Session.ActiveTask.Status)
	require.NotNil(t, result.Session.NextAction)
	require.Equal(t, "continue", result.Session.NextAction.Type)
	require.Equal(t, "operator pause", result.Session.NextAction.Reason)
	require.Equal(t, "resume later", result.Session.NextAction.SuggestedPrompt)
	require.Len(t, result.Session.Todos, 2)
	require.Equal(t, "blocked", result.Session.Todos[0].Status)
	require.Equal(t, "pending", result.Session.Todos[1].Status)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "paused", got.Status)
	require.Len(t, got.Messages, initialMessages+1)
	require.Equal(t, result.Message.ID, got.Messages[len(got.Messages)-1].ID)
}

func TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator(t *testing.T) {
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
	now := time.Date(2026, 6, 6, 12, 30, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.ActiveTask = &agentTask{
		ID:        "task-auth-session-mutation",
		Goal:      "authorize a workspace action",
		Kind:      "tool",
		Status:    "blocked",
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.Todos = []agentTodo{
		{ID: "todo-auth-running", Title: "run protected action", Status: "running", Priority: 1, UpdatedAt: now},
		{ID: "todo-auth-pending", Title: "verify result", Status: "pending", Priority: 2, UpdatedAt: now},
	}
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-session-mutation",
		ActionName:     "workspace.files.write",
		Arguments:      map[string]any{"path": "README.md"},
		RequiredLevel:  "workspace_write",
		Policy:         "manual",
		Reason:         "workspace write requires operator confirmation",
		OriginalPrompt: "write README.md",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mgr.mu.Unlock()

	req := agentAuthorizationDecisionRequest{
		Action:          "deny",
		AuthorizationID: "auth-session-mutation",
		Operator:        "operator-1",
		Language:        "en",
		OperationLevel:  "safe_scan",
		Reason:          "not within scope",
	}

	called := false
	eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
		called = true
		require.Equal(t, runtime.SessionMutationResolveAuthorization, req.Kind)
		require.Equal(t, sessionID, req.Turn.SessionID)
		require.Equal(t, "task-auth-session-mutation", req.Turn.TaskID)
		require.Equal(t, "run_auth-session-mutation", req.Turn.RunID)
		require.Equal(t, "turn_auth-session-mutation", req.Turn.TurnID)
		require.Equal(t, "operator", req.Turn.Origin)
		require.Equal(t, "safe_scan", req.Turn.OperationLevel)
		require.Equal(t, "session_mutation", req.Turn.Metadata["mutator"])
		require.Equal(t, string(runtime.SessionMutationResolveAuthorization), req.Metadata["kind"])
		require.Equal(t, "deny", req.Metadata["decision"])
		require.Equal(t, "auth-session-mutation", req.Metadata["authorization_id"])
		require.Equal(t, "safe_scan", req.Metadata["operation_level"])
		require.Equal(t, true, req.Metadata["operation_level_provided"])

		payload, ok := req.Payload.(runtime.AgentLoopResolveAuthorizationPayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "deny", payload.Decision)
		require.Equal(t, "auth-session-mutation", payload.AuthorizationID)
		require.Equal(t, "not within scope", payload.Reason)
		require.Equal(t, "operator-1", payload.Operator)
		require.Equal(t, "en", payload.Language)
		require.True(t, payload.OperationLevelProvided)

		resultPayload, err := mgr.resolveAuthorizationLegacy(ctx, req.Turn.SessionID, payload.Decision, payload.AuthorizationID, payload.Reason, payload.Operator, payload.Language, req.Turn.OperationLevel, payload.OperationLevelProvided)
		require.NoError(t, err)
		return runtime.SessionMutationResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "handled by test mutator",
			Payload: resultPayload,
			Data:    map[string]any{"mutator": "test"},
		}, nil
	}))

	result, handled, err := mgr.runRuntimeResolveAuthorizationMutation(context.Background(), sessionID, "deny", req, "operator-1", "en", "safe_scan")

	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, called)
	require.True(t, result.Mutated)
	require.Equal(t, sessionID, result.Session.SessionID)
	require.Equal(t, "operator-1", result.Session.Operator)
	require.Equal(t, "safe_scan", result.OperationLevel)
	require.Equal(t, "denied", result.Pending.Status)
	require.Equal(t, "auth-session-mutation", result.Pending.ID)
	require.Equal(t, "agent", result.Message.Role)
	require.Equal(t, "step", result.Message.Phase)
	require.Contains(t, result.Message.Content, "Authorization denied")
	require.Nil(t, result.Session.PendingAuthorization)
	require.Equal(t, "blocked", result.Session.Status)
	require.NotNil(t, result.Session.ActiveTask)
	require.Equal(t, "blocked", result.Session.ActiveTask.Status)
	require.NotNil(t, result.Session.NextAction)
	require.Equal(t, "blocked", result.Session.NextAction.Type)
	require.Equal(t, "not within scope", result.Session.NextAction.Reason)
	require.Len(t, result.Session.Todos, 2)
	require.Equal(t, "blocked", result.Session.Todos[0].Status)
	require.Equal(t, "blocked", result.Session.Todos[1].Status)

	got, err := mgr.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Nil(t, got.PendingAuthorization)
	require.Equal(t, "blocked", got.Status)
	require.Equal(t, result.Message.ID, got.Messages[len(got.Messages)-1].ID)
}

func TestAgentManagerResolveAuthorizationCompactionUsesMutationTurnRef(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "safe_scan",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID
	now := time.Date(2026, 6, 6, 13, 30, 0, 0, time.UTC)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Status = "waiting_authorization"
	session.Messages = nil
	for i := 0; i < agentSessionMaxMessages+4; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		session.Messages = append(session.Messages, agentMessage{
			ID:        fmt.Sprintf("msg-auth-compaction-%03d", i),
			Role:      role,
			Content:   fmt.Sprintf("authorization history %03d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	session.ActiveTask = &agentTask{
		ID:          "task-auth-compaction",
		Goal:        "deny with memory compaction",
		Kind:        "tool",
		Status:      "blocked",
		ActiveRunID: "run-auth-compaction",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.PendingAuthorization = &agentAuthorizationRequest{
		ID:             "auth-compaction",
		ActionName:     "workspace.files.write",
		Arguments:      map[string]any{"path": "README.md"},
		RequiredLevel:  "workspace_write",
		Policy:         "manual",
		Reason:         "workspace write requires operator confirmation",
		OriginalPrompt: "write README.md",
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mgr.mu.Unlock()

	_, err = mgr.DecideAgentAuthorization(context.Background(), sessionID, agentAuthorizationDecisionRequest{
		Action:          "deny",
		AuthorizationID: "auth-compaction",
		Operator:        "operator-1",
		Language:        "en",
		OperationLevel:  "safe_scan",
		Reason:          "not within scope",
	}, routerServices{})
	require.NoError(t, err)

	var compacted *runtime.Event
	for _, ev := range store.Events() {
		if ev.Type == "memory_compacted" {
			event := ev
			compacted = &event
			break
		}
	}
	require.NotNil(t, compacted, "expected memory_compacted event")
	require.Equal(t, "task-auth-compaction", compacted.TaskID)
	require.Equal(t, "run-auth-compaction", compacted.RunID)
	require.Equal(t, "turn_auth-compaction", compacted.TurnID)
	require.Equal(t, sessionID, compacted.Data["session_id"])
	require.Greater(t, compacted.Data["compacted_count"], 0)
}
