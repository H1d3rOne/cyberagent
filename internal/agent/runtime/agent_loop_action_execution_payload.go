package runtime

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
