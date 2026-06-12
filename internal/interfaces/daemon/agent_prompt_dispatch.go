package daemon

import (
	"context"
	"regexp"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func (m *agentManager) executeAgentPlannerActionResultWithToolServices(ctx context.Context, sessionID, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding, action agentPlannerAction, authorization runtime.AgentLoopAuthorization) (runtime.AgentLoopActionExecutorResult, error) {
	name := normalizeAgentPlannerActionName(action.Name)
	args := action.Arguments
	if args == nil {
		args = map[string]any{}
	}

	if spec, ok := toolRegistry().Lookup(name); ok {
		if spec.Handler == nil {
			err := services.NewError(services.CodeInternal, "planner action handler is not configured", map[string]any{"action": name}, nil)
			return daemonPlannerActionExecutionResultFromRuntime(nil, nil, err), err
		}
		return m.executeAgentRegistryHandlerResultWithToolServices(ctx, sessionID, language, operator, operationLevel, toolServices, spec, args, authorization)
	}

	err := services.NewError(services.CodeUnsupported, "unsupported planner action", map[string]any{"action": action.Name, "normalized_action": name}, nil)
	return daemonPlannerActionExecutionResultFromRuntime(nil, nil, err), err
}

func (m *agentManager) routeReadOnlySystemHeuristicActionsWithToolServices(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding) ([]agentMessage, []agentToolCall, error) {
	return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
}

func (m *agentManager) routeHeuristicRegistryActionsWithToolServices(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding, authorization runtime.AgentLoopAuthorization, allowed func(string, map[string]any) bool, sliceName string) ([]agentMessage, []agentToolCall, error) {
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
		loopReq := agentPlannerActionExecutionRequestFromRuntime(
			sessionID,
			agentRuntimeTurnRef{},
			prompt,
			language,
			operator,
			operationLevel,
			toolServices,
			authorization,
		)
		loopReq.Mode = runtime.AgentLoopModePlannerActionExecution
		loopReq.Action = agentPlannerActionReplanCyclePlanActionFromAgentPlannerAction(action)
		actionResult, err := daemonAgentLoopActionExecutor{manager: m}.ExecuteAgentLoopAction(ctx, runtime.AgentLoopActionExecutorRequest{
			Request: loopReq,
			Action:  loopReq.Action,
		})
		if err != nil {
			tool := failedAgentToolCall(firstNonEmpty(actionName, "agent.action"), err)
			tool.Result = map[string]any{"action": actionName}
			messages = append(messages, m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil))
			toolCalls = append(toolCalls, tool)
			return messages, toolCalls, nil
		}
		actionMessages := agentMessagesFromAgentLoopMessages(actionResult.Messages)
		actionToolCalls := agentLoopToolCallItemsToAgentToolCalls(actionResult.ToolCalls)
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

func agentAdvancedHTTPRouteAction(actionName string, args map[string]any) bool {
	if normalizeAgentPlannerActionName(actionName) != "http.request" {
		return false
	}
	if agentHTTPMethodIsSafe(firstNonEmpty(stringFromAny(args["method"]), "GET")) {
		return false
	}
	return strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["url"]),
		stringFromAny(args["target"]),
		stringFromAny(args["input"]),
	)) != ""
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

func agentWorkspaceMutationRouteAction(actionName string, args map[string]any) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "workspace.files.write", "workspace.files.patch":
		return strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)) != ""
	case "workspace.files.apply", "workspace.files.discard":
		changeID := strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["change_id"]),
			stringFromAny(args["id"]),
			stringFromAny(args["input"]),
		))
		groupID := strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["group_id"]),
			stringFromAny(args["group"]),
		))
		return changeID != "" ||
			groupID != "" ||
			boolFromAny(args["all"]) ||
			strings.EqualFold(stringFromAny(args["scope"]), "all")
	case "workspace.files.delete", "workspace.files.mkdir":
		return strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			stringFromAny(args["input"]),
		)) != ""
	case "workspace.files.move", "workspace.files.copy":
		fromPath := strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["from"]),
			stringFromAny(args["source"]),
			stringFromAny(args["old"]),
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		))
		toPath := strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["to"]),
			stringFromAny(args["target"]),
			stringFromAny(args["new"]),
			stringFromAny(args["dest"]),
			stringFromAny(args["destination"]),
		))
		return fromPath != "" && toPath != ""
	default:
		return false
	}
}

