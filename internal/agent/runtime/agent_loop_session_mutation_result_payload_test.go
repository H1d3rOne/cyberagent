package runtime

import (
	"testing"
	"time"
)

func TestAgentLoopRecordUserMessageResultPayloadCloneIsolatesMutableFields(t *testing.T) {
	lastMessageAt := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	lastCompactedAt := time.Date(2026, 6, 8, 9, 1, 0, 0, time.UTC)
	originalLastCompactedAt := lastCompactedAt
	appliedAt := time.Date(2026, 6, 8, 9, 2, 0, 0, time.UTC)
	originalAppliedAt := appliedAt

	payload := AgentLoopRecordUserMessageResultPayload{
		Session: AgentLoopSessionSnapshot{
			SessionID:      "session-result",
			Operator:       "operator-1",
			Language:       "en",
			OperationLevel: "safe_scan",
			Status:         "running",
			Messages: []AgentLoopMessage{{
				ID:      "msg-session",
				Role:    "user",
				Content: "scan safely",
				ToolCalls: []AgentLoopToolCall{{
					ID:       "tool-session",
					Name:     "daemon.health",
					Status:   "succeeded",
					Result:   map[string]any{"source": "session"},
					Metadata: map[string]any{"phase": "history"},
				}},
				SuggestedPrompts: []string{"continue"},
			}},
			ActiveTask: &AgentLoopSessionTask{
				ID:     "task-result",
				Goal:   "inspect target",
				Status: "running",
			},
			Todos: []AgentLoopSessionTodo{{
				ID:            "todo-result",
				Title:         "collect evidence",
				ContextRefIDs: []string{"ctx-1"},
			}},
			ToolCalls: []AgentLoopToolCall{{
				ID:     "tool-top",
				Name:   "workspace.files.list",
				Result: map[string]any{"path": "."},
			}},
			ContextRefs: []AgentLoopContextRef{{
				ID:    "ctx-1",
				Kind:  "file",
				RefID: "notes.md",
			}},
			PendingChanges: []AgentLoopWorkspaceChange{{
				ID:        "change-1",
				Path:      "notes.md",
				AppliedAt: &appliedAt,
			}},
			PendingAuthorization: &AgentLoopAuthorizationRequestSnapshot{
				ID:         "auth-1",
				ActionName: "workspace.files.write",
				Arguments:  map[string]any{"path": "notes.md"},
				Status:     "pending",
			},
			ApprovalState: &AgentLoopApprovalStateSnapshot{
				ApprovalKind: "agent_authorization",
				State:        "pending",
				ApprovalID:   "auth-1",
			},
			MemorySummary: &AgentLoopMemorySummary{
				Summary:       "session memory",
				MessageCount:  3,
				LastMessageID: "msg-session",
				LastMessageAt: &lastMessageAt,
			},
			NextAction: &AgentLoopNextAction{
				Type:            "continue",
				SuggestedPrompt: "continue safely",
			},
			RunSummary: &AgentLoopSessionRunSummary{
				RunID:    "run-1",
				Status:   "succeeded",
				Findings: 1,
			},
			UIState: &AgentLoopUIState{
				State: "running",
				Busy:  true,
			},
		},
		UserMessage: AgentLoopMessage{
			ID:               "msg-user",
			Role:             "user",
			Content:          "scan safely",
			SuggestedPrompts: []string{"revise"},
		},
		History: []AgentLoopMessage{{
			ID:      "msg-history",
			Role:    "agent",
			Content: "history",
		}},
		MemorySummary: &AgentLoopMemorySummary{
			Summary:       "result memory",
			MessageCount:  4,
			LastMessageID: "msg-user",
			LastMessageAt: &lastMessageAt,
		},
		Compaction: &AgentLoopMemoryCompaction{
			Summary: &AgentLoopMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
			},
			CompactedCount:  2,
			LastCompactedID: "msg-old",
			LastCompactedAt: &lastCompactedAt,
			Trigger:         "turn_count",
		},
		Mutated: true,
	}

	cloned := payload.Clone()

	payload.Session.Messages[0].Content = "mutated session"
	payload.Session.Messages[0].ToolCalls[0].Result["source"] = "mutated"
	payload.Session.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated"
	payload.Session.Messages[0].SuggestedPrompts[0] = "mutated"
	payload.Session.ActiveTask.Goal = "mutated"
	payload.Session.Todos[0].ContextRefIDs[0] = "mutated"
	payload.Session.ToolCalls[0].Result["path"] = "mutated"
	payload.Session.ContextRefs[0].RefID = "mutated"
	*payload.Session.PendingChanges[0].AppliedAt = appliedAt.Add(time.Hour)
	payload.Session.PendingAuthorization.Arguments["path"] = "mutated"
	payload.Session.ApprovalState.State = "mutated"
	*payload.Session.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)
	payload.Session.NextAction.SuggestedPrompt = "mutated"
	payload.Session.RunSummary.Status = "mutated"
	payload.Session.UIState.State = "mutated"
	payload.UserMessage.SuggestedPrompts[0] = "mutated"
	payload.History[0].Content = "mutated"
	payload.MemorySummary.Summary = "mutated"
	*payload.MemorySummary.LastMessageAt = lastMessageAt.Add(2 * time.Hour)
	payload.Compaction.Summary.Summary = "mutated"
	*payload.Compaction.Summary.LastMessageAt = lastMessageAt.Add(3 * time.Hour)
	*payload.Compaction.LastCompactedAt = lastCompactedAt.Add(time.Hour)

	if cloned.Session.Messages[0].Content != "scan safely" ||
		cloned.Session.Messages[0].ToolCalls[0].Result["source"] != "session" ||
		cloned.Session.Messages[0].ToolCalls[0].Metadata["phase"] != "history" ||
		cloned.Session.Messages[0].SuggestedPrompts[0] != "continue" {
		t.Fatalf("clone shares session messages: %#v", cloned.Session.Messages[0])
	}
	if cloned.Session.ActiveTask == payload.Session.ActiveTask || cloned.Session.ActiveTask.Goal != "inspect target" {
		t.Fatalf("clone shares active task: %#v", cloned.Session.ActiveTask)
	}
	if cloned.Session.Todos[0].ContextRefIDs[0] != "ctx-1" {
		t.Fatalf("clone shares todo context refs: %#v", cloned.Session.Todos[0])
	}
	if cloned.Session.ToolCalls[0].Result["path"] != "." {
		t.Fatalf("clone shares top-level tool calls: %#v", cloned.Session.ToolCalls[0])
	}
	if cloned.Session.ContextRefs[0].RefID != "notes.md" {
		t.Fatalf("clone shares context refs: %#v", cloned.Session.ContextRefs[0])
	}
	if cloned.Session.PendingChanges[0].AppliedAt == payload.Session.PendingChanges[0].AppliedAt ||
		cloned.Session.PendingChanges[0].AppliedAt == nil ||
		!cloned.Session.PendingChanges[0].AppliedAt.Equal(originalAppliedAt) {
		t.Fatalf("clone shares pending change AppliedAt: %#v", cloned.Session.PendingChanges[0].AppliedAt)
	}
	if cloned.Session.PendingAuthorization == payload.Session.PendingAuthorization ||
		cloned.Session.PendingAuthorization.Arguments["path"] != "notes.md" {
		t.Fatalf("clone shares pending authorization: %#v", cloned.Session.PendingAuthorization)
	}
	if cloned.Session.ApprovalState == payload.Session.ApprovalState || cloned.Session.ApprovalState.State != "pending" {
		t.Fatalf("clone shares approval state: %#v", cloned.Session.ApprovalState)
	}
	if cloned.Session.MemorySummary == payload.Session.MemorySummary ||
		cloned.Session.MemorySummary.LastMessageAt == payload.Session.MemorySummary.LastMessageAt ||
		cloned.Session.MemorySummary.LastMessageAt == nil ||
		!cloned.Session.MemorySummary.LastMessageAt.Equal(originalLastMessageAt) {
		t.Fatalf("clone shares session memory summary: %#v", cloned.Session.MemorySummary)
	}
	if cloned.Session.NextAction == payload.Session.NextAction || cloned.Session.NextAction.SuggestedPrompt != "continue safely" {
		t.Fatalf("clone shares next action: %#v", cloned.Session.NextAction)
	}
	if cloned.Session.RunSummary == payload.Session.RunSummary || cloned.Session.RunSummary.Status != "succeeded" {
		t.Fatalf("clone shares run summary: %#v", cloned.Session.RunSummary)
	}
	if cloned.Session.UIState == payload.Session.UIState || cloned.Session.UIState.State != "running" {
		t.Fatalf("clone shares UI state: %#v", cloned.Session.UIState)
	}
	if cloned.UserMessage.SuggestedPrompts[0] != "revise" || cloned.History[0].Content != "history" {
		t.Fatalf("clone shares top-level messages: user=%#v history=%#v", cloned.UserMessage, cloned.History)
	}
	if cloned.MemorySummary == payload.MemorySummary ||
		cloned.MemorySummary.LastMessageAt == payload.MemorySummary.LastMessageAt ||
		cloned.MemorySummary.LastMessageAt == nil ||
		!cloned.MemorySummary.LastMessageAt.Equal(originalLastMessageAt) {
		t.Fatalf("clone shares result memory summary: %#v", cloned.MemorySummary)
	}
	if cloned.Compaction == payload.Compaction ||
		cloned.Compaction.Summary == payload.Compaction.Summary ||
		cloned.Compaction.Summary.LastMessageAt == payload.Compaction.Summary.LastMessageAt ||
		cloned.Compaction.Summary.LastMessageAt == nil ||
		!cloned.Compaction.Summary.LastMessageAt.Equal(originalLastMessageAt) ||
		cloned.Compaction.LastCompactedAt == payload.Compaction.LastCompactedAt ||
		cloned.Compaction.LastCompactedAt == nil ||
		!cloned.Compaction.LastCompactedAt.Equal(originalLastCompactedAt) {
		t.Fatalf("clone shares compaction: %#v", cloned.Compaction)
	}
	if !cloned.Mutated || cloned.Session.SessionID != "session-result" {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
}

