package daemon

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

func (m *agentManager) handleAgentWorkspaceWrite(sessionID, relPath, content, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	content = agentCleanWorkspaceWriteContent(content)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要写入的工作空间文件路径，例如：写入文件 notes.md 内容: hello。",
			"Please provide the workspace file path to write, for example: write file notes.md content: hello.",
		), nil, nil)}, nil, nil
	}
	if strings.TrimSpace(content) == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要写入文件的内容，例如：写入文件 notes.md 内容: hello。",
			"Please provide the file content, for example: write file notes.md content: hello.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.write", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	data := []byte(content)
	err := m.workspace.WriteFile(sessionID, relPath, data)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id":      sessionID,
		"path":            relPath,
		"size":            len(data),
		"content_preview": agentLimitBlock(content, 500),
	}

	message := m.message(language, "tool", agentFormatWorkspaceWrite(language, relPath, content, len(data)), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDelete(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要删除的工作空间文件路径，例如：删除文件 notes.md。",
			"Please provide the workspace file path to delete, for example: delete file notes.md.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.delete", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	err := m.workspace.DeleteFile(sessionID, relPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id": sessionID,
		"path":       relPath,
		"status":     "deleted",
	}

	message := m.message(language, "tool", agentFormatWorkspaceDelete(language, relPath), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceMove(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error) {
	fromPath = agentCleanWorkspaceWritePath(fromPath)
	toPath = agentCleanWorkspaceWritePath(toPath)
	if fromPath == "" || fromPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要移动的源文件路径，例如：重命名文件 notes.md 为 archive/notes.md。",
			"Please provide the source file path to move, for example: rename file notes.md to archive/notes.md.",
		), nil, nil)}, nil, nil
	}
	if toPath == "" || toPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要移动到的目标文件路径，例如：重命名文件 notes.md 为 archive/notes.md。",
			"Please provide the destination file path to move to, for example: rename file notes.md to archive/notes.md.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.move", fromPath+" -> "+toPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	err := m.workspace.MoveFile(sessionID, fromPath, toPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id": sessionID,
		"from":       fromPath,
		"path":       toPath,
		"status":     "moved",
	}

	message := m.message(language, "tool", agentFormatWorkspaceMove(language, fromPath, toPath), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceCopy(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error) {
	fromPath = agentCleanWorkspaceWritePath(fromPath)
	toPath = agentCleanWorkspaceWritePath(toPath)
	if fromPath == "" || fromPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要复制的源文件路径，例如：复制文件 notes.md 到 archive/notes-copy.md。",
			"Please provide the source file path to copy, for example: copy file notes.md to archive/notes-copy.md.",
		), nil, nil)}, nil, nil
	}
	if toPath == "" || toPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要复制到的目标文件路径，例如：复制文件 notes.md 到 archive/notes-copy.md。",
			"Please provide the destination file path to copy to, for example: copy file notes.md to archive/notes-copy.md.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.copy", fromPath+" -> "+toPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	err := m.workspace.CopyFile(sessionID, fromPath, toPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id": sessionID,
		"from":       fromPath,
		"path":       toPath,
		"status":     "copied",
	}

	message := m.message(language, "tool", agentFormatWorkspaceCopy(language, fromPath, toPath), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceMkdir(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要创建的工作空间目录路径，例如：创建目录 reports。",
			"Please provide the workspace directory path to create, for example: mkdir reports.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.mkdir", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	err := m.workspace.MakeDir(sessionID, relPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id": sessionID,
		"path":       relPath,
		"status":     "created",
	}

	message := m.message(language, "tool", agentFormatWorkspaceMkdir(language, relPath), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) patchWorkspaceFile(sessionID, relPath, oldText, newText string) (map[string]any, error) {
	change, _, err := m.buildWorkspaceChange(sessionID, relPath, oldText, newText, "workspace.files.patch")
	if err != nil {
		return nil, err
	}
	if err := m.applyWorkspaceChangeToFile(sessionID, change); err != nil {
		return nil, err
	}
	return map[string]any{
		"session_id":      sessionID,
		"path":            change.Path,
		"size_before":     change.SizeBefore,
		"size_after":      change.SizeAfter,
		"replacements":    change.Replacements,
		"diff_preview":    change.DiffPreview,
		"content_preview": change.ContentPreview,
	}, nil
}

func (m *agentManager) buildWorkspaceChange(sessionID, relPath, oldText, newText, toolName string) (agentWorkspaceChange, string, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if relPath == "" || relPath == "." {
		return agentWorkspaceChange{}, "", services.NewError(services.CodeBadRequest, toolName+" requires path", map[string]any{"field": "path"}, nil)
	}
	if oldText == "" {
		return agentWorkspaceChange{}, "", services.NewError(services.CodeBadRequest, toolName+" requires old text", map[string]any{"field": "old"}, nil)
	}
	if _, err := m.workspace.Create(sessionID); err != nil {
		return agentWorkspaceChange{}, "", err
	}
	data, err := m.workspace.ReadFile(sessionID, relPath)
	if err != nil {
		return agentWorkspaceChange{}, "", err
	}
	before := string(data)
	replacements := strings.Count(before, oldText)
	switch {
	case replacements == 0:
		return agentWorkspaceChange{}, "", services.NewError(services.CodeConflict, "old text was not found in workspace file", map[string]any{"path": relPath}, nil)
	case replacements > 1:
		return agentWorkspaceChange{}, "", services.NewError(services.CodeConflict, "old text is ambiguous; provide a longer exact block", map[string]any{"path": relPath, "matches": replacements}, nil)
	}
	after := strings.Replace(before, oldText, newText, 1)
	now := time.Now().UTC()
	change := agentWorkspaceChange{
		ID:             agentWorkspaceChangeID(now, relPath),
		Path:           relPath,
		OldText:        oldText,
		NewText:        newText,
		SizeBefore:     len(before),
		SizeAfter:      len(after),
		Replacements:   1,
		DiffPreview:    agentDiffPreview(relPath, oldText, newText),
		ContentPreview: agentLimitBlock(after, 500),
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return change, after, nil
}

func (m *agentManager) prepareWorkspaceChangeApply(sessionID string, change agentWorkspaceChange) (preparedWorkspaceApply, error) {
	if change.Path == "" || change.OldText == "" {
		return preparedWorkspaceApply{}, services.NewError(services.CodeBadRequest, "workspace change is incomplete", map[string]any{"change_id": change.ID}, nil)
	}
	if _, err := m.workspace.Create(sessionID); err != nil {
		return preparedWorkspaceApply{}, err
	}
	data, err := m.workspace.ReadFile(sessionID, change.Path)
	if err != nil {
		return preparedWorkspaceApply{}, err
	}
	before := string(data)
	replacements := strings.Count(before, change.OldText)
	switch {
	case replacements == 0:
		return preparedWorkspaceApply{}, services.NewError(services.CodeConflict, "staged old text is no longer present in workspace file", map[string]any{"change_id": change.ID, "path": change.Path}, nil)
	case replacements > 1:
		return preparedWorkspaceApply{}, services.NewError(services.CodeConflict, "staged old text is now ambiguous; regenerate the diff", map[string]any{"change_id": change.ID, "path": change.Path, "matches": replacements}, nil)
	}
	after := strings.Replace(before, change.OldText, change.NewText, 1)
	return preparedWorkspaceApply{Change: change, Before: before, After: after}, nil
}

func (m *agentManager) applyWorkspaceChangeToFile(sessionID string, change agentWorkspaceChange) error {
	prepared, err := m.prepareWorkspaceChangeApply(sessionID, change)
	if err != nil {
		return err
	}
	return m.workspace.WriteFile(sessionID, prepared.Change.Path, []byte(prepared.After))
}

func agentWorkspaceChangeID(now time.Time, relPath string) string {
	clean := regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(relPath, "_")
	clean = strings.Trim(clean, "_")
	if clean == "" {
		clean = "file"
	}
	if len(clean) > 32 {
		clean = clean[:32]
	}
	return fmt.Sprintf("change_%s_%s", now.Format("20060102_150405_000000000"), clean)
}

func agentWorkspaceGroupID(now time.Time) string {
	return strings.Replace(runs.NewJobID(now), "job_", "changeset_", 1)
}

func (m *agentManager) stageWorkspaceFileChange(sessionID, relPath, oldText, newText, toolCallID string) (map[string]any, error) {
	change, _, err := m.buildWorkspaceChange(sessionID, relPath, oldText, newText, "workspace.files.diff")
	if err != nil {
		return nil, err
	}
	change.ToolCallID = toolCallID
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
	now := time.Now().UTC()
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].Status == "pending" && session.PendingChanges[idx].Path == change.Path {
			session.PendingChanges[idx].Status = "superseded"
			session.PendingChanges[idx].UpdatedAt = now
		}
	}
	session.PendingChanges = append([]agentWorkspaceChange{change}, session.PendingChanges...)
	if len(session.PendingChanges) > 20 {
		session.PendingChanges = session.PendingChanges[:20]
	}
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()
	return workspaceChangeResult(sessionID, change), nil
}

func (m *agentManager) stageWorkspaceFileChanges(sessionID string, inputs []agentWorkspaceChangeInput, toolCallID string) (map[string]any, error) {
	if len(inputs) == 0 {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.diff_many requires changes", map[string]any{"field": "changes"}, nil)
	}
	if len(inputs) > 20 {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.diff_many supports at most 20 changes", map[string]any{"field": "changes", "max": 20}, nil)
	}
	now := time.Now().UTC()
	groupID := agentWorkspaceGroupID(now)
	seenPaths := map[string]struct{}{}
	changes := make([]agentWorkspaceChange, 0, len(inputs))
	results := make([]map[string]any, 0, len(inputs))
	for idx, input := range inputs {
		change, _, err := m.buildWorkspaceChange(sessionID, input.Path, input.Old, input.New, "workspace.files.diff_many")
		if err != nil {
			return nil, err
		}
		if _, ok := seenPaths[change.Path]; ok {
			return nil, services.NewError(services.CodeConflict, "workspace.files.diff_many cannot include the same path twice", map[string]any{"path": change.Path, "index": idx}, nil)
		}
		seenPaths[change.Path] = struct{}{}
		change.ID = fmt.Sprintf("%s_%02d", change.ID, idx+1)
		change.GroupID = groupID
		change.ToolCallID = toolCallID
		changes = append(changes, change)
	}

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
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].Status != "pending" {
			continue
		}
		if _, ok := seenPaths[session.PendingChanges[idx].Path]; ok {
			session.PendingChanges[idx].Status = "superseded"
			session.PendingChanges[idx].UpdatedAt = now
		}
	}
	for _, change := range changes {
		results = append(results, workspaceChangeResult(sessionID, change))
	}
	session.PendingChanges = append(changes, session.PendingChanges...)
	if len(session.PendingChanges) > 50 {
		session.PendingChanges = session.PendingChanges[:50]
	}
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	return map[string]any{
		"session_id": sessionID,
		"group_id":   groupID,
		"count":      len(changes),
		"changes":    results,
		"status":     "pending",
	}, nil
}

