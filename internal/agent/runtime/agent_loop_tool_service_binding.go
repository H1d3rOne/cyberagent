package runtime

// AgentLoopToolServiceBinding is the runtime boundary for tool/action services
// needed by daemon compatibility adapters. Engine-native runners should prefer
// ActionServices; Handle remains an opaque compatibility input for older daemon
// adapters that cannot yet consume the capability set directly.
type AgentLoopToolServiceBinding struct {
	Kind           string
	Handle         any
	ActionServices AgentLoopActionServiceSet
	Metadata       map[string]any
}

// Clone returns a copy with mutable metadata isolated. Handle is intentionally
// copied as-is so explicit compatibility adapters keep the exact service
// references they were bound with.
func (binding AgentLoopToolServiceBinding) Clone() AgentLoopToolServiceBinding {
	cloned := binding
	cloned.ActionServices = binding.ActionServices.Clone()
	cloned.Metadata = CloneAgentLoopMapStringAny(binding.Metadata)
	return cloned
}

// CloneAgentLoopToolServiceBinding clones a tool service binding boundary
// handle.
func CloneAgentLoopToolServiceBinding(binding AgentLoopToolServiceBinding) AgentLoopToolServiceBinding {
	return binding.Clone()
}
