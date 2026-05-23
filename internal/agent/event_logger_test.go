package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestEventLoggerToolCall(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewEventLogger(dir, "test-job")
	if err != nil {
		t.Fatal(err)
	}

	logger.Log(Event{
		Type:      EventToolCall,
		ToolName:  "http_probe",
		ToolInput: json.RawMessage(`{"url":"http://example.com"}`),
	})

	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "http_probe") {
		t.Error("event log should contain tool name")
	}
	if !strings.Contains(content, "test-job") {
		t.Error("event log should contain job ID")
	}
}

func TestEventLoggerFinding(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewEventLogger(dir, "test-job")
	if err != nil {
		t.Fatal(err)
	}

	logger.Log(Event{
		Type:    EventFinding,
		Message: "Missing X-Frame-Options",
		Finding: &contracts.Finding{
			Title:    "Missing X-Frame-Options",
			Severity: contracts.SeverityMedium,
		},
	})

	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "Missing X-Frame-Options") {
		t.Error("event log should contain finding message")
	}
	if !strings.Contains(content, "medium") {
		t.Error("event log should contain severity")
	}
}

func TestEventLoggerSequence(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewEventLogger(dir, "test-job")
	if err != nil {
		t.Fatal(err)
	}

	logger.Log(Event{Type: EventToolCall, ToolName: "tool1"})
	logger.Log(Event{Type: EventToolCall, ToolName: "tool2"})
	logger.Log(Event{Type: EventDone, Message: "done"})

	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 events, got %d", len(lines))
	}
}

func TestEventLoggerSkipsTextDelta(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewEventLogger(dir, "test-job")
	if err != nil {
		t.Fatal(err)
	}

	logger.Log(Event{Type: EventTextDelta, Message: "streaming text"})

	path := filepath.Join(dir, "events.jsonl")
	data, _ := os.ReadFile(path)
	if len(data) > 0 {
		t.Error("text_delta events should not be logged")
	}
}
