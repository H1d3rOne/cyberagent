package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

func (m *agentManager) syncTerminalActiveRunsLocked(ctx context.Context) {
	for _, session := range m.sessions {
		if m.syncTerminalActiveRunLocked(ctx, session) {
			_ = m.persistSessionLocked(session)
		}
	}
}

func (m *agentManager) syncTerminalActiveRunLocked(ctx context.Context, session *agentSession) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
	runID := strings.TrimSpace(session.ActiveTask.ActiveRunID)
	if runID == "" {
		return false
	}
	detail, err := m.runs.GetRun(ctx, runID)
	if err != nil {
		return false
	}
	changed := m.refreshActiveRunContextSnapshotLocked(session, detail)
	if m.refreshApprovalGateForSessionLocked(ctx, session) {
		return true
	}
	status := detail.Summary.Status
	if status == "" && detail.Job != nil {
		status = detail.Job.Status
	}
	if !agentRunTerminalStatus(status) {
		return changed
	}
	if session.Status == "paused" || session.Status == "cancelled" {
		return changed
	}
	switch status {
	case contracts.StatusSucceeded, contracts.StatusPartiallySucceeded:
		return normalizeCompletedActiveRunSession(session, status, detail) || changed
	case contracts.StatusFailed, contracts.StatusCancelled:
		return normalizeStoppedActiveRunSession(session, status) || changed
	default:
		return changed
	}
}

func (m *agentManager) refreshActiveRunContextSnapshotLocked(session *agentSession, detail services.RunDetail) bool {
	if session == nil || session.ActiveTask == nil || strings.TrimSpace(detail.RunID) == "" {
		return false
	}
	runID := detail.RunID
	reportSnapshot := m.activeRunReportSnapshot(context.Background(), runID, detail)
	summary := agentRunContextSummary(session.Language, detail, reportSnapshot)
	changed := false
	changed = m.addContextRefLocked(session, agentContextRef{
		ID:      "run:" + runID,
		Kind:    "run",
		RefID:   runID,
		Label:   "run " + shortAgentID(runID, 18),
		To:      "/scan/" + runID,
		Summary: summary,
	}) || changed
	if detail.Summary.Report.JSON {
		changed = m.addContextRefLocked(session, agentContextRef{
			ID:      "report_json:" + runID,
			Kind:    "report_json",
			RefID:   runID,
			Label:   "report " + shortAgentID(runID, 18),
			To:      "/scan/" + runID,
			Summary: agentReportContextSummary(session.Language, detail, reportSnapshot),
		}) || changed
	}
	if detail.Summary.Counts.Artifacts > 0 {
		changed = m.addContextRefLocked(session, agentContextRef{
			ID:      "artifact_index:" + runID,
			Kind:    "artifact_index",
			RefID:   runID,
			Label:   "artifacts " + shortAgentID(runID, 18),
			To:      "/scan/" + runID,
			Summary: agentArtifactContextSummary(session.Language, detail, reportSnapshot),
		}) || changed
	}
	if detail.Summary.Counts.Findings > 0 {
		changed = m.addContextRefLocked(session, agentContextRef{
			ID:      "finding:" + runID,
			Kind:    "finding",
			RefID:   runID,
			Label:   agentText(session.Language, fmt.Sprintf("%d 个发现", detail.Summary.Counts.Findings), fmt.Sprintf("%d findings", detail.Summary.Counts.Findings)),
			To:      "/scan/" + runID,
			Summary: agentFindingContextSummary(session.Language, detail, reportSnapshot),
		}) || changed
	}
	if detail.Summary.Counts.Approvals > 0 {
		pending := detail.Summary.Counts.PendingApprovals
		changed = m.addContextRefLocked(session, agentContextRef{
			ID:      "approval:" + runID,
			Kind:    "approval",
			RefID:   runID,
			Label:   agentText(session.Language, fmt.Sprintf("%d/%d 待审批", pending, detail.Summary.Counts.Approvals), fmt.Sprintf("%d/%d pending approvals", pending, detail.Summary.Counts.Approvals)),
			To:      "/scan/" + runID,
			Summary: agentText(session.Language, "运行中的人工审批队列", "Human approval queue for this run"),
		}) || changed
	}
	if summary != "" && session.ActiveTask.Summary != summary && shouldRefreshActiveRunSummary(session.ActiveTask.Summary) {
		session.ActiveTask.Summary = summary
		session.ActiveTask.UpdatedAt = time.Now().UTC()
		session.UpdatedAt = session.ActiveTask.UpdatedAt
		changed = true
	}
	return changed
}

