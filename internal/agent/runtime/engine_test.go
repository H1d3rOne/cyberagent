package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

func newTestEngine() (*Engine, *MemStore) {
	store := NewMemStore()
	return NewEngine(store), store
}

type recoverableTestStore struct {
	*MemStore
}

func newRecoverableTestEngine() (*Engine, *recoverableTestStore) {
	store := &recoverableTestStore{MemStore: NewMemStore()}
	return NewEngine(store), store
}

func (s *recoverableTestStore) ListRunsByState(_ context.Context, states ...RunState) ([]Run, error) {
	if len(states) == 0 {
		return nil, nil
	}
	wanted := make(map[string]bool, len(states))
	for _, state := range states {
		trimmed := strings.TrimSpace(string(state))
		if trimmed != "" {
			wanted[trimmed] = true
		}
	}
	if len(wanted) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Run
	for _, run := range s.runs {
		if wanted[run.State] {
			out = append(out, run)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func eventTypes(events []Event, runID string) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		if runID == "" || ev.RunID == runID {
			out = append(out, ev.Type)
		}
	}
	return out
}

func hasEvent(events []Event, typ, taskID string) bool {
	for _, ev := range events {
		if ev.Type == typ && ev.TaskID == taskID {
			return true
		}
	}
	return false
}

func assertContainsInOrder(t *testing.T, got, want []string) {
	t.Helper()
	pos := 0
	for _, w := range want {
		found := false
		for pos < len(got) {
			if got[pos] == w {
				found = true
				pos++
				break
			}
			pos++
		}
		if !found {
			t.Fatalf("missing event %q in order; got=%v want=%v", w, got, want)
		}
	}
}

func TestSubmitSimpleChat(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()

	taskID, err := e.Submit(ctx, Task{ID: "task-chat", Origin: "user", Goal: "hello"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if taskID != "task-chat" {
		t.Fatalf("Submit() taskID = %q, want task-chat", taskID)
	}

	if !hasEvent(store.Events(), "task_created", "task-chat") {
		t.Fatalf("task_created event missing: %#v", store.Events())
	}
}

func TestExecuteTurnDispatchesAttachedExecutor(t *testing.T) {
	e, _ := newTestEngine()
	ctx := context.WithValue(context.Background(), struct{}{}, "ctx-value")
	expectedTime := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	expectedTurn := TurnContext{
		SessionID:      "session-turn",
		TaskID:         "task-turn",
		RunID:          "run-turn",
		TurnID:         "turn-1",
		Prompt:         "scan example",
		Origin:         "user",
		OperationLevel: "safe_scan",
		Timestamp:      expectedTime,
		Metadata:       map[string]any{"source": "test"},
	}
	called := false
	e.AttachTurnExecutor(TurnExecutorFunc(func(gotCtx context.Context, got TurnContext) (TurnResult, error) {
		called = true
		if gotCtx != ctx {
			t.Fatalf("ExecuteTurn ctx = %#v, want original ctx", gotCtx)
		}
		if got.SessionID != expectedTurn.SessionID ||
			got.TaskID != expectedTurn.TaskID ||
			got.RunID != expectedTurn.RunID ||
			got.TurnID != expectedTurn.TurnID ||
			got.Prompt != expectedTurn.Prompt ||
			got.Origin != expectedTurn.Origin ||
			got.OperationLevel != expectedTurn.OperationLevel ||
			!got.Timestamp.Equal(expectedTurn.Timestamp) ||
			got.Metadata["source"] != expectedTurn.Metadata["source"] {
			t.Fatalf("TurnContext = %#v, want %#v", got, expectedTurn)
		}
		return TurnResult{
			Status: TurnResultFinished,
			Reason: "done",
			Data:   map[string]any{"executor": "test"},
		}, nil
	}))

	result, err := e.ExecuteTurn(ctx, expectedTurn)
	if err != nil {
		t.Fatalf("ExecuteTurn() error = %v", err)
	}
	if !called {
		t.Fatalf("ExecuteTurn() did not call attached executor")
	}
	if result.Status != TurnResultFinished || result.Reason != "done" || result.Data["executor"] != "test" {
		t.Fatalf("ExecuteTurn() result = %#v", result)
	}
}

func TestExecuteTurnWithoutExecutorReturnsStableError(t *testing.T) {
	e, _ := newTestEngine()

	result, err := e.ExecuteTurn(context.Background(), TurnContext{SessionID: "session-turn"})
	if !errors.Is(err, ErrTurnExecutorNotAttached) {
		t.Fatalf("ExecuteTurn() error = %v, want ErrTurnExecutorNotAttached", err)
	}
	if result.Status != TurnResultBlocked || !errors.Is(result.Error, ErrTurnExecutorNotAttached) || result.Reason == "" {
		t.Fatalf("ExecuteTurn() result = %#v, want blocked missing-executor result", result)
	}
}

func TestRunLLMTurnDispatchesAttachedRunner(t *testing.T) {
	e, _ := newTestEngine()
	ctx := context.WithValue(context.Background(), struct{}{}, "ctx-value")
	expectedTurn := TurnContext{
		SessionID:      "session-llm",
		TaskID:         "task-llm",
		RunID:          "run-llm",
		TurnID:         "turn-llm",
		Prompt:         "plan and execute",
		Origin:         "user",
		OperationLevel: "safe_scan",
		Metadata:       map[string]any{"source": "test"},
	}
	expectedPayload := map[string]any{"opaque": "daemon-private"}
	called := false
	e.AttachLLMTurnRunner(LLMTurnRunnerFunc(func(gotCtx context.Context, got LLMTurnRequest) (LLMTurnResult, error) {
		called = true
		if gotCtx != ctx {
			t.Fatalf("RunLLMTurn ctx = %#v, want original ctx", gotCtx)
		}
		if got.Turn.SessionID != expectedTurn.SessionID ||
			got.Turn.TaskID != expectedTurn.TaskID ||
			got.Turn.RunID != expectedTurn.RunID ||
			got.Turn.TurnID != expectedTurn.TurnID ||
			got.Turn.Prompt != expectedTurn.Prompt ||
			got.Turn.Origin != expectedTurn.Origin ||
			got.Turn.OperationLevel != expectedTurn.OperationLevel ||
			got.Turn.Metadata["source"] != expectedTurn.Metadata["source"] {
			t.Fatalf("LLMTurnRequest.Turn = %#v, want %#v", got.Turn, expectedTurn)
		}
		payload, ok := got.Payload.(map[string]any)
		if !ok || payload["opaque"] != expectedPayload["opaque"] {
			t.Fatalf("LLMTurnRequest.Payload = %#v, want %#v", got.Payload, expectedPayload)
		}
		if got.Metadata["phase"] != "llm_loop" {
			t.Fatalf("LLMTurnRequest.Metadata = %#v, want phase=llm_loop", got.Metadata)
		}
		return LLMTurnResult{
			Status:  TurnResultFinished,
			Reason:  "done",
			Payload: "result-payload",
			Data:    map[string]any{"runner": "test"},
		}, nil
	}))

	result, err := e.RunLLMTurn(ctx, LLMTurnRequest{
		Turn:     expectedTurn,
		Payload:  expectedPayload,
		Metadata: map[string]any{"phase": "llm_loop"},
	})
	if err != nil {
		t.Fatalf("RunLLMTurn() error = %v", err)
	}
	if !called {
		t.Fatalf("RunLLMTurn() did not call attached runner")
	}
	if result.Status != TurnResultFinished || result.Reason != "done" || result.Payload != "result-payload" || result.Data["runner"] != "test" {
		t.Fatalf("RunLLMTurn() result = %#v", result)
	}
}

func TestRunLLMTurnWithoutRunnerReturnsStableError(t *testing.T) {
	e, _ := newTestEngine()

	result, err := e.RunLLMTurn(context.Background(), LLMTurnRequest{Turn: TurnContext{SessionID: "session-llm"}})
	if !errors.Is(err, ErrLLMTurnRunnerNotAttached) {
		t.Fatalf("RunLLMTurn() error = %v, want ErrLLMTurnRunnerNotAttached", err)
	}
	if result.Status != TurnResultBlocked || !errors.Is(result.Error, ErrLLMTurnRunnerNotAttached) || result.Reason == "" {
		t.Fatalf("RunLLMTurn() result = %#v, want blocked missing-runner result", result)
	}
}

func TestRunLLMStageDispatchesAttachedRunner(t *testing.T) {
	e, _ := newTestEngine()
	ctx := context.WithValue(context.Background(), struct{}{}, "ctx-value")
	expectedTurn := TurnContext{
		SessionID:      "session-stage",
		TaskID:         "task-stage",
		RunID:          "run-stage",
		TurnID:         "turn-stage",
		Prompt:         "create plan",
		Origin:         "user",
		OperationLevel: "safe_scan",
		Metadata:       map[string]any{"source": "test"},
	}
	expectedPayload := map[string]any{"opaque": "daemon-private-stage"}
	called := false
	e.AttachLLMStageRunner(LLMStageRunnerFunc(func(gotCtx context.Context, got LLMStageRequest) (LLMStageResult, error) {
		called = true
		if gotCtx != ctx {
			t.Fatalf("RunLLMStage ctx = %#v, want original ctx", gotCtx)
		}
		if got.Stage != LLMStagePlannerChat {
			t.Fatalf("LLMStageRequest.Stage = %q, want %q", got.Stage, LLMStagePlannerChat)
		}
		if got.Turn.SessionID != expectedTurn.SessionID ||
			got.Turn.TaskID != expectedTurn.TaskID ||
			got.Turn.RunID != expectedTurn.RunID ||
			got.Turn.TurnID != expectedTurn.TurnID ||
			got.Turn.Prompt != expectedTurn.Prompt ||
			got.Turn.Origin != expectedTurn.Origin ||
			got.Turn.OperationLevel != expectedTurn.OperationLevel ||
			got.Turn.Metadata["source"] != expectedTurn.Metadata["source"] {
			t.Fatalf("LLMStageRequest.Turn = %#v, want %#v", got.Turn, expectedTurn)
		}
		payload, ok := got.Payload.(map[string]any)
		if !ok || payload["opaque"] != expectedPayload["opaque"] {
			t.Fatalf("LLMStageRequest.Payload = %#v, want %#v", got.Payload, expectedPayload)
		}
		if got.Metadata["phase"] != "planner_chat" {
			t.Fatalf("LLMStageRequest.Metadata = %#v, want phase=planner_chat", got.Metadata)
		}
		return LLMStageResult{
			Status:  TurnResultFinished,
			Reason:  "done",
			Payload: "stage-result-payload",
			Data:    map[string]any{"runner": "test-stage"},
		}, nil
	}))

	result, err := e.RunLLMStage(ctx, LLMStageRequest{
		Turn:     expectedTurn,
		Stage:    LLMStagePlannerChat,
		Payload:  expectedPayload,
		Metadata: map[string]any{"phase": "planner_chat"},
	})
	if err != nil {
		t.Fatalf("RunLLMStage() error = %v", err)
	}
	if !called {
		t.Fatalf("RunLLMStage() did not call attached runner")
	}
	if result.Status != TurnResultFinished || result.Reason != "done" || result.Payload != "stage-result-payload" || result.Data["runner"] != "test-stage" {
		t.Fatalf("RunLLMStage() result = %#v", result)
	}
}

func TestRunLLMStageWithoutRunnerReturnsStableError(t *testing.T) {
	e, _ := newTestEngine()

	result, err := e.RunLLMStage(context.Background(), LLMStageRequest{
		Turn:  TurnContext{SessionID: "session-stage"},
		Stage: LLMStagePlannerChat,
	})
	if !errors.Is(err, ErrLLMStageRunnerNotAttached) {
		t.Fatalf("RunLLMStage() error = %v, want ErrLLMStageRunnerNotAttached", err)
	}
	if result.Status != TurnResultBlocked || !errors.Is(result.Error, ErrLLMStageRunnerNotAttached) || result.Reason == "" {
		t.Fatalf("RunLLMStage() result = %#v, want blocked missing-runner result", result)
	}
}

func TestMutateSessionDispatchesAttachedMutator(t *testing.T) {
	e, _ := newTestEngine()
	ctx := context.WithValue(context.Background(), struct{}{}, "ctx-value")
	expectedTurn := TurnContext{
		SessionID:      "session-mutation",
		TaskID:         "task-mutation",
		RunID:          "run-mutation",
		TurnID:         "turn-mutation",
		Prompt:         "append session messages",
		Origin:         "user",
		OperationLevel: "safe_scan",
		Metadata:       map[string]any{"source": "test"},
	}
	expectedPayload := map[string]any{"opaque": "daemon-private-session"}
	called := false
	e.AttachSessionMutator(SessionMutatorFunc(func(gotCtx context.Context, got SessionMutationRequest) (SessionMutationResult, error) {
		called = true
		if gotCtx != ctx {
			t.Fatalf("MutateSession ctx = %#v, want original ctx", gotCtx)
		}
		if got.Kind != SessionMutationAppendMessages {
			t.Fatalf("SessionMutationRequest.Kind = %q, want %q", got.Kind, SessionMutationAppendMessages)
		}
		if got.Turn.SessionID != expectedTurn.SessionID ||
			got.Turn.TaskID != expectedTurn.TaskID ||
			got.Turn.RunID != expectedTurn.RunID ||
			got.Turn.TurnID != expectedTurn.TurnID ||
			got.Turn.Prompt != expectedTurn.Prompt ||
			got.Turn.Origin != expectedTurn.Origin ||
			got.Turn.OperationLevel != expectedTurn.OperationLevel ||
			got.Turn.Metadata["source"] != expectedTurn.Metadata["source"] {
			t.Fatalf("SessionMutationRequest.Turn = %#v, want %#v", got.Turn, expectedTurn)
		}
		payload, ok := got.Payload.(map[string]any)
		if !ok || payload["opaque"] != expectedPayload["opaque"] {
			t.Fatalf("SessionMutationRequest.Payload = %#v, want %#v", got.Payload, expectedPayload)
		}
		if got.Metadata["phase"] != "session_append" {
			t.Fatalf("SessionMutationRequest.Metadata = %#v, want phase=session_append", got.Metadata)
		}
		return SessionMutationResult{
			Status:  TurnResultFinished,
			Reason:  "done",
			Payload: "session-result-payload",
			Data:    map[string]any{"mutator": "test-session"},
		}, nil
	}))

	result, err := e.MutateSession(ctx, SessionMutationRequest{
		Turn:     expectedTurn,
		Kind:     SessionMutationAppendMessages,
		Payload:  expectedPayload,
		Metadata: map[string]any{"phase": "session_append"},
	})
	if err != nil {
		t.Fatalf("MutateSession() error = %v", err)
	}
	if !called {
		t.Fatalf("MutateSession() did not call attached mutator")
	}
	if result.Status != TurnResultFinished || result.Reason != "done" || result.Payload != "session-result-payload" || result.Data["mutator"] != "test-session" {
		t.Fatalf("MutateSession() result = %#v", result)
	}
}

func TestMutateSessionWithoutMutatorReturnsStableError(t *testing.T) {
	e, _ := newTestEngine()

	result, err := e.MutateSession(context.Background(), SessionMutationRequest{
		Turn: TurnContext{SessionID: "session-mutation"},
		Kind: SessionMutationAppendMessages,
	})
	if !errors.Is(err, ErrSessionMutatorNotAttached) {
		t.Fatalf("MutateSession() error = %v, want ErrSessionMutatorNotAttached", err)
	}
	if result.Status != TurnResultBlocked || !errors.Is(result.Error, ErrSessionMutatorNotAttached) || result.Reason == "" {
		t.Fatalf("MutateSession() result = %#v, want blocked missing-mutator result", result)
	}
}

func TestBeginTurnPublishesLegacyLifecycleEvents(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()
	longPrompt := strings.Repeat("p", 1001)
	longGoal := strings.Repeat("g", 361)
	startedAt := time.Date(2026, 6, 6, 12, 30, 0, 0, time.UTC)

	turn, err := e.BeginTurn(ctx, TurnContext{
		SessionID:      "session-begin",
		TaskID:         "task-begin",
		RunID:          "run-begin",
		TurnID:         "turn-begin",
		MessageID:      "msg-begin",
		Prompt:         longPrompt,
		Goal:           longGoal,
		OperationLevel: "safe_scan",
		Timestamp:      startedAt,
	})
	if err != nil {
		t.Fatalf("BeginTurn() error = %v", err)
	}
	if turn.Origin != "user" || turn.TurnIndex != 1 {
		t.Fatalf("BeginTurn() normalized turn = %#v, want origin=user turn_index=1", turn)
	}

	task, err := store.LoadTask(ctx, "task-begin")
	if err != nil {
		t.Fatalf("LoadTask() error = %v", err)
	}
	if task.SessionID != "session-begin" || task.Origin != "user" || task.OperationLevel != "safe_scan" {
		t.Fatalf("task snapshot = %#v", task)
	}
	if len(task.Goal) != 363 || !strings.HasSuffix(task.Goal, "…") {
		t.Fatalf("task goal was not legacy-trimmed: len=%d value=%q", len(task.Goal), task.Goal)
	}
	if len(task.Prompt) != 1003 || !strings.HasSuffix(task.Prompt, "…") {
		t.Fatalf("task prompt was not legacy-trimmed: len=%d", len(task.Prompt))
	}
	run, err := store.LoadRun(ctx, "run-begin")
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if run.TaskID != "task-begin" || run.State != string(RunRunning) || run.Turn != 1 {
		t.Fatalf("run snapshot = %#v", run)
	}
	e.mu.Lock()
	savedTurn := e.turns["turn-begin"]
	e.mu.Unlock()
	if savedTurn == nil || savedTurn.State != string(TurnRunning) || savedTurn.Index != 1 {
		t.Fatalf("saved turn = %#v, want running index=1", savedTurn)
	}

	events := store.Events()
	got := eventTypes(events, "run-begin")
	want := []string{EventRunStarted, EventTurnStarted}
	assertContainsInOrder(t, got, want)
	var sawTaskCreated, sawTurnStarted bool
	for _, ev := range events {
		switch ev.Type {
		case EventTaskCreated:
			if ev.TaskID == "task-begin" {
				sawTaskCreated = true
				if ev.Data["session_id"] != "session-begin" || ev.Data["origin"] != "user" {
					t.Fatalf("task_created data = %#v", ev.Data)
				}
			}
		case EventTurnStarted:
			sawTurnStarted = true
			if ev.TaskID != "task-begin" || ev.RunID != "run-begin" || ev.TurnID != "turn-begin" {
				t.Fatalf("turn_started IDs = %#v", ev)
			}
			if ev.Data["session_id"] != "session-begin" || ev.Data["message_id"] != "msg-begin" || ev.Data["prompt"] != longPrompt || ev.Data["origin"] != "user" {
				t.Fatalf("turn_started data = %#v", ev.Data)
			}
			if !ev.Timestamp.Equal(startedAt) {
				t.Fatalf("turn_started timestamp = %s, want %s", ev.Timestamp, startedAt)
			}
		}
	}
	if !sawTaskCreated || !sawTurnStarted {
		t.Fatalf("expected task_created and turn_started, got %#v", events)
	}
}

func TestFinishTurnPublishesFinishedAndApprovalEvents(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    TurnResultStatus
		wantRun   RunState
		wantEvent string
	}{
		{name: "finished", status: TurnResultFinished, wantRun: RunDone, wantEvent: EventTaskFinished},
		{name: "approval", status: TurnResultApprovalRequired, wantRun: RunAwaitingApproval, wantEvent: EventApprovalRequired},
		{name: "failed-status", status: TurnResultFailed, wantRun: RunFailed, wantEvent: EventTaskFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, store := newTestEngine()
			ctx := context.Background()
			turn, err := e.BeginTurn(ctx, TurnContext{
				SessionID: "session-" + tc.name,
				TaskID:    "task-" + tc.name,
				RunID:     "run-" + tc.name,
				TurnID:    "turn-" + tc.name,
				MessageID: "msg-" + tc.name,
				Prompt:    "finish " + tc.name,
				Origin:    "user",
			})
			if err != nil {
				t.Fatalf("BeginTurn() error = %v", err)
			}

			if err := e.FinishTurn(ctx, turn, TurnResult{Status: tc.status, Reason: "done"}); err != nil {
				t.Fatalf("FinishTurn() error = %v", err)
			}
			run, err := store.LoadRun(ctx, turn.RunID)
			if err != nil {
				t.Fatalf("LoadRun() error = %v", err)
			}
			if run.State != string(tc.wantRun) {
				t.Fatalf("run state = %q, want %q", run.State, tc.wantRun)
			}
			e.mu.Lock()
			savedTurn := e.turns[turn.TurnID]
			e.mu.Unlock()
			if savedTurn == nil || savedTurn.State != string(TurnFinished) {
				t.Fatalf("saved turn = %#v, want finished", savedTurn)
			}

			got := eventTypes(store.Events(), turn.RunID)
			assertContainsInOrder(t, got, []string{EventRunStarted, EventTurnFinished, tc.wantEvent})
			var sawTurnFinished bool
			for _, ev := range store.Events() {
				if ev.Type != EventTurnFinished || ev.RunID != turn.RunID {
					continue
				}
				sawTurnFinished = true
				if ev.Data["session_id"] != turn.SessionID || ev.Data["status"] != string(tc.status) || ev.Data["reason"] != "done" {
					t.Fatalf("turn_finished data = %#v", ev.Data)
				}
			}
			if !sawTurnFinished {
				t.Fatalf("turn_finished missing: %#v", store.Events())
			}
		})
	}
}

func TestFinishTurnPublishesTurnFailedOnResultError(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()
	turn, err := e.BeginTurn(ctx, TurnContext{
		SessionID: "session-error",
		TaskID:    "task-error",
		RunID:     "run-error",
		TurnID:    "turn-error",
		MessageID: "msg-error",
		Prompt:    "error",
		Origin:    "user",
	})
	if err != nil {
		t.Fatalf("BeginTurn() error = %v", err)
	}

	boom := errors.New("boom")
	if err := e.FinishTurn(ctx, turn, TurnResult{Status: TurnResultFailed, Reason: "boom", Error: boom}); err != nil {
		t.Fatalf("FinishTurn() error = %v", err)
	}
	run, err := store.LoadRun(ctx, turn.RunID)
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if run.State != string(RunFailed) {
		t.Fatalf("run state = %q, want failed", run.State)
	}
	e.mu.Lock()
	savedTurn := e.turns[turn.TurnID]
	e.mu.Unlock()
	if savedTurn == nil || savedTurn.State != string(TurnFailed) {
		t.Fatalf("saved turn = %#v, want failed", savedTurn)
	}

	got := eventTypes(store.Events(), turn.RunID)
	assertContainsInOrder(t, got, []string{EventRunStarted, EventTurnFailed, EventTaskFailed})
	for _, ev := range store.Events() {
		if ev.Type != EventTurnFailed {
			continue
		}
		if ev.Data["session_id"] != turn.SessionID || ev.Data["status"] != string(TurnResultFailed) || ev.Data["reason"] != "boom" || ev.Data["error"] != "boom" {
			t.Fatalf("turn_failed data = %#v", ev.Data)
		}
		return
	}
	t.Fatalf("turn_failed missing: %#v", store.Events())
}

func TestEventEnvelopeHasTopLevelSessionID(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()

	e.Publish(ctx, Event{
		Type: "model_token",
		Data: map[string]any{"session_id": "session-envelope", "text": "hello"},
	})

	events := store.Events()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1: %#v", len(events), events)
	}
	if events[0].SessionID != "session-envelope" {
		t.Fatalf("Event.SessionID = %q, want session-envelope", events[0].SessionID)
	}
	encoded, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("Marshal(Event) error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("Unmarshal(Event JSON) error = %v", err)
	}
	if payload["session_id"] != "session-envelope" {
		t.Fatalf("top-level session_id = %#v, want session-envelope; payload=%s", payload["session_id"], encoded)
	}
}

