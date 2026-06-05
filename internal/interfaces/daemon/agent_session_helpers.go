package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

type agentApprovalDecisionTarget struct {
	RunID      string
	ApprovalID string
}

func (m *agentManager) resolveAgentApprovalDecisionTarget(ctx context.Context, sessionID, prompt string, svc routerServices) (agentApprovalDecisionTarget, error) {
	runID := agentExtractRunID(prompt)
	approvalID := agentExtractApprovalID(prompt)
	if runID == "" {
		if session, err := m.GetSession(ctx, sessionID); err == nil && session.ActiveTask != nil {
			runID = strings.TrimSpace(session.ActiveTask.ActiveRunID)
		}
	}
	if runID != "" {
		approvals, err := svc.approvals.ListApprovals(ctx, runID)
		if err != nil {
			return agentApprovalDecisionTarget{}, err
		}
		if approvalID == "" {
			pending := agentPendingApprovals(approvals)
			if len(pending) == 1 {
				approvalID = pending[0].ID
			} else if len(pending) > 1 {
				return agentApprovalDecisionTarget{}, services.NewError(services.CodeBadRequest, "multiple pending approvals; specify approval_id or step_id", map[string]any{"run_id": runID, "pending": agentApprovalIDs(pending)}, nil)
			}
		}
		if approvalID == "" {
			return agentApprovalDecisionTarget{}, services.NewError(services.CodeBadRequest, "approval id or step id is required", map[string]any{"run_id": runID}, nil)
		}
		return agentApprovalDecisionTarget{RunID: runID, ApprovalID: approvalID}, nil
	}

	page, err := svc.runs.ListRuns(ctx, services.RunListRequest{Limit: 30})
	if err != nil {
		return agentApprovalDecisionTarget{}, err
	}
	var matches []agentApprovalDecisionTarget
	for _, run := range page.Items {
		approvals, listErr := svc.approvals.ListApprovals(ctx, run.RunID)
		if listErr != nil {
			continue
		}
		if approvalID != "" {
			if _, ok := agentFindApproval(approvals, approvalID); ok {
				matches = append(matches, agentApprovalDecisionTarget{RunID: run.RunID, ApprovalID: approvalID})
			}
			continue
		}
		for _, approval := range approvals {
			if approval.Status == contracts.HITLPending {
				matches = append(matches, agentApprovalDecisionTarget{RunID: run.RunID, ApprovalID: approval.ID})
			}
		}
	}
	switch len(matches) {
	case 0:
		return agentApprovalDecisionTarget{}, services.NewError(services.CodeBadRequest, "no matching pending approval found; include run_id and approval_id", nil, nil)
	case 1:
		return matches[0], nil
	default:
		return agentApprovalDecisionTarget{}, services.NewError(services.CodeBadRequest, "multiple matching approvals; include run_id and approval_id", nil, nil)
	}
}

func (m *agentManager) refreshApprovalGateForSessionLocked(ctx context.Context, session *agentSession) bool {
	if session == nil || session.ActiveTask == nil || strings.TrimSpace(session.ActiveTask.ActiveRunID) == "" {
		return false
	}
	runID := strings.TrimSpace(session.ActiveTask.ActiveRunID)
	approvals, err := m.approvals.ListApprovals(ctx, runID)
	if err != nil {
		return false
	}
	pending := 0
	for _, approval := range approvals {
		if approval.Status == contracts.HITLPending {
			pending++
		}
	}
	if pending == 0 {
		now := time.Now().UTC()
		todosChanged := resolveAgentApprovalGateTodos(session, now)
		if session.NextAction != nil && session.NextAction.Type == "approval_required" || todosChanged {
			session.NextAction = &agentNextAction{
				Type:            "continue",
				Reason:          agentText(session.Language, "审批已处理，可继续跟踪运行。", "Approvals have been handled; continue tracking the run."),
				SuggestedPrompt: agentSuggestedContinuePrompt(session.Language, session),
				AutoContinue:    true,
			}
			if session.Status == "idle" || session.Status == "blocked" {
				session.Status = "running"
			}
			if session.ActiveTask.Status == "idle" || session.ActiveTask.Status == "blocked" || session.ActiveTask.Status == "" {
				session.ActiveTask.Status = "running"
			}
			session.UpdatedAt = now
			session.ActiveTask.UpdatedAt = session.UpdatedAt
			return true
		}
		return false
	}
	now := time.Now().UTC()
	for idx := range session.Todos {
		if agentTodoBlocksOnApprovalGate(session.Todos[idx]) {
			markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", agentText(session.Language, "等待人工审批", "Waiting for human approval"))
		}
	}
	session.Status = "running"
	session.ActiveTask.Status = "running"
	session.ActiveTask.UpdatedAt = now
	session.NextAction = &agentNextAction{
		Type:            "approval_required",
		Reason:          agentText(session.Language, fmt.Sprintf("运行 %s 有 %d 个待审批动作。", runID, pending), fmt.Sprintf("Run %s has %d pending approval action(s).", runID, pending)),
		SuggestedPrompt: agentText(session.Language, "先处理审批队列，然后恢复已批准运行", "Review the approval queue, then resume the approved run"),
		AutoContinue:    false,
	}
	session.UpdatedAt = now
	return true
}