func (m *agentManager) activeRunReportSnapshot(ctx context.Context, runID string, detail services.RunDetail) agentRunReportSnapshot {
	snapshot := agentRunReportSnapshot{}
	if detail.Summary.Report.JSON {
		if report, err := m.runs.GetReport(ctx, runID, "json"); err == nil {
			snapshot = parseAgentRunReportSnapshot(report.Bytes)
		}
	}
	if len(snapshot.Artifacts) == 0 && detail.Summary.Counts.Artifacts > 0 {
		if index, err := m.runs.GetArtifactIndex(ctx, runID); err == nil {
			for _, artifact := range index.Artifacts {
				snapshot.Artifacts = append(snapshot.Artifacts, agentRunArtifactSnapshot{
					ID:       artifact.ID,
					Type:     string(artifact.Type),
					Path:     artifact.Path,
					MIMEType: artifact.MIMEType,
				})
				if len(snapshot.Artifacts) >= 3 {
					break
				}
			}
		}
	}
	return snapshot
}

func parseAgentRunReportSnapshot(data []byte) agentRunReportSnapshot {
	var raw struct {
		Findings  []agentRunFindingSnapshot  `json:"findings"`
		Evidence  []agentRunEvidenceSnapshot `json:"evidence"`
		Artifacts []agentRunArtifactSnapshot `json:"artifacts"`
	}
	if len(data) == 0 || json.Unmarshal(data, &raw) != nil {
		return agentRunReportSnapshot{}
	}
	snapshot := agentRunReportSnapshot{}
	for _, finding := range raw.Findings {
		finding.Title = trimSummary(finding.Title, 120)
		finding.Category = trimSummary(finding.Category, 80)
		finding.Impact = trimSummary(finding.Impact, 120)
		finding.Remediation = trimSummary(finding.Remediation, 140)
		if len(finding.EvidenceIDs) > 3 {
			finding.EvidenceIDs = append([]string(nil), finding.EvidenceIDs[:3]...)
		}
		snapshot.Findings = append(snapshot.Findings, finding)
		if len(snapshot.Findings) >= 3 {
			break
		}
	}
	for _, evidence := range raw.Evidence {
		evidence.Title = trimSummary(evidence.Title, 100)
		evidence.Summary = trimSummary(evidence.Summary, 160)
		snapshot.Evidence = append(snapshot.Evidence, evidence)
		if len(snapshot.Evidence) >= 3 {
			break
		}
	}
	for _, artifact := range raw.Artifacts {
		artifact.Path = trimSummary(artifact.Path, 140)
		artifact.MIMEType = trimSummary(artifact.MIMEType, 80)
		snapshot.Artifacts = append(snapshot.Artifacts, artifact)
		if len(snapshot.Artifacts) >= 3 {
			break
		}
	}
	return snapshot
}

func agentReportDraftPath(runID, outputPath string) string {
	outputPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(outputPath)))
	if outputPath == "" || outputPath == "." || strings.HasPrefix(outputPath, "../") || filepath.IsAbs(outputPath) {
		return filepath.ToSlash(filepath.Join("reports", agentCleanToken(runID)+"-draft.md"))
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".md") {
		outputPath += ".md"
	}
	return outputPath
}

