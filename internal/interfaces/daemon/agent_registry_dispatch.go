package daemon

import (
	"context"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
)

type agentRegistryHandlerResult struct {
	Messages  []agentMessage
	ToolCalls []agentToolCall
}

type agentRegistryHandlerRuntime struct {
	Manager              *agentManager
	Services             routerServices
	AuthProfiles         []agentHTTPAuthProfile
	DefaultAuthProfileID string
}

func (m *agentManager) executeAgentRegistryHandler(ctx context.Context, sessionID, language, operator, operationLevel string, svc routerServices, spec *toolregistry.ToolSpec, args map[string]any, authProfiles []agentHTTPAuthProfile, defaultAuthProfileID string) ([]agentMessage, []agentToolCall, error) {
	if spec == nil || spec.Handler == nil {
		return nil, nil, nil
	}
	output, err := spec.Handler(ctx, toolregistry.HandlerInput{
		SessionID:      sessionID,
		Operator:       operator,
		Language:       language,
		OperationLevel: operationLevel,
		RawArgs:        cloneMapStringAny(args),
		Runtime: agentRegistryHandlerRuntime{
			Manager:              m,
			Services:             svc,
			AuthProfiles:         append([]agentHTTPAuthProfile(nil), authProfiles...),
			DefaultAuthProfileID: defaultAuthProfileID,
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return m.registryHandlerOutputToMessages(language, spec.Name, output)
}

func (m *agentManager) registryHandlerOutputToMessages(language, toolName string, output any) ([]agentMessage, []agentToolCall, error) {
	switch result := output.(type) {
	case agentRegistryHandlerResult:
		return result.Messages, result.ToolCalls, nil
	case *agentRegistryHandlerResult:
		if result == nil {
			return nil, nil, nil
		}
		return result.Messages, result.ToolCalls, nil
	case agentMessage:
		return []agentMessage{result}, nil, nil
	case []agentMessage:
		return result, nil, nil
	case agentToolCall:
		return []agentMessage{m.message(language, "tool", agentText(language, "工具已通过 registry handler 执行。", "Tool executed through registry handler."), []agentToolCall{result}, nil)}, []agentToolCall{result}, nil
	case []agentToolCall:
		return []agentMessage{m.message(language, "tool", agentText(language, "工具已通过 registry handler 执行。", "Tool executed through registry handler."), result, nil)}, result, nil
	default:
		tool := runningAgentToolCall(toolName, "registry.handler")
		tool = finishAgentToolCall(tool, nil)
		if result, ok := output.(map[string]any); ok {
			tool.Result = result
		} else {
			tool.Result = map[string]any{"result": output}
		}
		return []agentMessage{m.message(language, "tool", agentText(language, "工具已通过 registry handler 执行。", "Tool executed through registry handler."), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
}
