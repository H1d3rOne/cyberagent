package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/command"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func (m *agentManager) handleAgentCommand(ctx context.Context, sessionID, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	commands := agentExtractCommands(prompt)
	if len(commands) == 0 {
		if cmd := agentExtractCommand(prompt); cmd != "" {
			commands = []string{cmd}
		}
	}
	if len(commands) == 0 {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"我识别到你要执行命令，但没有提取到具体命令。请使用：命令: ls -la 或 command: pwd。",
			"I detected a command intent, but no command was extracted. Use: command: ls -la or command: pwd.",
		), nil, nil)}, nil, nil
	}

	ws, err := m.workspace.Create(sessionID)
	if err != nil {
		tool := runningAgentToolCall("workspace.command", strings.Join(commands, " && "))
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	executor := command.NewExecutor(ws.Path, 0, 0)
	toolCalls := make([]agentToolCall, 0, len(commands))
	var firstResult *command.Result
	for idx, cmd := range commands {
		tool := runningAgentToolCall("workspace.command", cmd)
		result, execErr := executor.Execute(ctx, cmd, "")
		if execErr != nil {
			execErr = agentCommandServiceError(execErr, cmd, "")
		}
		tool = finishAgentToolCall(tool, execErr)
		if execErr != nil {
			tool.Error = &daemonJobError{Code: services.ErrorCodeOf(execErr), Message: execErr.Error()}
			tool.Result = map[string]any{
				"command":       cmd,
				"command_index": idx + 1,
				"command_count": len(commands),
				"workdir":       ws.Path,
			}
			toolCalls = append(toolCalls, tool)
			break
		}
		if firstResult == nil {
			firstResult = result
		}
		tool.Result = map[string]any{
			"command":       result.Command,
			"command_index": idx + 1,
			"command_count": len(commands),
			"exit_code":     result.ExitCode,
			"stdout":        result.Stdout,
			"stderr":        result.Stderr,
			"duration_ms":   result.DurationMs,
			"workdir":       result.WorkDir,
		}
		toolCalls = append(toolCalls, tool)
		if result.ExitCode != 0 {
			break
		}
	}

	if len(toolCalls) == 1 && len(commands) == 1 && toolCalls[0].Status == contracts.StatusSucceeded && firstResult != nil {
		content := agentFormatCommandResult(language, firstResult)
		message := m.message(language, "tool", content, toolCalls, nil)
		return []agentMessage{message}, toolCalls, nil
	}

	content := agentFormatCommandBatchResult(language, toolCalls)
	message := m.message(language, "tool", content, toolCalls, nil)
	return []agentMessage{message}, toolCalls, nil
}

func agentCommandServiceError(err error, cmd, workdir string) error {
	if err == nil {
		return nil
	}
	if code := services.ErrorCodeOf(err); code != services.CodeInternal {
		return err
	}
	message := err.Error()
	code := services.CodeInternal
	switch {
	case strings.Contains(message, "command not allowed"):
		code = services.CodePolicyDenied
	case strings.Contains(message, "empty command"),
		strings.Contains(message, "workdir"),
		strings.Contains(message, "argument escapes workspace"):
		code = services.CodeBadRequest
	}
	return services.NewError(code, message, map[string]any{
		"command": cmd,
		"workdir": workdir,
	}, nil)
}

