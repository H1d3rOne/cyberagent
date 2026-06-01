package daemon

import (
	"fmt"
	"strings"
	"time"
)

func agentRunContextHasFindings(result map[string]any) bool {
	if result == nil {
		return false
	}
	return intFromAny(result["finding_count"]) > 0 || intFromAny(result["findings"]) > 0
}

func (m *agentManager) ensureRunEvidenceActionTodosLocked(session *agentSession, call agentToolCall, now time.Time) bool {
	if session == nil {
		return false
	}
	runID := stringFromAny(call.Result["run_id"])
	if runID == "" {
		if session.ActiveTask != nil {
			runID = session.ActiveTask.ActiveRunID
		}
	}
	for _, todo := range session.Todos {
		if strings.HasPrefix(todo.ID, "todo_run_action_") {
			return false
		}
	}
	refs := contextRefIDsFromToolCall(call)
	if runID != "" {
		refs = appendUniqueString(refs, "run:"+runID, stringFromAny(call.Result["context"])+":"+runID)
	}
	start := len(session.Todos) + 1
	titles := []string{
		agentText(session.Language, "关联 finding、evidence 与 artifact 证据链", "Correlate findings, evidence and artifact chain"),
		agentText(session.Language, "执行误报检查并明确影响边界", "Perform false-positive checks and clarify impact boundary"),
		agentText(session.Language, "生成修复建议与最小复测计划", "Produce remediation guidance and minimal retest plan"),
	}
	for idx, title := range titles {
		status := "pending"
		if idx == 0 {
			status = "running"
		}
		session.Todos = append(session.Todos, agentTodo{
			ID:            fmt.Sprintf("todo_run_action_%02d", idx+1),
			Title:         title,
			Status:        status,
			Priority:      start + idx,
			ToolCallID:    call.ID,
			ContextRefIDs: refs,
			UpdatedAt:     now,
		})
	}
	if session.ActiveTask != nil {
		session.ActiveTask.Status = "running"
		session.ActiveTask.Summary = trimSummary(agentText(session.Language,
			"已读取运行 "+emptyDash(runID)+" 的报告发现，正在把证据转成误报检查、修复和复测行动计划。",
			"Read findings for run "+emptyDash(runID)+"; converting evidence into false-positive checks, remediation and retest plan.",
		), 360)
		session.ActiveTask.UpdatedAt = now
	}
	session.Status = "running"
	session.NextAction = &agentNextAction{
		Type:            "continue",
		Reason:          agentText(session.Language, "报告中存在发现，已自动生成后续安全行动计划。", "Findings exist in the report; generated the follow-up security action plan."),
		SuggestedPrompt: agentSuggestedEvidenceActionPrompt(session.Language, runID),
		AutoContinue:    true,
	}
	return true
}

func (m *agentManager) advanceRunEvidenceActionTodoLocked(session *agentSession, call agentToolCall, now time.Time) {
	if session == nil {
		return
	}
	advanced := false
	for idx := range session.Todos {
		if !strings.HasPrefix(session.Todos[idx].ID, "todo_run_action_") || session.Todos[idx].Status != "running" {
			continue
		}
		markAgentTodoByIndex(session.Todos, idx, "done", now, call.ID, "")
		for next := idx + 1; next < len(session.Todos); next++ {
			if strings.HasPrefix(session.Todos[next].ID, "todo_run_action_") && session.Todos[next].Status == "pending" {
				markAgentTodoByIndex(session.Todos, next, "running", now, call.ID, "")
				advanced = true
				break
			}
		}
		if !advanced {
			advanced = true
		}
		break
	}
	if !advanced {
		return
	}
	runID := stringFromAny(call.Result["run_id"])
	if runID == "" && session.ActiveTask != nil {
		runID = session.ActiveTask.ActiveRunID
	}
	if sessionHasRunEvidenceActionTodos(session) {
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(session.Language, "已推进一个证据行动 TODO，可继续完成误报检查、修复或复测计划。", "Advanced one evidence-action TODO; continue to complete false-positive check, remediation, or retest plan."),
			SuggestedPrompt: agentSuggestedEvidenceActionPrompt(session.Language, runID),
			AutoContinue:    true,
		}
		return
	}
	session.Status = "idle"
	if session.ActiveTask != nil {
		session.ActiveTask.Status = "done"
		session.ActiveTask.Summary = trimSummary(agentText(session.Language,
			"运行 "+emptyDash(runID)+" 的发现已完成证据链关联、误报检查、修复建议与复测计划整理。",
			"Completed evidence correlation, false-positive checks, remediation guidance and retest planning for run "+emptyDash(runID)+".",
		), 360)
		session.ActiveTask.UpdatedAt = now
	}
	session.NextAction = &agentNextAction{
		Type:            "wait_user",
		Reason:          agentText(session.Language, "证据行动计划已完成，等待用户要求导出报告、复测或开始新任务。", "Evidence action plan completed; waiting for export, retest, or a new task."),
		SuggestedPrompt: agentSuggestedRunSummaryPrompt(session.Language, runID),
	}
}

