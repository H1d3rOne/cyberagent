package runtime

import (
	"context"
	"errors"
)

var (
	// ErrLLMTurnRunnerNotAttached is returned when RunLLMTurn is called before
	// a daemon/engine-native LLM turn runner has been attached.
	ErrLLMTurnRunnerNotAttached = errors.New("runtime: llm turn runner not attached")
)

// LLMTurnRequest is the temporary engine-owned boundary for the legacy daemon
// LLM tool loop. Turn carries stable runtime identifiers; Payload is reserved
// for migration adapters while daemon-private planner/session types are still
// outside the runtime package.
type LLMTurnRequest struct {
	Turn     TurnContext
	Payload  any
	Metadata map[string]any
}

// LLMTurnResult reports the LLM tool loop outcome. Payload is reserved for
// migration adapters that still return daemon-private message/tool-call types.
type LLMTurnResult struct {
	Status  TurnResultStatus
	Reason  string
	Error   error
	Payload any
	Data    map[string]any
}

// LLMTurnRunner executes the LLM planner/tool/replan loop for one turn.
//
// Deprecated: prefer AgentLoopRunner with Engine.RunAgentLoop for new bounded
// agent-loop integrations. LLMTurnRunner remains as an active compatibility
// seam for existing daemon LLMTurn callers until that migration gate clears.
type LLMTurnRunner interface {
	RunLLMTurn(ctx context.Context, req LLMTurnRequest) (LLMTurnResult, error)
}

// LLMTurnRunnerFunc adapts a function into LLMTurnRunner for tests and narrow
// migration adapters.
//
// Deprecated: use AgentLoopRunnerFunc for new bounded agent-loop adapters.
type LLMTurnRunnerFunc func(context.Context, LLMTurnRequest) (LLMTurnResult, error)

func (f LLMTurnRunnerFunc) RunLLMTurn(ctx context.Context, req LLMTurnRequest) (LLMTurnResult, error) {
	if f == nil {
		return missingLLMTurnRunnerResult(ErrLLMTurnRunnerNotAttached), ErrLLMTurnRunnerNotAttached
	}
	return f(ctx, req)
}

// AttachLLMTurnRunner sets the runner used by RunLLMTurn. Passing nil is a
// supported detach operation and restores the explicit unsupported result.
//
// Deprecated: attach an AgentLoopRunner and call Engine.RunAgentLoop for new
// bounded agent-loop integrations.
func (e *Engine) AttachLLMTurnRunner(runner LLMTurnRunner) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.llmTurnRunner = runner
	e.mu.Unlock()
}

// LLMTurnRunner returns the currently attached LLM turn runner, if any.
//
// Deprecated: use AgentLoopRunner for new bounded agent-loop integrations.
func (e *Engine) LLMTurnRunner() LLMTurnRunner {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.llmTurnRunner
}

// RunLLMTurn dispatches an LLMTurnRequest to the attached runner. It does not
// emit lifecycle events by itself; BeginTurn/FinishTurn own persisted state.
//
// Deprecated: prefer Engine.RunAgentLoop for new bounded agent-loop callers;
// this method is retained as an active runtime API compatibility seam.
func (e *Engine) RunLLMTurn(ctx context.Context, req LLMTurnRequest) (LLMTurnResult, error) {
	if e == nil {
		return missingLLMTurnRunnerResult(ErrNilEngine), ErrNilEngine
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runner := e.LLMTurnRunner()
	if runner == nil {
		return missingLLMTurnRunnerResult(ErrLLMTurnRunnerNotAttached), ErrLLMTurnRunnerNotAttached
	}
	return runner.RunLLMTurn(ctx, req)
}

func missingLLMTurnRunnerResult(err error) LLMTurnResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return LLMTurnResult{
		Status: TurnResultBlocked,
		Reason: reason,
		Error:  err,
	}
}
