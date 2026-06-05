package daemon

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

func agentHeuristicActionsFromPrompt(prompt string) []agentPlannerAction {
	switch {
	case agentIsScheduleCreateIntent(prompt):
		return []agentPlannerAction{{Name: "schedule.create", Arguments: agentScheduleArgsFromPrompt(prompt, "")}}
	case agentIsScheduleListIntent(prompt):
		return []agentPlannerAction{{Name: "schedule.list", Arguments: map[string]any{}}}
	case agentIsScheduleDeleteIntent(prompt):
		return []agentPlannerAction{{Name: "schedule.delete", Arguments: map[string]any{"id": agentExtractScheduleID(prompt)}}}
	case agentIsPortScanIntent(prompt):
		return agentPortScanActionsFromPrompt(prompt)
	case agentHasCapabilityGapIntent(prompt):
		return nil
	case agentIsScanIntent(prompt):
		return agentHeuristicScanActionsFromPrompt(prompt)
	case agentIsResumeApprovedIntent(prompt):
		return []agentPlannerAction{{Name: "runs.resume_approved", Arguments: map[string]any{"run_id": agentExtractRunID(prompt)}}}
	case agentIsReportDraftIntent(prompt):
		return []agentPlannerAction{{Name: "reports.draft", Arguments: map[string]any{"run_id": agentExtractRunID(prompt)}}}
	case agentIsApprovalDecisionIntent(prompt):
		action := "runs.approvals.approve"
		if agentApprovalDecisionAction(prompt) == "deny" {
			action = "runs.approvals.deny"
		}
		return []agentPlannerAction{{Name: action, Arguments: map[string]any{
			"run_id":      agentExtractRunID(prompt),
			"approval_id": agentExtractApprovalID(prompt),
		}}}
	case agentIsRunContextAnalysisIntent(prompt):
		return []agentPlannerAction{{Name: "runs.context", Arguments: map[string]any{
			"run_id":  agentExtractRunID(prompt),
			"context": agentRunContextKind(prompt),
		}}}
	case agentIsCommandIntent(prompt):
		cmd := ""
		if commands := agentExtractCommands(prompt); len(commands) > 0 {
			cmd = strings.Join(commands, " && ")
		}
		if cmd == "" {
			cmd = agentExtractCommand(prompt)
		}
		if cmd == "" {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.command", Arguments: map[string]any{"command": cmd}}}
	case agentIsWorkspaceRebaseIntent(prompt):
		oldText, newText := agentExtractWorkspaceRebaseTexts(prompt)
		return []agentPlannerAction{{Name: "workspace.files.rebase", Arguments: map[string]any{
			"change_id": agentExtractWorkspaceChangeID(prompt),
			"old":       oldText,
			"new":       newText,
		}}}
	case agentIsWorkspaceDiscardAllIntent(prompt):
		return []agentPlannerAction{{Name: "workspace.files.discard", Arguments: map[string]any{"all": true}}}
	case agentIsWorkspaceDiscardIntent(prompt):
		args := map[string]any{"change_id": agentExtractWorkspaceChangeID(prompt)}
		if groupID := agentExtractWorkspaceGroupID(prompt); groupID != "" {
			args["group_id"] = groupID
		}
		return []agentPlannerAction{{Name: "workspace.files.discard", Arguments: args}}
	case agentIsWorkspaceApplyAllIntent(prompt):
		return []agentPlannerAction{{Name: "workspace.files.apply", Arguments: map[string]any{"all": true}}}
	case agentIsWorkspaceApplyIntent(prompt):
		args := map[string]any{"change_id": agentExtractWorkspaceChangeID(prompt)}
		if groupID := agentExtractWorkspaceGroupID(prompt); groupID != "" {
			args["group_id"] = groupID
		}
		return []agentPlannerAction{{Name: "workspace.files.apply", Arguments: args}}
	case agentIsWorkspaceDeleteIntent(prompt):
		path := agentExtractPath(prompt)
		if path == "" || path == "." {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.files.delete", Arguments: map[string]any{"path": path}}}
	case agentIsWorkspaceMoveIntent(prompt):
		fromPath, toPath := agentExtractWorkspaceMovePaths(prompt)
		if fromPath == "" || fromPath == "." || toPath == "" || toPath == "." {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.files.move", Arguments: map[string]any{"from": fromPath, "to": toPath}}}
	case agentIsWorkspaceCopyIntent(prompt):
		fromPath, toPath := agentExtractWorkspaceMovePaths(prompt)
		if fromPath == "" || fromPath == "." || toPath == "" || toPath == "." {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.files.copy", Arguments: map[string]any{"from": fromPath, "to": toPath}}}
	case agentIsWorkspaceMkdirIntent(prompt):
		path := agentExtractWorkspaceMkdirPath(prompt)
		if path == "" || path == "." {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.files.mkdir", Arguments: map[string]any{"path": path}}}
	case agentIsWorkspaceDiffIntent(prompt):
		relPath, oldText, newText := agentExtractWorkspacePatch(prompt)
		if relPath == "" || relPath == "." {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.files.diff", Arguments: map[string]any{"path": relPath, "old": oldText, "new": newText}}}
	case agentIsWorkspacePatchIntent(prompt):
		relPath, oldText, newText := agentExtractWorkspacePatch(prompt)
		if relPath == "" || relPath == "." {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.files.patch", Arguments: map[string]any{"path": relPath, "old": oldText, "new": newText}}}
	case agentIsWorkspaceWriteIntent(prompt):
		relPath, content := agentExtractWorkspaceWrite(prompt)
		if relPath == "" || relPath == "." {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.files.write", Arguments: map[string]any{"path": relPath, "content": content}}}
	case agentIsApprovalIntent(prompt):
		return []agentPlannerAction{{Name: "runs.approvals.list", Arguments: map[string]any{"run_id": agentExtractRunID(prompt)}}}
	case agentIsHealthIntent(prompt):
		return []agentPlannerAction{{Name: "daemon.health", Arguments: map[string]any{}}}
	case agentIsPluginIntent(prompt):
		return []agentPlannerAction{{Name: "plugins.list", Arguments: map[string]any{}}}
	case agentIsRunIntent(prompt):
		return []agentPlannerAction{{Name: "runs.list", Arguments: map[string]any{}}}
	case agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt):
		return agentHTTPProbeActionsFromPrompt(prompt)
	case agentIsWorkspaceSearchIntent(prompt):
		query := agentExtractWorkspaceSearchQuery(prompt)
		if query == "" {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.search", Arguments: map[string]any{
			"query": query,
			"path":  agentExtractWorkspaceSearchPath(prompt, query),
		}}}
	case agentIsWorkspaceTreeIntent(prompt):
		path := agentExtractPath(prompt)
		if path == "" {
			path = "."
		}
		return []agentPlannerAction{{Name: "workspace.files.tree", Arguments: map[string]any{"path": path}}}
	case agentIsWorkspaceReadIntent(prompt):
		path := agentExtractPath(prompt)
		if path == "" {
			return nil
		}
		return []agentPlannerAction{{Name: "workspace.files.read", Arguments: map[string]any{"path": path}}}
	case agentIsWorkspaceListIntent(prompt) || agentIsWorkspaceIntent(prompt):
		path := agentExtractPath(prompt)
		if path == "" {
			path = "."
		}
		return []agentPlannerAction{{Name: "workspace.files.list", Arguments: map[string]any{"path": path}}}
	default:
		return nil
	}
}

func agentManualAuthorizationActionFromPrompt(prompt string) (agentPlannerAction, bool) {
	for _, action := range agentHeuristicActionsFromPrompt(prompt) {
		if agentPlannerActionRequiresManualAuthorization(action.Name) {
			return action, true
		}
	}
	return agentPlannerAction{}, false
}

func agentPromptRequestsImmediateExecution(prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || !agentNeedsDaemonTool(prompt) {
		return false
	}
	// "建立任务/制定计划" should remain at Plan and wait for Continue.
	if regexp.MustCompile(`(?i)(plan|draft|design|create\s+(?:a\s+)?task|make\s+(?:a\s+)?plan|规划|计划|制定|建立.*任务|创建.*任务)`).MatchString(prompt) &&
		!regexp.MustCompile(`(?i)(execute|run|start|immediately|right\s+now|now|直接|立即|马上|现在|先执行|先运行|启动|开始|执行|运行)`).MatchString(prompt) {
		return false
	}
	return agentIsCommandIntent(prompt) ||
		agentIsScanIntent(prompt) ||
		agentIsPortScanIntent(prompt) ||
		agentIsResumeApprovedIntent(prompt) ||
		agentIsApprovalDecisionIntent(prompt) ||
		agentIsWorkspaceIntent(prompt) ||
		agentIsRunContextAnalysisIntent(prompt) ||
		agentIsApprovalIntent(prompt) ||
		agentIsHealthIntent(prompt) ||
		agentIsPluginIntent(prompt) ||
		agentIsRunIntent(prompt) ||
		agentIsWorkspaceSearchIntent(prompt) ||
		agentIsWorkspaceTreeIntent(prompt) ||
		agentIsHTTPRequestIntent(prompt) ||
		agentIsAPIFunctionalTestIntent(prompt)
}

func agentShouldRouteBeforeLLMSynthesis(prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return false
	}
	if agentIsScheduleExecutionIntent(prompt) {
		return true
	}
	// When the operator explicitly names simple allowlisted shell commands,
	// execute those exact deterministic steps first and ask the LLM to synthesize
	// the observations. This prevents weaker providers from "planning" around
	// direct instructions like "先执行 pwd 和 ls -la" without actually doing them.
	if !agentIsCommandIntent(prompt) {
		return false
	}
	commands := agentExtractCommands(prompt)
	if len(commands) == 0 {
		return false
	}
	for _, command := range commands {
		if !agentPlannerCommandAllowed(command) {
			return false
		}
	}
	return true
}

func agentPlannerActionsFromLLMToolUse(resp *llm.ChatResponse) []agentPlannerAction {
	if resp == nil {
		return nil
	}
	actions := make([]agentPlannerAction, 0)
	for _, block := range resp.Content {
		switch toolUse := block.(type) {
		case llm.ToolUseBlock:
			action := agentPlannerAction{
				Name:      normalizeAgentPlannerActionName(toolUse.Name),
				Arguments: normalizeAgentPlannerActionArguments(string(toolUse.Input)),
			}
			if action.Name != "" {
				actions = append(actions, action)
			}
		case *llm.ToolUseBlock:
			if toolUse == nil {
				continue
			}
			action := agentPlannerAction{
				Name:      normalizeAgentPlannerActionName(toolUse.Name),
				Arguments: normalizeAgentPlannerActionArguments(string(toolUse.Input)),
			}
			if action.Name != "" {
				actions = append(actions, action)
			}
		}
	}
	return actions
}

var agentURLPattern = regexp.MustCompile(`https?://[^\s"'<>，。)]+`)
var agentBareNetworkTargetPattern = regexp.MustCompile(`(?i)(?:^|[\s（(])((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}(?::\d{1,5})?(?:/[^\s"'<>，。)]*)?)`)
var agentBarePortScanHostPattern = regexp.MustCompile(`(?i)(?:^|[\s（(])((?:localhost|(?:\d{1,3}\.){3}\d{1,3}|(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63})(?::\d{1,5})?)`)

func agentExtractURL(prompt string) string {
	if explicit := agentCleanToken(agentURLPattern.FindString(prompt)); explicit != "" {
		return explicit
	}
	return agentNormalizeURLInput(agentExtractBareNetworkTarget(prompt))
}

func agentNormalizeURLInput(value string) string {
	value = agentCleanToken(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "http://") || strings.HasPrefix(strings.ToLower(value), "https://") {
		return value
	}
	if agentLooksLikeBareNetworkTarget(value) {
		return "https://" + value
	}
	return value
}

func agentExtractBareNetworkTarget(prompt string) string {
	for _, match := range agentBareNetworkTargetPattern.FindAllStringSubmatch(prompt, -1) {
		if len(match) < 2 {
			continue
		}
		candidate := agentCleanToken(match[1])
		if agentLooksLikeBareNetworkTarget(candidate) {
			return candidate
		}
	}
	return ""
}

func agentExtractPortScanHostFromPrompt(prompt string) string {
	if target := agentExtractURL(prompt); target != "" {
		return agentExtractPortScanHost(target)
	}
	for _, match := range agentBarePortScanHostPattern.FindAllStringSubmatch(prompt, -1) {
		if len(match) < 2 {
			continue
		}
		host := agentExtractPortScanHost(agentCleanToken(match[1]))
		if host != "" {
			return host
		}
	}
	return ""
}

func agentLooksLikeBareNetworkTarget(value string) bool {
	value = agentCleanToken(value)
	if value == "" || strings.Contains(value, "://") || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	host := value
	if slash := strings.Index(host, "/"); slash >= 0 {
		host = host[:slash]
	}
	if colon := strings.LastIndex(host, ":"); colon > strings.LastIndex(host, "]") {
		port := host[colon+1:]
		if port == "" {
			return false
		}
		for _, r := range port {
			if r < '0' || r > '9' {
				return false
			}
		}
		host = host[:colon]
	}
	host = strings.Trim(host, "[]")
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	lastDot := strings.LastIndex(host, ".")
	tld := strings.ToLower(host[lastDot+1:])
	if len(tld) < 2 || len(tld) > 63 {
		return false
	}
	if agentLooksLikeWorkspaceFileExtension(tld) {
		return false
	}
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func agentLooksLikeWorkspaceFileExtension(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "md", "txt", "log", "json", "yaml", "yml", "toml", "xml", "csv",
		"go", "py", "js", "jsx", "ts", "tsx", "css", "scss", "html", "htm",
		"sh", "bash", "zsh", "sql", "rs", "java", "c", "cc", "cpp", "h",
		"png", "jpg", "jpeg", "gif", "svg", "webp", "pdf", "zip", "gz", "tar",
		"list", "context", "draft", "safe", "command", "files", "approvals", "plugins", "runs", "web", "request", "health":
		return true
	default:
		return false
	}
}

func agentExtractScope(prompt, target string) string {
	lower := strings.ToLower(prompt)
	for _, marker := range []string{"scope=", "scope:", "scope：", "范围=", "范围:", "范围："} {
		index := strings.Index(lower, strings.ToLower(marker))
		if index >= 0 {
			value := prompt[index+len(marker):]
			fields := strings.FieldsFunc(value, func(r rune) bool {
				return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '，' || r == ';' || r == '；'
			})
			if len(fields) > 0 {
				return agentCleanToken(fields[0])
			}
		}
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func agentCleanToken(value string) string {
	return strings.Trim(strings.TrimSpace(value), "，。),]\"'")
}

func agentIsRunContextAnalysisIntent(prompt string) bool {
	return (agentExtractRunID(prompt) != "" || agentMentionsActiveRun(prompt)) &&
		regexp.MustCompile(`(?i)(report|finding|artifact|evidence|events?|timeline|log|approvals?|approve|deny|context|报告|发现|漏洞|证据|工件|产物|事件|时间线|日志|审批|批准|拒绝|上下文|修复建议|误报)`).MatchString(prompt)
}

func agentMentionsActiveRun(prompt string) bool {
	return regexp.MustCompile(`(?i)(active\s+run|current\s+run|this\s+run|selected\s+run|active\s+scan|current\s+scan|this\s+scan|当前运行|当前任务|当前扫描|这个运行|这个扫描|本次运行|本次扫描)`).MatchString(prompt)
}

func agentIsReportDraftIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(report\s*draft|draft\s*report|write\s+(?:a\s+)?report|generate\s+(?:a\s+)?report|报告草稿|生成报告|报告初稿|撰写报告|输出报告)`).MatchString(prompt)
}

var (
	agentScheduleVerbRe   = regexp.MustCompile(`(?i)(定时|周期(?:性)?|每\s*(?:天|日|小时|分钟|周|月|个小时)|每\s*\d+\s*(?:分钟|小时|天|秒|min|hour|day|s)|cron|schedule|scheduled|recurring|every\s+\d+\s*(?:m|min|minute|h|hr|hour|d|day|s|sec|second)s?|every\s+(?:day|hour|minute|week|month|night|morning)|nightly|hourly|daily|weekly|monthly)`)
	agentScheduleTimeRe   = regexp.MustCompile(`(?i)(?:上午|下午|晚上|早上|凌晨)?\s*\d{1,2}[:：]\d{2}\s*(?:执行|运行|触发|开始|run|start)`)
	agentScheduleListRe   = regexp.MustCompile(`(?i)(列出.*定时|查看.*定时|有哪些定时|定时任务列表|所有定时|list\s+schedules?|show\s+schedules?|schedules\s*\?)`)
	agentScheduleDeleteRe = regexp.MustCompile(`(?i)(删除.*定时|取消.*定时|移除.*定时|delete\s+schedule|remove\s+schedule|cancel\s+schedule)`)
	agentCronExprRe       = regexp.MustCompile(`(?i)(@(?:every\s+\S+|hourly|daily|weekly|monthly|yearly|annually|midnight|reboot))|(\b(?:\*|\d+|\*/\d+|\d+(?:[,-]\d+)*)\s+(?:\*|\d+|\*/\d+|\d+(?:[,-]\d+)*)\s+(?:\*|\d+|\*/\d+|\d+(?:[,-]\d+)*)\s+(?:\*|\d+|\*/\d+|\d+(?:[,-]\d+)*)\s+(?:\*|\?|\d+|\*/\d+|\d+(?:[,-]\d+)*|MON|TUE|WED|THU|FRI|SAT|SUN)\b)`)
	agentScheduleIDRe     = regexp.MustCompile(`(?i)(?:schedule|定时(?:任务)?|cron)\s*(?:id\s*[:=]?\s*)?([A-Za-z0-9][A-Za-z0-9\-_]{2,})`)
)

func agentIsScheduleExecutionIntent(prompt string) bool {
	return agentIsScheduleCreateIntent(prompt) || agentIsScheduleListIntent(prompt) || agentIsScheduleDeleteIntent(prompt)
}

func agentIsScheduleCreateIntent(prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return false
	}
	if agentScheduleListRe.MatchString(prompt) || agentScheduleDeleteRe.MatchString(prompt) {
		return false
	}
	return agentScheduleVerbRe.MatchString(prompt) || agentScheduleTimeRe.MatchString(prompt)
}

func agentIsScheduleListIntent(prompt string) bool {
	return agentScheduleListRe.MatchString(prompt)
}

func agentIsScheduleDeleteIntent(prompt string) bool {
	return agentScheduleDeleteRe.MatchString(prompt)
}

func agentExtractCronExpression(prompt string) string {
	if m := agentCronExprRe.FindString(prompt); m != "" {
		return strings.TrimSpace(m)
	}
	low := strings.ToLower(prompt)
	if m := regexp.MustCompile(`(?i)(上午|下午|晚上|早上|凌晨)?\s*(\d{1,2})[:：](\d{2})\s*(?:执行|运行|触发|开始|run|start)?`).FindStringSubmatch(prompt); len(m) == 4 && (agentScheduleVerbRe.MatchString(prompt) || agentScheduleTimeRe.MatchString(prompt)) {
		hour, _ := strconv.Atoi(m[2])
		minute, _ := strconv.Atoi(m[3])
		period := strings.TrimSpace(m[1])
		if (period == "下午" || period == "晚上") && hour >= 1 && hour < 12 {
			hour += 12
		}
		if hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 {
			return fmt.Sprintf("%d %d * * *", minute, hour)
		}
	}
	switch {
	case regexp.MustCompile(`(?i)每\s*分钟|every\s+minute|nightly|每分钟`).MatchString(low):
		return "* * * * *"
	case regexp.MustCompile(`(?i)每\s*小时|每\s*个\s*小时|every\s+hour|hourly`).MatchString(low):
		return "0 * * * *"
	case regexp.MustCompile(`(?i)每\s*(?:天|日)|every\s+day|daily`).MatchString(low):
		return "0 0 * * *"
	case regexp.MustCompile(`(?i)每\s*周|every\s+week|weekly`).MatchString(low):
		return "0 0 * * 0"
	case regexp.MustCompile(`(?i)每\s*月|every\s+month|monthly`).MatchString(low):
		return "0 0 1 * *"
	}
	if m := regexp.MustCompile(`(?i)每\s*(\d+)\s*(分钟|小时|天|min|minute|hour|day)`).FindStringSubmatch(prompt); len(m) == 3 {
		unit := strings.ToLower(m[2])
		switch unit {
		case "分钟", "min", "minute":
			return "@every " + m[1] + "m"
		case "小时", "hour":
			return "@every " + m[1] + "h"
		case "天", "day":
			return "@every " + m[1] + "h"
		}
	}
	if m := regexp.MustCompile(`(?i)every\s+(\d+)\s*(m|min|minute|h|hr|hour|d|day)s?`).FindStringSubmatch(prompt); len(m) == 3 {
		unit := strings.ToLower(m[2])
		switch unit {
		case "m", "min", "minute":
			return "@every " + m[1] + "m"
		case "h", "hr", "hour":
			return "@every " + m[1] + "h"
		case "d", "day":
			return "@every " + (func() string {
				if n, err := strconv.Atoi(m[1]); err == nil {
					return strconv.Itoa(n*24) + "h"
				}
				return m[1] + "h"
			})()
		}
	}
	return ""
}

func agentExtractScheduleID(prompt string) string {
	if m := agentScheduleIDRe.FindStringSubmatch(prompt); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func agentScheduleExecutionPrompt(prompt string) string {
	cleaned := strings.TrimSpace(prompt)
	if cleaned == "" {
		return cleaned
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)@(?:every\s+\S+|hourly|daily|weekly|monthly|yearly|annually|midnight|reboot)`),
		regexp.MustCompile(`(?i)\b(?:\*|\d+|\*/\d+|\d+(?:[,-]\d+)*)\s+(?:\*|\d+|\*/\d+|\d+(?:[,-]\d+)*)\s+(?:\*|\d+|\*/\d+|\d+(?:[,-]\d+)*)\s+(?:\*|\d+|\*/\d+|\d+(?:[,-]\d+)*)\s+(?:\*|\?|\d+|\*/\d+|\d+(?:[,-]\d+)*|MON|TUE|WED|THU|FRI|SAT|SUN)\b`),
		regexp.MustCompile(`(?i)(?:每天|每日|每晚|每小时|每分钟|每周|每月|周期(?:性)?|定时|scheduled|recurring|hourly|daily|weekly|monthly|nightly)`),
		regexp.MustCompile(`(?i)每\s*\d+\s*(?:分钟|小时|天|秒|min|hour|day|s)`),
		regexp.MustCompile(`(?i)every\s+\d+\s*(?:m|min|minute|h|hr|hour|d|day|s|sec|second)s?`),
		regexp.MustCompile(`(?i)every\s+(?:day|hour|minute|week|month|night|morning)`),
		regexp.MustCompile(`(?i)(?:上午|下午|晚上|早上|凌晨)?\s*\d{1,2}[:：]\d{2}\s*(?:执行|运行|触发|开始|run|start)?`),
		regexp.MustCompile(`(?i)(?:执行|运行|触发|开始)\s*$`),
	} {
		cleaned = re.ReplaceAllString(cleaned, " ")
	}
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	cleaned = strings.Trim(cleaned, " ，,。.;；:-")
	if cleaned == "" {
		return strings.TrimSpace(prompt)
	}
	return cleaned
}

func agentScheduleArgsFromPrompt(prompt, language string) map[string]any {
	args := map[string]any{
		"cron":     agentExtractCronExpression(prompt),
		"language": language,
	}
	target := agentExtractURL(prompt)
	if target != "" && agentIsScanIntent(prompt) {
		args["action"] = "agent_prompt"
		args["target"] = target
		args["prompt"] = "扫描 " + target
		args["ops_level"] = "safe_scan"
		args["profile"] = "safe"
		return args
	}
	args["action"] = "agent_prompt"
	args["prompt"] = agentScheduleExecutionPrompt(prompt)
	args["ops_level"] = "safe_scan"
	return args
}

func agentIsWebSearchIntent(prompt string) bool {
	if agentExtractURL(prompt) != "" {
		return false
	}
	return regexp.MustCompile(`(?i)(web\s*search|internet\s*search|search\s+(?:the\s+)?web|look\s*up|查询|搜索|检索|联网搜索|网上搜索|查一下|搜一下)`).MatchString(prompt)
}

func agentIsWebFetchIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" {
		return false
	}
	if agentIsScanIntent(prompt) || agentIsPortScanIntent(prompt) || agentIsContentDiscoveryIntent(prompt) || agentIsJSRouteAnalysisIntent(prompt) || agentIsAPIAuthTestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt) {
		return false
	}
	return regexp.MustCompile(`(?i)(read\s+(?:this\s+)?(?:url|page|article)|fetch\s+(?:this\s+)?(?:url|page|article)|summari[sz]e\s+(?:this\s+)?(?:url|page|article)|open\s+(?:this\s+)?url|读取|阅读|获取页面|抓取页面|总结.*(?:链接|网页|文章)|读一下|看一下)`).MatchString(prompt)
}

func agentIsScanIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" {
		return false
	}
	if agentIsScheduleCreateIntent(prompt) {
		return false
	}
	if agentIsPortScanIntent(prompt) || agentIsContentDiscoveryIntent(prompt) || agentIsJSRouteAnalysisIntent(prompt) {
		return false
	}
	if agentIsAPIAuthTestIntent(prompt) && !regexp.MustCompile(`(?i)(safe\s*scan|web\s*scan|vuln(?:erability)?\s*scan|security\s*(scan|audit|assessment)|安全扫描|漏洞扫描|漏洞检测)`).MatchString(prompt) {
		return false
	}
	return regexp.MustCompile(`(?i)(\bscan\s+https?://|扫描\s*https?://|扫描.*scope\s*=|web\s*scan|safe\s*scan|vuln(?:erability)?\s*scan|security\s*(scan|audit|assessment)|scan\s+for\s+(vuln|security)|audit\s+(?:the\s+)?(?:site|api|web)|assess\s+(?:the\s+)?(?:site|api|web)\s+security|扫描.*漏洞|漏洞.*扫描|检测.*漏洞|测试.*漏洞|扫描漏洞|漏洞扫描|安全扫描|Web\s*扫描|网站扫描|API\s*扫描|扫描网站|扫描接口|安全检测|安全测试|漏洞检测|漏洞测试)`).MatchString(prompt)
}

func agentIsPortScanIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" && agentExtractPortScanHostFromPrompt(prompt) == "" {
		return false
	}
	if agentIsHTTPPortFallbackIntent(prompt) {
		return false
	}
	return regexp.MustCompile(`(?i)(open\s+ports?|ports?\s+open|port\s+(scan|enum|enumeration|discovery)|which\s+ports?|nmap|masscan|端口|开放.*端口|端口.*开放|哪些端口|开了哪些端口)`).MatchString(prompt)
}

func agentIsHTTPPortFallbackIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" && agentExtractPortScanHostFromPrompt(prompt) == "" {
		return false
	}
	return regexp.MustCompile(`(?i)(http\(s\)|https?|web\s*ports?|80\s*/\s*443|80\s+and\s+443|common\s+web\s+ports?|只验证.*(?:http|web).*端口|验证.*(?:80|443)|常见\s*web\s*端口|HTTP\s*响应|HTTPS?\s*响应)`).MatchString(prompt) &&
		regexp.MustCompile(`(?i)(only|validate|probe|check|只|仅|验证|探测|检查)`).MatchString(prompt)
}

func agentIsContentDiscoveryIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" {
		return false
	}
	return regexp.MustCompile(`(?i)(content\s+discover|directory\s+(?:brute|fuzz|enum|enumeration|discover)|path\s+(?:brute|fuzz|enum|enumeration|discover)|dirbuster|dirsearch|gobuster|ffuf|feroxbuster|hidden\s+(?:dir|directory|path|file)|discover\s+(?:dirs?|directories|paths?|files)|目录(?:扫描|爆破|枚举|发现)|路径(?:爆破|枚举|发现|扫描)|隐藏(?:目录|路径|文件)|敏感(?:目录|路径|文件)|备份文件|字典扫描)`).MatchString(prompt)
}

func agentIsJSRouteAnalysisIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" && !regexp.MustCompile(`(?i)(workspace|工作空间|file|文件)`).MatchString(prompt) {
		return false
	}
	return regexp.MustCompile(`(?i)(js\s*(?:route|endpoint|api)|javascript\s*(?:route|endpoint|api)|front[-\s]?end\s*(?:route|endpoint)|extract\s+routes?|route\s+extract|analy[sz]e\s+js|js_route_analyzer|JS\s*(?:路由|接口|端点)|JavaScript\s*(?:路由|接口|端点)|前端(?:路由|接口)|提取.*(?:JS|JavaScript).*(?:接口|路由|端点)|从.*(?:JS|JavaScript).*(?:接口|路由|端点))`).MatchString(prompt)
}

func agentIsAPIAuthTestIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" {
		return false
	}
	return regexp.MustCompile(`(?i)(auth(?:entication|orization)?|login|logout|jwt|bearer|token|cookie|session|rbac|idor|access\s+control|privilege|permission|bypass\s+auth|认证|鉴权|授权|登录|登出|令牌|会话|权限|越权|水平越权|垂直越权|未授权|鉴权绕过|认证绕过|Cookie|JWT)`).MatchString(prompt)
}

func agentIsAPIFunctionalTestIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" {
		return false
	}
	return regexp.MustCompile(`(?i)(api\s*(?:functional|smoke|contract|schema|endpoint)\s*test|test\s+(?:the\s+)?api|call\s+(?:the\s+)?api|接口(?:功能|连通|冒烟|契约|调用|请求)|调用接口|测试接口|检查接口|验证接口)`).MatchString(prompt)
}

func agentIsHTTPRequestIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" {
		return false
	}
	return regexp.MustCompile(`(?i)(http|request|fetch|curl|headers?|status|probe|\bGET\b|\bHEAD\b|\bOPTIONS\b|\bPOST\b|\bPUT\b|\bPATCH\b|\bDELETE\b|访问|请求|抓取|获取|响应|状态码|响应头|探测)`).MatchString(prompt)
}

func agentIsExplicitHTTPRequestWithMethodOrRequest(prompt string) bool {
	if agentExtractURL(prompt) == "" {
		return false
	}
	return regexp.MustCompile(`(?i)(\bGET\b|\bHEAD\b|\bOPTIONS\b|\bPOST\b|\bPUT\b|\bPATCH\b|\bDELETE\b|method\s*[:=]|curl|http\s+request|request|fetch|请求|调用|访问|响应头|状态码)`).MatchString(prompt)
}

func agentPromptMentionsSpecificPathProbe(prompt string) bool {
	if !agentIsHTTPRequestIntent(prompt) {
		return false
	}
	target := agentExtractURL(prompt)
	if target == "" {
		return false
	}
	parsed, err := url.Parse(target)
	return err == nil && parsed.Path != "" && parsed.Path != "/"
}

func agentPromptRequestsEntryPageOnly(prompt string) bool {
	return agentIsHTTPRequestIntent(prompt) &&
		regexp.MustCompile(`(?i)(entry\s+page|home\s+page|index\s+page|入口页|首页|只请求|仅请求|只获取|仅获取)`).MatchString(prompt)
}

