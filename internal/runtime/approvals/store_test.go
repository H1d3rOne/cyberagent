package approvals

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
	"github.com/H1d3rOne/cyberagent/internal/runtime/state"
)

func TestStoreListAndResolveApproval(t *testing.T) {
	runDir := t.TempDir()
	request := contracts.HITLRequest{
		ID:               "approval_step_001",
		JobID:            "job_001",
		StepID:           "step_001",
		PolicyDecisionID: "policy_step_001",
		Status:           contracts.HITLPending,
		Reason:           "step_requires_approval",
		RequestedAction:  "execute_step",
		Options:          []string{"approve", "deny"},
		CreatedAt:        contracts.Now(),
	}
	writeApprovalsForTest(t, runDir, []contracts.HITLRequest{request})

	stateStore, err := state.Open(filepath.Join(runDir, "state", "bolt.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	if err := stateStore.SaveHITLRequest(request); err != nil {
		t.Fatalf("SaveHITLRequest() error = %v", err)
	}
	stateStore.Close()

	store := NewStore(runDir)
	requests, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(requests) != 1 || requests[0].ID != request.ID || requests[0].Status != contracts.HITLPending {
		t.Fatalf("unexpected requests: %#v", requests)
	}

	resolved, err := store.Resolve("step_001", DecisionApprove)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Status != contracts.HITLApproved || resolved.ResolvedAt == nil || resolved.Resolution["decision"] != string(DecisionApprove) {
		t.Fatalf("unexpected resolved request: %#v", resolved)
	}

	var onDisk []contracts.HITLRequest
	readJSONForTest(t, filepath.Join(runDir, "approvals.json"), &onDisk)
	if len(onDisk) != 1 || onDisk[0].Status != contracts.HITLApproved {
		t.Fatalf("unexpected approvals.json: %#v", onDisk)
	}

	stateStore, err = state.Open(filepath.Join(runDir, "state", "bolt.db"))
	if err != nil {
		t.Fatalf("reopen state store: %v", err)
	}
	persisted, err := stateStore.LoadHITLRequest(request.ID)
	stateStore.Close()
	if err != nil {
		t.Fatalf("LoadHITLRequest() error = %v", err)
	}
	if persisted.Status != contracts.HITLApproved {
		t.Fatalf("unexpected persisted request: %#v", persisted)
	}

	eventStore := events.NewStore(filepath.Join(runDir, "events.jsonl"))
	allEvents, err := eventStore.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll events: %v", err)
	}
	if len(allEvents) != 1 || allEvents[0].Type != contracts.EventApprovalResolved || allEvents[0].StepID != request.StepID {
		t.Fatalf("unexpected events: %#v", allEvents)
	}

	calls, err := command.NewRecorder(filepath.Join(runDir, "commands.log")).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll commands: %v", err)
	}
	if len(calls) != 1 || calls[0].ToolName != "cyberagent.approvals" || calls[0].Status != contracts.StatusSucceeded {
		t.Fatalf("unexpected command records: %#v", calls)
	}
}

func TestStoreResolveRejectsNonPending(t *testing.T) {
	runDir := t.TempDir()
	request := contracts.HITLRequest{
		ID:              "approval_step_001",
		JobID:           "job_001",
		StepID:          "step_001",
		Status:          contracts.HITLApproved,
		Reason:          "already approved",
		RequestedAction: "execute_step",
		CreatedAt:       contracts.Now(),
	}
	writeApprovalsForTest(t, runDir, []contracts.HITLRequest{request})
	if _, err := NewStore(runDir).Resolve(request.ID, DecisionDeny); err == nil {
		t.Fatalf("expected resolving non-pending approval to fail")
	}
}