func (m *agentManager) rebaseWorkspaceFileChange(sessionID, changeID, oldText, newText, toolCallID string) (map[string]any, error) {
	changeID = agentCleanToken(changeID)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if changeID == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.rebase requires change_id", map[string]any{"field": "change_id"}, nil)
	}
	if oldText == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.rebase requires current old text", map[string]any{"field": "old"}, nil)
	}

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
	var original agentWorkspaceChange
	found := false
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID {
			original = session.PendingChanges[idx]
			found = true
			break
		}
	}
	m.mu.Unlock()

	if !found {
		return nil, services.NewError(services.CodeNotFound, "workspace staged change not found", map[string]any{"change_id": changeID}, nil)
	}
	if original.Status != "pending" && original.Status != "conflict" {
		return nil, services.NewError(services.CodeConflict, "workspace staged change cannot be rebased", map[string]any{"change_id": changeID, "status": original.Status}, nil)
	}
	if newText == "" {
		newText = original.NewText
	}
	change, _, err := m.buildWorkspaceChange(sessionID, original.Path, oldText, newText, "workspace.files.rebase")
	if err != nil {
		return nil, err
	}
	change.GroupID = original.GroupID
	change.ToolCallID = toolCallID

	now := time.Now().UTC()
	m.mu.Lock()
	session, ok = m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID ||
			(session.PendingChanges[idx].Status == "pending" && session.PendingChanges[idx].Path == change.Path) {
			session.PendingChanges[idx].Status = "superseded"
			session.PendingChanges[idx].Error = ""
			session.PendingChanges[idx].UpdatedAt = now
		}
	}
	session.PendingChanges = append([]agentWorkspaceChange{change}, session.PendingChanges...)
	if len(session.PendingChanges) > 50 {
		session.PendingChanges = session.PendingChanges[:50]
	}
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	result := workspaceChangeResult(sessionID, change)
	result["rebased_from"] = changeID
	return result, nil
}

