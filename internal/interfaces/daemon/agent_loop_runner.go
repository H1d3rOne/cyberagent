package daemon

import (
	"context"
	"errors"
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonAgentLoopRunner struct {
	manager *agentManager
}

func (r daemonAgentLoopRunner) RunAgentLoop(ctx context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
	if r.manager == nil {
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        "daemon agent loop runner manager not attached",
		}, fmt.Errorf("daemon agent loop runner manager not attached")
	}

	switch req.Mode {
	case runtime.AgentLoopModeLLMToolLoop:
		return r.manager.runAgentLLMToolLoopAgentLoopResult(ctx, req)
	case runtime.AgentLoopModeReplanContinuation:
		return r.manager.runAgentReplanActionContinuationAgentLoopResult(ctx, req), nil
	case runtime.AgentLoopModePlannerActionReplanCycle:
		return r.manager.runAgentPlannerActionReplanCycleAgentLoop(ctx, req), nil
	default:
		err := fmt.Errorf("unsupported agent loop mode %q", req.Mode)
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
		}, err
	}
}

func (m *agentManager) runRuntimeAgentLoopResult(ctx context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
	req = req.Clone()
	eng := m.runtimeEngine()
	if eng == nil || eng.AgentLoopRunner() == nil {
		return m.runDaemonAgentLoopDirectResult(ctx, req)
	}
	result, err := eng.RunAgentLoop(ctx, req)
	if errors.Is(err, runtime.ErrAgentLoopRunnerNotAttached) {
		return m.runDaemonAgentLoopDirectResult(ctx, req)
	}
	return result, err
}

func (m *agentManager) runDaemonAgentLoopDirectResult(ctx context.Context, req runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
	switch req.Mode {
	case runtime.AgentLoopModeLLMToolLoop:
		return m.runAgentLLMToolLoopAgentLoopResult(ctx, req)
	case runtime.AgentLoopModeReplanContinuation:
		return m.runAgentReplanActionContinuationAgentLoopResult(ctx, req), nil
	case runtime.AgentLoopModePlannerActionReplanCycle:
		return m.runAgentPlannerActionReplanCycleAgentLoop(ctx, req), nil
	default:
		err := fmt.Errorf("unsupported agent loop mode %q", req.Mode)
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
		}, err
	}
}
