package daemon

import (
	"context"
	"errors"
)

type daemonRunContextApprovalActionService interface {
	RunContext(ctx context.Context, sessionID, runID, contextKind, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
	ReportDraft(ctx context.Context, sessionID, runID, language string, resolver daemonAgentLoopActionServiceResolver, outputPath string) ([]agentMessage, []agentToolCall, error)
	ApprovalsList(ctx context.Context, runID, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
	ResumeApproved(ctx context.Context, sessionID, prompt, language, operator string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
	ApprovalDecision(ctx context.Context, sessionID, prompt, language, operator string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error)
}

type daemonRunContextApprovalManagerActionService struct {
	manager *agentManager
}

func daemonRunContextApprovalActionServiceForManager(manager *agentManager) daemonRunContextApprovalActionService {
	if manager == nil {
		return nil
	}
	return daemonRunContextApprovalManagerActionService{manager: manager}
}

func (service daemonRunContextApprovalManagerActionService) requireManager() (*agentManager, error) {
	if service.manager == nil {
		return nil, errors.New("daemon: run-context approval action service has no manager")
	}
	return service.manager, nil
}

func (service daemonRunContextApprovalManagerActionService) RunContext(ctx context.Context, sessionID, runID, contextKind, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentRunContextTool(ctx, sessionID, runID, contextKind, language, resolver)
}

func (service daemonRunContextApprovalManagerActionService) ReportDraft(ctx context.Context, sessionID, runID, language string, resolver daemonAgentLoopActionServiceResolver, outputPath string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentReportDraft(ctx, sessionID, runID, language, resolver, outputPath)
}

func (service daemonRunContextApprovalManagerActionService) ApprovalsList(ctx context.Context, runID, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	if runID == "" {
		return manager.handleAgentApprovals(ctx, language, resolver)
	}
	return manager.handleAgentRunContextAnalysis(ctx, "读取运行 "+runID+" 的 approvals 审批队列", language, resolver)
}

func (service daemonRunContextApprovalManagerActionService) ResumeApproved(ctx context.Context, sessionID, prompt, language, operator string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentResumeApproved(ctx, sessionID, prompt, language, operator, resolver)
}

func (service daemonRunContextApprovalManagerActionService) ApprovalDecision(ctx context.Context, sessionID, prompt, language, operator string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentApprovalDecision(ctx, sessionID, prompt, language, operator, resolver)
}
