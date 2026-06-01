package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
	"github.com/H1d3rOne/cyberagent/internal/runtime/state"
)

func TestApprovalServiceListApproveDenyAndErrors(t *testing.T) {
	runsDir := t.TempDir()
	layout := createApprovalRun(t, runsDir, "run_approvals", []contracts.HITLRequest{
		testApproval("approval_001", "job_001", "step_001"),
		testApproval("approval_002", "job_001", "step_002"),
	})

	service := NewApprovalService(runsDir)
	requests, err := service.ListApprovals(context.Background(), "run_approvals")
	if err != nil {
		t.Fatalf("ListApprovals() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("approvals = %d, want 2: %#v", len(requests), requests)
	}

	approved, err := service.Approve(context.Background(), ApprovalDecisionRequest{
		RunIDOrPath:        "run_approvals",
		ApprovalIDOrStepID: "step_001",
		Operator:           "service-test",
		Comment:            "approved from service test",
	})
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved.Status != contracts.HITLApproved || approved.Resolution["resolved_by"] != "service-test" || approved.Resolution["comment"] != "approved from service test" {
		t.Fatalf("unexpected approved request: %#v", approved)
	}

	denied, err := service.Deny(context.Background(), ApprovalDecisionRequest{
		RunIDOrPath:        layout.Path,
		ApprovalIDOrStepID: "approval_002",
		Operator:           "service-test",
	})
	if err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	if denied.Status != contracts.HITLRejected || denied.Resolution["resolved_by"] != "service-test" {
		t.Fatalf("unexpected denied request: %#v", denied)
	}

	persisted := loadHITL(t, layout.BoltPath, "approval_001")
	if persisted.Status != contracts.HITLApproved {
		t.Fatalf("state approval status = %s, want approved", persisted.Status)
	}

	allEvents, err := events.NewStore(layout.EventsPath).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll events: %v", err)
	}
	if len(allEvents) != 2 || allEvents[0].Type != contracts.EventApprovalResolved || allEvents[1].Type != contracts.EventApprovalResolved {
		t.Fatalf("unexpected approval events: %#v", allEvents)
	}
	calls, err := command.NewRecorder(layout.CommandsPath).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll commands: %v", err)
	}
	if len(calls) != 2 || calls[0].ToolName != "cyberagent.approvals" || calls[1].ToolName != "cyberagent.approvals" {
		t.Fatalf("unexpected approval command calls: %#v", calls)
	}

	_, err = service.Approve(context.Background(), ApprovalDecisionRequest{RunIDOrPath: "run_approvals", ApprovalIDOrStepID: "approval_001"})
	if err == nil || ErrorCodeOf(err) != CodeConflict {
		t.Fatalf("re-approve code = %s, err=%v", ErrorCodeOf(err), err)
	}
	_, err = service.Approve(context.Background(), ApprovalDecisionRequest{RunIDOrPath: "run_approvals", ApprovalIDOrStepID: "missing"})
	if err == nil || ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("missing approval code = %s, err=%v", ErrorCodeOf(err), err)
	}
	_, err = service.Approve(context.Background(), ApprovalDecisionRequest{RunIDOrPath: "run_approvals"})
	if err == nil || ErrorCodeOf(err) != CodeBadRequest {
		t.Fatalf("empty approval code = %s, err=%v", ErrorCodeOf(err), err)
	}
}

