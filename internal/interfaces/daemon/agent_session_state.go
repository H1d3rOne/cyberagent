package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

func (m *agentManager) loadSessions() error {
	if strings.TrimSpace(m.storeDir) == "" {
		return nil
	}
	entries, err := os.ReadDir(m.storeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(m.storeDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var session agentSession
		if err := json.Unmarshal(data, &session); err != nil || session.SessionID == "" {
			continue
		}
		session.normalize()
		changed := normalizeCompletedOneShotAgentSession(&session)
		if normalizeLegacyDoctorMessages(&session) {
			changed = true
		}
		cloned := session
		m.sessions[session.SessionID] = &cloned
		if changed {
			_ = m.persistSessionLocked(&cloned)
		}
	}
	return nil
}

func (m *agentManager) loadSessionLocked(sessionID string) (*agentSession, error) {
	sessionID, err := normalizeAgentSessionID(sessionID)
	if err != nil {
		return nil, err
	}
	path := m.sessionPath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, services.NewError(services.CodeNotFound, "agent session not found", map[string]any{"session_id": sessionID}, nil)
		}
		return nil, services.NewError(services.CodeInternal, "read agent session", map[string]any{"session_id": sessionID}, err)
	}
	var session agentSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, services.NewError(services.CodeInternal, "decode agent session", map[string]any{"session_id": sessionID}, err)
	}
	if session.SessionID == "" {
		session.SessionID = sessionID
	}
	session.normalize()
	changed := normalizeCompletedOneShotAgentSession(&session)
	if normalizeLegacyDoctorMessages(&session) {
		changed = true
	}
	m.sessions[session.SessionID] = &session
	if changed {
		_ = m.persistSessionLocked(&session)
	}
	return &session, nil
}

func (m *agentManager) persistSessionLocked(session *agentSession) error {
	if session == nil || strings.TrimSpace(session.SessionID) == "" {
		return nil
	}
	session.normalize()
	if err := os.MkdirAll(m.storeDir, 0o755); err != nil {
		return services.NewError(services.CodeInternal, "create agent session store", map[string]any{"path": m.storeDir}, err)
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return services.NewError(services.CodeInternal, "encode agent session", map[string]any{"session_id": session.SessionID}, err)
	}
	path := m.sessionPath(session.SessionID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return services.NewError(services.CodeInternal, "write agent session", map[string]any{"session_id": session.SessionID}, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return services.NewError(services.CodeInternal, "commit agent session", map[string]any{"session_id": session.SessionID}, err)
	}
	return nil
}

func (m *agentManager) sessionPath(sessionID string) string {
	return filepath.Join(m.storeDir, sessionID+".json")
}

func (s *agentSession) normalize() {
	if s.Status == "" {
		s.Status = "idle"
	}
	s.OperationLevel = normalizeAgentOperationLevel(s.OperationLevel)
	if s.Messages == nil {
		s.Messages = []agentMessage{}
	}
	if s.Todos == nil {
		s.Todos = []agentTodo{}
	}
	if s.ToolCalls == nil {
		s.ToolCalls = []agentToolCall{}
	}
	if s.ContextRefs == nil {
		s.ContextRefs = []agentContextRef{}
	}
	if s.PendingChanges == nil {
		s.PendingChanges = []agentWorkspaceChange{}
	}
	if s.NextAction == nil {
		s.NextAction = &agentNextAction{Type: "wait_user"}
	}
}

func normalizeCompletedOneShotAgentSession(session *agentSession) bool {
	if session == nil || len(session.Todos) == 0 || len(session.ToolCalls) == 0 {
		return false
	}
	if session.PendingAuthorization != nil && strings.EqualFold(session.PendingAuthorization.Status, "pending") {
		return false
	}
	if session.ActiveTask != nil && strings.TrimSpace(session.ActiveTask.ActiveRunID) != "" {
		return false
	}
	if sessionHasScanToolCall(session) {
		return false
	}
	last := lastSuccessfulAgentToolCall(session)
	if last == nil || !agentToolCompletesLegacyOneShotTask(strings.ToLower(last.Name)) {
		return false
	}
	needsFix := session.Status == "running" || (session.ActiveTask != nil && session.ActiveTask.Status == "running")
	if session.NextAction != nil && session.NextAction.Type == "continue" {
		needsFix = true
	}
	for _, todo := range session.Todos {
		if todo.Status == "running" || todo.Status == "pending" {
			needsFix = true
			break
		}
	}
	if !needsFix {
		return false
	}
	now := time.Now().UTC()
	for idx := range session.Todos {
		if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
			markAgentTodoByIndex(session.Todos, idx, "done", now, last.ID, "")
		}
	}
	session.Status = "idle"
	if session.ActiveTask != nil {
		session.ActiveTask.Status = "done"
		session.ActiveTask.UpdatedAt = now
	}
	session.NextAction = &agentNextAction{Type: "finished", Reason: "task steps completed"}
	return true
}

