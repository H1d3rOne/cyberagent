package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRuntimeEventCatalogFixture(t *testing.T) {
	entries := EventCatalogEntries()
	if len(entries) == 0 {
		t.Fatal("EventCatalogEntries() returned no entries")
	}

	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Type == "" || entry.Category == "" || entry.Phase == "" || entry.Status == "" || entry.Severity == "" || entry.ProducerStatus == "" {
			t.Fatalf("catalog entry has empty required field: %#v", entry)
		}
		if seen[entry.Type] {
			t.Fatalf("duplicate catalog type %q", entry.Type)
		}
		seen[entry.Type] = true
	}
	for _, typ := range []string{
		EventTaskCreated,
		EventRunStarted,
		EventTurnStarted,
		EventModelToken,
		EventToolStarted,
		EventToolFinished,
		EventApprovalRequired,
		EventApprovalResolved,
		EventTaskFinished,
		EventTaskFailed,
	} {
		entry := EventCatalog(typ)
		if entry.Type != typ || entry.ProducerStatus != EventProducerEmitted {
			t.Fatalf("EventCatalog(%q) = %#v, want emitted entry", typ, entry)
		}
	}
	for _, typ := range []string{EventRunRecovered, EventStreamReplayStart, EventRuntimeError} {
		entry := EventCatalog(typ)
		if entry.Type != typ || entry.ProducerStatus != EventProducerReserved {
			t.Fatalf("EventCatalog(%q) = %#v, want reserved entry", typ, entry)
		}
	}

	assertRuntimeEventsFixtureJSON(t, filepath.Join("catalog.json"), entries)
}

func TestRuntimeEventCatalogUnknownFallback(t *testing.T) {
	entry := EventCatalog(" legacy.custom ")
	if entry.Type != "legacy.custom" || entry.Category != "system" || entry.Status != "unknown" || entry.Severity != "neutral" || entry.ProducerStatus != EventProducerReserved {
		t.Fatalf("unknown fallback = %#v", entry)
	}
}

func TestRuntimeEventEnvelopeFixtures(t *testing.T) {
	for _, tc := range runtimeEventEnvelopeFixtureCases() {
		t.Run(tc.name, func(t *testing.T) {
			actual := NormalizeEventEnvelopeWithSource(tc.event, tc.source)
			assertRuntimeEventsFixtureJSON(t, tc.fixture, actual)
		})
	}
}

func TestRuntimeEventSequenceFixtures(t *testing.T) {
	for _, tc := range runtimeEventSequenceFixtureCases() {
		t.Run(tc.name, func(t *testing.T) {
			actual := make([]EventEnvelopeV1, 0, len(tc.events))
			for _, ev := range tc.events {
				actual = append(actual, NormalizeEventEnvelopeWithSource(ev, tc.source))
			}
			assertRuntimeEventsFixtureJSON(t, tc.fixture, actual)
		})
	}
}

type runtimeEventEnvelopeFixtureCase struct {
	name    string
	fixture string
	source  string
	event   Event
}

type runtimeEventSequenceFixtureCase struct {
	name    string
	fixture string
	source  string
	events  []Event
}

func runtimeEventFixtureTime(offset time.Duration) time.Time {
	return time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC).Add(offset)
}

