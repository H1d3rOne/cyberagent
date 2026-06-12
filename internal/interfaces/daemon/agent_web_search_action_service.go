package daemon

import (
	"context"
	"errors"
)

type daemonWebSearchActionService interface {
	WebSearch(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error)
}

type daemonWebSearchManagerActionService struct {
	manager *agentManager
}

func daemonWebSearchActionServiceForManager(manager *agentManager) daemonWebSearchActionService {
	if manager == nil {
		return nil
	}
	return daemonWebSearchManagerActionService{manager: manager}
}

func (service daemonWebSearchManagerActionService) requireManager() (*agentManager, error) {
	if service.manager == nil {
		return nil, errors.New("daemon: web search action service has no manager")
	}
	return service.manager, nil
}

func (service daemonWebSearchManagerActionService) WebSearch(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWebSearch(ctx, language, resolver, cloneMapStringAny(args))
}
