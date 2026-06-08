package runtime

// AgentLoopPlannerParseResult is the runtime-neutral result of interpreting an
// LLM planner/synthesis response into loop content, an optional plan, and
// planner metadata. Daemon adapters still own provider calls and legacy planner
// parsing during migration, but they can pass this DTO across runtime seams
// without exposing daemon-private planner response structs.
type AgentLoopPlannerParseResult struct {
	Content             string
	MessagePhase        string
	Plan                *AgentLoopPlan
	Planner             bool
	Fallback            bool
	ScheduleOverride    bool
	NativeToolCallCount int
	ParseError          string
	TodoCount           int
	ActionCount         int
	Metadata            map[string]any
}

// Clone returns a copy with mutable plan and metadata fields isolated.
func (result AgentLoopPlannerParseResult) Clone() AgentLoopPlannerParseResult {
	cloned := result
	cloned.Plan = CloneAgentLoopPlan(result.Plan)
	cloned.Metadata = CloneAgentLoopMapStringAny(result.Metadata)
	return cloned
}

// CloneAgentLoopPlannerParseResult clones a planner parse result.
func CloneAgentLoopPlannerParseResult(result AgentLoopPlannerParseResult) AgentLoopPlannerParseResult {
	return result.Clone()
}
