package runtime

import "testing"

func TestAgentLoopReplanCycleApplyActionAppendUpdatesStateAndClones(t *testing.T) {
	state := AgentLoopState{
		ResponseMessages: []AgentLoopMessage{{ID: "msg-existing-response"}},
		ToolCalls:        []AgentLoopToolCall{{ID: "tool-existing", Result: map[string]any{"existing": true}}},
		LoopHistory:      []AgentLoopMessage{{ID: "msg-existing-history"}},
	}
	messages := []AgentLoopMessage{{
		ID:      "msg-action",
		Role:    "tool",
		Content: "action completed",
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-embedded-action",
			Metadata: map[string]any{"phase": "step"},
		}},
	}}
	toolCalls := []AgentLoopToolCall{{
		ID:       "tool-action",
		Result:   map[string]any{"ok": true},
		Metadata: map[string]any{"phase": "step"},
	}}

	decision := AgentLoopReplanCycleApplyActionAppend(state, messages, toolCalls, false, "planner_loop")

	if decision.Kind != AgentLoopReplanCycleAppendAction ||
		!decision.Append ||
		decision.AppendWithPlanner ||
		decision.Origin != "planner_loop" ||
		decision.Stage != AgentLoopReplanCycleAppendStageStep ||
		!decision.UpdateHistory {
		t.Fatalf("unexpected action decision metadata: %#v", decision)
	}
	if got, want := messageIDs(decision.UpdatedState.ResponseMessages), []string{"msg-existing-response", "msg-action"}; !equalStrings(got, want) {
		t.Fatalf("response messages = %#v, want %#v", got, want)
	}
	if got, want := messageIDs(decision.UpdatedState.LoopHistory), []string{"msg-existing-history", "msg-action"}; !equalStrings(got, want) {
		t.Fatalf("loop history = %#v, want %#v", got, want)
	}
	if got, want := toolCallIDs(decision.UpdatedState.ToolCalls), []string{"tool-existing", "tool-action"}; !equalStrings(got, want) {
		t.Fatalf("tool calls = %#v, want %#v", got, want)
	}

	state.ResponseMessages[0].ID = "mutated-state"
	state.ToolCalls[0].Result["existing"] = false
	messages[0].Content = "mutated-message"
	messages[0].ToolCalls[0].Metadata["phase"] = "mutated"
	toolCalls[0].Result["ok"] = false
	toolCalls[0].Metadata["phase"] = "mutated"

	if decision.UpdatedState.ResponseMessages[0].ID != "msg-existing-response" ||
		decision.Messages[0].Content != "action completed" ||
		decision.Messages[0].ToolCalls[0].Metadata["phase"] != "step" ||
		decision.ToolCalls[0].Result["ok"] != true ||
		decision.ToolCalls[0].Metadata["phase"] != "step" {
		t.Fatalf("decision shares mutable inputs: %#v", decision)
	}
}

func TestAgentLoopReplanCycleApplyActionAppendPreservesEmptyMessageToolCallTiming(t *testing.T) {
	toolCalls := []AgentLoopToolCall{{ID: "tool-empty-action"}}

	before := AgentLoopReplanCycleApplyActionAppend(AgentLoopState{}, nil, toolCalls, true, "planner_loop")
	if before.Append {
		t.Fatalf("empty action messages should not request append: %#v", before)
	}
	if got, want := toolCallIDs(before.UpdatedState.ToolCalls), []string{"tool-empty-action"}; !equalStrings(got, want) {
		t.Fatalf("before-empty tool calls = %#v, want %#v", got, want)
	}

	after := AgentLoopReplanCycleApplyActionAppend(AgentLoopState{}, nil, toolCalls, false, "planner_loop")
	if after.Append {
		t.Fatalf("empty action messages should not request append: %#v", after)
	}
	if len(after.UpdatedState.ToolCalls) != 0 || len(after.ToolCalls) != 0 {
		t.Fatalf("after-empty tool calls should not be appended: %#v", after)
	}
}

