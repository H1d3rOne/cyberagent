package daemon

import (
	"context"
	"errors"
)

type daemonPortScannerActionService interface {
	PortScan(ctx context.Context, prompt, language string) ([]agentMessage, []agentToolCall, error)
}

type daemonPortScannerManagerActionService struct {
	manager *agentManager
}

func daemonPortScannerActionServiceForManager(manager *agentManager) daemonPortScannerActionService {
	if manager == nil {
		return nil
	}
	return daemonPortScannerManagerActionService{manager: manager}
}

func (service daemonPortScannerManagerActionService) requireManager() (*agentManager, error) {
	if service.manager == nil {
		return nil, errors.New("daemon: port scanner action service has no manager")
	}
	return service.manager, nil
}

func (service daemonPortScannerManagerActionService) PortScan(ctx context.Context, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentPortScan(ctx, prompt, language)
}