func normalizeLegacyDoctorMessages(session *agentSession) bool {
	if session == nil {
		return false
	}
	changed := false
	for idx := range session.Messages {
		if !agentMessageHasSucceededDoctorTool(session.Messages[idx]) {
			continue
		}
		normalized, ok := normalizeLegacyDoctorMessageContent(session.Messages[idx].Content, session.Language)
		if !ok {
			continue
		}
		session.Messages[idx].Content = normalized
		changed = true
	}
	return changed
}

func agentMessageHasSucceededDoctorTool(message agentMessage) bool {
	for _, call := range message.ToolCalls {
		if call.Status == contracts.StatusSucceeded && strings.Contains(strings.ToLower(call.Name), "doctor") {
			return true
		}
	}
	return false
}

func normalizeLegacyDoctorMessageContent(content, language string) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || strings.Contains(trimmed, "必需检查") || strings.Contains(trimmed, "Required checks") {
		return "", false
	}
	if !strings.Contains(trimmed, "体检") || !strings.Contains(trimmed, "通过") || !strings.Contains(trimmed, "异常") {
		return "", false
	}
	version := "v0.1.0"
	if match := regexp.MustCompile(`版本\s*([^。,\s]+)`).FindStringSubmatch(trimmed); len(match) == 2 {
		version = match[1]
	}
	checks := 0
	if match := regexp.MustCompile(`检查项\s*(\d+)`).FindStringSubmatch(trimmed); len(match) == 2 {
		fmt.Sscanf(match[1], "%d", &checks)
	}
	optionalMissing := 0
	if match := regexp.MustCompile(`异常\s*(\d+)`).FindStringSubmatch(trimmed); len(match) == 2 {
		fmt.Sscanf(match[1], "%d", &optionalMissing)
	}
	if optionalMissing <= 0 {
		return "", false
	}
	if checks <= 0 {
		checks = optionalMissing
	}
	return agentText(language,
		fmt.Sprintf("Daemon 在线，版本 %s。必需检查通过，检查项 %d；历史体检中的 %d 个可选工具缺失已归一化为可选项（不影响内置 safe 能力）。", version, checks, optionalMissing),
		fmt.Sprintf("Daemon online, version %s. Required checks passed, %d checks total; %d optional missing tool(s) from a legacy doctor summary are now normalized as optional and non-blocking.", version, checks, optionalMissing),
	), true
}

func sessionHasScanToolCall(session *agentSession) bool {
	if session == nil {
		return false
	}
	for _, call := range session.ToolCalls {
		if strings.Contains(strings.ToLower(call.Name), "scans/web") {
			return true
		}
	}
	return false
}

func lastSuccessfulAgentToolCall(session *agentSession) *agentToolCall {
	if session == nil {
		return nil
	}
	for i := len(session.ToolCalls) - 1; i >= 0; i-- {
		if session.ToolCalls[i].Status == contracts.StatusSucceeded {
			return &session.ToolCalls[i]
		}
	}
	for i := len(session.Messages) - 1; i >= 0; i-- {
		for j := len(session.Messages[i].ToolCalls) - 1; j >= 0; j-- {
			if session.Messages[i].ToolCalls[j].Status == contracts.StatusSucceeded {
				return &session.Messages[i].ToolCalls[j]
			}
		}
	}
	return nil
}

