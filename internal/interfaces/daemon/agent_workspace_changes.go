package daemon

import (
	"fmt"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func (m *agentManager) discardWorkspaceFileChange(sessionID, changeID string) (map[string]any, error) {
	changeID = agentCleanToken(changeID)
	if changeID == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.discard requires change_id", map[string]any{"field": "change_id"}, nil)
	}

	now := time.Now().UTC()
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	changeIndex := -1
	var change agentWorkspaceChange
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID {
			changeIndex = idx
			change = session.PendingChanges[idx]
			break
		}
	}
	if changeIndex < 0 {
		m.mu.Unlock()
		return nil, services.NewError(services.CodeNotFound, "workspace staged change not found", map[string]any{"change_id": changeID}, nil)
	}
	if change.Status != "pending" && change.Status != "conflict" {
		m.mu.Unlock()
		return nil, services.NewError(services.CodeConflict, "workspace staged change is not pending", map[string]any{"change_id": changeID, "status": change.Status}, nil)
	}
	change.Status = "discarded"
	change.Error = ""
	change.UpdatedAt = now
	session.PendingChanges[changeIndex] = change
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	result := workspaceChangeResult(sessionID, change)
	result["status"] = "discarded"
	return result, nil
}

func (m *agentManager) pendingWorkspaceChangeIDs(sessionID string) ([]string, error) {
	return m.workspaceChangeIDsByStatus(sessionID, "pending")
}

func (m *agentManager) discardableWorkspaceChangeIDs(sessionID string) ([]string, error) {
	return m.workspaceChangeIDsByStatus(sessionID, "pending", "conflict")
}

func (m *agentManager) workspaceChangeIDsByGroup(sessionID, groupID string, statuses ...string) ([]string, error) {
	changes, err := m.workspaceChangesByGroup(sessionID, groupID, statuses...)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(changes))
	for _, change := range changes {
		ids = append(ids, change.ID)
	}
	return ids, nil
}

func (m *agentManager) workspaceChangesByGroup(sessionID, groupID string, statuses ...string) ([]agentWorkspaceChange, error) {
	groupID = agentCleanToken(groupID)
	if groupID == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace change group requires group_id", map[string]any{"field": "group_id"}, nil)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return nil, loadErr
		}
		session = loaded
	}
	allowed := map[string]struct{}{}
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}
	changes := make([]agentWorkspaceChange, 0, len(session.PendingChanges))
	for idx := range session.PendingChanges {
		change := session.PendingChanges[idx]
		if change.GroupID != groupID || change.ID == "" {
			continue
		}
		if _, ok := allowed[change.Status]; ok {
			changes = append(changes, change)
		}
	}
	if len(changes) == 0 {
		return nil, services.NewError(services.CodeNotFound, "workspace change group has no matching changes", map[string]any{"group_id": groupID}, nil)
	}
	return changes, nil
}

func (m *agentManager) workspaceChangeIDsByStatus(sessionID string, statuses ...string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return nil, loadErr
		}
		session = loaded
	}
	allowed := map[string]struct{}{}
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}
	ids := make([]string, 0, len(session.PendingChanges))
	for idx := range session.PendingChanges {
		change := session.PendingChanges[idx]
		if _, ok := allowed[change.Status]; ok && change.ID != "" {
			ids = append(ids, change.ID)
		}
	}
	return ids, nil
}

func workspaceBatchResult(sessionID, action string, requested int, succeeded []map[string]any, failed []map[string]any) map[string]any {
	status := "succeeded"
	switch {
	case requested == 0:
		status = "empty"
	case len(failed) == requested:
		status = "failed"
	case len(failed) > 0:
		status = "partial"
	}
	return map[string]any{
		"session_id":      sessionID,
		"action":          action,
		"status":          status,
		"requested_count": requested,
		"succeeded_count": len(succeeded),
		"failed_count":    len(failed),
		"succeeded":       succeeded,
		"failed":          failed,
	}
}

func workspaceChangeResult(sessionID string, change agentWorkspaceChange) map[string]any {
	return map[string]any{
		"session_id":      sessionID,
		"change_id":       change.ID,
		"group_id":        change.GroupID,
		"path":            change.Path,
		"size_before":     change.SizeBefore,
		"size_after":      change.SizeAfter,
		"replacements":    change.Replacements,
		"diff_preview":    change.DiffPreview,
		"content_preview": change.ContentPreview,
		"status":          change.Status,
		"error":           change.Error,
		"created_at":      change.CreatedAt,
		"updated_at":      change.UpdatedAt,
	}
}