func agentToolCompletesLocalTask(name string) bool {
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
		strings.Contains(name, "http.request"),
		strings.Contains(name, "llm"):
		return true
	default:
		return false
	}
}

func (m *agentManager) updateNextActionLocked(session *agentSession) {
	now := time.Now().UTC()
	if session.ActiveTask != nil {
		session.ActiveTask.UpdatedAt = now
	}
	if session.PendingAuthorization != nil && strings.EqualFold(session.PendingAuthorization.Status, "pending") {
		session.Status = "waiting_authorization"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
		}
		if session.NextAction == nil || session.NextAction.Type != "authorization_required" {
			session.NextAction = &agentNextAction{
				Type:            "authorization_required",
				Reason:          session.PendingAuthorization.Reason,
				SuggestedPrompt: agentAuthorizationSuggestedPrompt(session.Language, session.PendingAuthorization),
				AutoContinue:    false,
			}
		}
		session.UpdatedAt = now
		return
	}
	if session.NextAction != nil && session.NextAction.Type == "approval_required" {
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
		}
		session.UpdatedAt = now
		return
	}
	if session.NextAction != nil && session.NextAction.Type == "wait_user" && agentNextActionIsEvidenceActionComplete(session.NextAction) {
		session.Status = "idle"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "done"
		}
		session.UpdatedAt = now
		return
	}
	pending := 0
	running := 0
	blocked := 0
	for _, todo := range session.Todos {
		switch todo.Status {
		case "pending":
			pending++
		case "running":
			running++
		case "blocked":
			blocked++
		}
	}
	switch {
	case blocked > 0:
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
		}
		if session.NextAction == nil || session.NextAction.Type != "blocked" {
			session.NextAction = &agentNextAction{Type: "blocked", Reason: "tool or policy step is blocked"}
		}
	case running > 0 || pending > 0:
		session.Status = "running"
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          "task has pending or running steps",
			SuggestedPrompt: agentSuggestedContinuePrompt(session.Language, session),
			AutoContinue:    session.ActiveTask != nil && strings.TrimSpace(session.ActiveTask.ActiveRunID) != "",
		}
	default:
		session.Status = "idle"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "done"
		}
		session.NextAction = &agentNextAction{Type: "finished", Reason: "task steps completed"}
	}
	session.UpdatedAt = now
}

func agentNextActionIsEvidenceActionComplete(next *agentNextAction) bool {
	if next == nil {
		return false
	}
	reason := strings.ToLower(strings.TrimSpace(next.Reason))
	return strings.Contains(reason, "证据行动计划已完成") ||
		strings.Contains(reason, "evidence action plan completed")
}

func agentTaskKind(prompt string) string {
	if decision := agentClassifyCapabilityIntent(prompt); decision.ID != "" && decision.ID != agentIntentGeneralChat {
		return decision.Kind
	}
	switch {
	case agentIsPortScanIntent(prompt):
		return "port_scan"
	case agentIsScanIntent(prompt):
		return "scan"
	case agentIsResumeApprovedIntent(prompt):
		return "approval"
	case agentIsRunContextAnalysisIntent(prompt):
		return "context_analysis"
	case agentIsApprovalIntent(prompt):
		return "approval"
	case agentIsHealthIntent(prompt):
		return "health"
	case agentIsPluginIntent(prompt):
		return "tools"
	case agentIsRunIntent(prompt):
		return "runs"
	default:
		return "chat"
	}
}

