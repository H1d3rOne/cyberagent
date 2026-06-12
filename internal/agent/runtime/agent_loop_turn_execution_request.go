package runtime

// AgentLoopTurnExecutionRequest builds a runtime-neutral AgentLoopRequest for
// turn-execution callers that only need the surrounding request context.
func AgentLoopTurnExecutionRequest(turn TurnContext, language, operator string, toolServices AgentLoopToolServiceBinding, metadata map[string]any) AgentLoopRequest {
	return AgentLoopRequest{
		Turn:         CloneAgentLoopTurnContext(turn),
		Language:     language,
		Operator:     operator,
		ToolServices: toolServices.Clone(),
		Metadata:     CloneAgentLoopMapStringAny(metadata),
	}
}

// AgentLoopActionExecutionRequest builds a runtime-neutral AgentLoopRequest for
// planner action execution callers, including cloned authorization context.
func AgentLoopActionExecutionRequest(turn TurnContext, language, operator string, toolServices AgentLoopToolServiceBinding, authorization AgentLoopAuthorization, metadata map[string]any) AgentLoopRequest {
	req := AgentLoopTurnExecutionRequest(turn, language, operator, toolServices, metadata)
	req.Authorization = authorization.Clone()
	return req
}

// AgentLoopPlannerActionExecutionRequest builds an AgentLoop request for a
// single planner action execution mode. It reuses the existing action
// execution request DTOs while carrying the action explicitly on the loop
// request for AgentLoopRunner dispatch.
func AgentLoopPlannerActionExecutionRequest(turn TurnContext, language, operator string, toolServices AgentLoopToolServiceBinding, authorization AgentLoopAuthorization, action AgentLoopAction, metadata map[string]any) AgentLoopRequest {
	req := AgentLoopActionExecutionRequest(turn, language, operator, toolServices, authorization, metadata)
	req.Mode = AgentLoopModePlannerActionExecution
	req.Action = action.Clone()
	return req
}
