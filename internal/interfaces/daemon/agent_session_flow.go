package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
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
	now := time.Now().UTC()

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return agentControlResponse{}, loadErr
		}
		session = loaded
	}
	session.Operator = operator
	session.Language = language
	session.OperationLevel = operationLevel
	previousNextAction := cloneAgentNextAction(session.NextAction)

	if session.PendingAuthorization != nil &&
		strings.EqualFold(session.PendingAuthorization.Status, "pending") &&
		(action == "continue" || action == "revise") {
		session.Status = "waiting_authorization"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "authorization_required",
			Reason:          session.PendingAuthorization.Reason,
			SuggestedPrompt: agentAuthorizationSuggestedPrompt(language, session.PendingAuthorization),
			AutoContinue:    false,
		}
		message := agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, "当前有挂起的工具授权，请先点击允许或拒绝；在此之前不会继续执行任务。", "A tool authorization is pending. Approve or deny it first; the task will not continue before that."), nil, nil), "step")
		session.Messages = append(session.Messages, message)
		session.UpdatedAt = now
		compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
		if err := m.persistSessionLocked(session); err != nil {
			m.mu.Unlock()
			return agentControlResponse{}, err
		}
		sessionState := cloneAgentSession(session)
		m.mu.Unlock()
		m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
		return agentControlResponse{Session: sessionState, Message: message}, nil
	}

	var message agentMessage
	var cancelJobID string
	switch action {
	case "continue":
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		m.ensureRunningTodoLocked(session, now)
		previousAutoContinue := previousNextAction != nil &&
			previousNextAction.Type == "continue" &&
			previousNextAction.AutoContinue &&
			strings.TrimSpace(req.Prompt) == "" &&
			strings.TrimSpace(previousNextAction.SuggestedPrompt) != ""
		previousSuggestedPrompt := ""
		if previousAutoContinue {
			previousSuggestedPrompt = previousNextAction.SuggestedPrompt
		}
		suggestedPrompt := firstNonEmpty(strings.TrimSpace(req.Prompt), strings.TrimSpace(previousSuggestedPrompt), agentSuggestedContinuePrompt(language, session))
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "用户请求继续当前任务。", "The operator requested continuing the current task."),
			SuggestedPrompt: suggestedPrompt,
			AutoContinue:    previousAutoContinue,
		}
		if agentEvidenceActionHasApprovalGate(suggestedPrompt) {
			session.NextAction = &agentNextAction{
				Type:            "approval_required",
				Reason:          agentText(language, "当前继续动作仍包含待审批 gate，需先处理审批后再继续。", "The continue action still contains a pending approval gate; resolve approvals before continuing."),
				SuggestedPrompt: suggestedPrompt,
			}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "收到：继续当前任务。我会基于当前计划、工具调用和工作区上下文推进下一步。", "Received: continue the current task. I will proceed based on the current plan, tool calls and workspace context."), nil, nil)
	case "revise":
		revision := strings.TrimSpace(req.Prompt)
		if revision == "" {
			revision = strings.TrimSpace(req.Reason)
		}
		if revision == "" {
			revision = agentText(language, "修订当前任务计划。", "Revise the current task plan.")
		}
		session.Status = "running"
		if session.ActiveTask == nil {
			session.ActiveTask = &agentTask{
				ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
				Goal:      agentText(language, "修订安全分析任务", "Revise security analysis task"),
				Kind:      "planning",
				Status:    "running",
				Summary:   revision,
				CreatedAt: now,
				UpdatedAt: now,
			}
		} else {
			session.ActiveTask.Status = "running"
			session.ActiveTask.Summary = trimSummary(revision, 360)
			session.ActiveTask.UpdatedAt = now
		}
		isEvidenceAction := agentIsEvidenceActionPrompt(revision)
		evidenceActionRequiresApproval := false
		if isEvidenceAction {
			evidenceActionRequiresApproval = agentEvidenceActionHasApprovalGate(revision)
			session.Todos = agentEvidenceActionTodos(language, revision, now)
		} else {
			m.ensureRunningTodoLocked(session, now)
			if len(session.Todos) == 0 {
				session.Todos = agentInitialTodos(language, "chat", now)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "用户要求修订当前任务计划。", "The operator requested revising the current task plan."),
			SuggestedPrompt: revision,
		}
		if evidenceActionRequiresApproval {
			session.NextAction = &agentNextAction{
				Type:            "approval_required",
				Reason:          agentText(language, "当前证据行动计划存在待审批 gate，需先处理审批后再继续。", "The evidence action plan has a pending approval gate; resolve approvals before continuing."),
				SuggestedPrompt: revision,
			}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "收到：我会按你的修订要求更新当前任务计划，并在下一轮继续推进。", "Received: I will update the current task plan according to your revision and continue in the next turn."), nil, nil)
	case "pause":
		session.Status = "paused"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = agentText(language, "用户请求暂停当前 Agent 会话。", "The operator requested pausing the current Agent session.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(req.Prompt), agentSuggestedContinuePrompt(language, session)),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已暂停当前 Agent 会话；不会清空上下文。发送继续/Resume 可从当前计划恢复。", "Paused the current Agent session without clearing context. Send continue/resume to proceed from the current plan."), nil, nil)
	case "cancel":
		session.PendingAuthorization = nil
		session.Status = "cancelled"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		cancelJobID = strings.TrimSpace(req.JobID)
		if cancelJobID == "" {
			cancelJobID = m.findActiveJobIDLocked(session, strings.TrimSpace(req.RunID))
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = agentText(language, "用户请求取消当前 Agent 任务。", "The operator requested cancelling the current Agent task.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "skipped", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(req.Prompt), agentText(language, "开启新任务并说明授权目标与 scope", "start a new task and provide the authorized target and scope")),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已取消当前 Agent 任务；如存在运行中的 daemon job，会尝试取消。", "Cancelled the current Agent task; if a daemon job is running, I will try to cancel it."), nil, nil)
	case "stop":
		session.PendingAuthorization = nil
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		cancelJobID = strings.TrimSpace(req.JobID)
		if cancelJobID == "" {
			cancelJobID = m.findActiveJobIDLocked(session, strings.TrimSpace(req.RunID))
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = agentText(language, "用户请求停止当前任务。", "The operator requested stopping the current task.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(req.Prompt), agentSuggestedStopPrompt(language, session)),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已暂停当前 Agent 任务状态；如存在运行中的 daemon job，会尝试取消。", "The current Agent task state is paused; if a daemon job is running, I will try to cancel it."), nil, nil)
	case "new_task":
		session.Status = "idle"
		session.ActiveTask = nil
		session.Todos = []agentTodo{}
		session.ToolCalls = []agentToolCall{}
		session.ContextRefs = []agentContextRef{}
		session.PendingChanges = []agentWorkspaceChange{}
		session.PendingAuthorization = nil
		session.MemorySummary = nil
		session.NextAction = &agentNextAction{
			Type:   "wait_user",
			Reason: agentText(language, "已开启新任务，等待用户输入目标、scope 或下一步指令。", "New task started; waiting for target, scope, or next instruction."),
		}
		if req.ClearInput {
			session.Messages = []agentMessage{}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已开启新的 Agent 任务上下文。", "Started a new Agent task context."), nil, nil)
	default:
		m.mu.Unlock()
		return agentControlResponse{}, services.NewError(services.CodeBadRequest, "unsupported agent control action", map[string]any{"action": req.Action}, nil)
	}
	session.Messages = append(session.Messages, message)
	session.UpdatedAt = now
	compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return agentControlResponse{}, err
	}
	sessionState := cloneAgentSession(session)
	m.mu.Unlock()
	m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
	m.emitAgentRuntimeEvents(ctx, m.runtimeEngine(), sessionID, sessionState, message)

	if (action == "stop" || action == "cancel") && cancelJobID != "" && svc.jobs != nil {
		if job, cancelErr := svc.jobs.CancelJob(ctx, cancelJobID); cancelErr == nil {
			tool := runningAgentToolCall("POST /api/v1/jobs/{job_id}/cancel", shortAgentID(cancelJobID, 22))
			tool = finishAgentToolCall(tool, nil)
			tool.Result = map[string]any{"job_id": job.JobID, "run_id": job.RunID, "status": string(job.Status)}
			m.appendMessages(sessionID, m.message(language, "tool", agentText(language, "daemon job 已收到取消请求。", "daemon job received the cancellation request."), []agentToolCall{tool}, nil))
		} else if services.ErrorCodeOf(cancelErr) != services.CodeConflict {
			tool := failedAgentToolCall("POST /api/v1/jobs/{job_id}/cancel", cancelErr)
			m.appendMessages(sessionID, m.message(language, "tool", cancelErr.Error(), []agentToolCall{tool}, nil))
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
	now := time.Now().UTC()

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return agentControlResponse{}, loadErr
		}
		session = loaded
	}
	pending := cloneAgentAuthorizationRequest(session.PendingAuthorization)
	if pending == nil || !strings.EqualFold(pending.Status, "pending") {
		m.mu.Unlock()
		return agentControlResponse{}, services.NewError(services.CodeConflict, "no pending agent authorization", map[string]any{"session_id": sessionID}, nil)
	}
	if requestedID := strings.TrimSpace(req.AuthorizationID); requestedID != "" && requestedID != pending.ID {
		m.mu.Unlock()
		return agentControlResponse{}, services.NewError(services.CodeConflict, "authorization id does not match pending request", map[string]any{"authorization_id": requestedID, "pending_authorization_id": pending.ID}, nil)
	}
	session.Operator = operator
	session.Language = language
	if strings.TrimSpace(req.OperationLevel) == "" {
		operationLevel = normalizeAgentOperationLevel(session.OperationLevel)
	}
	if decision == "approve" && !agentOperationLevelAtLeast(operationLevel, pending.RequiredLevel) {
		operationLevel = normalizeAgentOperationLevel(pending.RequiredLevel)
	}
	session.OperationLevel = operationLevel

	var message agentMessage
	switch decision {
	case "deny":
		pending.Status = "denied"
		pending.UpdatedAt = now
		session.PendingAuthorization = nil
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", agentText(language, "操作者拒绝授权该工具动作。", "Operator denied authorization for this tool action."))
			}
		}
		reason := firstNonEmpty(strings.TrimSpace(req.Reason), agentText(language, "操作者拒绝授权，挂起动作未执行。", "Operator denied authorization; the pending action was not executed."))
		session.NextAction = &agentNextAction{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "换一个低风险方案或重新说明授权边界。", "Choose a lower-risk alternative or restate the authorization boundary."),
		}
		message = agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, fmt.Sprintf("已拒绝授权：`%s` 未执行。", pending.ActionName), fmt.Sprintf("Authorization denied: `%s` was not executed.", pending.ActionName)), nil, nil), "step")
		session.Messages = append(session.Messages, message)
		session.UpdatedAt = now
		compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
		if err := m.persistSessionLocked(session); err != nil {
			m.mu.Unlock()
			return agentControlResponse{}, err
		}
		sessionState := cloneAgentSession(session)
		m.mu.Unlock()
		m.emitRuntimeApprovalResolved(ctx, sessionID, sessionState, *pending, "denied")
		m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
		m.emitAgentRuntimeEvents(ctx, m.runtimeEngine(), sessionID, sessionState, message)
		return agentControlResponse{Session: sessionState, Message: message, ApprovalState: canonicalAgentAuthorizationApproval(pending, "denied")}, nil
	case "approve":
		pending.Status = "approved"
		pending.UpdatedAt = now
		session.PendingAuthorization = nil
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "授权已通过，继续执行挂起工具动作。", "Authorization approved; continuing the pending tool action."),
			SuggestedPrompt: pending.OriginalPrompt,
			AutoContinue:    true,
		}
		message = agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, fmt.Sprintf("已授权，继续执行挂起动作：`%s`。", pending.ActionName), fmt.Sprintf("Authorized. Continuing pending action: `%s`.", pending.ActionName)), nil, nil), "step")
		session.Messages = append(session.Messages, message)
		session.UpdatedAt = now
		compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
		if err := m.persistSessionLocked(session); err != nil {
			m.mu.Unlock()
			return agentControlResponse{}, err
		}
		approvedSessionState := cloneAgentSession(session)
		m.mu.Unlock()
		m.emitRuntimeApprovalResolved(ctx, sessionID, approvedSessionState, *pending, "approved")
		m.emitRuntimeMemoryCompacted(ctx, sessionID, approvedSessionState, agentRuntimeTurnRef{}, compaction)
		m.emitAgentRuntimeEvents(ctx, m.runtimeEngine(), sessionID, approvedSessionState, message)
	default:
		m.mu.Unlock()
		return agentControlResponse{}, services.NewError(services.CodeBadRequest, "unsupported authorization decision", map[string]any{"action": req.Action}, nil)
	}

	action := agentPlannerAction{Name: pending.ActionName, Arguments: cloneMapStringAny(pending.Arguments)}
	actionMessages, actionToolCalls, execErr := m.executeAgentPlannerAction(ctx, sessionID, language, operator, operationLevel, svc, action, req.AuthProfiles, req.DefaultAuthProfileID)
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
		m.appendMessages(sessionID, actionMessages...)
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
			synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, sessionID, agentRuntimeTurnRef{}, language, llmReq, history, memorySummary, actionMessages)
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
				m.appendMessages(sessionID, warningMessage)
				responseMessages = append(responseMessages, warningMessage)
				responseToolCalls = append(responseToolCalls, warningTool)
			} else if len(synthesisMessages) > 0 {
				m.appendMessagesWithPlanner(sessionID, synthesisPlan, synthesisMessages...)
				responseMessages = append(responseMessages, synthesisMessages...)
				responseToolCalls = append(responseToolCalls, synthesisToolCalls...)
				if synthesisPlan != nil && synthesisPlan.NextAction != nil && synthesisPlan.NextAction.AutoContinue {
					nextState, _ := m.GetSession(ctx, sessionID)
					loopHistory := cloneAgentMessages(nextState.Messages)
					followupMessages, followupToolCalls := m.runAgentReplanActionContinuation(ctx, sessionID, agentRuntimeTurnRef{}, pending.OriginalPrompt, language, operator, operationLevel, llmReq, loopHistory, memorySummary, svc, synthesisPlan, reqMaxAgentReplans(llmReq)-1)
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
	return agentControlResponse{Session: sessionState, Message: message, ApprovalState: canonicalAgentAuthorizationApproval(pending, "approved")}, nil
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
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return sendAgentMessageResponse{}, loadErr
		}
		session = loaded
	}
	session.Operator = operator
	session.Language = language
	session.OperationLevel = operationLevel
	userMessage = m.newMessageLocked(language, "user", prompt, nil, nil)
	session.Messages = append(session.Messages, userMessage)
	session.UpdatedAt = userMessage.Timestamp
	if m.shouldStartNewAgentTaskLocked(session, prompt, language) {
		m.startAgentTaskLocked(session, prompt, language)
	} else {
		m.touchAgentTaskForMessageLocked(session, prompt, language)
	}
	compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	history = cloneAgentMessages(session.Messages)
	memorySummary = cloneAgentMemorySummary(session.MemorySummary)
	_ = m.persistSessionLocked(session)
	sessionSnapshot = cloneAgentSession(session)
	m.mu.Unlock()
	turnRef = m.beginRuntimeTurn(ctx, sessionID, prompt, firstNonEmpty(req.Origin, "user"), operationLevel, sessionSnapshot, userMessage)
	m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionSnapshot, turnRef, compaction)

	if controlReq, ok := agentControlRequestFromPrompt(prompt, operator, language); ok {
		controlReq.OperationLevel = operationLevel
		controlResp, controlErr := m.ControlSession(ctx, sessionID, controlReq, svc)
		if controlErr != nil {
			errMessage := m.message(language, "agent", controlErr.Error(), []agentToolCall{failedAgentToolCall("agent.control", controlErr)}, nil)
			m.appendMessages(sessionID, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
		}
		if controlReq.Action == "continue" && !agentLLMConfigured(req) {
			if contextMessages, contextToolCalls, ok := m.handleAgentActiveRunContinue(ctx, sessionID, language, svc); ok {
				m.appendMessages(sessionID, contextMessages...)
				sessionState, getErr := m.GetSession(ctx, sessionID)
				if getErr != nil {
					sessionState = controlResp.Session
				}
				responseMessages := append([]agentMessage{userMessage, controlResp.Message}, contextMessages...)
				return buildAgentSendResponse(sessionID, responseMessages, contextToolCalls, sessionState), nil
			}
			if autoPrompt := agentExecutableAutoContinuePrompt(controlResp.Session.NextAction); autoPrompt != "" && agentCanAutoRouteContinuePrompt(autoPrompt) {
				autoMessages, autoToolCalls, routeErr := m.routePrompt(ctx, sessionID, autoPrompt, language, operator, operationLevel, svc)
				if routeErr != nil {
					errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.auto_continue", routeErr)}, nil)
					m.appendMessages(sessionID, errMessage)
					sessionState, getErr := m.GetSession(ctx, sessionID)
					if getErr != nil {
						sessionState = controlResp.Session
					}
					return buildAgentSendResponse(sessionID, []agentMessage{userMessage, controlResp.Message, errMessage}, errMessage.ToolCalls, sessionState), nil
				}
				autoMessages, autoToolCalls = tagAgentAutoContinueMessages(autoMessages, autoToolCalls, autoPrompt)
				m.appendMessages(sessionID, autoMessages...)
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
			m.appendMessages(sessionID, warningMessage)
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
		m.appendMessages(sessionID, blockMessage)
		latestState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, []agentMessage{userMessage, blockMessage}, nil, latestState), nil
	}

	if agentLLMConfigured(req) && agentShouldRouteBeforeLLMSynthesis(prompt) {
		messages, toolCalls, routeErr := m.routePrompt(ctx, sessionID, prompt, language, operator, operationLevel, svc)
		if routeErr != nil {
			errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.route", routeErr)}, nil)
			m.appendMessages(sessionID, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
		}
		messages = agentMessagesWithPhase(messages, "step")
		toolCalls = agentToolCallsWithPhase(toolCalls, "step")
		m.appendMessages(sessionID, messages...)
		responseMessages := append([]agentMessage{userMessage}, messages...)
		if len(toolCalls) > 0 {
			synthesisHistory := append(cloneAgentMessages(history), messages...)
			synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, synthesisHistory, memorySummary, messages)
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
				m.appendMessages(sessionID, warningMessage)
				responseMessages = append(responseMessages, warningMessage)
				toolCalls = append(toolCalls, warningTool)
			} else if len(synthesisMessages) > 0 {
				m.appendMessagesWithPlanner(sessionID, synthesisPlan, synthesisMessages...)
				responseMessages = append(responseMessages, synthesisMessages...)
				toolCalls = append(toolCalls, synthesisToolCalls...)
				if synthesisPlan != nil && synthesisPlan.NextAction != nil && synthesisPlan.NextAction.AutoContinue {
					loopHistory := append(synthesisHistory, synthesisMessages...)
					followupMessages, followupToolCalls := m.runAgentReplanActionContinuation(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, loopHistory, memorySummary, svc, synthesisPlan, reqMaxAgentReplans(req)-1)
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
			m.appendMessages(sessionID, warningMessage, authMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, warningMessage, authMessage}, []agentToolCall{warningTool}, sessionState), nil
		}
		messages, toolCalls, routeErr := m.routePrompt(ctx, sessionID, prompt, language, operator, operationLevel, svc)
		if routeErr != nil {
			errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.route", routeErr)}, nil)
			m.appendMessages(sessionID, warningMessage, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, warningMessage, errMessage}, append([]agentToolCall{warningTool}, errMessage.ToolCalls...), sessionState), nil
		}
		messages = append([]agentMessage{warningMessage}, messages...)
		toolCalls = append([]agentToolCall{warningTool}, toolCalls...)
		m.appendMessages(sessionID, messages...)
		responseMessages := append([]agentMessage{userMessage}, messages...)
		sessionState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
	}

	messages, toolCalls, err := m.routePrompt(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	if err != nil {
		errMessage := m.message(language, "agent", err.Error(), []agentToolCall{failedAgentToolCall("agent.route", err)}, nil)
		m.appendMessages(sessionID, errMessage)
		sessionState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
	}

	m.appendMessages(sessionID, messages...)
	responseMessages := append([]agentMessage{userMessage}, messages...)
	if agentLLMConfigured(req) && len(toolCalls) > 0 {
		synthesisHistory := append(cloneAgentMessages(history), messages...)
		synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, synthesisHistory, memorySummary, messages)
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
			m.appendMessages(sessionID, warningMessage)
			responseMessages = append(responseMessages, warningMessage)
			toolCalls = append(toolCalls, warningTool)
		} else if len(synthesisMessages) > 0 {
			m.appendMessagesWithPlanner(sessionID, synthesisPlan, synthesisMessages...)
			responseMessages = append(responseMessages, synthesisMessages...)
			toolCalls = append(toolCalls, synthesisToolCalls...)
		}
	}
	sessionState, _ := m.GetSession(ctx, sessionID)
	return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
}
