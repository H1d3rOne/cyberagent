package runtime

import "time"

// AgentLoopRecordUserMessageResultPayload is the runtime-neutral result for a
// record_user_message session mutation. It intentionally carries only session
// snapshot DTOs and loop DTOs; daemon legacy structs convert at adapter edges.
type AgentLoopRecordUserMessageResultPayload struct {
	Session       AgentLoopSessionSnapshot
	UserMessage   AgentLoopMessage
	History       []AgentLoopMessage
	MemorySummary *AgentLoopMemorySummary
	Compaction    *AgentLoopMemoryCompaction
	Mutated       bool
}

// Clone returns a copy with all mutable fields isolated.
func (payload AgentLoopRecordUserMessageResultPayload) Clone() AgentLoopRecordUserMessageResultPayload {
	cloned := payload
	cloned.Session = payload.Session.Clone()
	cloned.UserMessage = payload.UserMessage.Clone()
	cloned.History = CloneAgentLoopMessages(payload.History)
	cloned.MemorySummary = CloneAgentLoopMemorySummary(payload.MemorySummary)
	cloned.Compaction = CloneAgentLoopMemoryCompaction(payload.Compaction)
	return cloned
}

// CloneAgentLoopRecordUserMessageResultPayload clones a record-user-message
// mutation result payload.
func CloneAgentLoopRecordUserMessageResultPayload(payload AgentLoopRecordUserMessageResultPayload) AgentLoopRecordUserMessageResultPayload {
	return payload.Clone()
}

// AgentLoopControlSessionResultPayload is the runtime-neutral result for a
// control_session mutation. It carries the updated session snapshot, the
// operator-facing control message, optional job cancellation hint, and optional
// memory compaction artifact without depending on daemon-private structs.
type AgentLoopControlSessionResultPayload struct {
	Session     AgentLoopSessionSnapshot
	Message     AgentLoopMessage
	CancelJobID string
	Compaction  *AgentLoopMemoryCompaction
	Mutated     bool
}

// Clone returns a copy with all mutable fields isolated.
func (payload AgentLoopControlSessionResultPayload) Clone() AgentLoopControlSessionResultPayload {
	cloned := payload
	cloned.Session = payload.Session.Clone()
	cloned.Message = payload.Message.Clone()
	cloned.Compaction = CloneAgentLoopMemoryCompaction(payload.Compaction)
	return cloned
}

// CloneAgentLoopControlSessionResultPayload clones a control-session mutation
// result payload.
func CloneAgentLoopControlSessionResultPayload(payload AgentLoopControlSessionResultPayload) AgentLoopControlSessionResultPayload {
	return payload.Clone()
}

// AgentLoopResolveAuthorizationResultPayload is the runtime-neutral result for
// a resolve_authorization mutation. It carries the updated session snapshot,
// resolved authorization snapshot, operator-facing message, effective operation
// level, and optional compaction artifact without daemon-private structs.
type AgentLoopResolveAuthorizationResultPayload struct {
	Session        AgentLoopSessionSnapshot
	Pending        AgentLoopAuthorizationRequestSnapshot
	Message        AgentLoopMessage
	OperationLevel string
	Compaction     *AgentLoopMemoryCompaction
	Mutated        bool
}

// Clone returns a copy with all mutable fields isolated.
func (payload AgentLoopResolveAuthorizationResultPayload) Clone() AgentLoopResolveAuthorizationResultPayload {
	cloned := payload
	cloned.Session = payload.Session.Clone()
	cloned.Pending = payload.Pending.Clone()
	cloned.Message = payload.Message.Clone()
	cloned.Compaction = CloneAgentLoopMemoryCompaction(payload.Compaction)
	return cloned
}

// CloneAgentLoopResolveAuthorizationResultPayload clones a resolve-
// authorization mutation result payload.
func CloneAgentLoopResolveAuthorizationResultPayload(payload AgentLoopResolveAuthorizationResultPayload) AgentLoopResolveAuthorizationResultPayload {
	return payload.Clone()
}

