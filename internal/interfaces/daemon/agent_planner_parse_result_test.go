package daemon

import (
	"encoding/json"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
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