func runtimeEventEnvelopeFixtureCases() []runtimeEventEnvelopeFixtureCase {
	return []runtimeEventEnvelopeFixtureCase{
		{
			name:    "model_token",
			fixture: filepath.Join("envelopes", "model_token.json"),
			source:  EventSourceRuntimeEngine,
			event: Event{
				Seq:       1,
				Type:      EventModelToken,
				SessionID: "session-normal",
				TaskID:    "task-normal",
				RunID:     "run-normal",
				TurnID:    "turn-normal",
				Data: map[string]any{
					"session_id": "session-normal",
					"phase":      "plan",
					"provider":   "openai",
					"delta":      "Hello",
					"text":       "Hello",
				},
				Timestamp: runtimeEventFixtureTime(0),
			},
		},
		{
			name:    "tool_finished_success",
			fixture: filepath.Join("envelopes", "tool_finished_success.json"),
			source:  EventSourceRuntimeEngine,
			event: Event{
				Seq:       2,
				Type:      EventToolFinished,
				SessionID: "session-tool",
				TaskID:    "task-tool",
				RunID:     "run-tool",
				TurnID:    "turn-tool",
				Data: map[string]any{
					"session_id":  "session-tool",
					"tool":        "workspace.command",
					"tool_call":   map[string]any{"id": "tool-call-1", "status": "succeeded"},
					"status":      "succeeded",
					"ok":          true,
					"duration_ms": 123,
				},
				Timestamp: runtimeEventFixtureTime(time.Second),
			},
		},
		{
			name:    "approval_required_agent_authorization",
			fixture: filepath.Join("envelopes", "approval_required_agent_authorization.json"),
			source:  EventSourceRuntimeEngine,
			event: Event{
				Seq:       3,
				Type:      EventApprovalRequired,
				SessionID: "session-approval",
				TaskID:    "task-approval",
				RunID:     "run-approval",
				Data: map[string]any{
					"session_id":     "session-approval",
					"approval_kind":  "agent_authorization",
					"approval_id":    "auth-1",
					"action_name":    "workspace.files.write",
					"required_level": "write",
					"reason":         "needs write approval",
				},
				Timestamp: runtimeEventFixtureTime(2 * time.Second),
			},
		},
		{
			name:    "tool_finished_failed",
			fixture: filepath.Join("envelopes", "tool_finished_failed.json"),
			source:  EventSourceRuntimeEngine,
			event: Event{
				Seq:       7,
				Type:      EventToolFinished,
				SessionID: "session-tool",
				TaskID:    "task-tool",
				RunID:     "run-tool",
				TurnID:    "turn-tool",
				Data: map[string]any{
					"session_id":  "session-tool",
					"tool":        "workspace.command",
					"tool_call":   map[string]any{"id": "tool-call-2", "status": "failed"},
					"status":      "failed",
					"error":       "command failed",
					"error_code":  "exit_2",
					"ui_hint":     "Show command stderr.",
					"duration_ms": 456,
				},
				Timestamp: runtimeEventFixtureTime(6 * time.Second),
			},
		},
		{
			name:    "approval_resolved_approved",
			fixture: filepath.Join("envelopes", "approval_resolved_approved.json"),
			source:  EventSourceRuntimeEngine,
			event: Event{
				Seq:       4,
				Type:      EventApprovalResolved,
				SessionID: "session-approval",
				TaskID:    "task-approval",
				RunID:     "run-approval",
				Data: map[string]any{
					"session_id":    "session-approval",
					"approval_kind": "agent_authorization",
					"approval_id":   "auth-1",
					"action_name":   "workspace.files.write",
					"decision":      "approved",
				},
				Timestamp: runtimeEventFixtureTime(3 * time.Second),
			},
		},
		{
			name:    "task_failed_recovery",
			fixture: filepath.Join("envelopes", "task_failed_recovery.json"),
			source:  EventSourceRuntimeEngine,
			event: Event{
				Seq:       5,
				Type:      EventTaskFailed,
				SessionID: "session-recovery",
				TaskID:    "task-recovery",
				RunID:     "run-recovery",
				Data: map[string]any{
					"session_id":     "session-recovery",
					"recovery":       true,
					"recoverable":    true,
					"previous_state": "running",
					"reason":         "daemon_restarted",
					"run_id":         "run-recovery",
					"task_id":        "task-recovery",
				},
				Timestamp: runtimeEventFixtureTime(4 * time.Second),
			},
		},
		{
			name:    "unknown_event_type",
			fixture: filepath.Join("legacy", "unknown_event_type.json"),
			source:  EventSourceCoreRunEvent,
			event: Event{
				Seq:       6,
				Type:      "legacy.custom",
				SessionID: "session-legacy",
				TaskID:    "task-legacy",
				Data: map[string]any{
					"session_id": "session-legacy",
					"message":    "legacy event payload",
				},
				Timestamp: runtimeEventFixtureTime(5 * time.Second),
			},
		},
	}
}

