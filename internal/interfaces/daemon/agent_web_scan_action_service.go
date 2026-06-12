package daemon

import (
	"context"
	"errors"
)

type daemonWebScanActionService interface {
	WebScan(ctx context.Context, prompt, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
}

type daemonWebScanManagerActionService struct {
	manager *agentManager
}

func daemonWebScanActionServiceForManager(manager *agentManager) daemonWebScanActionService {
	if manager == nil {
		return nil
	}
	return daemonWebScanManagerActionService{manager: manager}
}

func (service daemonWebScanManagerActionService) requireManager() (*agentManager, error) {
	if service.manager == nil {
		return nil, errors.New("daemon: web scan action service has no manager")
	}
	return service.manager, nil
}

func (service daemonWebScanManagerActionService) WebScan(ctx context.Context, prompt, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentScan(ctx, prompt, language, resolver)
}
