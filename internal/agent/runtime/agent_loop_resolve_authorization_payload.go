package runtime

// AgentLoopResolveAuthorizationPayload is the runtime-neutral session mutation
// payload for approving or denying a pending authorization. SessionID and
// operation level are carried by SessionMutationRequest.Turn.
type AgentLoopResolveAuthorizationPayload struct {
	Decision               string
	AuthorizationID        string
	Reason                 string
	Operator               string
	Language               string
	OperationLevelProvided bool
}

// Clone returns a value copy for consistency with runtime mutation payloads.
func (payload AgentLoopResolveAuthorizationPayload) Clone() AgentLoopResolveAuthorizationPayload {
	return payload
}

// CloneAgentLoopResolveAuthorizationPayload clones a resolve-authorization mutation payload.
func CloneAgentLoopResolveAuthorizationPayload(payload AgentLoopResolveAuthorizationPayload) AgentLoopResolveAuthorizationPayload {
	return payload.Clone()
}