func agentTaskGoal(language, kind, prompt string) string {
	if kind == "scan" {
		target := agentExtractURL(prompt)
		scope := agentExtractScope(prompt, target)
		return agentText(language, "扫描授权目标 "+emptyDash(target)+" scope="+emptyDash(scope), "Scan authorized target "+emptyDash(target)+" scope="+emptyDash(scope))
	}
	if kind == "http_probe" {
		target := agentExtractURL(prompt)
		return agentText(language, "探测 HTTP 响应 "+emptyDash(target), "Probe HTTP response for "+emptyDash(target))
	}
	if kind == "port_scan" {
		target := firstNonEmpty(agentExtractURL(prompt), agentExtractPortScanHostFromPrompt(prompt))
		return agentText(language, "枚举开放端口 "+emptyDash(target), "Enumerate open ports for "+emptyDash(target))
	}
	if kind == "content_discovery" || kind == "js_route_analysis" || kind == "api_auth_test" {
		return agentCapabilityGoal(language, agentClassifyCapabilityIntent(prompt), prompt)
	}
	if kind == "context_analysis" {
		return agentText(language, "分析运行上下文与证据", "Analyze run context and evidence")
	}
	if kind == "approval" {
		if agentIsResumeApprovedIntent(prompt) {
			return agentText(language, "恢复已批准运行步骤", "Resume approved run steps")
		}
		return agentText(language, "检查待审批动作", "Check pending approvals")
	}
	if kind == "health" {
		return agentText(language, "检查 daemon 健康状态", "Check daemon health")
	}
	if kind == "tools" {
		return agentText(language, "列出 Agent 工具和插件能力", "List Agent tools and plugin capabilities")
	}
	if kind == "runs" {
		return agentText(language, "查看运行历史和报告上下文", "Review run history and report context")
	}
	return agentText(language, "回答并澄清安全任务", "Answer and clarify the security task")
}

func agentTaskSummary(language, kind string) string {
	switch kind {
	case "scan":
		return agentText(language, "Agent 将校验 scope、启动 safe profile、跟踪运行并收集报告上下文。", "The Agent will validate scope, start the safe profile, track the run and collect report context.")
	case "http_probe":
		return agentText(language, "Agent 将执行最小 HTTP 请求并根据响应状态、响应头和正文摘要给出结论。", "The Agent will run the minimal HTTP request and summarize status, headers, and body snippet.")
	case "port_scan":
		return agentText(language, "Agent 会识别这是端口枚举任务；若缺少 nmap/masscan 类工具，会先提示可选替代方案。", "The Agent will classify this as port enumeration; if nmap/masscan-style tools are missing, it will offer alternatives first.")
	case "content_discovery":
		return agentText(language, "Agent 会识别这是内容发现/目录枚举任务；缺少等价工具时不会误用漏洞扫描代替。", "The Agent will classify this as content discovery/directory enumeration; without an equivalent tool it will not substitute vulnerability scanning.")
	case "js_route_analysis":
		return agentText(language, "Agent 会识别这是 JS 路由/前端接口提取任务；缺少分析器时等待插件或 JS 文件。", "The Agent will classify this as JS route/front-end endpoint extraction; without an analyzer it waits for a plugin or JS files.")
	case "api_auth_test":
		return agentText(language, "Agent 会识别这是认证/鉴权测试；需要具体 endpoint、凭据/Auth Profile 或专用流程。", "The Agent will classify this as auth/authorization testing; it needs exact endpoints, credentials/Auth Profile, or a dedicated workflow.")
	case "web_search":
		return agentText(language, "Agent 将使用配置的 Web Search provider 检索公开信息并返回来源摘要。", "The Agent will use the configured Web Search provider to look up public information and summarize sourced results.")
	case "web_fetch":
		return agentText(language, "Agent 将读取指定公开 URL，并在 SSRF 防护下返回可读内容摘要。", "The Agent will read the specified public URL and return readable content under SSRF protections.")
	case "context_analysis":
		return agentText(language, "Agent 将读取 run / artifact / finding 上下文并给出下一步建议。", "The Agent will read run / artifact / finding context and suggest next actions.")
	default:
		return agentText(language, "Agent 将选择最小必要工具并返回可执行结果。", "The Agent will select the minimal required tools and return actionable results.")
	}
}

