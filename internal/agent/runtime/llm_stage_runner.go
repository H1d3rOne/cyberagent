package runtime

import (
	"context"
	"errors"
)

var (
	// ErrLLMStageRunnerNotAttached is returned when RunLLMStage is called
	// before a daemon/engine-native LLM stage runner has been attached.
	ErrLLMStageRunnerNotAttached = errors.New("runtime: llm stage runner not attached")
)

// LLMStage identifies one bounded step inside an LLM turn. The initial
// migration slice uses planner_chat; tool_synthesis is reserved for the next
// split of the legacy daemon loop.
type LLMStage string

const (
	LLMStagePlannerChat   LLMStage = "planner_chat"
	LLMStageToolSynthesis LLMStage = "tool_synthesis"
)

// LLMStageRequest is the engine-owned boundary for one bounded LLM stage.
// Turn carries stable runtime identifiers; Payload remains a migration adapter
// for daemon-private planner/session types until those types move runtime-side.
type LLMStageRequest struct {
	Turn     TurnContext
	Stage    LLMStage
	Payload  any
	Metadata map[string]any
}

// LLMStageResult reports a bounded LLM stage outcome. Payload is reserved for
// migration adapters that still return daemon-private message/tool-call/plan
// types.
type LLMStageResult struct {
	Status  TurnResultStatus
	Reason  string
	Error   error
	Payload any
	Data    map[string]any
}

// LLMStageRunner executes one bounded stage inside an LLM turn.
type LLMStageRunner interface {
	RunLLMStage(ctx context.Context, req LLMStageRequest) (LLMStageResult, error)
}

// LLMStageRunnerFunc adapts a function into LLMStageRunner for tests and
// narrow migration adapters.
type LLMStageRunnerFunc func(context.Context, LLMStageRequest) (LLMStageResult, error)

func (f LLMStageRunnerFunc) RunLLMStage(ctx context.Context, req LLMStageRequest) (LLMStageResult, error) {
	if f == nil {
		return missingLLMStageRunnerResult(ErrLLMStageRunnerNotAttached), ErrLLMStageRunnerNotAttached
	}
	return f(ctx, req)
}

// AttachLLMStageRunner sets the runner used by RunLLMStage. Passing nil is a
// supported detach operation and restores the explicit unsupported result.
func (e *Engine) AttachLLMStageRunner(runner LLMStageRunner) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.llmStageRunner = runner
	e.mu.Unlock()
}

// LLMStageRunner returns the currently attached LLM stage runner, if any.
func (e *Engine) LLMStageRunner() LLMStageRunner {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.llmStageRunner
}

// RunLLMStage dispatches an LLMStageRequest to the attached runner. It does not
// emit lifecycle events by itself; BeginTurn/FinishTurn own persisted state.
func (e *Engine) RunLLMStage(ctx context.Context, req LLMStageRequest) (LLMStageResult, error) {
	if e == nil {
		return missingLLMStageRunnerResult(ErrNilEngine), ErrNilEngine
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runner := e.LLMStageRunner()
	if runner == nil {
		return missingLLMStageRunnerResult(ErrLLMStageRunnerNotAttached), ErrLLMStageRunnerNotAttached
	}
	return runner.RunLLMStage(ctx, req)
}

func missingLLMStageRunnerResult(err error) LLMStageResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return LLMStageResult{
		Status: TurnResultBlocked,
		Reason: reason,
		Error:  err,
	}
}
