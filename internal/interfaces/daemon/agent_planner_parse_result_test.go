package daemon

import (
	"encoding/json"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentLLMPlannerChatParseResultParsesJSONAndMetadata(t *testing.T) {
	resp := &llm.ChatResponse{Content: []llm.ContentBlock{llm.TextBlock{Text: `{
		"phase":"plan",
		"answer":"planned health check",
		"task":{"goal":"check daemon","kind":"ops","status":"running","summary":"ok"},
		"actions":[{"name":"daemon.health","arguments":{"probe":"json"}}]
	}`}}}

	result := agentLLMPlannerChatParseResult(resp, "", "en")

	require.True(t, result.Planner)
	require.False(t, result.Fallback)
	require.Equal(t, "planned health check", result.Content)
	require.Equal(t, "plan", result.MessagePhase)
	require.Equal(t, 1, result.ActionCount)
	require.Equal(t, true, result.Metadata["planner"])
	require.Equal(t, 1, result.Metadata["planner_actions"])
	require.NotNil(t, result.Plan)
	require.Equal(t, "daemon.health", result.Plan.Actions[0].Name)
	require.Equal(t, "json", result.Plan.Actions[0].Arguments["probe"])
}

func TestAgentLLMPlannerChatParseResultMergesNativeToolUse(t *testing.T) {
	resp := &llm.ChatResponse{Content: []llm.ContentBlock{llm.ToolUseBlock{
		ID:    "toolu-native",
		Name:  "daemon.health",
		Input: json.RawMessage(`{"probe":"native"}`),
	}}}

	result := agentLLMPlannerChatParseResult(resp, "", "en")

	require.False(t, result.Planner)
	require.Equal(t, 1, result.NativeToolCallCount)
	require.Equal(t, 1, result.ActionCount)
	require.Equal(t, "plan", result.MessagePhase)
	require.Contains(t, result.Content, "low-risk built-in tools")
	require.Equal(t, false, result.Metadata["planner"])
	require.Equal(t, 1, result.Metadata["planner_native_tool_calls"])
	require.NotNil(t, result.Plan)
	require.Len(t, result.Plan.ToolCalls, 1)
	require.Equal(t, "daemon.health", result.Plan.ToolCalls[0].Name)
	require.Equal(t, "native", result.Plan.ToolCalls[0].Arguments["probe"])
}

func TestAgentLLMPlannerChatParseResultScheduleOverride(t *testing.T) {
	resp := &llm.ChatResponse{Content: []llm.ContentBlock{llm.TextBlock{Text: `{
		"phase":"plan",
		"answer":"I can help schedule that.",
		"task":{"goal":"schedule work","kind":"schedule","status":"running","summary":"missing action"},
		"todos":[]
	}`}}}

	result := agentLLMPlannerChatParseResult(resp, "每小时扫描 https://example.com", "zh")

	require.True(t, result.Planner)
	require.True(t, result.Fallback)
	require.True(t, result.ScheduleOverride)
	require.Equal(t, "plan", result.MessagePhase)
	require.Equal(t, true, result.Metadata["planner_schedule_override"])
	require.Equal(t, 1, result.Metadata["planner_actions"])
	require.NotNil(t, result.Plan)
	actions := agentLoopPlanActions(result.Plan)
	require.Len(t, actions, 1)
	require.Equal(t, "schedule.create", actions[0].Name)
}

func TestAgentLLMToolSynthesisParseResultPlainAndEmptyFallback(t *testing.T) {
	plain := agentLLMToolSynthesisParseResult(&llm.ChatResponse{Content: []llm.ContentBlock{llm.TextBlock{Text: "All observations are complete."}}}, "en")

	require.True(t, plain.Planner)
	require.True(t, plain.Fallback)
	require.Equal(t, "All observations are complete.", plain.Content)
	require.Equal(t, "final_answer", plain.MessagePhase)
	require.Equal(t, true, plain.Metadata["planner"])
	require.Equal(t, true, plain.Metadata["planner_fallback"])
	require.NotNil(t, plain.Plan)

	empty := agentLLMToolSynthesisParseResult(&llm.ChatResponse{}, "en")
	require.False(t, empty.Planner)
	require.Equal(t, "final_answer", empty.MessagePhase)
	require.Contains(t, empty.Content, "empty synthesis")
	require.Equal(t, false, empty.Metadata["planner"])
	_, hasTodoMetadata := empty.Metadata["planner_todos"]
	require.False(t, hasTodoMetadata)
	require.NotNil(t, empty.Plan)
	require.Equal(t, "final_answer", empty.Plan.Phase)
}

func TestAgentLLMStageResultPayloadFromPlannerParseBuildsRuntimeDTO(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	tool := agentToolCall{
		ID:     "tool-planner",
		Name:   "LLM chat",
		Result: map[string]any{"planner": true},
	}
	parseResult := runtime.AgentLoopPlannerParseResult{
		Content:      "planned response",
		MessagePhase: "plan",
		Plan: &runtime.AgentLoopPlan{
			Phase:     "plan",
			Answer:    "planned response",
			Reasoning: "reasoning summary",
			Actions: []runtime.AgentLoopAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"probe": "runtime"},
			}},
		},
		Metadata: map[string]any{"planner": true},
	}

	payload := mgr.agentLLMStageResultPayloadFromPlannerParse("en", tool, parseResult, "step_replan")

	require.Len(t, payload.Messages, 1)
	require.Len(t, payload.ToolCalls, 1)
	require.Equal(t, "planned response", payload.Messages[0].Content)
	require.Equal(t, "plan", payload.Messages[0].Phase)
	require.Equal(t, "reasoning summary", payload.Messages[0].Reasoning)
	require.Len(t, payload.Messages[0].ToolCalls, 1)
	require.Equal(t, "tool-planner", payload.Messages[0].ToolCalls[0].ID)
	require.Equal(t, "plan", payload.ToolCalls[0].Metadata["phase"])
	require.NotNil(t, payload.Plan)
	require.Equal(t, "daemon.health", payload.Plan.Actions[0].Name)

	payload.Plan.Actions[0].Arguments["probe"] = "mutated"
	require.Equal(t, "runtime", parseResult.Plan.Actions[0].Arguments["probe"])
}

