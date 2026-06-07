package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

func (m *agentManager) handleAgentLLMChat(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, language string, req sendAgentMessageRequest, history []agentMessage, memory *agentMemorySummary) ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
	attempts := m.agentLLMProviderAttempts(req)
	tool := runningAgentToolCall("LLM chat", agentLLMAttemptDetail(attempts))
	chatCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	response, streamed, usedAttempt, fallbackLog, err := m.chatAgentLLMWithFallback(chatCtx, sessionID, turnRef, "plan", attempts, func(attempt agentLLMProviderAttempt) llm.ChatRequest {
		instructions := agentInstructionsWithAuthProfiles(language, attempt.Request.Instructions, attempt.Request.AuthProfiles, attempt.Request.DefaultAuthProfileID)
		instructions = appendAgentInstructionBlock(instructions, m.agentMCPRegistryPrompt(language))
		return llm.ChatRequest{
			Model:       strings.TrimSpace(attempt.Request.Model),
			System:      agentPlannerSystemPrompt(language, instructions, attempt.Request.OperationLevel),
			Messages:    agentLLMHistory(history, memory, language),
			Tools:       toolRegistry().LLMDefinitions(),
			MaxTokens:   1600,
			Temperature: 0.2,
		}
	})
	tool = agentToolCallWithLLMFallbackMetadata(tool, usedAttempt, fallbackLog, streamed, turnRef.TurnID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.CodeBadRequest, Message: err.Error()}
		return nil, []agentToolCall{tool}, nil, err
	}
	tool.Result = agentLLMToolResult(usedAttempt, response, fallbackLog, "")
	content := agentLLMText(response)
	plan, parseErr := parseAgentPlannerResponse(content)
	if parseErr == nil && plan != nil {
		plan.Phase = firstNonEmpty(agentNormalizePhase(plan.Phase), "plan")
		tool.Result["planner"] = true
		tool.Result["planner_todos"] = len(plan.Todos)
		tool.Result["planner_actions"] = len(agentPlannerActions(plan))
		if strings.TrimSpace(plan.Answer) != "" {
			content = strings.TrimSpace(plan.Answer)
		} else if content != "" {
			plan.Answer = content
		}
	} else {
		tool.Result["planner"] = false
		if parseErr != nil {
			tool.Result["planner_parse_error"] = parseErr.Error()
		}
		if fallback := agentFallbackPlannerFromPrompt(agentLatestUserPrompt(history), content, language); fallback != nil {
			plan = fallback
			content = strings.TrimSpace(fallback.Answer)
			tool.Result["planner"] = true
			tool.Result["planner_fallback"] = true
			tool.Result["planner_todos"] = len(plan.Todos)
			tool.Result["planner_actions"] = len(agentPlannerActions(plan))
		}
	}
	nativeActions := agentPlannerActionsFromLLMToolUse(response)
	if len(nativeActions) > 0 {
		if plan == nil {
			plan = agentFallbackPlannerFromPrompt(agentLatestUserPrompt(history), content, language)
			if plan == nil {
				plan = &agentPlannerResponse{Phase: "plan"}
			}
		}
		plan.ToolCalls = append(plan.ToolCalls, nativeActions...)
		tool.Result["planner_native_tool_calls"] = len(nativeActions)
		tool.Result["planner_actions"] = len(agentPlannerActions(plan))
	}
	latestPrompt := agentLatestUserPrompt(history)
	if plan != nil && len(agentPlannerActions(plan)) == 0 && agentIsScheduleExecutionIntent(latestPrompt) {
		if fallback := agentFallbackPlannerFromPrompt(latestPrompt, "", language); fallback != nil && len(agentPlannerActions(fallback)) > 0 {
			plan = fallback
			content = strings.TrimSpace(fallback.Answer)
			tool.Result["planner"] = true
			tool.Result["planner_fallback"] = true
			tool.Result["planner_schedule_override"] = true
			tool.Result["planner_todos"] = len(plan.Todos)
			tool.Result["planner_actions"] = len(agentPlannerActions(plan))
		}
	}
	if strings.TrimSpace(content) == "" {
		if len(nativeActions) > 0 {
			content = agentText(language, "我将先调用低风险内置工具检查当前上下文。", "I will first call low-risk built-in tools to inspect the current context.")
			plan.Answer = content
		} else {
			content = agentText(language, "AI provider 返回了空内容。", "The AI provider returned empty content.")
		}
	}
	messagePhase := "plan"
	if plan != nil {
		if normalized := agentNormalizePhase(plan.Phase); normalized != "" {
			messagePhase = normalized
		}
		if len(agentPlannerActions(plan)) > 0 {
			messagePhase = "plan"
		}
	}
	tool = agentToolCallWithPhase(tool, messagePhase)
	message := agentMessageWithPhase(m.message(language, "agent", content, []agentToolCall{tool}, nil), messagePhase)
	if plan != nil {
		message.Reasoning = trimSummary(plan.Reasoning, 800)
	}
	applyAgentPlannerToMessage(&message, plan)
	return []agentMessage{message}, []agentToolCall{tool}, plan, nil
}

func (m *agentManager) synthesizeAgentToolResults(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, language string, req sendAgentMessageRequest, history []agentMessage, memory *agentMemorySummary, toolMessages []agentMessage) ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
	if len(toolMessages) == 0 {
		return nil, nil, nil, nil
	}

	attempts := m.agentLLMProviderAttempts(req)
	tool := runningAgentToolCall("LLM synthesis", agentLLMAttemptDetail(attempts))
	tool = withAgentToolMetadata(tool, map[string]interface{}{"phase": "tool_synthesis"})

	chatCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	response, streamed, usedAttempt, fallbackLog, err := m.chatAgentLLMWithFallback(chatCtx, sessionID, turnRef, "step_replan", attempts, func(attempt agentLLMProviderAttempt) llm.ChatRequest {
		instructions := agentInstructionsWithAuthProfiles(language, attempt.Request.Instructions, attempt.Request.AuthProfiles, attempt.Request.DefaultAuthProfileID)
		instructions = appendAgentInstructionBlock(instructions, m.agentMCPRegistryPrompt(language))
		return llm.ChatRequest{
			Model:       strings.TrimSpace(attempt.Request.Model),
			System:      agentToolSynthesisSystemPrompt(language, instructions, attempt.Request.OperationLevel),
			Messages:    agentLLMToolSynthesisHistory(history, memory, toolMessages, language),
			MaxTokens:   1200,
			Temperature: 0.2,
		}
	})
	tool = agentToolCallWithLLMFallbackMetadata(tool, usedAttempt, fallbackLog, streamed, turnRef.TurnID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.CodeBadRequest, Message: err.Error()}
		return nil, []agentToolCall{tool}, nil, err
	}

	tool.Result = agentLLMToolResult(usedAttempt, response, fallbackLog, "step_replan")

	content := agentLLMText(response)
	plan, parseErr := parseAgentPlannerResponse(content)
	if parseErr == nil && plan != nil {
		plan.Phase = firstNonEmpty(agentNormalizePhase(plan.Phase), "step_replan")
		tool.Result["planner"] = true
		tool.Result["planner_todos"] = len(plan.Todos)
		tool.Result["planner_actions"] = len(agentPlannerActions(plan))
		if strings.TrimSpace(plan.Answer) != "" {
			content = strings.TrimSpace(plan.Answer)
		} else if content != "" {
			plan.Answer = content
		}
	} else {
		tool.Result["planner"] = false
		if parseErr != nil {
			tool.Result["planner_parse_error"] = parseErr.Error()
		}
		plain := strings.TrimSpace(content)
		if plain != "" {
			plan = agentPlannerFromPlainAnswer(plain, "step_replan", language)
			tool.Result["planner"] = true
			tool.Result["planner_fallback"] = true
			tool.Result["planner_todos"] = len(plan.Todos)
			tool.Result["planner_actions"] = len(agentPlannerActions(plan))
		}
	}
	if strings.TrimSpace(content) == "" {
		content = agentText(language, "工具执行完成，但 AI provider 返回了空总结。", "Tools executed, but the AI provider returned an empty synthesis.")
		plan = agentPlannerFromPlainAnswer(content, "final_answer", language)
	}

	phase := "step_replan"
	if plan == nil || len(agentPlannerActions(plan)) == 0 {
		phase = "final_answer"
		if plan != nil && agentNormalizePhase(plan.Phase) == "" {
			plan.Phase = phase
		}
	}
	tool = agentToolCallWithPhase(tool, phase)
	message := agentMessageWithPhase(m.message(language, "agent", content, []agentToolCall{tool}, nil), phase)
	if plan != nil {
		message.Reasoning = trimSummary(plan.Reasoning, 800)
	}
	applyAgentPlannerToMessage(&message, plan)
	return []agentMessage{message}, []agentToolCall{tool}, plan, nil
}

func (m *agentManager) chatAgentLLM(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, phase string, provider llm.Provider, req llm.ChatRequest) (*llm.ChatResponse, bool, error) {
	eng := m.runtimeEngine()
	if eng == nil || turnRef.TaskID == "" || turnRef.TurnID == "" {
		resp, err := provider.Chat(ctx, req)
		return resp, false, err
	}
	streamProvider, ok := provider.(llm.StreamProvider)
	if !ok {
		resp, err := provider.Chat(ctx, req)
		return resp, false, err
	}
	stream, err := streamProvider.StreamChat(ctx, req)
	if err != nil {
		if strings.Contains(err.Error(), "does not support streaming") {
			resp, chatErr := provider.Chat(ctx, req)
			return resp, false, chatErr
		}
		return nil, false, err
	}

	var text strings.Builder
	var blocks []llm.ContentBlock
	stopReason := llm.StopReasonEndTurn
	var usage llm.Usage
	streamed := false
	for ev := range stream {
		switch ev.Type {
		case "text_delta":
			if ev.TextDelta == "" {
				continue
			}
			streamed = true
			text.WriteString(ev.TextDelta)
			eng.Publish(ctx, runtime.Event{
				Type:   "model_token",
				TaskID: turnRef.TaskID,
				RunID:  turnRef.RunID,
				TurnID: turnRef.TurnID,
				Data: map[string]any{
					"session_id": sessionID,
					"phase":      phase,
					"provider":   provider.Name(),
					"delta":      ev.TextDelta,
					"text":       ev.TextDelta,
				},
				Timestamp: time.Now().UTC(),
			})
		case "thinking_delta", "reasoning_delta":
			if ev.TextDelta == "" {
				continue
			}
			streamed = true
			eng.Publish(ctx, runtime.Event{
				Type:   "model_reasoning",
				TaskID: turnRef.TaskID,
				RunID:  turnRef.RunID,
				TurnID: turnRef.TurnID,
				Data: map[string]any{
					"session_id": sessionID,
					"phase":      phase,
					"provider":   provider.Name(),
					"delta":      ev.TextDelta,
					"text":       ev.TextDelta,
				},
				Timestamp: time.Now().UTC(),
			})
		case "tool_use":
			if ev.Block != nil {
				blocks = append(blocks, ev.Block)
			}
		case "done":
			if ev.StopReason != "" {
				stopReason = ev.StopReason
			}
			usage = ev.Usage
		case "error":
			if ev.Error != nil {
				return nil, streamed, ev.Error
			}
		}
	}
	if strings.TrimSpace(text.String()) != "" {
		blocks = append([]llm.ContentBlock{llm.TextBlock{Text: text.String()}}, blocks...)
	}
	return &llm.ChatResponse{
		Content:    blocks,
		StopReason: stopReason,
		Usage:      usage,
	}, streamed, nil
}