func TestAgentLoopControlSessionResultPayloadCloneIsolatesMutableFields(t *testing.T) {
	lastMessageAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	lastCompactedAt := time.Date(2026, 6, 8, 10, 1, 0, 0, time.UTC)
	originalLastCompactedAt := lastCompactedAt

	payload := AgentLoopControlSessionResultPayload{
		Session: AgentLoopSessionSnapshot{
			SessionID: "session-control-result",
			Status:    "paused",
			Messages: []AgentLoopMessage{{
				ID:      "msg-session-control",
				Role:    "agent",
				Content: "paused",
				ToolCalls: []AgentLoopToolCall{{
					ID:       "tool-session-control",
					Name:     "daemon.control",
					Result:   map[string]any{"status": "paused"},
					Metadata: map[string]any{"phase": "control"},
				}},
				SuggestedPrompts: []string{"continue"},
			}},
			NextAction: &AgentLoopNextAction{
				Type:            "continue",
				Reason:          "operator pause",
				SuggestedPrompt: "resume later",
			},
			MemorySummary: &AgentLoopMemorySummary{
				Summary:       "session memory",
				MessageCount:  3,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
			},
		},
		Message: AgentLoopMessage{
			ID:      "msg-control",
			Role:    "agent",
			Content: "control message",
			ToolCalls: []AgentLoopToolCall{{
				ID:     "tool-control",
				Name:   "daemon.pause",
				Result: map[string]any{"action": "pause"},
			}},
			SuggestedPrompts: []string{"resume"},
		},
		CancelJobID: "job-cancel",
		Compaction: &AgentLoopMemoryCompaction{
			Summary: &AgentLoopMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
			},
			LastCompactedID: "msg-old",
			LastCompactedAt: &lastCompactedAt,
			Trigger:         "turn_count",
		},
		Mutated: true,
	}

	cloned := payload.Clone()

	payload.Session.Messages[0].Content = "mutated session"
	payload.Session.Messages[0].ToolCalls[0].Result["status"] = "mutated"
	payload.Session.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated"
	payload.Session.Messages[0].SuggestedPrompts[0] = "mutated"
	payload.Session.NextAction.SuggestedPrompt = "mutated"
	*payload.Session.MemorySummary.LastMessageAt = lastMessageAt.Add(time.Hour)
	payload.Message.Content = "mutated message"
	payload.Message.ToolCalls[0].Result["action"] = "mutated"
	payload.Message.SuggestedPrompts[0] = "mutated"
	payload.Compaction.Summary.Summary = "mutated"
	*payload.Compaction.Summary.LastMessageAt = lastMessageAt.Add(2 * time.Hour)
	*payload.Compaction.LastCompactedAt = lastCompactedAt.Add(time.Hour)

	if cloned.Session.Messages[0].Content != "paused" ||
		cloned.Session.Messages[0].ToolCalls[0].Result["status"] != "paused" ||
		cloned.Session.Messages[0].ToolCalls[0].Metadata["phase"] != "control" ||
		cloned.Session.Messages[0].SuggestedPrompts[0] != "continue" {
		t.Fatalf("clone shares session messages: %#v", cloned.Session.Messages[0])
	}
	if cloned.Session.NextAction == payload.Session.NextAction || cloned.Session.NextAction.SuggestedPrompt != "resume later" {
		t.Fatalf("clone shares next action: %#v", cloned.Session.NextAction)
	}
	if cloned.Session.MemorySummary == payload.Session.MemorySummary ||
		cloned.Session.MemorySummary.LastMessageAt == payload.Session.MemorySummary.LastMessageAt ||
		cloned.Session.MemorySummary.LastMessageAt == nil ||
		!cloned.Session.MemorySummary.LastMessageAt.Equal(originalLastMessageAt) {
		t.Fatalf("clone shares session memory summary: %#v", cloned.Session.MemorySummary)
	}
	if cloned.Message.Content != "control message" ||
		cloned.Message.ToolCalls[0].Result["action"] != "pause" ||
		cloned.Message.SuggestedPrompts[0] != "resume" {
		t.Fatalf("clone shares control message: %#v", cloned.Message)
	}
	if cloned.Compaction == payload.Compaction ||
		cloned.Compaction.Summary == payload.Compaction.Summary ||
		cloned.Compaction.Summary.LastMessageAt == payload.Compaction.Summary.LastMessageAt ||
		cloned.Compaction.Summary.LastMessageAt == nil ||
		!cloned.Compaction.Summary.LastMessageAt.Equal(originalLastMessageAt) ||
		cloned.Compaction.LastCompactedAt == payload.Compaction.LastCompactedAt ||
		cloned.Compaction.LastCompactedAt == nil ||
		!cloned.Compaction.LastCompactedAt.Equal(originalLastCompactedAt) {
		t.Fatalf("clone shares compaction: %#v", cloned.Compaction)
	}
	if cloned.CancelJobID != "job-cancel" || !cloned.Mutated || cloned.Session.SessionID != "session-control-result" {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
}

