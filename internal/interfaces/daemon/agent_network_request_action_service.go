package daemon

import (
	"context"
	"errors"
)

type daemonNetworkRequestActionService interface {
	HTTPRequest(ctx context.Context, method, targetURL string, requestHeaders map[string]string, requestBodyText, language string, authProfile *agentHTTPAuthProfileUsage) ([]agentMessage, []agentToolCall, error)
	WebFetch(ctx context.Context, language string, args map[string]any) ([]agentMessage, []agentToolCall, error)
}

type daemonNetworkRequestManagerActionService struct {
	manager *agentManager
}

func daemonNetworkRequestActionServiceForManager(manager *agentManager) daemonNetworkRequestActionService {
	if manager == nil {
		return nil
	}
	return daemonNetworkRequestManagerActionService{manager: manager}
}

func (service daemonNetworkRequestManagerActionService) requireManager() (*agentManager, error) {
	if service.manager == nil {
		return nil, errors.New("daemon: network request action service has no manager")
	}
	return service.manager, nil
}

func (service daemonNetworkRequestManagerActionService) HTTPRequest(ctx context.Context, method, targetURL string, requestHeaders map[string]string, requestBodyText, language string, authProfile *agentHTTPAuthProfileUsage) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentHTTPRequest(ctx, method, targetURL, requestHeaders, requestBodyText, language, authProfile)
}

func (service daemonNetworkRequestManagerActionService) WebFetch(ctx context.Context, language string, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWebFetch(ctx, language, cloneMapStringAny(args))
}
