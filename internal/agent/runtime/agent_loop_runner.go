package runtime

import (
	"context"
	"errors"
)

var (
	// ErrAgentLoopRunnerNotAttached is returned when RunAgentLoop is called
	// before a daemon/engine-native agent loop runner has been attached.
	ErrAgentLoopRunnerNotAttached = errors.New("runtime: agent loop runner not attached")
)

// AgentLoopRunner executes one bounded agent loop step. The migration starts
// by attaching the daemon legacy shared-cycle adapter; later slices can move
// ownership into Engine without changing callers.
type AgentLoopRunner interface {
	RunAgentLoop(ctx context.Context, req AgentLoopRequest) (AgentLoopResult, error)
}

// AgentLoopRunnerFunc adapts a function into AgentLoopRunner for tests and
// narrow migration adapters.
type AgentLoopRunnerFunc func(context.Context, AgentLoopRequest) (AgentLoopResult, error)

func (f AgentLoopRunnerFunc) RunAgentLoop(ctx context.Context, req AgentLoopRequest) (AgentLoopResult, error) {
	if f == nil {
		return missingAgentLoopRunnerResult(ErrAgentLoopRunnerNotAttached), ErrAgentLoopRunnerNotAttached
	}
	return f(ctx, req)
}

// AttachAgentLoopRunner sets the runner used by RunAgentLoop. Passing nil is a
// supported detach operation and restores the explicit unsupported result.
func (e *Engine) AttachAgentLoopRunner(runner AgentLoopRunner) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.agentLoopRunner = runner
	e.mu.Unlock()
}

// AgentLoopRunner returns the currently attached agent loop runner, if any.
func (e *Engine) AgentLoopRunner() AgentLoopRunner {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.agentLoopRunner
}

// RunAgentLoop dispatches an AgentLoopRequest to the attached runner. It keeps
// the loop DTO boundary isolated by cloning request/result mutable fields while
// preserving documented opaque handle references such as Turn.Payload and tool
// service Handle.
func (e *Engine) RunAgentLoop(ctx context.Context, req AgentLoopRequest) (AgentLoopResult, error) {
	if e == nil {
		return missingAgentLoopRunnerResult(ErrNilEngine), ErrNilEngine
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runner := e.AgentLoopRunner()
	if runner == nil {
		return missingAgentLoopRunnerResult(ErrAgentLoopRunnerNotAttached), ErrAgentLoopRunnerNotAttached
	}
	result, err := runner.RunAgentLoop(ctx, req.Clone())
	return result.Clone(), err
}

func missingAgentLoopRunnerResult(err error) AgentLoopResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return AgentLoopResult{
		StopReason:        reason,
		CapabilityBlocked: true,
		Metadata: map[string]any{
			"error": reason,
		},
	}
}
