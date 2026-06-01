package runtime

import (
	"context"
	"encoding/json"
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

func TestRecoverInterruptedRunsMarksFailedAndPublishesReplayableEvent(t *testing.T) {
	e, store := newRecoverableTestEngine()
	ctx := context.Background()
	task := Task{ID: "task-recovery", SessionID: "session-recovery", Origin: "user", Goal: "resume after crash"}
	if _, err := e.Submit(ctx, task); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-recovery", TaskID: task.ID, State: string(RunRunning), CreatedAt: time.Now().UTC().Add(-time.Minute)}); err != nil {
		t.Fatalf("SubmitRun() error = %v", err)
	}
	if _, err := e.SubmitRun(ctx, Run{ID: "run-recovery-nolink", State: string(RunAwaitingApproval), CreatedAt: time.Now().UTC().Add(-2 * time.Minute)}); err != nil {
		t.Fatalf("SubmitRun(no-link) error = %v", err)
	}

	recovered, err := e.RecoverInterruptedRuns(ctx)
	if err != nil {
		t.Fatalf("RecoverInterruptedRuns() error = %v", err)
	}
	if recovered != 2 {
		t.Fatalf("RecoverInterruptedRuns() recovered = %d, want 2", recovered)
	}

	run, err := store.LoadRun(ctx, "run-recovery")
	if err != nil {
		t.Fatalf("LoadRun(run-recovery) error = %v", err)
	}
	if run.State != string(RunFailed) {
		t.Fatalf("run-recovery state = %q, want %q", run.State, RunFailed)
	}

	runNoLink, err := store.LoadRun(ctx, "run-recovery-nolink")
	if err != nil {
		t.Fatalf("LoadRun(run-recovery-nolink) error = %v", err)
	}
	if runNoLink.State != string(RunFailed) {
		t.Fatalf("run-recovery-nolink state = %q, want %q", runNoLink.State, RunFailed)
	}

	events, err := store.ListEvents(ctx, EventFilter{FromSeq: 0, SessionID: "session-recovery"}, 20)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	var recoveryEvents []Event
	for _, ev := range events {
		if ev.Type == "task_failed" {
			recoveryEvents = append(recoveryEvents, ev)
		}
	}
	if len(recoveryEvents) != 1 {
		t.Fatalf("recovery task_failed events = %#v, want 1", recoveryEvents)
	}
	ev := recoveryEvents[0]
	if ev.SessionID != task.SessionID || ev.TaskID != task.ID || ev.RunID != "run-recovery" {
		t.Fatalf("recovery event envelope = %#v, want task/run/session linked", ev)
	}
	if ev.Data["recovery"] != true || ev.Data["recoverable"] != true || ev.Data["reason"] != "daemon_restarted" {
		t.Fatalf("recovery event data = %#v, want recovery metadata", ev.Data)
	}
	if ev.Data["previous_state"] != string(RunRunning) {
		t.Fatalf("recovery previous_state = %#v, want %q", ev.Data["previous_state"], RunRunning)
	}
	if ev.Data["run_id"] != "run-recovery" || ev.Data["task_id"] != task.ID {
		t.Fatalf("recovery ids = %#v, want run/task ids", ev.Data)
	}

	replayed, err := store.ListEvents(ctx, EventFilter{FromSeq: ev.Seq - 1, SessionID: task.SessionID}, 10)
	if err != nil {
		t.Fatalf("ListEvents(replay) error = %v", err)
	}
	if len(replayed) == 0 || replayed[len(replayed)-1].Seq != ev.Seq {
		t.Fatalf("replayed events = %#v, want recovery event replayed", replayed)
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