func TestAgentLoopLLMStageResultPayloadToLegacyClonesRuntimePayload(t *testing.T) {
	payload := runtime.AgentLoopLLMStageResultPayload{
		Messages: []runtime.AgentLoopMessage{{
			ID:      "msg-runtime",
			Role:    "agent",
			Content: "runtime message",
			Phase:   "final_answer",
		}},
		ToolCalls: []runtime.AgentLoopToolCall{{
			ID:       "tool-runtime",
			Name:     "LLM synthesis",
			Metadata: map[string]any{"phase": "final_answer"},
		}},
		Plan: &runtime.AgentLoopPlan{
			Phase:  "final_answer",
			Answer: "runtime message",
			Actions: []runtime.AgentLoopAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"probe": "runtime"},
			}},
		},
	}

	messages, toolCalls, plan := agentLoopLLMStageResultPayloadToLegacy(payload)

	require.Len(t, messages, 1)
	require.Len(t, toolCalls, 1)
	require.NotNil(t, plan)
	require.Equal(t, "runtime message", messages[0].Content)
	require.Equal(t, "final_answer", toolCalls[0].Metadata["phase"])
	require.Equal(t, "runtime", plan.Actions[0].Arguments["probe"])

	messages[0].Content = "mutated message"
	toolCalls[0].Metadata["phase"] = "mutated_phase"
	plan.Actions[0].Arguments["probe"] = "mutated"
	require.Equal(t, "runtime message", payload.Messages[0].Content)
	require.Equal(t, "final_answer", payload.ToolCalls[0].Metadata["phase"])
	require.Equal(t, "runtime", payload.Plan.Actions[0].Arguments["probe"])
}