func (m *agentManager) runAgentLLMToolLoop(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	planMessages, planToolCalls, plan, err := m.runRuntimeAgentLLMPlannerChat(ctx, sessionID, turnRef, prompt, language, operationLevel, req, history, memorySummary)
	if err != nil {
		return planMessages, planToolCalls, err
	}

	m.appendMessagesWithPlannerForTurn(ctx, sessionID, turnRef, prompt, string(runtime.LLMStagePlannerChat), operationLevel, plan, planMessages...)
	cycleState := agentLoopStateFromLegacy(
		planMessages,
		planToolCalls,
		append(cloneAgentMessages(history), planMessages...),
	)

	maxLoops := reqMaxAgentReplans(req)
	cyclePlan := agentLoopPlanFromPlannerResponse(plan)
	for step := 0; step < maxLoops; step++ {
		if !agentLoopPlanHasActions(cyclePlan) {
			// Respect a structured model plan that intentionally stops at Plan and
			// asks the operator/runtime to continue. Fallback actions are only
			// created when the latest operator prompt clearly asks to execute now
			// but the provider forgot to emit actions.
			if step == 0 && agentPromptRequestsImmediateExecution(prompt) {
				fallback := agentFallbackPlannerFromPrompt(prompt, "", language)
				cyclePlan = agentLoopPlanFromPlannerResponse(fallback)
				if !agentLoopPlanHasActions(cyclePlan) {
					break
				}
			} else {
				break
			}
		}

		cycle := m.runRuntimeAgentPlannerActionReplanCycleAgentLoop(ctx, runtime.AgentLoopRequest{
			Mode:          runtime.AgentLoopModeLLMToolLoop,
			Turn:          agentPlannerActionReplanCycleTurnContext(sessionID, turnRef, prompt, req.Origin, operationLevel),
			Language:      language,
			Operator:      operator,
			LLMRequest:    agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req),
			History:       runtime.CloneAgentLoopMessages(cycleState.LoopHistory),
			MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(memorySummary),
			InitialPlan:   runtime.CloneAgentLoopPlan(cyclePlan),
			State:         cycleState.Clone(),
			Authorization: agentPlannerActionReplanCycleAuthorizationFromMessageRequest(req),
			ToolServices:  agentLoopToolServiceBindingFromRouterServices(svc),
			MaxLoops:      maxLoops,
			Metadata: agentPlannerActionReplanCycleMetadataFromOptions(agentPlannerActionReplanCycleOptions{
				ActionAppendOrigin:                      "planner_loop",
				AppendActionToolCallsBeforeEmptyMessage: true,
			}),
		})
		cycleState = cycle.State.Clone()
		if !agentPlannerActionReplanCycleContinueFromAgentLoopResult(cycle) {
			break
		}
		if !agentLoopPlanHasActions(cycle.FinalPlan) {
			break
		}
		if !agentLoopPlanShouldAutoContinue(cycle.FinalPlan) {
			break
		}
		cyclePlan = runtime.CloneAgentLoopPlan(cycle.FinalPlan)
	}

	return agentMessagesFromAgentLoopMessages(cycleState.ResponseMessages), agentLoopToolCallItemsToAgentToolCalls(cycleState.ToolCalls), nil
}

func (m *agentManager) runAgentReplanActionContinuation(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices, plan *agentPlannerResponse, maxLoops int) ([]agentMessage, []agentToolCall) {
	if maxLoops <= 0 {
		return nil, nil
	}
	cycleState := agentLoopStateFromLegacy(
		[]agentMessage{},
		[]agentToolCall{},
		history,
	)

	cyclePlan := agentLoopPlanFromPlannerResponse(plan)
	for step := 0; step < maxLoops; step++ {
		if !agentLoopPlanHasActions(cyclePlan) {
			break
		}
		if !agentLoopPlanShouldAutoContinue(cyclePlan) {
			break
		}

		cycle := m.runRuntimeAgentPlannerActionReplanCycleAgentLoop(ctx, runtime.AgentLoopRequest{
			Mode:          runtime.AgentLoopModeReplanContinuation,
			Turn:          agentPlannerActionReplanCycleTurnContext(sessionID, turnRef, prompt, req.Origin, operationLevel),
			Language:      language,
			Operator:      operator,
			LLMRequest:    agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req),
			History:       runtime.CloneAgentLoopMessages(cycleState.LoopHistory),
			MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(memorySummary),
			InitialPlan:   runtime.CloneAgentLoopPlan(cyclePlan),
			State:         cycleState.Clone(),
			Authorization: agentPlannerActionReplanCycleAuthorizationFromMessageRequest(req),
			ToolServices:  agentLoopToolServiceBindingFromRouterServices(svc),
			MaxLoops:      maxLoops,
			Metadata: agentPlannerActionReplanCycleMetadataFromOptions(agentPlannerActionReplanCycleOptions{
				ActionAppendOrigin: daemonTurnExecutionReplanActionContinuation,
			}),
		})
		cycleState = cycle.State.Clone()
		if !agentPlannerActionReplanCycleContinueFromAgentLoopResult(cycle) {
			break
		}
		cyclePlan = runtime.CloneAgentLoopPlan(cycle.FinalPlan)
	}
	return agentMessagesFromAgentLoopMessages(cycleState.ResponseMessages), agentLoopToolCallItemsToAgentToolCalls(cycleState.ToolCalls)
}

type agentPlannerActionReplanCycleOptions struct {
	ActionAppendOrigin                      string
	AppendActionToolCallsBeforeEmptyMessage bool
}

type agentPlannerActionReplanCycleMessages struct {
	items []runtime.AgentLoopMessage
}

func agentPlannerActionReplanCycleMessagesFromAgentMessages(messages []agentMessage) agentPlannerActionReplanCycleMessages {
	if messages == nil {
		return agentPlannerActionReplanCycleMessages{}
	}
	return agentPlannerActionReplanCycleMessages{items: agentLoopMessagesFromAgentMessages(messages)}
}

func agentPlannerActionReplanCycleMessagesFromAgentLoopMessages(messages []runtime.AgentLoopMessage) agentPlannerActionReplanCycleMessages {
	if messages == nil {
		return agentPlannerActionReplanCycleMessages{}
	}
	return agentPlannerActionReplanCycleMessages{items: runtime.CloneAgentLoopMessages(messages)}
}

func (messages agentPlannerActionReplanCycleMessages) legacyAgentMessages() []agentMessage {
	return agentMessagesFromAgentLoopMessages(messages.items)
}

func (messages agentPlannerActionReplanCycleMessages) agentLoopMessages() []runtime.AgentLoopMessage {
	return runtime.CloneAgentLoopMessages(messages.items)
}

func (messages agentPlannerActionReplanCycleMessages) len() int {
	return len(messages.items)
}

func (messages agentPlannerActionReplanCycleMessages) withPhase(phase string) agentPlannerActionReplanCycleMessages {
	out := messages.clone()
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return out
	}
	for i := range out.items {
		out.items[i] = agentLoopMessageWithPhase(out.items[i], normalized)
	}
	return out
}

func (messages *agentPlannerActionReplanCycleMessages) appendMessages(values agentPlannerActionReplanCycleMessages) {
	if values.len() == 0 {
		return
	}
	messages.items = append(messages.items, values.clone().items...)
}

func (messages *agentPlannerActionReplanCycleMessages) appendLegacyAgentMessages(values ...agentMessage) {
	if len(values) == 0 {
		return
	}
	messages.appendMessages(agentPlannerActionReplanCycleMessagesFromAgentMessages(values))
}

func (messages agentPlannerActionReplanCycleMessages) clone() agentPlannerActionReplanCycleMessages {
	if messages.items == nil {
		return agentPlannerActionReplanCycleMessages{}
	}
	return agentPlannerActionReplanCycleMessages{items: runtime.CloneAgentLoopMessages(messages.items)}
}

type agentPlannerActionReplanCycleToolCalls struct {
	items []runtime.AgentLoopToolCall
}

func agentPlannerActionReplanCycleToolCallsFromAgentToolCalls(calls []agentToolCall) agentPlannerActionReplanCycleToolCalls {
	if calls == nil {
		return agentPlannerActionReplanCycleToolCalls{}
	}
	items := make([]runtime.AgentLoopToolCall, len(calls))
	for i, call := range calls {
		items[i] = agentLoopToolCallFromAgentToolCall(call)
	}
	return agentPlannerActionReplanCycleToolCalls{items: items}
}

func agentPlannerActionReplanCycleToolCallsFromAgentLoopToolCalls(calls []runtime.AgentLoopToolCall) agentPlannerActionReplanCycleToolCalls {
	if calls == nil {
		return agentPlannerActionReplanCycleToolCalls{}
	}
	return agentPlannerActionReplanCycleToolCalls{items: runtime.CloneAgentLoopToolCalls(calls)}
}

func (calls agentPlannerActionReplanCycleToolCalls) legacyAgentToolCalls() []agentToolCall {
	if calls.items == nil {
		return nil
	}
	out := make([]agentToolCall, len(calls.items))
	for i, call := range calls.items {
		out[i] = agentToolCallFromAgentLoopToolCall(call)
	}
	return out
}

func (calls agentPlannerActionReplanCycleToolCalls) agentLoopToolCalls() []runtime.AgentLoopToolCall {
	return runtime.CloneAgentLoopToolCalls(calls.items)
}

func (calls agentPlannerActionReplanCycleToolCalls) len() int {
	return len(calls.items)
}

func (calls agentPlannerActionReplanCycleToolCalls) withPhase(phase string) agentPlannerActionReplanCycleToolCalls {
	out := calls.clone()
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return out
	}
	for i := range out.items {
		out.items[i] = agentLoopToolCallWithPhase(out.items[i], normalized)
	}
	return out
}

func (calls *agentPlannerActionReplanCycleToolCalls) appendToolCalls(values agentPlannerActionReplanCycleToolCalls) {
	if values.len() == 0 {
		return
	}
	calls.items = append(calls.items, values.clone().items...)
}

func (calls *agentPlannerActionReplanCycleToolCalls) appendLegacyAgentToolCalls(values ...agentToolCall) {
	if len(values) == 0 {
		return
	}
	calls.appendToolCalls(agentPlannerActionReplanCycleToolCallsFromAgentToolCalls(values))
}

func (calls agentPlannerActionReplanCycleToolCalls) clone() agentPlannerActionReplanCycleToolCalls {
	if calls.items == nil {
		return agentPlannerActionReplanCycleToolCalls{}
	}
	return agentPlannerActionReplanCycleToolCalls{items: runtime.CloneAgentLoopToolCalls(calls.items)}
}

func agentLoopMessagesFromAgentMessages(messages []agentMessage) []runtime.AgentLoopMessage {
	if messages == nil {
		return nil
	}
	out := make([]runtime.AgentLoopMessage, len(messages))
	for i, message := range messages {
		out[i] = agentLoopMessageFromAgentMessage(message)
	}
	return out
}

func agentMessagesFromAgentLoopMessages(messages []runtime.AgentLoopMessage) []agentMessage {
	if messages == nil {
		return nil
	}
	out := make([]agentMessage, len(messages))
	for i, message := range messages {
		out[i] = agentMessageFromAgentLoopMessage(message)
	}
	return out
}