func agentToolCompletesLegacyOneShotTask(name string) bool {
	switch {
	case strings.Contains(name, "doctor"),
		strings.Contains(name, "plugins"),
		strings.Contains(name, "runs"),
		strings.Contains(name, "approvals"),
		strings.Contains(name, "context"),
		strings.Contains(name, "workspace.command"),
		strings.Contains(name, "workspace.prepare"),
		strings.Contains(name, "workspace.files.list"),
		strings.Contains(name, "workspace.files.tree"),
		strings.Contains(name, "workspace.files.read"),
		strings.Contains(name, "workspace.search"),
		strings.Contains(name, "workspace.files.write"),
		strings.Contains(name, "workspace.files.patch"),
		strings.Contains(name, "workspace.files.diff"),
		strings.Contains(name, "workspace.files.rebase"),
		strings.Contains(name, "workspace.files.apply"),
		strings.Contains(name, "workspace.files.discard"),
		strings.Contains(name, "workspace.files.delete"),
		strings.Contains(name, "workspace.files.move"),
		strings.Contains(name, "workspace.files.copy"),
		strings.Contains(name, "workspace.files.mkdir"),
		strings.Contains(name, "http.request"):
		return true
	default:
		return false
	}
}

func (m *agentManager) shouldStartNewAgentTaskLocked(session *agentSession, prompt, language string) bool {
	if session == nil {
		return true
	}
	if session.ActiveTask == nil {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(session.ActiveTask.Status))
	if status == "done" || status == "blocked" {
		return true
	}
	if agentIsApprovalIntent(prompt) || agentIsHealthIntent(prompt) || agentIsPluginIntent(prompt) || agentIsRunIntent(prompt) {
		return false
	}
	if agentIsScanIntent(prompt) {
		return !m.hasActiveRunForSameScanLocked(session, prompt)
	}
	return false
}

func (m *agentManager) hasActiveRunForSameScanLocked(session *agentSession, prompt string) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
	currentTarget := strings.TrimSpace(agentExtractURL(prompt))
	if currentTarget == "" {
		return false
	}
	for _, tool := range session.ToolCalls {
		if tool.Name != "POST /api/v1/scans/web" {
			continue
		}
		var previousRunID string
		if tool.Result != nil {
			previousRunID = stringFromAny(tool.Result["run_id"])
		}
		if previousRunID == "" {
			continue
		}
		if session.ActiveTask.ActiveRunID == previousRunID {
			return true
		}
	}
	return false
}

func (m *agentManager) startAgentTaskLocked(session *agentSession, prompt, language string) {
	now := time.Now().UTC()
	kind := agentTaskKind(prompt)
	goal := agentTaskGoal(language, kind, prompt)
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
		Goal:      goal,
		Kind:      kind,
		Status:    "running",
		Summary:   agentTaskSummary(language, kind),
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.Todos = agentInitialTodos(language, kind, now)
	session.NextAction = &agentNextAction{
		Type:   "continue",
		Reason: agentText(language, "正在规划并选择工具。", "Planning and selecting tools."),
	}
	session.UpdatedAt = now
	m.applyAgentRunLinkFromMessagesLocked(session, session.Messages)
}

func (m *agentManager) applyAgentRunLinkFromMessagesLocked(session *agentSession, messages []agentMessage) {
	if session == nil || len(messages) == 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		for _, tool := range message.ToolCalls {
			if tool.Result == nil {
				continue
			}
			runID := stringFromAny(tool.Result["run_id"])
			if runID == "" {
				continue
			}
			if session.ActiveTask == nil {
				now := time.Now().UTC()
				session.ActiveTask = &agentTask{
					ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
					Goal:      agentText(session.Language, "跟踪运行上下文", "Track run context"),
					Kind:      "ops",
					Status:    "running",
					CreatedAt: now,
					UpdatedAt: now,
				}
			}
			if session.ActiveTask.ActiveRunID == "" {
				session.ActiveTask.ActiveRunID = runID
			}
			if session.ActiveTask.Summary == "" {
				session.ActiveTask.Summary = agentText(session.Language, "正在跟踪运行 "+shortAgentID(runID, 18)+" 的上下文。", "Tracking context for run "+shortAgentID(runID, 18)+".")
			}
			return
		}
	}
}

