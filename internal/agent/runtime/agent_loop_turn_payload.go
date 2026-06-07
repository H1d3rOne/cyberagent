package runtime

// AgentLoopTurnPayload is the runtime-neutral payload for a full LLM/tool turn.
// Daemon adapters bind services separately and convert legacy request/history /
// memory types at ingress and egress boundaries.
type AgentLoopTurnPayload struct {
	Turn          TurnContext
	Language      string
	Operator      string
	LLMRequest    AgentLoopLLMRequest
	History       []AgentLoopMessage
	MemorySummary *AgentLoopMemorySummary
}

// Clone returns a copy with runtime loop DTO mutable fields isolated. The
// TurnContext opaque Payload field is intentionally copied as-is; runtime loop
// DTOs should carry typed fields outside Turn.Payload.
func (payload AgentLoopTurnPayload) Clone() AgentLoopTurnPayload {
	cloned := payload
	cloned.Turn = CloneAgentLoopTurnContext(payload.Turn)
	cloned.LLMRequest = payload.LLMRequest.Clone()
	cloned.History = CloneAgentLoopMessages(payload.History)
	cloned.MemorySummary = CloneAgentLoopMemorySummary(payload.MemorySummary)
	return cloned
}

// CloneAgentLoopTurnPayload clones a full LLM/tool turn payload.
func CloneAgentLoopTurnPayload(payload AgentLoopTurnPayload) AgentLoopTurnPayload {
	return payload.Clone()
}

// AgentLoopTurnResultPayload is the runtime-neutral result payload for a full
// LLM/tool turn. Daemon adapters convert it back to legacy message/tool-call
// slices at compatibility boundaries.
type AgentLoopTurnResultPayload struct {
	Messages  []AgentLoopMessage
	ToolCalls []AgentLoopToolCall
}

// Clone returns a copy with message/tool-call mutable fields isolated.
func (payload AgentLoopTurnResultPayload) Clone() AgentLoopTurnResultPayload {
	return AgentLoopTurnResultPayload{
		Messages:  CloneAgentLoopMessages(payload.Messages),
		ToolCalls: CloneAgentLoopToolCalls(payload.ToolCalls),
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