func agentInitialTodos(language, kind string, now time.Time) []agentTodo {
	titles := []string{}
	switch kind {
	case "scan":
		titles = []string{
			agentText(language, "确认目标与 scope", "Confirm target and scope"),
			agentText(language, "生成 safe profile 执行计划", "Create safe-profile plan"),
			agentText(language, "启动 daemon Web 扫描", "Start daemon web scan"),
			agentText(language, "跟踪运行事件、证据和审批", "Track run events, evidence and approvals"),
			agentText(language, "读取报告并总结下一步", "Read report and summarize next actions"),
		}
	case "port_scan":
		titles = []string{
			agentText(language, "确认目标与端口枚举范围", "Confirm target and port-enumeration scope"),
			agentText(language, "检查是否有端口扫描专用工具", "Check for a dedicated port-scanning tool"),
			agentText(language, "选择 nmap/masscan 插件或 HTTP 探测替代方案", "Choose nmap/masscan plugin or HTTP-probe fallback"),
			agentText(language, "汇总开放端口证据与限制", "Summarize open-port evidence and limitations"),
		}
	case "http_probe":
		titles = []string{
			agentText(language, "确认 URL 与 HTTP 方法", "Confirm URL and HTTP method"),
			agentText(language, "执行最小 HTTP 请求", "Run the minimal HTTP request"),
			agentText(language, "总结状态码、响应头和正文摘要", "Summarize status, headers, and body snippet"),
		}
	case "content_discovery":
		titles = []string{
			agentText(language, "确认这是内容发现/目录枚举", "Confirm this is content discovery/directory enumeration"),
			agentText(language, "检查是否有 ffuf/gobuster/dirsearch 等价工具", "Check for equivalent ffuf/gobuster/dirsearch tool"),
			agentText(language, "等待插件或具体路径验证方案", "Wait for plugin or exact-path validation fallback"),
		}
	case "js_route_analysis":
		titles = []string{
			agentText(language, "确认这是 JS 路由/前端接口提取", "Confirm this is JS route/front-end endpoint extraction"),
			agentText(language, "检查是否有 JS 路由分析器", "Check for a JS route analyzer"),
			agentText(language, "等待插件、JS 文件或入口页验证方案", "Wait for plugin, JS files, or entry-page fallback"),
		}
	case "api_auth_test":
		titles = []string{
			agentText(language, "确认认证/鉴权测试目标", "Confirm auth/authorization test target"),
			agentText(language, "收集 endpoint、method、Auth Profile/凭据上下文", "Collect endpoint, method, Auth Profile/credential context"),
			agentText(language, "选择 http.request 验证或专用插件流程", "Choose http.request validation or dedicated plugin workflow"),
		}
	case "web_search":
		titles = []string{
			agentText(language, "确认检索问题", "Confirm search query"),
			agentText(language, "使用 Web Search provider 检索", "Search with the Web Search provider"),
			agentText(language, "汇总来源和关键信息", "Summarize sources and key information"),
		}
	case "web_fetch":
		titles = []string{
			agentText(language, "确认要读取的 URL", "Confirm URL to read"),
			agentText(language, "获取并提取页面内容", "Fetch and extract page content"),
			agentText(language, "总结页面要点", "Summarize page takeaways"),
		}
	case "context_analysis":
		titles = []string{
			agentText(language, "定位 run / finding / artifact", "Locate run / finding / artifact"),
			agentText(language, "读取上下文和报告", "Read context and report"),
			agentText(language, "总结影响、证据和复测建议", "Summarize impact, evidence and retest advice"),
		}
	default:
		titles = []string{
			agentText(language, "理解用户意图", "Understand user intent"),
			agentText(language, "选择必要工具或本地回答", "Select required tool or local answer"),
			agentText(language, "返回结果和下一步", "Return result and next action"),
		}
	}
	todos := make([]agentTodo, 0, len(titles))
	for i, title := range titles {
		status := "pending"
		if i == 0 {
			status = "running"
		}
		todos = append(todos, agentTodo{
			ID:        fmt.Sprintf("todo_%02d", i+1),
			Title:     title,
			Status:    status,
			Priority:  i + 1,
			UpdatedAt: now,
		})
	}
	return todos
}

func agentIsEvidenceActionPrompt(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "evidence") && (strings.Contains(normalized, "next action") || strings.Contains(normalized, "action plan")) {
		return true
	}
	return strings.Contains(prompt, "证据") && (strings.Contains(prompt, "下一步行动计划") || strings.Contains(prompt, "行动计划"))
}

func agentEvidenceActionHasApprovalGate(prompt string) bool {
	normalized := strings.ToLower(prompt)
	return strings.Contains(normalized, "pending_approvals=1") ||
		strings.Contains(normalized, "pending approvals:") ||
		strings.Contains(normalized, "approval gate") ||
		strings.Contains(prompt, "待审批") ||
		strings.Contains(prompt, "审批 gate")
}