func resolveAgentApprovalGateTodos(session *agentSession, now time.Time) bool {
	if session == nil {
		return false
	}
	changed := false
	running := false
	for idx := range session.Todos {
		todo := session.Todos[idx]
		if todo.Status == "done" {
			continue
		}
		switch {
		case agentTodoWasBlockedByApprovalGate(todo) || agentTodoIsExplicitApprovalGate(todo):
			nextStatus := "done"
			if agentApprovalTodoShouldResume(todo) && todo.Status == "blocked" {
				nextStatus = "running"
			}
			if todo.Status != nextStatus || todo.BlockedReason != "" {
				markAgentTodoByIndex(session.Todos, idx, nextStatus, now, "", "")
				changed = true
			}
		}
		if session.Todos[idx].Status == "running" {
			running = true
		}
	}
	if running {
		return changed
	}
	for idx := range session.Todos {
		if session.Todos[idx].Status == "pending" {
			markAgentTodoByIndex(session.Todos, idx, "running", now, "", "")
			return true
		}
	}
	return changed
}

func agentTodoIsExplicitApprovalGate(todo agentTodo) bool {
	title := strings.ToLower(strings.TrimSpace(todo.Title))
	if title == "" {
		return false
	}
	hasApproval := strings.Contains(title, "审批") ||
		strings.Contains(title, "approval") ||
		strings.Contains(title, "approvals")
	if !hasApproval {
		return false
	}
	return strings.Contains(title, "待审批") ||
		strings.Contains(title, "pending approval") ||
		strings.Contains(title, "approval gate") ||
		strings.Contains(title, "审批 gate") ||
		strings.Contains(title, "审批队列") ||
		strings.Contains(title, "approval queue") ||
		strings.Contains(title, "处理") ||
		strings.Contains(title, "resolve")
}

func agentTodoBlocksOnApprovalGate(todo agentTodo) bool {
	title := strings.ToLower(strings.TrimSpace(todo.Title))
	if title == "" {
		return false
	}
	hasApproval := strings.Contains(title, "审批") ||
		strings.Contains(title, "approval") ||
		strings.Contains(title, "approvals")
	if !hasApproval {
		return false
	}
	return strings.Contains(title, "待审批") ||
		strings.Contains(title, "approval gate") ||
		strings.Contains(title, "pending approval") ||
		strings.Contains(title, "审批 gate") ||
		strings.Contains(title, "审批队列") ||
		strings.Contains(title, "approval queue") ||
		strings.Contains(title, "跟踪") ||
		strings.Contains(title, "track") ||
		strings.Contains(title, "monitor") ||
		strings.Contains(title, "处理") ||
		strings.Contains(title, "resolve") ||
		strings.Contains(title, "检查") ||
		strings.Contains(title, "check") ||
		strings.Contains(title, "确认") ||
		strings.Contains(title, "confirm")
}

func agentApprovalTodoShouldResume(todo agentTodo) bool {
	title := strings.ToLower(strings.TrimSpace(todo.Title))
	return (strings.Contains(title, "跟踪") ||
		strings.Contains(title, "track") ||
		strings.Contains(title, "monitor")) &&
		!strings.Contains(title, "待审批") &&
		!strings.Contains(title, "pending approval") &&
		!strings.Contains(title, "approval gate") &&
		!strings.Contains(title, "审批 gate")
}