func TestStoreResolveCancelAndEditArgsRoundTrip(t *testing.T) {
	runDir := t.TempDir()
	now := contracts.Now()
	requests := []contracts.HITLRequest{
		{
			ID:               "approval_cancel",
			JobID:            "job_001",
			StepID:           "step_cancel",
			PolicyDecisionID: "policy_cancel",
			Status:           contracts.HITLPending,
			Reason:           "cancel me",
			RequestedAction:  "execute_step",
			Options:          []string{"approve", "deny", "cancel"},
			CreatedAt:        now,
		},
		{
			ID:               "approval_edit",
			JobID:            "job_001",
			StepID:           "step_edit",
			PolicyDecisionID: "policy_edit",
			Status:           contracts.HITLPending,
			Reason:           "edit me",
			RequestedAction:  "execute_step",
			Options:          []string{"approve", "deny", "edit_args"},
			CreatedAt:        now,
		},
	}
	writeApprovalsForTest(t, runDir, requests)

	stateStore, err := state.Open(filepath.Join(runDir, "state", "bolt.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	for _, request := range requests {
		if err := stateStore.SaveHITLRequest(request); err != nil {
			_ = stateStore.Close()
			t.Fatalf("SaveHITLRequest() error = %v", err)
		}
	}
	stateStore.Close()

	store := NewStore(runDir)
	cancelled, err := store.ResolveWithMetadata("step_cancel", DecisionCancel, ResolutionMetadata{ResolvedBy: "tester", Comment: "stop it"})
	if err != nil {
		t.Fatalf("ResolveWithMetadata(cancel) error = %v", err)
	}
	if cancelled.Status != contracts.HITLCancelled || cancelled.Resolution["decision"] != string(DecisionCancel) || cancelled.Resolution["comment"] != "stop it" {
		t.Fatalf("unexpected cancelled approval: %#v", cancelled)
	}

	editArgs := map[string]any{"target": "https://example.com/edited", "method": "POST"}
	modified, err := store.ResolveWithMetadata("approval_edit", DecisionEditArgs, ResolutionMetadata{ResolvedBy: "tester", EditArgs: editArgs})
	if err != nil {
		t.Fatalf("ResolveWithMetadata(edit_args) error = %v", err)
	}
	if modified.Status != contracts.HITLModified || modified.Resolution["decision"] != string(DecisionEditArgs) {
		t.Fatalf("unexpected modified approval: %#v", modified)
	}
	returnedEditArgs, ok := modified.Resolution["edit_args"].(map[string]any)
	if !ok || returnedEditArgs["target"] != editArgs["target"] || returnedEditArgs["method"] != editArgs["method"] {
		t.Fatalf("unexpected edit_args in modified approval: %#v", modified.Resolution["edit_args"])
	}

	var onDisk []contracts.HITLRequest
	readJSONForTest(t, filepath.Join(runDir, "approvals.json"), &onDisk)
	if len(onDisk) != 2 || onDisk[0].Status != contracts.HITLCancelled || onDisk[1].Status != contracts.HITLModified {
		t.Fatalf("unexpected approvals.json statuses: %#v", onDisk)
	}
	diskEditArgs := onDisk[1].Resolution["edit_args"].(map[string]any)
	if diskEditArgs["target"] != editArgs["target"] || diskEditArgs["method"] != editArgs["method"] {
		t.Fatalf("unexpected approvals.json edit_args: %#v", diskEditArgs)
	}

	stateStore, err = state.Open(filepath.Join(runDir, "state", "bolt.db"))
	if err != nil {
		t.Fatalf("reopen state store: %v", err)
	}
	persisted, err := stateStore.LoadHITLRequest("approval_edit")
	stateStore.Close()
	if err != nil {
		t.Fatalf("LoadHITLRequest(approval_edit) error = %v", err)
	}
	if persisted.Status != contracts.HITLModified || persisted.Resolution["decision"] != string(DecisionEditArgs) {
		t.Fatalf("unexpected persisted modified approval: %#v", persisted)
	}

	allEvents, err := events.NewStore(filepath.Join(runDir, "events.jsonl")).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll events: %v", err)
	}
	if len(allEvents) != 2 || allEvents[0].Data["decision"] != string(DecisionCancel) || allEvents[1].Data["decision"] != string(DecisionEditArgs) {
		t.Fatalf("unexpected approval events: %#v", allEvents)
	}
	if asMap := allEvents[1].Data["edit_args"].(map[string]any); asMap["target"] != editArgs["target"] {
		t.Fatalf("unexpected event edit_args: %#v", asMap)
	}

	calls, err := command.NewRecorder(filepath.Join(runDir, "commands.log")).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll commands: %v", err)
	}
	if len(calls) != 2 || calls[0].Metadata["decision"] != string(DecisionCancel) || calls[1].Metadata["decision"] != string(DecisionEditArgs) {
		t.Fatalf("unexpected approval command calls: %#v", calls)
	}
	if asMap := calls[1].Metadata["edit_args"].(map[string]any); asMap["method"] != editArgs["method"] {
		t.Fatalf("unexpected command edit_args: %#v", asMap)
	}
}

func writeApprovalsForTest(t *testing.T, runDir string, requests []contracts.HITLRequest) {
	t.Helper()
	encoded, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		t.Fatalf("marshal approvals: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "approvals.json"), append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write approvals.json: %v", err)
	}
}

func readJSONForTest(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
