package daemon

import (
	"context"
	"errors"
)

type daemonScheduleActionService interface {
	ScheduleCreate(ctx context.Context, sessionID, language, operator string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error)
	ScheduleList(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
	ScheduleDelete(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error)
}

type daemonScheduleManagerActionService struct {
	manager *agentManager
}

func daemonScheduleActionServiceForManager(manager *agentManager) daemonScheduleActionService {
	if manager == nil {
		return nil
	}
	return daemonScheduleManagerActionService{manager: manager}
}

func (service daemonScheduleManagerActionService) requireManager() (*agentManager, error) {
	if service.manager == nil {
		return nil, errors.New("daemon: schedule action service has no manager")
	}
	return service.manager, nil
}

func (service daemonScheduleManagerActionService) ScheduleCreate(ctx context.Context, sessionID, language, operator string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentScheduleCreate(ctx, sessionID, language, operator, resolver, cloneMapStringAny(args))
}

func (service daemonScheduleManagerActionService) ScheduleList(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentScheduleList(ctx, language, resolver)
}

func (service daemonScheduleManagerActionService) ScheduleDelete(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentScheduleDelete(ctx, language, resolver, cloneMapStringAny(args))
}