func TestLLMToolCall(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()

	if _, err := e.Submit(ctx, Task{ID: "task-tool", Origin: "user", Goal: "scan target"}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-tool", TaskID: "task-tool", State: string(RunRunning)}); err != nil {
		t.Fatalf("SubmitRun() error = %v", err)
	}
	e.runTurnLLMToolLoop(ctx, "run-tool")

	got := eventTypes(store.Events(), "run-tool")
	want := []string{"run_started", "turn_started", "model_token", "model_token", "model_token", "tool_started", "tool_finished"}
	assertContainsInOrder(t, got, want)
}

func TestScheduleOriginTask(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()

	if _, err := e.Submit(ctx, Task{ID: "task-schedule", Origin: "schedule", Goal: "nightly check"}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	events := store.Events()
	for _, ev := range events {
		if ev.Type == "task_created" && ev.TaskID == "task-schedule" {
			if got := ev.Data["origin"]; got != "schedule" {
				t.Fatalf("origin = %#v, want schedule", got)
			}
			return
		}
	}
	t.Fatalf("task_created schedule event missing: %#v", events)
}

func TestApprovalPauseResume(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()

	if _, err := e.Submit(ctx, Task{ID: "task-approval", Origin: "user", Goal: "needs approval"}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-approval", TaskID: "task-approval", State: string(RunRunning)}); err != nil {
		t.Fatalf("SubmitRun() error = %v", err)
	}

	if err := e.RequestApproval("run-approval", "manual gate"); err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}
	run, err := store.LoadRun(ctx, "run-approval")
	if err != nil {
		t.Fatalf("LoadRun() after RequestApproval error = %v", err)
	}
	if run.State != string(RunAwaitingApproval) {
		t.Fatalf("run state after RequestApproval = %q, want %q", run.State, RunAwaitingApproval)
	}

	if err := e.ResolveApproval("run-approval", "approved"); err != nil {
		t.Fatalf("ResolveApproval() error = %v", err)
	}
	run, err = store.LoadRun(ctx, "run-approval")
	if err != nil {
		t.Fatalf("LoadRun() after ResolveApproval error = %v", err)
	}
	if run.State != string(RunRunning) {
		t.Fatalf("run state after ResolveApproval = %q, want %q", run.State, RunRunning)
	}

	got := eventTypes(store.Events(), "run-approval")
	want := []string{"run_started", "approval_required", "approval_resolved"}
	assertContainsInOrder(t, got, want)
}