func agentBuildReportDraftMarkdown(language string, detail services.RunDetail, snapshot agentRunReportSnapshot, generatedAt time.Time) string {
	target := firstNonEmpty(detail.Summary.Target, detail.Summary.Scope)
	status := string(detail.Summary.Status)
	if status == "" && detail.Job != nil {
		status = string(detail.Job.Status)
	}
	var b strings.Builder
	zh := normalizeAgentLanguage(language) == "zh"
	if zh {
		fmt.Fprintf(&b, "# CyberAgent 报告草稿\n\n")
		fmt.Fprintf(&b, "> 草稿由 Agent 基于运行时报告和上下文生成，需要人工复核后再对外输出。\n\n")
		fmt.Fprintf(&b, "## 概览\n\n")
	} else {
		fmt.Fprintf(&b, "# CyberAgent Report Draft\n\n")
		fmt.Fprintf(&b, "> Draft generated from runtime report/context. Review manually before external use.\n\n")
		fmt.Fprintf(&b, "## Overview\n\n")
	}
	fmt.Fprintf(&b, "- Run ID: `%s`\n", detail.RunID)
	fmt.Fprintf(&b, "- Target: `%s`\n", emptyDash(target))
	fmt.Fprintf(&b, "- Scope: `%s`\n", emptyDash(detail.Summary.Scope))
	fmt.Fprintf(&b, "- Profile: `%s`\n", emptyDash(detail.Summary.Profile))
	fmt.Fprintf(&b, "- Status: `%s`\n", emptyDash(status))
	fmt.Fprintf(&b, "- Findings: %d\n", len(snapshot.Findings))
	fmt.Fprintf(&b, "- Evidence: %d\n", len(snapshot.Evidence))
	fmt.Fprintf(&b, "- Artifacts: %d\n", len(snapshot.Artifacts))
	fmt.Fprintf(&b, "- Generated: %s\n\n", generatedAt.Format(time.RFC3339))
	if zh {
		fmt.Fprintf(&b, "## 执行摘要\n\n")
		if len(snapshot.Findings) == 0 {
			fmt.Fprintf(&b, "本次运行暂未在 report.json 中发现 finding。仍建议结合事件、artifact 和授权范围进行人工复核。\n\n")
		} else {
			fmt.Fprintf(&b, "本次运行发现 %d 个 finding。优先按严重度、可复现证据和业务暴露面进行确认，并在修复后对同一 scope 执行复测。\n\n", len(snapshot.Findings))
		}
	} else {
		fmt.Fprintf(&b, "## Executive Summary\n\n")
		if len(snapshot.Findings) == 0 {
			fmt.Fprintf(&b, "No findings were present in report.json. Review events, artifacts, and scope before closing the task.\n\n")
		} else {
			fmt.Fprintf(&b, "The run produced %d finding(s). Prioritize by severity, reproducible evidence, and exposed business surface; retest the same scope after remediation.\n\n", len(snapshot.Findings))
		}
	}
	fmt.Fprintf(&b, "## Findings\n\n")
	agentAppendReportDraftFindings(&b, snapshot, language)
	if zh {
		fmt.Fprintf(&b, "\n## Evidence 摘要\n\n")
	} else {
		fmt.Fprintf(&b, "\n## Evidence Summary\n\n")
	}
	agentAppendReportDraftEvidence(&b, snapshot, language)
	if zh {
		fmt.Fprintf(&b, "\n## Artifacts 摘要\n\n")
	} else {
		fmt.Fprintf(&b, "\n## Artifact Summary\n\n")
	}
	agentAppendReportDraftArtifacts(&b, snapshot, language)
	if zh {
		fmt.Fprintf(&b, "\n## 修复与复测建议\n\n")
		fmt.Fprintf(&b, "1. 逐条核对 finding 的 evidence_ids 与原始 artifact，确认不是代理、CDN、开发服务器或测试夹具导致的误报。\n")
		fmt.Fprintf(&b, "2. 按 finding 的 remediation 或业务配置修复。\n")
		fmt.Fprintf(&b, "3. 用最小复现命令验证修复，再让 Agent 对相同 scope 重跑 safe profile。\n")
		fmt.Fprintf(&b, "4. 将复测结果追加到本草稿的 Evidence / Retest Notes 中。\n")
	} else {
		fmt.Fprintf(&b, "\n## Remediation and Retest\n\n")
		fmt.Fprintf(&b, "1. Correlate each finding's evidence_ids with raw artifacts and rule out proxy/CDN/dev-server/test-fixture false positives.\n")
		fmt.Fprintf(&b, "2. Apply each finding's remediation or the equivalent business configuration change.\n")
		fmt.Fprintf(&b, "3. Verify with the minimal reproduction command, then rerun the safe profile for the same scope.\n")
		fmt.Fprintf(&b, "4. Append retest results to this draft's Evidence / Retest Notes.\n")
	}
	return b.String()
}

