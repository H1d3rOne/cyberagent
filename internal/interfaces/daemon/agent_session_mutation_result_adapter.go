package daemon

import (
	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func daemonRecordUserMessageMutationResultPayloadFromSessionMutationResult(result runtime.SessionMutationResult) (daemonRecordUserMessageMutationResultPayload, bool) {
	if payload, ok := daemonSessionMutationResultPayload[daemonRecordUserMessageMutationResultPayload](result); ok {
		return payload, true
	}
	switch payload := result.Payload.(type) {
	case runtime.AgentLoopRecordUserMessageResultPayload:
		return daemonRecordUserMessageMutationResultPayloadFromAgentLoop(payload), true
	case *runtime.AgentLoopRecordUserMessageResultPayload:
		if payload == nil {
			return daemonRecordUserMessageMutationResultPayload{}, false
		}
		return daemonRecordUserMessageMutationResultPayloadFromAgentLoop(payload.Clone()), true
	default:
		return daemonRecordUserMessageMutationResultPayload{}, false
	}
}

func agentLoopRecordUserMessageResultPayloadFromDaemon(payload daemonRecordUserMessageMutationResultPayload) runtime.AgentLoopRecordUserMessageResultPayload {
	return runtime.AgentLoopRecordUserMessageResultPayload{
		Session:       agentLoopSessionSnapshotFromAgentSession(payload.Session),
		UserMessage:   agentLoopMessageFromAgentMessage(payload.UserMessage),
		History:       agentLoopMessagesFromAgentMessages(payload.History),
		MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(payload.MemorySummary),
		Compaction:    agentLoopMemoryCompactionFromAgentMemoryCompaction(payload.Compaction),
		Mutated:       payload.Mutated,
	}
}

func daemonRecordUserMessageMutationResultPayloadFromAgentLoop(payload runtime.AgentLoopRecordUserMessageResultPayload) daemonRecordUserMessageMutationResultPayload {
	payload = payload.Clone()
	return daemonRecordUserMessageMutationResultPayload{
		Session:       agentSessionFromAgentLoopSessionSnapshot(payload.Session),
		UserMessage:   agentMessageFromAgentLoopMessage(payload.UserMessage),
		History:       agentMessagesFromAgentLoopMessages(payload.History),
		MemorySummary: agentMemorySummaryPtrFromAgentLoopMemorySummary(payload.MemorySummary),
		Compaction:    agentMemoryCompactionFromAgentLoopMemoryCompaction(payload.Compaction),
		Mutated:       payload.Mutated,
	}
}

func daemonControlSessionMutationResultPayloadFromSessionMutationResult(result runtime.SessionMutationResult) (daemonControlSessionMutationResultPayload, bool) {
	if payload, ok := daemonSessionMutationResultPayload[daemonControlSessionMutationResultPayload](result); ok {
		return payload, true
	}
	switch payload := result.Payload.(type) {
	case runtime.AgentLoopControlSessionResultPayload:
		return daemonControlSessionMutationResultPayloadFromAgentLoop(payload), true
	case *runtime.AgentLoopControlSessionResultPayload:
		if payload == nil {
			return daemonControlSessionMutationResultPayload{}, false
		}
		return daemonControlSessionMutationResultPayloadFromAgentLoop(payload.Clone()), true
	default:
		return daemonControlSessionMutationResultPayload{}, false
	}
}

func agentLoopControlSessionResultPayloadFromDaemon(payload daemonControlSessionMutationResultPayload) runtime.AgentLoopControlSessionResultPayload {
	return runtime.AgentLoopControlSessionResultPayload{
		Session:     agentLoopSessionSnapshotFromAgentSession(payload.Session),
		Message:     agentLoopMessageFromAgentMessage(payload.Message),
		CancelJobID: payload.CancelJobID,
		Compaction:  agentLoopMemoryCompactionFromAgentMemoryCompaction(payload.Compaction),
		Mutated:     payload.Mutated,
	}
}

func daemonControlSessionMutationResultPayloadFromAgentLoop(payload runtime.AgentLoopControlSessionResultPayload) daemonControlSessionMutationResultPayload {
	payload = payload.Clone()
	return daemonControlSessionMutationResultPayload{
		Session:     agentSessionFromAgentLoopSessionSnapshot(payload.Session),
		Message:     agentMessageFromAgentLoopMessage(payload.Message),
		CancelJobID: payload.CancelJobID,
		Compaction:  agentMemoryCompactionFromAgentLoopMemoryCompaction(payload.Compaction),
		Mutated:     payload.Mutated,
	}
}

func daemonResolveAuthorizationMutationResultPayloadFromSessionMutationResult(result runtime.SessionMutationResult) (daemonResolveAuthorizationMutationResultPayload, bool) {
	if payload, ok := daemonSessionMutationResultPayload[daemonResolveAuthorizationMutationResultPayload](result); ok {
		return payload, true
	}
	switch payload := result.Payload.(type) {
	case runtime.AgentLoopResolveAuthorizationResultPayload:
		return daemonResolveAuthorizationMutationResultPayloadFromAgentLoop(payload), true
	case *runtime.AgentLoopResolveAuthorizationResultPayload:
		if payload == nil {
			return daemonResolveAuthorizationMutationResultPayload{}, false
		}
		return daemonResolveAuthorizationMutationResultPayloadFromAgentLoop(payload.Clone()), true
	default:
		return daemonResolveAuthorizationMutationResultPayload{}, false
	}
}

func agentLoopResolveAuthorizationResultPayloadFromDaemon(payload daemonResolveAuthorizationMutationResultPayload) runtime.AgentLoopResolveAuthorizationResultPayload {
	pending := agentLoopAuthorizationRequestSnapshotFromAgentAuthorizationRequest(&payload.Pending)
	result := runtime.AgentLoopResolveAuthorizationResultPayload{
		Session:        agentLoopSessionSnapshotFromAgentSession(payload.Session),
		Message:        agentLoopMessageFromAgentMessage(payload.Message),
		OperationLevel: payload.OperationLevel,
		Compaction:     agentLoopMemoryCompactionFromAgentMemoryCompaction(payload.Compaction),
		Mutated:        payload.Mutated,
	}
	if pending != nil {
		result.Pending = *pending
	}
	return result
}

func daemonResolveAuthorizationMutationResultPayloadFromAgentLoop(payload runtime.AgentLoopResolveAuthorizationResultPayload) daemonResolveAuthorizationMutationResultPayload {
	payload = payload.Clone()
	pending := agentAuthorizationRequestFromAgentLoopAuthorizationRequestSnapshot(&payload.Pending)
	result := daemonResolveAuthorizationMutationResultPayload{
		Session:        agentSessionFromAgentLoopSessionSnapshot(payload.Session),
		Message:        agentMessageFromAgentLoopMessage(payload.Message),
		OperationLevel: payload.OperationLevel,
		Compaction:     agentMemoryCompactionFromAgentLoopMemoryCompaction(payload.Compaction),
		Mutated:        payload.Mutated,
	}
	if pending != nil {
		result.Pending = *pending
	}
	return result
}

func daemonRequestAuthorizationMutationResultPayloadFromSessionMutationResult(result runtime.SessionMutationResult) (daemonRequestAuthorizationMutationResultPayload, bool) {
	if payload, ok := daemonSessionMutationResultPayload[daemonRequestAuthorizationMutationResultPayload](result); ok {
		return payload, true
	}
	switch payload := result.Payload.(type) {
	case runtime.AgentLoopRequestAuthorizationResultPayload:
		return daemonRequestAuthorizationMutationResultPayloadFromAgentLoop(payload), true
	case *runtime.AgentLoopRequestAuthorizationResultPayload:
		if payload == nil {
			return daemonRequestAuthorizationMutationResultPayload{}, false
		}
		return daemonRequestAuthorizationMutationResultPayloadFromAgentLoop(payload.Clone()), true
	default:
		return daemonRequestAuthorizationMutationResultPayload{}, false
	}
}

func agentLoopRequestAuthorizationResultPayloadFromDaemon(payload daemonRequestAuthorizationMutationResultPayload) runtime.AgentLoopRequestAuthorizationResultPayload {
	authorization := agentLoopAuthorizationRequestSnapshotFromAgentAuthorizationRequest(&payload.Authorization)
	result := runtime.AgentLoopRequestAuthorizationResultPayload{
		Session: agentLoopSessionSnapshotFromAgentSession(payload.Session),
		Message: agentLoopMessageFromAgentMessage(payload.Message),
		Mutated: payload.Mutated,
	}
	if authorization != nil {
		result.Authorization = *authorization
	}
	return result
}

func daemonRequestAuthorizationMutationResultPayloadFromAgentLoop(payload runtime.AgentLoopRequestAuthorizationResultPayload) daemonRequestAuthorizationMutationResultPayload {
	payload = payload.Clone()
	authorization := agentAuthorizationRequestFromAgentLoopAuthorizationRequestSnapshot(&payload.Authorization)
	result := daemonRequestAuthorizationMutationResultPayload{
		Session: agentSessionFromAgentLoopSessionSnapshot(payload.Session),
		Message: agentMessageFromAgentLoopMessage(payload.Message),
		Mutated: payload.Mutated,
	}
	if authorization != nil {
		result.Authorization = *authorization
	}
	return result
}

func agentLoopSessionSnapshotFromAgentSession(session agentSession) runtime.AgentLoopSessionSnapshot {
	return runtime.AgentLoopSessionSnapshot{
		SessionID:            session.SessionID,
		CreatedAt:            session.CreatedAt,
		UpdatedAt:            session.UpdatedAt,
		Operator:             session.Operator,
		Language:             session.Language,
		OperationLevel:       session.OperationLevel,
		Status:               session.Status,
		Messages:             agentLoopMessagesFromAgentMessages(session.Messages),
		ActiveTask:           agentLoopSessionTaskFromAgentTask(session.ActiveTask),
		Todos:                agentLoopSessionTodosFromAgentTodos(session.Todos),
		ToolCalls:            agentLoopToolCallItemsFromAgentToolCalls(session.ToolCalls),
		ContextRefs:          agentLoopContextRefsFromAgentContextRefs(session.ContextRefs),
		PendingChanges:       agentLoopWorkspaceChangesFromAgentWorkspaceChanges(session.PendingChanges),
		PendingAuthorization: agentLoopAuthorizationRequestSnapshotFromAgentAuthorizationRequest(session.PendingAuthorization),
		ApprovalState:        agentLoopApprovalStateSnapshotFromApprovalCanonicalState(session.ApprovalState),
		MemorySummary:        agentLoopMemorySummaryPtrFromAgentMemorySummary(session.MemorySummary),
		NextAction:           agentLoopNextActionFromAgentNextAction(session.NextAction),
		RunSummary:           agentLoopSessionRunSummaryFromAgentSessionRunSummary(session.RunSummary),
		UIState:              agentLoopUIStateFromAgentUIState(session.UIState),
	}
}

func agentSessionFromAgentLoopSessionSnapshot(snapshot runtime.AgentLoopSessionSnapshot) agentSession {
	snapshot = snapshot.Clone()
	return agentSession{
		SessionID:            snapshot.SessionID,
		CreatedAt:            snapshot.CreatedAt,
		UpdatedAt:            snapshot.UpdatedAt,
		Operator:             snapshot.Operator,
		Language:             snapshot.Language,
		OperationLevel:       snapshot.OperationLevel,
		Status:               snapshot.Status,
		Messages:             agentMessagesFromAgentLoopMessages(snapshot.Messages),
		ActiveTask:           agentTaskFromAgentLoopSessionTask(snapshot.ActiveTask),
		Todos:                agentTodosFromAgentLoopSessionTodos(snapshot.Todos),
		ToolCalls:            agentLoopToolCallItemsToAgentToolCalls(snapshot.ToolCalls),
		ContextRefs:          agentContextRefsFromAgentLoopContextRefs(snapshot.ContextRefs),
		PendingChanges:       agentWorkspaceChangesFromAgentLoopWorkspaceChanges(snapshot.PendingChanges),
		PendingAuthorization: agentAuthorizationRequestFromAgentLoopAuthorizationRequestSnapshot(snapshot.PendingAuthorization),
		ApprovalState:        approvalCanonicalStateFromAgentLoopApprovalStateSnapshot(snapshot.ApprovalState),
		MemorySummary:        agentMemorySummaryPtrFromAgentLoopMemorySummary(snapshot.MemorySummary),
		NextAction:           agentNextActionFromAgentLoopNextAction(snapshot.NextAction),
		RunSummary:           agentSessionRunSummaryFromAgentLoopSessionRunSummary(snapshot.RunSummary),
		UIState:              agentUIStateFromAgentLoopUIState(snapshot.UIState),
	}
}

func agentLoopSessionTaskFromAgentTask(task *agentTask) *runtime.AgentLoopSessionTask {
	if task == nil {
		return nil
	}
	return &runtime.AgentLoopSessionTask{
		ID:          task.ID,
		Goal:        task.Goal,
		Kind:        task.Kind,
		Status:      task.Status,
		ActiveRunID: task.ActiveRunID,
		Summary:     task.Summary,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
}

func agentTaskFromAgentLoopSessionTask(task *runtime.AgentLoopSessionTask) *agentTask {
	if task == nil {
		return nil
	}
	return &agentTask{
		ID:          task.ID,
		Goal:        task.Goal,
		Kind:        task.Kind,
		Status:      task.Status,
		ActiveRunID: task.ActiveRunID,
		Summary:     task.Summary,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
}

func agentLoopSessionTodosFromAgentTodos(todos []agentTodo) []runtime.AgentLoopSessionTodo {
	if todos == nil {
		return nil
	}
	out := make([]runtime.AgentLoopSessionTodo, len(todos))
	for i, todo := range todos {
		out[i] = runtime.AgentLoopSessionTodo{
			ID:            todo.ID,
			Title:         todo.Title,
			Detail:        todo.Detail,
			Status:        todo.Status,
			Priority:      todo.Priority,
			ToolCallID:    todo.ToolCallID,
			ContextRefIDs: runtime.CloneAgentLoopStringSlice(todo.ContextRefIDs),
			BlockedReason: todo.BlockedReason,
			UpdatedAt:     todo.UpdatedAt,
		}
	}
	return out
}

func agentTodosFromAgentLoopSessionTodos(todos []runtime.AgentLoopSessionTodo) []agentTodo {
	if todos == nil {
		return nil
	}
	out := make([]agentTodo, len(todos))
	for i, todo := range todos {
		out[i] = agentTodo{
			ID:            todo.ID,
			Title:         todo.Title,
			Detail:        todo.Detail,
			Status:        todo.Status,
			Priority:      todo.Priority,
			ToolCallID:    todo.ToolCallID,
			ContextRefIDs: runtime.CloneAgentLoopStringSlice(todo.ContextRefIDs),
			BlockedReason: todo.BlockedReason,
			UpdatedAt:     todo.UpdatedAt,
		}
	}
	return out
}

func agentLoopContextRefsFromAgentContextRefs(refs []agentContextRef) []runtime.AgentLoopContextRef {
	if refs == nil {
		return nil
	}
	out := make([]runtime.AgentLoopContextRef, len(refs))
	for i, ref := range refs {
		out[i] = runtime.AgentLoopContextRef{
			ID:      ref.ID,
			Kind:    ref.Kind,
			RefID:   ref.RefID,
			Label:   ref.Label,
			To:      ref.To,
			Summary: ref.Summary,
		}
	}
	return out
}

func agentContextRefsFromAgentLoopContextRefs(refs []runtime.AgentLoopContextRef) []agentContextRef {
	if refs == nil {
		return nil
	}
	out := make([]agentContextRef, len(refs))
	for i, ref := range refs {
		out[i] = agentContextRef{
			ID:      ref.ID,
			Kind:    ref.Kind,
			RefID:   ref.RefID,
			Label:   ref.Label,
			To:      ref.To,
			Summary: ref.Summary,
		}
	}
	return out
}

func agentLoopWorkspaceChangesFromAgentWorkspaceChanges(changes []agentWorkspaceChange) []runtime.AgentLoopWorkspaceChange {
	if changes == nil {
		return nil
	}
	out := make([]runtime.AgentLoopWorkspaceChange, len(changes))
	for i, change := range changes {
		out[i] = runtime.AgentLoopWorkspaceChange{
			ID:             change.ID,
			GroupID:        change.GroupID,
			Path:           change.Path,
			OldText:        change.OldText,
			NewText:        change.NewText,
			SizeBefore:     change.SizeBefore,
			SizeAfter:      change.SizeAfter,
			Replacements:   change.Replacements,
			DiffPreview:    change.DiffPreview,
			ContentPreview: change.ContentPreview,
			Status:         change.Status,
			Error:          change.Error,
			CreatedAt:      change.CreatedAt,
			UpdatedAt:      change.UpdatedAt,
			AppliedAt:      runtime.CloneAgentLoopTimePtr(change.AppliedAt),
			ToolCallID:     change.ToolCallID,
		}
	}
	return out
}

func agentWorkspaceChangesFromAgentLoopWorkspaceChanges(changes []runtime.AgentLoopWorkspaceChange) []agentWorkspaceChange {
	if changes == nil {
		return nil
	}
	out := make([]agentWorkspaceChange, len(changes))
	for i, change := range changes {
		out[i] = agentWorkspaceChange{
			ID:             change.ID,
			GroupID:        change.GroupID,
			Path:           change.Path,
			OldText:        change.OldText,
			NewText:        change.NewText,
			SizeBefore:     change.SizeBefore,
			SizeAfter:      change.SizeAfter,
			Replacements:   change.Replacements,
			DiffPreview:    change.DiffPreview,
			ContentPreview: change.ContentPreview,
			Status:         change.Status,
			Error:          change.Error,
			CreatedAt:      change.CreatedAt,
			UpdatedAt:      change.UpdatedAt,
			AppliedAt:      runtime.CloneAgentLoopTimePtr(change.AppliedAt),
			ToolCallID:     change.ToolCallID,
		}
	}
	return out
}

func agentLoopAuthorizationRequestSnapshotFromAgentAuthorizationRequest(request *agentAuthorizationRequest) *runtime.AgentLoopAuthorizationRequestSnapshot {
	if request == nil {
		return nil
	}
	return &runtime.AgentLoopAuthorizationRequestSnapshot{
		ID:             request.ID,
		ActionName:     request.ActionName,
		Arguments:      runtime.CloneAgentLoopMapStringAny(request.Arguments),
		RequiredLevel:  request.RequiredLevel,
		Policy:         request.Policy,
		Reason:         request.Reason,
		OriginalPrompt: request.OriginalPrompt,
		Status:         request.Status,
		CreatedAt:      request.CreatedAt,
		UpdatedAt:      request.UpdatedAt,
	}
}

func agentAuthorizationRequestFromAgentLoopAuthorizationRequestSnapshot(request *runtime.AgentLoopAuthorizationRequestSnapshot) *agentAuthorizationRequest {
	if request == nil {
		return nil
	}
	return &agentAuthorizationRequest{
		ID:             request.ID,
		ActionName:     request.ActionName,
		Arguments:      runtime.CloneAgentLoopMapStringAny(request.Arguments),
		RequiredLevel:  request.RequiredLevel,
		Policy:         request.Policy,
		Reason:         request.Reason,
		OriginalPrompt: request.OriginalPrompt,
		Status:         request.Status,
		CreatedAt:      request.CreatedAt,
		UpdatedAt:      request.UpdatedAt,
	}
}

func agentLoopApprovalStateSnapshotFromApprovalCanonicalState(state *approvalCanonicalState) *runtime.AgentLoopApprovalStateSnapshot {
	if state == nil {
		return nil
	}
	return &runtime.AgentLoopApprovalStateSnapshot{
		ApprovalKind:    state.ApprovalKind,
		State:           state.State,
		Decision:        state.Decision,
		ResumeAvailable: state.ResumeAvailable,
		ApprovalID:      state.ApprovalID,
		RunID:           state.RunID,
		StepID:          state.StepID,
		ActionName:      state.ActionName,
		RequiredLevel:   state.RequiredLevel,
		Reason:          state.Reason,
	}
}

func approvalCanonicalStateFromAgentLoopApprovalStateSnapshot(state *runtime.AgentLoopApprovalStateSnapshot) *approvalCanonicalState {
	if state == nil {
		return nil
	}
	return &approvalCanonicalState{
		ApprovalKind:    state.ApprovalKind,
		State:           state.State,
		Decision:        state.Decision,
		ResumeAvailable: state.ResumeAvailable,
		ApprovalID:      state.ApprovalID,
		RunID:           state.RunID,
		StepID:          state.StepID,
		ActionName:      state.ActionName,
		RequiredLevel:   state.RequiredLevel,
		Reason:          state.Reason,
	}
}

func agentLoopNextActionFromAgentNextAction(action *agentNextAction) *runtime.AgentLoopNextAction {
	if action == nil {
		return nil
	}
	return &runtime.AgentLoopNextAction{
		Type:            action.Type,
		Reason:          action.Reason,
		SuggestedPrompt: action.SuggestedPrompt,
		AutoContinue:    action.AutoContinue,
	}
}

func agentNextActionFromAgentLoopNextAction(action *runtime.AgentLoopNextAction) *agentNextAction {
	if action == nil {
		return nil
	}
	return &agentNextAction{
		Type:            action.Type,
		Reason:          action.Reason,
		SuggestedPrompt: action.SuggestedPrompt,
		AutoContinue:    action.AutoContinue,
	}
}

func agentLoopSessionRunSummaryFromAgentSessionRunSummary(summary *agentSessionRunSummary) *runtime.AgentLoopSessionRunSummary {
	if summary == nil {
		return nil
	}
	return &runtime.AgentLoopSessionRunSummary{
		RunID:            summary.RunID,
		Status:           string(summary.Status),
		Target:           summary.Target,
		Findings:         summary.Findings,
		Artifacts:        summary.Artifacts,
		Events:           summary.Events,
		Approvals:        summary.Approvals,
		PendingApprovals: summary.PendingApprovals,
		ReportJSON:       summary.ReportJSON,
		Summary:          summary.Summary,
		LeadFinding:      summary.LeadFinding,
		LeadEvidence:     summary.LeadEvidence,
		LeadArtifact:     summary.LeadArtifact,
	}
}

func agentSessionRunSummaryFromAgentLoopSessionRunSummary(summary *runtime.AgentLoopSessionRunSummary) *agentSessionRunSummary {
	if summary == nil {
		return nil
	}
	return &agentSessionRunSummary{
		RunID:            summary.RunID,
		Status:           contracts.Status(summary.Status),
		Target:           summary.Target,
		Findings:         summary.Findings,
		Artifacts:        summary.Artifacts,
		Events:           summary.Events,
		Approvals:        summary.Approvals,
		PendingApprovals: summary.PendingApprovals,
		ReportJSON:       summary.ReportJSON,
		Summary:          summary.Summary,
		LeadFinding:      summary.LeadFinding,
		LeadEvidence:     summary.LeadEvidence,
		LeadArtifact:     summary.LeadArtifact,
	}
}

func agentLoopUIStateFromAgentUIState(state *agentUIState) *runtime.AgentLoopUIState {
	if state == nil {
		return nil
	}
	return &runtime.AgentLoopUIState{
		State:          state.State,
		Phase:          state.Phase,
		Tone:           state.Tone,
		Busy:           state.Busy,
		RequiresAction: state.RequiresAction,
		HasError:       state.HasError,
		Source:         state.Source,
		Message:        state.Message,
		TaskID:         state.TaskID,
		ToolCallID:     state.ToolCallID,
		ToolName:       state.ToolName,
		ApprovalID:     state.ApprovalID,
	}
}

func agentUIStateFromAgentLoopUIState(state *runtime.AgentLoopUIState) *agentUIState {
	if state == nil {
		return nil
	}
	return &agentUIState{
		State:          state.State,
		Phase:          state.Phase,
		Tone:           state.Tone,
		Busy:           state.Busy,
		RequiresAction: state.RequiresAction,
		HasError:       state.HasError,
		Source:         state.Source,
		Message:        state.Message,
		TaskID:         state.TaskID,
		ToolCallID:     state.ToolCallID,
		ToolName:       state.ToolName,
		ApprovalID:     state.ApprovalID,
	}
}

func agentLoopMemoryCompactionFromAgentMemoryCompaction(compaction *agentMemoryCompaction) *runtime.AgentLoopMemoryCompaction {
	if compaction == nil {
		return nil
	}
	return &runtime.AgentLoopMemoryCompaction{
		Summary:          agentLoopMemorySummaryPtrFromAgentMemorySummary(compaction.Summary),
		CompactedCount:   compaction.CompactedCount,
		RetainedCount:    compaction.RetainedCount,
		PreviousCount:    compaction.PreviousCount,
		LastCompactedID:  compaction.LastCompactedID,
		LastCompactedAt:  runtime.CloneAgentLoopTimePtr(compaction.LastCompactedAt),
		Trigger:          compaction.Trigger,
		TriggerThreshold: compaction.TriggerThreshold,
		OccurredAt:       compaction.OccurredAt,
	}
}

func agentMemoryCompactionFromAgentLoopMemoryCompaction(compaction *runtime.AgentLoopMemoryCompaction) *agentMemoryCompaction {
	if compaction == nil {
		return nil
	}
	return &agentMemoryCompaction{
		Summary:          agentMemorySummaryPtrFromAgentLoopMemorySummary(compaction.Summary),
		CompactedCount:   compaction.CompactedCount,
		RetainedCount:    compaction.RetainedCount,
		PreviousCount:    compaction.PreviousCount,
		LastCompactedID:  compaction.LastCompactedID,
		LastCompactedAt:  runtime.CloneAgentLoopTimePtr(compaction.LastCompactedAt),
		Trigger:          compaction.Trigger,
		TriggerThreshold: compaction.TriggerThreshold,
		OccurredAt:       compaction.OccurredAt,
	}
}