func TestAgentLoopReplanCycleApplyWarningAppendIsResponseOnly(t *testing.T) {
	state := AgentLoopState{
		ResponseMessages: []AgentLoopMessage{{ID: "msg-action"}},
		ToolCalls:        []AgentLoopToolCall{{ID: "tool-action"}},
		LoopHistory:      []AgentLoopMessage{{ID: "msg-history"}, {ID: "msg-action"}},
	}
	warningMessages := []AgentLoopMessage{{ID: "msg-warning", Role: "tool", Content: "synthesis failed"}}
	warningToolCalls := []AgentLoopToolCall{{ID: "tool-warning"}}

	decision := AgentLoopReplanCycleApplyWarningAppend(state, warningMessages, warningToolCalls, "planner_loop")

	if decision.Kind != AgentLoopReplanCycleAppendWarning ||
		!decision.Append ||
		decision.AppendWithPlanner ||
		decision.UpdateHistory ||
		decision.Stage != AgentLoopReplanCycleAppendStageStepReplan {
		t.Fatalf("unexpected warning decision metadata: %#v", decision)
	}
	if got, want := messageIDs(decision.UpdatedState.ResponseMessages), []string{"msg-action", "msg-warning"}; !equalStrings(got, want) {
		t.Fatalf("response messages = %#v, want %#v", got, want)
	}
	if got, want := messageIDs(decision.UpdatedState.LoopHistory), []string{"msg-history", "msg-action"}; !equalStrings(got, want) {
		t.Fatalf("loop history = %#v, want %#v", got, want)
	}
	if got, want := toolCallIDs(decision.UpdatedState.ToolCalls), []string{"tool-action", "tool-warning"}; !equalStrings(got, want) {
		t.Fatalf("tool calls = %#v, want %#v", got, want)
	}
}

func TestAgentLoopReplanCycleApplySynthesisAppendUpdatesHistoryAndPlan(t *testing.T) {
	state := AgentLoopState{
		ResponseMessages: []AgentLoopMessage{{ID: "msg-action"}},
		ToolCalls:        []AgentLoopToolCall{{ID: "tool-action"}},
		LoopHistory:      []AgentLoopMessage{{ID: "msg-history"}, {ID: "msg-action"}},
	}
	synthesisMessages := []AgentLoopMessage{{ID: "msg-synthesis", Role: "agent", Content: "continue"}}
	synthesisToolCalls := []AgentLoopToolCall{{ID: "tool-synthesis", Result: map[string]any{"ok": true}}}
	plan := &AgentLoopPlan{
		Phase: "step_replan",
		Actions: []AgentLoopAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "next"},
		}},
	}

	decision := AgentLoopReplanCycleApplySynthesisAppend(state, synthesisMessages, synthesisToolCalls, plan)

	if decision.Kind != AgentLoopReplanCycleAppendSynthesis ||
		!decision.Append ||
		!decision.AppendWithPlanner ||
		decision.Origin != string(LLMStageToolSynthesis) ||
		decision.Stage != AgentLoopReplanCycleAppendStageStepReplan ||
		!decision.UpdateHistory {
		t.Fatalf("unexpected synthesis decision metadata: %#v", decision)
	}
	if got, want := messageIDs(decision.UpdatedState.ResponseMessages), []string{"msg-action", "msg-synthesis"}; !equalStrings(got, want) {
		t.Fatalf("response messages = %#v, want %#v", got, want)
	}
	if got, want := messageIDs(decision.UpdatedState.LoopHistory), []string{"msg-history", "msg-action", "msg-synthesis"}; !equalStrings(got, want) {
		t.Fatalf("loop history = %#v, want %#v", got, want)
	}
	if got, want := toolCallIDs(decision.UpdatedState.ToolCalls), []string{"tool-action", "tool-synthesis"}; !equalStrings(got, want) {
		t.Fatalf("tool calls = %#v, want %#v", got, want)
	}

	plan.Actions[0].Arguments["probe"] = "mutated"
	synthesisToolCalls[0].Result["ok"] = false
	if decision.Plan.Actions[0].Arguments["probe"] != "next" || decision.ToolCalls[0].Result["ok"] != true {
		t.Fatalf("synthesis decision shares mutable inputs: %#v", decision)
	}
}

func TestAgentLoopReplanCycleApplySynthesisAppendKeepsPlanWhenNoMessages(t *testing.T) {
	plan := &AgentLoopPlan{Phase: "final_answer", Answer: "done"}

	decision := AgentLoopReplanCycleApplySynthesisAppend(AgentLoopState{}, nil, []AgentLoopToolCall{{ID: "tool-unused"}}, plan)

	if decision.Append || decision.AppendWithPlanner || decision.UpdateHistory {
		t.Fatalf("empty synthesis messages should not append: %#v", decision)
	}
	if len(decision.UpdatedState.ResponseMessages) != 0 || len(decision.UpdatedState.ToolCalls) != 0 || len(decision.UpdatedState.LoopHistory) != 0 {
		t.Fatalf("empty synthesis messages should not mutate state: %#v", decision.UpdatedState)
	}
	if decision.Plan == nil || decision.Plan.Answer != "done" {
		t.Fatalf("synthesis plan was not preserved: %#v", decision.Plan)
	}
}

func messageIDs(messages []AgentLoopMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func toolCallIDs(calls []AgentLoopToolCall) []string {
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		ids = append(ids, call.ID)
	}
	return ids
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
