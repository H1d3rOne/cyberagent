package runtime

// AgentLoopMode selects the engine-owned loop runner behavior. The public
// outer modes preserve the daemon legacy split while sharing one
// runtime-neutral request / result contract; the cycle mode is an internal
// compatibility sub-mode during migration.
type AgentLoopMode string

const (
	// AgentLoopModeLLMToolLoop starts with planner chat, then runs Plan → Action
	// → StepReplan until the plan stops or reaches MaxLoops.
	AgentLoopModeLLMToolLoop AgentLoopMode = "llm_tool_loop"

	// AgentLoopModeReplanContinuation starts from an existing plan and only
	// continues the Action → StepReplan cycle when auto-continue is allowed.
	AgentLoopModeReplanContinuation AgentLoopMode = "replan_action_continuation"

	// AgentLoopModePlannerActionReplanCycle executes exactly one Action →
	// StepReplan cycle. It is the side-effect compatibility sub-mode used by
	// engine-owned outer loop runners while the action/synthesis adapters still
	// live in daemon.
	AgentLoopModePlannerActionReplanCycle AgentLoopMode = "planner_action_replan_cycle"
)

// AgentLoopRequest is the runtime-neutral boundary for the future
// engine-native LLM/tool/replan loop runner. Current daemon adapters can build
// this DTO without moving execution ownership yet; the opaque Turn.Payload is
// intentionally not used for loop state.
type AgentLoopRequest struct {
	Turn     TurnContext
	Mode     AgentLoopMode
	Language string
	Operator string

	LLMRequest    AgentLoopLLMRequest
	History       []AgentLoopMessage
	MemorySummary *AgentLoopMemorySummary
	InitialPlan   *AgentLoopPlan
	State         AgentLoopState

	Authorization AgentLoopAuthorization
	ToolServices  AgentLoopToolServiceBinding
	MaxLoops      int
	Metadata      map[string]any
}

// Clone returns a copy with all runtime loop DTO mutable fields isolated.
// Service binding Handle and Turn.Payload are preserved by reference, matching
// existing migration boundary semantics.
func (req AgentLoopRequest) Clone() AgentLoopRequest {
	cloned := req
	cloned.Turn = CloneAgentLoopTurnContext(req.Turn)
	cloned.LLMRequest = req.LLMRequest.Clone()
	cloned.History = CloneAgentLoopMessages(req.History)
	cloned.MemorySummary = CloneAgentLoopMemorySummary(req.MemorySummary)
	cloned.InitialPlan = CloneAgentLoopPlan(req.InitialPlan)
	cloned.State = req.State.Clone()
	cloned.Authorization = req.Authorization.Clone()
	cloned.ToolServices = req.ToolServices.Clone()
	cloned.Metadata = CloneAgentLoopMapStringAny(req.Metadata)
	return cloned
}

// CloneAgentLoopRequest clones a loop runner request.
func CloneAgentLoopRequest(req AgentLoopRequest) AgentLoopRequest {
	return req.Clone()
}

// AgentLoopResult is the runtime-neutral outcome for the future engine-native
// LLM/tool/replan loop runner.
type AgentLoopResult struct {
	State                 AgentLoopState
	Messages              []AgentLoopMessage
	ToolCalls             []AgentLoopToolCall
	FinalPlan             *AgentLoopPlan
	StopReason            string
	AuthorizationRequired bool
	CapabilityBlocked     bool
	LoopCount             int
	ActionQueueIDs        []string
	Metadata              map[string]any
}

// Clone returns a copy with all runtime loop result mutable fields isolated.
func (result AgentLoopResult) Clone() AgentLoopResult {
	cloned := result
	cloned.State = result.State.Clone()
	cloned.Messages = CloneAgentLoopMessages(result.Messages)
	cloned.ToolCalls = CloneAgentLoopToolCalls(result.ToolCalls)
	cloned.FinalPlan = CloneAgentLoopPlan(result.FinalPlan)
	cloned.ActionQueueIDs = CloneAgentLoopStringSlice(result.ActionQueueIDs)
	cloned.Metadata = CloneAgentLoopMapStringAny(result.Metadata)
	return cloned
}

// CloneAgentLoopResult clones a loop runner result.
func CloneAgentLoopResult(result AgentLoopResult) AgentLoopResult {
	return result.Clone()
}

// CloneAgentLoopStringSlice clones an optional string slice.
func CloneAgentLoopStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
