package runtime

// AgentLoopTurnResultPayload is the runtime-neutral result payload for a full
// LLM/tool turn. Daemon adapters convert it back to legacy message/tool-call
// slices at compatibility boundaries.
type AgentLoopTurnResultPayload struct {
	Messages  []AgentLoopMessage
	ToolCalls []AgentLoopToolCall
	FinalPlan *AgentLoopPlan

	StopReason string
	LoopCount  int
	MaxLoops   int
	Metadata   map[string]any
}

// Clone returns a copy with message/tool-call mutable fields isolated.
func (payload AgentLoopTurnResultPayload) Clone() AgentLoopTurnResultPayload {
	return AgentLoopTurnResultPayload{
		Messages:   CloneAgentLoopMessages(payload.Messages),
		ToolCalls:  CloneAgentLoopToolCalls(payload.ToolCalls),
		FinalPlan:  CloneAgentLoopPlan(payload.FinalPlan),
		StopReason: payload.StopReason,
		LoopCount:  payload.LoopCount,
		MaxLoops:   payload.MaxLoops,
		Metadata:   CloneAgentLoopMapStringAny(payload.Metadata),
	}
}

// CloneAgentLoopTurnResultPayload clones a full LLM/tool turn result payload.
func CloneAgentLoopTurnResultPayload(payload AgentLoopTurnResultPayload) AgentLoopTurnResultPayload {
	return payload.Clone()
}

// CloneAgentLoopTurnContext clones mutable metadata on a TurnContext. The
// opaque Payload field is preserved by reference because runtime-neutral loop
// DTOs keep their typed migration payload outside the TurnContext payload.
func CloneAgentLoopTurnContext(turn TurnContext) TurnContext {
	cloned := turn
	cloned.Metadata = CloneAgentLoopMapStringAny(turn.Metadata)
	return cloned
}
