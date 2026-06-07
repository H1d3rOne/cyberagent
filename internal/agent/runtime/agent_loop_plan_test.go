package runtime

import "testing"

func TestAgentLoopPlanCloneIsolatesMutableFields(t *testing.T) {
	plan := AgentLoopPlan{
		Phase:            "step_replan",
		Answer:           "answer",
		SuggestedPrompts: []string{"continue"},
		RecommendationOptions: []AgentLoopRecommendationOption{{
			ID:      "rec-1",
			Title:   "Continue",
			Detail:  "detail",
			Prompt:  "continue",
			Command: "/continue",
		}},
		Task:        &AgentLoopPlanTask{Goal: "goal", ActiveRunID: "run-1", Summary: "summary"},
		Todos:       []AgentLoopPlanTodo{{Title: "todo", ContextRefIDs: []string{"ctx-1"}}},
		ContextRefs: []AgentLoopContextRef{{ID: "ctx-1", To: "message:1", Summary: "ctx summary"}},
		NextAction:  &AgentLoopNextAction{Type: "continue", Reason: "reason", SuggestedPrompt: "next", AutoContinue: true},
		Actions:     []AgentLoopAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "clone"}}},
		ToolCalls:   []AgentLoopAction{{Name: "runs.list", Arguments: map[string]any{"limit": 1}}},
	}

	cloned := plan.Clone()
	plan.SuggestedPrompts[0] = "mutated"
	plan.RecommendationOptions[0].Title = "Mutated"
	plan.Task.Goal = "mutated"
	plan.Todos[0].ContextRefIDs[0] = "ctx-mutated"
	plan.ContextRefs[0].Summary = "mutated"
	plan.NextAction.AutoContinue = false
	plan.Actions[0].Arguments["probe"] = "mutated"
	plan.ToolCalls[0].Arguments["limit"] = 99

	if cloned.SuggestedPrompts[0] != "continue" || cloned.RecommendationOptions[0].Title != "Continue" {
		t.Fatalf("clone shares prompt/recommendation fields: %#v", cloned)
	}
	if cloned.Task.Goal != "goal" || cloned.Todos[0].ContextRefIDs[0] != "ctx-1" || cloned.ContextRefs[0].Summary != "ctx summary" {
		t.Fatalf("clone shares nested plan fields: %#v", cloned)
	}
	if !cloned.NextAction.AutoContinue || cloned.Actions[0].Arguments["probe"] != "clone" || cloned.ToolCalls[0].Arguments["limit"] != 1 {
		t.Fatalf("clone shares next action/action fields: %#v", cloned)
	}
}
