// Package runtime is the Phase 2 skeleton for the unified Task/Run/Turn
// model. The current cut intentionally exposes empty stubs: the goal is to
// land the package boundary in git so subsequent commits can incrementally
// fill the engine without churning the daemon's existing flows.
//
// Lifecycle (target state, not yet enforced):
//
//	Task        any unit of agent work (user prompt, schedule fire, replan,
//	            subagent invocation). One Task can have many Runs.
//	Run         one execution attempt of a Task. State machine drives
//	            queue → run → (await approval | succeed | fail | cancel).
//	Turn        one LLM-and-tool exchange inside a Run. Multiple Turns may
//	            occur per Run when tools/replanning happen.
//	Event       append-only audit + UI feed (task_created, turn_started,
//	            model_token, tool_started/finished, approval_required, ...).
//
// Engine wraps these via Submit/Stream and persists through Store.
package runtime

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Task is one unit of agent work.
type Task struct {
	ID      string
	Origin  string // "user" | "schedule" | "approval" | "replan" | "subagent"
	Goal    string
	Created time.Time
}

// Run is one execution attempt of a Task.
type Run struct {
	ID        string
	TaskID    string
	State     string // "queued" | "running" | "awaiting_approval" | "succeeded" | "failed" | "cancelled"
	Turn      int
	CreatedAt time.Time
}

// Turn is one LLM-and-tool exchange inside a Run.
type Turn struct {
	ID    string
	RunID string
	Index int
	State string
}

// Event is the append-only timeline entry feeding both audit log and
// streaming UI.
type Event struct {
	Type      string // task_created|turn_started|model_token|tool_started|tool_finished|approval_required|approval_resolved|task_finished|task_failed|...
	TurnID    string
	RunID     string
	TaskID    string
	Data      map[string]any
	Timestamp time.Time
}

// Store is the persistence boundary. The Phase 2 in-memory implementation
// keeps things in maps; Phase 6 swaps in SQLite without changing this
// signature.
type Store interface {
	AppendEvent(ctx context.Context, e Event) error
	SaveTask(ctx context.Context, t Task) error
	SaveRun(ctx context.Context, r Run) error
	SaveTurn(ctx context.Context, tu Turn) error
	LoadTask(ctx context.Context, id string) (Task, error)
	LoadRun(ctx context.Context, id string) (Run, error)
	ListRuns(ctx context.Context, taskID string) ([]Run, error)
}

// Engine is the central coordinator. The Phase 2 cut wires Submit/Stream
// only; runTurn/approval routing land in subsequent commits.
type Engine struct {
	store  Store
	mu     sync.Mutex
	tasks  map[string]*Task
	runs   map[string]*Run
	turns  map[string]*Turn
	events chan Event
	subs   []chan Event
}

// NewEngine returns a fresh engine bound to store. Subscribers are added
// later via Stream().
func NewEngine(s Store) *Engine {
	return &Engine{
		store:  s,
		tasks:  map[string]*Task{},
		runs:   map[string]*Run{},
		turns:  map[string]*Turn{},
		events: make(chan Event, 256),
	}
}

// Submit registers a new Task and emits a task_created event. The current
// cut does not yet schedule a Run; that happens once the engine wires into
// the daemon's planner loop.
func (e *Engine) Submit(ctx context.Context, t Task) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t.Created.IsZero() {
		t.Created = time.Now()
	}
	e.tasks[t.ID] = &t
	if e.store != nil {
		if err := e.store.SaveTask(ctx, t); err != nil {
			return "", err
		}
	}
	e.publish(ctx, Event{
		Type:      "task_created",
		TaskID:    t.ID,
		Data:      map[string]any{"origin": t.Origin, "goal": t.Goal},
		Timestamp: time.Now(),
	})
	return t.ID, nil
}

// Stream returns the global event channel. Subscribers fan out via subs in
// later commits; the Phase 2 skeleton exposes the raw channel so callers
// can wire telemetry tests.
func (e *Engine) Stream() <-chan Event {
	return e.events
}