func (m *agentManager) handleAgentWorkspacePatch(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要修改的工作空间文件路径，例如：修改文件 notes.md old: hello new: world。",
			"Please provide the workspace file path to patch, for example: patch file notes.md old: hello new: world.",
		), nil, nil)}, nil, nil
	}
	if oldText == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要精确替换的 old 文本，例如：修改文件 notes.md old: hello new: world。",
			"Please provide the exact old text to replace, for example: patch file notes.md old: hello new: world.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.patch", relPath)
	result, err := m.patchWorkspaceFile(sessionID, relPath, oldText, newText)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "path": relPath}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result

	message := m.message(language, "tool", agentFormatWorkspacePatch(language, relPath, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDiff(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要生成 diff 的工作空间文件路径，例如：预览修改文件 notes.md old: hello new: world。",
			"Please provide the workspace file path to diff, for example: preview patch file notes.md old: hello new: world.",
		), nil, nil)}, nil, nil
	}
	if oldText == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要预览替换的 old 文本，例如：预览修改文件 notes.md old: hello new: world。",
			"Please provide the exact old text to preview, for example: preview patch file notes.md old: hello new: world.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.diff", relPath)
	result, err := m.stageWorkspaceFileChange(sessionID, relPath, oldText, newText, tool.ID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "path": relPath}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result

	message := m.message(language, "tool", agentFormatWorkspaceDiff(language, relPath, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDiffMany(sessionID string, changes []agentWorkspaceChangeInput, language string) ([]agentMessage, []agentToolCall, error) {
	if len(changes) == 0 {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供 changes 数组，例如：[{\"path\":\"a.md\",\"old\":\"before\",\"new\":\"after\"}]。",
			"Please provide a changes array, for example: [{\"path\":\"a.md\",\"old\":\"before\",\"new\":\"after\"}].",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.diff_many", fmt.Sprintf("%d changes", len(changes)))
	result, err := m.stageWorkspaceFileChanges(sessionID, changes, tool.ID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "count": len(changes)}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceDiffMany(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceRebase(sessionID, changeID, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	changeID = agentCleanToken(changeID)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if changeID == "" || oldText == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供 change_id 和当前文件中的精确 old 文本，例如：重新生成 change_id: change_xxx old: 当前文本。",
			"Please provide change_id and the current exact old text, for example: rebase change_id: change_xxx old: current text.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.rebase", changeID)
	result, err := m.rebaseWorkspaceFileChange(sessionID, changeID, oldText, newText, tool.ID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "change_id": changeID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceRebase(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceApply(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error) {
	changeID = agentCleanToken(changeID)
	if changeID == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要应用的 change_id，例如：应用 change_id: change_20260527_...。",
			"Please provide the change_id to apply, for example: apply change_id: change_20260527_...",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.apply", changeID)
	result, err := m.applyWorkspaceFileChange(sessionID, changeID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "change_id": changeID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result

	message := m.message(language, "tool", agentFormatWorkspaceApply(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceApplyAll(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("workspace.files.apply", "all")
	changeIDs, err := m.pendingWorkspaceChangeIDs(sessionID)
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "action": "apply_all"}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	applied := make([]map[string]any, 0, len(changeIDs))
	failed := make([]map[string]any, 0)
	for _, changeID := range changeIDs {
		result, applyErr := m.applyWorkspaceFileChange(sessionID, changeID)
		if applyErr != nil {
			failed = append(failed, map[string]any{
				"change_id": changeID,
				"code":      services.ErrorCodeOf(applyErr),
				"message":   applyErr.Error(),
			})
			continue
		}
		applied = append(applied, result)
	}

	result := workspaceBatchResult(sessionID, "apply_all", len(changeIDs), applied, failed)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceBatch(language, "apply_all", result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) applyWorkspaceChangeGroup(sessionID, groupID string) (map[string]any, error) {
	changes, err := m.workspaceChangesByGroup(sessionID, groupID, "pending")
	if err != nil {
		return nil, err
	}
	prepared := make([]preparedWorkspaceApply, 0, len(changes))
	failed := make([]map[string]any, 0)
	for _, change := range changes {
		item, prepareErr := m.prepareWorkspaceChangeApply(sessionID, change)
		if prepareErr != nil {
			if services.ErrorCodeOf(prepareErr) == services.CodeConflict {
				m.markWorkspaceChangeConflict(sessionID, change.ID, prepareErr)
			}
			failed = append(failed, map[string]any{
				"change_id": change.ID,
				"path":      change.Path,
				"code":      services.ErrorCodeOf(prepareErr),
				"message":   prepareErr.Error(),
			})
			continue
		}
		prepared = append(prepared, item)
	}
	if len(failed) > 0 {
		return nil, services.NewError(services.CodeConflict, "workspace change group preflight failed; no files were modified", map[string]any{
			"group_id": groupID,
			"failed":   failed,
		}, nil)
	}

	written := make([]preparedWorkspaceApply, 0, len(prepared))
	for _, item := range prepared {
		if writeErr := m.workspace.WriteFile(sessionID, item.Change.Path, []byte(item.After)); writeErr != nil {
			for i := len(written) - 1; i >= 0; i-- {
				_ = m.workspace.WriteFile(sessionID, written[i].Change.Path, []byte(written[i].Before))
			}
			return nil, services.NewError(services.CodeInternal, "workspace change group apply failed; attempted rollback", map[string]any{
				"group_id": groupID,
				"path":     item.Change.Path,
			}, writeErr)
		}
		written = append(written, item)
	}

	appliedChanges := make([]agentWorkspaceChange, 0, len(prepared))
	applied := make([]map[string]any, 0, len(prepared))
	now := time.Now().UTC()
	for _, item := range prepared {
		change := item.Change
		change.Status = "applied"
		change.Error = ""
		change.UpdatedAt = now
		change.AppliedAt = &now
		appliedChanges = append(appliedChanges, change)
		applied = append(applied, workspaceChangeResult(sessionID, change))
	}
	if err := m.markWorkspaceChangesApplied(sessionID, appliedChanges); err != nil {
		return nil, err
	}
	result := workspaceBatchResult(sessionID, "apply_group", len(changes), applied, failed)
	result["group_id"] = groupID
	result["atomic"] = true
	return result, nil
}

func (m *agentManager) handleAgentWorkspaceApplyGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error) {
	groupID = agentCleanToken(groupID)
	if groupID == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要应用的 group_id，例如：应用 group_id: changeset_20260527_...。",
			"Please provide the group_id to apply, for example: apply group_id: changeset_20260527_...",
		), nil, nil)}, nil, nil
	}
	tool := runningAgentToolCall("workspace.files.apply", groupID)
	result, err := m.applyWorkspaceChangeGroup(sessionID, groupID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "group_id": groupID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceBatch(language, "apply_group", result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDiscard(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error) {
	changeID = agentCleanToken(changeID)
	if changeID == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要丢弃的 change_id，例如：丢弃 change_id: change_20260527_...。",
			"Please provide the change_id to discard, for example: discard change_id: change_20260527_...",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.discard", changeID)
	result, err := m.discardWorkspaceFileChange(sessionID, changeID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "change_id": changeID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result

	message := m.message(language, "tool", agentFormatWorkspaceDiscard(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) discardWorkspaceChangeGroup(sessionID, groupID string) (map[string]any, error) {
	changeIDs, err := m.workspaceChangeIDsByGroup(sessionID, groupID, "pending", "conflict")
	if err != nil {
		return nil, err
	}
	discarded := make([]map[string]any, 0, len(changeIDs))
	failed := make([]map[string]any, 0)
	for _, changeID := range changeIDs {
		result, discardErr := m.discardWorkspaceFileChange(sessionID, changeID)
		if discardErr != nil {
			failed = append(failed, map[string]any{
				"change_id": changeID,
				"code":      services.ErrorCodeOf(discardErr),
				"message":   discardErr.Error(),
			})
			continue
		}
		discarded = append(discarded, result)
	}
	result := workspaceBatchResult(sessionID, "discard_group", len(changeIDs), discarded, failed)
	result["group_id"] = groupID
	return result, nil
}

func (m *agentManager) handleAgentWorkspaceDiscardGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error) {
	groupID = agentCleanToken(groupID)
	if groupID == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要丢弃的 group_id，例如：丢弃 group_id: changeset_20260527_...。",
			"Please provide the group_id to discard, for example: discard group_id: changeset_20260527_...",
		), nil, nil)}, nil, nil
	}
	tool := runningAgentToolCall("workspace.files.discard", groupID)
	result, err := m.discardWorkspaceChangeGroup(sessionID, groupID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "group_id": groupID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceBatch(language, "discard_group", result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDiscardAll(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("workspace.files.discard", "all")
	changeIDs, err := m.discardableWorkspaceChangeIDs(sessionID)
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "action": "discard_all"}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	discarded := make([]map[string]any, 0, len(changeIDs))
	failed := make([]map[string]any, 0)
	for _, changeID := range changeIDs {
		result, discardErr := m.discardWorkspaceFileChange(sessionID, changeID)
		if discardErr != nil {
			failed = append(failed, map[string]any{
				"change_id": changeID,
				"code":      services.ErrorCodeOf(discardErr),
				"message":   discardErr.Error(),
			})
			continue
		}
		discarded = append(discarded, result)
	}

	result := workspaceBatchResult(sessionID, "discard_all", len(changeIDs), discarded, failed)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceBatch(language, "discard_all", result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func agentDiffPreview(path, oldText, newText string) string {
	oldLines := strings.Split(strings.TrimRight(oldText, "\n"), "\n")
	newLines := strings.Split(strings.TrimRight(newText, "\n"), "\n")
	lines := []string{"--- " + path, "+++ " + path}
	for _, line := range oldLines {
		if line == "" && len(oldLines) == 1 {
			continue
		}
		lines = append(lines, "-"+line)
	}
	for _, line := range newLines {
		if line == "" && len(newLines) == 1 {
			continue
		}
		lines = append(lines, "+"+line)
	}
	return agentLimitBlock(strings.Join(lines, "\n"), 1600)
}
