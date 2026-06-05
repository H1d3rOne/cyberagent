package daemon

import (
	"context"
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

	if spec, ok := toolRegistry().Lookup(name); ok {
		if spec.Handler == nil {
			return nil, nil, services.NewError(services.CodeInternal, "planner action handler is not configured", map[string]any{"action": name}, nil)
		}
		return m.executeAgentRegistryHandler(ctx, sessionID, language, operator, operationLevel, svc, spec, args, authProfiles, defaultAuthProfileID)
	}

	return nil, nil, services.NewError(services.CodeUnsupported, "unsupported planner action", map[string]any{"action": action.Name, "normalized_action": name}, nil)
}

func (m *agentManager) routeReadOnlySystemHeuristicActions(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	return m.routeHeuristicRegistryActions(ctx, sessionID, prompt, language, operator, operationLevel, svc, nil, "", agentReadOnlySystemRouteAction, "read-only/system registry route slice")
}

func (m *agentManager) routeSafeHTTPHeuristicActions(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	return m.routeHeuristicRegistryActions(ctx, sessionID, prompt, language, operator, operationLevel, svc, nil, "", agentSafeHTTPRouteAction, "safe HTTP registry route slice")
}

func (m *agentManager) routeSafePortScanHeuristicActions(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	return m.routeHeuristicRegistryActions(ctx, sessionID, prompt, language, operator, operationLevel, svc, nil, "", agentSafePortScanRouteAction, "safe port scan registry route slice")
}

func (m *agentManager) routeSafeScanHeuristicActions(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	return m.routeHeuristicRegistryActions(ctx, sessionID, prompt, language, operator, operationLevel, svc, nil, "", agentSafeScanRouteAction, "safe scan registry route slice")
}

func (m *agentManager) routeWorkspaceStagedChangeHeuristicActions(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	return m.routeHeuristicRegistryActions(ctx, sessionID, prompt, language, operator, operationLevel, svc, nil, "", agentWorkspaceStagedChangeRouteAction, "workspace staged-change registry route slice")
}

func (m *agentManager) routeReportDraftHeuristicActions(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	return m.routeHeuristicRegistryActions(ctx, sessionID, prompt, language, operator, operationLevel, svc, nil, "", agentReportDraftRouteAction, "report draft registry route slice")
}

func (m *agentManager) routeHeuristicRegistryActions(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices, authProfiles []agentHTTPAuthProfile, defaultAuthProfileID string, allowed func(string, map[string]any) bool, sliceName string) ([]agentMessage, []agentToolCall, error) {
	actions := agentHeuristicActionsFromPrompt(prompt)
	if len(actions) == 0 {
		return []agentMessage{m.message(language, "agent", agentExplain(language), nil, nil)}, nil, nil
	}
	if len(actions) > 6 {
		actions = actions[:6]
	}

	messages := make([]agentMessage, 0, len(actions))
	toolCalls := make([]agentToolCall, 0, len(actions))
	for _, action := range actions {
		actionName := normalizeAgentPlannerActionName(action.Name)
		if action.Arguments == nil {
			action.Arguments = map[string]any{}
		}
		if allowed == nil || !allowed(actionName, action.Arguments) {
			return nil, nil, services.NewError(services.CodeUnsupported, "heuristic action is outside the "+sliceName, map[string]any{"action": action.Name, "normalized_action": actionName}, nil)
		}
		actionMessages, actionToolCalls, err := m.executeAgentPlannerAction(ctx, sessionID, language, operator, operationLevel, svc, action, authProfiles, defaultAuthProfileID)
		if err != nil {
			tool := failedAgentToolCall(firstNonEmpty(actionName, "agent.action"), err)
			tool.Result = map[string]any{"action": actionName}
			messages = append(messages, m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil))
			toolCalls = append(toolCalls, tool)
			return messages, toolCalls, nil
		}
		messages = append(messages, actionMessages...)
		toolCalls = append(toolCalls, actionToolCalls...)
	}
	return messages, toolCalls, nil
}

