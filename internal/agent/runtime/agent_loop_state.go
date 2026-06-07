package runtime

// AgentLoopState is the runtime-neutral mutable state carried across planner
// action/replan cycles. Daemon-specific session structs convert at legacy
// ingress/egress boundaries.
type AgentLoopState struct {
	ResponseMessages []AgentLoopMessage
	ToolCalls        []AgentLoopToolCall
	LoopHistory      []AgentLoopMessage
}

// Clone returns a deep-enough copy for runtime loop DTO boundaries.
func (state AgentLoopState) Clone() AgentLoopState {
	return AgentLoopState{
		ResponseMessages: CloneAgentLoopMessages(state.ResponseMessages),
		ToolCalls:        CloneAgentLoopToolCalls(state.ToolCalls),
		LoopHistory:      CloneAgentLoopMessages(state.LoopHistory),
	}
}

// CloneAgentLoopState clones planner action/replan cycle state.
func CloneAgentLoopState(state AgentLoopState) AgentLoopState {
	return state.Clone()
}
