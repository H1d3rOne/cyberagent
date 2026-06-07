package runtime

import "testing"

func TestAgentLoopAppendMessagesPayloadCloneIsolatesMutableFields(t *testing.T) {
	payload := AgentLoopAppendMessagesPayload{
		Plan: &AgentLoopPlan{
			Answer: "planned",
			Task:   &AgentLoopPlanTask{Goal: "goal"},
			Todos:  []AgentLoopPlanTodo{{Title: "todo", ContextRefIDs: []string{"ctx-1"}}},
		},
		Messages: []AgentLoopMessage{{
			ID:               "msg-1",
			SuggestedPrompts: []string{"next"},
			ToolCalls:        []AgentLoopToolCall{{ID: "tool-1", Result: map[string]any{"ok": true}}},
		}},
	}

	cloned := payload.Clone()
	payload.Plan.Task.Goal = "changed"
	payload.Plan.Todos[0].ContextRefIDs[0] = "changed"
	payload.Messages[0].SuggestedPrompts[0] = "changed"
	payload.Messages[0].ToolCalls[0].Result["ok"] = false

	if cloned.Plan.Task.Goal != "goal" {
		t.Fatalf("cloned plan task goal = %q, want isolated goal", cloned.Plan.Task.Goal)
	}
	if cloned.Plan.Todos[0].ContextRefIDs[0] != "ctx-1" {
		t.Fatalf("cloned plan todo context ref = %q, want isolated ctx-1", cloned.Plan.Todos[0].ContextRefIDs[0])
	}
	if cloned.Messages[0].SuggestedPrompts[0] != "next" {
		t.Fatalf("cloned message prompt = %q, want isolated next", cloned.Messages[0].SuggestedPrompts[0])
	}
	if cloned.Messages[0].ToolCalls[0].Result["ok"] != true {
		t.Fatalf("cloned tool result = %#v, want isolated ok=true", cloned.Messages[0].ToolCalls[0].Result["ok"])
	}
}
