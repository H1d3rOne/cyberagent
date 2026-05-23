package state

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestStoreSaveLoadJobPlanStep(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state", "bolt.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	job := contracts.NewWebScanJob("job_001", "target_001", "scope_001", "safe", "runs/job_001")
	if err := store.SaveJob(job); err != nil {
		t.Fatalf("SaveJob() error = %v", err)
	}
	loadedJob, err := store.LoadJob(job.ID)
	if err != nil {
		t.Fatalf("LoadJob() error = %v", err)
	}
	if loadedJob.ID != job.ID || loadedJob.Status != contracts.StatusCreated {
		t.Fatalf("unexpected loaded job: %#v", loadedJob)
	}

	plan := contracts.NewPlan("plan_001", job.ID, "builtin.web.safe.v1", []string{"step_001"})
	if err := store.SavePlan(plan); err != nil {
		t.Fatalf("SavePlan() error = %v", err)
	}
	loadedPlan, err := store.LoadPlan(plan.ID)
	if err != nil {
		t.Fatalf("LoadPlan() error = %v", err)
	}
	if loadedPlan.ID != plan.ID || loadedPlan.JobID != job.ID {
		t.Fatalf("unexpected loaded plan: %#v", loadedPlan)
	}

	step := contracts.NewStep("step_001", job.ID, plan.ID, "HTTP Probe", contracts.StepTypeBuiltinTool, contracts.RiskLow, contracts.ToolRef{Name: "builtin.http_probe", Version: "v1"})
	if err := store.SaveStep(step); err != nil {
		t.Fatalf("SaveStep() error = %v", err)
	}
	loadedStep, err := store.LoadStep(step.ID)
	if err != nil {
		t.Fatalf("LoadStep() error = %v", err)
	}
	if loadedStep.ID != step.ID || loadedStep.Tool.Name != step.Tool.Name {
		t.Fatalf("unexpected loaded step: %#v", loadedStep)
	}

	steps, err := store.ListSteps(job.ID)
	if err != nil {
		t.Fatalf("ListSteps() error = %v", err)
	}
	if len(steps) != 1 || steps[0].ID != step.ID {
		t.Fatalf("unexpected steps: %#v", steps)
	}

	decision := contracts.PolicyDecision{
		ID:        "policy_step_001",
		JobID:     job.ID,
		StepID:    step.ID,
		Decision:  contracts.PolicyRequireApproval,
		Reasons:   []string{"step_requires_approval"},
		CreatedAt: contracts.Now(),
	}
	if err := store.SavePolicyDecision(decision); err != nil {
		t.Fatalf("SavePolicyDecision() error = %v", err)
	}
	loadedDecision, err := store.LoadPolicyDecision(decision.ID)
	if err != nil {
		t.Fatalf("LoadPolicyDecision() error = %v", err)
	}
	if loadedDecision.ID != decision.ID || loadedDecision.Decision != contracts.PolicyRequireApproval {
		t.Fatalf("unexpected loaded policy decision: %#v", loadedDecision)
	}
	decisions, err := store.ListPolicyDecisions(job.ID)
	if err != nil {
		t.Fatalf("ListPolicyDecisions() error = %v", err)
	}
	if len(decisions) != 1 || decisions[0].ID != decision.ID {
		t.Fatalf("unexpected policy decisions: %#v", decisions)
	}

	request := contracts.HITLRequest{
		ID:               "approval_step_001",
		JobID:            job.ID,
		StepID:           step.ID,
		PolicyDecisionID: decision.ID,
		Status:           contracts.HITLPending,
		Reason:           "step_requires_approval",
		RequestedAction:  "execute_step",
		Options:          []string{"approve", "deny"},
		CreatedAt:        contracts.Now(),
	}
	if err := store.SaveHITLRequest(request); err != nil {
		t.Fatalf("SaveHITLRequest() error = %v", err)
	}
	loadedRequest, err := store.LoadHITLRequest(request.ID)
	if err != nil {
		t.Fatalf("LoadHITLRequest() error = %v", err)
	}
	if loadedRequest.ID != request.ID || loadedRequest.Status != contracts.HITLPending || loadedRequest.PolicyDecisionID != decision.ID {
		t.Fatalf("unexpected loaded HITL request: %#v", loadedRequest)
	}
	requests, err := store.ListHITLRequests(job.ID)
	if err != nil {
		t.Fatalf("ListHITLRequests() error = %v", err)
	}
	if len(requests) != 1 || requests[0].ID != request.ID {
		t.Fatalf("unexpected HITL requests: %#v", requests)
	}
}

func TestStoreLoadMissing(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state", "bolt.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = store.LoadJob("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadJob missing error = %v, want ErrNotFound", err)
	}
}
