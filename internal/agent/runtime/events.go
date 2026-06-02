package runtime

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const RuntimeEventSchemaVersion = "runtime.event.v1"

const (
	EventTaskCreated       = "task_created"
	EventTaskFinished      = "task_finished"
	EventTaskFailed        = "task_failed"
	EventTaskCancelled     = "task_cancelled"
	EventRunStarted        = "run_started"
	EventRunRecovered      = "run_recovered"
	EventTurnStarted       = "turn_started"
	EventTurnFinished      = "turn_finished"
	EventTurnFailed        = "turn_failed"
	EventModelToken        = "model_token"
	EventModelReasoning    = "model_reasoning"
	EventToolStarted       = "tool_started"
	EventToolFinished      = "tool_finished"
	EventToolFailed        = "tool_failed"
	EventTodoUpdated       = "todo_updated"
	EventMemoryCompacted   = "memory_compacted"
	EventApprovalRequired  = "approval_required"
	EventApprovalResolved  = "approval_resolved"
	EventApprovalCancelled = "approval_cancelled"
	EventStreamReplayStart = "stream_replay_started"
	EventStreamReplayDone  = "stream_replay_complete"
	EventStreamGapDetected = "stream_gap_detected"
	EventRuntimeError      = "runtime_error"
)

const (
	EventProducerEmitted  = "emitted"
	EventProducerReserved = "reserved"
)

const (
	EventSourceRuntimeEngine = "runtime_engine"
	EventSourceDaemonBridge  = "daemon_bridge"
	EventSourceCoreRunEvent  = "core_run_event"
	EventSourceSQLiteReplay  = "sqlite_replay"
)

// EventCatalogEntry describes the stable UI-facing contract for one runtime
// event type. The catalog is intentionally broader than today's producers:
// reserved entries let frontend normalizers tolerate future events without
// forcing behavior changes in this slice.
type EventCatalogEntry struct {
	Type           string `json:"type"`
	Category       string `json:"category"`
	Phase          string `json:"phase"`
	Status         string `json:"status"`
	Severity       string `json:"severity"`
	ProducerStatus string `json:"producer_status"`
	Description    string `json:"description,omitempty"`
}