func runtimeEventSequenceFixtureCases() []runtimeEventSequenceFixtureCase {
	return []runtimeEventSequenceFixtureCase{
		{
			name:    "normal_tool_turn",
			fixture: filepath.Join("sequences", "normal_tool_turn.json"),
			source:  EventSourceRuntimeEngine,
			events: []Event{
				{
					Seq:       10,
					Type:      EventTurnStarted,
					SessionID: "session-normal",
					TaskID:    "task-normal",
					RunID:     "run-normal",
					TurnID:    "turn-normal",
					Data:      map[string]any{"session_id": "session-normal", "message_id": "msg-normal", "origin": "user"},
					Timestamp: runtimeEventFixtureTime(10 * time.Second),
				},
				{
					Seq:       11,
					Type:      EventModelToken,
					SessionID: "session-normal",
					TaskID:    "task-normal",
					RunID:     "run-normal",
					TurnID:    "turn-normal",
					Data:      map[string]any{"session_id": "session-normal", "phase": "plan", "delta": "Hello", "text": "Hello"},
					Timestamp: runtimeEventFixtureTime(11 * time.Second),
				},
				{
					Seq:       12,
					Type:      EventToolStarted,
					SessionID: "session-normal",
					TaskID:    "task-normal",
					RunID:     "run-normal",
					TurnID:    "turn-normal",
					Data:      map[string]any{"session_id": "session-normal", "tool": "workspace.command", "tool_call": map[string]any{"id": "tool-call-normal", "status": "running"}, "status": "running"},
					Timestamp: runtimeEventFixtureTime(12 * time.Second),
				},
				{
					Seq:       13,
					Type:      EventToolFinished,
					SessionID: "session-normal",
					TaskID:    "task-normal",
					RunID:     "run-normal",
					TurnID:    "turn-normal",
					Data:      map[string]any{"session_id": "session-normal", "tool": "workspace.command", "tool_call": map[string]any{"id": "tool-call-normal", "status": "succeeded"}, "status": "succeeded", "ok": true},
					Timestamp: runtimeEventFixtureTime(13 * time.Second),
				},
				{
					Seq:       14,
					Type:      EventTurnFinished,
					SessionID: "session-normal",
					TaskID:    "task-normal",
					RunID:     "run-normal",
					TurnID:    "turn-normal",
					Data:      map[string]any{"session_id": "session-normal", "status": "finished", "reason": "done"},
					Timestamp: runtimeEventFixtureTime(14 * time.Second),
				},
			},
		},
		{
			name:    "approval_gate_agent_authorization",
			fixture: filepath.Join("sequences", "approval_gate_agent_authorization.json"),
			source:  EventSourceRuntimeEngine,
			events: []Event{
				{
					Seq:       20,
					Type:      EventApprovalRequired,
					SessionID: "session-approval",
					TaskID:    "task-approval",
					RunID:     "run-approval",
					Data:      map[string]any{"session_id": "session-approval", "approval_kind": "agent_authorization", "approval_id": "auth-1", "action_name": "workspace.files.write", "required_level": "write", "reason": "needs write approval"},
					Timestamp: runtimeEventFixtureTime(20 * time.Second),
				},
				{
					Seq:       21,
					Type:      EventApprovalResolved,
					SessionID: "session-approval",
					TaskID:    "task-approval",
					RunID:     "run-approval",
					Data:      map[string]any{"session_id": "session-approval", "approval_kind": "agent_authorization", "approval_id": "auth-1", "action_name": "workspace.files.write", "decision": "approved"},
					Timestamp: runtimeEventFixtureTime(21 * time.Second),
				},
			},
		},
		{
			name:    "recovery_interrupted_run",
			fixture: filepath.Join("sequences", "recovery_interrupted_run.json"),
			source:  EventSourceRuntimeEngine,
			events: []Event{
				{
					Seq:       30,
					Type:      EventTaskFailed,
					SessionID: "session-recovery",
					TaskID:    "task-recovery",
					RunID:     "run-recovery",
					Data:      map[string]any{"session_id": "session-recovery", "recovery": true, "recoverable": true, "previous_state": "awaiting_approval", "reason": "daemon_restarted", "run_id": "run-recovery", "task_id": "task-recovery"},
					Timestamp: runtimeEventFixtureTime(30 * time.Second),
				},
			},
		},
		{
			name:    "stream_replay_from_seq",
			fixture: filepath.Join("sequences", "stream_replay_from_seq.json"),
			source:  EventSourceSQLiteReplay,
			events: []Event{
				{
					Seq:       41,
					Type:      EventModelToken,
					SessionID: "session-replay",
					TaskID:    "task-replay",
					RunID:     "run-replay",
					TurnID:    "turn-replay",
					Data:      map[string]any{"session_id": "session-replay", "phase": "plan", "delta": "two", "text": "two"},
					Timestamp: runtimeEventFixtureTime(41 * time.Second),
				},
				{
					Seq:       42,
					Type:      EventModelToken,
					SessionID: "session-replay",
					TaskID:    "task-replay",
					RunID:     "run-replay",
					TurnID:    "turn-replay",
					Data:      map[string]any{"session_id": "session-replay", "phase": "plan", "delta": "three", "text": "three"},
					Timestamp: runtimeEventFixtureTime(42 * time.Second),
				},
			},
		},
	}
}

func assertRuntimeEventsFixtureJSON(t *testing.T, rel string, actual any) {
	t.Helper()
	pretty, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		t.Fatalf("marshal actual fixture %s: %v", rel, err)
	}
	pretty = append(pretty, '\n')
	path := runtimeEventsFixturePath(rel)
	if os.Getenv("UPDATE_RUNTIME_EVENT_FIXTURES") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture dir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, pretty, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v\nset UPDATE_RUNTIME_EVENT_FIXTURES=1 to regenerate", path, err)
	}
	if !jsonDeepEqual(wantBytes, pretty) {
		t.Fatalf("fixture %s mismatch\nactual:\n%s", path, string(pretty))
	}
}

func runtimeEventsFixturePath(rel string) string {
	return filepath.Join("..", "..", "..", "testdata", "runtime-events", "v1", rel)
}

func jsonDeepEqual(a, b []byte) bool {
	var va any
	var vb any
	if err := json.Unmarshal(a, &va); err != nil {
		panic(fmt.Sprintf("fixture JSON is invalid: %v", err))
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		panic(fmt.Sprintf("actual JSON is invalid: %v", err))
	}
	return reflect.DeepEqual(va, vb)
}
