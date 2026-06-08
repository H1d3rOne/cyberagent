package runtime

// AgentLoopActionGateKind identifies the runtime-neutral authorization or
// capability gate outcome for one planner action.
type AgentLoopActionGateKind string

const (
	AgentLoopActionGateAllow                       AgentLoopActionGateKind = "allow"
	AgentLoopActionGateCapabilityBlocked           AgentLoopActionGateKind = "capability_blocked"
	AgentLoopActionGateExplicitIntentRequired      AgentLoopActionGateKind = "explicit_operator_intent_required"
	AgentLoopActionGateOperationLevelRequired      AgentLoopActionGateKind = "operation_level_required"
	AgentLoopActionGateManualAuthorizationRequired AgentLoopActionGateKind = "manual_tool_authorization_required"
)

// AgentLoopActionGateDecision is the side-effect-free action gate decision. It
// intentionally carries only runtime-neutral strings so daemon adapters can
// decide and then orchestrate request_authorization/capability_blocked
// mutations without exposing daemon-private action structs.
type AgentLoopActionGateDecision struct {
	Kind            AgentLoopActionGateKind
	ActionName      string
	RequiredLevel   string
	Policy          string
	Reason          string
	IntentID        string
	SuggestedPrompt string
}

// Clone returns a value copy for consistency with runtime result DTOs.
func (decision AgentLoopActionGateDecision) Clone() AgentLoopActionGateDecision {
	return decision
}

// CloneAgentLoopActionGateDecision clones an action gate decision.
func CloneAgentLoopActionGateDecision(decision AgentLoopActionGateDecision) AgentLoopActionGateDecision {
	return decision.Clone()
}

// AgentLoopActionGateResult is the runtime-neutral result of applying one gate
// decision to the current loop. Side effects (for now) are still executed by
// daemon SessionMutator adapters, while their operator-visible output is
// returned here.
type AgentLoopActionGateResult struct {
	Decision              AgentLoopActionGateDecision
	Messages              []AgentLoopMessage
	ToolCalls             []AgentLoopToolCall
	AuthorizationRequired bool
	Handled               bool
}

// Clone returns a copy with mutable message/tool-call fields isolated.
func (result AgentLoopActionGateResult) Clone() AgentLoopActionGateResult {
	cloned := result
	cloned.Decision = result.Decision.Clone()
	cloned.Messages = CloneAgentLoopMessages(result.Messages)
	cloned.ToolCalls = CloneAgentLoopToolCalls(result.ToolCalls)
	return cloned
}

// CloneAgentLoopActionGateResult clones one action gate result.
func CloneAgentLoopActionGateResult(result AgentLoopActionGateResult) AgentLoopActionGateResult {
	return result.Clone()
}

// CloneAgentLoopActionGateResultPtr clones an optional action gate result.
func CloneAgentLoopActionGateResultPtr(result *AgentLoopActionGateResult) *AgentLoopActionGateResult {
	if result == nil {
		return nil
	}
	cloned := result.Clone()
	return &cloned
}

// AgentLoopActionBatchResult is the runtime-neutral outcome of executing a
// bounded planner action batch. Daemon adapters still own tool execution and
// session mutation side effects during migration, but the batch egress can move
// across runtime seams without daemon-private accumulator structs.
type AgentLoopActionBatchResult struct {
	Messages              []AgentLoopMessage
	ToolCalls             []AgentLoopToolCall
	AuthorizationRequired bool
	StopGate              *AgentLoopActionGateResult
	ActionCount           int
	ProcessedActionCount  int
	Truncated             bool
	Metadata              map[string]any
}

// Clone returns a copy with mutable result fields isolated.
func (result AgentLoopActionBatchResult) Clone() AgentLoopActionBatchResult {
	cloned := result
	cloned.Messages = CloneAgentLoopMessages(result.Messages)
	cloned.ToolCalls = CloneAgentLoopToolCalls(result.ToolCalls)
	cloned.StopGate = CloneAgentLoopActionGateResultPtr(result.StopGate)
	cloned.Metadata = CloneAgentLoopMapStringAny(result.Metadata)
	return cloned
}

// CloneAgentLoopActionBatchResult clones an action batch result.
func CloneAgentLoopActionBatchResult(result AgentLoopActionBatchResult) AgentLoopActionBatchResult {
	return result.Clone()
}
