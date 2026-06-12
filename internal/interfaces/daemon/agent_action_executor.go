package daemon

import (
	"context"
	"errors"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonAgentLoopActionExecutor struct {
	manager *agentManager
}

func (executor daemonAgentLoopActionExecutor) ExecuteAgentLoopAction(ctx context.Context, req runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
	if executor.manager == nil {
		err := errors.New("daemon: agent loop action executor manager not attached")
		return runtime.AgentLoopActionExecutorResult{
			Status: runtime.TurnResultBlocked,
			Reason: err.Error(),
			Error:  err,
			Metadata: map[string]any{
				"executor": "daemon_legacy_planner_action_executor",
			},
		}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	boundary := daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop(req.Request, req.Action)
	result, err := daemonPlannerActionExecutionExecutorForManager(executor.manager).ExecutePlannerAction(ctx, boundary)
	return result, err
}