// AgentLoopSessionSnapshot is the runtime-neutral session state carried by
// session mutation result DTOs. It mirrors the operator-visible session
// boundary without depending on daemon-private structs.
type AgentLoopSessionSnapshot struct {
	SessionID            string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Operator             string
	Language             string
	OperationLevel       string
	Status               string
	Messages             []AgentLoopMessage
	ActiveTask           *AgentLoopSessionTask
	Todos                []AgentLoopSessionTodo
	ToolCalls            []AgentLoopToolCall
	ContextRefs          []AgentLoopContextRef
	PendingChanges       []AgentLoopWorkspaceChange
	PendingAuthorization *AgentLoopAuthorizationRequestSnapshot
	ApprovalState        *AgentLoopApprovalStateSnapshot
	MemorySummary        *AgentLoopMemorySummary
	NextAction           *AgentLoopNextAction
	RunSummary           *AgentLoopSessionRunSummary
	UIState              *AgentLoopUIState
}

// Clone returns a copy with all mutable fields isolated.
func (snapshot AgentLoopSessionSnapshot) Clone() AgentLoopSessionSnapshot {
	cloned := snapshot
	cloned.Messages = CloneAgentLoopMessages(snapshot.Messages)
	cloned.ActiveTask = CloneAgentLoopSessionTask(snapshot.ActiveTask)
	cloned.Todos = CloneAgentLoopSessionTodos(snapshot.Todos)
	cloned.ToolCalls = CloneAgentLoopToolCalls(snapshot.ToolCalls)
	cloned.ContextRefs = CloneAgentLoopContextRefs(snapshot.ContextRefs)
	cloned.PendingChanges = CloneAgentLoopWorkspaceChanges(snapshot.PendingChanges)
	cloned.PendingAuthorization = CloneAgentLoopAuthorizationRequestSnapshot(snapshot.PendingAuthorization)
	cloned.ApprovalState = CloneAgentLoopApprovalStateSnapshot(snapshot.ApprovalState)
	cloned.MemorySummary = CloneAgentLoopMemorySummary(snapshot.MemorySummary)
	cloned.NextAction = CloneAgentLoopNextAction(snapshot.NextAction)
	cloned.RunSummary = CloneAgentLoopSessionRunSummary(snapshot.RunSummary)
	cloned.UIState = CloneAgentLoopUIState(snapshot.UIState)
	return cloned
}

// CloneAgentLoopSessionSnapshot clones a session snapshot.
func CloneAgentLoopSessionSnapshot(snapshot AgentLoopSessionSnapshot) AgentLoopSessionSnapshot {
	return snapshot.Clone()
}

