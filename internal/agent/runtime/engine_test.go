package runtime

import (
	"context"
	"testing"
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
