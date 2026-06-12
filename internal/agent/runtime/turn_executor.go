package runtime

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrTurnExecutorNotAttached is returned when ExecuteTurn is called before
	// a daemon/engine-native executor has been attached.
	ErrTurnExecutorNotAttached = errors.New("runtime: turn executor not attached")

	// ErrNilEngine is returned by nil-safe Engine turn-executor methods.
	ErrNilEngine = errors.New("runtime: nil engine")
)

// TurnResultStatus is the high-level outcome reported by a turn executor. The
// status values intentionally mirror the daemon's existing turn completion
// strings so the seam can be introduced without changing UI/runtime behavior.
type TurnResultStatus string

const (
	TurnResultRunning          TurnResultStatus = "running"
	TurnResultFinished         TurnResultStatus = "finished"
	TurnResultFailed           TurnResultStatus = "failed"
	TurnResultApprovalRequired TurnResultStatus = "approval_required"
	TurnResultBlocked          TurnResultStatus = "blocked"
)

// TurnContext is the engine-owned boundary object for one turn execution. It
// carries stable identifiers plus a temporary opaque payload for migration
// adapters while daemon-private planner/session/service types move
// runtime-side.
type TurnContext struct {
	SessionID      string
	TaskID         string
	RunID          string
	TurnID         string
	MessageID      string
	Prompt         string
	Goal           string
	Origin         string
	OperationLevel string
	TurnIndex      int
	Timestamp      time.Time
	Payload        any
	Metadata       map[string]any
}

// TurnResult is the executor's summary of a turn. Error is kept alongside the
// returned error so callers that persist results can preserve the original
// failure text without relying on error wrapping.
type TurnResult struct {
	Status  TurnResultStatus
	Reason  string
	Error   error
	Payload any
	Data    map[string]any
}

// TurnExecutor executes a single runtime turn. The first attached
// implementation is the daemon legacy adapter; engine-native execution can
// replace it later without changing Engine callers.
//
// Deprecated: prefer AgentLoopRunner with Engine.RunAgentLoop for
// deterministic-route, replan-action-continuation, and
// planner-action-execution integrations. TurnExecutor remains as an active
// external compatibility seam for existing ExecuteTurn callers until that
// migration gate clears.
type TurnExecutor interface {
	ExecuteTurn(ctx context.Context, turn TurnContext) (TurnResult, error)
}

// TurnExecutorFunc adapts a function into TurnExecutor for tests and narrow
// adapters.
//
// Deprecated: use AgentLoopRunnerFunc or AgentLoopActionExecutorFunc for new
// bounded agent-loop adapters.
type TurnExecutorFunc func(context.Context, TurnContext) (TurnResult, error)

func (f TurnExecutorFunc) ExecuteTurn(ctx context.Context, turn TurnContext) (TurnResult, error) {
	if f == nil {
		return missingTurnExecutorResult(ErrTurnExecutorNotAttached), ErrTurnExecutorNotAttached
	}
	return f(ctx, turn)
}

// AttachTurnExecutor sets the executor used by ExecuteTurn. Passing nil is a
// supported detach operation and restores the explicit unsupported result.
//
// Deprecated: attach an AgentLoopRunner/AgentLoopActionExecutor and call
// Engine.RunAgentLoop for new bounded agent-loop integrations.
func (e *Engine) AttachTurnExecutor(executor TurnExecutor) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.turnExecutor = executor
	e.mu.Unlock()
}

// TurnExecutor returns the currently attached executor, if any.
//
// Deprecated: use AgentLoopRunner or AgentLoopActionExecutor for new bounded
// agent-loop integrations.
func (e *Engine) TurnExecutor() TurnExecutor {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.turnExecutor
}

// ExecuteTurn dispatches a TurnContext to the attached executor. It does not
// emit lifecycle events by itself; BeginTurn/FinishTurn own the persisted
// lifecycle state while the executor only runs one turn.
//
// Compatibility: new AgentLoop-shaped turn execution should prefer
// Engine.RunAgentLoop when the caller already has an AgentLoopRequest.
//
// Deprecated: prefer Engine.RunAgentLoop for AgentLoop-shaped callers; this
// method is retained as an active runtime API compatibility seam.
func (e *Engine) ExecuteTurn(ctx context.Context, turn TurnContext) (TurnResult, error) {
	if e == nil {
		return missingTurnExecutorResult(ErrNilEngine), ErrNilEngine
	}
	if ctx == nil {
		ctx = context.Background()
	}
	executor := e.TurnExecutor()
	if executor == nil {
		return missingTurnExecutorResult(ErrTurnExecutorNotAttached), ErrTurnExecutorNotAttached
	}
	return executor.ExecuteTurn(ctx, turn)
}

func missingTurnExecutorResult(err error) TurnResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return TurnResult{
		Status: TurnResultBlocked,
		Reason: reason,
		Error:  err,
	}
}
