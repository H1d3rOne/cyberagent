package daemon

import (
	"context"
	"strings"
	"time"

	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

type agentRegistryHandlerResult struct {
	Messages  []agentMessage
	ToolCalls []agentToolCall
}

type agentRegistryHandlerRuntime struct {
	Manager              *agentManager
	Services             agentruntime.AgentLoopToolServiceBinding
	Authorization        agentruntime.AgentLoopAuthorization
	AuthProfiles         []agentHTTPAuthProfile
	DefaultAuthProfileID string
}

func (rt agentRegistryHandlerRuntime) actionServiceResolver() (daemonAgentLoopActionServiceResolver, bool) {
	return agentLoopActionServiceResolverFromActionServices(rt.Services.ActionServices)
}

func (rt agentRegistryHandlerRuntime) authorizationContext() agentruntime.AgentLoopAuthorization {
	authorization := rt.Authorization.Clone()
	if len(authorization.AuthProfiles) == 0 && authorization.DefaultAuthProfileID == "" && (len(rt.AuthProfiles) > 0 || rt.DefaultAuthProfileID != "") {
		authorization = agentruntime.AgentLoopAuthorization{
			AuthProfiles:         agentPlannerActionReplanCycleAuthProfilesFromHTTPProfiles(rt.AuthProfiles),
			DefaultAuthProfileID: rt.DefaultAuthProfileID,
		}
	}
	return authorization.Clone()
}

func (m *agentManager) executeAgentRegistryHandlerResultWithToolServices(ctx context.Context, sessionID, language, operator, operationLevel string, toolServices agentruntime.AgentLoopToolServiceBinding, spec *toolregistry.ToolSpec, args map[string]any, authorization agentruntime.AgentLoopAuthorization) (agentruntime.AgentLoopActionExecutorResult, error) {
	if spec == nil || spec.Handler == nil {
		return daemonPlannerActionExecutionResultFromRuntime(nil, nil, nil), nil
	}
	authorization = authorization.Clone()
	toolServices = agentLoopToolServiceBindingWithWorkspaceActionService(toolServices, daemonWorkspaceActionServiceForManager(m))
	toolServices = agentLoopToolServiceBindingWithNetworkRequestActionService(toolServices, daemonNetworkRequestActionServiceForManager(m))
	toolServices = agentLoopToolServiceBindingWithPortScannerActionService(toolServices, daemonPortScannerActionServiceForManager(m))
	toolServices = agentLoopToolServiceBindingWithWebSearchActionService(toolServices, daemonWebSearchActionServiceForManager(m))
	toolServices = agentLoopToolServiceBindingWithWebScanActionService(toolServices, daemonWebScanActionServiceForManager(m))
	toolServices = agentLoopToolServiceBindingWithScheduleActionService(toolServices, daemonScheduleActionServiceForManager(m))
	toolServices = agentLoopToolServiceBindingWithDaemonStatusActionService(toolServices, daemonStatusActionServiceForManager(m))
	toolServices = agentLoopToolServiceBindingWithRunContextApprovalActionService(toolServices, daemonRunContextApprovalActionServiceForManager(m))
	output, err := spec.Handler(ctx, toolregistry.HandlerInput{
		SessionID:      sessionID,
		Operator:       operator,
		Language:       language,
		OperationLevel: operationLevel,
		RawArgs:        cloneMapStringAny(args),
		Runtime: agentRegistryHandlerRuntime{
			Manager:              m,
			Services:             toolServices.Clone(),
			Authorization:        authorization,
			AuthProfiles:         agentPlannerActionReplanCycleAuthProfilesToHTTPProfiles(authorization.AuthProfiles),
			DefaultAuthProfileID: authorization.DefaultAuthProfileID,
		},
	})
	if err != nil {
		return daemonPlannerActionExecutionResultFromRuntime(nil, nil, err), err
	}
	return m.registryHandlerOutputToActionExecutorResult(language, spec.Name, output)
}

func (m *agentManager) registryHandlerOutputToActionExecutorResult(language, toolName string, output any) (agentruntime.AgentLoopActionExecutorResult, error) {
	switch result := output.(type) {
	case agentruntime.AgentLoopActionExecutorResult:
		return registryHandlerRuntimeActionExecutorResult(result), nil
	case *agentruntime.AgentLoopActionExecutorResult:
		if result == nil {
			return daemonPlannerActionExecutionResultFromRuntime(nil, nil, nil), nil
		}
		return registryHandlerRuntimeActionExecutorResult(*result), nil
	}

	messages, toolCalls, err := m.registryHandlerOutputToAgentLoopResult(language, toolName, output)
	if err != nil {
		return daemonPlannerActionExecutionResultFromRuntime(nil, nil, err), err
	}
	return daemonPlannerActionExecutionResultFromRuntime(messages, toolCalls, nil), nil
}

func registryHandlerRuntimeActionExecutorResult(result agentruntime.AgentLoopActionExecutorResult) agentruntime.AgentLoopActionExecutorResult {
	out := result.Clone()
	if out.Status == "" {
		out.Status = agentruntime.TurnResultFinished
		if out.Error != nil {
			out.Status = agentruntime.TurnResultFailed
		}
	}
	if out.Reason == "" {
		out.Reason = "daemon legacy planner action executor handled action"
		if out.Error != nil {
			out.Reason = out.Error.Error()
		}
	}
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	if _, exists := out.Metadata["executor"]; !exists {
		out.Metadata["executor"] = "daemon_legacy_planner_action_executor"
	}
	return out.Clone()
}

func (m *agentManager) registryHandlerOutputToAgentLoopResult(language, toolName string, output any) ([]agentruntime.AgentLoopMessage, []agentruntime.AgentLoopToolCall, error) {
	switch result := output.(type) {
	case agentRegistryHandlerResult:
		return agentLoopMessagesFromAgentMessages(result.Messages), agentLoopToolCallItemsFromAgentToolCalls(result.ToolCalls), nil
	case *agentRegistryHandlerResult:
		if result == nil {
			return nil, nil, nil
		}
		return agentLoopMessagesFromAgentMessages(result.Messages), agentLoopToolCallItemsFromAgentToolCalls(result.ToolCalls), nil
	case agentruntime.AgentLoopMessage:
		return []agentruntime.AgentLoopMessage{result.Clone()}, nil, nil
	case []agentruntime.AgentLoopMessage:
		return agentruntime.CloneAgentLoopMessages(result), nil, nil
	case agentruntime.AgentLoopToolCall:
		toolCalls := []agentruntime.AgentLoopToolCall{result.Clone()}
		return []agentruntime.AgentLoopMessage{registryHandlerRuntimeToolMessage(language, toolCalls)}, toolCalls, nil
	case []agentruntime.AgentLoopToolCall:
		toolCalls := agentruntime.CloneAgentLoopToolCalls(result)
		return []agentruntime.AgentLoopMessage{registryHandlerRuntimeToolMessage(language, toolCalls)}, toolCalls, nil
	case agentMessage:
		return []agentruntime.AgentLoopMessage{agentLoopMessageFromAgentMessage(result)}, nil, nil
	case []agentMessage:
		return agentLoopMessagesFromAgentMessages(result), nil, nil
	case agentToolCall:
		toolCalls := []agentruntime.AgentLoopToolCall{agentLoopToolCallFromAgentToolCall(result)}
		return []agentruntime.AgentLoopMessage{registryHandlerRuntimeToolMessage(language, toolCalls)}, toolCalls, nil
	case []agentToolCall:
		toolCalls := agentLoopToolCallItemsFromAgentToolCalls(result)
		return []agentruntime.AgentLoopMessage{registryHandlerRuntimeToolMessage(language, toolCalls)}, toolCalls, nil
	default:
		tool := runningRegistryHandlerAgentLoopToolCall(toolName, "registry.handler")
		tool = finishRegistryHandlerAgentLoopToolCall(tool, nil)
		if result, ok := output.(map[string]any); ok {
			tool.Result = cloneMapStringAny(result)
		} else {
			tool.Result = map[string]any{"result": output}
		}
		toolCalls := []agentruntime.AgentLoopToolCall{tool}
		return []agentruntime.AgentLoopMessage{registryHandlerRuntimeToolMessage(language, toolCalls)}, toolCalls, nil
	}
}

func registryHandlerRuntimeToolMessage(language string, toolCalls []agentruntime.AgentLoopToolCall) agentruntime.AgentLoopMessage {
	now := time.Now().UTC()
	return agentruntime.AgentLoopMessage{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "msg_", 1),
		Role:      "tool",
		Content:   agentText(language, "工具已通过 registry handler 执行。", "Tool executed through registry handler."),
		Timestamp: now,
		ToolCalls: agentruntime.CloneAgentLoopToolCalls(toolCalls),
	}
}

