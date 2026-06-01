package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

func TestStoreSaveLoadAndFilterEvents(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "runtime", "runtime.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	created := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	task := agentruntime.Task{
		ID:             "task-1",
		SessionID:      "session-a",
		Origin:         "user",
		Goal:           "scan target",
		Prompt:         "scan https://example.com",
		OperationLevel: "safe",
		ScheduleID:     "schedule-1",
		Created:        created,
	}
	if err := store.SaveTask(ctx, task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}
	run1 := agentruntime.Run{ID: "run-1", TaskID: task.ID, State: string(agentruntime.RunRunning), Turn: 1, CreatedAt: created.Add(2 * time.Second)}
	run0 := agentruntime.Run{ID: "run-0", TaskID: task.ID, State: string(agentruntime.RunPending), Turn: 0, CreatedAt: created.Add(time.Second)}
	if err := store.SaveRun(ctx, run1); err != nil {
		t.Fatalf("SaveRun(run1) error = %v", err)
	}
	if err := store.SaveRun(ctx, run0); err != nil {
		t.Fatalf("SaveRun(run0) error = %v", err)
	}
	turn := agentruntime.Turn{ID: "turn-1", RunID: run1.ID, Index: 1, State: string(agentruntime.TurnRunning)}
	if err := store.SaveTurn(ctx, turn); err != nil {
		t.Fatalf("SaveTurn() error = %v", err)
	}

	if err := store.AppendEvent(ctx, agentruntime.Event{Seq: 1, Type: "model_token", TaskID: task.ID, RunID: run1.ID, TurnID: turn.ID, Data: map[string]any{"session_id": "session-a", "text": "one"}, Timestamp: created}); err != nil {
		t.Fatalf("AppendEvent(1) error = %v", err)
	}
	if err := store.AppendEvent(ctx, agentruntime.Event{Seq: 2, Type: "model_token", TaskID: task.ID, RunID: run1.ID, TurnID: turn.ID, Data: map[string]any{"session_id": "session-b", "text": "two"}, Timestamp: created.Add(time.Second)}); err != nil {
		t.Fatalf("AppendEvent(2) error = %v", err)
	}
	if err := store.AppendEvent(ctx, agentruntime.Event{Seq: 3, Type: "tool_finished", TaskID: task.ID, RunID: run1.ID, TurnID: turn.ID, Data: map[string]any{"session_id": "session-a", "ok": true}, Timestamp: created.Add(2 * time.Second)}); err != nil {
		t.Fatalf("AppendEvent(3) error = %v", err)
	}

	loadedTask, err := store.LoadTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("LoadTask() error = %v", err)
	}
	if loadedTask.SessionID != task.SessionID || loadedTask.Prompt != task.Prompt || loadedTask.ScheduleID != task.ScheduleID {
		t.Fatalf("unexpected loaded task: %#v", loadedTask)
	}
	loadedRun, err := store.LoadRun(ctx, run1.ID)
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if loadedRun.State != string(agentruntime.RunRunning) || loadedRun.Turn != 1 {
		t.Fatalf("unexpected loaded run: %#v", loadedRun)
	}
	runs, err := store.ListRuns(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 2 || runs[0].ID != "run-0" || runs[1].ID != "run-1" {
		t.Fatalf("unexpected run order: %#v", runs)
	}

	events, err := store.ListEvents(ctx, agentruntime.EventFilter{SessionID: "session-a", FromSeq: 1}, 10)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].Seq != 3 || events[0].Type != "tool_finished" || events[0].Data["ok"] != true {
		t.Fatalf("unexpected filtered events: %#v", events)
	}
	limited, err := store.ListEvents(ctx, agentruntime.EventFilter{TaskID: task.ID}, 2)
	if err != nil {
		t.Fatalf("ListEvents(limit) error = %v", err)
	}
	if len(limited) != 2 || limited[0].Seq != 2 || limited[1].Seq != 3 {
		t.Fatalf("unexpected limited events: %#v", limited)
	}
	maxSeq, err := store.MaxEventSeq(ctx)
	if err != nil {
		t.Fatalf("MaxEventSeq() error = %v", err)
	}
	if maxSeq != 3 {
		t.Fatalf("MaxEventSeq() = %d, want 3", maxSeq)
	}
}