func agentTodoWasBlockedByApprovalGate(todo agentTodo) bool {
	if todo.Status != "blocked" {
		return false
	}
	reason := strings.ToLower(strings.TrimSpace(todo.BlockedReason))
	return strings.Contains(reason, "审批") ||
		strings.Contains(reason, "approval") ||
		strings.Contains(reason, "human")
}

func (m *agentManager) appendMessages(sessionID string, messages ...agentMessage) {
	m.appendMessagesWithPlanner(sessionID, nil, messages...)
}

func (m *agentManager) appendMessagesWithPlanner(sessionID string, plan *agentPlannerResponse, messages ...agentMessage) {
	if m.runRuntimeAppendMessagesMutation(context.Background(), sessionID, plan, messages...) {
		return
	}
	_, _ = m.appendMessagesWithPlannerLegacy(sessionID, plan, messages...)
}

func (m *agentManager) appendMessagesForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, origin, operationLevel string, messages ...agentMessage) {
	m.appendMessagesWithPlannerForTurn(ctx, sessionID, turnRef, prompt, origin, operationLevel, nil, messages...)
}

func (m *agentManager) appendMessagesWithPlannerForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, origin, operationLevel string, plan *agentPlannerResponse, messages ...agentMessage) {
	if m.runRuntimeAppendMessagesMutationForTurn(ctx, sessionID, turnRef, prompt, origin, operationLevel, plan, messages...) {
		return
	}
	_, _ = m.appendMessagesWithPlannerLegacy(sessionID, plan, messages...)
}

func firstAgentMessageID(messages []agentMessage, fallback string) string {
	for _, message := range messages {
		if id := strings.TrimSpace(message.ID); id != "" {
			return id
		}
	}
	return strings.TrimSpace(fallback)
}

func (m *agentManager) appendMessagesWithPlannerLegacy(sessionID string, plan *agentPlannerResponse, messages ...agentMessage) (daemonAppendMessagesMutationResultPayload, bool) {
	eng := m.runtimeEngine()
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return daemonAppendMessagesMutationResultPayload{}, false
	}
	session.Messages = append(session.Messages, messages...)
	m.applyAgentMessagesLocked(session, messages...)
	if plan != nil {
		m.applyAgentPlannerLocked(session, plan, session.Language)
	}
	if len(messages) > 0 {
		session.UpdatedAt = messages[len(messages)-1].Timestamp
	}
	compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	_ = m.persistSessionLocked(session)
	sessionState := cloneAgentSession(session)
	m.mu.Unlock()
	m.emitRuntimeMemoryCompacted(context.Background(), sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
	m.emitAgentRuntimeEvents(context.Background(), eng, sessionID, sessionState, messages...)
	return daemonAppendMessagesMutationResultPayload{
		Session:    sessionState,
		Compaction: compaction,
		Mutated:    true,
	}, true
}

func (m *agentManager) message(language, role, content string, toolCalls []agentToolCall, links []agentLink) agentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.newMessageLocked(language, role, content, toolCalls, links)
}

func (m *agentManager) newMessageLocked(_ string, role, content string, toolCalls []agentToolCall, links []agentLink) agentMessage {
	now := time.Now().UTC()
	return agentMessage{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "msg_", 1),
		Role:      role,
		Content:   content,
		Timestamp: now,
		ToolCalls: cloneAgentToolCalls(toolCalls),
		Links:     cloneAgentLinks(links),
	}
}

// applyAgentPlannerToMessage promotes planner-declared suggestions and
// recommendation options onto the agentMessage so the frontend can render them
// from authoritative fields instead of re-parsing the message content.
func applyAgentPlannerToMessage(message *agentMessage, plan *agentPlannerResponse) {
	if message == nil || plan == nil {
		return
	}
	prompts := collectAgentPlannerSuggestedPrompts(plan)
	if len(prompts) > 0 {
		message.SuggestedPrompts = prompts
	}
	options := dedupeAgentRecommendationOptions(plan.RecommendationOptions)
	if len(options) > 0 {
		message.RecommendationOptions = options
	}
}