// publish writes to the audit store and best-effort fans the event into
// the broadcast channel. Slow consumers drop events rather than block the
// engine — the durable record lives in Store.
func (e *Engine) publish(ctx context.Context, ev Event) {
	if e.store != nil {
		_ = e.store.AppendEvent(ctx, ev)
	}
	select {
	case e.events <- ev:
	default:
	}
}

// SubmitRun registers a Run for an existing Task and emits run_started.
func (e *Engine) SubmitRun(ctx context.Context, r Run) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	e.runs[r.ID] = &r
	if e.store != nil {
		if err := e.store.SaveRun(ctx, r); err != nil {
			return "", err
		}
	}
	e.publish(ctx, Event{
		Type:      "run_started",
		RunID:     r.ID,
		TaskID:    r.TaskID,
		Data:      map[string]any{"state": r.State, "turn": r.Turn},
		Timestamp: time.Now(),
	})
	return r.ID, nil
}

// runTurnLLMToolLoop emits a deterministic token/tool sequence for the
// engine_test.go main-link tests. Real LLM integration lands in Phase 3.
func (e *Engine) runTurnLLMToolLoop(ctx context.Context, runID string) {
	turnID := runID + "-turn-1"
	e.publish(ctx, Event{Type: "turn_started", RunID: runID, TurnID: turnID, Timestamp: time.Now()})
	for i := 0; i < 3; i++ {
		e.publish(ctx, Event{Type: "model_token", RunID: runID, TurnID: turnID, Data: map[string]any{"i": i}, Timestamp: time.Now()})
	}
	e.publish(ctx, Event{Type: "tool_started", RunID: runID, TurnID: turnID, Data: map[string]any{"tool": "scan.web.safe"}, Timestamp: time.Now()})
	e.publish(ctx, Event{Type: "tool_finished", RunID: runID, TurnID: turnID, Data: map[string]any{"tool": "scan.web.safe", "ok": true}, Timestamp: time.Now()})
	e.publish(ctx, Event{Type: "turn_finished", RunID: runID, TurnID: turnID, Timestamp: time.Now()})
}

// RequestApproval pauses a running run and emits approval_required.
func (e *Engine) RequestApproval(runID, reason string) error {
	ctx := context.Background()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.runs[runID]
	if !ok {
		return errors.New("run not found: " + runID)
	}
	if err := ValidateRunTransition(RunState(r.State), RunAwaitingApproval); err != nil {
		return err
	}
	r.State = string(RunAwaitingApproval)
	if e.store != nil {
		_ = e.store.SaveRun(ctx, *r)
	}
	e.publish(ctx, Event{
		Type:      "approval_required",
		RunID:     runID,
		TaskID:    r.TaskID,
		Data:      map[string]any{"reason": reason},
		Timestamp: time.Now(),
	})
	return nil
}

// ResolveApproval resumes a run that was paused for approval.
func (e *Engine) ResolveApproval(runID, decision string) error {
	ctx := context.Background()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.runs[runID]
	if !ok {
		return errors.New("run not found: " + runID)
	}
	if err := ValidateRunTransition(RunState(r.State), RunRunning); err != nil {
		return err
	}
	r.State = string(RunRunning)
	if e.store != nil {
		_ = e.store.SaveRun(ctx, *r)
	}
	e.publish(ctx, Event{
		Type:      "approval_resolved",
		RunID:     runID,
		TaskID:    r.TaskID,
		Data:      map[string]any{"decision": decision},
		Timestamp: time.Now(),
	})
	return nil
}

// FinishRun closes a run with a terminal state and emits task_finished or
// task_failed.
func (e *Engine) FinishRun(ctx context.Context, runID string, ok bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok2 := e.runs[runID]
	if !ok2 {
		return errors.New("run not found: " + runID)
	}
	target := RunDone
	evType := "task_finished"
	if !ok {
		target = RunFailed
		evType = "task_failed"
	}
	if err := ValidateRunTransition(RunState(r.State), target); err != nil {
		return err
	}
	r.State = string(target)
	if e.store != nil {
		_ = e.store.SaveRun(ctx, *r)
	}
	e.publish(ctx, Event{Type: evType, RunID: runID, TaskID: r.TaskID, Timestamp: time.Now()})
	return nil
}
