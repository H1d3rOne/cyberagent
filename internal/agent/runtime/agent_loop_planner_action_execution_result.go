package runtime

// AgentLoopPlannerActionExecutionResultFromActionExecutorResult converts the
// single-action executor egress DTO into the AgentLoop result used by
// planner_action_execution mode. It is runtime-owned so daemon adapters only
// need to provide runtime DTOs after any legacy message/tool-call conversion.
func AgentLoopPlannerActionExecutionResultFromActionExecutorResult(actionName string, result AgentLoopActionExecutorResult, resultErr error) AgentLoopResult {
	messages := CloneAgentLoopMessages(result.Messages)
	toolCalls := CloneAgentLoopToolCalls(result.ToolCalls)
	metadata := CloneAgentLoopMapStringAny(result.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["mode"] = string(AgentLoopModePlannerActionExecution)
	metadata[AgentLoopActionNameMetadataKey] = actionName
	metadata["message_count"] = len(messages)
	metadata["tool_call_count"] = len(toolCalls)
	if result.Status != "" {
		metadata["action_executor_status"] = string(result.Status)
	}
	stopReason := "planner_action_executed"
	if result.Reason != "" {
		stopReason = result.Reason
	}
	if resultErr != nil {
		stopReason = resultErr.Error()
		metadata["error"] = resultErr.Error()
	}
	return AgentLoopResult{
		State: AgentLoopState{
			ResponseMessages: CloneAgentLoopMessages(messages),
			ToolCalls:        CloneAgentLoopToolCalls(toolCalls),
		},
		Messages:          messages,
		ToolCalls:         toolCalls,
		StopReason:        stopReason,
		CapabilityBlocked: result.Status == TurnResultBlocked,
		Metadata:          metadata,
	}
}

// AgentLoopPlannerActionExecutionResultFromTurnPayload converts the legacy
// TurnExecutor compatibility payload into the AgentLoop result used by
// planner_action_execution mode. This keeps compatibility egress composition
// aligned with the runtime action executor path while the TurnExecutor fallback
// still exists.
func AgentLoopPlannerActionExecutionResultFromTurnPayload(actionName string, payload AgentLoopTurnResultPayload, resultErr error) AgentLoopResult {
	messages := CloneAgentLoopMessages(payload.Messages)
	toolCalls := CloneAgentLoopToolCalls(payload.ToolCalls)
	metadata := CloneAgentLoopMapStringAny(payload.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["mode"] = string(AgentLoopModePlannerActionExecution)
	metadata[AgentLoopActionNameMetadataKey] = actionName
	metadata["message_count"] = len(messages)
	metadata["tool_call_count"] = len(toolCalls)
	stopReason := "planner_action_executed"
	if payload.StopReason != "" {
		stopReason = payload.StopReason
	}
	if resultErr != nil {
		stopReason = resultErr.Error()
		metadata["error"] = resultErr.Error()
	}
	return AgentLoopResult{
		State: AgentLoopState{
			ResponseMessages: CloneAgentLoopMessages(messages),
			ToolCalls:        CloneAgentLoopToolCalls(toolCalls),
		},
		Messages:   messages,
		ToolCalls:  toolCalls,
		StopReason: stopReason,
		Metadata:   metadata,
	}
}
