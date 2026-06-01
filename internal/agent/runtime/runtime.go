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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const eventReplayLimit = 512

// Task is one unit of agent work.
type Task struct {
	ID             string
	SessionID      string
	Origin         string // "user" | "schedule" | "approval" | "replan" | "subagent"
	Goal           string
	Prompt         string
	OperationLevel string
	ScheduleID     string
	Created        time.Time
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
	Seq       int64          `json:"seq,omitempty"`
	Type      string         `json:"type"` // task_created|turn_started|model_token|tool_started|tool_finished|approval_required|approval_resolved|task_finished|task_failed|...
	SessionID string         `json:"session_id,omitempty"`
	TurnID    string         `json:"turn_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// EventFilter scopes a streaming subscription. Empty fields match all
// events. SessionID matches the V1 envelope's top-level Event.SessionID.
type EventFilter struct {
	TaskID    string
	RunID     string
	SessionID string
	FromSeq   int64
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
	ListEvents(ctx context.Context, filter EventFilter, limit int) ([]Event, error)
}

// Engine is the central coordinator. The Phase 2 cut wires Submit/Stream
// only; runTurn/approval routing land in subsequent commits.
type Engine struct {
	store Store
	mu    sync.Mutex
	tasks map[string]*Task
	runs  map[string]*Run
	turns map[string]*Turn

	events  chan Event // legacy single-consumer stream; prefer Subscribe.
	nextSeq atomic.Int64
	eventMu sync.Mutex // serializes append/replay/register so reconnects do not miss events.

	subMu     sync.Mutex
	nextSubID int
	subs      map[int]eventSubscriber
}

type eventSubscriber struct {
	filter EventFilter
	ch     chan Event
}

type eventSeqStore interface {
	MaxEventSeq(ctx context.Context) (int64, error)
}

type recoverableRunStore interface {
	ListRunsByState(ctx context.Context, states ...RunState) ([]Run, error)
}

// NewEngine returns a fresh engine bound to store. Subscribers are added
// later via Stream(). Stores may expose MaxEventSeq so a restarted daemon
// continues event IDs above the durable high-water mark.
func NewEngine(s Store) *Engine {
	engine := &Engine{
		store:  s,
		tasks:  map[string]*Task{},
		runs:   map[string]*Run{},
		turns:  map[string]*Turn{},
		events: make(chan Event, 256),
		subs:   map[int]eventSubscriber{},
	}
	if seqStore, ok := s.(eventSeqStore); ok {
		if seq, err := seqStore.MaxEventSeq(context.Background()); err == nil && seq > 0 {
			engine.nextSeq.Store(seq)
		}
	}
	return engine
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
		Type:   "task_created",
		TaskID: t.ID,
		Data: map[string]any{
			"session_id":      t.SessionID,
			"origin":          t.Origin,
			"goal":            t.Goal,
			"prompt":          t.Prompt,
			"operation_level": t.OperationLevel,
			"schedule_id":     t.ScheduleID,
		},
		Timestamp: time.Now(),
	})
	return t.ID, nil
}

// Stream returns the legacy global event channel. It is intentionally kept
// for compatibility with the Phase 2 seam; new callers should use Subscribe
// so multiple consumers do not steal events from each other.
func (e *Engine) Stream() <-chan Event {
	return e.events
}

// Subscribe returns a fan-out stream of future events matching filter. The
// returned channel is closed when ctx is cancelled. Delivery is best-effort:
// slow subscribers drop events instead of blocking the engine; Store remains
// the durable source for replay/recovery.
func (e *Engine) Subscribe(ctx context.Context, filter EventFilter) <-chan Event {
	if ctx == nil {
		ctx = context.Background()
	}
	ch := make(chan Event, eventReplayLimit+128)
	var replay []Event
	e.eventMu.Lock()
	if e.store != nil {
		if events, err := e.store.ListEvents(ctx, filter, eventReplayLimit); err == nil {
			replay = events
		}
	}
	e.subMu.Lock()
	e.nextSubID++
	id := e.nextSubID
	e.subs[id] = eventSubscriber{filter: filter, ch: ch}
	e.subMu.Unlock()
	// Send replay before releasing eventMu so live publishes cannot overtake
	// the reconnect snapshot in the subscriber channel. Delivery remains
	// best-effort; dropped replay entries can still be requested again with a
	// lower FromSeq.
	for _, ev := range replay {
		select {
		case ch <- ev:
		default:
		}
	}
	e.eventMu.Unlock()

	go func() {
		<-ctx.Done()
		e.subMu.Lock()
		if sub, ok := e.subs[id]; ok {
			delete(e.subs, id)
			close(sub.ch)
		}
		e.subMu.Unlock()
	}()
	return ch
}

// Publish appends and broadcasts a pre-built runtime event. It is the bridge
// used by the legacy daemon while SendMessage/tool execution is still being
// migrated into Engine.runTurn.
func (e *Engine) Publish(ctx context.Context, ev Event) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	e.publish(ctx, ev)
}

// publish writes to the audit store and best-effort fans the event into
// the broadcast channel. Slow consumers drop events rather than block the
// engine — the durable record lives in Store.
func (e *Engine) publish(ctx context.Context, ev Event) {
	e.eventMu.Lock()
	defer e.eventMu.Unlock()
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	ev = normalizeEventEnvelope(ev)
	if ev.Seq == 0 {
		ev.Seq = e.nextSeq.Add(1)
	}
	if e.store != nil {
		_ = e.store.AppendEvent(ctx, ev)
	}
	select {
	case e.events <- ev:
	default:
	}
	e.subMu.Lock()
	for _, sub := range e.subs {
		if !eventMatchesFilter(ev, sub.filter) {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
		}
	}
	e.subMu.Unlock()
}

func eventMatchesFilter(ev Event, filter EventFilter) bool {
	if filter.TaskID != "" && ev.TaskID != filter.TaskID {
		return false
	}
	if filter.RunID != "" && ev.RunID != filter.RunID {
		return false
	}
	if filter.SessionID != "" {
		if eventSessionID(ev) != filter.SessionID {
			return false
		}
	}
	if filter.FromSeq > 0 && ev.Seq <= filter.FromSeq {
		return false
	}
	return true
}

// RecoverInterruptedRuns marks durable, non-terminal runs left behind by a daemon
// crash as failed and publishes task_failed recovery events for audit/UI replay.
// It is intentionally opt-in: stores that do not expose recoverableRunStore are
// left untouched, preserving legacy/in-memory behaviour.
func (e *Engine) RecoverInterruptedRuns(ctx context.Context) (int, error) {
	if e == nil || e.store == nil {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runStore, ok := e.store.(recoverableRunStore)
	if !ok {
		return 0, nil
	}
	runs, err := runStore.ListRunsByState(ctx, RunPending, RunRunning, RunAwaitingApproval)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, run := range runs {
		previousState := strings.TrimSpace(run.State)
		if previousState == "" || IsRunTerminal(RunState(previousState)) {
			continue
		}
		sessionID := ""
		if strings.TrimSpace(run.TaskID) != "" {
			if task, err := e.store.LoadTask(ctx, run.TaskID); err == nil {
				sessionID = strings.TrimSpace(task.SessionID)
				e.mu.Lock()
				e.tasks[task.ID] = &task
				e.mu.Unlock()
			}
		}
		run.State = string(RunFailed)
		if err := e.store.SaveRun(ctx, run); err != nil {
			return recovered, fmt.Errorf("mark interrupted run %s failed: %w", run.ID, err)
		}
		e.mu.Lock()
		runCopy := run
		e.runs[run.ID] = &runCopy
		e.mu.Unlock()
		e.publish(ctx, Event{
			Type:      "task_failed",
			SessionID: sessionID,
			RunID:     run.ID,
			TaskID:    run.TaskID,
			Data: map[string]any{
				"recovery":       true,
				"recoverable":    true,
				"previous_state": previousState,
				"reason":         "daemon_restarted",
				"run_id":         run.ID,
				"task_id":        run.TaskID,
			},
			Timestamp: time.Now().UTC(),
		})
		recovered++
	}
	return recovered, nil
}

// SubmitRun registers a Run for an existing Task and emits run_started.
func (e *Engine) SubmitRun(ctx context.Context, r Run) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	e.runs[r.ID] = &r
	sessionID := e.sessionIDForTaskLocked(r.TaskID)
	if e.store != nil {
		if err := e.store.SaveRun(ctx, r); err != nil {
			return "", err
		}
	}
	e.publish(ctx, Event{
		Type:      "run_started",
		RunID:     r.ID,
		TaskID:    r.TaskID,
		Data:      map[string]any{"session_id": sessionID, "state": r.State, "turn": r.Turn},
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
	sessionID := e.sessionIDForTaskLocked(r.TaskID)
	if e.store != nil {
		if err := e.store.SaveRun(ctx, *r); err != nil {
			return fmt.Errorf("save run %s: %w", runID, err)
		}
	}
	e.publish(ctx, Event{
		Type:      "approval_required",
		RunID:     runID,
		TaskID:    r.TaskID,
		Data:      map[string]any{"session_id": sessionID, "reason": reason},
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
	sessionID := e.sessionIDForTaskLocked(r.TaskID)
	if e.store != nil {
		if err := e.store.SaveRun(ctx, *r); err != nil {
			return fmt.Errorf("save run %s: %w", runID, err)
		}
	}
	e.publish(ctx, Event{
		Type:      "approval_resolved",
		RunID:     runID,
		TaskID:    r.TaskID,
		Data:      map[string]any{"session_id": sessionID, "decision": decision},
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
	sessionID := e.sessionIDForTaskLocked(r.TaskID)
	if e.store != nil {
		if err := e.store.SaveRun(ctx, *r); err != nil {
			return fmt.Errorf("save run %s: %w", runID, err)
		}
	}
	e.publish(ctx, Event{Type: evType, RunID: runID, TaskID: r.TaskID, Data: map[string]any{"session_id": sessionID}, Timestamp: time.Now()})
	return nil
}

func (e *Engine) sessionIDForTaskLocked(taskID string) string {
	if task, ok := e.tasks[taskID]; ok && task != nil {
		return task.SessionID
	}
	return ""
}

func normalizeEventEnvelope(ev Event) Event {
	ev.SessionID = eventSessionID(ev)
	return ev
}

func eventSessionID(ev Event) string {
	if sessionID := strings.TrimSpace(ev.SessionID); sessionID != "" {
		return sessionID
	}
	if ev.Data == nil {
		return ""
	}
	sessionID, _ := ev.Data["session_id"].(string)
	return strings.TrimSpace(sessionID)
}
