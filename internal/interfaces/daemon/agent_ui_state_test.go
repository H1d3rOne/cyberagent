package daemon

import (
	"errors"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestAgentUIStateDeterministicMappingAcrossResponses(t *testing.T) {
	now := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name           string
		session        agentSession
		wantState      string
		wantPhase      string
		wantTone       string
		wantBusy       bool
		wantAction     bool
		wantError      bool
		wantSource     string
		wantToolCallID string
		wantApprovalID string
	}{
		{
			name: "idle",
			session: agentSession{
				SessionID:  "agent_ui_idle",
				Status:     "idle",
				NextAction: &agentNextAction{Type: "wait_user", Reason: "waiting input"},
			},
			wantState:  agentUIStateIdle,
			wantPhase:  "idle",
			wantTone:   "neutral",
			wantSource: "session",
		},
		{
			name: "running",
			session: agentSession{
				SessionID:  "agent_ui_running",
				Status:     "running",
				ActiveTask: &agentTask{ID: "task-running", Goal: "run task", Status: "running", CreatedAt: now, UpdatedAt: now},
				NextAction: &agentNextAction{Type: "continue", Reason: "keep going"},
			},
			wantState:  agentUIStateRunning,
			wantPhase:  "task",
			wantTone:   "info",
			wantBusy:   true,
			wantSource: "session",
		},
		{
			name: "failed",
			session: agentSession{
				SessionID:  "agent_ui_failed",
				Status:     "failed",
				ActiveTask: &agentTask{ID: "task-failed", Goal: "fail task", Status: "failed", Summary: "task failed", CreatedAt: now, UpdatedAt: now},
				NextAction: &agentNextAction{Type: "blocked", Reason: "needs recovery"},
			},
			wantState:  agentUIStateFailed,
			wantPhase:  "task",
			wantTone:   "danger",
			wantError:  true,
			wantSource: "next_action",
		},
		{
			name: "done",
			session: agentSession{
				SessionID:  "agent_ui_done",
				Status:     "idle",
				ActiveTask: &agentTask{ID: "task-done", Goal: "done task", Status: "done", Summary: "completed", CreatedAt: now, UpdatedAt: now},
				NextAction: &agentNextAction{Type: "finished", Reason: "done"},
			},
			wantState:  agentUIStateDone,
			wantPhase:  "final_answer",
			wantTone:   "success",
			wantSource: "active_task",
		},
		{
			name: "approval",
			session: agentSession{
				SessionID:  "agent_ui_approval",
				Status:     "running",
				ActiveTask: &agentTask{ID: "task-approval", Goal: "approval task", Status: "running", CreatedAt: now, UpdatedAt: now},
				PendingAuthorization: &agentAuthorizationRequest{
					ID:            "auth-1",
					ActionName:    "workspace.files.write",
					RequiredLevel: "write",
					Reason:        "needs approval",
					Status:        "pending",
					CreatedAt:     now,
					UpdatedAt:     now,
				},
			},
			wantState:      agentUIStateApproval,
			wantPhase:      "approval",
			wantTone:       "warning",
			wantAction:     true,
			wantSource:     "pending_authorization",
			wantApprovalID: "auth-1",
		},
		{
			name: "tool_running",
			session: agentSession{
				SessionID:  "agent_ui_tool_running",
				Status:     "running",
				ActiveTask: &agentTask{ID: "task-tool", Goal: "tool task", Status: "running", CreatedAt: now, UpdatedAt: now},
				ToolCalls: []agentToolCall{
					withAgentToolMetadata(agentToolCall{ID: "tool-running", Name: "workspace.command", Status: contracts.StatusRunning, Detail: "running command", StartedAt: now}, map[string]interface{}{"phase": "workspace"}),
				},
			},
			wantState:      agentUIStateToolRunning,
			wantPhase:      "workspace",
			wantTone:       "info",
			wantBusy:       true,
			wantSource:     "tool_call",
			wantToolCallID: "tool-running",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertAgentUIState(t, "list", agentSessionSummaryFrom(&tc.session).UIState, tc.wantState, tc.wantPhase, tc.wantTone, tc.wantBusy, tc.wantAction, tc.wantError, tc.wantSource, tc.wantToolCallID, tc.wantApprovalID)
			assertAgentUIState(t, "detail", cloneAgentSession(&tc.session).UIState, tc.wantState, tc.wantPhase, tc.wantTone, tc.wantBusy, tc.wantAction, tc.wantError, tc.wantSource, tc.wantToolCallID, tc.wantApprovalID)
			assertAgentUIState(t, "send", buildAgentSendResponse(tc.session.SessionID, nil, nil, tc.session).UIState, tc.wantState, tc.wantPhase, tc.wantTone, tc.wantBusy, tc.wantAction, tc.wantError, tc.wantSource, tc.wantToolCallID, tc.wantApprovalID)
		})
	}
}

func TestAgentUIStateSendResponseUsesLatestSessionState(t *testing.T) {
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	tool := failedAgentToolCall("workspace.command", errors.New("command failed"))
	tool.ID = "tool-failed"
	tool.StartedAt = now
	session := agentSession{
		SessionID:  "agent_ui_send_failed",
		Status:     "running",
		ActiveTask: &agentTask{ID: "task-send", Goal: "send task", Status: "running", CreatedAt: now, UpdatedAt: now},
		ToolCalls:  []agentToolCall{tool},
	}
	response := buildAgentSendResponse(session.SessionID, nil, nil, session)
	assertAgentUIState(t, "send", response.UIState, agentUIStateFailed, "workspace", "danger", false, false, true, "tool_call", "tool-failed", "")
}

func assertAgentUIState(t *testing.T, label string, got *agentUIState, wantState, wantPhase, wantTone string, wantBusy, wantAction, wantError bool, wantSource, wantToolCallID, wantApprovalID string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s ui_state = nil, want %s", label, wantState)
	}
	if got.State != wantState || got.Phase != wantPhase || got.Tone != wantTone || got.Busy != wantBusy || got.RequiresAction != wantAction || got.HasError != wantError || got.Source != wantSource {
		t.Fatalf("%s ui_state = %#v, want state=%s phase=%s tone=%s busy=%v action=%v error=%v source=%s", label, got, wantState, wantPhase, wantTone, wantBusy, wantAction, wantError, wantSource)
	}
	if got.ToolCallID != wantToolCallID {
		t.Fatalf("%s tool_call_id = %q, want %q; ui_state=%#v", label, got.ToolCallID, wantToolCallID, got)
	}
	if got.ApprovalID != wantApprovalID {
		t.Fatalf("%s approval_id = %q, want %q; ui_state=%#v", label, got.ApprovalID, wantApprovalID, got)
	}
}
