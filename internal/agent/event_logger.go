package agent

import (
	"fmt"
	"sync/atomic"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
)

type EventLogger struct {
	bus   *events.Bus
	jobID string
	seq   atomic.Int64
}

func NewEventLogger(workspace string, jobID string) (*EventLogger, error) {
	store := events.NewStore(workspace + "/events.jsonl")
	bus := events.NewBus(store)
	return &EventLogger{bus: bus, jobID: jobID}, nil
}

func (l *EventLogger) Log(e Event) {
	seq := l.seq.Add(1)
	id := fmt.Sprintf("agent_event_%d", seq)

	var eventType contracts.EventType
	var level contracts.EventLevel
	var message string
	data := map[string]any{}

	switch e.Type {
	case EventToolCall:
		eventType = contracts.EventToolStarted
		level = contracts.EventLevelInfo
		message = fmt.Sprintf("tool call: %s", e.ToolName)
		data["tool"] = e.ToolName
		if e.ToolInput != nil {
			data["input"] = string(e.ToolInput)
		}
	case EventToolResult:
		eventType = contracts.EventToolFinished
		level = contracts.EventLevelInfo
		message = fmt.Sprintf("tool result: %s", e.ToolName)
		data["tool"] = e.ToolName
		data["preview"] = e.Message
	case EventFinding:
		eventType = contracts.EventFindingCreated
		level = contracts.EventLevelWarn
		message = e.Message
		if e.Finding != nil {
			data["severity"] = string(e.Finding.Severity)
			data["title"] = e.Finding.Title
		}
	case EventError:
		eventType = contracts.EventError
		level = contracts.EventLevelError
		message = fmt.Sprintf("%s: %s", e.ToolName, e.Message)
		data["tool"] = e.ToolName
	case EventDone:
		eventType = contracts.EventJobFinished
		level = contracts.EventLevelInfo
		message = "agent scan completed"
	default:
		return
	}

	event := contracts.NewEvent(id, l.jobID, "agent", eventType, level, message)
	event.Data = data
	l.bus.Publish(event)
}