func runningRegistryHandlerAgentLoopToolCall(name, detail string) agentruntime.AgentLoopToolCall {
	now := time.Now().UTC()
	return agentruntime.AgentLoopToolCall{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "tool_", 1),
		Name:      name,
		Status:    contracts.StatusRunning,
		Detail:    detail,
		StartedAt: now,
		Metadata:  agentToolDefaultMetadata(name),
	}
}

func finishRegistryHandlerAgentLoopToolCall(tool agentruntime.AgentLoopToolCall, err error) agentruntime.AgentLoopToolCall {
	now := time.Now().UTC()
	tool.FinishedAt = &now
	if err != nil {
		tool.Status = contracts.StatusFailed
		return tool
	}
	tool.Status = contracts.StatusSucceeded
	return tool
}

func (m *agentManager) registryHandlerOutputToMessages(language, toolName string, output any) ([]agentMessage, []agentToolCall, error) {
	if result, ok := output.(agentruntime.AgentLoopActionExecutorResult); ok {
		result = registryHandlerRuntimeActionExecutorResult(result)
		return agentMessagesFromAgentLoopMessages(result.Messages), agentLoopToolCallItemsToAgentToolCalls(result.ToolCalls), nil
	}
	if result, ok := output.(*agentruntime.AgentLoopActionExecutorResult); ok {
		if result == nil {
			return nil, nil, nil
		}
		converted := registryHandlerRuntimeActionExecutorResult(*result)
		return agentMessagesFromAgentLoopMessages(converted.Messages), agentLoopToolCallItemsToAgentToolCalls(converted.ToolCalls), nil
	}
	messages, toolCalls, err := m.registryHandlerOutputToAgentLoopResult(language, toolName, output)
	return agentMessagesFromAgentLoopMessages(messages), agentLoopToolCallItemsToAgentToolCalls(toolCalls), err
}