func (m *agentManager) ensureAgentActiveTaskSummaryLocked(session *agentSession, answer string) {
	if session == nil || strings.TrimSpace(answer) == "" {
		return
	}
	trimmed := strings.TrimSpace(answer)
	if len(trimmed) > 320 {
		trimmed = trimmed[:320]
	}
	if session.ActiveTask == nil {
		now := time.Now().UTC()
		session.ActiveTask = &agentTask{
			ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
			Goal:      agentText(session.Language, "总结 Agent 回复", "Summarize agent reply"),
			Kind:      "planning",
			Status:    "running",
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	if session.ActiveTask.Summary == "" {
		session.ActiveTask.Summary = trimmed
	}
	session.ActiveTask.UpdatedAt = time.Now().UTC()
}

func (m *agentManager) touchAgentTaskForMessageLocked(session *agentSession, prompt, language string) {
	if session == nil {
		return
	}
	now := time.Now().UTC()
	session.Status = "running"
	session.UpdatedAt = now
	if session.ActiveTask != nil {
		if session.ActiveTask.Status != "blocked" && session.ActiveTask.Status != "done" {
			session.ActiveTask.Status = "running"
		}
		session.ActiveTask.UpdatedAt = now
	}
	if session.NextAction == nil {
		session.NextAction = &agentNextAction{
			Type:   "continue",
			Reason: agentText(language, "用户继续当前任务上下文。", "Operator continues the current task context."),
		}
	}
}

func (m *agentManager) applyAgentMessagesLocked(session *agentSession, messages ...agentMessage) {
	for _, message := range messages {
		if len(message.ToolCalls) > 0 {
			m.applyAgentToolCallsLocked(session, message.ToolCalls...)
		}
		for _, link := range message.Links {
			m.addContextRefLocked(session, contextRefFromLink(link, message.Content))
		}
	}
	m.applyAgentRunLinkFromMessagesLocked(session, messages)
	_ = m.refreshApprovalGateForSessionLocked(context.Background(), session)
	m.updateNextActionLocked(session)
}

func (m *agentManager) applyAgentPlannerLocked(session *agentSession, plan *agentPlannerResponse, language string) {
	if session == nil || plan == nil {
		return
	}
	now := time.Now().UTC()
	language = normalizeAgentLanguage(language)
	session.Language = language

	if plan.Answer != "" {
		m.ensureAgentActiveTaskSummaryLocked(session, plan.Answer)
	}

	if plan.Task != nil {
		if session.ActiveTask == nil {
			session.ActiveTask = &agentTask{
				ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
				Kind:      "planning",
				Status:    "running",
				CreatedAt: now,
				UpdatedAt: now,
			}
		}
		if goal := strings.TrimSpace(plan.Task.Goal); goal != "" {
			session.ActiveTask.Goal = trimSummary(goal, 260)
		}
		if kind := normalizeAgentPlannerTaskKind(plan.Task.Kind); kind != "" {
			session.ActiveTask.Kind = kind
		}
		if status := normalizeAgentPlannerTaskStatus(plan.Task.Status); status != "" {
			session.ActiveTask.Status = status
			session.Status = agentSessionStatusFromTask(status)
		}
		if activeRunID := strings.TrimSpace(plan.Task.ActiveRunID); activeRunID != "" {
			session.ActiveTask.ActiveRunID = agentCleanToken(activeRunID)
		}
		if summary := strings.TrimSpace(plan.Task.Summary); summary != "" {
			session.ActiveTask.Summary = trimSummary(summary, 360)
		}
		if strings.TrimSpace(session.ActiveTask.Goal) == "" {
			session.ActiveTask.Goal = agentText(language, "规划安全分析任务", "Plan security analysis task")
		}
		if strings.TrimSpace(session.ActiveTask.Kind) == "" {
			session.ActiveTask.Kind = "planning"
		}
		if strings.TrimSpace(session.ActiveTask.Status) == "" {
			session.ActiveTask.Status = "running"
		}
		session.ActiveTask.UpdatedAt = now
	}

	if len(plan.ContextRefs) > 0 {
		for _, ref := range plan.ContextRefs {
			m.addContextRefLocked(session, sanitizeAgentPlannerContextRef(ref))
		}
	}

	if len(plan.Todos) > 0 {
		todos := make([]agentTodo, 0, len(plan.Todos))
		for _, todo := range plan.Todos {
			title := strings.TrimSpace(todo.Title)
			if title == "" {
				continue
			}
			priority := todo.Priority
			if priority <= 0 {
				priority = len(todos) + 1
			}
			todos = append(todos, agentTodo{
				ID:            fmt.Sprintf("todo_%02d", len(todos)+1),
				Title:         trimSummary(title, 180),
				Detail:        trimSummary(todo.Detail, 360),
				Status:        normalizeAgentTodoStatus(todo.Status),
				Priority:      priority,
				ContextRefIDs: append([]string(nil), todo.ContextRefIDs...),
				BlockedReason: trimSummary(todo.BlockedReason, 260),
				UpdatedAt:     now,
			})
		}
		if len(todos) > 0 {
			todos = mergeAgentRecoveryTodos(todos, session.Todos)
			session.Todos = todos
			m.updateSessionStatusFromTodosLocked(session)
		}
	}

	if plan.NextAction != nil {
		next := sanitizeAgentNextAction(plan.NextAction)
		if next != nil {
			session.NextAction = next
			switch next.Type {
			case "blocked", "approval_required":
				if next.Type == "blocked" {
					session.Status = "blocked"
					if session.ActiveTask != nil {
						session.ActiveTask.Status = "blocked"
					}
				}
			case "finished":
				session.Status = "idle"
				if session.ActiveTask != nil {
					session.ActiveTask.Status = "done"
				}
			case "wait_user":
				if session.Status == "" || session.Status == "running" {
					session.Status = "idle"
				}
			case "continue":
				session.Status = "running"
				if session.ActiveTask != nil && session.ActiveTask.Status != "blocked" {
					session.ActiveTask.Status = "running"
				}
			}
		}
	}

	// Parse reasoning from Planner response
	if reasoning := strings.TrimSpace(plan.Reasoning); reasoning != "" {
		// Store reasoning in the last agent message
		for i := len(session.Messages) - 1; i >= 0; i-- {
			if session.Messages[i].Role == "agent" {
				session.Messages[i].Reasoning = trimSummary(reasoning, 800)
				break
			}
		}
	}

	session.UpdatedAt = now
}

func normalizeAgentPlannerTaskKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "chat", "triage", "planning", "ops", "scan", "context_analysis", "approval", "tools", "runs", "health":
		return strings.ToLower(strings.TrimSpace(raw))
	case "context-analysis", "context analysis":
		return "context_analysis"
	default:
		return ""
	}
}

func normalizeAgentPlannerTaskStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "running", "blocked", "done", "idle":
		return strings.ToLower(strings.TrimSpace(raw))
	case "complete", "completed", "finished":
		return "done"
	default:
		return ""
	}
}

func agentSessionStatusFromTask(taskStatus string) string {
	switch normalizeAgentPlannerTaskStatus(taskStatus) {
	case "blocked":
		return "blocked"
	case "done", "idle":
		return "idle"
	case "running":
		return "running"
	default:
		return "running"
	}
}

func normalizeAgentTodoStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending", "running", "done", "blocked", "skipped":
		return strings.ToLower(strings.TrimSpace(raw))
	case "todo", "open":
		return "pending"
	case "in_progress", "in-progress", "progress":
		return "running"
	case "complete", "completed", "finished":
		return "done"
	default:
		return "pending"
	}
}

func mergeAgentRecoveryTodos(planned, existing []agentTodo) []agentTodo {
	if len(existing) == 0 {
		return planned
	}
	seen := map[string]struct{}{}
	for _, todo := range planned {
		if todo.ID != "" {
			seen[todo.ID] = struct{}{}
		}
		if todo.ToolCallID != "" {
			seen["tool:"+todo.ToolCallID] = struct{}{}
		}
	}
	out := append([]agentTodo(nil), planned...)
	for _, todo := range existing {
		if !strings.HasPrefix(todo.ID, "todo_recover_") {
			continue
		}
		if _, ok := seen[todo.ID]; ok {
			continue
		}
		if todo.ToolCallID != "" {
			if _, ok := seen["tool:"+todo.ToolCallID]; ok {
				continue
			}
			seen["tool:"+todo.ToolCallID] = struct{}{}
		}
		todo.Priority = len(out) + 1
		out = append(out, todo)
		seen[todo.ID] = struct{}{}
	}
	return out
}

func sanitizeAgentPlannerContextRef(ref agentContextRef) agentContextRef {
	ref.ID = agentCleanToken(ref.ID)
	ref.Kind = strings.ToLower(agentCleanToken(ref.Kind))
	ref.RefID = agentCleanToken(ref.RefID)
	ref.Label = trimSummary(ref.Label, 120)
	ref.To = strings.TrimSpace(ref.To)
	ref.Summary = trimSummary(ref.Summary, 180)
	if ref.Kind == "" {
		ref.Kind = "context"
	}
	if ref.RefID == "" {
		ref.RefID = ref.ID
	}
	if ref.ID == "" && ref.Kind != "" && ref.RefID != "" {
		ref.ID = ref.Kind + ":" + ref.RefID
	}
	if ref.Label == "" {
		ref.Label = ref.Kind + " " + shortAgentID(ref.RefID, 18)
	}
	return ref
}