func collectAgentPlannerSuggestedPrompts(plan *agentPlannerResponse) []string {
	if plan == nil {
		return nil
	}
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	add := func(value string) {
		v := strings.TrimSpace(value)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	add(plan.SuggestedPrompt)
	add(plan.SuggestPrompt)
	for _, prompt := range plan.SuggestedPrompts {
		add(prompt)
	}
	if plan.NextAction != nil {
		add(plan.NextAction.SuggestedPrompt)
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func dedupeAgentRecommendationOptions(options []agentRecommendationOpt) []agentRecommendationOpt {
	if len(options) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(options))
	out := make([]agentRecommendationOpt, 0, len(options))
	for i, opt := range options {
		title := strings.TrimSpace(opt.Title)
		detail := strings.TrimSpace(opt.Detail)
		prompt := strings.TrimSpace(opt.Prompt)
		if title == "" && detail == "" && prompt == "" {
			continue
		}
		key := strings.ToLower(detail)
		if key == "" {
			key = strings.ToLower(title)
		}
		if key == "" {
			key = strings.ToLower(prompt)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		id := strings.TrimSpace(opt.ID)
		if id == "" {
			id = fmt.Sprintf("recommendation_%d", i+1)
		}
		out = append(out, agentRecommendationOpt{
			ID:      id,
			Title:   title,
			Detail:  detail,
			Prompt:  prompt,
			Command: strings.TrimSpace(opt.Command),
		})
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func agentNormalizePhase(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "plan", "planning":
		return "plan"
	case "step", "execute", "executing", "tool", "tool_execution":
		return "step"
	case "step_replan", "stepreplan", "replan", "replanning", "step-replan":
		return "step_replan"
	case "final_answer", "finalanswer", "final", "answer", "done", "complete":
		return "final_answer"
	default:
		return ""
	}
}

func agentMessageWithPhase(message agentMessage, phase string) agentMessage {
	message.Phase = agentNormalizePhase(phase)
	for i := range message.ToolCalls {
		message.ToolCalls[i] = agentToolCallWithPhase(message.ToolCalls[i], message.Phase)
	}
	return message
}

func agentMessagesWithPhase(messages []agentMessage, phase string) []agentMessage {
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return messages
	}
	out := append([]agentMessage(nil), messages...)
	for i := range out {
		out[i] = agentMessageWithPhase(out[i], normalized)
	}
	return out
}

func agentToolCallWithPhase(call agentToolCall, phase string) agentToolCall {
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return call
	}
	if call.Metadata == nil {
		call.Metadata = map[string]interface{}{}
	}
	if _, exists := call.Metadata["phase"]; exists {
		call.Metadata["agent_phase"] = normalized
	} else {
		call.Metadata["phase"] = normalized
	}
	return call
}

func agentToolCallsWithPhase(calls []agentToolCall, phase string) []agentToolCall {
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return calls
	}
	out := append([]agentToolCall(nil), calls...)
	for i := range out {
		out[i] = agentToolCallWithPhase(out[i], normalized)
	}
	return out
}

func normalizeAgentSessionID(raw string) (string, error) {
	sessionID := strings.TrimSpace(raw)
	if sessionID == "" {
		return "", services.NewError(services.CodeBadRequest, "session id is required", map[string]any{"field": "session_id"}, nil)
	}
	if strings.Contains(sessionID, "\x00") || strings.ContainsAny(sessionID, `/\`) || strings.Contains(sessionID, "..") {
		return "", services.NewError(services.CodeBadRequest, "invalid session id", map[string]any{"field": "session_id"}, nil)
	}
	return sessionID, nil
}

func normalizeAgentOperator(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "local-user"
	}
	return raw
}

func normalizeAgentLanguage(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "en", "en-us":
		return "en"
	default:
		return "zh"
	}
}

func normalizeAgentOperationLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "read_only", "readonly", "read-only", "ro", "只读":
		return "read_only"
	case "workspace_write", "workspace-write", "workspace", "write", "工作区写入":
		return "workspace_write"
	case "safe_scan", "safe-scan", "scan", "扫描", "安全扫描":
		return "safe_scan"
	case "approval_ops", "approval-ops", "approval", "approvals", "approve", "审批":
		return "approval_ops"
	case "bypass", "bypass_permissions", "bypass-permissions", "danger_full_access", "danger-full-access", "绕过权限", "跳过权限":
		return "bypass"
	case "", "advanced", "full", "高权限", "高级":
		return "advanced"
	default:
		return "advanced"
	}
}

func agentOperationLevelLabel(level, language string) string {
	switch normalizeAgentOperationLevel(level) {
	case "read_only":
		return agentText(language, "只读", "Read-only")
	case "workspace_write":
		return agentText(language, "工作区写入", "Workspace write")
	case "safe_scan":
		return agentText(language, "安全扫描", "Safe scan")
	case "approval_ops":
		return agentText(language, "审批操作", "Approval ops")
	case "advanced":
		return agentText(language, "高级", "Advanced")
	case "bypass":
		return agentText(language, "Bypass 权限", "Bypass permissions")
	default:
		return agentText(language, "只读", "Read-only")
	}
}

func agentOperationLevelDescription(level, language string) string {
	switch normalizeAgentOperationLevel(level) {
	case "read_only":
		return agentText(language,
			"只能执行 daemon 健康检查、插件/运行/审批读取、工作区列文件/文件树/搜索/读文件和白名单只读命令。",
			"May only run daemon health checks, plugin/run/approval reads, workspace file listing/tree/search/reading, and allowlisted read-only commands.",
		)
	case "workspace_write":
		return agentText(language,
			"包含只读能力，并允许写入、生成 diff、patch、应用、丢弃、删除、重命名、复制文件或创建当前 Agent 工作区目录。",
			"Includes read-only capabilities and permits writing, staging diffs, patching, applying, discarding, deleting, renaming/copying files, or creating directories in the Agent workspace.",
		)
	case "safe_scan":
		return agentText(language,
			"包含工作区能力，并允许对明确授权目标发起低风险 HTTP 请求或启动 safe profile Web/API 扫描；外部工具默认仍由扫描策略控制。",
			"Includes workspace capabilities and permits low-risk HTTP requests or safe-profile Web/API scans for explicitly authorized targets; external tools remain controlled by scan policy.",
		)
	case "approval_ops":
		return agentText(language,
			"包含安全扫描能力，并允许批准/拒绝审批与恢复已批准运行。",
			"Includes safe-scan capabilities and permits approving/denying approvals and resuming approved runs.",
		)
	case "advanced":
		return agentText(language,
			"最高内置权限：允许当前 Agent 已实现的工具动作；仍受 scope、白名单命令和显式用户意图约束。",
			"Highest built-in level: permits implemented Agent tool actions while still enforcing scope, command allowlists, and explicit operator intent.",
		)
	default:
		return agentOperationLevelDescription("read_only", language)
	}
}

func agentOperationLevelRank(level string) int {
	switch normalizeAgentOperationLevel(level) {
	case "read_only":
		return 0
	case "workspace_write":
		return 1
	case "safe_scan":
		return 2
	case "approval_ops":
		return 3
	case "advanced":
		return 4
	case "bypass":
		return 5
	default:
		return 0
	}
}

func agentOperationBypassPermissions(level string) bool {
	return normalizeAgentOperationLevel(level) == "bypass"
}

func agentOperationLevelAtLeast(level, required string) bool {
	return agentOperationLevelRank(level) >= agentOperationLevelRank(required)
}

func agentOperationLevelRequiredForAction(actionName string) string {
	// Delegate to the canonical tool registry. Unknown actions (plugin / MCP
	// names) fall through to read_only to preserve legacy behaviour during the
	// migration window — the registry-side default is "advanced", but flipping
	// that here would gate previously-allowed plugin invocations.
	if spec, ok := toolRegistry().Lookup(actionName); ok {
		return spec.RequiredLevel
	}
	return "read_only"
}

func agentOperationLevelRequiredForPlannerAction(actionName string, args map[string]any) string {
	normalized := normalizeAgentPlannerActionName(actionName)
	if normalized == "http.request" {
		method := firstNonEmpty(stringFromAny(args["method"]), "GET")
		if agentHTTPMethodIsSafe(method) {
			return "safe_scan"
		}
		return "advanced"
	}
	return agentOperationLevelRequiredForAction(normalized)
}

func agentPlannerActionRequiresManualAuthorization(actionName string) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "scan.web.safe",
		"ports.scan.safe",
		"http.request",
		"workspace.files.write",
		"workspace.files.patch",
		"workspace.files.diff_many",
		"workspace.files.rebase",
		"workspace.files.apply",
		"workspace.files.discard",
		"workspace.files.delete",
		"workspace.files.move",
		"workspace.files.copy",
		"workspace.files.mkdir",
		"reports.draft",
		"runs.approvals.approve",
		"runs.approvals.deny",
		"runs.resume_approved":
		return true
	default:
		return false
	}
}
