package daemon

import (
	"context"
	"errors"
)

type daemonWorkspaceActionService interface {
	Command(ctx context.Context, sessionID, command, language string) ([]agentMessage, []agentToolCall, error)
	List(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error)
	Tree(sessionID, relPath string, depth, limit int, includeHidden bool, language string) ([]agentMessage, []agentToolCall, error)
	Read(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error)
	Search(sessionID, query, relPath string, regexMode, caseSensitive bool, limit int, language string) ([]agentMessage, []agentToolCall, error)
	Write(sessionID, relPath, content, language string) ([]agentMessage, []agentToolCall, error)
	Patch(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error)
	Diff(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error)
	DiffMany(sessionID string, changes []agentWorkspaceChangeInput, language string) ([]agentMessage, []agentToolCall, error)
	Rebase(sessionID, changeID, oldText, newText, language string) ([]agentMessage, []agentToolCall, error)
	Apply(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error)
	ApplyAll(sessionID, language string) ([]agentMessage, []agentToolCall, error)
	ApplyGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error)
	Discard(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error)
	DiscardAll(sessionID, language string) ([]agentMessage, []agentToolCall, error)
	DiscardGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error)
	Delete(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error)
	Move(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error)
	Copy(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error)
	Mkdir(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error)
}

type daemonWorkspaceManagerActionService struct {
	manager *agentManager
}

func daemonWorkspaceActionServiceForManager(manager *agentManager) daemonWorkspaceActionService {
	if manager == nil {
		return nil
	}
	return daemonWorkspaceManagerActionService{manager: manager}
}

func (service daemonWorkspaceManagerActionService) requireManager() (*agentManager, error) {
	if service.manager == nil {
		return nil, errors.New("daemon: workspace action service has no manager")
	}
	return service.manager, nil
}

func (service daemonWorkspaceManagerActionService) Command(ctx context.Context, sessionID, command, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentCommand(ctx, sessionID, "command: "+command, language)
}

func (service daemonWorkspaceManagerActionService) List(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceList(sessionID, "path="+relPath, language)
}

func (service daemonWorkspaceManagerActionService) Tree(sessionID, relPath string, depth, limit int, includeHidden bool, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceTree(sessionID, relPath, depth, limit, includeHidden, language)
}

func (service daemonWorkspaceManagerActionService) Read(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceRead(sessionID, "file="+relPath, language)
}

func (service daemonWorkspaceManagerActionService) Search(sessionID, query, relPath string, regexMode, caseSensitive bool, limit int, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceSearch(sessionID, query, relPath, regexMode, caseSensitive, limit, language)
}

func (service daemonWorkspaceManagerActionService) Write(sessionID, relPath, content, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceWrite(sessionID, relPath, content, language)
}

func (service daemonWorkspaceManagerActionService) Patch(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspacePatch(sessionID, relPath, oldText, newText, language)
}

func (service daemonWorkspaceManagerActionService) Diff(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceDiff(sessionID, relPath, oldText, newText, language)
}

func (service daemonWorkspaceManagerActionService) DiffMany(sessionID string, changes []agentWorkspaceChangeInput, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceDiffMany(sessionID, changes, language)
}

func (service daemonWorkspaceManagerActionService) Rebase(sessionID, changeID, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceRebase(sessionID, changeID, oldText, newText, language)
}

func (service daemonWorkspaceManagerActionService) Apply(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceApply(sessionID, changeID, language)
}

func (service daemonWorkspaceManagerActionService) ApplyAll(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceApplyAll(sessionID, language)
}

func (service daemonWorkspaceManagerActionService) ApplyGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceApplyGroup(sessionID, groupID, language)
}

func (service daemonWorkspaceManagerActionService) Discard(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceDiscard(sessionID, changeID, language)
}

func (service daemonWorkspaceManagerActionService) DiscardAll(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceDiscardAll(sessionID, language)
}

func (service daemonWorkspaceManagerActionService) DiscardGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceDiscardGroup(sessionID, groupID, language)
}

func (service daemonWorkspaceManagerActionService) Delete(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceDelete(sessionID, relPath, language)
}

func (service daemonWorkspaceManagerActionService) Move(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceMove(sessionID, fromPath, toPath, language)
}

func (service daemonWorkspaceManagerActionService) Copy(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceCopy(sessionID, fromPath, toPath, language)
}

func (service daemonWorkspaceManagerActionService) Mkdir(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	manager, err := service.requireManager()
	if err != nil {
		return nil, nil, err
	}
	return manager.handleAgentWorkspaceMkdir(sessionID, relPath, language)
}
