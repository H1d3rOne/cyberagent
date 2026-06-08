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