func agentAppendReportDraftFindings(b *strings.Builder, snapshot agentRunReportSnapshot, language string) {
	if len(snapshot.Findings) == 0 {
		fmt.Fprintf(b, "%s\n", agentText(language, "暂无 finding。", "No findings."))
		return
	}
	for idx, finding := range snapshot.Findings {
		title := firstNonEmpty(finding.Title, finding.ID, fmt.Sprintf("Finding %d", idx+1))
		fmt.Fprintf(b, "### %d. %s\n\n", idx+1, title)
		fmt.Fprintf(b, "- Severity: `%s`\n", emptyDash(string(finding.Severity)))
		fmt.Fprintf(b, "- Category: `%s`\n", emptyDash(finding.Category))
		if len(finding.EvidenceIDs) > 0 {
			fmt.Fprintf(b, "- Evidence IDs: `%s`\n", strings.Join(finding.EvidenceIDs, "`, `"))
		}
		if finding.Impact != "" {
			fmt.Fprintf(b, "- Impact: %s\n", finding.Impact)
		}
		if finding.Remediation != "" {
			fmt.Fprintf(b, "- Remediation: %s\n", finding.Remediation)
		}
		fmt.Fprintf(b, "\n")
	}
}

func agentAppendReportDraftEvidence(b *strings.Builder, snapshot agentRunReportSnapshot, language string) {
	if len(snapshot.Evidence) == 0 {
		fmt.Fprintf(b, "%s\n", agentText(language, "暂无 evidence 摘要。", "No evidence summary."))
		return
	}
	for _, evidence := range snapshot.Evidence {
		fmt.Fprintf(b, "- `%s` · %s · %s", emptyDash(evidence.ID), emptyDash(string(evidence.Type)), emptyDash(evidence.Title))
		if evidence.Summary != "" {
			fmt.Fprintf(b, " — %s", evidence.Summary)
		}
		if evidence.ArtifactID != "" {
			fmt.Fprintf(b, " (artifact `%s`)", evidence.ArtifactID)
		}
		fmt.Fprintf(b, "\n")
	}
}

func agentAppendReportDraftArtifacts(b *strings.Builder, snapshot agentRunReportSnapshot, language string) {
	if len(snapshot.Artifacts) == 0 {
		fmt.Fprintf(b, "%s\n", agentText(language, "暂无 artifact 摘要。", "No artifact summary."))
		return
	}
	for _, artifact := range snapshot.Artifacts {
		fmt.Fprintf(b, "- `%s` · %s · `%s`", emptyDash(artifact.ID), emptyDash(string(artifact.Type)), emptyDash(artifact.Path))
		if artifact.MIMEType != "" {
			fmt.Fprintf(b, " · %s", artifact.MIMEType)
		}
		fmt.Fprintf(b, "\n")
	}
}

func shouldRefreshActiveRunSummary(current string) bool {
	current = strings.TrimSpace(current)
	if current == "" {
		return true
	}
	lower := strings.ToLower(current)
	return strings.Contains(current, "正在跟踪运行") ||
		strings.Contains(lower, "tracking context for run") ||
		strings.Contains(current, "跟踪运行事件、证据和审批") ||
		strings.Contains(lower, "track run events")
}

