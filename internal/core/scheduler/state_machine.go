package scheduler

import (
	"fmt"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// StepStateMachine owns the MVP lifecycle transitions for a schedulable step.
type StepStateMachine struct{}

// NewStepStateMachine creates a deterministic step lifecycle state machine.
func NewStepStateMachine() StepStateMachine {
	return StepStateMachine{}
}

// Start moves a pending/ready/created step to running and sets StartedAt once.
func (m StepStateMachine) Start(step contracts.Step) (contracts.Step, error) {
	if !canStart(step.Status) {
		return contracts.Step{}, fmt.Errorf("cannot start step %s from status %s", step.ID, step.Status)
	}
	step.Status = contracts.StatusRunning
	if step.StartedAt == nil {
		started := contracts.Now()
		step.StartedAt = &started
	}
	return step, nil
}

// Succeed moves a running step to succeeded and sets FinishedAt.
func (m StepStateMachine) Succeed(step contracts.Step) (contracts.Step, error) {
	if step.Status != contracts.StatusRunning {
		return contracts.Step{}, fmt.Errorf("cannot succeed step %s from status %s", step.ID, step.Status)
	}
	step.Status = contracts.StatusSucceeded
	step.Error = ""
	finish(&step)
	return step, nil
}

// Fail moves a running step to failed and records a reason.
func (m StepStateMachine) Fail(step contracts.Step, err error) (contracts.Step, error) {
	if step.Status != contracts.StatusRunning {
		return contracts.Step{}, fmt.Errorf("cannot fail step %s from status %s", step.ID, step.Status)
	}
	step.Status = contracts.StatusFailed
	if err != nil {
		step.Error = err.Error()
	}
	finish(&step)
	return step, nil
}

// Skip moves a pending/running step to skipped and records a reason.
func (m StepStateMachine) Skip(step contracts.Step, reason string) (contracts.Step, error) {
	if !canSkip(step.Status) {
		return contracts.Step{}, fmt.Errorf("cannot skip step %s from status %s", step.ID, step.Status)
	}
	step.Status = contracts.StatusSkipped
	step.Error = strings.TrimSpace(reason)
	finish(&step)
	return step, nil
}

func canStart(status contracts.Status) bool {
	switch status {
	case contracts.StatusPending, contracts.StatusReady, contracts.StatusCreated, "":
		return true
	default:
		return false
	}
}

func canSkip(status contracts.Status) bool {
	switch status {
	case contracts.StatusPending, contracts.StatusReady, contracts.StatusCreated, contracts.StatusRunning, contracts.StatusWaitingApproval, contracts.StatusBlocked, "":
		return true
	default:
		return false
	}
}

func finish(step *contracts.Step) {
	finished := contracts.Now()
	step.FinishedAt = &finished
}
