package runtime

// AgentLoopLLMStagePayload is the runtime-neutral payload for one bounded LLM
// stage inside the agent loop. ToolMessages is populated for tool_synthesis and
// left empty for planner_chat.
type AgentLoopLLMStagePayload struct {
	Turn          TurnContext
	Language      string
	LLMRequest    AgentLoopLLMRequest
	History       []AgentLoopMessage
	MemorySummary *AgentLoopMemorySummary
	ToolMessages  []AgentLoopMessage
}

// Clone returns a copy with runtime loop DTO mutable fields isolated. The
// TurnContext opaque Payload field is intentionally copied as-is; runtime loop
// DTOs should carry typed fields outside Turn.Payload.
func (payload AgentLoopLLMStagePayload) Clone() AgentLoopLLMStagePayload {
	cloned := payload
	cloned.Turn = CloneAgentLoopTurnContext(payload.Turn)
	cloned.LLMRequest = payload.LLMRequest.Clone()
	cloned.History = CloneAgentLoopMessages(payload.History)
	cloned.MemorySummary = CloneAgentLoopMemorySummary(payload.MemorySummary)
	cloned.ToolMessages = CloneAgentLoopMessages(payload.ToolMessages)
	return cloned
}

// CloneAgentLoopLLMStagePayload clones a bounded LLM stage payload.
func CloneAgentLoopLLMStagePayload(payload AgentLoopLLMStagePayload) AgentLoopLLMStagePayload {
	return payload.Clone()
}

// AgentLoopLLMStageResultPayload is the runtime-neutral result payload for one
// bounded LLM stage. Daemon adapters convert it back to legacy
// message/tool-call/planner response types at compatibility boundaries.
type AgentLoopLLMStageResultPayload struct {
	Messages  []AgentLoopMessage
	ToolCalls []AgentLoopToolCall
	Plan      *AgentLoopPlan
}

// Clone returns a copy with message/tool-call/plan mutable fields isolated.
func (payload AgentLoopLLMStageResultPayload) Clone() AgentLoopLLMStageResultPayload {
	return AgentLoopLLMStageResultPayload{
		Messages:  CloneAgentLoopMessages(payload.Messages),
		ToolCalls: CloneAgentLoopToolCalls(payload.ToolCalls),
		Plan:      CloneAgentLoopPlan(payload.Plan),
	}
}

// CloneAgentLoopLLMStageResultPayload clones a bounded LLM stage result payload.
func CloneAgentLoopLLMStageResultPayload(payload AgentLoopLLMStageResultPayload) AgentLoopLLMStageResultPayload {
	return payload.Clone()
}