func TestStoreEventsUseTopLevelSessionIDAndRecentWindow(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for i := 1; i <= 4; i++ {
		if err := store.AppendEvent(ctx, agentruntime.Event{
			Type:      "model_token",
			SessionID: "session-top",
			Data:      map[string]any{"i": i},
			Timestamp: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("AppendEvent(%d) error = %v", i, err)
		}
	}

	recent, err := store.ListEvents(ctx, agentruntime.EventFilter{SessionID: "session-top"}, 2)
	if err != nil {
		t.Fatalf("ListEvents(recent) error = %v", err)
	}
	if len(recent) != 2 || recent[0].Seq != 3 || recent[1].Seq != 4 {
		t.Fatalf("recent = %#v, want seq 3,4", recent)
	}
	for _, ev := range recent {
		if ev.SessionID != "session-top" {
			t.Fatalf("event SessionID = %q, want session-top: %#v", ev.SessionID, ev)
		}
	}

	afterSeq, err := store.ListEvents(ctx, agentruntime.EventFilter{SessionID: "session-top", FromSeq: 2}, 10)
	if err != nil {
		t.Fatalf("ListEvents(fromSeq) error = %v", err)
	}
	if len(afterSeq) != 2 || afterSeq[0].Seq != 3 || afterSeq[1].Seq != 4 {
		t.Fatalf("afterSeq = %#v, want seq 3,4", afterSeq)
	}
}

func TestStoreRejectsEmptyIDsAndMissingRows(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveTask(ctx, agentruntime.Task{}); err == nil {
		t.Fatal("SaveTask(empty) error = nil, want error")
	}
	if err := store.SaveRun(ctx, agentruntime.Run{}); err == nil {
		t.Fatal("SaveRun(empty) error = nil, want error")
	}
	if err := store.SaveTurn(ctx, agentruntime.Turn{}); err == nil {
		t.Fatal("SaveTurn(empty) error = nil, want error")
	}
	if err := store.AppendEvent(ctx, agentruntime.Event{}); err == nil {
		t.Fatal("AppendEvent(empty type) error = nil, want error")
	}
	if _, err := store.LoadTask(ctx, "missing"); err == nil {
		t.Fatal("LoadTask(missing) error = nil, want error")
	}
	if _, err := store.LoadRun(ctx, "missing"); err == nil {
		t.Fatal("LoadRun(missing) error = nil, want error")
	}
}

func TestStorePersistsAcrossOpenAndCreatesForwardTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	ctx := context.Background()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.SaveTask(ctx, agentruntime.Task{ID: "task-persist", SessionID: "session-persist", Origin: "schedule", Goal: "nightly", Created: time.Now().UTC()}); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}
	if err := store.AppendEvent(ctx, agentruntime.Event{Seq: 7, Type: "task_created", TaskID: "task-persist", Data: map[string]any{"session_id": "session-persist"}, Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer reopened.Close()
	loadedTask, err := reopened.LoadTask(ctx, "task-persist")
	if err != nil {
		t.Fatalf("LoadTask(reopen) error = %v", err)
	}
	if loadedTask.SessionID != "session-persist" || loadedTask.Origin != "schedule" {
		t.Fatalf("unexpected reopened task: %#v", loadedTask)
	}
	maxSeq, err := reopened.MaxEventSeq(ctx)
	if err != nil {
		t.Fatalf("MaxEventSeq(reopen) error = %v", err)
	}
	if maxSeq != 7 {
		t.Fatalf("MaxEventSeq(reopen) = %d, want 7", maxSeq)
	}
	for _, table := range []string{"sessions", "tasks", "runs", "turns", "events", "approvals", "schedules", "memories"} {
		if !sqliteTableExists(t, reopened.db, table) {
			t.Fatalf("schema table %q missing", table)
		}
	}
}

func TestEngineSeedsEventSeqFromSQLiteStore(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.AppendEvent(ctx, agentruntime.Event{Seq: 41, Type: "model_token", Data: map[string]any{"session_id": "session-seq"}, Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	engine := agentruntime.NewEngine(store)
	engine.Publish(ctx, agentruntime.Event{Type: "model_token", Data: map[string]any{"session_id": "session-seq", "text": "next"}})
	events, err := store.ListEvents(ctx, agentruntime.EventFilter{SessionID: "session-seq", FromSeq: 41}, 10)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].Seq != 42 || events[0].Data["text"] != "next" {
		t.Fatalf("unexpected next event after seeded engine: %#v", events)
	}
}


func TestListRunsByState(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	created := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	task := agentruntime.Task{ID: "task-list-runs", SessionID: "session-list-runs", Origin: "user", Goal: "recover"}
	if err := store.SaveTask(ctx, task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}
	runs := []agentruntime.Run{
		{ID: "run-pending-b", TaskID: task.ID, State: string(agentruntime.RunPending), CreatedAt: created.Add(2 * time.Second)},
		{ID: "run-running", TaskID: task.ID, State: string(agentruntime.RunRunning), CreatedAt: created.Add(3 * time.Second)},
		{ID: "run-awaiting", TaskID: task.ID, State: string(agentruntime.RunAwaitingApproval), CreatedAt: created.Add(4 * time.Second)},
		{ID: "run-pending-a", TaskID: task.ID, State: string(agentruntime.RunPending), CreatedAt: created.Add(time.Second)},
		{ID: "run-done", TaskID: task.ID, State: string(agentruntime.RunDone), CreatedAt: created.Add(5 * time.Second)},
		{ID: "run-failed", TaskID: task.ID, State: string(agentruntime.RunFailed), CreatedAt: created.Add(6 * time.Second)},
	}
	for _, run := range runs {
		if err := store.SaveRun(ctx, run); err != nil {
			t.Fatalf("SaveRun(%s) error = %v", run.ID, err)
		}
	}

	got, err := store.ListRunsByState(ctx, agentruntime.RunPending, agentruntime.RunRunning, agentruntime.RunAwaitingApproval)
	if err != nil {
		t.Fatalf("ListRunsByState() error = %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("ListRunsByState() len = %d, want 4: %#v", len(got), got)
	}
	wantIDs := []string{"run-pending-a", "run-pending-b", "run-running", "run-awaiting"}
	for i, run := range got {
		if run.ID != wantIDs[i] {
			t.Fatalf("ListRunsByState()[%d].ID = %q, want %q; runs=%#v", i, run.ID, wantIDs[i], got)
		}
	}

	empty, err := store.ListRunsByState(ctx)
	if err != nil {
		t.Fatalf("ListRunsByState(empty) error = %v", err)
	}
	if empty != nil {
		t.Fatalf("ListRunsByState(empty) = %#v, want nil", empty)
	}

	filtered, err := store.ListRunsByState(ctx, agentruntime.RunState(""), agentruntime.RunState("   "), agentruntime.RunDone)
	if err != nil {
		t.Fatalf("ListRunsByState(filtered) error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "run-done" {
		t.Fatalf("ListRunsByState(filtered) = %#v, want only run-done", filtered)
	}

	missing, err := store.ListRunsByState(ctx, agentruntime.RunState("missing"))
	if err != nil {
		t.Fatalf("ListRunsByState(missing) error = %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("ListRunsByState(missing) = %#v, want empty", missing)
	}
}

func TestListRunsByStateReturnsErrorAfterClose(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, err = store.ListRunsByState(context.Background(), agentruntime.RunPending)
	if err == nil {
		t.Fatal("ListRunsByState(closed) error = nil, want error")
	}
}

func sqliteTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
	if err == nil {
		return name == table
	}
	if err == sql.ErrNoRows {
		return false
	}
	t.Fatalf("query sqlite_master for %s: %v", table, err)
	return false
}