func agentHeuristicScanActionsFromPrompt(prompt string) []agentPlannerAction {
	target := agentExtractURL(prompt)
	if target == "" {
		return nil
	}
	return []agentPlannerAction{{Name: "scan.web.safe", Arguments: map[string]any{
		"target": target,
		"scope":  agentExtractScope(prompt, target),
	}}}
}

func agentHTTPProbeActionsFromPrompt(prompt string) []agentPlannerAction {
	target := agentExtractURL(prompt)
	if target == "" {
		return nil
	}
	method := agentExtractHTTPMethod(prompt)
	if agentIsHTTPPortFallbackIntent(prompt) {
		host := agentExtractPortScanHostFromPrompt(prompt)
		if host == "" {
			if parsed, err := url.Parse(target); err == nil {
				host = parsed.Host
			}
		}
		host = strings.TrimSpace(host)
		if host != "" {
			return []agentPlannerAction{
				{Name: "http.request", Arguments: map[string]any{"url": "https://" + strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://"), "method": http.MethodHead}},
				{Name: "http.request", Arguments: map[string]any{"url": "http://" + strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://"), "method": http.MethodHead}},
			}
		}
	}
	return []agentPlannerAction{{Name: "http.request", Arguments: map[string]any{
		"url":    target,
		"method": method,
	}}}
}

func agentPortScanActionsFromPrompt(prompt string) []agentPlannerAction {
	target := agentExtractURL(prompt)
	host := agentExtractPortScanHostFromPrompt(prompt)
	if host == "" {
		return nil
	}
	if target == "" {
		target = host
	}
	ports, portLabel, err := agentExtractSafePortScanPorts(prompt)
	if err != nil {
		// Keep the planner conservative: if the requested range is too broad,
		// do not emit a tool action; route/fallback will return a blocked plan.
		return nil
	}
	return []agentPlannerAction{{Name: "ports.scan.safe", Arguments: map[string]any{
		"target":       target,
		"host":         host,
		"ports":        ports,
		"port_profile": portLabel,
	}}}
}
