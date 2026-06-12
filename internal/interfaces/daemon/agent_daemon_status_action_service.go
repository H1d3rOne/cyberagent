package daemon

import (
	"context"
	"errors"
)

type daemonStatusActionService interface {
	Health(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
	Plugins(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
	Runs(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
}

type daemonStatusManagerActionService struct {
	manager *agentManager
}

func daemonStatusActionServiceForManager(manager *agentManager) daemonStatusActionService {
	if manager == nil {
		return nil
	}
	return daemonStatusManagerActionService{manager: manager}
}

func (service daemonStatusManagerActionService) requireManager() (*agentManager, error) {
	if service.manager == nil {
		return nil, errors.New("daemon: status action service has no manager")
	}
	return service.manager, nil
}

func (service daemonStatusManagerActionService) Health(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentHealth(ctx, language, resolver)
}

func (service daemonStatusManagerActionService) Plugins(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentPlugins(ctx, language, resolver)
}

func (service daemonStatusManagerActionService) Runs(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentRuns(ctx, language, resolver)
}