func agentLoopMessageFromAgentMessage(message agentMessage) runtime.AgentLoopMessage {
	return runtime.AgentLoopMessage{
		ID:                    message.ID,
		Role:                  message.Role,
		Content:               message.Content,
		Timestamp:             message.Timestamp,
		Phase:                 message.Phase,
		ToolCalls:             agentLoopToolCallItemsFromAgentToolCalls(message.ToolCalls),
		Links:                 agentLoopLinksFromAgentLinks(message.Links),
		Reasoning:             message.Reasoning,
		SuggestedPrompts:      append([]string(nil), message.SuggestedPrompts...),
		RecommendationOptions: agentLoopRecommendationOptionsFromAgentRecommendationOptions(message.RecommendationOptions),
	}
}

func agentMessageFromAgentLoopMessage(message runtime.AgentLoopMessage) agentMessage {
	return agentMessage{
		ID:                    message.ID,
		Role:                  message.Role,
		Content:               message.Content,
		Timestamp:             message.Timestamp,
		Phase:                 message.Phase,
		ToolCalls:             agentLoopToolCallItemsToAgentToolCalls(message.ToolCalls),
		Links:                 agentLinksFromAgentLoopLinks(message.Links),
		Reasoning:             message.Reasoning,
		SuggestedPrompts:      append([]string(nil), message.SuggestedPrompts...),
		RecommendationOptions: agentRecommendationOptionsFromAgentLoopRecommendationOptions(message.RecommendationOptions),
	}
}

func agentLoopMessageWithPhase(message runtime.AgentLoopMessage, phase string) runtime.AgentLoopMessage {
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return message.Clone()
	}
	out := message.Clone()
	out.Phase = normalized
	for i := range out.ToolCalls {
		out.ToolCalls[i] = agentLoopToolCallWithPhase(out.ToolCalls[i], normalized)
	}
	return out
}

func agentLoopToolCallFromAgentToolCall(call agentToolCall) runtime.AgentLoopToolCall {
	return runtime.AgentLoopToolCall{
		ID:         call.ID,
		Name:       call.Name,
		Status:     call.Status,
		Detail:     call.Detail,
		StartedAt:  call.StartedAt,
		FinishedAt: runtime.CloneAgentLoopTimePtr(call.FinishedAt),
		Result:     cloneMapStringAny(call.Result),
		Error:      agentLoopToolCallErrorFromDaemonJobError(call.Error),
		Metadata:   cloneMapStringAny(call.Metadata),
	}
}

func agentLoopToolCallErrorFromDaemonJobError(err *daemonJobError) *runtime.AgentLoopToolCallError {
	if err == nil {
		return nil
	}
	return &runtime.AgentLoopToolCallError{
		Code:    err.Code,
		Message: err.Message,
	}
}

func agentToolCallFromAgentLoopToolCall(call runtime.AgentLoopToolCall) agentToolCall {
	return agentToolCall{
		ID:         call.ID,
		Name:       call.Name,
		Status:     call.Status,
		Detail:     call.Detail,
		StartedAt:  call.StartedAt,
		FinishedAt: runtime.CloneAgentLoopTimePtr(call.FinishedAt),
		Result:     cloneMapStringAny(call.Result),
		Error:      daemonJobErrorFromAgentLoopToolCallError(call.Error),
		Metadata:   cloneMapStringAny(call.Metadata),
	}
}

func daemonJobErrorFromAgentLoopToolCallError(err *runtime.AgentLoopToolCallError) *daemonJobError {
	if err == nil {
		return nil
	}
	return &daemonJobError{
		Code:    err.Code,
		Message: err.Message,
	}
}

func agentLoopToolCallWithPhase(call runtime.AgentLoopToolCall, phase string) runtime.AgentLoopToolCall {
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return call.Clone()
	}
	out := call.Clone()
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	if _, exists := out.Metadata["phase"]; exists {
		out.Metadata["agent_phase"] = normalized
	} else {
		out.Metadata["phase"] = normalized
	}
	return out
}

func agentLoopToolCallItemsFromAgentToolCalls(calls []agentToolCall) []runtime.AgentLoopToolCall {
	if calls == nil {
		return nil
	}
	out := make([]runtime.AgentLoopToolCall, len(calls))
	for i, call := range calls {
		out[i] = agentLoopToolCallFromAgentToolCall(call)
	}
	return out
}

func agentLoopToolCallItemsToAgentToolCalls(calls []runtime.AgentLoopToolCall) []agentToolCall {
	if calls == nil {
		return nil
	}
	out := make([]agentToolCall, len(calls))
	for i, call := range calls {
		out[i] = agentToolCallFromAgentLoopToolCall(call)
	}
	return out
}

func agentLoopLinksFromAgentLinks(links []agentLink) []runtime.AgentLoopLink {
	if links == nil {
		return nil
	}
	out := make([]runtime.AgentLoopLink, len(links))
	for i, link := range links {
		out[i] = runtime.AgentLoopLink{
			To:    link.To,
			Label: link.Label,
			Kind:  link.Kind,
		}
	}
	return out
}

func agentLinksFromAgentLoopLinks(links []runtime.AgentLoopLink) []agentLink {
	if links == nil {
		return nil
	}
	out := make([]agentLink, len(links))
	for i, link := range links {
		out[i] = agentLink{
			To:    link.To,
			Label: link.Label,
			Kind:  link.Kind,
		}
	}
	return out
}

func agentLoopRecommendationOptionsFromAgentRecommendationOptions(options []agentRecommendationOpt) []runtime.AgentLoopRecommendationOption {
	if options == nil {
		return nil
	}
	out := make([]runtime.AgentLoopRecommendationOption, len(options))
	for i, option := range options {
		out[i] = runtime.AgentLoopRecommendationOption{
			ID:      option.ID,
			Title:   option.Title,
			Detail:  option.Detail,
			Prompt:  option.Prompt,
			Command: option.Command,
		}
	}
	return out
}

func agentRecommendationOptionsFromAgentLoopRecommendationOptions(options []runtime.AgentLoopRecommendationOption) []agentRecommendationOpt {
	if options == nil {
		return nil
	}
	out := make([]agentRecommendationOpt, len(options))
	for i, option := range options {
		out[i] = agentRecommendationOpt{
			ID:      option.ID,
			Title:   option.Title,
			Detail:  option.Detail,
			Prompt:  option.Prompt,
			Command: option.Command,
		}
	}
	return out
}

func agentPlannerActionReplanCycleStateFromLegacy(responseMessages []agentMessage, toolCalls []agentToolCall, loopHistory []agentMessage) agentPlannerActionReplanCycleState {
	return agentPlannerActionReplanCycleState{
		state: agentLoopStateFromLegacy(responseMessages, toolCalls, loopHistory),
	}
}

func agentLoopStateFromLegacy(responseMessages []agentMessage, toolCalls []agentToolCall, loopHistory []agentMessage) runtime.AgentLoopState {
	return runtime.AgentLoopState{
		ResponseMessages: agentLoopMessagesFromAgentMessages(responseMessages),
		ToolCalls:        agentLoopToolCallItemsFromAgentToolCalls(toolCalls),
		LoopHistory:      agentLoopMessagesFromAgentMessages(loopHistory),
	}
}

type agentPlannerActionReplanCycleState struct {
	state runtime.AgentLoopState
}

func (state agentPlannerActionReplanCycleState) clone() agentPlannerActionReplanCycleState {
	return agentPlannerActionReplanCycleState{state: state.state.Clone()}
}

func (state agentPlannerActionReplanCycleState) responseMessages() agentPlannerActionReplanCycleMessages {
	return agentPlannerActionReplanCycleMessages{items: runtime.CloneAgentLoopMessages(state.state.ResponseMessages)}
}

func (state agentPlannerActionReplanCycleState) toolCalls() agentPlannerActionReplanCycleToolCalls {
	return agentPlannerActionReplanCycleToolCalls{items: runtime.CloneAgentLoopToolCalls(state.state.ToolCalls)}
}

func (state agentPlannerActionReplanCycleState) loopHistory() agentPlannerActionReplanCycleMessages {
	return agentPlannerActionReplanCycleMessages{items: runtime.CloneAgentLoopMessages(state.state.LoopHistory)}
}

func (state agentPlannerActionReplanCycleState) legacyResponseMessages() []agentMessage {
	return agentMessagesFromAgentLoopMessages(state.state.ResponseMessages)
}

func (state agentPlannerActionReplanCycleState) legacyToolCalls() []agentToolCall {
	return agentLoopToolCallItemsToAgentToolCalls(state.state.ToolCalls)
}

func (state agentPlannerActionReplanCycleState) legacyLoopHistory() []agentMessage {
	return agentMessagesFromAgentLoopMessages(state.state.LoopHistory)
}

func (state *agentPlannerActionReplanCycleState) appendResponseMessages(messages agentPlannerActionReplanCycleMessages) {
	if messages.len() == 0 {
		return
	}
	state.state.ResponseMessages = append(state.state.ResponseMessages, messages.clone().items...)
}

func (state *agentPlannerActionReplanCycleState) appendLoopHistory(messages agentPlannerActionReplanCycleMessages) {
	if messages.len() == 0 {
		return
	}
	state.state.LoopHistory = append(state.state.LoopHistory, messages.clone().items...)
}

func (state *agentPlannerActionReplanCycleState) appendToolCalls(calls agentPlannerActionReplanCycleToolCalls) {
	if calls.len() == 0 {
		return
	}
	state.state.ToolCalls = append(state.state.ToolCalls, calls.clone().items...)
}

func agentPlannerActionReplanCycleAuthorizationFromMessageRequest(req sendAgentMessageRequest) runtime.AgentLoopAuthorization {
	return runtime.AgentLoopAuthorization{
		AuthProfiles:         agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(req.AuthProfiles),
		DefaultAuthProfileID: req.DefaultAuthProfileID,
	}
}

func agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(profiles []agentHTTPAuthProfile) []runtime.AgentLoopAuthProfile {
	if len(profiles) == 0 {
		return nil
	}
	out := make([]runtime.AgentLoopAuthProfile, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, agentPlannerActionReplanCycleAuthProfileFromHTTPProfile(profile))
	}
	return out
}

func agentPlannerActionReplanCycleAuthProfileFromHTTPProfile(profile agentHTTPAuthProfile) runtime.AgentLoopAuthProfile {
	return runtime.AgentLoopAuthProfile{
		ID:          profile.ID,
		Name:        profile.Name,
		Type:        profile.Type,
		BaseURL:     profile.BaseURL,
		Token:       profile.Token,
		Username:    profile.Username,
		Password:    profile.Password,
		HeaderName:  profile.HeaderName,
		HeaderValue: profile.HeaderValue,
		Cookie:      profile.Cookie,
		Enabled:     runtime.CloneAgentLoopBoolPtr(profile.Enabled),
	}
}

func agentHTTPAuthProfileFromAgentLoopAuthProfile(profile runtime.AgentLoopAuthProfile) agentHTTPAuthProfile {
	return agentHTTPAuthProfile{
		ID:          profile.ID,
		Name:        profile.Name,
		Type:        profile.Type,
		BaseURL:     profile.BaseURL,
		Token:       profile.Token,
		Username:    profile.Username,
		Password:    profile.Password,
		HeaderName:  profile.HeaderName,
		HeaderValue: profile.HeaderValue,
		Cookie:      profile.Cookie,
		Enabled:     runtime.CloneAgentLoopBoolPtr(profile.Enabled),
	}
}

func agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(profiles []runtime.AgentLoopAuthProfile) []agentHTTPAuthProfile {
	if len(profiles) == 0 {
		return nil
	}
	out := make([]agentHTTPAuthProfile, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, agentHTTPAuthProfileFromAgentLoopAuthProfile(profile))
	}
	return out
}

func agentPlannerActionReplanCycleLLMRequestFromMessageRequest(req sendAgentMessageRequest) runtime.AgentLoopLLMRequest {
	return runtime.AgentLoopLLMRequest{
		Origin:               req.Origin,
		OperationLevel:       req.OperationLevel,
		Provider:             req.Provider,
		Model:                req.Model,
		APIKey:               req.APIKey,
		BaseURL:              req.BaseURL,
		Instructions:         req.Instructions,
		AuthProfiles:         agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(req.AuthProfiles),
		DefaultAuthProfileID: req.DefaultAuthProfileID,
	}
}

func sendMessageRequestFromAgentLoopLLMRequest(req runtime.AgentLoopLLMRequest) sendAgentMessageRequest {
	return sendAgentMessageRequest{
		Origin:               req.Origin,
		OperationLevel:       req.OperationLevel,
		Provider:             req.Provider,
		Model:                req.Model,
		APIKey:               req.APIKey,
		BaseURL:              req.BaseURL,
		Instructions:         req.Instructions,
		AuthProfiles:         agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(req.AuthProfiles),
		DefaultAuthProfileID: req.DefaultAuthProfileID,
	}
}

type agentPlannerActionReplanCycleToolContext struct {
	services runtime.AgentLoopToolServiceBinding
}

func agentPlannerActionReplanCycleToolContextFromRouterServices(svc routerServices) agentPlannerActionReplanCycleToolContext {
	return agentPlannerActionReplanCycleToolContext{services: agentLoopToolServiceBindingFromRouterServices(svc)}
}

func (toolCtx agentPlannerActionReplanCycleToolContext) legacyRouterServices() routerServices {
	return legacyRouterServicesFromAgentLoopToolServiceBinding(toolCtx.services)
}

type agentPlannerActionReplanCyclePlan struct {
	present             bool
	item                runtime.AgentLoopPlan
	actionsSnapshot     []runtime.AgentLoopAction
	actionCountSnapshot int
	autoContinue        bool
}

func agentPlannerActionReplanCyclePlanFromPlannerResponse(plan *agentPlannerResponse) agentPlannerActionReplanCyclePlan {
	if plan == nil {
		return agentPlannerActionReplanCyclePlan{}
	}
	actionsSnapshot := agentPlannerActionReplanCyclePlanActionsFromAgentPlannerActions(agentPlannerActions(plan))
	return agentPlannerActionReplanCyclePlan{
		present:             true,
		item:                agentPlannerActionReplanCyclePlanItemFromAgentPlannerResponse(*plan),
		actionsSnapshot:     actionsSnapshot,
		actionCountSnapshot: len(actionsSnapshot),
		autoContinue:        plan.NextAction != nil && plan.NextAction.AutoContinue,
	}
}

func (plan agentPlannerActionReplanCyclePlan) legacyPlannerResponse() *agentPlannerResponse {
	if !plan.present {
		return nil
	}
	return agentPlannerResponseFromAgentLoopPlan(plan.item)
}

func (plan agentPlannerActionReplanCyclePlan) agentLoopPlan() *runtime.AgentLoopPlan {
	if !plan.present {
		return nil
	}
	cloned := plan.item.Clone()
	if len(cloned.Actions) == 0 && len(cloned.ToolCalls) == 0 && len(plan.actionsSnapshot) > 0 {
		cloned.Actions = runtime.CloneAgentLoopActions(plan.actionsSnapshot)
	}
	return &cloned
}

func agentPlannerActionReplanCyclePlanFromAgentLoopPlan(plan *runtime.AgentLoopPlan) agentPlannerActionReplanCyclePlan {
	if plan == nil {
		return agentPlannerActionReplanCyclePlan{}
	}
	cloned := plan.Clone()
	return agentPlannerActionReplanCyclePlanFromPlannerResponse(agentPlannerResponseFromAgentLoopPlan(cloned))
}

func agentLoopPlanFromPlannerResponse(plan *agentPlannerResponse) *runtime.AgentLoopPlan {
	return agentPlannerActionReplanCyclePlanFromPlannerResponse(plan).agentLoopPlan()
}

func agentLoopPlanActions(plan *runtime.AgentLoopPlan) []runtime.AgentLoopAction {
	if plan == nil {
		return nil
	}
	rawActions := runtime.CloneAgentLoopActions(plan.Actions)
	rawActions = append(rawActions, runtime.CloneAgentLoopActions(plan.ToolCalls)...)

	actions := make([]runtime.AgentLoopAction, 0, len(rawActions))
	seen := map[string]struct{}{}
	for _, action := range rawActions {
		name := normalizeAgentPlannerActionName(action.Name)
		if name == "" {
			name = normalizeAgentPlannerActionName(stringFromAny(action.Arguments["name"]))
		}
		if name == "" {
			continue
		}
		action.Name = name
		if action.Arguments == nil {
			action.Arguments = map[string]any{}
		} else {
			action.Arguments = runtime.CloneAgentLoopActionArguments(action.Arguments)
		}
		key := agentPlannerActionDedupeKey(name, action.Arguments)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		actions = append(actions, action)
	}
	return actions
}

func agentLoopPlanHasActions(plan *runtime.AgentLoopPlan) bool {
	return len(agentLoopPlanActions(plan)) > 0
}

func agentLoopPlanShouldAutoContinue(plan *runtime.AgentLoopPlan) bool {
	return agentLoopPlanHasActions(plan) && plan != nil && plan.NextAction != nil && plan.NextAction.AutoContinue
}

func legacyPlannerResponseFromAgentLoopPlan(plan *runtime.AgentLoopPlan) *agentPlannerResponse {
	if plan == nil {
		return nil
	}
	return agentPlannerResponseFromAgentLoopPlan(plan.Clone())
}

func (plan agentPlannerActionReplanCyclePlan) actions() []agentPlannerAction {
	return agentPlannerActionReplanCyclePlanActionsToAgentPlannerActions(plan.actionsSnapshot)
}

func (plan agentPlannerActionReplanCyclePlan) actionCount() int {
	return plan.actionCountSnapshot
}

func (plan agentPlannerActionReplanCyclePlan) hasActions() bool {
	return plan.actionCount() > 0
}

func (plan agentPlannerActionReplanCyclePlan) shouldAutoContinue() bool {
	return plan.hasActions() && plan.autoContinue
}

func agentPlannerActionReplanCyclePlanItemFromAgentPlannerResponse(plan agentPlannerResponse) runtime.AgentLoopPlan {
	return runtime.AgentLoopPlan{
		Phase:                 plan.Phase,
		Answer:                plan.Answer,
		SuggestedPrompt:       plan.SuggestedPrompt,
		SuggestPrompt:         plan.SuggestPrompt,
		SuggestedPrompts:      append([]string(nil), plan.SuggestedPrompts...),
		RecommendationOptions: agentPlannerActionReplanCyclePlanRecommendationOptionsFromAgentRecommendationOptions(plan.RecommendationOptions),
		Task:                  agentPlannerActionReplanCyclePlanTaskFromAgentPlannerTask(plan.Task),
		Todos:                 agentPlannerActionReplanCyclePlanTodosFromAgentPlannerTodos(plan.Todos),
		ContextRefs:           agentPlannerActionReplanCyclePlanContextRefsFromAgentContextRefs(plan.ContextRefs),
		NextAction:            agentPlannerActionReplanCyclePlanNextActionFromAgentNextAction(plan.NextAction),
		Reasoning:             plan.Reasoning,
		Actions:               agentPlannerActionReplanCyclePlanActionsFromAgentPlannerActions(plan.Actions),
		ToolCalls:             agentPlannerActionReplanCyclePlanActionsFromAgentPlannerActions(plan.ToolCalls),
	}
}

func agentPlannerResponseFromAgentLoopPlan(plan runtime.AgentLoopPlan) *agentPlannerResponse {
	return &agentPlannerResponse{
		Phase:                 plan.Phase,
		Answer:                plan.Answer,
		SuggestedPrompt:       plan.SuggestedPrompt,
		SuggestPrompt:         plan.SuggestPrompt,
		SuggestedPrompts:      append([]string(nil), plan.SuggestedPrompts...),
		RecommendationOptions: agentPlannerActionReplanCyclePlanRecommendationOptionsToAgentRecommendationOptions(plan.RecommendationOptions),
		Task:                  agentPlannerActionReplanCyclePlanTaskToAgentPlannerTask(plan.Task),
		Todos:                 agentPlannerActionReplanCyclePlanTodosToAgentPlannerTodos(plan.Todos),
		ContextRefs:           agentPlannerActionReplanCyclePlanContextRefsToAgentContextRefs(plan.ContextRefs),
		NextAction:            agentPlannerActionReplanCyclePlanNextActionToAgentNextAction(plan.NextAction),
		Reasoning:             plan.Reasoning,
		Actions:               agentPlannerActionReplanCyclePlanActionsToAgentPlannerActions(plan.Actions),
		ToolCalls:             agentPlannerActionReplanCyclePlanActionsToAgentPlannerActions(plan.ToolCalls),
	}
}

func agentPlannerActionReplanCyclePlanRecommendationOptionsFromAgentRecommendationOptions(options []agentRecommendationOpt) []runtime.AgentLoopRecommendationOption {
	if options == nil {
		return nil
	}
	out := make([]runtime.AgentLoopRecommendationOption, len(options))
	for i, option := range options {
		out[i] = runtime.AgentLoopRecommendationOption{
			ID:      option.ID,
			Title:   option.Title,
			Detail:  option.Detail,
			Prompt:  option.Prompt,
			Command: option.Command,
		}
	}
	return out
}

func agentPlannerActionReplanCyclePlanRecommendationOptionsToAgentRecommendationOptions(options []runtime.AgentLoopRecommendationOption) []agentRecommendationOpt {
	if options == nil {
		return nil
	}
	out := make([]agentRecommendationOpt, len(options))
	for i, option := range options {
		out[i] = agentRecommendationOpt{
			ID:      option.ID,
			Title:   option.Title,
			Detail:  option.Detail,
			Prompt:  option.Prompt,
			Command: option.Command,
		}
	}
	return out
}

func agentPlannerActionReplanCyclePlanTaskFromAgentPlannerTask(task *agentPlannerTask) *runtime.AgentLoopPlanTask {
	if task == nil {
		return nil
	}
	return &runtime.AgentLoopPlanTask{
		Goal:        task.Goal,
		Kind:        task.Kind,
		Status:      task.Status,
		ActiveRunID: task.ActiveRunID,
		Summary:     task.Summary,
	}
}

func agentPlannerActionReplanCyclePlanTaskToAgentPlannerTask(task *runtime.AgentLoopPlanTask) *agentPlannerTask {
	if task == nil {
		return nil
	}
	return &agentPlannerTask{
		Goal:        task.Goal,
		Kind:        task.Kind,
		Status:      task.Status,
		ActiveRunID: task.ActiveRunID,
		Summary:     task.Summary,
	}
}

