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