func (m *agentManager) handleAgentWorkspace(ctx context.Context, sessionID, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	_ = ctx

	switch {
	case agentIsWorkspaceRebaseIntent(prompt):
		oldText, newText := agentExtractWorkspaceRebaseTexts(prompt)
		return m.handleAgentWorkspaceRebase(sessionID, agentExtractWorkspaceChangeID(prompt), oldText, newText, language)
	case agentIsWorkspaceDiscardAllIntent(prompt):
		return m.handleAgentWorkspaceDiscardAll(sessionID, language)
	case agentIsWorkspaceDiscardIntent(prompt):
		if groupID := agentExtractWorkspaceGroupID(prompt); groupID != "" {
			return m.handleAgentWorkspaceDiscardGroup(sessionID, groupID, language)
		}
		return m.handleAgentWorkspaceDiscard(sessionID, agentExtractWorkspaceChangeID(prompt), language)
	case agentIsWorkspaceApplyAllIntent(prompt):
		return m.handleAgentWorkspaceApplyAll(sessionID, language)
	case agentIsWorkspaceApplyIntent(prompt):
		if groupID := agentExtractWorkspaceGroupID(prompt); groupID != "" {
			return m.handleAgentWorkspaceApplyGroup(sessionID, groupID, language)
		}
		return m.handleAgentWorkspaceApply(sessionID, agentExtractWorkspaceChangeID(prompt), language)
	case agentIsWorkspaceDeleteIntent(prompt):
		return m.handleAgentWorkspaceDelete(sessionID, agentExtractPath(prompt), language)
	case agentIsWorkspaceMoveIntent(prompt):
		fromPath, toPath := agentExtractWorkspaceMovePaths(prompt)
		return m.handleAgentWorkspaceMove(sessionID, fromPath, toPath, language)
	case agentIsWorkspaceCopyIntent(prompt):
		fromPath, toPath := agentExtractWorkspaceMovePaths(prompt)
		return m.handleAgentWorkspaceCopy(sessionID, fromPath, toPath, language)
	case agentIsWorkspaceMkdirIntent(prompt):
		return m.handleAgentWorkspaceMkdir(sessionID, agentExtractWorkspaceMkdirPath(prompt), language)
	case agentIsWorkspaceDiffIntent(prompt):
		relPath, oldText, newText := agentExtractWorkspacePatch(prompt)
		return m.handleAgentWorkspaceDiff(sessionID, relPath, oldText, newText, language)
	case agentIsWorkspacePatchIntent(prompt):
		relPath, oldText, newText := agentExtractWorkspacePatch(prompt)
		return m.handleAgentWorkspacePatch(sessionID, relPath, oldText, newText, language)
	case agentIsWorkspaceWriteIntent(prompt):
		relPath, content := agentExtractWorkspaceWrite(prompt)
		return m.handleAgentWorkspaceWrite(sessionID, relPath, content, language)
	case agentIsWorkspaceSearchIntent(prompt):
		query := agentExtractWorkspaceSearchQuery(prompt)
		return m.handleAgentWorkspaceSearch(sessionID, query, agentExtractWorkspaceSearchPath(prompt, query), false, false, 80, language)
	case agentIsWorkspaceTreeIntent(prompt):
		return m.handleAgentWorkspaceTree(sessionID, agentExtractPath(prompt), 3, 120, false, language)
	case agentIsWorkspaceReadIntent(prompt):
		return m.handleAgentWorkspaceRead(sessionID, prompt, language)
	case agentIsWorkspaceListIntent(prompt):
		return m.handleAgentWorkspaceList(sessionID, prompt, language)
	default:
		return m.handleAgentWorkspaceReady(sessionID, language)
	}
}