func agentPlannerActionReplanCyclePlanTodosFromAgentPlannerTodos(todos []agentPlannerTodo) []runtime.AgentLoopPlanTodo {
	if todos == nil {
		return nil
	}
	out := make([]runtime.AgentLoopPlanTodo, len(todos))
	for i, todo := range todos {
		out[i] = runtime.AgentLoopPlanTodo{
			Title:         todo.Title,
			Detail:        todo.Detail,
			Status:        todo.Status,
			Priority:      todo.Priority,
			ContextRefIDs: append([]string(nil), todo.ContextRefIDs...),
			BlockedReason: todo.BlockedReason,
		}
	}
	return out
}

func agentPlannerActionReplanCyclePlanTodosToAgentPlannerTodos(todos []runtime.AgentLoopPlanTodo) []agentPlannerTodo {
	if todos == nil {
		return nil
	}
	out := make([]agentPlannerTodo, len(todos))
	for i, todo := range todos {
		out[i] = agentPlannerTodo{
			Title:         todo.Title,
			Detail:        todo.Detail,
			Status:        todo.Status,
			Priority:      todo.Priority,
			ContextRefIDs: append([]string(nil), todo.ContextRefIDs...),
			BlockedReason: todo.BlockedReason,
		}
	}
	return out
}

func agentPlannerActionReplanCyclePlanContextRefsFromAgentContextRefs(refs []agentContextRef) []runtime.AgentLoopContextRef {
	if refs == nil {
		return nil
	}
	out := make([]runtime.AgentLoopContextRef, len(refs))
	for i, ref := range refs {
		out[i] = runtime.AgentLoopContextRef{
			ID:      ref.ID,
			Kind:    ref.Kind,
			RefID:   ref.RefID,
			Label:   ref.Label,
			To:      ref.To,
			Summary: ref.Summary,
		}
	}
	return out
}

func agentPlannerActionReplanCyclePlanContextRefsToAgentContextRefs(refs []runtime.AgentLoopContextRef) []agentContextRef {
	if refs == nil {
		return nil
	}
	out := make([]agentContextRef, len(refs))
	for i, ref := range refs {
		out[i] = agentContextRef{
			ID:      ref.ID,
			Kind:    ref.Kind,
			RefID:   ref.RefID,
			Label:   ref.Label,
			To:      ref.To,
			Summary: ref.Summary,
		}
	}
	return out
}

func agentPlannerActionReplanCyclePlanNextActionFromAgentNextAction(nextAction *agentNextAction) *runtime.AgentLoopNextAction {
	if nextAction == nil {
		return nil
	}
	return &runtime.AgentLoopNextAction{
		Type:            nextAction.Type,
		Reason:          nextAction.Reason,
		SuggestedPrompt: nextAction.SuggestedPrompt,
		AutoContinue:    nextAction.AutoContinue,
	}
}

func agentPlannerActionReplanCyclePlanNextActionToAgentNextAction(nextAction *runtime.AgentLoopNextAction) *agentNextAction {
	if nextAction == nil {
		return nil
	}
	return &agentNextAction{
		Type:            nextAction.Type,
		Reason:          nextAction.Reason,
		SuggestedPrompt: nextAction.SuggestedPrompt,
		AutoContinue:    nextAction.AutoContinue,
	}
}

func agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action agentPlannerAction) runtime.AgentLoopAction {
	return runtime.AgentLoopAction{
		Name:      action.Name,
		Arguments: runtime.CloneAgentLoopActionArguments(action.Arguments),
	}
}

func cloneAgentLoopAction(action runtime.AgentLoopAction) runtime.AgentLoopAction {
	return runtime.AgentLoopAction{
		Name:      action.Name,
		Arguments: runtime.CloneAgentLoopActionArguments(action.Arguments),
	}
}

func agentPlannerActionFromAgentLoopAction(action runtime.AgentLoopAction) agentPlannerAction {
	return agentPlannerAction{
		Name:      action.Name,
		Arguments: runtime.CloneAgentLoopActionArguments(action.Arguments),
	}
}

func agentPlannerActionReplanCyclePlanActionsFromAgentPlannerActions(actions []agentPlannerAction) []runtime.AgentLoopAction {
	if actions == nil {
		return nil
	}
	out := make([]runtime.AgentLoopAction, len(actions))
	for i, action := range actions {
		out[i] = agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action)
	}
	return out
}

func agentPlannerActionReplanCyclePlanActionsToAgentPlannerActions(actions []runtime.AgentLoopAction) []agentPlannerAction {
	if actions == nil {
		return nil
	}
	out := make([]agentPlannerAction, len(actions))
	for i, action := range actions {
		out[i] = agentPlannerActionFromAgentLoopAction(action)
	}
	return out
}

func cloneAgentPlannerResponse(plan *agentPlannerResponse) *agentPlannerResponse {
	if plan == nil {
		return nil
	}
	return agentPlannerResponseFromAgentLoopPlan(agentPlannerActionReplanCyclePlanItemFromAgentPlannerResponse(*plan))
}

func cloneAgentPlannerActions(actions []agentPlannerAction) []agentPlannerAction {
	return agentPlannerActionReplanCyclePlanActionsToAgentPlannerActions(agentPlannerActionReplanCyclePlanActionsFromAgentPlannerActions(actions))
}

type agentPlannerActionReplanCycleMemorySummary struct {
	present bool
	item    runtime.AgentLoopMemorySummary
}

func agentPlannerActionReplanCycleMemorySummaryFromAgentMemorySummary(summary *agentMemorySummary) agentPlannerActionReplanCycleMemorySummary {
	if summary == nil {
		return agentPlannerActionReplanCycleMemorySummary{}
	}
	return agentPlannerActionReplanCycleMemorySummary{
		present: true,
		item:    agentLoopMemorySummaryFromAgentMemorySummary(*summary),
	}
}

func (summary agentPlannerActionReplanCycleMemorySummary) legacyAgentMemorySummary() *agentMemorySummary {
	if !summary.present {
		return nil
	}
	return agentMemorySummaryFromAgentLoopMemorySummary(summary.item)
}

func (summary agentPlannerActionReplanCycleMemorySummary) agentLoopMemorySummary() *runtime.AgentLoopMemorySummary {
	if !summary.present {
		return nil
	}
	cloned := summary.item.Clone()
	return &cloned
}

func agentPlannerActionReplanCycleMemorySummaryFromAgentLoopMemorySummary(summary *runtime.AgentLoopMemorySummary) agentPlannerActionReplanCycleMemorySummary {
	if summary == nil {
		return agentPlannerActionReplanCycleMemorySummary{}
	}
	return agentPlannerActionReplanCycleMemorySummary{
		present: true,
		item:    summary.Clone(),
	}
}

func agentLoopMemorySummaryFromAgentMemorySummary(summary agentMemorySummary) runtime.AgentLoopMemorySummary {
	var lastMessageAt *time.Time
	if summary.LastMessageAt != nil {
		last := *summary.LastMessageAt
		lastMessageAt = &last
	}
	return runtime.AgentLoopMemorySummary{
		Summary:       summary.Summary,
		MessageCount:  summary.MessageCount,
		LastMessageID: summary.LastMessageID,
		LastMessageAt: lastMessageAt,
		UpdatedAt:     summary.UpdatedAt,
	}
}

func agentMemorySummaryFromAgentLoopMemorySummary(summary runtime.AgentLoopMemorySummary) *agentMemorySummary {
	var lastMessageAt *time.Time
	if summary.LastMessageAt != nil {
		last := *summary.LastMessageAt
		lastMessageAt = &last
	}
	return &agentMemorySummary{
		Summary:       summary.Summary,
		MessageCount:  summary.MessageCount,
		LastMessageID: summary.LastMessageID,
		LastMessageAt: lastMessageAt,
		UpdatedAt:     summary.UpdatedAt,
	}
}

type agentPlannerActionReplanCycleRequest struct {
	Turn          runtime.TurnContext
	Language      string
	Operator      string
	LLMRequest    runtime.AgentLoopLLMRequest
	Authorization runtime.AgentLoopAuthorization
	MemorySummary agentPlannerActionReplanCycleMemorySummary
	ToolContext   agentPlannerActionReplanCycleToolContext
	Plan          agentPlannerActionReplanCyclePlan
	State         agentPlannerActionReplanCycleState
	Options       agentPlannerActionReplanCycleOptions
}

type agentPlannerActionReplanCycleResult struct {
	State    agentPlannerActionReplanCycleState
	Plan     agentPlannerActionReplanCyclePlan
	Continue bool
}

const (
	agentPlannerActionReplanCycleMetadataActionAppendOrigin                      = "action_append_origin"
	agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage = "append_action_tool_calls_before_empty_message"
	agentPlannerActionReplanCycleMetadataContinue                                = "continue"
	agentPlannerActionReplanCycleStopReasonLegacyDaemonLoopExecuted              = "legacy_daemon_agent_loop_executed"
)

func (req agentPlannerActionReplanCycleRequest) agentLoopRequest(mode runtime.AgentLoopMode) runtime.AgentLoopRequest {
	state := req.State.state.Clone()
	return runtime.AgentLoopRequest{
		Turn:          runtime.CloneAgentLoopTurnContext(req.Turn),
		Mode:          mode,
		Language:      req.Language,
		Operator:      req.Operator,
		LLMRequest:    req.LLMRequest.Clone(),
		History:       runtime.CloneAgentLoopMessages(state.LoopHistory),
		MemorySummary: req.MemorySummary.agentLoopMemorySummary(),
		InitialPlan:   req.Plan.agentLoopPlan(),
		State:         state,
		Authorization: req.Authorization.Clone(),
		ToolServices:  req.ToolContext.services.Clone(),
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataActionAppendOrigin:                      req.Options.ActionAppendOrigin,
			agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage: req.Options.AppendActionToolCallsBeforeEmptyMessage,
		},
	}
}

func agentPlannerActionReplanCycleRequestFromAgentLoopRequest(loopReq runtime.AgentLoopRequest, options agentPlannerActionReplanCycleOptions) agentPlannerActionReplanCycleRequest {
	cloned := loopReq.Clone()
	if options == (agentPlannerActionReplanCycleOptions{}) {
		options = agentPlannerActionReplanCycleOptionsFromAgentLoopMetadata(cloned.Metadata)
	}
	return agentPlannerActionReplanCycleRequest{
		Turn:          agentPlannerActionReplanCycleTurnFromAgentLoopRequest(cloned),
		Language:      cloned.Language,
		Operator:      cloned.Operator,
		LLMRequest:    cloned.LLMRequest.Clone(),
		Authorization: cloned.Authorization.Clone(),
		MemorySummary: agentPlannerActionReplanCycleMemorySummaryFromAgentLoopMemorySummary(cloned.MemorySummary),
		ToolContext:   agentPlannerActionReplanCycleToolContext{services: cloned.ToolServices.Clone()},
		Plan:          agentPlannerActionReplanCycleInitialPlanFromAgentLoopRequest(cloned),
		State:         agentPlannerActionReplanCycleState{state: agentPlannerActionReplanCycleStateFromAgentLoopRequest(cloned)},
		Options:       options,
	}
}

