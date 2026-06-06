package daemon

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/workspace"
)

func (m *agentManager) handleAgentActiveRunContinueForTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, bool) {
	sessionState, err := m.GetSession(ctx, sessionID)
	if err != nil || sessionState.ActiveTask == nil {
		return nil, nil, false
	}
	runID := strings.TrimSpace(sessionState.ActiveTask.ActiveRunID)
	if runID == "" {
		return nil, nil, false
	}
	evidenceStage := agentEvidenceActionStageFromTodos(sessionState.Todos)
	contextKind := "events"
	if detail, detailErr := svc.runs.GetRun(ctx, runID); detailErr == nil {
		switch {
		case detail.Summary.Counts.PendingApprovals > 0:
			contextKind = "approvals"
		case detail.Summary.Report.JSON:
			contextKind = "report_json"
		case detail.Summary.Counts.Findings > 0:
			contextKind = "findings"
		case detail.Summary.Counts.Artifacts > 0:
			contextKind = "artifact_index"
		default:
			contextKind = "events"
		}
	}
	prompt := fmt.Sprintf("读取运行 %s 的 %s 上下文", runID, contextKind)
	messages, toolCalls, routeErr := m.routePromptWithRuntimeDeterministicRoute(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, svc)
	if routeErr != nil || len(messages) == 0 {
		return nil, nil, false
	}
	if len(toolCalls) > 0 && agentRunContextHasFindings(toolCalls[0].Result) {
		conclusion := agentEvidenceActionStageConclusion(language, runID, evidenceStage, toolCalls[0].Result)
		if conclusion != "" {
			messages = append(messages, m.message(language, "agent", conclusion, nil, []agentLink{{To: "/scan/" + runID, Label: agentText(language, "打开运行详情", "Open run detail"), Kind: "run"}}))
		}
	}
	return messages, toolCalls, true
}