func (m *agentManager) applyWorkspaceFileChange(sessionID, changeID string) (map[string]any, error) {
	changeID = agentCleanToken(changeID)
	if changeID == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.apply requires change_id", map[string]any{"field": "change_id"}, nil)
	}

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
	m.mu.Unlock()

	if changeIndex < 0 {
		return nil, services.NewError(services.CodeNotFound, "workspace staged change not found", map[string]any{"change_id": changeID}, nil)
	}
	if change.Status != "pending" {
		return nil, services.NewError(services.CodeConflict, "workspace staged change is not pending", map[string]any{"change_id": changeID, "status": change.Status}, nil)
	}
	if err := m.applyWorkspaceChangeToFile(sessionID, change); err != nil {
		if services.ErrorCodeOf(err) == services.CodeConflict {
			m.markWorkspaceChangeConflict(sessionID, changeID, err)
		}
		return nil, err
	}

	now := time.Now().UTC()
	change.Status = "applied"
	change.UpdatedAt = now
	change.AppliedAt = &now
	m.mu.Lock()
	session, ok = m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID {
			session.PendingChanges[idx] = change
			break
		}
	}
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	result := workspaceChangeResult(sessionID, change)
	result["status"] = "applied"
	return result, nil
}

func (m *agentManager) markWorkspaceChangeConflict(sessionID, changeID string, cause error) {
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID != changeID {
			continue
		}
		if session.PendingChanges[idx].Status == "pending" {
			session.PendingChanges[idx].Status = "conflict"
			session.PendingChanges[idx].Error = cause.Error()
			session.PendingChanges[idx].UpdatedAt = now
			session.UpdatedAt = now
			_ = m.persistSessionLocked(session)
		}
		return
	}
}

func (m *agentManager) markWorkspaceChangesApplied(sessionID string, applied []agentWorkspaceChange) error {
	now := time.Now().UTC()
	byID := map[string]agentWorkspaceChange{}
	for _, change := range applied {
		change.Status = "applied"
		change.Error = ""
		change.UpdatedAt = now
		change.AppliedAt = &now
		byID[change.ID] = change
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return loadErr
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if change, ok := byID[session.PendingChanges[idx].ID]; ok {
			session.PendingChanges[idx] = change
		}
	}
	session.UpdatedAt = now
	return m.persistSessionLocked(session)
}