func agentPlannerActionReplanCycleTurnFromAgentLoopRequest(loopReq runtime.AgentLoopRequest) runtime.TurnContext {
	return runtime.CloneAgentLoopTurnContext(loopReq.Turn)
}

func agentPlannerActionReplanCycleTurnRefFromAgentLoopRequest(loopReq runtime.AgentLoopRequest) agentRuntimeTurnRef {
	return agentRuntimeTurnRefFromTurn(loopReq.Turn)
}

func agentPlannerActionReplanCycleStateFromAgentLoopRequest(loopReq runtime.AgentLoopRequest) runtime.AgentLoopState {
	state := loopReq.State.Clone()
	if len(state.LoopHistory) == 0 && len(loopReq.History) > 0 {
		state.LoopHistory = runtime.CloneAgentLoopMessages(loopReq.History)
	}
	return state
}

func agentPlannerActionReplanCycleInitialAgentLoopPlanFromRequest(loopReq runtime.AgentLoopRequest) *runtime.AgentLoopPlan {
	return runtime.CloneAgentLoopPlan(loopReq.InitialPlan)
}

func agentPlannerActionReplanCycleInitialPlanFromAgentLoopRequest(loopReq runtime.AgentLoopRequest) agentPlannerActionReplanCyclePlan {
	return agentPlannerActionReplanCyclePlanFromAgentLoopPlan(agentPlannerActionReplanCycleInitialAgentLoopPlanFromRequest(loopReq))
}

func agentPlannerActionReplanCycleOptionsFromAgentLoopRequest(loopReq runtime.AgentLoopRequest) agentPlannerActionReplanCycleOptions {
	options := agentPlannerActionReplanCycleOptionsFromAgentLoopMetadata(loopReq.Metadata)
	if options.ActionAppendOrigin == "" {
		options.ActionAppendOrigin = "planner_loop"
	}
	return options
}

func agentPlannerActionReplanCycleOptionsFromAgentLoopMetadata(metadata map[string]any) agentPlannerActionReplanCycleOptions {
	return agentPlannerActionReplanCycleOptions{
		ActionAppendOrigin:                      stringFromAny(metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin]),
		AppendActionToolCallsBeforeEmptyMessage: boolFromAny(metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage]),
	}
}

func agentPlannerActionReplanCycleMetadataFromOptions(options agentPlannerActionReplanCycleOptions) map[string]any {
	return map[string]any{
		agentPlannerActionReplanCycleMetadataActionAppendOrigin:                      options.ActionAppendOrigin,
		agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage: options.AppendActionToolCallsBeforeEmptyMessage,
	}
}

func (result agentPlannerActionReplanCycleResult) agentLoopResult(stopReason string) runtime.AgentLoopResult {
	state := result.State.state.Clone()
	return runtime.AgentLoopResult{
		State:      state,
		Messages:   runtime.CloneAgentLoopMessages(state.ResponseMessages),
		ToolCalls:  runtime.CloneAgentLoopToolCalls(state.ToolCalls),
		FinalPlan:  result.Plan.agentLoopPlan(),
		StopReason: stopReason,
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataContinue: result.Continue,
		},
	}
}

func agentPlannerActionReplanCycleResultFromAgentLoopResult(loopResult runtime.AgentLoopResult) agentPlannerActionReplanCycleResult {
	cloned := normalizeAgentPlannerActionReplanCycleAgentLoopResult(loopResult)
	state := cloned.State.Clone()
	return agentPlannerActionReplanCycleResult{
		State:    agentPlannerActionReplanCycleState{state: state},
		Plan:     agentPlannerActionReplanCyclePlanFromAgentLoopPlan(cloned.FinalPlan),
		Continue: boolFromAny(cloned.Metadata[agentPlannerActionReplanCycleMetadataContinue]),
	}
}

func normalizeAgentPlannerActionReplanCycleAgentLoopResult(loopResult runtime.AgentLoopResult) runtime.AgentLoopResult {
	cloned := loopResult.Clone()
	if len(cloned.State.ResponseMessages) == 0 && len(cloned.Messages) > 0 {
		cloned.State.ResponseMessages = runtime.CloneAgentLoopMessages(cloned.Messages)
	}
	if len(cloned.State.ToolCalls) == 0 && len(cloned.ToolCalls) > 0 {
		cloned.State.ToolCalls = runtime.CloneAgentLoopToolCalls(cloned.ToolCalls)
	}
	return cloned
}

func agentPlannerActionReplanCycleContinueFromAgentLoopResult(loopResult runtime.AgentLoopResult) bool {
	return boolFromAny(loopResult.Metadata[agentPlannerActionReplanCycleMetadataContinue])
}

type agentPlannerActionReplanCycleActionExecutionResult struct {
	Messages              agentPlannerActionReplanCycleMessages
	ToolCalls             agentPlannerActionReplanCycleToolCalls
	AuthorizationRequired bool
}

type agentPlannerActionReplanCycleSynthesisResult struct {
	Messages   agentPlannerActionReplanCycleMessages
	ToolCalls  agentPlannerActionReplanCycleToolCalls
	Plan       agentPlannerActionReplanCyclePlan
	legacyPlan *agentPlannerResponse
}

func agentPlannerActionReplanCycleTurnContext(sessionID string, turnRef agentRuntimeTurnRef, prompt, origin, operationLevel string) runtime.TurnContext {
	return runtime.TurnContext{
		SessionID:      sessionID,
		TaskID:         turnRef.TaskID,
		RunID:          turnRef.RunID,
		TurnID:         turnRef.TurnID,
		Prompt:         prompt,
		Origin:         origin,
		OperationLevel: operationLevel,
	}
}

func (m *agentManager) runRuntimeAgentPlannerActionReplanCycle(ctx context.Context, mode runtime.AgentLoopMode, req agentPlannerActionReplanCycleRequest) agentPlannerActionReplanCycleResult {
	return agentPlannerActionReplanCycleResultFromAgentLoopResult(m.runRuntimeAgentPlannerActionReplanCycleAgentLoop(ctx, req.agentLoopRequest(mode)))
}

func (m *agentManager) runRuntimeAgentPlannerActionReplanCycleAgentLoop(ctx context.Context, loopReq runtime.AgentLoopRequest) runtime.AgentLoopResult {
	req := loopReq.Clone()
	eng := m.runtimeEngine()
	if eng == nil {
		return normalizeAgentPlannerActionReplanCycleAgentLoopResult(m.runAgentPlannerActionReplanCycleAgentLoop(ctx, req))
	}
	loopResult, err := eng.RunAgentLoop(ctx, req)
	if err != nil {
		return normalizeAgentPlannerActionReplanCycleAgentLoopResult(m.runAgentPlannerActionReplanCycleAgentLoop(ctx, req))
	}
	return normalizeAgentPlannerActionReplanCycleAgentLoopResult(loopResult)
}

func (m *agentManager) runAgentPlannerActionReplanCycle(ctx context.Context, req agentPlannerActionReplanCycleRequest) agentPlannerActionReplanCycleResult {
	return agentPlannerActionReplanCycleResultFromAgentLoopResult(m.runAgentPlannerActionReplanCycleAgentLoop(ctx, req.agentLoopRequest("")))
}

func (m *agentManager) runAgentPlannerActionReplanCycleAgentLoop(ctx context.Context, loopReq runtime.AgentLoopRequest) runtime.AgentLoopResult {
	loopReq = loopReq.Clone()
	turn := agentPlannerActionReplanCycleTurnFromAgentLoopRequest(loopReq)
	turnRef := agentPlannerActionReplanCycleTurnRefFromAgentLoopRequest(loopReq)
	options := agentPlannerActionReplanCycleOptionsFromAgentLoopRequest(loopReq)
	state := agentPlannerActionReplanCycleStateFromAgentLoopRequest(loopReq)
	finalPlan := agentPlannerActionReplanCycleInitialAgentLoopPlanFromRequest(loopReq)
	finish := func(continueLoop bool) runtime.AgentLoopResult {
		clonedState := state.Clone()
		return runtime.AgentLoopResult{
			State:      clonedState,
			Messages:   runtime.CloneAgentLoopMessages(clonedState.ResponseMessages),
			ToolCalls:  runtime.CloneAgentLoopToolCalls(clonedState.ToolCalls),
			FinalPlan:  runtime.CloneAgentLoopPlan(finalPlan),
			StopReason: agentPlannerActionReplanCycleStopReasonLegacyDaemonLoopExecuted,
			Metadata: map[string]any{
				agentPlannerActionReplanCycleMetadataContinue: continueLoop,
			},
		}
	}

	actionExecution := m.runAgentPlannerActionReplanCycleActionExecutionAgentLoop(ctx, loopReq)
	actionMessages := agentPlannerActionReplanCycleMessagesFromAgentLoopMessages(actionExecution.Messages).withPhase("step")
	actionToolCalls := agentPlannerActionReplanCycleToolCallsFromAgentLoopToolCalls(actionExecution.ToolCalls).withPhase("step")
	if actionMessages.len() > 0 {
		m.appendMessagesForTurn(ctx, turn.SessionID, turnRef, turn.Prompt, options.ActionAppendOrigin, turn.OperationLevel, actionMessages.legacyAgentMessages()...)
		state.ResponseMessages = append(state.ResponseMessages, actionMessages.agentLoopMessages()...)
		state.LoopHistory = append(state.LoopHistory, actionMessages.agentLoopMessages()...)
	}
	if options.AppendActionToolCallsBeforeEmptyMessage {
		state.ToolCalls = append(state.ToolCalls, actionToolCalls.agentLoopToolCalls()...)
	}
	if actionMessages.len() == 0 {
		return finish(false)
	}
	if !options.AppendActionToolCallsBeforeEmptyMessage {
		state.ToolCalls = append(state.ToolCalls, actionToolCalls.agentLoopToolCalls()...)
	}
	if actionExecution.AuthorizationRequired {
		return finish(false)
	}

	synthesis, synthesisErr := m.runAgentPlannerActionReplanCycleSynthesisAgentLoop(
		ctx,
		loopReq,
		state.Clone(),
		actionMessages.agentLoopMessages(),
	)
	if synthesisErr != nil {
		warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
		warningTool = agentToolCallWithPhase(warningTool, "step_replan")
		warningMessage := agentMessageWithPhase(m.message(
			loopReq.Language,
			"tool",
			agentText(loopReq.Language, "工具已执行，但 StepReplan 总结失败："+synthesisErr.Error(), "Tools executed, but StepReplan synthesis failed: "+synthesisErr.Error()),
			[]agentToolCall{warningTool},
			nil,
		), "step_replan")
		warningMessages := agentPlannerActionReplanCycleMessagesFromAgentMessages([]agentMessage{warningMessage})
		warningToolCalls := agentPlannerActionReplanCycleToolCallsFromAgentToolCalls([]agentToolCall{warningTool})
		m.appendMessagesForTurn(ctx, turn.SessionID, turnRef, turn.Prompt, options.ActionAppendOrigin, turn.OperationLevel, warningMessages.legacyAgentMessages()...)
		state.ResponseMessages = append(state.ResponseMessages, warningMessages.agentLoopMessages()...)
		state.ToolCalls = append(state.ToolCalls, warningToolCalls.agentLoopToolCalls()...)
		return finish(false)
	}
	synthesisMessages := agentPlannerActionReplanCycleMessagesFromAgentLoopMessages(synthesis.Messages)
	synthesisToolCalls := agentPlannerActionReplanCycleToolCallsFromAgentLoopToolCalls(synthesis.ToolCalls)
	synthesisPlan := runtime.CloneAgentLoopPlan(synthesis.FinalPlan)
	if synthesisMessages.len() > 0 {
		m.appendMessagesWithPlannerForTurn(ctx, turn.SessionID, turnRef, turn.Prompt, string(runtime.LLMStageToolSynthesis), turn.OperationLevel, legacyPlannerResponseFromAgentLoopPlan(synthesisPlan), synthesisMessages.legacyAgentMessages()...)
		state.ResponseMessages = append(state.ResponseMessages, synthesisMessages.agentLoopMessages()...)
		state.LoopHistory = append(state.LoopHistory, synthesisMessages.agentLoopMessages()...)
		state.ToolCalls = append(state.ToolCalls, synthesisToolCalls.agentLoopToolCalls()...)
	}
	finalPlan = synthesisPlan
	return finish(true)
}

