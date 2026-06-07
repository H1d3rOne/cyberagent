package runtime

// AgentLoopControlSessionPayload is the runtime-neutral session mutation payload
// for operator control actions. SessionID, prompt, and operation level are
// carried by SessionMutationRequest.Turn.
type AgentLoopControlSessionPayload struct {
	Action     string
	Reason     string
	RunID      string
	JobID      string
	ClearInput bool
	Operator   string
	Language   string
}

// Clone returns a value copy for consistency with runtime mutation payloads.
func (payload AgentLoopControlSessionPayload) Clone() AgentLoopControlSessionPayload {
	return payload
}

// CloneAgentLoopControlSessionPayload clones a control-session mutation payload.
func CloneAgentLoopControlSessionPayload(payload AgentLoopControlSessionPayload) AgentLoopControlSessionPayload {
	return payload.Clone()
}
