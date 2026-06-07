package runtime

// AgentLoopToolServiceBinding is an opaque runtime boundary handle for tool
// services needed by daemon compatibility adapters. Engine-native runners
// should prefer typed runtime services over Handle; during migration the daemon
// adapter stores its legacy service bundle in Handle and clones only metadata.
type AgentLoopToolServiceBinding struct {
	Kind     string
	Handle   any
	Metadata map[string]any
}

// Clone returns a copy with mutable metadata isolated. Handle is intentionally
// copied as-is so compatibility adapters keep the exact service references
// (jobs, run service, approval service, provider store, etc.) they were bound
// with.
func (binding AgentLoopToolServiceBinding) Clone() AgentLoopToolServiceBinding {
	cloned := binding
	cloned.Metadata = CloneAgentLoopMapStringAny(binding.Metadata)
	return cloned
}

// CloneAgentLoopToolServiceBinding clones a tool service binding boundary
// handle.
func CloneAgentLoopToolServiceBinding(binding AgentLoopToolServiceBinding) AgentLoopToolServiceBinding {
	return binding.Clone()
}