func (m *agentManager) runAgentPlannerActionReplanCycleActionExecution(ctx context.Context, req agentPlannerActionReplanCycleRequest) agentPlannerActionReplanCycleActionExecutionResult {
	result := m.runAgentPlannerActionReplanCycleActionExecutionAgentLoop(ctx, req.agentLoopRequest(""))
	return agentPlannerActionReplanCycleActionExecutionResult{
		Messages:              agentPlannerActionReplanCycleMessagesFromAgentLoopMessages(result.Messages),
		ToolCalls:             agentPlannerActionReplanCycleToolCallsFromAgentLoopToolCalls(result.ToolCalls),
		AuthorizationRequired: result.AuthorizationRequired,
	}
}

func (m *agentManager) runAgentPlannerActionReplanCycleActionExecutionAgentLoop(ctx context.Context, loopReq runtime.AgentLoopRequest) runtime.AgentLoopResult {
	req := loopReq.Clone()
	turnRef := agentRuntimeTurnRefFromTurn(req.Turn)
	actions := agentLoopPlanActions(req.InitialPlan)
	actionMessages, actionToolCalls, authorizationRequired := m.executeAgentPlannerActionList(
		ctx,
		req.Turn.SessionID,
		turnRef,
		req.Turn.Prompt,
		req.Language,
		req.Operator,
		req.Turn.OperationLevel,
		legacyRouterServicesFromAgentLoopToolServiceBinding(req.ToolServices),
		actions,
		agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(req.Authorization.AuthProfiles),
		req.Authorization.DefaultAuthProfileID,
	)
	return runtime.AgentLoopResult{
		Messages:              agentLoopMessagesFromAgentMessages(actionMessages),
		ToolCalls:             agentLoopToolCallItemsFromAgentToolCalls(actionToolCalls),
		AuthorizationRequired: authorizationRequired,
	}
}

func (m *agentManager) runAgentPlannerActionReplanCycleSynthesis(ctx context.Context, req agentPlannerActionReplanCycleRequest, state agentPlannerActionReplanCycleState, actionMessages agentPlannerActionReplanCycleMessages) (agentPlannerActionReplanCycleSynthesisResult, error) {
	result, synthesisErr := m.runAgentPlannerActionReplanCycleSynthesisAgentLoop(ctx, req.agentLoopRequest(""), state.state, actionMessages.agentLoopMessages())
	if synthesisErr != nil {
		return agentPlannerActionReplanCycleSynthesisResult{}, synthesisErr
	}
	plan := agentPlannerActionReplanCyclePlanFromAgentLoopPlan(result.FinalPlan)
	return agentPlannerActionReplanCycleSynthesisResult{
		Messages:   agentPlannerActionReplanCycleMessagesFromAgentLoopMessages(result.Messages),
		ToolCalls:  agentPlannerActionReplanCycleToolCallsFromAgentLoopToolCalls(result.ToolCalls),
		Plan:       plan,
		legacyPlan: plan.legacyPlannerResponse(),
	}, nil
}

func (m *agentManager) runAgentPlannerActionReplanCycleSynthesisAgentLoop(ctx context.Context, loopReq runtime.AgentLoopRequest, state runtime.AgentLoopState, actionMessages []runtime.AgentLoopMessage) (runtime.AgentLoopResult, error) {
	req := loopReq.Clone()
	state = state.Clone()
	actionMessages = runtime.CloneAgentLoopMessages(actionMessages)
	turnRef := agentRuntimeTurnRefFromTurn(req.Turn)
	synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.runRuntimeAgentLLMToolSynthesis(
		ctx,
		req.Turn.SessionID,
		turnRef,
		req.Turn.Prompt,
		req.Language,
		req.Turn.OperationLevel,
		sendMessageRequestFromAgentLoopLLMRequest(req.LLMRequest),
		agentMessagesFromAgentLoopMessages(state.LoopHistory),
		agentMemorySummaryPtrFromAgentLoopMemorySummary(req.MemorySummary),
		agentMessagesFromAgentLoopMessages(actionMessages),
	)
	if synthesisErr != nil {
		return runtime.AgentLoopResult{}, synthesisErr
	}
	return runtime.AgentLoopResult{
		Messages:  agentLoopMessagesFromAgentMessages(synthesisMessages),
		ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(synthesisToolCalls),
		FinalPlan: agentPlannerActionReplanCyclePlanFromPlannerResponse(synthesisPlan).agentLoopPlan(),
	}, nil
}

func reqMaxAgentReplans(req sendAgentMessageRequest) int {
	// Keep the first implementation conservative: one Plan plus at most three
	// Step/Replan cycles is enough for health/workspace/run/scan workflows and
	// prevents a weak provider from looping.
	_ = req
	return 3
}

type agentPlannerActionGateKind string

const (
	agentPlannerActionGateAllow                       agentPlannerActionGateKind = "allow"
	agentPlannerActionGateCapabilityBlocked           agentPlannerActionGateKind = "capability_blocked"
	agentPlannerActionGateExplicitIntentRequired      agentPlannerActionGateKind = "explicit_operator_intent_required"
	agentPlannerActionGateOperationLevelRequired      agentPlannerActionGateKind = "operation_level_required"
	agentPlannerActionGateManualAuthorizationRequired agentPlannerActionGateKind = "manual_tool_authorization_required"
)

type agentPlannerActionGateDecision struct {
	Kind            agentPlannerActionGateKind
	ActionName      string
	RequiredLevel   string
	Policy          string
	Reason          string
	IntentID        string
	SuggestedPrompt string
}

func decideAgentPlannerActionGate(operatorPrompt, language, operationLevel string, action agentPlannerAction) agentPlannerActionGateDecision {
	return decideAgentPlannerActionGateFromAgentLoopAction(operatorPrompt, language, operationLevel, agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action))
}

func decideAgentPlannerActionGateFromAgentLoopAction(operatorPrompt, language, operationLevel string, action runtime.AgentLoopAction) agentPlannerActionGateDecision {
	actionName := normalizeAgentPlannerActionName(action.Name)
	if actionName == "" {
		actionName = strings.TrimSpace(action.Name)
	}
	requiredLevel := agentOperationLevelRequiredForPlannerAction(actionName, action.Arguments)
	decision := agentPlannerActionGateDecision{
		Kind:          agentPlannerActionGateAllow,
		ActionName:    actionName,
		RequiredLevel: requiredLevel,
	}

	if mismatchReason, suggestedPrompt, mismatch := agentPlannerActionCapabilityMismatch(operatorPrompt, actionName, language); mismatch {
		decision.Kind = agentPlannerActionGateCapabilityBlocked
		decision.IntentID = agentClassifyCapabilityIntent(operatorPrompt).ID
		decision.Reason = mismatchReason
		decision.SuggestedPrompt = suggestedPrompt
		return decision
	}

	if agentOperationBypassPermissions(operationLevel) {
		return decision
	}

	if !agentPlannerActionAllowedByPrompt(operatorPrompt, actionName) {
		decision.Kind = agentPlannerActionGateExplicitIntentRequired
		decision.Policy = "explicit_operator_intent_required"
		decision.Reason = agentText(language, "模型计划了一个需要操作者明确确认的动作；已暂停执行，等待授权。", "The model planned an action that requires explicit operator confirmation; execution is paused until authorization.")
		return decision
	}

	if !agentOperationLevelAtLeast(operationLevel, requiredLevel) {
		decision.Kind = agentPlannerActionGateOperationLevelRequired
		decision.Policy = "operation_level_required"
		decision.Reason = agentText(
			language,
			fmt.Sprintf("当前权限级别为 %s，执行 %s 需要 %s；已暂停执行，等待授权升级。", operationLevel, actionName, requiredLevel),
			fmt.Sprintf("Current operation level is %s; executing %s requires %s. Execution is paused until authorization is granted.", operationLevel, actionName, requiredLevel),
		)
		return decision
	}

	if agentPlannerActionRequiresManualAuthorization(actionName) {
		decision.Kind = agentPlannerActionGateManualAuthorizationRequired
		decision.Policy = "manual_tool_authorization_required"
		decision.Reason = agentText(
			language,
			fmt.Sprintf("%s 属于需要人工确认的工具动作；已暂停执行，等待授权。", actionName),
			fmt.Sprintf("%s is a tool action that requires human confirmation; execution is paused until authorization.", actionName),
		)
		return decision
	}

	return decision
}

func newAgentPlannerActionQueueID(now time.Time) string {
	return strings.Replace(runs.NewJobID(now.UTC()), "job_", "queue_", 1)
}

func agentPlannerActionQueueMetadata(queueID string, index, count int, actionName string) map[string]interface{} {
	return map[string]interface{}{
		"action_queue_id": queueID,
		"action_index":    index,
		"action_count":    count,
		"action_name":     actionName,
	}
}

func (m *agentManager) executeAgentPlannerActions(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, operatorPrompt, language, operator, operationLevel string, svc routerServices, plan *agentPlannerResponse, authProfiles []agentHTTPAuthProfile, defaultAuthProfileID string) ([]agentMessage, []agentToolCall, bool) {
	return m.executeAgentPlannerActionList(ctx, sessionID, turnRef, operatorPrompt, language, operator, operationLevel, svc, agentPlannerActionReplanCyclePlanActionsFromAgentPlannerActions(agentPlannerActions(plan)), authProfiles, defaultAuthProfileID)
}

