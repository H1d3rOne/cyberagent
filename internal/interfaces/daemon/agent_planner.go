package daemon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	agentplanner "github.com/H1d3rOne/cyberagent/internal/agent/planner"
)

// Deprecated: use planner.RecommendationOption from internal/agent/planner.
type agentRecommendationOpt = agentplanner.RecommendationOption

// Deprecated: use planner.ContextRef from internal/agent/planner.
type agentContextRef = agentplanner.ContextRef

// Deprecated: use planner.NextAction from internal/agent/planner.
type agentNextAction = agentplanner.NextAction

// Deprecated: use planner.Response from internal/agent/planner.
type agentPlannerResponse = agentplanner.Response

// Deprecated: use planner.Action from internal/agent/planner.
type agentPlannerAction = agentplanner.Action

// Deprecated: use planner.Task from internal/agent/planner.
type agentPlannerTask = agentplanner.Task

// Deprecated: use planner.Todo from internal/agent/planner.
type agentPlannerTodo = agentplanner.Todo

func parseAgentPlannerResponse(raw string) (*agentPlannerResponse, error) {
	return agentplanner.ParseResponse(raw, func(plan *agentplanner.Response) bool {
		return len(agentPlannerActions(plan)) > 0
	})
}

func normalizeAgentPlannerSuggestedPrompt(plan *agentPlannerResponse) {
	agentplanner.NormalizeSuggestedPrompt(plan)
}

func agentPlannerPrimarySuggestedPrompt(plan *agentPlannerResponse) string {
	return agentplanner.PrimarySuggestedPrompt(plan)
}

func normalizeAgentPlannerActionArguments(value any) map[string]any {
	return agentplanner.NormalizeActionArguments(value)
}

func agentLatestUserPrompt(history []agentMessage) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" && strings.TrimSpace(history[i].Content) != "" {
			return strings.TrimSpace(history[i].Content)
		}
	}
	return ""
}

func agentToolObservationPrompt(language string, toolMessages []agentMessage) string {
	var builder strings.Builder
	builder.WriteString(agentText(
		language,
		"以下是刚刚执行完成的低风险内置工具 observation。请进入 StepReplan：先判断任务是否完成；如仍缺少低风险步骤，可输出新的 actions/tool_calls 和 auto_continue=true；如已完成，请进入 FinalAnswer 且不要再请求工具。\n",
		"Below are the observations from the low-risk built-in tools that just executed. Enter StepReplan: first decide whether the task is complete; if a low-risk step is still missing, you may output new actions/tool_calls with auto_continue=true; if complete, enter FinalAnswer and do not request more tools.\n",
	))
	for i, message := range toolMessages {
		content := strings.TrimSpace(message.Content)
		if content == "" && len(message.ToolCalls) == 0 {
			continue
		}
		if content != "" {
			content = agentLimitBlock(content, 3000)
		}
		builder.WriteString(fmt.Sprintf("\nObservation %d", i+1))
		if len(message.ToolCalls) > 0 {
			names := make([]string, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				name := strings.TrimSpace(call.Name)
				if name == "" {
					name = "tool"
				}
				names = append(names, fmt.Sprintf("%s:%s", name, call.Status))
			}
			builder.WriteString(" [")
			builder.WriteString(strings.Join(names, ", "))
			builder.WriteString("]")
		}
		builder.WriteString(":\n")
		if content != "" {
			builder.WriteString(content)
			builder.WriteString("\n")
		}
		for _, call := range message.ToolCalls {
			summary := agentSummarizeToolCall(call)
			if summary != "" {
				builder.WriteString(summary)
				builder.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func agentSummarizeToolCall(call agentToolCall) string {
	parts := []string{
		"tool=" + emptyString(call.Name, "tool"),
		"status=" + string(call.Status),
	}
	if detail := strings.TrimSpace(call.Detail); detail != "" {
		parts = append(parts, "detail="+agentLimitBlock(detail, 240))
	}
	if call.Error != nil && strings.TrimSpace(call.Error.Message) != "" {
		if call.Error.Code != "" {
			parts = append(parts, "error_code="+string(call.Error.Code))
		}
		parts = append(parts, "error="+agentLimitBlock(call.Error.Message, 500))
	}
	if len(call.Result) > 0 {
		trimmed := make(map[string]any, len(call.Result))
		keys := make([]string, 0, len(call.Result))
		for key := range call.Result {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := call.Result[key]
			switch v := value.(type) {
			case string:
				trimmed[key] = agentLimitBlock(v, 1000)
			default:
				trimmed[key] = v
			}
		}
		if encoded, err := json.Marshal(trimmed); err == nil {
			parts = append(parts, "result="+agentLimitBlock(string(encoded), 2000))
		}
	}
	return "- " + strings.Join(parts, " | ")
}

func agentLLMText(resp *llm.ChatResponse) string {
	if resp == nil {
		return ""
	}
	parts := make([]string, 0, len(resp.Content))
	for _, block := range resp.Content {
		switch text := block.(type) {
		case llm.TextBlock:
			if strings.TrimSpace(text.Text) != "" {
				parts = append(parts, strings.TrimSpace(text.Text))
			}
		case *llm.TextBlock:
			if text != nil && strings.TrimSpace(text.Text) != "" {
				parts = append(parts, strings.TrimSpace(text.Text))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