func TestApprovalServiceCancelAndEditArgs(t *testing.T) {
	runsDir := t.TempDir()
	layout := createApprovalRun(t, runsDir, "run_approval_decisions", []contracts.HITLRequest{
		testApproval("approval_cancel", "job_001", "step_cancel"),
		testApproval("approval_edit", "job_001", "step_edit"),
	})

	service := NewApprovalService(runsDir)
	cancelled, err := service.Cancel(context.Background(), ApprovalDecisionRequest{
		RunIDOrPath:        "run_approval_decisions",
		ApprovalIDOrStepID: "step_cancel",
		Operator:           "service-test",
		Comment:            "operator cancelled",
	})
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if cancelled.Status != contracts.HITLCancelled || cancelled.Resolution["decision"] != "cancel" || cancelled.Resolution["comment"] != "operator cancelled" {
		t.Fatalf("unexpected cancelled request: %#v", cancelled)
	}

	editArgs := map[string]any{"target": "https://example.com/edited", "method": "POST"}
	modified, err := service.EditArgs(context.Background(), ApprovalDecisionRequest{
		RunIDOrPath:        layout.Path,
		ApprovalIDOrStepID: "approval_edit",
		Operator:           "service-test",
		EditArgs:           editArgs,
	})
	if err != nil {
		t.Fatalf("EditArgs() error = %v", err)
	}
	if modified.Status != contracts.HITLModified || modified.Resolution["decision"] != "edit_args" {
		t.Fatalf("unexpected modified request: %#v", modified)
	}
	modifiedArgs, ok := modified.Resolution["edit_args"].(map[string]any)
	if !ok || modifiedArgs["target"] != editArgs["target"] || modifiedArgs["method"] != editArgs["method"] {
		t.Fatalf("unexpected modified edit_args: %#v", modified.Resolution["edit_args"])
	}

	persistedCancel := loadHITL(t, layout.BoltPath, "approval_cancel")
	if persistedCancel.Status != contracts.HITLCancelled {
		t.Fatalf("state cancel status = %s, want cancelled", persistedCancel.Status)
	}
	persistedEdit := loadHITL(t, layout.BoltPath, "approval_edit")
	if persistedEdit.Status != contracts.HITLModified || persistedEdit.Resolution["decision"] != "edit_args" {
		t.Fatalf("unexpected persisted edit request: %#v", persistedEdit)
	}

	allEvents, err := events.NewStore(layout.EventsPath).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll events: %v", err)
	}
	if len(allEvents) != 2 || allEvents[0].Data["decision"] != "cancel" || allEvents[1].Data["decision"] != "edit_args" {
		t.Fatalf("unexpected approval events: %#v", allEvents)
	}
	commands, err := command.NewRecorder(layout.CommandsPath).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll commands: %v", err)
	}
	if len(commands) != 2 || commands[0].Metadata["decision"] != "cancel" || commands[1].Metadata["decision"] != "edit_args" {
		t.Fatalf("unexpected approval command calls: %#v", commands)
	}
}

func TestApprovalServiceResumeApprovedWrapperNoApprovedSteps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runsDir := t.TempDir()
	runID := "run_resume_empty"
	runDir := filepath.Join(runsDir, runID)
	if _, err := coreruntime.RunWebScan(context.Background(), coreruntime.WebScanRequest{
		Target:          server.URL,
		Scope:           server.URL,
		Out:             runDir,
		Profile:         "safe",
		NoExternalTools: true,
		BuildInfo:       coreruntime.BuildInfo{Version: "v0.1.2-test"},
		CommandName:     "scan web",
	}); err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}

	result, err := NewApprovalService(runsDir).ResumeApproved(context.Background(), ResumeApprovedRequest{RunIDOrPath: runID, Operator: "service-test"})
	if err != nil {
		t.Fatalf("ResumeApproved() error = %v", err)
	}
	if result.RunID != runID || result.RunDir != runDir || len(result.Resumed) != 0 || len(result.Skipped) != 0 {
		t.Fatalf("unexpected resume wrapper result: %#v", result)
	}
}

func createApprovalRun(t *testing.T, runsDir, runID string, requests []contracts.HITLRequest) runs.Layout {
	t.Helper()
	layout, err := runs.NewManager(runsDir).Create(runID)
	if err != nil {
		t.Fatalf("Create run: %v", err)
	}
	writeApprovalsJSON(t, layout.ApprovalsPath, requests)
	store, err := state.Open(layout.BoltPath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	for _, request := range requests {
		if err := store.SaveHITLRequest(request); err != nil {
			_ = store.Close()
			t.Fatalf("SaveHITLRequest() error = %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close state: %v", err)
	}
	return layout
}

func testApproval(id, jobID, stepID string) contracts.HITLRequest {
	return contracts.HITLRequest{
		ID:               id,
		JobID:            jobID,
		StepID:           stepID,
		PolicyDecisionID: "policy_" + stepID,
		Status:           contracts.HITLPending,
		Reason:           "requires approval",
		RequestedAction:  "execute_step",
		Options:          []string{"approve", "deny"},
		CreatedAt:        contracts.Now(),
	}
}

func writeApprovalsJSON(t *testing.T, path string, requests []contracts.HITLRequest) {
	t.Helper()
	encoded, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		t.Fatalf("marshal approvals: %v", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write approvals: %v", err)
	}
}

func loadHITL(t *testing.T, boltPath, id string) contracts.HITLRequest {
	t.Helper()
	store, err := state.Open(boltPath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer store.Close()
	request, err := store.LoadHITLRequest(id)
	if err != nil {
		t.Fatalf("LoadHITLRequest() error = %v", err)
	}
	return request
}
