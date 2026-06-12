package runtime

// AgentLoopReplanCycleAppendKind identifies the shared planner
// action/replan-cycle append decision being applied.
type AgentLoopReplanCycleAppendKind string

const (
	AgentLoopReplanCycleAppendAction    AgentLoopReplanCycleAppendKind = "action"
	AgentLoopReplanCycleAppendWarning   AgentLoopReplanCycleAppendKind = "warning"
	AgentLoopReplanCycleAppendSynthesis AgentLoopReplanCycleAppendKind = "synthesis"
)

const (
	AgentLoopReplanCycleAppendStageStep       = "step"
	AgentLoopReplanCycleAppendStageStepReplan = "step_replan"
)

// AgentLoopReplanCycleAppendDecision is the runtime-owned append/persist
// placement decision for one shared planner action/replan cycle edge. Daemon
// adapters consume this DTO to dispatch SessionMutationAppendMessages and
// convert legacy message/plan structs, while the state mutation choice remains
// side-effect-free and runtime-owned.
type AgentLoopReplanCycleAppendDecision struct {
	Kind              AgentLoopReplanCycleAppendKind
	Append            bool
	AppendWithPlanner bool
	Origin            string
	Stage             string
	Plan              *AgentLoopPlan
	Messages          []AgentLoopMessage
	ToolCalls         []AgentLoopToolCall
	UpdateHistory     bool
	UpdatedState      AgentLoopState
}

// Clone returns a copy with all mutable DTO fields isolated.
func (decision AgentLoopReplanCycleAppendDecision) Clone() AgentLoopReplanCycleAppendDecision {
	cloned := decision
	cloned.Plan = CloneAgentLoopPlan(decision.Plan)
	cloned.Messages = CloneAgentLoopMessages(decision.Messages)
	cloned.ToolCalls = CloneAgentLoopToolCalls(decision.ToolCalls)
	cloned.UpdatedState = decision.UpdatedState.Clone()
	return cloned
}

// CloneAgentLoopReplanCycleAppendDecision clones a shared-cycle append
// decision.
func CloneAgentLoopReplanCycleAppendDecision(decision AgentLoopReplanCycleAppendDecision) AgentLoopReplanCycleAppendDecision {
	return decision.Clone()
}

// AgentLoopReplanCycleApplyActionAppend applies the legacy shared-cycle action
// append placement rules without side effects:
//   - non-empty action messages are appended to response messages and loop
//     history;
//   - action tool calls are appended before the empty-message stop only when
//     appendToolCallsBeforeEmptyMessage is true;
//   - otherwise action tool calls are appended only after action messages are
//     present.
func AgentLoopReplanCycleApplyActionAppend(state AgentLoopState, messages []AgentLoopMessage, toolCalls []AgentLoopToolCall, appendToolCallsBeforeEmptyMessage bool, origin string) AgentLoopReplanCycleAppendDecision {
	updated := state.Clone()
	clonedMessages := CloneAgentLoopMessages(messages)
	appendMessages := len(clonedMessages) > 0
	var appendedToolCalls []AgentLoopToolCall

	if appendMessages {
		updated.ResponseMessages = append(updated.ResponseMessages, CloneAgentLoopMessages(clonedMessages)...)
		updated.LoopHistory = append(updated.LoopHistory, CloneAgentLoopMessages(clonedMessages)...)
	}
	if appendToolCallsBeforeEmptyMessage || appendMessages {
		appendedToolCalls = CloneAgentLoopToolCalls(toolCalls)
		updated.ToolCalls = append(updated.ToolCalls, CloneAgentLoopToolCalls(appendedToolCalls)...)
	}

	return AgentLoopReplanCycleAppendDecision{
		Kind:          AgentLoopReplanCycleAppendAction,
		Append:        appendMessages,
		Origin:        origin,
		Stage:         AgentLoopReplanCycleAppendStageStep,
		Messages:      clonedMessages,
		ToolCalls:     appendedToolCalls,
		UpdateHistory: appendMessages,
		UpdatedState:  updated,
	}.Clone()
}

// AgentLoopReplanCycleApplyWarningAppend applies the warning append placement
// used when action execution succeeded but StepReplan synthesis failed. Warning
// messages are response-only and do not extend loop history.
func AgentLoopReplanCycleApplyWarningAppend(state AgentLoopState, messages []AgentLoopMessage, toolCalls []AgentLoopToolCall, origin string) AgentLoopReplanCycleAppendDecision {
	updated := state.Clone()
	clonedMessages := CloneAgentLoopMessages(messages)
	appendMessages := len(clonedMessages) > 0
	var appendedToolCalls []AgentLoopToolCall

	if appendMessages {
		appendedToolCalls = CloneAgentLoopToolCalls(toolCalls)
		updated.ResponseMessages = append(updated.ResponseMessages, CloneAgentLoopMessages(clonedMessages)...)
		updated.ToolCalls = append(updated.ToolCalls, CloneAgentLoopToolCalls(appendedToolCalls)...)
	}

	return AgentLoopReplanCycleAppendDecision{
		Kind:          AgentLoopReplanCycleAppendWarning,
		Append:        appendMessages,
		Origin:        origin,
		Stage:         AgentLoopReplanCycleAppendStageStepReplan,
		Messages:      clonedMessages,
		ToolCalls:     appendedToolCalls,
		UpdateHistory: false,
		UpdatedState:  updated,
	}.Clone()
}

// AgentLoopReplanCycleApplySynthesisAppend applies the shared-cycle synthesis
// append placement. Synthesis messages are appended with planner state through
// the tool_synthesis mutation origin and extend both response messages and loop
// history. The final plan is cloned into the decision even when no messages are
// appended so callers can preserve current final-plan egress semantics.
func AgentLoopReplanCycleApplySynthesisAppend(state AgentLoopState, messages []AgentLoopMessage, toolCalls []AgentLoopToolCall, plan *AgentLoopPlan) AgentLoopReplanCycleAppendDecision {
	updated := state.Clone()
	clonedMessages := CloneAgentLoopMessages(messages)
	appendMessages := len(clonedMessages) > 0
	var appendedToolCalls []AgentLoopToolCall

	if appendMessages {
		appendedToolCalls = CloneAgentLoopToolCalls(toolCalls)
		updated.ResponseMessages = append(updated.ResponseMessages, CloneAgentLoopMessages(clonedMessages)...)
		updated.LoopHistory = append(updated.LoopHistory, CloneAgentLoopMessages(clonedMessages)...)
		updated.ToolCalls = append(updated.ToolCalls, CloneAgentLoopToolCalls(appendedToolCalls)...)
	}

	return AgentLoopReplanCycleAppendDecision{
		Kind:              AgentLoopReplanCycleAppendSynthesis,
		Append:            appendMessages,
		AppendWithPlanner: appendMessages,
		Origin:            string(LLMStageToolSynthesis),
		Stage:             AgentLoopReplanCycleAppendStageStepReplan,
		Plan:              CloneAgentLoopPlan(plan),
		Messages:          clonedMessages,
		ToolCalls:         appendedToolCalls,
		UpdateHistory:     appendMessages,
		UpdatedState:      updated,
	}.Clone()
}
