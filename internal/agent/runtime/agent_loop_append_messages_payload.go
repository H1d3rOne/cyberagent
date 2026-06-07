package runtime

// AgentLoopAppendMessagesPayload is the runtime-neutral session mutation
// payload for appending messages and optionally applying planner state. The
// session identity is carried by SessionMutationRequest.Turn.SessionID.
type AgentLoopAppendMessagesPayload struct {
	Plan     *AgentLoopPlan
	Messages []AgentLoopMessage
}

// Clone returns a copy with message and planner mutable fields isolated.
func (payload AgentLoopAppendMessagesPayload) Clone() AgentLoopAppendMessagesPayload {
	return AgentLoopAppendMessagesPayload{
		Plan:     CloneAgentLoopPlan(payload.Plan),
		Messages: CloneAgentLoopMessages(payload.Messages),
	}
}

// CloneAgentLoopAppendMessagesPayload clones an append-messages mutation payload.
func CloneAgentLoopAppendMessagesPayload(payload AgentLoopAppendMessagesPayload) AgentLoopAppendMessagesPayload {
	return payload.Clone()
}
