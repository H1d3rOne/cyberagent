package runtime

import "testing"

func TestAgentLoopEgressResultInjectsMetadataAndClones(t *testing.T) {
	egress := AgentLoopEgressResult{
		State: AgentLoopState{
			ResponseMessages: []AgentLoopMessage{{ID: "msg-egress-state", Content: "state"}},
			ToolCalls:        []AgentLoopToolCall{{ID: "tool-egress-state", Result: map[string]any{"ok": true}}},
		},
		FinalPlan:  &AgentLoopPlan{Phase: "step_replan", Actions: []AgentLoopAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "egress"}}}},
		StopReason: AgentLoopStopReasonMaxLoops,
		Continue:   false,
		LoopCount:  2,
		MaxLoops:   2,
		Metadata:   map[string]any{"mode": string(AgentLoopModeLLMToolLoop)},
	}

	result := egress.AgentLoopResult()
	egress.State.ResponseMessages[0].Content = "mutated"
	egress.State.ToolCalls[0].Result["ok"] = false
	egress.FinalPlan.Actions[0].Arguments["probe"] = "mutated"
	egress.Metadata["mode"] = "mutated"

	if result.StopReason != AgentLoopStopReasonMaxLoops || result.LoopCount != 2 {
		t.Fatalf("unexpected egress scalar fields: %#v", result)
	}
	if result.Metadata[AgentLoopEgressMetadataStopReason] != AgentLoopStopReasonMaxLoops ||
		result.Metadata[AgentLoopEgressMetadataContinue] != false ||
		result.Metadata[AgentLoopEgressMetadataLoopCount] != 2 ||
		result.Metadata[AgentLoopEgressMetadataMaxLoops] != 2 ||
		result.Metadata["mode"] != string(AgentLoopModeLLMToolLoop) {
		t.Fatalf("missing normalized egress metadata: %#v", result.Metadata)
	}
	if result.Messages[0].Content != "state" || result.ToolCalls[0].Result["ok"] != true {
		t.Fatalf("egress result shares state-backed fields: %#v %#v", result.Messages, result.ToolCalls)
	}
	if result.FinalPlan == nil || result.FinalPlan.Actions[0].Arguments["probe"] != "egress" {
		t.Fatalf("egress result shares final plan: %#v", result.FinalPlan)
	}
}
