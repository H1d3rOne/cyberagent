package runtime

// AgentLoopAuthorizationRequestPayload is the runtime-neutral session mutation
// payload for creating a pending authorization request. SessionID, operator
// prompt, and operation level are carried by SessionMutationRequest.Turn.
type AgentLoopAuthorizationRequestPayload struct {
	Language      string
	Action        AgentLoopAction
	ActionName    string
	RequiredLevel string
	Policy        string
	Reason        string
}

// Clone returns a copy with action argument maps isolated.
func (payload AgentLoopAuthorizationRequestPayload) Clone() AgentLoopAuthorizationRequestPayload {
	cloned := payload
	cloned.Action = payload.Action.Clone()
	return cloned
}

// CloneAgentLoopAuthorizationRequestPayload clones an authorization request mutation payload.
func CloneAgentLoopAuthorizationRequestPayload(payload AgentLoopAuthorizationRequestPayload) AgentLoopAuthorizationRequestPayload {
	return payload.Clone()
}
