package scheduler

import (
	"errors"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestStepStateMachineStartAndSucceed(t *testing.T) {
	machine := NewStepStateMachine()
	step := testStep()

	started, err := machine.Start(step)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.Status != contracts.StatusRunning || started.StartedAt == nil {
		t.Fatalf("unexpected started step: %#v", started)
	}

	succeeded, err := machine.Succeed(started)
	if err != nil {
		t.Fatalf("Succeed() error = %v", err)
	}
	if succeeded.Status != contracts.StatusSucceeded || succeeded.FinishedAt == nil || succeeded.Error != "" {
		t.Fatalf("unexpected succeeded step: %#v", succeeded)
	}
}

func TestStepStateMachineFail(t *testing.T) {
	machine := NewStepStateMachine()
	started, err := machine.Start(testStep())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	failed, err := machine.Fail(started, errors.New("boom"))
	if err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if failed.Status != contracts.StatusFailed || failed.FinishedAt == nil || failed.Error != "boom" {
		t.Fatalf("unexpected failed step: %#v", failed)
	}
}

func TestStepStateMachineSkipFromPendingAndRunning(t *testing.T) {
	machine := NewStepStateMachine()

	skippedPending, err := machine.Skip(testStep(), "not needed")
	if err != nil {
		t.Fatalf("Skip(pending) error = %v", err)
	}
	if skippedPending.Status != contracts.StatusSkipped || skippedPending.FinishedAt == nil || skippedPending.Error != "not needed" {
		t.Fatalf("unexpected skipped pending step: %#v", skippedPending)
	}

	started, err := machine.Start(testStep())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	skippedRunning, err := machine.Skip(started, "policy")
	if err != nil {
		t.Fatalf("Skip(running) error = %v", err)
	}
	if skippedRunning.Status != contracts.StatusSkipped || skippedRunning.FinishedAt == nil || skippedRunning.Error != "policy" {
		t.Fatalf("unexpected skipped running step: %#v", skippedRunning)
	}
}

func TestStepStateMachineRejectsInvalidTransitions(t *testing.T) {
	machine := NewStepStateMachine()
	step := testStep()
	step.Status = contracts.StatusSucceeded

	if _, err := machine.Start(step); err == nil {
		t.Fatalf("expected start from succeeded to fail")
	}
	if _, err := machine.Succeed(testStep()); err == nil {
		t.Fatalf("expected succeed from pending to fail")
	}
	if _, err := machine.Fail(testStep(), errors.New("boom")); err == nil {
		t.Fatalf("expected fail from pending to fail")
	}
	if _, err := machine.Skip(step, "already done"); err == nil {
		t.Fatalf("expected skip from succeeded to fail")
	}
}

func testStep() contracts.Step {
	return contracts.NewStep("step_001", "job_001", "plan_001", "Test Step", contracts.StepTypeBuiltinTool, contracts.RiskLow, contracts.ToolRef{Name: "builtin.test", Version: "v1"})
}