func agentRunContextSummary(language string, detail services.RunDetail, snapshot agentRunReportSnapshot) string {
	status := detail.Summary.Status
	if status == "" && detail.Job != nil {
		status = detail.Job.Status
	}
	target := detail.Summary.Target
	if target == "" && detail.Job != nil {
		target = detail.Job.TargetID
	}
	if target == "" {
		target = "-"
	}
	counts := detail.Summary.Counts
	leadFinding := agentLeadFindingText(snapshot)
	leadEvidence := agentLeadEvidenceText(snapshot)
	extraZH := ""
	extraEN := ""
	if leadFinding != "" {
		extraZH += " 关键发现：" + leadFinding + "。"
		extraEN += " Key finding: " + leadFinding + "."
	}
	if leadEvidence != "" {
		extraZH += " 关键证据：" + leadEvidence + "。"
		extraEN += " Key evidence: " + leadEvidence + "."
	}
	return trimSummary(agentText(language,
		fmt.Sprintf("运行 %s · %s · target=%s · findings=%d · artifacts=%d · pending_approvals=%d。%s", shortAgentID(detail.RunID, 18), emptyDash(string(status)), target, counts.Findings, counts.Artifacts, counts.PendingApprovals, extraZH),
		fmt.Sprintf("Run %s · %s · target=%s · findings=%d · artifacts=%d · pending_approvals=%d.%s", shortAgentID(detail.RunID, 18), emptyDash(string(status)), target, counts.Findings, counts.Artifacts, counts.PendingApprovals, extraEN),
	), 360)
}

func agentReportContextSummary(language string, detail services.RunDetail, snapshot agentRunReportSnapshot) string {
	leadFinding := agentLeadFindingText(snapshot)
	leadEvidence := agentLeadEvidenceText(snapshot)
	if leadFinding != "" || leadEvidence != "" {
		return trimSummary(agentText(language,
			fmt.Sprintf("%d 个发现，%d 条证据。%s%s", detail.Summary.Counts.Findings, len(snapshot.Evidence), prefixIfNotEmpty("关键发现：", leadFinding), prefixIfNotEmpty(" 关键证据：", leadEvidence)),
			fmt.Sprintf("%d findings, %d evidence items. %s%s", detail.Summary.Counts.Findings, len(snapshot.Evidence), prefixIfNotEmpty("Key finding: ", leadFinding), prefixIfNotEmpty(" Key evidence: ", leadEvidence)),
		), 220)
	}
	return agentText(language, fmt.Sprintf("%d findings · %d evidence artifacts", detail.Summary.Counts.Findings, detail.Summary.Counts.Artifacts), fmt.Sprintf("%d findings · %d evidence artifacts", detail.Summary.Counts.Findings, detail.Summary.Counts.Artifacts))
}

func agentFindingContextSummary(language string, detail services.RunDetail, snapshot agentRunReportSnapshot) string {
	leadFinding := agentLeadFindingText(snapshot)
	if leadFinding != "" {
		return trimSummary(agentText(language,
			"关键发现："+leadFinding+"。建议按影响、证据、误报、修复、复测处理。",
			"Key finding: "+leadFinding+". Triage as impact, evidence, false positive, remediation, retest.",
		), 220)
	}
	_ = detail
	return agentText(language, "来自当前运行报告，建议按影响、证据、误报、修复、复测处理。", "From the active run report; triage as impact, evidence, false positive, remediation, retest.")
}

func agentArtifactContextSummary(language string, detail services.RunDetail, snapshot agentRunReportSnapshot) string {
	leadArtifact := agentLeadArtifactText(snapshot)
	leadEvidence := agentLeadEvidenceText(snapshot)
	if leadArtifact != "" || leadEvidence != "" {
		return trimSummary(agentText(language,
			fmt.Sprintf("%d 个 artifact。%s%s", detail.Summary.Counts.Artifacts, prefixIfNotEmpty("关键 artifact：", leadArtifact), prefixIfNotEmpty(" 关联证据：", leadEvidence)),
			fmt.Sprintf("%d artifacts. %s%s", detail.Summary.Counts.Artifacts, prefixIfNotEmpty("Key artifact: ", leadArtifact), prefixIfNotEmpty(" Related evidence: ", leadEvidence)),
		), 220)
	}
	return agentText(language, fmt.Sprintf("%d 个证据 artifact", detail.Summary.Counts.Artifacts), fmt.Sprintf("%d artifact(s)", detail.Summary.Counts.Artifacts))
}