// EventEnvelopeV1 is the canonical, UI-facing view over runtime.Event. It does
// not replace runtime.Event persistence or the legacy SSE payload; it gives
// tests and frontend reducers a stable contract to normalize into.
type EventEnvelopeV1 struct {
	SchemaVersion string         `json:"schema_version"`
	Seq           int64          `json:"seq"`
	EventID       string         `json:"event_id"`
	Type          string         `json:"type"`
	Category      string         `json:"category"`
	Phase         string         `json:"phase"`
	Status        string         `json:"status"`
	Severity      string         `json:"severity"`
	Source        string         `json:"source"`
	SessionID     string         `json:"session_id,omitempty"`
	TaskID        string         `json:"task_id,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
	TurnID        string         `json:"turn_id,omitempty"`
	ToolCallID    string         `json:"tool_call_id,omitempty"`
	ApprovalID    string         `json:"approval_id,omitempty"`
	ApprovalKind  string         `json:"approval_kind,omitempty"`
	TraceID       string         `json:"trace_id,omitempty"`
	Timestamp     string         `json:"timestamp"`
	DurationMs    *int64         `json:"duration_ms,omitempty"`
	Message       string         `json:"message,omitempty"`
	Data          map[string]any `json:"data"`
	Error         *EnvelopeError `json:"error,omitempty"`
	Links         []EnvelopeLink `json:"links,omitempty"`
}

type EnvelopeError struct {
	Code      string `json:"code,omitempty"`
	Message   string `json:"message"`
	Retryable *bool  `json:"retryable,omitempty"`
	UIHint    string `json:"ui_hint,omitempty"`
}

type EnvelopeLink struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

var runtimeEventCatalog = map[string]EventCatalogEntry{
	EventTaskCreated:       catalog(EventTaskCreated, "task", "task", "created", "info", EventProducerEmitted, "Agent task created."),
	EventTaskFinished:      catalog(EventTaskFinished, "task", "final_answer", "succeeded", "success", EventProducerEmitted, "Agent task finished successfully."),
	EventTaskFailed:        catalog(EventTaskFailed, "task", "final_answer", "failed", "error", EventProducerEmitted, "Agent task failed."),
	EventTaskCancelled:     catalog(EventTaskCancelled, "task", "system", "cancelled", "warning", EventProducerReserved, "Agent task cancelled."),
	EventRunStarted:        catalog(EventRunStarted, "run", "run", "running", "info", EventProducerEmitted, "Runtime run started."),
	EventRunRecovered:      catalog(EventRunRecovered, "recovery", "recovery", "recovered", "warning", EventProducerReserved, "Runtime run recovered after interruption."),
	EventTurnStarted:       catalog(EventTurnStarted, "turn", "step", "running", "info", EventProducerEmitted, "Runtime turn started."),
	EventTurnFinished:      catalog(EventTurnFinished, "turn", "step", "succeeded", "success", EventProducerEmitted, "Runtime turn finished."),
	EventTurnFailed:        catalog(EventTurnFailed, "turn", "step", "failed", "error", EventProducerEmitted, "Runtime turn failed."),
	EventModelToken:        catalog(EventModelToken, "model", "plan", "streaming", "info", EventProducerEmitted, "Model token or text delta."),
	EventModelReasoning:    catalog(EventModelReasoning, "model", "plan", "available", "info", EventProducerEmitted, "Model reasoning summary."),
	EventToolStarted:       catalog(EventToolStarted, "tool", "tool", "running", "info", EventProducerEmitted, "Tool execution started."),
	EventToolFinished:      catalog(EventToolFinished, "tool", "tool", "succeeded", "success", EventProducerEmitted, "Tool execution finished."),
	EventToolFailed:        catalog(EventToolFailed, "tool", "tool", "failed", "error", EventProducerReserved, "Tool execution failed."),
	EventTodoUpdated:       catalog(EventTodoUpdated, "todo", "runbook", "updated", "info", EventProducerEmitted, "Runbook TODO list updated."),
	EventMemoryCompacted:   catalog(EventMemoryCompacted, "memory", "memory", "compacted", "info", EventProducerEmitted, "Session memory compacted."),
	EventApprovalRequired:  catalog(EventApprovalRequired, "approval", "approval", "pending", "warning", EventProducerEmitted, "Approval or authorization required."),
	EventApprovalResolved:  catalog(EventApprovalResolved, "approval", "approval", "resolved", "info", EventProducerEmitted, "Approval or authorization resolved."),
	EventApprovalCancelled: catalog(EventApprovalCancelled, "approval", "approval", "cancelled", "warning", EventProducerReserved, "Approval cancelled."),
	EventStreamReplayStart: catalog(EventStreamReplayStart, "stream", "stream", "replaying", "info", EventProducerReserved, "Runtime stream replay started."),
	EventStreamReplayDone:  catalog(EventStreamReplayDone, "stream", "stream", "live", "success", EventProducerReserved, "Runtime stream replay complete."),
	EventStreamGapDetected: catalog(EventStreamGapDetected, "stream", "stream", "stale", "warning", EventProducerReserved, "Runtime stream gap detected."),
	EventRuntimeError:      catalog(EventRuntimeError, "system", "system", "failed", "error", EventProducerReserved, "Runtime system error."),
}

func catalog(typ, category, phase, status, severity, producerStatus, description string) EventCatalogEntry {
	return EventCatalogEntry{Type: typ, Category: category, Phase: phase, Status: status, Severity: severity, ProducerStatus: producerStatus, Description: description}
}

// EventCatalog returns one catalog entry by type. Unknown types are mapped to a
// stable system/unknown fallback so frontend reducers can keep rendering.
func EventCatalog(typ string) EventCatalogEntry {
	if entry, ok := runtimeEventCatalog[strings.TrimSpace(typ)]; ok {
		return entry
	}
	return catalog(strings.TrimSpace(typ), "system", "system", "unknown", "neutral", EventProducerReserved, "Unknown runtime event type.")
}

// EventCatalogEntries returns all known catalog entries sorted by type.
func EventCatalogEntries() []EventCatalogEntry {
	entries := make([]EventCatalogEntry, 0, len(runtimeEventCatalog))
	for _, entry := range runtimeEventCatalog {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Type < entries[j].Type })
	return entries
}

// NormalizeEventEnvelope normalizes a legacy runtime.Event into the stable V1
// envelope using runtime_engine as the default source.
func NormalizeEventEnvelope(ev Event) EventEnvelopeV1 {
	return NormalizeEventEnvelopeWithSource(ev, EventSourceRuntimeEngine)
}

// NormalizeEventEnvelopeWithSource normalizes a legacy runtime.Event into the
// stable V1 envelope while preserving legacy data for backwards compatibility.
func NormalizeEventEnvelopeWithSource(ev Event, source string) EventEnvelopeV1 {
	ev = normalizeEventEnvelope(ev)
	entry := EventCatalog(ev.Type)
	data := cloneEventData(ev.Data)
	phase := firstNonEmptyString(stringFromData(data, "phase"), entry.Phase)
	status := eventStatus(entry, data)
	category := eventCategory(entry, data)
	severity := eventSeverity(entry, status)
	source = firstNonEmptyString(strings.TrimSpace(source), EventSourceRuntimeEngine)
	message := eventMessage(ev, data)
	envelope := EventEnvelopeV1{
		SchemaVersion: RuntimeEventSchemaVersion,
		Seq:           ev.Seq,
		EventID:       eventEnvelopeID(ev),
		Type:          strings.TrimSpace(ev.Type),
		Category:      category,
		Phase:         phase,
		Status:        status,
		Severity:      severity,
		Source:        source,
		SessionID:     ev.SessionID,
		TaskID:        strings.TrimSpace(ev.TaskID),
		RunID:         strings.TrimSpace(ev.RunID),
		TurnID:        strings.TrimSpace(ev.TurnID),
		ToolCallID:    eventToolCallID(data),
		ApprovalID:    eventApprovalID(data),
		ApprovalKind:  stringFromData(data, "approval_kind"),
		TraceID:       stringFromData(data, "trace_id"),
		Timestamp:     ev.Timestamp.UTC().Format("2006-01-02T15:04:05Z07:00"),
		DurationMs:    int64PtrFromAny(data["duration_ms"]),
		Message:       message,
		Data:          data,
		Error:         eventError(data),
		Links:         eventLinks(ev, data),
	}
	if envelope.Data == nil {
		envelope.Data = map[string]any{}
	}
	return envelope
}

func eventEnvelopeID(ev Event) string {
	if id := stringFromData(ev.Data, "event_id"); id != "" {
		return id
	}
	if ev.Seq > 0 {
		return fmt.Sprintf("runtime:%d", ev.Seq)
	}
	parts := []string{strings.TrimSpace(ev.Type), strings.TrimSpace(ev.SessionID), strings.TrimSpace(ev.TaskID), strings.TrimSpace(ev.RunID), strings.TrimSpace(ev.TurnID)}
	return "runtime:" + strings.Join(parts, ":")
}

func eventCategory(entry EventCatalogEntry, data map[string]any) string {
	if entry.Type == EventTaskFailed && boolFromData(data, "recovery") {
		return "recovery"
	}
	return entry.Category
}

func eventStatus(entry EventCatalogEntry, data map[string]any) string {
	switch entry.Type {
	case EventTurnFinished:
		if status := stringFromData(data, "status"); status != "" {
			return status
		}
	case EventToolFinished:
		if hasErrorData(data) {
			return "failed"
		}
		if status := stringFromToolCall(data); status != "" {
			return normalizeToolStatus(status)
		}
		if status := stringFromData(data, "status"); status != "" {
			return normalizeToolStatus(status)
		}
		if ok, exists := boolData(data, "ok"); exists && !ok {
			return "failed"
		}
	case EventApprovalResolved:
		if decision := stringFromData(data, "decision"); decision != "" {
			return normalizeApprovalDecision(decision)
		}
		if status := stringFromData(data, "status"); status != "" {
			return normalizeApprovalDecision(status)
		}
	}
	return entry.Status
}

func eventSeverity(entry EventCatalogEntry, status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "approved", "resolved", "live", "done", "finished":
		return "success"
	case "failed", "error", "stale":
		return "error"
	case "pending", "approval_required", "cancelled", "modified", "replaying", "blocked", "awaiting_approval":
		return "warning"
	case "unknown":
		return "neutral"
	default:
		return entry.Severity
	}
}

func normalizeToolStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "ok", "done", "finished":
		return "succeeded"
	case "failed", "failure", "error":
		return "failed"
	case "cancelled", "canceled":
		return "cancelled"
	case "running":
		return "running"
	default:
		return strings.TrimSpace(status)
	}
}

func normalizeApprovalDecision(decision string) string {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve", "approved", "hitl_approved":
		return "approved"
	case "deny", "denied", "reject", "rejected", "hitl_rejected":
		return "denied"
	case "cancel", "cancelled", "canceled", "hitl_cancelled":
		return "cancelled"
	case "edit_args", "edit-args", "modified", "hitl_modified":
		return "modified"
	default:
		return strings.TrimSpace(decision)
	}
}

func eventMessage(ev Event, data map[string]any) string {
	for _, key := range []string{"message", "text", "reason", "summary", "tool"} {
		if value := stringFromData(data, key); value != "" {
			return value
		}
	}
	return strings.TrimSpace(ev.Type)
}

func eventToolCallID(data map[string]any) string {
	if id := stringFromData(data, "tool_call_id"); id != "" {
		return id
	}
	return nestedString(data["tool_call"], "id", "ID")
}

func eventApprovalID(data map[string]any) string {
	if id := stringFromData(data, "approval_id"); id != "" {
		return id
	}
	if id := stringFromData(data, "approval_request_id"); id != "" {
		return id
	}
	return nestedString(data["authorization"], "id", "ID")
}

func eventLinks(ev Event, data map[string]any) []EnvelopeLink {
	links := []EnvelopeLink{}
	if ev.SessionID != "" {
		links = append(links, EnvelopeLink{Kind: "session", ID: ev.SessionID})
	}
	if ev.TaskID != "" {
		links = append(links, EnvelopeLink{Kind: "task", ID: ev.TaskID})
	}
	if ev.RunID != "" {
		links = append(links, EnvelopeLink{Kind: "run", ID: ev.RunID})
	}
	if ev.TurnID != "" {
		links = append(links, EnvelopeLink{Kind: "turn", ID: ev.TurnID})
	}
	if toolCallID := eventToolCallID(data); toolCallID != "" {
		links = append(links, EnvelopeLink{Kind: "tool_call", ID: toolCallID})
	}
	if approvalID := eventApprovalID(data); approvalID != "" {
		links = append(links, EnvelopeLink{Kind: "approval", ID: approvalID})
	}
	return links
}

func eventError(data map[string]any) *EnvelopeError {
	if errText := stringFromData(data, "error"); errText != "" {
		return &EnvelopeError{Code: stringFromData(data, "error_code"), Message: errText, UIHint: stringFromData(data, "ui_hint")}
	}
	errValue, ok := data["error"]
	if !ok || errValue == nil {
		return nil
	}
	if envelopeError := eventErrorFromValue(errValue); envelopeError != nil {
		if envelopeError.Code == "" {
			envelopeError.Code = stringFromData(data, "error_code")
		}
		if envelopeError.UIHint == "" {
			envelopeError.UIHint = stringFromData(data, "ui_hint")
		}
		return envelopeError
	}
	return nil
}

func eventErrorFromValue(value any) *EnvelopeError {
	if message := stringFromAny(value); message != "" {
		return &EnvelopeError{Message: message}
	}
	message := firstNonEmptyString(nestedString(value, "message", "Message"))
	if message == "" {
		return nil
	}
	return &EnvelopeError{
		Code:      firstNonEmptyString(nestedString(value, "code", "Code")),
		Message:   message,
		Retryable: nestedBool(value, "retryable", "Retryable"),
		UIHint:    firstNonEmptyString(nestedString(value, "ui_hint", "UIHint")),
	}
}

func hasErrorData(data map[string]any) bool {
	if stringFromData(data, "error") != "" {
		return true
	}
	_, ok := data["error"]
	return ok && data["error"] != nil
}

func stringFromToolCall(data map[string]any) string {
	if status := nestedString(data["tool_call"], "status", "Status"); status != "" {
		return status
	}
	return ""
}

func nestedString(value any, names ...string) string {
	if value == nil {
		return ""
	}
	if m, ok := value.(map[string]any); ok {
		for _, name := range names {
			if s := stringFromAny(m[name]); s != "" {
				return s
			}
		}
		return ""
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	for _, name := range names {
		field := v.FieldByName(name)
		if field.IsValid() && field.CanInterface() {
			if s := stringFromAny(field.Interface()); s != "" {
				return s
			}
		}
	}
	return ""
}

func nestedBool(value any, names ...string) *bool {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		for _, name := range names {
			if b, ok := boolData(m, name); ok {
				return &b
			}
		}
		return nil
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	for _, name := range names {
		field := v.FieldByName(name)
		if field.IsValid() && field.CanInterface() {
			if b, ok := boolFromAny(field.Interface()); ok {
				return &b
			}
		}
	}
	return nil
}

func stringFromData(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	return strings.TrimSpace(stringFromAny(data[key]))
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case error:
		return v.Error()
	case fmt.Stringer:
		return v.String()
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() && rv.Kind() == reflect.String {
			return rv.String()
		}
		return ""
	}
}

func boolFromData(data map[string]any, key string) bool {
	value, _ := boolData(data, key)
	return value
}

func boolData(data map[string]any, key string) (bool, bool) {
	if data == nil {
		return false, false
	}
	switch v := data[key].(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	}
	return false, false
}

func boolFromAny(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() && rv.Kind() == reflect.Bool {
			return rv.Bool(), true
		}
	}
	return false, false
}

func int64PtrFromAny(value any) *int64 {
	switch v := value.(type) {
	case int:
		out := int64(v)
		return &out
	case int64:
		out := v
		return &out
	case int32:
		out := int64(v)
		return &out
	case float64:
		out := int64(v)
		return &out
	case float32:
		out := int64(v)
		return &out
	default:
		return nil
	}
}

func cloneEventData(data map[string]any) map[string]any {
	if data == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(data))
	for key, value := range data {
		out[key] = value
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