func TestSubscribeFanOutAndSessionFilter(t *testing.T) {
	e, _ := newTestEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subA := e.Subscribe(ctx, EventFilter{SessionID: "session-a"})
	subB := e.Subscribe(ctx, EventFilter{SessionID: "session-a"})
	subOther := e.Subscribe(ctx, EventFilter{SessionID: "session-b"})

	if _, err := e.Submit(ctx, Task{ID: "task-a", SessionID: "session-a", Origin: "user", Goal: "hello"}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	for name, ch := range map[string]<-chan Event{"subA": subA, "subB": subB} {
		select {
		case ev := <-ch:
			if ev.Type != "task_created" || ev.TaskID != "task-a" {
				t.Fatalf("%s got %#v, want task_created/task-a", name, ev)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s timed out waiting for fan-out event", name)
		}
	}
	select {
	case ev := <-subOther:
		t.Fatalf("session-b subscriber received session-a event: %#v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSubscribeReplaysFromSeq(t *testing.T) {
	e, _ := newTestEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e.Publish(ctx, Event{Type: "model_token", TaskID: "task-replay", Data: map[string]any{"session_id": "session-replay", "text": "one"}})
	e.Publish(ctx, Event{Type: "model_token", TaskID: "task-replay", Data: map[string]any{"session_id": "session-replay", "text": "two"}})

	ch := e.Subscribe(ctx, EventFilter{SessionID: "session-replay", FromSeq: 1})
	select {
	case ev := <-ch:
		if ev.Seq != 2 || ev.Data["text"] != "two" {
			t.Fatalf("replayed event = %#v, want seq=2 text=two", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replayed event")
	}

	e.Publish(ctx, Event{Type: "model_token", TaskID: "task-replay", Data: map[string]any{"session_id": "session-replay", "text": "three"}})
	select {
	case ev := <-ch:
		if ev.Seq != 3 || ev.Data["text"] != "three" {
			t.Fatalf("live event = %#v, want seq=3 text=three", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event after replay")
	}
}

func TestSubscribeReplaysRecentHistoryFromZeroThenLive(t *testing.T) {
	e, _ := newTestEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < eventReplayLimit+2; i++ {
		e.Publish(ctx, Event{Type: "model_token", SessionID: "session-recent", Data: map[string]any{"i": i + 1}})
	}

	ch := e.Subscribe(ctx, EventFilter{SessionID: "session-recent"})
	for i := 0; i < eventReplayLimit; i++ {
		select {
		case ev := <-ch:
			if i == 0 && ev.Seq != 3 {
				t.Fatalf("first replay seq = %d, want 3", ev.Seq)
			}
			if i == eventReplayLimit-1 && ev.Seq != int64(eventReplayLimit+2) {
				t.Fatalf("last replay seq = %d, want %d", ev.Seq, eventReplayLimit+2)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for replay event %d/%d", i+1, eventReplayLimit)
		}
	}

	e.Publish(ctx, Event{Type: "model_token", SessionID: "session-recent", Data: map[string]any{"i": "live"}})
	select {
	case ev := <-ch:
		if ev.Seq != int64(eventReplayLimit+3) || ev.Data["i"] != "live" {
			t.Fatalf("live event = %#v, want seq=%d i=live", ev, eventReplayLimit+3)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event after recent replay")
	}
}

func TestMemStoreListEventsRecentWindowAndFromSeq(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()
	for i := 1; i <= 4; i++ {
		e.Publish(ctx, Event{Type: "model_token", SessionID: "session-list", Data: map[string]any{"i": i}})
	}

	recent, err := store.ListEvents(ctx, EventFilter{SessionID: "session-list"}, 2)
	if err != nil {
		t.Fatalf("ListEvents(recent) error = %v", err)
	}
	if len(recent) != 2 || recent[0].Seq != 3 || recent[1].Seq != 4 {
		t.Fatalf("recent events = %#v, want seq 3,4", recent)
	}

	afterSeq, err := store.ListEvents(ctx, EventFilter{SessionID: "session-list", FromSeq: 2}, 2)
	if err != nil {
		t.Fatalf("ListEvents(fromSeq) error = %v", err)
	}
	if len(afterSeq) != 2 || afterSeq[0].Seq != 3 || afterSeq[1].Seq != 4 {
		t.Fatalf("fromSeq events = %#v, want seq 3,4", afterSeq)
	}
}

func TestMemStoreLoadListAndStateValidation(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()
	task := Task{ID: "task-load", SessionID: "session-load", Origin: "user", Goal: "load"}
	if err := store.SaveTask(ctx, task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}
	if err := store.SaveTurn(ctx, Turn{ID: "turn-load", RunID: "run-load", Index: 1, State: string(TurnFinished)}); err != nil {
		t.Fatalf("SaveTurn() error = %v", err)
	}
	run1 := Run{ID: "run-new", TaskID: task.ID, State: string(RunRunning), CreatedAt: time.Now().UTC().Add(time.Second)}
	run0 := Run{ID: "run-old", TaskID: task.ID, State: string(RunPending), CreatedAt: time.Now().UTC()}
	if err := store.SaveRun(ctx, run1); err != nil {
		t.Fatalf("SaveRun(run1) error = %v", err)
	}
	if err := store.SaveRun(ctx, run0); err != nil {
		t.Fatalf("SaveRun(run0) error = %v", err)
	}

	loaded, err := store.LoadTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("LoadTask() error = %v", err)
	}
	if loaded.SessionID != task.SessionID {
		t.Fatalf("loaded SessionID = %q, want %q", loaded.SessionID, task.SessionID)
	}
	if _, err := store.LoadTask(ctx, "missing"); err == nil {
		t.Fatal("LoadTask(missing) error = nil, want error")
	}
	runs, err := store.ListRuns(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 2 || runs[0].ID != "run-old" || runs[1].ID != "run-new" {
		t.Fatalf("runs = %#v, want old,new order", runs)
	}

	if !IsTaskTerminal(TaskDone) || IsTaskTerminal(TaskRunning) {
		t.Fatal("IsTaskTerminal returned unexpected values")
	}
	if err := ValidateTaskTransition(TaskQueued, TaskRunning); err != nil {
		t.Fatalf("ValidateTaskTransition(queued,running) error = %v", err)
	}
	if err := ValidateTaskTransition(TaskDone, TaskRunning); err == nil {
		t.Fatal("ValidateTaskTransition(done,running) error = nil, want error")
	}
	if err := ValidateRunTransition(RunRunning, RunDone); err != nil {
		t.Fatalf("ValidateRunTransition(running,done) error = %v", err)
	}
	if err := ValidateRunTransition(RunDone, RunRunning); err == nil {
		t.Fatal("ValidateRunTransition(done,running) error = nil, want error")
	}
}

func TestRecoverInterruptedRunsMarksPendingRunningFailedAndPreservesAwaitingApproval(t *testing.T) {
	e, store := newRecoverableTestEngine()
	ctx := context.Background()
	task := Task{ID: "task-recovery", SessionID: "session-recovery", Origin: "user", Goal: "resume after crash"}
	if _, err := e.Submit(ctx, task); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-pending-recovery", TaskID: task.ID, State: string(RunPending), CreatedAt: time.Now().UTC().Add(-2 * time.Minute)}); err != nil {
		t.Fatalf("SubmitRun(pending) error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-running-recovery", TaskID: task.ID, State: string(RunRunning), CreatedAt: time.Now().UTC().Add(-time.Minute)}); err != nil {
		t.Fatalf("SubmitRun(running) error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-awaiting-recovery", TaskID: task.ID, State: string(RunAwaitingApproval), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SubmitRun(awaiting) error = %v", err)
	}

	recovered, err := e.RecoverInterruptedRuns(ctx)
	if err != nil {
		t.Fatalf("RecoverInterruptedRuns() error = %v", err)
	}
	if recovered != 3 {
		t.Fatalf("RecoverInterruptedRuns() recovered = %d, want 3", recovered)
	}

	for _, runID := range []string{"run-pending-recovery", "run-running-recovery"} {
		run, err := store.LoadRun(ctx, runID)
		if err != nil {
			t.Fatalf("LoadRun(%s) error = %v", runID, err)
		}
		if run.State != string(RunFailed) {
			t.Fatalf("%s state = %q, want %q", runID, run.State, RunFailed)
		}
	}

	awaiting, err := store.LoadRun(ctx, "run-awaiting-recovery")
	if err != nil {
		t.Fatalf("LoadRun(run-awaiting-recovery) error = %v", err)
	}
	if awaiting.State != string(RunAwaitingApproval) {
		t.Fatalf("run-awaiting-recovery state = %q, want %q", awaiting.State, RunAwaitingApproval)
	}

	events, err := store.ListEvents(ctx, EventFilter{FromSeq: 0, SessionID: "session-recovery"}, 50)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	taskFailedByRun := map[string]Event{}
	var awaitingMarker *Event
	for _, ev := range events {
		switch ev.Type {
		case EventTaskFailed:
			if boolFromData(ev.Data, "recovery") {
				taskFailedByRun[ev.RunID] = ev
			}
		case EventRunRecovered:
			if ev.RunID == "run-awaiting-recovery" {
				copy := ev
				awaitingMarker = &copy
			}
		}
	}
	if len(taskFailedByRun) != 2 {
		t.Fatalf("recovery task_failed by run = %#v, want pending+running only; events=%#v", taskFailedByRun, events)
	}
	for runID, previousState := range map[string]RunState{
		"run-pending-recovery": RunPending,
		"run-running-recovery": RunRunning,
	} {
		ev, ok := taskFailedByRun[runID]
		if !ok {
			t.Fatalf("missing recovery task_failed for %s in %#v", runID, events)
		}
		if ev.SessionID != task.SessionID || ev.TaskID != task.ID || ev.RunID != runID {
			t.Fatalf("recovery event envelope = %#v, want task/run/session linked", ev)
		}
		if ev.Data["recovery"] != true || ev.Data["recoverable"] != true || ev.Data["reason"] != "daemon_restarted" {
			t.Fatalf("recovery event data = %#v, want recovery metadata", ev.Data)
		}
		if ev.Data["previous_state"] != string(previousState) {
			t.Fatalf("recovery previous_state for %s = %#v, want %q", runID, ev.Data["previous_state"], previousState)
		}
		if ev.Data["run_id"] != runID || ev.Data["task_id"] != task.ID {
			t.Fatalf("recovery ids = %#v, want run/task ids", ev.Data)
		}
	}
	if _, ok := taskFailedByRun["run-awaiting-recovery"]; ok {
		t.Fatalf("awaiting approval recovery should not emit terminal task_failed: %#v", taskFailedByRun["run-awaiting-recovery"])
	}
	if awaitingMarker == nil {
		t.Fatalf("missing non-terminal run_recovered marker for awaiting approval: events=%#v", events)
	}
	if awaitingMarker.SessionID != task.SessionID || awaitingMarker.TaskID != task.ID {
		t.Fatalf("awaiting marker envelope = %#v, want task/session linked", *awaitingMarker)
	}
	if awaitingMarker.Data["recovery"] != true ||
		awaitingMarker.Data["recoverable"] != true ||
		awaitingMarker.Data["previous_state"] != string(RunAwaitingApproval) ||
		awaitingMarker.Data["reason"] != "daemon_restarted" ||
		awaitingMarker.Data["approval_kind"] != "run_hitl" ||
		awaitingMarker.Data["approval_pending"] != true ||
		awaitingMarker.Data["recovery_action"] != "approval_preserved" ||
		awaitingMarker.Data["terminal_emitted"] != false {
		t.Fatalf("unexpected awaiting marker data: %#v", awaitingMarker.Data)
	}

	replayed, err := store.ListEvents(ctx, EventFilter{FromSeq: awaitingMarker.Seq - 1, RunID: awaitingMarker.RunID}, 10)
	if err != nil {
		t.Fatalf("ListEvents(replay) error = %v", err)
	}
	if len(replayed) == 0 || replayed[len(replayed)-1].Seq != awaitingMarker.Seq {
		t.Fatalf("replayed events = %#v, want awaiting recovery marker replayed", replayed)
	}
}

func TestRecoverInterruptedRunsIsIdempotentForAwaitingApproval(t *testing.T) {
	e, store := newRecoverableTestEngine()
	ctx := context.Background()
	task := Task{ID: "task-recovery-idempotent", SessionID: "session-recovery-idempotent", Origin: "user", Goal: "resume after crash"}
	if _, err := e.Submit(ctx, task); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-awaiting-idempotent", TaskID: task.ID, State: string(RunAwaitingApproval), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SubmitRun(awaiting) error = %v", err)
	}

	recovered, err := e.RecoverInterruptedRuns(ctx)
	if err != nil {
		t.Fatalf("RecoverInterruptedRuns(first) error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverInterruptedRuns(first) recovered = %d, want 1", recovered)
	}
	recovered, err = e.RecoverInterruptedRuns(ctx)
	if err != nil {
		t.Fatalf("RecoverInterruptedRuns(second) error = %v", err)
	}
	if recovered != 0 {
		t.Fatalf("RecoverInterruptedRuns(second) recovered = %d, want 0", recovered)
	}
	run, err := store.LoadRun(ctx, "run-awaiting-idempotent")
	if err != nil {
		t.Fatalf("LoadRun(awaiting) error = %v", err)
	}
	if run.State != string(RunAwaitingApproval) {
		t.Fatalf("awaiting state after repeated recovery = %q, want %q", run.State, RunAwaitingApproval)
	}

	events, err := store.ListEvents(ctx, EventFilter{RunID: run.ID}, 20)
	if err != nil {
		t.Fatalf("ListEvents(awaiting) error = %v", err)
	}
	recoveryMarkers := 0
	terminalFailures := 0
	for _, ev := range events {
		switch ev.Type {
		case EventRunRecovered:
			if boolFromData(ev.Data, "recovery") {
				recoveryMarkers++
			}
		case EventTaskFailed:
			if boolFromData(ev.Data, "recovery") {
				terminalFailures++
			}
		}
	}
	if recoveryMarkers != 1 || terminalFailures != 0 {
		t.Fatalf("awaiting recovery markers=%d terminalFailures=%d events=%#v, want 1/0", recoveryMarkers, terminalFailures, events)
	}
}

func TestFinishRunPublishesTerminalEvents(t *testing.T) {
	e, store := newTestEngine()
	ctx := context.Background()
	if _, err := e.Submit(ctx, Task{ID: "task-finish", SessionID: "session-finish", Origin: "user", Goal: "finish"}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-finish", TaskID: "task-finish", State: string(RunRunning)}); err != nil {
		t.Fatalf("SubmitRun() error = %v", err)
	}
	if err := e.FinishRun(ctx, "run-finish", true); err != nil {
		t.Fatalf("FinishRun(success) error = %v", err)
	}
	run, err := store.LoadRun(ctx, "run-finish")
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if run.State != string(RunDone) {
		t.Fatalf("run state = %q, want %q", run.State, RunDone)
	}
	events := store.Events()
	if !hasEvent(events, "task_finished", "task-finish") {
		t.Fatalf("task_finished missing: %#v", events)
	}
	if events[len(events)-1].SessionID != "session-finish" {
		t.Fatalf("terminal SessionID = %q, want session-finish", events[len(events)-1].SessionID)
	}
	if err := e.FinishRun(ctx, "missing", false); err == nil {
		t.Fatal("FinishRun(missing) error = nil, want error")
	}
}