func tagAgentLocalRouteMessages(messages []agentMessage, calls []agentToolCall, source string) ([]agentMessage, []agentToolCall) {
	source = strings.TrimSpace(source)
	if source == "" {
		return messages, calls
	}
	metadata := map[string]interface{}{"source": source}
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

func agentReadOnlySystemRouteAction(actionName string, _ map[string]any) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "workspace.command",
		"workspace.files.list",
		"workspace.files.tree",
		"workspace.files.read",
		"workspace.search",
		"daemon.health",
		"plugins.list",
		"runs.list",
		"runs.context",
		"runs.approvals.list",
		"schedule.list":
		return true
	default:
		return false
	}
}

func agentSafeHTTPRouteAction(actionName string, args map[string]any) bool {
	if normalizeAgentPlannerActionName(actionName) != "http.request" {
		return false
	}
	return agentHTTPMethodIsSafe(firstNonEmpty(stringFromAny(args["method"]), "GET"))
}

func agentSafePortScanRouteAction(actionName string, args map[string]any) bool {
	if normalizeAgentPlannerActionName(actionName) != "ports.scan.safe" {
		return false
	}
	return strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["host"]),
		agentExtractPortScanHostFromPrompt(stringFromAny(args["target"])),
		stringFromAny(args["target"]),
		stringFromAny(args["input"]),
	)) != ""
}

func agentSafeScanRouteAction(actionName string, args map[string]any) bool {
	if normalizeAgentPlannerActionName(actionName) != "scan.web.safe" {
		return false
	}
	return strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["target"]),
		stringFromAny(args["url"]),
		stringFromAny(args["input"]),
	)) != ""
}

func agentWorkspaceStagedChangeRouteAction(actionName string, args map[string]any) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "workspace.files.diff":
		return strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)) != ""
	case "workspace.files.diff_many":
		return len(workspaceChangeInputsFromAny(args["changes"])) > 0 || len(workspaceChangeInputsFromAny(args["input"])) > 0
	case "workspace.files.rebase":
		return strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["change_id"]),
			stringFromAny(args["id"]),
		)) != ""
	default:
		return false
	}
}

func agentReportDraftRouteAction(actionName string, _ map[string]any) bool {
	return normalizeAgentPlannerActionName(actionName) == "reports.draft"
}

func (m *agentManager) routePrompt(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) (messages []agentMessage, toolCalls []agentToolCall, err error) {
	routeMetadataSource := "deterministic_route/direct_handler"
	defer func() {
		if err == nil {
			messages, toolCalls = tagAgentLocalRouteMessages(messages, toolCalls, routeMetadataSource)
		}
	}()

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
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsScheduleDeleteIntent(prompt):
		return m.handleAgentScheduleDelete(language, svc, map[string]any{"id": agentExtractScheduleID(prompt)})
	case agentIsResumeApprovedIntent(prompt):
		return m.handleAgentResumeApproved(ctx, sessionID, prompt, language, operator, svc)
	case agentIsApprovalDecisionIntent(prompt):
		return m.handleAgentApprovalDecision(ctx, sessionID, prompt, language, operator, svc)
	case agentIsReportDraftIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReportDraftHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsRunContextAnalysisIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsPortScanIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeSafePortScanHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
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
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeSafeScanHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsCommandIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsApprovalIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsHealthIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsPluginIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsRunIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt):
		if agentHTTPMethodIsSafe(agentExtractHTTPMethod(prompt)) {
			routeMetadataSource = "deterministic_route/registry_dispatch"
			return m.routeSafeHTTPHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
		}
		return m.handleAgentHTTPRequest(ctx, agentExtractHTTPMethod(prompt), agentExtractURL(prompt), nil, "", language, nil)
	case agentIsWorkspaceDiffIntent(prompt) || agentIsWorkspaceRebaseIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeWorkspaceStagedChangeHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsWorkspaceSearchIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsWorkspaceTreeIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	case agentIsWorkspaceReadIntent(prompt) || agentIsWorkspaceListIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeReadOnlySystemHeuristicActions(ctx, sessionID, prompt, language, operator, operationLevel, svc)
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
