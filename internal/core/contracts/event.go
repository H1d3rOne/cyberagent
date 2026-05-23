package contracts

import "time"

// EventLevel describes event severity for display.
type EventLevel string

const (
	EventLevelDebug EventLevel = "debug"
	EventLevelInfo  EventLevel = "info"
	EventLevelWarn  EventLevel = "warn"
	EventLevelError EventLevel = "error"
)

// EventType describes an append-only runtime fact.
type EventType string

const (
	EventJobCreated        EventType = "job.created"
	EventJobStarted        EventType = "job.started"
	EventJobFinished       EventType = "job.finished"
	EventPlanCreated       EventType = "plan.created"
	EventPlanUpdated       EventType = "plan.updated"
	EventStepStarted       EventType = "step.started"
	EventStepFinished      EventType = "step.finished"
	EventStepFailed        EventType = "step.failed"
	EventStepSkipped       EventType = "step.skipped"
	EventPolicyChecked     EventType = "policy.checked"
	EventApprovalRequested EventType = "approval.requested"
	EventApprovalResolved  EventType = "approval.resolved"
	EventToolStarted       EventType = "tool.started"
	EventToolOutput        EventType = "tool.output"
	EventToolFinished      EventType = "tool.finished"
	EventArtifactCreated   EventType = "artifact.created"
	EventEvidenceCreated   EventType = "evidence.created"
	EventFindingCreated    EventType = "finding.created"
	EventReflectionCreated EventType = "reflection.created"
	EventReportCreated     EventType = "report.created"
	EventError             EventType = "error"
	EventWarning           EventType = "warning"
)

// Event is an append-only fact record.
type Event struct {
	ID        string         `json:"id"`
	JobID     string         `json:"job_id"`
	StepID    string         `json:"step_id,omitempty"`
	Type      EventType      `json:"type"`
	Level     EventLevel     `json:"level"`
	Timestamp time.Time      `json:"timestamp"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
}

// NewEvent creates a timestamped event.
func NewEvent(id, jobID, stepID string, typ EventType, level EventLevel, message string) Event {
	return Event{ID: id, JobID: jobID, StepID: stepID, Type: typ, Level: level, Timestamp: Now(), Message: message}
}