func agentLeadFindingText(snapshot agentRunReportSnapshot) string {
	if len(snapshot.Findings) == 0 {
		return ""
	}
	finding := snapshot.Findings[0]
	parts := []string{}
	if finding.Severity != "" {
		parts = append(parts, strings.ToUpper(finding.Severity))
	}
	if finding.Title != "" {
		parts = append(parts, finding.Title)
	}
	if finding.Category != "" {
		parts = append(parts, "category="+finding.Category)
	}
	if finding.Remediation != "" {
		parts = append(parts, "fix="+finding.Remediation)
	}
	return trimSummary(strings.Join(parts, " · "), 180)
}

func agentLeadEvidenceText(snapshot agentRunReportSnapshot) string {
	if len(snapshot.Evidence) == 0 {
		return ""
	}
	evidence := snapshot.Evidence[0]
	parts := []string{}
	if evidence.ID != "" {
		parts = append(parts, evidence.ID)
	}
	if evidence.Type != "" {
		parts = append(parts, string(evidence.Type))
	}
	if evidence.Title != "" {
		parts = append(parts, evidence.Title)
	}
	if evidence.Summary != "" {
		parts = append(parts, evidence.Summary)
	}
	return trimSummary(strings.Join(parts, " · "), 180)
}

func agentLeadArtifactText(snapshot agentRunReportSnapshot) string {
	if len(snapshot.Artifacts) == 0 {
		return ""
	}
	artifact := snapshot.Artifacts[0]
	parts := []string{}
	if artifact.ID != "" {
		parts = append(parts, artifact.ID)
	}
	if artifact.Type != "" {
		parts = append(parts, artifact.Type)
	}
	if artifact.Path != "" {
		parts = append(parts, artifact.Path)
	}
	return trimSummary(strings.Join(parts, " · "), 160)
}

func prefixIfNotEmpty(prefix, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + value
}

func agentRunTerminalStatus(status contracts.Status) bool {
	switch status {
	case contracts.StatusSucceeded, contracts.StatusPartiallySucceeded, contracts.StatusFailed, contracts.StatusCancelled:
		return true
	default:
		return false
	}
}

func normalizeCompletedActiveRunSession(session *agentSession, status contracts.Status, detail services.RunDetail) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
	runID := strings.TrimSpace(session.ActiveTask.ActiveRunID)
	if sessionHasRunEvidenceActionTodos(session) {
		now := time.Now().UTC()
		session.Status = "running"
		session.ActiveTask.Status = "running"
		session.ActiveTask.Summary = trimSummary(agentText(session.Language,
			"运行 "+session.ActiveTask.ActiveRunID+" 已结束（"+string(status)+"），报告发现已转入误报检查、修复和复测行动计划。",
			"Run "+session.ActiveTask.ActiveRunID+" has finished ("+string(status)+"); findings are now in false-positive check, remediation and retest action planning.",
		), 360)
		session.ActiveTask.UpdatedAt = now
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          "active run reached terminal status and follow-up evidence action todos remain",
			SuggestedPrompt: agentSuggestedEvidenceActionPrompt(session.Language, session.ActiveTask.ActiveRunID),
			AutoContinue:    true,
		}
		session.UpdatedAt = now
		return true
	}
	needsFix := session.Status == "running" || session.ActiveTask.Status == "running"
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
			markAgentTodoByIndex(session.Todos, idx, "done", now, "", "")
		}
	}
	session.Status = "idle"
	session.ActiveTask.Status = "idle"
	session.ActiveTask.Summary = trimSummary(agentText(session.Language,
		"运行 "+session.ActiveTask.ActiveRunID+" 已结束（"+string(status)+"），等待用户要求总结证据或继续分析。",
		"Run "+session.ActiveTask.ActiveRunID+" has finished ("+string(status)+"); waiting for the operator to request evidence summary or follow-up analysis.",
	), 360)
	session.ActiveTask.UpdatedAt = now
	session.NextAction = &agentNextAction{
		Type:            "wait_user",
		Reason:          "active run reached terminal status: " + string(status),
		SuggestedPrompt: agentSuggestedRunSummaryPrompt(session.Language, session.ActiveTask.ActiveRunID),
	}
	if runID != "" && !sessionHasTerminalRunSummaryMessage(session, runID) {
		session.Messages = append(session.Messages, agentSyntheticRunSummaryMessage(session.Language, runID, status, detail))
	}
	return true
}