// AgentLoopSessionTask is the runtime-neutral active task snapshot.
type AgentLoopSessionTask struct {
	ID          string
	Goal        string
	Kind        string
	Status      string
	ActiveRunID string
	Summary     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CloneAgentLoopSessionTask clones an optional session task.
func CloneAgentLoopSessionTask(task *AgentLoopSessionTask) *AgentLoopSessionTask {
	if task == nil {
		return nil
	}
	cloned := *task
	return &cloned
}

// AgentLoopSessionTodo is the runtime-neutral todo snapshot.
type AgentLoopSessionTodo struct {
	ID            string
	Title         string
	Detail        string
	Status        string
	Priority      int
	ToolCallID    string
	ContextRefIDs []string
	BlockedReason string
	UpdatedAt     time.Time
}

// CloneAgentLoopSessionTodos clones todo snapshots.
func CloneAgentLoopSessionTodos(todos []AgentLoopSessionTodo) []AgentLoopSessionTodo {
	if todos == nil {
		return nil
	}
	out := make([]AgentLoopSessionTodo, len(todos))
	copy(out, todos)
	for i := range todos {
		out[i].ContextRefIDs = CloneAgentLoopStringSlice(todos[i].ContextRefIDs)
	}
	return out
}

// AgentLoopWorkspaceChange is the runtime-neutral pending workspace change
// snapshot carried by session mutation results.
type AgentLoopWorkspaceChange struct {
	ID             string
	GroupID        string
	Path           string
	OldText        string
	NewText        string
	SizeBefore     int
	SizeAfter      int
	Replacements   int
	DiffPreview    string
	ContentPreview string
	Status         string
	Error          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	AppliedAt      *time.Time
	ToolCallID     string
}

// Clone returns a copy with pointer fields isolated.
func (change AgentLoopWorkspaceChange) Clone() AgentLoopWorkspaceChange {
	cloned := change
	cloned.AppliedAt = CloneAgentLoopTimePtr(change.AppliedAt)
	return cloned
}

// CloneAgentLoopWorkspaceChanges clones workspace change snapshots.
func CloneAgentLoopWorkspaceChanges(changes []AgentLoopWorkspaceChange) []AgentLoopWorkspaceChange {
	if changes == nil {
		return nil
	}
	out := make([]AgentLoopWorkspaceChange, len(changes))
	for i, change := range changes {
		out[i] = change.Clone()
	}
	return out
}

// AgentLoopAuthorizationRequestSnapshot is the runtime-neutral pending or
// resolved authorization request carried by session mutation results.
type AgentLoopAuthorizationRequestSnapshot struct {
	ID             string
	ActionName     string
	Arguments      map[string]any
	RequiredLevel  string
	Policy         string
	Reason         string
	OriginalPrompt string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Clone returns a copy with argument maps isolated.
func (request AgentLoopAuthorizationRequestSnapshot) Clone() AgentLoopAuthorizationRequestSnapshot {
	cloned := request
	cloned.Arguments = CloneAgentLoopMapStringAny(request.Arguments)
	return cloned
}

// CloneAgentLoopAuthorizationRequestSnapshot clones an optional authorization
// request snapshot.
func CloneAgentLoopAuthorizationRequestSnapshot(request *AgentLoopAuthorizationRequestSnapshot) *AgentLoopAuthorizationRequestSnapshot {
	if request == nil {
		return nil
	}
	cloned := request.Clone()
	return &cloned
}

// AgentLoopApprovalStateSnapshot is the runtime-neutral canonical approval UI
// state snapshot.
type AgentLoopApprovalStateSnapshot struct {
	ApprovalKind    string
	State           string
	Decision        string
	ResumeAvailable bool
	ApprovalID      string
	RunID           string
	StepID          string
	ActionName      string
	RequiredLevel   string
	Reason          string
}

// CloneAgentLoopApprovalStateSnapshot clones an optional approval state.
func CloneAgentLoopApprovalStateSnapshot(state *AgentLoopApprovalStateSnapshot) *AgentLoopApprovalStateSnapshot {
	if state == nil {
		return nil
	}
	cloned := *state
	return &cloned
}

// AgentLoopSessionRunSummary is the runtime-neutral run summary embedded in a
// session snapshot.
type AgentLoopSessionRunSummary struct {
	RunID            string
	Status           string
	Target           string
	Findings         int
	Artifacts        int
	Events           int
	Approvals        int
	PendingApprovals int
	ReportJSON       bool
	Summary          string
	LeadFinding      string
	LeadEvidence     string
	LeadArtifact     string
}

// CloneAgentLoopSessionRunSummary clones an optional run summary.
func CloneAgentLoopSessionRunSummary(summary *AgentLoopSessionRunSummary) *AgentLoopSessionRunSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	return &cloned
}

// AgentLoopUIState is the runtime-neutral UI state embedded in a session
// snapshot.
type AgentLoopUIState struct {
	State          string
	Phase          string
	Tone           string
	Busy           bool
	RequiresAction bool
	HasError       bool
	Source         string
	Message        string
	TaskID         string
	ToolCallID     string
	ToolName       string
	ApprovalID     string
}

// CloneAgentLoopUIState clones an optional UI state.
func CloneAgentLoopUIState(state *AgentLoopUIState) *AgentLoopUIState {
	if state == nil {
		return nil
	}
	cloned := *state
	return &cloned
}

// AgentLoopMemoryCompaction is the runtime-neutral compaction artifact emitted
// by session mutation results.
type AgentLoopMemoryCompaction struct {
	Summary          *AgentLoopMemorySummary
	CompactedCount   int
	RetainedCount    int
	PreviousCount    int
	LastCompactedID  string
	LastCompactedAt  *time.Time
	Trigger          string
	TriggerThreshold int
	OccurredAt       time.Time
}

// Clone returns a copy with pointer fields isolated.
func (compaction AgentLoopMemoryCompaction) Clone() AgentLoopMemoryCompaction {
	cloned := compaction
	cloned.Summary = CloneAgentLoopMemorySummary(compaction.Summary)
	cloned.LastCompactedAt = CloneAgentLoopTimePtr(compaction.LastCompactedAt)
	return cloned
}

// CloneAgentLoopMemoryCompaction clones an optional memory compaction artifact.
func CloneAgentLoopMemoryCompaction(compaction *AgentLoopMemoryCompaction) *AgentLoopMemoryCompaction {
	if compaction == nil {
		return nil
	}
	cloned := compaction.Clone()
	return &cloned
}