func (m *agentManager) handleAgentWorkspaceReady(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("workspace.prepare", sessionID)
	ws, err := m.workspace.Create(sessionID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	tool.Result = map[string]any{"session_id": sessionID, "path": ws.Path}
	content := agentText(language,
		fmt.Sprintf("工作空间已准备就绪。\n\n- session: %s\n- path: %s\n\n可直接让我：列出文件、读取文件、执行安全允许列表内的命令。", sessionID, ws.Path),
		fmt.Sprintf("Workspace is ready.\n\n- session: %s\n- path: %s\n\nYou can ask me to list files, read files, or execute allowlisted commands.", sessionID, ws.Path),
	)
	message := m.message(language, "tool", content, []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceList(sessionID, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	relPath := agentExtractPath(prompt)
	tool := runningAgentToolCall("workspace.files.list", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	files, err := m.workspace.ListFiles(sessionID, relPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{"session_id": sessionID, "path": relPath, "count": len(files), "files": files}

	content := agentFormatWorkspaceList(language, relPath, files)
	message := m.message(language, "tool", content, []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceRead(sessionID, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	relPath := agentExtractPath(prompt)
	if relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要读取的工作空间文件路径，例如：读取文件 notes.md。",
			"Please provide the workspace file path to read, for example: read file notes.md.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.read", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	data, err := m.workspace.ReadFile(sessionID, relPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	content := string(data)
	tool.Result = map[string]any{"session_id": sessionID, "path": relPath, "size": len(data), "content": content}

	message := m.message(language, "tool", agentFormatWorkspaceRead(language, relPath, content), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

type agentWorkspaceTreeEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
	Depth int    `json:"depth"`
}

type agentWorkspaceSearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func (m *agentManager) handleAgentWorkspaceTree(sessionID, relPath string, depth, limit int, includeHidden bool, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = firstNonEmpty(relPath, ".")
	if depth <= 0 {
		depth = 3
	}
	if depth > 8 {
		depth = 8
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 500 {
		limit = 500
	}

	tool := runningAgentToolCall("workspace.files.tree", relPath)
	ws, err := m.workspace.Create(sessionID)
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	root, cleanRel, err := agentWorkspaceResolvePath(ws.Path, relPath)
	var entries []agentWorkspaceTreeEntry
	truncated := false
	if err == nil {
		stopWalk := errors.New("agent workspace tree limit reached")
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			name := d.Name()
			if !includeHidden && strings.HasPrefix(name, ".") && path != root {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			entryDepth := 0
			if rel != "." {
				entryDepth = len(strings.Split(rel, "/"))
			}
			if entryDepth > depth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if rel != "." {
				info, _ := d.Info()
				size := int64(0)
				if info != nil && !d.IsDir() {
					size = info.Size()
				}
				p := rel
				if cleanRel != "." {
					p = filepath.ToSlash(filepath.Join(cleanRel, rel))
				}
				entries = append(entries, agentWorkspaceTreeEntry{Path: p, IsDir: d.IsDir(), Size: size, Depth: entryDepth})
				if len(entries) >= limit {
					truncated = true
					return stopWalk
				}
			}
			if d.IsDir() && entryDepth >= depth {
				return filepath.SkipDir
			}
			return nil
		})
		if errors.Is(err, stopWalk) {
			err = nil
		}
	}

	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id":      sessionID,
		"path":            cleanRel,
		"depth":           depth,
		"limit":           limit,
		"include_hidden":  includeHidden,
		"count":           len(entries),
		"entries":         entries,
		"truncated":       truncated,
		"workspace_path":  ws.Path,
		"workspace_scope": "agent_workspace",
	}
	message := m.message(language, "tool", agentFormatWorkspaceTree(language, cleanRel, entries, truncated), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceSearch(sessionID, query, relPath string, regexMode, caseSensitive bool, limit int, language string) ([]agentMessage, []agentToolCall, error) {
	query = strings.TrimSpace(query)
	relPath = firstNonEmpty(relPath, ".")
	if limit <= 0 {
		limit = 80
	}
	if limit > 300 {
		limit = 300
	}

	tool := runningAgentToolCall("workspace.search", query)
	ws, err := m.workspace.Create(sessionID)
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	root, cleanRel, err := agentWorkspaceResolvePath(ws.Path, relPath)
	var matcher *regexp.Regexp
	if err == nil && regexMode {
		pattern := query
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		matcher, err = regexp.Compile(pattern)
	}

	matches := make([]agentWorkspaceSearchMatch, 0, minInt(limit, 64))
	filesScanned := 0
	truncated := false
	if err == nil {
		stopWalk := errors.New("agent workspace search limit reached")
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				if d != nil && d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			info, infoErr := d.Info()
			if infoErr != nil || info.Size() > 1024*1024 {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || agentLooksBinary(data) {
				return nil
			}
			filesScanned++
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			for idx, line := range lines {
				if agentLineMatches(line, query, matcher, regexMode, caseSensitive) {
					rel, relErr := filepath.Rel(root, path)
					if relErr != nil {
						continue
					}
					rel = filepath.ToSlash(rel)
					if cleanRel != "." {
						rel = filepath.ToSlash(filepath.Join(cleanRel, rel))
					}
					matches = append(matches, agentWorkspaceSearchMatch{
						Path: rel,
						Line: idx + 1,
						Text: agentLimitBlock(strings.TrimSpace(line), 500),
					})
					if len(matches) >= limit {
						truncated = true
						return stopWalk
					}
				}
			}
			return nil
		})
		if errors.Is(err, stopWalk) {
			err = nil
		}
	}

	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id":     sessionID,
		"path":           cleanRel,
		"query":          query,
		"regex":          regexMode,
		"case_sensitive": caseSensitive,
		"limit":          limit,
		"files_scanned":  filesScanned,
		"count":          len(matches),
		"matches":        matches,
		"truncated":      truncated,
	}
	message := m.message(language, "tool", agentFormatWorkspaceSearch(language, query, matches, filesScanned, truncated), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentHTTPRequest(ctx context.Context, method, targetURL string, requestHeaders map[string]string, requestBodyText string, language string, authProfile *agentHTTPAuthProfileUsage) ([]agentMessage, []agentToolCall, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nil, nil, services.NewError(
			services.CodePolicyDenied,
			"http.request allows only GET, HEAD, OPTIONS, POST, PUT, PATCH, or DELETE",
			map[string]any{"method": method},
			nil,
		)
	}
	if agentHTTPMethodIsSafe(method) && strings.TrimSpace(requestBodyText) != "" {
		return nil, nil, services.NewError(
			services.CodePolicyDenied,
			"http.request body is only allowed with POST, PUT, PATCH, or DELETE",
			map[string]any{"method": method},
			nil,
		)
	}
	if len(requestBodyText) > 1024*1024 {
		return nil, nil, services.NewError(services.CodeBadRequest, "http.request body is too large", map[string]any{"max_bytes": 1024 * 1024}, nil)
	}
	targetURL = agentNormalizeURLInput(targetURL)
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, nil, services.NewError(services.CodeBadRequest, "http.request requires an http/https url", map[string]any{"url": targetURL}, err)
	}
	if err := agentURLOutboundAllowed(parsed); err != nil {
		return nil, nil, err
	}

	tool := runningAgentToolCall("http.request", method+" "+parsed.String())
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var requestBody io.Reader
	if strings.TrimSpace(requestBodyText) != "" {
		requestBody = strings.NewReader(requestBodyText)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, parsed.String(), requestBody)
	if err == nil {
		req.Header.Set("User-Agent", "CyberAgent/agent-http-tool")
		req.Header.Set("Accept", "*/*")
		for key, value := range requestHeaders {
			req.Header.Set(key, value)
		}
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	var result map[string]any
	if err == nil {
		var resp *http.Response
		resp, err = client.Do(req)
		if resp != nil {
			defer resp.Body.Close()
			limited := io.LimitReader(resp.Body, 64*1024+1)
			body, readErr := io.ReadAll(limited)
			if readErr != nil && err == nil {
				err = readErr
			}
			truncated := len(body) > 64*1024
			if truncated {
				body = body[:64*1024]
			}
			headers := map[string]string{}
			for key, values := range resp.Header {
				headers[key] = strings.Join(values, ", ")
			}
			result = map[string]any{
				"method":             method,
				"url":                parsed.String(),
				"status":             resp.Status,
				"status_code":        resp.StatusCode,
				"headers":            headers,
				"request_headers":    agentRedactHTTPHeaders(requestHeaders),
				"request_body_bytes": len(requestBodyText),
				"body":               string(body),
				"body_bytes":         len(body),
				"body_truncated":     truncated,
			}
			if authProfile != nil && authProfile.Applied {
				result["auth_profile"] = authProfile
			}
		}
	}
	tool = finishAgentToolCall(tool, err)
	if result != nil {
		tool.Result = result
	}
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	message := m.message(language, "tool", agentFormatHTTPRequest(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func agentWorkspaceResolvePath(wsPath, relPath string) (string, string, error) {
	cleanRel := filepath.Clean(strings.TrimSpace(relPath))
	if cleanRel == "" || cleanRel == string(filepath.Separator) {
		cleanRel = "."
	}
	if filepath.IsAbs(cleanRel) {
		return "", cleanRel, fmt.Errorf("absolute workspace paths are not allowed")
	}
	wsClean := filepath.Clean(wsPath)
	absPath := filepath.Clean(filepath.Join(wsClean, cleanRel))
	if absPath != wsClean && !strings.HasPrefix(absPath, wsClean+string(os.PathSeparator)) {
		return "", cleanRel, fmt.Errorf("path traversal not allowed")
	}
	return absPath, filepath.ToSlash(cleanRel), nil
}

func agentLooksBinary(data []byte) bool {
	limit := minInt(len(data), 8000)
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func agentLineMatches(line, query string, matcher *regexp.Regexp, regexMode, caseSensitive bool) bool {
	if regexMode {
		return matcher != nil && matcher.MatchString(line)
	}
	if caseSensitive {
		return strings.Contains(line, query)
	}
	return strings.Contains(strings.ToLower(line), strings.ToLower(query))
}
