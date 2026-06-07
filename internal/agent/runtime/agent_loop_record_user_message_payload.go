package runtime

// AgentLoopRecordUserMessagePayload is the runtime-neutral session mutation
// payload for recording an operator prompt as a user message. SessionID,
// prompt, and operation level are carried by SessionMutationRequest.Turn.
type AgentLoopRecordUserMessagePayload struct {
	Operator string
	Language string
}

// Clone returns a value copy for consistency with runtime mutation payloads.
func (payload AgentLoopRecordUserMessagePayload) Clone() AgentLoopRecordUserMessagePayload {
	return payload
}

// CloneAgentLoopRecordUserMessagePayload clones a record-user-message mutation payload.
func CloneAgentLoopRecordUserMessagePayload(payload AgentLoopRecordUserMessagePayload) AgentLoopRecordUserMessagePayload {
	return payload.Clone()
}
