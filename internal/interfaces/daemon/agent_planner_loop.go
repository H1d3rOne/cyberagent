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

	m.appendMessagesWithPlanner(sessionID, plan, planMessages...)
	responseMessages := append([]agentMessage{}, planMessages...)
	toolCalls := append([]agentToolCall{}, planToolCalls...)
	loopHistory := append(cloneAgentMessages(history), planMessages...)

	maxLoops := reqMaxAgentReplans(req)
	for step := 0; step < maxLoops; step++ {
		if len(agentPlannerActions(plan)) == 0 {
			// Respect a structured model plan that intentionally stops at Plan and
			// asks the operator/runtime to continue. Fallback actions are only
			// created when the latest operator prompt clearly asks to execute now
			// but the provider forgot to emit actions.
			if step == 0 && agentPromptRequestsImmediateExecution(prompt) {
				fallback := agentFallbackPlannerFromPrompt(prompt, "", language)
				if fallback != nil && len(agentPlannerActions(fallback)) > 0 {
					plan = fallback
				} else {
					break
				}
			} else {
				break
			}
		}

		actionMessages, actionToolCalls, authorizationRequired := m.executeAgentPlannerActions(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc, plan, req.AuthProfiles, req.DefaultAuthProfileID)
		actionMessages = agentMessagesWithPhase(actionMessages, "step")
		actionToolCalls = agentToolCallsWithPhase(actionToolCalls, "step")
		if len(actionMessages) > 0 {
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, "planner_loop", operationLevel, actionMessages...)
			responseMessages = append(responseMessages, actionMessages...)
			loopHistory = append(loopHistory, actionMessages...)
		}
		toolCalls = append(toolCalls, actionToolCalls...)
		if authorizationRequired {
			break
		}
		if len(actionMessages) == 0 {
			break
		}

		synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.runRuntimeAgentLLMToolSynthesis(ctx, sessionID, turnRef, prompt, language, operationLevel, req, loopHistory, memorySummary, actionMessages)
		if synthesisErr != nil {
			warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
			warningTool = agentToolCallWithPhase(warningTool, "step_replan")
			warningMessage := agentMessageWithPhase(m.message(
				language,
				"tool",
				agentText(language, "工具已执行，但 StepReplan 总结失败："+synthesisErr.Error(), "Tools executed, but StepReplan synthesis failed: "+synthesisErr.Error()),
				[]agentToolCall{warningTool},
				nil,
			), "step_replan")
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, "planner_loop", operationLevel, warningMessage)
			responseMessages = append(responseMessages, warningMessage)
			toolCalls = append(toolCalls, warningTool)
			break
		}
		if len(synthesisMessages) > 0 {
			m.appendMessagesWithPlanner(sessionID, synthesisPlan, synthesisMessages...)
			responseMessages = append(responseMessages, synthesisMessages...)
			loopHistory = append(loopHistory, synthesisMessages...)
			toolCalls = append(toolCalls, synthesisToolCalls...)
		}

		plan = synthesisPlan
		if plan == nil || len(agentPlannerActions(plan)) == 0 {
			break
		}
		if plan.NextAction == nil || !plan.NextAction.AutoContinue {
			break
		}
	}

	return responseMessages, toolCalls, nil
}

func (m *agentManager) runAgentReplanActionContinuation(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, svc routerServices, plan *agentPlannerResponse, maxLoops int) ([]agentMessage, []agentToolCall) {
	if maxLoops <= 0 {
		return nil, nil
	}
	responseMessages := []agentMessage{}
	toolCalls := []agentToolCall{}
	loopHistory := cloneAgentMessages(history)

	for step := 0; step < maxLoops; step++ {
		if plan == nil || len(agentPlannerActions(plan)) == 0 {
			break
		}
		if plan.NextAction == nil || !plan.NextAction.AutoContinue {
			break
		}

		actionMessages, actionToolCalls, authorizationRequired := m.executeAgentPlannerActions(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc, plan, req.AuthProfiles, req.DefaultAuthProfileID)
		actionMessages = agentMessagesWithPhase(actionMessages, "step")
		actionToolCalls = agentToolCallsWithPhase(actionToolCalls, "step")
		if len(actionMessages) == 0 {
			break
		}
		m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, daemonTurnExecutionReplanActionContinuation, operationLevel, actionMessages...)
		responseMessages = append(responseMessages, actionMessages...)
		toolCalls = append(toolCalls, actionToolCalls...)
		loopHistory = append(loopHistory, actionMessages...)
		if authorizationRequired {
			break
		}

		synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.runRuntimeAgentLLMToolSynthesis(ctx, sessionID, turnRef, prompt, language, operationLevel, req, loopHistory, memorySummary, actionMessages)
		if synthesisErr != nil {
			warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
			warningTool = agentToolCallWithPhase(warningTool, "step_replan")
			warningMessage := agentMessageWithPhase(m.message(
				language,
				"tool",
				agentText(language, "工具已执行，但 StepReplan 总结失败："+synthesisErr.Error(), "Tools executed, but StepReplan synthesis failed: "+synthesisErr.Error()),
				[]agentToolCall{warningTool},
				nil,
			), "step_replan")
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, daemonTurnExecutionReplanActionContinuation, operationLevel, warningMessage)
			responseMessages = append(responseMessages, warningMessage)
			toolCalls = append(toolCalls, warningTool)
			break
		}
		if len(synthesisMessages) > 0 {
			m.appendMessagesWithPlanner(sessionID, synthesisPlan, synthesisMessages...)
			responseMessages = append(responseMessages, synthesisMessages...)
			toolCalls = append(toolCalls, synthesisToolCalls...)
			loopHistory = append(loopHistory, synthesisMessages...)
		}

		plan = synthesisPlan
	}
	return responseMessages, toolCalls
}

