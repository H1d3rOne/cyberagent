// Package runtime — in-memory Store implementation. Used by tests and the
// initial daemon wiring; production deployments will swap in a SQLite-backed
// Store in Phase 6 without changing the interface.
package runtime

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// MemStore satisfies Store with goroutine-safe maps and an append-only event
// log. Reads use RWMutex so the streaming UI can fan out without blocking
// engine writes.
type MemStore struct {
	mu     sync.RWMutex
	tasks  map[string]Task
	runs   map[string]Run
	turns  map[string]Turn
	events []Event
}

// NewMemStore returns an empty in-memory Store.
func NewMemStore() *MemStore {
	return &MemStore{
		tasks: map[string]Task{},
		runs:  map[string]Run{},
		turns: map[string]Turn{},
	}
}

// SaveTask persists a Task. Overwrites are intentional: callers re-save the
// same ID when state transitions.
func (m *MemStore) SaveTask(_ context.Context, t Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[t.ID] = t
	return nil
}

// SaveRun persists a Run.
func (m *MemStore) SaveRun(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[r.ID] = r
	return nil
}

// SaveTurn persists a Turn.
func (m *MemStore) SaveTurn(_ context.Context, t Turn) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turns[t.ID] = t
	return nil
}

// AppendEvent appends one Event to the log.
func (m *MemStore) AppendEvent(_ context.Context, e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
	return nil
}

// LoadTask returns a Task by ID. Errors when the ID is unknown so callers do
// not silently operate on a zero-value Task.
func (m *MemStore) LoadTask(_ context.Context, id string) (Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return Task{}, errors.New("runtime: task not found: " + id)
	}
	return t, nil
}

// LoadRun returns a Run by ID.
func (m *MemStore) LoadRun(_ context.Context, id string) (Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, errors.New("runtime: run not found: " + id)
	}
	return r, nil
}

// ListRuns returns every Run for one Task in CreatedAt-ascending order so
// callers can iterate in submission order without re-sorting.
func (m *MemStore) ListRuns(_ context.Context, taskID string) ([]Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Run
	for _, r := range m.runs {
		if r.TaskID == taskID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ListEvents returns a filtered snapshot of the append-only event log. It is
// used for SSE reconnect replay before the caller starts receiving live
// fan-out events. Results preserve append order and are capped by limit when
// limit > 0.
func (m *MemStore) ListEvents(_ context.Context, filter EventFilter, limit int) ([]Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Event, 0, len(m.events))
	for _, ev := range m.events {
		if !eventMatchesFilter(ev, filter) {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Events returns a snapshot of the event log for tests. Production callers
// should prefer Engine.Subscribe with EventFilter/FromSeq.
func (m *MemStore) Events() []Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Event, len(m.events))
	copy(out, m.events)
	return out
}
