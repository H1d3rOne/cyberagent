package daemon

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func (m *agentManager) executeAgentPlannerAction(ctx context.Context, sessionID, language, operator, operationLevel string, svc routerServices, action agentPlannerAction, authProfiles []agentHTTPAuthProfile, defaultAuthProfileID string) ([]agentMessage, []agentToolCall, error) {
	name := normalizeAgentPlannerActionName(action.Name)
	args := action.Arguments
	if args == nil {
		args = map[string]any{}
	}

	if spec, ok := toolRegistry().Lookup(name); ok && spec.Handler != nil {
		return m.executeAgentRegistryHandler(ctx, sessionID, language, operator, operationLevel, svc, spec, args, authProfiles, defaultAuthProfileID)
	}

	// Deprecated: legacy per-tool dispatch. Migrate cases into ToolSpec.Handler
	// in small slices until the registry is the only execution source.
	switch name {
	case "scan.web.safe":
		target := firstNonEmpty(
			stringFromAny(args["target"]),
			stringFromAny(args["url"]),
			stringFromAny(args["input"]),
		)
		scope := firstNonEmpty(
			stringFromAny(args["scope"]),
			stringFromAny(args["scope_url"]),
			target,
		)
		prompt := "扫描 " + target
		if scope != "" {
			prompt += " scope=" + scope
		}
		return m.handleAgentScan(ctx, prompt, language, svc)
	case "ports.scan.safe":
		target := firstNonEmpty(
			stringFromAny(args["target"]),
			stringFromAny(args["host"]),
			stringFromAny(args["url"]),
			stringFromAny(args["input"]),
		)
		if target == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "ports.scan.safe requires target or host", map[string]any{"action": action.Name}, nil)
		}
		prompt := "扫描 " + target + " 开放端口"
		if portsSpec := agentPortSpecFromPlannerArgument(args["ports"]); portsSpec != "" {
			prompt += " ports=" + portsSpec
		}
		return m.handleAgentPortScan(ctx, prompt, language)
	case "workspace.command":
		cmd := firstNonEmpty(
			stringFromAny(args["command"]),
			stringFromAny(args["cmd"]),
			stringFromAny(args["input"]),
		)
		if cmd == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.command requires command", map[string]any{"action": action.Name}, nil)
		}
		if !agentPlannerCommandAllowed(cmd) {
			return nil, nil, services.NewError(services.CodePolicyDenied, "planner command is not in the low-risk allowlist; ask the operator to run it explicitly", map[string]any{"command": cmd}, nil)
		}
		return m.handleAgentCommand(ctx, sessionID, "command: "+cmd, language)
	case "workspace.files.list":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			stringFromAny(args["input"]),
			".",
		)
		return m.handleAgentWorkspaceList(sessionID, "path="+relPath, language)
	case "workspace.files.tree":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			stringFromAny(args["input"]),
			".",
		)
		depth := intFromAny(args["depth"])
		if depth <= 0 {
			depth = 3
		}
		limit := intFromAny(args["limit"])
		if limit <= 0 {
			limit = 120
		}
		includeHidden := boolFromAny(args["include_hidden"])
		return m.handleAgentWorkspaceTree(sessionID, relPath, depth, limit, includeHidden, language)
	case "workspace.files.read":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
			stringFromAny(args["input"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.read requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceRead(sessionID, "file="+relPath, language)
	case "workspace.search":
		query := firstNonEmpty(
			stringFromAny(args["query"]),
			stringFromAny(args["pattern"]),
			stringFromAny(args["text"]),
			stringFromAny(args["input"]),
		)
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			".",
		)
		if query == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.search requires query", map[string]any{"action": action.Name}, nil)
		}
		limit := intFromAny(args["limit"])
		if limit <= 0 {
			limit = 80
		}
		return m.handleAgentWorkspaceSearch(sessionID, query, relPath, boolFromAny(args["regex"]), boolFromAny(args["case_sensitive"]), limit, language)
	case "workspace.files.write":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		content := firstNonEmpty(
			stringFromAny(args["content"]),
			stringFromAny(args["body"]),
			stringFromAny(args["text"]),
			stringFromAny(args["input"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.write requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceWrite(sessionID, relPath, content, language)
	case "workspace.files.patch":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(args["old"]),
			stringFromAny(args["old_text"]),
			stringFromAny(args["before"]),
			stringFromAny(args["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(args["new"]),
			stringFromAny(args["new_text"]),
			stringFromAny(args["after"]),
			stringFromAny(args["replace"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.patch requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspacePatch(sessionID, relPath, oldText, newText, language)
	case "workspace.files.diff":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(args["old"]),
			stringFromAny(args["old_text"]),
			stringFromAny(args["before"]),
			stringFromAny(args["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(args["new"]),
			stringFromAny(args["new_text"]),
			stringFromAny(args["after"]),
			stringFromAny(args["replace"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.diff requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceDiff(sessionID, relPath, oldText, newText, language)
	case "workspace.files.diff_many":
		changes := workspaceChangeInputsFromAny(args["changes"])
		if len(changes) == 0 {
			changes = workspaceChangeInputsFromAny(args["input"])
		}
		return m.handleAgentWorkspaceDiffMany(sessionID, changes, language)
	case "workspace.files.rebase":
		changeID := firstNonEmpty(
			stringFromAny(args["change_id"]),
			stringFromAny(args["id"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(args["old"]),
			stringFromAny(args["old_text"]),
			stringFromAny(args["before"]),
			stringFromAny(args["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(args["new"]),
			stringFromAny(args["new_text"]),
			stringFromAny(args["after"]),
			stringFromAny(args["replace"]),
		)
		return m.handleAgentWorkspaceRebase(sessionID, changeID, oldText, newText, language)
	case "workspace.files.apply":
		changeID := firstNonEmpty(
			stringFromAny(args["change_id"]),
			stringFromAny(args["id"]),
			stringFromAny(args["input"]),
		)
		groupID := firstNonEmpty(stringFromAny(args["group_id"]), stringFromAny(args["group"]))
		if groupID != "" {
			return m.handleAgentWorkspaceApplyGroup(sessionID, groupID, language)
		}
		if boolFromAny(args["all"]) || strings.EqualFold(changeID, "all") || strings.EqualFold(stringFromAny(args["scope"]), "all") {
			return m.handleAgentWorkspaceApplyAll(sessionID, language)
		}
		if changeID == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.apply requires change_id", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceApply(sessionID, changeID, language)
	case "workspace.files.discard":
		changeID := firstNonEmpty(
			stringFromAny(args["change_id"]),
			stringFromAny(args["id"]),
			stringFromAny(args["input"]),
		)
		groupID := firstNonEmpty(stringFromAny(args["group_id"]), stringFromAny(args["group"]))
		if groupID != "" {
			return m.handleAgentWorkspaceDiscardGroup(sessionID, groupID, language)
		}
		if boolFromAny(args["all"]) || strings.EqualFold(changeID, "all") || strings.EqualFold(stringFromAny(args["scope"]), "all") {
			return m.handleAgentWorkspaceDiscardAll(sessionID, language)
		}
		if changeID == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.discard requires change_id", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceDiscard(sessionID, changeID, language)
	case "workspace.files.delete":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
			stringFromAny(args["input"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.delete requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceDelete(sessionID, relPath, language)
	case "workspace.files.move":
		fromPath := firstNonEmpty(
			stringFromAny(args["from"]),
			stringFromAny(args["source"]),
			stringFromAny(args["old"]),
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		toPath := firstNonEmpty(
			stringFromAny(args["to"]),
			stringFromAny(args["target"]),
			stringFromAny(args["new"]),
			stringFromAny(args["dest"]),
			stringFromAny(args["destination"]),
		)
		if fromPath == "" || toPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.move requires from and to", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceMove(sessionID, fromPath, toPath, language)
	case "workspace.files.copy":
		fromPath := firstNonEmpty(
			stringFromAny(args["from"]),
			stringFromAny(args["source"]),
			stringFromAny(args["old"]),
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		toPath := firstNonEmpty(
			stringFromAny(args["to"]),
			stringFromAny(args["target"]),
			stringFromAny(args["new"]),
			stringFromAny(args["dest"]),
			stringFromAny(args["destination"]),
		)
		if fromPath == "" || toPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.copy requires from and to", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceCopy(sessionID, fromPath, toPath, language)
	case "workspace.files.mkdir":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			stringFromAny(args["input"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.mkdir requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceMkdir(sessionID, relPath, language)
	case "http.request":
		targetURL := firstNonEmpty(
			stringFromAny(args["url"]),
			stringFromAny(args["target"]),
			stringFromAny(args["input"]),
		)
		method := firstNonEmpty(stringFromAny(args["method"]), "GET")
		if targetURL == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "http.request requires url", map[string]any{"action": action.Name}, nil)
		}
		targetURL = agentNormalizeURLInput(targetURL)
		headers := stringMapFromAny(args["headers"])
		if contentType := stringFromAny(args["content_type"]); contentType != "" {
			headers["Content-Type"] = contentType
		}
		authProfileID := firstNonEmpty(
			stringFromAny(args["auth_profile_id"]),
			stringFromAny(args["auth_profile"]),
			stringFromAny(args["profile_id"]),
			stringFromAny(args["profile"]),
		)
		profileHeaders, profileUsage, profileErr := agentResolveHTTPAuthProfileHeaders(authProfiles, authProfileID, defaultAuthProfileID, targetURL)
		if profileErr != nil {
			return nil, nil, profileErr
		}
		for key, value := range profileHeaders {
			if _, exists := headers[key]; !exists {
				headers[key] = value
			}
		}
		body := firstNonEmpty(
			stringFromAny(args["body"]),
			stringFromAny(args["content"]),
			stringFromAny(args["data"]),
			stringFromAny(args["input_body"]),
		)
		return m.handleAgentHTTPRequest(ctx, method, targetURL, headers, body, language, profileUsage)
	case "daemon.health":
		return m.handleAgentHealth(ctx, language, svc)
	case "plugins.list":
		return m.handleAgentPlugins(ctx, language, svc)
	case "runs.list":
		return m.handleAgentRuns(ctx, language, svc)
	case "runs.context":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		)
		contextKind := firstNonEmpty(
			stringFromAny(args["context"]),
			stringFromAny(args["kind"]),
			stringFromAny(args["section"]),
			stringFromAny(args["type"]),
		)
		return m.handleAgentRunContextTool(ctx, sessionID, runID, contextKind, language, svc)
	case "reports.draft":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		)
		outputPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["output_path"]),
			stringFromAny(args["file"]),
		)
		return m.handleAgentReportDraft(ctx, sessionID, runID, language, svc, outputPath)
	case "runs.approvals.list":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		)
		if runID == "" {
			return m.handleAgentApprovals(ctx, language, svc)
		}
		return m.handleAgentRunContextAnalysis(ctx, "读取运行 "+runID+" 的 approvals 审批队列", language, svc)
	case "runs.approvals.approve", "runs.approvals.deny":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
		)
		approvalID := firstNonEmpty(
			stringFromAny(args["approval_id"]),
			stringFromAny(args["approval"]),
			stringFromAny(args["step_id"]),
			stringFromAny(args["step"]),
			stringFromAny(args["id"]),
		)
		comment := stringFromAny(args["comment"])
		verb := "批准"
		if name == "runs.approvals.deny" {
			verb = "拒绝"
		}
		prompt := verb + "当前审批"
		if runID != "" && approvalID != "" {
			prompt = fmt.Sprintf("%s运行 %s 的 %s", verb, runID, approvalID)
		} else if runID != "" {
			prompt = fmt.Sprintf("%s运行 %s 的当前审批", verb, runID)
		} else if approvalID != "" {
			prompt = fmt.Sprintf("%s %s", verb, approvalID)
		}
		if strings.TrimSpace(comment) != "" {
			prompt += " comment=" + comment
		}
		return m.handleAgentApprovalDecision(ctx, sessionID, prompt, language, operator, svc)
	case "runs.resume_approved":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		)
		prompt := "恢复已批准当前运行"
		if runID != "" {
			prompt = "恢复已批准运行 " + runID
		}
		return m.handleAgentResumeApproved(ctx, sessionID, prompt, language, operator, svc)
	case "schedule.create":
		return m.handleAgentScheduleCreate(sessionID, language, operator, svc, args)
	case "schedule.list":
		return m.handleAgentScheduleList(language, svc)
	case "schedule.delete":
		return m.handleAgentScheduleDelete(language, svc, args)
	case "web.search":
		return m.handleAgentWebSearch(ctx, language, svc, args)
	case "web.fetch":
		return m.handleAgentWebFetch(ctx, language, args)
	default:
		return nil, nil, services.NewError(services.CodeUnsupported, "unsupported planner action", map[string]any{"action": action.Name}, nil)
	}
}

func (m *agentManager) routePrompt(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	requiredLevel := agentOperationLevelRequiredForPrompt(prompt)
	if !agentOperationLevelAtLeast(operationLevel, requiredLevel) {
		return []agentMessage{m.message(
			language,
			"agent",
			agentOperationLevelDeniedError(language, operationLevel, requiredLevel, "natural_language_route").Error(),
			nil,
			nil,
		)}, nil, nil
	}
	switch {
	case agentIsScheduleCreateIntent(prompt):
		return m.handleAgentScheduleCreate(sessionID, language, operator, svc, agentScheduleArgsFromPrompt(prompt, language))
	case agentIsScheduleListIntent(prompt):
		return m.handleAgentScheduleList(language, svc)
	case agentIsScheduleDeleteIntent(prompt):
		return m.handleAgentScheduleDelete(language, svc, map[string]any{"id": agentExtractScheduleID(prompt)})
	case agentIsResumeApprovedIntent(prompt):
		return m.handleAgentResumeApproved(ctx, sessionID, prompt, language, operator, svc)
	case agentIsApprovalDecisionIntent(prompt):
		return m.handleAgentApprovalDecision(ctx, sessionID, prompt, language, operator, svc)
	case agentIsReportDraftIntent(prompt):
		return m.handleAgentReportDraft(ctx, sessionID, agentExtractRunID(prompt), language, svc, "")
	case agentIsRunContextAnalysisIntent(prompt):
		return m.handleAgentRunContextTool(ctx, sessionID, agentExtractRunID(prompt), agentRunContextKind(prompt), language, svc)
	case agentIsPortScanIntent(prompt):
		return m.handleAgentPortScan(ctx, prompt, language)
	case agentHasCapabilityGapIntent(prompt):
		plan := agentCapabilityGapPlannerFromPrompt(prompt, "", language)
		if plan != nil {
			message := agentMessageWithPhase(m.message(language, "agent", plan.Answer, nil, nil), "plan")
			message.Reasoning = trimSummary(plan.Reasoning, 800)
			m.appendMessagesWithPlanner(sessionID, plan)
			return []agentMessage{message}, nil, nil
		}
		return []agentMessage{m.message(language, "agent", agentExplain(language), nil, nil)}, nil, nil
	case agentIsScanIntent(prompt):
		return m.handleAgentScan(ctx, prompt, language, svc)
	case agentIsCommandIntent(prompt):
		return m.handleAgentCommand(ctx, sessionID, prompt, language)
	case agentIsApprovalIntent(prompt):
		return m.handleAgentApprovals(ctx, language, svc)
	case agentIsHealthIntent(prompt):
		return m.handleAgentHealth(ctx, language, svc)
	case agentIsPluginIntent(prompt):
		return m.handleAgentPlugins(ctx, language, svc)
	case agentIsRunIntent(prompt):
		return m.handleAgentRuns(ctx, language, svc)
	case agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt):
		return m.handleAgentHTTPRequest(ctx, agentExtractHTTPMethod(prompt), agentExtractURL(prompt), nil, "", language, nil)
	case agentIsWorkspaceSearchIntent(prompt):
		query := agentExtractWorkspaceSearchQuery(prompt)
		return m.handleAgentWorkspaceSearch(sessionID, query, agentExtractWorkspaceSearchPath(prompt, query), false, false, 80, language)
	case agentIsWorkspaceTreeIntent(prompt):
		return m.handleAgentWorkspaceTree(sessionID, agentExtractPath(prompt), 3, 120, false, language)
	case agentIsWorkspaceIntent(prompt):
		return m.handleAgentWorkspace(ctx, sessionID, prompt, language)
	default:
		_ = operator
		return []agentMessage{m.message(language, "agent", agentExplain(language), nil, nil)}, nil, nil
	}
}

func agentExecutableAutoContinuePrompt(next *agentNextAction) string {
	if next == nil || next.Type != "continue" || !next.AutoContinue {
		return ""
	}
	prompt := strings.TrimSpace(next.SuggestedPrompt)
	if prompt == "" {
		return ""
	}
	if agentControlActionFromPrompt(prompt) == "continue" {
		remainder := strings.TrimSpace(agentControlPromptFromPrompt(prompt, "continue"))
		if remainder == "" {
			return ""
		}
		prompt = remainder
	}
	return prompt
}

func agentCanAutoRouteContinuePrompt(prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return false
	}
	if agentIsControlIntent(prompt) || agentIsScanIntent(prompt) || agentIsPortScanIntent(prompt) || agentIsResumeApprovedIntent(prompt) || agentIsApprovalDecisionIntent(prompt) {
		return false
	}
	if agentIsCommandIntent(prompt) {
		commands := agentExtractCommands(prompt)
		if len(commands) == 0 {
			if cmd := agentExtractCommand(prompt); cmd != "" {
				commands = []string{cmd}
			}
		}
		if len(commands) == 0 {
			return false
		}
		for _, cmd := range commands {
			if !agentPlannerCommandAllowed(cmd) {
				return false
			}
		}
		return true
	}
	if agentIsWorkspaceIntent(prompt) {
		return (agentIsWorkspaceListIntent(prompt) || agentIsWorkspaceTreeIntent(prompt) || agentIsWorkspaceSearchIntent(prompt) || agentIsWorkspaceReadIntent(prompt) || agentIsWorkspacePrepareOnlyIntent(prompt)) &&
			!agentIsWorkspaceWriteIntent(prompt) &&
			!agentIsWorkspacePatchIntent(prompt) &&
			!agentIsWorkspaceDiffIntent(prompt) &&
			!agentIsWorkspaceApplyIntent(prompt) &&
			!agentIsWorkspaceApplyAllIntent(prompt) &&
			!agentIsWorkspaceDiscardIntent(prompt) &&
			!agentIsWorkspaceDiscardAllIntent(prompt) &&
			!agentIsWorkspaceRebaseIntent(prompt)
	}
	return agentIsRunContextAnalysisIntent(prompt) ||
		agentIsHealthIntent(prompt) ||
		agentIsPluginIntent(prompt) ||
		agentIsRunIntent(prompt) ||
		agentIsApprovalIntent(prompt)
}

func agentIsWorkspacePrepareOnlyIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(workspace|工作空间)`).MatchString(prompt) &&
		!agentIsWorkspaceListIntent(prompt) &&
		!agentIsWorkspaceTreeIntent(prompt) &&
		!agentIsWorkspaceSearchIntent(prompt) &&
		!agentIsWorkspaceReadIntent(prompt)
}

func tagAgentAutoContinueMessages(messages []agentMessage, calls []agentToolCall, prompt string) ([]agentMessage, []agentToolCall) {
	metadata := map[string]interface{}{
		"phase":                "auto_continue",
		"auto_continue":        true,
		"auto_continue_prompt": trimSummary(prompt, 220),
	}
	for i := range calls {
		calls[i] = withAgentToolMetadata(calls[i], metadata)
	}
	for i := range messages {
		for j := range messages[i].ToolCalls {
			messages[i].ToolCalls[j] = withAgentToolMetadata(messages[i].ToolCalls[j], metadata)
		}
	}
	return messages, calls
}