func agentEvidenceActionTodos(language, prompt string, now time.Time) []agentTodo {
	hasApproval := agentEvidenceActionHasApprovalGate(prompt)
	titles := []string{}
	if hasApproval {
		titles = append(titles, agentText(language, "处理待审批 gate 并确认是否可继续", "Resolve pending approval gate and confirm whether execution can continue"))
	}
	titles = append(titles,
		agentText(language, "读取 run/report/artifact/approval 证据上下文", "Read run/report/artifact/approval evidence context"),
		agentText(language, "关联关键 finding、artifact 与 step", "Correlate key findings, artifacts and steps"),
		agentText(language, "形成下一步安全动作与复测计划", "Produce next safe action and retest plan"),
	)
	if !hasApproval {
		titles = append(titles, agentText(language, "执行低风险只读验证或等待人工确认", "Run low-risk read-only validation or wait for operator confirmation"))
	}
	todos := make([]agentTodo, 0, len(titles))
	for i, title := range titles {
		status := "pending"
		if i == 0 {
			status = "running"
		}
		todos = append(todos, agentTodo{
			ID:        fmt.Sprintf("todo_evidence_%02d", i+1),
			Title:     title,
			Status:    status,
			Priority:  i + 1,
			UpdatedAt: now,
		})
	}
	return todos
}

func markAgentTodoByIndex(todos []agentTodo, idx int, status string, now time.Time, toolCallID, blockedReason string) {
	if idx < 0 || idx >= len(todos) {
		return
	}
	todos[idx].Status = status
	todos[idx].UpdatedAt = now
	if toolCallID != "" {
		todos[idx].ToolCallID = toolCallID
	}
	todos[idx].BlockedReason = blockedReason
}

func (m *agentManager) ensureRunningTodoLocked(session *agentSession, now time.Time) {
	if len(session.Todos) == 0 {
		return
	}
	for idx := range session.Todos {
		if session.Todos[idx].Status == "running" {
			return
		}
	}
	for idx := range session.Todos {
		if session.Todos[idx].Status == "pending" || session.Todos[idx].Status == "blocked" {
			markAgentTodoByIndex(session.Todos, idx, "running", now, "", "")
			return
		}
	}
}

func (m *agentManager) findActiveJobIDLocked(session *agentSession, runID string) string {
	for i := len(session.ToolCalls) - 1; i >= 0; i-- {
		call := session.ToolCalls[i]
		if call.Result == nil {
			continue
		}
		callRunID := stringFromAny(call.Result["run_id"])
		if runID != "" && callRunID != "" && callRunID != runID {
			continue
		}
		if jobID := stringFromAny(call.Result["job_id"]); jobID != "" {
			return jobID
		}
	}
	return ""
}

func agentSuggestedContinuePrompt(language string, session *agentSession) string {
	if session != nil && session.ActiveTask != nil && session.ActiveTask.ActiveRunID != "" {
		if sessionHasRunEvidenceActionTodos(session) {
			return agentSuggestedEvidenceActionPrompt(language, session.ActiveTask.ActiveRunID)
		}
		return agentText(language, "继续跟踪运行 "+session.ActiveTask.ActiveRunID+" 的上下文、证据并总结下一步", "continue tracking run "+session.ActiveTask.ActiveRunID+" context and evidence, then summarize next steps")
	}
	return agentText(language, "继续当前任务", "continue the current task")
}

func sessionHasRunEvidenceActionTodos(session *agentSession) bool {
	if session == nil {
		return false
	}
	for _, todo := range session.Todos {
		if strings.HasPrefix(todo.ID, "todo_run_action_") && (todo.Status == "running" || todo.Status == "pending") {
			return true
		}
	}
	return false
}

func agentEvidenceActionStageFromTodos(todos []agentTodo) int {
	for _, todo := range todos {
		if !strings.HasPrefix(todo.ID, "todo_run_action_") || todo.Status != "running" {
			continue
		}
		switch todo.ID {
		case "todo_run_action_01":
			return 1
		case "todo_run_action_02":
			return 2
		case "todo_run_action_03":
			return 3
		}
	}
	for _, todo := range todos {
		if strings.HasPrefix(todo.ID, "todo_run_action_") {
			return 1
		}
	}
	return 0
}