func (m *agentManager) handleAgentRunContextAnalysis(ctx context.Context, prompt, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	runID := agentExtractRunID(prompt)
	if runID == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我已识别为上下文分析，但缺少 run id。请从运行详情页的 finding / artifact 按钮交给 Agent。", "I recognized a context-analysis request, but no run id was found. Send it from a finding / artifact button on the run detail page."), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("GET /api/v1/runs/{run_id}/context", shortAgentID(runID, 22))
	detail, err := svc.runs.GetRun(ctx, runID)
	contextKind := agentRunContextKind(prompt)
	if err == nil {
		base := map[string]any{
			"run_id":    runID,
			"context":   contextKind,
			"target":    detail.Summary.Target,
			"status":    string(detail.Summary.Status),
			"events":    detail.Summary.Counts.Events,
			"artifacts": detail.Summary.Counts.Artifacts,
			"findings":  detail.Summary.Counts.Findings,
			"approvals": detail.Summary.Counts.Approvals,
		}
		switch contextKind {
		case "artifact_index":
			var index any
			index, err = svc.runs.GetArtifactIndex(ctx, runID)
			if err == nil {
				base["artifact_count"] = agentArtifactIndexCount(index)
				base["sample_artifacts"] = agentArtifactIndexSamples(index, 5)
			}
		case "events":
			var page services.EventPage
			page, err = svc.runs.GetEvents(ctx, runID, "", 12)
			if err == nil {
				base["event_count"] = len(page.Items)
				base["eof"] = page.EOF
				base["next_cursor"] = page.NextCursor
				base["sample_events"] = agentEventPageSamples(page, 6)
			}
		case "approvals":
			var approvals []contracts.HITLRequest
			approvals, err = svc.approvals.ListApprovals(ctx, runID)
			if err == nil {
				base["approval_count"] = len(approvals)
				base["pending"] = agentPendingApprovalCount(approvals)
				base["sample_approvals"] = agentApprovalSamples(approvals, 5)
			}
		case "findings":
			var report services.RunReport
			report, err = svc.runs.GetReport(ctx, runID, "json")
			if err == nil {
				findings, evidence, artifacts := agentReportJSONCounts(report.Bytes)
				base["finding_count"] = findings
				base["evidence_count"] = evidence
				base["artifact_count"] = artifacts
				base["sample_findings"] = agentReportJSONFindingSamples(report.Bytes, 5)
			} else if services.ErrorCodeOf(err) == services.CodeNotFound {
				err = nil
				base["report_available"] = false
				base["report_status"] = "pending"
				base["finding_count"] = 0
				base["evidence_count"] = 0
				base["artifact_count"] = detail.Summary.Counts.Artifacts
				if page, pageErr := svc.runs.GetEvents(ctx, runID, "", 8); pageErr == nil {
					base["event_count"] = len(page.Items)
					base["sample_events"] = agentEventPageSamples(page, 5)
				}
			}
		default:
			contextKind = "report_json"
			base["context"] = contextKind
			var report services.RunReport
			report, err = svc.runs.GetReport(ctx, runID, "json")
			if err == nil {
				findings, evidence, artifacts := agentReportJSONCounts(report.Bytes)
				base["format"] = report.Format
				base["content_type"] = report.ContentType
				base["bytes"] = len(report.Bytes)
				base["finding_count"] = findings
				base["evidence_count"] = evidence
				base["artifact_count"] = artifacts
				base["sample_findings"] = agentReportJSONFindingSamples(report.Bytes, 5)
			} else if services.ErrorCodeOf(err) == services.CodeNotFound {
				err = nil
				base["report_available"] = false
				base["report_status"] = "pending"
				base["finding_count"] = detail.Summary.Counts.Findings
				base["evidence_count"] = 0
				base["artifact_count"] = detail.Summary.Counts.Artifacts
				if page, pageErr := svc.runs.GetEvents(ctx, runID, "", 8); pageErr == nil {
					base["event_count"] = len(page.Items)
					base["sample_events"] = agentEventPageSamples(page, 5)
				}
			}
		}
		tool.Result = base
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	content := agentBuildRunContextObservation(language, detail, contextKind, tool.Result)
	message := m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/scan/" + runID, Label: agentText(language, "打开运行详情", "Open run detail"), Kind: "run"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentRunContextTool(ctx context.Context, sessionID, runID, contextKind, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		if session, err := m.GetSession(ctx, sessionID); err == nil && session.ActiveTask != nil {
			runID = strings.TrimSpace(session.ActiveTask.ActiveRunID)
		}
	}
	if runID == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要 run id 或当前活动运行，才能读取运行上下文。", "I need a run ID or an active run to read run context."), nil, nil)}, nil, nil
	}
	contextKind = agentNormalizeRunContextKind(contextKind)
	return m.handleAgentRunContextAnalysis(ctx, fmt.Sprintf("读取运行 %s 的 %s 上下文", runID, contextKind), language, svc)
}

