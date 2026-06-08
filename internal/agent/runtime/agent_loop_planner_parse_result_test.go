package runtime

import "testing"

func TestAgentLoopPlannerParseResultCloneIsolatesMutableFields(t *testing.T) {
	result := AgentLoopPlannerParseResult{
		Content:      "parsed answer",
		MessagePhase: "plan",
		Plan: &AgentLoopPlan{
			Phase:   "plan",
			Answer:  "parsed answer",
			Actions: []AgentLoopAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "initial"}}},
		},
		Planner:             true,
		Fallback:            true,
		ScheduleOverride:    true,
		NativeToolCallCount: 1,
		ParseError:          "invalid json",
		TodoCount:           2,
		ActionCount:         1,
		Metadata:            map[string]any{"planner": true, "planner_actions": 1},
	}

	cloned := result.Clone()
	result.Plan.Actions[0].Arguments["probe"] = "mutated"
	result.Metadata["planner_actions"] = 9

	if !cloned.Planner || !cloned.Fallback || !cloned.ScheduleOverride || cloned.NativeToolCallCount != 1 || cloned.ParseError != "invalid json" || cloned.TodoCount != 2 || cloned.ActionCount != 1 {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
	if cloned.Plan == nil || cloned.Plan.Actions[0].Arguments["probe"] != "initial" {
		t.Fatalf("clone shares plan: %#v", cloned.Plan)
	}
	if cloned.Metadata["planner_actions"] != 1 {
		t.Fatalf("clone shares metadata: %#v", cloned.Metadata)
	}
}
