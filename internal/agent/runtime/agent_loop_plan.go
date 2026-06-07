package runtime

// AgentLoopPlan is the runtime-neutral planner/replan envelope carried between
// bounded LLM, tool execution, and session mutation stages. Daemon-specific
// planner JSON parsing and legacy session adapters convert at the edge.
type AgentLoopPlan struct {
	Phase                 string
	Answer                string
	SuggestedPrompt       string
	SuggestPrompt         string
	SuggestedPrompts      []string
	RecommendationOptions []AgentLoopRecommendationOption
	Task                  *AgentLoopPlanTask
	Todos                 []AgentLoopPlanTodo
	ContextRefs           []AgentLoopContextRef
	NextAction            *AgentLoopNextAction
	Reasoning             string
	Actions               []AgentLoopAction
	ToolCalls             []AgentLoopAction
}

// AgentLoopRecommendationOption is an operator-visible suggested next step.
type AgentLoopRecommendationOption struct {
	ID      string
	Title   string
	Detail  string
	Prompt  string
	Command string
}

// AgentLoopPlanTask is the task summary emitted by a planner loop.
type AgentLoopPlanTask struct {
	Goal        string
	Kind        string
	Status      string
	ActiveRunID string
	Summary     string
}

// AgentLoopPlanTodo is one planner checklist item.
type AgentLoopPlanTodo struct {
	Title         string
	Detail        string
	Status        string
	Priority      int
	ContextRefIDs []string
	BlockedReason string
}

// AgentLoopContextRef points planner output at evidence, files, or run state.
type AgentLoopContextRef struct {
	ID      string
	Kind    string
	RefID   string
	Label   string
	To      string
	Summary string
}

// AgentLoopNextAction describes the next planner loop decision.
type AgentLoopNextAction struct {
	Type            string
	Reason          string
	SuggestedPrompt string
	AutoContinue    bool
}

// AgentLoopAction is a planner-emitted tool/action request.
type AgentLoopAction struct {
	Name      string
	Arguments map[string]any
}

// Clone returns a deep-enough copy for runtime loop DTO boundaries. Maps are
// cloned at the top level, matching the daemon legacy planner action semantics.
func (plan AgentLoopPlan) Clone() AgentLoopPlan {
	cloned := plan
	cloned.SuggestedPrompts = append([]string(nil), plan.SuggestedPrompts...)
	cloned.RecommendationOptions = CloneAgentLoopRecommendationOptions(plan.RecommendationOptions)
	cloned.Task = CloneAgentLoopPlanTask(plan.Task)
	cloned.Todos = CloneAgentLoopPlanTodos(plan.Todos)
	cloned.ContextRefs = CloneAgentLoopContextRefs(plan.ContextRefs)
	cloned.NextAction = CloneAgentLoopNextAction(plan.NextAction)
	cloned.Actions = CloneAgentLoopActions(plan.Actions)
	cloned.ToolCalls = CloneAgentLoopActions(plan.ToolCalls)
	return cloned
}

// CloneAgentLoopPlan clones an optional runtime-neutral planner envelope.
func CloneAgentLoopPlan(plan *AgentLoopPlan) *AgentLoopPlan {
	if plan == nil {
		return nil
	}
	cloned := plan.Clone()
	return &cloned
}

// CloneAgentLoopRecommendationOptions clones recommendation option slices.
func CloneAgentLoopRecommendationOptions(options []AgentLoopRecommendationOption) []AgentLoopRecommendationOption {
	if options == nil {
		return nil
	}
	out := make([]AgentLoopRecommendationOption, len(options))
	copy(out, options)
	return out
}

// CloneAgentLoopPlanTask clones an optional task summary.
func CloneAgentLoopPlanTask(task *AgentLoopPlanTask) *AgentLoopPlanTask {
	if task == nil {
		return nil
	}
	cloned := *task
	return &cloned
}

// CloneAgentLoopPlanTodos clones todo slices and their context ref ID slices.
func CloneAgentLoopPlanTodos(todos []AgentLoopPlanTodo) []AgentLoopPlanTodo {
	if todos == nil {
		return nil
	}
	out := make([]AgentLoopPlanTodo, len(todos))
	copy(out, todos)
	for i := range todos {
		out[i].ContextRefIDs = append([]string(nil), todos[i].ContextRefIDs...)
	}
	return out
}

// CloneAgentLoopContextRefs clones context reference slices.
func CloneAgentLoopContextRefs(refs []AgentLoopContextRef) []AgentLoopContextRef {
	if refs == nil {
		return nil
	}
	out := make([]AgentLoopContextRef, len(refs))
	copy(out, refs)
	return out
}

// CloneAgentLoopNextAction clones an optional next action.
func CloneAgentLoopNextAction(nextAction *AgentLoopNextAction) *AgentLoopNextAction {
	if nextAction == nil {
		return nil
	}
	cloned := *nextAction
	return &cloned
}

// CloneAgentLoopActions clones action slices and their top-level argument maps.
func CloneAgentLoopActions(actions []AgentLoopAction) []AgentLoopAction {
	if actions == nil {
		return nil
	}
	out := make([]AgentLoopAction, len(actions))
	for i, action := range actions {
		out[i] = AgentLoopAction{
			Name:      action.Name,
			Arguments: CloneAgentLoopActionArguments(action.Arguments),
		}
	}
	return out
}

// CloneAgentLoopActionArguments clones a top-level action argument map.
func CloneAgentLoopActionArguments(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}
