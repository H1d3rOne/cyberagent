package daemon

import (
	"context"
	"fmt"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func (m *agentManager) ControlSession(ctx context.Context, sessionID string, req agentControlRequest, svc routerServices) (agentControlResponse, error) {
	if err := contextError(ctx); err != nil {
		return agentControlResponse{}, err
	}
	sessionID, err := normalizeAgentSessionID(sessionID)
	if err != nil {
		return agentControlResponse{}, err
	}
	action := normalizeAgentControlAction(req.Action)
	if action == "" {
		return agentControlResponse{}, services.NewError(services.CodeBadRequest, "unsupported agent control action", map[string]any{"action": req.Action}, nil)
	}
	language := normalizeAgentLanguage(req.Language)
	operator := normalizeAgentOperator(req.Operator)
	operationLevel := normalizeAgentOperationLevel(req.OperationLevel)

	controlTurnRef := m.controlSessionMutationTurnRef(ctx, sessionID, action, req)
	resolved, handled, controlErr := m.runRuntimeControlSessionMutationForTurn(ctx, sessionID, controlTurnRef, action, req, operator, language, operationLevel)
	if controlErr != nil {
		return agentControlResponse{}, controlErr
	}
	if !handled {
		resolved, controlErr = m.controlSessionLegacyForTurn(ctx, controlTurnRef, sessionID, action, req.Prompt, req.Reason, req.RunID, req.JobID, req.ClearInput, operator, language, operationLevel)
		if controlErr != nil {
			return agentControlResponse{}, controlErr
		}
	}
	sessionState := resolved.Session
	message := resolved.Message
	cancelJobID := resolved.CancelJobID

	if (action == "stop" || action == "cancel") && cancelJobID != "" && svc.jobs != nil {
		if job, cancelErr := svc.jobs.CancelJob(ctx, cancelJobID); cancelErr == nil {
			tool := runningAgentToolCall("POST /api/v1/jobs/{job_id}/cancel", shortAgentID(cancelJobID, 22))
			tool = finishAgentToolCall(tool, nil)
			tool.Result = map[string]any{"job_id": job.JobID, "run_id": job.RunID, "status": string(job.Status)}
			cancelMessage := m.message(language, "tool", agentText(language, "daemon job 已收到取消请求。", "daemon job received the cancellation request."), []agentToolCall{tool}, nil)
			cancelTurnRef := m.externalSessionMutationTurnRef(ctx, sessionID, cancelMessage.ID)
			m.appendMessagesForTurn(ctx, sessionID, cancelTurnRef, req.Prompt, string(runtime.SessionMutationControlSession), operationLevel, cancelMessage)
		} else if services.ErrorCodeOf(cancelErr) != services.CodeConflict {
			tool := failedAgentToolCall("POST /api/v1/jobs/{job_id}/cancel", cancelErr)
			cancelMessage := m.message(language, "tool", cancelErr.Error(), []agentToolCall{tool}, nil)
			cancelTurnRef := m.externalSessionMutationTurnRef(ctx, sessionID, cancelMessage.ID)
			m.appendMessagesForTurn(ctx, sessionID, cancelTurnRef, req.Prompt, string(runtime.SessionMutationControlSession), operationLevel, cancelMessage)
		}
	}

	return agentControlResponse{Session: sessionState, Message: message}, nil
}

func (m *agentManager) DecideAgentAuthorization(ctx context.Context, sessionID string, req agentAuthorizationDecisionRequest, svc routerServices) (agentControlResponse, error) {
	if err := contextError(ctx); err != nil {
		return agentControlResponse{}, err
	}
	sessionID, err := normalizeAgentSessionID(sessionID)
	if err != nil {
		return agentControlResponse{}, err
	}
	decision := normalizeAgentAuthorizationDecision(req.Action)
	if decision == "" {
		return agentControlResponse{}, services.NewError(services.CodeBadRequest, "unsupported authorization decision", map[string]any{"action": req.Action}, nil)
	}
	language := normalizeAgentLanguage(req.Language)
	operator := normalizeAgentOperator(req.Operator)
	operationLevel := normalizeAgentOperationLevel(req.OperationLevel)

	resolveTurnRef := m.resolveAuthorizationMutationTurnRef(ctx, sessionID, decision, req)
	resolved, handled, resolveErr := m.runRuntimeResolveAuthorizationMutationForTurn(ctx, sessionID, resolveTurnRef, decision, req, operator, language, operationLevel)
	if resolveErr != nil {
		return agentControlResponse{}, resolveErr
	}
	if !handled {
		resolved, resolveErr = m.resolveAuthorizationLegacyForTurn(ctx, resolveTurnRef, sessionID, decision, req.AuthorizationID, req.Reason, operator, language, operationLevel, strings.TrimSpace(req.OperationLevel) != "")
		if resolveErr != nil {
			return agentControlResponse{}, resolveErr
		}
	}
	pending := resolved.Pending
	message := resolved.Message
	operationLevel = resolved.OperationLevel
	if decision == "deny" {
		return agentControlResponse{Session: resolved.Session, Message: message, ApprovalState: canonicalAgentAuthorizationApproval(&pending, "denied")}, nil
	}

	externalTurnRef := m.externalSessionMutationTurnRef(ctx, sessionID, pending.ID)
	action := agentPlannerAction{Name: pending.ActionName, Arguments: cloneMapStringAny(pending.Arguments)}
	actionMessages, actionToolCalls, execErr := m.runRuntimePlannerActionExecution(ctx, sessionID, externalTurnRef, pending.OriginalPrompt, language, operator, operationLevel, svc, action, req.AuthProfiles, req.DefaultAuthProfileID)
	if execErr != nil {
		tool := failedAgentToolCall(pending.ActionName, execErr)
		tool.Result = map[string]any{"action": pending.ActionName, "authorization_id": pending.ID}
		actionMessages = []agentMessage{m.message(language, "tool", execErr.Error(), []agentToolCall{tool}, nil)}
		actionToolCalls = []agentToolCall{tool}
	}
	actionMessages = agentMessagesWithPhase(actionMessages, "step")
	actionToolCalls = agentToolCallsWithPhase(actionToolCalls, "step")
	authorizationMetadata := map[string]interface{}{
		"authorization_id":     pending.ID,
		"authorization_status": "approved",
		"action_name":          pending.ActionName,
	}
	for i := range actionToolCalls {
		actionToolCalls[i] = withAgentToolMetadata(actionToolCalls[i], authorizationMetadata)
	}
	for i := range actionMessages {
		for j := range actionMessages[i].ToolCalls {
			actionMessages[i].ToolCalls[j] = withAgentToolMetadata(actionMessages[i].ToolCalls[j], authorizationMetadata)
		}
	}
	if len(actionMessages) > 0 {
		m.appendMessagesForTurn(ctx, sessionID, externalTurnRef, pending.OriginalPrompt, daemonTurnExecutionPlannerActionExecution, operationLevel, actionMessages...)
	}
	responseMessages := append([]agentMessage{message}, actionMessages...)
	responseToolCalls := append([]agentToolCall{}, actionToolCalls...)

	llmReq := sendAgentMessageRequest{
		Operator:             operator,
		Language:             language,
		OperationLevel:       operationLevel,
		Provider:             req.Provider,
		Model:                req.Model,
		APIKey:               req.APIKey,
		BaseURL:              req.BaseURL,
		Instructions:         req.Instructions,
		AuthProfiles:         req.AuthProfiles,
		DefaultAuthProfileID: req.DefaultAuthProfileID,
	}
	if execErr == nil && !req.SkipSynthesis && agentLLMConfigured(llmReq) && len(actionMessages) > 0 {
		sessionState, getErr := m.GetSession(ctx, sessionID)
		if getErr == nil {
			history := cloneAgentMessages(sessionState.Messages)
			memorySummary := cloneAgentMemorySummary(sessionState.MemorySummary)
			synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.runRuntimeAgentLLMToolSynthesis(ctx, sessionID, externalTurnRef, pending.OriginalPrompt, language, operationLevel, llmReq, history, memorySummary, actionMessages)
			if synthesisErr != nil {
				warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
				warningTool = agentToolCallWithPhase(warningTool, "step_replan")
				warningMessage := agentMessageWithPhase(m.message(
					language,
					"tool",
					agentText(language, "授权动作已执行，但 StepReplan 总结失败："+synthesisErr.Error(), "Authorized action executed, but StepReplan synthesis failed: "+synthesisErr.Error()),
					[]agentToolCall{warningTool},
					nil,
				), "step_replan")
				m.appendMessagesForTurn(ctx, sessionID, externalTurnRef, pending.OriginalPrompt, string(runtime.LLMStageToolSynthesis), operationLevel, warningMessage)
				responseMessages = append(responseMessages, warningMessage)
				responseToolCalls = append(responseToolCalls, warningTool)
			} else if len(synthesisMessages) > 0 {
				m.appendMessagesWithPlannerForTurn(ctx, sessionID, externalTurnRef, pending.OriginalPrompt, string(runtime.LLMStageToolSynthesis), operationLevel, synthesisPlan, synthesisMessages...)
				responseMessages = append(responseMessages, synthesisMessages...)
				responseToolCalls = append(responseToolCalls, synthesisToolCalls...)
				if synthesisPlan != nil && synthesisPlan.NextAction != nil && synthesisPlan.NextAction.AutoContinue {
					nextState, _ := m.GetSession(ctx, sessionID)
					loopHistory := cloneAgentMessages(nextState.Messages)
					followupMessages, followupToolCalls := m.runRuntimeReplanActionContinuation(ctx, sessionID, externalTurnRef, pending.OriginalPrompt, language, operator, operationLevel, llmReq, loopHistory, memorySummary, svc, synthesisPlan, reqMaxAgentReplans(llmReq)-1)
					responseMessages = append(responseMessages, followupMessages...)
					responseToolCalls = append(responseToolCalls, followupToolCalls...)
				}
			}
		}
	}

	sessionState, getErr := m.GetSession(ctx, sessionID)
	if getErr != nil {
		return agentControlResponse{}, getErr
	}
	_ = responseToolCalls
	return agentControlResponse{Session: sessionState, Message: message, ApprovalState: canonicalAgentAuthorizationApproval(&pending, "approved")}, nil
}

func (m *agentManager) SendMessage(ctx context.Context, sessionID string, req sendAgentMessageRequest, svc routerServices) (resp sendAgentMessageResponse, err error) {
	if err := contextError(ctx); err != nil {
		return sendAgentMessageResponse{}, err
	}
	sessionID, err = normalizeAgentSessionID(sessionID)
	if err != nil {
		return sendAgentMessageResponse{}, err
	}
	prompt := strings.TrimSpace(req.Message)
	if prompt == "" {
		return sendAgentMessageResponse{}, services.NewError(services.CodeBadRequest, "message is required", map[string]any{"field": "message"}, nil)
	}
	operator := normalizeAgentOperator(req.Operator)
	language := normalizeAgentLanguage(req.Language)
	operationLevel := normalizeAgentOperationLevel(req.OperationLevel)
	var turnRef agentRuntimeTurnRef
	defer func() {
		m.finishRuntimeTurn(ctx, sessionID, turnRef, resp, err)
	}()

	var history []agentMessage
	var memorySummary *agentMemorySummary
	userMessage := agentMessage{}
	sessionSnapshot := agentSession{}
	recorded, handled, recordErr := m.runRuntimeRecordUserMessageMutation(ctx, sessionID, prompt, operator, language, operationLevel)
	if recordErr != nil {
		return sendAgentMessageResponse{}, recordErr
	}
	if !handled {
		recorded, recordErr = m.recordUserMessageLegacy(sessionID, prompt, operator, language, operationLevel)
		if recordErr != nil {
			return sendAgentMessageResponse{}, recordErr
		}
	}
	userMessage = recorded.UserMessage
	sessionSnapshot = recorded.Session
	history = cloneAgentMessages(recorded.History)
	memorySummary = cloneAgentMemorySummary(recorded.MemorySummary)
	turnRef = m.beginRuntimeTurn(ctx, sessionID, prompt, firstNonEmpty(req.Origin, "user"), operationLevel, sessionSnapshot, userMessage)
	m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionSnapshot, turnRef, recorded.Compaction)

	if controlReq, ok := agentControlRequestFromPrompt(prompt, operator, language); ok {
		controlReq.OperationLevel = operationLevel
		controlResp, controlErr := m.ControlSession(ctx, sessionID, controlReq, svc)
		if controlErr != nil {
			errMessage := m.message(language, "agent", controlErr.Error(), []agentToolCall{failedAgentToolCall("agent.control", controlErr)}, nil)
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, string(runtime.SessionMutationControlSession), operationLevel, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
		}
		if controlReq.Action == "continue" && !agentLLMConfigured(req) {
			if contextMessages, contextToolCalls, ok := m.handleAgentActiveRunContinueForTurn(ctx, sessionID, turnRef, language, operator, operationLevel, svc); ok {
				m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, "active_run_continue", operationLevel, contextMessages...)
				sessionState, getErr := m.GetSession(ctx, sessionID)
				if getErr != nil {
					sessionState = controlResp.Session
				}
				responseMessages := append([]agentMessage{userMessage, controlResp.Message}, contextMessages...)
				return buildAgentSendResponse(sessionID, responseMessages, contextToolCalls, sessionState), nil
			}
			if autoPrompt := agentExecutableAutoContinuePrompt(controlResp.Session.NextAction); autoPrompt != "" && agentCanAutoRouteContinuePrompt(autoPrompt) {
				autoMessages, autoToolCalls, routeErr := m.routePromptWithRuntimeDeterministicRoute(ctx, sessionID, turnRef, autoPrompt, language, operator, operationLevel, svc)
				if routeErr != nil {
					errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.auto_continue", routeErr)}, nil)
					m.appendMessagesForTurn(ctx, sessionID, turnRef, autoPrompt, daemonTurnExecutionDeterministicRoute, operationLevel, errMessage)
					sessionState, getErr := m.GetSession(ctx, sessionID)
					if getErr != nil {
						sessionState = controlResp.Session
					}
					return buildAgentSendResponse(sessionID, []agentMessage{userMessage, controlResp.Message, errMessage}, errMessage.ToolCalls, sessionState), nil
				}
				autoMessages, autoToolCalls = tagAgentAutoContinueMessages(autoMessages, autoToolCalls, autoPrompt)
				m.appendMessagesForTurn(ctx, sessionID, turnRef, autoPrompt, daemonTurnExecutionDeterministicRoute, operationLevel, autoMessages...)
				sessionState, getErr := m.GetSession(ctx, sessionID)
				if getErr != nil {
					sessionState = controlResp.Session
				}
				responseMessages := append([]agentMessage{userMessage, controlResp.Message}, autoMessages...)
				return buildAgentSendResponse(sessionID, responseMessages, autoToolCalls, sessionState), nil
			}
		}
		if agentLLMConfigured(req) && (controlReq.Action == "continue" || controlReq.Action == "revise") {
			sessionState, getErr := m.GetSession(ctx, sessionID)
			if getErr == nil {
				history = cloneAgentMessages(sessionState.Messages)
				memorySummary = cloneAgentMemorySummary(sessionState.MemorySummary)
			}
			messages, toolCalls, err := m.runRuntimeLLMToolLoop(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc)
			if err == nil {
				responseMessages := append([]agentMessage{userMessage, controlResp.Message}, messages...)
				sessionState, _ = m.GetSession(ctx, sessionID)
				return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
			}
			warningTool := failedAgentToolCall("LLM chat", err)
			if len(toolCalls) > 0 {
				warningTool = toolCalls[0]
			}
			warningMessage := m.message(
				language,
				"tool",
				agentText(language, "AI provider 调用失败，已保留控制状态更新："+err.Error(), "AI provider call failed; the control-state update was preserved: "+err.Error()),
				[]agentToolCall{warningTool},
				nil,
			)
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, "llm_tool_loop", operationLevel, warningMessage)
			sessionState, _ = m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, controlResp.Message, warningMessage}, []agentToolCall{warningTool}, sessionState), nil
		}
		sessionState, getErr := m.GetSession(ctx, sessionID)
		if getErr != nil {
			sessionState = controlResp.Session
		}
		return buildAgentSendResponse(sessionID, []agentMessage{userMessage, controlResp.Message}, nil, sessionState), nil
	}

	if sessionState, getErr := m.GetSession(ctx, sessionID); getErr == nil &&
		sessionState.PendingAuthorization != nil &&
		strings.EqualFold(sessionState.PendingAuthorization.Status, "pending") {
		blockMessage := agentMessageWithPhase(m.message(language, "agent", agentText(language, "当前任务已暂停在工具授权门。请先在授权卡片中点击“允许并继续”或“拒绝”；我不会在授权前执行新的步骤。", "The current task is paused at a tool authorization gate. Approve or deny the authorization card first; I will not execute new steps before authorization."), nil, nil), "step")
		m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, "authorization_gate", operationLevel, blockMessage)
		latestState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, []agentMessage{userMessage, blockMessage}, nil, latestState), nil
	}

	if agentLLMConfigured(req) && agentShouldRouteBeforeLLMSynthesis(prompt) {
		messages, toolCalls, routeErr := m.routePromptWithRuntimeDeterministicRoute(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc)
		if routeErr != nil {
			errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.route", routeErr)}, nil)
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, daemonTurnExecutionDeterministicRoute, operationLevel, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
		}
		messages = agentMessagesWithPhase(messages, "step")
		toolCalls = agentToolCallsWithPhase(toolCalls, "step")
		m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, daemonTurnExecutionDeterministicRoute, operationLevel, messages...)
		responseMessages := append([]agentMessage{userMessage}, messages...)
		if len(toolCalls) > 0 {
			synthesisHistory := append(cloneAgentMessages(history), messages...)
			synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.runRuntimeAgentLLMToolSynthesis(ctx, sessionID, turnRef, prompt, language, operationLevel, req, synthesisHistory, memorySummary, messages)
			if synthesisErr != nil {
				warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
				warningTool = withAgentToolMetadata(warningTool, map[string]interface{}{"phase": "tool_synthesis"})
				warningTool = agentToolCallWithPhase(warningTool, "step_replan")
				warningMessage := agentMessageWithPhase(m.message(
					language,
					"tool",
					agentText(language, "工具已执行，但 AI 结果总结失败："+synthesisErr.Error(), "Tools executed, but AI synthesis failed: "+synthesisErr.Error()),
					[]agentToolCall{warningTool},
					nil,
				), "step_replan")
				m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, string(runtime.LLMStageToolSynthesis), operationLevel, warningMessage)
				responseMessages = append(responseMessages, warningMessage)
				toolCalls = append(toolCalls, warningTool)
			} else if len(synthesisMessages) > 0 {
				m.appendMessagesWithPlannerForTurn(ctx, sessionID, turnRef, prompt, string(runtime.LLMStageToolSynthesis), operationLevel, synthesisPlan, synthesisMessages...)
				responseMessages = append(responseMessages, synthesisMessages...)
				toolCalls = append(toolCalls, synthesisToolCalls...)
				if synthesisPlan != nil && synthesisPlan.NextAction != nil && synthesisPlan.NextAction.AutoContinue {
					loopHistory := append(synthesisHistory, synthesisMessages...)
					followupMessages, followupToolCalls := m.runRuntimeReplanActionContinuation(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, loopHistory, memorySummary, svc, synthesisPlan, reqMaxAgentReplans(req)-1)
					responseMessages = append(responseMessages, followupMessages...)
					toolCalls = append(toolCalls, followupToolCalls...)
				}
			}
		}
		sessionState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
	}

	if agentLLMConfigured(req) {
		messages, toolCalls, err := m.runRuntimeLLMToolLoop(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc)
		if err == nil {
			responseMessages := append([]agentMessage{userMessage}, messages...)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
		}
		warningTool := failedAgentToolCall("LLM chat", err)
		if len(toolCalls) > 0 {
			warningTool = toolCalls[0]
		}
		warningMessage := m.message(
			language,
			"tool",
			agentText(language, "AI provider 调用失败，已回退到本地规则 Agent："+err.Error(), "AI provider call failed; falling back to the local rule Agent: "+err.Error()),
			[]agentToolCall{warningTool},
			nil,
		)
		if action, ok := agentManualAuthorizationActionFromPrompt(prompt); ok {
			actionName := normalizeAgentPlannerActionName(action.Name)
			requiredLevel := agentOperationLevelRequiredForPlannerAction(actionName, action.Arguments)
			authMessage := m.requestAgentAuthorization(
				sessionID,
				language,
				prompt,
				action,
				actionName,
				requiredLevel,
				"manual_tool_authorization_required",
				agentText(
					language,
					fmt.Sprintf("AI provider 失败后检测到 %s 需要人工确认；已暂停执行，等待授权。", actionName),
					fmt.Sprintf("After the AI provider failed, %s was detected as requiring human confirmation; execution is paused until authorization.", actionName),
				),
			)
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, "llm_fallback_authorization", operationLevel, warningMessage, authMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, warningMessage, authMessage}, []agentToolCall{warningTool}, sessionState), nil
		}
		messages, toolCalls, routeErr := m.routePromptWithRuntimeDeterministicRoute(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc)
		if routeErr != nil {
			errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.route", routeErr)}, nil)
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, "llm_fallback_route", operationLevel, warningMessage, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, warningMessage, errMessage}, append([]agentToolCall{warningTool}, errMessage.ToolCalls...), sessionState), nil
		}
		messages = append([]agentMessage{warningMessage}, messages...)
		toolCalls = append([]agentToolCall{warningTool}, toolCalls...)
		m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, "llm_fallback_route", operationLevel, messages...)
		responseMessages := append([]agentMessage{userMessage}, messages...)
		sessionState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
	}

	messages, toolCalls, routeErr := m.routePromptWithRuntimeDeterministicRoute(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc)
	err = routeErr
	if err != nil {
		errMessage := m.message(language, "agent", err.Error(), []agentToolCall{failedAgentToolCall("agent.route", err)}, nil)
		m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, daemonTurnExecutionDeterministicRoute, operationLevel, errMessage)
		sessionState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
	}

	m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, daemonTurnExecutionDeterministicRoute, operationLevel, messages...)
	responseMessages := append([]agentMessage{userMessage}, messages...)
	if agentLLMConfigured(req) && len(toolCalls) > 0 {
		synthesisHistory := append(cloneAgentMessages(history), messages...)
		synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.runRuntimeAgentLLMToolSynthesis(ctx, sessionID, turnRef, prompt, language, operationLevel, req, synthesisHistory, memorySummary, messages)
		if synthesisErr != nil {
			warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
			warningTool = withAgentToolMetadata(warningTool, map[string]interface{}{"phase": "tool_synthesis"})
			warningMessage := m.message(
				language,
				"tool",
				agentText(language, "工具已执行，但 AI 结果总结失败："+synthesisErr.Error(), "Tools executed, but AI synthesis failed: "+synthesisErr.Error()),
				[]agentToolCall{warningTool},
				nil,
			)
			m.appendMessagesForTurn(ctx, sessionID, turnRef, prompt, string(runtime.LLMStageToolSynthesis), operationLevel, warningMessage)
			responseMessages = append(responseMessages, warningMessage)
			toolCalls = append(toolCalls, warningTool)
		} else if len(synthesisMessages) > 0 {
			m.appendMessagesWithPlannerForTurn(ctx, sessionID, turnRef, prompt, string(runtime.LLMStageToolSynthesis), operationLevel, synthesisPlan, synthesisMessages...)
			responseMessages = append(responseMessages, synthesisMessages...)
			toolCalls = append(toolCalls, synthesisToolCalls...)
		}
	}
	sessionState, _ := m.GetSession(ctx, sessionID)
	return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
}