func sanitizeAgentNextAction(action *agentNextAction) *agentNextAction {
	if action == nil {
		return nil
	}
	nextType := strings.ToLower(strings.TrimSpace(action.Type))
	switch nextType {
	case "continue", "wait_user", "approval_required", "finished", "blocked":
	case "wait-user", "wait":
		nextType = "wait_user"
	case "approval", "approve":
		nextType = "approval_required"
	case "done", "complete", "completed":
		nextType = "finished"
	default:
		nextType = "continue"
	}
	return &agentNextAction{
		Type:            nextType,
		Reason:          trimSummary(action.Reason, 260),
		SuggestedPrompt: trimSummary(action.SuggestedPrompt, 260),
		AutoContinue:    action.AutoContinue && nextType == "continue",
	}
}

func (m *agentManager) updateSessionStatusFromTodosLocked(session *agentSession) {
	if session == nil {
		return
	}
	blocked := false
	runningOrPending := false
	for _, todo := range session.Todos {
		switch todo.Status {
		case "blocked":
			blocked = true
		case "running", "pending":
			runningOrPending = true
		}
	}
	switch {
	case blocked:
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
		}
	case runningOrPending:
		session.Status = "running"
		if session.ActiveTask != nil && session.ActiveTask.Status != "blocked" {
			session.ActiveTask.Status = "running"
		}
	default:
		session.Status = "idle"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "done"
		}
	}
}

func (m *agentManager) applyAgentToolCallsLocked(session *agentSession, calls ...agentToolCall) {
	for _, call := range calls {
		session.ToolCalls = upsertAgentToolCall(session.ToolCalls, call)
		m.updateTodosForToolLocked(session, call)
		for _, ref := range contextRefsFromToolCall(call) {
			m.addContextRefLocked(session, ref)
		}
	}
	if len(session.ToolCalls) > 40 {
		session.ToolCalls = append([]agentToolCall(nil), session.ToolCalls[len(session.ToolCalls)-40:]...)
	}
}

