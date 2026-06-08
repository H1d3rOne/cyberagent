package runtime

const (
	// AgentLoopEgressMetadataStopReason stores the normalized loop stop reason
	// in result payload metadata for adapters that cannot consume the typed
	// AgentLoopResult directly yet.
	AgentLoopEgressMetadataStopReason = "stop_reason"
	AgentLoopEgressMetadataLoopCount  = "loop_count"
	AgentLoopEgressMetadataMaxLoops   = "max_loops"
	AgentLoopEgressMetadataContinue   = "continue"

	AgentLoopStopReasonPlannerError             = "planner_error"
	AgentLoopStopReasonStepReplanSynthesisError = "step_replan_synthesis_error"
	AgentLoopStopReasonNoActions                = "no_actions"
	AgentLoopStopReasonAutoContinueDisabled     = "auto_continue_disabled"
	AgentLoopStopReasonCycleStopped             = "cycle_stopped"
	AgentLoopStopReasonMaxLoops                 = "max_loops"
	AgentLoopStopReasonLegacyDaemonLoopExecuted = "legacy_daemon_agent_loop_executed"
)

// AgentLoopEgressResult is a runtime-neutral loop egress DTO used by daemon
// adapters while loop ownership is still being migrated. It mirrors the parts
// of AgentLoopResult that are needed by legacy turn payloads and injects a
// uniform stop-reason/loop-count metadata envelope.
type AgentLoopEgressResult struct {
	State      AgentLoopState
	Messages   []AgentLoopMessage
	ToolCalls  []AgentLoopToolCall
	FinalPlan  *AgentLoopPlan
	StopReason string
	Continue   bool
	LoopCount  int
	MaxLoops   int
	Metadata   map[string]any
}

// Clone returns a copy with all mutable fields isolated.
func (result AgentLoopEgressResult) Clone() AgentLoopEgressResult {
	cloned := result
	cloned.State = result.State.Clone()
	cloned.Messages = CloneAgentLoopMessages(result.Messages)
	cloned.ToolCalls = CloneAgentLoopToolCalls(result.ToolCalls)
	cloned.FinalPlan = CloneAgentLoopPlan(result.FinalPlan)
	cloned.Metadata = CloneAgentLoopMapStringAny(result.Metadata)
	return cloned
}

// CloneAgentLoopEgressResult clones an agent loop egress result.
func CloneAgentLoopEgressResult(result AgentLoopEgressResult) AgentLoopEgressResult {
	return result.Clone()
}

// MetadataWithEgressFields clones Metadata and injects the standard egress
// fields. Existing caller metadata is preserved unless overwritten by the
// canonical egress values.
func (result AgentLoopEgressResult) MetadataWithEgressFields() map[string]any {
	metadata := CloneAgentLoopMapStringAny(result.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata[AgentLoopEgressMetadataStopReason] = result.StopReason
	metadata[AgentLoopEgressMetadataContinue] = result.Continue
	metadata[AgentLoopEgressMetadataLoopCount] = result.LoopCount
	metadata[AgentLoopEgressMetadataMaxLoops] = result.MaxLoops
	return metadata
}

// AgentLoopResult converts the egress DTO into the top-level loop result.
func (result AgentLoopEgressResult) AgentLoopResult() AgentLoopResult {
	state := result.State.Clone()
	messages := CloneAgentLoopMessages(result.Messages)
	if len(messages) == 0 && len(state.ResponseMessages) > 0 {
		messages = CloneAgentLoopMessages(state.ResponseMessages)
	}
	toolCalls := CloneAgentLoopToolCalls(result.ToolCalls)
	if len(toolCalls) == 0 && len(state.ToolCalls) > 0 {
		toolCalls = CloneAgentLoopToolCalls(state.ToolCalls)
	}
	return AgentLoopResult{
		State:      state,
		Messages:   messages,
		ToolCalls:  toolCalls,
		FinalPlan:  CloneAgentLoopPlan(result.FinalPlan),
		StopReason: result.StopReason,
		LoopCount:  result.LoopCount,
		Metadata:   result.MetadataWithEgressFields(),
	}
}