func reqMaxAgentReplans(req sendAgentMessageRequest) int {
	// Keep the first implementation conservative: one Plan plus at most three
	// Step/Replan cycles is enough for health/workspace/run/scan workflows and
	// prevents a weak provider from looping.
	_ = req
	return 3
}

func (m *agentManager) executeAgentPlannerActions(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, operatorPrompt, language, operator, operationLevel string, svc routerServices, plan *agentPlannerResponse, authProfiles []agentHTTPAuthProfile, defaultAuthProfileID string) ([]agentMessage, []agentToolCall, bool) {
	actions := agentPlannerActions(plan)
	if len(actions) == 0 {
		return nil, nil, false
	}
	if len(actions) > 6 {
		actions = actions[:6]
	}

	var messages []agentMessage
	var toolCalls []agentToolCall
	queueID := strings.Replace(runs.NewJobID(time.Now().UTC()), "job_", "queue_", 1)
	actionCount := len(actions)
	for idx, action := range actions {
		actionIndex := idx + 1
		actionName := normalizeAgentPlannerActionName(action.Name)
		if actionName == "" {
			actionName = strings.TrimSpace(action.Name)
		}
		bypassPermissions := agentOperationBypassPermissions(operationLevel)
		var actionMessages []agentMessage
		var actionToolCalls []agentToolCall
		var err error
		requiredLevel := agentOperationLevelRequiredForPlannerAction(actionName, action.Arguments)
		if mismatchReason, suggestedPrompt, mismatch := agentPlannerActionCapabilityMismatch(operatorPrompt, actionName, language); mismatch {
			policyErr := services.NewError(services.CodePolicyDenied, "non-equivalent tool action blocked", map[string]any{
				"intent":              agentClassifyCapabilityIntent(operatorPrompt).ID,
				"action":              actionName,
				"suggested_prompt":    suggestedPrompt,
				"capability_mismatch": true,
			}, nil)
			tool := failedAgentToolCall("agent.capability_match", policyErr)
			tool.Result = map[string]any{
				"intent":           agentClassifyCapabilityIntent(operatorPrompt).ID,
				"action":           actionName,
				"suggested_prompt": suggestedPrompt,
			}
			m.markAgentCapabilityBlocked(sessionID, language, mismatchReason, suggestedPrompt)
			message := agentMessageWithPhase(m.message(language, "agent", mismatchReason, []agentToolCall{tool}, nil), "step_replan")
			messages = append(messages, message)
			toolCalls = append(toolCalls, agentToolCallWithPhase(tool, "step_replan"))
			return messages, toolCalls, true
		} else if !bypassPermissions && !agentPlannerActionAllowedByPrompt(operatorPrompt, actionName) {
			authMessage := m.requestAgentAuthorization(
				sessionID,
				language,
				operatorPrompt,
				action,
				actionName,
				requiredLevel,
				"explicit_operator_intent_required",
				agentText(language, "模型计划了一个需要操作者明确确认的动作；已暂停执行，等待授权。", "The model planned an action that requires explicit operator confirmation; execution is paused until authorization."),
			)
			authMessage = agentMessageWithPhase(authMessage, "step")
			messages = append(messages, authMessage)
			return messages, toolCalls, true
		} else if !bypassPermissions && !agentOperationLevelAtLeast(operationLevel, requiredLevel) {
			authMessage := m.requestAgentAuthorization(
				sessionID,
				language,
				operatorPrompt,
				action,
				actionName,
				requiredLevel,
				"operation_level_required",
				agentText(
					language,
					fmt.Sprintf("当前权限级别为 %s，执行 %s 需要 %s；已暂停执行，等待授权升级。", operationLevel, actionName, requiredLevel),
					fmt.Sprintf("Current operation level is %s; executing %s requires %s. Execution is paused until authorization is granted.", operationLevel, actionName, requiredLevel),
				),
			)
			authMessage = agentMessageWithPhase(authMessage, "step")
			messages = append(messages, authMessage)
			return messages, toolCalls, true
		} else if !bypassPermissions && agentPlannerActionRequiresManualAuthorization(actionName) {
			authMessage := m.requestAgentAuthorization(
				sessionID,
				language,
				operatorPrompt,
				action,
				actionName,
				requiredLevel,
				"manual_tool_authorization_required",
				agentText(
					language,
					fmt.Sprintf("%s 属于需要人工确认的工具动作；已暂停执行，等待授权。", actionName),
					fmt.Sprintf("%s is a tool action that requires human confirmation; execution is paused until authorization.", actionName),
				),
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
		actionMetadata := map[string]interface{}{
			"action_queue_id": queueID,
			"action_index":    actionIndex,
			"action_count":    actionCount,
			"action_name":     actionName,
		}
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

func (m *agentManager) requestAgentAuthorization(sessionID, language, operatorPrompt string, action agentPlannerAction, actionName, requiredLevel, policy, reason string) agentMessage {
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
	}

	return agentMessageWithPhase(m.newMessageLocked(language, "agent", agentAuthorizationPromptMessage(language, auth), nil, nil), "step")
}

func (m *agentManager) markAgentCapabilityBlocked(sessionID, language, reason, suggestedPrompt string) {
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
		return
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
