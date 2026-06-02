package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestApprovalCanonicalAgentAuthorizationAcrossResponses(t *testing.T) {
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	session := agentSession{
		SessionID: "agent_auth_canonical",
		Status:    "running",
		ActiveTask: &agentTask{
			ID:        "task-auth-canonical",
			Goal:      "authorization task",
			Status:    "running",
			CreatedAt: now,
			UpdatedAt: now,
		},
		PendingAuthorization: &agentAuthorizationRequest{
			ID:            "auth-canonical-1",
			ActionName:    "workspace.files.write",
			RequiredLevel: "workspace_write",
			Reason:        "explicit operator intent required",
			Status:        "pending",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}

	assertApprovalCanonicalState(t, "summary", agentSessionSummaryFrom(&session).ApprovalState, approvalKindAgentAuthorization, approvalCanonicalStatePending, "", false, "auth-canonical-1", "", "", "workspace.files.write", "workspace_write")
	assertApprovalCanonicalState(t, "detail", cloneAgentSession(&session).ApprovalState, approvalKindAgentAuthorization, approvalCanonicalStatePending, "", false, "auth-canonical-1", "", "", "workspace.files.write", "workspace_write")
	assertApprovalCanonicalState(t, "send", buildAgentSendResponse(session.SessionID, nil, nil, session).ApprovalState, approvalKindAgentAuthorization, approvalCanonicalStatePending, "", false, "auth-canonical-1", "", "", "workspace.files.write", "workspace_write")

	assertApprovalCanonicalState(t, "approved decision", canonicalAgentAuthorizationApproval(session.PendingAuthorization, "approved"), approvalKindAgentAuthorization, approvalCanonicalStateApproved, approvalCanonicalStateApproved, false, "auth-canonical-1", "", "", "workspace.files.write", "workspace_write")
	assertApprovalCanonicalState(t, "denied decision", canonicalAgentAuthorizationApproval(session.PendingAuthorization, "deny"), approvalKindAgentAuthorization, approvalCanonicalStateDenied, approvalCanonicalStateDenied, false, "auth-canonical-1", "", "", "workspace.files.write", "workspace_write")

	session.ApprovalState = &approvalCanonicalState{ApprovalKind: approvalKindAgentAuthorization, State: approvalCanonicalStatePending}
	session.normalize()
	if session.ApprovalState != nil {
		t.Fatalf("normalize should clear derived approval_state before persistence: %#v", session.ApprovalState)
	}
}

func TestApprovalCanonicalRunHITLMapping(t *testing.T) {
	now := time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC)
	cases := []struct {
		name           string
		status         contracts.HITLStatus
		resolution     map[string]any
		wantState      string
		wantDecision   string
		wantResume     bool
		wantActionName string
	}{
		{
			name:           "pending",
			status:         contracts.HITLPending,
			wantState:      approvalCanonicalStatePending,
			wantActionName: "execute_step",
		},
		{
			name:           "approved",
			status:         contracts.HITLApproved,
			resolution:     map[string]any{"decision": "approve"},
			wantState:      approvalCanonicalStateApproved,
			wantDecision:   approvalCanonicalStateApproved,
			wantResume:     true,
			wantActionName: "execute_step",
		},
		{
			name:           "rejected",
			status:         contracts.HITLRejected,
			resolution:     map[string]any{"decision": "deny"},
			wantState:      approvalCanonicalStateDenied,
			wantDecision:   approvalCanonicalStateDenied,
			wantActionName: "execute_step",
		},
		{
			name:           "cancelled",
			status:         contracts.HITLCancelled,
			resolution:     map[string]any{"decision": "cancel"},
			wantState:      approvalCanonicalStateCancelled,
			wantDecision:   approvalCanonicalStateCancelled,
			wantActionName: "execute_step",
		},
		{
			name:           "modified",
			status:         contracts.HITLModified,
			resolution:     map[string]any{"decision": "edit_args"},
			wantState:      approvalCanonicalStateModified,
			wantDecision:   approvalCanonicalStateModified,
			wantActionName: "execute_step",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := contracts.HITLRequest{
				ID:              "approval-" + tc.name,
				JobID:           "job-canonical",
				StepID:          "step-" + tc.name,
				Status:          tc.status,
				Reason:          "requires approval",
				RequestedAction: tc.wantActionName,
				CreatedAt:       now,
				Resolution:      tc.resolution,
			}
			got := canonicalRunHITLApproval("run-canonical", request)
			assertApprovalCanonicalState(t, tc.name, got, approvalKindRunHITL, tc.wantState, tc.wantDecision, tc.wantResume, request.ID, "run-canonical", request.StepID, tc.wantActionName, "")
		})
	}
}

