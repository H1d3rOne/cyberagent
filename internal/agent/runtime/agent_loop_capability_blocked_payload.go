package runtime

// AgentLoopCapabilityBlockedPayload is the runtime-neutral session mutation
// payload for marking a session blocked by a capability mismatch. SessionID,
// operator prompt, and operation level are carried by SessionMutationRequest.Turn.
type AgentLoopCapabilityBlockedPayload struct {
	Language        string
	ActionName      string
	IntentID        string
	Reason          string
	SuggestedPrompt string
}

// Clone returns a value copy for consistency with runtime mutation payloads.
func (payload AgentLoopCapabilityBlockedPayload) Clone() AgentLoopCapabilityBlockedPayload {
	return payload
}

// CloneAgentLoopCapabilityBlockedPayload clones a capability-blocked mutation payload.
func CloneAgentLoopCapabilityBlockedPayload(payload AgentLoopCapabilityBlockedPayload) AgentLoopCapabilityBlockedPayload {
	return payload.Clone()
}
