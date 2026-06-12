package runtime

import (
	"context"
	"errors"
)

var (
	// ErrAgentLoopActionExecutorNotAttached is returned when ExecuteAgentLoopAction
	// is called before a runtime-native planner action executor has been attached.
	ErrAgentLoopActionExecutorNotAttached = errors.New("runtime: agent loop action executor not attached")
)

// AgentLoopActionExecutorRequest is the runtime-native contract for executing
// one planner action. It keeps the existing AgentLoopRequest/action DTOs intact
// so daemon compatibility adapters can move behind this seam before the
// remaining opaque tool-service Handle is replaced with typed services.
type AgentLoopActionExecutorRequest struct {
	Request AgentLoopRequest
	Action  AgentLoopAction
}

// Clone returns a copy with request/action mutable fields isolated.
func (req AgentLoopActionExecutorRequest) Clone() AgentLoopActionExecutorRequest {
	return AgentLoopActionExecutorRequest{
		Request: req.Request.Clone(),
		Action:  req.Action.Clone(),
	}
}

// AgentLoopActionExecutorResult is the runtime-native result for one planner
// action execution. Messages/tool calls are runtime DTOs so callers do not need
// daemon-private agentMessage/agentToolCall egress adapters at this boundary.
type AgentLoopActionExecutorResult struct {
	Status    TurnResultStatus
	Reason    string
	Error     error
	Messages  []AgentLoopMessage
	ToolCalls []AgentLoopToolCall
	Metadata  map[string]any
}

// Clone returns a copy with message/tool-call/metadata fields isolated.
func (result AgentLoopActionExecutorResult) Clone() AgentLoopActionExecutorResult {
	cloned := result
	cloned.Messages = CloneAgentLoopMessages(result.Messages)
	cloned.ToolCalls = CloneAgentLoopToolCalls(result.ToolCalls)
	cloned.Metadata = CloneAgentLoopMapStringAny(result.Metadata)
	return cloned
}

// AgentLoopActionExecutor executes one runtime planner action. It is the typed
// service seam that planner_action_execution can move to before the broader
// AgentLoop mode switch.
type AgentLoopActionExecutor interface {
	ExecuteAgentLoopAction(ctx context.Context, req AgentLoopActionExecutorRequest) (AgentLoopActionExecutorResult, error)
}

// AgentLoopActionExecutorFunc adapts a function into AgentLoopActionExecutor
// for tests and narrow migration adapters.
type AgentLoopActionExecutorFunc func(context.Context, AgentLoopActionExecutorRequest) (AgentLoopActionExecutorResult, error)

func (f AgentLoopActionExecutorFunc) ExecuteAgentLoopAction(ctx context.Context, req AgentLoopActionExecutorRequest) (AgentLoopActionExecutorResult, error) {
	if f == nil {
		return missingAgentLoopActionExecutorResult(ErrAgentLoopActionExecutorNotAttached), ErrAgentLoopActionExecutorNotAttached
	}
	return f(ctx, req)
}

// AttachAgentLoopActionExecutor sets the executor used by ExecuteAgentLoopAction.
// Passing nil is a supported detach operation and restores the explicit
// unsupported result.
func (e *Engine) AttachAgentLoopActionExecutor(executor AgentLoopActionExecutor) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.agentLoopActionExecutor = executor
	e.mu.Unlock()
}

// AgentLoopActionExecutor returns the currently attached action executor, if any.
func (e *Engine) AgentLoopActionExecutor() AgentLoopActionExecutor {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.agentLoopActionExecutor
}

// ExecuteAgentLoopAction dispatches one planner action through the attached
// runtime-native action executor. It clones request/result mutable DTO fields
// while preserving documented opaque compatibility handles inside
// AgentLoopToolServiceBinding.
func (e *Engine) ExecuteAgentLoopAction(ctx context.Context, req AgentLoopActionExecutorRequest) (AgentLoopActionExecutorResult, error) {
	if e == nil {
		return missingAgentLoopActionExecutorResult(ErrNilEngine), ErrNilEngine
	}
	if ctx == nil {
		ctx = context.Background()
	}
	executor := e.AgentLoopActionExecutor()
	if executor == nil {
		return missingAgentLoopActionExecutorResult(ErrAgentLoopActionExecutorNotAttached), ErrAgentLoopActionExecutorNotAttached
	}
	result, err := executor.ExecuteAgentLoopAction(ctx, req.Clone())
	return result.Clone(), err
}

func missingAgentLoopActionExecutorResult(err error) AgentLoopActionExecutorResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return AgentLoopActionExecutorResult{
		Status: TurnResultBlocked,
		Reason: reason,
		Error:  err,
	}
}