func (m *agentManager) handleAgentReportDraft(ctx context.Context, sessionID, runID, language string, svc routerServices, outputPath string) ([]agentMessage, []agentToolCall, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		if session, err := m.GetSession(ctx, sessionID); err == nil && session.ActiveTask != nil {
			runID = strings.TrimSpace(session.ActiveTask.ActiveRunID)
		}
	}
	if runID == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要 run id 或当前活动运行，才能生成报告草稿。", "I need a run ID or an active run to generate a report draft."), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("reports.draft", shortAgentID(runID, 22))
	detail, err := svc.runs.GetRun(ctx, runID)
	var (
		ws       *workspace.Workspace
		snapshot agentRunReportSnapshot
		content  string
	)
	if err == nil {
		snapshot = m.activeRunReportSnapshot(ctx, runID, detail)
		content = agentBuildReportDraftMarkdown(language, detail, snapshot, time.Now().UTC())
		outputPath = agentReportDraftPath(runID, outputPath)
		ws, err = m.workspace.Create(sessionID)
	}
	if err == nil {
		err = m.workspace.WriteFile(sessionID, outputPath, []byte(content))
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"run_id":          runID,
		"path":            outputPath,
		"bytes":           len(content),
		"workspace_path":  ws.Path,
		"target":          detail.Summary.Target,
		"status":          string(detail.Summary.Status),
		"finding_count":   len(snapshot.Findings),
		"evidence_count":  len(snapshot.Evidence),
		"artifact_count":  len(snapshot.Artifacts),
		"context":         "report_draft",
		"workspace_scope": "agent_workspace",
		"draft_format":    "markdown",
		"review_required": true,
	}
	message := m.message(language, "tool", agentText(
		language,
		fmt.Sprintf("已生成报告草稿：%s（run=%s，findings=%d）。请在工作区预览并人工复核后再对外输出。", outputPath, shortAgentID(runID, 22), len(snapshot.Findings)),
		fmt.Sprintf("Generated report draft: %s (run=%s, findings=%d). Preview it in the workspace and review before external use.", outputPath, shortAgentID(runID, 22), len(snapshot.Findings)),
	), []agentToolCall{tool}, []agentLink{{To: "/scan/" + runID, Label: agentText(language, "打开运行详情", "Open run detail"), Kind: "run"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentScan(ctx context.Context, prompt, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	target := agentExtractURL(prompt)
	if target == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要一个 http/https 授权目标 URL 才能开始扫描。", "I need an authorized http/https target URL to start a scan."), nil, nil)}, nil, nil
	}
	scope := agentExtractScope(prompt, target)
	allowExternalTools := agentWantsExternalTools(prompt)
	plan := m.message(language, "agent", agentText(
		language,
		fmt.Sprintf("已生成执行计划：校验 scope=%s → 使用 safe profile → %s → 收集证据与报告。", emptyDash(scope), agentExternalToolPhrase(language, allowExternalTools)),
		fmt.Sprintf("Plan generated: validate scope=%s → use safe profile → %s → collect evidence and report.", emptyDash(scope), agentExternalToolPhrase(language, allowExternalTools)),
	), nil, nil)

	tool := runningAgentToolCall("POST /api/v1/scans/web", agentHostLabel(target))
	response, err := svc.jobs.StartWebScan(ctx, startWebScanJobRequest{
		Target:          target,
		Scope:           scope,
		Profile:         "safe",
		NoExternalTools: !allowExternalTools,
		EnabledPlugins:  nil,
	})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{plan, m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"job_id":            response.JobID,
		"run_id":            response.RunID,
		"run_url":           response.RunURL,
		"status_url":        response.StatusURL,
		"target":            target,
		"scope":             scope,
		"profile":           "safe",
		"no_external_tools": !allowExternalTools,
		"next_step":         "track_run_context",
	}
	done := m.message(language, "tool", agentText(
		language,
		fmt.Sprintf("已把任务交给 daemon：job=%s，run=%s。运行详情会持续同步事件、证据、审批和报告。", shortAgentID(response.JobID, 22), shortAgentID(response.RunID, 22)),
		fmt.Sprintf("Task handed to daemon: job=%s, run=%s. The run detail will keep syncing events, evidence, approvals and reports.", shortAgentID(response.JobID, 22), shortAgentID(response.RunID, 22)),
	), []agentToolCall{tool}, []agentLink{{To: "/scan/" + response.RunID, Label: agentText(language, "打开运行", "Open run"), Kind: "run"}})
	return []agentMessage{plan, done}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentPortScan(ctx context.Context, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	target := agentExtractURL(prompt)
	host := agentExtractPortScanHostFromPrompt(prompt)
	if host == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要一个授权目标主机，才能执行端口探测。", "I need an authorized target host to run a port probe."), nil, nil)}, nil, nil
	}
	if target == "" {
		target = host
	}
	ports, portLabel, err := agentExtractSafePortScanPorts(prompt)
	if err != nil {
		tool := failedAgentToolCall("ports.scan.safe", err)
		return []agentMessage{m.message(language, "agent", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	plan := m.message(language, "agent", agentText(
		language,
		fmt.Sprintf("已生成端口探测计划：目标=%s → 端口集=%s → TCP connect 安全探测 → 汇总开放端口与限制。", host, portLabel),
		fmt.Sprintf("Port probe plan generated: target=%s → port set=%s → safe TCP connect probe → summarize open ports and limitations.", host, portLabel),
	), nil, nil)

	timeout := 750 * time.Millisecond
	tool := runningAgentToolCall("ports.scan.safe", host)
	results := agentScanTCPPorts(ctx, host, ports, timeout)
	openPorts := make([]int, 0)
	closedCount := 0
	errorSamples := make([]string, 0, 3)
	for _, result := range results {
		if result.Open {
			openPorts = append(openPorts, result.Port)
			continue
		}
		closedCount++
		if result.Error != "" && len(errorSamples) < 3 {
			errorSamples = append(errorSamples, fmt.Sprintf("%d/tcp: %s", result.Port, result.Error))
		}
	}
	sort.Ints(openPorts)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"target":        target,
		"host":          host,
		"profile":       "safe_tcp_connect",
		"port_profile":  portLabel,
		"scanned_ports": ports,
		"scanned_count": len(ports),
		"open_ports":    openPorts,
		"open_count":    len(openPorts),
		"closed_count":  closedCount,
		"timeout_ms":    timeout.Milliseconds(),
		"error_samples": errorSamples,
		"limitations":   "TCP connect probe only; filtered UDP services and deep service fingerprints are out of scope for this built-in.",
	}
	openText := agentText(language, "未发现开放端口", "No open ports detected")
	if len(openPorts) > 0 {
		parts := make([]string, 0, len(openPorts))
		for _, port := range openPorts {
			parts = append(parts, fmt.Sprintf("%d/tcp", port))
		}
		openText = strings.Join(parts, ", ")
	}
	message := m.message(language, "tool", agentText(
		language,
		fmt.Sprintf("端口探测已完成。\n\n- target: `%s`\n- profile: `safe_tcp_connect`\n- scanned: `%d` ports (%s)\n- open: %s\n\n限制：这是 TCP connect 安全探测，不等同于 nmap 全量服务识别；UDP、被过滤端口和深度指纹需要专用插件。", host, len(ports), portLabel, openText),
		fmt.Sprintf("Port probe completed.\n\n- target: `%s`\n- profile: `safe_tcp_connect`\n- scanned: `%d` ports (%s)\n- open: %s\n\nLimitations: this is a safe TCP connect probe, not full nmap service fingerprinting; UDP, filtered ports, and deep fingerprints need a dedicated plugin.", host, len(ports), portLabel, openText),
	), []agentToolCall{tool}, nil)
	return []agentMessage{plan, message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentHealth(ctx context.Context, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("GET /api/v1/doctor", "daemon")
	version := svc.version.GetVersion(ctx)
	doctor := svc.version.Doctor(ctx)
	summary := summarizeDoctorChecks(doctor.Checks)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"version":                version.Version,
		"ok":                     doctor.Healthy,
		"checks":                 len(doctor.Checks),
		"required_issues":        summary.RequiredIssues,
		"optional_missing":       summary.OptionalMissing,
		"warnings":               summary.Warnings,
		"required_issue_names":   summary.RequiredIssueNames,
		"optional_missing_names": summary.OptionalMissingNames,
		"warning_names":          summary.WarningNames,
	}
	message := m.message(language, "tool", agentText(
		language,
		agentDoctorSummaryText("zh", version.Version, len(doctor.Checks), summary),
		agentDoctorSummaryText("en", version.Version, len(doctor.Checks), summary),
	), []agentToolCall{tool}, []agentLink{{To: "/settings", Label: agentText(language, "连接设置", "Connection"), Kind: "settings"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentPlugins(ctx context.Context, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("GET /api/v1/plugins", "registry")
	plugins, err := svc.plugins.ListPlugins(ctx)
	if err == nil {
		var doctor services.PluginDoctorReport
		doctor, err = svc.plugins.Doctor(ctx)
		tool.Result = map[string]any{"plugins": len(plugins), "ok": doctor.OK, "issues": countUnhealthyPlugins(doctor)}
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	sample := make([]string, 0, 5)
	for _, plugin := range plugins {
		if len(sample) >= 5 {
			break
		}
		sample = append(sample, plugin.ID)
	}
	message := m.message(language, "tool", agentText(
		language,
		fmt.Sprintf("发现 %d 个插件/工具。Agent 会优先使用内置轻量能力，再按策略申请高风险插件。%s", len(plugins), agentSamplePhrase(language, sample)),
		fmt.Sprintf("Discovered %d plugins/tools. The Agent prefers built-in lightweight tools, then requests policy approval for higher-risk plugins. %s", len(plugins), agentSamplePhrase(language, sample)),
	), []agentToolCall{tool}, []agentLink{{To: "/ops", Label: agentText(language, "工具面板", "Tool panel"), Kind: "tools"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentRuns(ctx context.Context, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("GET /api/v1/runs", "runs-dir")
	page, err := svc.runs.ListRuns(ctx, services.RunListRequest{Limit: 10})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{"runs": len(page.Items), "eof": page.EOF}
	lines := make([]string, 0, 4)
	for i, run := range page.Items {
		if i >= 4 {
			break
		}
		lines = append(lines, fmt.Sprintf("%s · %s · %s", run.Status, agentHostLabel(run.Target), shortAgentID(run.RunID, 18)))
	}
	content := agentText(language, "还没有运行记录。你可以直接让我扫描一个授权目标。", "No runs yet. You can ask me to scan an authorized target.")
	if len(lines) > 0 {
		content = agentText(language, fmt.Sprintf("最近 %d 个运行：\n%s", len(page.Items), strings.Join(lines, "\n")), fmt.Sprintf("Recent %d runs:\n%s", len(page.Items), strings.Join(lines, "\n")))
	}
	message := m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/history", Label: agentText(language, "运行历史", "Run history"), Kind: "runs"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentApprovals(ctx context.Context, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("GET /api/v1/runs/*/approvals", "policy")
	page, err := svc.runs.ListRuns(ctx, services.RunListRequest{Limit: 12})
	pending := 0
	firstRun := ""
	checkedRuns := 0
	if err == nil {
		for _, run := range page.Items {
			checkedRuns++
			approvals, listErr := svc.approvals.ListApprovals(ctx, run.RunID)
			if listErr != nil {
				continue
			}
			for _, approval := range approvals {
				if approval.Status == contracts.HITLPending {
					pending++
					if firstRun == "" {
						firstRun = run.RunID
					}
				}
			}
		}
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"pending":      pending,
		"checked_runs": checkedRuns,
		"context":      "approval_queue",
	}
	if firstRun != "" {
		tool.Result["run_id"] = firstRun
	}
	content := agentText(language, "没有待审批动作。高风险或外部工具步骤会在这里等待人工决策。", "No pending approvals. Higher-risk or external-tool steps will wait here for human decisions.")
	links := []agentLink{{To: "/history", Label: agentText(language, "运行历史", "Runs"), Kind: "runs"}}
	if pending > 0 {
		content = agentText(language, fmt.Sprintf("发现 %d 个待审批动作。请进入运行详情查看风险摘要并 approve/deny。", pending), fmt.Sprintf("Found %d pending approval actions. Open the run detail to review risk summary and approve/deny.", pending))
		links = []agentLink{{To: "/scan/" + firstRun, Label: agentText(language, "处理审批", "Review"), Kind: "approval"}}
	}
	message := m.message(language, "tool", content, []agentToolCall{tool}, links)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentResumeApproved(ctx context.Context, sessionID, prompt, language, operator string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	runID := agentExtractRunID(prompt)
	if runID == "" {
		if session, err := m.GetSession(ctx, sessionID); err == nil && session.ActiveTask != nil {
			runID = strings.TrimSpace(session.ActiveTask.ActiveRunID)
		}
	}
	if runID == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要 run_id 或当前 active run 才能恢复已批准步骤。", "I need a run_id or current active run to resume approved steps."), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("POST /api/v1/runs/{run_id}/resume-approved", shortAgentID(runID, 22))
	result, err := svc.approvals.ResumeApproved(ctx, services.ResumeApprovedRequest{
		RunIDOrPath: runID,
		Operator:    operator,
	})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{
			"run_id":  runID,
			"context": "resume_approved",
		}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"run_id":        result.RunID,
		"run_dir":       result.RunDir,
		"resumed":       result.Resumed,
		"skipped":       result.Skipped,
		"resumed_count": len(result.Resumed),
		"skipped_count": len(result.Skipped),
		"context":       "resume_approved",
	}

	content := agentText(language,
		fmt.Sprintf("已执行恢复已批准步骤：run=%s，resumed=%d，skipped=%d。Agent 将继续跟踪运行事件、证据和报告。", result.RunID, len(result.Resumed), len(result.Skipped)),
		fmt.Sprintf("Resume-approved completed: run=%s, resumed=%d, skipped=%d. The Agent will continue tracking events, evidence and report.", result.RunID, len(result.Resumed), len(result.Skipped)),
	)
	message := m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/scan/" + result.RunID, Label: agentText(language, "打开运行", "Open run"), Kind: "run"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentApprovalDecision(ctx context.Context, sessionID, prompt, language, operator string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	action := agentApprovalDecisionAction(prompt)
	if action == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我识别到审批相关请求，但没有明确是批准还是拒绝。请说明“批准 approval_xxx”或“拒绝 approval_xxx”。", "I detected an approval request, but it is not clear whether to approve or deny. Say `approve approval_xxx` or `deny approval_xxx`."), nil, nil)}, nil, nil
	}
	target, targetErr := m.resolveAgentApprovalDecisionTarget(ctx, sessionID, prompt, svc)
	if targetErr != nil {
		return []agentMessage{m.message(language, "agent", targetErr.Error(), nil, nil)}, nil, nil
	}

	comment := agentExtractApprovalComment(prompt)
	toolName := fmt.Sprintf("POST /api/v1/runs/{run_id}/approvals/{approval_id}/%s", action)
	tool := runningAgentToolCall(toolName, shortAgentID(target.ApprovalID, 22))
	req := services.ApprovalDecisionRequest{
		RunIDOrPath:        target.RunID,
		ApprovalIDOrStepID: target.ApprovalID,
		Operator:           operator,
		Comment:            comment,
	}
	var (
		resolved contracts.HITLRequest
		err      error
	)
	switch action {
	case "approve":
		resolved, err = svc.approvals.Approve(ctx, req)
	case "deny":
		resolved, err = svc.approvals.Deny(ctx, req)
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{
			"run_id":      target.RunID,
			"approval_id": target.ApprovalID,
			"action":      action,
			"context":     "approval_decision",
		}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	pending := 0
	if approvals, listErr := svc.approvals.ListApprovals(ctx, target.RunID); listErr == nil {
		pending = agentPendingApprovalCount(approvals)
	}
	tool.Result = map[string]any{
		"run_id":           target.RunID,
		"approval_id":      resolved.ID,
		"step_id":          resolved.StepID,
		"requested_action": resolved.RequestedAction,
		"status":           string(resolved.Status),
		"action":           action,
		"pending":          pending,
		"context":          "approval_decision",
	}
	if comment != "" {
		tool.Result["comment"] = comment
	}

	decisionText := agentText(language, "已批准", "Approved")
	if action == "deny" {
		decisionText = agentText(language, "已拒绝", "Denied")
	}
	content := agentText(language,
		fmt.Sprintf("%s审批 %s（step=%s，run=%s）。剩余待审批：%d。", decisionText, resolved.ID, emptyDash(resolved.StepID), target.RunID, pending),
		fmt.Sprintf("%s approval %s (step=%s, run=%s). Pending approvals left: %d.", decisionText, resolved.ID, emptyDash(resolved.StepID), target.RunID, pending),
	)
	if pending == 0 {
		content += agentText(language, " Agent 已刷新审批 gate，可继续跟踪运行或恢复已批准步骤。", " The Agent refreshed the approval gate; continue tracking the run or resume approved steps.")
	}
	message := m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/scan/" + target.RunID, Label: agentText(language, "打开运行", "Open run"), Kind: "run"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}