func agentApprovalOperationRouteAction(actionName string, args map[string]any) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "runs.approvals.approve", "runs.approvals.deny":
		approvalID := strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["approval_id"]),
			stringFromAny(args["approval"]),
			stringFromAny(args["step_id"]),
			stringFromAny(args["step"]),
		))
		runID := strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		))
		return approvalID != "" || runID != "" || boolFromAny(args["current"]) || boolFromAny(args["active"])
	case "runs.resume_approved":
		runID := strings.TrimSpace(firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		))
		return runID != "" || boolFromAny(args["current"]) || boolFromAny(args["active"])
	default:
		return false
	}
}

func agentReportDraftRouteAction(actionName string, _ map[string]any) bool {
	return normalizeAgentPlannerActionName(actionName) == "reports.draft"
}

func agentScheduleRouteAction(actionName string, _ map[string]any) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "schedule.create", "schedule.list", "schedule.delete":
		return true
	default:
		return false
	}
}

func (m *agentManager) routePromptWithToolServices(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding) (messages []agentMessage, toolCalls []agentToolCall, err error) {
	return m.routePromptForTurnWithToolServices(ctx, sessionID, agentRuntimeTurnRef{}, prompt, language, operator, operationLevel, toolServices)
}

func (m *agentManager) routePromptForTurnWithToolServices(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operator, operationLevel string, toolServices runtime.AgentLoopToolServiceBinding) (messages []agentMessage, toolCalls []agentToolCall, err error) {
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
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentScheduleRouteAction, "schedule registry route slice")
	case agentIsScheduleListIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentScheduleRouteAction, "schedule registry route slice")
	case agentIsScheduleDeleteIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentScheduleRouteAction, "schedule registry route slice")
	case agentIsResumeApprovedIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentApprovalOperationRouteAction, "approval operation registry route slice")
	case agentIsApprovalDecisionIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentApprovalOperationRouteAction, "approval operation registry route slice")
	case agentIsReportDraftIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReportDraftRouteAction, "report draft registry route slice")
	case agentIsRunContextAnalysisIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsPortScanIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentSafePortScanRouteAction, "safe port scan registry route slice")
	case agentHasCapabilityGapIntent(prompt):
		plan := agentCapabilityGapPlannerFromPrompt(prompt, "", language)
		if plan != nil {
			message := agentMessageWithPhase(m.message(language, "agent", plan.Answer, nil, nil), "plan")
			message.Reasoning = trimSummary(plan.Reasoning, 800)
			m.appendMessagesWithPlannerForTurn(ctx, sessionID, turnRef, prompt, daemonTurnExecutionDeterministicRoute, operationLevel, plan)
			return []agentMessage{message}, nil, nil
		}
		return []agentMessage{m.message(language, "agent", agentExplain(language), nil, nil)}, nil, nil
	case agentIsScanIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentSafeScanRouteAction, "safe scan registry route slice")
	case agentIsCommandIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsWorkspaceDiffIntent(prompt) || agentIsWorkspaceRebaseIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentWorkspaceStagedChangeRouteAction, "workspace staged-change registry route slice")
	case agentIsWorkspaceWriteIntent(prompt) ||
		agentIsWorkspacePatchIntent(prompt) ||
		agentIsWorkspaceApplyIntent(prompt) ||
		agentIsWorkspaceApplyAllIntent(prompt) ||
		agentIsWorkspaceDiscardIntent(prompt) ||
		agentIsWorkspaceDiscardAllIntent(prompt) ||
		agentIsWorkspaceDeleteIntent(prompt) ||
		agentIsWorkspaceMoveIntent(prompt) ||
		agentIsWorkspaceCopyIntent(prompt) ||
		agentIsWorkspaceMkdirIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentWorkspaceMutationRouteAction, "workspace mutation registry route slice")
	case agentIsApprovalIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsHealthIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsPluginIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsRunIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt):
		if agentHTTPMethodIsSafe(agentExtractHTTPMethod(prompt)) {
			routeMetadataSource = "deterministic_route/registry_dispatch"
			return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentSafeHTTPRouteAction, "safe HTTP registry route slice")
		}
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentAdvancedHTTPRouteAction, "advanced HTTP registry route slice")
	case agentIsWorkspaceSearchIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsWorkspaceTreeIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsWorkspaceReadIntent(prompt) || agentIsWorkspaceListIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
	case agentIsWorkspaceIntent(prompt):
		routeMetadataSource = "deterministic_route/registry_dispatch"
		return m.routeHeuristicRegistryActionsWithToolServices(ctx, sessionID, prompt, language, operator, operationLevel, toolServices, runtime.AgentLoopAuthorization{}, agentReadOnlySystemRouteAction, "read-only/system registry route slice")
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
