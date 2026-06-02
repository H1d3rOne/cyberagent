package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/search"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func (m *agentManager) handleAgentScheduleCreate(sessionID, language, operator string, svc routerServices, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	if svc.schedules == nil {
		return nil, nil, services.NewError(services.CodeUnsupported, "scheduler not available", nil, nil)
	}
	tool := runningAgentToolCall("POST /api/v1/schedules", "from_agent")

	rawAction := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["action"]),
		stringFromAny(args["kind"]),
		stringFromAny(args["type"]),
	)))
	cron := strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["cron"]),
		stringFromAny(args["expression"]),
		stringFromAny(args["schedule"]),
	))
	if cron == "" {
		err := services.NewError(services.CodeBadRequest, "schedule.create requires cron expression", map[string]any{"action": "schedule.create"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	spec := scheduleSpec{
		Cron:      cron,
		Name:      strings.TrimSpace(stringFromAny(args["name"])),
		Operator:  firstNonEmpty(operator, "agent"),
		Language:  firstNonEmpty(stringFromAny(args["language"]), language),
		SessionID: strings.TrimSpace(firstNonEmpty(stringFromAny(args["session_id"]), sessionID)),
		Prompt:    strings.TrimSpace(stringFromAny(args["prompt"])),
		OpsLevel:  strings.TrimSpace(firstNonEmpty(stringFromAny(args["ops_level"]), stringFromAny(args["operation_level"]))),
		Target:    strings.TrimSpace(firstNonEmpty(stringFromAny(args["target"]), stringFromAny(args["url"]))),
		Scope:     strings.TrimSpace(stringFromAny(args["scope"])),
		Profile:   strings.TrimSpace(stringFromAny(args["profile"])),
	}
	if v, ok := args["enabled"].(bool); ok {
		spec.Enabled = v
	} else {
		spec.Enabled = true
	}

	switch rawAction {
	case "scan_web", "scan.web", "scan", "web_scan", "web.scan", "scanweb":
		spec.Action = scheduleActionAgentPrompt
		if spec.Prompt == "" {
			spec.Prompt = schedulePromptFromLegacyScanWeb(spec)
		}
	case "agent_prompt", "agent.prompt", "prompt", "chat", "ask", "agent", "":
		spec.Action = scheduleActionAgentPrompt
	case "ports.scan.safe", "port.scan", "port_scan", "ports_scan", "ports.scan":
		spec.Action = scheduleActionAgentPrompt
		if spec.Prompt == "" {
			target := strings.TrimSpace(firstNonEmpty(spec.Target, stringFromAny(args["host"]), stringFromAny(args["input"])))
			if target != "" {
				spec.Prompt = "扫描 " + target + " 开放端口"
				if portsSpec := agentPortSpecFromPlannerArgument(args["ports"]); portsSpec != "" {
					spec.Prompt += " ports=" + portsSpec
				}
			}
		}
	case "http.request", "web.search", "web.fetch", "workspace.command", "workspace.files.list", "workspace.files.tree", "workspace.files.read", "workspace.search", "reports.draft":
		spec.Action = scheduleActionAgentPrompt
	default:
		if spec.Prompt != "" {
			spec.Action = scheduleActionAgentPrompt
		} else {
			spec.Action = scheduleAction(rawAction)
		}
	}

	if spec.Action == scheduleActionAgentPrompt && spec.Prompt == "" {
		err := services.NewError(services.CodeBadRequest, "schedule.create requires prompt", map[string]any{"action": "schedule.create"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	if spec.Name == "" {
		spec.Name = agentText(language, "定时提示词", "scheduled prompt")
	}

	created, err := svc.schedules.Create(spec)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	next := ""
	if created.NextRunAt != nil {
		next = created.NextRunAt.Local().Format(time.RFC3339)
	}
	tool.Result = map[string]any{
		"id":          created.ID,
		"action":      string(created.Action),
		"cron":        created.Cron,
		"enabled":     created.Enabled,
		"ops_level":   created.OpsLevel,
		"next_run_at": next,
	}
	content := agentText(
		language,
		fmt.Sprintf("已创建定时任务 %s（%s，cron=%s）。下次触发：%s", created.Name, created.Action, created.Cron, firstNonEmpty(next, agentText(language, "未排定", "not scheduled"))),
		fmt.Sprintf("Created schedule %s (%s, cron=%s). Next fire: %s", created.Name, created.Action, created.Cron, firstNonEmpty(next, "not scheduled")),
	)
	links := []agentLink{{To: "/tasks", Label: agentText(language, "查看定时任务", "View schedules"), Kind: "schedules"}}
	return []agentMessage{m.message(language, "tool", content, []agentToolCall{tool}, links)}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentScheduleList(language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	if svc.schedules == nil {
		return nil, nil, services.NewError(services.CodeUnsupported, "scheduler not available", nil, nil)
	}
	tool := runningAgentToolCall("GET /api/v1/schedules", "")
	items := svc.schedules.List()
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{"count": len(items)}

	if len(items) == 0 {
		content := agentText(language, "当前没有定时任务。", "No schedules configured.")
		return []agentMessage{m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/tasks", Label: agentText(language, "任务页", "Tasks"), Kind: "schedules"}})}, []agentToolCall{tool}, nil
	}
	limit := len(items)
	if limit > 5 {
		limit = 5
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		s := items[i]
		next := "-"
		if s.NextRunAt != nil {
			next = s.NextRunAt.Local().Format(time.RFC3339)
		}
		flag := agentText(language, "启用", "on")
		if !s.Enabled {
			flag = agentText(language, "停用", "off")
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %s cron=%s next=%s (%s)", s.ID, s.Action, s.Name, s.Cron, next, flag))
	}
	more := ""
	if len(items) > limit {
		more = agentText(language, fmt.Sprintf(" 等共 %d 条。", len(items)), fmt.Sprintf(" of %d total.", len(items)))
	}
	content := agentText(language, "定时任务："+strings.Join(lines, "; ")+more, "Schedules: "+strings.Join(lines, "; ")+more)
	links := []agentLink{{To: "/tasks", Label: agentText(language, "管理定时任务", "Manage schedules"), Kind: "schedules"}}
	return []agentMessage{m.message(language, "tool", content, []agentToolCall{tool}, links)}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentScheduleDelete(language string, svc routerServices, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	if svc.schedules == nil {
		return nil, nil, services.NewError(services.CodeUnsupported, "scheduler not available", nil, nil)
	}
	id := strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["id"]),
		stringFromAny(args["schedule_id"]),
		stringFromAny(args["name"]),
	))
	tool := runningAgentToolCall("DELETE /api/v1/schedules/"+shortAgentID(id, 22), "")
	if id == "" {
		err := services.NewError(services.CodeBadRequest, "schedule.delete requires id", map[string]any{"action": "schedule.delete"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	err := svc.schedules.Delete(id)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{"id": id, "deleted": true}
	content := agentText(language, "已删除定时任务 "+id+"。", "Deleted schedule "+id+".")
	links := []agentLink{{To: "/tasks", Label: agentText(language, "任务页", "Tasks"), Kind: "schedules"}}
	return []agentMessage{m.message(language, "tool", content, []agentToolCall{tool}, links)}, []agentToolCall{tool}, nil
}

func agentLoadWebSearchConfig(svc routerServices) (search.Config, bool, error) {
	if svc.providers == nil {
		return search.Config{}, false, nil
	}
	cfg, err := svc.providers.Load()
	if err != nil {
		return search.Config{}, false, err
	}
	if cfg.WebSearch == nil {
		return search.Config{}, false, nil
	}
	return search.Config{
		Provider:   cfg.WebSearch.Provider,
		APIKey:     cfg.WebSearch.APIKey,
		BaseURL:    cfg.WebSearch.BaseURL,
		MaxResults: cfg.WebSearch.MaxResults,
	}, true, nil
}

func (m *agentManager) handleAgentWebSearch(ctx context.Context, language string, svc routerServices, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	query := strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["query"]),
		stringFromAny(args["q"]),
		stringFromAny(args["input"]),
	))
	tool := runningAgentToolCall("web.search", query)
	if query == "" {
		err := services.NewError(services.CodeBadRequest, "web.search requires query", map[string]any{"action": "web.search"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	cfg, ok, loadErr := agentLoadWebSearchConfig(svc)
	if loadErr != nil {
		tool = finishAgentToolCall(tool, loadErr)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(loadErr), Message: loadErr.Error()}
		return []agentMessage{m.message(language, "tool", loadErr.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	if !ok {
		err := services.NewError(services.CodeBadRequest, "web.search not configured; open Settings → Web Search and set a provider + api key", map[string]any{"action": "web.search"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	provider, err := search.NewFromConfig(cfg)
	if err != nil {
		if errors.Is(err, search.ErrNotConfigured) {
			err = services.NewError(services.CodeBadRequest, "web.search not configured; open Settings → Web Search and set a provider + api key", map[string]any{"action": "web.search"}, err)
		}
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	if cfg.BaseURL != "" {
		if parsed, parseErr := url.Parse(cfg.BaseURL); parseErr == nil && parsed != nil {
			if guardErr := agentURLOutboundAllowed(parsed); guardErr != nil {
				tool = finishAgentToolCall(tool, guardErr)
				tool.Error = &daemonJobError{Code: services.ErrorCodeOf(guardErr), Message: guardErr.Error()}
				return []agentMessage{m.message(language, "tool", guardErr.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
			}
		}
	}
	max := intFromAny(args["max_results"])
	if max <= 0 {
		max = cfg.MaxResults
	}
	if max <= 0 {
		max = 5
	}
	if max > 10 {
		max = 10
	}
	reqCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	result, err := provider.Search(reqCtx, search.Query{
		Query:      query,
		MaxResults: max,
		Locale:     stringFromAny(args["locale"]),
	})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"provider": result.Provider,
		"query":    result.Query,
		"items":    result.Items,
	}
	return []agentMessage{m.message(language, "tool", agentFormatWebSearch(language, result), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWebFetch(ctx context.Context, language string, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	rawURL := strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["url"]),
		stringFromAny(args["target"]),
		stringFromAny(args["input"]),
	))
	mode := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringFromAny(args["mode"]), "readable")))
	if mode != "readable" && mode != "raw" {
		mode = "readable"
	}
	maxChars := intFromAny(args["max_chars"])
	if maxChars <= 0 {
		maxChars = 8000
	}
	if maxChars > 20000 {
		maxChars = 20000
	}
	tool := runningAgentToolCall("web.fetch", rawURL)
	if rawURL == "" {
		err := services.NewError(services.CodeBadRequest, "web.fetch requires url", map[string]any{"action": "web.fetch"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	rawURL = agentNormalizeURLInput(rawURL)
	parsed, perr := url.Parse(rawURL)
	if perr != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		err := services.NewError(services.CodeBadRequest, "web.fetch requires an http/https url", map[string]any{"url": rawURL}, perr)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	if guardErr := agentURLOutboundAllowed(parsed); guardErr != nil {
		tool = finishAgentToolCall(tool, guardErr)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(guardErr), Message: guardErr.Error()}
		return []agentMessage{m.message(language, "tool", guardErr.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsed.String(), nil)
	if err == nil {
		req.Header.Set("User-Agent", "CyberAgent/agent-web-fetch")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain;q=0.9,*/*;q=0.8")
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			if req.URL != nil {
				if guardErr := agentURLOutboundAllowed(req.URL); guardErr != nil {
					return guardErr
				}
			}
			return nil
		},
	}
	var resp *http.Response
	if err == nil {
		resp, err = client.Do(req)
	}
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 1<<20+1)
	bodyBytes, readErr := io.ReadAll(limited)
	if readErr != nil {
		tool = finishAgentToolCall(tool, readErr)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(readErr), Message: readErr.Error()}
		return []agentMessage{m.message(language, "tool", readErr.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	contentType := resp.Header.Get("Content-Type")
	body := string(bodyBytes)
	title := ""
	content := body
	if mode == "readable" && strings.Contains(strings.ToLower(contentType), "html") {
		title, content = htmlToReadable(body)
	}
	truncated := false
	if len(content) > maxChars {
		content = content[:maxChars]
		truncated = true
	}
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"url":           parsed.String(),
		"mode":          mode,
		"status":        resp.Status,
		"status_code":   resp.StatusCode,
		"content_type":  contentType,
		"title":         title,
		"content":       content,
		"content_chars": len(content),
		"truncated":     truncated,
	}
	return []agentMessage{m.message(language, "tool", agentFormatWebFetch(language, parsed.String(), title, content, contentType, truncated), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
}