func sessionHasTerminalRunSummaryMessage(session *agentSession, runID string) bool {
	if session == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	for _, message := range session.Messages {
		if message.Role == "agent" && message.Phase == "final_answer" && strings.Contains(message.Content, runID) {
			return true
		}
	}
	return false
}

func agentSyntheticRunSummaryMessage(language, runID string, status contracts.Status, detail services.RunDetail) agentMessage {
	now := time.Now().UTC()
	counts := detail.Summary.Counts
	target := firstNonEmpty(detail.Summary.Target, detail.Summary.Scope)
	if target == "" && detail.Job != nil {
		target = detail.Job.TargetID
	}
	reportText := agentText(language, "报告尚未生成", "report pending")
	if detail.Summary.Report.JSON {
		reportText = agentText(language, "报告已生成", "report ready")
	}
	content := agentText(language,
		fmt.Sprintf("任务已完成：运行 `%s` 状态为 `%s`。\n\n- 目标：%s\n- 事件：%d\n- 证据/Artifact：%d\n- 发现：%d\n- 审批：%d（待处理 %d）\n- 报告：%s\n\n下一步建议：发送 `%s`，我会读取报告/事件并给出证据链、风险摘要和复测建议。", runID, status, emptyDash(target), counts.Events, counts.Artifacts, counts.Findings, counts.Approvals, counts.PendingApprovals, reportText, agentSuggestedRunSummaryPrompt(language, runID)),
		fmt.Sprintf("Task completed: run `%s` status is `%s`.\n\n- Target: %s\n- Events: %d\n- Artifacts: %d\n- Findings: %d\n- Approvals: %d (pending %d)\n- Report: %s\n\nSuggested next step: send `%s`; I will read the report/events and summarize evidence chains, risk, and retest guidance.", runID, status, emptyDash(target), counts.Events, counts.Artifacts, counts.Findings, counts.Approvals, counts.PendingApprovals, reportText, agentSuggestedRunSummaryPrompt(language, runID)),
	)
	return agentMessage{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "msg_", 1),
		Role:      "agent",
		Content:   content,
		Timestamp: now,
		Phase:     "final_answer",
		Links:     []agentLink{{To: "/scan/" + runID, Label: agentText(language, "打开运行详情", "Open run detail"), Kind: "run"}},
	}
}

func normalizeStoppedActiveRunSession(session *agentSession, status contracts.Status) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
	if session.Status == "blocked" && session.ActiveTask.Status == "blocked" && session.NextAction != nil && session.NextAction.Type == "blocked" {
		return false
	}
	now := time.Now().UTC()
	reason := "active run stopped with terminal status: " + string(status)
	for idx := range session.Todos {
		if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
			markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
		}
	}
	session.Status = "blocked"
	session.ActiveTask.Status = "blocked"
	session.ActiveTask.Summary = trimSummary(agentText(session.Language,
		"运行 "+session.ActiveTask.ActiveRunID+" 已停止（"+string(status)+"），需要分析失败/取消原因后再继续。",
		"Run "+session.ActiveTask.ActiveRunID+" stopped ("+string(status)+"); analyze the failure/cancellation before continuing.",
	), 360)
	session.ActiveTask.UpdatedAt = now
	session.NextAction = &agentNextAction{
		Type:            "blocked",
		Reason:          reason,
		SuggestedPrompt: agentSuggestedStoppedRunPrompt(session.Language, session.ActiveTask.ActiveRunID),
	}
	return true
}
