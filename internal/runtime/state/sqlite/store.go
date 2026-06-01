// Package sqlite provides the production runtime.Store implementation for the
// unified agent Task/Run/Turn event stream.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	_ "modernc.org/sqlite"
)

// Store persists runtime tasks, runs, turns, and append-only events in SQLite.
// The extra tables in the schema are intentionally created now so approvals,
// schedules, and memory snapshots can migrate onto the same database without a
// later destructive migration.
type Store struct {
	db   *sql.DB
	path string
	mu   sync.Mutex
}

// Open opens or creates a SQLite runtime store at path.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite runtime store path is empty")
	}
	if !isInMemoryDSN(path) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create runtime sqlite dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open runtime sqlite store: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, path: path}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func runtimeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func isInMemoryDSN(path string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(path))
	return trimmed == ":memory:" || strings.Contains(trimmed, "mode=memory")
}

func (s *Store) init(ctx context.Context) error {
	ctx = runtimeContext(ctx)
	stmts := []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			origin TEXT,
			goal TEXT,
			prompt TEXT,
			operation_level TEXT,
			schedule_id TEXT,
			created_at TEXT NOT NULL,
			json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_tasks_session_created ON tasks(session_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS runs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			state TEXT NOT NULL,
			turn INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_runs_task_created ON runs(task_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS turns (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			idx INTEGER NOT NULL DEFAULT 0,
			state TEXT,
			json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_turns_run_idx ON turns(run_id, idx)`,
		`CREATE TABLE IF NOT EXISTS events (
			seq INTEGER PRIMARY KEY,
			type TEXT NOT NULL,
			turn_id TEXT,
			run_id TEXT,
			task_id TEXT,
			session_id TEXT,
			data_json TEXT,
			timestamp TEXT NOT NULL,
			json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_task_seq ON events(task_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_run_seq ON events(run_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_session_seq ON events(session_id, seq)`,
		`CREATE TABLE IF NOT EXISTS approvals (
			id TEXT PRIMARY KEY,
			run_id TEXT,
			task_id TEXT,
			state TEXT,
			payload_json TEXT,
			created_at TEXT,
			updated_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS schedules (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			payload_json TEXT,
			created_at TEXT,
			updated_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			task_id TEXT,
			payload_json TEXT,
			created_at TEXT,
			updated_at TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init runtime sqlite store: %w", err)
		}
	}
	return nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Path returns the database path supplied to Open.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// SaveTask upserts a runtime task snapshot.
func (s *Store) SaveTask(ctx context.Context, t agentruntime.Task) error {
	ctx = runtimeContext(ctx)
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("runtime sqlite: task id is empty")
	}
	created := normalizeTime(t.Created)
	t.Created = created
	payload, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal runtime task %s: %w", t.ID, err)
	}
	return withTx(ctx, s.db, func(tx *sql.Tx) error {
		if strings.TrimSpace(t.SessionID) != "" {
			if err := upsertSession(ctx, tx, t.SessionID, created); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO tasks (id, session_id, origin, goal, prompt, operation_level, schedule_id, created_at, json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				session_id=excluded.session_id,
				origin=excluded.origin,
				goal=excluded.goal,
				prompt=excluded.prompt,
				operation_level=excluded.operation_level,
				schedule_id=excluded.schedule_id,
				created_at=excluded.created_at,
				json=excluded.json`,
			t.ID, t.SessionID, t.Origin, t.Goal, t.Prompt, t.OperationLevel, t.ScheduleID, formatTime(created), string(payload))
		if err != nil {
			return fmt.Errorf("save runtime task %s: %w", t.ID, err)
		}
		return nil
	})
}

// SaveRun upserts a runtime run snapshot.
func (s *Store) SaveRun(ctx context.Context, r agentruntime.Run) error {
	ctx = runtimeContext(ctx)
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("runtime sqlite: run id is empty")
	}
	created := normalizeTime(r.CreatedAt)
	r.CreatedAt = created
	payload, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal runtime run %s: %w", r.ID, err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO runs (id, task_id, state, turn, created_at, json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			task_id=excluded.task_id,
			state=excluded.state,
			turn=excluded.turn,
			created_at=excluded.created_at,
			json=excluded.json`,
		r.ID, r.TaskID, r.State, r.Turn, formatTime(created), string(payload))
	if err != nil {
		return fmt.Errorf("save runtime run %s: %w", r.ID, err)
	}
	return nil
}

// SaveTurn upserts a runtime turn snapshot.
func (s *Store) SaveTurn(ctx context.Context, tu agentruntime.Turn) error {
	ctx = runtimeContext(ctx)
	if strings.TrimSpace(tu.ID) == "" {
		return errors.New("runtime sqlite: turn id is empty")
	}
	payload, err := json.Marshal(tu)
	if err != nil {
		return fmt.Errorf("marshal runtime turn %s: %w", tu.ID, err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO turns (id, run_id, idx, state, json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			run_id=excluded.run_id,
			idx=excluded.idx,
			state=excluded.state,
			json=excluded.json`,
		tu.ID, tu.RunID, tu.Index, tu.State, string(payload))
	if err != nil {
		return fmt.Errorf("save runtime turn %s: %w", tu.ID, err)
	}
	return nil
}

// AppendEvent appends one timeline event. Engine normally assigns Seq; when
// the store is used directly in tests/tools, a missing Seq is filled from the
// database high-water mark.
func (s *Store) AppendEvent(ctx context.Context, ev agentruntime.Event) error {
	ctx = runtimeContext(ctx)
	if strings.TrimSpace(ev.Type) == "" {
		return errors.New("runtime sqlite: event type is empty")
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	sessionID := eventSessionID(ev)
	ev.SessionID = sessionID
	dataJSON, err := json.Marshal(ev.Data)
	if err != nil {
		return fmt.Errorf("marshal runtime event data: %w", err)
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal runtime event: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ev.Seq <= 0 {
		seq, err := s.maxEventSeqLocked(ctx)
		if err != nil {
			return err
		}
		ev.Seq = seq + 1
		payload, err = json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("marshal runtime event: %w", err)
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO events (seq, type, turn_id, run_id, task_id, session_id, data_json, timestamp, json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.Seq, ev.Type, ev.TurnID, ev.RunID, ev.TaskID, sessionID, string(dataJSON), formatTime(ev.Timestamp), string(payload))
	if err != nil {
		return fmt.Errorf("append runtime event seq=%d type=%s: %w", ev.Seq, ev.Type, err)
	}
	return nil
}

// LoadTask returns a task snapshot by ID.
func (s *Store) LoadTask(ctx context.Context, id string) (agentruntime.Task, error) {
	ctx = runtimeContext(ctx)
	var payload string
	if err := s.db.QueryRowContext(ctx, `SELECT json FROM tasks WHERE id = ?`, id).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agentruntime.Task{}, fmt.Errorf("runtime: task not found: %s", id)
		}
		return agentruntime.Task{}, fmt.Errorf("load runtime task %s: %w", id, err)
	}
	var t agentruntime.Task
	if err := json.Unmarshal([]byte(payload), &t); err != nil {
		return agentruntime.Task{}, fmt.Errorf("decode runtime task %s: %w", id, err)
	}
	return t, nil
}

// LoadRun returns a run snapshot by ID.
func (s *Store) LoadRun(ctx context.Context, id string) (agentruntime.Run, error) {
	ctx = runtimeContext(ctx)
	var payload string
	if err := s.db.QueryRowContext(ctx, `SELECT json FROM runs WHERE id = ?`, id).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agentruntime.Run{}, fmt.Errorf("runtime: run not found: %s", id)
		}
		return agentruntime.Run{}, fmt.Errorf("load runtime run %s: %w", id, err)
	}
	var r agentruntime.Run
	if err := json.Unmarshal([]byte(payload), &r); err != nil {
		return agentruntime.Run{}, fmt.Errorf("decode runtime run %s: %w", id, err)
	}
	return r, nil
}

// ListRuns returns runs for taskID in CreatedAt order.
func (s *Store) ListRuns(ctx context.Context, taskID string) ([]agentruntime.Run, error) {
	ctx = runtimeContext(ctx)
	rows, err := s.db.QueryContext(ctx, `SELECT json FROM runs WHERE task_id = ? ORDER BY created_at ASC, id ASC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list runtime runs for task %s: %w", taskID, err)
	}
	defer rows.Close()
	var out []agentruntime.Run
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan runtime run: %w", err)
		}
		var r agentruntime.Run
		if err := json.Unmarshal([]byte(payload), &r); err != nil {
			return nil, fmt.Errorf("decode runtime run: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime runs: %w", err)
	}
	return out, nil
}

// ListEvents returns a filtered event snapshot. FromSeq queries return the
// first matching events after FromSeq in ascending Seq order. With FromSeq=0
// and limit>0, the query selects the newest limit rows by descending Seq and
// returns that recent window in ascending order for SSE replay.
func (s *Store) ListEvents(ctx context.Context, filter agentruntime.EventFilter, limit int) ([]agentruntime.Event, error) {
	ctx = runtimeContext(ctx)
	query := `SELECT json FROM events`
	var clauses []string
	var args []any
	if filter.FromSeq > 0 {
		clauses = append(clauses, `seq > ?`)
		args = append(args, filter.FromSeq)
	}
	if strings.TrimSpace(filter.TaskID) != "" {
		clauses = append(clauses, `task_id = ?`)
		args = append(args, filter.TaskID)
	}
	if strings.TrimSpace(filter.RunID) != "" {
		clauses = append(clauses, `run_id = ?`)
		args = append(args, filter.RunID)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		clauses = append(clauses, `session_id = ?`)
		args = append(args, filter.SessionID)
	}
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	reverse := filter.FromSeq <= 0 && limit > 0
	if reverse {
		query += ` ORDER BY seq DESC`
	} else {
		query += ` ORDER BY seq ASC`
	}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runtime events: %w", err)
	}
	defer rows.Close()
	out := make([]agentruntime.Event, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan runtime event: %w", err)
		}
		var ev agentruntime.Event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			return nil, fmt.Errorf("decode runtime event: %w", err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime events: %w", err)
	}
	if reverse {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, nil
}

// MaxEventSeq returns the durable event high-water mark. runtime.NewEngine uses
// this optional method to avoid sequence reuse after daemon restart.
func (s *Store) MaxEventSeq(ctx context.Context) (int64, error) {
	ctx = runtimeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxEventSeqLocked(ctx)
}

func (s *Store) maxEventSeqLocked(ctx context.Context) (int64, error) {
	ctx = runtimeContext(ctx)
	var seq sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM events`).Scan(&seq); err != nil {
		return 0, fmt.Errorf("load runtime max event seq: %w", err)
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}

func upsertSession(ctx context.Context, tx *sql.Tx, id string, at time.Time) error {
	now := formatTime(normalizeTime(at))
	_, err := tx.ExecContext(ctx, `INSERT INTO sessions (id, created_at, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at`, id, now, now)
	if err != nil {
		return fmt.Errorf("upsert runtime session %s: %w", id, err)
	}
	return nil
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func normalizeTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func formatTime(t time.Time) string {
	return normalizeTime(t).Format(time.RFC3339Nano)
}

func eventSessionID(ev agentruntime.Event) string {
	if sessionID := strings.TrimSpace(ev.SessionID); sessionID != "" {
		return sessionID
	}
	if ev.Data != nil {
		if sessionID, ok := ev.Data["session_id"].(string); ok {
			return strings.TrimSpace(sessionID)
		}
	}
	return ""
}

var _ agentruntime.Store = (*Store)(nil)
