package daemon

import (
	"context"
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
	case runtime.AgentLoopModeLLMToolLoop, runtime.AgentLoopModeReplanContinuation:
		cycleReq := agentPlannerActionReplanCycleRequestFromAgentLoopRequest(req, agentPlannerActionReplanCycleOptions{})
		cycleResult := r.manager.runAgentPlannerActionReplanCycle(ctx, cycleReq)
		return cycleResult.agentLoopResult("legacy_daemon_agent_loop_executed"), nil
	default:
		err := fmt.Errorf("unsupported agent loop mode %q", req.Mode)
		return runtime.AgentLoopResult{
			CapabilityBlocked: true,
			StopReason:        err.Error(),
		}, err
	}
}