func (m *agentManager) updateTodosForToolLocked(session *agentSession, call agentToolCall) {
	now := time.Now().UTC()
	if len(session.Todos) == 0 {
		return
	}
	if call.Status == contracts.StatusFailed {
		recovery := agentRecoveryForFailedTool(session.Language, call)
		reason := firstNonEmpty(recovery.Reason, agentToolError(call))
		markAgentTodoByIndex(session.Todos, len(session.Todos)-1, "blocked", now, call.ID, reason)
		upsertAgentRecoveryTodo(session, call, recovery, now)
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.Summary = trimSummary(reason, 360)
			session.ActiveTask.UpdatedAt = now
		}
		session.Status = "blocked"
		session.NextAction = &agentNextAction{
			Type:            firstNonEmpty(recovery.Type, "blocked"),
			Reason:          reason,
			SuggestedPrompt: recovery.SuggestedPrompt,
			AutoContinue:    recovery.AutoContinue && recovery.Type == "continue",
		}
		return
	}

	name := strings.ToLower(call.Name)
	switch {
	case strings.Contains(name, "scans/web"):
		markAgentTodoByIndex(session.Todos, 0, "done", now, "", "")
		markAgentTodoByIndex(session.Todos, 1, "done", now, "", "")
		markAgentTodoByIndex(session.Todos, 2, "done", now, call.ID, "")
		markAgentTodoByIndex(session.Todos, 3, "running", now, call.ID, "")
		if len(session.Todos) > 4 {
			session.Todos[4].ContextRefIDs = appendUniqueString(session.Todos[4].ContextRefIDs, contextRefIDsFromToolCall(call)...)
		}
		if session.ActiveTask != nil {
			if runID := stringFromAny(call.Result["run_id"]); runID != "" {
				session.ActiveTask.ActiveRunID = runID
			}
			if target := stringFromAny(call.Result["target"]); target != "" {
				session.ActiveTask.Summary = trimSummary(agentText(session.Language,
					fmt.Sprintf("已启动 safe profile 扫描：target=%s，scope=%s，run=%s。正在跟踪事件、证据、审批和报告。", target, emptyDash(stringFromAny(call.Result["scope"])), emptyDash(stringFromAny(call.Result["run_id"]))),
					fmt.Sprintf("Started safe-profile scan: target=%s, scope=%s, run=%s. Tracking events, evidence, approvals and report.", target, emptyDash(stringFromAny(call.Result["scope"])), emptyDash(stringFromAny(call.Result["run_id"]))),
				), 360)
			}
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
	case strings.Contains(name, "resume-approved"):
		if session.ActiveTask != nil {
			if runID := stringFromAny(call.Result["run_id"]); runID != "" {
				session.ActiveTask.ActiveRunID = runID
			}
			session.ActiveTask.Status = "running"
			session.ActiveTask.Summary = trimSummary(agentText(session.Language,
				"已恢复已批准步骤，继续跟踪运行事件、证据和报告。",
				"Resumed approved steps; continue tracking run events, evidence and report.",
			), 360)
			session.ActiveTask.UpdatedAt = now
		}
		hasRunning := false
		firstPending := -1
		for idx := range session.Todos {
			if session.Todos[idx].Status == "done" {
				continue
			}
			title := strings.ToLower(session.Todos[idx].Title)
			if strings.Contains(title, "恢复") ||
				strings.Contains(title, "resume") ||
				agentTodoWasBlockedByApprovalGate(session.Todos[idx]) ||
				agentTodoIsExplicitApprovalGate(session.Todos[idx]) {
				markAgentTodoByIndex(session.Todos, idx, "done", now, call.ID, "")
				continue
			}
			if session.Todos[idx].Status == "running" {
				hasRunning = true
			}
			if session.Todos[idx].Status == "pending" && firstPending < 0 {
				firstPending = idx
			}
		}
		if !hasRunning && firstPending >= 0 {
			markAgentTodoByIndex(session.Todos, firstPending, "running", now, call.ID, "")
			hasRunning = true
		}
		if !hasRunning {
			session.Todos = append(session.Todos, agentTodo{
				ID:            fmt.Sprintf("todo_resume_track_%d", len(session.Todos)+1),
				Title:         agentText(session.Language, "跟踪恢复后的运行事件、证据和报告", "Track events, evidence and report after resume"),
				Status:        "running",
				Priority:      len(session.Todos) + 1,
				ToolCallID:    call.ID,
				ContextRefIDs: contextRefIDsFromToolCall(call),
				UpdatedAt:     now,
			})
		}
	case strings.Contains(name, "runs/{run_id}/context"):
		contextKind := stringFromAny(call.Result["context"])
		if session.ActiveTask != nil && session.ActiveTask.Kind == "scan" && len(session.Todos) >= 5 {
			markAgentTodoByIndex(session.Todos, 3, "done", now, call.ID, "")
			switch contextKind {
			case "report_json", "findings":
				markAgentTodoByIndex(session.Todos, 4, "done", now, call.ID, "")
				if agentRunContextHasFindings(call.Result) {
					if !m.ensureRunEvidenceActionTodosLocked(session, call, now) {
						m.advanceRunEvidenceActionTodoLocked(session, call, now)
					}
				}
			default:
				if session.Todos[4].Status == "pending" {
					markAgentTodoByIndex(session.Todos, 4, "running", now, call.ID, "")
				}
			}
			if session.ActiveTask.ActiveRunID == "" {
				if runID := stringFromAny(call.Result["run_id"]); runID != "" {
					session.ActiveTask.ActiveRunID = runID
				}
			}
			session.ActiveTask.UpdatedAt = now
		} else {
			for idx := range session.Todos {
				if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
					markAgentTodoByIndex(session.Todos, idx, "done", now, call.ID, "")
				}
			}
		}
	case strings.Contains(name, "/approvals/") && (strings.Contains(name, "/approve") || strings.Contains(name, "/deny")):
		if session.ActiveTask != nil {
			if runID := stringFromAny(call.Result["run_id"]); runID != "" {
				session.ActiveTask.ActiveRunID = runID
			}
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		resolveAgentApprovalGateTodos(session, now)
	case agentToolCompletesLocalTask(name):
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "done", now, call.ID, "")
			}
		}
	}
}
