package runtime

import (
	"context"
	"testing"
	"time"
)

func newTestEngine() (*Engine, *MemStore) {
	store := NewMemStore()
	return NewEngine(store), store
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
