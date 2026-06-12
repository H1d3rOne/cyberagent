package runtime

const (
	// AgentLoopTurnExecutionExecutorMetadataKey stores the executor adapter that
	// dispatched a runtime turn.
	AgentLoopTurnExecutionExecutorMetadataKey = "executor"
	// AgentLoopTurnExecutionExecutorMetadataValue is the stable executor label for
	// runtime turn execution requests.
	AgentLoopTurnExecutionExecutorMetadataValue = "turn_execution"
	// AgentLoopTurnExecutionKindMetadataKey stores the turn execution kind.
	AgentLoopTurnExecutionKindMetadataKey = "kind"
	// AgentLoopTurnExecutionOperationLevelMetadataKey stores the active operation
	// level for the runtime turn.
	AgentLoopTurnExecutionOperationLevelMetadataKey = "operation_level"
	// AgentLoopTurnExecutionMaxLoopsMetadataKey stores the loop bound for turn
	// execution requests that run or resume a bounded loop.
	AgentLoopTurnExecutionMaxLoopsMetadataKey = "max_loops"
)

// AgentLoopTurnExecutionMetadata clones existing turn metadata and stamps the
// runtime-neutral turn execution fields. Extra maps are merged after the base
// executor/kind/operation_level fields.
func AgentLoopTurnExecutionMetadata(metadata map[string]any, kind, operationLevel string, metadataMaps ...map[string]any) map[string]any {
	base := map[string]any{
		AgentLoopTurnExecutionExecutorMetadataKey:       AgentLoopTurnExecutionExecutorMetadataValue,
		AgentLoopTurnExecutionKindMetadataKey:           kind,
		AgentLoopTurnExecutionOperationLevelMetadataKey: operationLevel,
	}
	return MergeAgentLoopMetadata(append([]map[string]any{metadata, base}, metadataMaps...)...)
}

// AgentLoopTurnExecutionTurnFromRequest builds a TurnContext for turn
// execution callers whose payload is the surrounding AgentLoopRequest.
func AgentLoopTurnExecutionTurnFromRequest(req AgentLoopRequest, kind string, metadataMaps ...map[string]any) TurnContext {
	return AgentLoopTurnExecutionTurnWithPayload(req, kind, req.Clone(), metadataMaps...)
}

// AgentLoopTurnExecutionBoundaryFromRequest builds a TurnContext for
// turn-execution loop boundaries without replacing the request turn payload.
func AgentLoopTurnExecutionBoundaryFromRequest(req AgentLoopRequest, kind string, metadataMaps ...map[string]any) TurnContext {
	return AgentLoopTurnExecutionTurnWithPayload(req, kind, req.Turn.Payload, metadataMaps...)
}

// AgentLoopTurnExecutionTurnWithPayload builds a TurnContext for turn
// execution callers while keeping payload ownership explicit. The returned turn
// clones request turn metadata, stamps turn-execution metadata, sets the origin
// to kind, and normalizes an empty turn index to 1.
func AgentLoopTurnExecutionTurnWithPayload(req AgentLoopRequest, kind string, payload any, metadataMaps ...map[string]any) TurnContext {
	turn := CloneAgentLoopTurnContext(req.Turn)
	turn.Origin = kind
	if turn.TurnIndex == 0 {
		turn.TurnIndex = 1
	}
	turn.Payload = payload
	turn.Metadata = AgentLoopTurnExecutionMetadata(turn.Metadata, kind, turn.OperationLevel, metadataMaps...)
	return turn
}