func (m *agentManager) executeAgentPlannerActionList(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, operatorPrompt, language, operator, operationLevel string, svc routerServices, actions []runtime.AgentLoopAction, authProfiles []agentHTTPAuthProfile, defaultAuthProfileID string) ([]agentMessage, []agentToolCall, bool) {
	actions = runtime.CloneAgentLoopActions(actions)
	if len(actions) == 0 {
		return nil, nil, false
	}
	if len(actions) > 6 {
		actions = actions[:6]
	}

	var messages []agentMessage
	var toolCalls []agentToolCall
	queueID := newAgentPlannerActionQueueID(time.Now().UTC())
	actionCount := len(actions)
	for idx, action := range actions {
		actionIndex := idx + 1
		gateDecision := decideAgentPlannerActionGateFromAgentLoopAction(operatorPrompt, language, operationLevel, action)
		actionName := gateDecision.ActionName
		var actionMessages []agentMessage
		var actionToolCalls []agentToolCall
		var err error
		if gateDecision.Kind == agentPlannerActionGateCapabilityBlocked {
			policyErr := services.NewError(services.CodePolicyDenied, "non-equivalent tool action blocked", map[string]any{
				"intent":              gateDecision.IntentID,
				"action":              actionName,
				"suggested_prompt":    gateDecision.SuggestedPrompt,
				"capability_mismatch": true,
			}, nil)
			tool := failedAgentToolCall("agent.capability_match", policyErr)
			tool.Result = map[string]any{
				"intent":           gateDecision.IntentID,
				"action":           actionName,
				"suggested_prompt": gateDecision.SuggestedPrompt,
			}
			m.markAgentCapabilityBlockedForTurn(ctx, sessionID, turnRef, operatorPrompt, language, operationLevel, actionName, gateDecision.IntentID, gateDecision.Reason, gateDecision.SuggestedPrompt)
			message := agentMessageWithPhase(m.message(language, "agent", gateDecision.Reason, []agentToolCall{tool}, nil), "step_replan")
			messages = append(messages, message)
			toolCalls = append(toolCalls, agentToolCallWithPhase(tool, "step_replan"))
			return messages, toolCalls, true
		} else if gateDecision.Kind == agentPlannerActionGateExplicitIntentRequired {
			authMessage := m.requestAgentAuthorizationForTurn(
				ctx,
				sessionID,
				turnRef,
				language,
				operatorPrompt,
				operationLevel,
				action,
				actionName,
				gateDecision.RequiredLevel,
				gateDecision.Policy,
				gateDecision.Reason,
			)
			authMessage = agentMessageWithPhase(authMessage, "step")
			messages = append(messages, authMessage)
			return messages, toolCalls, true
		} else if gateDecision.Kind == agentPlannerActionGateOperationLevelRequired {
			authMessage := m.requestAgentAuthorizationForTurn(
				ctx,
				sessionID,
				turnRef,
				language,
				operatorPrompt,
				operationLevel,
				action,
				actionName,
				gateDecision.RequiredLevel,
				gateDecision.Policy,
				gateDecision.Reason,
			)
			authMessage = agentMessageWithPhase(authMessage, "step")
			messages = append(messages, authMessage)
			return messages, toolCalls, true
		} else if gateDecision.Kind == agentPlannerActionGateManualAuthorizationRequired {
			authMessage := m.requestAgentAuthorizationForTurn(
				ctx,
				sessionID,
				turnRef,
				language,
				operatorPrompt,
				operationLevel,
				action,
				actionName,
				gateDecision.RequiredLevel,
				gateDecision.Policy,
				gateDecision.Reason,
			)
			authMessage = agentMessageWithPhase(authMessage, "step")
			messages = append(messages, authMessage)
			return messages, toolCalls, true
		} else {
			actionMessages, actionToolCalls, err = m.runRuntimePlannerActionExecution(ctx, sessionID, turnRef, operatorPrompt, language, operator, operationLevel, svc, action, authProfiles, defaultAuthProfileID)
		}
		if err != nil {
			if actionName == "" {
				actionName = "agent.action"
			}
			tool := failedAgentToolCall(actionName, err)
			tool.Result = map[string]any{
				"action": actionName,
			}
			var serviceErr *services.ServiceError
			if errors.As(err, &serviceErr) && len(serviceErr.Details) > 0 {
				tool.Result["details"] = serviceErr.Details
				for key, value := range serviceErr.Details {
					if key == "action" {
						continue
					}
					tool.Result[key] = value
				}
			}
			actionMessages = []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}
			actionToolCalls = []agentToolCall{tool}
		}
		actionMetadata := agentPlannerActionQueueMetadata(queueID, actionIndex, actionCount, actionName)
		for i := range actionToolCalls {
			actionToolCalls[i] = withAgentToolMetadata(actionToolCalls[i], actionMetadata)
		}
		for i := range actionMessages {
			for j := range actionMessages[i].ToolCalls {
				actionMessages[i].ToolCalls[j] = withAgentToolMetadata(actionMessages[i].ToolCalls[j], actionMetadata)
			}
		}
		messages = append(messages, actionMessages...)
		toolCalls = append(toolCalls, actionToolCalls...)
	}
	return messages, toolCalls, false
}

func (m *agentManager) requestAgentAuthorizationForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, language, operatorPrompt, operationLevel string, action runtime.AgentLoopAction, actionName, requiredLevel, policy, reason string) agentMessage {
	if payload, handled := m.runRuntimeRequestAuthorizationMutationForTurn(ctx, sessionID, turnRef, operatorPrompt, language, operationLevel, action, actionName, requiredLevel, policy, reason); handled {
		return payload.Message
	}
	payload := m.requestAgentAuthorizationLegacy(sessionID, language, operatorPrompt, agentPlannerActionFromAgentLoopAction(action), actionName, requiredLevel, policy, reason)
	return payload.Message
}

func (m *agentManager) requestAgentAuthorizationLegacy(sessionID, language, operatorPrompt string, action agentPlannerAction, actionName, requiredLevel, policy, reason string) daemonRequestAuthorizationMutationResultPayload {
	language = normalizeAgentLanguage(language)
	actionName = firstNonEmpty(normalizeAgentPlannerActionName(actionName), strings.TrimSpace(action.Name), "agent.action")
	requiredLevel = normalizeAgentOperationLevel(firstNonEmpty(requiredLevel, "read_only"))
	policy = firstNonEmpty(strings.TrimSpace(policy), "authorization_required")
	now := time.Now().UTC()
	auth := &agentAuthorizationRequest{
		ID:             strings.Replace(runs.NewJobID(now), "job_", "auth_", 1),
		ActionName:     actionName,
		Arguments:      cloneMapStringAny(action.Arguments),
		RequiredLevel:  requiredLevel,
		Policy:         policy,
		Reason:         trimSummary(reason, 360),
		OriginalPrompt: trimSummary(operatorPrompt, 1000),
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if auth.Arguments == nil {
		auth.Arguments = map[string]any{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr == nil {
			session = loaded
			ok = true
		}
	}
	if ok && session != nil {
		session.PendingAuthorization = cloneAgentAuthorizationRequest(auth)
		session.Status = "waiting_authorization"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "authorization_required",
			Reason:          auth.Reason,
			SuggestedPrompt: agentAuthorizationSuggestedPrompt(language, auth),
			AutoContinue:    false,
		}
		session.UpdatedAt = now
		_ = m.persistSessionLocked(session)
		message := agentMessageWithPhase(m.newMessageLocked(language, "agent", agentAuthorizationPromptMessage(language, auth), nil, nil), "step")
		return daemonRequestAuthorizationMutationResultPayload{
			Session:       cloneAgentSession(session),
			Authorization: *cloneAgentAuthorizationRequest(auth),
			Message:       message,
			Mutated:       true,
		}
	}

	message := agentMessageWithPhase(m.newMessageLocked(language, "agent", agentAuthorizationPromptMessage(language, auth), nil, nil), "step")
	return daemonRequestAuthorizationMutationResultPayload{
		Authorization: *cloneAgentAuthorizationRequest(auth),
		Message:       message,
		Mutated:       false,
	}
}

func (m *agentManager) markAgentCapabilityBlockedForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, operatorPrompt, language, operationLevel, actionName, intentID, reason, suggestedPrompt string) {
	if _, handled := m.runRuntimeCapabilityBlockedMutationForTurn(ctx, sessionID, turnRef, operatorPrompt, language, operationLevel, actionName, intentID, reason, suggestedPrompt); handled {
		return
	}
	m.markAgentCapabilityBlockedLegacy(sessionID, language, actionName, intentID, reason, suggestedPrompt)
}

func (m *agentManager) markAgentCapabilityBlockedLegacy(sessionID, language, actionName, intentID, reason, suggestedPrompt string) daemonCapabilityBlockedMutationResultPayload {
	actionName = firstNonEmpty(normalizeAgentPlannerActionName(actionName), strings.TrimSpace(actionName), "agent.action")
	intentID = strings.TrimSpace(intentID)
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr == nil {
			session = loaded
			ok = true
		}
	}
	if !ok || session == nil {
		return daemonCapabilityBlockedMutationResultPayload{
			ActionName:      actionName,
			IntentID:        intentID,
			Reason:          reason,
			SuggestedPrompt: suggestedPrompt,
			Mutated:         false,
		}
	}
	session.Status = "blocked"
	if session.ActiveTask != nil {
		session.ActiveTask.Status = "blocked"
		session.ActiveTask.UpdatedAt = now
	}
	for idx := range session.Todos {
		if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
			markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
		}
	}
	session.NextAction = &agentNextAction{
		Type:            "blocked",
		Reason:          trimSummary(reason, 360),
		SuggestedPrompt: suggestedPrompt,
		AutoContinue:    false,
	}
	session.UpdatedAt = now
	_ = m.persistSessionLocked(session)
	return daemonCapabilityBlockedMutationResultPayload{
		Session:         cloneAgentSession(session),
		ActionName:      actionName,
		IntentID:        intentID,
		Reason:          reason,
		SuggestedPrompt: suggestedPrompt,
		Mutated:         true,
	}
}

func agentAuthorizationPromptMessage(language string, auth *agentAuthorizationRequest) string {
	if auth == nil {
		return agentText(language, "需要授权确认：Agent 已暂停执行，等待操作者确认后继续。", "Authorization required: the Agent paused execution and is waiting for operator approval.")
	}
	target := agentAuthorizationTargetSummary(auth)
	if target == "" {
		target = agentText(language, "未识别到单独目标；请检查动作参数。", "No standalone target detected; review the action arguments.")
	}
	if normalizeAgentLanguage(language) == "en" {
		return fmt.Sprintf(
			"Authorization required. I paused before executing `%s`.\n\n- required level: `%s`\n- policy: `%s`\n- target/scope: %s\n\nApprove to run this exact pending action, or deny to stop it.",
			auth.ActionName,
			auth.RequiredLevel,
			auth.Policy,
			target,
		)
	}
	return fmt.Sprintf(
		"需要授权确认。我已在执行 `%s` 前暂停。\n\n- 所需权限：`%s`\n- 策略：`%s`\n- 目标/范围：%s\n\n点击允许后会继续执行这一个已挂起动作；点击拒绝则停止该动作。",
		auth.ActionName,
		auth.RequiredLevel,
		auth.Policy,
		target,
	)
}

func agentAuthorizationSuggestedPrompt(language string, auth *agentAuthorizationRequest) string {
	if auth == nil {
		return ""
	}
	target := agentAuthorizationTargetSummary(auth)
	if normalizeAgentLanguage(language) == "en" {
		return fmt.Sprintf("Review and approve pending action `%s`%s", auth.ActionName, suffixIfNotEmpty(" for ", target))
	}
	return fmt.Sprintf("确认是否允许执行挂起动作 `%s`%s", auth.ActionName, suffixIfNotEmpty("：", target))
}

func agentAuthorizationTargetSummary(auth *agentAuthorizationRequest) string {
	if auth == nil {
		return ""
	}
	args := auth.Arguments
	if args == nil {
		return ""
	}
	values := []string{}
	for _, key := range []string{"target", "url", "scope", "scope_url", "path", "file", "command", "run_id", "approval_id"} {
		value := strings.TrimSpace(stringFromAny(args[key]))
		if value != "" {
			values = append(values, key+"="+trimSummary(value, 120))
		}
	}
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, " ")
}

func suffixIfNotEmpty(prefix, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return prefix + value
}
