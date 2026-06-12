package daemon

import "github.com/H1d3rOne/cyberagent/internal/agent/runtime"

func agentReplanActionContinuationTurnContextFromRequest(loopReq runtime.AgentLoopRequest) runtime.TurnContext {
	return runtime.AgentLoopTurnExecutionTurnFromRequest(loopReq, daemonTurnExecutionReplanActionContinuation, map[string]any{
		runtime.AgentLoopTurnExecutionMaxLoopsMetadataKey: loopReq.MaxLoops,
	})
}