func agentEvidenceActionStageConclusion(language, runID string, stage int, result map[string]any) string {
	findings := intFromAny(result["finding_count"])
	if findings == 0 {
		findings = intFromAny(result["findings"])
	}
	evidence := intFromAny(result["evidence_count"])
	artifacts := intFromAny(result["artifact_count"])
	switch stage {
	case 0:
		return agentText(language,
			fmt.Sprintf("已基于运行 %s 的报告生成后续行动计划：先关联 %d 个 finding 与 %d 条 evidence / %d 个 artifact，再做误报检查、影响边界、修复建议和复测计划。", emptyDash(runID), findings, evidence, artifacts),
			fmt.Sprintf("Generated a follow-up action plan from run %s: correlate %d finding(s) with %d evidence item(s) / %d artifact(s), then perform false-positive checks, impact scoping, remediation and retest planning.", emptyDash(runID), findings, evidence, artifacts),
		)
	case 1:
		return agentText(language,
			fmt.Sprintf("证据链关联结论：运行 %s 当前至少包含 %d 个 finding、%d 条 evidence、%d 个 artifact；下一步应按 evidence_ids/step_id 把每个发现映射到原始响应、头信息或工具输出，避免凭标题直接下结论。", emptyDash(runID), findings, evidence, artifacts),
			fmt.Sprintf("Evidence-chain conclusion: run %s has at least %d finding(s), %d evidence item(s), and %d artifact(s). Next, map each finding via evidence_ids/step_id to raw responses, headers, or tool output before drawing conclusions.", emptyDash(runID), findings, evidence, artifacts),
		)
	case 2:
		return agentText(language,
			fmt.Sprintf("误报检查与影响边界：对运行 %s 的发现，应优先确认是否由反向代理、CDN、开发服务器或测试夹具导致；再结合授权目标的真实暴露面判断业务影响。", emptyDash(runID)),
			fmt.Sprintf("False-positive and impact boundary: for run %s, first check whether reverse proxies, CDNs, dev servers, or test fixtures explain the findings; then scope business impact against the authorized target exposure.", emptyDash(runID)),
		)
	case 3:
		return agentText(language,
			fmt.Sprintf("修复与复测计划：对运行 %s，建议按证据优先级修复配置或代码，再用最小复现命令验证，最后让 Agent 对同一 scope 重跑 safe profile 确认 finding 是否消失。", emptyDash(runID)),
			fmt.Sprintf("Remediation and retest plan: for run %s, fix configuration or code by evidence priority, verify with a minimal reproduction command, then ask the Agent to rerun the safe profile on the same scope.", emptyDash(runID)),
		)
	default:
		return ""
	}
}

func agentSuggestedEvidenceActionPrompt(language, runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return agentText(language,
			"基于当前运行 findings 生成影响、证据、误报检查、修复与复测计划",
			"build an impact, evidence, false-positive check, remediation and retest plan from the current run findings",
		)
	}
	return agentText(language,
		"基于运行 "+runID+" 的 findings 生成影响、证据、误报检查、修复与复测计划",
		"build an impact, evidence, false-positive check, remediation and retest plan from findings in run "+runID,
	)
}

func agentSuggestedRunSummaryPrompt(language, runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return agentText(language, "总结最近一次运行的证据、发现和下一步建议", "summarize the latest run evidence, findings, and next steps")
	}
	return agentText(language, "总结运行 "+runID+" 的证据、发现和下一步建议", "summarize evidence, findings, and next steps for run "+runID)
}

func agentSuggestedStoppedRunPrompt(language, runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return agentText(language, "分析最近一次停止运行的错误、证据和下一步恢复建议", "analyze the latest stopped run error, evidence, and recovery suggestions")
	}
	return agentText(language, "分析运行 "+runID+" 的停止原因、已有证据和下一步恢复建议", "analyze why run "+runID+" stopped, existing evidence, and recovery suggestions")
}

func agentSuggestedStopPrompt(language string, session *agentSession) string {
	if session != nil && session.ActiveTask != nil && session.ActiveTask.ActiveRunID != "" {
		return agentText(language, "分析已停止运行 "+session.ActiveTask.ActiveRunID+" 的当前证据", "analyze the current evidence for stopped run "+session.ActiveTask.ActiveRunID)
	}
	return agentText(language, "解释当前任务为何停止，并给出下一步", "explain why the current task stopped and suggest next steps")
}
