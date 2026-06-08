package runtime

const (
	// AgentLoopActionExecutionExecutorMetadataKey stores the executor adapter
	// that dispatched a planner action execution turn.
	AgentLoopActionExecutionExecutorMetadataKey = "executor"
	// AgentLoopActionExecutionExecutorMetadataValue is the stable executor label
	// for planner action execution turn requests.
	AgentLoopActionExecutionExecutorMetadataValue = "turn_execution"
	// AgentLoopActionExecutionKindMetadataKey stores the turn execution kind.
	AgentLoopActionExecutionKindMetadataKey = "kind"
	// AgentLoopActionExecutionOperationLevelMetadataKey stores the active
	// operation level for the planner action execution turn.
	AgentLoopActionExecutionOperationLevelMetadataKey = "operation_level"
)

// AgentLoopActionExecutionPayload is the runtime-neutral turn executor payload
// for executing one planner action with its surrounding loop request context.
type AgentLoopActionExecutionPayload struct {
	Request AgentLoopRequest
	Action  AgentLoopAction
}

// Clone returns a copy with request and action mutable fields isolated.
func (payload AgentLoopActionExecutionPayload) Clone() AgentLoopActionExecutionPayload {
	return AgentLoopActionExecutionPayload{
		Request: payload.Request.Clone(),
		Action:  payload.Action.Clone(),
	}
}

// CloneAgentLoopActionExecutionPayload clones a planner action execution payload.
func CloneAgentLoopActionExecutionPayload(payload AgentLoopActionExecutionPayload) AgentLoopActionExecutionPayload {
	return payload.Clone()
}

// AgentLoopActionExecutionTurnMetadata clones existing turn metadata and stamps
// the runtime-neutral action execution fields used by daemon turn executors.
func AgentLoopActionExecutionTurnMetadata(metadata map[string]any, kind, operationLevel, actionName string) map[string]any {
	return MergeAgentLoopMetadata(metadata, map[string]any{
		AgentLoopActionExecutionExecutorMetadataKey:       AgentLoopActionExecutionExecutorMetadataValue,
		AgentLoopActionExecutionKindMetadataKey:           kind,
		AgentLoopActionExecutionOperationLevelMetadataKey: operationLevel,
		AgentLoopActionNameMetadataKey:                    actionName,
	})
}
