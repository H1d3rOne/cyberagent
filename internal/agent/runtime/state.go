// state.go declares the named state types and the legal transitions among
// them. The runtime package previously used raw strings for Task/Run/Turn
// state (see runtime.go), which makes typos silent and forbids the compiler
// from flagging illegal transitions. Centralising the constants and exposing
// ValidateTaskTransition / ValidateRunTransition lets the Engine and the
// MemStore guard their writes without scattering the rules across the package.
//
// The named types intentionally do not change the existing Task/Run/Turn
// struct field declarations — those still hold strings — so this file is
// additive and safe to land before the engine is filled in. Engine code that
// needs a typed view should convert via TaskState(s.State) at the boundary.
package runtime

import "fmt"

// TaskState is the lifecycle of a top-level Task: queue → run → terminal.
// Pausing a running Task surfaces as "paused" so the scheduler / approval
// gate can hold a Task without losing accumulated context.
type TaskState string

const (
	TaskQueued    TaskState = "queued"
	TaskRunning   TaskState = "running"
	TaskPaused    TaskState = "paused"
	TaskDone      TaskState = "done"
	TaskFailed    TaskState = "failed"
	TaskCancelled TaskState = "cancelled"
)

// RunState is the lifecycle of one execution attempt. AwaitingApproval is
// the explicit "blocked on operator decision" pause distinct from a Task-
// level Paused — the run still owns its turn buffer while the gate is open.
type RunState string

const (
	RunPending          RunState = "pending"
	RunRunning          RunState = "running"
	RunAwaitingApproval RunState = "awaiting_approval"
	RunDone             RunState = "done"
	RunFailed           RunState = "failed"
	RunCancelled        RunState = "cancelled"
)

// TurnState is the lifecycle of a single LLM-plus-tool exchange. Turns are
// short-lived: they either finish cleanly, fail, or stay pending while the
// run hands off to an approval gate (the run, not the turn, holds the wait).
type TurnState string

const (
	TurnPending  TurnState = "pending"
	TurnRunning  TurnState = "running"
	TurnFinished TurnState = "finished"
	TurnFailed   TurnState = "failed"
)

// IsTaskTerminal reports whether a TaskState is final. Terminal tasks must
// not transition further; the engine guards Submit / Resume against this.
func IsTaskTerminal(s TaskState) bool {
	return s == TaskDone || s == TaskFailed || s == TaskCancelled
}

// IsRunTerminal reports whether a RunState is final. Used by the engine and
// the persistence layer to short-circuit further writes.
func IsRunTerminal(s RunState) bool {
	return s == RunDone || s == RunFailed || s == RunCancelled
}

// ValidateTaskTransition returns nil iff the transition is allowed. The rule
// table mirrors the comment on Task.State in runtime.go and is the single
// source of truth for legal state movement; both the engine and the store
// should call this before persisting any change so a misordered call site
// fails loudly instead of corrupting state.
func ValidateTaskTransition(from, to TaskState) error {
	if IsTaskTerminal(from) {
		return fmt.Errorf("task already terminal: %s", from)
	}
	switch from {
	case TaskQueued:
		if to != TaskRunning && to != TaskCancelled {
			return fmt.Errorf("queued->%s invalid", to)
		}
	case TaskRunning:
		if to != TaskPaused && to != TaskDone && to != TaskFailed && to != TaskCancelled {
			return fmt.Errorf("running->%s invalid", to)
		}
	case TaskPaused:
		if to != TaskRunning && to != TaskCancelled {
			return fmt.Errorf("paused->%s invalid", to)
		}
	default:
		// Catches typos / unknown raw strings cast into TaskState. Without this
		// the function would silently accept any from-state not in the case
		// list above (e.g. a misspelled "runing" or a future state added
		// elsewhere) and let an invalid transition through.
		return fmt.Errorf("unknown task state transition: %s -> %s", from, to)
	}
	return nil
}

// ValidateRunTransition returns nil iff the transition is allowed. Same
// guarantees as ValidateTaskTransition; AwaitingApproval is the only state
// that may resume back to Running, mirroring the approval-gate semantics in
// the plan document (Phase 2 §approval_required).
func ValidateRunTransition(from, to RunState) error {
	if IsRunTerminal(from) {
		return fmt.Errorf("run already terminal: %s", from)
	}
	switch from {
	case RunPending:
		if to != RunRunning && to != RunCancelled {
			return fmt.Errorf("pending->%s invalid", to)
		}
	case RunRunning:
		if to != RunAwaitingApproval && to != RunDone && to != RunFailed && to != RunCancelled {
			return fmt.Errorf("running->%s invalid", to)
		}
	case RunAwaitingApproval:
		if to != RunRunning && to != RunFailed && to != RunCancelled {
			return fmt.Errorf("awaiting_approval->%s invalid", to)
		}
	default:
		// Same defensive default as ValidateTaskTransition: unknown from-state
		// (typo or future addition not yet covered here) must fail loudly so
		// the bad write surfaces in the engine path instead of silently
		// succeeding.
		return fmt.Errorf("unknown run state transition: %s -> %s", from, to)
	}
	return nil
}