func TestRunHITLApprovalResponsePreservesLegacyFieldsAndAddsCanonicalState(t *testing.T) {
	request := contracts.HITLRequest{
		ID:              "approval-json",
		JobID:           "job-json",
		StepID:          "step-json",
		Status:          contracts.HITLApproved,
		Reason:          "approval reason",
		RequestedAction: "execute_step",
		Resolution:      map[string]any{"decision": "approve"},
		CreatedAt:       time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
	}
	encoded, err := json.Marshal(runHITLApprovalResponseFor("run-json", request))
	if err != nil {
		t.Fatalf("Marshal(runHITLApprovalResponse) error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("Unmarshal(runHITLApprovalResponse JSON) error = %v", err)
	}
	if payload["id"] != request.ID || payload["status"] != string(contracts.HITLApproved) || payload["requested_action"] != request.RequestedAction {
		t.Fatalf("legacy HITL fields not preserved: %s", encoded)
	}
	state := asMap(t, payload["approval_state"])
	if state["approval_kind"] != approvalKindRunHITL ||
		state["state"] != approvalCanonicalStateApproved ||
		state["decision"] != approvalCanonicalStateApproved ||
		state["resume_available"] != true ||
		state["approval_id"] != request.ID ||
		state["run_id"] != "run-json" ||
		state["step_id"] != request.StepID {
		t.Fatalf("unexpected approval_state JSON: %#v payload=%s", state, encoded)
	}
}

func TestApprovalCanonicalAgentAuthorizationRuntimeEvents(t *testing.T) {
	store := agentruntime.NewMemStore()
	engine := agentruntime.NewEngine(store)
	manager := newAgentManager(t.TempDir())
	manager.AttachEngine(engine)
	now := time.Date(2026, 6, 3, 13, 0, 0, 0, time.UTC)
	session := agentSession{
		SessionID: "agent_auth_runtime",
		Status:    "running",
		ActiveTask: &agentTask{
			ID:        "task-auth-runtime",
			Goal:      "authorization runtime task",
			Status:    "running",
			CreatedAt: now,
			UpdatedAt: now,
		},
		PendingAuthorization: &agentAuthorizationRequest{
			ID:            "auth-runtime-1",
			ActionName:    "http.request",
			RequiredLevel: "safe_scan",
			Reason:        "http request requires authorization",
			Status:        "pending",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}

	manager.emitAgentRuntimeEvents(context.Background(), engine, session.SessionID, session, agentMessage{})
	required := firstApprovalRuntimeEvent(store.Events(), "approval_required")
	if required == nil {
		t.Fatalf("missing approval_required runtime event: %#v", store.Events())
	}
	if required.Data["approval_kind"] != approvalKindAgentAuthorization {
		t.Fatalf("approval_required approval_kind = %#v, want %s", required.Data["approval_kind"], approvalKindAgentAuthorization)
	}
	assertApprovalCanonicalState(t, "approval_required event", approvalCanonicalFromEventData(t, required.Data["approval_state"]), approvalKindAgentAuthorization, approvalCanonicalStatePending, "", false, "auth-runtime-1", "", "", "http.request", "safe_scan")

	manager.emitRuntimeApprovalResolved(context.Background(), session.SessionID, session, *session.PendingAuthorization, "approved")
	resolved := firstApprovalRuntimeEvent(store.Events(), "approval_resolved")
	if resolved == nil {
		t.Fatalf("missing approval_resolved runtime event: %#v", store.Events())
	}
	if resolved.Data["approval_kind"] != approvalKindAgentAuthorization || resolved.Data["decision"] != "approved" {
		t.Fatalf("unexpected approval_resolved data: %#v", resolved.Data)
	}
	assertApprovalCanonicalState(t, "approval_resolved event", approvalCanonicalFromEventData(t, resolved.Data["approval_state"]), approvalKindAgentAuthorization, approvalCanonicalStateApproved, approvalCanonicalStateApproved, false, "auth-runtime-1", "", "", "http.request", "safe_scan")
}

func approvalCanonicalFromEventData(t *testing.T, value any) *approvalCanonicalState {
	t.Helper()
	state, ok := value.(*approvalCanonicalState)
	if !ok {
		t.Fatalf("approval_state event data is %T, want *approvalCanonicalState: %#v", value, value)
	}
	return state
}

func assertApprovalCanonicalState(t *testing.T, label string, got *approvalCanonicalState, wantKind, wantState, wantDecision string, wantResume bool, wantApprovalID, wantRunID, wantStepID, wantActionName, wantRequiredLevel string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s approval_state = nil, want kind=%s state=%s", label, wantKind, wantState)
	}
	if got.ApprovalKind != wantKind ||
		got.State != wantState ||
		got.Decision != wantDecision ||
		got.ResumeAvailable != wantResume ||
		got.ApprovalID != wantApprovalID ||
		got.RunID != wantRunID ||
		got.StepID != wantStepID ||
		got.ActionName != wantActionName ||
		got.RequiredLevel != wantRequiredLevel {
		t.Fatalf("%s approval_state = %#v, want kind=%s state=%s decision=%s resume=%v approval=%s run=%s step=%s action=%s level=%s", label, got, wantKind, wantState, wantDecision, wantResume, wantApprovalID, wantRunID, wantStepID, wantActionName, wantRequiredLevel)
	}
}