func TestAgentLoopResolveAuthorizationResultPayloadCloneIsolatesMutableFields(t *testing.T) {
	lastMessageAt := time.Date(2026, 6, 8, 11, 0, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	lastCompactedAt := time.Date(2026, 6, 8, 11, 1, 0, 0, time.UTC)
	originalLastCompactedAt := lastCompactedAt

	payload := AgentLoopResolveAuthorizationResultPayload{
		Session: AgentLoopSessionSnapshot{
			SessionID: "session-auth-result",
			Status:    "blocked",
			Messages: []AgentLoopMessage{{
				ID:      "msg-session-auth",
				Role:    "agent",
				Content: "authorization denied",
				ToolCalls: []AgentLoopToolCall{{
					ID:       "tool-session-auth",
					Name:     "approval.resolved",
					Result:   map[string]any{"decision": "deny"},
					Metadata: map[string]any{"phase": "approval"},
				}},
			}},
			NextAction: &AgentLoopNextAction{
				Type:   "blocked",
				Reason: "not within scope",
			},
		},
		Pending: AgentLoopAuthorizationRequestSnapshot{
			ID:            "auth-result",
			ActionName:    "workspace.files.write",
			Arguments:     map[string]any{"path": "README.md"},
			RequiredLevel: "workspace_write",
			Status:        "denied",
		},
		Message: AgentLoopMessage{
			ID:      "msg-auth",
			Role:    "agent",
			Content: "Authorization denied",
			ToolCalls: []AgentLoopToolCall{{
				ID:     "tool-auth",
				Name:   "approval.resolved",
				Result: map[string]any{"authorization_id": "auth-result"},
			}},
			SuggestedPrompts: []string{"revise scope"},
		},
		OperationLevel: "safe_scan",
		Compaction: &AgentLoopMemoryCompaction{
			Summary: &AgentLoopMemorySummary{
				Summary:       "compacted memory",
				MessageCount:  2,
				LastMessageID: "msg-old",
				LastMessageAt: &lastMessageAt,
			},
			LastCompactedID: "msg-old",
			LastCompactedAt: &lastCompactedAt,
			Trigger:         "turn_count",
		},
		Mutated: true,
	}

	cloned := payload.Clone()

	payload.Session.Messages[0].Content = "mutated session"
	payload.Session.Messages[0].ToolCalls[0].Result["decision"] = "mutated"
	payload.Session.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated"
	payload.Session.NextAction.Reason = "mutated"
	payload.Pending.Arguments["path"] = "mutated"
	payload.Message.Content = "mutated message"
	payload.Message.ToolCalls[0].Result["authorization_id"] = "mutated"
	payload.Message.SuggestedPrompts[0] = "mutated"
	payload.Compaction.Summary.Summary = "mutated"
	*payload.Compaction.Summary.LastMessageAt = lastMessageAt.Add(time.Hour)
	*payload.Compaction.LastCompactedAt = lastCompactedAt.Add(time.Hour)

	if cloned.Session.Messages[0].Content != "authorization denied" ||
		cloned.Session.Messages[0].ToolCalls[0].Result["decision"] != "deny" ||
		cloned.Session.Messages[0].ToolCalls[0].Metadata["phase"] != "approval" {
		t.Fatalf("clone shares session messages: %#v", cloned.Session.Messages[0])
	}
	if cloned.Session.NextAction == payload.Session.NextAction || cloned.Session.NextAction.Reason != "not within scope" {
		t.Fatalf("clone shares next action: %#v", cloned.Session.NextAction)
	}
	if cloned.Pending.Arguments["path"] != "README.md" {
		t.Fatalf("clone shares authorization arguments: %#v", cloned.Pending.Arguments)
	}
	if cloned.Message.Content != "Authorization denied" ||
		cloned.Message.ToolCalls[0].Result["authorization_id"] != "auth-result" ||
		cloned.Message.SuggestedPrompts[0] != "revise scope" {
		t.Fatalf("clone shares authorization message: %#v", cloned.Message)
	}
	if cloned.Compaction == payload.Compaction ||
		cloned.Compaction.Summary == payload.Compaction.Summary ||
		cloned.Compaction.Summary.LastMessageAt == payload.Compaction.Summary.LastMessageAt ||
		cloned.Compaction.Summary.LastMessageAt == nil ||
		!cloned.Compaction.Summary.LastMessageAt.Equal(originalLastMessageAt) ||
		cloned.Compaction.LastCompactedAt == payload.Compaction.LastCompactedAt ||
		cloned.Compaction.LastCompactedAt == nil ||
		!cloned.Compaction.LastCompactedAt.Equal(originalLastCompactedAt) {
		t.Fatalf("clone shares compaction: %#v", cloned.Compaction)
	}
	if cloned.OperationLevel != "safe_scan" || !cloned.Mutated || cloned.Session.SessionID != "session-auth-result" {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
}

func TestAgentLoopRequestAuthorizationResultPayloadCloneIsolatesMutableFields(t *testing.T) {
	payload := AgentLoopRequestAuthorizationResultPayload{
		Session: AgentLoopSessionSnapshot{
			SessionID: "session-request-auth-result",
			Status:    "waiting_authorization",
			PendingAuthorization: &AgentLoopAuthorizationRequestSnapshot{
				ID:         "auth-request-result",
				ActionName: "workspace.files.write",
				Arguments:  map[string]any{"path": "README.md"},
				Status:     "pending",
			},
			NextAction: &AgentLoopNextAction{
				Type:            "authorization_required",
				Reason:          "workspace write requires approval",
				SuggestedPrompt: "approve or deny",
			},
		},
		Authorization: AgentLoopAuthorizationRequestSnapshot{
			ID:            "auth-request-result",
			ActionName:    "workspace.files.write",
			Arguments:     map[string]any{"path": "README.md"},
			RequiredLevel: "workspace_write",
			Policy:        "manual",
			Status:        "pending",
		},
		Message: AgentLoopMessage{
			ID:      "msg-request-auth",
			Role:    "agent",
			Content: "Authorization required",
			ToolCalls: []AgentLoopToolCall{{
				ID:     "tool-request-auth",
				Name:   "approval.requested",
				Result: map[string]any{"authorization_id": "auth-request-result"},
			}},
			SuggestedPrompts: []string{"approve"},
		},
		Mutated: true,
	}

	cloned := payload.Clone()

	payload.Session.PendingAuthorization.Arguments["path"] = "mutated"
	payload.Session.NextAction.SuggestedPrompt = "mutated"
	payload.Authorization.Arguments["path"] = "mutated"
	payload.Message.Content = "mutated"
	payload.Message.ToolCalls[0].Result["authorization_id"] = "mutated"
	payload.Message.SuggestedPrompts[0] = "mutated"

	if cloned.Session.PendingAuthorization == payload.Session.PendingAuthorization ||
		cloned.Session.PendingAuthorization.Arguments["path"] != "README.md" {
		t.Fatalf("clone shares session pending authorization: %#v", cloned.Session.PendingAuthorization)
	}
	if cloned.Session.NextAction == payload.Session.NextAction || cloned.Session.NextAction.SuggestedPrompt != "approve or deny" {
		t.Fatalf("clone shares next action: %#v", cloned.Session.NextAction)
	}
	if cloned.Authorization.Arguments["path"] != "README.md" {
		t.Fatalf("clone shares authorization arguments: %#v", cloned.Authorization.Arguments)
	}
	if cloned.Message.Content != "Authorization required" ||
		cloned.Message.ToolCalls[0].Result["authorization_id"] != "auth-request-result" ||
		cloned.Message.SuggestedPrompts[0] != "approve" {
		t.Fatalf("clone shares authorization message: %#v", cloned.Message)
	}
	if !cloned.Mutated || cloned.Session.SessionID != "session-request-auth-result" {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
}

func TestAgentLoopCapabilityBlockedResultPayloadCloneIsolatesMutableFields(t *testing.T) {
	payload := AgentLoopCapabilityBlockedResultPayload{
		Session: AgentLoopSessionSnapshot{
			SessionID: "session-capability-result",
			Status:    "blocked",
			Todos: []AgentLoopSessionTodo{{
				ID:            "todo-capability",
				Status:        "blocked",
				ContextRefIDs: []string{"ctx-capability"},
			}},
			NextAction: &AgentLoopNextAction{
				Type:            "blocked",
				Reason:          "capability mismatch",
				SuggestedPrompt: "use safe scan",
			},
		},
		ActionName:      "scan.web.safe",
		IntentID:        "content_discovery",
		Reason:          "capability mismatch",
		SuggestedPrompt: "use safe scan",
		Mutated:         true,
	}

	cloned := payload.Clone()

	payload.Session.Todos[0].ContextRefIDs[0] = "mutated"
	payload.Session.NextAction.SuggestedPrompt = "mutated"

	if cloned.Session.Todos[0].ContextRefIDs[0] != "ctx-capability" {
		t.Fatalf("clone shares todo context refs: %#v", cloned.Session.Todos[0])
	}
	if cloned.Session.NextAction == payload.Session.NextAction || cloned.Session.NextAction.SuggestedPrompt != "use safe scan" {
		t.Fatalf("clone shares next action: %#v", cloned.Session.NextAction)
	}
	if cloned.ActionName != "scan.web.safe" ||
		cloned.IntentID != "content_discovery" ||
		cloned.Reason != "capability mismatch" ||
		cloned.SuggestedPrompt != "use safe scan" ||
		!cloned.Mutated {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
}
